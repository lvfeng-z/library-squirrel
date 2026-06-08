# 视频缩略图方案

> 前置依赖：[PersistentStore 模块](filestore-module-plan.md) ✅ 已实现

## 背景

当前系统中，WorkCard 使用 `<el-image>` 展示作品的封面图，取的是 Resource 的 `workStore.filePath`（通过 PersistentStore 的 HTTP 文件服务 `/store/` 渲染）。对于视频类型资源（`.mp4`, `.avi`, `.mkv` 等），`<el-image>` 无法渲染视频文件，会显示错误图标。

**目标**：主程序在资源下载完成后，通过专用接口向插件请求缩略图数据；插件根据作品类型决定是否提供缩略图，主程序负责持久化存储和前端展示。

**设计决策**：
1. 缩略图通过专用 `GetThumbnail` 接口获取，与 `Start`/`Resume` 解耦
2. 主程序在资源下载完成（EOF 校验通过）后调用 `GetThumbnail`，插件根据作品类型自行决定是否返回缩略图
3. `TaskResourceDTO` 仅清理 proto 遗留字段，Go DTO 保持现有扁平结构不变

## 已完成的基础设施

以下部分在 PersistentStore 迁移和备份集成过程中已同步完成，无需额外工作：

| 组件 | 状态 | 说明 |
|------|------|------|
| Resource 实体 `ThumbnailStoreID` | ✅ | `backend/base/model/entity/resource.go`，`sql.NullInt64` |
| SDK `ResourceFullDTO` | ✅ | 包含 `ThumbnailStoreID *int64` + `ThumbnailStore *PersistentStoreDTO` |
| 作品搜索 JOIN | ✅ | `search/repository.go` 的 `QueryWorkPage` SQL 已 LEFT JOIN `persistent_store ts ON r.thumbnail_store_id = ts.id` |
| WorkSet 封面加载 | ❌ 需修复 | `search/service.go` 的 `QueryWorkSetPage` 仅加载 WorkStore，未加载 ThumbnailStore |
| 备份/恢复处理 | ✅ | `store_backup_orchestrator.go` 按 `StoreTypeThumbnail` 处理 |
| 前端 `buildStoreUrl` | ✅ | `UrlUtil.ts`，构建 `/store/{path}` URL |

## 协议变更

### TaskResourceDTO 清理（proto only）

Proto 中 `TaskResourceDTO` 现有 10 个字段，清理规则：
- `resourceId`、`url`、`localPath`、`remotePath`、`completeness`：遗留字段，Go DTO 未映射、宿主侧未消费，**移除**
- `type`：宿主侧 `model.go` 从未读取此字段（资源类型通过 `PersistentStore.FilenameExtension` 文件扩展名判断），**移除**

保留 4 个有效字段并重编号：

**文件**: `library-squirrel-sdk/proto/plugin.proto`

```protobuf
// 任务资源信息（清理后）
message TaskResourceDTO {
  string format = 1;
  int64 size = 2;
  string suggestName = 3;
  optional bool continuable = 4;
}
```

**文件**: `library-squirrel-sdk/dto/handler_dto.go`

Go DTO 同步移除 `Type` 字段：

```go
type TaskResourceDTO struct {
    Format      string `json:"format"`      // 文件格式/扩展名（如 "jpg"、"mp4"）
    Size        int64  `json:"size"`        // 远程文件大小
    SuggestName string `json:"suggestName"` // 插件建议文件名
    Continuable *bool  `json:"continuable"` // 是否支持续传
}
```

**文件**: `library-squirrel-sdk/transport/plugin_server.go`

`workResponseToProto` 同步移除 `Type` 映射：

```go
if r.Resource != nil {
    pb.Resource = &gen.TaskResourceDTO{
        Format:      r.Resource.Format,
        Size:        r.Resource.Size,
        SuggestName: r.Resource.SuggestName,
        Continuable: r.Resource.Continuable,
    }
}
```

**文件**: `backend/plugin/extension/task_handler_proxy.go`

`protoToWorkResponse` 同步移除 `Type` 映射：

```go
if pb.Resource != nil {
    resp.Resource = &pluginsdkdto.TaskResourceDTO{
        Format:      pb.Resource.Format,
        Size:        pb.Resource.Size,
        SuggestName: pb.Resource.SuggestName,
        Continuable: pb.Resource.Continuable,
    }
}
```

### 新增 GetThumbnail RPC

**文件**: `library-squirrel-sdk/proto/plugin.proto`

