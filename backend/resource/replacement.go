package resource

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/library-squirrel/backend/base/logger"
	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/shareLock"
	"github.com/library-squirrel/backend/storeRegistry"
)

// ==== 替换链能力（resource 模块提供，插件任务与 share-receive 两发起方复用）====

// StoreRef 被软删 store 行的回滚引用（能力输出与回滚登记载体，不感知任务语义）。
// BackupID=0 表示未完成行废弃分支（无备份文件，复活仅清软删标志、文件缺席交文件监控对账裁决）
type StoreRef struct {
	StoreID    int64  // persistent_store 行 ID
	ResourceID int64  // 所属 resource（回滚后完整度重算）
	BackupID   int64  // 备份清单行 ID（0=无备份）
	FilePath   string // 原文件 relPath（备份还原目标；无备份分支为空）
}

// RestoreScope 失败回滚复活的清单来源（两途二选一）：
//   - WorkID 数据驱动：插件任务场景——按作品+角色派生 victim（同键最新死代圈定，软删行即持久还原点）
//   - Victims 显式清单：策略任务场景——执行器在软删成功后登记的多作品清单
//
// 两途皆空则 no-op；Roles 为显式角色集合，空集=无 victim（「空选择=全量」的展开归发起方）
type RestoreScope struct {
	WorkID  int64
	Roles   []string
	Victims []StoreRef
}

// ReplaceStoreOps 替换链能力接口（resource 模块提供）。任务语义（板块选择、确认、任务状态）不进入能力——
// 输入输出为纯领域参数（workId/roles/StoreRef 清单），供插件任务与 share-receive 两发起方复用
type ReplaceStoreOps interface {
	// SoftDeleteWorkStoreRoles 替换前置软删：软删作品下指定角色集合的活行 store，返回被软删行清单（供回滚登记）。
	// roles 为显式角色集合（空集=不软删任何行）；「空选择=全量板块」的展开归发起方（如 taskManager
	// 展开为 store_type 封闭枚举全集后传入），能力不承接该语义
	SoftDeleteWorkStoreRoles(ctx context.Context, workId int64, roles []string) ([]StoreRef, error)
	// RestoreReplacedStores 失败回滚复活（按清单）：备份还原文件、复活 victim，并重算所属资源完整度
	RestoreReplacedStores(ctx context.Context, scope RestoreScope) error
}

// 依赖接口（由外部模块实现；与 taskManager 侧同名接口同签名不同包，结构性一致）

// ReplaceResourceLister 作品 → resource 查询（由 resource.Service 实现）
type ReplaceResourceLister interface {
	ListByWorkId(ctx context.Context, workId int64) ([]*domain.Resource, error)
}

// ReplaceResourceStoreLister resource → resource_store 关联批量查询（由 ResourceStoreRepository 实现）
type ReplaceResourceStoreLister interface {
	ListByResourceIds(ctx context.Context, resourceIds []int64) ([]*domain.ResourceStore, error)
}

// ReplaceStoreRowReader store 行含删读取与复活（由 persistentStore.Service 实现）
type ReplaceStoreRowReader interface {
	ListByIdsIncludeDeleted(ctx context.Context, ids []int64) []*domain.PersistentStore
	RestoreByIds(ctx context.Context, ids []int64) error
}

// ReplaceStoreDeleter 替换链 store 软删原语（由 persistentStore.Service 实现）
type ReplaceStoreDeleter interface {
	// DeleteWithBackup 删除 store 文件（移入 backup 建保管清单行并写行内 backup_id，记录随软删）；返回备份行 ID
	DeleteWithBackup(ctx context.Context, id int64) (int64, error)
	// SoftDeleteAndDiscardFile 软删记录并废弃其文件（未完成行分支：partial 文件无复原价值不入备份）
	SoftDeleteAndDiscardFile(ctx context.Context, id int64) error
}

// ReplaceBackupRestorer 备份文件还原（由 backup.Service 实现）
type ReplaceBackupRestorer interface {
	GetById(ctx context.Context, id int64) (*domain.Backup, error)
	GetBackupPath(backup *domain.Backup) string
	RestoreFile(ctx context.Context, backupPath string, targetPath string) error
	DeleteBackup(ctx context.Context, id int64) error
}

// ReplaceWorkLivenessReader 作品活行查询（由 work.Service 实现；回滚守卫：作品已软删则跳过）
type ReplaceWorkLivenessReader interface {
	GetById(ctx context.Context, id int64) (*domain.Work, error)
}

// ReplaceWorkDirProvider 工作目录提供（由 settings.Service 实现；备份还原目标绝对路径的根目录）
type ReplaceWorkDirProvider interface {
	GetWorkDir() string
}

