package extension

import (
	"context"
	"database/sql"
	"testing"

	"github.com/library-squirrel/backend/base/constant"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/localAuthor"
	"github.com/library-squirrel/backend/localTag"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/persistentStore"
	"github.com/library-squirrel/backend/reWorkAuthor"
	"github.com/library-squirrel/backend/reWorkSetWorkSet"
	"github.com/library-squirrel/backend/reWorkTag"
	"github.com/library-squirrel/backend/reWorkWorkSet"
	"github.com/library-squirrel/backend/resource"
	"github.com/library-squirrel/backend/site"
	"github.com/library-squirrel/backend/siteAuthor"
	"github.com/library-squirrel/backend/siteTag"
	"github.com/library-squirrel/backend/work"
	"github.com/library-squirrel/backend/workSet"
	"github.com/lvfeng-z/library-squirrel-sdk/gen"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// stubWorkDir WorkDirSource 测试桩（workdir 未配置 = 空串）
type stubWorkDir struct{ dir string }

func (s stubWorkDir) GetWorkDir() string { return s.dir }

// newTestLibraryQuery 构造接真实测试库（完整迁移 + 外键强制）的查询核心
func newTestLibraryQuery(t *testing.T, workDir string) (*libraryQueryProvider, *gorm.DB) {
	t.Helper()
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Fatalf("打开测试库失败: %v", err)
	}
	deps := LibraryQueryDeps{
		Works:          work.NewRepository(db),
		Sites:          site.NewRepository(db),
		Resources:      resource.NewRepository(db),
		ResourceStores: resource.NewResourceStoreRepository(db),
		Stores:         persistentStore.NewRepository(db),
		LocalAuthors:   localAuthor.NewRepository(db),
		SiteAuthors:    siteAuthor.NewRepository(db),
		LocalTags:      localTag.NewRepository(db),
		SiteTags:       siteTag.NewRepository(db),
		WorkTagRels:    reWorkTag.NewRepository(db),
		WorkSets:       workSet.NewRepository(db),
		WorkWorkSets:   reWorkWorkSet.NewRepository(db),
		WorkSetGraph:   reWorkSetWorkSet.NewRepository(db),
		WorkDir:        stubWorkDir{dir: workDir},
	}
	return NewLibraryQueryProvider(deps), db
}

// --- 种子辅助 ---

func seedSite(t *testing.T, db *gorm.DB, siteKey string) *entity.Site {
	t.Helper()
	s := entity.NewSite()
	s.SiteKey = siteKey
	s.SiteName = sql.NullString{String: siteKey + "名", Valid: true}
	if err := site.NewRepository(db).Create(context.Background(), s); err != nil {
		t.Fatalf("种子站点失败: %v", err)
	}
	return s
}

func seedWork(t *testing.T, db *gorm.DB, siteID int64, siteWorkID, name string) *entity.Work {
	t.Helper()
	w := entity.NewWork()
	w.SiteID = sql.NullInt64{Int64: siteID, Valid: true}
	w.SiteWorkID = sql.NullString{String: siteWorkID, Valid: true}
	w.SiteWorkName = sql.NullString{String: name, Valid: true}
	if err := work.NewRepository(db).Create(context.Background(), w); err != nil {
		t.Fatalf("种子作品失败: %v", err)
	}
	return w
}

func seedStore(t *testing.T, db *gorm.DB, filePath string) *entity.PersistentStore {
	t.Helper()
	st := entity.NewPersistentStore()
	st.FilePath = sql.NullString{String: filePath, Valid: true}
	st.FilenameExtension = sql.NullString{String: "jpg", Valid: true}
	st.CompletedAt = 111
	st.Width = sql.NullInt64{Int64: 800, Valid: true}
	st.Height = sql.NullInt64{Int64: 600, Valid: true}
	if err := persistentStore.NewRepository(db).Create(context.Background(), st); err != nil {
		t.Fatalf("种子 store 失败: %v", err)
	}
	return st
}

