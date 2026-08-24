package work

import (
	"context"
	"database/sql"
	"testing"

	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/persistentStore"

	"gorm.io/gorm"
)

// revivableEnv 复活派生测试环境：复用 purge 测试的内存库四表 + 真实 persistentStore 服务，
// 挂载链经 fixture（resourceDeleter/resourceStoreBatchReader 私有字段注入预置）
type revivableEnv struct {
	svc  *Service
	ps   *persistentStore.Service
	db   *gorm.DB
	rd   *purgeResourceDeleter
	rsBr *purgeRsBatchReader
}

// makeStoreRow 插一行 persistent_store 并按指定时间软删（backupId 写行内引用）
func makeStoreRow(t *testing.T, env *revivableEnv, path string, deletedAt int64, backupId int64) int64 {
	t.Helper()
	row := domain.NewPersistentStore()
	row.FilePath = sql.NullString{String: path, Valid: true}
	row.CompletedAt = 1
	if err := env.db.Create(row).Error; err != nil {
		t.Fatalf("插 store 失败: %v", err)
	}
	if deletedAt > 0 {
		// 备份清单行种子（persistent_store.backup_id 外键防线）；NULLIF(0)=NULL 表无备份
		if backupId > 0 {
			if err := env.db.Exec("INSERT OR IGNORE INTO backup (id, create_time, update_time) VALUES (?, 0, 0)", backupId).Error; err != nil {
				t.Fatalf("建备份行失败: %v", err)
			}
		}
		if err := env.db.Exec("UPDATE persistent_store SET deleted_at = ?, backup_id = NULLIF(?, 0) WHERE id = ?", deletedAt, backupId, row.GetID()).Error; err != nil {
			t.Fatalf("软删 store 失败: %v", err)
		}
	}
	return row.GetID()
}

// makeAssoc 挂载链预置一行关联（resource → store，带挂载键）
func makeAssoc(resourceId int64, role string, seq int, storeId int64) *domain.ResourceStore {
	rs := domain.NewResourceStore()
	rs.ResourceID = resourceId
	rs.StoreType = role
	rs.StoreSeq = seq
	rs.StoreID = storeId
	rs.Generation = domain.GenerationDownloaded
	return rs
}

// TestDeriveRevivableStoresPicksNewestDeadPerKey 同键最新死代：每键只圈最新死行，
// 更早死代与活行不进复活集；键含 resource 维度（同 role 跨资源不串键）
func TestDeriveRevivableStoresPicksNewestDeadPerKey(t *testing.T) {
	svc, psSvc, db := newPurgeTestEnv(t)
	env := &revivableEnv{svc: svc, ps: psSvc, db: db}

	res1 := domain.NewResource()
	res1.WorkID = 1
	res1.ResourceType = "image"
	if err := db.Create(res1).Error; err != nil {
		t.Fatalf("插 resource 失败: %v", err)
	}
	res2 := domain.NewResource()
	res2.WorkID = 1
	res2.ResourceType = "image"
	if err := db.Create(res2).Error; err != nil {
		t.Fatalf("插 resource 失败: %v", err)
	}

	const sharedPath = "store/resource/作者/同键多代.png"
	gen1 := makeStoreRow(t, env, sharedPath, 1000, 101)                   // res1 key(image,0) 第一代死行
	gen2 := makeStoreRow(t, env, sharedPath, 2000, 102)                   // res1 key(image,0) 最新死行（应被圈定）
	live := makeStoreRow(t, env, "store/resource/作者/活行.png", 0, 0)        // res1 key(image,1) 活行不进复活集
	other := makeStoreRow(t, env, "store/resource/作者/他资源.png", 3000, 103) // res2 key(image,0) 同 role 跨资源

	env.rd = &purgeResourceDeleter{db: db, resources: []*domain.Resource{res1, res2}}
	env.rsBr = &purgeRsBatchReader{rsMap: map[int64][]*domain.ResourceStore{
		res1.GetID(): {makeAssoc(res1.GetID(), domain.StoreTypeImage, 0, gen1), makeAssoc(res1.GetID(), domain.StoreTypeImage, 0, gen2), makeAssoc(res1.GetID(), domain.StoreTypeImage, 1, live)},
		res2.GetID(): {makeAssoc(res2.GetID(), domain.StoreTypeImage, 0, other)},
	}}
	svc.resourceDeleter = env.rd
	svc.resourceStoreBatchReader = env.rsBr

	rows := svc.deriveRevivableStores(context.Background(), 1)
	if len(rows) != 2 {
		t.Fatalf("复活集应为 2 行（res1 最新死代 + res2 最新死代），实际 %d 行", len(rows))
	}
	ids := map[int64]bool{}
	for _, row := range rows {
		ids[row.GetID()] = true
	}
	if !ids[gen2] {
		t.Fatalf("res1 键应圈最新死代 gen2(id=%d)，实际圈定 %v", gen2, ids)
	}
	if ids[gen1] {
		t.Fatalf("更早死代 gen1(id=%d) 不应进复活集", gen1)
	}
	if ids[live] {
		t.Fatalf("活行(id=%d) 不应进复活集", live)
	}
	if !ids[other] {
		t.Fatalf("res2 键的最新死代(id=%d) 应进复活集（键含 resource 维度不串键）", other)
	}
}

