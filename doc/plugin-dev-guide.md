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
  "id": "com.author.myPlugin",
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
| `id` | string | 是 | 插件标识（**= publicId 身份键**，反向域名格式 `com.作者.名称`，安装期强校验字符集与段结构） |
| `name` | string | 是 | 显示名称 |
| `version` | string | 是 | 语义化版本 |
| `author` | string | 是 | 作者（纯展示属性，不参与身份键） |
| `description` | string | 否 | 描述 |
| `entryFile` | string | 条件必填 | 可执行文件名（运行时插件必填，纯 UI 插件不需要） |
| `activation.type` | number | 是 | `0`=手动激活，`1`=启动时自动激活 |
| `contractVersion` | number | 是 | 编译期契约版本（与主程序协商，见「契约版本协商」）；当前 = 3 |
| `configSchemaVersion` | number | 否 | 配置 schema 版本（0/缺省=legacy 不管理；启用配置迁移时从 1 起递增，见 8.3）。与 contractVersion 正交：前者管插件配置结构，后者管 host↔plugin 协议 |
| `capabilities` | string[] | 否 | 可选能力声明（封闭枚举，见「能力声明」）；如 `["workOrderQuery"]` |
| `extensions` | object | 是 | 扩展点集合（见下） |

> 身份键与五条版本轴（version/contractVersion/configSchemaVersion/plugin_data schemaVersion/buildId）的全貌与变更时机速查，见第十八节。

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
| `resourceViewer` | `{contentType, source, resourceType, props?}` | 资源渲染器（Handler 被动响应型；resourceType = 资源类型查找键，渲染器接收 `{context: render.Context}` props——独立于主程序展示 DTO 的断链契约，见「资源渲染器契约」） |

**contentType 与 source 格式：**

| contentType | source 格式 | 说明 |
|---|---|---|
| `precompiled` | `{js, css?}` | 预编译 JS/CSS（推荐，性能最佳） |
| `vueSource` | `{vue, js?, css?}` | Vue SFC：`vue` 为源文件，`js/css` 为可选预编译缓存（存在则跳过运行时编译） |
| `code` | JS 代码字符串（行内） | 通过 `new Function` 执行，**不注入 Vue/WailsRuntime** |
| `html` | `{html}` | HTML 文件 |

> source 中的相对路径（`code` 类型除外）由后端自动转为 `http://wails.localhost:{port}/plugin/{author}/{id}/{cacheKey}/...` 完整 URL（cacheKey 为缓存键 = plugin.json `buildId`，未打标包回落 version）。

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

### 契约版本协商

`contractVersion` 是插件与主程序之间的**业务契约版本**（整数），与 go-plugin 的传输层 `ProtocolVersion` 分工（传输握手 / 业务契约）。主程序持有 `currentContractVersion`（首发 1）与 `minSupportedContractVersion`（首发 1），插件 manifest 声明自己编译时锁定的 `contractVersion`。

**校验**（安装期预检 + 加载期终检，硬拒绝 + 清晰提示）：
- 插件 `contractVersion` > 主程序 `current` → 插件太新，拒（提示升级主程序）。
- 插件 `contractVersion` < 主程序 `minSupported` → 插件太旧，拒（提示升级插件）。
- 缺字段（`contractVersion` 缺省 = 0）→ 视为当前契约放行（兼容旧/手编插件）。

**跟随 SDK**：插件作者按 SDK 的 `ContractVersion` 常量（`github.com/lvfeng-z/library-squirrel-sdk/transport.ContractVersion`）填 manifest 即可，无需自行判断。bump（提升契约版本）只在破坏性变更时由 SDK 侧发起（proto 加字段不 bump；删/改字段、改 DTO 结构/RPC 签名/前端 props 契约才 bump）。

### 能力声明

`capabilities` 是内置枚举数组，声明插件提供的**可选能力**（区别于 `extensions` 扩展点实例）。主程序查声明才调用对应接口或解析对应声明段。当前 2 值：

| 能力 | 含义 | 对应接口/声明 |
|---|---|---|
| `workOrderQuery` | 提供作品集作品原站序查询 | `WorkOrderQuerier`（TaskHandler 可选扩展） |
| `resourceTypeProvider` | 提供自定义资源类型声明 | manifest `resourceTypes` 段（见「自定义资源类型声明」） |

