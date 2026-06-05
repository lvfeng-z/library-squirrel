package dto

import (
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/util"
	sdkdto "github.com/lvfeng-z/library-squirrel-plugin-sdk/dto"
)

// NewResourceDTO 从 entity.Resource 创建 ResourceDTO
func NewResourceDTO(resource *entity.Resource) *sdkdto.ResourceDTO {
	if resource == nil {
		return nil
	}
	return &sdkdto.ResourceDTO{
		ID:               resource.GetID(),
		WorkID:           resource.WorkID,
		TaskID:           resource.TaskID,
		Enabled:          resource.Enabled,
		SuggestName:      util.NullStringToPointer(resource.SuggestName),
		ResourceComplete: resource.ResourceComplete,
		CreateTime:       resource.GetCreateTime(),
		UpdateTime:       resource.GetUpdateTime(),
	}
}

// ToResourceEntity 将 ResourceDTO 转换为 Resource 实体
func ToResourceEntity(dto *sdkdto.ResourceDTO) *entity.Resource {
	if dto == nil {
		return nil
	}

	newResource := entity.NewResource()

	// 设置基础字段
	if dto.ID != 0 {
		newResource.SetID(dto.ID)
	}

	// 设置业务字段
	newResource.WorkID = dto.WorkID
	newResource.TaskID = dto.TaskID
	newResource.Enabled = dto.Enabled
	newResource.ResourceComplete = dto.ResourceComplete

	if dto.SuggestName != nil {
		newResource.SuggestName.Valid = true
		newResource.SuggestName.String = *dto.SuggestName
	} else {
		newResource.SuggestName.Valid = false
	}

	// 设置时间字段（如果DTO中有值则使用，否则让Repository自动处理）
	if dto.CreateTime != 0 {
		newResource.SetCreateTime(dto.CreateTime)
	}
	if dto.UpdateTime != 0 {
		newResource.SetUpdateTime(dto.UpdateTime)
	}

	return newResource
}

// NewResourceFullDTO 从 entity.Resource 和关联的 PersistentStore 实体创建 ResourceFullDTO
func NewResourceFullDTO(resource *entity.Resource, workStore, thumbnailStore *entity.PersistentStore) *sdkdto.ResourceFullDTO {
	if resource == nil {
		return nil
	}
	return &sdkdto.ResourceFullDTO{
		ID:               resource.GetID(),
		WorkID:           resource.WorkID,
		TaskID:           resource.TaskID,
		Enabled:          resource.Enabled,
		SuggestName:      util.NullStringToPointer(resource.SuggestName),
		ResourceComplete: resource.ResourceComplete,
		WorkStoreID:      util.NullInt64ToPointer(resource.WorkStoreID),
		ThumbnailStoreID: util.NullInt64ToPointer(resource.ThumbnailStoreID),
		WorkStore:        NewPersistentStoreDTO(workStore),
		ThumbnailStore:   NewPersistentStoreDTO(thumbnailStore),
		CreateTime:       resource.GetCreateTime(),
		UpdateTime:       resource.GetUpdateTime(),
	}
}
