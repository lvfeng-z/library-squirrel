package dto

// SiteBrowserDTO 站点浏览器信息
type SiteBrowserDTO struct {
	ExtensionID    string `json:"extensionId"`
	PluginPublicID string `json:"pluginPublicId"`
	Name           string `json:"name"`
	PluginID       int64  `json:"pluginId"`
}

// GetID 获取完整ID
func (d *SiteBrowserDTO) GetID() string {
	return d.PluginPublicID + "-" + d.ExtensionID
}
