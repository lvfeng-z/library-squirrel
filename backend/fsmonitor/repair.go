package fsmonitor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/storeRegistry"
)

// RepairAction 修复动作
type RepairAction string

const (
	// ActionSync 同步：接受文件现状，DB 跟随更新（移动→改 file_path 指向新位置）
	ActionSync RepairAction = "sync"
	// ActionRestore 复原：恢复原状（移动→文件移回旧路径；删除→从备份还原，依赖 backup）
	ActionRestore RepairAction = "restore"
	// ActionAck 确认：接受变更不做复原（删除→标记记录失效，不再视为孤儿）
	ActionAck RepairAction = "ack"
)

// StoreRepairer persistent_store 修复能力（由 persistentStore.Service 实现适配）
type StoreRepairer interface {
	// UpdateFilePath 更新记录 file_path（移动同步）
	UpdateFilePath(ctx context.Context, id int64, newFilePath string) error
	// MarkInvalid 置记录失效（删除确认）
	MarkInvalid(ctx context.Context, id int64) error
	// RenameDirectoryPrefix 批量替换 file_path 的目录前缀（目录改名同步：oldPrefix/→newPrefix/）
	// 返回受影响行数（下级文件数）
	RenameDirectoryPrefix(ctx context.Context, oldPrefix string, newPrefix string) (int64, error)
}

// PendingChange 待修复的语义变更（入队后等用户确认）
type PendingChange struct {
	ID             int64 `json:"id"`
	SemanticChange       // 嵌入语义变更字段
}

// RepairManager 修复管理：维护待修复变更队列 + 执行用户确认的修复动作。
// store 域与 backup 域分立修复能力，按变更所属域路由
type RepairManager struct {
	repairer         StoreRepairer    // store 域修复（nil 时该域修复不可用，仅通知）
	backupRepairer   BackupRepairer   // backup 域修复（nil 时该域不入修复队列）
	backupRefCleaner BackupRefCleaner // backup 域引用清列联动（nil 时悬空引用由治理对账兜底）
	workDirGetter    func() string    // 复原(文件移回)时拼绝对路径

	mu      sync.Mutex
	nextID  int64
	pending map[int64]*PendingChange
}

// NewRepairManager 创建修复管理器
func NewRepairManager(repairer StoreRepairer, backupRepairer BackupRepairer, backupRefCleaner BackupRefCleaner, workDirGetter func() string) *RepairManager {
	return &RepairManager{
		repairer:         repairer,
		backupRepairer:   backupRepairer,
		backupRefCleaner: backupRefCleaner,
		workDirGetter:    workDirGetter,
		pending:          make(map[int64]*PendingChange),
	}
}

// Enqueue 入队一个语义变更，返回分配的待修复 ID（该域修复能力不可用时返回 0 表示不入队）
func (m *RepairManager) Enqueue(sc *SemanticChange) int64 {
	if m == nil || sc == nil {
		return 0
	}
	// 仅 Move/Delete 入队待修复；Untracked 首版仅提示不入队（决策2）
	if sc.Kind == SemanticUntracked {
		return 0
	}
	if sc.Domain == DomainBackup {
		if m.backupRepairer == nil {
			return 0 // backup 域修复能力不可用，不入队（前端只收到通知）
		}
	} else if m.repairer == nil {
		return 0 // store 域修复能力不可用，不入队（前端只收到通知）
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	id := m.nextID
	m.pending[id] = &PendingChange{ID: id, SemanticChange: *sc}
	return id
}

// ListPending 列出所有待修复变更（供前端展示确认列表）
func (m *RepairManager) ListPending() []PendingChange {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]PendingChange, 0, len(m.pending))
	for _, pc := range m.pending {
		result = append(result, *pc)
	}
	return result
}

// Confirm 执行用户确认的修复动作，成功后从队列移除
func (m *RepairManager) Confirm(ctx context.Context, id int64, action RepairAction) error {
	if m == nil || m.repairer == nil {
		return fmt.Errorf("修复能力不可用")
	}
	m.mu.Lock()
	pc, ok := m.pending[id]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("待修复变更 %d 不存在", id)
	}

	if err := m.apply(ctx, &pc.SemanticChange, action); err != nil {
		return err
	}

	m.mu.Lock()
	delete(m.pending, id)
	m.mu.Unlock()
	return nil
}

// apply 对单个语义变更执行修复动作（按所属域路由）
func (m *RepairManager) apply(ctx context.Context, sc *SemanticChange, action RepairAction) error {
	if sc.Domain == DomainBackup {
		return m.applyBackup(ctx, sc, action)
	}
	switch sc.Kind {
	case SemanticMove:
		return m.applyMove(ctx, sc, action)
	case SemanticDelete:
		return m.applyDelete(ctx, sc, action)
	case SemanticDirMove:
		return m.applyDirMove(ctx, sc, action)
	default:
		return fmt.Errorf("不支持的变更类型修复: %v", sc.Kind)
	}
}

