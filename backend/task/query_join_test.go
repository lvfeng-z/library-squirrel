package task

import (
	"context"
	"database/sql"
	"testing"

	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/base/query"
	"github.com/library-squirrel/backend/migration"

	"gorm.io/gorm"
)

// 本文件为任务查询挂作品任务领域表（work_task 左连接）的回归锚定：领域列过滤/排序经
// 全限定列名（work_task.site_id 等）引用，核心列经 task. 前缀消歧；内置类型任务（无领域行）
// 不因连接被过滤。

// seedPluginTask 真实库种一个插件任务对（核心行 + 作品领域行，id 同值）
func seedPluginTask(t *testing.T, db *gorm.DB, name string, siteID int64, siteWorkID string) *TaskWithWorkTask {
	t.Helper()
	core := &entity.Task{
		BaseEntity: &model.BaseEntity{},
		TaskName:   sql.NullString{String: name, Valid: true},
		Status:     int(TaskStatusCreated),
		TaskType:   sql.NullString{String: entity.TaskTypePluginDownload, Valid: true},
		HasChild:   sql.NullBool{Bool: false, Valid: true},
	}
	if err := db.Create(core).Error; err != nil {
		t.Fatalf("插核心行失败: %v", err)
	}
	wt := entity.NewWorkTask(core.GetID())
	wt.SiteID = sql.NullInt64{Int64: siteID, Valid: siteID != 0}
	wt.SiteWorkID = sql.NullString{String: siteWorkID, Valid: siteWorkID != ""}
	if err := db.Create(wt).Error; err != nil {
		t.Fatalf("插领域行失败: %v", err)
	}
	return &TaskWithWorkTask{Task: core, WorkTask: wt}
}

// TestQueryWithWorkTaskJoin 领域列过滤经 work_task 左连接命中：site_work_id/site_id 条件
// 圈定目标任务；内置类型任务（无领域行）在无领域条件时不被过滤、在有领域条件时不命中
func TestQueryWithWorkTaskJoin(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	wtStore := newTestWorkTaskStore(db)
	repo := NewRepository(db, wtStore, wtStore)
	svc := NewService(repo, &testTransactor{db: db}, nil, nil, nil, nil, nil)
	ctx := context.Background()

	// 站点种子（work_task.site_id 外键防线）
	seedSite := entity.NewSite()
	seedSite.SiteKey = testSiteKey
	seedSite.SiteName = sql.NullString{String: testSiteName, Valid: true}
	if err := db.Create(seedSite).Error; err != nil {
		t.Fatalf("建站点种子失败: %v", err)
	}
	siteKey := seedSite.GetID()

	pluginA := seedPluginTask(t, db, "插件任务A", siteKey, "sw-a")
	seedPluginTask(t, db, "插件任务B", siteKey, "sw-b")
	// 内置类型任务：无作品领域行
	builtin := entity.NewTask()
	builtin.TaskName = sql.NullString{String: "内置任务", Valid: true}
	builtin.TaskType = sql.NullString{String: "share-receive", Valid: true}
	builtin.HasChild = sql.NullBool{Bool: false, Valid: true}
	if err := db.Create(builtin).Error; err != nil {
		t.Fatalf("插内置任务失败: %v", err)
	}

	// 领域列过滤：site_work_id 命中单行
	got, err := svc.Page(ctx, &model.Page[entity.Task]{PageNumber: 1, PageSize: 10}, TaskQueryDTO{
		SiteWorkID: query.QueryAttribute[string]{Value: strPtr("sw-a")},
	})
	if err != nil {
		t.Fatalf("领域列过滤分页失败: %v", err)
	}
	if got.DataCount != 1 || len(got.Data) != 1 || got.Data[0].Task.GetID() != pluginA.Task.GetID() {
		t.Fatalf("site_work_id=sw-a 应恰命中插件任务A，得到 count=%d rows=%d", got.DataCount, len(got.Data))
	}
	if got.Data[0].WorkTask == nil || got.Data[0].WorkTask.SiteWorkID.String != "sw-a" {
		t.Fatalf("分页结果应装配作品领域行: %+v", got.Data[0].WorkTask)
	}

	// 无领域条件：核心行全集（含无领域行的内置任务，不被左连接过滤）
	all, err := svc.Page(ctx, &model.Page[entity.Task]{PageNumber: 1, PageSize: 10}, TaskQueryDTO{})
	if err != nil {
		t.Fatalf("无条件分页失败: %v", err)
	}
	if all.DataCount != 3 {
		t.Fatalf("无条件应命中全部 3 行（含内置任务），得到 %d", all.DataCount)
	}

	// 站点反查：ListBySiteAndSiteWorkID 经连接命中
	bySite, err := repo.ListBySiteAndSiteWorkID(ctx, siteKey, "sw-b")
	if err != nil {
		t.Fatalf("站点反查失败: %v", err)
	}
	if len(bySite) != 1 || bySite[0].TaskName.String != "插件任务B" {
		t.Fatalf("站点反查应命中插件任务B，得到 %+v", bySite)
	}

	// 树双查：核心行 + 领域行按共享主键关联（内置任务无领域行不在 map）
	rows, err := repo.ListTaskTree(ctx, []int64{pluginA.Task.GetID(), builtin.GetID()})
	if err != nil {
		t.Fatalf("树双查失败: %v", err)
	}
	if len(rows.Tasks) != 2 {
		t.Fatalf("树双查应含 2 核心行，得到 %d", len(rows.Tasks))
	}
	if len(rows.WorkTasks) != 1 || rows.WorkTasks[pluginA.Task.GetID()] == nil {
		t.Fatalf("树双查应恰 1 行作品领域行（内置任务无），得到 %+v", rows.WorkTasks)
	}
}

func strPtr(s string) *string { return &s }
