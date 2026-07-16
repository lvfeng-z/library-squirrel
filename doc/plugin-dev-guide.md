# Library Squirrel 插件开发指南

> 本文档是插件开发的权威指南，面向插件开发者。描述的能力与 `library-squirrel-sdk` 的 `dto.PluginContext` 接口严格一致。

## 一、概述

Library Squirrel 通过插件扩展支持的站点（pixiv、本地导入等）与 UI。插件是**独立可执行程序**（Windows 下 `.exe`），由主程序以**子进程**方式启动，通过 **gRPC** 通信。

主程序使用 [hashicorp/go-plugin](https://github.com/hashicorp/go-plugin) 库管理插件子进程的完整生命周期：进程启动、握手鉴权（MagicCookie）、gRPC 通道建立、崩溃检测、优雅关闭（先发 `Shutdown` RPC 等待插件清理，超时后强制终止）；Windows 下还通过 Job Object 将子进程归组，主进程异常退出时自动终止所有插件子进程，避免残留。插件侧调用 `pluginsdk.Serve(...)` 即接入这套机制（握手密钥与插件名由 SDK 固化，开发者无需关心）。

### 两种插件模式

| 模式 | 入口文件 | 子进程 | 适用 |
|---|---|---|---|
| **运行时插件** | 需要 `entryFile` | 需要 | 含 TaskHandler（下载任务）或 SiteBrowser（站点浏览）的插件 |
| **纯 UI 插件** | 不需要 | 不需要 | 仅提供 Slot 扩展（菜单、视图、弹窗、嵌入组件、入口卡片等） |

### 扩展点

| 扩展点 | 注册方式 | 说明 |
|---|---|---|
| TaskHandler | 运行时（代码注册） | 处理资源下载任务生命周期 |
| SiteBrowser | 运行时（代码注册） | 打开/关闭站点浏览器 |
| Slot: `view` | **声明式**（`plugin.json`） | 新增独立路由页面 |
| Slot: `replaceView` | **声明式** | 替换主程序已有页面（覆盖路由 component） |
| Slot: `embed` | **声明式** | 插入主程序具名插槽位 |
| Slot: `dialog` | **声明式** | 弹窗（模态层） |
| Slot: `menu` | **声明式** | 菜单项（跳转关联 view） |
| Slot: `siteBrowserList` | **声明式** | 站点浏览器入口卡片 |

### 插件依赖 SDK

```bash
go get github.com/lvfeng-z/library-squirrel-sdk
```

插件接口（`PluginContext`、`TaskHandler`、`SiteBrowser`、`WindowOptions` 等）定义在 SDK 的 `dto` 包。

## 二、快速开始

最小运行时插件（含一个 TaskHandler）：

```go
package main

import (
    pluginsdk "github.com/lvfeng-z/library-squirrel-sdk"
    sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

func main() {
    pluginsdk.Serve(&MyTaskHandler{},
        pluginsdk.WithActivate(func(ctx sdkdto.PluginContext) {
            ctx.RegisterTaskHandler("main", "我的任务处理器", "处理下载", &MyTaskHandler{})
            ctx.RegisterUrlListener("main", []string{`https?://example\.com/.*`})
        }),
    )
}

