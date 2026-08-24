package resource

import (
	"context"
	"database/sql"
	"testing"

	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/migration"

	"gorm.io/gorm"
)

// livenessEnv 内存库（resource/resource_store/persistent_store 三表）+ 真实仓储与服务——
// 关联保留形态的消费面焦点：活行过滤计数 / GetByType 活性 / 挂载只摘活行关联
type livenessEnv struct {
	svc  *Service
	repo *ResourceStoreRepository
	db   *gorm.DB
}

func newLivenessEnv(t *testing.T) *livenessEnv {
	t.Helper()
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	// 作品行种子（resource.work_id 外键防线，fixture 统一 WorkID=1）
	if err := db.Exec("INSERT INTO work (id, create_time, update_time, deleted_at) VALUES (1, 0, 0, 0)").Error; err != nil {
		t.Fatalf("建作品种子失败: %v", err)
	}
	if err := db.AutoMigrate(&domain.Resource{}, &domain.ResourceStore{}, &domain.PersistentStore{}); err != nil {
		t.Fatalf("迁移测试实体失败: %v", err)
	}
	repo := NewResourceStoreRepository(db)
	svc := NewService(NewRepository(db), repo)
	return &livenessEnv{svc: svc, repo: repo, db: db}
}

// seedDualAssociation 造双关联形态：同 (resource,role,seq) 键下一条死行关联 + 一条活行关联
// 返回 resourceId 与两行 store ID
func (e *livenessEnv) seedDualAssociation(t *testing.T, role string) (resourceId, deadId, liveId int64) {
	t.Helper()
	res := domain.NewResource()
	res.WorkID = 1
	res.ResourceType = "image"
	if err := e.db.Create(res).Error; err != nil {
		t.Fatalf("插 resource 失败: %v", err)
	}
	dead := domain.NewPersistentStore()
	dead.FilePath = sql.NullString{String: "store/resource/a/同键同路径.png", Valid: true}
	dead.CompletedAt = 1
	if err := e.db.Create(dead).Error; err != nil {
		t.Fatalf("插死行失败: %v", err)
	}
	// 备份清单行种子（persistent_store.backup_id 外键防线——死行引用须指向存在行）
	if err := e.db.Exec("INSERT INTO backup (id, create_time, update_time) VALUES (91, 0, 0)").Error; err != nil {
		t.Fatalf("建备份种子失败: %v", err)
	}
	if err := e.db.Exec("UPDATE persistent_store SET deleted_at = 1000, backup_id = 91 WHERE id = ?", dead.GetID()).Error; err != nil {
		t.Fatalf("软删失败: %v", err)
	}
	live := domain.NewPersistentStore()
	live.FilePath = sql.NullString{String: "store/resource/a/同键同路径.png", Valid: true}
	live.CompletedAt = 1
	if err := e.db.Create(live).Error; err != nil {
		t.Fatalf("插活行失败: %v", err)
	}
	deadRs := domain.NewResourceStore()
	deadRs.ResourceID = res.GetID()
	deadRs.StoreType = role
	deadRs.StoreSeq = 0
	deadRs.StoreID = dead.GetID()
	if err := e.db.Create(deadRs).Error; err != nil {
		t.Fatalf("插死行关联失败: %v", err)
	}
	liveRs := domain.NewResourceStore()
	liveRs.ResourceID = res.GetID()
	liveRs.StoreType = role
	liveRs.StoreSeq = 0
	liveRs.StoreID = live.GetID()
	if err := e.db.Create(liveRs).Error; err != nil {
		t.Fatalf("插活行关联失败: %v", err)
	}
	return res.GetID(), dead.GetID(), live.GetID()
}

// TestCountAliveTypesDualAssociation 双关联计数：同 role 死+活两关联只计活行 1 次
func TestCountAliveTypesDualAssociation(t *testing.T) {
	env := newLivenessEnv(t)
	resourceId, deadId, liveId := env.seedDualAssociation(t, domain.StoreTypeImage)
	_ = deadId

	counts, err := env.repo.CountAliveTypesByResourceId(context.Background(), resourceId)
	if err != nil {
		t.Fatalf("计数失败: %v", err)
	}
	if counts[domain.StoreTypeImage] != 1 {
		t.Fatalf("双关联下 image 应只计活行 1 次，实际 %d", counts[domain.StoreTypeImage])
	}
	if liveId <= 0 {
		t.Fatal("活行 ID 异常")
	}
}

// TestRecomputeResourceCompleteUnderDualAssociation 完整度重算（活行过滤）：
// image 类型资源在双关联形态下完整度=1（不因死行关联误判超量）
func TestRecomputeResourceCompleteUnderDualAssociation(t *testing.T) {
	env := newLivenessEnv(t)
	resourceId, _, _ := env.seedDualAssociation(t, domain.StoreTypeImage)
	env.db.Exec("UPDATE resource SET resource_complete = 2 WHERE id = ?", resourceId)

	env.svc.RecomputeResourceComplete(context.Background(), resourceId)

	var complete int64
	if err := env.db.Raw("SELECT resource_complete FROM resource WHERE id = ?", resourceId).Scan(&complete).Error; err != nil {
		t.Fatalf("查完整度失败: %v", err)
	}
	if complete != 1 {
		t.Fatalf("双关联形态下完整度应为 1（完整），实际 %d", complete)
	}
}

