package fsmonitor

import (
	"context"
	"fmt"
	"sync"

	"github.com/library-squirrel/backend/util"
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
	MarkInvalid(ctx context.Context, id int64, invalidAt int64) error
}

// PendingChange 待修复的语义变更（入队后等用户确认）
type PendingChange struct {
	ID            int64           `json:"id"`
	SemanticChange                // 嵌入语义变更字段
}

// RepairManager 修复管理：维护待修复变更队列 + 执行用户确认的修复动作
type RepairManager struct {
	repairer StoreRepairer // nil 时修复不可用（仅通知，不能修）

	mu      sync.Mutex
	nextID  int64
	pending map[int64]*PendingChange
}

// NewRepairManager 创建修复管理器
func NewRepairManager(repairer StoreRepairer) *RepairManager {
	return &RepairManager{
		repairer: repairer,
		pending:  make(map[int64]*PendingChange),
	}
}

// Enqueue 入队一个语义变更，返回分配的待修复 ID（repairer 为 nil 时返回 0 表示不入队）
func (m *RepairManager) Enqueue(sc *SemanticChange) int64 {
	if m == nil || sc == nil {
		return 0
	}
	// 仅 Move/Delete 入队待修复；Untracked 首版仅提示不入队（决策2）
	if sc.Kind == SemanticUntracked {
		return 0
	}
	if m.repairer == nil {
		return 0 // 修复能力不可用，不入队（前端只收到通知）
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

// apply 对单个语义变更执行修复动作
func (m *RepairManager) apply(ctx context.Context, sc *SemanticChange, action RepairAction) error {
	switch sc.Kind {
	case SemanticMove:
		return m.applyMove(ctx, sc, action)
	case SemanticDelete:
		return m.applyDelete(ctx, sc, action)
	default:
		return fmt.Errorf("不支持的变更类型修复: %v", sc.Kind)
	}
}

// applyMove 移动修复：sync=改 file_path 到新位置；restore=文件移回旧路径
func (m *RepairManager) applyMove(ctx context.Context, sc *SemanticChange, action RepairAction) error {
	switch action {
	case ActionSync:
		// DB file_path 同步到新位置
		return m.repairer.UpdateFilePath(ctx, sc.StoreID, sc.ToPath)
	case ActionRestore:
		// 文件从新路径移回旧路径
		if mover, ok := m.repairer.(FileMover); ok {
			return mover.MoveFile(sc.ToPath, sc.FromPath)
		}
		return fmt.Errorf("复原需要文件移动能力，当前未注入")
	default:
		return fmt.Errorf("移动变更不支持动作 %s", action)
	}
}

// applyDelete 删除修复：ack=标记记录失效；restore=从备份还原（依赖 backup）
func (m *RepairManager) applyDelete(ctx context.Context, sc *SemanticChange, action RepairAction) error {
	switch action {
	case ActionAck:
		// 接受删除，标记记录失效
		return m.repairer.MarkInvalid(ctx, sc.StoreID, util.GetCurrentTimestamp())
	case ActionRestore:
		// 从备份还原（首版简化：依赖 backup，后续接入）
		return fmt.Errorf("删除复原(从备份还原)待后续阶段接入")
	default:
		return fmt.Errorf("删除变更不支持动作 %s", action)
	}
}

// FileMover 可选：文件移动能力（移动复原用，由实现注入；首版可选）
type FileMover interface {
	MoveFile(from, to string) error
}
