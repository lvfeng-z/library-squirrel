# Bilibili 插件开发计划(薄消费者版)

> 依赖主程序平台能力,详见 `主程序多轨资源与多流任务重构方案.md`(下称【平台方案】)。
> 平台完成后,Bilibili 插件变薄:**下载/续传/备份/合成全部由主程序统一处理**,插件只负责"声明轨道、提供可续传 reader、扫码登录"。

## 一、范围与依赖关系

| 维度 | 决策 |
|---|---|
| 内容类型 | 投稿视频(BV/av)+ 图文动态(相册/opus)+ 专栏文章(cv) |
| 视频 | **多轨**:声明 `videoTrack`+`audioTrack` 两条流交给主程序下载(每条都是普通可续传 HTTP 文件 → 边下边落/续传/重启续传 全部由平台白送) |
| 认证 | OpenWindow 扫码登录,抓取 Cookie 加密存储(见第六节) |
| 合并 | **不在插件内**。轨道下载完成后,用户在作品上触发主程序的「合并」动作产出可播放单文件 |
| 多 P 视频 | 父任务(作品集 bvid)+ 每分 P 一个子任务,每 P 是一个多轨 Resource |
| 图文/专栏 | 单流(每张图一个 Resource,沿用 pixiv 多页模式);正文首版仅图片入库 |

**前置依赖**:本插件需【平台方案】阶段 0–4 完成(改造后的多轨 `TaskHandler` 接口 + 多流任务 + statusText + 合并动作)。开发顺序:平台先行,插件在其上消费。

## 二、为何变薄(对比旧版)

旧版需在插件内实现 ffmpeg 流式合并、两阶段 reader、确定性/skip-N 续传等重逻辑。依托平台多轨能力后:

- 视频下载 = 声明两条 `TrackStream`(video/audio),各用 `RetryReadCloser` 包一个 DASH URL。**和 pixiv 下单张图片等价**。
- 续传/重启/暂停恢复 = 平台按 Resource 聚合处理,插件只需实现 `Resume`(按各轨偏移重建 reader)。
- 合成 = 主程序「合并」动作,插件完全不碰 ffmpeg。

## 三、目录结构

```
library-squirrel-plugin-bilibili/   (module: github.com/lvfeng/library-squirrel-plugin-bilibili)
├── main.go                  # Serve(handler, WithBrowser, WithActivate, WithShutdown)
├── activate.go              # 注册站点/TaskHandler/SiteBrowser/URL监听 + 注入依赖
├── shutdown.go              # 清理活跃 reader
├── plugin.json              # 清单(见第七节)
├── go.mod                   # require SDK(新版,多轨 TaskHandler) + replace ../library-squirrel-sdk + go-qrcode
├── build.ps1
├── browser_util.go          # openURL(复制 pixiv)
├── site_browser.go          # SiteBrowser 存根,打开 bilibili
├── credential_manager.go    # Cookie 加载/保存/校验/触发扫码登录(加密存储)
├── bilibili_task_handler.go # 实现多轨 TaskHandler;按 ContentType 分派
├── internal/
│   ├── model/
│   │   ├── task_plugin_data.go  # TaskPluginData(带 Type 字段)+ 各类型字段
│   │   └── credentials.go       # Cookie 模型
│   ├── urlparser/
│   │   └── parser.go        # 解析 video/dynamic/article URL + 短链跟随;URL 监听正则
│   ├── bilibiliapi/
│   │   ├── client.go        # 带 CookieJar + UA 的 http.Client
│   │   ├── wbi.go           # WBI 签名 + nav 密钥缓存
│   │   ├── urls.go          # API URL 常量与构造
│   │   ├── video.go         # 视频详情 + DASH playurl 解析 + 标签
│   │   ├── dynamic.go       # 图文动态图片列表
│   │   ├── article.go       # 专栏图片 + 正文
│   │   ├── user.go          # UP 主信息
│   │   ├── login.go         # 扫码 generate/poll(poll 响应头取 cookie)
│   │   └── models.go        # API 响应结构体
│   └── download/
│       └── retry_reader.go  # 复制 pixiv 的可重试/续传 reader
├── assets/
│   ├── siteBrowserIcon.png
│   └── login.html           # 扫码登录展示页(OpenWindow 加载)
└── views/                   # login 相关静态资源
```

> 无 ffmpeg、无 merge.go、无复杂合成 reader。`retry_reader.go`/`browser_util.go`/`site_browser.go` 照搬 pixiv。

## 四、核心数据流(按 TaskHandler 方法 × 内容类型)

