package settings

import (
	"github.com/library-squirrel/backend/base/model"
)

// Handler 设置 Handler
type Handler struct {
	svc *Service
}

// NewHandler 创建设置 Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Get 获取设置
func (h *Handler) Get() *model.ApiResponse[*Settings] {
	result := h.svc.GetSettings()
	return model.Success(result)
}

// Save 保存设置
func (h *Handler) Save(changes []SettingChange) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.SaveSettings(changes))
}

// Reset 重置设置
func (h *Handler) Reset() *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.ResetSettings())
}
