# Slot 配置格式重构计划

## 目标

将 `extensions.slots[]` 的扁平结构重构为两层：
- **第一层**：所有 slotType 共有的通用属性
- **content**：因 slotType 而异的专属配置

## 字段归属分析

### 当前所有字段

| 字段 | embed | panel | view | menu | siteBrowserList |
|------|-------|-------|------|------|-----------------|
| `id` | ✓ | ✓ | ✓ | ✓ | ✓ |
| `name` | ✓ | ✓ | ✓ | ✓ | ✓ |
| `description` | ✓ | ✓ | ✓ | ✓ | ✓ |
| `slotType` | ✓ | ✓ | ✓ | ✓ | ✓ |
| `order` | ✓ | ✓ | ✓ | ✓ | ✓ |
| `icon` | - | - | - | ✓ | ✓ |
| `contentType` | ✓ | ✓ | ✓ | - | - |
| `content`（组件源） | ✓ | ✓ | ✓ | - | - |
| `position` | ✓ | ✓ | - | - | - |
| `width` | - | ✓ | - | - | - |
| `height` | - | ✓ | - | - | - |
| `title` | - | - | ✓ | - | - |
| `props` | ✓ | ✓ | ✓ | - | - |
| `viewId` | - | - | - | ✓ | - |
| `children` | - | - | - | ✓ | - |
| `contributionId` | ✓ | - | - | - | ✓ |

### 归类结果

**通用属性（第一层）**：`id`、`name`、`description`、`slotType`、`order`

**icon 的处理**：`icon` 仅 menu 和 siteBrowserList 使用。作为显示元数据放入对应类型的 content 中，而非第一层。embed/panel/view 不需要 icon，它们的展示完全由渲染组件决定。

## 新的 plugin.json 格式

### embed

```json
{
  "id": "classify-panel",
  "name": "目录分类",
  "description": "扫描时交互式目录分类面板",
  "slotType": "embed",
  "order": 1,
  "content": {
    "contentType": "precompiled",
    "source": {"js": "views/classify/classify-panel.js", "css": "views/classify/style.css"},
    "position": "dialog",
    "contributionId": "main",
    "props": {}
  }
}
```

### panel

```json
{
  "id": "detail-panel",
  "name": "作品详情",
  "slotType": "panel",
  "content": {
    "contentType": "precompiled",
    "source": {"js": "views/detail/detail.js", "css": "views/detail/style.css"},
    "position": "right-sidebar",
    "width": 400,
    "props": {}
  }
}
```

### view

```json
{
  "id": "browser-view",
  "name": "Pixiv 浏览器",
  "slotType": "view",
  "content": {
    "contentType": "precompiled",
    "source": {"js": "views/browser/browser.js", "css": "views/browser/style.css"},
    "title": "Pixiv",
    "props": {}
  }
}
```

### menu

```json
{
  "id": "pixiv-menu",
  "name": "Pixiv 工具",
  "slotType": "menu",
  "content": {
    "icon": "assets/menu-icon.png",
    "viewId": "browser-view"
  }
}
```

带子菜单：
```json
{
  "id": "tools-menu",
  "name": "工具集",
  "slotType": "menu",
  "content": {
    "icon": "assets/tools-icon.png",
    "children": [
      {"id": "tool-a", "name": "工具 A", "slotType": "menu", "content": {"viewId": "tool-a-view"}},
      {"id": "tool-b", "name": "工具 B", "slotType": "menu", "content": {"viewId": "tool-b-view"}}
    ]
  }
}
```

### siteBrowserList

```json
{
  "id": "pixiv-browser",
  "name": "Pixiv",
  "slotType": "siteBrowserList",
  "content": {
    "icon": "assets/icon.png",
    "contributionId": "main"
  }
}
```

### 各 content 格式汇总

| slotType | content 字段 |
|----------|-------------|
| `embed` | `contentType`, `source`, `position`, `contributionId?`, `props?` |
| `panel` | `contentType`, `source`, `position`, `width?`, `height?`, `props?` |
| `view` | `contentType`, `source`, `title?`, `props?` |
| `menu` | `icon?`, `viewId?`, `children?`（children 为递归 slot 声明） |
| `siteBrowserList` | `icon`, `contributionId` |

### source 字段格式（与 contentType 对应）

| contentType | source 格式 |
|-------------|------------|
| `precompiled` | `{"js": "path/to/file.js", "css": "path/to/style.css"}` |
| `vueSource` | `{"entry": "path/to/Component.vue"}` |
| `html` | `{"html": "path/to/file.html"}` |
| `code` | 行内 JS 字符串 |

## 数据流变更

```
plugin.json         → SlotDeclaration（新结构，content 嵌套）
                       ↓ app.go 解析：按 slotType 解析 content 到扁平字段
SlotConfig（不变）   → 内部领域模型，保持扁平
                       ↓ SlotConfigToResponse
SlotResponse（不变） → IPC DTO，保持扁平
                       ↓ 前端
useSlotSyncListener（不变）→ 前端加载逻辑，保持扁平
```

**核心原则**：只改 plugin.json 的声明格式和解析层（`SlotDeclaration` → `SlotConfig` 的转换），内部模型和前端不变，最小化影响范围。

## 代码变更清单

### 1. 后端 - DTO 层

**`backend/base/model/dto/plugin_types.go`**

`SlotDeclaration` 简化为：
```go
type SlotDeclaration struct {
    ID          string          `json:"id"`
    Name        string          `json:"name"`
    Description string          `json:"description,omitempty"`
    SlotType    string          `json:"slotType"`
    Order       int             `json:"order,omitempty"`
    Content     json.RawMessage `json:"content"`
}
```