func seedResourceWithStores(t *testing.T, db *gorm.DB, workID int64, stores ...*entity.PersistentStore) *entity.Resource {
	t.Helper()
	r := entity.NewResource()
	r.WorkID = workID
	r.ResourceType = "image"
	r.SuggestName = sql.NullString{String: "建议名", Valid: true}
	r.ResourceComplete = sql.NullInt64{Int64: 100, Valid: true}
	if err := resource.NewRepository(db).Create(context.Background(), r); err != nil {
		t.Fatalf("种子资源失败: %v", err)
	}
	rsRepo := resource.NewResourceStoreRepository(db)
	for i, st := range stores {
		rel := entity.NewResourceStore()
		rel.ResourceID = r.GetID()
		rel.StoreType = entity.StoreTypeImage
		rel.Generation = entity.GenerationDownloaded
		rel.StoreID = st.GetID()
		rel.StoreSeq = i
		if err := rsRepo.Create(context.Background(), rel); err != nil {
			t.Fatalf("种子资源关联失败: %v", err)
		}
	}
	return r
}

// --- 分页边界 ---

func TestQueryWorksPaginationClamp(t *testing.T) {
	p, db := newTestLibraryQuery(t, "")
	ctx := context.Background()
	s := seedSite(t, db, "pixiv")
	for i := 0; i < 5; i++ {
		seedWork(t, db, s.GetID(), string(rune('a'+i)), string(rune('A'+i)))
	}

	// 缺省分页（page/pageSize 均未置）：按 1/20 生效
	resp, err := p.queryWorks(ctx, &gen.QueryWorksRequest{})
	if err != nil {
		t.Fatalf("queryWorks: %v", err)
	}
	if resp.Page.Page != 1 || resp.Page.PageSize != 20 {
		t.Fatalf("缺省分页 = %d/%d, 期望 1/20", resp.Page.Page, resp.Page.PageSize)
	}
	if resp.Page.Total != 5 || len(resp.Items) != 5 {
		t.Fatalf("total=%d items=%d, 期望 5/5", resp.Page.Total, len(resp.Items))
	}

	// page_size 超上限截到 200
	resp, err = p.queryWorks(ctx, &gen.QueryWorksRequest{Page: &gen.PageRequest{Page: 1, PageSize: 1000}})
	if err != nil {
		t.Fatalf("queryWorks: %v", err)
	}
	if resp.Page.PageSize != 200 {
		t.Fatalf("pageSize 超限未截断: %d", resp.Page.PageSize)
	}

	// 常规翻页：total 如实、页内条数正确
	resp, err = p.queryWorks(ctx, &gen.QueryWorksRequest{Page: &gen.PageRequest{Page: 2, PageSize: 2}})
	if err != nil {
		t.Fatalf("queryWorks: %v", err)
	}
	if resp.Page.Total != 5 || len(resp.Items) != 2 {
		t.Fatalf("total=%d items=%d, 期望 5/2", resp.Page.Total, len(resp.Items))
	}

	// 站点键过滤未命中注册表：空集而非报错（过滤条件非寻址）
	resp, err = p.queryWorks(ctx, &gen.QueryWorksRequest{SiteKey: "nosuch"})
	if err != nil {
		t.Fatalf("queryWorks 未知站点键应返回空集: %v", err)
	}
	if resp.Page.Total != 0 || len(resp.Items) != 0 {
		t.Fatalf("未知站点键 total=%d items=%d, 期望 0/0", resp.Page.Total, len(resp.Items))
	}
}

// --- 软删活行过滤：作品轨 ---

