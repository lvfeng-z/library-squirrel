package migration

import (
	"testing"

	"gorm.io/gorm"
)

// TestTaskPidNamedIndexCreated task(pid) 命名索引（大选择集创建链加速，乙5）：完整迁移管线
// 后 sqlite_master 在册（AutoMigrate 不管二级索引，索引经命名迁移手写建，见 migrate.go 命名
// 迁移段）；全新库经外键表重建舞步后索引仍存续（舞步按 sqlite_master 现册索引逐一复原）；
// 二次运行幂等。
func TestTaskPidNamedIndexCreated(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	assertTaskPidIndex(t, db)

	// 二次迁移幂等（IF NOT EXISTS）
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("二次迁移应幂等，实际失败: %v", err)
	}
	assertTaskPidIndex(t, db)
}

// assertTaskPidIndex 断言 sqlite_master 在册 task 表的 idx_task_pid 索引且键列恰为 pid。
func assertTaskPidIndex(t *testing.T, db *gorm.DB) {
	t.Helper()
	var cnt int
	if err := db.Raw(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND tbl_name = 'task' AND name = 'idx_task_pid'",
	).Scan(&cnt).Error; err != nil {
		t.Fatalf("查 sqlite_master 失败: %v", err)
	}
	if cnt != 1 {
		t.Fatalf("期望 sqlite_master 在册 task 表 idx_task_pid 索引，实际 %d 行", cnt)
	}
	var keyCols []string
	if err := db.Raw(
		"SELECT name FROM pragma_index_info('idx_task_pid') ORDER BY seqno",
	).Scan(&keyCols).Error; err != nil {
		t.Fatalf("查 idx_task_pid 键列失败: %v", err)
	}
	if len(keyCols) != 1 || keyCols[0] != "pid" {
		t.Fatalf("期望 idx_task_pid 键列恰为 [pid]，得到 %v", keyCols)
	}
}
