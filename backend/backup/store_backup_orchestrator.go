package backup

import (
	"context"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/entity"

	"go.uber.org/zap"
)

// StoreResourceProvider 资源查询接口（由 resource.Service 实现）
type StoreResourceProvider interface {
	// ListByWorkId 查询作品关联的资源
	ListByWorkId(ctx context.Context, workId int64) ([]*entity.Resource, error)
	// GetById 根据 ID 获取资源
	GetById(ctx context.Context, id int64) (*entity.Resource, error)
}

// StoreResourceStoreReader resource_store 查询接口(按 resourceId 查关联 store)
type StoreResourceStoreReader interface {
	ListByResourceId(ctx context.Context, resourceId int64) ([]*entity.ResourceStore, error)
}

// StoreDeleter Store 删除接口（支持备份，由 persistentStore.Service 实现）
type StoreDeleter interface {
	// HardDelete 删除记录及对应文件（物理删记录）
	HardDelete(ctx context.Context, id int64, backup bool) (int64, error)
}

// StoreImporter Store 导入接口（将外部文件导入到 store 目录并创建 DB 记录，由 persistentStore.Service 实现）
type StoreImporter interface {
	StoreFromExternal(ctx context.Context, srcAbsPath string, relPath string, fileName string) (int64, error)
}

// ResourceUpdater Resource 更新接口（由 resource.Service 实现）
type ResourceUpdater interface {
	Updates(ctx context.Context, resource *entity.Resource) error
}

// BackupReader 备份查询接口（由 backup.Service 实现）
type BackupReader interface {
	GetById(ctx context.Context, id int64) (*entity.Backup, error)
	GetBackupPath(backup *entity.Backup) string
	Delete(ctx context.Context, id int64) error
}

// StoreBackupItem 单个 Store 的备份条目
type StoreBackupItem struct {
	// ResourceID 所属 Resource ID
	ResourceID int64
	// BackupID Backup 记录 ID（0 = 未备份，直接删除了）
	BackupID int64
	// StoreType store_type 字符串(main/thumbnail/videoTrack/...)
	StoreType string
	// Generation 生成方式(downloaded/derived)
	Generation string
	// NewStoreID 还原后新建的 PersistentStore ID（0=未还原/还原失败）
	// 由 RestoreAllStores 回填，供调用方据此重挂 resource_store；backup 自身不感知 resource_store
	NewStoreID int64
}

// StoreBackupOrchestratorImpl 资源存储备份编排器
// 封装替换场景下作品 Resource 全部 PersistentStore 的一站式备份和还原
type StoreBackupOrchestratorImpl struct {
	resourceProvider    StoreResourceProvider
	resourceStoreReader StoreResourceStoreReader
	storeDeleter        StoreDeleter
	storeImporter       StoreImporter
	backupReader        BackupReader
}

// NewStoreBackupOrchestrator 创建资源存储备份编排器
func NewStoreBackupOrchestrator(
	resourceProvider StoreResourceProvider,
	resourceStoreReader StoreResourceStoreReader,
	storeDeleter StoreDeleter,
	storeImporter StoreImporter,
	backupReader BackupReader,
) *StoreBackupOrchestratorImpl {
	return &StoreBackupOrchestratorImpl{
		resourceProvider:    resourceProvider,
		resourceStoreReader: resourceStoreReader,
		storeDeleter:        storeDeleter,
		storeImporter:       storeImporter,
		backupReader:        backupReader,
	}
}

// BackupStores 备份作品 Resource 指定 store_type 的 Store，返回备份清单
// storeTypes 为 store_type 字符串集合(main/thumbnail/...);空=备份全部
func (o *StoreBackupOrchestratorImpl) BackupStores(ctx context.Context, workId int64, storeTypes ...string) []*StoreBackupItem {
	typeSet := make(map[string]struct{}, len(storeTypes))
	for _, t := range storeTypes {
		typeSet[t] = struct{}{}
	}

	resources, err := o.resourceProvider.ListByWorkId(ctx, workId)
	if err != nil {
		logger.Log.Warnf("[StoreBackupOrchestrator] 查询作品 %d 资源失败（跳过备份）: %v", workId, err)
		return nil
	}

	var items []*StoreBackupItem
	for _, res := range resources {
		// 从 resource_store 查关联的 store 行(不再读旧列)
		storeRows, err := o.resourceStoreReader.ListByResourceId(ctx, res.GetID())
		if err != nil {
			logger.Log.Warnf("[StoreBackupOrchestrator] 查询 resource_store(resourceId=%d) 失败: %v", res.GetID(), err)
			continue
		}
		for _, rs := range storeRows {
			// 按 storeTypes 过滤(空=全部)
			if len(typeSet) > 0 {
				if _, ok := typeSet[rs.StoreType]; !ok {
					continue
				}
			}
			backupId, err := o.storeDeleter.HardDelete(ctx, rs.StoreID, true)
			if err != nil {
				logger.Log.Warnf("[StoreBackupOrchestrator] 备份 Store(id=%d, type=%s) 失败: %v", rs.StoreID, rs.StoreType, err)
			}
			items = append(items, &StoreBackupItem{
				ResourceID: res.GetID(),
				BackupID:   backupId,
				StoreType:  rs.StoreType,
				Generation: rs.Generation,
			})
		}
	}

	logger.Log.Infof("[StoreBackupOrchestrator] 作品 %d 备份完成: %d 个 Store 条目", workId, len(items))
	return items
}

// RestoreAllStores 从备份清单还原所有 Store
// 仅还原 BackupID > 0 的条目；BackupID<=0（备份未成功）计入 skipped 并告警
// 返回 restored（成功还原数）与 skipped（因备份缺失跳过的数），供调用方上报
func (o *StoreBackupOrchestratorImpl) RestoreAllStores(ctx context.Context, items []*StoreBackupItem) (restored, skipped int) {
	if len(items) == 0 {
		return 0, 0
	}

	logger.Log.Infof("[StoreBackupOrchestrator] 开始还原 %d 个 Store 条目", len(items))

	for _, item := range items {
		if item.BackupID <= 0 {
			skipped++
			logger.Log.Warnf("[StoreBackupOrchestrator] 跳过还原: type=%s, BackupID=%d（备份未成功，旧资源可能已丢失）", item.StoreType, item.BackupID)
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

		// 4. 通过 PersistentStore 将备份文件导入到 store 目录
		newStoreId, err := o.storeImporter.StoreFromExternal(ctx, backupAbsPath, originalFilePath, originalFileName)
		if err != nil {
			logger.Log.Warnf("[StoreBackupOrchestrator] 导入 Store 失败: %v", zap.Error(err))
			continue
		}
		item.NewStoreID = newStoreId

		// 5. 清理备份记录(resource_store 由调用方在还原后据 NewStoreID 重挂；backup 不感知)
		if err := o.backupReader.Delete(ctx, item.BackupID); err != nil {
			logger.Log.Warnf("[StoreBackupOrchestrator] 删除备份记录 %d 失败: %v", item.BackupID, err)
		}

		logger.Log.Infof("[StoreBackupOrchestrator] Store 已还原: type=%s, newStoreId=%d", item.StoreType, newStoreId)

		restored++
	}

	logger.Log.Infof("[StoreBackupOrchestrator] 还原完成: restored=%d, skipped=%d, total=%d", restored, skipped, len(items))
	return restored, skipped
}
