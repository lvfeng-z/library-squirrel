package settings

import (
	"github.com/library-squirrel/wails/pkg/model"
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
	if err := h.svc.SaveSettings(changes); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// Reset 重置设置
func (h *Handler) Reset() *model.ApiResponse[any] {
	if err := h.svc.ResetSettings(); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}
