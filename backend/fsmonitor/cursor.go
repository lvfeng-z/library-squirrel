package fsmonitor

import "context"

// Cursor USN 续读游标（卷 journal 实例 + workDir 绑定的续读起点）。
// 跨重启持久化：软件退出时保存 StartUsn，重启后续读无遗漏无重复。
// 平台无关 DTO，供 usnProvider 消费，隔离底层实体（fsmonitor 经接口访问 DB）。
type Cursor struct {
	// JournalID UsnJournalID，标识卷 journal 实例（卷格式化后变化 → 游标失效重建）
	JournalID uint64
	// StartUsn 下次续读起点
	StartUsn int64
	// WorkDir 绑定的 workDir 绝对路径
	WorkDir string
}

// CursorStore USN 游标持久化能力（由 repository 实现，app.go 注入；nil = 无持久化，每次全量对账）。
// 续读成功后由上层（usnProvider/编排）落 Save；游标读写经 repository 自动支持调用方事务（D6）。
type CursorStore interface {
	// Get 读取 (journalID, workDir) 绑定的游标；无记录返回 nil。
	Get(ctx context.Context, journalID uint64, workDir string) (*Cursor, error)
	// Save upsert 游标：(journalID, workDir) 已存在则更新 start_usn，否则新建。
	Save(ctx context.Context, c Cursor) error
}
