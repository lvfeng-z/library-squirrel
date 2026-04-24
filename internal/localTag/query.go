package localTag

import "github.com/library-squirrel/wails/pkg/query"

// LocalTagQueryDTO 本地标签查询条件
type LocalTagQueryDTO struct {
	ID             query.QueryAttribute[int64]  `json:"-" query:"id"`                             // 本地标签ID（程序设置，不从JSON解析）
	BaseLocalTagID query.QueryAttribute[int64]  `json:"baseLocalTagId" query:"base_local_tag_id"` // 基础本地标签ID
	LocalTagName   query.QueryAttribute[string] `json:"localTagName" query:"local_tag_name"`      // 本地标签名称（精确匹配）
	UpdateTime     query.QueryAttribute[int64]  `json:"updateTime" query:"update_time"`           // 更新时间（可用于排序）
	CreateTime     query.QueryAttribute[int64]  `json:"createTime" query:"create_time"`           // 创建时间（可用于排序）
	WorkId         query.QueryAttribute[int64]  `json:"workId" query:"work_id"`                   // 作品ID（用于关联查询）
}
