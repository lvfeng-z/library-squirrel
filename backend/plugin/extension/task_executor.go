package extension

import (
	"context"
	"io"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/dto"
	domain "github.com/library-squirrel/backend/base/model/entity"
	"go.uber.org/zap"

	"github.com/library-squirrel/backend/taskManager"
)

// TaskExecutorImpl 任务执行器实现
// 实现 taskManager.TaskExecutorInterface 接口
type TaskExecutorImpl struct {
	loader *Loader
}

// NewTaskExecutor 创建任务执行器
func NewTaskExecutor(loader *Loader) *TaskExecutorImpl {
	return &TaskExecutorImpl{loader: loader}
}

// CreateWorkInfo 创建作品信息
func (e *TaskExecutorImpl) CreateWorkInfo(ctx context.Context, task *domain.Task) (*dto.WorkResponse, error) {
	pluginPublicId := ""
	if task.PluginPublicID.Valid {
		pluginPublicId = task.PluginPublicID.String
	}
	contributionId := ""
	if task.PluginContributionID.Valid {
		contributionId = task.PluginContributionID.String
	}
	taskHandlerExt, err := e.getTaskHandler(pluginPublicId, contributionId)
	if err != nil {
		logger.Log.Error("获取TaskHandler失败", zap.String("pluginPublicId", pluginPublicId),
			zap.String("contributionId", contributionId), zap.Error(err))
		return nil, err
	}
	return taskHandlerExt.CreateWorkInfo(task)
}

// Start 开始任务
func (e *TaskExecutorImpl) Start(ctx context.Context, task *domain.Task, workId int64) (io.ReadCloser, *dto.WorkResponse, error) {
	// 从 taskHandlerRegistry 获取 TaskHandler
	pluginPublicId := ""
	if task.PluginPublicID.Valid {
		pluginPublicId = task.PluginPublicID.String
	}
	contributionId := ""
	if task.PluginContributionID.Valid {
		contributionId = task.PluginContributionID.String
	}
	taskHandlerExt, err := e.getTaskHandler(pluginPublicId, contributionId)
	if err != nil {
		logger.Log.Error("获取TaskHandler失败", zap.String("pluginPublicId", pluginPublicId),
			zap.String("contributionId", contributionId), zap.Error(err))
		return nil, nil, err
	}

	// 调用 TaskHandler.Start
	return taskHandlerExt.Start(task)
}

// Pause 暂停任务
func (e *TaskExecutorImpl) Pause(ctx context.Context, param *dto.TaskResParam) error {
	if param == nil || param.Task == nil {
		return nil
	}

	pluginPublicId := ""
	if param.Task.PluginPublicID.Valid {
		pluginPublicId = param.Task.PluginPublicID.String
	}
	contributionId := ""
	if param.Task.PluginContributionID.Valid {
		contributionId = param.Task.PluginContributionID.String
	}
	taskHandlerExt, err := e.getTaskHandler(pluginPublicId, contributionId)
	if err != nil {
		logger.Log.Error("获取TaskHandler失败", zap.String("pluginPublicId", pluginPublicId),
			zap.String("contributionId", contributionId), zap.Error(err))
		return err
	}

	return taskHandlerExt.Pause(param)
}

// Stop 停止任务
func (e *TaskExecutorImpl) Stop(ctx context.Context, param *dto.TaskResParam) error {
	if param == nil || param.Task == nil {
		return nil
	}

	pluginPublicId := ""
	if param.Task.PluginPublicID.Valid {
		pluginPublicId = param.Task.PluginPublicID.String
	}
	contributionId := ""
	if param.Task.PluginContributionID.Valid {
		contributionId = param.Task.PluginContributionID.String
	}
	taskHandlerExt, err := e.getTaskHandler(pluginPublicId, contributionId)
	if err != nil {
		logger.Log.Error("获取TaskHandler失败", zap.String("pluginPublicId", pluginPublicId),
			zap.String("contributionId", contributionId), zap.Error(err))
		return err
	}

	return taskHandlerExt.Stop(param)
}

// Resume 恢复任务
func (e *TaskExecutorImpl) Resume(ctx context.Context, param *dto.TaskResParam) (*dto.WorkResponse, error) {
	if param == nil || param.Task == nil {
		return nil, nil
	}

	pluginPublicId := ""
	if param.Task.PluginPublicID.Valid {
		pluginPublicId = param.Task.PluginPublicID.String
	}
	contributionId := ""
	if param.Task.PluginContributionID.Valid {
		contributionId = param.Task.PluginContributionID.String
	}
	taskHandlerExt, err := e.getTaskHandler(pluginPublicId, contributionId)
	if err != nil {
		logger.Log.Error("获取TaskHandler失败", zap.String("pluginPublicId", pluginPublicId),
			zap.String("contributionId", contributionId), zap.Error(err))
		return nil, err
	}

	return taskHandlerExt.Resume(param)
}

// getTaskHandler 从注册中心获取 TaskHandler
func (e *TaskExecutorImpl) getTaskHandler(pluginPublicId, contributionId string) (dto.TaskHandler, error) {
	return e.loader.GetTaskHandler(pluginPublicId, contributionId)
}

// Ensure TaskExecutorImpl implements taskManager.TaskExecutorInterface
var _ taskManager.TaskExecutorInterface = (*TaskExecutorImpl)(nil)
