package recycleBin

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/library-squirrel/backend/base/model/dto"
	entity2 "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/reWorkSetWorkSet"
	"github.com/library-squirrel/backend/reWorkWorkSet"
	"github.com/library-squirrel/backend/search"
	"github.com/library-squirrel/backend/util"
	"github.com/library-squirrel/backend/workSet"

	"gorm.io/gorm"
)

// wsTestEnv 作品集条目端到端环境：内存库 + workSet/search 真实服务 + recycleBin 服务
type wsTestEnv struct {
	svc     *Service
	workSet *workSet.Service
	db      *gorm.DB
}

// wsTransactor 真事务执行器（事务 DB 经 ctx 传递，仓储 dbFromCtx 感知）
type wsTransactor struct{ db *gorm.DB }

func (t *wsTransactor) ExecInTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	return database.WithTransactionContext(ctx, t.db, func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, database.TxKey, tx)
		return fn(txCtx)
	})
}

func newWsTestEnv(t *testing.T) *wsTestEnv {
	t.Helper()
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("内存 SQLite 不可用: %v", err)
	}
	// 站点行种子（work_set.site_id 外键防线，fixture 站侧键统一用 siteId=1）
	if err := db.Exec("INSERT OR IGNORE INTO site (id, create_time, update_time) VALUES (1, 0, 0)").Error; err != nil {
		t.Fatalf("建站点种子失败: %v", err)
	}
	workSetSvc := workSet.NewService(
		workSet.NewRepository(db),
		reWorkWorkSet.NewRepository(db),
		reWorkSetWorkSet.NewRepository(db),
		&wsTransactor{db: db}, nil, nil,
	)
	searchSvc := search.NewService(search.NewRepository(db), nil, nil, nil, nil, nil, nil, nil, nil, nil)
	return &wsTestEnv{
		svc:     NewService(nil, nil, nil, nil, nil, nil, nil, nil, nil, workSetSvc, searchSvc, nil, nil),
		workSet: workSetSvc,
		db:      db,
	}
}

// createDeletedWorkSet 建指定键作品集并软删（制造回收站条目；siteWorkSetId 空 = 本地手建集 NULL 键）
func (env *wsTestEnv) createDeletedWorkSet(t *testing.T, siteId int64, siteWorkSetId, name string) int64 {
	t.Helper()
	ws := entity2.NewWorkSet()
	if siteWorkSetId != "" {
		ws.SiteID.Valid, ws.SiteID.Int64 = true, siteId
		ws.SiteWorkSetID.Valid, ws.SiteWorkSetID.String = true, siteWorkSetId
	}
	if name != "" {
		ws.SiteWorkSetName.Valid, ws.SiteWorkSetName.String = true, name
	}
	if err := env.workSet.Save(context.Background(), ws); err != nil {
		t.Fatalf("建集失败: %v", err)
	}
	if err := env.workSet.SoftDeleteWorkSet(context.Background(), ws.GetID()); err != nil {
		t.Fatalf("软删失败: %v", err)
	}
	return ws.GetID()
}

// linkMember 挂成员关联
func (env *wsTestEnv) linkMember(t *testing.T, workId, wsId int64) {
	t.Helper()
	rel := entity2.NewReWorkWorkSet()
	rel.WorkID = sql.NullInt64{Int64: workId, Valid: true}
	rel.WorkSetID = sql.NullInt64{Int64: wsId, Valid: true}
	if err := env.db.Create(rel).Error; err != nil {
		t.Fatalf("挂成员关联失败: %v", err)
	}
}

// TestRestoreWorkSetLocalKey 本地手建集（键 NULL）无冲突直复
func TestRestoreWorkSetLocalKey(t *testing.T) {
	env := newWsTestEnv(t)
	ctx := context.Background()

	localId := env.createDeletedWorkSet(t, 0, "", "本地集")
	if _, err := env.svc.RestoreWorkSet(ctx, localId, false); err != nil {
		t.Fatalf("NULL 键复原应无冲突: %v", err)
	}
	if ws, err := env.workSet.GetById(ctx, localId); err != nil || ws == nil {
		t.Fatalf("复原后应可见: %v %v", ws, err)
	}
}

