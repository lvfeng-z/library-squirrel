package slot

import (
	"encoding/json"

	domain "github.com/library-squirrel/backend/base"
)

// SlotResponse 插槽 IPC 响应 DTO，字段名与前端约定一致
type SlotResponse struct {
	SlotID         string          `json:"slotId"`
	PluginID       int64           `json:"pluginId"`
	PluginPublicID string          `json:"pluginPublicId"`
	Name           string          `json:"name"`
	Description    string          `json:"description,omitempty"`
	Type           string          `json:"type"`                        // embed|panel|view|menu|siteBrowserList
	ContentType    string          `json:"contentType"`                 // vueSource|precompiled|code|html
	Content        json.RawMessage `json:"content,omitempty"`
	Position       string          `json:"position,omitempty"`
	Width          *int            `json:"width,omitempty"`
	Height         *int            `json:"height,omitempty"`
	Order          int             `json:"order,omitempty"`
	Title          string          `json:"title,omitempty"`
	Icon           string          `json:"icon,omitempty"`
	ViewId         string          `json:"viewId,omitempty"`
	ContributionId string          `json:"contributionId,omitempty"`
	Props          json.RawMessage `json:"props,omitempty"`
	Children       []SlotResponse  `json:"children,omitempty"`
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
		Width:          cfg.Width,
		Height:         cfg.Height,
		Order:          cfg.Order,
		Title:          cfg.Title,
		Icon:           cfg.Icon,
		ViewId:         cfg.ViewId,
		ContributionId: cfg.ContributionId,
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
