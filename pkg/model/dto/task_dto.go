package dto

import (
	"github.com/library-squirrel/wails/internal/util"
	entity2 "github.com/library-squirrel/wails/pkg/model/entity"
)

// TaskDTO 任务数据传输对象（无 sql.Null* 版本）
type TaskDTO struct {
	ID                   int64   `json:"id"`
	IsCollection         *int64  `json:"isCollection"`
	Pid                  *int64  `json:"pid"`
	TaskName             *string `json:"taskName"`
	SiteID               *int64  `json:"siteId"`
	SiteWorkID           *string `json:"siteWorkId"`
	URL                  *string `json:"url"`
	Status               int     `json:"status"`
	PendingResourceID    *int64  `json:"pendingResourceId"`
	Continuable          *int64  `json:"continuable"`
	PluginPublicID       *string `json:"pluginPublicId"`
	PluginContributionID *string `json:"pluginContributionId"`
	PluginData           *string `json:"pluginData"`
	ErrorMessage         *string `json:"errorMessage"`
	CreateTime           int64   `json:"createTime"`
	UpdateTime           int64   `json:"updateTime"`
}

// NewTaskDTO 从 entity.Task 创建 TaskDTO
func NewTaskDTO(task *entity2.Task) *TaskDTO {
	if task == nil {
		return nil
	}
	return &TaskDTO{
		ID:                   task.GetID(),
		IsCollection:         util.NullInt64ToPointer(task.IsCollection),
		Pid:                  util.NullInt64ToPointer(task.Pid),
		TaskName:             util.NullStringToPointer(task.TaskName),
		SiteID:               util.NullInt64ToPointer(task.SiteID),
		SiteWorkID:           util.NullStringToPointer(task.SiteWorkID),
		URL:                  util.NullStringToPointer(task.URL),
		Status:               task.Status,
		PendingResourceID:    util.NullInt64ToPointer(task.PendingResourceID),
		Continuable:          util.NullInt64ToPointer(task.Continuable),
		PluginPublicID:       util.NullStringToPointer(task.PluginPublicID),
		PluginContributionID: util.NullStringToPointer(task.PluginContributionID),
		PluginData:           util.NullStringToPointer(task.PluginData),
		ErrorMessage:         util.NullStringToPointer(task.ErrorMessage),
		CreateTime:           task.GetCreateTime(),
		UpdateTime:           task.GetUpdateTime(),
	}
}

// ToTaskEntity 将 TaskDTO 转换为 Task 实体
func ToTaskEntity(dto *TaskDTO) *entity2.Task {
	if dto == nil {
		return nil
	}

	entity := entity2.NewTask()

	// 设置基础字段
	if dto.ID != 0 {
		entity.SetID(dto.ID)
	}

	// 设置业务字段
	if dto.IsCollection != nil {
		entity.IsCollection.Valid = true
		entity.IsCollection.Int64 = *dto.IsCollection
	} else {
		entity.IsCollection.Valid = false
	}

	if dto.Pid != nil {
		entity.Pid.Valid = true
		entity.Pid.Int64 = *dto.Pid
	} else {
		entity.Pid.Valid = false
	}

	if dto.TaskName != nil {
		entity.TaskName.Valid = true
		entity.TaskName.String = *dto.TaskName
	} else {
		entity.TaskName.Valid = false
	}

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

	if dto.URL != nil {
		entity.URL.Valid = true
		entity.URL.String = *dto.URL
	} else {
		entity.URL.Valid = false
	}

	entity.Status = dto.Status

	if dto.PendingResourceID != nil {
		entity.PendingResourceID.Valid = true
		entity.PendingResourceID.Int64 = *dto.PendingResourceID
	} else {
		entity.PendingResourceID.Valid = false
	}

	if dto.Continuable != nil {
		entity.Continuable.Valid = true
		entity.Continuable.Int64 = *dto.Continuable
	} else {
		entity.Continuable.Valid = false
	}

	if dto.PluginPublicID != nil {
		entity.PluginPublicID.Valid = true
		entity.PluginPublicID.String = *dto.PluginPublicID
	} else {
		entity.PluginPublicID.Valid = false
	}

	if dto.PluginContributionID != nil {
		entity.PluginContributionID.Valid = true
		entity.PluginContributionID.String = *dto.PluginContributionID
	} else {
		entity.PluginContributionID.Valid = false
	}

	if dto.PluginData != nil {
		entity.PluginData.Valid = true
		entity.PluginData.String = *dto.PluginData
	} else {
		entity.PluginData.Valid = false
	}

	if dto.ErrorMessage != nil {
		entity.ErrorMessage.Valid = true
		entity.ErrorMessage.String = *dto.ErrorMessage
	} else {
		entity.ErrorMessage.Valid = false
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
