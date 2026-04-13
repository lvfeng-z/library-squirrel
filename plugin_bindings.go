package main

import (
	"context"

	"github.com/library-squirrel/wails/internal/plugin"
	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/model"
)

// ==================== Plugin Wails Bindings ====================

// PluginGetById 获取插件
func (app *App) PluginGetById(id int64) (*domain.Plugin, error) {
	return app.PluginService.GetById(context.Background(), id)
}

// PluginGetByPublicId 根据公开ID获取插件
func (app *App) PluginGetByPublicId(publicId string) (*domain.Plugin, error) {
	return app.PluginService.GetByPublicId(context.Background(), publicId)
}

// PluginQueryPage 分页查询插件
func (app *App) PluginQueryPage(query *plugin.PluginQueryDTO) (*model.Page[domain.Plugin], error) {
	return app.PluginService.Page(context.Background(), 1, 10, *query)
}

// PluginCheckInstalled 检查插件是否已安装
func (app *App) PluginCheckInstalled(publicId string) (bool, error) {
	return app.PluginService.CheckInstalled(context.Background(), publicId)
}

// PluginSave 保存插件
func (app *App) PluginSave(plugin *domain.Plugin) error {
	return app.PluginService.Save(context.Background(), plugin)
}

// PluginUpdate 更新插件
func (app *App) PluginUpdate(plugin *domain.Plugin) error {
	return app.PluginService.Update(context.Background(), plugin)
}

// PluginDelete 删除插件
func (app *App) PluginDelete(id int64) error {
	return app.PluginService.Delete(context.Background(), id)
}

// PluginInstallFromPath 从插件包路径安装插件
func (app *App) PluginInstallFromPath(packagePath string) (*domain.Plugin, error) {
	return app.PluginService.InstallFromPath(context.Background(), packagePath, domain.InstallTypeManual)
}

// PluginReinstall 重新安装插件
func (app *App) PluginReinstall(publicId string) (*domain.Plugin, error) {
	return app.PluginService.Reinstall(context.Background(), publicId, domain.InstallTypeManual)
}

// PluginReinstallFromPath 从指定路径重新安装插件
func (app *App) PluginReinstallFromPath(publicId string, packagePath string) (*domain.Plugin, error) {
	return app.PluginService.ReinstallFromPath(context.Background(), publicId, packagePath, domain.InstallTypeManual)
}

// PluginUninstall 卸载插件
func (app *App) PluginUninstall(publicId string) error {
	return app.PluginService.Uninstall(context.Background(), publicId)
}

// GetPluginVueFile 读取插件的 Vue 文件内容
func (app *App) GetPluginVueFile(pluginPublicId string, filePath string) (string, error) {
	return app.PluginService.ReadVueFile(pluginPublicId, filePath)
}
