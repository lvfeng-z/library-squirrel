package fsmonitor

import (
	"context"
	"path/filepath"
	"time"

	"github.com/library-squirrel/backend/base/logger"
)

// SemanticKind 语义变更类型（关联层产出，比原始 FileChange 更接近业务）
type SemanticKind int

const (
	// SemanticMove 移动/重命名：DB 记录的旧路径 → 文件新路径（内容指纹匹配）
	SemanticMove SemanticKind = iota
	// SemanticDelete 删除：DB 记录对应的文件消失（无指纹匹配的新文件）
	SemanticDelete
	// SemanticUntracked 未追踪：磁盘出现 DB 无记录的文件（外部新增）
	SemanticUntracked
)

// StoreRecord persistent_store 记录的精简视图（由 StoreReader 返回，避免关联层依赖 domain 实体）
type StoreRecord struct {
	ID                int64
	FilePath          string // 相对 workDir
	ContentFingerprint string
}

// StoreReader persistent_store 读取能力（由 persistentStore.Service 实现，经适配器注入）
// 关联层用此按指纹/路径查 DB 记录，判定文件变更是移动还是删除
type StoreReader interface {
	// GetByFingerprint 按内容指纹查已完成记录（排除 excludePath），无命中返回 nil
	GetByFingerprint(ctx context.Context, fingerprint string, excludePath string) (*StoreRecord, error)
	// GetByFilePathComplete 按路径查已完成记录，无命中返回 nil
	GetByFilePathComplete(ctx context.Context, filePath string) (*StoreRecord, error)
	// ListValidComplete 全量查有效(invalid_at=0)且已完成(status=1)的记录，供离线对账
	ListValidComplete(ctx context.Context) ([]StoreRecord, error)
}

// SemanticChange 语义变更（关联层产出，驱动修复层）
type SemanticChange struct {
	Kind       SemanticKind
	FromPath   string // Move: DB 记录的旧路径；Delete: 消失的路径；Untracked: 空
	ToPath     string // Move: 文件新路径；Untracked: 新出现的路径；Delete: 空
	StoreID    int64  // Move/Delete: 关联的 persistent_store 记录 ID；Untracked: 0
	DetectedAt int64  // 毫秒时间戳
}

// moveDedupWindow 移动去重窗口：Remove 后该窗口内同路径的 Delete 不重复报告
// （Linux inotify rename 会发 Remove(旧)+Create(新)，Create 已配对成 Move，Remove 不应再报 Delete）
const moveDedupWindow = 5 * time.Second

// Correlator 关联层：消费 FileChange，结合 DB 记录与内容指纹，产出语义变更。
// 依赖注入：Fingerprinter（算新文件指纹）+ StoreReader（查 DB）+ WorkDirGetter（拼绝对路径）。
// 任一为 nil 时降级（无法关联，原始事件被丢弃）。
type Correlator struct {
	fingerprinter ContentFingerprinter
	storeReader   StoreReader
	workDirGetter func() string

	// recentMoves 最近判定的 Move 的 FromPath → 时间戳，用于 Remove 去重
	// （Remove(旧) 到达时若该路径刚被判为 Move 的源，跳过 Delete 报告）
	recentMoves map[string]int64 // fromPath → detectedAt(ms)
}

// NewCorrelator 创建关联器
func NewCorrelator(fp ContentFingerprinter, sr StoreReader, workDirGetter func() string) *Correlator {
	return &Correlator{
		fingerprinter: fp,
		storeReader:   sr,
		workDirGetter: workDirGetter,
		recentMoves:   make(map[string]int64),
	}
}

// Process 将一个 FileChange 关联为语义变更（返回 nil 表示无需报告：如外部无关文件删除）
func (c *Correlator) Process(ctx context.Context, ev FileChange) *SemanticChange {
	if c.fingerprinter == nil || c.storeReader == nil {
		return nil // 降级：无法关联
	}
	now := ev.DetectedAt
	c.gcRecentMoves(now)

	switch ev.Kind {
	case ChangeCreate:
		return c.processCreate(ctx, ev, now)
	case ChangeRemove:
		return c.processRemove(ctx, ev, now)
	default:
		return nil
	}
}

// processCreate 处理文件出现：算指纹 → 查 DB 同指纹旧记录 → 命中=Move，否则=Untracked
func (c *Correlator) processCreate(ctx context.Context, ev FileChange, now int64) *SemanticChange {
	absPath := joinWorkDir(c.workDirGetter(), ev.Path)
	fp, err := c.fingerprinter.Fingerprint(ctx, absPath)
	if err != nil {
		logger.Log.Warnf("[fsmonitor] 关联层：算指纹失败 %s: %v", ev.Path, err)
		return &SemanticChange{Kind: SemanticUntracked, ToPath: ev.Path, DetectedAt: now}
	}
	old, err := c.storeReader.GetByFingerprint(ctx, fp.Digest, ev.Path)
	if err != nil {
		logger.Log.Warnf("[fsmonitor] 关联层：按指纹查记录失败: %v", err)
		return &SemanticChange{Kind: SemanticUntracked, ToPath: ev.Path, DetectedAt: now}
	}
	if old == nil {
		// 无同指纹旧记录 → 外部新增文件
		return &SemanticChange{Kind: SemanticUntracked, ToPath: ev.Path, DetectedAt: now}
	}
	// 命中：旧记录的文件被移动/重命名到新路径。记录去重，供后续 Remove(旧) 跳过 Delete
	c.recentMoves[old.FilePath] = now
	return &SemanticChange{
		Kind:       SemanticMove,
		FromPath:   old.FilePath,
		ToPath:     ev.Path,
		StoreID:    old.ID,
		DetectedAt: now,
	}
}

// processRemove 处理文件消失：查 DB 该路径记录 → 命中=Delete，但跳过近期 Move 的源（去重）
func (c *Correlator) processRemove(ctx context.Context, ev FileChange, now int64) *SemanticChange {
	// 去重：若该路径近期已被判为 Move 的源（Linux rename 的 Remove(旧) 紧随 Create(新) 之后），跳过
	if _, isMoveSource := c.recentMoves[ev.Path]; isMoveSource {
		return nil
	}
	old, err := c.storeReader.GetByFilePathComplete(ctx, ev.Path)
	if err != nil {
		logger.Log.Warnf("[fsmonitor] 关联层：按路径查记录失败: %v", err)
		return nil
	}
	if old == nil {
		// DB 无该路径记录 → 外部无关文件删除，不报告
		return nil
	}
	return &SemanticChange{
		Kind:       SemanticDelete,
		FromPath:   ev.Path,
		StoreID:    old.ID,
		DetectedAt: now,
	}
}

// gcRecentMoves 清理过期的去重记录（超过 moveDedupWindow）
func (c *Correlator) gcRecentMoves(nowMs int64) {
	windowMs := int64(moveDedupWindow / time.Millisecond)
	for path, ts := range c.recentMoves {
		if nowMs-ts > windowMs {
			delete(c.recentMoves, path)
		}
	}
}

// joinWorkDir 拼接 workDir 与相对路径（相对路径为正斜杠形式，filepath.Join 处理平台分隔符）
func joinWorkDir(workDir, relPath string) string {
	return filepath.Join(workDir, filepath.FromSlash(relPath))
}
