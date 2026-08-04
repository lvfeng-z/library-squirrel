package dto

import (
	entity2 "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/util"
)

// BackupDTO 备份数据传输对象
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

// ToBackupEntity 将 BackupDTO 转换为 Backup 实体
func ToBackupEntity(dto *BackupDTO) *entity2.Backup {
	if dto == nil {
		return nil
	}

	entity := entity2.NewBackup()

	// 设置基础字段
	if dto.ID != 0 {
		entity.SetID(dto.ID)
	}

	// 设置业务字段
	if dto.SourceType != nil {
		entity.SourceType.Valid = true
		entity.SourceType.Int64 = *dto.SourceType
	} else {
		entity.SourceType.Valid = false
	}

	if dto.SourceID != nil {
		entity.SourceID.Valid = true
		entity.SourceID.Int64 = *dto.SourceID
	} else {
		entity.SourceID.Valid = false
	}

	if dto.FileName != nil {
		entity.FileName.Valid = true
		entity.FileName.String = *dto.FileName
	} else {
		entity.FileName.Valid = false
	}

	if dto.FilePath != nil {
		entity.FilePath.Valid = true
		entity.FilePath.String = *dto.FilePath
	} else {
		entity.FilePath.Valid = false
	}

	if dto.Workdir != nil {
		entity.Workdir.Valid = true
		entity.Workdir.String = *dto.Workdir
	} else {
		entity.Workdir.Valid = false
	}

	return entity
}
