package entity

import (
	"database/sql"

	"github.com/library-squirrel/backend/base/model"
)

// ReWorkTag 作品与标签关联
type ReWorkTag struct {
	*model.BaseEntity
	WorkID     sql.NullInt64 `gorm:"column:work_id;uniqueIndex:idx_re_work_tag_work_local_tag;uniqueIndex:idx_re_work_tag_work_site_tag" json:"workId"`
	TagType    sql.NullInt64 `gorm:"column:tag_type" json:"tagType"`
	LocalTagID sql.NullInt64 `gorm:"column:local_tag_id;uniqueIndex:idx_re_work_tag_work_local_tag" json:"localTagId"`
	SiteTagID  sql.NullInt64 `gorm:"column:site_tag_id;uniqueIndex:idx_re_work_tag_work_site_tag" json:"siteTagId"`
}

func (ReWorkTag) TableName() string {
	return "re_work_tag"
}

func NewReWorkTag() *ReWorkTag {
	return &ReWorkTag{
		BaseEntity: &model.BaseEntity{},
	}
}