// MyTaskHandler 实现 sdkdto.TaskHandler 接口（见第六节）
type MyTaskHandler struct{}
// ... 实现 8 个方法
```

对应 `plugin.json`：

```json
{
  "id": "com.author.myPlugin_guid",
  "name": "myPlugin",
  "version": "1.0.0",
  "author": "author",
  "entryFile": "my_plugin.exe",
  "activation": {"type": 1},
  "extensions": {
    "taskHandlers": [{"id": "main"}],
    "staticResources": {"directories": ["assets/"]}
  }
}
```

> **关键**：插件入口是 `pluginsdk.Serve(handler, opts...)`，在 `main` 函数中调用。`WithActivate` 的回调签名是 `func(ctx sdkdto.PluginContext)`——这是 SDK 唯一约定的 Activate 形式。

## 三、plugin.json 清单

### 顶层字段

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `id` | string | 是 | 插件标识（建议 `com.作者.名称_guid`） |
| `name` | string | 是 | 显示名称 |
| `version` | string | 是 | 语义化版本 |
| `author` | string | 是 | 作者；**PublicID = `author/id`** |
| `description` | string | 否 | 描述 |
| `entryFile` | string | 条件必填 | 可执行文件名（运行时插件必填，纯 UI 插件不需要） |
| `activation.type` | number | 是 | `0`=手动激活，`1`=启动时自动激活 |
| `extensions` | object | 是 | 扩展点集合（见下） |

### extensions 字段

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `taskHandlers` | `[{id, name?, description?}]` | 三选一 | TaskHandler 声明（运行时注册） |
| `siteBrowsers` | `[{id, name?, description?}]` | 三选一 | SiteBrowser 声明（运行时注册） |
| `slots` | `[SlotDeclaration]` | 三选一 | UI 插槽（声明式注册，见 6.3） |
| `staticResources` | `{directories: string[]}` | 否 | 允许前端访问的资源目录白名单 |
| `settings` | `[SettingDeclaration]` | 否 | 用户可配置项（见 7.2） |

**校验规则**（安装时）：
- `id/name/version/author` 必填。
- `extensions` 必须存在，且 `taskHandlers/siteBrowsers/slots` 至少一个非空。
- `entryFile` 仅在含运行时扩展点（taskHandlers 或 siteBrowsers）时必填；纯 UI 插件可省略。
- 枚举值（slotType/contentType/position/settings.type）**安装时不校验**，错误值在激活/运行期暴露，请自行核对拼写。

### Slot 声明

```jsonc
{
  "id": "my-view",
  "name": "我的视图",
  "description": "可选描述",
  "slotType": "view",          // embed | view | replaceView | dialog | menu | siteBrowserList
  "order": 100,
  "content": { /* 按 slotType，见下 */ }
}
```

**content 按 slotType：**

| slotType | content 字段 | 说明 |
|---|---|---|
| `embed` | `{contentType, source, position, props?}` | 插入主程序具名插槽位（position = 插槽位标识，主程序定义） |
| `view` | `{contentType, source, title?, props?}` | 新增独立路由页面 |
| `replaceView` | `{contentType, source, target, props?}` | 替换主程序已有页面（target = 路由 name） |
| `dialog` | `{contentType, source, props?}` | 弹窗（模态层） |
| `menu` | `{icon?, viewId?, children?}` | 菜单项（点击跳转关联的 view slot；children 递归） |
| `siteBrowserList` | `{icon?, extensionId}` | 站点浏览器入口卡片（extensionId 必须等于某 siteBrowsers 的 id） |

**contentType 与 source 格式：**

| contentType | source 格式 | 说明 |
|---|---|---|
| `precompiled` | `{js, css?}` | 预编译 JS/CSS（推荐，性能最佳） |
| `vueSource` | `{vue, js?, css?}` | Vue SFC：`vue` 为源文件，`js/css` 为可选预编译缓存（存在则跳过运行时编译） |
| `code` | JS 代码字符串（行内） | 通过 `new Function` 执行，**不注入 Vue/WailsRuntime** |
| `html` | `{html}` | HTML 文件 |

> source 中的相对路径（`code` 类型除外）由后端自动转为 `http://wails.localhost:{port}/plugin/{author}/{id}/{version}/...` 完整 URL。

### settings 用户设置声明

```jsonc
"settings": [
  {
    "key": "downloadQuality",        // 必填，插件内唯一
    "type": "select",                // string | integer | boolean | select
    "title": "下载质量",              // 必填
    "description": "默认下载图片质量",
    "default": "original",           // 默认值（统一以 string 存储）
    "encrypted": false,              // true 时加密存储 + 前端密码框
    "group": "下载",                 // 分组（同组用分隔标题展示）
    "order": 1,                      // 组内排序
    "options": [                     // 仅 select 必填
      {"label": "原图", "value": "original"},
      {"label": "压缩", "value": "compressed"}
    ],
    "min": 0,                        // 仅 integer
    "max": 100                       // 仅 integer
  }
]
```

声明后，主程序在**插件管理页**自动渲染设置表单，用户编辑后存入插件自存信息；插件用 `ctx.GetValue(key)` 读取（见 7.2）。

## 四、插件入口

