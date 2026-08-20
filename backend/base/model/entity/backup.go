package entity

import (
	"database/sql"

	"github.com/library-squirrel/backend/base/model"
)

// Backup 备份保管清单行（纯文件仓库：只记保管位置与时间，不记来源——
// 来源关联由发起方业务行内嵌记录，如 persistent_store.backup_id / plugin.BackupID）
type Backup struct {
	*model.BaseEntity                // 嵌入基础实体
	FileName          sql.NullString `gorm:"column:file_name" json:"fileName"`
	// FilePath 保管文件相对路径（workDir 基准，正斜杠），形如 backup/2026/06/08/文件.mp4
	FilePath sql.NullString `gorm:"column:file_path" json:"filePath"`
	Workdir  sql.NullString `gorm:"column:workdir" json:"workdir"`
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
