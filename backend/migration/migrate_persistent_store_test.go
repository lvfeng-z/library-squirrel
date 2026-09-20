package migration

// persistent_store 哈希双列迁移测试：AutoMigrate 加列零回填——存量行两列 NULL
// （NULL=历史未校验语义），新行可写值，二次启动幂等。

import (
	"database/sql"
	"testing"

	"github.com/library-squirrel/backend/base/model/entity"
)

// TestPersistentStoreHashColumnsMigration 模拟存量库升级：先落一行（旧形态无哈希列），
// 摘除两列后重跑迁移——列恢复、存量行保持 NULL（零回填）、新行写值落库、再次迁移幂等
func TestPersistentStoreHashColumnsMigration(t *testing.T) {
	db, err := OpenTestDB()
	if err != nil {
		t.Skipf("测试库不可用: %v", err)
	}

	// 存量行（先于哈希列存在的旧形态）
	legacy := entity.NewPersistentStore()
	legacy.FilePath = sql.NullString{String: "store/work/author/legacy.mp4", Valid: true}
	if err := db.Create(legacy).Error; err != nil {
		t.Fatalf("插存量行失败: %v", err)
	}

	// 模拟旧库形态：摘除两列（升级前数据库无这两列）
	for _, col := range []string{"expected_sha256", "actual_sha256"} {
		if err := db.Exec("ALTER TABLE persistent_store DROP COLUMN " + col).Error; err != nil {
			t.Fatalf("摘除列 %s 失败: %v", col, err)
		}
	}

	// 升级启动：AutoMigrate 补列
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("迁移补列失败: %v", err)
	}
	for _, col := range []string{"expected_sha256", "actual_sha256"} {
		var colCnt int
		db.Raw("SELECT COUNT(*) FROM pragma_table_info('persistent_store') WHERE name = ?", col).Scan(&colCnt)
		if colCnt != 1 {
			t.Fatalf("迁移后应存在列 %s, pragma 命中 %d", col, colCnt)
		}
	}

	// 零回填：存量行两列保持 NULL（NULL=历史未校验，不伪造校验状态）
	var expNull, actNull sql.NullString
	db.Raw("SELECT expected_sha256, actual_sha256 FROM persistent_store WHERE id = ?", legacy.ID).Row().Scan(&expNull, &actNull)
	if expNull.Valid || actNull.Valid {
		t.Fatalf("存量行两列应保持 NULL（零回填）, 实际 expected=%v actual=%v", expNull, actNull)
	}

	// 新行写值落库（声明哈希与实测哈希各一）
	fresh := entity.NewPersistentStore()
	fresh.FilePath = sql.NullString{String: "store/work/author/fresh.mp4", Valid: true}
	fresh.ExpectedSha256 = sql.NullString{String: "aa11", Valid: true}
	fresh.ActualSha256 = sql.NullString{String: "bb22", Valid: true}
	if err := db.Create(fresh).Error; err != nil {
		t.Fatalf("插新行失败: %v", err)
	}
	var expGot, actGot sql.NullString
	db.Raw("SELECT expected_sha256, actual_sha256 FROM persistent_store WHERE id = ?", fresh.ID).Row().Scan(&expGot, &actGot)
	if !expGot.Valid || expGot.String != "aa11" || !actGot.Valid || actGot.String != "bb22" {
		t.Fatalf("新行哈希双列应落库, 实际 expected=%v actual=%v", expGot, actGot)
	}

	// 二次启动幂等：列与数据均不受影响
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("二次迁移失败: %v", err)
	}
	db.Raw("SELECT expected_sha256 FROM persistent_store WHERE id = ?", legacy.ID).Row().Scan(&expNull)
	db.Raw("SELECT expected_sha256 FROM persistent_store WHERE id = ?", fresh.ID).Row().Scan(&expGot)
	if expNull.Valid || !expGot.Valid || expGot.String != "aa11" {
		t.Fatalf("二次迁移不应改动数据, 存量行=%v 新行=%v", expNull, expGot)
	}
}