运行时插件的 `main` 函数调用 `pluginsdk.Serve`：

```go
func main() {
    pluginsdk.Serve(&MyTaskHandler{},
        pluginsdk.WithBrowser(&MySiteBrowser{}),       // 可选，注册 SiteBrowser
        pluginsdk.WithActivate(func(ctx sdkdto.PluginContext) {
            // 在此注册扩展点、添加站点、监听 URL 等
            ctx.AddSite([]*sdkdto.SiteDTO{...})
            ctx.RegisterTaskHandler("main", "名称", "描述", &MyTaskHandler{})
            ctx.RegisterSiteBrowser("main", "名称", "描述", &MySiteBrowser{})
            ctx.RegisterUrlListener("main", []string{`https?://example\.com/.*`})
        }),
        pluginsdk.WithShutdown(func() {
            // 可选，进程关闭前的清理
        }),
    )
}
```

- `Serve(handler, opts...)`：`handler`（TaskHandler）为必填位置参数。
- `WithActivate(fn)`：回调签名 `func(ctx sdkdto.PluginContext)`，主程序握手完成后调用。**这是注册扩展点的唯一时机**。
- `WithBrowser(browser)`：注册 SiteBrowser（也可在 Activate 内 `RegisterSiteBrowser`）。
- `WithShutdown(fn)`：进程关闭前回调（主程序 `UnloadPlugin` 时触发）。

> **依赖注入**：若你的 handler/browser 需要持有 `ctx` 或其他对象，用闭包捕获（参考 pixiv 的 `main.go`：先 new 出 handler/browser，再在 `WithActivate` 闭包里把 ctx 注入）。

## 五、PluginContext 完整 API

`ctx sdkdto.PluginContext` 是插件访问主程序能力的**唯一入口**，共 21 个方法：

| 分类 | 方法 | 签名 |
|---|---|---|
| 扩展点注册 | `RegisterTaskHandler` | `(id, name, desc string, handler TaskHandler) error` |
| | `RegisterSiteBrowser` | `(id, name, desc string, browser SiteBrowser) error` |
| | `UnregisterSiteBrowser` | `(id string) error` |
| 自存信息 | `GetValue` | `(key string) (string, error)` |
| | `SetValue` | `(key, value string) error` |
| | `SetValueEncrypted` | `(key, value string) error` |
| | `DeleteValue` | `(key string) error` |
| | `GetAllValues` | `() (map[string]string, error)` |
| 业务查询 | `GetWorkSetBySiteWorkSetId` | `(siteWorkSetId, siteName string) (*WorkSetDTO, error)` |
| | `AddSite` | `(sites []*SiteDTO) error` |
| 任务 | `RegisterUrlListener` | `(extensionId string, patterns []string) error` |
| | `UnregisterUrlListener` | `(extensionId string) error`（空则清该插件全部监听） |
| | `CreateTask` | `(url string) (*CreateTaskResult, error)` |
| 前端通信 | `PublishToFrontend` | `(topic string, data []byte) error` |
| | `SubscribeFrontend` | `(topic string) (<-chan []byte, error)` |
| | `UnsubscribeFrontend` | `(topic string) error` |
| 路径 | `GetPluginRoot` | `(isRelative bool) string` |
| 窗口 | `GetMainWindowHandle` | `() uintptr` |
| 日志 | `Infof` / `Debugf` / `Warnf` / `Errorf` | `(template string, args ...any)` |
| | `GetLogger` | `() Logger`（可 `Named(...)` 派生子 logger） |

## 六、扩展点

### 6.1 TaskHandler(运行时)

处理资源下载任务完整生命周期,实现 7 个方法:

```go
type TaskHandler interface {
    Create(url string) (*TaskCreateResult, error)
    CreateWorkInfo(task *TaskDTO) (*WorkResponse, error)
    Start(ctx context.Context, task *TaskDTO, storeRoles []string) ([]*StoreSpec, *WorkResponse, error)
    Retry(task *TaskDTO) (*WorkResponse, error)
    Pause(param *TaskResParam) error
    Stop(param *TaskResParam) error
    Resume(ctx context.Context, param *TaskResumeParam) ([]*StoreSpec, *WorkResponse, error)
}
```

- `Create`:解析 URL,返回任务信息。返回值用 `sdkdto.BatchResult(...)` 或 `sdkdto.StreamResult(...)` 构造(批量/流式)。可在响应中声明 `InvolvedRoles`(任务涉及的 store_type 集合,见下「创建期声明涉及板块」)。
- `CreateWorkInfo`:抓取作品元数据(标题/作者/标签等)。
- `Start`:首次或重下执行。`storeRoles` 为本次所选 store_type 子集(空=全量),据此选择性产出,避免生成被丢弃的 store。返回 StoreSpec 集合 + WorkResponse。
- `Retry`:重下执行(通常委托 Start)。
- `Pause`/`Stop`:任务级控制。应关闭 reader 上游连接(HTTP body / 文件句柄),使在途读取尽快返回。
- `Resume`:按 StreamOffsets 续传未完成 downloaded 轨;derived 轨未完成时由主程序另行调 Start 整轨重产。

#### 创建期声明涉及板块(InvolvedRoles)

`Create` 返回的 `TaskCreateResponse`(及子任务 `TaskCreateChildResponse`)可声明本任务涉及的 store_type 集合 `InvolvedRoles`(universe),供主程序与前端按任务感知:

- **声明 = hint,Start 产出 = truth**:`InvolvedRoles` 用于前端重执行 UI 只展示该任务涉及的板块、以及主程序首跑默认请求集;主程序挂载以插件 `Start` 实际产出的 StoreSpec 为准,**不据 universe 拒绝超出声明的 spec**(创建期声明不全时仍健壮)。
- **空/nil = 未确定(default)**:创建期信息不足以判定时留空,执行期插件按全量(空 storeRoles)自决产出;前端对 default 任务展示兜底集。
- **声明时机**:能据 URL 即定则定(如纯图片站点恒为 `[main]`,可省去无意义的缩略图派生请求);需深度元数据才能定则留空走 default。

例:纯图片站点 `Create` 声明 `InvolvedRoles=[main]` → 首跑 Start 请求集仅 `[main]`,不触发缩略图派生轨;重执行 UI 仅显示「资源」一项。

#### StoreSpec(资源产出声明)

```go
type StoreSpec struct {
    Role        string        // store_type: main | thumbnail | videoTrack | ...
    Generation  string        // downloaded(流式可续传) | derived(一次性派生)
    ReadCloser  io.ReadCloser // 资源数据流,调用方负责 Close
    Format      string        // 文件扩展名
    Size        int64         // 完整资源大小;-1 未知
    SuggestName string        // 插件建议文件名
    Continuable *bool         // 是否支持续传(derived 恒为 false)
    ResumeWriteOffset *int64  // 续传写入偏移(仅 Resume);nil=信任主程序磁盘 stat
}
```

- `downloaded`:流式下载资源(主图/视频轨),支持断点续传。
- `derived`:一次性派生产物(缩略图),整轨产出不可续传,ReadCloser 常用 `io.NopCloser(bytes.NewReader(payload))`。
- **`Format` 前导点约定(易错,主程序两套路径逻辑不一致,须分别遵守)**:
  - **downloaded 轨**(main/videoTrack/audioTrack 等):`Format` **带前导点**(如 `.mp4`、`.m4a`、`.jpg`、`.png`)。主程序 `resolveMainPath` 直接 `文件名 + Format` 拼接。
  - **derived 缩略图**(thumbnail):`Format` **不带前导点**(如 `jpg`、`png`)。主程序 `buildThumbnailRelPath`/`buildThumbnailFileName` 自拼 `_thumbnail.` + Format(自己加 dot);若带点会产出 `_thumbnail..jpg`,既命名错误又会因 `..` 子串触发静态服务路径穿越拦截 → 前端 404。

#### ctx 与 reader 契约(重要)

`Start`/`Resume` 的 `ctx` 是 gRPC stream context。任务暂停/停止时主程序取消该 ctx,经 gRPC stream 传播到插件,SDK 的 serveSpecsPull 据此 `Close` reader 中断在途读取。插件 reader 必须满足:

1. **`reader.Close()` 可中断阻塞中的 `Read`**。`*http.Response.Body`、`*os.File` 等合规 reader 天然满足;自定义 reader 需保证。
2. **reader 不要跨 RPC 复用**。每次 Start/Resume 新建 reader,不要缓存上次的 reader 在下次 Resume 复用。pull 模型下旧 serveSpecsPull goroutine 的退出与主程序命令切换不同步,跨 RPC 复用 reader 会让旧 goroutine 与新 Read 并发访问同一 reader,导致数据错位。
3. **`Size` 必须是完整资源大小**(非 Range 续传剩余字节数)。HTTP 206 的 `Content-Length` 是剩余字节数,需解析 `Content-Range: bytes start-end/total` 取 `total`,或用 `offset + Content-Length` 还原;误填剩余字节会使主程序完整性校验失效。
4. **ctx 取消后立即停止发送**。Stop 立即取消 ctx;Pause 在 setup 阶段立即取消 ctx(无在途 chunk),在 downloadLoop 阶段走优雅暂停(不取消 ctx,在途数据由主程序排空落盘),仅 drain 超时(2s)兜底时才取消 ctx。ctx 取消后插件不再发送 chunk,在途未发送的 chunk 不保证持久化,可丢弃(仅 setup/停止/兜底场景)。

#### Pause 的数据持久化边界

暂停按阶段分流:**setup 阶段(未进 downloadLoop)无在途 chunk,主程序立即取消 ctx 中断插件 RPC(快速、无损)**;**downloadLoop 阶段走优雅暂停**,不取消 ctx,置 `softPause` 标志,copyLoop 完成当前在途往返(已发起的 PullRequest 的数据照常 recv + 落盘)后退出,不再发起新 PullRequest。downloadLoop 阶段数据边界界定:

- **已持久化**:主程序 copyLoop 已 `storeWriter.Write` 的字节,等于磁盘文件大小(`os.Stat`)。Resume 从此继续。
- **常态排空(在途落盘)**:优雅暂停时已发起的 PullRequest 的数据,由 copyLoop 完成 recv + Write 落盘,磁盘 stat 天然对齐中断点,Resume 无重复下载。
- **异常超时丢失(仅兜底)**:`drainTimer`(2s)到期强制取消 ctx 时,在途未完成的 chunk 丢失(单个 ≤32K),退化为有损立即暂停,完整性由 Resume 从磁盘 stat 重下兜底,最终文件正确。

主程序以**磁盘 stat 为权威**,不依赖插件在途状态。setup 快速中断 + downloadLoop 常态无损(在途排空,stat 对齐)+ 异常兜底(2s 封顶有损),兼顾各阶段即时性与对齐。插件 Resume 逻辑不变(从 StreamOffsets 读 stat → 计算偏移 → 新建 reader)。

#### Resume 续传要点

- 主程序通过 `TaskResumeParam.StreamOffsets`(role → 该轨已落盘字节数,来自磁盘 stat)传入续传起点。
- downloaded 轨:据此 offset 发 Range 请求(或本地文件 Seek)从 offset 续读。
- derived 轨:不进 StreamOffsets,未完成时主程序另行 Start 整轨重产,Resume 无需产出 derived。
- 续传权威是磁盘已落盘字节数(StreamOffsets),不是 reader 内部计数——以 StreamOffsets 为准对齐。

#### 缩略图

缩略图作为 `Generation=derived` 的 StoreSpec 在 Start 里产出,而非单独接口。从作品元数据/URL 取缩略图字节包装返回;无则不产出该轨(非致命)。**`Format` 不带前导点**(如 `jpg`),主程序构建缩略图路径时自加 dot(见上 StoreSpec 的 Format 前导点约定)。

### 6.2 SiteBrowser（运行时）

```go
type SiteBrowser interface {
    Open() error
    Close() error
}
```

> SiteBrowser（运行时注册，业务功能）与 `siteBrowserList` Slot（声明式，UI 入口卡片）**是两个东西，必须同时存在**：Slot 提供点击入口，SiteBrowser 提供打开/关闭逻辑。Slot 的 `extensionId` 指向 SiteBrowser 的 `id`。

### 6.3 Slot（声明式）

通过 `plugin.json` 的 `extensions.slots` 声明，主程序启动时自动注册到前端。6 种 slotType × 4 种 contentType 的组合见第三节。

**组件加载**（主程序前端 `useSlotSyncListener`）：
- `precompiled`：动态 `import(js)` → 调用工厂函数 `module.default(Vue, WailsRuntime)` 注入依赖。
- `vueSource`：优先用预编译缓存 `js`，否则运行时编译 `vue` 源码。
- `code`：`new Function('return ' + code)()`。
- `html`：`fetch` HTML 后作为 Vue `template`。

**预编译组件构建**：插件前端用 Vite + `componentFactoryPlugin` 构建为工厂函数（替换 `vue`/`@wailsio/runtime` import 为注入变量，`export default` 转 `return`）。**禁止 `import { X as Y }` 的 `as` 语法**（工厂插件不兼容，需别名时直接修改变量名）。

## 七、插件自存信息

统一 KV 存储（`plugin_storage` 单表），明文与加密共存。**取代旧的 `plugin_data` 与加密存储**。

### 7.1 存取 API

```go
// 明文
ctx.SetValue("downloadPath", "/data")
v, _ := ctx.GetValue("downloadPath")        // 读

