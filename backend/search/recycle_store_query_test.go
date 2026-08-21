package search

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/library-squirrel/backend/base/model/dto"
	domain "github.com/library-squirrel/backend/base/model/entity"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newRecycleStoreTestEnv 内存库（work/resource/resource_store/persistent_store/site 五表）+ 真实搜索仓储
func newRecycleStoreTestEnv(t *testing.T) (*SearchRepository, *gorm.DB) {
	t.Helper()
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	if err := db.AutoMigrate(&domain.Work{}, &domain.Resource{}, &domain.ResourceStore{}, &domain.PersistentStore{}, &domain.Site{}); err != nil {
		t.Fatalf("迁移测试实体失败: %v", err)
	}
	return NewRepository(db), db
}

// rsFixtureEnv 四形态数据面构造器（一次建齐谓词三态用例的数据）
type rsFixtureEnv struct {
	db     *gorm.DB
	workDB int64 // 形态A：work 已软删 × store 软删 × 挂载在（聚合进作品条目，文件条目须排除）
	storeA int64
	workAL int64 // 形态B：work 存活 × store 软删 × 挂载在（MarkInvalid 失效行形态，纳入）
	storeB int64
	storeC int64 // 形态C：离链（无 resource_store 挂载），纳入
	storeD int64 // 形态D：store 活行，排除
}

func buildRsFixture(t *testing.T, db *gorm.DB) *rsFixtureEnv {
	t.Helper()
	env := &rsFixtureEnv{db: db}
	newWork := func() int64 {
		w := domain.NewWork()
		if err := db.Create(w).Error; err != nil {
			t.Fatalf("插 work 失败: %v", err)
		}
		return w.GetID()
	}
	newStore := func(name, ext string) int64 {
		s := domain.NewPersistentStore()
		s.FileName = sql.NullString{String: name, Valid: true}
		s.FilenameExtension = sql.NullString{String: ext, Valid: true}
		s.FilePath = sql.NullString{String: fmt.Sprintf("store/resource/作者/%s%s", name, ext), Valid: true}
		s.CompletedAt = 1
		if err := db.Create(s).Error; err != nil {
			t.Fatalf("插 store 失败: %v", err)
		}
		return s.GetID()
	}
	mount := func(workId, storeId int64) {
		r := domain.NewResource()
		r.WorkID = workId
		r.ResourceType = "image"
		if err := db.Create(r).Error; err != nil {
			t.Fatalf("插 resource 失败: %v", err)
		}
		rs := domain.NewResourceStore()
		rs.ResourceID = r.GetID()
		rs.StoreType = domain.StoreTypeImage
		rs.Generation = domain.GenerationDownloaded
		rs.StoreID = storeId
		if err := db.Create(rs).Error; err != nil {
			t.Fatalf("插 resource_store 失败: %v", err)
		}
	}
	softDeleteStore := func(storeId int64, backupId int64) {
		if err := db.Exec("UPDATE persistent_store SET deleted_at = 2000, backup_id = ? WHERE id = ?", backupId, storeId).Error; err != nil {
			t.Fatalf("软删 store 失败: %v", err)
		}
	}

	// 形态A：work 软删聚合（store 软删带备份）
	env.workDB = newWork()
	env.storeA = newStore("聚合作品主图", ".jpg")
	mount(env.workDB, env.storeA)
	softDeleteStore(env.storeA, 901)
	if err := db.Exec("UPDATE work SET deleted_at = 1500, site_work_name = ? WHERE id = ?", "已删作品", env.workDB).Error; err != nil {
		t.Fatalf("软删 work 失败: %v", err)
	}

	// 形态B：work 活 × store 软删挂载（MarkInvalid 失效行，backup_id=0）+ 形态B'：带备份（可复原态）
	env.workAL = newWork()
	if err := db.Exec("UPDATE work SET site_work_name = ? WHERE id = ?", "存活作品", env.workAL).Error; err != nil {
		t.Fatalf("更新 work 名失败: %v", err)
	}
	env.storeB = newStore("失效缩略图", ".png")
	mount(env.workAL, env.storeB)
	softDeleteStore(env.storeB, 0)

	storeB2 := newStore("可复原主图", ".jpg")
	mount(env.workAL, storeB2)
	softDeleteStore(storeB2, 902)

	// 形态C：离链（无挂载）
	env.storeC = newStore("离链残迹", ".mp4")
	softDeleteStore(env.storeC, 0)

	// 形态D：活行
	env.storeD = newStore("活行文件", ".mp4")
	return env
}

