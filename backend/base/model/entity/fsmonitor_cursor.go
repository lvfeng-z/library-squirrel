package entity

import "github.com/library-squirrel/backend/base/model"

// FsmonitorCursor USN 离线追溯的续读游标（跨重启持久化 USN Journal 续读起点）。
//
// 唯一键 (journal_id, work_dir)：卷 journal 实例（UsnJournalID，卷格式化后变化）+ workDir 绑定。
// workDir 切换或卷格式化（journal_id 变）→ 新建行，旧行自然失效（不再被命中）。
// 「最近一次成功续读时间」复用 BaseEntity.UpdateTime（语义一致，不另设 updated_at 列）。
type FsmonitorCursor struct {
	*model.BaseEntity
	// JournalID UsnJournalID，标识卷 journal 实例（卷格式化后变化 → 游标失效重建）
	JournalID uint64 `gorm:"column:journal_id;uniqueIndex:idx_fsmonitor_cursor_journal_workdir" json:"journalId"`
	// StartUsn 下次续读起点（USN 单调递增）
	StartUsn int64 `gorm:"column:start_usn" json:"startUsn"`
	// WorkDir 绑定的 workDir 绝对路径（游标与目录绑定）
	WorkDir string `gorm:"column:work_dir;uniqueIndex:idx_fsmonitor_cursor_journal_workdir" json:"workDir"`
}

func (FsmonitorCursor) TableName() string {
	return "fsmonitor_cursor"
}

// NewFsmonitorCursor 创建 USN 游标记录（工厂方法，禁止 &FsmonitorCursor{}）
func NewFsmonitorCursor() *FsmonitorCursor {
	return &FsmonitorCursor{BaseEntity: &model.BaseEntity{}}
}
