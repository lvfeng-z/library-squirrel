//go:build cgo

package resource

import (
	"context"
	"testing"

	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/migration"
	"gorm.io/gorm"
)

// 依赖 gorm.io/driver/sqlite 的 CGO sqlite，纯 Go 环境自动跳过本文件。
// 验证 ListStoreTypeSetsByWorkIds 的两跳批量拼接（work→resource→resource_store 行）与分组语义。

// newTestStoreTypeSetDB 建立隔离的内存 SQLite（每测试独立库）+ 建表，返回 Service 与 db。
// 活行角色集合查询 JOIN persistent_store（软删残留代不算「作品拥有该角色」），故三表齐建
func newTestStoreTypeSetDB(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Fatalf("打开内存 SQLite 失败: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("获取底层 sql.DB 失败: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	// 作品行种子（resource.work_id 外键防线，fixture 用 workId 100/200/300）
	if err := db.Exec("INSERT INTO work (id, create_time, update_time, deleted_at) VALUES (100, 0, 0, 0), (200, 0, 0, 0), (300, 0, 0, 0)").Error; err != nil {
		t.Fatalf("建作品种子失败: %v", err)
	}
	if err := db.AutoMigrate(&domain.Resource{}, &domain.ResourceStore{}, &domain.PersistentStore{}); err != nil {
		t.Fatalf("AutoMigrate 失败: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return NewService(NewRepository(db), NewResourceStoreRepository(db)), db
}

func mustCreateResource(t *testing.T, s *Service, ctx context.Context, workId int64) *domain.Resource {
	t.Helper()
	r := domain.NewResource()
	r.WorkID = workId
	r.ResourceType = "image"
	if err := s.repo.Create(ctx, r); err != nil {
		t.Fatalf("创建 resource 失败: %v", err)
	}
	return r
}

// mustCreateStore 建一行活 persistent_store + 指向它的关联（角色集合查询按行活性 JOIN，行须真实存在）
func mustCreateStore(t *testing.T, db *gorm.DB, ctx context.Context, resourceId int64, storeType string) {
	t.Helper()
	ps := domain.NewPersistentStore()
	ps.CompletedAt = 1
	if err := db.Create(ps).Error; err != nil {
		t.Fatalf("创建 persistent_store 失败: %v", err)
	}
	rs := domain.NewResourceStore()
	rs.ResourceID = resourceId
	rs.StoreType = storeType
	rs.Generation = domain.GenerationDownloaded
	rs.StoreID = ps.GetID()
	if err := db.Create(rs).Error; err != nil {
		t.Fatalf("创建 resource_store 失败: %v", err)
	}
}

func sameStringSet(a, b map[string]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			return false
		}
	}
	return true
}

// TestListStoreTypeSetsByWorkIds 验证：多作品多资源多行正确聚合为 {workId: store_type 集合}；
// 空 workIds/无资源作品分别返回空 map、不出键；零行作品出空集合键。
func TestListStoreTypeSetsByWorkIds(t *testing.T) {
	s, db := newTestStoreTypeSetDB(t)
	ctx := context.Background()

	// work 100：resource 1（image+thumbnail 行）、resource 2（videoTrack 行）
	r1 := mustCreateResource(t, s, ctx, 100)
	r2 := mustCreateResource(t, s, ctx, 100)
	// work 200：resource 3（仅 thumbnail 行）
	r3 := mustCreateResource(t, s, ctx, 200)
	// work 300：resource 4（零 store 行）
	_ = mustCreateResource(t, s, ctx, 300)

	mustCreateStore(t, db, ctx, r1.ID, "image")
	mustCreateStore(t, db, ctx, r1.ID, "thumbnail")
	mustCreateStore(t, db, ctx, r2.ID, "videoTrack")
	mustCreateStore(t, db, ctx, r3.ID, "thumbnail")

	got, err := s.ListStoreTypeSetsByWorkIds(ctx, []int64{100, 200, 300, 999})
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("有资源的作品应 3 个（999 无资源不出键），got %d: %v", len(got), got)
	}
	expect := map[int64]map[string]struct{}{
		100: {"image": {}, "thumbnail": {}, "videoTrack": {}},
		200: {"thumbnail": {}},
		300: {},
	}
	for workId, want := range expect {
		set, ok := got[workId]
		if !ok {
			t.Fatalf("work %d 应有键", workId)
		}
		if !sameStringSet(set, want) {
			t.Fatalf("work %d 集合不符: got %v want %v", workId, set, want)
		}
	}

	// 空 workIds 返回空 map 非 nil
	empty, err := s.ListStoreTypeSetsByWorkIds(ctx, nil)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("空 workIds 应返回空 map,nil 错误，got %v,%v", empty, err)
	}
}

// TestListStoreTypeSetsByWorkIds_SameTypeAcrossResources 验证同一作品多资源重复 store_type 去重。
func TestListStoreTypeSetsByWorkIds_SameTypeAcrossResources(t *testing.T) {
	s, db := newTestStoreTypeSetDB(t)
	ctx := context.Background()
	r1 := mustCreateResource(t, s, ctx, 100)
	r2 := mustCreateResource(t, s, ctx, 100)
	for _, r := range []*domain.Resource{r1, r2} {
		mustCreateStore(t, db, ctx, r.ID, "image")
	}
	got, err := s.ListStoreTypeSetsByWorkIds(ctx, []int64{100})
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	set := got[100]
	if !sameStringSet(set, map[string]struct{}{"image": {}}) {
		t.Fatalf("同类型应去重为 {image}，got %v", set)
	}
}
