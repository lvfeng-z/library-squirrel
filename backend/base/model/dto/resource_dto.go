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
		ID:                resource.GetID(),
		WorkID:            resource.WorkID,
		TaskID:            resource.TaskID,
		State:             resource.State,
		FilePath:          util.NullStringToPointer(resource.FilePath),
		FileName:          util.NullStringToPointer(resource.FileName),
		FilenameExtension: util.NullStringToPointer(resource.FilenameExtension),
		SuggestName:       util.NullStringToPointer(resource.SuggestName),
		ResourceSize:      util.NullInt64ToPointer(resource.ResourceSize),
		Workdir:           util.NullStringToPointer(resource.Workdir),
		ResourceComplete:  resource.ResourceComplete,
		CreateTime:        resource.GetCreateTime(),
		UpdateTime:        resource.GetUpdateTime(),
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
	newResource.State = dto.State
	newResource.ResourceComplete = dto.ResourceComplete

	if dto.FilePath != nil {
		newResource.FilePath.Valid = true
		newResource.FilePath.String = *dto.FilePath
	} else {
		newResource.FilePath.Valid = false
	}

	if dto.FileName != nil {
		newResource.FileName.Valid = true
		newResource.FileName.String = *dto.FileName
	} else {
		newResource.FileName.Valid = false
	}

	if dto.FilenameExtension != nil {
		newResource.FilenameExtension.Valid = true
		newResource.FilenameExtension.String = *dto.FilenameExtension
	} else {
		newResource.FilenameExtension.Valid = false
	}

	if dto.SuggestName != nil {
		newResource.SuggestName.Valid = true
		newResource.SuggestName.String = *dto.SuggestName
	} else {
		newResource.SuggestName.Valid = false
	}

	if dto.ResourceSize != nil {
		newResource.ResourceSize.Valid = true
		newResource.ResourceSize.Int64 = *dto.ResourceSize
	} else {
		newResource.ResourceSize.Valid = false
	}

	if dto.Workdir != nil {
		newResource.Workdir.Valid = true
		newResource.Workdir.String = *dto.Workdir
	} else {
		newResource.Workdir.Valid = false
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
