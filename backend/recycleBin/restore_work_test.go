package recycleBin

// 复原作品 FK 时序回归：RestoreWork 复原后备份清单行必须被删除（无外键孤儿）。
// 修复前 restoreWorkFiles 在 RestoreWorkStores 清 store.backup_id 之前执行 DeleteBackup
// （删备份记录），撞 persistent_store.backup_id → backup.id 外键拒绝，留孤儿备份行。
// 用真实 backup.Service（DeleteBackupRecord 走真实 DELETE）+ DB 版 workRestorer
// （RestoreWorkStores 真实清 backup_id），外键时序缺陷只有真库才现。

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/library-squirrel/backend/backup"
	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/persistentStore"
	"github.com/library-squirrel/backend/resource"
	"github.com/library-squirrel/backend/search"
	"github.com/library-squirrel/backend/shareLock"

	"gorm.io/gorm"
)

// dbWorkRestorer 真库版 WorkRestorer（复原链所需方法走真实 SQL）：RestoreWorkStores 真实清
// store 的 deleted_at 与 backup_id，DeleteBackup 才会在外键强制下放行
type dbWorkRestorer struct {
	db *gorm.DB
}

func (r *dbWorkRestorer) GetBySiteAndSiteWorkID(ctx context.Context, siteId int64, siteWorkId string) (*domain.Work, error) {
	return nil, nil
}
func (r *dbWorkRestorer) SoftDeleteWork(ctx context.Context, workId int64) error { return nil }
func (r *dbWorkRestorer) GetDeletedWork(ctx context.Context, id int64) (*domain.Work, error) {
	w := domain.NewWork()
	err := r.db.Unscoped().Where("id = ? AND deleted_at > 0", id).First(w).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return w, nil
}
func (r *dbWorkRestorer) RestoreDeletedWork(ctx context.Context, id int64) error {
	return r.db.Unscoped().Model(&domain.Work{}).Where("id = ?", id).Update("deleted_at", 0).Error
}
func (r *dbWorkRestorer) RestoreWorkStores(ctx context.Context, workId int64) error {
	// 本回归单作品单 store：清该作品全部软删 store 行的 deleted_at 与 backup_id
	// （等价于真库 RestoreWorkStores 在同键最新死代单代下的行为）
	return r.db.Unscoped().Model(&domain.PersistentStore{}).
		Where("deleted_at > 0 AND id IN (SELECT rs.store_id FROM resource_store rs JOIN resource res ON res.id = rs.resource_id WHERE res.work_id = ?)", workId).
		Updates(map[string]any{"deleted_at": 0, "backup_id": nil}).Error
}
func (r *dbWorkRestorer) ListWorkStoresIncludeDeleted(ctx context.Context, workId int64) []*domain.PersistentStore {
	var stores []*domain.PersistentStore
	r.db.Unscoped().
		Joins("JOIN resource_store rs ON rs.store_id = persistent_store.id").
		Joins("JOIN resource res ON res.id = rs.resource_id").
		Where("res.work_id = ?", workId).
		Find(&stores)
	return stores
}
func (r *dbWorkRestorer) ListRevivableWorkStores(ctx context.Context, workId int64) []*domain.PersistentStore {
	var stores []*domain.PersistentStore
	r.db.Unscoped().
		Joins("JOIN resource_store rs ON rs.store_id = persistent_store.id").
		Joins("JOIN resource res ON res.id = rs.resource_id").
		Where("res.work_id = ? AND persistent_store.deleted_at > 0", workId).
		Find(&stores)
	return stores
}
func (r *dbWorkRestorer) ListDeletedBefore(ctx context.Context, expireBefore int64) ([]*domain.Work, error) {
	return nil, nil
}
func (r *dbWorkRestorer) DeleteWorkAndSurroundingData(ctx context.Context, id int64) error {
	return nil
}

