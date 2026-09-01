package recycleBin

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/persistentStore"
	"github.com/library-squirrel/backend/resource"
	"github.com/library-squirrel/backend/search"
	"github.com/library-squirrel/backend/shareLock"

	"gorm.io/gorm"
)

// restoreStoreEnv 内存库（work/resource/resource_store/persistent_store 四表）+ 真实
// persistentStore/search/resource 服务（复原置换链焦点件：置换软删、复活、挂载身份查询、
// 锁守卫反查所属作品走真实 SQL），BackupReader 与完整度重算用记账桩；
// 挂真实作品锁注册中心（守卫测试登记/强制解锁用）
type restoreStoreEnv struct {
	svc       *Service
	db        *gorm.DB
	ps        *persistentStore.Service
	backup    *recordingBackupReader
	recompute *fakeResourceRecomputer
	workDir   string
	lock      shareLock.ShareLockRegistry
}

// newRestoreStoreEnv 构建复原置换链测试环境
func newRestoreStoreEnv(t *testing.T) *restoreStoreEnv {
	t.Helper()
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	if err := db.AutoMigrate(&domain.Work{}, &domain.Resource{}, &domain.ResourceStore{}, &domain.PersistentStore{}); err != nil {
		t.Fatalf("迁移测试实体失败: %v", err)
	}
	workDir := t.TempDir()
	ps := persistentStore.NewService(persistentStore.NewRepository(db), nil, func() string { return workDir })
	searchSvc := search.NewService(search.NewRepository(db), nil, nil, nil, nil, nil, nil, nil, nil, nil)
	backup := &recordingBackupReader{}
	recompute := &fakeResourceRecomputer{}
	lock := shareLock.NewShareLockRegistry()
	resourceSvc := resource.NewService(resource.NewRepository(db), resource.NewResourceStoreRepository(db), nil)
	svc := NewService(nil, backup, nil, searchSvc, ps, ps, recompute, nil, func() string { return workDir }, nil, nil, nil, nil, lock, resourceSvc)
	return &restoreStoreEnv{svc: svc, db: db, ps: ps, backup: backup, recompute: recompute, workDir: workDir, lock: lock}
}

// fakeResourceRecomputer 完整度重算记账桩
type fakeResourceRecomputer struct {
	calledResourceIds []int64
}

func (f *fakeResourceRecomputer) RecomputeResourceComplete(ctx context.Context, resourceId int64) {
	f.calledResourceIds = append(f.calledResourceIds, resourceId)
}

// seedVictim 造一个可复原条目：活作品 + 资源 + 软删 store 行（带 backup 引用与挂载键关联）
// 返回 resourceId 与 store 行 ID
func (e *restoreStoreEnv) seedVictim(t *testing.T, role string, backupId int64) (resourceId, victimId int64) {
	t.Helper()
	work := domain.NewWork()
	if err := e.db.Create(work).Error; err != nil {
		t.Fatalf("插 work 失败: %v", err)
	}
	res := domain.NewResource()
	res.WorkID = work.GetID()
	res.ResourceType = "image"
	if err := e.db.Create(res).Error; err != nil {
		t.Fatalf("插 resource 失败: %v", err)
	}
	row := domain.NewPersistentStore()
	row.FilePath = sql.NullString{String: "store/resource/a/被替换版本.png", Valid: true}
	row.CompletedAt = 1
	if err := e.db.Create(row).Error; err != nil {
		t.Fatalf("插 store 失败: %v", err)
	}
	// 备份清单行种子（persistent_store.backup_id 外键防线——行内引用须指向存在行）
	if backupId > 0 {
		if err := e.db.Exec("INSERT OR IGNORE INTO backup (id, create_time, update_time) VALUES (?, 0, 0)", backupId).Error; err != nil {
			t.Fatalf("建备份行失败: %v", err)
		}
	}
	if err := e.db.Exec("UPDATE persistent_store SET deleted_at = 1000, backup_id = NULLIF(?, 0) WHERE id = ?", backupId, row.GetID()).Error; err != nil {
		t.Fatalf("软删失败: %v", err)
	}
	rs := domain.NewResourceStore()
	rs.ResourceID = res.GetID()
	rs.StoreType = role
	rs.StoreSeq = 0
	rs.StoreID = row.GetID()
	if err := e.db.Create(rs).Error; err != nil {
		t.Fatalf("插关联失败: %v", err)
	}
	return res.GetID(), row.GetID()
}

