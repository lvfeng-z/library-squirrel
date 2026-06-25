# 插件静态资源服务与声明式插槽注册重构

## Context

当前插件系统的 Slot 注册由插件运行时通过 `PluginContext.RegisterSlot()` 执行，存在以下问题：
1. 纯 UI 插件也需要加载 DLL/子进程，浪费资源
2. 资源加载方式不统一（ReadVueFile、resource:// 协议、eval）
3. ContentType 前后端定义不一致
4. 增加后续子进程迁移的复杂度

**本次重构目标**：将 Slot 注册改为配置文件声明式，由主程序读取 `plugin.json` 直接注册；新增静态资源服务模块统一托管插件资源文件；使纯 UI 插件无需加载运行时即可完成注册。这是后续子进程迁移的前置步骤。

## 设计决策

| 决策 | 选择 |
|------|------|
| 配置文件位置 | 扩展 `plugin.json` 的 `extensions` 为结构化对象（与主程序"扩展点"命名一致） |
| 资源服务方式 | 扩展 Wails asset handler，使用 `resource://plugin/{publicId}/...` |
| 路径安全 | 仅允许访问 `staticResources` 中声明的目录 |
| ContentType 统一 | `vueSource` / `precompiled` / `code` / `html`（移除 `component`） |
| 缓存策略 | URL 含版本号 + ETag + Cache-Control immutable |
| 纯 UI 插件判定 | contributes 中无 taskHandler 和 siteBrowser |
| 安装时验证 | 不验证资源文件存在性，验证 id 合法性和 contributes 非空 |

## plugin.json Schema 变更

### Before
```json
{
  "contributes": [
    { "type": "taskHandler", "id": "pixiv-task" },
    { "type": "slot", "id": "pixiv-slot" }
  ]
}
```

### After
```json
{
  "entryFile": "entry.dll",
  "extensions": {
    "taskHandlers": [
      { "id": "pixiv-task", "name": "Pixiv任务" }
    ],
    "siteBrowsers": [
      { "id": "pixiv-browser", "name": "Pixiv浏览器" }
    ],
    "slots": [
      {
        "id": "pixiv-slot",
        "slotType": "siteBrowserList",
        "name": "Pixiv入口",
        "title": "Pixiv",
        "icon": "assets/icon.png",
        "contributionId": "pixiv-browser",
        "order": 0
      },
      {
        "id": "pixiv-browser-view",
        "slotType": "view",
        "name": "Pixiv浏览器",
        "contentType": "precompiled",
        "content": { "js": "views/browser.js", "css": "views/browser.css" }
      },
      {
        "id": "pixiv-menu",
        "slotType": "menu",
        "name": "Pixiv工具",
        "icon": "assets/menu-icon.png",
        "viewId": "pixiv-browser-view",
        "order": 10
      }
    ],
    "staticResources": { "directories": ["views/", "assets/"] }
  }
}
```

### Slot 各类型字段

| 字段 | siteBrowserList | view | panel | menu | embed |
|------|:-:|:-:|:-:|:-:|:-:|
| id / name / slotType | ✓ | ✓ | ✓ | ✓ | ✓ |
| contentType / content | - | ✓ | ✓ | - | ✓ |
| title / icon / order | ✓ | - | - | opt | - |
| contributionId | ✓ | - | - | - | - |
| position | - | - | ✓ | - | ✓ |
| width / height | - | - | opt | - | opt |
| viewId | - | - | - | opt | - |
| children | - | - | - | opt | - |
| props | - | opt | opt | - | opt |

### ContentType 与 content 字段结构

| ContentType | content 结构 | 说明 |
|-------------|-------------|------|
| `vueSource` | `{ "vue": "path.vue", "js?": "path.js", "css?": "path.css" }` | Vue SFC 源码 |
| `precompiled` | `{ "js": "path.js", "css?": "path.css" }` | 预编译 JS/CSS |
| `code` | 行内 JS 代码字符串 | 通过 new Function() 执行 |
| `html` | `{ "html": "path.html" }` | HTML 内容 |

## 后端变更

### Go 类型定义

**`backend/base/model/dto/plugin_types.go`** — 重构核心类型