// 加密（敏感数据，存密文、读自动解密）
ctx.SetValueEncrypted("apiToken", secret)
token, _ := ctx.GetValue("apiToken")        // 自动解密返回明文

// 删除 / 全部
ctx.DeleteValue("downloadPath")
all, _ := ctx.GetAllValues()                // map[key]明文值（加密项已解密）
```

读取（`GetValue`/`GetAllValues`）对加密项**透明解密**，统一返回明文。插件只在**写入**时分 `SetValue`/`SetValueEncrypted`。

### 7.2 用户设置（settings）

在 `plugin.json` 声明 `extensions.settings` 后：
- 主程序在插件管理页渲染表单（按 `type` 分发控件、按 `group` 分组），用户编辑后由主程序按声明的 `encrypted` 路由 `SetValue`/`SetValueEncrypted` 存入。
- 插件用 `ctx.GetValue(key)` 读取用户配置值（统一为 string，integer 等类型自行转换）。

## 八、前端通信

插件与前端通过 **Wails Events** 双向通信（经主程序 gRPC 桥接）。

### 协议

- **topic 命名**：`plugin:{plugin-name}:{feature}:{action}`（如 `plugin:local-import:classify:request`）。`{plugin-name}` 用连字符形式（对应 `name` 小写连字符）。
- **data 格式**：`[]byte`（JSON 序列化）。

### 插件 → 前端

```go
data, _ := json.Marshal(payload)
ctx.PublishToFrontend("plugin:my-plugin:feature:action", data)
```
前端：`Events.On("plugin:my-plugin:feature:action", cb)`。

### 前端 → 插件

```go
ch, _ := ctx.SubscribeFrontend("plugin:my-plugin:feature:request")
for data := range ch {
    var req Request
    json.Unmarshal(data, &req)
    // 处理...
    resp, _ := json.Marshal(response)
    ctx.PublishToFrontend("plugin:my-plugin:feature:response", resp)
}
```
前端：`Events.Emit("plugin:my-plugin:feature:request", data)`。

> **注意**：插件阻塞等待前端响应时，务必设置超时（避免永久阻塞）。长期订阅建议配合 `UnsubscribeFrontend` 释放资源。

## 8.1 插件前端组件调用主程序后端

插件通过 Slot（view/embed/dialog/replaceView）加载的前端组件运行在主程序渲染进程内，可通过 `window.__PLUGIN_CTX__.custom.apis` 直接调用主程序后端接口（Wails IPC）。

```ts
// 在插件前端组件内（precompiled JS 工厂函数）
onMounted(function() {
  var apis = window.__PLUGIN_CTX__ && window.__PLUGIN_CTX__.custom && window.__PLUGIN_CTX__.custom.apis
  if (!apis) return
  // 调用主程序后端——查询插件列表
  apis.pluginApi.pluginQueryPage({ page: 1, pageSize: 10, total: 0 }, {})
    .then(function(result) {
      // result.data.data = 插件列表（Page<T>.data）
    })
})
```

**可用 API**（共 21 个模块）：`localTagApi`、`localAuthorApi`、`siteTagApi`、`siteAuthorApi`、`siteApi`、`workApi`、`workSetApi`、`searchApi`、`taskApi`、`recycleBinApi`、`pluginApi`、`pluginSettingApi`、`settingsApi`、`fileSysUtilApi`、`appLauncherApi`、`siteBrowserApi`、`pluginTaskUrlListenerApi`、`windowApi` 等。

**还可用**：`window.__PLUGIN_CTX__.vue`（Vue 实例）、`window.__PLUGIN_CTX__.globals.$message/$notify/$confirm`（Element Plus 全局方法）、`window.__PLUGIN_CTX__.globals.$router/$store`（路由与 Pinia）、`window.__PLUGIN_CTX__.theme.getCurrent()`（当前主题 id）。

> 分页响应结构：`ApiResult<Page<T>>` → `result.data.data`（数据列表），`result.data.dataCount`（总数）。

## 九、原生窗口（OpenWindow，仅 Windows）

打开 WebView2 弹窗，常用于 OAuth 登录等需要浏览器交互的场景。

```go
import sdkwindow "github.com/lvfeng-z/library-squirrel-sdk/window"

