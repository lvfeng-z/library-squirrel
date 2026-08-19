package fsmonitor

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/util/fingerprint"
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
	// SemanticDirMove 目录移动/改名：目录路径前缀变更，下级所有文件 DB 路径需批量同步
	SemanticDirMove
)

// StoreRecord persistent_store 记录的精简视图（由 StoreReader 返回，避免关联层依赖 domain 实体）
type StoreRecord struct {
	ID                 int64
	FilePath           string // 相对 workDir
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

// dirScanSampleLimit 目录移动检测时采样下级文件上限，避免大目录全量算指纹的性能问题
const dirScanSampleLimit = 50

// Correlator 关联层：消费 FileChange，结合 DB 记录与内容指纹，产出语义变更。
// 依赖注入：Fingerprinter（算新文件指纹）+ StoreReader（查 DB）+ WorkDirGetter（拼绝对路径）。
// 任一为 nil 时降级（无法关联，原始事件被丢弃）。
type Correlator struct {
	fingerprinter fingerprint.Computer
	storeReader   StoreReader
	workDirGetter func() string

	// recentMoves 最近判定的 Move 的 FromPath → 时间戳，用于 Remove 去重
	// （Remove(旧) 到达时若该路径刚被判为 Move 的源，跳过 Delete 报告）
	recentMoves map[string]int64 // fromPath → detectedAt(ms)
}

// NewCorrelator 创建关联器
func NewCorrelator(fp fingerprint.Computer, sr StoreReader, workDirGetter func() string) *Correlator {
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
	case ChangeMove:
		return c.processMove(ctx, ev, now)
	default:
		return nil
	}
}

// processCreate 处理文件出现：算指纹 → 查 DB 同指纹旧记录 → 命中=Move，否则=Untracked
func (c *Correlator) processCreate(ctx context.Context, ev FileChange, now int64) *SemanticChange {
	if ev.IsDir {
		// 目录无内容指纹，改为下级扫描配对（检测目录改名）
		return c.processDirCreate(ctx, ev, now)
	}
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

// processMove 处理已配对的移动/重命名（ChangeMove）：旧路径查 DB 记录 → 命中即产 SemanticMove，
// 不依赖指纹（比 Create/Remove 指纹配对更精确）。专为 USN 离线 Move 服务（运行时 fsnotify
// 不产 ChangeMove，见 event.go/TREE 决策与约束）。
//
// 仅处理文件：目录改名由上层 usnProvider 以 ChangeCreate(IsDir) 发出、走 processDirCreate——
// 目录无 persistent_store 记录（GetByFilePathComplete 必返 nil），且 processDirCreate 已内含
// 「无追踪文件→Untracked 不入队」的噪声抑制；收到 ChangeMove(IsDir) 时防御性返回 nil。
//
// 连读重命名（X→Y→Z）的中间腿可能因 DB 路径尚未同步而查不到（DB 仍指 X，第二腿查 Y 落空）→
// 该腿丢弃，最终状态由全量对账兜底（D2）+ 修复层按 StoreID 合并冲突（§5.3）。
func (c *Correlator) processMove(ctx context.Context, ev FileChange, now int64) *SemanticChange {
	if ev.IsDir {
		return nil
	}
	old, err := c.storeReader.GetByFilePathComplete(ctx, ev.Path)
	if err != nil {
		logger.Log.Warnf("[fsmonitor] 关联层：移动按路径查记录失败: %v", err)
		return nil
	}
	if old == nil {
		// DB 无旧路径记录 → 非本库文件 rename，不报告（对账兜底）
		return nil
	}
	return &SemanticChange{
		Kind:       SemanticMove,
		FromPath:   ev.Path,
		ToPath:     ev.ToPath,
		StoreID:    old.ID,
		DetectedAt: now,
	}
}

// processDirCreate 处理目录出现：扫描新目录下级文件算指纹 → 查 DB 匹配旧记录 →
// 聚合最常见旧目录前缀 → 命中=DirMove(旧目录→新目录)，否则=Untracked(新建目录)。
// 采样下级文件上限 dirScanSampleLimit，避免大目录性能问题。
func (c *Correlator) processDirCreate(ctx context.Context, ev FileChange, now int64) *SemanticChange {
	workDir := c.workDirGetter()
	newDirAbs := joinWorkDir(workDir, ev.Path)
	prefixCount := make(map[string]int) // 旧目录前缀 → 命中文件数
	scanned := 0
	_ = filepath.Walk(newDirAbs, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if scanned >= dirScanSampleLimit {
			return filepath.SkipDir
		}
		scanned++
		fp, fpErr := c.fingerprinter.Fingerprint(ctx, path)
		if fpErr != nil {
			return nil
		}
		old, _ := c.storeReader.GetByFingerprint(ctx, fp.Digest, ev.Path+"/")
		if old != nil && old.FilePath != "" {
			// filepath.Dir 在 Windows 会把正斜杠规范成反斜杠，需 ToSlash 还原（DB file_path 是正斜杠）
			oldDir := filepath.ToSlash(filepath.Dir(old.FilePath))
			prefixCount[oldDir]++
		}
		return nil
	})
	var bestPrefix string
	bestCount := 0
	for prefix, n := range prefixCount {
		if n > bestCount {
			bestPrefix = prefix
			bestCount = n
		}
	}
	if bestCount > 0 && bestPrefix != ev.Path {
		return &SemanticChange{Kind: SemanticDirMove, FromPath: bestPrefix, ToPath: ev.Path, DetectedAt: now}
	}
	return &SemanticChange{Kind: SemanticUntracked, ToPath: ev.Path, DetectedAt: now}
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
