package plugin

import "github.com/library-squirrel/backend/base/query"

// PluginQueryDTO 插件查询条件
type PluginQueryDTO struct {
	ID             query.QueryAttribute[int64]  `json:"-" query:"id"`                           // 插件ID（程序设置，不从JSON解析）
	PublicID       query.QueryAttribute[string] `json:"publicId" query:"public_id"`             // 公开ID（精确匹配）
	Name           query.QueryAttribute[string] `json:"name" query:"name"`                      // 插件名称
	Author         query.QueryAttribute[string] `json:"author" query:"author"`                  // 作者
	Source         query.QueryAttribute[string] `json:"source" query:"source"`                  // 来源（bundled/local/url/marketplace，精确匹配）
	Version        query.QueryAttribute[string] `json:"version" query:"version"`                // 版本号（精确匹配）
	ActivationType query.QueryAttribute[string] `json:"activationType" query:"activation_type"` // 激活类型（精确匹配）
	Uninstalled    query.QueryAttribute[bool]   `json:"uninstalled" query:"uninstalled"`        // 是否已卸载（0=未卸载，1=已卸载）
	CreateTime     query.QueryAttribute[int64]  `json:"createTime" query:"create_time"`         // 创建时间（可用于排序）
	UpdateTime     query.QueryAttribute[int64]  `json:"updateTime" query:"update_time"`         // 更新时间（可用于排序）
}
