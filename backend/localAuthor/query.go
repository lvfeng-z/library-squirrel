package localAuthor

import "github.com/library-squirrel/wails/backend/base/query"

// LocalAuthorQueryDTO 本地作者查询条件
type LocalAuthorQueryDTO struct {
	ID            query.QueryAttribute[int64]  `json:"-" query:"id"`                      // 本地作者ID（程序设置，不从JSON解析）
	AuthorName    query.QueryAttribute[string] `json:"authorName" query:"author_name"`    // 作者名称（精确匹配）
	AuthorNameStr query.QueryAttribute[string] `json:"authorNameStr" query:"author_name"` // 作者名称（模糊匹配）
	Introduce     query.QueryAttribute[string] `json:"introduce" query:"introduce"`       // 介绍（模糊匹配）
	UpdateTime    query.QueryAttribute[int64]  `json:"updateTime" query:"update_time"`    // 更新时间（可用于排序）
	CreateTime    query.QueryAttribute[int64]  `json:"createTime" query:"create_time"`    // 创建时间（可用于排序）
}
