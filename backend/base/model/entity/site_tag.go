package entity

import (
	"database/sql"

	"github.com/library-squirrel/backend/base/model"
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
	Namespace     sql.NullString `gorm:"column:namespace" json:"namespace"` // 站点侧 namespace（language/character/parody/female/male/misc/general 等）；null=站点无 namespace（pixiv 等）
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