func TestQueryWorksExcludesSoftDeletedWork(t *testing.T) {
	p, db := newTestLibraryQuery(t, "")
	ctx := context.Background()
	s := seedSite(t, db, "pixiv")
	w1 := seedWork(t, db, s.GetID(), "1", "甲")
	w2 := seedWork(t, db, s.GetID(), "2", "乙")
	w3 := seedWork(t, db, s.GetID(), "3", "丙")

	// 软删 w3（外部变更失效形态：deleted_at 置时刻、无 backup 关联）
	if err := db.Exec("UPDATE work SET deleted_at = 12345 WHERE id = ?", w3.GetID()).Error; err != nil {
		t.Fatalf("软删作品失败: %v", err)
	}

	resp, err := p.queryWorks(ctx, &gen.QueryWorksRequest{})
	if err != nil {
		t.Fatalf("queryWorks: %v", err)
	}
	if resp.Page.Total != 2 || len(resp.Items) != 2 {
		t.Fatalf("软删作品未排除: total=%d items=%d", resp.Page.Total, len(resp.Items))
	}
	for _, item := range resp.Items {
		if item.Work.Id == w3.GetID() {
			t.Fatalf("软删作品出现在结果中: id=%d", item.Work.Id)
		}
	}

	// Get* 未命中返回 NotFound 语义码
	if _, err = p.getWorkById(ctx, w3.GetID()); status.Code(err) != codes.NotFound {
		t.Fatalf("软删作品 getWorkById 错误码 = %v, 期望 NotFound (err=%v)", status.Code(err), err)
	}
	if _, err = p.getWorkBySiteKey(ctx, "pixiv", "3"); status.Code(err) != codes.NotFound {
		t.Fatalf("软删作品 getWorkBySiteKey 错误码 = %v, 期望 NotFound (err=%v)", status.Code(err), err)
	}
	// 活作品两轨命中
	if _, err = p.getWorkById(ctx, w1.GetID()); err != nil {
		t.Fatalf("活作品 getWorkById: %v", err)
	}
	ws, err := p.getWorkBySiteKey(ctx, "pixiv", "2")
	if err != nil || ws.Work.Id != w2.GetID() {
		t.Fatalf("getWorkBySiteKey 命中失败: %v", err)
	}
	if ws.Site == nil || ws.Site.SiteKey != "pixiv" {
		t.Fatalf("WorkWithSite 未携带站点键")
	}
}

// --- 软删活行过滤：store 关联轨（ListAliveByResourceIds 语义）---

func TestListResourcesAliveStoreFilter(t *testing.T) {
	p, db := newTestLibraryQuery(t, "")
	ctx := context.Background()
	s := seedSite(t, db, "pixiv")
	w := seedWork(t, db, s.GetID(), "1", "甲")

	alive := seedStore(t, db, "store/work/ab/pixiv_1/image_000.jpg")
	dead := seedStore(t, db, "store/work/ab/pixiv_1/image_001.jpg")
	if err := db.Exec("UPDATE persistent_store SET deleted_at = 12345 WHERE id = ?", dead.GetID()).Error; err != nil {
		t.Fatalf("软删 store 失败: %v", err)
	}
	r := seedResourceWithStores(t, db, w.GetID(), alive, dead)

	resp, err := p.listResourcesByWorkId(ctx, w.GetID())
	if err != nil {
		t.Fatalf("listResourcesByWorkId: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("资源数 = %d, 期望 1", len(resp.Items))
	}
	item := resp.Items[0]
	if item.Id != r.GetID() || item.ResourceType != "image" {
		t.Fatalf("资源摘要映射错误: %+v", item)
	}
	if len(item.Stores) != 1 {
		t.Fatalf("活行 store 数 = %d, 期望 1（软删 store 的关联不出现）", len(item.Stores))
	}
	store := item.Stores[0]
	if store.Role != entity.StoreTypeImage || store.StoreSeq != 0 {
		t.Fatalf("store 角色/序号映射错误: %+v", store)
	}
	if store.FilePath == nil || *store.FilePath != "store/work/ab/pixiv_1/image_000.jpg" {
		t.Fatalf("file_path 映射错误（relPath 域正斜杠原样）: %v", store.FilePath)
	}
	if store.Width == nil || *store.Width != 800 || store.Height == nil || *store.Height != 600 {
		t.Fatalf("宽高映射错误: %v/%v", store.Width, store.Height)
	}
	if store.Size != nil {
		t.Fatalf("size 无库列来源应缺省: %v", store.Size)
	}

	// 作品未命中 → NotFound
	if _, err = p.listResourcesByWorkId(ctx, 99999); status.Code(err) != codes.NotFound {
		t.Fatalf("未命中作品错误码 = %v, 期望 NotFound (err=%v)", status.Code(err), err)
	}
}

// --- workdir 拒绝态 ---

func TestGetWorkDirRefusedWhenUnconfigured(t *testing.T) {
	p, _ := newTestLibraryQuery(t, "")
	resp, err := p.getWorkDir("com.example.plugin")
	if err == nil {
		t.Fatalf("未配置 workdir 应显式拒绝")
	}
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("未配置 workdir 错误码 = %v, 期望 FailedPrecondition (err=%v)", status.Code(err), err)
	}
	if resp != nil {
		t.Fatalf("未配置 workdir 不应返回响应（不返回空路径）")
	}
}

