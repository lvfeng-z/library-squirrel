package main

import (
	"context"

	"github.com/library-squirrel/wails/internal/task"
	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/model"
)

// ==================== Task Wails Bindings ====================

// TaskGetById 获取任务
func (app *App) TaskGetById(id int64) (*model.ApiResponse[*domain.Task], error) {
	task, err := app.TaskService.GetById(context.Background(), id)
	if err != nil {
		return model.Error[*domain.Task](err.Error()), nil
	}
	return model.Success(task), nil
}

// TaskQueryPage 分页查询任务
func (app *App) TaskQueryPage(query *task.TaskQueryDTO) (*model.ApiResponse[*model.Page[domain.Task]], error) {
	page, err := app.TaskService.Page(context.Background(), 1, 10, *query)
	if err != nil {
		return model.Error[*model.Page[domain.Task]](err.Error()), nil
	}
	return model.Success(page), nil
}

// TaskQueryParentPage 分页查询父任务
func (app *App) TaskQueryParentPage(query *task.TaskQueryDTO) (*model.ApiResponse[*model.Page[domain.Task]], error) {
	page, err := app.TaskService.QueryParentPageByDTO(context.Background(), 1, 10, *query)
	if err != nil {
		return model.Error[*model.Page[domain.Task]](err.Error()), nil
	}
	return model.Success(page), nil
}

// TaskSave 保存任务
func (app *App) TaskSave(task *domain.Task) error {
	return app.TaskService.Save(context.Background(), task)
}

// TaskUpdate 更新任务
func (app *App) TaskUpdate(task *domain.Task) error {
	return app.TaskService.Update(context.Background(), task)
}

// TaskDelete 删除任务
func (app *App) TaskDelete(id int64) error {
	return app.TaskService.Delete(context.Background(), id)
}

// TaskRefreshStatus 刷新任务状态
func (app *App) TaskRefreshStatus(taskId int64) (int64, error) {
	return app.TaskService.RefreshTaskStatus(context.Background(), taskId)
}

// TaskListTree 获取任务树列表
func (app *App) TaskListTree(taskIds []int64) (*model.ApiResponse[[]*domain.Task], error) {
	tasks, err := app.TaskService.ListTaskTree(context.Background(), taskIds)
	if err != nil {
		return model.Error[[]*domain.Task](err.Error()), nil
	}
	return model.Success(tasks), nil
}

// TaskListStatus 查询状态列表
func (app *App) TaskListStatus(ids []int64) (*model.ApiResponse[[]*task.TaskScheduleDTO], error) {
	schedules, err := app.TaskService.ListStatus(context.Background(), ids)
	if err != nil {
		return model.Error[[]*task.TaskScheduleDTO](err.Error()), nil
	}
	return model.Success(schedules), nil
}

// TaskListSchedule 查询任务进度列表
func (app *App) TaskListSchedule(ids []int64) (*model.ApiResponse[[]*task.TaskScheduleDTO], error) {
	schedules, err := app.TaskService.ListSchedule(context.Background(), ids)
	if err != nil {
		return model.Error[[]*task.TaskScheduleDTO](err.Error()), nil
	}
	return model.Success(schedules), nil
}

// TaskCreate 创建任务
func (app *App) TaskCreate(req *task.CreateTaskRequest) (*model.ApiResponse[*domain.Task], error) {
	task, err := app.TaskService.CreateTask(context.Background(), req)
	if err != nil {
		return model.Error[*domain.Task](err.Error()), nil
	}
	return model.Success(task), nil
}

// TaskCreateByURL 根据URL创建任务
func (app *App) TaskCreateByURL(url string) (*model.ApiResponse[*task.CreateTaskByURLResponse], error) {
	resp, err := app.TaskService.CreateTaskByURL(context.Background(), url)
	if err != nil {
		return model.Error[*task.CreateTaskByURLResponse](err.Error()), nil
	}
	return model.Success(resp), nil
}

// TaskQueryChildrenTaskPage 查询子任务分页
func (app *App) TaskQueryChildrenTaskPage(pid int64, query *task.TaskQueryDTO) (*model.ApiResponse[*model.Page[domain.Task]], error) {
	page, err := app.TaskService.QueryChildrenTaskPageByDTO(context.Background(), pid, 1, 10, *query)
	if err != nil {
		return model.Error[*model.Page[domain.Task]](err.Error()), nil
	}
	return model.Success(page), nil
}
