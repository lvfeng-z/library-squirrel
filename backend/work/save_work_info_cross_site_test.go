package work

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/library-squirrel/backend/base/constant"
	entity2 "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/reWorkAuthor"
	"github.com/library-squirrel/backend/reWorkTag"
	"github.com/library-squirrel/backend/reWorkWorkSet"
	"github.com/library-squirrel/backend/shareLock"
	"github.com/library-squirrel/backend/site"
	"github.com/library-squirrel/backend/siteAuthor"
	"github.com/library-squirrel/backend/siteTag"
	"github.com/library-squirrel/backend/workSet"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"

	"gorm.io/gorm"
)

// crossSiteTestWorkSetWriter 测试内 WorkSetWriter：仓储自带 BatchUpsert/ListBySiteAndSiteWorkSetIDs/
// GetBySiteAndSiteWorkSetID（嵌入即得），复合键单条 upsert 按「查行→改/建」组合实现（与 app.go
// workSetWriterAdapter 同构）
type crossSiteTestWorkSetWriter struct {
	*workSet.WorkSetRepository
}

func (w *crossSiteTestWorkSetWriter) SaveOrUpdateByCompositeKey(ctx context.Context, ws *entity2.WorkSet) (int64, error) {
	existing, err := w.GetBySiteAndSiteWorkSetID(ctx, ws.SiteID.Int64, ws.SiteWorkSetID.String)
	if err == nil && existing != nil {
		ws.ID = existing.ID
		if err := w.Updates(ctx, ws); err != nil {
			return 0, err
		}
		return existing.ID, nil
	}
	if err := w.Create(ctx, ws); err != nil {
		return 0, err
	}
	return ws.ID, nil
}

// newCrossSiteTestEnv 内存 FK 库 + saveWorkInfoInTx 焦点件；相较 newSaveWorkInfoTestEnv 增配
// 真实 SiteReader（作品站点键解析 + 跨站键按键寻址）与作品集写读件（WorkSetWriter/
// ReWorkWorkSetWriter 真实件，作品集跨站态需要），其余依赖同前（空输入早退不触）
func newCrossSiteTestEnv(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	svc := NewService(
		NewRepository(db),
		&txTransactor{db: db},
		nil, // LocalTagReader
		nil, // LocalAuthorReader
		nil, // SiteTagReader
		nil, // SiteAuthorReader
		site.NewService(site.NewRepository(db)), // SiteReader（真实件：跨站站点键解析）
		nil,                                     // ResourceReader
		reWorkTag.NewService(reWorkTag.NewRepository(db), nil), // ReWorkTagWriter（真实件）
		reWorkWorkSet.NewRepository(db),                         // ReWorkWorkSetWriter（真实件）
		nil,                                                     // ResourceDeleter
		siteAuthor.NewService(siteAuthor.NewRepository(db), nil, nil, nil, nil), // SiteAuthorWriter（真实件）
		siteTag.NewService(siteTag.NewRepository(db), nil, nil, nil, nil, nil),  // SiteTagWriter（真实件）
		&crossSiteTestWorkSetWriter{WorkSetRepository: workSet.NewRepository(db)}, // WorkSetWriter（真实件）
		reWorkAuthor.NewService(reWorkAuthor.NewRepository(db)),                   // ReWorkAuthorWriter（真实件）
		nil, // LocalTagBatchReader
		nil, // SiteTagBatchReader
		nil, // SiteBatchReader
		nil, // LocalAuthorBatchReader
		nil, // SiteAuthorBatchReader
		nil, // ResourceBatchReader
		nil, // ResourceStoreBatchReader
		nil, // StoreBatchReader
		nil, // ReWorkTagBatchReader
		nil, // LocalTagFindOrCreator
		nil, // LocalAuthorFindOrCreator
		nil, // StoreDeleter
		nil, // RunningTaskStopper
		nil, // ResourceStoreHardDeleter
		nil, // WorkSetRelationWriter
		nil, // CoverReferenceClearer
		shareLock.NewShareLockRegistry(), // WorkLockChecker（真实件：纯内存能力，零外部依赖）
	)
	return svc, db
}

// seedSite 建站点行（测试库不跑启动投影，直接插行等价注册表投影结果；键即身份）
func seedSite(t *testing.T, db *gorm.DB, key string) *entity2.Site {
	t.Helper()
	s := entity2.NewSite()
	s.SiteKey = key
	s.SiteName = sql.NullString{String: key + "-站名", Valid: true}
	if err := db.Create(s).Error; err != nil {
		t.Fatalf("插站点行失败 key=%s: %v", key, err)
	}
	return s
}

