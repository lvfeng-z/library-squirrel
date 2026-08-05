package work

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model"
	entity2 "github.com/library-squirrel/backend/base/model/entity"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
	"go.uber.org/zap"
)

// TestMain 为 work 包测试初始化 logger.Log（nop，无文件副作用）——
// applyWorkSetRelations 的环路跳过分支会记 Warnf，未初始化的 logger.Log 会 nil panic
func TestMain(m *testing.M) {
	logger.Log = zap.NewNop().Sugar()
	os.Exit(m.Run())
}

// 本文件单测 applyWorkSetRelations 编排逻辑（环路跳过 / 幂等建关系 / site_sort_order 映射 / 父集 upsert 调用）。
// 环境无 CGO 无法跑内存 SQLite，故用 fake 接口隔离 DB；SQL 构造由 reWorkSetWorkSet 包的 buildParentCaseExpression 单测覆盖。

// fakeTransactor 直接执行 fn（不开真实事务）
type fakeTransactor struct{}

func (fakeTransactor) ExecInTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

// fakeUpdatedOrder 记录一次 UpdateSiteSortOrdersForChild 调用
type fakeUpdatedOrder struct {
	childWorkSetId int64
	parentOrders   map[int64]int
}

// fakeWorkSetRelationWriter 记录 SaveRelation / UpdateSiteSortOrdersForChild；ancestors 预设控环路
type fakeWorkSetRelationWriter struct {
	ancestors     map[int64][]int64
	savedRels     []*entity2.ReWorkSetWorkSet
	updatedOrders []fakeUpdatedOrder
}

func (f *fakeWorkSetRelationWriter) CollectAncestorWorkSetIds(_ context.Context, workSetId int64) ([]int64, error) {
	return f.ancestors[workSetId], nil
}

func (f *fakeWorkSetRelationWriter) SaveRelation(_ context.Context, rel *entity2.ReWorkSetWorkSet) error {
	f.savedRels = append(f.savedRels, rel)
	return nil
}

func (f *fakeWorkSetRelationWriter) UpdateSiteSortOrdersForChild(_ context.Context, childWorkSetId int64, parentOrders map[int64]int) error {
	cp := make(map[int64]int, len(parentOrders))
	for k, v := range parentOrders {
		cp[k] = v
	}
	f.updatedOrders = append(f.updatedOrders, fakeUpdatedOrder{childWorkSetId: childWorkSetId, parentOrders: cp})
	return nil
}

// fakeWorkSetWriter 实现 WorkSetWriter；idMap 控制 upsert 回查返回的 DB ID
type fakeWorkSetWriter struct {
	idMap map[string]int64
}

func (f *fakeWorkSetWriter) BatchUpsert(_ context.Context, _ []*entity2.WorkSet) error { return nil }

func (f *fakeWorkSetWriter) ListBySiteAndSiteWorkSetIDs(_ context.Context, _ int64, siteWorkSetIds []string) ([]*entity2.WorkSet, error) {
	res := make([]*entity2.WorkSet, 0, len(siteWorkSetIds))
	for _, swsid := range siteWorkSetIds {
		id, ok := f.idMap[swsid]
		if !ok {
			continue
		}
		res = append(res, &entity2.WorkSet{
			BaseEntity:    &model.BaseEntity{ID: id},
			SiteWorkSetID: sql.NullString{String: swsid, Valid: true},
		})
	}
	return res, nil
}

func (f *fakeWorkSetWriter) SaveOrUpdateByCompositeKey(_ context.Context, _ *entity2.WorkSet) (int64, error) {
	return 0, nil
}

func (f *fakeWorkSetWriter) GetBySiteAndSiteWorkSetID(_ context.Context, _ int64, _ string) (*entity2.WorkSet, error) {
	return nil, nil
}

// relByParent 把 savedRels 按 parentWorkSetId 索引（断言用）
func relByParent(rels []*entity2.ReWorkSetWorkSet) map[int64]*entity2.ReWorkSetWorkSet {
	m := make(map[int64]*entity2.ReWorkSetWorkSet, len(rels))
	for _, r := range rels {
		m[r.ParentWorkSetID.Int64] = r
	}
	return m
}

