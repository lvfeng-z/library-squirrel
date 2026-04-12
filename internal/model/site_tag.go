package model

import (
	"database/sql"

	"github.com/library-squirrel/wails/pkg/model"
)

// SiteTag 站点标签
type SiteTag struct {
	*model.BaseEntity
	SiteID        sql.NullInt64  `gorm:"column:site_id;uniqueIndex:idx_site_tag_site_site_tag" json:"siteId"`
	SiteTagID     sql.NullString `gorm:"column:site_tag_id;uniqueIndex:idx_site_tag_site_site_tag" json:"siteTagId"`
	SiteTagName   sql.NullString `gorm:"column:site_tag_name" json:"siteTagName"`
	BaseSiteTagID sql.NullString `gorm:"column:base_site_tag_id" json:"baseSiteTagId"`
	Description   sql.NullString `gorm:"column:description" json:"description"`
	LocalTagID    sql.NullInt64  `gorm:"column:local_tag_id" json:"localTagId"`
	LastUse       sql.NullInt64  `gorm:"column:last_use" json:"lastUse"`
}

func NewSiteTag() *SiteTag {
	return &SiteTag{
		BaseEntity: &model.BaseEntity{},
	}
}

func (SiteTag) TableName() string {
	return "site_tag"
}
