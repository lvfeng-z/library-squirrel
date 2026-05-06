package entity

import (
	"database/sql"

	"github.com/library-squirrel/backend/base/model"
)

// Work 作品
type Work struct {
	*model.BaseEntity
	SiteID              sql.NullInt64  `gorm:"column:site_id;uniqueIndex:idx_work_site_site_work" json:"siteId"`
	SiteWorkID          sql.NullString `gorm:"column:site_work_id;uniqueIndex:idx_work_site_site_work" json:"siteWorkId"`
	SiteWorkName        sql.NullString `gorm:"column:site_work_name" json:"siteWorkName"`
	SiteAuthorID        sql.NullString `gorm:"column:site_author_id" json:"siteAuthorId"`
	SiteWorkDescription sql.NullString `gorm:"column:site_work_description" json:"siteWorkDescription"`
	SiteUploadTime      sql.NullInt64  `gorm:"column:site_upload_time" json:"siteUploadTime"`
	SiteUpdateTime      sql.NullInt64  `gorm:"column:site_update_time" json:"siteUpdateTime"`
	NickName            sql.NullString `gorm:"column:nick_name" json:"nickName"`
	LocalAuthorID       sql.NullInt64  `gorm:"column:local_author_id" json:"localAuthorId"`
	LastView            sql.NullInt64  `gorm:"column:last_view" json:"lastView"`
}

func NewWork() *Work {
	return &Work{
		BaseEntity: &model.BaseEntity{},
	}
}

func (Work) TableName() string {
	return "work"
}
