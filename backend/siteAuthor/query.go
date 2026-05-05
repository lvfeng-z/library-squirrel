package siteAuthor

import "github.com/library-squirrel/wails/backend/base/query"

// SiteAuthorQueryDTO 站点作者查询条件
type SiteAuthorQueryDTO struct {
	ID                   query.QueryAttribute[int64]  `json:"-" query:"id"`                              // 站点作者ID（程序设置，不从JSON解析）
	SiteID               query.QueryAttribute[int64]  `json:"siteId" query:"site_id"`                    // 站点ID
	SiteAuthorID         query.QueryAttribute[string] `json:"siteAuthorId" query:"site_author_id"`       // 站点作者ID（外部）
	LocalAuthorID        query.QueryAttribute[int64]  `json:"localAuthorId" query:"local_author_id"`     // 本地作者ID
	FixedAuthorName      query.QueryAttribute[string] `json:"fixedAuthorName" query:"fixed_author_name"` // 固定作者名称
	BoundOnLocalAuthorId query.QueryAttribute[bool]   `json:"boundOnLocalAuthorId" query:""`             // 是否绑定到指定本地作者（非数据库字段）
	AuthorName           query.QueryAttribute[string] `json:"authorName" query:"author_name"`            // 作者名称（模糊匹配）
	Introduce            query.QueryAttribute[string] `json:"introduce" query:"introduce"`               // 介绍（模糊匹配）
	UpdateTime           query.QueryAttribute[int64]  `json:"updateTime" query:"update_time"`            // 更新时间（可用于排序）
	CreateTime           query.QueryAttribute[int64]  `json:"createTime" query:"create_time"`            // 创建时间（可用于排序）
}
