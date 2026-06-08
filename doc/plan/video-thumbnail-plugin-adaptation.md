# 视频缩略图 — 插件适配指南

## 背景

主程序已完成视频缩略图功能的基础设施（详见 [video-thumbnail-plan.md](video-thumbnail-plan.md)）。插件需要适配以下两项变更：

1. **`TaskResourceDTO` 清理**：移除 `Type` 字段和 5 个遗留字段（`resourceId`、`url`、`localPath`、`remotePath`、`completeness`），Go DTO 仅保留 `Format`、`Size`、`SuggestName`、`Continuable`
2. **新增 `GetThumbnail` 接口**：SDK `TaskHandler` 接口新增方法，插件需实现

## SDK DTO 变更

### TaskResourceDTO（旧 → 新）

```go
// 之前（6 字段）：
type TaskResourceDTO struct {
    Type        string `json:"type"`        // 已移除
    Format      string `json:"format"`
    Size        int64  `json:"size"`
    SuggestName string `json:"suggestName"`
    Continuable *bool  `json:"continuable"`
    // 以下字段在 proto 中已被移除，但部分插件仍在使用（编译不通过）：
    // URL        string `json:"url"`         // 已移除
    // RemotePath string `json:"remotePath"`  // 已移除
    // LocalPath  string `json:"localPath"`   // 已移除
}

// 之后（4 字段）：
type TaskResourceDTO struct {
    Format      string `json:"format"`
    Size        int64  `json:"size"`
    SuggestName string `json:"suggestName"`
    Continuable *bool  `json:"continuable"`
}
```

### 新增 DTO

```go
// ThumbnailResponse 缩略图响应
type ThumbnailResponse struct {
    Data   []byte `json:"data"`   // 缩略图原始字节
    Format string `json:"format"` // 格式扩展名（如 "jpg"、"png"）
}
```

## TaskHandler 接口新增方法

```go
type TaskHandler interface {
    // ... 现有方法不变 ...

    // GetThumbnail 获取缩略图
    // taskData: 插件在 Create 阶段存储的任务数据（JSON）
    // 返回缩略图数据或 nil（插件决定不提供缩略图时返回 nil）
    GetThumbnail(taskData string) (*ThumbnailResponse, error)
}
```

## 适配步骤

### 第一步：清理 TaskResourceDTO 构建代码

将所有构建 `TaskResourceDTO` 的代码中使用的遗留字段替换为有效字段：

| 旧字段 | 替换方式 |
|--------|---------|
| `URL: xxx` | 移除（宿主侧已不需要） |
| `RemotePath: xxx` | 改用 `SuggestName: xxx`（文件名建议） |
| `LocalPath: xxx` | 移除（宿主侧已不需要） |
| `Type: xxx` | 移除（类型通过 PersistentStore.FilenameExtension 判断） |

**示例**：

```go
// 之前：
resource := &sdkdto.TaskResourceDTO{
    URL:    downloadURL,
    Format: "mp4",
    Size:   fileSize,
}
if suggestedName != "" {
    resource.RemotePath = suggestedName
}

// 之后：
resource := &sdkdto.TaskResourceDTO{
    Format:      "mp4",
    Size:        fileSize,
    SuggestName: suggestedName,
}
```

### 第二步：实现 GetThumbnail 方法

```go
func (h *YourTaskHandler) GetThumbnail(taskData string) (*sdkdto.ThumbnailResponse, error) {
    // 1. 解析 taskData（即 Create 阶段存入 PluginData 的 JSON）
    var data YourPluginData
    if err := json.Unmarshal([]byte(taskData), &data); err != nil {
        return nil, fmt.Errorf("解析任务数据失败: %w", err)
    }

    // 2. 判断是否需要缩略图
    //    普通图片作品：返回 nil, nil
    //    视频/动图/文章：返回缩略图数据
    if data.ThumbnailURL == "" {
        return nil, nil  // 不需要缩略图
    }

    // 3. 下载/生成缩略图
    thumbData, err := downloadThumbnail(data.ThumbnailURL)
    if err != nil {
        return nil, fmt.Errorf("缩略图下载失败: %w", err)
    }

    // 4. 返回响应
    return &sdkdto.ThumbnailResponse{
        Data:   thumbData,
        Format: "jpg",  // 格式扩展名
    }, nil
}
```

### 第三步（可选）：在 Create 中保存缩略图上下文

如果插件需要在 `GetThumbnail` 时获取缩略图 URL 等信息，需在 `Create` 阶段将其存入 PluginData：

```go
// 在 TaskPluginData 中新增字段
type TaskPluginData struct {
    // ... 现有字段 ...
    ThumbnailURL string `json:"thumbnailUrl"` // 缩略图 URL
}

// 在 Create 中提取并保存
taskPluginData.ThumbnailURL = extractThumbnailURL(apiResponse)
```

## 调用时序

```
Create(url)
  → 保存 thumbnailUrl 到 PluginData
  → 返回 TaskCreateResponse（含 PluginData JSON）

Start(task) / Resume(param)
  → 构建 TaskResourceDTO（仅 Format/Size/SuggestName/Continuable）
  → 返回下载流

... 主程序下载资源 ...

GetThumbnail(taskData)        ← 主程序在下载完成后调用
  → 从 taskData 解析 thumbnailUrl
  → 下载缩略图字节
  → 返回 ThumbnailResponse
```

## 不提供缩略图的插件

如果插件不需要支持缩略图，实现空方法即可：

```go
func (h *YourTaskHandler) GetThumbnail(taskData string) (*sdkdto.ThumbnailResponse, error) {
    return nil, nil
}
```

## 已完成适配的插件参考

| 插件 | TaskResourceDTO 清理 | Create 保存缩略图 URL | GetThumbnail 实现 |
|------|---------------------|---------------------|-------------------|
| pixiv | ✅ 移除 URL/RemotePath，改用 SuggestName | ✅ Browser API 和 App API 路径均提取 Small URL | ✅ 从 ThumbnailURL 下载 |
| local | ✅ 移除 URL/LocalPath | — | ✅ 返回 nil（TODO: 后续支持） |
