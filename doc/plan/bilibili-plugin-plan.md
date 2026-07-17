# Bilibili 插件开发计划(薄消费者版)

> 依赖主程序平台能力,详见 `主程序多轨资源与多流任务重构方案.md`(下称【平台方案】)。
> 平台完成后,Bilibili 插件变薄:**下载/续传/暂停/合并全部由主程序统一处理**,插件只负责"解析 URL、声明轨道、提供可续传 reader、扫码登录"。

> **修订说明(2026-07-15)**:本计划初稿写于 2026-06-26,其后 SDK 与主程序多轨基建持续演进,接口签名/类型命名已与初稿不一致。本次据现状核对(`proto/plugin.proto` + `dto/*.go` + 主程序 taskManager/resource)全面订正:接口对齐当前 SDK(`StoreSpec` 而非初稿的 `TrackStream`、无独立 `GetThumbnail`、Start/Resume 真实签名含 ctx/storeRoles/WorkResponse、补 `Generation` 字段)、补"与 pixiv 照搬陷阱"、更新前置依赖状态。读者以本版为准。

## 一、范围与依赖关系

| 维度 | 决策 |
|---|---|
| 内容类型 | 投稿视频(BV/av)+ 图文动态(相册/opus)+ 专栏文章(cv) |
| 视频 | **多轨**:声明 `videoTrack`+`audioTrack` 两条 `StoreSpec`(generation=downloaded,普通可续传 HTTP 文件 → 边下边落/续传/重启续传 全部由平台白送) |
| 认证 | OpenWindow 扫码登录,抓取 Cookie 加密存储(见第六节) |
| 合并 | **不在插件内**。轨道下载完成后,用户在作品上触发主程序的「合并」动作产出可播放单文件 |
| 多 P 视频 | 父任务(作品集 bvid)+ 每分 P 一个子任务,每 P 是一个多轨 Resource |
| 图文/专栏 | 单流(每张图一个 Resource,Role=`main`,沿用 pixiv 多页模式);正文首版仅图片入库 |

**前置依赖(据现状核对)**:本插件依赖【平台方案】阶段 0–5。现状——
- 阶段 0 数据模型 / 1 SDK 单接口 / 2 taskManager 多流 / 3 板块派生(runMode.storeRoles)/ 5 合并动作:**均已就绪**。
- 阶段 4 前端 `statusText`(资源级 phase):**未落地**(前端仅有静态状态码标签)。**非阻塞**——只影响任务列表状态描述丰富度,不影响多轨下载/续传/合并跑通;作主程序前端任务后续补,不进插件契约。

> 合并动作已就绪但**待 bilibili 产出含 video+audio 轨的数据实测**(见 K 节点);overwrite 产物挂 `merged` store（与 keep 同类型,仅多删原轨;语义已确认,注释/文档已对齐;打开优先级 merged>main 已落地）。

## 二、为何变薄(对比旧版)

旧版需在插件内实现 ffmpeg 流式合并、两阶段 reader、确定性/skip-N 续传等重逻辑。依托平台多轨能力后:

- 视频下载 = 声明两条 `StoreSpec`(video/audio),各用 `RetryReadCloser` 包一个 DASH URL。**和 pixiv 下单张图片等价**(多轨只是多 new 一个 reader)。
- 续传/重启/暂停恢复 = 平台按 Resource 聚合处理,插件只需实现 `Resume`(按各轨偏移重建 reader)。
- 合成 = 主程序「合并」动作,插件完全不碰 ffmpeg。
- reader 可续传能力(`RetryReadCloser`)**不在 SDK,需从 pixiv 插件 `internal/download/retry_reader.go` 整段照搬**(约 320 行,含 206 Content-Range 还原、暂停/恢复、ctx 切换、并发安全 Close)。

## 三、目录结构

