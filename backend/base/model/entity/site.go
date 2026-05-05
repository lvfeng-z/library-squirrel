package entity

import (
	"database/sql"

	"github.com/library-squirrel/wails/backend/base/model"
)

// Site 站点
type Site struct {
	*model.BaseEntity
	SiteName        sql.NullString `gorm:"column:site_name;uniqueIndex" json:"siteName"`
	SiteDescription sql.NullString `gorm:"column:site_description" json:"siteDescription"`
	Homepage        sql.NullString `gorm:"column:homepage" json:"homepage"`
}

func NewSite() *Site {
	return &Site{
		BaseEntity: &model.BaseEntity{},
	}
}

func (Site) TableName() string {
	return "site"
}
