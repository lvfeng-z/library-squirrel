package recycleBin

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/dto"
	domain "github.com/library-squirrel/backend/base/model/entity"
	pkgerr "github.com/library-squirrel/backend/error"
	"github.com/library-squirrel/backend/storeRegistry"
	"github.com/library-squirrel/backend/util"

	"gorm.io/gorm"
)

// 错误定义
var (
	ErrRecycleItemNotFound  = &pkgerr.BusinessError{Code: 404, Message: "回收站条目不存在"}
	ErrRecycleStoreNotFound = &pkgerr.BusinessError{Code: 404, Message: "回收站文件条目不存在"}
	ErrRestoreConflict      = &pkgerr.BusinessError{Code: 409, Message: "作品复原冲突：该作品已存在，请选择放弃或覆盖"}
)

// WorkRestorer 作品软删/复原能力接口（由 work.Service 实现）
type WorkRestorer interface {
	// GetBySiteAndSiteWorkID 检查 (site_id, site_work_id) 是否已存在活作品，返回已存在作品（nil 表示无冲突）
	GetBySiteAndSiteWorkID(ctx context.Context, siteId int64, siteWorkId string) (*domain.Work, error)
	// SoftDeleteWork 软删除作品（复原"覆盖"分支：占位新作品转入回收站，让出业务键与文件路径）
	SoftDeleteWork(ctx context.Context, workId int64) error
	// GetDeletedWork 按ID获取已软删作品（nil = 非已删条目）
	GetDeletedWork(ctx context.Context, id int64) (*domain.Work, error)
	// RestoreDeletedWork 清软删标志（复原核心，文件还原由本模块编排）
	RestoreDeletedWork(ctx context.Context, id int64) error
	// RestoreWorkStores 复原作品的全部 store 记录（清软删标志与 backup_id，文件还原后调用）
	RestoreWorkStores(ctx context.Context, workId int64) error
	// ListWorkStoresIncludeDeleted 取作品的全部 store 记录行（含已删行；行内 backup_id 定位备份、file_path 定位还原目标）
	ListWorkStoresIncludeDeleted(ctx context.Context, workId int64) []*domain.PersistentStore
	// ListDeletedBefore 查询软删时间早于 expireBefore（毫秒时间戳）的已删作品，供 TTL 清理
	ListDeletedBefore(ctx context.Context, expireBefore int64) ([]*domain.Work, error)
	// DeleteWorkAndSurroundingData 物理级联删除（彻底删除链：删 work 谱系行与 store 记录/文件）
	DeleteWorkAndSurroundingData(ctx context.Context, id int64) error
}

// BackupReader 备份读取/还原能力接口（由 backup.Service 实现）
type BackupReader interface {
	// GetById 根据 ID 获取备份保管清单行
	GetById(ctx context.Context, id int64) (*domain.Backup, error)
	// GetBackupPath 获取备份文件的绝对路径
	GetBackupPath(backup *domain.Backup) string
	// RestoreFile 从备份路径还原文件到目标绝对路径
	RestoreFile(ctx context.Context, backupPath string, targetPath string) error
	// DeleteBackup 删除备份的磁盘文件与清单行（文件缺失容忍）
	DeleteBackup(ctx context.Context, id int64) error
}

// RecycleWorkQuerier 回收站作品查询接口（由 search.Service 实现；条件体系复用作品搜索 SearchCondition）
type RecycleWorkQuerier interface {
	// QueryRecycleWorkPage 分页查询回收站作品（work 已删行）
	QueryRecycleWorkPage(ctx context.Context, page, pageSize int, conditions []*dto.SearchCondition, sortField string, sortDesc bool) (*model.Page[dto.RecycleWorkDTO], error)
}

// RecycleStoreQuerier 回收站文件条目查询接口（由 search.Service 实现；文件域条件体系见 RecycleStorePageQuery）
type RecycleStoreQuerier interface {
	// QueryRecycleStorePage 分页查询回收站文件条目（persistent_store 已删行，非「作品已删」聚合形态）
	QueryRecycleStorePage(ctx context.Context, page, pageSize int, query *dto.RecycleStorePageQuery) (*model.Page[dto.RecycleStoreDTO], error)
	// ListRecycleStoreIdsDeletedBefore 圈定删除时间早于 expireBefore 的文件条目 ID（TTL 清理；
	// 与列表查询同谓词，「作品已删」聚合行不被圈定）
	ListRecycleStoreIdsDeletedBefore(ctx context.Context, expireBefore int64) ([]int64, error)
}