// countRows 条件计数（空条件计全表）
func countRows(t *testing.T, db *gorm.DB, model any, cond string, args ...any) int64 {
	t.Helper()
	var n int64
	q := db.Model(model)
	if cond != "" {
		q = q.Where(cond, args...)
	}
	if err := q.Count(&n).Error; err != nil {
		t.Fatalf("计数失败 %T: %v", model, err)
	}
	return n
}

// newWorkTaskOfSite 构造指向指定站点的任务领域行入参（saveWorkInfoInTx 的站点归属来源）
func newWorkTaskOfSite(siteId int64) (*entity2.Task, *entity2.WorkTask) {
	task := entity2.NewTask()
	wt := entity2.NewWorkTask(1)
	wt.SiteID = sql.NullInt64{Int64: siteId, Valid: true}
	return task, wt
}

// TestSaveWorkInfoSiteKeyDefaultAndEqualToWorkSiteStayUpsert 站点键缺省（空）与等于作品站点键的
// 声明均走本站 upsert 轨道：行按声明内容新建、关联照常建立——等键条目若被误判为跨站会因行不存在
// 报错，本测锚定该判定不偏移（缺省路径行为零变化回归锚）
func TestSaveWorkInfoSiteKeyDefaultAndEqualToWorkSiteStayUpsert(t *testing.T) {
	svc, db := newCrossSiteTestEnv(t)
	pixiv := seedSite(t, db, "pixiv")

	siteWorkId := "same-site-work-1"
	workResp := &sdkdto.WorkResponse{
		Work: &sdkdto.WorkDTO{SiteWorkId: &siteWorkId, SiteWorkName: &siteWorkId},
		SiteAuthors: []*sdkdto.TaskSiteAuthorDTO{
			{SiteAuthorId: "px-a1", AuthorName: "缺省键作者"},
			{SiteAuthorId: "px-a2", AuthorName: "等键作者", SiteKey: "pixiv"},
		},
		SiteTags: []*sdkdto.TaskSiteTagDTO{
			{SiteTagId: "px-t1", TagName: "缺省键标签"},
			{SiteTagId: "px-t2", TagName: "等键标签", SiteKey: "pixiv"},
		},
		WorkSets: []*sdkdto.TaskWorkSetDTO{
			{SiteWorkSetId: "px-ws1", WorkSetName: "缺省键集"},
			{SiteWorkSetId: "px-ws2", WorkSetName: "等键集", SiteKey: "pixiv"},
		},
	}

	task, wt := newWorkTaskOfSite(pixiv.GetID())
	workId, err := svc.saveWorkInfoInTx(context.Background(), task, wt, workResp)
	if err != nil {
		t.Fatalf("缺省/等键声明应全走本站 upsert 成功: %v", err)
	}

	// 行建于本站（pixiv）
	if n := countRows(t, db, &entity2.SiteAuthor{}, "site_id = ?", pixiv.GetID()); n != 2 {
		t.Fatalf("本站站点作者应建 2 行，实际 %d", n)
	}
	if n := countRows(t, db, &entity2.SiteTag{}, "site_id = ?", pixiv.GetID()); n != 2 {
		t.Fatalf("本站站点标签应建 2 行，实际 %d", n)
	}
	if n := countRows(t, db, &entity2.WorkSet{}, "site_id = ?", pixiv.GetID()); n != 2 {
		t.Fatalf("本站作品集应建 2 行，实际 %d", n)
	}
	// 关联照常建立
	if n := countRows(t, db, &entity2.ReWorkAuthor{}, "work_id = ? AND author_type = ?", workId, constant.SITE); n != 2 {
		t.Fatalf("SITE 作者关联应 2 条，实际 %d", n)
	}
	if n := countRows(t, db, &entity2.ReWorkTag{}, "work_id = ? AND tag_type = ?", workId, constant.SITE); n != 2 {
		t.Fatalf("SITE 标签关联应 2 条，实际 %d", n)
	}
	if n := countRows(t, db, &entity2.ReWorkWorkSet{}, "work_id = ?", workId); n != 2 {
		t.Fatalf("作品集关联应 2 条，实际 %d", n)
	}
}