`TaskPluginData` 顶层带 `Type`(`"video"`/`"dynamic"`/`"article"`)。

### Create(url)
1. `urlparser` 解析 → 类型 + ID(b23.tv 短链先 HEAD 跟随重定向)。
2. **video**:view + playurl 取标题/简介/封面/UP主/标签/分P;每分 P 的 PluginData 存该 P 的 `cid`、目标画质的**视频流 URL**与**音频流 URL**、缩略图 URL。多 P → 父任务(作品集 bvid)+ 每分 P 子任务。
3. **dynamic/article**:取图片 URL 列表,父任务 + 每图一个子任务(单流)。

### Start(task) → []*TrackStream(多轨核心)
- **video**:返回两条 `TrackStream`:
  - `{Role:"videoTrack", ReadCloser: RetryReadCloser(视频流URL), Format, Size, Continuable:true}`
  - `{Role:"audioTrack", ReadCloser: RetryReadCloser(音频流URL), Format, Size, Continuable:true}`
  - 共享 `WorkResponse`(标题/UP主/标签/封面缩略图)。
- **dynamic/article**:单元素 `[]*TrackStream`(`{Role:"main", ...}`)。

### Resume(param) → []*TrackStream
- 按 `param.StreamOffsets[role]`(`TaskResumeParam`)为每条未完成轨重建 reader(各轨从自身偏移 Range 续传);已完成轨不返回(由平台跳过)。

### Pause / Stop(task 级,插件内部广播)
- 插件维护活跃 reader 表(对齐 pixiv `activeReaders`);Pause/Stop 广播到该任务全部 reader。**用户/平台只见任务级一个操作**。

### GetThumbnail(taskData)
- 封面/首图下载返回 jpg。

> 合成:插件不参与。视频作品下载完 videoTrack+audioTrack 后,平台标记 `mergeable`,用户触发主程序「合并」。

## 五、Bilibili API 调研要点

| 用途 | 接口 | 备注 |
|---|---|---|
| WBI 密钥 | `GET /x/web-interface/nav` | 取 `wbi_img.img_url`/`sub_url`,缓存 ~30min |
| 视频详情 | `GET /x/web-interface/wbi/view` | WBI 签名;标题/简介/封面/UP主/pages[]/pubdate |
| 视频流 | `GET /x/player/wbi/playurl` | WBI 签名;`fnval=4048` 取 DASH;登录态拿高画质 |
| 视频标签 | `GET /x/tag/archive/tags` | tid→tag |
| UP 主 | `GET /x/space/wbi/acc/info` | WBI 签名 |
| 图文动态 | `GET /x/polymer/web-dynamic/v1/detail` 或解析 opus 页 | 图片列表 |
| 专栏 | `GET /x/article/viewinfo` + 解析 cv 页 `__INITIAL_STATE__` | 图片 + 正文 |
| 扫码生成 | `GET /x/passport-login/web/qrcode/generate` | `{url, qrcode_key}` |
| 扫码轮询 | `GET /x/passport-login/web/qrcode/poll` | `data.code==0` 成功,`Set-Cookie` 含登录态 |

**WBI 签名**:img_key+sub_key(URL 文件名)→ 打乱表生成 mixin_key → 参数加 `wts` → key 排序 → URL 编码拼接 → 末尾 `&w_rid=<MD5(mixin_key)>`。

## 六、扫码登录(关键约束已确认)

**约束**:Bilibili `SESSDATA` 是 **HttpOnly**,SDK 的 OpenWindow(WebView2)只能 `ExecuteScript`,**读不到 HttpOnly cookie**。故"弹窗内登录→JS 读 cookie"不可行。

**采用方案**:Web 扫码官方 API + OpenWindow 承载二维码页:
1. 插件调 `generate` 拿 `{url, qrcode_key}`,用 `github.com/skip2/go-qrcode` 渲染二维码 PNG 到 `temp/`。
2. OpenWindow 加载插件静态页 `login.html`(展示二维码)。
3. 后台 goroutine 轮询 `poll`,`data.code==0` 成功 → **响应头 `Set-Cookie` 含 SESSDATA/bili_jct/DedeUserID**(Go CookieJar 捕获)→ `SetValueEncrypted` 加密存储。
4. `ExecuteScript` 在页面提示"登录成功"并 `Close`。

### settings 声明(plugin.json)
- `autoLogin`(boolean,默认 true):下载前是否自动确保登录态。
- `videoQuality`(select,默认 `80`/1080P):目标画质 `qn`。
- `preferCodec`(select,默认 `avc`):流编码优先级 `avc/hevc/av1`。

