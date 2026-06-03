# 视频作品缩略图展示方案

## 背景

当前系统中，WorkCard 使用 `<el-image>` 展示作品的封面图，取的是第一个 `enabled=true` 的 Resource 的 `filePath`。对于视频类型资源（`.mp4`, `.avi`, `.mkv` 等），`<el-image>` 无法渲染视频文件，会显示错误图标。

**目标**：视频类型作品在主页、作品集等页面中，以静态缩略图（从视频中截取一帧）的方式展示，与图片类型作品体验一致。

## 现状分析

| 项目 | 现状 |
|------|------|
| Resource 实体 | 有 `FilePath`、`FilenameExtension` 字段，无缩略图字段 |
| MediaType 枚举 | 已定义 `MediaTypeVideo = 2`，扩展名映射 `{".mp4", ".avi", ".mkv"}` |
| WorkCard.vue | 统一使用 `<el-image>` 渲染，不区分资源类型 |
| ResourceHandler | 直接返回原始文件，未处理缩略图请求 |
| 缩略图生成 | 无任何实现 |
| FFmpeg 依赖 | go.mod 中无视频处理库 |

## 方案设计

### 核心思路

在 Resource 实体上新增 `ThumbnailPath` 字段，后端在视频资源下载完成后调用 FFmpeg 截取一帧保存为 JPEG，前端优先使用缩略图路径展示。

### 1. 数据模型变更

#### 1.1 Resource 实体新增字段

**文件**: `backend/base/model/entity/resource.go`

```go
type Resource struct {
    *model.BaseEntity
    // ... 现有字段 ...
    ThumbnailPath sql.NullString `gorm:"column:thumbnail_path" json:"thumbnailPath"`
}
```

- 存储缩略图相对于 `workdir/resource/` 的相对路径（与 `FilePath` 一致）
- 仅视频资源有值，图片资源为 NULL
- GORM AutoMigrate 自动添加列

#### 1.2 SDK ResourceDTO 新增字段

**文件**: `library-squirrel-plugin-sdk/dto/resource_dto.go`

```go
type ResourceDTO struct {
    // ... 现有字段 ...
    ThumbnailPath *string `json:"thumbnailPath"`
}
```

SDK 变更后主项目和插件项目都需要更新绑定。

### 2. 缩略图生成（后端）

#### 2.1 新建 thumbnail 模块

```
backend/thumbnail/
  thumbnail.go       — ThumbnailGenerator 接口定义
  ffmpeg.go          — FFmpeg 实现（调用系统 FFmpeg）
  noop.go            — 空实现（FFmpeg 不可用时的降级方案）
```

#### 2.2 ThumbnailGenerator 接口

```go
// ThumbnailGenerator 视频缩略图生成器
type ThumbnailGenerator interface {
    // Generate 从视频文件生成缩略图，返回缩略图的相对路径
    // videoAbsPath: 视频文件的绝对路径
    // workDir: 工作目录
    // 返回相对于 workDir/resource/ 的缩略图路径
    Generate(ctx context.Context, videoAbsPath string, workDir string) (relativePath string, err error)
    // Available 检查生成器是否可用（FFmpeg 是否存在）
    Available() bool
}
```

#### 2.3 FFmpeg 实现

- 通过 `os/exec` 调用系统 FFmpeg（`ffmpeg` 或 `ffprobe`）
- 命令：`ffmpeg -i {videoPath} -ss 00:00:01 -frames:v 1 -q:v 2 {outputPath}`
  - `-ss 00:00:01`：截取第 1 秒的帧（避免黑屏开头）
  - `-frames:v 1`：只截取一帧
  - `-q:v 2`：JPEG 质量（2=高质量）
  - 如果视频短于 1 秒，FFmpeg 会自动取最后一帧
- 缩略图存放路径：与视频同目录，文件名加 `_thumb` 后缀，扩展名 `.jpg`
  - 例：视频 `author/video.mp4` → 缩略图 `author/video_thumb.jpg`
- FFmpeg 不可用时使用 `noopGenerator`，日志记录警告，`ThumbnailPath` 保持 NULL

#### 2.4 集成点：任务完成时触发

**文件**: `backend/taskManager/model.go` 的 `downloadLoop()` 方法

在下载完成（`readErr == io.EOF` 分支）后、`setState(TaskStateFinished)` 之前，新增缩略图生成逻辑：

```go
// 下载完成
if readErr == io.EOF {
    m.currentFile.Sync()
    // 校验完整性 ...
    m.clearPendingResourceID()

    // 生成视频缩略图
    m.generateThumbnailIfNeeded()

    m.setState(TaskStateFinished)
    return runResultDone
}
```

新增 `generateThumbnailIfNeeded` 方法：

```go
func (m *ManagedTask) generateThumbnailIfNeeded() {
    // 1. 检查是否有缩略图生成器
    // 2. 检查资源扩展名是否为视频类型
    // 3. 构建绝对路径，调用生成器
    // 4. 更新 Resource 实体的 ThumbnailPath 字段
}
```

这需要在 `ManagedTask` 上新增 `thumbnailGenerator ThumbnailGenerator` 依赖，由 `NewManagedTask` 注入。

### 3. 已有视频资源的批量缩略图生成

提供一个 Handler 方法，供前端在"设置"或首次检测到无缩略图的视频时手动触发：

```go
// GenerateMissingThumbnails 为所有缺少缩略图的视频资源批量生成缩略图
func (h *Handler) GenerateMissingThumbnails(ctx context.Context) (generated int, failed int, err error)
```

- 查询所有 `thumbnail_path IS NULL AND filename_extension IN (视频扩展名)` 的 Resource
- 逐个调用 `ThumbnailGenerator.Generate`
- 返回成功/失败数量

### 4. ResourceHandler 增强（后端）

