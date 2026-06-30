package recycleBin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model"
	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	pkgerr "github.com/library-squirrel/backend/error"
	"github.com/library-squirrel/backend/util"

	"gorm.io/gorm"
)

// 错误定义
var (
	ErrRecycleItemNotFound = &pkgerr.BusinessError{Code: 404, Message: "回收站条目不存在"}
	ErrRestoreConflict     = &pkgerr.BusinessError{Code: 409, Message: "作品复原冲突：该作品已存在，请选择放弃或覆盖"}
)

// WorkRestorer 作品复原接口（由 work.Service 实现）
type WorkRestorer interface {
	// GetBySiteAndSiteWorkID 检查 (site_id, site_work_id) 是否已存在作品，返回已存在作品（nil 表示无冲突）
	GetBySiteAndSiteWorkID(ctx context.Context, siteId int64, siteWorkId string) (*domain.Work, error)
	// HardDeleteWork 物理删除作品（复原覆盖分支：删除占用业务键的新作品）
	HardDeleteWork(ctx context.Context, workId int64) error
	// RestoreWorkFromSnapshot 从快照重建作品及其关联、resource，返回新作品 ID
	// storeIdByBackupId: backup 记录 ID → 还原后的新 persistent_store ID 映射
	RestoreWorkFromSnapshot(ctx context.Context, snapshot *WorkRecycleSnapshot, storeIdByBackupId map[int64]int64) (int64, error)
}

// BackupReader 备份读取接口（由 backup.Service 实现）
type BackupReader interface {
	// GetById 根据 ID 获取备份记录
	GetById(ctx context.Context, id int64) (*domain.Backup, error)
	// GetBackupPath 获取备份文件的绝对路径
	GetBackupPath(backup *domain.Backup) string
	// Delete 删除备份记录
	Delete(ctx context.Context, id int64) error
}

// StoreImporter 资源导入接口（由 persistentStore.Service 实现）
type StoreImporter interface {
	// StoreFromExternal 将外部文件导入到 store 目录并创建 DB 记录（文件移动）
	StoreFromExternal(ctx context.Context, srcAbsPath string, relPath string, fileName string) (int64, error)
}

