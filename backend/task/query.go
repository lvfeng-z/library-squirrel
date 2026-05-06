package task

import "github.com/library-squirrel/backend/base/query"

// TaskQueryDTO 任务查询条件
type TaskQueryDTO struct {
	ID                   query.QueryAttribute[int64]  `json:"-" query:"id"`                                        // 任务ID（程序设置，不从JSON解析）
	Pid                  query.QueryAttribute[int64]  `json:"pid" query:"pid"`                                     // 父任务ID
	SiteID               query.QueryAttribute[int64]  `json:"siteId" query:"site_id"`                              // 站点ID
	SiteWorkID           query.QueryAttribute[string] `json:"siteWorkId" query:"site_work_id"`                     // 站点作品ID
	Status               query.QueryAttribute[string] `json:"status" query:"status"`                               // 任务状态
	IsCollection         query.QueryAttribute[bool]   `json:"isCollection" query:"is_collection"`                  // 是否为合集（0=否，1=是）
	PluginPublicID       query.QueryAttribute[string] `json:"pluginPublicId" query:"plugin_public_id"`             // 插件公开ID
	PluginContributionID query.QueryAttribute[string] `json:"pluginContributionId" query:"plugin_contribution_id"` // 插件贡献ID
	Continuable          query.QueryAttribute[bool]   `json:"continuable" query:"continuable"`                     // 是否可继续（0=否，1=是）
	TaskName             query.QueryAttribute[string] `json:"taskName" query:"task_name"`                          // 任务名称（模糊匹配）
	CreateTime           query.QueryAttribute[int64]  `json:"createTime" query:"create_time"`                      // 创建时间（可用于排序）
	UpdateTime           query.QueryAttribute[int64]  `json:"updateTime" query:"update_time"`                      // 更新时间（可用于排序）
}