**文件**: `backend/assetserver/resource_handler.go`

无需修改。缩略图作为普通 JPEG 文件存储在 `workdir/resource/` 目录下，使用已有的 `/resource/{relativePath}` 路径即可访问。

### 5. 前端变更

#### 5.1 Resource 实体/DTO 增加字段

- `frontend/src/model/model/entity/Resource.ts` — 增加 `thumbnailPath` 字段
- TypeScript binding 自动生成后自动包含

#### 5.2 WorkCardItem 调整

**文件**: `frontend/src/model/model/dto/WorkCardItem.ts`

修改 `getActiveResource` 的资源传递逻辑，确保 `thumbnailPath` 被传递到 Resource 实体中（binding 更新后自动同步）。

#### 5.3 WorkCard.vue 展示逻辑

**文件**: `frontend/src/components/common/WorkCard.vue`

修改 `<el-image>` 的 `src` 逻辑：

```typescript
// 计算展示用的图片 URL
const displaySrc = computed(() => {
  const resource = props.work.resource
  if (!resource?.filePath) return ''
  // 优先使用缩略图（视频资源）
  if (resource.thumbnailPath) {
    return buildResourceUrl(resource.thumbnailPath, srcParamStr.value)
  }
  return buildResourceUrl(resource.filePath, srcParamStr.value)
})
```

模板中 `<el-image :src="displaySrc">`。

#### 5.4 视频标识角标（可选增强）

在 WorkCard 图片右上角添加一个小的视频图标角标，表示这是视频作品：

```vue
<div v-if="isVideoResource" class="work-card-video-badge">
  <el-icon><VideoPlay /></el-icon>
</div>
```

判断逻辑：
```typescript
const isVideoResource = computed(() => {
  const ext = props.work.resource?.filenameExtension?.toLowerCase()
  return ext && videoExtensions.includes(ext)
})
```

### 6. WorkSet 封面适配

**文件**: `backend/search/service.go` 的 `QueryWorkSetPage`

当前封面资源选取逻辑（`getCoverWorkId` → 获取第一个 enabled Resource）无需改动。因为 Resource 的 `thumbnailPath` 已经会被传递到前端，`WorkSetCard.vue` 的封面展示也需要做与 WorkCard.vue 同样的 `thumbnailPath` 优先判断。

### 7. 数据流全景

```
下载完成 → downloadLoop() 检测到视频扩展名
         → thumbnailGenerator.Generate()
         → FFmpeg 截取第 1 秒帧 → 保存为 video_thumb.jpg
         → 更新 Resource.thumbnail_path = "author/video_thumb.jpg"

展示时：
  前端 WorkCard → resource.thumbnailPath 存在?
    → 是: buildResourceUrl(thumbnailPath) → /resource/author/video_thumb.jpg
    → 否: buildResourceUrl(filePath)      → /resource/author/video.mp4 (或图片)
```

## 修改文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| **SDK** | | |
| `plugin-sdk/dto/resource_dto.go` | 修改 | ResourceDTO 新增 `ThumbnailPath *string` |
| **后端** | | |
| `backend/base/model/entity/resource.go` | 修改 | 新增 `ThumbnailPath` 字段 |
| `backend/base/model/dto/resource_dto.go` | 修改 | DTO 转换增加 ThumbnailPath |
| `backend/thumbnail/thumbnail.go` | 新建 | 接口定义 |
| `backend/thumbnail/ffmpeg.go` | 新建 | FFmpeg 实现 |
| `backend/thumbnail/noop.go` | 新建 | 空实现（降级） |
| `backend/taskManager/model.go` | 修改 | ManagedTask 增加 thumbnailGenerator 依赖和 generateThumbnailIfNeeded 方法 |
| `backend/taskManager/manager.go` | 修改 | NewManagedTask 注入 thumbnailGenerator |
| `backend/app.go` | 修改 | 初始化 ThumbnailGenerator 并注入 |
| `backend/base/model/dto/search.go` | 修改 | MediaExtMapping 补充 `.webm`, `.mov` 等格式 |
| **前端**（binding 更新后） | | |
| `frontend/src/model/model/entity/Resource.ts` | 修改 | 新增 thumbnailPath 字段 |
| `frontend/src/components/common/WorkCard.vue` | 修改 | 优先使用 thumbnailPath，可选添加视频角标 |
| `frontend/src/components/common/WorkSetCard.vue` | 修改 | 同上 |
| `frontend/src/model/model/constant/MediaType.ts` | 不变 | 已有 VIDEO 枚举 |
| `frontend/src/model/model/dto/WorkCardItem.ts` | 可能微调 | 确保 thumbnailPath 正确传递 |

## FFmpeg 依赖说明

- **推荐**：要求用户系统已安装 FFmpeg 并加入 PATH（桌面应用常见做法）
- **降级**：FFmpeg 不可用时，跳过缩略图生成，卡片展示默认视频图标（error slot）
- **未来可选**：打包精简版 FFmpeg 静态二进制（约 30-50MB），或首次启动时引导安装

## 执行顺序建议

1. SDK：ResourceDTO 新增 `ThumbnailPath` 字段 → 发布新版本
2. 主项目 `go.mod` 更新 SDK 依赖
3. 后端：Resource 实体新增字段 + DTO 转换更新
4. 后端：新建 thumbnail 模块（接口 + FFmpeg 实现 + noop 实现）
5. 后端：集成到 TaskManager（下载完成后生成缩略图）
6. 后端：app.go 初始化和依赖注入
7. 运行 `wails3 generate bindings -ts` 更新前端绑定
8. 前端：Resource 实体、WorkCard、WorkSetCard 适配
9. 后端（可选）：批量生成已有视频缩略图的 Handler
10. 前端（可选）：视频角标标识