// applyBackup backup 域修复：Delete 的 ack=删清单行+清引用（文件已失，restore 不适用）；
// Move 的 sync=行路径跟随新位置、restore=文件移回旧路径
func (m *RepairManager) applyBackup(ctx context.Context, sc *SemanticChange, action RepairAction) error {
	switch sc.Kind {
	case SemanticDelete:
		if action != ActionAck {
			return fmt.Errorf("备份缺失变更不支持动作 %s（文件已失，无从复原）", action)
		}
		// 删行幂等（行不存在视为成功，容忍重复条目确认）
		if err := m.backupRepairer.DeleteRow(ctx, sc.BackupID); err != nil {
			return fmt.Errorf("删除缺失备份清单行 %d 失败: %w", sc.BackupID, err)
		}
		if m.backupRefCleaner != nil {
			if err := m.backupRefCleaner.ClearBackupRefs(ctx, []int64{sc.BackupID}); err != nil {
				// 清列失败不阻断确认——悬空引用由备份治理既有反向对账兜底
				logger.Log.Warnf("[fsmonitor] 清除备份 %d 的悬空引用失败（将由治理对账兜底）: %v", sc.BackupID, err)
			}
		}
		logger.Log.Infof("[fsmonitor] 备份缺失确认完成：清单行 %d 已删除（%s）", sc.BackupID, sc.FromPath)
		return nil
	case SemanticMove:
		switch action {
		case ActionSync:
			// 清单行保管路径同步到新位置
			if err := m.backupRepairer.UpdateFilePath(ctx, sc.BackupID, sc.ToPath); err != nil {
				return fmt.Errorf("备份清单行 %d 路径同步失败: %w", sc.BackupID, err)
			}
			return nil
		case ActionRestore:
			// 文件从新路径移回清单行记录的旧路径（行不动，移回后一致）；
			// 两端登记抑制，避免复原触发自身的移动事件（自反馈）
			storeRegistry.Suppress(sc.FromPath)
			storeRegistry.Suppress(sc.ToPath)
			defer storeRegistry.Release(sc.FromPath)
			defer storeRegistry.Release(sc.ToPath)
			workDir := m.workDirGetter()
			fromAbs := filepath.Join(workDir, filepath.FromSlash(sc.ToPath))
			toAbs := filepath.Join(workDir, filepath.FromSlash(sc.FromPath))
			if err := os.Rename(fromAbs, toAbs); err != nil {
				return fmt.Errorf("备份复原(文件移回)失败: %w", err)
			}
			return nil
		default:
			return fmt.Errorf("备份移动变更不支持动作 %s", action)
		}
	default:
		return fmt.Errorf("backup 域不支持变更类型修复: %v", sc.Kind)
	}
}

// applyDirMove 目录移动修复：sync=批量替换下级前缀；restore=目录移回
func (m *RepairManager) applyDirMove(ctx context.Context, sc *SemanticChange, action RepairAction) error {
	switch action {
	case ActionSync:
		n, err := m.repairer.RenameDirectoryPrefix(ctx, sc.FromPath, sc.ToPath)
		if err != nil {
			return fmt.Errorf("目录前缀批量同步失败: %w", err)
		}
		logger.Log.Infof("[fsmonitor] 目录改名同步完成: %s → %s，更新 %d 条记录", sc.FromPath, sc.ToPath, n)
		return nil
	case ActionRestore:
		// 目录从新路径移回旧路径。抑制两端（FromPath/ToPath），避免复原触发自身的移动事件
		storeRegistry.Suppress(sc.FromPath)
		storeRegistry.Suppress(sc.ToPath)
		defer storeRegistry.Release(sc.FromPath)
		defer storeRegistry.Release(sc.ToPath)
		workDir := m.workDirGetter()
		fromAbs := filepath.Join(workDir, filepath.FromSlash(sc.ToPath))
		toAbs := filepath.Join(workDir, filepath.FromSlash(sc.FromPath))
		if err := os.Rename(fromAbs, toAbs); err != nil {
			return fmt.Errorf("目录复原(移回)失败: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("目录移动变更不支持动作 %s", action)
	}
}

// applyMove 移动修复：sync=改 file_path 到新位置；restore=文件移回旧路径
func (m *RepairManager) applyMove(ctx context.Context, sc *SemanticChange, action RepairAction) error {
	switch action {
	case ActionSync:
		// DB file_path 同步到新位置
		return m.repairer.UpdateFilePath(ctx, sc.StoreID, sc.ToPath)
	case ActionRestore:
		// 复原：文件从新路径移回旧路径（DB 仍指旧路径，移回后一致，无需改 DB）
		// 抑制两端，避免复原触发自身的移动事件（自反馈）
		storeRegistry.Suppress(sc.FromPath)
		storeRegistry.Suppress(sc.ToPath)
		defer storeRegistry.Release(sc.FromPath)
		defer storeRegistry.Release(sc.ToPath)
		workDir := m.workDirGetter()
		fromAbs := filepath.Join(workDir, filepath.FromSlash(sc.ToPath))
		toAbs := filepath.Join(workDir, filepath.FromSlash(sc.FromPath))
		if err := os.Rename(fromAbs, toAbs); err != nil {
			return fmt.Errorf("复原(文件移回)失败: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("移动变更不支持动作 %s", action)
	}
}

// applyDelete 删除修复：ack=标记记录失效；restore=从备份还原（依赖 backup）
func (m *RepairManager) applyDelete(ctx context.Context, sc *SemanticChange, action RepairAction) error {
	switch action {
	case ActionAck:
		// 接受删除，标记记录失效
		return m.repairer.MarkInvalid(ctx, sc.StoreID)
	case ActionRestore:
		// 从备份还原（首版简化：依赖 backup，后续接入）
		return fmt.Errorf("删除复原(从备份还原)待后续阶段接入")
	default:
		return fmt.Errorf("删除变更不支持动作 %s", action)
	}
}

// _ 确保 context 在未来扩展（批量确认等）可用
var _ = context.TODO