// Transactor 事务执行器
type Transactor interface {
	// ExecInTransaction 在事务中执行 fn，事务 DB 实例通过 ctx 传递
	ExecInTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

// RecycleBinSettingsProvider 回收站设置提供者接口（由 settings.Service 实现）
type RecycleBinSettingsProvider interface {
	// GetRecycleBinSettings 获取回收站自动清理设置（启用标志、保留天数）
	GetRecycleBinSettings() (enabled bool, retentionDays int)
}

// Service 回收站服务
type Service struct {
	repo           Repository
	workRestorer   WorkRestorer
	backupReader   BackupReader
	storeImporter  StoreImporter
	transactor     Transactor
	settingsReader RecycleBinSettingsProvider
	stopCh         chan struct{}
}

// NewService 创建回收站服务
func NewService(repo Repository, workRestorer WorkRestorer, backupReader BackupReader, storeImporter StoreImporter, transactor Transactor, settingsReader RecycleBinSettingsProvider) *Service {
	return &Service{
		repo:           repo,
		workRestorer:   workRestorer,
		backupReader:   backupReader,
		storeImporter:  storeImporter,
		transactor:     transactor,
		settingsReader: settingsReader,
		stopCh:         make(chan struct{}),
	}
}

// Save 保存回收站条目（供逻辑删除流程写入快照）
func (s *Service) Save(ctx context.Context, item *domain.RecycleItem) error {
	return s.repo.Save(ctx, item)
}

// GetById 根据 ID 获取回收站条目
func (s *Service) GetById(ctx context.Context, id int64) (*domain.RecycleItem, error) {
	return s.repo.GetById(ctx, id)
}

// Page 分页查询回收站列表
func (s *Service) Page(ctx context.Context, opt *database.PageOption) (*model.Page[domain.RecycleItem], error) {
	return s.repo.Page(ctx, opt)
}

// Delete 删除回收站条目
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// ListExpired 查询超过保留时长的回收站条目（供 TTL 自动清理）
func (s *Service) ListExpired(ctx context.Context, expireBefore int64) ([]*domain.RecycleItem, error) {
	return s.repo.ListExpired(ctx, expireBefore)
}

// RestoreWork 从回收站复原作品
// overwrite: 检测到 (site_id, site_work_id) 冲突时是否覆盖（先物理删除占用业务键的新作品）
// 冲突且 overwrite=false 时返回 ErrRestoreConflict
func (s *Service) RestoreWork(ctx context.Context, recycleItemId int64, overwrite bool) (int64, error) {
	// 1. 读快照
	item, err := s.repo.GetById(ctx, recycleItemId)
	if err != nil {
		return 0, err
	}
	if item == nil {
		return 0, ErrRecycleItemNotFound
	}
	snapshot, err := UnmarshalSnapshot(item.Snapshot)
	if err != nil {
		return 0, err
	}

	// 2. 冲突检查：(site_id, site_work_id) 是否已被新作品占用
	// GetBySiteAndSiteWorkID 查不到时返回 gorm.ErrRecordNotFound，表示无冲突，按 nil 处理
	if snapshot.Work.SiteID.Valid && snapshot.Work.SiteWorkID.Valid {
		existing, err := s.workRestorer.GetBySiteAndSiteWorkID(ctx, snapshot.Work.SiteID.Int64, snapshot.Work.SiteWorkID.String)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, err
		}
		if existing != nil {
			if !overwrite {
				return 0, ErrRestoreConflict
			}
			if err := s.workRestorer.HardDeleteWork(ctx, existing.GetID()); err != nil {
				return 0, fmt.Errorf("覆盖删除冲突作品失败: %w", err)
			}
		}
	}

	// 3. [事务外] 文件还原：逐 backup 还原文件 + 建新 persistent_store，得 backupId→newStoreId 映射
	// 走 SnapshotStoreBackups 适配器(v0/v1 兼容)
	storeIdByBackupId := make(map[int64]int64)
	for i := range snapshot.Resources {
		for _, sb := range SnapshotStoreBackups(&snapshot.Resources[i]) {
			if sb.BackupID <= 0 {
				continue
			}
			newStoreId, err := s.restoreStore(ctx, sb.BackupID)
			if err != nil {
				return 0, err
			}
			if newStoreId > 0 {
				storeIdByBackupId[sb.BackupID] = newStoreId
			}
		}
	}

	// 4. [事务内] 重建 work + 关联 + resource + 删 recycle_bin
	var newWorkId int64
	err = s.transactor.ExecInTransaction(ctx, func(txCtx context.Context) error {
		var err error
		newWorkId, err = s.workRestorer.RestoreWorkFromSnapshot(txCtx, snapshot, storeIdByBackupId)
		if err != nil {
			return err
		}
		return s.repo.Delete(txCtx, recycleItemId)
	})
	if err != nil {
		return 0, err
	}
	return newWorkId, nil
}

// restoreStore 从 backup 还原单个资源文件，返回新的 persistent_store ID
func (s *Service) restoreStore(ctx context.Context, backupId int64) (int64, error) {
	backupEntity, err := s.backupReader.GetById(ctx, backupId)
	if err != nil {
		return 0, fmt.Errorf("查询备份记录失败: %w", err)
	}
	if backupEntity == nil {
		return 0, nil
	}
	originalFilePath := ""
	if backupEntity.OriginalFilePath.Valid {
		originalFilePath = backupEntity.OriginalFilePath.String
	}
	originalFileName := ""
	if backupEntity.OriginalFileName.Valid {
		originalFileName = backupEntity.OriginalFileName.String
	}
	if originalFilePath == "" || originalFileName == "" {
		return 0, nil // 缺少原始路径信息，跳过
	}
	backupAbsPath := s.backupReader.GetBackupPath(backupEntity)
	newStoreId, err := s.storeImporter.StoreFromExternal(ctx, backupAbsPath, originalFilePath, originalFileName)
	if err != nil {
		return 0, fmt.Errorf("还原资源文件失败: %w", err)
	}
	// 清理 backup 记录（文件已被 StoreFromExternal 移走）
	if err := s.backupReader.Delete(ctx, backupId); err != nil {
		logger.Log.Warnf("清理备份记录 %d 失败: %v", backupId, err)
	}
	return newStoreId, nil
}

