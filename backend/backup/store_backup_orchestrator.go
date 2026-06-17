package backup

import (
	"context"
	"database/sql"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/entity"

	"go.uber.org/zap"
)

// StoreResourceProvider 资源查询接口（由 resource.Service 实现）
type StoreResourceProvider interface {
	// GetEnabledByWorkId 查询作品关联的启用资源
	GetEnabledByWorkId(ctx context.Context, workId int64) ([]*entity.Resource, error)
	// GetById 根据 ID 获取资源
	GetById(ctx context.Context, id int64) (*entity.Resource, error)
}

// StoreDeleter Store 删除接口（支持备份，由 persistentStore.Service 实现）
type StoreDeleter interface {
	Delete(ctx context.Context, id int64, backup bool) (int64, error)
}

// StoreImporter Store 导入接口（将外部文件导入到 store 目录并创建 DB 记录，由 persistentStore.Service 实现）
type StoreImporter interface {
	StoreFromExternal(ctx context.Context, srcAbsPath string, relPath string, fileName string) (int64, error)
}

// ResourceUpdater Resource 更新接口（由 resource.Service 实现）
type ResourceUpdater interface {
	Update(ctx context.Context, resource *entity.Resource) error
}

// BackupReader 备份查询接口（由 backup.Service 实现）
type BackupReader interface {
	GetById(ctx context.Context, id int64) (*entity.Backup, error)
	GetBackupPath(backup *entity.Backup) string
	Delete(ctx context.Context, id int64) error
}

// StoreType 标识 Resource 上不同类型的 Store 字段
// 扩展方式：在 Resource 新增 Store 字段时，在此追加常量
type StoreType int

const (
	// StoreTypeWork 作品主资源（WorkStoreID）
	StoreTypeWork StoreType = iota + 1
	// StoreTypeThumbnail 封面/缩略图（ThumbnailStoreID）
	StoreTypeThumbnail
)

// StoreBackupItem 单个 Store 的备份条目
type StoreBackupItem struct {
	// ResourceID 所属 Resource ID
	ResourceID int64
	// BackupID Backup 记录 ID（0 = 未备份，直接删除了）
	BackupID int64
	// StoreType Store 字段类型
	StoreType StoreType
}

// StoreBackupOrchestratorImpl 资源存储备份编排器
// 封装替换场景下作品 Resource 全部 PersistentStore 的一站式备份和还原
type StoreBackupOrchestratorImpl struct {
	resourceProvider StoreResourceProvider
	storeDeleter     StoreDeleter
	storeImporter    StoreImporter
	resourceUpdater  ResourceUpdater
	backupReader     BackupReader
}

// NewStoreBackupOrchestrator 创建资源存储备份编排器
func NewStoreBackupOrchestrator(
	resourceProvider StoreResourceProvider,
	storeDeleter StoreDeleter,
	storeImporter StoreImporter,
	resourceUpdater ResourceUpdater,
	backupReader BackupReader,
) *StoreBackupOrchestratorImpl {
	return &StoreBackupOrchestratorImpl{
		resourceProvider: resourceProvider,
		storeDeleter:     storeDeleter,
		storeImporter:    storeImporter,
		resourceUpdater:  resourceUpdater,
		backupReader:     backupReader,
	}
}

// BackupStores 备份作品 Resource 指定类型的 Store，返回备份清单
// 仅备份传入 types 命中的 Store 字段，用于板块隔离（如仅备份资源文件、不触及缩略图）
// Resource 记录不变（不禁用，保持 Enabled=true）
func (o *StoreBackupOrchestratorImpl) BackupStores(ctx context.Context, workId int64, types ...StoreType) []*StoreBackupItem {
	if len(types) == 0 {
		return nil
	}
	typeSet := make(map[StoreType]struct{}, len(types))
	for _, t := range types {
		typeSet[t] = struct{}{}
	}

	resources, err := o.resourceProvider.GetEnabledByWorkId(ctx, workId)
	if err != nil {
		logger.Log.Warnf("[StoreBackupOrchestrator] 查询作品 %d 启用资源失败（跳过备份）: %v", workId, err)
		return nil
	}

	var items []*StoreBackupItem
	for _, res := range resources {
		// WorkStoreID — 作品主资源
		if _, ok := typeSet[StoreTypeWork]; ok && res.WorkStoreID.Valid {
			backupId, err := o.storeDeleter.Delete(ctx, res.WorkStoreID.Int64, true)
			if err != nil {
				logger.Log.Warnf("[StoreBackupOrchestrator] 备份 WorkStore(id=%d) 失败: %v", res.WorkStoreID.Int64, err)
			}
			items = append(items, &StoreBackupItem{
				ResourceID: res.GetID(),
				BackupID:   backupId,
				StoreType:  StoreTypeWork,
			})
		}

		// ThumbnailStoreID — 缩略图
		if _, ok := typeSet[StoreTypeThumbnail]; ok && res.ThumbnailStoreID.Valid {
			backupId, err := o.storeDeleter.Delete(ctx, res.ThumbnailStoreID.Int64, true)
			if err != nil {
				logger.Log.Warnf("[StoreBackupOrchestrator] 备份 ThumbnailStore(id=%d) 失败: %v", res.ThumbnailStoreID.Int64, err)
			}
			items = append(items, &StoreBackupItem{
				ResourceID: res.GetID(),
				BackupID:   backupId,
				StoreType:  StoreTypeThumbnail,
			})
		}
	}

	logger.Log.Infof("[StoreBackupOrchestrator] 作品 %d 备份完成: %d 个 Store 条目", workId, len(items))
	return items
}

