package plugin

import (
	"context"

	"github.com/library-squirrel/wails/pkg/model"
	domain "github.com/library-squirrel/wails/pkg/model/dto"
	"github.com/library-squirrel/wails/pkg/model/entity"
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
func (h *Handler) Save(ctx context.Context, plugin *PluginParamDTO) *model.ApiResponse[int64] {
	domainPlugin := &entity.Plugin{}
	if plugin.PublicID != nil {
		domainPlugin.PublicID.Valid = true
		domainPlugin.PublicID.String = *plugin.PublicID
	}
	if plugin.Author != nil {
		domainPlugin.Author.Valid = true
		domainPlugin.Author.String = *plugin.Author
	}
	if plugin.Name != nil {
		domainPlugin.Name.Valid = true
		domainPlugin.Name.String = *plugin.Name
	}
	if plugin.Version != nil {
		domainPlugin.Version.Valid = true
		domainPlugin.Version.String = *plugin.Version
	}
	if plugin.EntryPath != nil {
		domainPlugin.EntryPath.Valid = true
		domainPlugin.EntryPath.String = *plugin.EntryPath
	}
	if plugin.RootPath != nil {
		domainPlugin.RootPath.Valid = true
		domainPlugin.RootPath.String = *plugin.RootPath
	}
	if plugin.ActivationType != nil {
		domainPlugin.ActivationType.Valid = true
		domainPlugin.ActivationType.String = *plugin.ActivationType
	}

	if err := h.svc.Save(ctx, domainPlugin); err != nil {
		return model.Error[int64](err.Error())
	}
	return model.Success(domainPlugin.GetID())
}

// Update 更新插件
func (h *Handler) Update(ctx context.Context, plugin *PluginParamDTO) *model.ApiResponse[any] {
	domainPlugin := &entity.Plugin{}
	domainPlugin.SetID(plugin.ID)
	if plugin.PublicID != nil {
		domainPlugin.PublicID.Valid = true
		domainPlugin.PublicID.String = *plugin.PublicID
	}
	if plugin.Author != nil {
		domainPlugin.Author.Valid = true
		domainPlugin.Author.String = *plugin.Author
	}
	if plugin.Name != nil {
		domainPlugin.Name.Valid = true
		domainPlugin.Name.String = *plugin.Name
	}
	if plugin.Version != nil {
		domainPlugin.Version.Valid = true
		domainPlugin.Version.String = *plugin.Version
	}
	if plugin.EntryPath != nil {
		domainPlugin.EntryPath.Valid = true
		domainPlugin.EntryPath.String = *plugin.EntryPath
	}
	if plugin.RootPath != nil {
		domainPlugin.RootPath.Valid = true
		domainPlugin.RootPath.String = *plugin.RootPath
	}
	if plugin.ActivationType != nil {
		domainPlugin.ActivationType.Valid = true
		domainPlugin.ActivationType.String = *plugin.ActivationType
	}

	if err := h.svc.Update(ctx, domainPlugin); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// InstallFromPath 从插件包路径安装插件
func (h *Handler) InstallFromPath(ctx context.Context, packagePath string, installType int) *model.ApiResponse[*domain.PluginDTO] {
	result, err := h.svc.InstallFromPath(ctx, packagePath, domain.InstallType(installType))
	if err != nil {
		return model.Error[*domain.PluginDTO](err.Error())
	}
	return model.Success(domain.NewPluginDTO(result))
}

// Reinstall 重新安装插件
func (h *Handler) Reinstall(ctx context.Context, pluginPublicId string, installType int) *model.ApiResponse[*domain.PluginDTO] {
	result, err := h.svc.Reinstall(ctx, pluginPublicId, domain.InstallType(installType))
	if err != nil {
		return model.Error[*domain.PluginDTO](err.Error())
	}
	return model.Success(domain.NewPluginDTO(result))
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
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*domain.PluginDTO] {
	result, err := h.svc.GetById(ctx, id)
	if err != nil {
		return model.Error[*domain.PluginDTO](err.Error())
	}
	return model.Success(domain.NewPluginDTO(result))
}

// GetByPublicId 根据公开ID获取插件
func (h *Handler) GetByPublicId(ctx context.Context, publicId string) *model.ApiResponse[*domain.PluginDTO] {
	result, err := h.svc.GetByPublicId(ctx, publicId)
	if err != nil {
		return model.Error[*domain.PluginDTO](err.Error())
	}
	return model.Success(domain.NewPluginDTO(result))
}

// Page 分页查询
func (h *Handler) Page(ctx context.Context, page *model.Page[domain.PluginDTO, PluginQueryDTO]) *model.ApiResponse[*model.Page[domain.PluginDTO, PluginQueryDTO]] {
	if page == nil {
		page = &model.Page[domain.PluginDTO, PluginQueryDTO]{}
	}
	result, err := h.svc.PageByDTO(ctx, page.PageNumber, page.PageSize, page.Query)
	if err != nil {
		return model.Error[*model.Page[domain.PluginDTO, PluginQueryDTO]](err.Error())
	}
	// 转换为 DTO
	data := make([]*domain.PluginDTO, 0, len(result.Data))
	for _, plugin := range result.Data {
		data = append(data, domain.NewPluginDTO(plugin))
	}
	return model.Success(&model.Page[domain.PluginDTO, PluginQueryDTO]{
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

// PluginParamDTO 插件数据传输对象（增删改参数）
type PluginParamDTO struct {
	ID             int64   `json:"id"`
	PublicID       *string `json:"publicId"`
	Author         *string `json:"author"`
	Name           *string `json:"name"`
	Version        *string `json:"version"`
	EntryPath      *string `json:"entryPath"`
	RootPath       *string `json:"rootPath"`
	ActivationType *string `json:"activationType"`
}
