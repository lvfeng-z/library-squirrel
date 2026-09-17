package entity

import (
	"database/sql"

	"github.com/library-squirrel/backend/base/model"
)

// ReWorkAuthor 作品与作者关联
type ReWorkAuthor struct {
	*model.BaseEntity
	AuthorType    sql.NullInt64 `gorm:"column:author_type" json:"authorType"`
	WorkID        sql.NullInt64 `gorm:"column:work_id;uniqueIndex:idx_re_work_author_work_local_author_role;uniqueIndex:idx_re_work_author_work_site_author_role" json:"workId"`
	LocalAuthorID sql.NullInt64 `gorm:"column:local_author_id;uniqueIndex:idx_re_work_author_work_local_author_role" json:"localAuthorId"`
	SiteAuthorID  sql.NullInt64 `gorm:"column:site_author_id;uniqueIndex:idx_re_work_author_work_site_author_role" json:"siteAuthorId"`
	// RoleName 关联级 role 维度值（作者在本作品上的角色分工，开放字符串，空串=无 role）。
	// 唯一键含本列——同作品同作者多 role 关联并存（身兼数职）
	RoleName  string        `gorm:"column:role_name;not null;default:'';uniqueIndex:idx_re_work_author_work_local_author_role;uniqueIndex:idx_re_work_author_work_site_author_role" json:"roleName"`
	SortOrder sql.NullInt64 `gorm:"column:sort_order" json:"sortOrder"`
	// Source 关联写入来源（constant.PLUGIN/MANUAL）：作品重拉只窄域重建插件来源关联，用户手动挂的关联不动；
	// 冲突 upsert 不写本列，先建行者的来源保持不变
	Source int64 `gorm:"column:source;not null;default:0" json:"source"`
}

func (ReWorkAuthor) TableName() string {
	return "re_work_author"
}

func NewReWorkAuthor() *ReWorkAuthor {
	return &ReWorkAuthor{
		BaseEntity: &model.BaseEntity{},
	}
}