// StoreCleaner store 行清理能力接口（由 persistentStore.Service 实现）
type StoreCleaner interface {
	// GetDeletedStore 按 ID 获取已软删记录行（nil = 行不存在或非已删态）
	GetDeletedStore(ctx context.Context, id int64) (*domain.PersistentStore, error)
	// CleanupFile 尽力删行 file_path 指向的磁盘文件（扑空容忍；操作抑制登记）
	CleanupFile(relPath string)
	// DeleteUnscopedByIds 批量物理删行（目标为已软删行）
	DeleteUnscopedByIds(ctx context.Context, ids []int64) error
}

// RecycleBinSettingsProvider 回收站设置提供者接口（由 settings.Service 实现）
type RecycleBinSettingsProvider interface {
	// GetRecycleBinSettings 获取回收站自动清理设置（启用标志、保留天数）
	GetRecycleBinSettings() (enabled bool, retentionDays int)
}

// Service 回收站服务（软删除模型：作品条目 = work 已删行聚合其内；文件条目 = persistent_store
// 已删行且非「作品已删」聚合形态——work 不可达（离链孤儿自愈落入）或 work 存活（MarkInvalid 失效行、
// J' 替换/merge 软删残留），条目单位是 store 行、TTL 按行自身 deleted_at）
type Service struct {
	workRestorer   WorkRestorer
	backupReader   BackupReader
	recycleQuerier RecycleWorkQuerier
	storeQuerier   RecycleStoreQuerier
	storeCleaner   StoreCleaner
	settingsReader RecycleBinSettingsProvider
	workDirGetter  func() string // 每次调用获取最新 workDir（文件还原拼绝对路径用）
	stopCh         chan struct{}
}

// NewService 创建回收站服务
func NewService(workRestorer WorkRestorer, backupReader BackupReader, recycleQuerier RecycleWorkQuerier, storeQuerier RecycleStoreQuerier, storeCleaner StoreCleaner, settingsReader RecycleBinSettingsProvider, workDirGetter func() string) *Service {
	return &Service{
		workRestorer:   workRestorer,
		backupReader:   backupReader,
		recycleQuerier: recycleQuerier,
		storeQuerier:   storeQuerier,
		storeCleaner:   storeCleaner,
		settingsReader: settingsReader,
		workDirGetter:  workDirGetter,
		stopCh:         make(chan struct{}),
	}
}

// PageWorks 分页查询回收站作品条目（转发作品搜索查询链）
// conditions 条件体系与作品搜索一致（作者/标签/站点/时间范围）；sortBy: createTime | 空=deleteTime
func (s *Service) PageWorks(ctx context.Context, page int, pageSize int, conditions []*dto.SearchCondition, sortBy string, sortOrder string) (*model.Page[dto.RecycleWorkDTO], error) {
	sortField := "deleted_at"
	if sortBy == "createTime" {
		sortField = "create_time"
	}
	sortDesc := sortOrder != "asc"
	result, err := s.recycleQuerier.QueryRecycleWorkPage(ctx, page, pageSize, conditions, sortField, sortDesc)
	if err != nil {
		return nil, err
	}
	s.fillExpireDaysLeft(result)
	return result, nil
}

// PageStores 分页查询回收站文件条目（persistent_store 已删行，非「作品已删」聚合形态；
// 文件域条件体系见 RecycleStorePageQuery）
func (s *Service) PageStores(ctx context.Context, page int, pageSize int, query *dto.RecycleStorePageQuery) (*model.Page[dto.RecycleStoreDTO], error) {
	result, err := s.storeQuerier.QueryRecycleStorePage(ctx, page, pageSize, query)
	if err != nil {
		return nil, err
	}
	s.fillStoreExpireDaysLeft(result)
	return result, nil
}

// fillExpireDaysLeft 按 TTL 设置填充各条目距自动清理的剩余整天数（向上取整、负值归 0；未启用时留 null）
func (s *Service) fillExpireDaysLeft(result *model.Page[dto.RecycleWorkDTO]) {
	enabled, retentionDays := s.settingsReader.GetRecycleBinSettings()
	if !enabled || retentionDays <= 0 {
		return
	}
	now := util.GetCurrentTimestamp()
	retentionMillis := int64(retentionDays) * 24 * 60 * 60 * 1000
	for _, item := range result.Data {
		daysLeft := int(retentionMillis-(now-item.DeleteTime)+24*60*60*1000-1) / (24 * 60 * 60 * 1000)
		if daysLeft < 0 {
			daysLeft = 0
		}
		item.ExpireDaysLeft = &daysLeft
	}
}

