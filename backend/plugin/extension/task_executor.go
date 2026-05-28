package extension

import (
	"context"
	"io"

	"github.com/library-squirrel/backend/base/logger"
	domain "github.com/library-squirrel/backend/base/model/entity"
	"go.uber.org/zap"

	"github.com/library-squirrel/backend/taskManager"
	sdkdto "github.com/lvfeng-z/library-squirrel-plugin-sdk/dto"
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
func (e *TaskExecutorImpl) CreateWorkInfo(ctx context.Context, task *domain.Task) (*sdkdto.WorkResponse, error) {
	pluginPublicId, contributionId := pluginIdsFromEntityTask(task)
	handler, err := e.getSDKTaskHandler(pluginPublicId, contributionId)
	if err != nil {
		logger.Log.Error("获取TaskHandler失败", zap.String("pluginPublicId", pluginPublicId),
			zap.String("contributionId", contributionId), zap.Error(err))
		return nil, err
	}
	return handler.CreateWorkInfo(EntityTaskToSDK(task))
}

// Start 开始任务
func (e *TaskExecutorImpl) Start(ctx context.Context, task *domain.Task, workId int64) (io.ReadCloser, *sdkdto.WorkResponse, error) {
	pluginPublicId, contributionId := pluginIdsFromEntityTask(task)
	handler, err := e.getSDKTaskHandler(pluginPublicId, contributionId)
	if err != nil {
		logger.Log.Error("获取TaskHandler失败", zap.String("pluginPublicId", pluginPublicId),
			zap.String("contributionId", contributionId), zap.Error(err))
		return nil, nil, err
	}
	return handler.Start(EntityTaskToSDK(task))
}

// Pause 暂停任务
func (e *TaskExecutorImpl) Pause(ctx context.Context, param *sdkdto.TaskResParam) error {
	if param == nil || param.Task == nil {
		return nil
	}
	pluginPublicId, contributionId := pluginIdsFromSDKTask(param.Task)
	handler, err := e.getSDKTaskHandler(pluginPublicId, contributionId)
	if err != nil {
		logger.Log.Error("获取TaskHandler失败", zap.String("pluginPublicId", pluginPublicId),
			zap.String("contributionId", contributionId), zap.Error(err))
		return err
	}
	return handler.Pause(param)
}

// Stop 停止任务
func (e *TaskExecutorImpl) Stop(ctx context.Context, param *sdkdto.TaskResParam) error {
	if param == nil || param.Task == nil {
		return nil
	}
	pluginPublicId, contributionId := pluginIdsFromSDKTask(param.Task)
	handler, err := e.getSDKTaskHandler(pluginPublicId, contributionId)
	if err != nil {
		logger.Log.Error("获取TaskHandler失败", zap.String("pluginPublicId", pluginPublicId),
			zap.String("contributionId", contributionId), zap.Error(err))
		return err
	}
	return handler.Stop(param)
}

// Resume 恢复任务
func (e *TaskExecutorImpl) Resume(ctx context.Context, param *sdkdto.TaskResParam) (*sdkdto.WorkResponse, error) {
	if param == nil || param.Task == nil {
		return nil, nil
	}
	pluginPublicId, contributionId := pluginIdsFromSDKTask(param.Task)
	handler, err := e.getSDKTaskHandler(pluginPublicId, contributionId)
	if err != nil {
		logger.Log.Error("获取TaskHandler失败", zap.String("pluginPublicId", pluginPublicId),
			zap.String("contributionId", contributionId), zap.Error(err))
		return nil, err
	}
	return handler.Resume(param)
}

// getSDKTaskHandler 从注册中心获取 SDK TaskHandler（直接使用 gRPC 代理）
func (e *TaskExecutorImpl) getSDKTaskHandler(pluginPublicId, contributionId string) (sdkdto.TaskHandler, error) {
	return e.loader.GetSDKTaskHandler(pluginPublicId, contributionId)
}

// pluginIdsFromEntityTask 从 entity.Task 提取插件ID
func pluginIdsFromEntityTask(task *domain.Task) (pluginPublicId, contributionId string) {
	if task.PluginPublicID.Valid {
		pluginPublicId = task.PluginPublicID.String
	}
	if task.PluginContributionID.Valid {
		contributionId = task.PluginContributionID.String
	}
	return
}

// pluginIdsFromSDKTask 从 sdkdto.TaskDTO 提取插件ID
func pluginIdsFromSDKTask(task *sdkdto.TaskDTO) (pluginPublicId, contributionId string) {
	if task.PluginPublicID != nil {
		pluginPublicId = *task.PluginPublicID
	}
	if task.PluginContributionID != nil {
		contributionId = *task.PluginContributionID
	}
	return
}

// Ensure TaskExecutorImpl implements taskManager.TaskExecutorInterface
var _ taskManager.TaskExecutorInterface = (*TaskExecutorImpl)(nil)