// ReplaceWorkLockChecker 作品锁查询（由 shareLock.ShareLockRegistry 实现）。替换前置软删
// 会移走作品的活行 store 文件，作品正被分享拉取持有时在途拉取会读到源文件消失，须前置拒绝
type ReplaceWorkLockChecker interface {
	IsLocked(ctx context.Context, workID int64) bool
}

// ReplacementService 替换链能力编排：软删作品下所选角色活行 store、失败回滚按清单复活。
// resource_store 关联与活行过滤归 resource 域；store 软删、文件备份、作品活性等能力经接口注入
type ReplacementService struct {
	resources    ReplaceResourceLister      // 作品 → resource
	assocs       ReplaceResourceStoreLister // resource → resource_store 关联
	storeRows    ReplaceStoreRowReader      // store 行含删读取与复活
	replacer     ReplaceStoreDeleter        // 软删（备份/废弃分流）
	backups      ReplaceBackupRestorer      // 备份文件还原
	workLiveness ReplaceWorkLivenessReader  // 作品活性守卫
	recompute    ResourceRecomputer         // 回滚后完整度重算
	workDir      ReplaceWorkDirProvider     // 文件还原目标根目录
	workLock     ReplaceWorkLockChecker     // 替换前置作品锁守卫
}

// NewReplacementService 创建替换链能力服务
func NewReplacementService(
	resources ReplaceResourceLister,
	assocs ReplaceResourceStoreLister,
	storeRows ReplaceStoreRowReader,
	replacer ReplaceStoreDeleter,
	backups ReplaceBackupRestorer,
	workLiveness ReplaceWorkLivenessReader,
	recompute ResourceRecomputer,
	workDir ReplaceWorkDirProvider,
	workLock ReplaceWorkLockChecker,
) *ReplacementService {
	return &ReplacementService{
		resources:    resources,
		assocs:       assocs,
		storeRows:    storeRows,
		replacer:     replacer,
		backups:      backups,
		workLiveness: workLiveness,
		recompute:    recompute,
		workDir:      workDir,
		workLock:     workLock,
	}
}

// SoftDeleteWorkStoreRoles 替换前置软删：软删作品资源下指定角色集合的活行 store（roles 为显式
// 集合，空集=不软删任何行——空选择的展开归发起方）。
// 已完成行经 DeleteWithBackup 移文件入 backup 并写行内 backup_id（同生共死，失败保全优先报错中断）；
// 未完成行废弃文件后无备份软删（partial 无复原价值）；已软删的历史残留行跳过。
// resource_store 关联不摘——软删行经挂载链可联作品、随作品级联净化，失败回滚复活即挂载回位。
// 返回被软删行清单（供回滚登记）；中途出错返回已软删的部分清单与错误
func (s *ReplacementService) SoftDeleteWorkStoreRoles(ctx context.Context, workId int64, roles []string) ([]StoreRef, error) {
	// 前置作品锁守卫：作品正被分享拉取持有时软删会移走在途拉取读取的源文件，拒绝执行
	// （返回哨兵错误供上层透传，用户知情强制解锁后重试本操作）
	if s.workLock.IsLocked(ctx, workId) {
		logger.Log.Infof("[Resource] 替换前置软删被作品锁拒绝: workId=%d 正被分享拉取持有", workId)
		return nil, shareLock.ErrWorkLocked
	}
	resources, err := s.resources.ListByWorkId(ctx, workId)
	if err != nil {
		return nil, fmt.Errorf("查询作品资源失败: %w", err)
	}
	resourceIds := make([]int64, 0, len(resources))
	for _, res := range resources {
		resourceIds = append(resourceIds, res.GetID())
	}
	assocs, err := s.assocs.ListByResourceIds(ctx, resourceIds)
	if err != nil {
		return nil, fmt.Errorf("查询资源 store 关联失败: %w", err)
	}
	roleSet := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		roleSet[r] = struct{}{}
	}
	storeIds := make([]int64, 0, len(assocs))
	resourceIdByStore := make(map[int64]int64, len(assocs))
	for _, rs := range assocs {
		if rs.StoreID <= 0 {
			continue
		}
		if _, ok := roleSet[rs.StoreType]; !ok {
			continue
		}
		storeIds = append(storeIds, rs.StoreID)
		resourceIdByStore[rs.StoreID] = rs.ResourceID
	}
	// 角色集为空(或作品无匹配角色关联)时无候选行:显式 no-op,不发查询
	if len(storeIds) == 0 {
		return nil, nil
	}
	rows := s.storeRows.ListByIdsIncludeDeleted(ctx, storeIds)
	victims := make([]StoreRef, 0, len(rows))
	for _, row := range rows {
		if row.DeletedAt != 0 {
			continue // 历史残留代不动
		}
		ref := StoreRef{
			StoreID:    row.GetID(),
			ResourceID: resourceIdByStore[row.GetID()],
			FilePath:   row.FilePath.String,
		}
		if row.CompletedAt > 0 {
			backupID, err := s.replacer.DeleteWithBackup(ctx, row.GetID())
			if err != nil {
				return victims, fmt.Errorf("软删 store(id=%d) 失败: %w", row.GetID(), err)
			}
			ref.BackupID = backupID
		} else {
			if err := s.replacer.SoftDeleteAndDiscardFile(ctx, row.GetID()); err != nil {
				return victims, fmt.Errorf("废弃未完成 store(id=%d) 失败: %w", row.GetID(), err)
			}
		}
		victims = append(victims, ref)
	}
	logger.Log.Infof("[Resource] 替换前置软删完成: workId=%d 软删 %d 个 store 行", workId, len(victims))
	return victims, nil
}