// TestRestoreWorkCleansBackupRecord 复原作品后备份清单行须被删除（真库外键强制下无孤儿行）
func TestRestoreWorkCleansBackupRecord(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	workDir := t.TempDir()
	ps := persistentStore.NewService(persistentStore.NewRepository(db), nil, func() string { return workDir })
	searchSvc := search.NewService(search.NewRepository(db), nil, nil, nil, nil, nil, nil, nil, nil, nil)
	lock := shareLock.NewShareLockRegistry()
	resourceSvc := resource.NewService(resource.NewRepository(db), resource.NewResourceStoreRepository(db))
	bkp := backup.NewService(backup.NewRepository(db), func() string { return workDir })
	svc := NewService(&dbWorkRestorer{db: db}, bkp, nil, searchSvc, ps, ps, &fakeResourceRecomputer{}, nil, func() string { return workDir }, nil, nil, nil, nil, lock, resourceSvc)

	work := domain.NewWork()
	if err := db.Create(work).Error; err != nil {
		t.Fatalf("插 work 失败: %v", err)
	}
	res := domain.NewResource()
	res.WorkID = work.GetID()
	res.ResourceType = "image"
	if err := db.Create(res).Error; err != nil {
		t.Fatalf("插 resource 失败: %v", err)
	}
	store := domain.NewPersistentStore()
	store.FilePath = sql.NullString{String: "store/resource/a/restore-target.png", Valid: true}
	store.CompletedAt = 1
	if err := db.Create(store).Error; err != nil {
		t.Fatalf("插 store 失败: %v", err)
	}

	// 备份清单行（workdir 基准，GetBackupPath = workdir + file_path）+ 备份文件（RestoreFile 校验文件存在）
	backupRel := "backup/2026/08/31/restore-source.png"
	if err := db.Exec("INSERT INTO backup (file_path, workdir, create_time, update_time) VALUES (?, ?, 0, 0)", backupRel, workDir).Error; err != nil {
		t.Fatalf("插备份行失败: %v", err)
	}
	var backupID int64
	if err := db.Raw("SELECT id FROM backup WHERE file_path = ?", backupRel).Scan(&backupID).Error; err != nil {
		t.Fatalf("查备份行失败: %v", err)
	}
	backupAbs := filepath.Join(workDir, filepath.FromSlash(backupRel))
	if err := os.MkdirAll(filepath.Dir(backupAbs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backupAbs, []byte("backup-content"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 软删 store 并写 backup_id（外键引用真实备份行）+ 挂载关联 + 软删作品
	if err := db.Exec("UPDATE persistent_store SET deleted_at = 1000, backup_id = ? WHERE id = ?", backupID, store.GetID()).Error; err != nil {
		t.Fatalf("软删 store 失败: %v", err)
	}
	rs := domain.NewResourceStore()
	rs.ResourceID = res.GetID()
	rs.StoreType = "image"
	rs.StoreID = store.GetID()
	if err := db.Create(rs).Error; err != nil {
		t.Fatalf("插关联失败: %v", err)
	}
	if err := db.Exec("UPDATE work SET deleted_at = 1000 WHERE id = ?", work.GetID()).Error; err != nil {
		t.Fatalf("软删 work 失败: %v", err)
	}

	if _, err := svc.RestoreWork(context.Background(), work.GetID(), false); err != nil {
		t.Fatalf("复原作品失败: %v", err)
	}

	// 断言：作品/ store 复原、store backup_id 清空、备份清单行被删除、文件还原到 store 路径
	var workDeleted int64
	if err := db.Raw("SELECT deleted_at FROM work WHERE id = ?", work.GetID()).Scan(&workDeleted).Error; err != nil {
		t.Fatal(err)
	}
	if workDeleted != 0 {
		t.Fatalf("作品应复原，deleted_at=%d", workDeleted)
	}
	var storeState struct {
		DeletedAt int64
		BackupID  sql.NullInt64
	}
	if err := db.Raw("SELECT deleted_at, backup_id FROM persistent_store WHERE id = ?", store.GetID()).Scan(&storeState).Error; err != nil {
		t.Fatal(err)
	}
	if storeState.DeletedAt != 0 || storeState.BackupID.Valid {
		t.Fatalf("store 应复原且清 backup_id，deleted_at=%d backup_id=%v", storeState.DeletedAt, storeState.BackupID)
	}
	var backupCount int64
	if err := db.Raw("SELECT COUNT(*) FROM backup WHERE id = ?", backupID).Scan(&backupCount).Error; err != nil {
		t.Fatal(err)
	}
	if backupCount != 0 {
		t.Fatalf("备份清单行 %d 应被删除（修复前外键时序缺陷留孤儿行），实际仍存在", backupID)
	}
	if _, err := os.Stat(filepath.Join(workDir, "store/resource/a/restore-target.png")); err != nil {
		t.Fatalf("还原文件应存在于 store 目标路径: %v", err)
	}
}
