package fsmonitor

import (
	"context"
	"os"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/storeRegistry"
)

// ChangeDomain 变更所属的监控域：store 域（persistent_store 资源文件）与 backup 域
// （backup 保管清单行文件）分立关联与修复策略，共用事件源/确认流/修复队列
type ChangeDomain int

const (
	// DomainStore persistent_store 资源文件域（默认零值，既有全部语义变更的所属域）
	DomainStore ChangeDomain = iota
	// DomainBackup backup 保管清单行文件域
	DomainBackup
)

// BackupRecord backup 保管清单行的精简视图（由 BackupReader 返回，避免关联层依赖 domain 实体）
type BackupRecord struct {
	ID       int64
	FilePath string // 相对 workDir 正斜杠（backup/ 子树下）
}

// BackupReader backup 保管清单读取能力（由 backup.Service 经 app 装配适配注入，nil = backup 域降级）。
// 各查询均以当前工作目录为界——工作目录迁移前的旧行不在监控树内
type BackupReader interface {
	// GetByFilePath 按保管路径精确查清单行，无命中返回 nil
	GetByFilePath(ctx context.Context, filePath string) (*BackupRecord, error)
	// ListByPathPrefix 按路径前缀查清单行（目录 Remove 圈定受影响行；含多级下级）
	ListByPathPrefix(ctx context.Context, prefix string) ([]BackupRecord, error)
	// ListAllInWorkDir 全量投影当前工作目录中保管路径有效的清单行（离线对账数据源）
	ListAllInWorkDir(ctx context.Context) ([]BackupRecord, error)
}

// BackupRepairer backup 保管清单修复能力（由 backup.Service 经 app 装配适配注入，nil = backup 域不入修复队列）
type BackupRepairer interface {
	// DeleteRow 删除清单行与磁盘文件（文件缺失容忍、行不存在幂等）
	DeleteRow(ctx context.Context, id int64) error
	// UpdateFilePath 更新清单行保管路径（移动同步：行路径跟随文件新位置）
	UpdateFilePath(ctx context.Context, id int64, newFilePath string) error
}

// BackupRefCleaner 引用清列联动（由 backupGovernance.Service 实现）：清单行删除后，
// 清除业务行（persistent_store.backup_id / plugin.BackupID 等）对该行的悬空引用，
// 令回收站可复原状态即时准确。引用方枚举知识归治理方，本接口只收 ID 集
type BackupRefCleaner interface {
	// ClearBackupRefs 按 ID 清除各引用方对该清单行的引用列
	ClearBackupRefs(ctx context.Context, backupIds []int64) error
}

// backupWatcher backup 域关联层：消费 backup 子树的 FileChange，按路径/前缀对照保管清单行，
// 产出 backup 域语义变更。与 store 域 Correlator 分立——保管清单无内容指纹列，
// 运行时不做指纹配对（外部改名降级为 Delete 报告）；仅 USN ChangeMove（携带旧→新路径配对）
// 能产出 Move。Create 事件不消费：外部文件落入 backup/ 不影响清单行真性
type backupWatcher struct {
	reader        BackupReader
	workDirGetter func() string
}

// newBackupWatcher 创建 backup 域关联器（reader 为 nil 时不构造，上层整体降级）
func newBackupWatcher(reader BackupReader, workDirGetter func() string) *backupWatcher {
	return &backupWatcher{reader: reader, workDirGetter: workDirGetter}
}

// Process 将一个 backup 子树 FileChange 关联为语义变更集合（返回空 = 无需报告）
func (w *backupWatcher) Process(ctx context.Context, ev FileChange) []*SemanticChange {
	switch ev.Kind {
	case ChangeRemove:
		return w.processRemove(ctx, ev)
	case ChangeMove:
		return w.processMove(ctx, ev)
	default:
		// ChangeCreate 不消费：无指纹配对能力，外部新增不构成清单行变更
		return nil
	}
}

// processRemove 处理 backup 子树文件/目录消失：按路径查行命中即 Delete；
// 目录消失按前缀圈行，逐行 stat 复核文件确实不在（目录可能是子树内改名，
// 行已失真但须排除窗口内瞬态）后逐行产出 Delete
func (w *backupWatcher) processRemove(ctx context.Context, ev FileChange) []*SemanticChange {
	now := ev.DetectedAt
	if ev.IsDir {
		rows, err := w.reader.ListByPathPrefix(ctx, ev.Path)
		if err != nil {
			logger.Log.Warnf("[fsmonitor] backup 域按前缀查清单行失败: %v", err)
			return nil
		}
		result := make([]*SemanticChange, 0, len(rows))
		for _, row := range rows {
			if row.FilePath == "" {
				continue
			}
			if fileExists(joinWorkDir(w.workDirGetter(), row.FilePath)) {
				continue // 文件在位（目录消失事件与文件移回并发等瞬态），该行不失真
			}
			result = append(result, &SemanticChange{
				Domain:     DomainBackup,
				Kind:       SemanticDelete,
				FromPath:   row.FilePath,
				BackupID:   row.ID,
				DetectedAt: now,
			})
		}
		return result
	}
	row, err := w.reader.GetByFilePath(ctx, ev.Path)
	if err != nil {
		logger.Log.Warnf("[fsmonitor] backup 域按路径查清单行失败: %v", err)
		return nil
	}
	if row == nil {
		// 无清单行的外部文件消失，不报告
		return nil
	}
	return []*SemanticChange{{
		Domain:     DomainBackup,
		Kind:       SemanticDelete,
		FromPath:   row.FilePath,
		BackupID:   row.ID,
		DetectedAt: now,
	}}
}

// processMove 处理已配对的移动/重命名（ChangeMove，USN 离线段产出）：旧路径查清单行，
// 命中且新路径仍在 backup 子树内即 Move（sync 可行）；新路径移出 backup 子树视为
// 文件被取走，产 Delete（保管语义已不成立，行不该跟随到外部路径）
func (w *backupWatcher) processMove(ctx context.Context, ev FileChange) []*SemanticChange {
	if ev.IsDir {
		return nil
	}
	row, err := w.reader.GetByFilePath(ctx, ev.Path)
	if err != nil {
		logger.Log.Warnf("[fsmonitor] backup 域移动按路径查清单行失败: %v", err)
		return nil
	}
	if row == nil {
		return nil
	}
	if !storeRegistry.InBackupDir(ev.ToPath) {
		return []*SemanticChange{{
			Domain:     DomainBackup,
			Kind:       SemanticDelete,
			FromPath:   row.FilePath,
			BackupID:   row.ID,
			DetectedAt: ev.DetectedAt,
		}}
	}
	return []*SemanticChange{{
		Domain:     DomainBackup,
		Kind:       SemanticMove,
		FromPath:   row.FilePath,
		ToPath:     ev.ToPath,
		BackupID:   row.ID,
		DetectedAt: ev.DetectedAt,
	}}
}

// fileExists 路径存在性检查（文件或目录皆真）
func fileExists(absPath string) bool {
	_, err := os.Stat(absPath)
	return err == nil
}
