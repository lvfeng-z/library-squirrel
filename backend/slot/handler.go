package slot

import (
	"github.com/library-squirrel/backend/base/model"
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
func (h *Handler) GetAllSlots() *model.ApiResponse[[]*SlotResponse] {
	configs := h.svc.GetAllSlots()
	result := make([]*SlotResponse, len(configs))
	for i, cfg := range configs {
		result[i] = SlotConfigToResponse(cfg)
	}
	return model.Success(result)
}
