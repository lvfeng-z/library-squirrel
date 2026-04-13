package main

import (
	"github.com/library-squirrel/wails/internal/extension"
	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/logger"
)

// ==================== Slot Wails Bindings ====================

// SetEventEmitter 设置 Wails 事件发射器（在应用启动后调用）
func (app *App) SetEventEmitter(emitter extension.WailsEventEmitter) {
	app.eventEmitter = emitter
	app.SlotPusher = extension.NewWailsSlotPusher(emitter)
	logger.Log.Info("Wails event emitter set for slot pusher")
}

// GetAllSlots 获取所有插槽配置
func (app *App) GetAllSlots() []*domain.SlotConfig {
	return app.SlotRegistry.GetSlotConfigs()
}