```protobuf
// 缩略图请求
message GetThumbnailRequest {
  string taskData = 1;  // 插件在 Create 阶段存储的任务数据（JSON）
}

// 缩略图响应
message GetThumbnailResponse {
  bytes data = 1;     // 缩略图原始字节
  string format = 2;  // 格式扩展名（如 "jpg"、"png"）
}

// 添加到 TaskHandlerService
service TaskHandlerService {
  // ... 现有 RPC ...
  rpc GetThumbnail(GetThumbnailRequest) returns (GetThumbnailResponse);
}
```

**设计说明**：
- `taskData` 是插件在 `Create` 阶段通过 `TaskCreateResponse.Task.PluginData` 存储的 JSON 字符串，包含了插件所需的全部上下文（如缩略图 URL、作品类型等）
- 插件根据 `taskData` 中的作品类型信息自行决定是否返回缩略图数据
- 如果插件认为不需要缩略图（如普通图片作品），返回空的 `GetThumbnailResponse`（data 为空字节）

## SDK 变更

### 新增 DTO

**文件**: `library-squirrel-sdk/dto/handler_dto.go`

```go
// ThumbnailResponse 缩略图响应
type ThumbnailResponse struct {
    Data   []byte `json:"data"`   // 缩略图原始字节
    Format string `json:"format"` // 格式扩展名
}
```

### TaskHandler 接口新增方法

**文件**: `library-squirrel-sdk/dto/task_handler.go`

```go
type TaskHandler interface {
    // ... 现有 7 个方法 ...

    // GetThumbnail 获取缩略图
    // taskData: 插件在 Create 阶段存储的任务数据（JSON）
    // 返回缩略图数据或 nil（插件决定不提供缩略图时返回 nil）
    GetThumbnail(taskData string) (*ThumbnailResponse, error)
}
```

### SDK 传输层实现

**文件**: `library-squirrel-sdk/transport/plugin_server.go`

在 `taskHandlerServer` 结构体上实现 `GetThumbnail` RPC：

```go
func (s *taskHandlerServer) GetThumbnail(ctx context.Context, req *gen.GetThumbnailRequest) (*gen.GetThumbnailResponse, error) {
    if s.handler == nil {
        return nil, status.Error(codes.Unimplemented, "handler not registered")
    }
    resp, err := s.handler.GetThumbnail(req.TaskData)
    if err != nil {
        return nil, err
    }
    if resp == nil {
        return &gen.GetThumbnailResponse{}, nil
    }
    return &gen.GetThumbnailResponse{
        Data:   resp.Data,
        Format: resp.Format,
    }, nil
}
```

## 宿主侧变更

### TaskExecutor 接口新增方法

**文件**: `backend/taskManager/model.go`

```go
type TaskExecutor interface {
    // ... 现有 5 个方法 ...

    // GetThumbnail 获取缩略图
    // taskData: 插件在 Create 阶段存储的任务数据（JSON）
    GetThumbnail(ctx context.Context, taskData string) (*sdkdto.ThumbnailResponse, error)
}
```

### task_handler_proxy.go 实现

**文件**: `backend/plugin/extension/task_handler_proxy.go`

```go
func (p *TaskHandlerProxy) GetThumbnail(ctx context.Context, taskData string) (*sdkdto.ThumbnailResponse, error) {
    resp, err := p.client.GetThumbnail(ctx, &gen.GetThumbnailRequest{
        TaskData: taskData,
    })
    if err != nil {
        return nil, err
    }
    if len(resp.Data) == 0 {
        return nil, nil
    }
    return &sdkdto.ThumbnailResponse{
        Data:   resp.Data,
        Format: resp.Format,
    }, nil
}
```

### saveThumbnail 方法（新增）

在 `downloadLoop` 的 EOF 处理中，下载完成校验通过后、`setState(TaskStateFinished)` 之前新增调用：

```go
// === 新增：向插件请求缩略图并保存 ===
m.saveThumbnail()
```

方法实现：

