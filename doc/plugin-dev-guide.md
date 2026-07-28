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

**本地协同开发 SDK**：若需同时改 SDK（跨仓库），在插件 `go.mod` 加 replace 指令指向本地 SDK 源码：

```
replace github.com/lvfeng-z/library-squirrel-sdk => ../library-squirrel-sdk
```

发布前确认 replace 不打包进正式版本（build.ps1 打包时注意），确保用正式 SDK 版本。

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
| `frontendExtensions` | `[FrontendExtensionDeclaration]` | 三选一 | UI 前端扩展（声明式注册，见 6.3） |
| `staticResources` | `{directories: string[]}` | 否 | 允许前端访问的资源目录白名单 |
| `settings` | `[SettingDeclaration]` | 否 | 用户可配置项（见 8.2） |

**校验规则**（安装时）：
- `id/name/version/author` 必填。
- `extensions` 必须存在，且 `taskHandlers/siteBrowsers/frontendExtensions` 至少一个非空。
- `entryFile` 仅在含运行时扩展点（taskHandlers 或 siteBrowsers）时必填；纯 UI 插件可省略。
- 枚举值（kind/contentType/position/settings.type）**安装时不校验**，错误值在激活/运行期暴露，请自行核对拼写。

### 前端扩展声明

```jsonc
{
  "id": "my-view",
  "name": "我的视图",
  "description": "可选描述",
  "kind": "view",          // embed | view | replaceView | dialog | menu | siteBrowserList（Slot）| resourceViewer（Handler）
  "order": 100,
  "content": { /* 按 kind，见下 */ }
}
```

**content 按 kind：**

| kind | content 字段 | 说明 |
|---|---|---|
| `embed` | `{contentType, source, position, props?}` | 插入主程序具名插槽位（position = 插槽位标识，主程序定义） |
| `view` | `{contentType, source, title?, props?}` | 新增独立路由页面 |
| `replaceView` | `{contentType, source, target, props?}` | 替换主程序已有页面（target = 路由 name） |
| `dialog` | `{contentType, source, props?}` | 弹窗（模态层） |
| `menu` | `{icon?, viewId?, children?}` | 菜单项（点击跳转关联的 view slot；children 递归） |
| `siteBrowserList` | `{icon?, extensionId}` | 站点浏览器入口卡片（extensionId 必须等于某 siteBrowsers 的 id） |
| `resourceViewer` | `{contentType, source, resourceType, props?}` | 资源渲染器（Handler 被动响应型；resourceType = 资源类型查找键，渲染器接收 `{resource, work}` props） |

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

声明后，主程序在**插件管理页**自动渲染设置表单，用户编辑后存入插件自存信息；插件用 `ctx.GetValue(key)` 读取（见 8.2）。

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
- **声明时机**:能据 URL 即定则定(如纯图片站点恒为 `[image]`,可省去无意义的缩略图派生请求);需深度元数据才能定则留空走 default。

例:纯图片站点 `Create` 声明 `InvolvedRoles=[image]` → 首跑 Start 请求集仅 `[image]`,不触发缩略图派生轨;重执行 UI 仅显示「图片」一项。

#### 声明资源类型(ResourceType,必填)

`TaskCreateResponse` / `TaskCreateChildResponse` 的 `ResourceType` 字段**必须声明**预定义值之一(`sdkdto.ResourceTypeImage`/`Video`/`Article`/`Document`/`Unknown`)。主程序**严格识别,不推断、不兜底**:

- `ResourceType` 空 / 非预定义值 → 写入路径抛错(任务创建/执行失败)
- `StoreSpec.Role` 非 `image`/`document`/`thumbnail`/`videoTrack`/`audioTrack`/`merged` 六角色 → 抛错
- `unknown` 是合法显式值(插件确实无法分类时声明),不报错

资源类型决定 store 角色组合(结构规约)+ 展示主体优先级 + 文件标准。完整规约见 **`doc/resource-type-spec.md`**。例:图片资源声明 `ResourceType: sdkdto.ResourceTypeImage`,Start 产出 `Role: sdkdto.StoreRoleImage`(+ 可选 `StoreRoleThumbnail`)。

#### StoreSpec(资源产出声明)

