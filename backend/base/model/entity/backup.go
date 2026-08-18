package entity

import (
	"database/sql"

	"github.com/library-squirrel/backend/base/model"
)

// Backup 备份
type Backup struct {
	*model.BaseEntity                // 嵌入基础实体
	SourceType        sql.NullInt64  `gorm:"column:source_type" json:"sourceType"`
	SourceID          sql.NullInt64  `gorm:"column:source_id" json:"sourceId"`
	FileName          sql.NullString `gorm:"column:file_name" json:"fileName"`
	FilePath          sql.NullString `gorm:"column:file_path" json:"filePath"`
	Workdir           sql.NullString `gorm:"column:workdir" json:"workdir"`
	// 软删除链写入的备份归属作品（复原/彻底删除按此聚合查询）；快照时代存量行与其他来源为 NULL
	WorkID sql.NullInt64 `gorm:"column:work_id;index" json:"workId"`
	// 资源备份时记录的原始路径信息，用于还原到原始位置
	OriginalFilePath          sql.NullString `gorm:"column:original_file_path" json:"originalFilePath"`
	OriginalFileName          sql.NullString `gorm:"column:original_file_name" json:"originalFileName"`
	OriginalFilenameExtension sql.NullString `gorm:"column:original_filename_extension" json:"originalFilenameExtension"`
}

// NewBackup 创建备份
func NewBackup() *Backup {
	return &Backup{
		BaseEntity: &model.BaseEntity{},
	}
}

// TableName 指定表名
func (Backup) TableName() string {
	return "backup"
}