```go
func (m *ManagedTask) saveThumbnail() {
    // 1. 前置检查：已有缩略图或无插件数据时跳过
    if m.resource.ThumbnailStoreID.Valid {
        return
    }
    if !m.task.PluginData.Valid || m.task.PluginData.String == "" {
        return
    }

    // 2. 调用插件获取缩略图
    thumbResp, err := m.pluginExec.GetThumbnail(m.ctx, m.task.PluginData.String)
    if err != nil {
        logger.Log.Warnf("缩略图获取失败: %v", err)
        return
    }
    if thumbResp == nil || len(thumbResp.Data) == 0 {
        return
    }

    // 3. 确定格式
    thumbFormat := thumbResp.Format
    if thumbFormat == "" {
        thumbFormat = "jpg"
    }

    // 4. 获取资源文件信息，构建缩略图相对路径和文件名
    store, err := m.storeReader.GetById(m.ctx, m.workStoreId)
    if err != nil || store == nil {
        logger.Log.Warnf("缩略图生成跳过: 获取 store 记录失败: %v", err)
        return
    }
    thumbRelPath := buildThumbnailRelPath(store.FilePath.String, thumbFormat)
    thumbFileName := buildThumbnailFileName(store.FileName.String, thumbFormat)

    // 5. 通过 Store 一步完成写入
    storeID, err := m.thumbnailStoreWriter.Store(
        m.ctx, thumbRelPath, thumbFileName, bytes.NewReader(thumbResp.Data),
    )
    if err != nil {
        logger.Log.Warnf("缩略图存储失败: %v", err)
        return
    }

    // 6. 更新 Resource 记录
    m.resource.ThumbnailStoreID = sql.NullInt64{Int64: storeID, Valid: true}
    m.resourceUpdater.Update(m.ctx, m.resource)
}
```

### 新增依赖：ThumbnailStoreWriter

在 `taskManager/model.go` 中定义接口（由 `persistentStore.Service` 实现）：

```go
// ThumbnailStoreWriter 缩略图存储接口，接受 io.Reader 一步完成存入
type ThumbnailStoreWriter interface {
    Store(ctx context.Context, relPath string, fileName string, reader io.Reader) (int64, error)
}
```

`PersistentStoreService.Store()` 方法签名完全匹配，无需适配器。

**依赖注入变更**：

- **`NewManagedTask`** 新增第 16 个参数 `thumbnailStoreWriter ThumbnailStoreWriter`
- **`NewManager`** 新增第 15 个参数 `thumbnailStoreWriter ThumbnailStoreWriter`
- **`newManagedTask`** 透传 `m.thumbnailStoreWriter`
- **`app.go`** 接线：

```go
app.TaskManagerService = taskManager.NewManager(
    // ... 现有参数 ...
    app.PersistentStoreService,  // StoreStreamer
    app.PersistentStoreService,  // StoreReader
    app.PersistentStoreService,  // ThumbnailStoreWriter ← 新增
)
```

### 辅助函数

```go
// buildThumbnailRelPath 构建缩略图相对路径
// "author/video.mp4", "jpg" → "thumbnail/author/video_thumbnail.jpg"
func buildThumbnailRelPath(resourceRelPath string, thumbFormat string) string {
    ext := filepath.Ext(resourceRelPath)
    base := strings.TrimSuffix(resourceRelPath, ext)
    return "thumbnail/" + base + "_thumbnail." + thumbFormat
}

// buildThumbnailFileName 构建缩略图文件名
// "video.mp4", "jpg" → "video_thumbnail.jpg"
func buildThumbnailFileName(resourceFileName string, thumbFormat string) string {
    ext := filepath.Ext(resourceFileName)
    base := strings.TrimSuffix(resourceFileName, ext)
    return base + "_thumbnail." + thumbFormat
}
```

### 修复：WorkSet 封面图加载 ThumbnailStore

**文件**: `backend/search/service.go`

`QueryWorkSetPage` 方法的 Phase 4.5 仅收集了 `WorkStoreID` 批量查询 PersistentStore，Phase 5 组装时 `thumbnailStore` 传了 `nil`。需同步收集 `ThumbnailStoreID`：

Phase 4.5 修改：
```go
// 收集所有 Store ID（WorkStoreID + ThumbnailStoreID）
var allStoreIds []int64
for _, resources := range resourcesMap {
    for _, res := range resources {
        if res.WorkStoreID.Valid && res.WorkStoreID.Int64 > 0 {
            allStoreIds = append(allStoreIds, res.WorkStoreID.Int64)
        }
        if res.ThumbnailStoreID.Valid && res.ThumbnailStoreID.Int64 > 0 {
            allStoreIds = append(allStoreIds, res.ThumbnailStoreID.Int64)
        }
    }
}
stores, err := s.storeBatchReader.GetByIds(ctx, allStoreIds)
```

Phase 5 组装修改：
```go
var workStore *entity2.PersistentStore
if res.WorkStoreID.Valid {
    workStore = storeMap[res.WorkStoreID.Int64]
}
var thumbStore *entity2.PersistentStore
if res.ThumbnailStoreID.Valid {
    thumbStore = storeMap[res.ThumbnailStoreID.Int64]
}
item.CoverResource = dto2.NewResourceFullDTO(res, workStore, thumbStore)
```

## 插件侧变更

### 通用：实现 GetThumbnail

