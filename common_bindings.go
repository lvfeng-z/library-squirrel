package main

import (
	"github.com/library-squirrel/wails/backend/base/model"
	"github.com/library-squirrel/wails/backend/fileSysUtil"
)

// ==================== Common Wails Bindings ====================

// Greet is a simple test method for Wails bindings
func (app *App) Greet(name string) string {
	return "Hello, " + name + "! Welcome to Library Squirrel."
}

// GetVersion returns the application version
func (app *App) GetVersion() string {
	return "1.0.0-wails"
}

// DirSelect opens a directory/file selection dialog
// openFile: true=select file, false=select directory
func (app *App) DirSelect(openFile bool) (*model.ApiResponse[*fileSysUtil.OpenDialogResult], error) {
	result, err := app.FileSysUtilService.DirSelect(openFile, false)
	if err != nil {
		return model.Error[*fileSysUtil.OpenDialogResult](err.Error()), nil
	}
	return model.Success(result), nil
}

// OpenPath opens a file with the system's default application
func (app *App) OpenPath(path string) error {
	return app.AppLauncherService.OpenPath(path)
}

// OpenExternal opens a URL in the default browser
func (app *App) OpenExternal(url string) error {
	return app.AppLauncherService.OpenExternal(url)
}
