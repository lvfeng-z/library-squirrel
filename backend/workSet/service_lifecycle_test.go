package workSet

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	entity2 "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/reWorkSetWorkSet"
	"github.com/library-squirrel/backend/reWorkWorkSet"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// lifecycleTransactor 真事务执行器（事务 DB 经 ctx 传递，仓储 dbFromCtx 感知）
type lifecycleTransactor struct{ db *gorm.DB }

func (t *lifecycleTransactor) ExecInTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	return database.WithTransactionContext(ctx, t.db, func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, database.TxKey, tx)
		return fn(txCtx)
	})
}

// newLifecycleService 内存库 + 真仓储事务组装 Service（fullWorkReader/workReader 本链不涉及，传 nil）
func newLifecycleService(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Skipf("内存 SQLite 不可用: %v", err)
	}
	if err := migration.AutoMigrate(db); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	svc := NewService(
		NewRepository(db),
		reWorkWorkSet.NewRepository(db),
		reWorkSetWorkSet.NewRepository(db),
		&lifecycleTransactor{db: db},
		nil, nil,
	)
	return svc, db
}

// insertWorkAndGetId 经 service 建集并返回 ID
func insertWorkAndGetId(t *testing.T, svc *Service, siteId int64, siteWorkSetId string) int64 {
	t.Helper()
	ws := newKeyedWorkSet(siteId, siteWorkSetId, "测试集")
	if err := svc.Save(context.Background(), ws); err != nil {
		t.Fatalf("建集失败: %v", err)
	}
	return ws.GetID()
}

// TestSoftDeleteWorkSetKeepsRelations 软删：行打时间戳、GORM 查询不可见、两关联表行原地保留
func TestSoftDeleteWorkSetKeepsRelations(t *testing.T) {
	svc, db := newLifecycleService(t)
	ctx := context.Background()

	wsId := insertWorkAndGetId(t, svc, 1, "abc")
	parentId := insertWorkAndGetId(t, svc, 1, "parent")

	// 挂成员关联 + DAG 父子关联
	rel := entity2.NewReWorkWorkSet()
	rel.WorkID = sql.NullInt64{Int64: 100, Valid: true}
	rel.WorkSetID = sql.NullInt64{Int64: wsId, Valid: true}
	if err := db.Create(rel).Error; err != nil {
		t.Fatalf("建成员关联失败: %v", err)
	}
	dag := entity2.NewReWorkSetWorkSet()
	dag.ParentWorkSetID = sql.NullInt64{Int64: parentId, Valid: true}
	dag.ChildWorkSetID = sql.NullInt64{Int64: wsId, Valid: true}
	if err := db.Create(dag).Error; err != nil {
		t.Fatalf("建父子关联失败: %v", err)
	}

	if err := svc.SoftDeleteWorkSet(ctx, wsId); err != nil {
		t.Fatalf("软删失败: %v", err)
	}

	// 行打时间戳（已删态）
	var deletedAt int64
	db.Raw(`SELECT deleted_at FROM work_set WHERE id = ?`, wsId).Scan(&deletedAt)
	if deletedAt == 0 {
		t.Fatal("软删后 deleted_at 应为非 0 时间戳")
	}
	// GORM 查询不可见（软删过滤）
	if ws, err := svc.GetById(ctx, wsId); err == nil && ws != nil {
		t.Fatal("软删后 GetById 不应命中")
	}
	// 已删条目可查（复原链入口）
	deleted, err := svc.GetDeletedWorkSet(ctx, wsId)
	if err != nil || deleted == nil {
		t.Fatalf("GetDeletedWorkSet 应命中已删条目: %v %v", deleted, err)
	}
	// 两关联表行原地保留
	var relCnt, dagCnt int64
	db.Raw(`SELECT COUNT(*) FROM re_work_work_set WHERE work_set_id = ?`, wsId).Scan(&relCnt)
	db.Raw(`SELECT COUNT(*) FROM re_work_set_work_set WHERE child_work_set_id = ?`, wsId).Scan(&dagCnt)
	if relCnt != 1 || dagCnt != 1 {
		t.Fatalf("软删应保留关联行，实际 成员=%d 父子=%d", relCnt, dagCnt)
	}

	// 复原：清标志回活、GetById 可见
	if err := svc.RestoreDeletedWorkSet(ctx, wsId); err != nil {
		t.Fatalf("复原失败: %v", err)
	}
	if ws, err := svc.GetById(ctx, wsId); err != nil || ws == nil {
		t.Fatalf("复原后 GetById 应命中: %v %v", ws, err)
	}
}

// TestSoftDeleteWorkSetNotFound 不存在或已软删的 id 报 record not found（软删过滤下 First 仅命中活行）
func TestSoftDeleteWorkSetNotFound(t *testing.T) {
	svc, _ := newLifecycleService(t)
	if err := svc.SoftDeleteWorkSet(context.Background(), 999); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("应报 record not found，实际: %v", err)
	}
}