新增按 slotType 解析的结构体：
```go
// EmbedSlotContent embed 类型配置
type EmbedSlotContent struct {
    ContentType    string          `json:"contentType"`
    Source         json.RawMessage `json:"source"`
    Position       string          `json:"position"`
    ContributionId string          `json:"contributionId,omitempty"`
    Props          json.RawMessage `json:"props,omitempty"`
}

// PanelSlotContent panel 类型配置
type PanelSlotContent struct {
    ContentType string          `json:"contentType"`
    Source      json.RawMessage `json:"source"`
    Position    string          `json:"position"`
    Width       *int            `json:"width,omitempty"`
    Height      *int            `json:"height,omitempty"`
    Props       json.RawMessage `json:"props,omitempty"`
}

// ViewSlotContent view 类型配置
type ViewSlotContent struct {
    ContentType string          `json:"contentType"`
    Source      json.RawMessage `json:"source"`
    Title       string          `json:"title,omitempty"`
    Props       json.RawMessage `json:"props,omitempty"`
}

// MenuSlotContent menu 类型配置
type MenuSlotContent struct {
    Icon     string           `json:"icon,omitempty"`
    ViewId   string           `json:"viewId,omitempty"`
    Children []SlotDeclaration `json:"children,omitempty"`
}

// SiteBrowserListSlotContent siteBrowserList 类型配置
type SiteBrowserListSlotContent struct {
    Icon          string `json:"icon,omitempty"`
    ContributionId string `json:"contributionId"`
}
```

### 2. 后端 - 解析层

**`app.go`** 的 slot 注册循环（331-360行）：

替换逐字段赋值为按 slotType 解析 content 并映射到 `SlotConfig` 扁平字段。大致逻辑：

```go
for _, slot := range ext.Slots {
    slotConfig := base.NewSlotConfig()
    // 通用字段
    slotConfig.Metadata.ID = slot.ID
    slotConfig.Metadata.Name = slot.Name
    slotConfig.Metadata.Description = slot.Description
    slotConfig.Metadata.PluginID = p.GetID()
    slotConfig.Metadata.PluginPublicID = publicId
    slotConfig.SlotType = base.SlotType(slot.SlotType)
    slotConfig.Order = slot.Order

    // 按 slotType 解析 content
    switch slotConfig.SlotType {
    case base.SlotTypeEmbed:
        var c dto.EmbedSlotContent
        json.Unmarshal(slot.Content, &c)
        slotConfig.ContentType = base.ContentType(c.ContentType)
        slotConfig.Content = resolveSourceURLs(c.Source, c.ContentType, publicId, version)
        slotConfig.Position = c.Position
        slotConfig.ContributionId = c.ContributionId
        slotConfig.Props = c.Props
    case base.SlotTypePanel:
        // 类似 embed，额外处理 width/height
    case base.SlotTypeView:
        // 类似 embed，额外处理 title
    case base.SlotTypeMenu:
        var c dto.MenuSlotContent
        json.Unmarshal(slot.Content, &c)
        if c.Icon != "" {
            slotConfig.Icon = app.StaticResourceService.ResolveURL(publicId, version, c.Icon)
        }
        slotConfig.ViewId = c.ViewId
        slotConfig.Children = convertSlotChildren(c.Children, p.GetID(), publicId, version)
    case base.SlotTypeSiteBrowserList:
        var c dto.SiteBrowserListSlotContent
        json.Unmarshal(slot.Content, &c)
        if c.Icon != "" {
            slotConfig.Icon = app.StaticResourceService.ResolveURL(publicId, version, c.Icon)
        }
        slotConfig.ContributionId = c.ContributionId
    }

    // 注册到 SlotRegistry ...
}
```

**`resolveContentURLs` 函数**：

当前签名 `resolveContentURLs(content, contentType, publicId, version)` 对 content 中的 `map[string]string` 做路径转换。重构后 `source` 字段格式不变（仍是 `map[string]string`），该函数改名为 `resolveSourceURLs`，接收 `source json.RawMessage`，逻辑不变。

### 3. 后端 - 不变的部分

- `backend/base/slot.go`（`SlotConfig`）— 保持扁平，不变
- `backend/slot/dto.go`（`SlotResponse`）— 保持扁平，不变
- `backend/slot/handler.go` — 不变
- `backend/plugin/extension/slot_registry.go` — 不变

### 4. 前端 - 不变

前端的 `SlotResponse`、`useSlotSyncListener.ts`、`SlotConfigs.ts`、`SlotTypes.ts` 均保持不变。因为 `SlotConfig` 和 `SlotResponse`（IPC DTO）维持扁平结构，前端收到的数据格式不变。

### 5. 插件 - 更新 plugin.json

- `plugin/package/lvfeng/com.lvfeng.localImport_.../1.0.0/plugin.json`
- `plugin/package/lvfeng/com.lvfeng.pixivSuite_.../1.0.0/plugin.json`

按新格式重写 `extensions.slots`。

### 6. 文档 - 更新

- `.claude/rules/plugin.md` — 更新 plugin.json 结构说明和 content 字段格式表格

## 影响范围

| 层 | 文件 | 变更程度 |
|----|------|---------|
| DTO | `backend/base/model/dto/plugin_types.go` | 重构 SlotDeclaration + 新增 Content 结构体 |
| 解析 | `app.go` | 重写 slot 注册循环 |
| 解析 | `app.go` resolveContentURLs | 改名 resolveSourceURLs，签名微调 |
| 内部模型 | `backend/base/slot.go` | 不变 |
| IPC | `backend/slot/dto.go` | 不变 |
| 前端 | 全部 | 不变 |
| 插件 | 两个 plugin.json | 格式重写 |
| 文档 | `.claude/rules/plugin.md` | 更新 |