所有插件的 `TaskHandler` 需新增 `GetThumbnail` 方法实现。

### pixiv 插件

**`internal/pixivapi/models.go`** — 扩展 API 模型以解析缩略图 URL：

```go
type ImageURLs struct {
    Original string `json:"original"`
    Small    string `json:"small"`    // 新增
}

type AppImageURLs struct {
    Original string `json:"original"`
    Small    string `json:"small"`    // 新增
}
```

**`internal/model/task_plugin_data.go`** — PluginData 新增缩略图 URL：

```go
type TaskPluginData struct {
    // ... 现有字段 ...
    ThumbnailURL string `json:"thumbnailUrl"` // 新增
}
```

**`task_handler.go`** — 两处改动：

1. **`Create`**：从 API 响应提取缩略图 URL 存入 PluginData（仅对动图、视频、文章类型设置，普通图片作品不设置 `ThumbnailURL`）
2. **`GetThumbnail`（新增）**：根据 PluginData 中的 `ThumbnailURL` 下载缩略图

```go
func (h *PixivTaskHandler) GetThumbnail(taskData string) (*sdkdto.ThumbnailResponse, error) {
    var data model.TaskPluginData
    if err := json.Unmarshal([]byte(taskData), &data); err != nil {
        return nil, err
    }
    // 普通图片作品无 ThumbnailURL，返回 nil 表示不需要缩略图
    if data.ThumbnailURL == "" {
        return nil, nil
    }
    // 下载缩略图
    thumbData, err := h.downloadThumbnail(data.ThumbnailURL)
    if err != nil {
        return nil, fmt.Errorf("缩略图下载失败: %w", err)
    }
    return &sdkdto.ThumbnailResponse{
        Data:   thumbData,
        Format: "jpg",
    }, nil
}
```

### local 插件

**`GetThumbnail`（新增）**：local 插件需要为动图、视频、文章类型的本地文件生成缩略图，但具体生成方式（如调用外部工具、内嵌缩略图库等）待定。当前阶段返回 nil（不提供缩略图）。

```go
func (h *LocalTaskHandler) GetThumbnail(taskData string) (*sdkdto.ThumbnailResponse, error) {
    // TODO: 为视频/动图/文章类型的本地文件生成缩略图
    return nil, nil
}
```

## 前端变更

### 非图片资源拦截

当资源无缩略图且资源本身不是图片类型时，直接将 `el-image` 的 `src` 设为空字符串，阻止其发起 HTTP 请求。否则 `<el-image>` 会尝试加载视频/PDF 等大文件，消耗大量时间并造成卡顿后才显示 error 占位。

通过 `workStore.filenameExtension` 判断资源类型：

```typescript
// 前端工具函数
const IMAGE_EXTENSIONS = new Set(['.jpg', '.jpeg', '.png', '.gif', '.webp', '.bmp'])

function isDisplayableImage(extension: string | null | undefined): boolean {
  if (!extension) return false
  return IMAGE_EXTENSIONS.has(extension.toLowerCase())
}
```

### WorkCard.vue

```typescript
const imagePath: Ref<string> = computed(() => {
  const resource = props.work.resource
  // 1. 优先使用缩略图
  if (resource?.thumbnailStore?.filePath) {
    return resource.thumbnailStore.filePath
  }
  // 2. 无缩略图时，仅对图片类型的资源返回路径；非图片类型返回空，阻止 el-image 加载
  if (resource?.workStore?.filePath && isDisplayableImage(resource.workStore.filenameExtension)) {
    return resource.workStore.filePath
  }
  return ''
})
```

### WorkSetCard.vue

```typescript
const coverFilePath: Ref<string> = computed(() => {
  const resource = props.workSet.coverResource
  // 1. 优先使用缩略图
  if (resource?.thumbnailStore?.filePath) {
    return resource.thumbnailStore.filePath
  }
  // 2. 无缩略图时，仅对图片类型返回路径
  if (resource?.workStore?.filePath && isDisplayableImage(resource.workStore.filenameExtension)) {
    return resource.workStore.filePath
  }
  return ''
})
```

当 `imagePath` / `coverFilePath` 为空时，`el-image` 的 `:src=""` 会直接触发 error 插槽显示 `Picture` 图标占位，无需等待加载失败。

