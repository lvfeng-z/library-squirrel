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
	ErrRecycleItemNotFound    = &pkgerr.BusinessError{Code: 404, Message: "回收站条目不存在"}
	ErrRecycleStoreNotFound   = &pkgerr.BusinessError{Code: 404, Message: "回收站文件条目不存在"}
	ErrRecycleWorkSetNotFound = &pkgerr.BusinessError{Code: 404, Message: "回收站作品集条目不存在"}
	ErrRestoreConflict        = &pkgerr.BusinessError{Code: 409, Message: "作品复原冲突：该作品已存在，请选择放弃或覆盖"}
	ErrRestoreWorkSetConflict = &pkgerr.BusinessError{Code: 409, Message: "作品集复原冲突：该作品集已存在，请选择放弃或覆盖"}
	// ErrRestoreStoreNoBackup 无备份（外部裁决失效行/备份缺失）不可复原——仅清理态
	ErrRestoreStoreNoBackup = &pkgerr.BusinessError{Code: 409, Message: "该条目无备份，不可复原"}
	// ErrRestoreStoreUnreachable 挂载链断或作品已软删——复原目标作品不可达
	ErrRestoreStoreUnreachable = &pkgerr.BusinessError{Code: 409, Message: "该条目所属作品不可达，不可复原"}
	// ErrRestoreStorePathConflict 还原目标路径被其他资源的活行占用（路径冲突异常态，须人工处理）
	ErrRestoreStorePathConflict = &pkgerr.BusinessError{Code: 409, Message: "还原目标路径被其他资源占用，请先处理该文件"}
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
	// RestoreWorkStores 复原作品的复活集 store 记录（同键最新死代，清软删标志与 backup_id，文件还原由本模块编排）
	RestoreWorkStores(ctx context.Context, workId int64) error
	// ListWorkStoresIncludeDeleted 取作品的全部 store 记录行（含已删行；彻底删除链按行内 backup_id 定位备份）
	ListWorkStoresIncludeDeleted(ctx context.Context, workId int64) []*domain.PersistentStore
	// ListRevivableWorkStores 取作品的复活集（同键最新死代行；文件还原链按此圈定，避免更早死代备份还原互相覆盖）
	ListRevivableWorkStores(ctx context.Context, workId int64) []*domain.PersistentStore
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

// WorkSetRestorer 作品集软删/复原能力接口（由 workSet.Service 实现）
type WorkSetRestorer interface {
	// GetDeletedWorkSet 按ID获取已软删作品集（nil = 非已删条目）
	GetDeletedWorkSet(ctx context.Context, id int64) (*domain.WorkSet, error)
	// GetBySiteAndSiteWorkSetID 查业务键 (site_id, site_work_set_id) 的活作品集（复原冲突检测；
	// 无占位时返回 gorm.ErrRecordNotFound）
	GetBySiteAndSiteWorkSetID(ctx context.Context, siteId int64, siteWorkSetId string) (*domain.WorkSet, error)
	// SoftDeleteWorkSet 软删除作品集（复原"覆盖"分支：占位新作品集转入回收站，让出业务键）
	SoftDeleteWorkSet(ctx context.Context, id int64) error
	// RestoreDeletedWorkSet 清软删标志（复原核心；关联行保留，一条 UPDATE 即全恢复）
	RestoreDeletedWorkSet(ctx context.Context, id int64) error
	// DeleteWorkSetAndAssociations 物理级联删除（彻底删除链：作品集行 + 成员关联 + 父子关联双向）
	DeleteWorkSetAndAssociations(ctx context.Context, id int64) error
	// ListDeletedBefore 查询软删时间早于 expireBefore（毫秒时间戳）的已删作品集，供 TTL 清理
	ListDeletedBefore(ctx context.Context, expireBefore int64) ([]*domain.WorkSet, error)
}

