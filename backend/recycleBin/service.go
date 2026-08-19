package recycleBin

import (
	"context"
	"errors"
	"fmt"
	"os"
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
	ErrRecycleItemNotFound = &pkgerr.BusinessError{Code: 404, Message: "回收站条目不存在"}
	ErrRestoreConflict     = &pkgerr.BusinessError{Code: 409, Message: "作品复原冲突：该作品已存在，请选择放弃或覆盖"}
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
	// RestoreWorkStores 复原作品的全部 store 记录（清软删标志，文件还原后调用）
	RestoreWorkStores(ctx context.Context, workId int64) error
	// ListWorkStoreFilePaths 取作品的全部 store 文件路径（含已删行；反查 backup 的路径清单）
	ListWorkStoreFilePaths(ctx context.Context, workId int64) []string
	// ListDeletedBefore 查询软删时间早于 expireBefore（毫秒时间戳）的已删作品，供 TTL 清理
	ListDeletedBefore(ctx context.Context, expireBefore int64) ([]*domain.Work, error)
	// DeleteWorkAndSurroundingData 物理级联删除（彻底删除链：删 work 谱系行与 store 记录/文件）
	DeleteWorkAndSurroundingData(ctx context.Context, id int64) error
}

// BackupReader 备份读取/还原能力接口（由 backup.Service 实现）
type BackupReader interface {
	// GetById 根据 ID 获取备份记录
	GetById(ctx context.Context, id int64) (*domain.Backup, error)
	// GetBackupPath 获取备份文件的绝对路径
	GetBackupPath(backup *domain.Backup) string
	// ListByOriginalPaths 按原始 store 路径集合批量查询备份记录（软删除链归属通路）
	ListByOriginalPaths(ctx context.Context, originalRelPaths []string) ([]*domain.Backup, error)
	// RestoreFile 从备份路径还原文件到目标绝对路径
	RestoreFile(ctx context.Context, backupPath string, targetPath string) error
	// Delete 删除备份记录
	Delete(ctx context.Context, id int64) error
}

// RecycleWorkQuerier 回收站作品查询接口（由 search.Service 实现；条件体系复用作品搜索 SearchCondition）
type RecycleWorkQuerier interface {
	// QueryRecycleWorkPage 分页查询回收站作品（work 已删行）
	QueryRecycleWorkPage(ctx context.Context, page, pageSize int, conditions []*dto.SearchCondition, sortField string, sortDesc bool) (*model.Page[dto.RecycleWorkDTO], error)
}

// RecycleBinSettingsProvider 回收站设置提供者接口（由 settings.Service 实现）
type RecycleBinSettingsProvider interface {
	// GetRecycleBinSettings 获取回收站自动清理设置（启用标志、保留天数）
	GetRecycleBinSettings() (enabled bool, retentionDays int)
}

// Service 回收站服务（软删除模型：回收站条目 = work 已删行）
type Service struct {
	workRestorer   WorkRestorer
	backupReader   BackupReader
	recycleQuerier RecycleWorkQuerier
	settingsReader RecycleBinSettingsProvider
	workDirGetter  func() string // 每次调用获取最新 workDir（文件还原拼绝对路径用）
	stopCh         chan struct{}
}

// NewService 创建回收站服务
func NewService(workRestorer WorkRestorer, backupReader BackupReader, recycleQuerier RecycleWorkQuerier, settingsReader RecycleBinSettingsProvider, workDirGetter func() string) *Service {
	return &Service{
		workRestorer:   workRestorer,
		backupReader:   backupReader,
		recycleQuerier: recycleQuerier,
		settingsReader: settingsReader,
		workDirGetter:  workDirGetter,
		stopCh:         make(chan struct{}),
	}
}

