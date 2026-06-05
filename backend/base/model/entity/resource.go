package entity

import (
	"database/sql"

	"github.com/library-squirrel/backend/base/model"
)

// Resource 资源
type Resource struct {
	*model.BaseEntity
	WorkID           int64          `gorm:"column:work_id;index:idx_resource_work_id" json:"workId"`
	TaskID           int64          `gorm:"column:task_id;index:idx_resource_task_id" json:"taskId"`
	Enabled          bool           `gorm:"column:enabled" json:"enabled"`
	SuggestName      sql.NullString `gorm:"column:suggest_name" json:"suggestName"`
	ResourceComplete int            `gorm:"column:resource_complete" json:"resourceComplete"`
	WorkStoreID      sql.NullInt64  `gorm:"column:work_store_id;index" json:"workStoreId"`     // 作品资源文件
	ThumbnailStoreID sql.NullInt64  `gorm:"column:thumbnail_store_id" json:"thumbnailStoreId"` // 封面/缩略图
}

func (Resource) TableName() string {
	return "resource"
}

func NewResource() *Resource {
	return &Resource{
		BaseEntity: &model.BaseEntity{},
	}
}
