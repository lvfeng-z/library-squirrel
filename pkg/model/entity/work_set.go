package entity

import (
	"database/sql"

	"github.com/library-squirrel/wails/pkg/model"
)

// WorkSet 作品集
type WorkSet struct {
	*model.BaseEntity
	SiteID                 sql.NullInt64  `gorm:"column:site_id;uniqueIndex:idx_work_set_site_site_set" json:"siteId"`
	SiteWorkSetID          sql.NullString `gorm:"column:site_work_set_id;uniqueIndex:idx_work_set_site_site_set" json:"siteWorkSetId"`
	SiteWorkSetName        sql.NullString `gorm:"column:site_work_set_name" json:"siteWorkSetName"`
	SiteAuthorID           sql.NullString `gorm:"column:site_author_id" json:"siteAuthorId"`
	SiteWorkSetDescription sql.NullString `gorm:"column:site_work_set_description" json:"siteWorkSetDescription"`
	SiteUploadTime         sql.NullInt64  `gorm:"column:site_upload_time" json:"siteUploadTime"`
	SiteUpdateTime         sql.NullInt64  `gorm:"column:site_update_time" json:"siteUpdateTime"`
	NickName               sql.NullString `gorm:"column:nick_name" json:"nickName"`
	LastView               sql.NullInt64  `gorm:"column:last_view" json:"lastView"`
}

func NewWorkSet() *WorkSet {
	return &WorkSet{
		BaseEntity: &model.BaseEntity{},
	}
}

// TableName 指定表名
func (WorkSet) TableName() string {
	return "work_set"
}
