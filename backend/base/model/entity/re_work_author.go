package entity

import (
	"database/sql"

	"github.com/library-squirrel/backend/base/model"
)

// ReWorkAuthor 作品与作者关联
type ReWorkAuthor struct {
	*model.BaseEntity
	AuthorType    sql.NullInt64  `gorm:"column:author_type" json:"authorType"`
	WorkID        sql.NullInt64  `gorm:"column:work_id;uniqueIndex:idx_re_work_author_work_local_author;uniqueIndex:idx_re_work_author_work_site_author" json:"workId"`
	LocalAuthorID sql.NullInt64  `gorm:"column:local_author_id;uniqueIndex:idx_re_work_author_work_local_author" json:"localAuthorId"`
	SiteAuthorID  sql.NullInt64  `gorm:"column:site_author_id;uniqueIndex:idx_re_work_author_work_site_author" json:"siteAuthorId"`
	RoleName      sql.NullString `gorm:"column:role_name" json:"roleName"`
	SortOrder     sql.NullInt64  `gorm:"column:sort_order" json:"sortOrder"`
}

func (ReWorkAuthor) TableName() string {
	return "re_work_author"
}