// seedLiveSameKey 造同键活行（当前代占位，同路径）
func (e *restoreStoreEnv) seedLiveSameKey(t *testing.T, resourceId int64, role string, path string) int64 {
	t.Helper()
	row := domain.NewPersistentStore()
	row.FilePath = sql.NullString{String: path, Valid: true}
	row.CompletedAt = 1
	if err := e.db.Create(row).Error; err != nil {
		t.Fatalf("插占位行失败: %v", err)
	}
	rs := domain.NewResourceStore()
	rs.ResourceID = resourceId
	rs.StoreType = role
	rs.StoreSeq = 0
	rs.StoreID = row.GetID()
	if err := e.db.Create(rs).Error; err != nil {
		t.Fatalf("插占位关联失败: %v", err)
	}
	return row.GetID()
}

// TestRestoreStoreSwapsCurrentGeneration 复原全链：同键活行（当前代）被置换软删入回收站、
// 本行复活（双列清）、备份清单行删除、完整度重算触发；关联零操作（双关联形态成立）
func TestRestoreStoreSwapsCurrentGeneration(t *testing.T) {
	env := newRestoreStoreEnv(t)
	const victimPath = "store/resource/a/被替换版本.png"
	resourceId, victimId := env.seedVictim(t, domain.StoreTypeImage, 501)
	placeholderId := env.seedLiveSameKey(t, resourceId, domain.StoreTypeImage, victimPath)

	if err := env.svc.RestoreStore(context.Background(), victimId); err != nil {
		t.Fatalf("复原失败: %v", err)
	}
	// 占位行被置换软删（无 FileMover 时 backup_id=0，行入回收站文件条目）
	var placeholderDeleted int64
	env.db.Raw("SELECT deleted_at FROM persistent_store WHERE id = ?", placeholderId).Scan(&placeholderDeleted)
	if placeholderDeleted == 0 {
		t.Fatalf("同键占位活行应被置换软删，实际仍活")
	}
	// 本行复活且双列清
	var victimDeleted, victimBackup int64
	env.db.Raw("SELECT deleted_at FROM persistent_store WHERE id = ?", victimId).Scan(&victimDeleted)
	env.db.Raw("SELECT backup_id FROM persistent_store WHERE id = ?", victimId).Scan(&victimBackup)
	if victimDeleted != 0 || victimBackup != 0 {
		t.Fatalf("复原后本行应复活且清 backup_id，实际 deleted_at=%d backup_id=%d", victimDeleted, victimBackup)
	}
	// 备份清单行删除 + 完整度重算
	if len(env.backup.deletedIds) != 1 || env.backup.deletedIds[0] != 501 {
		t.Fatalf("备份清单行 501 应被删除，实际 %v", env.backup.deletedIds)
	}
	if len(env.recompute.calledResourceIds) != 1 || env.recompute.calledResourceIds[0] != resourceId {
		t.Fatalf("完整度重算应触发一次且指向本资源，实际 %v", env.recompute.calledResourceIds)
	}
	// 双关联形态：两条关联都在
	var assocCount int64
	env.db.Raw("SELECT COUNT(*) FROM resource_store WHERE store_id IN ?", []int64{victimId, placeholderId}).Scan(&assocCount)
	if assocCount != 2 {
		t.Fatalf("双关联形态应保留两条关联（本行+被置换行），实际 %d", assocCount)
	}
}

// TestRestoreStoreRejectsNoBackup MarkInvalid 形态（无备份）拒绝复原
func TestRestoreStoreRejectsNoBackup(t *testing.T) {
	env := newRestoreStoreEnv(t)
	_, victimId := env.seedVictim(t, domain.StoreTypeImage, 0) // backup_id=0 失效行

	if err := env.svc.RestoreStore(context.Background(), victimId); !errors.Is(err, ErrRestoreStoreNoBackup) {
		t.Fatalf("无备份行应返回 ErrRestoreStoreNoBackup，实际 %v", err)
	}
}

// TestRestoreStoreRejectsDeadWorkMount 挂载链指向已软删作品时拒绝（作品条目域，不可置换）
func TestRestoreStoreRejectsDeadWorkMount(t *testing.T) {
	env := newRestoreStoreEnv(t)
	resourceId, victimId := env.seedVictim(t, domain.StoreTypeImage, 502)
	var workId int64
	env.db.Raw("SELECT work_id FROM resource WHERE id = ?", resourceId).Scan(&workId)
	env.db.Exec("UPDATE work SET deleted_at = 2000 WHERE id = ?", workId)

	if err := env.svc.RestoreStore(context.Background(), victimId); !errors.Is(err, ErrRestoreStoreUnreachable) {
		t.Fatalf("作品已软删应返回 ErrRestoreStoreUnreachable，实际 %v", err)
	}
}