// Purge 彻底删除回收站条目（不可恢复）
// 删除关联的 backup 磁盘文件与记录，以及回收站快照本身
func (s *Service) Purge(ctx context.Context, recycleItemId int64) error {
	// 1. 读快照，收集所有 backupId
	item, err := s.repo.GetById(ctx, recycleItemId)
	if err != nil {
		return err
	}
	if item == nil {
		return ErrRecycleItemNotFound
	}
	snapshot, err := UnmarshalSnapshot(item.Snapshot)
	if err != nil {
		return err
	}

	// 2. [事务外] 删 backup 磁盘文件 + 记录(走适配器,v0/v1 兼容)
	for i := range snapshot.Resources {
		for _, sb := range SnapshotStoreBackups(&snapshot.Resources[i]) {
			if sb.BackupID <= 0 {
				continue
			}
			if err := s.purgeBackup(ctx, sb.BackupID); err != nil {
				return err
			}
		}
	}

	// 3. 删 recycle_bin 快照（单条删除，原子操作）
	return s.repo.Delete(ctx, recycleItemId)
}

// purgeBackup 删除单个 backup 的磁盘文件与记录
func (s *Service) purgeBackup(ctx context.Context, backupId int64) error {
	backupEntity, err := s.backupReader.GetById(ctx, backupId)
	if err != nil {
		return fmt.Errorf("查询备份记录失败: %w", err)
	}
	if backupEntity == nil {
		return nil
	}
	// 删 backup 磁盘文件（不存在则忽略，best-effort）
	backupAbsPath := s.backupReader.GetBackupPath(backupEntity)
	if err := os.Remove(backupAbsPath); err != nil && !os.IsNotExist(err) {
		logger.Log.Warnf("删除备份文件失败（将仅删除记录）: %s, %v", backupAbsPath, err)
	}
	// 删 Backup 记录
	if err := s.backupReader.Delete(ctx, backupId); err != nil {
		return fmt.Errorf("删除备份记录失败: %w", err)
	}
	return nil
}

// StartCleanup 启动 TTL 自动清理后台 goroutine（启动即清理一次，随后每 24h）
// 必须在 NewService 后调用；应用关闭时调 Stop 终止
func (s *Service) StartCleanup() {
	go s.cleanupLoop()
}

// Stop 停止 TTL 自动清理后台 goroutine
func (s *Service) Stop() {
	select {
	case <-s.stopCh:
		// 已关闭，避免重复 close panic
	default:
		close(s.stopCh)
	}
}

// cleanupLoop TTL 清理循环：启动即清理一次，随后每 24h；stopCh 关闭时退出
func (s *Service) cleanupLoop() {
	// 启动即清理一次（清理上次运行期间过期的条目）
	s.runCleanup()
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.runCleanup()
		}
	}
}

// runCleanup 执行一次 TTL 清理：查询过期条目并逐条 Purge
func (s *Service) runCleanup() {
	enabled, retentionDays := s.settingsReader.GetRecycleBinSettings()
	if !enabled || retentionDays <= 0 {
		return
	}
	ctx := context.Background()
	// 过期阈值 = 当前时间 - 保留天数（毫秒）
	expireBefore := util.GetCurrentTimestamp() - int64(retentionDays)*24*60*60*1000
	items, err := s.repo.ListExpired(ctx, expireBefore)
	if err != nil {
		logger.Log.Warnf("[RecycleBin] 查询过期回收站条目失败: %v", err)
		return
	}
	for _, item := range items {
		if err := s.Purge(ctx, item.GetID()); err != nil {
			logger.Log.Warnf("[RecycleBin] 自动清理回收站条目 %d 失败: %v", item.GetID(), err)
		} else {
			logger.Log.Infof("[RecycleBin] 自动清理过期回收站条目 %d", item.GetID())
		}
	}
}
