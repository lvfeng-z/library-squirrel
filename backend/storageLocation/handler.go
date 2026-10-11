package storageLocation

import (
	"context"

	"github.com/library-squirrel/backend/base/model"
)

// Handler 存储位置 Handler（暴露给前端：三目录绝对路径查询）
type Handler struct {
	svc *Service
}

// NewHandler 创建存储位置 Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// GetStorageLocations 返回三目录（程序目录/用户数据目录/资源库目录）绝对路径
func (h *Handler) GetStorageLocations(ctx context.Context) *model.ApiResponse[*StorageLocations] {
	result, err := h.svc.GetStorageLocations(ctx)
	return model.HandleResult(result, err)
}