```
library-squirrel-plugin-bilibili/   (module: github.com/lvfeng/library-squirrel-plugin-bilibili)
├── main.go                  # Serve(handler, WithBrowser, WithActivate, WithShutdown)
├── activate.go              # 注册站点/TaskHandler/SiteBrowser/URL监听 + 注入依赖(含 SetProxyConfig/transport)
├── shutdown.go              # 清理活跃 reader(多轨:遍历 taskID→map[role])
├── plugin.json              # 清单(见第七节)
├── go.mod                   # require SDK(新版多轨) + replace ../library-squirrel-sdk + go-qrcode
├── build.ps1
├── browser_util.go          # openURL(照搬 pixiv)
├── site_browser.go          # SiteBrowser 存根,打开 bilibili
├── credential_manager.go    # Cookie 加载/保存/校验/触发扫码登录(加密存储)
├── bilibili_task_handler.go # 实现多轨 TaskHandler;按 ContentType 分派
├── internal/
│   ├── model/
│   │   ├── task_plugin_data.go  # 插件自定义 PluginData JSON schema(含 Type 字段)+ 各类型字段
│   │   └── credentials.go       # Cookie 模型
│   ├── urlparser/
│   │   └── parser.go        # 解析 video/dynamic/article URL + 短链跟随;URL 监听正则
│   ├── bilibiliapi/
│   │   ├── client.go        # 带 CookieJar + UA 的 http.Client(复用 transport 工厂)
│   │   ├── transport.go     # NewBilibiliTransport + 三级代理(照搬 pixiv,见第二节陷阱)
│   │   ├── transport_windows.go / transport_other.go  # 注册表读系统代理(照搬 pixiv)
│   │   ├── wbi.go           # WBI 签名 + nav 密钥缓存
│   │   ├── urls.go          # API URL 常量与构造
│   │   ├── video.go         # 视频详情 + DASH playurl 解析 + 标签
│   │   ├── dynamic.go       # 图文动态图片列表
│   │   ├── article.go       # 专栏图片 + 正文
│   │   ├── user.go          # UP 主信息
│   │   ├── login.go         # 扫码 generate/poll(poll 响应头取 cookie)
│   │   └── models.go        # API 响应结构体
│   └── download/
│       └── retry_reader.go  # 照搬 pixiv 的可重试/续传 reader(整段复制)
├── assets/
│   ├── siteBrowserIcon.png
│   └── login.html           # 扫码登录展示页(OpenWindow 加载)
└── views/                   # login 相关静态资源
```

> 无 ffmpeg、无 merge.go、无复杂合成 reader。`retry_reader.go`/`browser_util.go`/`site_browser.go` 照搬 pixiv。module 名用 `github.com/lvfeng/library-squirrel-plugin-bilibili`(无 `-z`,与 pixiv/local 两个插件一致;GitHub org 名 `lvfeng-z` 带 `-z`,二者各为其名)。

## 四、核心数据流(按 TaskHandler 方法 × 内容类型)

> 接口签名以当前 SDK `dto/task_handler.go` 为准。TaskHandler 共 7 个方法:`Create` / `CreateWorkInfo` / `Start` / `Retry` / `Pause` / `Stop` / `Resume`——**已无 `GetThumbnail`**(缩略图改为 derived 轨,见下)。

`PluginData` 在协议层是**裸字符串**(`TaskDTO.PluginData *string`),主程序不解析;插件自定义 JSON schema,顶层带 `Type`(`"video"`/`"dynamic"`/`"article"`)区分内容类型,Create 时序列化写入、Start/Resume 时反序列化。