// TestRestoreWorkStoresRevivesOnlyNewestGeneration 作品复原只复活同键最新死代：
// 双代同路径形态下无差别复活会令两活行同 file_path 撞部分唯一索引（生产迁移建索引），
// 此处建同款索引作锚定——复活最新代后索引不炸、更早代保持死态
func TestRestoreWorkStoresRevivesOnlyNewestGeneration(t *testing.T) {
	svc, psSvc, db := newPurgeTestEnv(t)
	env := &revivableEnv{svc: svc, ps: psSvc, db: db}
	// 手工建生产同款部分唯一索引（AutoMigrate 不建部分索引；全量迁移已建时跳过）
	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_persistent_store_file_path_active ON persistent_store(file_path) WHERE deleted_at = 0`).Error; err != nil {
		t.Fatalf("建部分唯一索引失败: %v", err)
	}

	res := domain.NewResource()
	res.WorkID = 1
	res.ResourceType = "image"
	if err := db.Create(res).Error; err != nil {
		t.Fatalf("插 resource 失败: %v", err)
	}
	const sharedPath = "store/resource/作者/双代同路径.png"
	gen1 := makeStoreRow(t, env, sharedPath, 1000, 201)
	gen2 := makeStoreRow(t, env, sharedPath, 2000, 202)

	svc.resourceDeleter = &purgeResourceDeleter{db: db, resources: []*domain.Resource{res}}
	svc.resourceStoreBatchReader = &purgeRsBatchReader{rsMap: map[int64][]*domain.ResourceStore{
		res.GetID(): {makeAssoc(res.GetID(), domain.StoreTypeImage, 0, gen1), makeAssoc(res.GetID(), domain.StoreTypeImage, 0, gen2)},
	}}

	if err := svc.RestoreWorkStores(context.Background(), 1); err != nil {
		t.Fatalf("作品复原 store 复活应成功（只复活最新代不撞索引）: %v", err)
	}
	var gen1DeletedAt, gen2DeletedAt int64
	if err := db.Raw("SELECT deleted_at FROM persistent_store WHERE id = ?", gen1).Scan(&gen1DeletedAt).Error; err != nil {
		t.Fatalf("查 gen1 失败: %v", err)
	}
	if err := db.Raw("SELECT deleted_at FROM persistent_store WHERE id = ?", gen2).Scan(&gen2DeletedAt).Error; err != nil {
		t.Fatalf("查 gen2 失败: %v", err)
	}
	if gen1DeletedAt == 0 {
		t.Fatalf("更早死代 gen1 应保持死态（deleted_at>0），实际已复活")
	}
	if gen2DeletedAt != 0 {
		t.Fatalf("最新死代 gen2 应被复活（deleted_at=0），实际仍为死态 %d", gen2DeletedAt)
	}
	var backupCleared sql.NullInt64
	if err := db.Raw("SELECT backup_id FROM persistent_store WHERE id = ?", gen2).Scan(&backupCleared).Error; err != nil {
		t.Fatalf("查 gen2 backup_id 失败: %v", err)
	}
	if backupCleared.Valid {
		t.Fatalf("复活应同清 backup_id，实际 %+v", backupCleared)
	}
}
