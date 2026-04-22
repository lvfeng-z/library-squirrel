package siteTag

import "github.com/library-squirrel/wails/pkg/query"

// SiteTagQueryDTO 站点标签查询条件
type SiteTagQueryDTO struct {
	ID                query.QueryAttribute[int64]  `json:"-" query:"id"`                           // 站点标签ID（程序设置，不从JSON解析）
	SiteID            query.QueryAttribute[int64]  `json:"siteId" query:"site_id"`                 // 站点ID
	SiteTagID         query.QueryAttribute[string] `json:"siteTagId" query:"site_tag_id"`          // 站点标签ID（外部）
	BaseSiteTagID     query.QueryAttribute[string] `json:"baseSiteTagId" query:"base_site_tag_id"` // 基础站点标签ID
	LocalTagID        query.QueryAttribute[int64]  `json:"localTagId" query:"local_tag_id"`        // 本地标签ID
	BoundOnLocalTagId query.QueryAttribute[bool]   `json:"boundOnLocalTagId" query:""`             // 是否绑定到指定本地标签（非数据库字段）
	SiteTagName       query.QueryAttribute[string] `json:"siteTagName" query:"site_tag_name"`      // 站点标签名称（模糊匹配）
	Description       query.QueryAttribute[string] `json:"description" query:"description"`        // 描述（模糊匹配）
	UpdateTime        query.QueryAttribute[int64]  `json:"updateTime" query:"update_time"`         // 更新时间（可用于排序）
	CreateTime        query.QueryAttribute[int64]  `json:"createTime" query:"create_time"`         // 创建时间（可用于排序）
}