func TestGetWorkDirReturnsConfiguredPath(t *testing.T) {
	p, _ := newTestLibraryQuery(t, `E:\library`)
	resp, err := p.getWorkDir("com.example.plugin")
	if err != nil {
		t.Fatalf("已配置 workdir 不应报错: %v", err)
	}
	if resp.Path != `E:\library` {
		t.Fatalf("workdir = %q, 期望原样返回", resp.Path)
	}
}

// --- 作品过滤语义（作者名/标签名，EXISTS 关联轨）---

func TestQueryWorksFiltersByAuthorAndTagName(t *testing.T) {
	p, db := newTestLibraryQuery(t, "")
	ctx := context.Background()
	s := seedSite(t, db, "pixiv")
	w1 := seedWork(t, db, s.GetID(), "1", "风景画")
	w2 := seedWork(t, db, s.GetID(), "2", "人物画")

	// w1 挂站点作者「作者甲」+ 本地标签「风景」；w2 挂站点作者「作者乙」
	sa1 := entity.NewSiteAuthor()
	sa1.SiteID = sql.NullInt64{Int64: s.GetID(), Valid: true}
	sa1.SiteAuthorID = sql.NullString{String: "a1", Valid: true}
	sa1.AuthorName = sql.NullString{String: "作者甲", Valid: true}
	sa2 := entity.NewSiteAuthor()
	sa2.SiteID = sql.NullInt64{Int64: s.GetID(), Valid: true}
	sa2.SiteAuthorID = sql.NullString{String: "a2", Valid: true}
	sa2.AuthorName = sql.NullString{String: "作者乙", Valid: true}
	raRepo := reWorkAuthor.NewRepository(db)
	if err := siteAuthor.NewRepository(db).CreateBatch(ctx, []*entity.SiteAuthor{sa1, sa2}); err != nil {
		t.Fatalf("种子站点作者失败: %v", err)
	}
	linkAuthor := func(workID int64, sa *entity.SiteAuthor) {
		ra := entity.NewReWorkAuthor()
		ra.WorkID = sql.NullInt64{Int64: workID, Valid: true}
		ra.SiteAuthorID = sql.NullInt64{Int64: sa.GetID(), Valid: true}
		ra.AuthorType = sql.NullInt64{Int64: constant.SITE, Valid: true}
		if err := raRepo.Create(ctx, ra); err != nil {
			t.Fatalf("种子作者关联失败: %v", err)
		}
	}
	linkAuthor(w1.GetID(), sa1)
	linkAuthor(w2.GetID(), sa2)

	lt := entity.NewLocalTag()
	lt.LocalTagName = sql.NullString{String: "风景", Valid: true}
	if err := localTag.NewRepository(db).Create(ctx, lt); err != nil {
		t.Fatalf("种子本地标签失败: %v", err)
	}
	rt := entity.NewReWorkTag()
	rt.WorkID = sql.NullInt64{Int64: w1.GetID(), Valid: true}
	rt.LocalTagID = sql.NullInt64{Int64: lt.GetID(), Valid: true}
	rt.TagType = sql.NullInt64{Int64: constant.LOCAL, Valid: true}
	if err := reWorkTag.NewRepository(db).Create(ctx, rt); err != nil {
		t.Fatalf("种子标签关联失败: %v", err)
	}

	resp, err := p.queryWorks(ctx, &gen.QueryWorksRequest{AuthorKeyword: "作者甲"})
	if err != nil {
		t.Fatalf("queryWorks 按作者名: %v", err)
	}
	if resp.Page.Total != 1 || resp.Items[0].Work.Id != w1.GetID() {
		t.Fatalf("按作者名过滤结果错误: total=%d", resp.Page.Total)
	}

	resp, err = p.queryWorks(ctx, &gen.QueryWorksRequest{TagKeyword: "风景"})
	if err != nil {
		t.Fatalf("queryWorks 按标签名: %v", err)
	}
	if resp.Page.Total != 1 || resp.Items[0].Work.Id != w1.GetID() {
		t.Fatalf("按标签名过滤结果错误: total=%d", resp.Page.Total)
	}

	resp, err = p.queryWorks(ctx, &gen.QueryWorksRequest{NameKeyword: "人物"})
	if err != nil {
		t.Fatalf("queryWorks 按名称: %v", err)
	}
	if resp.Page.Total != 1 || resp.Items[0].Work.Id != w2.GetID() {
		t.Fatalf("按名称过滤结果错误: total=%d", resp.Page.Total)
	}
}

