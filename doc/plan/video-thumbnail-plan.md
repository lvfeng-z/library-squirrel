# 视频缩略图方案

> 前置依赖：[PersistentStore 模块](filestore-module-plan.md) 已实现

## 背景

当前系统中，WorkCard 使用 `<el-image>` 展示作品的封面图，取的是第一个 `enabled=true` 的 Resource 的 `filePath`。对于视频类型资源（`.mp4`, `.avi`, `.mkv` 等），`<el-image>` 无法渲染视频文件，会显示错误图标。

**目标**：视频类型作品在主页、作品集等页面中，以静态缩略图（从视频中截取一帧）的方式展示。

## 数据模型变更

### Resource 实体新增字段

```go
// backend/base/model/entity/resource.go
type Resource struct {
    // ... 现有字段 ...
    ThumbnailStoreID sql.NullInt64 `gorm:"column:thumbnail_store_id" json:"thumbnailStoreId"`
}
```

- 指向 `persistent_store.id`，仅在视频资源时有值
- GORM AutoMigrate 自动添加列

### SDK ResourceDTO 新增字段

```go
type ResourceDTO struct {
    // ... 现有字段 ...
    ThumbnailStoreID *int64  `json:"thumbnailStoreId"`
    ThumbnailPath    *string `json:"thumbnailPath"` // join persistent_store 冗余字段
}
```

## 缩略图生成

### 集成点

**文件**: `backend/taskManager/model.go` 的 `downloadLoop` 方法

在下载完成（`readErr == io.EOF`）后、`setState(TaskStateFinished)` 之前，新增缩略图生成逻辑：

```go
if readErr == io.EOF {
    m.currentFile.Sync()
    // 校验完整性...
    m.clearPendingResourceID()

    // 生成视频缩略图
    m.generateThumbnailIfNeeded()

    m.setState(TaskStateFinished)
    return runResultDone
}
```

### generateThumbnailIfNeeded 方法

```go
func (m *ManagedTask) generateThumbnailIfNeeded() {
    // 1. 检查扩展名是否为视频类型
    ext := strings.ToLower(m.resource.FilenameExtension.String)
    if !isVideoExtension(ext) {
        return
    }
    // 2. 构建缩略图相对路径
    //    视频路径 "author/video.mp4" → 缩略图路径 "thumbnail/author/video_thumb.jpg"
    thumbRelPath := buildThumbnailRelPath(m.localPath, m.workDirProvider.GetWorkDir())
    // 3. 调用 FFmpeg 截帧到临时文件
    tmpFile, err := m.generateVideoThumbnail(m.localPath)
    if err != nil {
        logger.Log.Warnf("缩略图生成失败: %v", err)
        return // 缩略图失败不影响主流程
    }
    defer os.Remove(tmpFile)
    // 4. 通过 PersistentStore 存入
    storeID, err := m.persistentStore.StoreFromFile(m.ctx, thumbRelPath, buildThumbnailFileName(m.resource), tmpFile)
    if err != nil {
        logger.Log.Warnf("缩略图存储失败: %v", err)
        return
    }
    // 5. 更新 Resource 记录
    m.resource.ThumbnailStoreID = sql.NullInt64{Int64: storeID, Valid: true}
    m.resourceSaver.Update(m.ctx, m.resource)
}
```

### taskManager 依赖

`ManagedTask` 新增 `persistentStore PersistentStoreWriter` 依赖：

```go
type PersistentStoreWriter interface {
    StoreFromFile(ctx context.Context, relPath string, fileName string, srcAbsPath string) (int64, error)
}
```

### FFmpeg 截帧

作为 `taskManager` 或 `persistentStore` 包内的工具函数：

```go
// 截取视频第 1 秒的帧
func generateVideoThumbnail(videoPath string) (tmpPath string, err error) {
    tmpFile, _ := os.CreateTemp("", "thumbnail_*.jpg")
    tmpPath = tmpFile.Name()
    tmpFile.Close()

    cmd := exec.CommandContext(ctx, "ffmpeg",
        "-i", videoPath,
        "-ss", "00:00:01",
        "-frames:v", "1",
        "-q:v", "2",
        "-y",
        tmpPath,
    )
    err = cmd.Run()
    return tmpPath, err
}
```

- `-ss 00:00:01`：截取第 1 秒（避免黑屏开头）
- 短于 1 秒的视频 FFmpeg 自动取可用帧
- FFmpeg 不存在时 `cmd.Run()` 返回错误，不影响主流程

### 缩略图路径推导

```go
func buildThumbnailRelPath(videoRelPath string) string {
    // "author/video.mp4" → "thumbnail/author/video_thumb.jpg"
    ext := filepath.Ext(videoRelPath)
    base := strings.TrimSuffix(videoRelPath, ext)
    return "thumbnail/" + base + "_thumb.jpg"
}
```

### 批量补生成

提供 Handler 方法，为所有缺少缩略图的视频资源批量生成：

```go
func (h *Handler) GenerateMissingThumbnails(ctx context.Context) (generated int, failed int, err error)
```

## 搜索查询适配

**文件**: `backend/search/repository.go` 的 `QueryWorkPage`

resource 子查询 SQL join `persistent_store` 表，带入 `thumbnailPath`：

```sql
(SELECT JSON_GROUP_ARRAY(JSON_OBJECT(
  'id', r.id, ...,
  'thumbnailStoreId', r.thumbnail_store_id,
  'thumbnailPath', ps.file_path
)))
FROM resource r
LEFT JOIN persistent_store ps ON r.thumbnail_store_id = ps.id
WHERE t1.id = r.work_id) AS resources
```

## 前端变更

### WorkCard.vue

优先使用缩略图，可选添加视频角标：

```typescript
const displaySrc = computed(() => {
  const resource = props.work.resource
  if (!resource?.filePath) return ''
  if (resource.thumbnailPath) {
    return buildStoreUrl(resource.thumbnailPath, srcParamStr.value)
  }
  return buildResourceUrl(resource.filePath, srcParamStr.value)
})
```

### WorkSetCard.vue

同上逻辑。

## 修改文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| **SDK** | | |
| `plugin-sdk/dto/resource_dto.go` | 修改 | 新增 `ThumbnailStoreID`、`ThumbnailPath` |
| **后端** | | |
| `backend/base/model/entity/resource.go` | 修改 | 新增 `ThumbnailStoreID` |
| `backend/base/model/dto/resource_dto.go` | 修改 | DTO 转换增加 thumbnail 字段 |
| `backend/base/model/dto/search.go` | 修改 | 补充视频扩展名 |
| `backend/taskManager/model.go` | 修改 | 下载完成后生成缩略图 |
| `backend/taskManager/manager.go` | 修改 | 注入 PersistentStoreWriter |
| `backend/search/repository.go` | 修改 | SQL join persistent_store |
| **前端** | | |
| `frontend/src/model/model/entity/Resource.ts` | 修改 | 新增 thumbnailPath |
| `frontend/src/components/common/WorkCard.vue` | 修改 | 缩略图优先 + 可选视频角标 |
| `frontend/src/components/common/WorkSetCard.vue` | 修改 | 同上 |