// Page 分页查询回收站列表（转发作品搜索查询链）
// conditions 条件体系与作品搜索一致（作者/标签/站点/时间范围）；sortBy: createTime | 空=deleteTime
func (s *Service) Page(ctx context.Context, page int, pageSize int, conditions []*dto.SearchCondition, sortBy string, sortOrder string) (*model.Page[dto.RecycleWorkDTO], error) {
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

// restoreWorkFiles 还原作品的全部备份文件（backup → store/ 原路径）并删除已还原备份记录
// 目标路径在 store/ 监控白名单内，逐文件登记操作抑制避免还原写入被 fsmonitor 误报
func (s *Service) restoreWorkFiles(ctx context.Context, workId int64) error {
	// 备份清单按 original_file_path 反查：作品 store 路径集合（含删行）→ backup IN 匹配
	paths := s.workRestorer.ListWorkStoreFilePaths(ctx, workId)
	backups, err := s.backupReader.ListByOriginalPaths(ctx, paths)
	if err != nil {
		return fmt.Errorf("查询作品备份失败: %w", err)
	}
	for _, backup := range backups {
		if !backup.OriginalFilePath.Valid || backup.OriginalFilePath.String == "" {
			continue
		}
		relPath := backup.OriginalFilePath.String
		storeRegistry.Suppress(relPath)
		backupAbsPath := s.backupReader.GetBackupPath(backup)
		targetAbs := filepath.Join(s.workDirGetter(), relPath)
		err := s.backupReader.RestoreFile(ctx, backupAbsPath, targetAbs)
		storeRegistry.Release(relPath)
		if err != nil {
			logger.Log.Warnf("还原作品 %d 备份文件失败（跳过继续，部分复原）: %v", workId, err)
			continue
		}
		if err := s.backupReader.Delete(ctx, backup.GetID()); err != nil {
			logger.Log.Warnf("清理已还原备份记录 %d 失败: %v", backup.GetID(), err)
		}
	}
	return nil
}

// Purge 彻底删除回收站条目（不可恢复）
// 物理级联删 work 谱系行 + 清理 backup 磁盘文件与记录
func (s *Service) Purge(ctx context.Context, workId int64) error {
	// 1. 校验为已删条目
	work, err := s.workRestorer.GetDeletedWork(ctx, workId)
	if err != nil {
		return err
	}
	if work == nil {
		return ErrRecycleItemNotFound
	}

	// 2. 先收集作品 store 路径清单并反查备份（物理级联会删行，含删查询也无处可查）
	paths := s.workRestorer.ListWorkStoreFilePaths(ctx, workId)
	backups, err := s.backupReader.ListByOriginalPaths(ctx, paths)
	if err != nil {
		return err
	}

	// 3. 物理级联删除（事务内删行；store 原路径文件已在 backup，链内文件删除扑空无害）
	if err := s.workRestorer.DeleteWorkAndSurroundingData(ctx, workId); err != nil {
		return fmt.Errorf("物理删除作品数据失败: %w", err)
	}
	for _, backup := range backups {
		if err := s.purgeBackup(ctx, backup); err != nil {
			return err
		}
	}
	return nil
}

// purgeBackup 删除单个备份的磁盘文件与记录
func (s *Service) purgeBackup(ctx context.Context, backup *domain.Backup) error {
	backupAbsPath := s.backupReader.GetBackupPath(backup)
	if err := os.Remove(backupAbsPath); err != nil && !os.IsNotExist(err) {
		logger.Log.Warnf("删除备份文件失败（将仅删除记录）: %s, %v", backupAbsPath, err)
	}
	if err := s.backupReader.Delete(ctx, backup.GetID()); err != nil {
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

// runCleanup 执行一次 TTL 清理：查询过期已删作品并逐条 Purge
func (s *Service) runCleanup() {
	enabled, retentionDays := s.settingsReader.GetRecycleBinSettings()
	if !enabled || retentionDays <= 0 {
		return
	}
	ctx := context.Background()
	// 过期阈值 = 当前时间 - 保留天数（毫秒）
	expireBefore := util.GetCurrentTimestamp() - int64(retentionDays)*24*60*60*1000
	works, err := s.workRestorer.ListDeletedBefore(ctx, expireBefore)
	if err != nil {
		logger.Log.Warnf("[RecycleBin] 查询过期回收站条目失败: %v", err)
		return
	}
	for _, work := range works {
		if err := s.Purge(ctx, work.GetID()); err != nil {
			logger.Log.Warnf("[RecycleBin] 清理过期回收站条目 %d 失败: %v", work.GetID(), err)
		}
	}
}
