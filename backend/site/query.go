package site

import "github.com/library-squirrel/backend/base/query"

// SiteQueryDTO 站点查询条件
type SiteQueryDTO struct {
	ID         query.QueryAttribute[int64]  `json:"-" query:"id"`                   // 站点ID（程序设置，不从JSON解析）
	SiteName   query.QueryAttribute[string] `json:"siteName" query:"site_name"`     // 站点名称（精确匹配）
	Homepage   query.QueryAttribute[string] `json:"homepage" query:"homepage"`      // 主页地址（精确匹配）
	Enable     query.QueryAttribute[bool]   `json:"enable" query:"enable"`          // 是否启用
	UpdateTime query.QueryAttribute[int64]  `json:"updateTime" query:"update_time"` // 更新时间（可用于排序）
	CreateTime query.QueryAttribute[int64]  `json:"createTime" query:"create_time"` // 创建时间（可用于排序）
}
