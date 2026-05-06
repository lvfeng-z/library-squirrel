package siteBrowser

import "github.com/library-squirrel/backend/base/model"

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

// QueryPage 分页查询站点浏览器
func (h *Handler) QueryPage(page, pageSize int) *model.ApiResponse[*PageResult] {
	result := h.svc.Page(page, pageSize)
	return model.Success(result)
}

// GetByID 根据ID获取站点浏览器
func (h *Handler) GetByID(pluginPublicId string, contributionId string) *model.ApiResponse[*SiteBrowserDTO] {
	return model.HandleResult(h.svc.GetByID(pluginPublicId, contributionId))
}

// GetByPluginID 根据插件ID获取站点浏览器
func (h *Handler) GetByPluginID(pluginId int64) *model.ApiResponse[[]*SiteBrowserDTO] {
	result := h.svc.GetByPluginID(pluginId)
	return model.Success(result)
}

// Open 打开站点浏览器
func (h *Handler) Open(pluginPublicId string, contributionId string) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.Open(pluginPublicId, contributionId))
}
