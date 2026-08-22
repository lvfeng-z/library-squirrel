package search

import (
	"context"
	"database/sql"
	"testing"

	"github.com/library-squirrel/backend/base/model/dto"
	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/migration"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newWorkSetLivenessEnv 内存库（全量迁移）+ 真实搜索仓储（QueryWorkPage 联全部作品谱系表）
func newWorkSetLivenessEnv(t *testing.T) (*SearchRepository, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	if err := migration.AutoMigrate(db); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	return NewRepository(db), db
}

// wsLivenessFixture 数据面：活集 live(挂作品 w1)、已删集 dead(挂作品 w2)、游离作品 w3
type wsLivenessFixture struct {
	liveId int64
	deadId int64
	w1     int64
	w2     int64
	w3     int64
}

func buildWsLivenessFixture(t *testing.T, db *gorm.DB) *wsLivenessFixture {
	t.Helper()
	newWorkSet := func() int64 {
		ws := domain.NewWorkSet()
		if err := db.Create(ws).Error; err != nil {
			t.Fatalf("插作品集失败: %v", err)
		}
		return ws.GetID()
	}
	newWork := func() int64 {
		w := domain.NewWork()
		w.SiteWorkName = sql.NullString{String: "作品", Valid: true}
		if err := db.Create(w).Error; err != nil {
			t.Fatalf("插作品失败: %v", err)
		}
		return w.GetID()
	}
	link := func(workId, wsId int64) {
		rel := domain.NewReWorkWorkSet()
		rel.WorkID = sql.NullInt64{Int64: workId, Valid: true}
		rel.WorkSetID = sql.NullInt64{Int64: wsId, Valid: true}
		if err := db.Create(rel).Error; err != nil {
			t.Fatalf("挂成员关联失败: %v", err)
		}
	}
	f := &wsLivenessFixture{
		liveId: newWorkSet(),
		deadId: newWorkSet(),
		w1:     newWork(),
		w2:     newWork(),
		w3:     newWork(),
	}
	link(f.w1, f.liveId)
	link(f.w2, f.deadId)
	if err := db.Exec(`UPDATE work_set SET deleted_at = 1700000000000 WHERE id = ?`, f.deadId).Error; err != nil {
		t.Fatalf("软删作品集失败: %v", err)
	}
	return f
}

// TestQueryWorkSetPageExcludesDeleted 已软删作品集不入正常作品集列表（空条件与有条件两态）
func TestQueryWorkSetPageExcludesDeleted(t *testing.T) {
	repo, db := newWorkSetLivenessEnv(t)
	f := buildWsLivenessFixture(t, db)
	ctx := context.Background()

	// 空条件：仅活集
	items, total, err := repo.QueryWorkSetPageByConditions(ctx, 1, 10, nil)
	if err != nil {
		t.Fatalf("空条件查询失败: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].GetID() != f.liveId {
		t.Fatalf("空条件应仅含活集，实际 total=%d items=%d", total, len(items))
	}

	// 有条件（作品名全匹配命中两作品，活集经 w1 命中、已删集经 w2 不应命中）
	conds := []*dto.SearchCondition{{Type: dto.WorksSiteName, Value: "", Operator: dto.Like}}
	items, total, err = repo.QueryWorkSetPageByConditions(ctx, 1, 10, conds)
	if err != nil {
		t.Fatalf("条件查询失败: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].GetID() != f.liveId {
		t.Fatalf("有条件应仅含活集，实际 total=%d items=%d", total, len(items))
	}
}

// TestWorkSetConditionFiltersDeleted 「不在作品集 X 中」条件按端点活性过滤：
// 已删集的成员不被排除（否则已删集的成员从「不在任何活集」搜索中消失）
func TestWorkSetConditionFiltersDeleted(t *testing.T) {
	repo, db := newWorkSetLivenessEnv(t)
	f := buildWsLivenessFixture(t, db)

	conds := []*dto.SearchCondition{{
		Type:     dto.WorkSet,
		Value:    float64(f.deadId),
		Operator: dto.NotEqual,
	}}
	items, _, err := repo.QueryWorkPage(context.Background(), 1, 10, conds)
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	for _, it := range items {
		if it.Work != nil && it.Work.GetId() == f.w2 {
			return // 已删集成员 w2 出现 = 活性过滤生效
		}
	}
	t.Fatal("已删作品集的成员应不被「不在集 X 中」条件排除，w2 未出现在结果")
}
