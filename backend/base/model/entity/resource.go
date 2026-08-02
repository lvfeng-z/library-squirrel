package entity

import (
	"database/sql"

	"github.com/library-squirrel/backend/base/model"
)

// Resource 资源
// store 关联统一由 resource_store 表表达(1 Resource 挂 N typed store,含 store_type/generation)
type Resource struct {
	*model.BaseEntity
	WorkID           int64          `gorm:"column:work_id;index:idx_resource_work_id" json:"workId"`
	TaskID           int64          `gorm:"column:task_id;index:idx_resource_task_id" json:"taskId"`
	SuggestName      sql.NullString `gorm:"column:suggest_name" json:"suggestName"`
	ResourceComplete sql.NullInt64  `gorm:"column:resource_complete" json:"resourceComplete"`
	ResourceType     string         `gorm:"column:resource_type;not null" json:"resourceType"`
}

func (Resource) TableName() string {
	return "resource"
}

func NewResource() *Resource {
	return &Resource{
		BaseEntity: &model.BaseEntity{},
	}
}
