package workSet

import "github.com/library-squirrel/backend/base/query"

// WorkSetQueryDTO 作品集查询条件
type WorkSetQueryDTO struct {
	ID              query.QueryAttribute[int64]  `json:"-" query:"id"`                                      // 作品集ID（程序设置，不从JSON解析）
	SiteID          query.QueryAttribute[int64]  `json:"siteId" query:"site_id"`                            // 站点ID
	SiteWorkSetID   query.QueryAttribute[int64]  `json:"siteWorkSetId" query:"site_work_set_id"`            // 站点作品集ID
	SiteAuthorID    query.QueryAttribute[int64]  `json:"siteAuthorId" query:"site_author_id"`               // 站点作者ID
	NickName        query.QueryAttribute[string] `json:"nickName" query:"nick_name"`                        // 昵称（精确匹配）
	SiteWorkSetName query.QueryAttribute[string] `json:"siteWorkSetName" query:"site_work_set_name"`        // 站点作品集名称（模糊匹配）
	SiteWorkSetDesc query.QueryAttribute[string] `json:"siteWorkSetDesc" query:"site_work_set_description"` // 站点作品集描述（模糊匹配）
	NickNameStr     query.QueryAttribute[string] `json:"nickNameStr" query:"nick_name"`                     // 昵称（模糊匹配）
	CreateTime      query.QueryAttribute[int64]  `json:"createTime" query:"create_time"`                    // 创建时间（可用于排序）
	UpdateTime      query.QueryAttribute[int64]  `json:"updateTime" query:"update_time"`                    // 更新时间（可用于排序）
}
