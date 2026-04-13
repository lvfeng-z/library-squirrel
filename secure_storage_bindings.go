package main

import (
	"context"
)

// ==================== SecureStorage Wails Bindings ====================

// SecureStorageSet 存储加密值
func (app *App) SecureStorageSet(storageKey string, plainValue string, description string) (int64, error) {
	return app.SecureStorageService.Set(context.Background(), storageKey, plainValue, description)
}

// SecureStorageGetValue 获取解密后的值
func (app *App) SecureStorageGetValue(storageKey string) (string, error) {
	return app.SecureStorageService.GetValue(context.Background(), storageKey)
}

// SecureStorageRemove 删除存储键
func (app *App) SecureStorageRemove(storageKey string) (int64, error) {
	return app.SecureStorageService.Remove(context.Background(), storageKey)
}

// SecureStorageHasKey 检查存储键是否存在
func (app *App) SecureStorageHasKey(storageKey string) (bool, error) {
	return app.SecureStorageService.HasKey(context.Background(), storageKey)
}

// SecureStorageListKeys 获取所有存储键
func (app *App) SecureStorageListKeys() ([]string, error) {
	return app.SecureStorageService.ListKeys(context.Background())
}