```go
type StoreSpec struct {
    Role        string        // store_type: image | document | thumbnail | videoTrack | audioTrack | merged
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
- **`Format` 前导点约定**:扩展名(如 `.mp4`、`.jpg`、`.md`)。主程序 `resolveStorePath` 经 `normalizeExt` 统一补前导点(不带点会自动补),**带不带点都正确**,建议带点(与 ResourceType 文件标准一致)。命名规约(单 store `<bas>.<ext>` / 多 store `<bas>_<role>_<seq>[_<描述>].<ext>`,thumbnail 普通 role 无特例)详见 `doc/store-naming-convention.md`。

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

- 主程序通过 `TaskResumeParam.StreamOffsets` 传入续传起点:`[]*StoreResumeOffset{ role, store_seq, offset }`(身份化——同 role 多 store 按 store_seq 各自独立;offset 为磁盘 stat 已落盘字节数)。单例场景可用便捷方法 `param.OffsetForRole(role)` 取首个命中;N-同 role 多 store(如 article 内嵌图)须插件自行遍历按 store_seq 精确匹配。
- downloaded 轨:据此 offset 发 Range 请求(或本地文件 Seek)从 offset 续读。
- derived 轨:不进 StreamOffsets,未完成时主程序另行 Start 整轨重产,Resume 无需产出 derived。
- 续传权威是磁盘已落盘字节数(StreamOffsets),不是 reader 内部计数——以 StreamOffsets 为准对齐。

#### 缩略图

缩略图作为 `Generation=derived` 的 StoreSpec 在 Start 里产出,而非单独接口。从作品元数据/URL 取缩略图字节包装返回;无则不产出该轨(非致命)。**`Format` 不带前导点**(如 `jpg`),主程序构建缩略图路径时自加 dot(见上 StoreSpec 的 Format 前导点约定)。

#### CreateWorkInfo 与作品元数据落库

`Create` 产出的作品元数据**不直接入库**,而是先序列化进 `TaskCreateResponse.PluginData`(字符串),由主程序存入 task 记录;后续主程序调用 `CreateWorkInfo(task)` 反序列化 `PluginData` 并构建 `WorkResponse` 返回——**这是元数据落库的唯一入口**。数据流:

```
Create → PluginData(含 SiteAuthors/SiteTags/UnifiedWorkInfo)序列化
       → 主程序存 task.PluginData
CreateWorkInfo(task) → 反序列化 task.PluginData
                     → 构建 WorkResponse(TaskSiteAuthorDTO/TaskSiteTagDTO)
                     → 主程序写 work/author/tag 表
