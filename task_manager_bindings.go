package main

import (
	"context"
)

// ==================== TaskManager Wails Bindings ====================

// TaskManagerStartTree 启动任务树
func (app *App) TaskManagerStartTree(taskId int64) error {
	return app.TaskManagerService.StartTaskTree(context.Background(), taskId)
}

// TaskManagerPauseTree 暂停任务树
func (app *App) TaskManagerPauseTree(taskId int64) error {
	return app.TaskManagerService.PauseTaskTree(context.Background(), taskId)
}

// TaskManagerResumeTree 恢复任务树
func (app *App) TaskManagerResumeTree(taskId int64) error {
	return app.TaskManagerService.ResumeTaskTree(context.Background(), taskId)
}

// TaskManagerStopTree 停止任务树
func (app *App) TaskManagerStopTree(taskId int64) error {
	return app.TaskManagerService.StopTaskTree(context.Background(), taskId)
}

// TaskManagerRetryTree 重试任务树
func (app *App) TaskManagerRetryTree(taskId int64) error {
	return app.TaskManagerService.RetryTaskTree(context.Background(), taskId)
}
