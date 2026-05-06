package dto

import (
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/util"
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

// ToSiteEntity 将 SiteDTO 转换为 Site 实体
func ToSiteEntity(dto *SiteDTO) *entity.Site {
	if dto == nil {
		return nil
	}

	newSite := entity.NewSite()

	// 设置基础字段
	if dto.ID != 0 {
		newSite.SetID(dto.ID)
	}

	// 设置业务字段
	if dto.SiteName != nil {
		newSite.SiteName.Valid = true
		newSite.SiteName.String = *dto.SiteName
	} else {
		newSite.SiteName.Valid = false
	}

	if dto.SiteDescription != nil {
		newSite.SiteDescription.Valid = true
		newSite.SiteDescription.String = *dto.SiteDescription
	} else {
		newSite.SiteDescription.Valid = false
	}

	if dto.Homepage != nil {
		newSite.Homepage.Valid = true
		newSite.Homepage.String = *dto.Homepage
	} else {
		newSite.Homepage.Valid = false
	}

	// 设置时间字段（如果DTO中有值则使用，否则让Repository自动处理）
	if dto.CreateTime != 0 {
		newSite.SetCreateTime(dto.CreateTime)
	}
	if dto.UpdateTime != 0 {
		newSite.SetUpdateTime(dto.UpdateTime)
	}

	return newSite
}
