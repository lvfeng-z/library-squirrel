package fileSysUtil

import "github.com/library-squirrel/wails/pkg/model"

// Handler 文件系统工具 Handler
type Handler struct {
	svc *Service
}

// NewHandler 创建文件系统工具 Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// DirSelect 打开目录/文件选择对话框
func (h *Handler) DirSelect(openFile bool, isModal bool) *model.ApiResponse[*OpenDialogResult] {
	result, err := h.svc.DirSelect(openFile, isModal)
	if err != nil {
		return model.Error[*OpenDialogResult](err.Error())
	}
	return model.Success(result)
}

// DirSelectMultiple 打开多选目录/文件选择对话框
func (h *Handler) DirSelectMultiple(openFile bool, isModal bool) *model.ApiResponse[*OpenDialogResult] {
	result, err := h.svc.DirSelectMultiple(openFile, isModal)
	if err != nil {
		return model.Error[*OpenDialogResult](err.Error())
	}
	return model.Success(result)
}
