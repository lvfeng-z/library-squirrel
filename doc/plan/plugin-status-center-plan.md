# 插件状态中心计划

## 目标

在 PluginManage 页面中，选中插件后右侧展开详情面板，展示四类状态信息：
1. 运行时状态（进程是否在线、PID、激活时间）
2. 扩展点列表（TaskHandler、SiteBrowser、Slot）
3. 存储状态（PluginData 大小）
4. URL 监听规则（已注册的匹配模式）

加密存储统计暂不展示（需改表，后续再做）。

## 前端方案

在 `PluginManage.vue` 中，选中插件后通过 `el-drawer` 从右侧滑出状态面板。

- 使用 `el-drawer` 作为容器，`direction="rtl"`，宽度约 450px
- 选中插件行时打开 drawer，切换选中行时更新内容
- 关闭 drawer 时不影响插件列表布局
- 新增组件：`frontend/src/components/plugin/PluginStatusPanel.vue`，作为 drawer 内容

```
┌──────────────────────────────────┐
│  SearchTable（插件列表）          │
│                                  │
│                                  │
│                                  │
└──────────────────────────────────┘
                                   ┌─ el-drawer (右侧滑出) ──┐
                                   │ ┌─ 运行时状态 ─────────┐ │
                                   │ │ 在线 | PID           │ │
                                   │ │ 激活时间              │ │
                                   │ ├─ 扩展点 ────────────┤ │
                                   │ │ TaskHandlers        │ │
                                   │ │ SiteBrowsers        │ │
                                   │ │ Slots               │ │
                                   │ ├─ 存储状态 ──────────┤ │
                                   │ │ PluginData          │ │
                                   │ ├─ URL 监听 ──────────┤ │
                                   │ │ 模式列表             │ │
                                   │ └────────────────────┘ │
                                   └────────────────────────┘
```

## 后端 API

新增 `PluginStatus` handler 方法，一次性返回插件所有状态数据。

### DTO

`backend/plugin/status.go`：

```go
// PluginStatusDTO 插件状态
type PluginStatusDTO struct {
    // 运行时状态
    IsRunning     bool   `json:"isRunning"`
    PID           int    `json:"pid"`
    ActivatedAt   int64  `json:"activatedAt"`   // Unix 毫秒，0 表示未激活

    // 扩展点列表
    TaskHandlers  []ExtensionInfo `json:"taskHandlers"`
    SiteBrowsers  []ExtensionInfo `json:"siteBrowsers"`
    Slots         []SlotInfo      `json:"slots"`

    // 存储状态
    PluginDataSize int `json:"pluginDataSize"`  // 字节数

    // URL 监听规则
    UrlPatterns   []string `json:"urlPatterns"`
}

// ExtensionInfo 扩展点信息
type ExtensionInfo struct {
    ID          string `json:"id"`
    Name        string `json:"name"`
    Description string `json:"description"`
}

// SlotInfo 插槽信息
type SlotInfo struct {
    ID       string `json:"id"`
    Name     string `json:"name"`
    SlotType string `json:"slotType"`
}
```

### Handler

`backend/plugin/handler.go` 新增方法：

```go
func (h *Handler) GetPluginStatus(ctx context.Context, pluginPublicId string) *model.ApiResponse[*PluginStatusDTO]
```

### Service 依赖收集

`PluginStatusDTO` 的数据来源分散在多个组件中，需要在 Service 层聚合。Service 已有的依赖：
- `repo` — 获取插件实体（PluginData 大小）
- `pluginLoader` — 获取运行时状态（需新增注入）

需要额外注入的依赖（通过接口）：
- `RuntimeStatusProvider` — 从 Loader 获取进程状态
- `ExtensionListProvider` — 从三个 Registry 获取扩展点
- `UrlListenerProvider` — 从 Manager 获取 URL 模式

## 代码变更清单

### 后端

| 文件 | 变更 |
|------|------|
| `backend/plugin/status.go` | 新增 DTO 结构体 |
| `backend/plugin/handler.go` | 新增 `GetPluginStatus` 方法 |
| `backend/plugin/service.go` | 新增 `GetPluginStatus` 方法，聚合各数据源 |
| `backend/plugin/extension/loader.go` | 新增 `GetPluginRuntimeStatus(publicId)` 方法，`pluginEntry` 加 `activatedAt` 字段 |
| `backend/pluginTaskUrlListener/manager.go` | 新增 `ListPatternsByPlugin(publicId)` 方法 |

### 前端

| 文件 | 变更 |
|------|------|
| `frontend/src/components/plugin/PluginStatusPanel.vue` | 新增状态面板组件 |
| `frontend/src/views/PluginManage.vue` | 集成 el-drawer + PluginStatusPanel |
| `frontend/src/apis/http/wrappers/plugin.ts` | 新增 `pluginGetStatus` wrapper |
| `frontend/bindings/...` | 重新生成 Wails bindings |

### 不变

- 数据库 schema 不变
- SlotConfig、SlotResponse 不变
- 插件 SDK 不变

## 实施步骤

1. 后端：`loader.go` — 新增 `pluginEntry.activatedAt` + `GetPluginRuntimeStatus()`
2. 后端：`manager.go` — 新增 `ListPatternsByPlugin()`
3. 后端：`status.go` — 新增 DTO
4. 后端：`service.go` — 新增 `GetPluginStatus()` + 注入依赖接口
5. 后端：`handler.go` — 新增 handler 方法
6. 后端：`app.go` — 注入新依赖
7. 前端：生成 bindings + wrapper
8. 前端：新增 `PluginStatusPanel.vue`
9. 前端：改造 `PluginManage.vue`，集成 el-drawer
