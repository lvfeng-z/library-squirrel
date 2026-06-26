package extension

import (
	"encoding/json"

	domain "github.com/library-squirrel/backend/base"
	"github.com/library-squirrel/backend/base/model"
)

// SlotResponse 插槽 IPC 响应 DTO
type SlotResponse struct {
	SlotID         string          `json:"slotId"`
	PluginID       int64           `json:"pluginId"`
	PluginPublicID string          `json:"pluginPublicId"`
	Name           string          `json:"name"`
	Description    string          `json:"description,omitempty"`
	Type           string          `json:"type"`
	ContentType    string          `json:"contentType"`
	Content        json.RawMessage `json:"content,omitempty"`
	Position       string          `json:"position,omitempty"`
	Target         string          `json:"target,omitempty"`
	Order          int             `json:"order,omitempty"`
	Title          string          `json:"title,omitempty"`
	Icon           string          `json:"icon,omitempty"`
	ViewId         string          `json:"viewId,omitempty"`
	ExtensionId string          `json:"extensionId,omitempty"`
	Props          json.RawMessage `json:"props,omitempty"`
	Children       []SlotResponse  `json:"children,omitempty"`
}

// SlotHandler 插槽 Handler
type SlotHandler struct {
	registry *SlotRegistry
}

// NewSlotHandler 创建插槽 Handler
func NewSlotHandler(registry *SlotRegistry) *SlotHandler {
	return &SlotHandler{registry: registry}
}

// GetAllSlots 获取所有插槽
func (h *SlotHandler) GetAllSlots() *model.ApiResponse[[]*SlotResponse] {
	configs := h.registry.GetSlotConfigs()
	result := make([]*SlotResponse, len(configs))
	for i, cfg := range configs {
		result[i] = SlotConfigToResponse(cfg)
	}
	return model.Success(result)
}

// SlotConfigToResponse 将领域模型转换为 IPC 响应 DTO
func SlotConfigToResponse(cfg *domain.SlotConfig) *SlotResponse {
	if cfg == nil {
		return nil
	}
	resp := &SlotResponse{
		SlotID:         cfg.Metadata.ID,
		PluginID:       cfg.Metadata.PluginID,
		PluginPublicID: cfg.Metadata.PluginPublicID,
		Name:           cfg.Metadata.Name,
		Description:    cfg.Metadata.Description,
		Type:           string(cfg.SlotType),
		ContentType:    string(cfg.ContentType),
		Content:        cfg.Content,
		Position:       cfg.Position,
		Target:         cfg.Target,
		Order:          cfg.Order,
		Title:          cfg.Title,
		Icon:           cfg.Icon,
		ViewId:         cfg.ViewId,
		ExtensionId: cfg.ExtensionId,
		Props:          cfg.Props,
	}
	if len(cfg.Children) > 0 {
		resp.Children = make([]SlotResponse, len(cfg.Children))
		for i := range cfg.Children {
			childResp := SlotConfigToResponse(&cfg.Children[i])
			if childResp != nil {
				resp.Children[i] = *childResp
			}
		}
	}
	return resp
}