// RestoreReplacedStores 失败回滚复活：复活被替换软删的旧 store 行。
// 清单来源两途（RestoreScope）：WorkID 数据驱动派生（插件任务，同键最新死代圈定）或
// Victims 显式清单（策略任务，执行器在软删成功后登记）。先还原有备份 victim 的文件，
// 再批量复活行（RestoreByIds 双列同清，顺带清 backup_id），随后清理已还原的备份清单行——
// 行内 backup_id 未清时删清单行会撞 persistent_store.backup_id 外键拒绝；最后重算
// victim 所属资源完整度（替换开始时被重置为未校验，须刷回回滚前状态）。
// 内部失败为 warn-and-continue（与既有回滚链语义一致），方法返回 nil
func (s *ReplacementService) RestoreReplacedStores(ctx context.Context, scope RestoreScope) error {
	victims, victimResourceIds := s.resolveVictims(ctx, scope)
	if len(victims) == 0 {
		return nil
	}
	logger.Log.Infof("[Resource] 失败回滚: 复活 %d 个被替换 store 行", len(victims))
	reviveIds := make([]int64, 0, len(victims))
	restoredBackups := make([]int64, 0, len(victims))
	for _, ref := range victims {
		// 无备份 victim（未完成行废弃分支）：仅复活，文件缺席交文件监控对账裁决
		if ref.BackupID > 0 && ref.FilePath != "" {
			backup, berr := s.backups.GetById(ctx, ref.BackupID)
			if berr != nil || backup == nil {
				logger.Log.Warnf("[Resource] 查询备份 %d 失败，跳过该行文件还原: %v", ref.BackupID, berr)
			} else {
				relPath := ref.FilePath
				storeRegistry.Suppress(relPath)
				targetAbs := filepath.Join(s.workDir.GetWorkDir(), relPath)
				rerr := s.backups.RestoreFile(ctx, s.backups.GetBackupPath(backup), targetAbs)
				storeRegistry.Release(relPath)
				if rerr != nil {
					logger.Log.Warnf("[Resource] 还原 store 文件失败(跳过该行): %v", rerr)
					continue
				}
				restoredBackups = append(restoredBackups, ref.BackupID)
			}
		}
		reviveIds = append(reviveIds, ref.StoreID)
	}
	// 先复活行并清 backup_id（RestoreByIds 双列同清，persistentStore 复原语义），
	// 再删备份——行内引用未清时 DeleteBackup 删清单行会被 persistent_store.backup_id 外键拒绝
	if err := s.storeRows.RestoreByIds(ctx, reviveIds); err != nil {
		logger.Log.Warnf("[Resource] 复活被替换 store 行失败: %v", err)
	}
	for _, backupID := range restoredBackups {
		if derr := s.backups.DeleteBackup(ctx, backupID); derr != nil {
			logger.Log.Warnf("[Resource] 清理已还原备份 %d 失败: %v", backupID, derr)
		}
	}
	for _, resourceId := range victimResourceIds {
		s.recompute.RecomputeResourceComplete(ctx, resourceId)
	}
	return nil
}

