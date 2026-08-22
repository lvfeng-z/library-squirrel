package migration

import (
	"strings"
	"testing"

	entity2 "github.com/library-squirrel/backend/base/model/entity"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newMigratedWorkSetDB 建内存库并跑全量迁移（模拟应用启动终态）
func newMigratedWorkSetDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Skipf("内存 SQLite 不可用: %v", err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("全量迁移失败: %v", err)
	}
	return db
}

// insertWorkSet 插入指定键的活行（GORM Create 显式写 deleted_at=0）
func insertWorkSet(t *testing.T, db *gorm.DB, siteId int64, siteWorkSetId string) *entity2.WorkSet {
	t.Helper()
	ws := entity2.NewWorkSet()
	ws.SiteID.Valid, ws.SiteID.Int64 = true, siteId
	ws.SiteWorkSetID.Valid, ws.SiteWorkSetID.String = true, siteWorkSetId
	if err := db.Create(ws).Error; err != nil {
		t.Fatalf("插入作品集失败: %v", err)
	}
	return ws
}

// TestWorkSetThreeColumnIndex 迁移终态的三列唯一索引行为：活行占键、死行释放键、
// 同键同刻双删撞索引（该行为已知且接受——同毫秒双删无现实产道，失败为显式报错）
func TestWorkSetThreeColumnIndex(t *testing.T) {
	db := newMigratedWorkSetDB(t)

	// 活行占键：同键第二活行插入报 UNIQUE
	first := insertWorkSet(t, db, 1, "abc")
	dup := entity2.NewWorkSet()
	dup.SiteID.Valid, dup.SiteID.Int64 = true, 1
	dup.SiteWorkSetID.Valid, dup.SiteWorkSetID.String = true, "abc"
	err := db.Create(dup).Error
	if err == nil || !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatalf("同键活行重复插入应报 UNIQUE，实际: %v", err)
	}

	// 死行释放键：软删后同键插入新行成功，死行不复活
	if err := db.Delete(first).Error; err != nil {
		t.Fatalf("软删失败: %v", err)
	}
	insertWorkSet(t, db, 1, "abc")
	var total, alive int64
	db.Raw(`SELECT COUNT(*), SUM(CASE WHEN deleted_at = 0 THEN 1 ELSE 0 END) FROM work_set`).Row().Scan(&total, &alive)
	if total != 2 || alive != 1 {
		t.Fatalf("死行释放键后应共存 1 死 1 活，实际 total=%d alive=%d", total, alive)
	}

	// 同键同刻双删撞索引（删除时刻与既有死代相同值）
	err = db.Exec(`UPDATE work_set SET deleted_at = (SELECT MAX(deleted_at) FROM work_set WHERE deleted_at > 0) WHERE deleted_at = 0`).Error
	if err == nil || !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatalf("同键同刻双删应报 UNIQUE，实际: %v", err)
	}
}

// TestWorkSetMigrationIdempotentAndBackfill 迁移幂等（二次启动直跑无错）与
// deleted_at 存量回填（NULL × deleted_at=0 过滤不命中的坑——AutoMigrate 加列无默认值遗留 NULL，
// 每次启动的回填段把 NULL 归 0）
func TestWorkSetMigrationIdempotentAndBackfill(t *testing.T) {
	db := newMigratedWorkSetDB(t)
	insertWorkSet(t, db, 1, "abc")

	// 构造存量 NULL（模拟历史加列遗留），二次启动迁移应回填为 0
	if err := db.Exec(`UPDATE work_set SET deleted_at = NULL`).Error; err != nil {
		t.Fatalf("构造 NULL 失败: %v", err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("二次迁移失败: %v", err)
	}
	var nullCnt int64
	db.Raw(`SELECT COUNT(*) FROM work_set WHERE deleted_at IS NULL`).Scan(&nullCnt)
	if nullCnt != 0 {
		t.Fatalf("存量 NULL 应被回填为 0，剩余 %d 行", nullCnt)
	}

	// 三次启动幂等（索引存在性跳过 + 回填无操作）
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("三次迁移失败: %v", err)
	}
	var cnt int64
	db.Raw(`SELECT COUNT(*) FROM work_set WHERE deleted_at = 0`).Scan(&cnt)
	if cnt != 1 {
		t.Fatalf("迁移不应改动数据，活行应仍为 1，实际 %d", cnt)
	}
}
