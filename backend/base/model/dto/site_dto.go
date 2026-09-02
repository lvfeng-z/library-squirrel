package dto

import (
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/util"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// NewSiteDTO 从 entity.Site 创建 SiteDTO
func NewSiteDTO(site *entity.Site) *sdkdto.SiteDTO {
	if site == nil {
		return nil
	}
	return &sdkdto.SiteDTO{
		Id:         site.GetID(),
		SiteKey:    site.SiteKey,
		SiteName:   util.NullStringToPointer(site.SiteName),
		Homepage:   util.NullStringToPointer(site.Homepage),
		CreateTime:      site.GetCreateTime(),
		UpdateTime:      site.GetUpdateTime(),
	}
}

// ToSiteEntity 将 SiteDTO 转换为 Site 实体
func ToSiteEntity(dto *sdkdto.SiteDTO) *entity.Site {
	if dto == nil {
		return nil
	}

	newSite := entity.NewSite()

	// 设置基础字段
	if dto.Id != 0 {
		newSite.SetID(dto.Id)
	}

	// 站点唯一身份键（NOT NULL 列，经前端/插件侧传入）
	newSite.SiteKey = dto.SiteKey

	// 设置业务字段
	if dto.SiteName != nil {
		newSite.SiteName.Valid = true
		newSite.SiteName.String = *dto.SiteName
	} else {
		newSite.SiteName.Valid = false
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