// TestSaveWorkInfoCrossSiteHitAttachesExistingRowWithoutWriting 跨站引用命中既有行：只挂联
// （SITE 关联指向既有行 DB ID），声明携带的名称等元数据不写入既有行（行字段保持原值）、
// 也不为跨站键新建行；同批本站声明照常 upsert
func TestSaveWorkInfoCrossSiteHitAttachesExistingRowWithoutWriting(t *testing.T) {
	svc, db := newCrossSiteTestEnv(t)
	pixiv := seedSite(t, db, "pixiv")
	bili := seedSite(t, db, "bilibili")

	// 跨站目标既有行（字段为「不应被覆盖」哨兵值）
	biliAuthor := entity2.NewSiteAuthor()
	biliAuthor.SiteID = sql.NullInt64{Int64: bili.GetID(), Valid: true}
	biliAuthor.SiteAuthorID = sql.NullString{String: "bili-a1", Valid: true}
	biliAuthor.AuthorName = sql.NullString{String: "既有作者名-不可覆盖", Valid: true}
	biliAuthor.Introduce = sql.NullString{String: "既有介绍-不可覆盖", Valid: true}
	if err := db.Create(biliAuthor).Error; err != nil {
		t.Fatalf("插跨站作者行失败: %v", err)
	}
	biliTag := entity2.NewSiteTag()
	biliTag.SiteID = sql.NullInt64{Int64: bili.GetID(), Valid: true}
	biliTag.SiteTagID = sql.NullString{String: "bili-t1", Valid: true}
	biliTag.SiteTagName = sql.NullString{String: "既有标签名-不可覆盖", Valid: true}
	if err := db.Create(biliTag).Error; err != nil {
		t.Fatalf("插跨站标签行失败: %v", err)
	}
	biliWs := entity2.NewWorkSet()
	biliWs.SiteID = sql.NullInt64{Int64: bili.GetID(), Valid: true}
	biliWs.SiteWorkSetID = sql.NullString{String: "bili-ws1", Valid: true}
	biliWs.SiteWorkSetName = sql.NullString{String: "既有集名-不可覆盖", Valid: true}
	if err := db.Create(biliWs).Error; err != nil {
		t.Fatalf("插跨站作品集行失败: %v", err)
	}

	siteWorkId := "cross-site-work-1"
	workResp := &sdkdto.WorkResponse{
		Work: &sdkdto.WorkDTO{SiteWorkId: &siteWorkId, SiteWorkName: &siteWorkId},
		SiteAuthors: []*sdkdto.TaskSiteAuthorDTO{
			// 跨站：声明名与既有行名不同，断言不回写
			{SiteKey: "bilibili", SiteAuthorId: "bili-a1", AuthorName: "DTO声明名-不应落库"},
			// 同批本站声明照常 upsert
			{SiteAuthorId: "px-a1", AuthorName: "pixiv作者"},
		},
		SiteTags: []*sdkdto.TaskSiteTagDTO{
			{SiteKey: "bilibili", SiteTagId: "bili-t1", TagName: "DTO声明标签-不应落库"},
			{SiteTagId: "px-t1", TagName: "pixiv标签"},
		},
		WorkSets: []*sdkdto.TaskWorkSetDTO{
			{SiteKey: "bilibili", SiteWorkSetId: "bili-ws1", WorkSetName: "DTO声明集名-不应落库"},
			{SiteWorkSetId: "px-ws1", WorkSetName: "pixiv集"},
		},
	}

	task, wt := newWorkTaskOfSite(pixiv.GetID())
	workId, err := svc.saveWorkInfoInTx(context.Background(), task, wt, workResp)
	if err != nil {
		t.Fatalf("跨站命中既有行应只挂联成功: %v", err)
	}

	// 行不被改写：跨站行字段保持原值
	var gotAuthor entity2.SiteAuthor
	if err := db.Where("id = ?", biliAuthor.GetID()).First(&gotAuthor).Error; err != nil {
		t.Fatalf("回查跨站作者行失败: %v", err)
	}
	if gotAuthor.AuthorName.String != "既有作者名-不可覆盖" || gotAuthor.Introduce.String != "既有介绍-不可覆盖" {
		t.Fatalf("跨站作者行被改写: name=%q introduce=%q", gotAuthor.AuthorName.String, gotAuthor.Introduce.String)
	}
	var gotTag entity2.SiteTag
	if err := db.Where("id = ?", biliTag.GetID()).First(&gotTag).Error; err != nil {
		t.Fatalf("回查跨站标签行失败: %v", err)
	}
	if gotTag.SiteTagName.String != "既有标签名-不可覆盖" {
		t.Fatalf("跨站标签行被改写: name=%q", gotTag.SiteTagName.String)
	}
	var gotWs entity2.WorkSet
	if err := db.Where("id = ?", biliWs.GetID()).First(&gotWs).Error; err != nil {
		t.Fatalf("回查跨站作品集行失败: %v", err)
	}
	if gotWs.SiteWorkSetName.String != "既有集名-不可覆盖" {
		t.Fatalf("跨站作品集行被改写: name=%q", gotWs.SiteWorkSetName.String)
	}

	// 不为跨站键造行：bilibili 站点下三表仍各 1 行（仅种子行）
	if n := countRows(t, db, &entity2.SiteAuthor{}, "site_id = ?", bili.GetID()); n != 1 {
		t.Fatalf("跨站站点作者不应新建行，实际 %d 行", n)
	}
	if n := countRows(t, db, &entity2.SiteTag{}, "site_id = ?", bili.GetID()); n != 1 {
		t.Fatalf("跨站站点标签不应新建行，实际 %d 行", n)
	}
	if n := countRows(t, db, &entity2.WorkSet{}, "site_id = ?", bili.GetID()); n != 1 {
		t.Fatalf("跨站作品集不应新建行，实际 %d 行", n)
	}

	// 挂联：SITE 关联指向既有行 DB ID；同批本站声明行建于 pixiv 且各关联 2 条
	if n := countRows(t, db, &entity2.ReWorkAuthor{}, "work_id = ? AND author_type = ? AND site_author_id = ?", workId, constant.SITE, biliAuthor.GetID()); n != 1 {
		t.Fatalf("SITE 作者关联应指向既有跨站行 DB ID，实际 %d 条", n)
	}
	if n := countRows(t, db, &entity2.ReWorkTag{}, "work_id = ? AND tag_type = ? AND site_tag_id = ?", workId, constant.SITE, biliTag.GetID()); n != 1 {
		t.Fatalf("SITE 标签关联应指向既有跨站行 DB ID，实际 %d 条", n)
	}
	if n := countRows(t, db, &entity2.ReWorkWorkSet{}, "work_id = ? AND work_set_id = ?", workId, biliWs.GetID()); n != 1 {
		t.Fatalf("作品集关联应指向既有跨站行 DB ID，实际 %d 条", n)
	}
	if n := countRows(t, db, &entity2.SiteAuthor{}, "site_id = ?", pixiv.GetID()); n != 1 {
		t.Fatalf("同批本站作者应建 1 行，实际 %d", n)
	}
	if n := countRows(t, db, &entity2.ReWorkAuthor{}, "work_id = ? AND author_type = ?", workId, constant.SITE); n != 2 {
		t.Fatalf("SITE 作者关联共应 2 条（跨站 1 + 本站 1），实际 %d", n)
	}
	if n := countRows(t, db, &entity2.ReWorkTag{}, "work_id = ? AND tag_type = ?", workId, constant.SITE); n != 2 {
		t.Fatalf("SITE 标签关联共应 2 条，实际 %d", n)
	}
	if n := countRows(t, db, &entity2.ReWorkWorkSet{}, "work_id = ?", workId); n != 2 {
		t.Fatalf("作品集关联共应 2 条，实际 %d", n)
	}
}

