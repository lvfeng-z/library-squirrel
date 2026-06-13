package entity

import (
	"database/sql"

	"github.com/library-squirrel/backend/base/model"
)

// ReWorkAuthor 作品与作者关联
type ReWorkAuthor struct {
	*model.BaseEntity
	AuthorType    sql.NullInt64  `gorm:"column:author_type" json:"authorType"`
	WorkID        sql.NullInt64  `gorm:"column:work_id;index:idx_re_work_author_local_author_id;index:idx_re_work_author_site_author_id" json:"workId"`
	LocalAuthorID sql.NullInt64  `gorm:"column:local_author_id;index:idx_re_work_author_local_author_id" json:"localAuthorId"`
	SiteAuthorID  sql.NullInt64  `gorm:"column:site_author_id;index:idx_re_work_author_site_author_id" json:"siteAuthorId"`
	RoleName      sql.NullString `gorm:"column:role_name" json:"roleName"`
	SortOrder     sql.NullInt64  `gorm:"column:sort_order" json:"sortOrder"`
}

func (ReWorkAuthor) TableName() string {
	return "re_work_author"
}
