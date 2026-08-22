package workSet

import (
	"context"
	"testing"

	entity2 "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/migration"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newUpsertTestRepo 建内存库（全量迁移终态，含三列唯一索引）并返回仓储
func newUpsertTestRepo(t *testing.T) *WorkSetRepository {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Skipf("内存 SQLite 不可用: %v", err)
	}
	if err := migration.AutoMigrate(db); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	return NewRepository(db)
}

// newKeyedWorkSet 构造指定键与名称的作品集实体
func newKeyedWorkSet(siteId int64, siteWorkSetId, name string) *entity2.WorkSet {
	ws := entity2.NewWorkSet()
	ws.SiteID.Valid, ws.SiteID.Int64 = true, siteId
	ws.SiteWorkSetID.Valid, ws.SiteWorkSetID.String = true, siteWorkSetId
	if name != "" {
		ws.SiteWorkSetName.Valid, ws.SiteWorkSetName.String = true, name
	}
	return ws
}

// TestUpsertLiveRowUpdate 键有活行：upsert 更新元数据（含 NULL 覆盖语义——AssignmentColumns 显式列覆盖）
func TestUpsertLiveRowUpdate(t *testing.T) {
	repo := newUpsertTestRepo(t)
	ctx := context.Background()

	if err := repo.Upsert(ctx, newKeyedWorkSet(1, "abc", "v1")); err != nil {
		t.Fatalf("首插失败: %v", err)
	}
	// 名称置空（SiteWorkSetName 不设值）——AssignmentColumns 语义应把已有值覆盖为 NULL
	if err := repo.Upsert(ctx, newKeyedWorkSet(1, "abc", "")); err != nil {
		t.Fatalf("活行 upsert 失败: %v", err)
	}
	var cnt int64
	var name *string
	repo.GORM().Raw(`SELECT COUNT(*), MAX(site_work_set_name) FROM work_set WHERE deleted_at = 0`).Row().Scan(&cnt, &name)
	if cnt != 1 {
		t.Fatalf("活行 upsert 应更新不新建，行数=%d", cnt)
	}
	if name != nil {
		t.Fatalf("NULL 覆盖语义失效，site_work_set_name=%v", *name)
	}
}

// TestUpsertDeadRowNotRevived 键被已删行占位：upsert 新建放行、已删行不复活
func TestUpsertDeadRowNotRevived(t *testing.T) {
	repo := newUpsertTestRepo(t)
	ctx := context.Background()

	first := newKeyedWorkSet(1, "abc", "旧代")
	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("首插失败: %v", err)
	}
	if err := repo.Delete(ctx, first.GetID()); err != nil {
		t.Fatalf("软删失败: %v", err)
	}
	if err := repo.Upsert(ctx, newKeyedWorkSet(1, "abc", "新代")); err != nil {
		t.Fatalf("死行占位时 upsert 失败: %v", err)
	}
	var total, alive int64
	var aliveName string
	repo.GORM().Raw(`SELECT COUNT(*), SUM(CASE WHEN deleted_at = 0 THEN 1 ELSE 0 END) FROM work_set`).Row().Scan(&total, &alive)
	repo.GORM().Raw(`SELECT site_work_set_name FROM work_set WHERE deleted_at = 0`).Scan(&aliveName)
	if total != 2 || alive != 1 {
		t.Fatalf("应共存 1 死 1 活，实际 total=%d alive=%d", total, alive)
	}
	if aliveName != "新代" {
		t.Fatalf("活行应为新代，实际 %q", aliveName)
	}
}

// TestBatchUpsertMixed 混合批：活行键更新、死行键新建、全新键新建
func TestBatchUpsertMixed(t *testing.T) {
	repo := newUpsertTestRepo(t)
	ctx := context.Background()

	live := newKeyedWorkSet(1, "k1", "旧名")
	if err := repo.Create(ctx, live); err != nil {
		t.Fatalf("k1 首插失败: %v", err)
	}
	dead := newKeyedWorkSet(1, "k2", "死代")
	if err := repo.Create(ctx, dead); err != nil {
		t.Fatalf("k2 首插失败: %v", err)
	}
	if err := repo.Delete(ctx, dead.GetID()); err != nil {
		t.Fatalf("k2 软删失败: %v", err)
	}

	batch := []*entity2.WorkSet{
		newKeyedWorkSet(1, "k1", "k1 新名"), // 活行键 → 更新
		newKeyedWorkSet(1, "k2", "k2 新代"), // 死行键 → 新建
		newKeyedWorkSet(1, "k3", "k3 全新"), // 全新键 → 新建
	}
	if err := repo.BatchUpsert(ctx, batch); err != nil {
		t.Fatalf("混合批 upsert 失败: %v", err)
	}

	var total, alive int64
	repo.GORM().Raw(`SELECT COUNT(*), SUM(CASE WHEN deleted_at = 0 THEN 1 ELSE 0 END) FROM work_set`).Row().Scan(&total, &alive)
	if total != 4 || alive != 3 {
		t.Fatalf("混合批后期望 total=4 alive=3，实际 total=%d alive=%d", total, alive)
	}
	var k1Name string
	repo.GORM().Raw(`SELECT site_work_set_name FROM work_set WHERE site_work_set_id = 'k1' AND deleted_at = 0`).Scan(&k1Name)
	if k1Name != "k1 新名" {
		t.Fatalf("k1 活行应更新为 k1 新名，实际 %q", k1Name)
	}
}
