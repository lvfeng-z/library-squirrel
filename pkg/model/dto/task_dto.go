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

// TaskParamDTO 任务数据传输对象（增删改参数）
type TaskParamDTO struct {
	ID                   int64   `json:"id"`
	IsCollection         *int64  `json:"isCollection,omitempty"`
	Pid                  *int64  `json:"pid,omitempty"`
	TaskName             *string `json:"taskName,omitempty"`
	SiteID               *int64  `json:"siteId,omitempty"`
	SiteWorkID           *string `json:"siteWorkId,omitempty"`
	URL                  *string `json:"url,omitempty"`
	Status               int     `json:"status,omitempty"`
	PendingResourceID    *int64  `json:"pendingResourceId,omitempty"`
	Continuable          *int64  `json:"continuable,omitempty"`
	PluginPublicID       *string `json:"pluginPublicId,omitempty"`
	PluginContributionID *string `json:"pluginContributionId,omitempty"`
	PluginData           *string `json:"pluginData,omitempty"`
}