// fillStoreExpireDaysLeft 文件条目的 TTL 剩余天数填充（与作品条目共享保留期设置，按行自身删除时间计）
func (s *Service) fillStoreExpireDaysLeft(result *model.Page[dto.RecycleStoreDTO]) {
	enabled, retentionDays := s.settingsReader.GetRecycleBinSettings()
	if !enabled || retentionDays <= 0 {
		return
	}
	now := util.GetCurrentTimestamp()
	retentionMillis := int64(retentionDays) * 24 * 60 * 60 * 1000
	for _, item := range result.Data {
		daysLeft := int(retentionMillis-(now-item.DeleteTime)+24*60*60*1000-1) / (24 * 60 * 60 * 1000)
		if daysLeft < 0 {
			daysLeft = 0
		}
		item.ExpireDaysLeft = &daysLeft
	}
}

// RestoreWork 复原已软删作品
// overwrite: 检测到 (site_id, site_work_id) 被活作品占位时是否覆盖（占位作品转入回收站）
// 冲突且 overwrite=false 时返回 ErrRestoreConflict
func (s *Service) RestoreWork(ctx context.Context, workId int64, overwrite bool) (int64, error) {
	// 1. 校验为已删条目
	work, err := s.workRestorer.GetDeletedWork(ctx, workId)
	if err != nil {
		return 0, err
	}
	if work == nil {
		return 0, ErrRecycleItemNotFound
	}

	// 2. 冲突检测：业务键是否被活作品占位（重新下载场景）
	// GetBySiteAndSiteWorkID 查不到时返回 gorm.ErrRecordNotFound，表示无冲突，按 nil 处理
	if work.SiteID.Valid && work.SiteWorkID.Valid {
		existing, err := s.workRestorer.GetBySiteAndSiteWorkID(ctx, work.SiteID.Int64, work.SiteWorkID.String)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, err
		}
		if existing != nil && existing.GetID() != workId {
			if !overwrite {
				return 0, ErrRestoreConflict
			}
			// 覆盖：占位新作品转入回收站（文件移 backup 让出 store/ 路径、业务键释放），反悔可再复原
			if err := s.workRestorer.SoftDeleteWork(ctx, existing.GetID()); err != nil {
				return 0, fmt.Errorf("覆盖转移冲突作品失败: %w", err)
			}
		}
	}

	// 3. [事务外] 文件还原（backup → store/ 原路径；缺文件警告后继续=部分复原）
	if err := s.restoreWorkFiles(ctx, workId); err != nil {
		return 0, err
	}

	// 4. 复活 store 记录 + 清作品软删标志（从属行从未离开，仅状态复位）
	if err := s.workRestorer.RestoreWorkStores(ctx, workId); err != nil {
		return 0, err
	}
	if err := s.workRestorer.RestoreDeletedWork(ctx, workId); err != nil {
		return 0, err
	}
	return workId, nil
}

// restoreWorkFiles 还原作品的全部备份文件（backup → store/ 原路径）并删除已还原备份清单行
// 备份按行内 backup_id 定位（与作品经 store 行精确圈定，同路径多代互不干扰）；
// 目标路径在 store/ 监控白名单内，逐文件登记操作抑制避免还原写入被 fsmonitor 误报
func (s *Service) restoreWorkFiles(ctx context.Context, workId int64) error {
	stores := s.workRestorer.ListWorkStoresIncludeDeleted(ctx, workId)
	for _, st := range stores {
		if st.BackupID <= 0 || !st.FilePath.Valid {
			// 无备份行（外部删除失效或备份失败残留）无文件可还原
			continue
		}
		backup, err := s.backupReader.GetById(ctx, st.BackupID)
		if err != nil || backup == nil {
			logger.Log.Warnf("还原作品 %d 备份 %d 失败（清单行缺失，跳过）: %v", workId, st.BackupID, err)
			continue
		}
		relPath := st.FilePath.String
		storeRegistry.Suppress(relPath)
		backupAbsPath := s.backupReader.GetBackupPath(backup)
		targetAbs := filepath.Join(s.workDirGetter(), relPath)
		err = s.backupReader.RestoreFile(ctx, backupAbsPath, targetAbs)
		storeRegistry.Release(relPath)
		if err != nil {
			logger.Log.Warnf("还原作品 %d 备份文件失败（跳过继续，部分复原）: %v", workId, err)
			continue
		}
		if err := s.backupReader.DeleteBackup(ctx, st.BackupID); err != nil {
			logger.Log.Warnf("清理已还原备份 %d 失败: %v", st.BackupID, err)
		}
	}
	return nil
}