```go
// 新增：PluginExtensions 替代旧的 []PluginContribute
type PluginExtensions struct {
    TaskHandlers    []TaskHandlerDeclaration `json:"taskHandlers,omitempty"`
    SiteBrowsers    []SiteBrowserDeclaration `json:"siteBrowsers,omitempty"`
    Slots           []SlotDeclaration        `json:"slots,omitempty"`
    StaticResources *StaticResourcesConfig   `json:"staticResources,omitempty"`
}

// SlotDeclaration 插槽声明（plugin.json 中每个 slot 条目）
type SlotDeclaration struct {
    ID            string          `json:"id"`
    Name          string          `json:"name"`
    Description   string          `json:"description,omitempty"`
    SlotType      string          `json:"slotType"`
    ContentType   string          `json:"contentType,omitempty"`
    Content       json.RawMessage `json:"content,omitempty"`
    Title         string          `json:"title,omitempty"`
    Icon          string          `json:"icon,omitempty"`
    Order         int             `json:"order,omitempty"`
    Position      string          `json:"position,omitempty"`
    Width         *int            `json:"width,omitempty"`
    Height        *int            `json:"height,omitempty"`
    Props         json.RawMessage `json:"props,omitempty"`
    ViewId        string          `json:"viewId,omitempty"`
    ContributionId string         `json:"contributionId,omitempty"`
    Children      []SlotDeclaration `json:"children,omitempty"`
}

type TaskHandlerDeclaration struct { ID, Name, Description string }
type SiteBrowserDeclaration struct { ID, Name, Description string }
type StaticResourcesConfig struct { Directories []string `json:"directories"` }

// PluginManifest.Extensions 类型变更（原 Contributes）
type PluginManifest struct {
    // ... 其他字段不变
    Extensions *PluginExtensions `json:"extensions"` // 从 []PluginContribute 改为 *PluginExtensions
}
```

**`backend/base/slot.go`** — SlotConfig 扩展 + ContentType 更新

```go
type ContentType string
const (
    ContentTypeVueSource   ContentType = "vueSource"
    ContentTypePrecompiled ContentType = "precompiled"
    ContentTypeCode        ContentType = "code"
    ContentTypeHTML        ContentType = "html"
    // 移除 ContentTypeComponent
)

type SlotConfig struct {
    *pkgmodel.ExtensionMetadata
    SlotType       SlotType
    Content        json.RawMessage // 从 string 改为 json.RawMessage
    ContentType    ContentType
    Title          string
    Icon           string
    Order          int
    // 新增字段
    Position       string
    Width          *int
    Height         *int
    Props          json.RawMessage
    ViewId         string
    ContributionId string
}
```

### 新增文件

**`backend/plugin/extension/static_resource_service.go`**

```go
type StaticResourceService struct {
    mu      sync.RWMutex
    plugins map[string]*pluginResourceMapping // key: pluginPublicId
}

type pluginResourceMapping struct {
    rootPath    string   // 插件根目录绝对路径
    allowedDirs []string // staticResources 中声明的允许目录
    version     string   // 版本号（用于缓存）
}

func NewStaticResourceService() *StaticResourceService
func (s *StaticResourceService) RegisterPlugin(publicId, absRootPath string, allowedDirs []string, version string)
func (s *StaticResourceService) UnregisterPlugin(publicId string)
func (s *StaticResourceService) ServeHTTP(w http.ResponseWriter, r *http.Request)
```

`ServeHTTP` 逻辑：
1. 解析 URL 路径 `/plugin/{publicId}/{version}/{relativePath}`
2. 查找 publicId → pluginResourceMapping
3. 安全校验：`filepath.Clean` + 无 `..` + 在 allowedDirs 前缀内 + 最终路径在 rootPath 下
4. 设置 Content-Type（按扩展名）、Cache-Control（`immutable`）、ETag
5. `http.ServeFile` 返回文件

**`backend/plugin/extension/asset_handler.go`**

```go
type PluginAwareAssetHandler struct {
    frontendHandler http.Handler              // 嵌入式前端资源
    pluginService   *StaticResourceService    // 插件资源
}

func (h *PluginAwareAssetHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    if strings.HasPrefix(r.URL.Path, "/plugin/") {
        h.pluginService.ServeHTTP(w, r)
        return
    }
    h.frontendHandler.ServeHTTP(w, r)
}
```

### 修改文件

**`app.go`**
- 新增 `StaticResourceService` 字段
- `NewApp()` 中创建 `StaticResourceService`（在 registries 之后、services 之前）
- 新增 `CreateAssetHandler(assets fs.FS) http.Handler` 方法
- **重写 `loadInstalledPlugins()`**：

```
for each plugin in DB (not uninstalled):
  1. 读取磁盘上的 plugin.json
  2. 解析为 PluginManifest
  3. 判断是否为纯 UI 插件：无 taskHandlers && 无 siteBrowsers
  4. 注册静态资源：StaticResourceService.RegisterPlugin(...)
  5. 声明式注册 Slot：遍历 manifest.Extensions.Slots
     → 构建 SlotConfig → SlotRegistry.Register()
     → WailsSlotPusher 推送事件到前端
  6. 非纯 UI 插件：构建 PluginContext → Loader.LoadPlugin()
```

**`main.go`**
- 将 `Assets` 从 `application.AssetFileServerFS(assets)` 替换为 `app.CreateAssetHandler(assets)`

**`backend/plugin/service.go`**
- `loadPluginPackage()`：更新验证逻辑适配新的 `*PluginExtensions` 结构
  - `Extensions` 不可为 nil
  - 至少包含 taskHandlers / siteBrowsers / slots 之一
  - `EntryFile` 仅在有 taskHandlers 或 siteBrowsers 时必填
