package dto

// RecycleWorkSetPageQuery 回收站作品集条目分页查询请求（作品集域平铺条件；
// 作品集无标签/作者关联，与作品条目的 SearchCondition 标签体系分轨）
type RecycleWorkSetPageQuery struct {
	Name           string `json:"name"`           // 名称模糊（site_work_set_name / nick_name 任一命中）
	SiteId         *int64 `json:"siteId"`         // 站点（nil=不限）
	DeleteTimeFrom int64  `json:"deleteTimeFrom"` // 删除时间范围起（毫秒，0=不限）
	DeleteTimeTo   int64  `json:"deleteTimeTo"`   // 删除时间范围止（毫秒，0=不限）
	SortOrder      string `json:"sortOrder"`      // 按删除时间排序方向：asc | 其他=desc（默认）
}

// RecycleWorkSetDTO 回收站作品集条目（work_set 已删行；关联行保留，复原即清标志）
type RecycleWorkSetDTO struct {
	ID            int64   `json:"id"`            // work_set 行 ID（操作键）
	SiteID        *int64  `json:"siteId"`        // 站点 ID（无站点为 null）
	SiteWorkSetID *string `json:"siteWorkSetId"` // 站点作品集 ID（本地手建集为 null）
	Name          string  `json:"name"`          // 站点作品集名（NULL 归空串）
	NickName      string  `json:"nickName"`      // 本地昵称（NULL 归空串）
	SiteName      string  `json:"siteName"`      // 站点名（LEFT JOIN，无站点为空串）
	// AliveMemberCount 活成员作品数（已删作品不计；成员关联保留，作品复原后自动回位）
	AliveMemberCount int64 `json:"aliveMemberCount"`
	CreateTime       int64 `json:"createTime"` // 创建时间
	DeleteTime       int64 `json:"deleteTime"` // 删除时间（work_set.deleted_at，TTL 基准）
	// ExpireDaysLeft 距 TTL 自动清理的剩余整天数（向上取整，负值归 0）；自动清理未启用时为 null
	ExpireDaysLeft *int `json:"expireDaysLeft"`
}