// --- 标签关联的 namespace 维度 ---

func TestListTagsByWorkIdNamespaceDimension(t *testing.T) {
	p, db := newTestLibraryQuery(t, "")
	ctx := context.Background()
	s := seedSite(t, db, "ehentai")
	w := seedWork(t, db, s.GetID(), "1", "甲")

	lt := entity.NewLocalTag()
	lt.LocalTagName = sql.NullString{String: "本地标签", Valid: true}
	if err := localTag.NewRepository(db).Create(ctx, lt); err != nil {
		t.Fatalf("种子本地标签失败: %v", err)
	}
	st := entity.NewSiteTag()
	st.SiteID = sql.NullInt64{Int64: s.GetID(), Valid: true}
	st.SiteTagID = sql.NullString{String: "t1", Valid: true}
	st.SiteTagName = sql.NullString{String: "site tag", Valid: true}
	if err := siteTag.NewRepository(db).Create(ctx, st); err != nil {
		t.Fatalf("种子站点标签失败: %v", err)
	}

	rtRepo := reWorkTag.NewRepository(db)
	localRel := entity.NewReWorkTag()
	localRel.WorkID = sql.NullInt64{Int64: w.GetID(), Valid: true}
	localRel.LocalTagID = sql.NullInt64{Int64: lt.GetID(), Valid: true}
	localRel.TagType = sql.NullInt64{Int64: constant.LOCAL, Valid: true}
	siteRel := entity.NewReWorkTag()
	siteRel.WorkID = sql.NullInt64{Int64: w.GetID(), Valid: true}
	siteRel.SiteTagID = sql.NullInt64{Int64: st.GetID(), Valid: true}
	siteRel.TagType = sql.NullInt64{Int64: constant.SITE, Valid: true}
	siteRel.Namespace = "character"
	if err := rtRepo.CreateBatch(ctx, []*entity.ReWorkTag{localRel, siteRel}); err != nil {
		t.Fatalf("种子标签关联失败: %v", err)
	}

	resp, err := p.listTagsByWorkId(ctx, w.GetID())
	if err != nil {
		t.Fatalf("listTagsByWorkId: %v", err)
	}
	if len(resp.LocalTags) != 1 || resp.LocalTags[0].Tag.Id != lt.GetID() || resp.LocalTags[0].Namespace != "" {
		t.Fatalf("本地标签条目错误: %+v", resp.LocalTags)
	}
	if len(resp.SiteTags) != 1 {
		t.Fatalf("站点标签条目数 = %d, 期望 1", len(resp.SiteTags))
	}
	entry := resp.SiteTags[0]
	if entry.Tag.Id != st.GetID() || entry.Tag.SiteKey != "ehentai" {
		t.Fatalf("站点标签条目映射错误: %+v", entry)
	}
	if entry.Namespace != "character" {
		t.Fatalf("关联级 namespace 缺失: %q", entry.Namespace)
	}
}