// TestRestoreWorkSetConflictAndOverwrite Valid 键撞活集：拒绝 → 覆盖（占位集转回收站、本集复原、占位集可再复原）
func TestRestoreWorkSetConflictAndOverwrite(t *testing.T) {
	env := newWsTestEnv(t)
	ctx := context.Background()

	deadId := env.createDeletedWorkSet(t, 1, "abc", "旧代")
	// 垫 2ms：三列唯一索引下同键两死行的删除时刻须互异——连续软删同毫秒会撞索引
	//（该行为已知且接受，真实操作两次删除间隔远超毫秒；测试垫时间模拟真实间隔）
	time.Sleep(2 * time.Millisecond)
	// 重新下载同键建新活集
	newWs := entity2.NewWorkSet()
	newWs.SiteID.Valid, newWs.SiteID.Int64 = true, 1
	newWs.SiteWorkSetID.Valid, newWs.SiteWorkSetID.String = true, "abc"
	newWs.SiteWorkSetName.Valid, newWs.SiteWorkSetName.String = true, "新代"
	if err := env.workSet.Save(ctx, newWs); err != nil {
		t.Fatalf("建占位集失败: %v", err)
	}

	// 拒绝分支
	if _, err := env.svc.RestoreWorkSet(ctx, deadId, false); !errors.Is(err, ErrRestoreWorkSetConflict) {
		t.Fatalf("应报复原冲突，实际: %v", err)
	}

	// 覆盖分支：占位集转回收站，本集复原
	if _, err := env.svc.RestoreWorkSet(ctx, deadId, true); err != nil {
		t.Fatalf("覆盖复原失败: %v", err)
	}
	if ws, err := env.workSet.GetById(ctx, deadId); err != nil || ws == nil {
		t.Fatalf("复原后应可见: %v %v", ws, err)
	}
	// 占位集已软删且可再复原（三列索引：同键两死行删除时刻互异不撞——再覆盖软删 deadId 与
	// 前次软删 newWs 是相邻毫秒对，垫 2ms 模拟真实间隔）
	if _, err := env.workSet.GetDeletedWorkSet(ctx, newWs.GetID()); err != nil {
		t.Fatalf("查占位集已删态失败: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	if _, err := env.svc.RestoreWorkSet(ctx, newWs.GetID(), true); err != nil {
		t.Fatalf("占位集应可再复原: %v", err)
	}
}

// TestWorkSetEntryQueryAndPurge 条目查询（基线/名称过滤/活成员数）与彻底删除级联
func TestWorkSetEntryQueryAndPurge(t *testing.T) {
	env := newWsTestEnv(t)
	ctx := context.Background()

	deadId := env.createDeletedWorkSet(t, 1, "abc", "图集一")
	// 挂两个成员：一活一死（活成员数只计活作品）
	w1 := entity2.NewWork()
	if err := env.db.Create(w1).Error; err != nil {
		t.Fatalf("建作品失败: %v", err)
	}
	w2 := entity2.NewWork()
	if err := env.db.Create(w2).Error; err != nil {
		t.Fatalf("建作品失败: %v", err)
	}
	if err := env.db.Delete(w2).Error; err != nil {
		t.Fatalf("软删作品失败: %v", err)
	}
	env.linkMember(t, w1.GetID(), deadId)
	env.linkMember(t, w2.GetID(), deadId)
	// 活集（不应入条目）
	alive := entity2.NewWorkSet()
	if err := env.workSet.Save(ctx, alive); err != nil {
		t.Fatalf("建活集失败: %v", err)
	}

	svc := search.NewService(search.NewRepository(env.db), nil, nil, nil, nil, nil, nil, nil, nil, nil)

	// 基线：仅已删集；活成员数=1（死作品不计）
	page, err := svc.QueryRecycleWorkSetPage(ctx, 1, 10, nil)
	if err != nil {
		t.Fatalf("条目查询失败: %v", err)
	}
	if len(page.Data) != 1 || page.Data[0].ID != deadId {
		t.Fatalf("条目应仅含已删集，实际 %v", page.Data)
	}
	if page.Data[0].AliveMemberCount != 1 {
		t.Fatalf("活成员数应为 1（死作品不计），实际 %d", page.Data[0].AliveMemberCount)
	}

	// 名称过滤：命中与不命中
	hitPage, err := svc.QueryRecycleWorkSetPage(ctx, 1, 10, &dto.RecycleWorkSetPageQuery{Name: "图集"})
	if err != nil || len(hitPage.Data) != 1 {
		t.Fatalf("名称过滤应命中 1 条，实际 %v err=%v", hitPage.Data, err)
	}
	missPage, err := svc.QueryRecycleWorkSetPage(ctx, 1, 10, &dto.RecycleWorkSetPageQuery{Name: "不存在"})
	if err != nil || len(missPage.Data) != 0 {
		t.Fatalf("名称过滤不应命中，实际 %v err=%v", missPage.Data, err)
	}

	// 彻底删除：级联清行与关联
	if err := env.svc.PurgeWorkSet(ctx, deadId); err != nil {
		t.Fatalf("彻底删除失败: %v", err)
	}
	var wsCnt, relCnt int64
	env.db.Raw(`SELECT COUNT(*) FROM work_set WHERE id = ?`, deadId).Scan(&wsCnt)
	env.db.Raw(`SELECT COUNT(*) FROM re_work_work_set WHERE work_set_id = ?`, deadId).Scan(&relCnt)
	if wsCnt != 0 || relCnt != 0 {
		t.Fatalf("彻底删除应级联清行，实际 集=%d 关联=%d", wsCnt, relCnt)
	}
}

// TestWorkSetTTLCleanup TTL 圈定与清理：过期清、未到期不清
func TestWorkSetTTLCleanup(t *testing.T) {
	env := newWsTestEnv(t)
	ctx := context.Background()

	expiredId := env.createDeletedWorkSet(t, 1, "expired", "")
	freshId := env.createDeletedWorkSet(t, 1, "fresh", "")
	// expired 回拨删除时间到 40 天前（超过 30 天保留期）
	if err := env.db.Exec(`UPDATE work_set SET deleted_at = ? WHERE id = ?`,
		util.GetCurrentTimestamp()-40*24*60*60*1000, expiredId).Error; err != nil {
		t.Fatalf("回拨删除时间失败: %v", err)
	}

	before, err := env.workSet.ListDeletedBefore(ctx, util.GetCurrentTimestamp()-30*24*60*60*1000)
	if err != nil {
		t.Fatalf("圈定失败: %v", err)
	}
	if len(before) != 1 || before[0].GetID() != expiredId {
		t.Fatalf("圈定应仅含过期条目，实际 %v", before)
	}
	for _, ws := range before {
		if err := env.svc.PurgeWorkSet(ctx, ws.GetID()); err != nil {
			t.Fatalf("清理失败: %v", err)
		}
	}
	var expiredCnt, freshCnt int64
	env.db.Raw(`SELECT COUNT(*) FROM work_set WHERE id = ?`, expiredId).Scan(&expiredCnt)
	env.db.Raw(`SELECT COUNT(*) FROM work_set WHERE id = ?`, freshId).Scan(&freshCnt)
	if expiredCnt != 0 || freshCnt != 1 {
		t.Fatalf("过期应清、未到期应留，实际 expired=%d fresh=%d", expiredCnt, freshCnt)
	}
}