// listIds 取一次查询的条目 ID 集
func listIds(t *testing.T, repo *SearchRepository, query *dto.RecycleStorePageQuery) map[int64]struct{} {
	t.Helper()
	items, _, err := repo.QueryRecycleStorePage(context.Background(), 1, 50, query)
	if err != nil {
		t.Fatalf("查询文件条目失败: %v", err)
	}
	set := make(map[int64]struct{}, len(items))
	for _, item := range items {
		set[item.ID] = struct{}{}
	}
	return set
}

// TestRecycleStorePredicateThreeStates 谓词三态锚定：work 已删聚合行排除（保护作品条目复原能力）；
// work 活挂载行纳入（MarkInvalid 失效行由此获得可见性）；离链行纳入（孤儿自愈）；活行排除
func TestRecycleStorePredicateThreeStates(t *testing.T) {
	repo, db := newRecycleStoreTestEnv(t)
	env := buildRsFixture(t, db)
	set := listIds(t, repo, nil)

	if _, ok := set[env.storeA]; ok {
		t.Fatalf("work 已删聚合行（storeA=%d）应被谓词排除（归作品条目）", env.storeA)
	}
	if _, ok := set[env.storeB]; !ok {
		t.Fatalf("work 活挂载的失效行（storeB=%d，MarkInvalid 形态）应纳入文件条目", env.storeB)
	}
	if _, ok := set[env.storeC]; !ok {
		t.Fatalf("离链软删行（storeC=%d）应纳入文件条目（孤儿自愈）", env.storeC)
	}
	if _, ok := set[env.storeD]; ok {
		t.Fatalf("活行（storeD=%d）不应进入文件条目", env.storeD)
	}
}

// TestRecycleStoreCanRestoreAndContext 可复原性与作品上下文组装：
// 有备份+挂载活作品 → CanRestore 且带作品上下文；无备份挂载 → 不可复原；有备份离链 → 不可复原且无作品上下文
func TestRecycleStoreCanRestoreAndContext(t *testing.T) {
	repo, db := newRecycleStoreTestEnv(t)
	env := buildRsFixture(t, db)
	items, _, err := repo.QueryRecycleStorePage(context.Background(), 1, 50, nil)
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	byId := make(map[int64]*dto.RecycleStoreDTO, len(items))
	for _, item := range items {
		byId[item.ID] = item
	}
	// 形态B'（storeB2，backup_id=902 挂载活作品）——查 fixture 里 storeB2 的 ID：经 workAL 挂载带备份的那行
	var storeB2 int64
	if err := db.Raw("SELECT id FROM persistent_store WHERE backup_id = 902").Scan(&storeB2).Error; err != nil {
		t.Fatalf("查 storeB2 失败: %v", err)
	}
	if item, ok := byId[storeB2]; ok {
		if !item.CanRestore || !item.HasBackup {
			t.Fatalf("有备份+挂载活作品的行应可复原（backup_id=902），实际 CanRestore=%v HasBackup=%v", item.CanRestore, item.HasBackup)
		}
		if item.WorkId == nil || *item.WorkId != env.workAL || item.WorkName != "存活作品" {
			t.Fatalf("有主链行应带作品上下文（workId=%d），实际 %v / %q", env.workAL, item.WorkId, item.WorkName)
		}
	} else {
		t.Fatalf("storeB2 应在文件条目中")
	}
	if item, ok := byId[env.storeB]; ok {
		if item.CanRestore || item.HasBackup {
			t.Fatalf("无备份失效行（MarkInvalid 形态）不可复原，实际 CanRestore=%v HasBackup=%v", item.CanRestore, item.HasBackup)
		}
	} else {
		t.Fatalf("storeB 应在文件条目中")
	}
	if item, ok := byId[env.storeC]; ok {
		if item.CanRestore {
			t.Fatalf("离链行（无挂载）不可复原")
		}
		if item.WorkId != nil || item.WorkName != "" {
			t.Fatalf("离链行不应带作品上下文，实际 WorkId=%v WorkName=%q", item.WorkId, item.WorkName)
		}
	} else {
		t.Fatalf("storeC 应在文件条目中")
	}
}

