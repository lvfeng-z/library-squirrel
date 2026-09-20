package resource

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/library-squirrel/backend/backup"
	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/persistentStore"
	"github.com/library-squirrel/backend/shareLock"
	"go.uber.org/zap"

	"github.com/library-squirrel/backend/base/logger"
)

// TestMain 测试进程无 logger.Init——注入 Nop 防全局 logger.Log 为 nil
func TestMain(m *testing.M) {
	logger.Log = zap.NewNop().Sugar()
	os.Exit(m.Run())
}

// ==== 回滚复活 FK 时序回归测试 ====

// noop 系列：回滚复活路径不触达的依赖（显式 Victims 清单时 resolveVictims 不查作品/资源）
type noopReplaceResourceLister struct{}

func (noopReplaceResourceLister) ListByWorkId(ctx context.Context, workId int64) ([]*domain.Resource, error) {
	return nil, nil
}

type noopReplaceAssocLister struct{}

func (noopReplaceAssocLister) ListByResourceIds(ctx context.Context, ids []int64) ([]*domain.ResourceStore, error) {
	return nil, nil
}

type noopReplaceDeleter struct{}

func (noopReplaceDeleter) DeleteWithBackup(ctx context.Context, id int64) (int64, error) {
	return 0, nil
}
func (noopReplaceDeleter) SoftDeleteAndDiscardFile(ctx context.Context, id int64) error { return nil }

type noopReplaceWorkLiveness struct{}

func (noopReplaceWorkLiveness) GetById(ctx context.Context, id int64) (*domain.Work, error) {
	return nil, nil
}

type noopReplaceRecompute struct{}

func (noopReplaceRecompute) RecomputeResourceComplete(ctx context.Context, resourceId int64) {}

// noopAssocRemover 回滚摘关联 no-op 桩（不触达丢弃清单的用例使用）
type noopAssocRemover struct{}

func (noopAssocRemover) DeleteByStoreIds(ctx context.Context, storeIds []int64) error { return nil }

// noopRowHardDeleter 回滚物理删行 no-op 桩（不触达丢弃清单的用例使用）
type noopRowHardDeleter struct{}

func (noopRowHardDeleter) HardDelete(ctx context.Context, id int64, backup bool) (int64, error) {
	return 0, nil
}

type replaceWorkDir struct{ workDir string }

func (w replaceWorkDir) GetWorkDir() string { return w.workDir }

// unusedFileMover 测试路径不调用 MoveToBackup（复活不触发备份移动），意外调用则报错暴露
type unusedFileMover struct{}

func (unusedFileMover) MoveToBackup(ctx context.Context, absFilePath string) (int64, error) {
	return 0, fmt.Errorf("回滚复活路径不应触发 MoveToBackup")
}