实现 `WorkOrderQuerier` 的插件（如 pixiv）须声明 `"capabilities": ["workOrderQuery"]`；声明自定义资源类型的插件须含 `"resourceTypeProvider"`（主程序据此解析 `resourceTypes` 段）。未声明者省略或 `[]`。能力不单独版本化，演进由全局 `contractVersion` 兜底。

### 自定义资源类型声明（resourceTypes 段）

插件可声明**自定义资源类型**，经主程序注册进 `ResourceTypeRegistry` 后，插件 Create 即可声明该类型资源（`TaskCreateResponse.ResourceType` 填声明的类型值）。要求主程序 `contractVersion`≥3。

**声明方式**：plugin.json 顶层加 `resourceTypes` 段 + `capabilities` 含 `resourceTypeProvider` 通行证：

```json
{
  "capabilities": ["resourceTypeProvider"],
  "resourceTypes": [
    {
      "type": "com.example.interactiveNovel",
      "roles": [
        {"storeType": "document", "min": 1, "max": 1},
        {"storeType": "image", "min": 0, "max": 0}
      ],
      "primaryRoles": ["document"]
    }
  ]
}
```

**字段**：
- `type`：类型值，**强制反向域名前缀**（如 `com.example.xxx`），禁止裸通用词（防抢占内置名）。插件 Create 时填此值。
- `roles`：结构角色 + 基数。`storeType` 必须 ∈ 内置 7 角色（`image`/`document`/`thumbnail`/`videoTrack`/`audioTrack`/`videoMain`/`audioMain`）；**插件自定义 store 角色当前不支持**（延后）。`min`=最少数量(0=可选,1=必含)，`max`=最多数量(0=不限,1=单例)。
- `primaryRoles`：展示主体优先级链，每项须在 `roles.storeType` 集合内。

**注册时强校验**（守卫严格识别不变量；坏 spec 拒绝并记日志跳过，不株连插件其他能力）：
- `type` 缺反向域名前缀 → 拒；`roles.storeType` 非 7 角色 / `min`>`max` / `primaryRoles` 不在 roles → 拒。
- 两插件声明同 `type` → 后注册者拒 + 日志告警。

**渲染/完整性自动跟随**：自定义类型注册后，前端 resourceViewer 按该类型查找插件 `resourceViewer` 渲染器（命中覆盖内置，无则降级 UnknownRenderer）；完整性按 `roles` 基数自动判定。完整规约见 `doc/resource-type-spec.md` 第七节。

### 资源渲染器契约（render.Context）

`resourceViewer` 扩展点的渲染器组件接收 `{context: render.Context}` props（运行时注入），**不是**主程序的 `WorkFullDTO`/`ResourceFullDTO`。`render.Context` 是 SDK 定义的插件渲染契约类型（`github.com/lvfeng-z/library-squirrel-sdk/dto/render`）：

- **独立断链**：初始字段集对齐主程序 `WorkFullDTO`（含 work/site/authors/tags/resource 全量），此后独立演进——主程序展示 DTO 变更不传导至 `render.Context`，破坏性变更由 `contractVersion` 保护。
- **前端引用**：插件前端组件（TS）从 `@bindings/.../library-squirrel-sdk/dto/render` 引用 `Context` 类型。Go 侧映射权威在主程序（`dto.ToRenderContext`），插件不写映射。

插件作者按 `render.Context` 结构开发渲染器，不要依赖主程序展示 DTO 的字段细节（那些会随主程序 UI 演进而变）。

### 共享枚举常量

store_type / resource_type / generation 的字符串值**单一真相源**在 SDK `contract` 子包（`github.com/lvfeng-z/library-squirrel-sdk/contract`）。插件经 SDK dto 包的常量别名引用：

- `sdkdto.StoreRoleImage`/`StoreRoleDocument`/`StoreRoleThumbnail`/`StoreRoleVideoTrack`/`StoreRoleAudioTrack`/`StoreRoleVideoMain`/`StoreRoleAudioMain`（store_type；dto 保留 StoreRole 旧名）
- `sdkdto.ResourceTypeImage`/`Video`/`Article`/`Document`/`Audio`/`Unknown`（resource_type）
- `sdkdto.GenerationDownloaded`/`GenerationDerived`（generation）

