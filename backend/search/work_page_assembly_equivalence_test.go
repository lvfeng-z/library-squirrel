package search

import (
	"context"
	"database/sql"
	"reflect"
	"testing"

	"github.com/library-squirrel/backend/base/constant"
	dto2 "github.com/library-squirrel/backend/base/model/dto"
	domain "github.com/library-squirrel/backend/base/model/entity"

	"gorm.io/gorm"
)

// fullSpectrumFixture 主页展示组装的全谱数据面：全谱作品（站点 + SITE/LOCAL 作者 + local/site 两类标签 +
// image 资源挂 image/thumbnail/指向软删 store 行的 image 共三代挂载）与音频作品
// （audio 资源挂 audioMain/thumbnail，无站点）
type fullSpectrumFixture struct {
	fullWorkId    int64
	audioWorkId   int64
	deadStorePath string // 指向软删 persistent_store 行的挂载对应 file_path（不应入展示面）
	imageMainPath string // 全谱作品展示主体（image 角色）file_path
	audioMainPath string // 音频作品展示主体（audioMain 角色）file_path
}

// buildFullSpectrumFixture 种全谱数据；软删 store 行经裸 UPDATE 打 deleted_at（模拟替换/merge 残留形态：
// resource_store 关联保留、指向已删 persistent_store 行）
func buildFullSpectrumFixture(t *testing.T, db *gorm.DB) *fullSpectrumFixture {
	t.Helper()
	siteRow := domain.NewSite()
	siteRow.SiteKey = "pixiv"
	siteRow.SiteName = sql.NullString{String: "pixiv", Valid: true}
	if err := db.Create(siteRow).Error; err != nil {
		t.Fatalf("插站点失败: %v", err)
	}

	siteAuthorRow := domain.NewSiteAuthor()
	siteAuthorRow.SiteID = sql.NullInt64{Int64: siteRow.GetID(), Valid: true}
	siteAuthorRow.SiteAuthorID = sql.NullString{String: "sa-1", Valid: true}
	siteAuthorRow.AuthorName = sql.NullString{String: "站点作者甲", Valid: true}
	if err := db.Create(siteAuthorRow).Error; err != nil {
		t.Fatalf("插站点作者失败: %v", err)
	}

	localAuthorRow := domain.NewLocalAuthor()
	localAuthorRow.AuthorName = sql.NullString{String: "本地作者乙", Valid: true}
	if err := db.Create(localAuthorRow).Error; err != nil {
		t.Fatalf("插本地作者失败: %v", err)
	}

	localTagRow := domain.NewLocalTag()
	localTagRow.LocalTagName = sql.NullString{String: "本地标签", Valid: true}
	if err := db.Create(localTagRow).Error; err != nil {
		t.Fatalf("插本地标签失败: %v", err)
	}

	siteTagRow := domain.NewSiteTag()
	siteTagRow.SiteID = sql.NullInt64{Int64: siteRow.GetID(), Valid: true}
	siteTagRow.SiteTagID = sql.NullString{String: "st-1", Valid: true}
	siteTagRow.SiteTagName = sql.NullString{String: "站点标签", Valid: true}
	siteTagRow.Namespace = sql.NullString{String: "character", Valid: true}
	// 站点标签绑定本地标签（SiteTags 嵌套 LocalTag 的数据源）
	siteTagRow.LocalTagID = sql.NullInt64{Int64: localTagRow.GetID(), Valid: true}
	if err := db.Create(siteTagRow).Error; err != nil {
		t.Fatalf("插站点标签失败: %v", err)
	}

	newWork := func(name string, siteId int64) int64 {
		w := domain.NewWork()
		w.SiteWorkName = sql.NullString{String: name, Valid: true}
		if siteId > 0 {
			w.SiteID = sql.NullInt64{Int64: siteId, Valid: true}
		}
		if err := db.Create(w).Error; err != nil {
			t.Fatalf("插作品失败: %v", err)
		}
		return w.GetID()
	}
	fullWorkId := newWork("全谱作品", siteRow.GetID())
	audioWorkId := newWork("音频作品", 0)

	linkSiteAuthor := domain.NewReWorkAuthor()
	linkSiteAuthor.AuthorType = sql.NullInt64{Int64: constant.SITE, Valid: true}
	linkSiteAuthor.WorkID = sql.NullInt64{Int64: fullWorkId, Valid: true}
	linkSiteAuthor.SiteAuthorID = sql.NullInt64{Int64: siteAuthorRow.GetID(), Valid: true}
	linkSiteAuthor.RoleName = sql.NullString{String: "作者", Valid: true}
	linkSiteAuthor.SortOrder = sql.NullInt64{Int64: 1, Valid: true}
	if err := db.Create(linkSiteAuthor).Error; err != nil {
		t.Fatalf("挂站点作者关联失败: %v", err)
	}

	linkLocalAuthor := domain.NewReWorkAuthor()
	linkLocalAuthor.AuthorType = sql.NullInt64{Int64: constant.LOCAL, Valid: true}
	linkLocalAuthor.WorkID = sql.NullInt64{Int64: fullWorkId, Valid: true}
	linkLocalAuthor.LocalAuthorID = sql.NullInt64{Int64: localAuthorRow.GetID(), Valid: true}
	if err := db.Create(linkLocalAuthor).Error; err != nil {
		t.Fatalf("挂本地作者关联失败: %v", err)
	}

	linkLocalTag := domain.NewReWorkTag()
	linkLocalTag.WorkID = sql.NullInt64{Int64: fullWorkId, Valid: true}
	linkLocalTag.TagType = sql.NullInt64{Int64: constant.LOCAL, Valid: true}
	linkLocalTag.LocalTagID = sql.NullInt64{Int64: localTagRow.GetID(), Valid: true}
	if err := db.Create(linkLocalTag).Error; err != nil {
		t.Fatalf("挂本地标签关联失败: %v", err)
	}

	linkSiteTag := domain.NewReWorkTag()
	linkSiteTag.WorkID = sql.NullInt64{Int64: fullWorkId, Valid: true}
	linkSiteTag.TagType = sql.NullInt64{Int64: constant.SITE, Valid: true}
	linkSiteTag.SiteTagID = sql.NullInt64{Int64: siteTagRow.GetID(), Valid: true}
	linkSiteTag.Namespace = sql.NullString{String: "character", Valid: true} // site 关联镜像所指 site_tag.namespace
	if err := db.Create(linkSiteTag).Error; err != nil {
		t.Fatalf("挂站点标签关联失败: %v", err)
	}

	newStore := func(path, ext string) int64 {
		ps := domain.NewPersistentStore()
		ps.FilePath = sql.NullString{String: path, Valid: true}
		ps.FileName = sql.NullString{String: path, Valid: true}
		ps.FilenameExtension = sql.NullString{String: ext, Valid: true}
		ps.CompletedAt = 1700000000000
		if err := db.Create(ps).Error; err != nil {
			t.Fatalf("插 store 行失败: %v", err)
		}
		return ps.GetID()
	}
	mount := func(resourceId int64, storeType string, seq int, storeId int64) {
		rs := domain.NewResourceStore()
		rs.ResourceID = resourceId
		rs.StoreType = storeType
		rs.Generation = domain.GenerationDownloaded
		rs.StoreID = storeId
		rs.StoreSeq = seq
		if err := db.Create(rs).Error; err != nil {
			t.Fatalf("挂 store 关联失败: %v", err)
		}
	}

	f := &fullSpectrumFixture{
		fullWorkId:  fullWorkId,
		audioWorkId: audioWorkId,
	}

	// 全谱作品：image 资源挂三代 store——image 活行（展示主体）、thumbnail 活行、image 序 1 指向软删行
	f.imageMainPath = "store/resource/作者甲/全谱作品.jpg"
	imageStoreId := newStore(f.imageMainPath, ".jpg")
	thumbStoreId := newStore("store/resource/作者甲/全谱作品_thumbnail_000.jpg", ".jpg")
	f.deadStorePath = "store/resource/作者甲/全谱作品_旧代.jpg"
	deadStoreId := newStore(f.deadStorePath, ".jpg")

	imageRes := domain.NewResource()
	imageRes.WorkID = fullWorkId
	imageRes.ResourceType = domain.ResourceTypeImage
	imageRes.ResourceComplete = sql.NullInt64{Int64: 1, Valid: true}
	if err := db.Create(imageRes).Error; err != nil {
		t.Fatalf("插 image 资源失败: %v", err)
	}
	mount(imageRes.GetID(), domain.StoreTypeImage, 0, imageStoreId)
	mount(imageRes.GetID(), domain.StoreTypeThumbnail, 0, thumbStoreId)
	mount(imageRes.GetID(), domain.StoreTypeImage, 1, deadStoreId)
	if err := db.Exec(`UPDATE persistent_store SET deleted_at = 1700000000000 WHERE id = ?`, deadStoreId).Error; err != nil {
		t.Fatalf("软删 store 行失败: %v", err)
	}

	// 音频作品：audio 资源挂 audioMain（可播放主体）+ thumbnail
	f.audioMainPath = "store/resource/作者乙/音频作品.mp3"
	audioMainStoreId := newStore(f.audioMainPath, ".mp3")
	audioThumbStoreId := newStore("store/resource/作者乙/音频作品_thumbnail_000.jpg", ".jpg")

	audioRes := domain.NewResource()
	audioRes.WorkID = audioWorkId
	audioRes.ResourceType = domain.ResourceTypeAudio
	audioRes.ResourceComplete = sql.NullInt64{Int64: 1, Valid: true}
	if err := db.Create(audioRes).Error; err != nil {
		t.Fatalf("插 audio 资源失败: %v", err)
	}
	mount(audioRes.GetID(), domain.StoreTypeAudioMain, 0, audioMainStoreId)
	mount(audioRes.GetID(), domain.StoreTypeThumbnail, 0, audioThumbStoreId)

	return f
}

