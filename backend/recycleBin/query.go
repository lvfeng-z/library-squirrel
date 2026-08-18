package recycleBin

import "github.com/library-squirrel/backend/base/model/dto"

// RecyclePageQuery 回收站分页查询请求
// 条件体系复用作品搜索 SearchCondition（作者/标签/站点/时间范围等），
// 时间范围用 WorksCreateTime/WorksDeleteTime + GreaterOrEqual/LessOrEqual 成对表达
type RecyclePageQuery struct {
	Conditions []*dto.SearchCondition `json:"conditions"` // 筛选条件（与作品搜索同构）
	SortBy     string                 `json:"sortBy"`     // 排序列：createTime | 空=deleteTime
	SortOrder  string                 `json:"sortOrder"`  // 排序方向：asc | desc（默认 desc）
}
