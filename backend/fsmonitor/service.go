package fsmonitor

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/library-squirrel/backend/base/logger"
)

// EventEmitter 前端事件发射器(由 Wails EventManager 实现)。
// 本地定义避免反向依赖 taskManager/plugin 包(与 resource/merge_service.go 的本地接口同模式)。
type EventEmitter interface {
	Emit(eventName string, data ...any) bool
}

// Service 工作目录监控编排服务。
// 持有平台依赖集合(Deps)，编排实时事件源/关联/通知/修复。
type Service struct {
	deps          *Deps
	correlator    *Correlator   // 关联层（Fingerprinter + StoreReader 就绪时启用，否则 nil 降级）
	repair        *RepairManager // 修复层（StoreRepairer 就绪时启用，否则 nil 仅通知）
	workDirGetter func() string
	emitter       func() EventEmitter // 闭包延迟读取，避开初始化时序(SetEventEmitter 之前)

	initialWorkDir string
	liveCancel     context.CancelFunc
	liveDone       chan struct{}

	stopOnce sync.Once
	stopCh   chan struct{}
}

// NewService 创建监控服务。
// workDirGetter/emitter 均为闭包延迟读取，构造期其底层依赖未就绪也无妨。
// correlator 在 Fingerprinter + StoreReader 都就绪时构造；repair 在 StoreRepairer 就绪时构造。
func NewService(deps *Deps, workDirGetter func() string, emitter func() EventEmitter) *Service {
	s := &Service{
		deps:           deps,
		workDirGetter:  workDirGetter,
		emitter:        emitter,
		initialWorkDir: workDirGetter(),
		stopCh:         make(chan struct{}),
	}
	if deps.Fingerprinter != nil && deps.StoreReader != nil {
		s.correlator = NewCorrelator(deps.Fingerprinter, deps.StoreReader, workDirGetter)
	}
	if deps.StoreRepairer != nil {
		s.repair = NewRepairManager(deps.StoreRepairer)
	}
	return s
}

// ListPendingChanges 列出待修复变更（供前端展示确认列表）
func (s *Service) ListPendingChanges() []PendingChange {
	if s == nil || s.repair == nil {
		return nil
	}
	return s.repair.ListPending()
}

// ConfirmChange 用户确认修复动作（sync/restore/ack）
func (s *Service) ConfirmChange(ctx context.Context, id int64, action RepairAction) error {
	if s == nil || s.repair == nil {
		return fmt.Errorf("修复能力不可用")
	}
	return s.repair.Confirm(ctx, id, action)
}

// Start 启动监控：离线对账(启动时一次性) + 实时事件源消费 + workDir 变更轮询。
// 实时事件源未注入(nil)时仅跳过事件消费，离线对账与 workDir 轮询仍执行。
func (s *Service) Start() {
	// 离线对账：检测软件未运行期间的变更（启动时一次性）
	if s.deps != nil && s.deps.Scanner != nil && s.correlator != nil {
		go s.runOfflineReconcile()
	}
	if s.deps != nil && s.deps.LiveSource != nil {
		s.startLive()
		logger.Log.Infof("[fsmonitor] 工作目录监控已启动: %s", s.initialWorkDir)
	} else {
		logger.Log.Warnf("[fsmonitor] 实时事件源未注入，运行时监控不可用")
	}
	go s.watchWorkDir()
}

// runOfflineReconcile 离线对账：Scan 产 DiffSet → 指纹配对关联 → 入队待修复 + 通知
func (s *Service) runOfflineReconcile() {
	ctx := context.Background()
	diff, err := s.deps.Scanner.Scan(ctx)
	if err != nil {
		logger.Log.Warnf("[fsmonitor] 离线对账失败: %v", err)
		return
	}
	if len(diff.Missing) == 0 && len(diff.Untracked) == 0 {
		logger.Log.Infof("[fsmonitor] 离线对账完成：无变更")
		return
	}
	// 全量查 DB 有效记录（带指纹），构建 Missing 记录的 ID→指纹映射用于配对
	dbRecords, _ := s.deps.StoreReader.ListValidComplete(ctx)
	fpByID := make(map[int64]string, len(dbRecords))
	for _, r := range dbRecords {
		fpByID[r.ID] = r.ContentFingerprint
	}
	// Untracked 现场算指纹 → 路径→指纹
	workDir := s.workDirGetter()
	untrackedFP := make(map[string]string, len(diff.Untracked))
	for _, u := range diff.Untracked {
		abs := joinWorkDir(workDir, u.FilePath)
		fp, err := s.deps.Fingerprinter.Fingerprint(ctx, abs)
		if err != nil {
			logger.Log.Warnf("[fsmonitor] 离线对账：算指纹失败 %s: %v", u.FilePath, err)
			continue
		}
		untrackedFP[u.FilePath] = fp.Digest
	}
	// 指纹配对：Missing 的 DB 指纹 == Untracked 的现场指纹 → Move
	usedUntracked := make(map[string]bool)
	for _, m := range diff.Missing {
		dbFP := fpByID[m.StoreID]
		if dbFP == "" {
			continue // Missing 记录无落库指纹，无法配对 → 留作 Delete
		}
		matched := ""
		for uPath, uFP := range untrackedFP {
			if usedUntracked[uPath] {
				continue
			}
			if uFP == dbFP {
				matched = uPath
				break
			}
		}
		if matched != "" {
			usedUntracked[matched] = true
			s.dispatchSemanticChange(&SemanticChange{
				Kind: SemanticMove, FromPath: m.FilePath, ToPath: matched, StoreID: m.StoreID, DetectedAt: now(),
			})
		} else {
			// 无配对 Untracked → 删除
			s.dispatchSemanticChange(&SemanticChange{
				Kind: SemanticDelete, FromPath: m.FilePath, StoreID: m.StoreID, DetectedAt: now(),
			})
		}
	}
	// 剩余未配对 Untracked → 外部新增
	for _, u := range diff.Untracked {
		if !usedUntracked[u.FilePath] {
			s.dispatchSemanticChange(&SemanticChange{
				Kind: SemanticUntracked, ToPath: u.FilePath, DetectedAt: now(),
			})
		}
	}
	logger.Log.Infof("[fsmonitor] 离线对账完成：Missing=%d Untracked=%d", len(diff.Missing), len(diff.Untracked))
}