// TestGetByTypeSkipsDeadAssociation GetByType 活性：双关联下命中活行关联；
// 仅剩死行关联时 NotFound（merge 幂等/缺轨判定取活代）
func TestGetByTypeSkipsDeadAssociation(t *testing.T) {
	env := newLivenessEnv(t)
	resourceId, deadId, liveId := env.seedDualAssociation(t, domain.StoreTypeImage)

	rs, err := env.repo.GetByType(context.Background(), resourceId, domain.StoreTypeImage)
	if err != nil {
		t.Fatalf("双关联下 GetByType 应命中活行: %v", err)
	}
	if rs.StoreID != liveId {
		t.Fatalf("应命中活行(id=%d)，实际 id=%d", liveId, rs.StoreID)
	}

	// 删掉活行（含其关联）→ 仅剩死行关联：NotFound
	env.db.Exec("DELETE FROM resource_store WHERE store_id = ?", liveId)
	env.db.Exec("UPDATE persistent_store SET deleted_at = 0 WHERE id = ?", liveId)
	if _, err := env.repo.GetByType(context.Background(), resourceId, domain.StoreTypeImage); err != gorm.ErrRecordNotFound {
		t.Fatalf("仅死行关联时应 NotFound，实际 err=%v", err)
	}
	_ = deadId
}

// TestDeleteByResourceIdAndTypesKeepsDeadAssoc 挂载只摘活行关联：
// 同 role 的死行关联保留（关联保留形态的生产侧约定），活行关联被摘
func TestDeleteByResourceIdAndTypesKeepsDeadAssoc(t *testing.T) {
	env := newLivenessEnv(t)
	resourceId, deadId, liveId := env.seedDualAssociation(t, domain.StoreTypeImage)

	if err := env.repo.DeleteByResourceIdAndTypes(context.Background(), resourceId, []string{domain.StoreTypeImage}); err != nil {
		t.Fatalf("摘除失败: %v", err)
	}
	var deadAssoc, liveAssoc int64
	env.db.Raw("SELECT COUNT(*) FROM resource_store WHERE store_id = ?", deadId).Scan(&deadAssoc)
	env.db.Raw("SELECT COUNT(*) FROM resource_store WHERE store_id = ?", liveId).Scan(&liveAssoc)
	if deadAssoc != 1 {
		t.Fatalf("死行关联应保留（关联保留），实际剩 %d", deadAssoc)
	}
	if liveAssoc != 0 {
		t.Fatalf("活行关联应被摘除，实际剩 %d", liveAssoc)
	}
}

// TestDeleteByStoreIds 按 store ID 摘关联（失败还原链清理新建行关联的通路）
func TestDeleteByStoreIds(t *testing.T) {
	env := newLivenessEnv(t)
	resourceId, _, liveId := env.seedDualAssociation(t, domain.StoreTypeImage)

	if err := env.repo.DeleteByStoreIds(context.Background(), []int64{liveId}); err != nil {
		t.Fatalf("按 store ID 摘关联失败: %v", err)
	}
	var n int64
	env.db.Raw("SELECT COUNT(*) FROM resource_store WHERE store_id = ?", liveId).Scan(&n)
	if n != 0 {
		t.Fatalf("指定 store 的关联应被摘除，实际剩 %d", n)
	}
	_ = resourceId
}

// TestListStoreTypeSetsByWorkIdsExcludesDeadStores 覆盖确认行级判定的活行角色集合：
// 仅剩软删残留的角色不算「作品拥有该角色」（merge overwrite 轨道残留不再触发覆盖确认弹窗）
func TestListStoreTypeSetsByWorkIdsExcludesDeadStores(t *testing.T) {
	env := newLivenessEnv(t)
	// 双关联键（image）：死代+活代 → image 入集合；纯死键（thumbnail）→ 不入集合
	resourceId, _, _ := env.seedDualAssociation(t, domain.StoreTypeImage)
	var workId int64
	env.db.Raw("SELECT work_id FROM resource WHERE id = ?", resourceId).Scan(&workId)
	thumb := domain.NewPersistentStore()
	thumb.FilePath = sql.NullString{String: "store/resource/a/轨道残留.png", Valid: true}
	thumb.CompletedAt = 1
	if err := env.db.Create(thumb).Error; err != nil {
		t.Fatalf("插轨道残留行失败: %v", err)
	}
	if err := env.db.Exec("UPDATE persistent_store SET deleted_at = 1000 WHERE id = ?", thumb.GetID()).Error; err != nil {
		t.Fatalf("软删轨道残留失败: %v", err)
	}
	thumbRs := domain.NewResourceStore()
	thumbRs.ResourceID = resourceId
	thumbRs.StoreType = domain.StoreTypeThumbnail
	thumbRs.StoreSeq = 0
	thumbRs.StoreID = thumb.GetID()
	if err := env.db.Create(thumbRs).Error; err != nil {
		t.Fatalf("插轨道残留关联失败: %v", err)
	}

	sets, err := env.svc.ListStoreTypeSetsByWorkIds(context.Background(), []int64{workId})
	if err != nil {
		t.Fatalf("查询角色集合失败: %v", err)
	}
	set := sets[workId]
	if _, ok := set[domain.StoreTypeImage]; !ok {
		t.Fatalf("双关联键的活代 image 应入角色集合，实际 %v", set)
	}
	if _, ok := set[domain.StoreTypeThumbnail]; ok {
		t.Fatalf("纯死行残留的 thumbnail 不应入角色集合（覆盖确认误弹根因），实际 %v", set)
	}
}
