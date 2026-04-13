package main

import (
	"github.com/library-squirrel/wails/internal/appLauncher"
)

// ==================== AppLauncher Wails Bindings ====================

// AppLauncherOpenImage 使用系统默认应用打开图片
func (app *App) AppLauncherOpenImage(url string) error {
	return app.AppLauncherService.OpenImage(url)
}

// AppLauncherOpen 使用指定应用打开文件
func (app *App) AppLauncherOpen(appEnum appLauncher.ExternalAppEnum, filePath string) error {
	return app.AppLauncherService.Open(appEnum, filePath)
}