```

关键 DTO 字段(**留空则该字段不入库/不前端展示**):

- `TaskSiteAuthorDTO`:`SiteAuthorID`、`AuthorName`、`Introduce`(作者简介/签名)、`Homepage`、`Rank`(等级等)、`FixedAuthorName` 等。
- `TaskSiteTagDTO`:`SiteTagID`、`TagName`、`Description`(标签简介)。

**兜底原则**:作者/标签的富信息(简介/等级)获取失败时(站点风控、字段不存在等),回退基本字段(name + homepage),标记为非致命——`CreateWorkInfo` 不应因富信息缺失而整体失败。站点返回的字段名/结构务必先 curl 确认(见第十五节),勿盲猜字段名。

### 6.2 SiteBrowser（运行时）

```go
type SiteBrowser interface {
    Open() error
    Close() error
}
```

> SiteBrowser（运行时注册，业务功能）与 `siteBrowserList` Slot（声明式，UI 入口卡片）**是两个东西，必须同时存在**：Slot 提供点击入口，SiteBrowser 提供打开/关闭逻辑。Slot 的 `extensionId` 指向 SiteBrowser 的 `id`。

### 6.3 前端扩展（声明式）

通过 `plugin.json` 的 `extensions.frontendExtensions` 声明，主程序启动时自动注册到前端。6 种 kind × 4 种 contentType 的组合见第三节。

**组件加载**（主程序前端 `useSlotSyncListener`）：
- `precompiled`：动态 `import(js)` → 调用工厂函数 `module.default(Vue, WailsRuntime)` 注入依赖。
- `vueSource`：优先用预编译缓存 `js`，否则运行时编译 `vue` 源码。
- `code`：`new Function('return ' + code)()`。
- `html`：`fetch` HTML 后作为 Vue `template`。

**预编译组件构建**：插件前端用 Vite + `componentFactoryPlugin` 构建为工厂函数（替换 `vue`/`@wailsio/runtime` import 为注入变量，`export default` 转 `return`）。**禁止 `import { X as Y }` 的 `as` 语法**（工厂插件不兼容，需别名时直接修改变量名）。

## 七、站点交互：HTTP 客户端、代理、风控与登录态

爬虫型插件（下载/抓取外部站点资源）的核心挑战在于与站点的博弈——代理环境、风控拦截、登录态。本节给出经实践验证的模式。

### 7.1 HTTP 客户端与代理

用户常处于 VPN/代理环境，插件 HTTP 客户端必须正确决策代理：

- **代理决策顺序**：插件显式设置（用户在 `settings` 填 `proxyUrl`）> 系统代理 > 环境变量（`HTTP_PROXY`/`HTTPS_PROXY`）。
- **Windows 系统代理要走注册表**：`http.ProxyFromEnvironment` 只读环境变量、不读 Windows 系统代理（注册表 `ProxyServer`），导致"规则模式 VPN"（设系统代理、不设 env）下插件直连被限速/拦截。需自行读注册表补全。
- **连接复用（keep-alive）opt-in 而非默认**：某些代理（尤其 HTTP 隧道）对 keep-alive 连接发送 `Unsolicited response`，导致复用连接读到脏数据。建议 API/下载路径的 `http.Transport` 默认 `DisableKeepAlives=true`，仅在确认代理健康时显式开启连接池（配 `IdleConnTimeout`/`ResponseHeaderTimeout` 降低取到陈旧连接的概率）。
- **下载路径与 API 路径用不同 Transport**：API 路径（短请求、风控敏感）禁用 keep-alive；下载路径（大文件、重连代价高）可开启连接复用（opt-in）。

### 7.2 站点风控识别与应对

站点对程序化请求做风控，常见返回码（以 bilibili 为例，其他站点类似）：

| 现象 | 典型码 | 含义 | 应对 |
|---|---|---|---|
| 未登录 | `-101` / HTTP 401 | 账号未登录 | 走登录流程（见 7.3） |
| 风控校验失败 | `-352` | 指纹/行为风控 | 补浏览器指纹 cookie（buvid3/buvid4 等） |
| 访问权限不足 | `-403` | 高级风控（缺关键指纹参数） | 攻坚（如 acc/info 缺 `w_webid`）或暂接受兜底 |
| 请求过快 | `412` / `-799` | 频控 | 降频 + 退避重试 |

**应对原则**：
1. **两段解析响应**：先解通用外壳 `{code, message}`，`code != 0` 直接返回干净错误（避免错误响应体结构差异引发迷惑性 parse 失败）；`code == 0` 再解全包到业务结构。
2. **非致命回退**：元数据富信息（作者简介、标签描述等）获取失败时回退基本字段（name + id + homepage），不影响主流程下载。
3. **勿盲信过时资料**：站点风控持续升级，社区逆向方案（博客/issue）可能过时（bilibili 曾从 `dm_img_*` 参数升级到 `w_webid`）。先 curl 实测当前响应（见第十五节），再定方案。

### 7.3 登录态管理

许多站点资源（高清/个人信息）需登录态。登录方式按站点：

- **OAuth**（如 pixiv）：`OpenWindow` 打开授权页 → 拦截 callback URL（见第十节）。
- **扫码登录**（如 bilibili）：调站点 QR generate 接口取二维码 → `OpenWindow` 用 `data:URL` Navigate 显示二维码（**不要用 `ExecuteScript(document.write)`，有 UAF 风险，见第十节**）→ 轮询 QR poll 接口，成功时响应 `Set-Cookie` 携带登录态。

**登录态持久化**（跨重启）：

```go
// 登录成功后：导出 cookie 加密存储
cookies := client.ExportLoginCookies()       // []*http.Cookie
data, _ := json.Marshal(cookies)
ctx.SetValueEncrypted("login_cookies", string(data))

