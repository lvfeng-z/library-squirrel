package main

import (
	"github.com/library-squirrel/wails/internal/settings"
)

// ==================== Settings Wails Bindings ====================

// SettingsGetSettings 获取所有设置
func (app *App) SettingsGetSettings() *settings.Settings {
	return app.SettingsService.GetSettings()
}

// SettingsSaveSettings 保存设置变更
func (app *App) SettingsSaveSettings(changes []settings.SettingChange) error {
	return app.SettingsService.SaveSettings(changes)
}

// SettingsResetSettings 重置设置到默认值
func (app *App) SettingsResetSettings() error {
	return app.SettingsService.ResetSettings()
}