// RecycleWorkSetQuerier 回收站作品集条目查询接口（由 search.Service 实现；作品集域平铺条件体系）
type RecycleWorkSetQuerier interface {
	// QueryRecycleWorkSetPage 分页查询回收站作品集条目（work_set 已删行）
	QueryRecycleWorkSetPage(ctx context.Context, page, pageSize int, query *dto.RecycleWorkSetPageQuery) (*model.Page[dto.RecycleWorkSetDTO], error)
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
	// GetRecycleStoreMount 查询单个 store 行的挂载身份与作品活性（复原置换链）
	GetRecycleStoreMount(ctx context.Context, storeId int64) (*dto.StoreMountDTO, error)
	// GetAliveStoreIdByKey 查挂载键 (resource_id, store_type, store_seq) 下的活行 store ID（无则 0）
	GetAliveStoreIdByKey(ctx context.Context, resourceId int64, storeType string, storeSeq int) (int64, error)
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

// StoreRestorer store 行复原置换能力接口（由 persistentStore.Service 实现；版本回滚置换链）
type StoreRestorer interface {
	// GetById 按 ID 查活行记录行（置换对象完成态分派用；nil = 行不存在或已删）
	GetById(ctx context.Context, id int64) (*domain.PersistentStore, error)
	// GetByFilePath 按路径查活行记录行（置换的路径占位检测；nil = 无活行占位）
	GetByFilePath(ctx context.Context, filePath string) (*domain.PersistentStore, error)
	// DeleteWithBackup 删除 store 文件（移入 backup 建保管清单行并写行内 backup_id，记录随软删）
	DeleteWithBackup(ctx context.Context, id int64) (int64, error)
	// SoftDeleteAndDiscardFile 软删记录并废弃其文件（未完成占位行分支：partial 文件不入备份）
	SoftDeleteAndDiscardFile(ctx context.Context, id int64) error
	// RestoreByIds 批量复活记录（清软删标志与 backup_id；文件还原回 store/ 后调用）
	RestoreByIds(ctx context.Context, ids []int64) error
}

// ResourceRecomputer 资源完整度重算（由 resource.Service 实现；复原置换改变角色构成后刷新）
type ResourceRecomputer interface {
	RecomputeResourceComplete(ctx context.Context, resourceId int64)
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
	workRestorer          WorkRestorer
	backupReader          BackupReader
	recycleQuerier        RecycleWorkQuerier
	storeQuerier          RecycleStoreQuerier
	storeCleaner          StoreCleaner
	storeRestorer         StoreRestorer
	resourceRecomputer    ResourceRecomputer
	settingsReader        RecycleBinSettingsProvider
	workDirGetter         func() string // 每次调用获取最新 workDir（文件还原拼绝对路径用）
	workSetRestorer       WorkSetRestorer
	recycleWorkSetQuerier RecycleWorkSetQuerier
	stopCh                chan struct{}
}

// NewService 创建回收站服务
func NewService(workRestorer WorkRestorer, backupReader BackupReader, recycleQuerier RecycleWorkQuerier, storeQuerier RecycleStoreQuerier, storeCleaner StoreCleaner, storeRestorer StoreRestorer, resourceRecomputer ResourceRecomputer, settingsReader RecycleBinSettingsProvider, workDirGetter func() string, workSetRestorer WorkSetRestorer, recycleWorkSetQuerier RecycleWorkSetQuerier) *Service {
	return &Service{
		workRestorer:          workRestorer,
		backupReader:          backupReader,
		recycleQuerier:        recycleQuerier,
		storeQuerier:          storeQuerier,
		storeCleaner:          storeCleaner,
		storeRestorer:         storeRestorer,
		resourceRecomputer:    resourceRecomputer,
		settingsReader:        settingsReader,
		workDirGetter:         workDirGetter,
		workSetRestorer:       workSetRestorer,
		recycleWorkSetQuerier: recycleWorkSetQuerier,
		stopCh:                make(chan struct{}),
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

// PageWorkSets 分页查询回收站作品集条目（work_set 已删行；作品集域平铺条件体系见 RecycleWorkSetPageQuery）
func (s *Service) PageWorkSets(ctx context.Context, page int, pageSize int, query *dto.RecycleWorkSetPageQuery) (*model.Page[dto.RecycleWorkSetDTO], error) {
	result, err := s.recycleWorkSetQuerier.QueryRecycleWorkSetPage(ctx, page, pageSize, query)
	if err != nil {
		return nil, err
	}
	s.fillWorkSetExpireDaysLeft(result)
	return result, nil
}

// computeExpireDaysLeft 按 TTL 设置计算距自动清理的剩余整天数（向上取整、负值归 0；未启用时 null）
func (s *Service) computeExpireDaysLeft(deleteTime int64) *int {
	enabled, retentionDays := s.settingsReader.GetRecycleBinSettings()
	if !enabled || retentionDays <= 0 {
		return nil
	}
	now := util.GetCurrentTimestamp()
	retentionMillis := int64(retentionDays) * 24 * 60 * 60 * 1000
	daysLeft := int(retentionMillis-(now-deleteTime)+24*60*60*1000-1) / (24 * 60 * 60 * 1000)
	if daysLeft < 0 {
		daysLeft = 0
	}
	return &daysLeft
}

// fillExpireDaysLeft 作品条目的 TTL 剩余天数填充
func (s *Service) fillExpireDaysLeft(result *model.Page[dto.RecycleWorkDTO]) {
	for _, item := range result.Data {
		item.ExpireDaysLeft = s.computeExpireDaysLeft(item.DeleteTime)
	}
}

// fillStoreExpireDaysLeft 文件条目的 TTL 剩余天数填充（与作品条目共享保留期设置，按行自身删除时间计）
func (s *Service) fillStoreExpireDaysLeft(result *model.Page[dto.RecycleStoreDTO]) {
	for _, item := range result.Data {
		item.ExpireDaysLeft = s.computeExpireDaysLeft(item.DeleteTime)
	}
}

// fillWorkSetExpireDaysLeft 作品集条目的 TTL 剩余天数填充（共享保留期设置）
func (s *Service) fillWorkSetExpireDaysLeft(result *model.Page[dto.RecycleWorkSetDTO]) {
	for _, item := range result.Data {
		item.ExpireDaysLeft = s.computeExpireDaysLeft(item.DeleteTime)
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

	// 4. 复活 store 记录（复活集=同键最新死代）+ 清作品软删标志（从属行从未离开，仅状态复位）
	if err := s.workRestorer.RestoreWorkStores(ctx, workId); err != nil {
		return 0, err
	}
	if err := s.workRestorer.RestoreDeletedWork(ctx, workId); err != nil {
		return 0, err
	}
	return workId, nil
}

// restoreWorkFiles 还原作品复活集的备份文件（backup → store/ 原路径）并删除已还原备份清单行
// 复活集=同键最新死代（与 RestoreWorkStores 共用派生）——更早死代的备份不动，避免同路径多代
// 文件还原互相覆盖；备份按行内 backup_id 定位（与作品经 store 行精确圈定，同路径多代互不干扰）；
// 目标路径在 store/ 监控白名单内，逐文件登记操作抑制避免还原写入被 fsmonitor 误报
func (s *Service) restoreWorkFiles(ctx context.Context, workId int64) error {
	stores := s.workRestorer.ListRevivableWorkStores(ctx, workId)
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

// RestoreStore 复原文件条目（版本回滚置换：行内备份还原为当前版本，被置换的当前活行转入回收站，
// 其自身可再复原——回滚即一次替换，机制单态）。前置：行已软删、有备份、挂载链可达活作品。
// 关联零操作：本行关联保留（复活即挂载回位）、被置换行关联保留成死（双关联标准形态）
func (s *Service) RestoreStore(ctx context.Context, storeId int64) error {
	// 1. 校验：已删行 + 行内备份 + 挂载活作品
	st, err := s.storeCleaner.GetDeletedStore(ctx, storeId)
	if err != nil {
		return err
	}
	if st == nil {
		return ErrRecycleStoreNotFound
	}
	if st.BackupID <= 0 || !st.FilePath.Valid {
		return ErrRestoreStoreNoBackup
	}
	mount, err := s.storeQuerier.GetRecycleStoreMount(ctx, storeId)
	if err != nil {
		return err
	}
	if mount.ResourceId == 0 || !mount.WorkAlive {
		return ErrRestoreStoreUnreachable
	}

	// 2. 置换同键当前代：挂载键 (resource,role,seq) 的活行让位（软删入回收站，可再复原）——
	// 不置换会出现同键双活行，破坏挂载不变量并令完整度计数翻倍
	if aliveId, err := s.storeQuerier.GetAliveStoreIdByKey(ctx, mount.ResourceId, mount.Role, mount.Seq); err != nil {
		return err
	} else if aliveId > 0 && aliveId != storeId {
		if err := s.swapOutLiveRow(ctx, aliveId); err != nil {
			return err
		}
	}

	// 3. 置换同路径占位行：还原目标路径上的其他活行让出路径；跨资源占位为路径冲突异常态，拒绝人工处理
	if holder, err := s.storeRestorer.GetByFilePath(ctx, st.FilePath.String); err != nil {
		return err
	} else if holder != nil && holder.GetID() != storeId {
		holderMount, err := s.storeQuerier.GetRecycleStoreMount(ctx, holder.GetID())
		if err != nil {
			return err
		}
		if holderMount.ResourceId != mount.ResourceId {
			return ErrRestoreStorePathConflict
		}
		if err := s.swapOutLiveRow(ctx, holder.GetID()); err != nil {
			return err
		}
	}

	// 4. 文件还原（backup → 原路径；操作抑制登记防 fsmonitor 误报）
	backup, err := s.backupReader.GetById(ctx, st.BackupID)
	if err != nil || backup == nil {
		return fmt.Errorf("查询备份 %d 失败: %w", st.BackupID, err)
	}
	relPath := st.FilePath.String
	storeRegistry.Suppress(relPath)
	backupAbsPath := s.backupReader.GetBackupPath(backup)
	targetAbs := filepath.Join(s.workDirGetter(), relPath)
	rerr := s.backupReader.RestoreFile(ctx, backupAbsPath, targetAbs)
	storeRegistry.Release(relPath)
	if rerr != nil {
		return fmt.Errorf("还原备份文件失败: %w", rerr)
	}

	// 5. 复活本行（清软删标志与 backup_id）
	if err := s.storeRestorer.RestoreByIds(ctx, []int64{storeId}); err != nil {
		return fmt.Errorf("复活记录失败: %w", err)
	}

	// 6. 清备份清单行（行已复活引用已清；失败则残余清单行由无主治理兜底）
	if err := s.backupReader.DeleteBackup(ctx, st.BackupID); err != nil {
		logger.Log.Warnf("清理已还原备份 %d 失败: %v", st.BackupID, err)
	}

	// 7. 重算完整度（角色构成可能变化，如合并回滚补回轨道）
	s.resourceRecomputer.RecomputeResourceComplete(ctx, mount.ResourceId)
	return nil
}

// swapOutLiveRow 置换活行：已完成入备份软删（自身入回收站可再复原），未完成废弃文件软删
// （partial 文件无复原价值不入备份）
func (s *Service) swapOutLiveRow(ctx context.Context, id int64) error {
	row, err := s.storeRestorer.GetById(ctx, id)
	if err != nil {
		return err
	}
	if row == nil {
		return nil // 已非活行（并发变更），幂等跳过
	}
	if row.CompletedAt > 0 {
		if _, err := s.storeRestorer.DeleteWithBackup(ctx, id); err != nil {
			return fmt.Errorf("置换当前版本(id=%d) 失败: %w", id, err)
		}
		return nil
	}
	if err := s.storeRestorer.SoftDeleteAndDiscardFile(ctx, id); err != nil {
		return fmt.Errorf("置换未完成占位行(id=%d) 失败: %w", id, err)
	}
	return nil
}

// RestoreWorkSet 复原已软删作品集（关联行保留，清标志即全恢复——层级/成员/封面）
// overwrite: 检测到业务键 (site_id, site_work_set_id) 被活作品集占位时是否覆盖（占位作品集转入回收站）
// 冲突且 overwrite=false 时返回 ErrRestoreWorkSetConflict；本地手建集（键 NULL）不参与唯一性、无冲突可能
func (s *Service) RestoreWorkSet(ctx context.Context, workSetId int64, overwrite bool) (int64, error) {
	// 1. 校验为已删条目
	ws, err := s.workSetRestorer.GetDeletedWorkSet(ctx, workSetId)
	if err != nil {
		return 0, err
	}
	if ws == nil {
		return 0, ErrRecycleWorkSetNotFound
	}

	// 2. 冲突检测：业务键是否被活作品集占位（重新下载同键作品集场景）
	if ws.SiteID.Valid && ws.SiteWorkSetID.Valid {
		existing, err := s.workSetRestorer.GetBySiteAndSiteWorkSetID(ctx, ws.SiteID.Int64, ws.SiteWorkSetID.String)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, err
		}
		if existing != nil && existing.GetID() != workSetId {
			if !overwrite {
				return 0, ErrRestoreWorkSetConflict
			}
			// 覆盖：占位新作品集转入回收站让出业务键，反悔可再复原
			if err := s.workSetRestorer.SoftDeleteWorkSet(ctx, existing.GetID()); err != nil {
				return 0, fmt.Errorf("覆盖转移冲突作品集失败: %w", err)
			}
		}
	}

	// 3. 清软删标志
	if err := s.workSetRestorer.RestoreDeletedWorkSet(ctx, workSetId); err != nil {
		return 0, err
	}
	return workSetId, nil
}

// PurgeWorkSet 彻底删除回收站作品集条目（不可恢复）
// 物理级联删作品集行 + 成员关联 + 父子关联双向；作品集无自有文件与备份，无文件面
func (s *Service) PurgeWorkSet(ctx context.Context, workSetId int64) error {
	ws, err := s.workSetRestorer.GetDeletedWorkSet(ctx, workSetId)
	if err != nil {
		return err
	}
	if ws == nil {
		return ErrRecycleWorkSetNotFound
	}
	return s.workSetRestorer.DeleteWorkSetAndAssociations(ctx, workSetId)
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

	// 第三轮：过期作品集条目（级联清关联行；共享保留期设置）
	if s.workSetRestorer != nil {
		workSets, err := s.workSetRestorer.ListDeletedBefore(ctx, expireBefore)
		if err != nil {
			logger.Log.Warnf("[RecycleBin] 查询过期作品集条目失败: %v", err)
			return
		}
		for _, ws := range workSets {
			if err := s.PurgeWorkSet(ctx, ws.GetID()); err != nil {
				logger.Log.Warnf("[RecycleBin] 清理过期作品集条目 %d 失败: %v", ws.GetID(), err)
			}
		}
	}
}
