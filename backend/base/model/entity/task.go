package entity

import (
	"database/sql"

	"github.com/library-squirrel/backend/base/model"
)

// TaskTypePluginDownload 插件下载任务的 task_type 取值（作品任务领域行 work_task 的行集来源）
const TaskTypePluginDownload = "plugin-download"

// Task 任务核心控制行：承载生命周期与树形关系；插件下载领域字段在 work_task（1:1 共享主键）、
// 分享接收领域字段在 share_task，均不落本表
type Task struct {
	*model.BaseEntity                // 嵌入基础实体
	HasChild          sql.NullBool   `gorm:"column:has_child" json:"hasChild"`
	Pid               sql.NullInt64  `gorm:"column:pid" json:"pid"`
	TaskName          sql.NullString `gorm:"column:task_name" json:"taskName"`
	Status            int            `gorm:"column:status" json:"status"`
	ErrorMessage      sql.NullString `gorm:"column:error_message" json:"errorMessage"`
	TaskType          sql.NullString `gorm:"column:task_type" json:"taskType"` // 任务类型:plugin-download=插件下载;其余取值=内置类型(经 taskManager 注册的执行面策略执行)
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
