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

// ToSiteEntity 将 SiteDTO 转换为 Site 实体
func ToSiteEntity(dto *SiteDTO) *entity.Site {
	if dto == nil {
		return nil
	}

	entity := &entity.Site{}

	// 设置基础字段
	if dto.ID != 0 {
		entity.SetID(dto.ID)
	}

	// 设置业务字段
	if dto.SiteName != nil {
		entity.SiteName.Valid = true
		entity.SiteName.String = *dto.SiteName
	} else {
		entity.SiteName.Valid = false
	}

	if dto.SiteDescription != nil {
		entity.SiteDescription.Valid = true
		entity.SiteDescription.String = *dto.SiteDescription
	} else {
		entity.SiteDescription.Valid = false
	}

	if dto.Homepage != nil {
		entity.Homepage.Valid = true
		entity.Homepage.String = *dto.Homepage
	} else {
		entity.Homepage.Valid = false
	}

	// 设置时间字段（如果DTO中有值则使用，否则让Repository自动处理）
	if dto.CreateTime != 0 {
		entity.SetCreateTime(dto.CreateTime)
	}
	if dto.UpdateTime != 0 {
		entity.SetUpdateTime(dto.UpdateTime)
	}

	return entity
}
