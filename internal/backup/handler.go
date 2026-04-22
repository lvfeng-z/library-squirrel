package backup

import (
	"context"
	"database/sql"

	"github.com/library-squirrel/wails/pkg/model"
	domain "github.com/library-squirrel/wails/pkg/model/entity"
)

// Handler 备份 Handler
type Handler struct {
	svc *Service
}

// NewHandler 创建备份 Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Create 创建备份
func (h *Handler) Create(ctx context.Context, sourceType int, sourceId int64, fileName string, sourcePath string, workDir string) *model.ApiResponse[*BackupResultDTO] {
	result, err := h.svc.CreateBackup(ctx, sourceType, sourceId, fileName, sourcePath, workDir)
	if err != nil {
		return model.Error[*BackupResultDTO](err.Error())
	}
	return model.Success(ToBackupResultDTO(result))
}

// CreatePluginBackup 创建插件备份
func (h *Handler) CreatePluginBackup(ctx context.Context, sourceId int64, fileName string, sourcePath string, workDir string) *model.ApiResponse[*BackupResultDTO] {
	result, err := h.svc.CreatePluginBackup(ctx, sourceId, fileName, sourcePath, workDir)
	if err != nil {
		return model.Error[*BackupResultDTO](err.Error())
	}
	return model.Success(ToBackupResultDTO(result))
}

// GetById 根据ID获取备份
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*BackupResultDTO] {
	result, err := h.svc.GetById(ctx, id)
	if err != nil {
		return model.Error[*BackupResultDTO](err.Error())
	}
	return model.Success(ToBackupResultDTO(result))
}

// GetPluginBackup 获取插件备份
func (h *Handler) GetPluginBackup(ctx context.Context, sourceId int64) *model.ApiResponse[*BackupResultDTO] {
	result, err := h.svc.GetPluginBackup(ctx, sourceId)
	if err != nil {
		return model.Error[*BackupResultDTO](err.Error())
	}
	return model.Success(ToBackupResultDTO(result))
}

// BackupResultDTO 备份返回结果DTO（用于屏蔽sql.Null*类型）
type BackupResultDTO struct {
	ID         int64   `json:"id"`
	SourceType *int64  `json:"sourceType"`
	SourceID   *int64  `json:"sourceId"`
	FileName   *string `json:"fileName"`
	FilePath   *string `json:"filePath"`
	Workdir    *string `json:"workdir"`
	CreateTime int64   `json:"createTime"`
	UpdateTime int64   `json:"updateTime"`
}

// ToBackupResultDTO 将 domain.Backup 转换为 BackupResultDTO
func ToBackupResultDTO(backup *domain.Backup) *BackupResultDTO {
	if backup == nil {
		return nil
	}
	return &BackupResultDTO{
		ID:         backup.GetID(),
		SourceType: nullInt64ToPointer(backup.SourceType),
		SourceID:   nullInt64ToPointer(backup.SourceID),
		FileName:   nullStringToPointer(backup.FileName),
		FilePath:   nullStringToPointer(backup.FilePath),
		Workdir:    nullStringToPointer(backup.Workdir),
		CreateTime: backup.GetCreateTime(),
		UpdateTime: backup.GetUpdateTime(),
	}
}

// nullStringToPointer 将 sql.NullString 转换为 *string
func nullStringToPointer(ns sql.NullString) *string {
	if ns.Valid {
		return &ns.String
	}
	return nil
}

// nullInt64ToPointer 将 sql.NullInt64 转换为 *int64
func nullInt64ToPointer(ns sql.NullInt64) *int64 {
	if ns.Valid {
		return &ns.Int64
	}
	return nil
}
