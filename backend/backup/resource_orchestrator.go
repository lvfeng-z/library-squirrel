package backup

import (
	"context"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/entity"
)

// ResourceProvider 资源操作接口（由调用方定义，resource.Service 实现）
type ResourceProvider interface {
	// GetEnabledByWorkId 查询作品关联的启用资源
	GetEnabledByWorkId(ctx context.Context, workId int64) ([]*entity.Resource, error)
	// GetById 根据 ID 获取资源
	GetById(ctx context.Context, id int64) (*entity.Resource, error)
	// Update 更新资源
	Update(ctx context.Context, resource *entity.Resource) error
}

// ResourceBackupOrchestrator 资源备份编排器
// 封装资源备份/禁用/还原的完整生命周期，独立于任务执行逻辑
type ResourceBackupOrchestrator struct {
	resourceProvider ResourceProvider
}

// NewResourceBackupOrchestrator 创建资源备份编排器
func NewResourceBackupOrchestrator(resourceProvider ResourceProvider) *ResourceBackupOrchestrator {
	return &ResourceBackupOrchestrator{
		resourceProvider: resourceProvider,
	}
}

// BackupAndDisable 禁用作品的启用资源
// PersistentStore 管理的资源仅禁用，文件由 PersistentStore 负责清理
// 返回已处理的资源 ID 列表（用于后续还原）
func (o *ResourceBackupOrchestrator) BackupAndDisable(ctx context.Context, workId int64) []int64 {
	resources, err := o.resourceProvider.GetEnabledByWorkId(ctx, workId)
	if err != nil {
		logger.Log.Warnf("[ResourceBackupOrchestrator] 查询作品 %d 启用资源失败（跳过禁用）: %v", workId, err)
		return nil
	}

	var ids []int64
	for _, res := range resources {
		res.Enabled = false
		if err := o.resourceProvider.Update(ctx, res); err != nil {
			logger.Log.Warnf("[ResourceBackupOrchestrator] 禁用资源 %d 失败: %v", res.GetID(), err)
			continue
		}
		ids = append(ids, res.GetID())
	}
	return ids
}

// Restore 重新启用已禁用的资源
func (o *ResourceBackupOrchestrator) Restore(ctx context.Context, resourceIds []int64) {
	if len(resourceIds) == 0 {
		return
	}

	logger.Log.Infof("[ResourceBackupOrchestrator] 开始还原 %d 个资源", len(resourceIds))

	var restoredCount int
	for _, resourceId := range resourceIds {
		res, err := o.resourceProvider.GetById(ctx, resourceId)
		if err != nil || res == nil {
			logger.Log.Warnf("[ResourceBackupOrchestrator] 资源 %d 查询失败，跳过还原", resourceId)
			continue
		}
		res.Enabled = true
		if err := o.resourceProvider.Update(ctx, res); err != nil {
			logger.Log.Warnf("[ResourceBackupOrchestrator] 重新启用资源 %d 失败: %v", resourceId, err)
			continue
		}
		restoredCount++
	}

	logger.Log.Infof("[ResourceBackupOrchestrator] 还原完成: %d/%d 个资源已还原", restoredCount, len(resourceIds))
}
