package task

import "github.com/library-squirrel/backend/base/query"

// TaskQueryDTO 任务查询条件。
// 列名一律带表前缀：查询语句挂 work_task 左连接（领域列在领域表），无前缀的 create_time
// 等同名列在连接后产生歧义引用
type TaskQueryDTO struct {
	ID                query.QueryAttribute[int64]  `json:"-" query:"task.id"`                                       // 任务ID（程序设置，不从JSON解析）
	Pid               query.QueryAttribute[int64]  `json:"pid" query:"task.pid"`                                    // 父任务ID
	SiteID            query.QueryAttribute[int64]  `json:"siteId" query:"work_task.site_id"`                        // 站点ID（领域列）
	SiteWorkID        query.QueryAttribute[string] `json:"siteWorkId" query:"work_task.site_work_id"`               // 站点作品ID（领域列）
	Status            query.QueryAttribute[int64]  `json:"status" query:"task.status"`                              // 任务状态
	HasChild          query.QueryAttribute[bool]   `json:"hasChild" query:"task.has_child"`                         // 是否有子任务
	PluginPublicID    query.QueryAttribute[string] `json:"pluginPublicId" query:"work_task.plugin_public_id"`       // 插件公开ID（领域列）
	PluginExtensionID query.QueryAttribute[string] `json:"pluginExtensionId" query:"work_task.plugin_extension_id"` // 插件贡献ID（领域列）
	Continuable       query.QueryAttribute[bool]   `json:"continuable" query:"work_task.continuable"`               // 是否可继续（领域列；0=否，1=是）
	TaskName          query.QueryAttribute[string] `json:"taskName" query:"task.task_name"`                         // 任务名称（模糊匹配）
	CreateTime        query.QueryAttribute[int64]  `json:"createTime" query:"task.create_time"`                     // 创建时间（可用于排序）
	UpdateTime        query.QueryAttribute[int64]  `json:"updateTime" query:"task.update_time"`                     // 更新时间（可用于排序）
}
