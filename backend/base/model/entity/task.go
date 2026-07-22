package entity

import (
	"database/sql"

	"github.com/library-squirrel/backend/base/model"
)

// Task 任务
type Task struct {
	*model.BaseEntity                // 嵌入基础实体
	HasChild          sql.NullBool   `gorm:"column:has_child" json:"hasChild"`
	Pid               sql.NullInt64  `gorm:"column:pid" json:"pid"`
	TaskName          sql.NullString `gorm:"column:task_name" json:"taskName"`
	SiteID            sql.NullInt64  `gorm:"column:site_id" json:"siteId"`
	SiteWorkID        sql.NullString `gorm:"column:site_work_id" json:"siteWorkId"`
	URL               sql.NullString `gorm:"column:url" json:"url"`
	Status            int            `gorm:"column:status" json:"status"`
	PendingResourceID sql.NullInt64  `gorm:"column:pending_resource_id" json:"pendingResourceId"`
	Continuable       sql.NullBool   `gorm:"column:continuable" json:"continuable"`
	PluginPublicID    sql.NullString `gorm:"column:plugin_public_id" json:"pluginPublicId"`
	PluginExtensionID sql.NullString `gorm:"column:plugin_extension_id" json:"pluginExtensionId"`
	PluginData        sql.NullString `gorm:"column:plugin_data" json:"pluginData"`
	ErrorMessage      sql.NullString `gorm:"column:error_message" json:"errorMessage"`
	StoreRoles        sql.NullString `gorm:"column:store_roles" json:"storeRoles"`            // 本次执行所选 store_type 集合(逗号分隔);空/全集表示全量
	InvolvedRoles     sql.NullString `gorm:"column:involved_roles" json:"involvedRoles"`      // 任务涉及的 store_type 集合(创建期声明,universe;逗号分隔);NULL=未确定/默认,执行期插件下全量
	ResourceType      sql.NullString `gorm:"column:resource_type" json:"resourceType"`        // 任务产生的 resource 的资源类型(创建期声明,预定义值);NULL=未声明
	IncludeWorkInfo   bool           `gorm:"column:include_work_info" json:"includeWorkInfo"` // 是否执行作品元数据板块
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
