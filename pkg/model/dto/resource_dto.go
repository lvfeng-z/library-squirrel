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
