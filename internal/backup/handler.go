package backup

import (
	"context"

	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/model"
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
func (h *Handler) Create(ctx context.Context, sourceType int, sourceId int64, fileName string, sourcePath string, workDir string) *model.ApiResponse[*domain.Backup] {
	result, err := h.svc.CreateBackup(ctx, sourceType, sourceId, fileName, sourcePath, workDir)
	if err != nil {
		return model.Error[*domain.Backup](err.Error())
	}
	return model.Success(result)
}

// CreatePluginBackup 创建插件备份
func (h *Handler) CreatePluginBackup(ctx context.Context, sourceId int64, fileName string, sourcePath string, workDir string) *model.ApiResponse[*domain.Backup] {
	result, err := h.svc.CreatePluginBackup(ctx, sourceId, fileName, sourcePath, workDir)
	if err != nil {
		return model.Error[*domain.Backup](err.Error())
	}
	return model.Success(result)
}

// GetById 根据ID获取备份
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*domain.Backup] {
	result, err := h.svc.GetById(ctx, id)
	if err != nil {
		return model.Error[*domain.Backup](err.Error())
	}
	return model.Success(result)
}

// GetPluginBackup 获取插件备份
func (h *Handler) GetPluginBackup(ctx context.Context, sourceId int64) *model.ApiResponse[*domain.Backup] {
	result, err := h.svc.GetPluginBackup(ctx, sourceId)
	if err != nil {
		return model.Error[*domain.Backup](err.Error())
	}
	return model.Success(result)
}
