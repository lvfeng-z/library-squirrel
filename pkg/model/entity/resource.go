package entity

import (
	"database/sql"

	"github.com/library-squirrel/wails/pkg/model"
)

// Resource 资源
type Resource struct {
	*model.BaseEntity
	WorkID            int64          `gorm:"column:work_id;index:idx_resource_work_id" json:"workId"`
	TaskID            int64          `gorm:"column:task_id;index:idx_resource_task_id" json:"taskId"`
	State             int            `gorm:"column:state" json:"state"`
	FilePath          sql.NullString `gorm:"column:file_path" json:"filePath"`
	FileName          sql.NullString `gorm:"column:file_name" json:"fileName"`
	FilenameExtension sql.NullString `gorm:"column:filename_extension" json:"filenameExtension"`
	SuggestName       sql.NullString `gorm:"column:suggest_name" json:"suggestName"`
	ResourceSize      sql.NullInt64  `gorm:"column:resource_size" json:"resourceSize"`
	Workdir           sql.NullString `gorm:"column:workdir" json:"workdir"`
	ResourceComplete  int            `gorm:"column:resource_complete" json:"resourceComplete"`
}

func (Resource) TableName() string {
	return "resource"
}

func NewResource() *Resource {
	return &Resource{
		BaseEntity: &model.BaseEntity{},
	}
}