// PurgeWork 彻底删除回收站作品条目（不可恢复）
// 物理级联删 work 谱系行 + store 记录行 + 按行内 backup_id 清理备份磁盘文件与清单行
func (s *Service) PurgeWork(ctx context.Context, workId int64) error {
	// 1. 校验为已删条目
	work, err := s.workRestorer.GetDeletedWork(ctx, workId)
	if err != nil {
		return err
	}
	if work == nil {
		return ErrRecycleItemNotFound
	}

	// 2. 级联前收集行内 backup_id（物理级联会删行，之后无处可查）
	stores := s.workRestorer.ListWorkStoresIncludeDeleted(ctx, workId)
	backupIds := make([]int64, 0, len(stores))
	for _, st := range stores {
		if st.BackupID > 0 {
			backupIds = append(backupIds, st.BackupID)
		}
	}

	// 3. 物理级联删除（事务内删 work 谱系行与 store 记录行；原路径文件已在 backup，无文件可删）
	if err := s.workRestorer.DeleteWorkAndSurroundingData(ctx, workId); err != nil {
		return fmt.Errorf("物理删除作品数据失败: %w", err)
	}
	for _, backupId := range backupIds {
		if err := s.backupReader.DeleteBackup(ctx, backupId); err != nil {
			return fmt.Errorf("清理备份 %d 失败: %w", backupId, err)
		}
	}
	return nil
}

// PurgeStore 彻底删除回收站文件条目（不可恢复，条目单位=store 行）
// 尽力删行内 file_path 指向的文件 + 物理删行 + 按行内 backup_id 消费式删备份（终态清理义务，
// 不产生「删行不清备份」的通路；失败模式=无主备份由治理兜底，与 PurgeWork 同族）
func (s *Service) PurgeStore(ctx context.Context, storeId int64) error {
	// 1. 校验为已删条目
	st, err := s.storeCleaner.GetDeletedStore(ctx, storeId)
	if err != nil {
		return err
	}
	if st == nil {
		return ErrRecycleStoreNotFound
	}

	// 2. 尽力删文件：正常软删行文件已移 backup/ 或已离场（扑空无害，备份文件由步骤 4 按清单行删除）；
	// file_path 指向 backup/ 域的历史残迹行（无保管清单行的散落文件）文件随行清除，不删即不可见垃圾
	if st.FilePath.Valid && st.FilePath.String != "" {
		s.storeCleaner.CleanupFile(st.FilePath.String)
	}

	// 3. 物理删行
	if err := s.storeCleaner.DeleteUnscopedByIds(ctx, []int64{storeId}); err != nil {
		return fmt.Errorf("物理删除 store 记录失败: %w", err)
	}

	// 4. 消费式清理行内引用的备份
	if st.BackupID > 0 {
		if err := s.backupReader.DeleteBackup(ctx, st.BackupID); err != nil {
			return fmt.Errorf("清理备份 %d 失败: %w", st.BackupID, err)
		}
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

// cleanupLoop TTL 清理循环：启动即清理一次（清理上次运行期间过期的条目），随后每 24h；stopCh 关闭时退出
func (s *Service) cleanupLoop() {
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

// runCleanup 执行一次 TTL 清理：两轮——过期作品条目（work 级，级联清从属行与备份）后
// 过期文件条目（store 行级）；作品与文件条目共享保留期设置
func (s *Service) runCleanup() {
	enabled, retentionDays := s.settingsReader.GetRecycleBinSettings()
	if !enabled || retentionDays <= 0 {
		return
	}
	ctx := context.Background()
	// 过期阈值 = 当前时间 - 保留天数（毫秒）
	expireBefore := util.GetCurrentTimestamp() - int64(retentionDays)*24*60*60*1000

	// 第一轮：过期作品条目
	works, err := s.workRestorer.ListDeletedBefore(ctx, expireBefore)
	if err != nil {
		logger.Log.Warnf("[RecycleBin] 查询过期回收站条目失败: %v", err)
		return
	}
	for _, work := range works {
		if err := s.PurgeWork(ctx, work.GetID()); err != nil {
			logger.Log.Warnf("[RecycleBin] 清理过期回收站条目 %d 失败: %v", work.GetID(), err)
		}
	}

	// 第二轮：过期文件条目（圈定与列表查询同谓词——「作品已删」聚合行已被第一轮级联处理，不被此轮圈定）
	storeIds, err := s.storeQuerier.ListRecycleStoreIdsDeletedBefore(ctx, expireBefore)
	if err != nil {
		logger.Log.Warnf("[RecycleBin] 查询过期文件条目失败: %v", err)
		return
	}
	for _, storeId := range storeIds {
		if err := s.PurgeStore(ctx, storeId); err != nil {
			logger.Log.Warnf("[RecycleBin] 清理过期文件条目 %d 失败: %v", storeId, err)
		}
	}
}
