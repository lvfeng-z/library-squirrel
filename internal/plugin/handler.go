package plugin

import (
	"context"
	"database/sql"

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
func (h *Handler) InstallFromPath(ctx context.Context, packagePath string, installType int) *model.ApiResponse[*PluginResultDTO] {
	result, err := h.svc.InstallFromPath(ctx, packagePath, domain.InstallType(installType))
	if err != nil {
		return model.Error[*PluginResultDTO](err.Error())
	}
	return model.Success(ToPluginResultDTO(result))
}

// Reinstall 重新安装插件
func (h *Handler) Reinstall(ctx context.Context, pluginPublicId string, installType int) *model.ApiResponse[*PluginResultDTO] {
	result, err := h.svc.Reinstall(ctx, pluginPublicId, domain.InstallType(installType))
	if err != nil {
		return model.Error[*PluginResultDTO](err.Error())
	}
	return model.Success(ToPluginResultDTO(result))
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
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*PluginResultDTO] {
	result, err := h.svc.GetById(ctx, id)
	if err != nil {
		return model.Error[*PluginResultDTO](err.Error())
	}
	return model.Success(ToPluginResultDTO(result))
}

// GetByPublicId 根据公开ID获取插件
func (h *Handler) GetByPublicId(ctx context.Context, publicId string) *model.ApiResponse[*PluginResultDTO] {
	result, err := h.svc.GetByPublicId(ctx, publicId)
	if err != nil {
		return model.Error[*PluginResultDTO](err.Error())
	}
	return model.Success(ToPluginResultDTO(result))
}

// Page 分页查询
func (h *Handler) Page(ctx context.Context, page, pageSize int, queryDTO *PluginQueryDTO) *model.ApiResponse[*model.Page[PluginResultDTO]] {
	if queryDTO == nil {
		queryDTO = &PluginQueryDTO{}
	}
	result, err := h.svc.Page(ctx, page, pageSize, *queryDTO)
	if err != nil {
		return model.Error[*model.Page[PluginResultDTO]](err.Error())
	}
	// 转换为 ResultDTO
	data := make([]*PluginResultDTO, 0, len(result.Data))
	for _, plugin := range result.Data {
		data = append(data, ToPluginResultDTO(plugin))
	}
	return model.Success(&model.Page[PluginResultDTO]{
		PageNumber:   result.PageNumber,
		PageSize:     result.PageSize,
		PageCount:    result.PageCount,
		DataCount:    result.DataCount,
		CurrentCount: result.CurrentCount,
		Query:        result.Query,
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

// PluginResultDTO 插件返回结果DTO（用于屏蔽sql.Null*类型）
type PluginResultDTO struct {
	ID             int64   `json:"id"`
	PublicID      *string `json:"publicId"`
	Author        *string `json:"author"`
	Name          *string `json:"name"`
	Version       *string `json:"version"`
	EntryPath     *string `json:"entryPath"`
	RootPath      *string `json:"rootPath"`
	BackupID      *int64  `json:"backupId"`
	SortNum       *int64  `json:"sortNum"`
	PluginData    *string `json:"pluginData"`
	Uninstalled   *int64  `json:"uninstalled"`
	ActivationType *string `json:"activationType"`
	CreateTime    int64   `json:"createTime"`
	UpdateTime    int64   `json:"updateTime"`
}

// ToPluginResultDTO 将 domain.Plugin 转换为 PluginResultDTO
func ToPluginResultDTO(plugin *domain.Plugin) *PluginResultDTO {
	if plugin == nil {
		return nil
	}
	return &PluginResultDTO{
		ID:             plugin.GetID(),
		PublicID:      nullStringToPointer(plugin.PublicID),
		Author:        nullStringToPointer(plugin.Author),
		Name:          nullStringToPointer(plugin.Name),
		Version:       nullStringToPointer(plugin.Version),
		EntryPath:     nullStringToPointer(plugin.EntryPath),
		RootPath:      nullStringToPointer(plugin.RootPath),
		BackupID:      nullInt64ToPointer(plugin.BackupID),
		SortNum:       nullInt64ToPointer(plugin.SortNum),
		PluginData:    nullStringToPointer(plugin.PluginData),
		Uninstalled:   nullInt64ToPointer(plugin.Uninstalled),
		ActivationType: nullStringToPointer(plugin.ActivationType),
		CreateTime:    plugin.GetCreateTime(),
		UpdateTime:    plugin.GetUpdateTime(),
	}
}

// nullStringToPointer 将 sql.NullString 转换为 *string
func nullStringToPointer(ns sql.NullString) *string {
	if ns.Valid {
		return &ns.String
	}
	return nil
}

// nullInt64ToPointer 将 sql.NullInt64 转换为 *int64
func nullInt64ToPointer(ns sql.NullInt64) *int64 {
	if ns.Valid {
		return &ns.Int64
	}
	return nil
}
