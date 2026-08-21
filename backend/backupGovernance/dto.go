package backupGovernance

// BackupDTO 备份保管清单行 DTO（备份管理面板列表；命名字段组合，不嵌实体）
type BackupDTO struct {
	ID         int64  `json:"id"`         // 清单行 ID
	FileName   string `json:"fileName"`   // 保管文件名
	FilePath   string `json:"filePath"`   // 保管相对路径（workDir 基准，正斜杠）
	Workdir    string `json:"workdir"`    // 保管时的 workDir
	CreateTime int64  `json:"createTime"` // 保管时刻（毫秒时间戳）
	// Referenced 是否被业务行引用（有主）；与治理对账同一引用集（含软删 store 行/已卸载插件行引用）
	Referenced bool `json:"referenced"`
	// FileSize 文件磁盘占用（字节，os.Stat；文件缺失计 0）。仅展示，不参与服务端排序
	// （大小无库列，按大小排序需全量 Stat）
	FileSize int64 `json:"fileSize"`
}

// BackupStatsDTO 备份占用统计（监视哨同源数据面）：总占用、有主/无主拆分、
// 按引用方分组（数量/占用/最老引用年龄，超 90 天观察阈值由前端高亮）、无主超期圈定
type BackupStatsDTO struct {
	TotalCount      int   `json:"totalCount"`      // 清单行总数
	TotalBytes      int64 `json:"totalBytes"`      // 总磁盘占用
	ReferencedCount int   `json:"referencedCount"` // 有主清单行数
	ReferencedBytes int64 `json:"referencedBytes"` // 有主占用
	OrphanedCount   int   `json:"orphanedCount"`   // 无主清单行数
	OrphanedBytes   int64 `json:"orphanedBytes"`   // 无主占用
	// ExpiredOrphanIDs 无主且超保留期的清单行 ID：「清理全部无主」的删除圈定，
	// 与治理正向判据同源（防误杀替换在途还原点/崩溃窗口新孤儿）
	ExpiredOrphanIDs []int64           `json:"expiredOrphanIds"`
	Referencers      []ReferencerStats `json:"referencers"` // 按引用方分组统计
}
