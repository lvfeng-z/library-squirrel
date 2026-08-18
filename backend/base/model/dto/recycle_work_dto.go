package dto

// RecycleWorkDTO 回收站作品条目（列表展示用）
// 软删除模型下回收站条目即 work 已删行，删除时间为 work.deleted_at
type RecycleWorkDTO struct {
	ID          int64   `json:"id"` // work.id（复原/彻底删除的操作键）
	SiteID      *int64  `json:"siteId"`
	SiteName    string  `json:"siteName"` // 站点名（查询时 LEFT JOIN site 拼出）
	SiteWorkID  *string `json:"siteWorkId"`
	WorkName    *string `json:"workName"`
	AuthorNames string  `json:"authorNames"` // 作者名顿号拼接（本地作者名优先，无本地关联回退站点作者名；SQL GROUP_CONCAT 聚合）
	CreateTime  int64   `json:"createTime"`  // 作品入库时间
	DeleteTime  int64   `json:"deleteTime"`  // 软删时间（work.deleted_at）
	// 距 TTL 自动清理的剩余整天数（向上取整，负值归 0）；自动清理未启用时为 null（service 按 TTL 设置组装）
	ExpireDaysLeft *int `json:"expireDaysLeft"`
	// 预览图 store 相对路径（与作品卡片同优先级：缩略图优先，回退图片资源的主图；无图为空串）
	// 软删期间文件在 backup/，经 /store/ 服务的 backup 兜底仍可访问
	PreviewPath string `json:"previewPath"`
}