## 七、plugin.json(草案)

```jsonc
{
  "id": "com.lvfeng.bilibiliSuite_<guid>",
  "name": "bilibiliSuite",
  "version": "1.0.0",
  "author": "lvfeng",
  "description": "Library Squirrel 的 Bilibili 套件:下载投稿视频(多轨)、图文动态、专栏",
  "entryFile": "bilibili_plugin.exe",
  "activation": {"type": 1},
  "extensions": {
    "taskHandlers": [{"id": "main"}],
    "siteBrowsers": [{"id": "main"}],
    "slots": [
      {"id": "bilibili-site-browser-list", "name": "Bilibili",
       "slotType": "siteBrowserList",
       "content": {"icon": "assets/siteBrowserIcon.png", "extensionId": "main"}}
    ],
    "staticResources": {"directories": ["assets/", "views/"]},
    "settings": [
      {"key": "autoLogin", "type": "boolean", "title": "下载前自动登录", "group": "下载", "order": 1, "default": "true"},
      {"key": "videoQuality", "type": "select", "title": "视频画质", "group": "下载", "order": 2, "default": "80",
       "options": [
         {"label": "360P", "value": "16"}, {"label": "480P", "value": "32"},
         {"label": "720P", "value": "64"}, {"label": "1080P", "value": "80"},
         {"label": "1080P+", "value": "112"}, {"label": "1080P 60帧", "value": "116"},
         {"label": "4K", "value": "120"}, {"label": "HDR", "value": "125"}
       ]},
      {"key": "preferCodec", "type": "select", "title": "优先编码", "group": "下载", "order": 3, "default": "avc",
       "options": [{"label": "H.264/AVC", "value": "avc"}, {"label": "H.265/HEVC", "value": "hevc"}, {"label": "AV1", "value": "av1"}]}
    ]
  }
}
```

## 八、build.ps1
按 dev-guide 第十一节:`go build` → 复制 `plugin.json`/`bilibili_plugin.exe`/`assets/`/`views/` 到 `dist/` → 压缩(抑制 PowerShell 进度 bug)。无需打包 ffmpeg(合并在主程序侧)。

## 九、开发阶段(平台完成后)
- **阶段 A · 脚手架**:目录、`go.mod`(replace SDK 新版)、`main.go`/`activate.go`/`shutdown.go`/`site_browser.go`/`browser_util.go`;`plugin.json`;跑通加载、注册站点、URL 监听、入口卡片。
- **阶段 B · URL 解析 + 匿名视频详情**:`urlparser`、`client`/`wbi`、`video`(view+playurl DASH);`Create` 跑通,主程序能建任务、展示作品信息。
- **阶段 C · 多轨下载(核心)**:`Start`/`Resume` 返回 videoTrack+audioTrack 两条 `RetryReadCloser`;验证平台多流并发/暂停/恢复/重启续传;下载后作品标记 `mergeable`。
- **阶段 D · 扫码登录**:`credential_manager` + `login`(generate/轮询)+ `login.html` + OpenWindow;登录态注入 API,验证高画质。
- **阶段 E · 图文动态 + 专栏 + 多 P**:`dynamic`/`article` 单流分派;多 P 父子任务 + 作品集。
- **阶段 F · 收尾**:settings 接入、错误/日志中文化、`build.ps1` 验证、README。

## 十、验证清单
- [ ] 粘贴 BV/opus/cv 链接 → 建任务 → 作品信息正确(标题/UP主/标签/封面)。
- [ ] 视频下载得到 videoTrack+audioTrack 两个 store;暂停/恢复/重启续传正确(平台能力)。
- [ ] 下载完成后作品可被主程序「合并」(keep/overwrite)产出可外部播放的单文件。
- [ ] 多 P 视频正确拆分为多任务(作品集 bvid)。
- [ ] 扫码登录后可下载 1080P+;登录态加密存储、重启保持。
- [ ] 图文动态/专栏每张图分别落盘(单流)。
- [ ] 卸载插件无残留进程;`build.ps1` 产物安装后资源不 404。

## 十一、风险与开放问题
1. 强依赖【平台方案】阶段 0–4;平台未完成前插件无法验证多轨。
2. Bilibili API 非官方,可能随站点改版失效;执行中据实调整字段。
3. 专栏正文是否入库为文本(首版仅图片,正文待定)。
4. 高画质/番剧可能需大会员,首版不支持付费内容,遇受限画质降级到可用最高画质。
5. WBI 密钥缓存失效与风控(buvid3 等)需实现中观察调整。
