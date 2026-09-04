package search

import (
	"context"
	"database/sql"
	"testing"

	"github.com/library-squirrel/backend/localAuthor"
	"github.com/library-squirrel/backend/localTag"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/persistentStore"
	"github.com/library-squirrel/backend/reWorkAuthor"
	"github.com/library-squirrel/backend/reWorkTag"
	"github.com/library-squirrel/backend/resource"
	"github.com/library-squirrel/backend/site"
	"github.com/library-squirrel/backend/siteTag"
	"github.com/library-squirrel/backend/work"

	domain "github.com/library-squirrel/backend/base/model/entity"

	"gorm.io/gorm"
)

// newWorkPageServiceEnv 内存库（全量迁移 + FK 强制）+ 真 work.Service（真仓储直连库）注入的 search service。
// 组装涉及的批量读取依赖（本地/站点作者、本地/站点标签、站点、资源、store 挂载、store 行）全部真件，
// 与主页作品链无关的依赖（作品集封面、lastUse 更新、写入器等）nil。
func newWorkPageServiceEnv(t *testing.T) (*Service, *work.Service, *SearchRepository, *gorm.DB) {
	t.Helper()
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}

	psSvc := persistentStore.NewService(persistentStore.NewRepository(db), nil, func() string { return t.TempDir() })
	resourceSvc := resource.NewService(resource.NewRepository(db), resource.NewResourceStoreRepository(db), psSvc)
	localTagSvc := localTag.NewService(localTag.NewRepository(db), nil, nil, nil)
	siteTagSvc := siteTag.NewService(siteTag.NewRepository(db), nil, nil, nil, nil, nil)
	siteSvc := site.NewService(site.NewRepository(db))
	localAuthorSvc := localAuthor.NewService(localAuthor.NewRepository(db), nil, nil, nil, nil)
	reWorkAuthorSvc := reWorkAuthor.NewService(reWorkAuthor.NewRepository(db))
	reWorkTagSvc := reWorkTag.NewService(reWorkTag.NewRepository(db), nil)

	workSvc := work.NewService(
		work.NewRepository(db), // Repository
		nil,                    // Transactor（主页链无事务）
		nil,                    // LocalTagReader
		nil,                    // LocalAuthorReader
		nil,                    // SiteTagReader
		nil,                    // SiteAuthorReader
		nil,                    // SiteReader
		nil,                    // ResourceReader
		nil,                    // ReWorkTagWriter
		nil,                    // ReWorkWorkSetWriter
		nil,                    // ResourceDeleter
		nil,                    // SiteAuthorWriter
		nil,                    // SiteTagWriter
		nil,                    // WorkSetWriter
		nil,                    // ReWorkAuthorWriter
		localTagSvc,            // LocalTagBatchReader
		siteTagSvc,             // SiteTagBatchReader
		siteSvc,                // SiteBatchReader
		localAuthorSvc,         // LocalAuthorBatchReader
		reWorkAuthorSvc,        // SiteAuthorBatchReader
		resourceSvc,            // ResourceBatchReader
		resourceSvc,            // ResourceStoreBatchReader
		psSvc,                  // StoreBatchReader
		reWorkTagSvc,           // ReWorkTagBatchReader
		nil,                    // LocalTagFindOrCreator
		nil,                    // LocalAuthorFindOrCreator
		nil,                    // StoreDeleter
		nil,                    // RunningTaskStopper
		nil,                    // ResourceStoreHardDeleter
		nil,                    // WorkSetRelationWriter
		nil,                    // CoverReferenceClearer
		nil,                    // WorkLockChecker
	)

	svc := NewService(
		NewRepository(db), // Repository
		nil,               // CoverResolver
		nil,               // WorkSetPageWorkReader
		nil,               // WorkSetPageResourceReader
		nil,               // StoreBatchReader
		nil,               // ResourceStoreBatchReader
		nil,               // LocalTagUpdater
		nil,               // SiteTagUpdater
		nil,               // LocalAuthorUpdater
		nil,               // SiteAuthorUpdater
		workSvc,           // FullWorkAssembler
	)
	return svc, workSvc, NewRepository(db), db
}

