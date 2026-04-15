package plugin

import (
	"context"

	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/model"
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

// InstallFromPath 从插件包路径安装插件
func (h *Handler) InstallFromPath(ctx context.Context, packagePath string, installType int) *model.ApiResponse[*domain.Plugin] {
	result, err := h.svc.InstallFromPath(ctx, packagePath, domain.InstallType(installType))
	if err != nil {
		return model.Error[*domain.Plugin](err.Error())
	}
	return model.Success(result)
}

// Reinstall 重新安装插件
func (h *Handler) Reinstall(ctx context.Context, pluginPublicId string, installType int) *model.ApiResponse[*domain.Plugin] {
	result, err := h.svc.Reinstall(ctx, pluginPublicId, domain.InstallType(installType))
	if err != nil {
		return model.Error[*domain.Plugin](err.Error())
	}
	return model.Success(result)
}

// Uninstall 卸载插件
func (h *Handler) Uninstall(ctx context.Context, pluginPublicId string) *model.ApiResponse[any] {
	if err := h.svc.Uninstall(ctx, pluginPublicId); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// SetUninstalled 设置插件为已卸载状态
func (h *Handler) SetUninstalled(ctx context.Context, pluginId int64) *model.ApiResponse[any] {
	if err := h.svc.SetUninstalled(ctx, pluginId); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// ========== 查询操作 ==========

// GetById 根据ID获取
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*domain.Plugin] {
	result, err := h.svc.GetById(ctx, id)
	if err != nil {
		return model.Error[*domain.Plugin](err.Error())
	}
	return model.Success(result)
}

// GetByPublicId 根据公开ID获取插件
func (h *Handler) GetByPublicId(ctx context.Context, publicId string) *model.ApiResponse[*domain.Plugin] {
	result, err := h.svc.GetByPublicId(ctx, publicId)
	if err != nil {
		return model.Error[*domain.Plugin](err.Error())
	}
	return model.Success(result)
}

// Page 分页查询
func (h *Handler) Page(ctx context.Context, page, pageSize int, queryDTO *PluginQueryDTO) *model.ApiResponse[*model.Page[domain.Plugin]] {
	if queryDTO == nil {
		queryDTO = &PluginQueryDTO{}
	}
	result, err := h.svc.Page(ctx, page, pageSize, *queryDTO)
	if err != nil {
		return model.Error[*model.Page[domain.Plugin]](err.Error())
	}
	return model.Success(result)
}

// CheckInstalled 检查插件是否已安装
func (h *Handler) CheckInstalled(ctx context.Context, publicId string) *model.ApiResponse[bool] {
	result, err := h.svc.CheckInstalled(ctx, publicId)
	if err != nil {
		return model.Error[bool](err.Error())
	}
	return model.Success(result)
}

// GetPluginRoot 获取插件根目录
func (h *Handler) GetPluginRoot() *model.ApiResponse[string] {
	result := h.svc.GetPluginRoot()
	return model.Success(result)
}

// ReadVueFile 读取插件的 Vue 文件内容
func (h *Handler) ReadVueFile(pluginPublicId string, filePath string) *model.ApiResponse[string] {
	result, err := h.svc.ReadVueFile(pluginPublicId, filePath)
	if err != nil {
		return model.Error[string](err.Error())
	}
	return model.Success(result)
}
