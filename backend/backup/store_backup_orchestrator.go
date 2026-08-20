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
}

// StoreResourceStoreReader resource_store 查询接口(按 resourceId 查关联 store)
type StoreResourceStoreReader interface {
	ListByResourceId(ctx context.Context, resourceId int64) ([]*entity.ResourceStore, error)
}

// StoreDeleter Store 删除接口（支持备份，由 persistentStore.Service 实现）
type StoreDeleter interface {
	// HardDelete 删除记录及对应文件（物理删记录）
	HardDelete(ctx context.Context, id int64, backup bool) (int64, error)
	// GetById 按 ID 查记录行（备份前取 file_path/file_name 快照供还原）
	GetById(ctx context.Context, id int64) (*entity.PersistentStore, error)
}

// StoreImporter Store 导入接口（将外部文件导入到 store 目录并创建 DB 记录，由 persistentStore.Service 实现）
type StoreImporter interface {
	StoreFromExternal(ctx context.Context, srcAbsPath string, relPath string, fileName string) (int64, error)
}

// BackupReader 备份查询接口（由 backup.Service 实现）
type BackupReader interface {
	GetById(ctx context.Context, id int64) (*entity.Backup, error)
	GetBackupPath(backup *entity.Backup) string
	DeleteBackup(ctx context.Context, id int64) error
}

// StoreBackupItem 单个 Store 的备份条目（备份方内存快照：还原所需的行信息由发起方自行记录，
// 不落 backup 表——backup 为纯保管清单，不记来源）
type StoreBackupItem struct {
	// ResourceID 所属 Resource ID
	ResourceID int64
	// BackupID Backup 保管清单行 ID（0 = 未备份，直接删除了）
	BackupID int64
	// FilePath 备份时行内 file_path 快照（workDir 相对路径，还原目标位置）
	FilePath string
	// FileName 备份时行内 file_name 快照（还原导入的原始文件名）
	FileName string
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
			// 删行前快照 file_path/file_name（物理删行后行内信息无处可查，还原目标由本内存清单承载）
			filePath, fileName := "", ""
			if record, err := o.storeDeleter.GetById(ctx, rs.StoreID); err == nil && record != nil {
				if record.FilePath.Valid {
					filePath = record.FilePath.String
				}
				if record.FileName.Valid {
					fileName = record.FileName.String
				}
			} else {
				logger.Log.Warnf("[StoreBackupOrchestrator] 查询 store 行(id=%d) 失败，快照为空", rs.StoreID)
			}
			backupId, err := o.storeDeleter.HardDelete(ctx, rs.StoreID, true)
			if err != nil {
				logger.Log.Warnf("[StoreBackupOrchestrator] 备份 Store(id=%d, type=%s) 失败: %v", rs.StoreID, rs.StoreType, err)
			}
			items = append(items, &StoreBackupItem{
				ResourceID: res.GetID(),
				BackupID:   backupId,
				FilePath:   filePath,
				FileName:   fileName,
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
		if item.FilePath == "" || item.FileName == "" {
			skipped++
			logger.Log.Warnf("[StoreBackupOrchestrator] 跳过还原: type=%s, BackupID=%d（备份时行快照缺失，无法定位还原目标）", item.StoreType, item.BackupID)
			continue
		}

		// 1. 获取 Backup 保管清单行
		backupEntity, err := o.backupReader.GetById(ctx, item.BackupID)
		if err != nil || backupEntity == nil {
			logger.Log.Warnf("[StoreBackupOrchestrator] 查询备份记录 %d 失败，跳过还原", item.BackupID)
			continue
		}

		// 2. 获取备份文件绝对路径
		backupAbsPath := o.backupReader.GetBackupPath(backupEntity)

		// 3. 通过 PersistentStore 将备份文件导入到 store 目录（还原目标取备份时的行快照）
		newStoreId, err := o.storeImporter.StoreFromExternal(ctx, backupAbsPath, item.FilePath, item.FileName)
		if err != nil {
			logger.Log.Warnf("[StoreBackupOrchestrator] 导入 Store 失败: %v", zap.Error(err))
			continue
		}
		item.NewStoreID = newStoreId

		// 4. 清理备份清单行与文件(resource_store 由调用方在还原后据 NewStoreID 重挂；backup 不感知)
		if err := o.backupReader.DeleteBackup(ctx, item.BackupID); err != nil {
			logger.Log.Warnf("[StoreBackupOrchestrator] 删除备份 %d 失败: %v", item.BackupID, err)
		}

		logger.Log.Infof("[StoreBackupOrchestrator] Store 已还原: type=%s, newStoreId=%d", item.StoreType, newStoreId)

		restored++
	}

	logger.Log.Infof("[StoreBackupOrchestrator] 还原完成: restored=%d, skipped=%d, total=%d", restored, skipped, len(items))
	return restored, skipped
}