// --- 作品集父子导航 ---

func TestWorkSetNavigation(t *testing.T) {
	p, db := newTestLibraryQuery(t, "")
	ctx := context.Background()
	s := seedSite(t, db, "pixiv")
	w := seedWork(t, db, s.GetID(), "1", "甲")

	wsRepo := workSet.NewRepository(db)
	parent := entity.NewWorkSet()
	parent.SiteID = sql.NullInt64{Int64: s.GetID(), Valid: true}
	parent.SiteWorkSetID = sql.NullString{String: "p1", Valid: true}
	parent.SiteWorkSetName = sql.NullString{String: "父集", Valid: true}
	child := entity.NewWorkSet()
	child.SiteID = sql.NullInt64{Int64: s.GetID(), Valid: true}
	child.SiteWorkSetID = sql.NullString{String: "c1", Valid: true}
	child.SiteWorkSetName = sql.NullString{String: "子集", Valid: true}
	if err := wsRepo.CreateBatch(ctx, []*entity.WorkSet{parent, child}); err != nil {
		t.Fatalf("种子作品集失败: %v", err)
	}
	graph := reWorkSetWorkSet.NewRepository(db)
	rel := entity.NewReWorkSetWorkSet()
	rel.ParentWorkSetID = sql.NullInt64{Int64: parent.GetID(), Valid: true}
	rel.ChildWorkSetID = sql.NullInt64{Int64: child.GetID(), Valid: true}
	if err := graph.Create(ctx, rel); err != nil {
		t.Fatalf("种子父子关系失败: %v", err)
	}
	wwRepo := reWorkWorkSet.NewRepository(db)
	member := entity.NewReWorkWorkSet()
	member.WorkID = sql.NullInt64{Int64: w.GetID(), Valid: true}
	member.WorkSetID = sql.NullInt64{Int64: parent.GetID(), Valid: true}
	if err := wwRepo.Create(ctx, member); err != nil {
		t.Fatalf("种子成员关系失败: %v", err)
	}

	// 身份键命中 + id 句柄
	got, err := p.getWorkSetBySiteKey(ctx, "pixiv", "p1")
	if err != nil || got.Id != parent.GetID() {
		t.Fatalf("getWorkSetBySiteKey: %v", err)
	}
	if _, err = p.getWorkSetById(ctx, child.GetID()); err != nil {
		t.Fatalf("getWorkSetById: %v", err)
	}

	// 作品归属的作品集
	sets, err := p.listWorkSetsByWorkId(ctx, w.GetID())
	if err != nil || len(sets.Items) != 1 || sets.Items[0].Id != parent.GetID() {
		t.Fatalf("listWorkSetsByWorkId: %v items=%d", err, len(sets.Items))
	}

	// 父子导航
	childs, err := p.listChildWorkSets(ctx, parent.GetID())
	if err != nil || len(childs.Items) != 1 || childs.Items[0].Id != child.GetID() {
		t.Fatalf("listChildWorkSets: %v items=%d", err, len(childs.Items))
	}
	parents, err := p.listParentWorkSets(ctx, child.GetID())
	if err != nil || len(parents.Items) != 1 || parents.Items[0].Id != parent.GetID() {
		t.Fatalf("listParentWorkSets: %v items=%d", err, len(parents.Items))
	}

	// 锚定作品集未命中 → NotFound
	if _, err = p.listParentWorkSets(ctx, 99999); status.Code(err) != codes.NotFound {
		t.Fatalf("未命中作品集错误码 = %v, 期望 NotFound (err=%v)", status.Code(err), err)
	}
}
