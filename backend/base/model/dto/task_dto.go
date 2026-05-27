package dto

import (
	entity2 "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/util"
)

// TaskDTO 任务数据传输对象（无 sql.Null* 版本）
type TaskDTO struct {
	ID                   int64   `json:"id"`
	HasChild             *bool   `json:"hasChild"`
	Pid                  *int64  `json:"pid"`
	TaskName             *string `json:"taskName"`
	SiteID               *int64  `json:"siteId"`
	SiteWorkID           *string `json:"siteWorkId"`
	URL                  *string `json:"url"`
	Status               int     `json:"status"`
	PendingResourceID    *int64  `json:"pendingResourceId"`
	Continuable          *bool   `json:"continuable"`
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
		HasChild:             util.NullBoolToPointer(task.HasChild),
		Pid:                  util.NullInt64ToPointer(task.Pid),
		TaskName:             util.NullStringToPointer(task.TaskName),
		SiteID:               util.NullInt64ToPointer(task.SiteID),
		SiteWorkID:           util.NullStringToPointer(task.SiteWorkID),
		URL:                  util.NullStringToPointer(task.URL),
		Status:               task.Status,
		PendingResourceID:    util.NullInt64ToPointer(task.PendingResourceID),
		Continuable:          util.NullBoolToPointer(task.Continuable),
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
	if dto.HasChild != nil {
		entity.HasChild.Valid = true
		entity.HasChild.Bool = *dto.HasChild
	} else {
		entity.HasChild.Valid = false
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
		entity.Continuable.Bool = *dto.Continuable
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

// ========== 任务进度相关 DTO ==========

// TaskProgressDTO 任务进度DTO（组合 TaskDTO + 进度/站点名称/进度百分比字段）
type TaskProgressDTO struct {
	Task     *TaskDTO `json:"task,omitempty"`
	Total    *int64   `json:"total,omitempty"`
	Finished *int64   `json:"finished,omitempty"`
	SiteName *string  `json:"siteName,omitempty"`
	Schedule *int     `json:"schedule,"` // 任务进度百分比（100 表示完成）
}

// NewTaskProgressDTO 从 TaskDTO 创建 TaskProgressDTO
func NewTaskProgressDTO(taskDTO *TaskDTO) *TaskProgressDTO {
	if taskDTO == nil {
		return nil
	}
	return &TaskProgressDTO{
		Task: taskDTO,
	}
}

// TaskProgressTreeDTO 任务进度树DTO（组合 TaskProgressDTO + 树形结构字段）
type TaskProgressTreeDTO struct {
	TaskProgress *TaskProgressDTO       `json:"taskProgress,omitempty"`
	Children     []*TaskProgressTreeDTO `json:"children,omitempty"`
	HasChildren  *bool                  `json:"hasChildren,omitempty"`
	IsLeaf       *bool                  `json:"isLeaf,omitempty"`
}

// NewTaskProgressTreeDTO 从 TaskDTO 创建 TaskProgressTreeDTO
func NewTaskProgressTreeDTO(taskDTO *TaskDTO) *TaskProgressTreeDTO {
	if taskDTO == nil {
		return nil
	}
	hasChildren := taskDTO.HasChild != nil && *taskDTO.HasChild
	return &TaskProgressTreeDTO{
		TaskProgress: NewTaskProgressDTO(taskDTO),
		Children:     make([]*TaskProgressTreeDTO, 0),
		HasChildren:  &hasChildren,
		IsLeaf:       new(!hasChildren),
	}
}

// ========== 任务创建/树数据请求 DTO ==========

// CreateTaskRequest 创建任务请求
type CreateTaskRequest struct {
	Pid                  int64  `json:"pid"`
	TaskName             string `json:"taskName"`
	SiteID               int    `json:"siteId"`
	SiteWorkID           string `json:"siteWorkId"`
	URL                  string `json:"url"`
	HasChild             bool   `json:"hasChild"`
	PluginPublicID       string `json:"pluginPublicId"`
	PluginContributionID string `json:"pluginContributionId"`
	PluginData           string `json:"pluginData"`
}

// TreeDataPageDTO 任务树数据分页DTO
type TreeDataPageDTO struct {
	TreeID   int64                  `json:"treeId"`
	TreeName string                 `json:"treeName"`
	Total    int64                  `json:"total"`
	Tasks    []*TaskProgressTreeDTO `json:"tasks"`
}
