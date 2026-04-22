package work

import "github.com/library-squirrel/wails/pkg/query"

// WorkQueryDTO 作品查询条件
type WorkQueryDTO struct {
	ID            query.QueryAttribute[int64]  `json:"-" query:"id"`                               // 作品ID（程序设置，不从JSON解析）
	SiteID        query.QueryAttribute[int64]  `json:"siteId" query:"site_id"`                     // 站点ID
	SiteWorkID    query.QueryAttribute[string] `json:"siteWorkId" query:"site_work_id"`            // 站点作品ID
	SiteAuthorID  query.QueryAttribute[int64]  `json:"siteAuthorId" query:"site_author_id"`        // 站点作者ID
	LocalAuthorID query.QueryAttribute[int64]  `json:"localAuthorId" query:"local_author_id"`      // 本地作者ID
	NickName      query.QueryAttribute[string] `json:"nickName" query:"nick_name"`                 // 昵称（精确匹配）
	SiteWorkName  query.QueryAttribute[string] `json:"siteWorkName" query:"site_work_name"`        // 站点作品名称（模糊匹配）
	SiteWorkDesc  query.QueryAttribute[string] `json:"siteWorkDesc" query:"site_work_description"` // 站点作品描述（模糊匹配）
	NickNameStr   query.QueryAttribute[string] `json:"nickNameStr" query:"nick_name"`              // 昵称（模糊匹配）
	CreateTime    query.QueryAttribute[int64]  `json:"createTime" query:"create_time"`             // 创建时间（可用于排序）
	UpdateTime    query.QueryAttribute[int64]  `json:"updateTime" query:"update_time"`             // 更新时间（可用于排序）
}
