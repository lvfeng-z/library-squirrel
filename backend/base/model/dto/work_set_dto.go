package dto

import (
	entity2 "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/util"
)

// WorkSetDTO 作品集数据传输对象（无 sql.Null* 版本）
type WorkSetDTO struct {
	ID                     int64   `json:"id"`
	SiteID                 *int64  `json:"siteId"`
	SiteWorkSetID          *string `json:"siteWorkSetId"`
	SiteWorkSetName        *string `json:"siteWorkSetName"`
	SiteAuthorID           *string `json:"siteAuthorId"`
	SiteWorkSetDescription *string `json:"siteWorkSetDescription"`
	SiteUploadTime         *int64  `json:"siteUploadTime"`
	SiteUpdateTime         *int64  `json:"siteUpdateTime"`
	NickName               *string `json:"nickName"`
	LastView               *int64  `json:"lastView"`
	CreateTime             int64   `json:"createTime"`
	UpdateTime             int64   `json:"updateTime"`
}

// NewWorkSetDTO 从 entity.WorkSet 创建 WorkSetDTO
func NewWorkSetDTO(workSet *entity2.WorkSet) *WorkSetDTO {
	if workSet == nil {
		return nil
	}
	return &WorkSetDTO{
		ID:                     workSet.GetID(),
		SiteID:                 util.NullInt64ToPointer(workSet.SiteID),
		SiteWorkSetID:          util.NullStringToPointer(workSet.SiteWorkSetID),
		SiteWorkSetName:        util.NullStringToPointer(workSet.SiteWorkSetName),
		SiteAuthorID:           util.NullStringToPointer(workSet.SiteAuthorID),
		SiteWorkSetDescription: util.NullStringToPointer(workSet.SiteWorkSetDescription),
		SiteUploadTime:         util.NullInt64ToPointer(workSet.SiteUploadTime),
		SiteUpdateTime:         util.NullInt64ToPointer(workSet.SiteUpdateTime),
		NickName:               util.NullStringToPointer(workSet.NickName),
		LastView:               util.NullInt64ToPointer(workSet.LastView),
		CreateTime:             workSet.GetCreateTime(),
		UpdateTime:             workSet.GetUpdateTime(),
	}
}

// ToWorkSetEntity 将 WorkSetDTO 转换为 WorkSet 实体
func ToWorkSetEntity(dto *WorkSetDTO) *entity2.WorkSet {
	if dto == nil {
		return nil
	}

	entity := entity2.NewWorkSet()

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

	if dto.SiteWorkSetID != nil {
		entity.SiteWorkSetID.Valid = true
		entity.SiteWorkSetID.String = *dto.SiteWorkSetID
	} else {
		entity.SiteWorkSetID.Valid = false
	}

	if dto.SiteWorkSetName != nil {
		entity.SiteWorkSetName.Valid = true
		entity.SiteWorkSetName.String = *dto.SiteWorkSetName
	} else {
		entity.SiteWorkSetName.Valid = false
	}

	if dto.SiteAuthorID != nil {
		entity.SiteAuthorID.Valid = true
		entity.SiteAuthorID.String = *dto.SiteAuthorID
	} else {
		entity.SiteAuthorID.Valid = false
	}

	if dto.SiteWorkSetDescription != nil {
		entity.SiteWorkSetDescription.Valid = true
		entity.SiteWorkSetDescription.String = *dto.SiteWorkSetDescription
	} else {
		entity.SiteWorkSetDescription.Valid = false
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

// WorkSetWithWorksResultDTO 作品集及其作品信息（作品包含完整关联数据）
type WorkSetWithWorksResultDTO struct {
	WorkSet *WorkSetDTO    `json:"workSet"`
	Works   []*WorkFullDTO `json:"works,omitempty"`
}

// WorkSetWithCoverDTO 作品集及其封面作品信息
type WorkSetWithCoverDTO struct {
	WorkSet       *WorkSetDTO  `json:"workSet"`
	CoverWork     *WorkDTO     `json:"coverWork,omitempty"`
	CoverResource *ResourceDTO `json:"coverResource,omitempty"`
}
