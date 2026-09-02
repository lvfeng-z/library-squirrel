package entity

import (
	"database/sql"

	"github.com/library-squirrel/backend/base/model"
)

// Site 站点
type Site struct {
	*model.BaseEntity
	// SiteKey 站点唯一身份键（SDK identity 注册表分配的品牌 slug，如 "pixiv"）。
	// 跨库站点匹配、查重、关联一律以此键为准；SiteName 仅作展示
	SiteKey   string         `gorm:"column:site_key;not null;uniqueIndex" json:"siteKey"`
	SiteName  sql.NullString `gorm:"column:site_name" json:"siteName"`
	Homepage  sql.NullString `gorm:"column:homepage" json:"homepage"`
}

func NewSite() *Site {
	return &Site{
		BaseEntity: &model.BaseEntity{},
	}
}

func (Site) TableName() string {
	return "site"
}