win, err := sdkwindow.OpenWindow(sdkdto.WindowOptions{
    Title:  "登录",
    Width:  800,
    Height: 700,
    URL:    "https://...",
    DataPath: "",  // WebView2 用户数据目录，空用默认
    OnNavigationStarting: func(uri string) bool {
        return !strings.HasPrefix(uri, "myapp://")  // 返回 false 拦截导航
    },
}, ctx.GetMainWindowHandle())  // ownerHWND，传主窗口句柄使弹窗置顶
if err != nil { ... }

// 等待回调 URL（阻塞，带超时）
callback, err := win.WaitForNavigation("myapp://", 5*60*1000)
win.Close()
```

**WindowOptions 字段**：`Title`、`Width`、`Height`（0 时默认 800×600）、`URL`、`DataPath`、`OnNavigationStarting`。

**WindowHandle 方法**：`Close()`、`SetTitle(title)`、`WaitForNavigation(urlPrefix, timeoutMs)`、`ExecuteScript(js)`、`Done() <-chan struct{}`。

> `ctx.GetMainWindowHandle()` 返回主窗口 HWND（由主程序在 Activate 时注入），作为 `ownerHWND` 让弹窗置顶主窗口。**仅 Windows 支持**，其他平台返回错误。

## 十、静态资源

### URL 规约

```
http://wails.localhost:{backend-port}/plugin/{author}/{id}/{version}/{relativePath}
```

- `publicId = author/id`（URL 中展开为两段）。
- 后端在注册 Slot 时自动把 `source`/`icon` 的相对路径转为上述完整 URL，前端组件直接用。
- 插件代码内可用 `ctx.GetPluginRoot(false)` 获取插件根目录绝对路径。

### 目录白名单

只有声明在 `extensions.staticResources.directories` 中的目录可通过 HTTP 访问。**未声明的目录前端无法加载**（即使打包进 dist）。

```json
"staticResources": {"directories": ["views/", "assets/"]}
```

### 缓存

静态资源带 `ETag` + `Cache-Control: public, max-age=31536000, immutable`（基于版本号），版本升级自动失效。

## 十一、打包与发布

### build.ps1 步骤

插件 `build.ps1` 按以下顺序：

1. **前端构建**（有 UI 组件时）：`cd frontend && yarn install && yarn build`（输出预编译 JS/CSS 到 `views/`）。
2. **Go 构建**：`go build -o {entryFile} .`（生成可执行文件）。
3. **打包 dist**：复制 `plugin.json`、可执行文件、`views/`、`assets/` 等到 `dist/`。
4. **压缩**：`Compress-Archive dist/* -DestinationPath dist/{name}.zip`。

> **PowerShell 注意**：`Compress-Archive` 的进度条在某些 PowerShell 版本触发 `IndexOutOfRangeException` bug。可在压缩前设 `$ProgressPreference = 'SilentlyContinue'` 抑制进度输出。

### build.ps1 必须复制所有资源目录

`plugin.json` 的 `staticResources.directories` 声明的目录（如 `views/`、`assets/`）必须被 `build.ps1` 复制到 `dist/`。**漏复制会导致安装后前端组件 404**。

### dist 目录结构

```
dist/
├── plugin.json
├── my_plugin.exe        # 运行时插件（纯 UI 插件无此项）
├── views/               # 预编译组件（有 UI 时）
│   └── panel.js / style.css
├── assets/              # 图标等
└── my-plugin.zip        # 安装包
```

### 安装

- **手动安装**：插件管理页「安装」→ 选择 ZIP → 主程序 `InstallFromPath` 解压到 `plugin/package/{publicId}/{version}/` 并写库。
- **捆绑插件**：放 `resources/bundled-plugins/`，主程序首次启动自动安装（已安装则跳过）。

### 版本管理

- **单版本**：同一 `publicId` 不支持多版本共存，DB 只保留一条记录。
- **升级**：通过「修复」（`Reinstall` 从备份恢复）或「选择安装包修复」（`ReinstallFromPath` 指定新 ZIP），会先卸载旧版（删目录）再重装。

## 十二、卸载行为

卸载插件时，主程序执行：
1. 停止插件子进程（`Shutdown` RPC → 超时强杀）。
2. 清理扩展点：TaskHandler/SiteBrowser/UrlListener/Slot 全部注销。
3. 清理静态资源注册。
4. **前端刷新**：如果插件含 `view`/`replaceView` slot，前端执行 `window.location.reload()` 重新加载（清除已渲染的插件组件与模块缓存，保持当前页面 URL）。
5. 插件前端组件的 CSS 通过 `link[data-plugin-id]` 标签管理，可在 `useSlotSyncListener` 的 `unloadPluginStyles` 中清理。

> reload 后主程序内置路由（`routes.ts` 静态定义）能直接匹配当前 URL，无需等待动态注册。

## 十三、完整示例

参考真实插件：
- **pixiv 插件**（`library-squirrel-plugin-pixiv`）：运行时插件，含 TaskHandler + SiteBrowser + OAuth 登录（OpenWindow）+ 统一自存信息（token 加密存储）+ siteBrowserList Slot + 测试用 view/replaceView/embed/dialog/menu slot + 前端调用主程序后端（`window.__PLUGIN_CTX__`）。
- **local-import 插件**（`library-squirrel-plugin-local`）：含前端通信（`PublishToFrontend`/`SubscribeFrontend`）+ 预编译 Vue 组件 dialog Slot。

## 十四、最佳实践与陷阱

1. **Activate 用标准签名**：`WithActivate(func(ctx))`，需要依赖注入时用闭包捕获，不要改签名。
2. **优先声明式 Slot**：UI 扩展用 `plugin.json` 声明，无需子进程（纯 UI 插件）。
3. **静态资源目录要声明 + build.ps1 要复制**：`staticResources.directories` 必须包含所有前端要访问的目录，且 `build.ps1` 必须复制到 `dist/`，否则安装后 404。
4. **路径用相对的**：`plugin.json` 中 `icon`、`source` 路径用相对插件根目录的相对路径，后端自动转完整 URL。
5. **敏感数据用 SetValueEncrypted**：token、密钥等用加密存储，读取透明解密。
6. **前端通信加超时**：阻塞等待前端响应必须设超时，避免永久阻塞。
7. **OpenWindow 传 ownerHWND**：`ctx.GetMainWindowHandle()` 作为 owner，否则弹窗不置顶。
8. **预编译组件禁用 `import as`**：需要别名直接修改变量名。
9. **vueSource 的 source 是 `{vue, js?, css?}`**：不是 `{entry}`；`js/css` 是可选预编译缓存。
10. **错误处理**：所有 API 返回 error，务必处理并用 `ctx.Errorf` 记录日志。
11. **插件前端调主程序后端**：用 `window.__PLUGIN_CTX__.custom.apis`，不要硬编码 Wails method ID。
12. **replaceView 的 target 要匹配路由 name**：主程序路由 name 在 `routes.ts` 静态定义（如 `taskManage`、`settings`），插件 target 必须与之完全一致。
13. **embed 需主程序暴露插槽位**：插件声明 embed slot 的 `position` 必须对应主程序某处 `<EmbedSlotRenderer position="xxx">` 才会渲染。
