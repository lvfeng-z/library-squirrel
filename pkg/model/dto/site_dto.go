package dto

import (
	"github.com/library-squirrel/wails/internal/util"
	"github.com/library-squirrel/wails/pkg/model/entity"
)

// SiteDTO 站点数据传输对象（无 sql.Null* 版本）
type SiteDTO struct {
	ID              int64   `json:"id"`
	SiteName        *string `json:"siteName"`
	SiteDescription *string `json:"siteDescription"`
	Homepage        *string `json:"homepage"`
	CreateTime      int64   `json:"createTime"`
	UpdateTime      int64   `json:"updateTime"`
}

// NewSiteDTO 从 entity.Site 创建 SiteDTO
func NewSiteDTO(site *entity.Site) *SiteDTO {
	if site == nil {
		return nil
	}
	return &SiteDTO{
		ID:              site.GetID(),
		SiteName:        util.NullStringToPointer(site.SiteName),
		SiteDescription: util.NullStringToPointer(site.SiteDescription),
		Homepage:        util.NullStringToPointer(site.Homepage),
		CreateTime:      site.GetCreateTime(),
		UpdateTime:      site.GetUpdateTime(),
	}
}
