package extension

import (
	"context"
	"io"

	"github.com/library-squirrel/backend/base/logger"
	domain "github.com/library-squirrel/backend/base/model/entity"
	"go.uber.org/zap"

	"github.com/library-squirrel/backend/taskManager"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// TaskExecutorImpl 任务执行器实现
// 实现 taskManager.TaskExecutorInterface 接口
type TaskExecutorImpl struct {
	registry *TaskHandlerRegistry
}

// NewTaskExecutor 创建任务执行器
func NewTaskExecutor(registry *TaskHandlerRegistry) *TaskExecutorImpl {
	return &TaskExecutorImpl{registry: registry}
}

// CreateWorkInfo 创建作品信息
func (e *TaskExecutorImpl) CreateWorkInfo(ctx context.Context, task *domain.Task) (*sdkdto.WorkResponse, error) {
	pluginPublicId, extensionId := pluginIdsFromEntityTask(task)
	handler, err := e.getSDKTaskHandler(pluginPublicId, extensionId)
	if err != nil {
		logger.Log.Error("获取TaskHandler失败", zap.String("pluginPublicId", pluginPublicId),
			zap.String("extensionId", extensionId), zap.Error(err))
		return nil, err
	}
	return handler.CreateWorkInfo(EntityTaskToSDK(task))
}

// Start 开始任务
func (e *TaskExecutorImpl) Start(ctx context.Context, task *domain.Task) (io.ReadCloser, *sdkdto.WorkResponse, error) {
	pluginPublicId, extensionId := pluginIdsFromEntityTask(task)
	handler, err := e.getSDKTaskHandler(pluginPublicId, extensionId)
	if err != nil {
		logger.Log.Error("获取TaskHandler失败", zap.String("pluginPublicId", pluginPublicId),
			zap.String("extensionId", extensionId), zap.Error(err))
		return nil, nil, err
	}
	return handler.Start(EntityTaskToSDK(task))
}

// Pause 暂停任务
func (e *TaskExecutorImpl) Pause(ctx context.Context, param *sdkdto.TaskResParam) error {
	if param == nil || param.Task == nil {
		return nil
	}
	pluginPublicId, extensionId := pluginIdsFromSDKTask(param.Task)
	handler, err := e.getSDKTaskHandler(pluginPublicId, extensionId)
	if err != nil {
		logger.Log.Error("获取TaskHandler失败", zap.String("pluginPublicId", pluginPublicId),
			zap.String("extensionId", extensionId), zap.Error(err))
		return err
	}
	return handler.Pause(param)
}

// Stop 停止任务
func (e *TaskExecutorImpl) Stop(ctx context.Context, param *sdkdto.TaskResParam) error {
	if param == nil || param.Task == nil {
		return nil
	}
	pluginPublicId, extensionId := pluginIdsFromSDKTask(param.Task)
	handler, err := e.getSDKTaskHandler(pluginPublicId, extensionId)
	if err != nil {
		logger.Log.Error("获取TaskHandler失败", zap.String("pluginPublicId", pluginPublicId),
			zap.String("extensionId", extensionId), zap.Error(err))
		return err
	}
	return handler.Stop(param)
}

// Resume 恢复任务
func (e *TaskExecutorImpl) Resume(ctx context.Context, param *sdkdto.TaskResParam) (io.ReadCloser, *sdkdto.WorkResponse, error) {
	if param == nil || param.Task == nil {
		return nil, nil, nil
	}
	pluginPublicId, extensionId := pluginIdsFromSDKTask(param.Task)
	handler, err := e.getSDKTaskHandler(pluginPublicId, extensionId)
	if err != nil {
		logger.Log.Error("获取TaskHandler失败", zap.String("pluginPublicId", pluginPublicId),
			zap.String("extensionId", extensionId), zap.Error(err))
		return nil, nil, err
	}
	return handler.Resume(param)
}

// GetThumbnail 获取缩略图
func (e *TaskExecutorImpl) GetThumbnail(ctx context.Context, task *domain.Task) (*sdkdto.ThumbnailResponse, error) {
	pluginPublicId, extensionId := pluginIdsFromEntityTask(task)
	handler, err := e.getSDKTaskHandler(pluginPublicId, extensionId)
	if err != nil {
		logger.Log.Warn("GetThumbnail 获取TaskHandler失败", zap.String("pluginPublicId", pluginPublicId),
			zap.String("extensionId", extensionId), zap.Error(err))
		return nil, err
	}
	taskData := ""
	if task.PluginData.Valid {
		taskData = task.PluginData.String
	}
	return handler.GetThumbnail(taskData)
}

// getSDKTaskHandler 从注册中心获取 TaskHandler
func (e *TaskExecutorImpl) getSDKTaskHandler(pluginPublicId, extensionId string) (sdkdto.TaskHandler, error) {
	return e.registry.GetTaskHandler(pluginPublicId, extensionId)
}

// pluginIdsFromEntityTask 从 entity.Task 提取插件ID
func pluginIdsFromEntityTask(task *domain.Task) (pluginPublicId, extensionId string) {
	if task.PluginPublicID.Valid {
		pluginPublicId = task.PluginPublicID.String
	}
	if task.PluginExtensionID.Valid {
		extensionId = task.PluginExtensionID.String
	}
	return
}

// pluginIdsFromSDKTask 从 sdkdto.TaskDTO 提取插件ID
func pluginIdsFromSDKTask(task *sdkdto.TaskDTO) (pluginPublicId, extensionId string) {
	if task.PluginPublicID != nil {
		pluginPublicId = *task.PluginPublicID
	}
	if task.PluginExtensionID != nil {
		extensionId = *task.PluginExtensionID
	}
	return
}

// Ensure TaskExecutorImpl implements taskManager.TaskExecutorInterface
var _ taskManager.TaskExecutorInterface = (*TaskExecutorImpl)(nil)