// 启动期恢复
raw, _ := ctx.GetValue("login_cookies")
var cookies []*http.Cookie
json.Unmarshal([]byte(raw), &cookies)
client.ImportCookies(cookies)
```

`http.Client` 用 `CookieJar` 自动管理 cookie（登录响应的 `Set-Cookie` 自动捕获，后续请求自动带）。敏感 cookie（SESSDATA 等）务必用 `SetValueEncrypted` 加密存储。

**未登录引导模式**（推荐）：`Create` 检测未登录时，异步弹登录窗 + 返回"请扫码后重试"错误，主程序提示用户；用户登录后重新 Create。避免阻塞主线程等登录。

```go
func (h *Handler) Create(url string) (*sdkdto.TaskCreateResult, error) {
    if !h.loggedIn() {
        h.autoLoginAsync()   // 异步弹登录窗（CAS 去重，避免重复弹）
        return nil, errors.New("未登录，请完成登录后重试（登录窗口已弹出）")
    }
    // ... 正常 Create
}
```

## 八、插件自存信息

统一 KV 存储（`plugin_storage` 单表），明文与加密共存。**取代旧的 `plugin_data` 与加密存储**。

### 8.1 存取 API

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

### 8.2 用户设置（settings）

在 `plugin.json` 声明 `extensions.settings` 后：
- 主程序在插件管理页渲染表单（按 `type` 分发控件、按 `group` 分组），用户编辑后由主程序按声明的 `encrypted` 路由 `SetValue`/`SetValueEncrypted` 存入。
- 插件用 `ctx.GetValue(key)` 读取用户配置值（统一为 string，integer 等类型自行转换）。

## 九、前端通信

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

### 9.1 插件前端组件调用主程序后端

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

## 十、原生窗口（OpenWindow，仅 Windows）

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

> **`ExecuteScript` UAF 风险**：`ExecuteScript` 通过 COM 异步回调执行 JS，handler 经 `unsafe.Pointer` 传递，存在生命周期 UAF（use-after-free）风险——可能触发插件子进程堆损坏（Windows 下 `0xc0000374`）。需要往窗口注入内容的场景（如 `document.write` 显示二维码/HTML），改用 **`data:URL` + 重新 Navigate** 规避（把内容编码为 `data:text/html;base64,...` 作为窗口 URL，或初始 `URL` 直接用 data URL）。仅做导航拦截（`WaitForNavigation`）不触发此问题。

> `ctx.GetMainWindowHandle()` 返回主窗口 HWND（由主程序在 Activate 时注入），作为 `ownerHWND` 让弹窗置顶主窗口。**仅 Windows 支持**，其他平台返回错误。

## 十一、静态资源

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

## 十二、打包与发布

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

## 十三、卸载行为

卸载插件时，主程序执行：
1. 停止插件子进程（`Shutdown` RPC → 超时强杀）。
2. 清理扩展点：TaskHandler/SiteBrowser/UrlListener/Slot 全部注销。
3. 清理静态资源注册。
4. **前端刷新**：如果插件含 `view`/`replaceView` slot，前端执行 `window.location.reload()` 重新加载（清除已渲染的插件组件与模块缓存，保持当前页面 URL）。
5. 插件前端组件的 CSS 通过 `link[data-plugin-id]` 标签管理，可在 `useSlotSyncListener` 的 `unloadPluginStyles` 中清理。

> reload 后主程序内置路由（`routes.ts` 静态定义）能直接匹配当前 URL，无需等待动态注册。

## 十四、完整示例

参考真实插件：
- **pixiv 插件**（`library-squirrel-plugin-pixiv`）：运行时插件，含 TaskHandler + SiteBrowser + OAuth 登录（OpenWindow）+ 统一自存信息（token 加密存储）+ siteBrowserList Slot + 测试用 view/replaceView/embed/dialog/menu slot + 前端调用主程序后端（`window.__PLUGIN_CTX__`）。
- **local-import 插件**（`library-squirrel-plugin-local`）：含前端通信（`PublishToFrontend`/`SubscribeFrontend`）+ 预编译 Vue 组件 dialog Slot。

## 十五、调试与诊断

插件是独立子进程，调试不同于普通 Go 程序。本节给出经实践验证的诊断方法。

### 15.1 日志查看

- **插件业务日志**：写入主程序 `log/server.log`，前缀 `Plugin[插件名].模块`（如 `Plugin[bilibiliSuite].BilibiliAPI`）。插件用 `ctx.Infof/Warnf/Errorf` 写入。
- **子进程崩溃**：插件 panic 输出在主日志，前缀 `plugin.插件文件名.exe:`（如 `plugin.bilibili_plugin.exe: panic: ...`）——子进程崩溃先找这个前缀。
- **前端日志**（仅 dev）：`log/frontend.log`，含插件前端组件的 `console.*` 与未捕获异常。
- **SDK/框架日志**：`[DEBUG] plugin.插件文件名.exe:` 前缀（如 RetryReader 建连、gRPC 握手），主日志 DEBUG 级别可见。

### 15.2 第三方 API/页面数据源验证（写代码前必做）

**不要盲猜站点 API/页面的字段名与结构**——先 curl 实测确认。常见踩坑：假设字段名 `abstract`/`description` 实际是 `content`/`short_content`；假设对象含某字段实际不含（如用户信息对象无 `sign`，sign 在别处或根本不返回）。

```bash
# API：带 UA/Referer/cookie 模拟插件请求
curl -sL -A "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 ..." \
  -e "https://目标站点/" "https://api.xxx.com/endpoint?param=1" | head -c 1000

