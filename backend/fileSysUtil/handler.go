package fileSysUtil

import "github.com/library-squirrel/backend/base/model"

// Handler 文件系统工具 Handler
type Handler struct {
	svc *Service
}

// NewHandler 创建文件系统工具 Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// OpenDialog 打开文件/目录选择对话框
func (h *Handler) OpenDialog(options *OpenDialogOptions) *model.ApiResponse[*OpenDialogResult] {
	return model.HandleResult(h.svc.OpenDialog(options))
}
