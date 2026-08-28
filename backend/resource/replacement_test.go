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
	storeRel := "store/resource/x.mp4"
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

	// 装配 ReplacementService：store 行复活走真仓储、备份走真 backup.Service，其余依赖 no-op
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