### Create(url) → *TaskCreateResult
1. `urlparser` 解析 → 类型 + ID(b23.tv 短链先 HEAD 跟随重定向)。
2. **video**:view 取标题/简介/封面/UP主/标签/分P。**Create 不调 playurl、不存流 URL**——只存该 P 的 `avid`/`cid`(稳定标识)+缩略图 URL;流 URL 由 Start/Resume 下载时重取(见下「DASH 流 URL 时效」)。多 P → 父任务(作品集 bvid)+ 每分 P 子任务。
3. **dynamic/article**:取图片 URL 列表,父任务 + 每图一个子任务(单流)。
4. **声明 `InvolvedRoles`**(创建期 universe,决定任务涉及哪些资源板块,前端据此展示可选板块):
   - video:`[StoreRoleVideoTrack, StoreRoleAudioTrack, StoreRoleThumbnail]`(封面作 derived 缩略图轨)。
   - dynamic/article:`[StoreRoleMain]`(沿用 pixiv)。

> **DASH 流 URL 时效(重要契约)**:bilibili DASH 流 URL(playurl 返回)带**签名时效**(约小时级,过期返 403),且可能与请求 IP 绑定。故 **Create 不存储 URL**;每次 **Start/Resume 用 `avid+cid+当前画质/编码` 重取 playurl** 拿最新流(同 IP 取同 IP 下)。pixiv 的 pximg URL 不过期故无需此机制——bilibili 特有。

### Start(ctx, task, storeRoles) → ([]*StoreSpec, *WorkResponse, error)(多轨核心)
> `storeRoles` 由平台按 `InvolvedRoles` 派生(首次全量)或 Redownload 时显式选择;空集表示全量。逐轨用 `wantsRole(storeRoles, role)` 判定是否产出。
- **video**:先用 `avid+cid+当前画质/编码` 重取 playurl DASH(见上「DASH 流 URL 时效」),再按 storeRoles 条件产出两条 `StoreSpec`(generation=downloaded):
  - `{Role: StoreRoleVideoTrack, ReadCloser: NewRetryReadCloser(视频流URL), Generation: "downloaded", Format:".mp4", Size, Continuable: ptr(true)}`
  - `{Role: StoreRoleAudioTrack, ReadCloser: NewRetryReadCloser(音频流URL), Generation: "downloaded", Format:".m4a", Size, Continuable: ptr(true)}`
  - 第二返回值 `*WorkResponse`:标题/UP主/标签/封面缩略图(跨轨共享,返回一次)。
- **dynamic/article**:单元素 `[]*StoreSpec`(`{Role: StoreRoleMain, Generation: "downloaded", ...}`)。
- **缩略图(无独立方法)**:封面作为 `{Role: StoreRoleThumbnail, Generation: "derived", ReadCloser: io.NopCloser(bytes), Format:"jpg", Continuable: ptr(false)}` 一次性派生轨并入流集合(由 `wantsRole(storeRoles, thumbnail)` 决定)。**`Format` 不带前导点**(`jpg`)——主程序 `buildThumbnailRelPath` 自拼 `_thumbnail.`+format,带点会产出 `_thumbnail..jpg` 触发 404(见 plugin-dev-guide.md Format 前导点契约)。

### Resume(ctx, param) → ([]*StoreSpec, *WorkResponse, error)
- 先用 `avid+cid+当前画质/编码` 重取 playurl 取最新流 URL(同 Start,见「DASH 流 URL 时效」),再遍历 `param.StreamOffsets`(`map[string]int64`,role → 该轨已写入字节数,仅未完成的 downloaded 轨出现):为每个未完成 role new 一个 reader、`SetValidBytes(offset)` 从偏移续传、返回对应 role 的 `StoreSpec`;已完成轨不出现在 map 中(平台跳过)。
- derived 轨未完成则整轨重产。

### Pause / Stop(param) —— 任务级,广播到该任务全部 reader
- 参数 `*TaskResParam`(含 task,无 role/stream 字段)。插件在内部维护活跃 reader 表(见下「照搬陷阱 1」),Pause/Stop 取出该任务**全部 role 的 reader** 逐个 `Pause()`/`Close()`。**用户/平台只见任务级一个操作**。

### Retry(task) / CreateWorkInfo(task)
- `Retry(task) → (*WorkResponse, error)`:重试场景返回作品信息。
- `CreateWorkInfo(task) → (*WorkResponse, error)`:补全作品信息。

