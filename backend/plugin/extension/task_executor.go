package extension

import (
	"context"

	"github.com/library-squirrel/backend/base/logger"
	domain "github.com/library-squirrel/backend/base/model/entity"
	"go.uber.org/zap"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// TaskExecutorImpl 任务执行器实现
// 实现 download.PluginExecutor 接口（装配层的 pluginExecFactoryAdapter 返回处编译期校验）
type TaskExecutorImpl struct {
	registry *WorkFetchRegistry
}

// NewTaskExecutor 创建任务执行器
func NewTaskExecutor(registry *WorkFetchRegistry) *TaskExecutorImpl {
	return &TaskExecutorImpl{registry: registry}
}

// CreateWorkInfo 创建作品信息（插件身份在作品任务领域行）
func (e *TaskExecutorImpl) CreateWorkInfo(ctx context.Context, task *domain.Task, workTask *domain.WorkTask) (*sdkdto.WorkResponse, error) {
	pluginPublicId, extensionId := pluginIdsFromWorkTask(workTask)
	handler, err := e.getSDKWorkFetcher(pluginPublicId, extensionId)
	if err != nil {
		logger.Log.Error("获取作品拉取扩展失败", zap.String("pluginPublicId", pluginPublicId),
			zap.String("extensionId", extensionId), zap.Error(err))
		return nil, err
	}
	sdkTask := EntityTaskToSDK(task, workTask)
	// SDK WorkFetcher 接口的 unary 方法无 ctx 参数；具体代理类型承接调用方 ctx（取消即打断
	// gRPC 等待），非代理替身（测试）回落接口方法
	if proxy, ok := handler.(*WorkFetchProxy); ok {
		return proxy.CreateWorkInfoWithContext(ctx, sdkTask)
	}
	return handler.CreateWorkInfo(sdkTask)
}

// Start 开始任务,按 storeRoles 选择性返回 StoreSpec 流集合(含下载型 downloaded 与派生型 derived)与作品信息
func (e *TaskExecutorImpl) Start(ctx context.Context, task *domain.Task, workTask *domain.WorkTask, storeRoles []string) ([]*sdkdto.StoreSpec, *sdkdto.WorkResponse, error) {
	pluginPublicId, extensionId := pluginIdsFromWorkTask(workTask)
	handler, err := e.getSDKWorkFetcher(pluginPublicId, extensionId)
	if err != nil {
		logger.Log.Error("获取作品拉取扩展失败", zap.String("pluginPublicId", pluginPublicId),
			zap.String("extensionId", extensionId), zap.Error(err))
		return nil, nil, err
	}
	return handler.Start(ctx, EntityTaskToSDK(task, workTask), storeRoles)
}

// Pause 暂停任务
func (e *TaskExecutorImpl) Pause(ctx context.Context, param *sdkdto.TaskResParam) error {
	if param == nil || param.Task == nil {
		return nil
	}
	pluginPublicId, extensionId := pluginIdsFromSDKTask(param.Task)
	handler, err := e.getSDKWorkFetcher(pluginPublicId, extensionId)
	if err != nil {
		logger.Log.Error("获取作品拉取扩展失败", zap.String("pluginPublicId", pluginPublicId),
			zap.String("extensionId", extensionId), zap.Error(err))
		return err
	}
	if proxy, ok := handler.(*WorkFetchProxy); ok {
		return proxy.PauseWithContext(ctx, param)
	}
	return handler.Pause(param)
}

// Stop 停止任务
func (e *TaskExecutorImpl) Stop(ctx context.Context, param *sdkdto.TaskResParam) error {
	if param == nil || param.Task == nil {
		return nil
	}
	pluginPublicId, extensionId := pluginIdsFromSDKTask(param.Task)
	handler, err := e.getSDKWorkFetcher(pluginPublicId, extensionId)
	if err != nil {
		logger.Log.Error("获取作品拉取扩展失败", zap.String("pluginPublicId", pluginPublicId),
			zap.String("extensionId", extensionId), zap.Error(err))
		return err
	}
	if proxy, ok := handler.(*WorkFetchProxy); ok {
		return proxy.StopWithContext(ctx, param)
	}
	return handler.Stop(param)
}

// Resume 恢复任务:按 TaskResumeParam.StreamOffsets 续传,返回新的 StoreSpec 流集合
func (e *TaskExecutorImpl) Resume(ctx context.Context, param *sdkdto.TaskResumeParam) ([]*sdkdto.StoreSpec, *sdkdto.WorkResponse, error) {
	if param == nil || param.Task == nil {
		return nil, nil, nil
	}
	pluginPublicId, extensionId := pluginIdsFromSDKTask(param.Task)
	handler, err := e.getSDKWorkFetcher(pluginPublicId, extensionId)
	if err != nil {
		logger.Log.Error("获取作品拉取扩展失败", zap.String("pluginPublicId", pluginPublicId),
			zap.String("extensionId", extensionId), zap.Error(err))
		return nil, nil, err
	}
	return handler.Resume(ctx, param)
}

// getSDKWorkFetcher 从注册中心获取 WorkFetcher
func (e *TaskExecutorImpl) getSDKWorkFetcher(pluginPublicId, extensionId string) (sdkdto.WorkFetcher, error) {
	return e.registry.GetWorkFetcher(pluginPublicId, extensionId)
}

// pluginIdsFromWorkTask 从作品任务领域行提取插件ID（插件身份字段归属领域行）
func pluginIdsFromWorkTask(workTask *domain.WorkTask) (pluginPublicId, extensionId string) {
	if workTask == nil {
		return
	}
	if workTask.PluginPublicID.Valid {
		pluginPublicId = workTask.PluginPublicID.String
	}
	if workTask.PluginExtensionID.Valid {
		extensionId = workTask.PluginExtensionID.String
	}
	return
}

// pluginIdsFromSDKTask 从 sdkdto.TaskDTO 提取插件ID
func pluginIdsFromSDKTask(task *sdkdto.TaskDTO) (pluginPublicId, extensionId string) {
	if task.PluginPublicId != nil {
		pluginPublicId = *task.PluginPublicId
	}
	if task.PluginExtensionId != nil {
		extensionId = *task.PluginExtensionId
	}
	return
}
