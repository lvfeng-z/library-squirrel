package workSet

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/library-squirrel/backend/base/model"
	dto2 "github.com/library-squirrel/backend/base/model/dto"
	entity2 "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/reWorkSetWorkSet"
	"github.com/library-squirrel/backend/reWorkWorkSet"

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

// lifecycleWorkReader 真库作品读取器（GORM 管线软删过滤天然生效；封面链消费）
type lifecycleWorkReader struct{ db *gorm.DB }

func (r *lifecycleWorkReader) ListByIds(ctx context.Context, ids []int64) ([]*entity2.Work, error) {
	var works []*entity2.Work
	err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&works).Error
	return works, err
}

// newLifecycleService 内存库 + 真仓储事务组装 Service（fullWorkReader 不涉传 nil；workReader 真库）
func newLifecycleService(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("内存 SQLite 不可用: %v", err)
	}
	// 站点行种子（work_set.site_id 外键防线，fixture 统一用 siteId=1）
	if err := db.Exec("INSERT OR IGNORE INTO site (id, create_time, update_time) VALUES (1, 0, 0)").Error; err != nil {
		t.Fatalf("建站点种子失败: %v", err)
	}
	svc := NewService(
		NewRepository(db),
		reWorkWorkSet.NewRepository(db),
		reWorkSetWorkSet.NewRepository(db),
		&lifecycleTransactor{db: db},
		nil,
		&lifecycleWorkReader{db: db},
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

// insertWorkRow 插入作品行并返回 ID（re_work_work_set 外键的父行种子）
func insertWorkRow(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	w := entity2.NewWork()
	if err := db.Create(w).Error; err != nil {
		t.Fatalf("建作品行失败: %v", err)
	}
	return w.GetID()
}

// TestSoftDeleteWorkSetKeepsRelations 软删：行打时间戳、GORM 查询不可见、两关联表行原地保留
func TestSoftDeleteWorkSetKeepsRelations(t *testing.T) {
	svc, db := newLifecycleService(t)
	ctx := context.Background()

	wsId := insertWorkAndGetId(t, svc, 1, "abc")
	parentId := insertWorkAndGetId(t, svc, 1, "parent")

	// 挂成员关联 + DAG 父子关联（成员关联的 work_id 须指向真实作品行——外键防线）
	rel := entity2.NewReWorkWorkSet()
	rel.WorkID = sql.NullInt64{Int64: insertWorkRow(t, db), Valid: true}
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

// TestSetCoverTransitiveMember 封面可指向传递包含内任意作品（含子集作品）；
// 非传递包含内的作品拒绝；移除仍为直接成员专属
func TestSetCoverTransitiveMember(t *testing.T) {
	svc, db := newLifecycleService(t)
	ctx := context.Background()

	parentId := insertWorkAndGetId(t, svc, 1, "parent")
	childId := insertWorkAndGetId(t, svc, 1, "child")
	if err := svc.AddChildWorkSet(ctx, parentId, childId); err != nil {
		t.Fatalf("建父子关系失败: %v", err)
	}
	// 作品仅直接属于 child（子集成员）
	childWork := entity2.NewWork()
	if err := db.Create(childWork).Error; err != nil {
		t.Fatalf("建作品失败: %v", err)
	}
	rel := entity2.NewReWorkWorkSet()
	rel.WorkID = sql.NullInt64{Int64: childWork.GetID(), Valid: true}
	rel.WorkSetID = sql.NullInt64{Int64: childId, Valid: true}
	if err := db.Create(rel).Error; err != nil {
		t.Fatalf("挂成员失败: %v", err)
	}

	// 子集作品设 parent 封面：成功（封面是集级引用，不依赖直接关联行）
	if err := svc.SetCoverWork(ctx, parentId, childWork.GetID()); err != nil {
		t.Fatalf("子集作品设封面应成功: %v", err)
	}
	coverId, err := svc.GetCoverWorkId(ctx, parentId)
	if err != nil || coverId != childWork.GetID() {
		t.Fatalf("封面应查回子集作品 %d，实际 %d err=%v", childWork.GetID(), coverId, err)
	}
	// 再设直接成员为封面：覆盖旧引用（集级单值）
	directWork := entity2.NewWork()
	if err := db.Create(directWork).Error; err != nil {
		t.Fatalf("建作品失败: %v", err)
	}
	relDirect := entity2.NewReWorkWorkSet()
	relDirect.WorkID = sql.NullInt64{Int64: directWork.GetID(), Valid: true}
	relDirect.WorkSetID = sql.NullInt64{Int64: parentId, Valid: true}
	if err := db.Create(relDirect).Error; err != nil {
		t.Fatalf("挂直接成员失败: %v", err)
	}
	if err := svc.SetCoverWork(ctx, parentId, directWork.GetID()); err != nil {
		t.Fatalf("直接成员设封面失败: %v", err)
	}
	if coverId, _ = svc.GetCoverWorkId(ctx, parentId); coverId != directWork.GetID() {
		t.Fatalf("后设封面应覆盖，实际 %d", coverId)
	}

	// 非传递包含内的作品：拒绝
	outsider := entity2.NewWork()
	if err := db.Create(outsider).Error; err != nil {
		t.Fatalf("建作品失败: %v", err)
	}
	if err := svc.SetCoverWork(ctx, parentId, outsider.GetID()); !errors.Is(err, ErrWorkNotInWorkSet) {
		t.Fatalf("非成员设封面应报 ErrWorkNotInWorkSet，实际: %v", err)
	}

	// 子集作品从 parent 移除：仍拒绝（移除是直接成员专属）
	if err := svc.RemoveBatchFromWorkSet(ctx, parentId, []int64{childWork.GetID()}); !errors.Is(err, ErrWorkNotInWorkSet) {
		t.Fatalf("子集作品移除应报 ErrWorkNotInWorkSet，实际: %v", err)
	}
	// 直接成员移除：成功
	if err := svc.RemoveBatchFromWorkSet(ctx, parentId, []int64{directWork.GetID()}); err != nil {
		t.Fatalf("直接成员移除失败: %v", err)
	}
}

// TestCoverNoFallbackToMember 封面无兜底转投（兜底路径已随外键化退役）：显式封面指向软删作品时
// 封面落空（不转投首个活成员）；软删期结束（复原）后封面恢复显示。
// purge 链首步清封面引用后悬空引用不复存在，未设置封面的集也不再默认取首个成员
func TestCoverNoFallbackToMember(t *testing.T) {
	svc, db := newLifecycleService(t)
	ctx := context.Background()

	wsId := insertWorkAndGetId(t, svc, 1, "abc")
	newWork := func(sort int64) int64 {
		w := entity2.NewWork()
		if err := db.Create(w).Error; err != nil {
			t.Fatalf("建作品失败: %v", err)
		}
		rel := entity2.NewReWorkWorkSet()
		rel.WorkID = sql.NullInt64{Int64: w.GetID(), Valid: true}
		rel.WorkSetID = sql.NullInt64{Int64: wsId, Valid: true}
		rel.SortOrder = sql.NullInt64{Int64: sort, Valid: true}
		if err := db.Create(rel).Error; err != nil {
			t.Fatalf("挂成员失败: %v", err)
		}
		return w.GetID()
	}
	first, second := newWork(1), newWork(2)

	// 封面设 first 后软删它：封面落空，不转投 second
	if err := svc.SetCoverWork(ctx, wsId, first); err != nil {
		t.Fatalf("设封面失败: %v", err)
	}
	if err := db.Exec(`UPDATE work SET deleted_at = 1700000000000 WHERE id = ?`, first).Error; err != nil {
		t.Fatalf("软删作品失败: %v", err)
	}
	page, err := svc.QueryPageWithCover(ctx, &model.Page[dto2.WorkSetWithCoverDTO]{PageNumber: 1, PageSize: 10}, WorkSetQueryDTO{})
	if err != nil {
		t.Fatalf("分页查询失败: %v", err)
	}
	if len(page.Data) != 1 {
		t.Fatalf("应有 1 个集，实际 %d", len(page.Data))
	}
	if page.Data[0].CoverWork != nil {
		t.Fatalf("封面指向软删作品时应落空（无兜底转投），实际命中作品 %d", page.Data[0].CoverWork.GetId())
	}

	// 复原封面作品：封面恢复显示
	if err := db.Exec(`UPDATE work SET deleted_at = 0 WHERE id = ?`, first).Error; err != nil {
		t.Fatalf("复原作品失败: %v", err)
	}
	page, err = svc.QueryPageWithCover(ctx, &model.Page[dto2.WorkSetWithCoverDTO]{PageNumber: 1, PageSize: 10}, WorkSetQueryDTO{})
	if err != nil {
		t.Fatalf("复原后分页查询失败: %v", err)
	}
	if page.Data[0].CoverWork == nil || page.Data[0].CoverWork.GetId() != first {
		t.Fatalf("复原后封面应恢复为 %d，实际 %v", first, page.Data[0].CoverWork)
	}
	_ = second
}

// TestDeleteWorkSetAndAssociations 彻底删除：软删后级联物理删行与两关联行，全表归零
func TestDeleteWorkSetAndAssociations(t *testing.T) {
	svc, db := newLifecycleService(t)
	ctx := context.Background()

	wsId := insertWorkAndGetId(t, svc, 1, "abc")
	childSetId := insertWorkAndGetId(t, svc, 1, "child")
	rel := entity2.NewReWorkWorkSet()
	rel.WorkID = sql.NullInt64{Int64: insertWorkRow(t, db), Valid: true}
	rel.WorkSetID = sql.NullInt64{Int64: wsId, Valid: true}
	if err := db.Create(rel).Error; err != nil {
		t.Fatalf("建成员关联失败: %v", err)
	}
	dag := entity2.NewReWorkSetWorkSet()
	dag.ParentWorkSetID = sql.NullInt64{Int64: wsId, Valid: true}
	dag.ChildWorkSetID = sql.NullInt64{Int64: childSetId, Valid: true}
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
