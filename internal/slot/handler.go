package slot

import (
	domain "github.com/library-squirrel/wails/pkg"
	"github.com/library-squirrel/wails/pkg/model"
)

// Handler 插槽 Handler
type Handler struct {
	svc *SlotSyncService
}

// NewHandler 创建插槽 Handler
func NewHandler(svc *SlotSyncService) *Handler {
	return &Handler{svc: svc}
}

// GetAllSlots 获取所有插槽
func (h *Handler) GetAllSlots() *model.ApiResponse[[]*domain.SlotConfig] {
	result := h.svc.GetAllSlots()
	return model.Success(result)
}