// RestoreAllStores 从备份清单还原所有 Store 并更新对应 Resource
// 仅还原 BackupID > 0 的条目；BackupID == 0 的条目跳过（对应 Resource 字段保持 null）
func (o *StoreBackupOrchestratorImpl) RestoreAllStores(ctx context.Context, items []*StoreBackupItem) {
	if len(items) == 0 {
		return
	}

	logger.Log.Infof("[StoreBackupOrchestrator] 开始还原 %d 个 Store 条目", len(items))
	restoredCount := 0

	for _, item := range items {
		// BackupID == 0 的条目无需还原（被直接删除的 Store）
		if item.BackupID <= 0 {
			continue
		}

		// 1. 获取 Backup 记录
		backupEntity, err := o.backupReader.GetById(ctx, item.BackupID)
		if err != nil || backupEntity == nil {
			logger.Log.Warnf("[StoreBackupOrchestrator] 查询备份记录 %d 失败，跳过还原", item.BackupID)
			continue
		}

		// 2. 从 Backup 记录获取原始路径信息
		originalFilePath := ""
		if backupEntity.OriginalFilePath.Valid {
			originalFilePath = backupEntity.OriginalFilePath.String
		}
		originalFileName := ""
		if backupEntity.OriginalFileName.Valid {
			originalFileName = backupEntity.OriginalFileName.String
		}

		if originalFilePath == "" || originalFileName == "" {
			logger.Log.Warnf("[StoreBackupOrchestrator] 备份记录 %d 缺少原始路径信息，跳过还原", item.BackupID)
			continue
		}

		// 3. 获取备份文件绝对路径
		backupAbsPath := o.backupReader.GetBackupPath(backupEntity)

		// TODO 此处依赖PersistentStore是否合理
		// 4. 通过 PersistentStore 将备份文件导入到 store 目录（文件移动 + DB 记录由 PersistentStore 全权负责）
		newStoreId, err := o.storeImporter.StoreFromExternal(ctx, backupAbsPath, originalFilePath, originalFileName)
		if err != nil {
			logger.Log.Warnf("[StoreBackupOrchestrator] 导入 Store 失败: %v", zap.Error(err))
			continue
		}

		// 5. 获取 Resource 并更新对应 Store 字段
		resource, err := o.resourceProvider.GetById(ctx, item.ResourceID)
		if err != nil || resource == nil {
			logger.Log.Warnf("[StoreBackupOrchestrator] 查询 Resource(id=%d) 失败，跳过更新", item.ResourceID)
			continue
		}

		switch item.StoreType {
		case StoreTypeWork:
			resource.WorkStoreID = sql.NullInt64{Int64: newStoreId, Valid: true}
		case StoreTypeThumbnail:
			resource.ThumbnailStoreID = sql.NullInt64{Int64: newStoreId, Valid: true}
		}

		if err := o.resourceUpdater.Update(ctx, resource); err != nil {
			logger.Log.Warnf("[StoreBackupOrchestrator] 更新 Resource(id=%d) 失败: %v", item.ResourceID, err)
			continue
		}

		// 6. 清理备份记录
		if err := o.backupReader.Delete(ctx, item.BackupID); err != nil {
			logger.Log.Warnf("[StoreBackupOrchestrator] 删除备份记录 %d 失败: %v", item.BackupID, err)
		}

		restoredCount++
	}

	logger.Log.Infof("[StoreBackupOrchestrator] 还原完成: %d/%d 个 Store 已还原", restoredCount, len(items))
}