// TestQueryWorkPageAssemblyEquivalence 主页链（service.QueryWorkPage：条件圈定作品 ID + work 模块组装）
// 产出与组装单产源 GetFullWorkInfoByIds 直调逐字段一致；并锚定三个组装行为：
// 音频作品展示主体非空（ResolvePrimaryStore 按类型 PrimaryRoles 含 audioMain）、
// 软删 store 行不入资源 stores（活行过滤）、作品顶层站点被填充（站点批量查询 Phase）
func TestQueryWorkPageAssemblyEquivalence(t *testing.T) {
	svc, workSvc, repo, db := newWorkPageServiceEnv(t)
	f := buildFullSpectrumFixture(t, db)
	ctx := context.Background()

	page, err := svc.QueryWorkPage(ctx, 1, 10, nil)
	if err != nil {
		t.Fatalf("主页链查询失败: %v", err)
	}
	if page.DataCount != 2 || len(page.Data) != 2 {
		t.Fatalf("应查得两作品，实际 total=%d items=%d", page.DataCount, len(page.Data))
	}

	// 同条件圈定作品 ID 后直调组装单产源，主页链产出须与之逐字段一致
	ids, _, err := repo.QueryWorkIdPage(ctx, 1, 10, nil)
	if err != nil {
		t.Fatalf("圈定作品 ID 失败: %v", err)
	}
	direct, err := workSvc.GetFullWorkInfoByIds(ctx, ids)
	if err != nil {
		t.Fatalf("直调组装失败: %v", err)
	}
	if len(direct) != len(page.Data) {
		t.Fatalf("主页链与直调条数不一致: %d vs %d", len(page.Data), len(direct))
	}
	if !reflect.DeepEqual(page.Data, direct) {
		t.Fatal("主页链产出与组装单产源直调不一致（逐字段 DeepEqual 失败）")
	}

	var fullItem, audioItem *dto2.WorkFullDTO
	for _, it := range page.Data {
		switch it.Work.GetId() {
		case f.fullWorkId:
			fullItem = it
		case f.audioWorkId:
			audioItem = it
		}
	}
	if fullItem == nil || audioItem == nil {
		t.Fatalf("两作品均应出现在结果: 全谱=%v 音频=%v", fullItem != nil, audioItem != nil)
	}

	// 顶层站点填充（有站点作品）
	if fullItem.Site == nil || fullItem.Site.SiteKey != "pixiv" {
		t.Fatalf("全谱作品顶层站点应为 pixiv，实际 %+v", fullItem.Site)
	}

	// 全谱维度落位：SITE/LOCAL 作者、local/site 标签（站点标签含嵌套站点与绑定本地标签）
	if len(fullItem.SiteAuthors) == 0 {
		t.Fatalf("全谱作品应挂站点作者，实际 %d 个", len(fullItem.SiteAuthors))
	}
	if fullItem.SiteAuthors[0].RoleName != "作者" {
		t.Fatalf("站点作者角色名应为「作者」，实际 %q", fullItem.SiteAuthors[0].RoleName)
	}
	if len(fullItem.LocalAuthors) == 0 {
		t.Fatalf("全谱作品应挂本地作者，实际 %d 个", len(fullItem.LocalAuthors))
	}
	if len(fullItem.LocalTags) == 0 {
		t.Fatalf("全谱作品应挂本地标签，实际 %d 个", len(fullItem.LocalTags))
	}
	if len(fullItem.SiteTags) == 0 {
		t.Fatalf("全谱作品应挂站点标签，实际 %d 个", len(fullItem.SiteTags))
	}
	if fullItem.SiteTags[0].Site == nil || fullItem.SiteTags[0].Site.SiteKey != "pixiv" {
		t.Fatalf("站点标签应嵌套站点 pixiv，实际 %+v", fullItem.SiteTags[0].Site)
	}
	if fullItem.SiteTags[0].LocalTag == nil {
		t.Fatal("站点标签应嵌套绑定的本地标签")
	}

	// 软删 store 行不入资源 stores：仅两代活行（image + thumbnail）入展示面，软删行 file_path 不现
	if fullItem.Resource == nil {
		t.Fatal("全谱作品应挂资源")
	}
	if len(fullItem.Resource.Stores) != 2 {
		t.Fatalf("全谱作品资源应仅含两代活行 store，实际 %d 条", len(fullItem.Resource.Stores))
	}
	for _, s := range fullItem.Resource.Stores {
		if s.Store != nil && s.Store.FilePath != nil && *s.Store.FilePath == f.deadStorePath {
			t.Fatal("软删 store 行不应入资源 stores")
		}
	}
	// image 类型展示主体派生（PrimaryRoles 首选 image）
	if fullItem.Resource.WorkStore == nil || fullItem.Resource.WorkStore.FilePath == nil || *fullItem.Resource.WorkStore.FilePath != f.imageMainPath {
		t.Fatalf("全谱作品展示主体应为 image 活行 %s，实际 %+v", f.imageMainPath, fullItem.Resource.WorkStore)
	}

	// 音频作品展示主体非空（PrimaryRoles 含 audioMain——按类型规约派生，而非硬编码角色链）
	if audioItem.Resource == nil {
		t.Fatal("音频作品应挂资源")
	}
	if audioItem.Resource.WorkStore == nil || audioItem.Resource.WorkStore.FilePath == nil || *audioItem.Resource.WorkStore.FilePath != f.audioMainPath {
		t.Fatalf("音频作品展示主体应为 audioMain %s，实际 %+v", f.audioMainPath, audioItem.Resource.WorkStore)
	}
}