> 合成:插件不参与。视频作品下载完 videoTrack+audioTrack 后,平台标记可合并,用户触发主程序「合并」(merged store,见第十节)。

### StoreSpec 字段速查(据 SDK `dto/handler_dto.go`)
| 字段 | 类型 | 说明 |
|---|---|---|
| `Role` | string | 轨标识(`main`/`videoTrack`/`audioTrack`/`thumbnail`/`merged` 等,开放枚举) |
| `Generation` | string | `downloaded`(可 Range 续传)/ `derived`(一次性派生)——**多轨核心字段** |
| `ReadCloser` | io.ReadCloser | reader 实例(json:"-" 不序列化) |
| `Format` | string | 文件扩展名 |
| `Size` | int64 | 完整资源大小(-1 未知;**非 Range 剩余字节**) |
| `SuggestName` | string | 插件建议文件名 |
| `Continuable` | *bool | 可续传(derived 恒 false) |
| `ResumeWriteOffset` | *int64 | 仅 Resume 用;nil=信任平台 stat,非 nil=插件指定续传写入位置 |

### 与 pixiv 照搬陷阱(不能无脑照的三处)
1. **活跃 reader 表必须从单值改多轨映射(正确性)**:pixiv 是 `activeReaders sync.Map` 的 `taskID → *RetryReadCloser`(单值,因一 task 只一轨 main)。bilibili 一视频 task 有 video+audio 两个 reader,直接照搬会让第二个 `Store` 覆盖第一个 → **Pause/Stop 只停一条轨,另一条泄漏**。必须改 `taskID → map[role]*RetryReadCloser`,Pause/Stop/Shutdown 遍历内层 map 所有 role。
2. **transport 工厂可整体照搬但要改头**:三级代理(显式 settings > Windows 注册表 > env)+ 连接复用 opt-in(connectionReuse 默认关)+ handler 级共享 Transport 这套照搬 pixiv(`NewBilibiliTransport(disableKeepAlives)`,双流共享连接池收益更大)。但下载请求的 `Referer`/鉴权头要换成 bilibili 的(pixiv 的 `Referer: pixiv.net` 是其防盗链特有)。
3. **单 task 多轨 ≠ pixiv 的多子任务**:pixiv"多页 = 多个独立 task(每张图一轨 main)";bilibili 是"一个 task 内含 video+audio 两轨"。Create 父子结构、InvolvedRoles、Start/Resume 流产出都要按多轨设计,**不能套 pixiv per-image 子任务模式**。

### reader 契约(强制,遵守 `doc/plugin-dev-guide.md:288-291`)
- 每次 Start/Resume **新建 reader**,不跨 RPC 复用(pull 模型下旧 serveSpecsPull goroutine 退出与命令切换不同步,复用 reader 致并发访问、数据错位)。
- `reader.Close` 必须能中断阻塞的 `Read`(供 Pause/Stop 用)。
- `Size` 必须是完整资源大小,非 Range 剩余字节。
- ctx 取消后立即停止发送(`RetryReadCloser.SetCtx` 跨 RPC 更新建连 ctx)。

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
2. OpenWindow 加载插件静态页 `login.html`(展示二维码)。`window.OpenWindow(options, ownerHWND)`(包级函数,非 PluginContext 方法;`ownerHWND` 取自 `ctx.GetMainWindowHandle()`,仅 Windows)。
3. 后台 goroutine 轮询 `poll`,`data.code==0` 成功 → **响应头 `Set-Cookie` 含 SESSDATA/bili_jct/DedeUserID**(Go CookieJar 捕获)→ `ctx.SetValueEncrypted(key, value)` 加密存储。
4. `ExecuteScript` 在页面提示"登录成功"并 `Close`。

