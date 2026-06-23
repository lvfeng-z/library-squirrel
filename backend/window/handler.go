package window

import "github.com/library-squirrel/backend/base/model"

// Handler 窗口 Handler
type Handler struct {
	svc *Service
}

// NewHandler 创建窗口 Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// SetTitleBarColor 设置主窗口标题栏背景色与文字色
// bg/text 为 #RRGGBB 格式，仅 Windows 11 (22000+) 完整生效
func (h *Handler) SetTitleBarColor(bg, text string) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.SetTitleBarColor(bg, text))
}