# 页面：确认 __INITIAL_STATE__ / 关键字段是否存在
curl -sL -A "Mozilla/5.0 ..." "https://www.xxx.com/page/123" | grep -oE "字段名|__INITIAL_STATE__"
```

验证要点：响应字段名、字段是否真有值（数据源可能"字段存在但内容稀缺"——如标签简介多数为空，非 bug，兜底处理）、风控码（-101/-352/-403 等）。

### 15.3 诊断日志技巧

定位风控/字段问题时，在请求处打印请求参数 + 响应 body 片段：

```go
c.logger.Infof("请求诊断: params=%v cookie项数=%d", params, len(jar.Cookies(u)))
// 业务错误统一附 body 片段（截断避免刷屏）
return fmt.Errorf("API 业务错误: code=%d message=%s body=%s", code, msg, truncate(body, 300))
```

诊断完成后移除诊断日志或降级为 `Debugf`，避免污染生产日志。

### 15.4 常见陷阱

- **Go regexp repeat 上限 1000**：`regexp.MustCompile("x.{0,1500}")` 启动期 panic（`invalid repeat count`），插件子进程直接退出，主程序看到"插件启动失败 exit status 2"。改用 `.*?` 非贪婪匹配到边界标记，或 `{0,1000}` 以内。
- **页面 JSON 嵌套提取**：`__INITIAL_STATE__` 里的对象常多层嵌套（如 `module_author.avatar.fallback_layers.layers[]`），正则 `"x":\{.*?\}` 会停在第一个内嵌 `}`——应匹配到下一个字段边界（如 `"x":.*?module_collection`）而非 `}`。
- **插件改动需重装才生效**：插件是子进程 + ZIP 分发，改代码后必须 `build.ps1` 重新打包 + 主程序重装（"修复"或重装 ZIP），仅 `go build` 不重装不会生效。
- **子进程 panic 未必有清晰上报**：插件 init 期 panic（如包级 `regexp.MustCompile` 失败）子进程直接退出，主程序日志的 `plugin.xxx.exe: panic` 行是定位关键。

## 十六、最佳实践与陷阱

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
14. **站点数据源先 curl 验证**：写代码取站点 API/页面前，先 curl 确认响应结构与字段名（见第十五节），勿盲猜字段——字段名常与文档/社区资料不符，且数据源可能内容稀缺（字段在但值空，兜底处理）。
15. **风控失败非致命回退**：作者/标签富信息（简介/等级）被站点风控挡住时，回退基本字段（name + homepage），不因富信息缺失让 `CreateWorkInfo` 整体失败（见 7.2、第六节 CreateWorkInfo 落库）。
16. **登录态加密持久化 + 未登录引导**：cookie/credential 用 `SetValueEncrypted` 加密存储跨重启；`Create` 检测未登录时异步弹登录窗 + 返回"登录后重试"，不阻塞主线程（见 7.3）。
17. **HTTP Transport 分离 + 代理决策**：API 路径（风控敏感）与下载路径（重连代价高）用不同 Transport；代理走"显式设置 > 系统代理(注册表) > env"，`DisableKeepAlives` 默认开、连接复用 opt-in（见 7.1）。
18. **`ExecuteScript` 有 UAF 风险**：注入窗口内容改用 `data:URL` Navigate，不要 `ExecuteScript(document.write)`（见第十节）。
