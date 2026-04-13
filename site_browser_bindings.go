package main

import (
	"github.com/library-squirrel/wails/internal/siteBrowser"
)

// ==================== SiteBrowser Wails Bindings ====================

// SiteBrowserQueryPage 查询站点浏览器（实际上SiteBrowser不需要分页，这里返回所有）
func (app *App) SiteBrowserQueryPage() ([]*siteBrowser.SiteBrowserDTO, error) {
	return app.SiteBrowserService.List(), nil
}

// SiteBrowserList 获取所有站点浏览器
func (app *App) SiteBrowserList() ([]*siteBrowser.SiteBrowserDTO, error) {
	return app.SiteBrowserService.List(), nil
}

// SiteBrowserGetById 根据ID获取站点浏览器
func (app *App) SiteBrowserGetById(pluginPublicId string, contributionId string) (*siteBrowser.SiteBrowserDTO, error) {
	return app.SiteBrowserService.GetByID(pluginPublicId, contributionId)
}

// SiteBrowserGetByPluginId 根据插件ID获取站点浏览器
func (app *App) SiteBrowserGetByPluginId(pluginId int64) ([]*siteBrowser.SiteBrowserDTO, error) {
	return app.SiteBrowserService.GetByPluginID(pluginId), nil
}

// SiteBrowserOpen 打开站点浏览器
func (app *App) SiteBrowserOpen(pluginPublicId string, contributionId string) error {
	return app.SiteBrowserService.Open(pluginPublicId, contributionId)
}
