package extension

import (
	"encoding/json"

	domain "github.com/library-squirrel/backend/base"
	"github.com/library-squirrel/backend/base/model"
)

// FrontendExtensionResponse 前端扩展 IPC 响应 DTO（Type 字段值=Kind 字符串，前端按 slot.type 分流消费，保留字段名避免前端无谓改动）
type FrontendExtensionResponse struct {
	ID             string                      `json:"frontendExtensionId"`
	PluginID       int64                       `json:"pluginId"`
	PluginPublicID string                      `json:"pluginPublicId"`
	Name           string                      `json:"name"`
	Description    string                      `json:"description,omitempty"`
	Type           string                      `json:"type"`
	ContentType    string                      `json:"contentType"`
	Content        json.RawMessage             `json:"content,omitempty"`
	Position       string                      `json:"position,omitempty"`
	Target         string                      `json:"target,omitempty"`
	Order          int                         `json:"order,omitempty"`
	Title          string                      `json:"title,omitempty"`
	Icon           string                      `json:"icon,omitempty"`
	ViewId         string                      `json:"viewId,omitempty"`
	ExtensionId    string                      `json:"extensionId,omitempty"`
	ResourceType   string                      `json:"resourceType,omitempty"`
	Props          json.RawMessage             `json:"props,omitempty"`
	Children       []FrontendExtensionResponse `json:"children,omitempty"`
}

// FrontendExtensionHandler 前端扩展 Handler（IPC 入口）
type FrontendExtensionHandler struct {
	registry *FrontendExtensionRegistry
}

// NewFrontendExtensionHandler 创建前端扩展 Handler
func NewFrontendExtensionHandler(registry *FrontendExtensionRegistry) *FrontendExtensionHandler {
	return &FrontendExtensionHandler{registry: registry}
}

// GetAllFrontendExtensions 获取所有前端扩展
func (h *FrontendExtensionHandler) GetAllFrontendExtensions() *model.ApiResponse[[]*FrontendExtensionResponse] {
	configs := h.registry.GetFrontendExtensionConfigs()
	result := make([]*FrontendExtensionResponse, len(configs))
	for i, cfg := range configs {
		result[i] = FrontendExtensionConfigToResponse(cfg)
	}
	return model.Success(result)
}

// FrontendExtensionConfigToResponse 将领域模型转换为 IPC 响应 DTO
func FrontendExtensionConfigToResponse(cfg *domain.FrontendExtensionConfig) *FrontendExtensionResponse {
	if cfg == nil {
		return nil
	}
	resp := &FrontendExtensionResponse{
		ID:             cfg.Metadata.ID,
		PluginID:       cfg.Metadata.PluginID,
		PluginPublicID: cfg.Metadata.PluginPublicID,
		Name:           cfg.Metadata.Name,
		Description:    cfg.Metadata.Description,
		Type:           string(cfg.Kind),
		ContentType:    string(cfg.ContentType),
		Content:        cfg.Content,
		Position:       cfg.Position,
		Target:         cfg.Target,
		Order:          cfg.Order,
		Title:          cfg.Title,
		Icon:           cfg.Icon,
		ViewId:         cfg.ViewId,
		ExtensionId:    cfg.ExtensionId,
		ResourceType:   cfg.ResourceType,
		Props:          cfg.Props,
	}
	if len(cfg.Children) > 0 {
		resp.Children = make([]FrontendExtensionResponse, len(cfg.Children))
		for i := range cfg.Children {
			childResp := FrontendExtensionConfigToResponse(&cfg.Children[i])
			if childResp != nil {
				resp.Children[i] = *childResp
			}
		}
	}
	return resp
}
