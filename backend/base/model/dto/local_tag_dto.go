package dto

import (
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/util"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// NewLocalTagDTO 从 entity.LocalTag 创建 LocalTagDTO
func NewLocalTagDTO(tag *entity.LocalTag) *sdkdto.LocalTagDTO {
	if tag == nil {
		return nil
	}
	return &sdkdto.LocalTagDTO{
		ID:             tag.GetID(),
		LocalTagName:   util.NullStringToPointer(tag.LocalTagName),
		BaseLocalTagID: util.NullInt64ToPointer(tag.BaseLocalTagID),
		Description:    util.NullStringToPointer(tag.Description),
		LastUse:        util.NullInt64ToPointer(tag.LastUse),
		CreateTime:     tag.GetCreateTime(),
		UpdateTime:     tag.GetUpdateTime(),
	}
}

// ToLocalTagEntity 将 LocalTagDTO 转换为 LocalTag 实体
func ToLocalTagEntity(dto *sdkdto.LocalTagDTO) *entity.LocalTag {
	if dto == nil {
		return nil
	}

	entity := entity.NewLocalTag()

	// 设置基础字段
	if dto.ID != 0 {
		entity.SetID(dto.ID)
	}

	// 设置业务字段
	if dto.LocalTagName != nil {
		entity.LocalTagName.Valid = true
		entity.LocalTagName.String = *dto.LocalTagName
	} else {
		entity.LocalTagName.Valid = false
	}

	if dto.BaseLocalTagID != nil {
		entity.BaseLocalTagID.Valid = true
		entity.BaseLocalTagID.Int64 = *dto.BaseLocalTagID
	} else {
		entity.BaseLocalTagID.Valid = false
	}

	if dto.Description != nil {
		entity.Description.Valid = true
		entity.Description.String = *dto.Description
	} else {
		entity.Description.Valid = false
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

// LocalTagWithBaseTagDTO 本地标签及其基础标签数据传输对象
type LocalTagWithBaseTagDTO struct {
	LocalTag *sdkdto.LocalTagDTO `json:"localTag,omitempty"`
	BaseTag  *sdkdto.LocalTagDTO `json:"baseTag,omitempty"`
}

// NewLocalTagWithBaseTagDTO 从 entity.LocalTag 创建 LocalTagWithBaseTagDTO
func NewLocalTagWithBaseTagDTO(tag *entity.LocalTag, baseTag *entity.LocalTag) *LocalTagWithBaseTagDTO {
	if tag == nil {
		return nil
	}
	return &LocalTagWithBaseTagDTO{
		LocalTag: NewLocalTagDTO(tag),
		BaseTag:  NewLocalTagDTO(baseTag),
	}
}