**禁止硬编码字面量**（`"image"`/`"videoTrack"`/`"downloaded"` 等）——改 SDK contract 一处即经编译期同步到所有引用点；硬编码绕过编译期保护，升级时易漂移。

## 四、插件入口

运行时插件的 `main` 函数调用 `pluginsdk.Serve`：

```go
func main() {
    pluginsdk.Serve(&MyTaskHandler{},
        pluginsdk.WithBrowser(&MySiteBrowser{}),       // 可选，注册 SiteBrowser
        pluginsdk.WithActivate(func(ctx sdkdto.PluginContext) {
            // 在此注册扩展点、监听 URL 等（站点行由主程序按 identity 注册表自动投影，插件无需建站）
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

`ctx sdkdto.PluginContext` 是插件访问主程序能力的**唯一入口**，共 22 个方法：

| 分类 | 方法 | 签名 |
|---|---|---|
| 扩展点注册 | `RegisterTaskHandler` | `(id, name, desc string, handler TaskHandler) error` |
| | `RegisterSiteBrowser` | `(id, name, desc string, browser SiteBrowser) error` |
| | `UnregisterSiteBrowser` | `(id string) error` |
| 自存信息 | `GetValue` | `(key string) (*StorageValue, error)`（`StorageValue.Value` 明文 + `SchemaVersion`，见 8.3；key 不存在返回 nil） |
| | `SetValue` | `(key, value string) error` |
| | `SetValueEncrypted` | `(key, value string) error` |
| | `DeleteValue` | `(key string) error` |
| | `GetAllValues` | `() (map[string]*StorageValue, error)` |
| 任务 | `RegisterUrlListener` | `(extensionId string, patterns []string) error` |
| | `UnregisterUrlListener` | `(extensionId string) error`（空则清该插件全部监听） |
| | `CreateTask` | `(url string) (*CreateTaskResult, error)` |
| 前端通信 | `PublishToFrontend` | `(topic string, data []byte) error` |
| | `SubscribeFrontend` | `(topic string) (<-chan []byte, error)` |
| | `UnsubscribeFrontend` | `(topic string) error` |
| 路径 | `GetPluginRoot` | `(isRelative bool) string` |
| | `GetStoreRelPath` | `(taskId int64, role string, storeSeq int) (string, error)` — 查询当前任务资源中指定 store 的真实落盘路径（workDir 相对）；插件 Start 时资源尚未创建，故按 `taskId` 查、主程序映射到当前 `PendingResourceID`。供 document lazy 生成等路径可知后按真实文件名引用兄弟文件 |
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

#### Create 返回的任务结构契约(重要)

`Create` 返回 `*TaskCreateResult`,内含一串 `*TaskCreateResponse`(`sdkdto.BatchResult([...])` 批量 / `sdkdto.StreamResult(ch)` 流式二选一)。**每个 `TaskCreateResponse` 是一个自洽的工作单元**,主程序按其 `Children` 是否为空落盘:

| 响应结构 | 落盘任务 | 计入 AddedQuantity |
|---|---|---|
| **无 Children**(独立 leaf) | 1 个任务:pid=0、HasChild=false | 1 |
| **有 Children**(任务组) | 1 个 parent(HasChild=true、pid=0)+ 每个 child(pid=parent.id、HasChild=false) | len(Children)(parent 容器不计) |

- **不折叠**:即便 `Children` 只有 1 个,也建 parent+1child(2 任务),不把单 child 提升为 leaf。parent 是"作品/目录"容器、child 是"可下载单元",语义不同。
- **字段落位**:parent 容器只取 `TaskName`/`Url`/`SiteKey`/`InvolvedRoles`/`ResourceType`(不带 `SiteWorkId`/`PluginData`);leaf 与 child 取全部身份字段(含 `SiteWorkId`/`PluginData`)。`SiteKey` 必填(用 `identity.*` 常量,见「十九、站点注册与站点身份」)——缺失则该响应被跳过(不入库);child 未显式带键时继承 parent 的键。`SiteName` 不参与身份判定(主程序忽略,可不填)。
- **流式合并(stream 专属)**:一个超大 work 可拆成多个响应流式发,**复用同一 `PluginTaskId`**——主程序把同 `PluginTaskId` 的连续响应合并到同一 parent(children 累加、不重复建 parent)。`PluginTaskId` 是插件稳定的 work 标识;**勿用 `TaskName` 当合并键**(展示名易重复,且 local 一次导入内 `TaskName` 恒定,用它会误把不同目录焊成一个 work)。批量(array)路径不合并,每个响应须自洽。流式适合"work 数量多想渐进反馈"或"单 work 巨大需拆分"的场景(如本地导入含大量文件的目录)。

四例:

| 场景 | 交付 | 响应结构 | 落盘 | 计数 |
|---|---|---|---|---|
| 本地单文件导入 | stream,1 响应 | 独立 leaf(无 Children) | 1 leaf | 1 |
| 本地目录 5 文件 | stream,1 响应 | 任务组 Children=[5] | 1 parent + 5 child | 5 |
| pixiv 单图作品 | array,1 响应 | 任务组 Children=[1] | 1 parent + 1 child | 1 |
| pixiv 5 图作品 | array,1 响应 | 任务组 Children=[5] | 1 parent + 5 child | 5 |

> 站点类插件(如 pixiv)始终发任务组(作品概念,单图也是 Children=[1]);本地单文件发独立 leaf。这是**插件编码的选择**,主程序规则统一(尊重编码、不替它折叠)。一次 `Create` 可返回多个响应(如本地选多个目录),每个独立按上表落盘。

#### 创建期声明涉及板块(InvolvedRoles)

`Create` 返回的 `TaskCreateResponse`(及子任务 `TaskCreateChildResponse`)可声明本任务涉及的 store_type 集合 `InvolvedRoles`(universe),供主程序与前端按任务感知:

- **声明 = hint,Start 产出 = truth**:`InvolvedRoles` 用于前端重执行 UI 只展示该任务涉及的板块、以及主程序首跑默认请求集;主程序挂载以插件 `Start` 实际产出的 StoreSpec 为准,**不据 universe 拒绝超出声明的 spec**(创建期声明不全时仍健壮)。
- **空/nil = 未确定(default)**:创建期信息不足以判定时留空,执行期插件按全量(空 storeRoles)自决产出;前端对 default 任务展示兜底集。
- **声明时机**:能据 URL 即定则定(如纯图片站点恒为 `[image]`,可省去无意义的缩略图派生请求);需深度元数据才能定则留空走 default。

例:纯图片站点 `Create` 声明 `InvolvedRoles=[image]` → 首跑 Start 请求集仅 `[image]`,不触发缩略图派生轨;重执行 UI 仅显示「图片」一项。

#### 声明资源类型(ResourceType,必填)

`TaskCreateResponse` / `TaskCreateChildResponse` 的 `ResourceType` 字段**必须声明**内置值之一(`sdkdto.ResourceTypeImage`/`Video`/`Article`/`Document`/`Audio`/`Unknown`)或插件经 `resourceTypes` 段注册的自定义类型。主程序**严格识别,不推断、不兜底**:

- `ResourceType` 空 / 未注册值 → 写入路径抛错(任务创建/执行失败)
- `StoreSpec.Role` 非 `image`/`document`/`thumbnail`/`videoTrack`/`audioTrack`/`videoMain`/`audioMain` 七角色 → 抛错
- `unknown` 是合法显式值(插件确实无法分类时声明),不报错

资源类型决定 store 角色组合(结构规约)+ 展示主体优先级 + 文件标准。完整规约见 **`doc/resource-type-spec.md`**。例:图片资源声明 `ResourceType: sdkdto.ResourceTypeImage`,Start 产出 `Role: sdkdto.StoreRoleImage`(+ 可选 `StoreRoleThumbnail`)。**禁止硬编码字面量**——store_type/resource_type/generation 一律用 SDK dto 常量(`sdkdto.StoreRole*`/`ResourceType*`/`Generation*`，真相源在 SDK `contract` 子包)，改一处即处处同步;硬编码绕过编译期保护，升级时易漂移（见「共享枚举常量」）。

#### StoreSpec(资源产出声明)

```go
type StoreSpec struct {
    Role        string        // store_type: image | document | thumbnail | videoTrack | audioTrack | videoMain | audioMain
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

#### plugin_data 格式版本约定

`TaskCreateResponse.PluginData` 是插件自定义的不透明字符串，主程序不解析其内容（仅原样存入 task 记录、执行时原样回传，见上一小节数据流）。插件升级后，新版本 TaskHandler 可能需要识别旧版本写入的 PluginData 格式。约定插件**自行管理 PluginData 的格式版本**：

1. **写入**：序列化 PluginData 时在 JSON 顶层带版本字段（建议命名 `schemaVersion`，整数，初值 1）。
2. **读取兜底**：反序列化后按 `schemaVersion` 分支解析；**字段缺失（旧数据）按默认旧格式（v0）兜底**，不报错。
3. **未知高版本 fail-fast**：读到高于自身支持的 `schemaVersion`（数据由更新版本插件写入；含用户从高版本回滚到低版本插件的场景）时返回明确错误（如"任务数据由更高版本插件创建，请升级插件或重建任务"），禁止尽力解析以防静默数据损坏。**此为插件自决的约定，主程序无法强制**——主程序不解析 PluginData 内容，是否实现完全由插件负责。
4. **递增时机**：`schemaVersion` 仅在 PluginData 结构**破坏性变更**（删字段/改字段语义/改类型）时递增；加可选字段不必递增（向前兼容）。
5. **责任声明**：PluginData 的格式兼容性与跨版本迁移由**插件自负**，主程序不解析、不裁决、不强制迁移。

> 与 `contractVersion`（host↔plugin 协议代际，见「契约版本协商」）和 `configSchemaVersion`（插件配置 KV 结构，见 8.3）是三个独立维度：contractVersion 管 RPC/proto 契约、configSchemaVersion 管 `plugin_storage` 配置、PluginData 的 `schemaVersion` 管 task 内部数据格式，互不耦合。

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
v, _ := ctx.GetValue("downloadPath")        // v 是 *StorageValue：v.Value 为明文，v.SchemaVersion 为写入时的配置 schema 版本
path := ""
if v != nil { path = v.Value }

// 加密（敏感数据，存密文、读自动解密）
ctx.SetValueEncrypted("apiToken", secret)
av, _ := ctx.GetValue("apiToken")           // 自动解密，av.Value 为明文
token := ""
if av != nil { token = av.Value }

// 删除 / 全部
ctx.DeleteValue("downloadPath")
all, _ := ctx.GetAllValues()                // map[key]*StorageValue（加密项已解密）
```

读取（`GetValue`/`GetAllValues`）对加密项**透明解密**，返回 `*StorageValue`（含明文 `Value` 与 `SchemaVersion`，见 8.3）；key 不存在时 `GetValue` 返回 `nil`。插件只在**写入**时分 `SetValue`/`SetValueEncrypted`。

### 8.2 用户设置（settings）

在 `plugin.json` 声明 `extensions.settings` 后：
- 主程序在插件管理页渲染表单（按 `type` 分发控件、按 `group` 分组），用户编辑后由主程序按声明的 `encrypted` 路由 `SetValue`/`SetValueEncrypted` 存入。
- 插件用 `ctx.GetValue(key).Value` 读取用户配置值（统一为 string，integer 等类型自行转换）；`GetValue` 返回 `*StorageValue`，key 不存在时为 `nil`。

### 8.3 配置 schema 版本与迁移

插件升级时旧版本写入的配置（settings 与自存 KV）仍保留在 `plugin_storage`（升级保留 `plugin_id`、不清数据）。若新版本改了配置结构（key 改名、值格式变、加密预期变），旧值会**语义丢失**——读到旧值喂入新逻辑，静默错配。`configSchemaVersion`（plugin.json 顶层整数）让插件感知并迁移旧配置：

- 每条值写入时由主程序盖 `schema_version` 戳（= 插件当前声明的 `configSchemaVersion`）；插件 `GetValue` 时拿到每条值的 `SchemaVersion`。
- 插件在 `Activate` 开头用 SDK 助手 `plugin.MigrateConfig` 自行迁移。

```go
import "github.com/lvfeng-z/library-squirrel-sdk/plugin"

func Activate(ctx pluginsdk.PluginContext, ...) {
    // 第一件事：配置迁移（幂等，best-effort）
    plugin.MigrateConfig(ctx, 3, map[int]plugin.MigrateStep{
        2: func(ctx pluginsdk.PluginContext) error { // v1 → v2：dlQuality 改名 downloadQuality
            old, _ := ctx.GetValue("dlQuality")
            if old != nil && old.SchemaVersion < 2 {
                ctx.SetValue("downloadQuality", old.Value) // 主程序自动盖新版本戳
                ctx.DeleteValue("dlQuality")
            }
            return nil
        },
        3: func(ctx pluginsdk.PluginContext) error { /* v2 → v3 */ return nil },
    })
    // 注册扩展点、读配置 ...
}
```

要点：
- **`configSchemaVersion=0`（缺省）= legacy**：不参与版本管理，主程序不检测/告警，插件也不调 `MigrateConfig`——行为同未启用。要用迁移从 `1` 起递增。
- **best-effort + 幂等重跑**：某步报错时跳过该 key、记日志、继续激活（不阻断）；下次 `Activate` 重跑时已迁 key（版本=目标）自动跳过——断点续传。此重跑**与任务模块的失败重跑无关**。
- **不保证跨 key 原子性**：联动配置迁移中存在暂时不一致窗口，靠幂等重跑最终一致；建议联动 key 放同一迁移步。
- **降级**（行版本 > 声明，如装旧版覆盖新版）：主程序告警、不自动迁移。
- 主程序激活后会扫 `plugin_storage` 行，存在 `schema_version < configSchemaVersion` 的行则日志告警（迁移未完成或插件未声明 `MigrateConfig`），作为安全网。

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
http://wails.localhost:{backend-port}/plugin/{publicId}/{cacheKey}/{relativePath}
```

- `publicId` 即插件 `id`（反向域名，URL 中占一段）。
- `cacheKey` 为缓存键 = plugin.json `buildId`（构建身份标识，构建时由 build.ps1 以 `git describe` 注入；未打标包回落 `version`）。构建变化必换 URL/ETag，长缓存随之失效。
- 后端在注册前端扩展时自动把 `source`/`icon` 的相对路径转为上述完整 URL，前端组件直接用。
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
- **升级**：通过「修复」（`Reinstall` 从备份恢复）或「选择安装包修复」（`ReinstallFromPath` 指定新 ZIP）。流程：停旧版运行时并删除旧版目录（`deactivate`，仅停进程与删文件，**不标记卸载、不清插件配置**）→ 以重装模式解压新版、复用同 `publicId` 的 DB 记录（保留 ID 与 CreateTime，覆盖版本/路径等字段）→ 激活新版。**升级保留插件的 `plugin_storage` 配置数据**（仅替换程序文件与 DB 记录字段），为后续配置版本迁移提供前提。

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

## 十七、插件信任模型（来源追溯与运行门控）

主程序对第三方插件采用**来源追溯 + 知情同意 + 运行门控**的最小信任模型。插件作者需了解：

- **来源由主程序判定，不需插件声明**：插件来源（bundled 官方捆绑 / local 本地安装 / url / marketplace）由主程序按安装入口自动判定并写入 `plugin` 表，**`plugin.json` 无需也无可声明 source/trust/integrity 字段**——这类自声明可被伪造，不作信任锚。
- **安装时用户知情同意**：用户安装第三方插件（非官方捆绑）时，主程序弹窗告知「该插件将获得完整宿主能力（读写资源库、创建任务、网络、原生窗口）」并由用户确认。确认后插件标记为 trusted；绕过确认的异常安装标记为未信任。
- **未信任插件不运行**：`trusted=false` 的插件**不会被激活**（不启动子进程）。用户需在插件管理页手动「信任」后才运行——这是主程序与用户之间的信任契约，不经插件代码，插件作者无能为力。
- **运行后能力仍全开**：一旦 trusted 运行，插件经 `PluginContext` 拥有的全部宿主能力（数据读写、任务、网络、窗口）**不受信任状态裁剪**。当前模型不做沙箱隔离与能力裁剪（完整沙箱属后续规划）；即用户「信任」即等于授予完整权限，**请你在插件文档中如实说明插件的权限行为**。
- **构建身份（buildId）**：构建管线（build.ps1）自动把 `git describe` 输出写入 plugin.json `buildId`，主程序以它判同构建（捆绑插件升级检测）并作前端资产缓存键。插件作者无需手写、不应手改（见第十八节）。
- **受限模式**：用户可开启「受限模式」（设置页开关），启用后启动时仅激活官方捆绑插件、跳过所有第三方——用于排查问题时的安全启动。第三方插件在受限模式下不运行。
17. **HTTP Transport 分离 + 代理决策**：API 路径（风控敏感）与下载路径（重连代价高）用不同 Transport；代理走"显式设置 > 系统代理(注册表) > env"，`DisableKeepAlives` 默认开、连接复用 opt-in（见 7.1）。
18. **`ExecuteScript` 有 UAF 风险**：注入窗口内容改用 `data:URL` Navigate，不要 `ExecuteScript(document.write)`（见第十节）。
19. **manifest 声明 contractVersion**：发布前确认 `contractVersion` 与目标主程序契约版本一致（首发 1，跟随 SDK `transport.ContractVersion`）；不声明或版本不匹配会被主程序拒绝加载（见「契约版本协商」）。
20. **能力声明与实现一致**：实现 `WorkOrderQuerier` 须声明 `"capabilities": ["workOrderQuery"]`；声明而未实现、或实现而未声明，均不符契约（见「能力声明」）。
21. **resourceViewer 用 render.Context**：插件资源渲染器 props 是 `{context: render.Context}`（非主程序 `WorkFullDTO`）；类型从 SDK `dto/render` 引用，禁用主程序展示 DTO 替代（见「资源渲染器契约」）。
22. **共享枚举用 SDK 常量禁字面量**：store_type/resource_type/generation 一律用 `sdkdto.*` 常量，禁硬编码字面量（见「共享枚举常量」）。

## 十八、身份与版本体系速查（开发者视角）

整套体系是「**一个身份键 + 五条版本轴**」：身份键回答"这是哪个插件"，五条轴各自回答一个独立的版本问题。核心纪律：**每条轴只在它回答的那个问题变化时才动，不混用**。

### 身份键：id（= publicId），一次性决定，终身不变

- `id` 即全局身份键 publicId。格式为反向域名 `com.<你拥有的域名>.<插件名>`（个人开发者可用 `io.github.<用户名>.<插件名>`），至少两段 label、字符集限字母/数字/连字符/点——安装期强校验，含下划线、斜杠或单段的 id 直接被拒。
- **第一个版本发布后 id 永不改**：id 变更等于做了一个新插件（新记录、新目录、新身份），旧的用户设置（plugin_storage）与任务关联不会跟随。
- id 全局唯一，请用自己拥有的域名，防止与他人插件撞车。
- `author` 是纯展示属性：改名随意，对身份零影响。

### 五条版本轴：改了什么，才动什么

| 你改了什么 | 动哪条轴 | 谁维护 | 忘了动的后果 |
| --- | --- | --- | --- |
| 纯代码修复 / 内部重构 | 什么都不动 | — | 无后果，buildId 自动感知 |
| 功能发布、行为变化 | `version` | 插件作者 | 失去人读语义与升降级排序线索 |
| SDK 协议（gRPC 接口）破坏性变更 | `contractVersion`（见第三节「契约版本协商」） | 插件作者与主程序协商 | 主程序加载时拒绝（fail-fast），不会静默出错 |
| settings 设置项结构变化 | `configSchemaVersion`（见 8.3 配置迁移） | 插件作者 | 旧配置感知不到结构变化，`MigrateConfig` 不触发 |
| 任务 PluginData JSON 格式变化 | PluginData 顶层 `schemaVersion`（见第六节「plugin_data 格式版本约定」，插件完全自管） | 插件作者 | 旧代码读到高版本数据 fail-fast 拒绝执行，防静默损坏 |
| 任何源码状态变化 | `buildId` 自动变 | 构建管线（build.ps1 以 `git describe` 打标） | — |

### 三个判同信号的分工

- 判「**是不是同一份构建**」→ buildId：机器判同，驱动捆绑升级检测（已装与 zip 的 buildId 不一致即重装）与前端资产缓存失效（重构建必换 URL/ETag，见第十一节）。
- 判「**升还是降**」→ version：语义排序，人读。
- 判「**能否兼容运行**」→ contractVersion：协议边界，过新/过旧均拒绝。

### 开发迭代流

```
改代码 → 跑 build.ps1（自动打标 buildId）→ 主程序重启
  → buildId 与已装不同 → 自动升级重装 → 落新目录、清旧目录
  → plugin_storage 用户设置/凭据保留（按记录 ID 关联，重装不丢）
```

发布节奏：`version` bump + commit + 打 tag（如 `v1.2.0`，buildId 转为 tag 基准、获得人读语义）→ `task build:plugins` 产出 zip（产物校验 buildId 非空，漏打标整体失败）。

### 两个开发期特有的坑

1. **dirty 盲区**：`git describe --dirty` 只区分"有无未提交改动"——同一 commit 下，前后两天各构建一次 dirty 包，buildId 相同，捆绑升级判定会认为"没变"而漏掉重装。开发期遇到（改了代码但没 commit，升级没触发），用插件管理页的「修复 / 选择安装包修复」手动重装，或 commit 后再构建。
2. **不要手工编辑 zip 里的 buildId**：它同时是前端资产的缓存键，乱改会导致浏览器长缓存不失效（见第十一节缓存说明）。

## 十九、站点注册与站点身份（site_key）

站点身份 = `site_key`（SDK `identity` 注册表分配的品牌 slug，如 `pixiv`，站点在所有库、所有版本中唯一）；站点名纯展示、不参与任何身份判定。完整规范见 `doc/site-identity-spec.md`，本节给插件开发者最短路径。

### 何时需要注册

你的插件要对接**注册表未收录的新站点**时须先完成注册——站点行由主程序在启动期按注册表自动投影建行（`identity.All()` → insert-only，落注册表权威值），插件无需建站也无建站入口；未注册键的产物在导入/分享接收处被直接拒绝（报错文案即注册指引）。已收录站点（pixiv/bilibili/local，清单见 `identity/registry.go`）直接引用常量即可，无需注册。

### 注册步骤（PR 自助）

1. **选定键值**：取站点官方品牌名的稳定 slug（如 `mysite`），满足 `^[a-z][a-z0-9-]{1,30}$`（小写字母开头，小写字母/数字/连字符），不与既有条目近似。
2. **提 PR**：向 `github.com/lvfeng-z/library-squirrel-sdk` 的 `identity/registry.go` 常量块加条目，常量名用站点的 PascalCase 标识，并随 PR 写**站点级 ID 约定注记**（siteWorkId/siteTagId/siteAuthorId/siteWorkSetId 各取站点侧什么稳定 ID——首个接入插件的权威口径声明）：
   ```go
   // Mysite <一句话站点说明>。
   //
   // 站点级 ID 约定注记（同站点多插件产出收敛到同一条 norm）：
   //   siteWorkId    = ...
   //   siteTagId     = ...
   //   siteAuthorId  = ...
   //   siteWorkSetId = ...
   Mysite = Site{Key: "mysite", Name: "mysite", Homepage: "https://example.com"}
   ```
   虚拟站点（无真实主页）`Homepage` 留空串。
3. **维护者合并**：合并时查重（键值、常量名不与既有条目冲突，键值与站点官方品牌名一致）。
4. **生效预期**：注册表随 SDK 发布、主程序随 SDK 更新发布后生效——「合并 + 下次主程序发布」两步时延，注册时预留此窗口。判定：主程序启动后站点管理页经投影出现该站点行，且携带该键的产物导入/分享接收不再报未注册。

注册表只增不改：键一经发布永不重排、永不复用；站点改名只改权威名，键不动。

### 插件内使用 identity.* 常量（官方插件实例）

站点行由主程序启动期按 `identity` 注册表自动投影建行，插件无需建站也无建站入口——插件侧引用 `identity.*` 常量的场景是 Create 应答携带站点键：

**Create 应答带 SiteKey**（pixiv 插件 `task_handler.go`；必填——主程序按键解析站点归属，缺失则该响应被跳过）：

```go
parentTask := &sdkdto.TaskCreateResponse{
    TaskName:   title,
    SiteWorkId: illustID,
    Url:        url,
    SiteKey:    identity.Pixiv.Key,
    // ...其余字段
}
```

### 两条纪律

1. **一律用 `identity.*` 常量，禁止手抄键值字面量**——统一引用防止拼写变体与注册表演进脱节，抄错会静默指向不存在的键而被拒（或指向错误站点）。
2. **站点侧 ID 须为站点侧稳定 ID**（`SiteWorkId`/siteTagId/siteAuthorId/siteWorkSetId 取站点自身体系的持久标识，禁用展示名/列表序号/URL 临时参数）——稳定性契约与治理分层见规范第五、六节。