// TestSetCoverRejectsNonDirectMember 封面/移除的直接成员校验：子集作品无本集关联行，
// 对其操作应显式报错而非静默无效果
func TestSetCoverRejectsNonDirectMember(t *testing.T) {
	svc, db := newLifecycleService(t)
	ctx := context.Background()

	parentId := insertWorkAndGetId(t, svc, 1, "parent")
	childId := insertWorkAndGetId(t, svc, 1, "child")
	// 挂 child 为 parent 的子集，作品仅直接属于 child
	if err := svc.AddChildWorkSet(ctx, parentId, childId); err != nil {
		t.Fatalf("建父子关系失败: %v", err)
	}
	w := entity2.NewWork()
	if err := db.Create(w).Error; err != nil {
		t.Fatalf("建作品失败: %v", err)
	}
	rel := entity2.NewReWorkWorkSet()
	rel.WorkID = sql.NullInt64{Int64: w.GetID(), Valid: true}
	rel.WorkSetID = sql.NullInt64{Int64: childId, Valid: true}
	if err := db.Create(rel).Error; err != nil {
		t.Fatalf("挂成员失败: %v", err)
	}

	// 子集作品设 parent 封面：拒绝
	if err := svc.SetCoverWork(ctx, parentId, w.GetID()); !errors.Is(err, ErrWorkNotInWorkSet) {
		t.Fatalf("子集作品设封面应报 ErrWorkNotInWorkSet，实际: %v", err)
	}
	// 子集作品从 parent 移除：拒绝
	if err := svc.RemoveBatchFromWorkSet(ctx, parentId, []int64{w.GetID()}); !errors.Is(err, ErrWorkNotInWorkSet) {
		t.Fatalf("子集作品移除应报 ErrWorkNotInWorkSet，实际: %v", err)
	}
	// 封面标记未被污染（parent 下无 is_cover 行）
	var coverCnt int64
	db.Raw(`SELECT COUNT(*) FROM re_work_work_set WHERE work_set_id = ? AND is_cover = 1`, parentId).Scan(&coverCnt)
	if coverCnt != 0 {
		t.Fatalf("被拒绝的设封面不应残留标记，实际 %d 行", coverCnt)
	}

	// 直接成员设封面：成功且可查回
	if err := svc.LinkWorkToWorkSet(ctx, w.GetID(), parentId, false); err != nil {
		t.Fatalf("挂直接成员失败: %v", err)
	}
	if err := svc.SetCoverWork(ctx, parentId, w.GetID()); err != nil {
		t.Fatalf("直接成员设封面失败: %v", err)
	}
	coverId, err := svc.GetCoverWorkId(ctx, parentId)
	if err != nil || coverId != w.GetID() {
		t.Fatalf("封面应可查回 %d，实际 %d err=%v", w.GetID(), coverId, err)
	}
}

// TestDeleteWorkSetAndAssociations 彻底删除：软删后级联物理删行与两关联行，全表归零
func TestDeleteWorkSetAndAssociations(t *testing.T) {
	svc, db := newLifecycleService(t)
	ctx := context.Background()

	wsId := insertWorkAndGetId(t, svc, 1, "abc")
	rel := entity2.NewReWorkWorkSet()
	rel.WorkID = sql.NullInt64{Int64: 100, Valid: true}
	rel.WorkSetID = sql.NullInt64{Int64: wsId, Valid: true}
	if err := db.Create(rel).Error; err != nil {
		t.Fatalf("建成员关联失败: %v", err)
	}
	dag := entity2.NewReWorkSetWorkSet()
	dag.ParentWorkSetID = sql.NullInt64{Int64: wsId, Valid: true}
	dag.ChildWorkSetID = sql.NullInt64{Int64: 888, Valid: true}
	if err := db.Create(dag).Error; err != nil {
		t.Fatalf("建父子关联失败: %v", err)
	}

	if err := svc.SoftDeleteWorkSet(ctx, wsId); err != nil {
		t.Fatalf("软删失败: %v", err)
	}
	if err := svc.DeleteWorkSetAndAssociations(ctx, wsId); err != nil {
		t.Fatalf("级联删除失败: %v", err)
	}

	var wsCnt, relCnt, dagCnt int64
	db.Raw(`SELECT COUNT(*) FROM work_set WHERE id = ?`, wsId).Scan(&wsCnt)
	db.Raw(`SELECT COUNT(*) FROM re_work_work_set WHERE work_set_id = ?`, wsId).Scan(&relCnt)
	db.Raw(`SELECT COUNT(*) FROM re_work_set_work_set WHERE parent_work_set_id = ?`, wsId).Scan(&dagCnt)
	if wsCnt != 0 || relCnt != 0 || dagCnt != 0 {
		t.Fatalf("级联删除后应全消亡，实际 集=%d 成员=%d 父子=%d", wsCnt, relCnt, dagCnt)
	}
}
