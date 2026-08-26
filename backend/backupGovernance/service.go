package backupGovernance

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/util"

	"go.uber.org/zap"
)

// BackupCatalog 备份保管清单的目录面（由 backup.Service 实现）：治理对账的清单数据源。
// backup 是纯叶子能力包，只实现本接口，不感知治理与引用方
type BackupCatalog interface {
	// ListCreatedBefore 查询创建时间早于 beforeMs（毫秒）的清单行（正向无主候选扫描）
	ListCreatedBefore(ctx context.Context, beforeMs int64) ([]*entity.Backup, error)
	// ListAllIDs 全量投影现存清单行 ID（反向悬空判定的现存集）
	ListAllIDs(ctx context.Context) ([]int64, error)
	// PageBackups 分页查询保管清单（create_time 倒序）；includeIDs/excludeIDs 为 ID 集过滤，
	// nil=无该向过滤（引用态语义由治理方折算成 ID 集，backup 只做纯过滤）
	PageBackups(ctx context.Context, pageNumber, pageSize int, includeIDs []int64, excludeIDs []int64) (*model.Page[entity.Backup], error)
	// DeleteBackup 删除备份的磁盘文件与清单行（文件缺失容忍；真实删除失败不删记录并返回错误）
	DeleteBackup(ctx context.Context, id int64) error
	// DeleteBackupFile 仅删除备份的磁盘文件（清单行不动）——删除流「先文件后记录」Phase A
	DeleteBackupFile(ctx context.Context, id int64) error
	// DeleteBackupRecord 仅删除备份清单行（不动磁盘文件）——文件删除失败后用户明确选择「仅删记录」的降级路径
	DeleteBackupRecord(ctx context.Context, id int64) error
}

// BackupReferencer 备份引用方（由行内嵌 backup_id 列的业务方实现：persistentStore.Service、plugin.Service）。
// 引用方枚举是开放集合，登记义务落点在 app.go 装配处（漏登记=该方备份被治理误判无主清理）
type BackupReferencer interface {
	// Name 引用方展示名（监视哨统计分组与备份管理面板用）
	Name() string
	// ListReferencedBackupIDs 本方当前引用的全部清单行 ID。
	// persistent_store 一类带软删的表须含已删行——软删行是合法引用者（回收站待复原），
	// 漏含即活备份被误判无主清删
	ListReferencedBackupIDs(ctx context.Context) ([]int64, error)
	// ClearBackupRefsByBackupIDs 按引用目标清列（治理方算出悬空 ID 后调用，引用方无需感知清单全量）
	ClearBackupRefsByBackupIDs(ctx context.Context, ids []int64) error
}

// IllegalBackupRefSanitizer 非法引用态清除能力（可选实现）。实现方持有"活行不该带备份引用"的
// 不变量知识（persistentStore：backup_id 与软删标志单条 UPDATE 同生共死，活行携带非 0 值构造上
// 不可达），治理方对账时经类型断言调用做防御清列
type IllegalBackupRefSanitizer interface {
	// ClearIllegalAliveBackupRefs 清活行携带备份引用的非法态列，返回受影响行数
	ClearIllegalAliveBackupRefs(ctx context.Context) (int64, error)
}

// RetentionDaysProvider 无主备份保留期提供者（由 settings.Service 实现）
type RetentionDaysProvider interface {
	// GetBackupGovernanceRetentionDays 获取无主备份保留天数（恒 ≥1：小于 1 的取值已回退默认）
	GetBackupGovernanceRetentionDays() int
}

// ReferencerStats 按引用方分组的引用统计（监视哨 Warn 判定与备份管理面板展示共用）
type ReferencerStats struct {
	Name          string `json:"name"`          // 引用方展示名
	Count         int    `json:"count"`         // 引用的清单行数
	TotalBytes    int64  `json:"totalBytes"`    // 引用备份的磁盘占用（文件缺失计 0）
	OldestAgeDays int    `json:"oldestAgeDays"` // 最老引用年龄（天，按清单行创建时刻；0=无引用）
}

// observeThresholdDays 监视哨观察阈值（天）：引用方最老引用年龄超过即记 Warn 日志。
// 有主侧生命周期机制失效（引用方无终态清理调用）时年龄曲线单调上升，必然可见；
// 取回收站默认保留期 30 天的 3 倍，避开合法长寿命引用（已卸载插件重装包等）的常态区间
const observeThresholdDays = 90

