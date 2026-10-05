package pluginpreference

import (
	"context"

	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/dto"
)

// Handler 插件偏好管理面 Handler：记忆管理页「插件偏好」分区与插件设置区只读列表的
// 查询 + 按条目删除（只删不编辑；插件运行时面不经本 Handler，无删除通路）
type Handler struct {
	mgmt *ManagementService
}

// NewHandler 创建插件偏好管理面 Handler
func NewHandler(mgmt *ManagementService) *Handler {
	return &Handler{mgmt: mgmt}
}

// ListAllPreferences 全量偏好条目（含归属插件 id/公开 ID/显示名），记忆管理页按插件
// 分组消费
func (h *Handler) ListAllPreferences(ctx context.Context) *model.ApiResponse[[]*dto.PluginPreferenceEntryDTO] {
	entries, err := h.mgmt.ListAll(ctx)
	if err != nil {
		return model.HandleError[[]*dto.PluginPreferenceEntryDTO](err)
	}
	return model.Success(toEntryDTOs(entries))
}

// ListPreferencesByPlugin 按插件公开 ID 查询该插件偏好条目（插件设置区只读列表消费）
func (h *Handler) ListPreferencesByPlugin(ctx context.Context, pluginPublicId string) *model.ApiResponse[[]*dto.PluginPreferenceEntryDTO] {
	entries, err := h.mgmt.ListByPlugin(ctx, pluginPublicId)
	if err != nil {
		return model.HandleError[[]*dto.PluginPreferenceEntryDTO](err)
	}
	return model.Success(toEntryDTOs(entries))
}

// DeletePreference 按条目 id 删除一条偏好（管理面唯一删除入口；删除后插件读取即无
// 记录，下次问答重新发起）
func (h *Handler) DeletePreference(ctx context.Context, id int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.mgmt.Delete(ctx, id))
}

// toEntryDTOs 管理读模型 → 展示 DTO：值信封拆出标题/描述，标题空值回落偏好键（管理面
// 恒有可读文本）；值 JSON 解析失败按空信封降级，单行异常不阻塞整列表
func toEntryDTOs(entries []EntryWithPlugin) []*dto.PluginPreferenceEntryDTO {
	items := make([]*dto.PluginPreferenceEntryDTO, 0, len(entries))
	for _, e := range entries {
		var title, description string
		if v, err := parseValue(e.Value); err == nil && v != nil {
			title, description = v.Title, v.Description
		}
		if title == "" {
			title = e.PrefKey
		}
		items = append(items, &dto.PluginPreferenceEntryDTO{
			ID:             e.ID,
			PluginID:       e.PluginID,
			PluginPublicId: e.PluginPublicId,
			PluginName:     e.PluginName,
			PrefKey:        e.PrefKey,
			Title:          title,
			Description:    description,
			UpdateTime:     e.UpdateTime,
		})
	}
	return items
}
