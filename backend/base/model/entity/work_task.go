package entity

import (
	"database/sql"
	"fmt"

	"github.com/library-squirrel/backend/base/model"
)

// WorkTask 作品任务领域行：插件下载任务的领域字段载体，与所属 task 核心行 1:1——
// 主键 ID 即所属 task.id（共享主键值，无独立任务外键列），行集为全部插件任务行（含树形父行与子行）
type WorkTask struct {
	*model.BaseEntity                // ID 恒 = 所属 task.id
	SiteID            sql.NullInt64  `gorm:"column:site_id" json:"siteId"`
	SiteWorkID        sql.NullString `gorm:"column:site_work_id" json:"siteWorkId"`
	URL               sql.NullString `gorm:"column:url" json:"url"`
	PendingResourceID sql.NullInt64  `gorm:"column:pending_resource_id" json:"pendingResourceId"`
	Continuable       sql.NullBool   `gorm:"column:continuable" json:"continuable"` // 跨进程契约字段，本库只读（无写入点）
	PluginPublicID    sql.NullString `gorm:"column:plugin_public_id" json:"pluginPublicId"`
	PluginExtensionID sql.NullString `gorm:"column:plugin_extension_id" json:"pluginExtensionId"`
	PluginData        sql.NullString `gorm:"column:plugin_data" json:"pluginData"`
	StoreRoles        sql.NullString `gorm:"column:store_roles" json:"storeRoles"`       // 本次执行所选 store_type 集合(逗号分隔);空/全集表示全量
	InvolvedRoles     sql.NullString `gorm:"column:involved_roles" json:"involvedRoles"` // 任务涉及的 store_type 集合(创建期声明,universe;逗号分隔);NULL=未确定/默认,执行期插件下全量
	ResourceType      sql.NullString `gorm:"column:resource_type" json:"resourceType"`   // 任务产生的 resource 的资源类型(创建期声明,预定义值);NULL=未声明
	// 是否执行作品元数据板块；GORM Updates 跳零值，置 false 须经显式列更新
	IncludeWorkInfo bool `gorm:"column:include_work_info" json:"includeWorkInfo"`
}

// NewWorkTask 创建作品任务领域行，taskID 为所属 task 行 id，构造即赋共享主键。
// 非正 id 直接 panic：零值主键插入会被 SQLite 静默按 rowid 分配新值，
// 破坏与 task 行的 1:1 同值约束且不报错，故在构造口 fail-fast
func NewWorkTask(taskID int64) *WorkTask {
	if taskID <= 0 {
		panic(fmt.Sprintf("NewWorkTask: 所属任务 ID 须为正数，实际 %d", taskID))
	}
	return &WorkTask{
		BaseEntity: &model.BaseEntity{ID: taskID},
	}
}

// TableName 指定表名
func (WorkTask) TableName() string {
	return "work_task"
}