// Service 备份治理服务：对 backup 保管清单做双向对账——
// 正向=清单行无任何业务列引用且超保留期→清理（无主膨胀消除，兜底替换/合并/崩溃中断等零清理链路）；
// 反向=业务列引用的清单行已不存在→清列（悬空引用防御）；监视哨=按引用方分组统计引用年龄，
// 超观察阈值记 Warn（有主侧生命周期失效的可观测性）。
// 横切维护域（定位对齐 fsmonitor）：经接口编排 backup/persistentStore/plugin 各方能力，不直接读写他模块表
type Service struct {
	catalog     BackupCatalog
	referencers []BackupReferencer
	retention   RetentionDaysProvider
	stopCh      chan struct{}
	stopOnce    sync.Once
	runMu       sync.Mutex // 对账轮次互斥（手动触发与定时巡检并发时串行，避免双轮并跑重复清列）
	// 备份管理面板统计缓存：逐行文件字节数（全量 os.Stat 大库可达秒级，TTL 内复用；
	// 删除/巡检后失效）。引用态计数与超期圈定不进缓存——保留期调整/引用变化即时生效，
	// 「清理全部无主」不拿旧圈定。statsMu 同时串行化统计计算（缓存 map 的补写在锁内共享）
	statsMu        sync.Mutex
	statsBytesById map[int64]int64
	statsCachedAt  time.Time // 零值=未缓存
}

// NewService 创建备份治理服务。
// referencers 为引用方枚举的唯一登记处——新增「业务行引用 backup 清单行」的列时必须在此追加实现
func NewService(catalog BackupCatalog, referencers []BackupReferencer, retention RetentionDaysProvider) *Service {
	return &Service{
		catalog:     catalog,
		referencers: referencers,
		retention:   retention,
		stopCh:      make(chan struct{}),
	}
}

// Start 启动治理巡检后台 goroutine（启动即巡检一次 + 每 24h；应用退出时调 Stop）
func (s *Service) Start() {
	go s.loop()
}

// Stop 停止治理巡检后台 goroutine（须在数据库关闭前调用，避免巡检操作已关闭的 DB）
func (s *Service) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
}

// loop 巡检循环：启动即跑一次（清理上次运行期间累积的无主备份），随后每 24h
func (s *Service) loop() {
	s.RunOnce(context.Background())
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.RunOnce(context.Background())
		}
	}
}

// ReconciliationResult 一轮对账的清理统计（手动「立即巡检」返回给备份管理面板）
type ReconciliationResult struct {
	OrphansCleaned      int `json:"orphansCleaned"`      // 正向：清理的无主备份数
	DanglingRefsCleared int `json:"danglingRefsCleared"` // 反向：清除的悬空引用数
	IllegalRefsCleared  int `json:"illegalRefsCleared"`  // 防御：清除的活行非法引用数
}

// RunOnce 执行一轮双向对账（反向悬空清列 → 监视哨 → 正向无主清理），返回清理统计。
// 公开供备份管理面板手动触发；与定时巡检经 runMu 串行
func (s *Service) RunOnce(ctx context.Context) ReconciliationResult {
	s.runMu.Lock()
	defer s.runMu.Unlock()

	result := ReconciliationResult{}

	// 各引用方引用集（监视哨按方分组复用）
	refsByReferencer, referenced := s.collectReferenced(ctx)

	// 反向（防御性先行，与正向无数据依赖）：业务列引用的清单行已不存在 → 清列
	result.DanglingRefsCleared, result.IllegalRefsCleared = s.clearDanglingRefs(ctx, refsByReferencer)

	// 监视哨（有主侧可观测性）：按引用方分组统计引用年龄，超阈值 Warn
	s.watchReferencers(ctx, refsByReferencer)

	// 正向：超保留期且不被任何业务列引用 → 清文件与清单行。
	// 任一引用方查询失败（哨兵 nil）时整体熔断——该方引用的备份会呈现为零引用，进候选即误清
	for _, ids := range refsByReferencer {
		if ids == nil {
			return result
		}
	}
	result.OrphansCleaned = s.cleanupOrphans(ctx, referenced)
	return result
}