- **移除 `ReadVueFile()` 方法**

**`backend/plugin/handler.go`**
- **移除 `ReadVueFile` handler 方法**

**`backend/plugin/extension/registrar.go`**
- 从 `Registrar` 接口移除 `RegisterSlot` 方法
- 从 `registrar` 实现移除 `RegisterSlot`

**`backend/plugin/extension/plugin_context.go`**
- 从 `pluginContext` 移除 `UnregisterSlot`
- `PluginContextDeps` 中移除 `SlotRegistry`（slot 注册不再经过 PluginContext）

## 前端变更

**`frontend/src/model/model/constant/SlotTypes.ts`**
```typescript
export type SlotContentType = 'vueSource' | 'precompiled' | 'code' | 'html'
export type AnySlotContent = PrecompiledContent | VueSourceContent | HtmlContent | string
```

**`frontend/src/model/model/interface/SlotConfigs.ts`**
```typescript
export interface HtmlContent {
  html: string // HTML 文件相对路径
}
```

**`frontend/src/composables/useSlotSyncListener.ts`** — 核心变更

1. `loadCompiledComponent`：URL 构建改为 `resource://plugin/${pluginPublicId}/${version}/${jsPath}`
2. `loadVueSourceComponent`：用 `fetch(resource://plugin/...)` 替代 `PluginHandler.ReadVueFile()`
3. 新增 `html` 类型处理：`fetch` HTML 文件内容后渲染
4. 移除 `PluginHandler.ReadVueFile` 相关导入和调用

**`frontend/src/views/SiteBrowserManage.vue`**
- 图标路径更新为完整 `resource://plugin/{publicId}/{version}/...` URL

**`frontend/src/store/SlotRegistryStore.ts`**
- `SiteBrowserListSlotItem.imagePath` 改为存储完整 URL

## Plugin SDK 变更 (`library-squirrel-sdk`)

- 从 `PluginContext` 接口移除 `RegisterSlot` / `UnregisterSlot`
- 移除 `SlotType` / `ContentType` 类型定义（已改为配置驱动）

## 实施阶段

### Phase 1：后端类型重构（无行为变更）
1. 修改 `plugin_types.go`：新增 `PluginExtensions`、`SlotDeclaration` 等类型，暂保留旧 `PluginContribute`
2. 修改 `slot.go`：`Content` 改为 `json.RawMessage`，新增 `ContentTypeHTML`，新增 SlotConfig 扩展字段

### Phase 2：静态资源服务基础设施
3. 新建 `static_resource_service.go`
4. 新建 `asset_handler.go`
5. 修改 `app.go`：创建 StaticResourceService
6. 修改 `main.go`：替换 asset handler
- **验证**：应用启动正常，前端页面加载正常

### Phase 3：声明式 Slot 注册
7. 修改 `plugin_types.go`：`Extensions` 类型从 `[]PluginContribute` 切换到 `*PluginExtensions`
8. 修改 `plugin/service.go`：更新安装验证、移除 ReadVueFile
9. 修改 `plugin/handler.go`：移除 ReadVueFile
10. 重写 `app.go` 的 `loadInstalledPlugins()`：声明式注册 + 纯 UI 插件跳过 DLL
11. 修改 `registrar.go`：移除 RegisterSlot
12. 修改 `plugin_context.go`：移除 slot 相关方法
- **验证**：安装测试插件，slot 注册成功推送至前端

### Phase 4：前端适配
13. 修改 `SlotTypes.ts` + `SlotConfigs.ts`：新增 html 类型
14. 修改 `useSlotSyncListener.ts`：资源 URL 改为 resource://、移除 ReadVueFile
15. 修改 `SiteBrowserManage.vue`：图标 URL 更新
16. 修改 `SlotRegistryStore.ts`：imagePath 改为完整 URL
- **验证**：插件 Vue 组件通过 resource:// URL 正常加载渲染

### Phase 5：SDK 与清理
17. 更新 `library-squirrel-sdk`：移除 slot 相关接口和类型
18. 删除 `ContentTypeComponent` 相关代码
19. 运行 `wails3 generate bindings -ts` 重新生成绑定
20. 更新 `doc/ai-assistant/` 文档

## 验证方式

1. **编译验证**：每个 Phase 完成后确保 Go 编译通过
2. **启动验证**：应用正常启动，前端页面正常加载（Phase 2 后）
3. **静态资源**：手动注册测试目录，验证 `resource://plugin/...` 可访问文件、路径穿越返回 404
4. **声明式注册**：创建仅含 slot 声明的 plugin.json，验证 slot 推送到前端且无 DLL 加载
5. **资源加载**：插件 view 组件通过 resource:// URL 加载 Vue/JS/CSS/HTML
6. **绑定生成**：`wails3 generate bindings -ts` 成功
7. **端到端**：完整插件安装 → slot 注册 → 前端渲染 → 卸载 → 资源不可访问
