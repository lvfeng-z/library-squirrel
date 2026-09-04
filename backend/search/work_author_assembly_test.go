package search

import (
	"context"
	"database/sql"
	"testing"

	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/migration"

	"gorm.io/gorm"
)

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
// SQL 子查询 JSON 键须与 RankedLocalAuthor/RankedSiteAuthor 对齐（嵌套 author 对象 + roleName/sortOrder），
// 键不匹配时 json.Unmarshal 静默丢弃、作者元素退化为零值（authorName 空）——卡片作者栏空白即此形态
func TestQueryWorkPageAuthorAssembled(t *testing.T) {
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	repo := NewRepository(db)
	siteWorkId, localWorkId := buildAuthorFixture(t, db)

	items, _, err := repo.QueryWorkPage(context.Background(), 1, 10, nil)
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}

	foundSite := false
	foundLocal := false
	for _, it := range items {
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
				t.Fatalf("站点作者名应反序列化为「站点作者甲」，实际 %+v", got.Author)
			}
			if got.RoleName != "作者" {
				t.Fatalf("站点作者角色名应反序列化为「作者」，实际 %q", got.RoleName)
			}
		case localWorkId:
			foundLocal = true
			if len(it.LocalAuthors) == 0 {
				t.Fatalf("作品 %d 应挂 1 个本地作者，实际 %d 个", localWorkId, len(it.LocalAuthors))
			}
			got := it.LocalAuthors[0]
			if got.Author.GetAuthorName() != "本地作者乙" {
				t.Fatalf("本地作者名应反序列化为「本地作者乙」，实际 %q", got.Author.GetAuthorName())
			}
		}
	}
	if !foundSite || !foundLocal {
		t.Fatalf("两作品均应出现在分页结果: foundSite=%v foundLocal=%v", foundSite, foundLocal)
	}
}