// TestRestoreReplacedStoresBackupOrder FK 时序回归：回滚复活须先复活行并清 backup_id、
// 再删备份清单行——persistent_store.backup_id 外键下，行内引用未清时删备份行会被拒绝，
// 残留孤儿备份行（实测发现，replacement.go 原序 DeleteBackup 先于 RestoreByIds）。
// 真库断言：复原后行双列同清、文件回 store/、备份清单行随还原删除
func TestRestoreReplacedStoresBackupOrder(t *testing.T) {
	ctx := context.Background()
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	workDir := t.TempDir()
	storeRel := "store/work/x.mp4"
	storeAbs := filepath.Join(workDir, storeRel)
	if err := os.MkdirAll(filepath.Dir(storeAbs), 0o755); err != nil {
		t.Fatalf("建 store 目录失败: %v", err)
	}
	if err := os.WriteFile(storeAbs, []byte("content"), 0o644); err != nil {
		t.Fatalf("造源文件失败: %v", err)
	}

	// 造软删行 + 备份行（模拟 DeleteWithBackup 后的状态：行软删、行内 backup_id 指向备份行、文件已移入备份目录）
	storeRepo := persistentStore.NewRepository(db)
	storeRow := domain.NewPersistentStore()
	storeRow.FilePath = sql.NullString{String: storeRel, Valid: true}
	storeRow.CompletedAt = 1
	if err := storeRepo.Create(ctx, storeRow); err != nil {
		t.Fatalf("插入 store 行失败: %v", err)
	}
	backupDir := t.TempDir()
	backupAbs := filepath.Join(backupDir, "x.mp4")
	if err := os.Rename(storeAbs, backupAbs); err != nil {
		t.Fatalf("移文件入备份失败: %v", err)
	}
	if err := db.Exec("INSERT INTO backup (id, workdir, file_path, create_time, update_time) VALUES (?, ?, ?, 0, 0)",
		7, backupDir, "x.mp4").Error; err != nil {
		t.Fatalf("建备份行失败: %v", err)
	}
	if err := db.Exec("UPDATE persistent_store SET deleted_at = 1000, backup_id = 7 WHERE id = ?", storeRow.GetID()).Error; err != nil {
		t.Fatalf("软删 store 行失败: %v", err)
	}

	// 装配 ReplacementService：store 行复活/丢弃走真仓储、摘关联走真 resource_store 仓储、
	// 备份走真 backup.Service，其余依赖 no-op
	ps := persistentStore.NewService(storeRepo, unusedFileMover{}, func() string { return workDir })
	backupSvc := backup.NewService(backup.NewRepository(db), func() string { return workDir })
	svc := NewReplacementService(
		noopReplaceResourceLister{},
		noopReplaceAssocLister{},
		ps,
		noopReplaceDeleter{},
		backupSvc,
		noopReplaceWorkLiveness{},
		noopReplaceRecompute{},
		replaceWorkDir{workDir: workDir},
		shareLock.NewShareLockRegistry(),
		NewResourceStoreRepository(db),
		ps,
	)

	scope := RestoreScope{Victims: []StoreRef{{
		StoreID:  storeRow.GetID(),
		BackupID: 7,
		FilePath: storeRel,
	}}}
	if err := svc.RestoreReplacedStores(ctx, scope); err != nil {
		t.Fatalf("回滚复活失败: %v", err)
	}

	// 断言：行复活且 backup_id 清空（双列同清）
	var deletedAt int64
	var backupId sql.NullInt64
	if err := db.Raw("SELECT deleted_at, backup_id FROM persistent_store WHERE id = ?", storeRow.GetID()).Row().Scan(&deletedAt, &backupId); err != nil {
		t.Fatalf("读 store 行失败: %v", err)
	}
	if deletedAt != 0 || backupId.Valid {
		t.Fatalf("复原后须双列同清，实际 deleted_at=%d backup_id=%+v", deletedAt, backupId)
	}
	// 断言：文件回 store/ 目录、备份文件与清单行消失
	if _, err := os.Stat(storeAbs); err != nil {
		t.Fatalf("还原文件未回 store/ 目录: %v", err)
	}
	if _, err := os.Stat(backupAbs); err == nil {
		t.Fatalf("备份文件应随还原删除，实际仍存在")
	}
	var backupCount int64
	if err := db.Raw("SELECT COUNT(*) FROM backup WHERE id = 7").Scan(&backupCount).Error; err != nil {
		t.Fatalf("查备份行失败: %v", err)
	}
	if backupCount != 0 {
		t.Fatalf("备份清单行应随还原删除（FK 顺序错误会残留），实际残留 %d 行", backupCount)
	}
}