func TestApplyWorkSetRelations(t *testing.T) {
	t.Run("正常建立多父集并写site_sort_order", func(t *testing.T) {
		wsWriter := &fakeWorkSetWriter{idMap: map[string]int64{"P1": 100, "P2": 200}}
		relWriter := &fakeWorkSetRelationWriter{ancestors: map[int64][]int64{}} // 无祖先→不成环
		s := &Service{workSetWriter: wsWriter, transactor: fakeTransactor{}, workSetRelationWriter: relWriter}

		parents := []*sdkdto.WorkSetRelationEntry{
			{ParentSiteWorkSetId: "P1", ParentWorkSetName: "父集1", SortOrder: 0},
			{ParentSiteWorkSetId: "P2", ParentWorkSetName: "父集2", SortOrder: 5},
		}
		if err := s.applyWorkSetRelations(context.Background(), 1, 50, parents); err != nil {
			t.Fatalf("applyWorkSetRelations 失败: %v", err)
		}
		if len(relWriter.savedRels) != 2 {
			t.Fatalf("建立关系数 = %d, want 2", len(relWriter.savedRels))
		}
		bp := relByParent(relWriter.savedRels)
		if r := bp[100]; r == nil || r.ChildWorkSetID.Int64 != 50 || !r.SortOrder.Valid || r.SortOrder.Int64 != 0 {
			t.Errorf("P1→child 关系错误(初始本地序应取原站序 0): %+v", r)
		}
		if r := bp[200]; r == nil || r.ChildWorkSetID.Int64 != 50 || !r.SortOrder.Valid || r.SortOrder.Int64 != 5 {
			t.Errorf("P2→child 关系错误(初始本地序应取原站序 5): %+v", r)
		}
		if len(relWriter.updatedOrders) != 1 {
			t.Fatalf("UpdateSiteSortOrdersForChild 调用数 = %d, want 1", len(relWriter.updatedOrders))
		}
		upd := relWriter.updatedOrders[0]
		if upd.childWorkSetId != 50 {
			t.Errorf("更新 childId = %d, want 50", upd.childWorkSetId)
		}
		if upd.parentOrders[100] != 0 || upd.parentOrders[200] != 5 || len(upd.parentOrders) != 2 {
			t.Errorf("site_sort_order 映射 = %v, want {100:0, 200:5}", upd.parentOrders)
		}
	})

	t.Run("环路父集被跳过", func(t *testing.T) {
		// child=50；P1(=100) 的祖先含 50 → 建 100→50 会成环，须跳过；P2(=200) 正常建
		wsWriter := &fakeWorkSetWriter{idMap: map[string]int64{"P1": 100, "P2": 200}}
		relWriter := &fakeWorkSetRelationWriter{ancestors: map[int64][]int64{100: {50, 30}, 200: {}}}
		s := &Service{workSetWriter: wsWriter, transactor: fakeTransactor{}, workSetRelationWriter: relWriter}

		parents := []*sdkdto.WorkSetRelationEntry{
			{ParentSiteWorkSetId: "P1", ParentWorkSetName: "成环父", SortOrder: 0},
			{ParentSiteWorkSetId: "P2", ParentWorkSetName: "正常父", SortOrder: 1},
		}
		if err := s.applyWorkSetRelations(context.Background(), 1, 50, parents); err != nil {
			t.Fatalf("applyWorkSetRelations 失败: %v", err)
		}
		if len(relWriter.savedRels) != 1 || relWriter.savedRels[0].ParentWorkSetID.Int64 != 200 {
			t.Errorf("环路跳过后应只建 P2→50，实际 savedRels = %+v", relWriter.savedRels)
		}
		if len(relWriter.updatedOrders) != 1 || len(relWriter.updatedOrders[0].parentOrders) != 1 ||
			relWriter.updatedOrders[0].parentOrders[200] != 1 {
			t.Errorf("site_sort_order 应只含 {200:1}，实际 = %+v", relWriter.updatedOrders)
		}
	})

	t.Run("空parentSiteWorkSetId被过滤", func(t *testing.T) {
		wsWriter := &fakeWorkSetWriter{idMap: map[string]int64{"P1": 100}}
		relWriter := &fakeWorkSetRelationWriter{ancestors: map[int64][]int64{}}
		s := &Service{workSetWriter: wsWriter, transactor: fakeTransactor{}, workSetRelationWriter: relWriter}

		parents := []*sdkdto.WorkSetRelationEntry{
			{ParentSiteWorkSetId: "", ParentWorkSetName: "空", SortOrder: 0},
			{ParentSiteWorkSetId: "P1", ParentWorkSetName: "有效", SortOrder: 2},
		}
		if err := s.applyWorkSetRelations(context.Background(), 1, 50, parents); err != nil {
			t.Fatalf("applyWorkSetRelations 失败: %v", err)
		}
		if len(relWriter.savedRels) != 1 || relWriter.savedRels[0].ParentWorkSetID.Int64 != 100 {
			t.Errorf("空 parentSiteWorkSetId 应过滤、只建 P1，实际 = %+v", relWriter.savedRels)
		}
	})

	t.Run("全部父集无效时零写入", func(t *testing.T) {
		wsWriter := &fakeWorkSetWriter{idMap: map[string]int64{}}
		relWriter := &fakeWorkSetRelationWriter{ancestors: map[int64][]int64{}}
		s := &Service{workSetWriter: wsWriter, transactor: fakeTransactor{}, workSetRelationWriter: relWriter}

		parents := []*sdkdto.WorkSetRelationEntry{
			{ParentSiteWorkSetId: "", SortOrder: 0},
		}
		if err := s.applyWorkSetRelations(context.Background(), 1, 50, parents); err != nil {
			t.Fatalf("applyWorkSetRelations 失败: %v", err)
		}
		if len(relWriter.savedRels) != 0 || len(relWriter.updatedOrders) != 0 {
			t.Errorf("全无效时应零写入，savedRels=%+v updatedOrders=%+v", relWriter.savedRels, relWriter.updatedOrders)
		}
	})

	t.Run("空parents列表零写入", func(t *testing.T) {
		wsWriter := &fakeWorkSetWriter{idMap: map[string]int64{}}
		relWriter := &fakeWorkSetRelationWriter{ancestors: map[int64][]int64{}}
		s := &Service{workSetWriter: wsWriter, transactor: fakeTransactor{}, workSetRelationWriter: relWriter}

		if err := s.applyWorkSetRelations(context.Background(), 1, 50, nil); err != nil {
			t.Fatalf("applyWorkSetRelations 失败: %v", err)
		}
		if len(relWriter.savedRels) != 0 || len(relWriter.updatedOrders) != 0 {
			t.Errorf("空 parents 应零写入，savedRels=%+v updatedOrders=%+v", relWriter.savedRels, relWriter.updatedOrders)
		}
	})
}
