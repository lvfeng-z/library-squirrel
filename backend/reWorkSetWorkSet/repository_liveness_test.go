package reWorkSetWorkSet

import (
	"context"
	"database/sql"
	"testing"

	domain "github.com/library-squirrel/backend/base/model/entity"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newTraversalEnv 内存库（work_set + re_work_set_work_set）+ 真实仓储
func newTraversalEnv(t *testing.T) (*ReWorkSetWorkSetRepository, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	if err := db.AutoMigrate(&domain.WorkSet{}, &domain.ReWorkSetWorkSet{}); err != nil {
		t.Fatalf("迁移测试实体失败: %v", err)
	}
	return NewRepository(db), db
}

// insertWorkSetRow 插入作品集行并返回 ID
func insertWorkSetRow(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	ws := domain.NewWorkSet()
	if err := db.Create(ws).Error; err != nil {
		t.Fatalf("插作品集失败: %v", err)
	}
	return ws.GetID()
}

// addDagEdge 建 parent→child 边
func addDagEdge(t *testing.T, db *gorm.DB, parent, child int64) {
	t.Helper()
	rel := domain.NewReWorkSetWorkSet()
	rel.ParentWorkSetID = sql.NullInt64{Int64: parent, Valid: true}
	rel.ChildWorkSetID = sql.NullInt64{Int64: child, Valid: true}
	if err := db.Create(rel).Error; err != nil {
		t.Fatalf("建父子关联失败: %v", err)
	}
}

// containsAll 判断 ids 是否全部包含 targets
func containsAll(ids []int64, targets ...int64) bool {
	set := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	for _, target := range targets {
		if _, ok := set[target]; !ok {
			return false
		}
	}
	return true
}

// TestDescendantTraversalFiltersDeleted 传递包含遍历剪除已删子集：
// parent→child(已删)→grandchild 的枝经 child 不可达；grandchild 经其他活父集路径仍可达
func TestDescendantTraversalFiltersDeleted(t *testing.T) {
	repo, db := newTraversalEnv(t)

	parent := insertWorkSetRow(t, db)
	child := insertWorkSetRow(t, db)
	grandchild := insertWorkSetRow(t, db)
	parent2 := insertWorkSetRow(t, db)
	addDagEdge(t, db, parent, child)
	addDagEdge(t, db, child, grandchild)
	addDagEdge(t, db, parent2, grandchild)

	// 软删 child
	if err := db.Exec(`UPDATE work_set SET deleted_at = 1700000000000 WHERE id = ?`, child).Error; err != nil {
		t.Fatalf("软删失败: %v", err)
	}

	ctx := context.Background()
	// 经 parent：child 整枝剪除（child 与 grandchild 均不可达）
	ids, err := repo.CollectDescendantWorkSetIds(ctx, parent)
	if err != nil {
		t.Fatalf("查询后代失败: %v", err)
	}
	if containsAll(ids, child) || containsAll(ids, grandchild) {
		t.Fatalf("已删子集应整枝剪除，实际后代=%v", ids)
	}
	// 经 parent2（活路径）：grandchild 仍可达
	ids, err = repo.CollectDescendantWorkSetIds(ctx, parent2)
	if err != nil {
		t.Fatalf("查询后代失败: %v", err)
	}
	if !containsAll(ids, grandchild) {
		t.Fatalf("活路径后代应可达 grandchild，实际=%v", ids)
	}
}

// TestAncestorTraversalKeepsDeleted 祖先遍历不做活性过滤（环路检测的结构完整性依据）：
// A→B(已删)→C 的链上，C 的祖先须同时含已删 B 与活 A——过滤会让经已删节点闭合的环漏检
func TestAncestorTraversalKeepsDeleted(t *testing.T) {
	repo, db := newTraversalEnv(t)

	a := insertWorkSetRow(t, db)
	b := insertWorkSetRow(t, db)
	c := insertWorkSetRow(t, db)
	addDagEdge(t, db, a, b)
	addDagEdge(t, db, b, c)

	if err := db.Exec(`UPDATE work_set SET deleted_at = 1700000000000 WHERE id = ?`, b).Error; err != nil {
		t.Fatalf("软删失败: %v", err)
	}

	ids, err := repo.CollectAncestorWorkSetIds(context.Background(), c)
	if err != nil {
		t.Fatalf("查询祖先失败: %v", err)
	}
	if !containsAll(ids, a, b) {
		t.Fatalf("祖先遍历须全量含已删节点（B）与活节点（A），实际=%v", ids)
	}
}
