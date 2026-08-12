package fsmonitor

import (
	"context"
	"runtime"

	"github.com/library-squirrel/backend/base/logger"
)

// OfflineChangeProvider 离线变更追溯(基于持久日志)。
// 提供能力：基于操作系统持久变更日志追溯应用未运行期间发生的精确变更，
// 区别于全量对账(对账只发现"当前状态不一致"，无法区分"本次离线变的"还是"历史遗留")。
// 平台实现：Windows(USN Journal，NTFS 卷级持久变更日志)可实现；
//           Linux/macOS 无原生等价 → 不注入，退化为全量对账。
// 首版接口预留，暂不实现。
type OfflineChangeProvider interface {
	// ChangesSince 从给定游标续读变更，返回离线期间变更列表与新游标。
	// cursor 为 nil 时由实现决定起点。游标跨重启持久化由上层负责。
	ChangesSince(ctx context.Context, cursor OfflineCursor) (changes []FileChange, next OfflineCursor, err error)
}

// ReconciliationScanner 全量对账：比对工作目录实际文件与 persistent_store 记录，产出差异集。
// 首版离线主实现(三平台统一)。
type ReconciliationScanner interface {
	Scan(ctx context.Context) (DiffSet, error)
}

// ContentFingerprinter 内容指纹计算(size + 头部哈希)。
// 用途：store 落盘时算并落库；离线对账移动匹配键；运行时配对丢失兜底。
type ContentFingerprinter interface {
	Fingerprint(ctx context.Context, absPath string) (Fingerprint, error)
}

// Deps 工作目录监控所需依赖集合，各能力以指针持有，nil 表示该能力降级不可用。
type Deps struct {
	LiveSource      LiveEventSource       // 实时事件流，环境不可用时为 nil
	OfflineProvider OfflineChangeProvider // 离线追溯，仅 Windows 可用，首版为 nil
	Scanner         ReconciliationScanner // 全量对账，通用(离线对账实现就位后填充)
	Fingerprinter   ContentFingerprinter  // 内容指纹，通用
	StoreReader     StoreReader           // persistent_store 读取(按指纹/路径查记录)，关联层依赖
	StoreRepairer   StoreRepairer         // persistent_store 修复(改 file_path/置失效)，修复层依赖
}

// NewPlatformDeps 按当前操作系统构造可用依赖集合，不可用能力留 nil 并记录降级日志。
// usnEnabled 控制是否启用 USN 离线精确追溯（仅 Windows 且需管理员；开关开但非管理员则降级对账）。
func NewPlatformDeps(workDir string, usnEnabled bool) *Deps {
	d := &Deps{
		Fingerprinter: NewHeadFingerprinter(),
	}
	if src, err := NewFsnotifySource(workDir); err == nil {
		d.LiveSource = src
	} else {
		logger.Log.Warnf("[fsmonitor] 实时事件源不可用，运行时监控降级为仅启动对账: %v", err)
	}
	switch runtime.GOOS {
	case "windows":
		// USN 离线精确追溯门控（D8）：开关开启 + 管理员权限 才注入 provider
		if usnEnabled {
			if isElevated() {
				// TODO(C-4): 实现并注入 usnProvider（OfflineChangeProvider）；当前 provider 未实现，离线仍走全量对账
				logger.Log.Infof("[fsmonitor] USN 离线精确追溯已启用（管理员权限）；provider 待实现，离线暂走全量对账")
			} else {
				logger.Log.Warnf("[fsmonitor] USN 离线追溯开关已开启，但当前非管理员运行，降级为全量对账（USN 卷级读取需管理员权限）")
			}
		}
	}
	return d
}