// TestRestoreReplacedStoresDiscardsNewGenerationBeforeRevive 替换中断后回滚：旧代复活前先
// 丢弃登记的新代行（行+文件+关联）。真库锚定 file_path 部分唯一索引——新代活行与旧代同路径
// 在位时，复活（清 deleted_at 的 UPDATE）整批被唯一索引拒绝；丢弃先行则新代行消失、文件释放、
// 旧代复活并从备份还原文件
func TestRestoreReplacedStoresDiscardsNewGenerationBeforeRevive(t *testing.T) {
	ctx := context.Background()
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	workDir := t.TempDir()
	storeRel := "store/work/y.mp4"
	storeAbs := filepath.Join(workDir, storeRel)
	if err := os.MkdirAll(filepath.Dir(storeAbs), 0o755); err != nil {
		t.Fatalf("建 store 目录失败: %v", err)
	}
	if err := os.WriteFile(storeAbs, []byte("old-content"), 0o644); err != nil {
		t.Fatalf("造旧代文件失败: %v", err)
	}

	// 旧代行（已完成→备份软删，文件移入备份目录）
	storeRepo := persistentStore.NewRepository(db)
	victimRow := domain.NewPersistentStore()
	victimRow.FilePath = sql.NullString{String: storeRel, Valid: true}
	victimRow.CompletedAt = 1
	if err := storeRepo.Create(ctx, victimRow); err != nil {
		t.Fatalf("插入旧代 store 行失败: %v", err)
	}
	backupDir := t.TempDir()
	backupAbs := filepath.Join(backupDir, "y.mp4")
	if err := os.Rename(storeAbs, backupAbs); err != nil {
		t.Fatalf("移旧代文件入备份失败: %v", err)
	}
	if err := db.Exec("INSERT INTO backup (id, workdir, file_path, create_time, update_time) VALUES (?, ?, ?, 0, 0)",
		11, backupDir, "y.mp4").Error; err != nil {
		t.Fatalf("建备份行失败: %v", err)
	}
	if err := db.Exec("UPDATE persistent_store SET deleted_at = 1000, backup_id = 11 WHERE id = ?", victimRow.GetID()).Error; err != nil {
		t.Fatalf("软删旧代行失败: %v", err)
	}

	// 新代行（中断替换的残留：同路径活行、未完成的半成品文件）+ 指向它的挂载关联。
	// 关联带 resource 外键，先铺 work/resource 行
	if err := db.Exec("INSERT INTO work (id, create_time, update_time, deleted_at) VALUES (500, 0, 0, 0)").Error; err != nil {
		t.Fatalf("铺 work 行失败: %v", err)
	}
	if err := db.Exec("INSERT INTO resource (id, work_id, resource_type, create_time, update_time) VALUES (700, 500, 'image', 0, 0)").Error; err != nil {
		t.Fatalf("铺 resource 行失败: %v", err)
	}
	newRow := domain.NewPersistentStore()
	newRow.FilePath = sql.NullString{String: storeRel, Valid: true}
	newRow.CompletedAt = 0
	if err := storeRepo.Create(ctx, newRow); err != nil {
		t.Fatalf("插入新代 store 行失败: %v", err)
	}
	if err := os.WriteFile(storeAbs, []byte("half-download"), 0o644); err != nil {
		t.Fatalf("造新代半成品文件失败: %v", err)
	}
	assocRepo := NewResourceStoreRepository(db)
	assoc := domain.NewResourceStore()
	assoc.ResourceID = 700
	assoc.StoreType = domain.StoreTypeImage
	assoc.StoreSeq = 0
	assoc.StoreID = newRow.GetID()
	if err := assocRepo.Create(ctx, assoc); err != nil {
		t.Fatalf("插入新代挂载关联失败: %v", err)
	}

	// 降级重跑形成的受害者：早前会话新建的行（在新建登记清单内）后被软删为受害者——
	// 同 ID 同现两清单时按受害者处置（复活、挂载关联保留），不参与丢弃
	victim2 := domain.NewPersistentStore()
	victim2.FilePath = sql.NullString{String: "store/work/y2.png", Valid: true}
	victim2.CompletedAt = 0
	if err := storeRepo.Create(ctx, victim2); err != nil {
		t.Fatalf("插入第二受害者行失败: %v", err)
	}
	if err := db.Exec("UPDATE persistent_store SET deleted_at = 1001 WHERE id = ?", victim2.GetID()).Error; err != nil {
		t.Fatalf("软删第二受害者行失败: %v", err)
	}
	victim2Assoc := domain.NewResourceStore()
	victim2Assoc.ResourceID = 700
	victim2Assoc.StoreType = domain.StoreTypeImage
	victim2Assoc.StoreSeq = 1
	victim2Assoc.StoreID = victim2.GetID()
	if err := assocRepo.Create(ctx, victim2Assoc); err != nil {
		t.Fatalf("插入第二受害者挂载关联失败: %v", err)
	}

	ps := persistentStore.NewService(storeRepo, unusedFileMover{}, func() string { return workDir })
	backupSvc := backup.NewService(backup.NewRepository(db), func() string { return workDir })
	svc := NewReplacementService(
		noopReplaceResourceLister{},
		noopReplaceAssocLister{},
		ps,
		noopReplaceDeleter{},
		backupSvc,
		noopReplaceWorkLiveness{},
		noopReplaceRecompute{},
		replaceWorkDir{workDir: workDir},
		shareLock.NewShareLockRegistry(),
		assocRepo,
		ps,
	)

	scope := RestoreScope{
		Victims: []StoreRef{
			{
				StoreID:  victimRow.GetID(),
				BackupID: 11,
				FilePath: storeRel,
			},
			{StoreID: victim2.GetID(), ResourceID: 700},
		},
		// 第二受害者同 ID 混入丢弃清单（降级重跑形态），按受害者处置
		DiscardStoreIDs: []int64{newRow.GetID(), victim2.GetID()},
	}
	if err := svc.RestoreReplacedStores(ctx, scope); err != nil {
		t.Fatalf("回滚失败: %v", err)
	}

	// 断言：新代行物理删除、关联摘除、半成品文件消失
	var newCount int64
	if err := db.Raw("SELECT COUNT(*) FROM persistent_store WHERE id = ?", newRow.GetID()).Scan(&newCount).Error; err != nil {
		t.Fatalf("查新代行失败: %v", err)
	}
	if newCount != 0 {
		t.Fatalf("新代行应被物理删除，实际残留")
	}
	var assocCount int64
	if err := db.Raw("SELECT COUNT(*) FROM resource_store WHERE store_id = ?", newRow.GetID()).Scan(&assocCount).Error; err != nil {
		t.Fatalf("查新代关联失败: %v", err)
	}
	if assocCount != 0 {
		t.Fatalf("新代挂载关联应被摘除，实际残留 %d 行", assocCount)
	}
	// 断言：旧代行复活且双列同清、文件从备份还原
	var deletedAt int64
	var backupId sql.NullInt64
	if err := db.Raw("SELECT deleted_at, backup_id FROM persistent_store WHERE id = ?", victimRow.GetID()).Row().Scan(&deletedAt, &backupId); err != nil {
		t.Fatalf("读旧代行失败: %v", err)
	}
	if deletedAt != 0 || backupId.Valid {
		t.Fatalf("旧代行应复活且双列同清，实际 deleted_at=%d backup_id=%+v", deletedAt, backupId)
	}
	// 断言：同 ID 混入丢弃清单的第二受害者按受害者处置——行复活、挂载关联保留
	var victim2DeletedAt int64
	if err := db.Raw("SELECT deleted_at FROM persistent_store WHERE id = ?", victim2.GetID()).Row().Scan(&victim2DeletedAt); err != nil {
		t.Fatalf("读第二受害者行失败: %v", err)
	}
	if victim2DeletedAt != 0 {
		t.Fatalf("混入丢弃清单的受害者应复活，实际 deleted_at=%d", victim2DeletedAt)
	}
	var victim2AssocCount int64
	if err := db.Raw("SELECT COUNT(*) FROM resource_store WHERE store_id = ?", victim2.GetID()).Scan(&victim2AssocCount).Error; err != nil {
		t.Fatalf("查第二受害者关联失败: %v", err)
	}
	if victim2AssocCount != 1 {
		t.Fatalf("受害者的挂载关联应保留（复活即挂载回位），实际 %d 行", victim2AssocCount)
	}
	got, err := os.ReadFile(storeAbs)
	if err != nil {
		t.Fatalf("旧代文件应从备份还原: %v", err)
	}
	if string(got) != "old-content" {
		t.Fatalf("还原文件内容应为旧代内容，实际 %q", string(got))
	}
	var backupCount int64
	if err := db.Raw("SELECT COUNT(*) FROM backup WHERE id = 11").Scan(&backupCount).Error; err != nil {
		t.Fatalf("查备份行失败: %v", err)
	}
	if backupCount != 0 {
		t.Fatalf("备份清单行应随还原删除，实际残留 %d 行", backupCount)
	}
}
