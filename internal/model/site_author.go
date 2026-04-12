package model

import (
	"database/sql"

	"github.com/library-squirrel/wails/pkg/model"
)

// SiteAuthor 站点作者
type SiteAuthor struct {
	*model.BaseEntity
	SiteID               sql.NullInt64  `gorm:"column:site_id;uniqueIndex:idx_site_author_site_site_author" json:"siteId"`
	SiteAuthorID         sql.NullString `gorm:"column:site_author_id;uniqueIndex:idx_site_author_site_site_author" json:"siteAuthorId"`
	AuthorName           sql.NullString `gorm:"column:author_name" json:"authorName"`
	FixedAuthorName      sql.NullString `gorm:"column:fixed_author_name" json:"fixedAuthorName"`
	SiteAuthorNameBefore sql.NullString `gorm:"column:site_author_name_before" json:"siteAuthorNameBefore"`
	Introduce            sql.NullString `gorm:"column:introduce" json:"introduce"`
	LocalAuthorID        sql.NullInt64  `gorm:"column:local_author_id" json:"localAuthorId"`
	LastUse              sql.NullInt64  `gorm:"column:last_use" json:"lastUse"`
}

func NewSiteAuthor() *SiteAuthor {
	return &SiteAuthor{
		BaseEntity: &model.BaseEntity{},
	}
}

func (SiteAuthor) TableName() string {
	return "site_author"
}