// dispatchSemanticChange 派发语义变更：入队待修复 + 通知前端（离线对账与运行时共用）
func (s *Service) dispatchSemanticChange(sc *SemanticChange) {
	logger.Log.Infof("[fsmonitor] %s", formatSemanticChange(sc))
	pendingID := int64(0)
	if s.repair != nil {
		pendingID = s.repair.Enqueue(sc)
	}
	if em := s.emitter(); em != nil {
		em.Emit("fsmonitor:change", map[string]any{
			"id":       pendingID,
			"kind":     int(sc.Kind),
			"fromPath": sc.FromPath,
			"toPath":   sc.ToPath,
			"storeId":  sc.StoreID,
		})
	}
}

// now 当前毫秒时间戳（集中导入点，便于测试替换）
func now() int64 {
	return time.Now().UnixMilli()
}

// startLive 启动实时事件源与事件消费 goroutine。
func (s *Service) startLive() {
	ctx, cancel := context.WithCancel(context.Background())
	events, errs, err := s.deps.LiveSource.Start(ctx)
	if err != nil {
		logger.Log.Warnf("[fsmonitor] 启动实时事件源失败: %v", err)
		cancel()
		return
	}
	s.liveCancel = cancel
	s.liveDone = make(chan struct{})
	go func() {
		defer close(s.liveDone)
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-events:
				if !ok {
					return
				}
				s.handleFileChange(ctx, ev)
			case err, ok := <-errs:
				if !ok {
					return
				}
				logger.Log.Warnf("[fsmonitor] 实时事件源错误，运行时监控降级: %v", err)
				return
			}
		}
	}()
}

// handleFileChange 处理一个原始文件变更：经关联层产出语义变更 → 入队待修复 + 通知前端 + 日志
func (s *Service) handleFileChange(ctx context.Context, ev FileChange) {
	// 关联层未就绪（Fingerprinter/StoreReader 缺失）：仅记录原始事件
	if s.correlator == nil {
		logger.Log.Debugf("[fsmonitor] 原始事件(关联降级): %+v", ev)
		return
	}
	sc := s.correlator.Process(ctx, ev)
	if sc == nil {
		return // 无需报告（如外部无关文件删除、去重跳过）
	}
	s.dispatchSemanticChange(sc)
}

// formatSemanticChange 语义变更的可读串（含移动前后路径对比）
func formatSemanticChange(sc *SemanticChange) string {
	switch sc.Kind {
	case SemanticMove:
		return formatMove(sc.FromPath, sc.ToPath, sc.StoreID)
	case SemanticDelete:
		return formatDelete(sc.FromPath, sc.StoreID)
	case SemanticUntracked:
		return formatUntracked(sc.ToPath)
	default:
		return "未知变更"
	}
}

func formatMove(from, to string, storeID int64) string {
	return "文件移动/重命名: " + from + " → " + to + " (storeID=" + strconv.FormatInt(storeID, 10) + ")"
}
func formatDelete(path string, storeID int64) string {
	return "文件删除: " + path + " (storeID=" + strconv.FormatInt(storeID, 10) + ")"
}
func formatUntracked(path string) string {
	return "外部新增文件(未追踪): " + path
}

// watchWorkDir 周期检查 workDir 是否变更，变更则暂停监控并通知前端提示重启。
func (s *Service) watchWorkDir() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			current := s.workDirGetter()
			if current != "" && current != s.initialWorkDir {
				logger.Log.Infof("[fsmonitor] 工作目录变更 %s → %s，暂停监控等待重启", s.initialWorkDir, current)
				s.pauseForWorkDirChange()
				return
			}
		}
	}
}

// pauseForWorkDirChange 停止实时事件源并通知前端提示重启。
func (s *Service) pauseForWorkDirChange() {
	if s.liveCancel != nil {
		s.liveCancel()
		if s.liveDone != nil {
			<-s.liveDone
		}
		s.liveCancel = nil
	}
	if s.deps != nil && s.deps.LiveSource != nil {
		s.deps.LiveSource.Stop()
		s.deps.LiveSource = nil // 标记运行时监控下线
	}
	if em := s.emitter(); em != nil {
		em.Emit("fsmonitor:workdir-changed")
	}
}

// Stop 停止监控并释放资源。须在数据库关闭前调用。
func (s *Service) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
		if s.liveCancel != nil {
			s.liveCancel()
			if s.liveDone != nil {
				<-s.liveDone
			}
		}
		if s.deps != nil && s.deps.LiveSource != nil {
			s.deps.LiveSource.Stop()
		}
	})
}
