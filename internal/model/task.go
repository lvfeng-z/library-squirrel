package model

import (
	"database/sql"

	"github.com/library-squirrel/wails/pkg/model"
)

// Task 任务
type Task struct {
	*model.BaseEntity                   // 嵌入基础实体
	IsCollection         sql.NullInt64  `gorm:"column:is_collection" json:"isCollection"`
	Pid                  sql.NullInt64  `gorm:"column:pid" json:"pid"`
	TaskName             sql.NullString `gorm:"column:task_name" json:"taskName"`
	SiteID               sql.NullInt64  `gorm:"column:site_id" json:"siteId"`
	SiteWorkID           sql.NullString `gorm:"column:site_work_id" json:"siteWorkId"`
	URL                  sql.NullString `gorm:"column:url" json:"url"`
	Status               int            `gorm:"column:status" json:"status"`
	PendingResourceID    sql.NullInt64  `gorm:"column:pending_resource_id" json:"pendingResourceId"`
	Continuable          sql.NullInt64  `gorm:"column:continuable" json:"continuable"`
	PluginPublicID       sql.NullString `gorm:"column:plugin_public_id" json:"pluginPublicId"`
	PluginContributionID sql.NullString `gorm:"column:plugin_contribution_id" json:"pluginContributionId"`
	PluginData           sql.NullString `gorm:"column:plugin_data" json:"pluginData"`
	ErrorMessage         sql.NullString `gorm:"column:error_message" json:"errorMessage"`
}

// NewTask 创建任务
func NewTask() *Task {
	return &Task{
		BaseEntity: &model.BaseEntity{},
	}
}

// TableName 指定表名
func (Task) TableName() string {
	return "task"
}
