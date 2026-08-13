package storeRegistry

import (
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// suppression 操作抑制：让 fsmonitor 区分"软件自身的 store/ 写入"与"外部文件操作"。
// 写方（persistentStore 各落盘点、fsmonitor/repair 复原）在 Create/Remove/Rename 前
// Suppress 登记路径；读方（fsmonitor handleFileChange）查 IsSuppressed 命中即丢弃事件，
// 避免内部写入被误报为外部变更。Release 走宽限期覆盖 fsnotify 异步延迟。
//
// 键为 workDir 相对正斜杠路径（与 fsmonitor FileChange.Path / DB file_path 同基准）。
// 仅作用于运行时实时事件（fsnotify LiveSource）；离线对账不经抑制查询（程序启动时无内部写入）。
//
// 事件模型前提：fsnotify 只对文件 Create/Remove 发事件（source.go default 不处理 Write），
// 故抑制只需覆盖"文件出现/消失那一瞬 + fsnotify 延迟"，不需覆盖整个写入过程。

const (
	// suppressWriteTimeout Suppress 登记的最长存活：兜底防写方崩溃或忘 Release 致永久抑制。
	suppressWriteTimeout = 30 * time.Second
	// suppressGracePeriod Release 后保留登记的宽限期：覆盖 fsnotify 异步延迟到达的事件。
	suppressGracePeriod = 3 * time.Second
)

// suppressEntry 抑制登记项（仅过期时间戳，不需要引用计数——同路径并发写罕见，过期机制兜底）
type suppressEntry struct {
	expiry int64 // 过期毫秒时间戳（UnixMilli）
}

var (
	suppressMu      sync.RWMutex
	suppressSet     = make(map[string]suppressEntry)
	suppressEnabled atomic.Bool // D7 紧急回滚开关，默认 true（init 置位）
)

func init() {
	suppressEnabled.Store(true)
}

// SetSuppressEnabled 开关抑制功能（D7：settings.fsmonitor.suppressEnabled 注入）。
// false 时 IsSuppressed 恒返回 false，退回无抑制原状态（内部写入误报回来，对账兜底）。
func SetSuppressEnabled(enabled bool) {
	suppressEnabled.Store(enabled)
}

// nowMs 当前毫秒时间戳
func nowMs() int64 { return time.Now().UnixMilli() }

// suppressKey 路径归一为抑制键：清理 ./ ../ 与反斜杠，统一正斜杠
func suppressKey(relPath string) string {
	return filepath.ToSlash(filepath.Clean(relPath))
}

// Suppress 登记路径为"软件自身操作中"，fsmonitor 命中即丢弃事件。
// 存活至 suppressWriteTimeout（兜底）；操作完成后应调 Release 提前进入宽限期。
func Suppress(relPath string) {
	key := suppressKey(relPath)
	suppressMu.Lock()
	suppressSet[key] = suppressEntry{expiry: nowMs() + suppressWriteTimeout.Milliseconds()}
	purgeExpiredLocked()
	suppressMu.Unlock()
}

// Release 宽限释放：把过期时间刷新到当前 + suppressGracePeriod。
// 不立即删除——fsnotify 事件可能尚未到达，立即删会导致漏判误报。
// 仅刷新已登记项；未登记或已过期项忽略（不续命）。
func Release(relPath string) {
	key := suppressKey(relPath)
	suppressMu.Lock()
	if _, ok := suppressSet[key]; ok {
		suppressSet[key] = suppressEntry{expiry: nowMs() + suppressGracePeriod.Milliseconds()}
	}
	purgeExpiredLocked()
	suppressMu.Unlock()
}

// IsSuppressed 查询路径是否被抑制：精确命中或其任一祖先目录在集（前缀匹配），且未过期。
// 只读查询（RLock）；过期项的清理在写操作（Suppress/Release）时顺带进行。
// suppressEnabled 为 false 时恒返回 false（D7 紧急回滚开关，由 settings 注入）。
func IsSuppressed(relPath string) bool {
	if !suppressEnabled.Load() {
		return false
	}
	key := suppressKey(relPath)
	now := nowMs()
	suppressMu.RLock()
	defer suppressMu.RUnlock()
	if e, ok := suppressSet[key]; ok && e.expiry > now {
		return true
	}
	// 逐级查祖先目录：a/b/c.jpg → a/b → a（目录登记命中下级文件）
	dir := key
	for {
		idx := strings.LastIndex(dir, "/")
		if idx < 0 {
			break
		}
		dir = dir[:idx]
		if e, ok := suppressSet[dir]; ok && e.expiry > now {
			return true
		}
	}
	// 后代匹配：key 是某登记项的祖先目录（文件登记命中其父目录的 Create 事件，
	// 覆盖 StoreStream 的 os.MkdirAll 触发的目录 Create）
	prefix := key + "/"
	for k, e := range suppressSet {
		if e.expiry > now && strings.HasPrefix(k, prefix) {
			return true
		}
	}
	return false
}

// purgeExpiredLocked 清理过期项（须持有写锁）。写操作时顺带调用，防不查询的键堆积。
func purgeExpiredLocked() {
	now := nowMs()
	for k, e := range suppressSet {
		if e.expiry <= now {
			delete(suppressSet, k)
		}
	}
}
