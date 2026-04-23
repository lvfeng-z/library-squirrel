package dto

import (
	"github.com/library-squirrel/wails/internal/util"
	"github.com/library-squirrel/wails/pkg/model/entity"
)

// ResourceDTO 资源数据传输对象（无 sql.Null* 版本）
type ResourceDTO struct {
	ID                int64   `json:"id"`
	WorkID            int64   `json:"workId"`
	TaskID            int64   `json:"taskId"`
	State             int     `json:"state"`
	FilePath          *string `json:"filePath"`
	FileName          *string `json:"fileName"`
	FilenameExtension *string `json:"filenameExtension"`
	SuggestName       *string `json:"suggestName"`
	ResourceSize      *int64  `json:"resourceSize"`
	Workdir           *string `json:"workdir"`
	ResourceComplete  int     `json:"resourceComplete"`
	CreateTime        int64   `json:"createTime"`
	UpdateTime        int64   `json:"updateTime"`
}

// NewResourceDTO 从 entity.Resource 创建 ResourceDTO
func NewResourceDTO(resource *entity.Resource) *ResourceDTO {
	if resource == nil {
		return nil
	}
	return &ResourceDTO{
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
func ToResourceEntity(dto *ResourceDTO) *entity.Resource {
	if dto == nil {
		return nil
	}

	entity := &entity.Resource{}

	// 设置基础字段
	if dto.ID != 0 {
		entity.SetID(dto.ID)
	}

	// 设置业务字段
	entity.WorkID = dto.WorkID
	entity.TaskID = dto.TaskID
	entity.State = dto.State
	entity.ResourceComplete = dto.ResourceComplete

	if dto.FilePath != nil {
		entity.FilePath.Valid = true
		entity.FilePath.String = *dto.FilePath
	} else {
		entity.FilePath.Valid = false
	}

	if dto.FileName != nil {
		entity.FileName.Valid = true
		entity.FileName.String = *dto.FileName
	} else {
		entity.FileName.Valid = false
	}

	if dto.FilenameExtension != nil {
		entity.FilenameExtension.Valid = true
		entity.FilenameExtension.String = *dto.FilenameExtension
	} else {
		entity.FilenameExtension.Valid = false
	}

	if dto.SuggestName != nil {
		entity.SuggestName.Valid = true
		entity.SuggestName.String = *dto.SuggestName
	} else {
		entity.SuggestName.Valid = false
	}

	if dto.ResourceSize != nil {
		entity.ResourceSize.Valid = true
		entity.ResourceSize.Int64 = *dto.ResourceSize
	} else {
		entity.ResourceSize.Valid = false
	}

	if dto.Workdir != nil {
		entity.Workdir.Valid = true
		entity.Workdir.String = *dto.Workdir
	} else {
		entity.Workdir.Valid = false
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