// resolveVictims 按 RestoreScope 清单来源解析 victim 清单与其涉及的 resource ID 集（回滚后完整度重算用）。
// 显式清单途直接采用登记清单；数据驱动途先过作品活性守卫（作品已软删则两代皆留已删态、归回收站作品条目管理）
func (s *ReplacementService) resolveVictims(ctx context.Context, scope RestoreScope) ([]StoreRef, []int64) {
	if len(scope.Victims) > 0 {
		resourceIds := make([]int64, 0)
		seen := make(map[int64]struct{}, len(scope.Victims))
		for _, ref := range scope.Victims {
			if ref.ResourceID > 0 {
				if _, ok := seen[ref.ResourceID]; !ok {
					seen[ref.ResourceID] = struct{}{}
					resourceIds = append(resourceIds, ref.ResourceID)
				}
			}
		}
		return scope.Victims, resourceIds
	}
	if scope.WorkID <= 0 {
		return nil, nil
	}
	if s.workLiveness != nil {
		work, err := s.workLiveness.GetById(ctx, scope.WorkID)
		if err == nil && work == nil {
			logger.Log.Warnf("[Resource] 失败回滚跳过: 作品 %d 已软删，被替换行归回收站作品条目", scope.WorkID)
			return nil, nil
		}
	}
	return s.deriveReplaceVictims(ctx, scope.WorkID, scope.Roles)
}

// replaceVictimKey 替换 victim 派生的挂载键（resource_id + store_type + store_seq，
// 与多轨续传身份、文件名消歧同维度）
type replaceVictimKey struct {
	resourceId int64
	role       string
	seq        int
}

// deriveReplaceVictims 圈定替换 victim：作品资源 × 指定角色集合，按挂载键取最新死代。
// 同时返回 victim 涉及的 resource ID 集（回滚后完整度重算用）。活行残留的键跳过——复活会与残留
// 活行同 file_path 撞部分唯一索引（活行残留仅在新建 store 清理失败的异常态出现）；
// 非替换任务的派生天然为空（新建作品的资源下无软删行）；roles 空集=无 victim
func (s *ReplacementService) deriveReplaceVictims(ctx context.Context, workId int64, roles []string) ([]StoreRef, []int64) {
	resources, err := s.resources.ListByWorkId(ctx, workId)
	if err != nil || len(resources) == 0 {
		return nil, nil
	}
	resourceIds := make([]int64, 0, len(resources))
	for _, res := range resources {
		resourceIds = append(resourceIds, res.GetID())
	}
	assocs, err := s.assocs.ListByResourceIds(ctx, resourceIds)
	if err != nil {
		logger.Log.Warnf("[Resource] 回滚派生查询关联失败: %v", err)
		return nil, nil
	}
	roleSet := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		roleSet[r] = struct{}{}
	}
	storeIds := make([]int64, 0, len(assocs))
	for _, rs := range assocs {
		if rs.StoreID <= 0 {
			continue
		}
		if _, ok := roleSet[rs.StoreType]; !ok {
			continue
		}
		storeIds = append(storeIds, rs.StoreID)
	}
	rows := s.storeRows.ListByIdsIncludeDeleted(ctx, storeIds)
	rowById := make(map[int64]*domain.PersistentStore, len(rows))
	for _, row := range rows {
		rowById[row.GetID()] = row
	}
	type keyState struct {
		hasLive    bool
		newestDead *domain.PersistentStore
	}
	states := make(map[replaceVictimKey]*keyState)
	for _, rs := range assocs {
		if rs.StoreID <= 0 {
			continue
		}
		if _, ok := roleSet[rs.StoreType]; !ok {
			continue
		}
		row := rowById[rs.StoreID]
		if row == nil {
			continue
		}
		key := replaceVictimKey{resourceId: rs.ResourceID, role: rs.StoreType, seq: rs.StoreSeq}
		state := states[key]
		if state == nil {
			state = &keyState{}
			states[key] = state
		}
		if row.DeletedAt == 0 {
			state.hasLive = true
			continue
		}
		if state.newestDead == nil || row.DeletedAt > state.newestDead.DeletedAt {
			state.newestDead = row
		}
	}
	victims := make([]StoreRef, 0, len(states))
	victimResourceIds := make([]int64, 0)
	seenResources := make(map[int64]struct{})
	for key, state := range states {
		if !state.hasLive && state.newestDead != nil {
			var backupID int64
			if state.newestDead.BackupID.Valid {
				backupID = state.newestDead.BackupID.Int64
			}
			victims = append(victims, StoreRef{
				StoreID:    state.newestDead.GetID(),
				ResourceID: key.resourceId,
				BackupID:   backupID,
				FilePath:   state.newestDead.FilePath.String,
			})
			if _, ok := seenResources[key.resourceId]; !ok {
				seenResources[key.resourceId] = struct{}{}
				victimResourceIds = append(victimResourceIds, key.resourceId)
			}
		}
	}
	return victims, victimResourceIds
}