// ClearBackupRefs 按 ID 清除各引用方对清单行的引用（backup 行已被删除后的联动清列，
// 供 fsmonitor 确认流在删行后即时调用——回收站可复原状态随之即时准确）。
// 与反向对账共用引用方枚举与清列能力，即时形态；单方失败 Warn 不中断，
// 残余悬空引用由既有反向对账兜底。返回首个失败错误供调用方记日志
func (s *Service) ClearBackupRefs(ctx context.Context, backupIds []int64) error {
	if len(backupIds) == 0 {
		return nil
	}
	var firstErr error
	for _, ref := range s.referencers {
		if err := ref.ClearBackupRefsByBackupIDs(ctx, backupIds); err != nil {
			logger.Log.Warn("[backupGovernance] 联动清列失败", zap.String("referencer", ref.Name()), zap.Int64s("backupIds", backupIds), zap.Error(err))
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// collectReferenced 汇总各引用方的引用集；返回按方分组的引用列表与并集
func (s *Service) collectReferenced(ctx context.Context) (refsByReferencer map[string][]int64, referenced map[int64]struct{}) {
	refsByReferencer = make(map[string][]int64, len(s.referencers))
	referenced = make(map[int64]struct{})
	for _, ref := range s.referencers {
		ids, err := ref.ListReferencedBackupIDs(ctx)
		if err != nil {
			// 单方查询失败按空集处理会让该方引用的备份进入无主候选，正向误清风险——整轮跳过正向
			logger.Log.Error("[backupGovernance] 查询引用集失败，本轮跳过正向清理", zap.String("referencer", ref.Name()), zap.Error(err))
			// 以哨兵 nil 标记失败方，供正向判定整体熔断
			refsByReferencer[ref.Name()] = nil
			continue
		}
		refsByReferencer[ref.Name()] = ids
		for _, id := range ids {
			referenced[id] = struct{}{}
		}
	}
	return refsByReferencer, referenced
}

// clearDanglingRefs 反向对账：引用集 ∖ 现存清单行 = 悬空引用，逐方清列。
// 返回（悬空清列数, 非法活行防御清列数）
func (s *Service) clearDanglingRefs(ctx context.Context, refsByReferencer map[string][]int64) (cleared int, illegalCleared int) {
	existing, err := s.catalog.ListAllIDs(ctx)
	if err != nil {
		logger.Log.Warn("[backupGovernance] 查询现存清单行失败，本轮跳过反向对账", zap.Error(err))
		return 0, 0
	}
	existingSet := make(map[int64]struct{}, len(existing))
	for _, id := range existing {
		existingSet[id] = struct{}{}
	}
	for _, ref := range s.referencers {
		ids := refsByReferencer[ref.Name()]
		dangling := make([]int64, 0)
		for _, id := range ids {
			if _, ok := existingSet[id]; !ok {
				dangling = append(dangling, id)
			}
		}
		if len(dangling) == 0 {
			continue
		}
		if err := ref.ClearBackupRefsByBackupIDs(ctx, dangling); err != nil {
			logger.Log.Warn("[backupGovernance] 清理悬空引用失败", zap.String("referencer", ref.Name()), zap.Int64s("backupIds", dangling), zap.Error(err))
		} else {
			cleared += len(dangling)
			logger.Log.Infof("[backupGovernance] 已清理 %d 条悬空引用（清单行已不存在）: %s", len(dangling), ref.Name())
		}
	}
	// 非法引用态防御清列（活行携带 备份引用，构造上不可达；实现方按需提供）
	for _, ref := range s.referencers {
		if san, ok := ref.(IllegalBackupRefSanitizer); ok {
			if n, err := san.ClearIllegalAliveBackupRefs(ctx); err != nil {
				logger.Log.Warn("[backupGovernance] 清理非法活行引用失败", zap.String("referencer", ref.Name()), zap.Error(err))
			} else if n > 0 {
				illegalCleared += int(n)
				logger.Log.Warnf("[backupGovernance] 检出并清理 %d 行活行非法备份引用（构造上不可达，疑外部直改数据库）: %s", n, ref.Name())
			}
		}
	}
	return cleared, illegalCleared
}

// watchReferencers 监视哨：按引用方分组统计引用数量/占用/最老引用年龄，最老年龄超观察阈值记 Warn。
// 合法长寿命引用（已卸载插件重装包等）会周期性命中 Warn——仅日志级噪音，接受为可观测性代价
func (s *Service) watchReferencers(ctx context.Context, refsByReferencer map[string][]int64) {
	stats, err := s.computeReferencerStats(ctx, refsByReferencer)
	if err != nil {
		logger.Log.Warn("[backupGovernance] 统计引用方数据失败，本轮跳过监视哨", zap.Error(err))
		return
	}
	for _, st := range stats {
		if st.Count > 0 {
			logger.Log.Debugf("[backupGovernance] 引用统计 %s: 数量=%d 占用=%d 年龄峰值=%d天", st.Name, st.Count, st.TotalBytes, st.OldestAgeDays)
		}
		if st.OldestAgeDays > observeThresholdDays {
			logger.Log.Warnf("[backupGovernance] 引用方 %s 的最老引用年龄 %d 天超过观察阈值 %d 天（引用 %d 份备份，占用 %d 字节）——生命周期清理路径可能缺失", st.Name, st.OldestAgeDays, observeThresholdDays, st.Count, st.TotalBytes)
		}
	}
}

// computeReferencerStats 按引用方分组计算引用统计（监视哨 Warn 判定与备份管理面板展示共用的同源数据面）。
// 引用集查询失败方（哨兵 nil）无可靠数据，跳过不参与统计
func (s *Service) computeReferencerStats(ctx context.Context, refsByReferencer map[string][]int64) ([]ReferencerStats, error) {
	if len(s.referencers) == 0 {
		return nil, nil
	}
	// 全量清单行（创建时刻与保管路径），按 ID 建索引
	rows, err := s.catalog.ListCreatedBefore(ctx, util.GetCurrentTimestamp())
	if err != nil {
		return nil, err
	}
	byId := make(map[int64]*entity.Backup, len(rows))
	for _, row := range rows {
		byId[row.GetID()] = row
	}
	now := util.GetCurrentTimestamp()
	dayMs := int64(24 * 60 * 60 * 1000)
	stats := make([]ReferencerStats, 0, len(s.referencers))
	for _, ref := range s.referencers {
		ids := refsByReferencer[ref.Name()]
		if ids == nil {
			continue // 引用集查询失败方（哨兵 nil）
		}
		st := ReferencerStats{Name: ref.Name(), Count: len(ids)}
		var oldestCreate int64
		for _, id := range ids {
			row, ok := byId[id]
			if !ok {
				continue // 悬空引用（反向对账已清列），不计入年龄与占用
			}
			if create := row.GetCreateTime(); create > 0 && (oldestCreate == 0 || create < oldestCreate) {
				oldestCreate = create
			}
			st.TotalBytes += backupFileBytes(row)
		}
		if oldestCreate > 0 {
			st.OldestAgeDays = int((now - oldestCreate) / dayMs)
		}
		stats = append(stats, st)
	}
	return stats, nil
}

// cleanupOrphans 正向对账：创建时间早于「现在 − 保留期」且不被任何业务列引用的清单行，逐个删除，返回清理数。
// 保留期是正确性参数：替换任务在途期间其还原点备份合法地零业务引用（内存清单/无清单），
// 保留期垫住该窗口；任一引用方查询失败时本轮整体熔断（collectReferenced 已置哨兵）
func (s *Service) cleanupOrphans(ctx context.Context, referenced map[int64]struct{}) int {
	retentionDays := s.retention.GetBackupGovernanceRetentionDays()
	expireBefore := util.GetCurrentTimestamp() - int64(retentionDays)*24*60*60*1000
	candidates, err := s.catalog.ListCreatedBefore(ctx, expireBefore)
	if err != nil {
		logger.Log.Warn("[backupGovernance] 查询无主候选失败", zap.Error(err))
		return 0
	}
	cleaned := 0
	for _, row := range candidates {
		if _, ok := referenced[row.GetID()]; ok {
			continue
		}
		if err := s.catalog.DeleteBackup(ctx, row.GetID()); err != nil {
			logger.Log.Warn("[backupGovernance] 清理无主备份失败", zap.Int64("backupId", row.GetID()), zap.Error(err))
			continue
		}
		cleaned++
	}
	if cleaned > 0 {
		logger.Log.Infof("[backupGovernance] 本轮清理无主备份 %d 份（保留期 %d 天）", cleaned, retentionDays)
	}
	return cleaned
}

// backupFileBytes 取备份文件的磁盘占用（os.Stat；文件缺失或路径无效计 0——文件存在性感知属 fsmonitor 域）
func backupFileBytes(row *entity.Backup) int64 {
	if !row.Workdir.Valid || !row.FilePath.Valid {
		return 0
	}
	info, err := os.Stat(filepath.Join(row.Workdir.String, row.FilePath.String))
	if err != nil {
		return 0
	}
	return info.Size()
}