// buildAuthorFixture 数据面：一作品挂 SITE 作者关联 + 一作品挂 LOCAL 作者关联（含 role_name/sort_order）
func buildAuthorFixture(t *testing.T, db *gorm.DB) (siteWorkId, localWorkId int64) {
	t.Helper()
	siteRow := domain.NewSite()
	siteRow.SiteKey = "bilibili"
	siteRow.SiteName = sql.NullString{String: "bilibili", Valid: true}
	if err := db.Create(siteRow).Error; err != nil {
		t.Fatalf("插站点失败: %v", err)
	}

	site := domain.NewSiteAuthor()
	site.SiteID = sql.NullInt64{Int64: siteRow.GetID(), Valid: true}
	site.SiteAuthorID = sql.NullString{String: "a-1", Valid: true}
	site.AuthorName = sql.NullString{String: "站点作者甲", Valid: true}
	if err := db.Create(site).Error; err != nil {
		t.Fatalf("插站点作者失败: %v", err)
	}

	local := domain.NewLocalAuthor()
	local.AuthorName = sql.NullString{String: "本地作者乙", Valid: true}
	if err := db.Create(local).Error; err != nil {
		t.Fatalf("插本地作者失败: %v", err)
	}

	newWork := func() int64 {
		w := domain.NewWork()
		w.SiteWorkName = sql.NullString{String: "作品", Valid: true}
		if err := db.Create(w).Error; err != nil {
			t.Fatalf("插作品失败: %v", err)
		}
		return w.GetID()
	}
	siteWorkId = newWork()
	localWorkId = newWork()

	linkSite := domain.NewReWorkAuthor()
	linkSite.AuthorType = sql.NullInt64{Int64: 1, Valid: true}
	linkSite.WorkID = sql.NullInt64{Int64: siteWorkId, Valid: true}
	linkSite.SiteAuthorID = sql.NullInt64{Int64: site.GetID(), Valid: true}
	linkSite.RoleName = sql.NullString{String: "作者", Valid: true}
	if err := db.Create(linkSite).Error; err != nil {
		t.Fatalf("挂站点作者关联失败: %v", err)
	}

	linkLocal := domain.NewReWorkAuthor()
	linkLocal.AuthorType = sql.NullInt64{Int64: 0, Valid: true}
	linkLocal.WorkID = sql.NullInt64{Int64: localWorkId, Valid: true}
	linkLocal.LocalAuthorID = sql.NullInt64{Int64: local.GetID(), Valid: true}
	if err := db.Create(linkLocal).Error; err != nil {
		t.Fatalf("挂本地作者关联失败: %v", err)
	}
	return siteWorkId, localWorkId
}

// TestQueryWorkPageAuthorAssembled 主页作品分页的作者组装回归：
// service 编排链（条件圈定作品 ID → work 模块批量组装）须把 SITE/LOCAL 作者名与角色名组装进 WorkFullDTO，
// 作者环节断裂时作品卡片作者栏空白
func TestQueryWorkPageAuthorAssembled(t *testing.T) {
	svc, _, _, db := newWorkPageServiceEnv(t)
	siteWorkId, localWorkId := buildAuthorFixture(t, db)

	page, err := svc.QueryWorkPage(context.Background(), 1, 10, nil)
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}

	foundSite := false
	foundLocal := false
	for _, it := range page.Data {
		if it.Work == nil {
			continue
		}
		switch it.Work.GetId() {
		case siteWorkId:
			foundSite = true
			if len(it.SiteAuthors) == 0 {
				t.Fatalf("作品 %d 应挂 1 个站点作者，实际 %d 个", siteWorkId, len(it.SiteAuthors))
			}
			got := it.SiteAuthors[0]
			if got.Author.AuthorName == nil || *got.Author.AuthorName != "站点作者甲" {
				t.Fatalf("站点作者名应为「站点作者甲」，实际 %+v", got.Author)
			}
			if got.RoleName != "作者" {
				t.Fatalf("站点作者角色名应为「作者」，实际 %q", got.RoleName)
			}
		case localWorkId:
			foundLocal = true
			if len(it.LocalAuthors) == 0 {
				t.Fatalf("作品 %d 应挂 1 个本地作者，实际 %d 个", localWorkId, len(it.LocalAuthors))
			}
			got := it.LocalAuthors[0]
			if got.Author.GetAuthorName() != "本地作者乙" {
				t.Fatalf("本地作者名应为「本地作者乙」，实际 %q", got.Author.GetAuthorName())
			}
		}
	}
	if !foundSite || !foundLocal {
		t.Fatalf("两作品均应出现在分页结果: foundSite=%v foundLocal=%v", foundSite, foundLocal)
	}
}