## 修改文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| **协议 / SDK** | | |
| `plugin-sdk/proto/plugin.proto` | 修改 | `TaskResourceDTO` 清理遗留字段和 `type`；新增 `GetThumbnail` RPC 及请求/响应消息 |
| `plugin-sdk/dto/handler_dto.go` | 修改 | 移除 `Type` 字段；新增 `ThumbnailResponse` DTO |
| `plugin-sdk/dto/task_handler.go` | 修改 | `TaskHandler` 接口新增 `GetThumbnail` 方法 |
| `plugin-sdk/transport/plugin_server.go` | 修改 | 移除 `Type` 映射；实现 `GetThumbnail` RPC 分发 |
| **后端** | | |
| `backend/plugin/extension/task_handler_proxy.go` | 修改 | 移除 `Type` 映射；实现 `GetThumbnail` 方法（调用 gRPC + 转换响应） |
| `backend/taskManager/model.go` | 修改 | `TaskExecutor` 接口新增 `GetThumbnail`；新增 `ThumbnailStoreWriter` 接口、`saveThumbnail` 及辅助函数 |
| `backend/taskManager/manager.go` | 修改 | `NewManager`/`newManagedTask` 透传 `thumbnailStoreWriter` |
| `backend/search/service.go` | 修改 | `QueryWorkSetPage` Phase 4.5 收集 `ThumbnailStoreID`，Phase 5 组装 thumbnailStore |
| `app.go` | 修改 | `NewManager` 调用新增 `PersistentStoreService` 参数 |
| **插件（pixiv）** | | |
| `plugin-pixiv-go/internal/pixivapi/models.go` | 修改 | `ImageURLs`/`AppImageURLs` 新增 `Small` 字段 |
| `plugin-pixiv-go/internal/model/task_plugin_data.go` | 修改 | 新增 `ThumbnailURL` 字段 |
| `plugin-pixiv-go/task_handler.go` | 修改 | 移除 `Type` 字段引用；`Create` 提取缩略图 URL；新增 `GetThumbnail` 实现 |
| **插件（local）** | | |
| `plugin-local/task_handler.go` | 修改 | 移除 `Type` 字段引用；新增 `GetThumbnail` 实现（当前返回 nil） |
| **前端** | | |
| `frontend/src/components/common/WorkCard.vue` | 修改 | 缩略图优先 + 非图片资源拦截 |
| `frontend/src/components/common/WorkSetCard.vue` | 修改 | 同上 |

## 执行注意事项

### 执行顺序

改动存在依赖链，需按以下顺序执行：

```
① SDK proto（清理 + 新增 GetThumbnail）+ DTO + TaskHandler 接口 + 传输层（4 个文件）
② protoc 重新生成 gen/plugin.pb.go
③ 宿主传输层 task_handler_proxy.go（移除 Type 映射 + 实现 GetThumbnail）
④ 宿主 taskManager model.go / manager.go（接口 + saveThumbnail + 依赖注入）
⑤ app.go 接线
⑥ 宿主 search/service.go（WorkSet 封面加载 ThumbnailStore）
⑦ 插件 pixiv / local（①②完成后才能编译）
⑧ 前端（独立于后端，可与⑦并行）
```

### Proto 重新生成

修改 `plugin-sdk/proto/plugin.proto` 后，需运行 protoc 重新生成 `plugin-sdk/gen/plugin.pb.go` 和 `plugin-sdk/gen/proto/plugin.pb.go`。执行方式参见 SDK 项目的构建脚本或 Makefile。

### PersistentStore 子目录

`thumbnail` 子目录已在 `backend/persistentStore/dir.go` 的 `registeredDirs` 中注册，无需额外操作。

### 预存编译问题

当前两个插件使用了 SDK Go DTO 中不存在的字段：

- pixiv 插件 `task_handler.go:369` 使用 `URL`、`RemotePath`（当前 SDK DTO 无此字段）
- local 插件 `task_handler.go:251` 使用 `URL`、`LocalPath`（当前 SDK DTO 无此字段）

这些是旧版 proto 的遗留引用。本次 proto 清理会确认这些字段已被移除，两个插件需同步清理遗留引用。**建议先完成 SDK 改动，再一次性更新所有插件。**

### 对比原方案的变化

| 项 | 原方案 | 调整后 |
|----|--------|--------|
| 缩略图传输时机 | 嵌入 Start/Resume 响应 | 资源下载完成后单独调用 GetThumbnail |
| TaskResourceDTO 结构 | 重构为组合 `{Work, Thumbnail}` | 仅清理 proto 遗留字段，Go DTO 不变 |
| model.go 字段访问路径 | `.Resource.X` → `.Resource.Work.X` | 无变化 |
| buildSuggestedFileName 签名 | 改为接收 `*WorkResourceDTO` | 不变 |
| resumeFromPersistedState | 需改为嵌套 DTO 构造 | 不变 |
| 插件现有方法适配 | Start/Resume 需适配新 DTO 结构 | 不变 |
