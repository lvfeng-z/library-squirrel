package plugin

import (
	"context"

	"github.com/library-squirrel/backend/base/model"
)

// SettingHandler 插件设置 Handler
type SettingHandler struct {
	svc *PluginSettingService
}

// NewSettingHandler 创建插件设置 Handler
func NewSettingHandler(svc *PluginSettingService) *SettingHandler {
	return &SettingHandler{svc: svc}
}

// GetSettings 获取插件设置项（声明 + 当前值）
func (h *SettingHandler) GetSettings(ctx context.Context, pluginPublicId string) *model.ApiResponse[[]SettingItem] {
	items, err := h.svc.GetSettings(ctx, pluginPublicId)
	if err != nil {
		return model.HandleError[[]SettingItem](err)
	}
	return model.Success(items)
}

// SaveSetting 保存单个设置项
func (h *SettingHandler) SaveSetting(ctx context.Context, pluginPublicId string, key string, value string) *model.ApiResponse[any] {
	if err := h.svc.SaveSetting(ctx, pluginPublicId, key, value); err != nil {
		return model.HandleError[any](err)
	}
	return model.Success[any](nil)
}

// ResetSetting 重置设置项为默认值
func (h *SettingHandler) ResetSetting(ctx context.Context, pluginPublicId string, key string) *model.ApiResponse[any] {
	if err := h.svc.ResetSetting(ctx, pluginPublicId, key); err != nil {
		return model.HandleError[any](err)
	}
	return model.Success[any](nil)
}