// TestSaveWorkInfoCrossSiteMissingRowErrors 跨站引用的站点已注册但声明的站点侧 ID 无既有行：
// 报错且错误信息携带站点键与站点侧 ID（插件可据此定位声明错误），不回退建行
func TestSaveWorkInfoCrossSiteMissingRowErrors(t *testing.T) {
	svc, db := newCrossSiteTestEnv(t)
	pixiv := seedSite(t, db, "pixiv")
	seedSite(t, db, "bilibili")

	buildResp := func(massage func(r *sdkdto.WorkResponse)) *sdkdto.WorkResponse {
		siteWorkId := "cross-missing-work"
		r := &sdkdto.WorkResponse{Work: &sdkdto.WorkDTO{SiteWorkId: &siteWorkId, SiteWorkName: &siteWorkId}}
		massage(r)
		return r
	}
	cases := []struct {
		name   string
		build  func() *sdkdto.WorkResponse
		model  any    // 不回退建行断言的目标表
		col    string // 站点侧 ID 列名
		sideId string
	}{
		{"站点作者", func() *sdkdto.WorkResponse {
			return buildResp(func(r *sdkdto.WorkResponse) {
				r.SiteAuthors = []*sdkdto.TaskSiteAuthorDTO{{SiteKey: "bilibili", SiteAuthorId: "ghost-a", AuthorName: "幽灵作者"}}
			})
		}, &entity2.SiteAuthor{}, "site_author_id", "ghost-a"},
		{"站点标签", func() *sdkdto.WorkResponse {
			return buildResp(func(r *sdkdto.WorkResponse) {
				r.SiteTags = []*sdkdto.TaskSiteTagDTO{{SiteKey: "bilibili", SiteTagId: "ghost-t", TagName: "幽灵标签"}}
			})
		}, &entity2.SiteTag{}, "site_tag_id", "ghost-t"},
		{"作品集", func() *sdkdto.WorkResponse {
			return buildResp(func(r *sdkdto.WorkResponse) {
				r.WorkSets = []*sdkdto.TaskWorkSetDTO{{SiteKey: "bilibili", SiteWorkSetId: "ghost-ws", WorkSetName: "幽灵集"}}
			})
		}, &entity2.WorkSet{}, "site_work_set_id", "ghost-ws"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			task, wt := newWorkTaskOfSite(pixiv.GetID())
			_, err := svc.saveWorkInfoInTx(context.Background(), task, wt, c.build())
			if err == nil {
				t.Fatalf("跨站行不存在应报错")
			}
			if !strings.Contains(err.Error(), "bilibili") || !strings.Contains(err.Error(), c.sideId) {
				t.Fatalf("错误信息应含站点键与站点侧 ID，实际: %v", err)
			}
			if n := countRows(t, db, c.model, c.col+" = ?", c.sideId); n != 0 {
				t.Fatalf("跨站行不存在不应回退建行，实际 %d 行", n)
			}
		})
	}
}