// TestRecycleStoreConditions 文件域条件过滤：文件名模糊 / 媒体类型扩展名集 / 备份状态 / 作品名（离链行天然不命中）
func TestRecycleStoreConditions(t *testing.T) {
	repo, db := newRecycleStoreTestEnv(t)
	env := buildRsFixture(t, db)

	// 文件名模糊
	set := listIds(t, repo, &dto.RecycleStorePageQuery{FileName: "离链"})
	if len(set) != 1 {
		t.Fatalf("文件名模糊「离链」应仅命中 1 行，实际 %d", len(set))
	}
	if _, ok := set[env.storeC]; !ok {
		t.Fatalf("文件名模糊应命中 storeC")
	}

	// 媒体类型→扩展名（视频 .mp4：离链行+活行，活行被谓词排除后仅剩离链行）
	media := int(dto.MediaTypeVideo)
	set = listIds(t, repo, &dto.RecycleStorePageQuery{MediaType: &media})
	if len(set) != 1 {
		t.Fatalf("媒体类型视频应仅命中离链 mp4 行，实际 %d", len(set))
	}
	if _, ok := set[env.storeC]; !ok {
		t.Fatalf("媒体类型视频应命中 storeC")
	}

	// 备份状态：有备份
	set = listIds(t, repo, &dto.RecycleStorePageQuery{HasBackup: ptrBool(true)})
	if len(set) != 1 {
		t.Fatalf("有备份应仅命中 storeB2（聚合行被谓词排除），实际 %d", len(set))
	}
	// 备份状态：无备份（MarkInvalid 行）
	set = listIds(t, repo, &dto.RecycleStorePageQuery{HasBackup: ptrBool(false)})
	if _, ok := set[env.storeB]; !ok {
		t.Fatalf("无备份过滤应命中 MarkInvalid 形态行 storeB")
	}
	if _, ok := set[env.storeC]; !ok {
		t.Fatalf("无备份过滤应命中离链行 storeC")
	}

	// 作品名（挂载活作品 site_work_name；离链行天然不命中）
	set = listIds(t, repo, &dto.RecycleStorePageQuery{WorkName: "存活"})
	if len(set) != 2 {
		t.Fatalf("作品名「存活」应命中该作品挂载的两行（storeB/storeB2），实际 %d", len(set))
	}
}

// TestListRecycleStoreIdsDeletedBefore TTL 圈定谓词：work 已删聚合行即使过期也不被圈定
// （保护作品条目复原能力）；其余形态过期被圈定、未过期不圈
func TestListRecycleStoreIdsDeletedBefore(t *testing.T) {
	repo, db := newRecycleStoreTestEnv(t)
	env := buildRsFixture(t, db)

	// fixture 各软删行 deleted_at=2000；阈值 2500（全部过期）
	ids, err := repo.ListRecycleStoreIdsDeletedBefore(context.Background(), 2500)
	if err != nil {
		t.Fatalf("TTL 圈定查询失败: %v", err)
	}
	idSet := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		idSet[id] = struct{}{}
	}
	if _, ok := idSet[env.storeA]; ok {
		t.Fatalf("work 已删聚合行（storeA）不应被 TTL 圈定（归作品条目第一轮级联处理）")
	}
	for _, id := range []int64{env.storeB, env.storeC} {
		if _, ok := idSet[id]; !ok {
			t.Fatalf("过期文件条目 %d 应被圈定", id)
		}
	}
	// 阈值 1000（全部未过期）
	ids, err = repo.ListRecycleStoreIdsDeletedBefore(context.Background(), 1000)
	if err != nil {
		t.Fatalf("TTL 圈定查询失败: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("未过期不应圈定，实际圈定 %d 行", len(ids))
	}
}

// ptrBool 布尔取指针（条件字段可空语义用）
func ptrBool(v bool) *bool {
	return &v
}
