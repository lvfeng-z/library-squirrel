package siteBrowser

import "github.com/library-squirrel/wails/pkg/model"

// Handler 站点浏览器 Handler
type Handler struct {
	svc *Service
}

// NewHandler 创建站点浏览器 Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// List 获取所有站点浏览器
func (h *Handler) List() *model.ApiResponse[[]*SiteBrowserDTO] {
	result := h.svc.List()
	return model.Success(result)
}

// GetByID 根据ID获取站点浏览器
func (h *Handler) GetByID(pluginPublicId string, contributionId string) *model.ApiResponse[*SiteBrowserDTO] {
	result, err := h.svc.GetByID(pluginPublicId, contributionId)
	if err != nil {
		return model.Error[*SiteBrowserDTO](err.Error())
	}
	return model.Success(result)
}

// GetByPluginID 根据插件ID获取站点浏览器
func (h *Handler) GetByPluginID(pluginId int64) *model.ApiResponse[[]*SiteBrowserDTO] {
	result := h.svc.GetByPluginID(pluginId)
	return model.Success(result)
}

// Open 打开站点浏览器
func (h *Handler) Open(pluginPublicId string, contributionId string) *model.ApiResponse[any] {
	if err := h.svc.Open(pluginPublicId, contributionId); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}
