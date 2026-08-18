package recycleBin

import "github.com/library-squirrel/backend/base/query"

// RecycleQueryDTO 回收站查询条件
// 时间范围由 Start/End 字段对表达（operator 由前端显式传 gte/lte）；
// 作者/标签存在快照 JSON 中（非数据库列），走应用层过滤，不进 Converter
type RecycleQueryDTO struct {
	DeleteTimeStart     query.QueryAttribute[int64] `json:"deleteTimeStart" query:"delete_time"`          // 删除时间起（gte）
	DeleteTimeEnd       query.QueryAttribute[int64] `json:"deleteTimeEnd" query:"delete_time"`            // 删除时间止（lte）
	WorkCreateTimeStart query.QueryAttribute[int64] `json:"workCreateTimeStart" query:"work_create_time"` // 创建时间起（gte）
	WorkCreateTimeEnd   query.QueryAttribute[int64] `json:"workCreateTimeEnd" query:"work_create_time"`   // 创建时间止（lte）
	SiteID              query.QueryAttribute[int64] `json:"siteId" query:"site_id"`                       // 站点（eq）
	LocalAuthorID       *int64                      `json:"localAuthorId"`                                // 本地作者（快照过滤）
	LocalTagID          *int64                      `json:"localTagId"`                                   // 本地标签（快照过滤）
	SortBy              *string                     `json:"sortBy"`                                       // 排序列：deleteTime | workCreateTime
	SortOrder           *string                     `json:"sortOrder"`                                    // 排序方向：asc | desc
}
