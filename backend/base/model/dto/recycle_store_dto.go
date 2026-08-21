package dto

// RecycleStorePageQuery 回收站文件条目分页查询请求（文件域条件体系，平铺字段直映射 SQL；
// 与作品条目的 SearchCondition 标签体系分轨——store 条目条件域与作品标签正交）
type RecycleStorePageQuery struct {
	FileName       string `json:"fileName"`       // 文件名模糊（file_name LIKE）
	FilePath       string `json:"filePath"`       // 路径模糊（file_path LIKE）
	MediaType      *int   `json:"mediaType"`      // 媒体类型→扩展名集过滤（dto.MediaExtMapping；nil=不过滤，未知值同不过滤）
	HasBackup      *bool  `json:"hasBackup"`      // 备份状态（true=backup_id>0 / false=0；nil=不过滤）
	WorkName       string `json:"workName"`       // 所属作品名模糊（挂载活作品 site_work_name；离链行天然不命中）
	DeleteTimeFrom int64  `json:"deleteTimeFrom"` // 删除时间范围起（毫秒，0=不限）
	DeleteTimeTo   int64  `json:"deleteTimeTo"`   // 删除时间范围止（毫秒，0=不限）
	SortOrder      string `json:"sortOrder"`      // 按删除时间排序方向：asc | 其他=desc（默认）
}

// RecycleStoreDTO 回收站文件条目（列表展示用）
// 软删除模型下文件条目 = persistent_store 已删行且非「作品已删」聚合形态（work 不可达或 work 存活）；
// 条目单位是 store 行非作品，TTL/过期按行自身 deleted_at，作品仅提供上下文展示
type RecycleStoreDTO struct {
	ID                int64  `json:"id"`                // persistent_store 行 ID（清理操作键）
	FileName          string `json:"fileName"`          // 文件名（NULL 归空串）
	FilePath          string `json:"filePath"`          // workDir 相对路径（正斜杠；软删期间文件在 backup/，/store/ 服务按行内 backup_id 兜底可访问）
	FilenameExtension string `json:"filenameExtension"` // 扩展名（含点）
	DeleteTime        int64  `json:"deleteTime"`        // 删除时间（persistent_store.deleted_at，TTL 基准）
	// HasBackup 行内是否引用备份清单行（backup_id>0；软删链移文件入 backup/ 时写入，MarkInvalid 失效行保持 0）
	HasBackup bool `json:"hasBackup"`
	// CanRestore 可复原性：HasBackup 且挂载链可达（活作品）——版本回滚置换入口的状态预铺（操作接通在 J' 软删化）
	CanRestore bool `json:"canRestore"`
	// WorkId / WorkName / SiteName 有主链（挂载活作品）的作品上下文；离链行 WorkId 为 null、名称为空串
	WorkId   *int64 `json:"workId"`
	WorkName string `json:"workName"`
	SiteName string `json:"siteName"`
	// ExpireDaysLeft 距 TTL 自动清理的剩余整天数（向上取整，负值归 0）；自动清理未启用时为 null
	ExpireDaysLeft *int `json:"expireDaysLeft"`
}