### settings 声明(plugin.json)
- `autoLogin`(boolean,默认 true):下载前是否自动确保登录态。
- `videoQuality`(select,默认 `80`/1080P):目标画质 `qn`。
- `preferCodec`(select,默认 `avc`):流编码优先级 `avc/hevc/av1`。
- `proxyUrl`(string,可选):显式代理 URL(照搬 pixiv transport 三级代理的最高优先级)。
- `connectionReuse`(boolean,默认 false):是否启用 keep-alive 连接复用(照搬 pixiv,默认关保代理环境稳定)。

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
       "options": [{"label": "H.264/AVC", "value": "avc"}, {"label": "H.265/HEVC", "value": "hevc"}, {"label": "AV1", "value": "av1"}]},
      {"key": "proxyUrl", "type": "string", "title": "代理地址", "group": "网络", "order": 4},
      {"key": "connectionReuse", "type": "boolean", "title": "启用连接复用", "group": "网络", "order": 5, "default": "false"}
    ]
  }
}
```

## 八、build.ps1
按 dev-guide 第十一节:`go build` → 复制 `plugin.json`/`bilibili_plugin.exe`/`assets/`/`views/` 到 `dist/` → 压缩(抑制 PowerShell 进度 bug)。无需打包 ffmpeg(合并在主程序侧)。

## 九、开发阶段(平台就绪后)
- **阶段 A · 脚手架**:目录、`go.mod`(replace SDK 新版)、`main.go`/`activate.go`/`shutdown.go`/`site_browser.go`/`browser_util.go`;`plugin.json`;跑通加载、注册站点、URL 监听、入口卡片。
- **阶段 B · URL 解析 + 匿名视频详情**:`urlparser`、`client`/`wbi`、`video`(view+playurl DASH);`Create` 跑通(含 `InvolvedRoles` 声明 `[videoTrack,audioTrack]`),主程序能建任务、展示作品信息。
- **阶段 C · 多轨下载(核心)**:`Start`/`Resume` 返回 videoTrack+audioTrack 两条 `StoreSpec`;验证平台多流并发/暂停/恢复/重启续传。
  - **必须**:activeReaders 表改 `taskID→map[role]` 多轨映射(见第四节陷阱 1);transport 工厂改 Referer/鉴权头(陷阱 2);reader 契约每次新建(第四节 reader 契约);下载后作品可被「合并」。
- **阶段 D · 扫码登录**:`credential_manager` + `login`(generate/轮询)+ `login.html` + OpenWindow;登录态注入 API,验证高画质。
- **阶段 E · 图文动态 + 专栏 + 多 P**:`dynamic`/`article` 单流分派;多 P 父子任务 + 作品集。
- **阶段 F · 收尾**:settings 接入、错误/日志中文化、`build.ps1` 验证、README。

## 十、验证清单
- [ ] 粘贴 BV/opus/cv 链接 → 建任务 → 作品信息正确(标题/UP主/标签/封面)。
- [ ] 视频下载得到 videoTrack+audioTrack 两个 store;暂停/恢复/重启续传正确(平台能力);**暂停后两条轨都被正确停止**(验证 activeReaders 多轨改造)。
- [ ] 下载完成后作品可被主程序「合并」(keep/overwrite 均产 merged store,overwrite 额外删原轨;打开优先级 `merged>main`,外部播放取 merged)。
- [ ] 多 P 视频正确拆分为多任务(作品集 bvid)。
- [ ] 扫码登录后可下载 1080P+;登录态加密存储、重启保持。
- [ ] 图文动态/专栏每张图分别落盘(单流)。
- [ ] 卸载插件无残留进程;`build.ps1` 产物安装后资源不 404。

## 十一、风险与开放问题
1. 强依赖【平台方案】阶段 0–3/5(已就绪);阶段 4 statusText 未落地但非阻塞(见第一节)。
2. Bilibili API 非官方,可能随站点改版失效;执行中据实调整字段。
3. 专栏正文是否入库为文本(首版仅图片,正文待定)。
4. 高画质/番剧可能需大会员,首版不支持付费内容,遇受限画质降级到可用最高画质。
5. WBI 密钥缓存失效与风控(buvid3 等)需实现中观察调整。

## 十二、待修问题（2026-07-17 用户反馈；新会话续作 ①② 所需定位）

> 节点 A 现处 [当前]（不 close），阶段 A–F + K 合并均已验，但有 3 个待修问题。③ 已修待测，①② 待实施。

### ③ 暂停→恢复后进度条总数据量=0（已修，待测）
- **根因**：`Resume` 产出的 `StoreSpec` 漏 `Size`——`resumeVideo`/`resumeImage`（`download.go`）调 `reader.GetHeaders()` 却丢弃返回的 size，spec 未填 Size → 主程序算总量=0 → 进度条异常。pixiv 的 Resume 本就填 Size 故无此问题。
- **已修**：`resumeVideo`/`resumeImage` 改为 `_, size, err := reader.GetHeaders()` 并在 spec 补 `Size: size`。
- **测法**：下载视频→暂停→恢复，进度条总量应正常。

### ① 作者介绍（sign/introduce）缺失（待实施）
- **现状**：
  - 视频：`buildSiteAuthors`（`bilibili_task_handler.go`）调 `GetUserInfo`（`bilibiliapi/user.go`，`/x/space/wbi/acc/info` WBI 签名）→ **返 -403 风控**（space 接口限制更严，与 nav -101 同类）→ 回退基本字段（name+homepage），无 sign。
  - 图文/专栏：`createImage`（`bilibili_task_handler.go`）的 `SiteAuthors` 仅 name+homepage，**未取 sign**。
- **数据源候选**：
  - `acc/info`：含 sign，但 -403 风控。修风控需更全指纹（buvid_fp/w_webid 等，投机，同 -101/-352 类；当前 buvid3 不充分）。
  - opus 页 `module_author`：图文/专栏的 opus 页 `__INITIAL_STATE__.detail.modules[MODULE_TYPE_AUTHOR].module_author` **可能有 sign 字段**（待确认——之前 dump 在 avatar/layers 截断，未看到 sign）。视频非 opus 页形态，不适用。
  - 视频 view 响应 `data.owner`：仅 mid/name/face，**无 sign**。
- **方案建议**：图文/专栏先确认并取 opus `module_author.sign`（扩 `parseOpusState`/`OpusInitialState`）；视频 acc/info -403 需攻坚风控（buvid_fp）或暂接受缺失（非致命，已兜底 name）。
- **关键代码**：`bilibiliapi/user.go`（GetUserInfo）、`bilibili_task_handler.go`（buildSiteAuthors、createImage 的 SiteAuthors）、`bilibiliapi/dynamic.go`（parseOpusState 的 MODULE_TYPE_AUTHOR 目前只取 mid/name）、`model.SiteAuthorEntry.Introduce`（字段已存在）。

### ② 标签介绍（abstract/description）缺失（待实施，方案明确）
- **现状**：视频 `buildSiteTags`（`bilibili_task_handler.go`）调 `GetTags`（`bilibiliapi/video.go`，`/x/tag/archive/tags`）→ 只取 tag_id+tag_name，**无描述**；`model.SiteTagEntry.Description` 未填。
- **数据源**：`/x/tag/info?tag_id=X`（bilibili-API-collect）返回标签详情含 `data.news.archive_audience`/`data.description` 等，取描述字段。
- **方案**：`GetTags` 拿到 tag 列表后，逐个调 `/x/tag/info` 取描述填入 `SiteTagEntry.Description`（或合并到一次结构）。注意控制调用频次（每标签一次）。
- **关键代码**：`bilibiliapi/video.go`（GetTags）、`bilibili_task_handler.go`（buildSiteTags，目前只设 TagName）、`bilibiliapi/urls.go`（加 `TagInfoURL = BaseAPI+"/x/tag/info"`）、`model.SiteTagEntry.Description`（字段已存在）。
- **注**：图文/专栏（createImage）目前不取标签，本次范围仅视频。

