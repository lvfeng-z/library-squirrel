package dto

import (
	"github.com/library-squirrel/wails/internal/util"
	entity2 "github.com/library-squirrel/wails/pkg/model/entity"
)

// BackupDTO 备份数据传输对象（无 sql.Null* 版本）
type BackupDTO struct {
	ID         int64   `json:"id"`
	SourceType *int64  `json:"sourceType"`
	SourceID   *int64  `json:"sourceId"`
	FileName   *string `json:"fileName"`
	FilePath   *string `json:"filePath"`
	Workdir    *string `json:"workdir"`
	CreateTime int64   `json:"createTime"`
	UpdateTime int64   `json:"updateTime"`
}

// NewBackupDTO 从 entity.Backup 创建 BackupDTO
func NewBackupDTO(backup *entity2.Backup) *BackupDTO {
	if backup == nil {
		return nil
	}
	return &BackupDTO{
		ID:         backup.GetID(),
		SourceType: util.NullInt64ToPointer(backup.SourceType),
		SourceID:   util.NullInt64ToPointer(backup.SourceID),
		FileName:   util.NullStringToPointer(backup.FileName),
		FilePath:   util.NullStringToPointer(backup.FilePath),
		Workdir:    util.NullStringToPointer(backup.Workdir),
		CreateTime: backup.GetCreateTime(),
		UpdateTime: backup.GetUpdateTime(),
	}
}