// TestSaveWorkInfoCrossSiteUnregisteredKeyErrors 声明的站点键未注册（站点表无行）：报错且错误
// 信息携带站点键；站点域不按名寻址、不造站点行（键即身份）
func TestSaveWorkInfoCrossSiteUnregisteredKeyErrors(t *testing.T) {
	svc, db := newCrossSiteTestEnv(t)
	pixiv := seedSite(t, db, "pixiv")

	buildResp := func(massage func(r *sdkdto.WorkResponse)) *sdkdto.WorkResponse {
		siteWorkId := "cross-unregistered-work"
		r := &sdkdto.WorkResponse{Work: &sdkdto.WorkDTO{SiteWorkId: &siteWorkId, SiteWorkName: &siteWorkId}}
		massage(r)
		return r
	}
	cases := []struct {
		name  string
		build func() *sdkdto.WorkResponse
	}{
		{"站点作者", func() *sdkdto.WorkResponse {
			return buildResp(func(r *sdkdto.WorkResponse) {
				r.SiteAuthors = []*sdkdto.TaskSiteAuthorDTO{{SiteKey: "no-such-site", SiteAuthorId: "x-a", AuthorName: "无名作者"}}
			})
		}},
		{"站点标签", func() *sdkdto.WorkResponse {
			return buildResp(func(r *sdkdto.WorkResponse) {
				r.SiteTags = []*sdkdto.TaskSiteTagDTO{{SiteKey: "no-such-site", SiteTagId: "x-t", TagName: "无名标签"}}
			})
		}},
		{"作品集", func() *sdkdto.WorkResponse {
			return buildResp(func(r *sdkdto.WorkResponse) {
				r.WorkSets = []*sdkdto.TaskWorkSetDTO{{SiteKey: "no-such-site", SiteWorkSetId: "x-ws", WorkSetName: "无名集"}}
			})
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			task, wt := newWorkTaskOfSite(pixiv.GetID())
			_, err := svc.saveWorkInfoInTx(context.Background(), task, wt, c.build())
			if err == nil {
				t.Fatalf("未注册站点键应报错")
			}
			if !strings.Contains(err.Error(), "no-such-site") {
				t.Fatalf("错误信息应含未注册站点键，实际: %v", err)
			}
		})
	}
	// 不为未注册键造站点行
	if n := countRows(t, db, &entity2.Site{}, ""); n != 1 {
		t.Fatalf("站点表应仍仅 1 行（不造行），实际 %d", n)
	}
}
