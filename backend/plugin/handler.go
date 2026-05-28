package plugin

import (
	"context"

	"github.com/library-squirrel/backend/base/model"
	domain "github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
	sdkdto "github.com/lvfeng-z/library-squirrel-plugin-sdk/dto"
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
func (h *Handler) Save(ctx context.Context, plugin *sdkdto.PluginDTO) *model.ApiResponse[int64] {
	domainPlugin := domain.ToPluginEntity(plugin)

	if err := h.svc.Save(ctx, domainPlugin); err != nil {
		return model.HandleError[int64](err)
	}
	return model.Success(domainPlugin.GetID())
}

// Update 更新插件
func (h *Handler) Update(ctx context.Context, plugin *sdkdto.PluginDTO) *model.ApiResponse[any] {
	domainPlugin := domain.ToPluginEntity(plugin)

	if err := h.svc.Update(ctx, domainPlugin); err != nil {
		return model.HandleError[any](err)
	}
	return model.Success[any](nil)
}

// InstallFromPath 从插件包路径安装插件
func (h *Handler) InstallFromPath(ctx context.Context, packagePath string, installType int) *model.ApiResponse[*sdkdto.PluginDTO] {
	result, err := h.svc.InstallFromPath(ctx, packagePath, domain.InstallType(installType))
	if err != nil {
		return model.HandleError[*sdkdto.PluginDTO](err)
	}
	return model.Success(domain.NewPluginDTO(result))
}

// Reinstall 重新安装插件
func (h *Handler) Reinstall(ctx context.Context, pluginPublicId string, installType int) *model.ApiResponse[*sdkdto.PluginDTO] {
	result, err := h.svc.Reinstall(ctx, pluginPublicId, domain.InstallType(installType))
	if err != nil {
		return model.HandleError[*sdkdto.PluginDTO](err)
	}
	return model.Success(domain.NewPluginDTO(result))
}

// ReinstallFromPath 从指定路径重新安装插件
func (h *Handler) ReinstallFromPath(ctx context.Context, pluginPublicId string, packagePath string, installType int) *model.ApiResponse[*sdkdto.PluginDTO] {
	result, err := h.svc.ReinstallFromPath(ctx, pluginPublicId, packagePath, domain.InstallType(installType))
	if err != nil {
		return model.HandleError[*sdkdto.PluginDTO](err)
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

// ========== 查询操作 ==========

// GetById 根据ID获取
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*sdkdto.PluginDTO] {
	result, err := h.svc.GetById(ctx, id)
	if err != nil {
		return model.HandleError[*sdkdto.PluginDTO](err)
	}
	return model.Success(domain.NewPluginDTO(result))
}

// GetByPublicId 根据公开ID获取插件
func (h *Handler) GetByPublicId(ctx context.Context, publicId string) *model.ApiResponse[*sdkdto.PluginDTO] {
	result, err := h.svc.GetByPublicId(ctx, publicId)
	if err != nil {
		return model.HandleError[*sdkdto.PluginDTO](err)
	}
	return model.Success(domain.NewPluginDTO(result))
}

// Page 分页查询
func (h *Handler) Page(ctx context.Context, page *model.Page[sdkdto.PluginDTO], query PluginQueryDTO) *model.ApiResponse[*model.Page[sdkdto.PluginDTO]] {
	if page == nil {
		page = &model.Page[sdkdto.PluginDTO]{}
	}
	entityPage := &model.Page[entity.Plugin]{
		PageNumber: page.PageNumber,
		PageSize:   page.PageSize,
	}
	result, err := h.svc.Page(ctx, entityPage, query)
	if err != nil {
		return model.HandleError[*model.Page[sdkdto.PluginDTO]](err)
	}
	// 转换为 DTO
	data := make([]*sdkdto.PluginDTO, 0, len(result.Data))
	for _, plugin := range result.Data {
		data = append(data, domain.NewPluginDTO(plugin))
	}
	return model.Success(&model.Page[sdkdto.PluginDTO]{
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
