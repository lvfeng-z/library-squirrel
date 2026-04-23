package dto

import (
	"github.com/library-squirrel/wails/internal/util"
	"github.com/library-squirrel/wails/pkg/model/entity"
)

// LocalTagDTO 本地标签数据传输对象（无 sql.Null* 版本）
type LocalTagDTO struct {
	ID             int64   `json:"id"`
	LocalTagName   *string `json:"localTagName"`
	BaseLocalTagID *int64  `json:"baseLocalTagId"`
	Description    *string `json:"description"`
	LastUse        *int64  `json:"lastUse"`
	CreateTime     int64   `json:"createTime"`
	UpdateTime     int64   `json:"updateTime"`
}

// NewLocalTagDTO 从 entity.LocalTag 创建 LocalTagDTO
func NewLocalTagDTO(tag *entity.LocalTag) *LocalTagDTO {
	if tag == nil {
		return nil
	}
	return &LocalTagDTO{
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
func ToLocalTagEntity(dto *LocalTagDTO) *entity.LocalTag {
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
