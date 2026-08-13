//go:build cgo

package fsmonitor

import (
	"context"
	"testing"

	domain "github.com/library-squirrel/backend/base/model/entity"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// 本文件为项目首个 DB 单测（//go:build cgo 隔离：依赖 gorm.io/driver/sqlite 的 CGO sqlite），
// 验证 USN 游标仓储的复合唯一键 (journal_id, work_dir) + upsert 语义（C-3 核心 D1）。
// 纯 Go 测试不受影响；CGO 不可用的环境自动跳过本文件。

// newTestCursorDB 建立隔离的内存 SQLite（:memory: + 单连接，每测试独立库）+ 建表，返回仓储与底层 db。
func newTestCursorDB(t *testing.T) (CursorStore, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开内存 SQLite 失败: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("获取底层 sql.DB 失败: %v", err)
	}
	sqlDB.SetMaxOpenConns(1) // :memory: 私有库，单连接保证读写一致 + 测试间隔离
	if err := db.AutoMigrate(&domain.FsmonitorCursor{}); err != nil {
		t.Fatalf("AutoMigrate 失败: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return NewCursorRepository(db), db
}

func cursorCount(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	if err := db.Model(&domain.FsmonitorCursor{}).Count(&n).Error; err != nil {
		t.Fatalf("统计游标行数失败: %v", err)
	}
	return n
}

// TestCursorStore_SaveAndGet 验证：空表 Get 返 nil；新建后命中；同键 Save 为 upsert（更新 start_usn 不新增行）。
func TestCursorStore_SaveAndGet(t *testing.T) {
	store, db := newTestCursorDB(t)
	ctx := context.Background()

	got, err := store.Get(ctx, 1, "E:/wd")
	if err != nil || got != nil {
		t.Fatalf("空表 Get 应返回 nil,nil，got %v,%v", got, err)
	}

	if err := store.Save(ctx, Cursor{JournalID: 1, StartUsn: 100, WorkDir: "E:/wd"}); err != nil {
		t.Fatalf("Save 新建失败: %v", err)
	}
	got, err = store.Get(ctx, 1, "E:/wd")
	if err != nil || got == nil {
		t.Fatalf("新建后 Get 应命中，got %v,%v", got, err)
	}
	if got.StartUsn != 100 || got.JournalID != 1 || got.WorkDir != "E:/wd" {
		t.Fatalf("游标字段错误: %+v", got)
	}
	if n := cursorCount(t, db); n != 1 {
		t.Fatalf("新建后应 1 行，got %d", n)
	}

	// upsert：同 (journalID, workDir) 更新 start_usn，不新增行
	if err := store.Save(ctx, Cursor{JournalID: 1, StartUsn: 500, WorkDir: "E:/wd"}); err != nil {
		t.Fatalf("Save upsert 失败: %v", err)
	}
	got, err = store.Get(ctx, 1, "E:/wd")
	if err != nil || got == nil || got.StartUsn != 500 {
		t.Fatalf("upsert 后 StartUsn 应 500，got %v,%v", got, err)
	}
	if n := cursorCount(t, db); n != 1 {
		t.Fatalf("upsert 后仍应 1 行（复合唯一键阻止重复），got %d", n)
	}
}

// TestCursorStore_DistinctKeys 验证：不同 journalID 或 workDir 各占独立行，互不覆盖。
func TestCursorStore_DistinctKeys(t *testing.T) {
	store, db := newTestCursorDB(t)
	ctx := context.Background()
	cases := []Cursor{
		{JournalID: 1, StartUsn: 10, WorkDir: "E:/wd"},
		{JournalID: 1, StartUsn: 20, WorkDir: "E:/other"}, // 同 journal 不同 workDir
		{JournalID: 2, StartUsn: 30, WorkDir: "E:/wd"},    // 不同 journal 同 workDir
	}
	for _, c := range cases {
		if err := store.Save(ctx, c); err != nil {
			t.Fatalf("Save 失败 %+v: %v", c, err)
		}
	}
	if n := cursorCount(t, db); n != 3 {
		t.Fatalf("三个不同键应 3 行，got %d", n)
	}
	for _, c := range cases {
		got, err := store.Get(ctx, c.JournalID, c.WorkDir)
		if err != nil || got == nil || got.StartUsn != c.StartUsn {
			t.Fatalf("键(%d,%s) 查询错：got %v,%v", c.JournalID, c.WorkDir, got, err)
		}
	}
}
