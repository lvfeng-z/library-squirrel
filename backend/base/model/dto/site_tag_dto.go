package dto

import (
	entity2 "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/util"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// SiteTagDTO 站点标签数据传输对象
type SiteTagDTO struct {
	ID            int64   `json:"id"`
	SiteID        *int64  `json:"siteId"`
	SiteTagID     *string `json:"siteTagId"`
	SiteTagName   *string `json:"siteTagName"`
	BaseSiteTagID *string `json:"baseSiteTagId"`
	Description   *string `json:"description"`
	LocalTagID    *int64  `json:"localTagId"`
	LastUse       *int64  `json:"lastUse"`
	CreateTime    int64   `json:"createTime"`
	UpdateTime    int64   `json:"updateTime"`
}

// NewSiteTagDTO 从 entity.SiteTag 创建 SiteTagDTO
func NewSiteTagDTO(tag *entity2.SiteTag) *SiteTagDTO {
	if tag == nil {
		return nil
	}
	return &SiteTagDTO{
		ID:            tag.GetID(),
		SiteID:        util.NullInt64ToPointer(tag.SiteID),
		SiteTagID:     util.NullStringToPointer(tag.SiteTagID),
		SiteTagName:   util.NullStringToPointer(tag.SiteTagName),
		BaseSiteTagID: util.NullStringToPointer(tag.BaseSiteTagID),
		Description:   util.NullStringToPointer(tag.Description),
		LocalTagID:    util.NullInt64ToPointer(tag.LocalTagID),
		LastUse:       util.NullInt64ToPointer(tag.LastUse),
		CreateTime:    tag.GetCreateTime(),
		UpdateTime:    tag.GetUpdateTime(),
	}
}

// ToSiteTagEntity 将 SiteTagDTO 转换为 SiteTag 实体
func ToSiteTagEntity(dto *SiteTagDTO) *entity2.SiteTag {
	if dto == nil {
		return nil
	}

	entity := entity2.NewSiteTag()

	// 设置基础字段
	if dto.ID != 0 {
		entity.SetID(dto.ID)
	}

	// 设置业务字段
	if dto.SiteID != nil {
		entity.SiteID.Valid = true
		entity.SiteID.Int64 = *dto.SiteID
	} else {
		entity.SiteID.Valid = false
	}

	if dto.SiteTagID != nil {
		entity.SiteTagID.Valid = true
		entity.SiteTagID.String = *dto.SiteTagID
	} else {
		entity.SiteTagID.Valid = false
	}

	if dto.SiteTagName != nil {
		entity.SiteTagName.Valid = true
		entity.SiteTagName.String = *dto.SiteTagName
	} else {
		entity.SiteTagName.Valid = false
	}

	if dto.BaseSiteTagID != nil {
		entity.BaseSiteTagID.Valid = true
		entity.BaseSiteTagID.String = *dto.BaseSiteTagID
	} else {
		entity.BaseSiteTagID.Valid = false
	}

	if dto.Description != nil {
		entity.Description.Valid = true
		entity.Description.String = *dto.Description
	} else {
		entity.Description.Valid = false
	}

	if dto.LocalTagID != nil {
		entity.LocalTagID.Valid = true
		entity.LocalTagID.Int64 = *dto.LocalTagID
	} else {
		entity.LocalTagID.Valid = false
	}

	if dto.LastUse != nil {
		entity.LastUse.Valid = true
		entity.LastUse.Int64 = *dto.LastUse
	} else {
		entity.LastUse.Valid = false
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

// SiteTagFullDTO 站点标签完整DTO（包含绑定的本地标签和来源站点信息）
type SiteTagFullDTO struct {
	SiteTag  *SiteTagDTO         `json:"siteTag,omitempty"`
	LocalTag *sdkdto.LocalTagDTO `json:"localTag,omitempty"`
	// 来源站点
	Site *sdkdto.SiteDTO `json:"site,omitempty"`
}

// NewSiteTagFullDTO 创建站点标签完整DTO
func NewSiteTagFullDTO(siteTag *entity2.SiteTag) *SiteTagFullDTO {
	if siteTag == nil {
		return nil
	}
	dto := &SiteTagFullDTO{
		SiteTag: &SiteTagDTO{
			ID:            siteTag.GetID(),
			SiteID:        util.NullInt64ToPointer(siteTag.SiteID),
			SiteTagID:     util.NullStringToPointer(siteTag.SiteTagID),
			SiteTagName:   util.NullStringToPointer(siteTag.SiteTagName),
			BaseSiteTagID: util.NullStringToPointer(siteTag.BaseSiteTagID),
			Description:   util.NullStringToPointer(siteTag.Description),
			LocalTagID:    util.NullInt64ToPointer(siteTag.LocalTagID),
			LastUse:       util.NullInt64ToPointer(siteTag.LastUse),
			CreateTime:    siteTag.GetCreateTime(),
			UpdateTime:    siteTag.GetUpdateTime(),
		},
	}
	return dto
}

// SiteTagLocalRelateDTO 站点标签与本地标签关联DTO（包含绑定的本地标签、来源站点信息和同名本地标签判断）
type SiteTagLocalRelateDTO struct {
	SiteTag             *SiteTagDTO         `json:"siteTag,omitempty"`
	LocalTag            *sdkdto.LocalTagDTO `json:"localTag,omitempty"`
	Site                *sdkdto.SiteDTO     `json:"site,omitempty"`
	HasSameNameLocalTag bool                `json:"hasSameNameLocalTag"`
}

// NewSiteTagLocalRelateDTO 创建站点标签与本地标签关联DTO
func NewSiteTagLocalRelateDTO(siteTag *entity2.SiteTag) *SiteTagLocalRelateDTO {
	if siteTag == nil {
		return nil
	}
	return &SiteTagLocalRelateDTO{
		SiteTag: NewSiteTagDTO(siteTag),
	}
}
