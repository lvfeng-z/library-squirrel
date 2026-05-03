package backup

import (
	"context"

	"github.com/library-squirrel/wails/pkg/model"
	dto2 "github.com/library-squirrel/wails/pkg/model/dto"
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
func (h *Handler) Create(ctx context.Context, sourceType int, sourceId int64, fileName string, sourcePath string, workDir string) *model.ApiResponse[*dto2.BackupDTO] {
	result, err := h.svc.CreateBackup(ctx, sourceType, sourceId, fileName, sourcePath, workDir)
	if err != nil {
		return model.HandleError[*dto2.BackupDTO](err)
	}
	return model.Success(dto2.NewBackupDTO(result))
}

// CreatePluginBackup 创建插件备份
func (h *Handler) CreatePluginBackup(ctx context.Context, sourceId int64, fileName string, sourcePath string, workDir string) *model.ApiResponse[*dto2.BackupDTO] {
	result, err := h.svc.CreatePluginBackup(ctx, sourceId, fileName, sourcePath, workDir)
	if err != nil {
		return model.HandleError[*dto2.BackupDTO](err)
	}
	return model.Success(dto2.NewBackupDTO(result))
}

// GetById 根据ID获取备份
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*dto2.BackupDTO] {
	result, err := h.svc.GetById(ctx, id)
	if err != nil {
		return model.HandleError[*dto2.BackupDTO](err)
	}
	return model.Success(dto2.NewBackupDTO(result))
}

// GetPluginBackup 获取插件备份
func (h *Handler) GetPluginBackup(ctx context.Context, sourceId int64) *model.ApiResponse[*dto2.BackupDTO] {
	result, err := h.svc.GetPluginBackup(ctx, sourceId)
	if err != nil {
		return model.HandleError[*dto2.BackupDTO](err)
	}
	return model.Success(dto2.NewBackupDTO(result))
}
