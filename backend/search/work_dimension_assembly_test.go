package search

import (
	"context"
	"database/sql"
	"testing"

	"github.com/library-squirrel/backend/base/constant"
	dto2 "github.com/library-squirrel/backend/base/model/dto"
	domain "github.com/library-squirrel/backend/base/model/entity"

	"gorm.io/gorm"
)

// 关联级维度（ns/role）的搜索行为与展示组装锚定：
// 空串语义（空=不过滤命中全部，非空过滤不命中空串行）+ D-17（SITE 标签不设 ns 命中该标签全部关联）；
// 组装条目聚合规则（标签按唯一标签去重、作者按关联行每行一条）。

// buildDimensionFixture 数据面：同作品同站点标签挂三种 ns（female/male/空串）+ 同站点作者挂两 role
func buildDimensionFixture(t *testing.T, db *gorm.DB) (workId, siteTagId, siteAuthorId int64) {
	t.Helper()
	siteRow := domain.NewSite()
	siteRow.SiteKey = "ehentai"
	siteRow.SiteName = sql.NullString{String: "ehentai", Valid: true}
	if err := db.Create(siteRow).Error; err != nil {
		t.Fatalf("插站点失败: %v", err)
	}
	w := domain.NewWork()
	w.SiteID = sql.NullInt64{Int64: siteRow.GetID(), Valid: true}
	w.SiteWorkID = sql.NullString{String: "dim-work", Valid: true}
	if err := db.Create(w).Error; err != nil {
		t.Fatalf("插作品失败: %v", err)
	}
	st := domain.NewSiteTag()
	st.SiteID = sql.NullInt64{Int64: siteRow.GetID(), Valid: true}
	st.SiteTagID = sql.NullString{String: "tagA", Valid: true}
	st.SiteTagName = sql.NullString{String: "tagA", Valid: true}
	if err := db.Create(st).Error; err != nil {
		t.Fatalf("插站点标签失败: %v", err)
	}
	for _, ns := range []string{"female", "male", ""} {
		rel := domain.NewReWorkTag()
		rel.WorkID = sql.NullInt64{Int64: w.GetID(), Valid: true}
		rel.TagType = sql.NullInt64{Int64: constant.SITE, Valid: true}
		rel.SiteTagID = sql.NullInt64{Int64: st.GetID(), Valid: true}
		rel.Namespace = ns
		if err := db.Create(rel).Error; err != nil {
			t.Fatalf("挂 ns=%q 关联失败: %v", ns, err)
		}
	}
	sa := domain.NewSiteAuthor()
	sa.SiteID = sql.NullInt64{Int64: siteRow.GetID(), Valid: true}
	sa.SiteAuthorID = sql.NullString{String: "sa-1", Valid: true}
	sa.AuthorName = sql.NullString{String: "作者甲", Valid: true}
	if err := db.Create(sa).Error; err != nil {
		t.Fatalf("插站点作者失败: %v", err)
	}
	for _, role := range []string{"原画", "脚本"} {
		rel := domain.NewReWorkAuthor()
		rel.WorkID = sql.NullInt64{Int64: w.GetID(), Valid: true}
		rel.AuthorType = sql.NullInt64{Int64: constant.SITE, Valid: true}
		rel.SiteAuthorID = sql.NullInt64{Int64: sa.GetID(), Valid: true}
		rel.RoleName = role
		if err := db.Create(rel).Error; err != nil {
			t.Fatalf("挂 role=%q 作者关联失败: %v", role, err)
		}
	}
	return w.GetID(), st.GetID(), sa.GetID()
}

// TestNamespaceSearchEmptyStringSemantics 空串搜索语义（锚 6/10，D-17）：
// 不设 ns = 命中该标签全部关联（含空串无 ns 与其他 ns）；设具体 ns = 仅命中该 ns 关联（空串行不命中）
func TestNamespaceSearchEmptyStringSemantics(t *testing.T) {
	_, workSvc, repo, db := newWorkPageServiceEnv(t)
	workId, siteTagId, _ := buildDimensionFixture(t, db)
	ctx := context.Background()

	// 不设 ns：命中（D-17——不再自动携带标签 ns 精确匹配，结果含无 ns 与其他 ns 的关联）
	ids, total, err := repo.QueryWorkIdPage(ctx, 1, 10, []*dto2.SearchCondition{
		{Type: dto2.SiteTag, Value: siteTagId},
	})
	if err != nil {
		t.Fatalf("不设 ns 查询失败: %v", err)
	}
	if total != 1 || len(ids) != 1 || ids[0] != workId {
		t.Fatalf("不设 ns 应命中该标签全部关联的作品，实际 total=%d ids=%v", total, ids)
	}

	// 设 female：命中（该 ns 关联存在）
	ids, _, err = repo.QueryWorkIdPage(ctx, 1, 10, []*dto2.SearchCondition{
		{Type: dto2.SiteTag, Value: siteTagId, Namespace: "female"},
	})
	if err != nil {
		t.Fatalf("female 过滤查询失败: %v", err)
	}
	if len(ids) != 1 || ids[0] != workId {
		t.Fatalf("female 过滤应命中作品，实际 ids=%v", ids)
	}

	// 设不存在的 ns：不命中（空串行与其他 ns 行均不匹配）
	ids, total, err = repo.QueryWorkIdPage(ctx, 1, 10, []*dto2.SearchCondition{
		{Type: dto2.SiteTag, Value: siteTagId, Namespace: "language"},
	})
	if err != nil {
		t.Fatalf("language 过滤查询失败: %v", err)
	}
	if total != 0 || len(ids) != 0 {
		t.Fatalf("不存在的 ns 过滤不应命中，实际 total=%d ids=%v", total, ids)
	}

	// 主页链（service.QueryWorkPage）与圈定直查一致走同一组装面，此处锚组装条目聚合规则：
	// 同标签三 ns 关联 → 标签条目去重为一条；同作者两 role 关联 → 作者条目两行（role 随行）
	full, err := workSvc.GetFullWorkInfoByIds(ctx, []int64{workId})
	if err != nil {
		t.Fatalf("组装查询失败: %v", err)
	}
	if len(full) != 1 {
		t.Fatalf("应组装 1 作品，实际 %d", len(full))
	}
	if n := len(full[0].SiteTags); n != 1 {
		t.Fatalf("同标签多 ns 的标签条目应去重为 1 条（条目不携带 ns），实际 %d", n)
	}
	if n := len(full[0].SiteAuthors); n != 2 {
		t.Fatalf("同作者多 role 的作者条目应每关联行一条（role 随行），实际 %d", n)
	}
	roles := map[string]bool{}
	for _, a := range full[0].SiteAuthors {
		roles[a.RoleName] = true
	}
	if !roles["原画"] || !roles["脚本"] {
		t.Fatalf("作者条目应携带各自 role，实际 %v", roles)
	}
}
