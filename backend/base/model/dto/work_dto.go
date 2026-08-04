package dto

import (
	entity2 "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/util"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// NewWorkDTO 从 entity.Work 创建 WorkDTO
func NewWorkDTO(work *entity2.Work) *sdkdto.WorkDTO {
	if work == nil {
		return nil
	}
	return &sdkdto.WorkDTO{
		ID:                  work.GetID(),
		SiteID:              util.NullInt64ToPointer(work.SiteID),
		SiteWorkID:          util.NullStringToPointer(work.SiteWorkID),
		SiteWorkName:        util.NullStringToPointer(work.SiteWorkName),
		SiteAuthorID:        util.NullStringToPointer(work.SiteAuthorID),
		SiteWorkDescription: util.NullStringToPointer(work.SiteWorkDescription),
		SiteUploadTime:      util.NullInt64ToPointer(work.SiteUploadTime),
		SiteUpdateTime:      util.NullInt64ToPointer(work.SiteUpdateTime),
		NickName:            util.NullStringToPointer(work.NickName),
		LocalAuthorID:       util.NullInt64ToPointer(work.LocalAuthorID),
		LastView:            util.NullInt64ToPointer(work.LastView),
		CreateTime:          work.GetCreateTime(),
		UpdateTime:          work.GetUpdateTime(),
	}
}

// WorkFullDTO 作品完整信息DTO
type WorkFullDTO struct {
	Work         *sdkdto.WorkDTO       `json:"work,omitempty"`
	LocalAuthors []*RankedLocalAuthor  `json:"localAuthors,omitempty"`
	SiteAuthors  []*RankedSiteAuthor   `json:"siteAuthors,omitempty"`
	Site         *sdkdto.SiteDTO       `json:"site,omitempty"`
	LocalTags    []*sdkdto.LocalTagDTO `json:"localTags,omitempty"`
	SiteTags     []*SiteTagFullDTO     `json:"siteTags,omitempty"`
	Resource     *ResourceFullDTO      `json:"resource,omitempty"` // 单个活跃资源（含 PersistentStore 信息）
}

// NewWorkFullDTO 创建WorkFullDTO
func NewWorkFullDTO(work *entity2.Work) *WorkFullDTO {
	return &WorkFullDTO{
		Work: NewWorkDTO(work),
	}
}

// ToWorkEntity 将 WorkDTO 转换为 Work 实体
func ToWorkEntity(dto *sdkdto.WorkDTO) *entity2.Work {
	if dto == nil {
		return nil
	}

	entity := entity2.NewWork()

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

	if dto.SiteWorkID != nil {
		entity.SiteWorkID.Valid = true
		entity.SiteWorkID.String = *dto.SiteWorkID
	} else {
		entity.SiteWorkID.Valid = false
	}

	if dto.SiteWorkName != nil {
		entity.SiteWorkName.Valid = true
		entity.SiteWorkName.String = *dto.SiteWorkName
	} else {
		entity.SiteWorkName.Valid = false
	}

	if dto.SiteAuthorID != nil {
		entity.SiteAuthorID.Valid = true
		entity.SiteAuthorID.String = *dto.SiteAuthorID
	} else {
		entity.SiteAuthorID.Valid = false
	}

	if dto.SiteWorkDescription != nil {
		entity.SiteWorkDescription.Valid = true
		entity.SiteWorkDescription.String = *dto.SiteWorkDescription
	} else {
		entity.SiteWorkDescription.Valid = false
	}

	if dto.SiteUploadTime != nil {
		entity.SiteUploadTime.Valid = true
		entity.SiteUploadTime.Int64 = *dto.SiteUploadTime
	} else {
		entity.SiteUploadTime.Valid = false
	}

	if dto.SiteUpdateTime != nil {
		entity.SiteUpdateTime.Valid = true
		entity.SiteUpdateTime.Int64 = *dto.SiteUpdateTime
	} else {
		entity.SiteUpdateTime.Valid = false
	}

	if dto.NickName != nil {
		entity.NickName.Valid = true
		entity.NickName.String = *dto.NickName
	} else {
		entity.NickName.Valid = false
	}

	if dto.LocalAuthorID != nil {
		entity.LocalAuthorID.Valid = true
		entity.LocalAuthorID.Int64 = *dto.LocalAuthorID
	} else {
		entity.LocalAuthorID.Valid = false
	}

	if dto.LastView != nil {
		entity.LastView.Valid = true
		entity.LastView.Int64 = *dto.LastView
	} else {
		entity.LastView.Valid = false
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
