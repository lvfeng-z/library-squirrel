package plugin

import (
	"context"

	"github.com/library-squirrel/backend/base/model"
	domain "github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
)

// Handler 插件 Handler
type Handler struct {
	svc *Service
}

// NewHandler 创建插件 Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// ========== 增删改操作 ==========

// Save 保存插件
func (h *Handler) Save(ctx context.Context, plugin *domain.PluginDTO) *model.ApiResponse[int64] {
	domainPlugin := domain.ToPluginEntity(plugin)

	if err := h.svc.Save(ctx, domainPlugin); err != nil {
		return model.HandleError[int64](err)
	}
	return model.Success(domainPlugin.GetID())
}

// Update 更新插件
func (h *Handler) Update(ctx context.Context, plugin *domain.PluginDTO) *model.ApiResponse[any] {
	domainPlugin := domain.ToPluginEntity(plugin)

	if err := h.svc.Update(ctx, domainPlugin); err != nil {
		return model.HandleError[any](err)
	}
	return model.Success[any](nil)
}

// InstallFromPath 从插件包路径安装插件。trusted 透传用户知情同意结果（true=用户已确认信任，false=绕过 UI 的异常安装）
func (h *Handler) InstallFromPath(ctx context.Context, packagePath string, trusted bool) *model.ApiResponse[*domain.PluginDTO] {
	result, err := h.svc.InstallFromPath(ctx, packagePath, trusted)
	if err != nil {
		return model.HandleError[*domain.PluginDTO](err)
	}
	return model.Success(domain.NewPluginDTO(result))
}

// Reinstall 重新安装插件。trusted 透传用户知情同意结果
func (h *Handler) Reinstall(ctx context.Context, pluginPublicId string, trusted bool) *model.ApiResponse[*domain.PluginDTO] {
	result, err := h.svc.Reinstall(ctx, pluginPublicId, trusted)
	if err != nil {
		return model.HandleError[*domain.PluginDTO](err)
	}
	return model.Success(domain.NewPluginDTO(result))
}

// ReinstallFromPath 从指定路径重新安装插件。trusted 透传用户知情同意结果
func (h *Handler) ReinstallFromPath(ctx context.Context, pluginPublicId string, packagePath string, trusted bool) *model.ApiResponse[*domain.PluginDTO] {
	result, err := h.svc.ReinstallFromPath(ctx, pluginPublicId, packagePath, trusted)
	if err != nil {
		return model.HandleError[*domain.PluginDTO](err)
	}
	return model.Success(domain.NewPluginDTO(result))
}

// Uninstall 卸载插件
func (h *Handler) Uninstall(ctx context.Context, pluginPublicId string) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.Uninstall(ctx, pluginPublicId))
}

// SetUninstalled 设置插件为已卸载状态
func (h *Handler) SetUninstalled(ctx context.Context, pluginId int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.SetUninstalled(ctx, pluginId))
}

// SetTrusted 设置插件信任状态（手动信任/取消信任）。trusted=true 时后端激活插件；
// trusted=false 即时停用运行时，force=前端确认对话框明示代价后强制停（跳过参与者否决检查）
func (h *Handler) SetTrusted(ctx context.Context, pluginPublicId string, trusted bool, force bool) *model.ApiResponse[*domain.PluginDTO] {
	result, err := h.svc.SetTrusted(ctx, pluginPublicId, trusted, force)
	if err != nil {
		return model.HandleError[*domain.PluginDTO](err)
	}
	return model.Success(domain.NewPluginDTO(result))
}

// ========== 查询操作 ==========

// GetById 根据ID获取
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*domain.PluginDTO] {
	result, err := h.svc.GetById(ctx, id)
	if err != nil {
		return model.HandleError[*domain.PluginDTO](err)
	}
	return model.Success(domain.NewPluginDTO(result))
}

// GetByPublicId 根据公开ID获取插件
func (h *Handler) GetByPublicId(ctx context.Context, publicId string) *model.ApiResponse[*domain.PluginDTO] {
	result, err := h.svc.GetByPublicId(ctx, publicId)
	if err != nil {
		return model.HandleError[*domain.PluginDTO](err)
	}
	return model.Success(domain.NewPluginDTO(result))
}

// Page 分页查询
func (h *Handler) Page(ctx context.Context, page *model.Page[domain.PluginDTO], query PluginQueryDTO) *model.ApiResponse[*model.Page[domain.PluginDTO]] {
	if page == nil {
		page = &model.Page[domain.PluginDTO]{}
	}
	entityPage := &model.Page[entity.Plugin]{
		PageNumber: page.PageNumber,
		PageSize:   page.PageSize,
	}
	result, err := h.svc.Page(ctx, entityPage, query)
	if err != nil {
		return model.HandleError[*model.Page[domain.PluginDTO]](err)
	}
	// 转换为 DTO
	data := make([]*domain.PluginDTO, 0, len(result.Data))
	for _, plugin := range result.Data {
		data = append(data, domain.NewPluginDTO(plugin))
	}
	return model.Success(&model.Page[domain.PluginDTO]{
		PageNumber:   result.PageNumber,
		PageSize:     result.PageSize,
		PageCount:    result.PageCount,
		DataCount:    result.DataCount,
		CurrentCount: result.CurrentCount,
		Data:         data,
	})
}

// CheckInstalled 检查插件是否已安装
func (h *Handler) CheckInstalled(ctx context.Context, publicId string) *model.ApiResponse[bool] {
	return model.HandleResult(h.svc.CheckInstalled(ctx, publicId))
}

// GetPluginRoot 获取插件根目录
func (h *Handler) GetPluginRoot() *model.ApiResponse[string] {
	result := h.svc.GetPluginRoot()
	return model.Success(result)
}

// GetPluginStatus 获取插件状态
func (h *Handler) GetPluginStatus(ctx context.Context, pluginPublicId string) *model.ApiResponse[*PluginStatusDTO] {
	result, err := h.svc.GetPluginStatus(ctx, pluginPublicId)
	if err != nil {
		return model.HandleError[*PluginStatusDTO](err)
	}
	return model.Success(result)
}
