package persistentStore

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	"github.com/library-squirrel/backend/migration"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// mockFileMover FileMover 测试替身：把源文件移到临时保管目录并返回递增清单行 ID；
// failNext=true 时下一次调用报错（模拟移动失败，验证软删不被触发）
type mockFileMover struct {
	failNext bool
	nextId   int64
	keepDir  string
	db       *gorm.DB
}

func (m *mockFileMover) MoveToBackup(ctx context.Context, absFilePath string) (int64, error) {
	if m.failNext {
		return 0, fmt.Errorf("模拟移动失败")
	}
	m.nextId++
	if err := os.Rename(absFilePath, filepath.Join(m.keepDir, fmt.Sprintf("keep_%d", m.nextId))); err != nil {
		return 0, err
	}
	// 落真实备份清单行（persistent_store.backup_id 外键防线——返回的引用须指向存在行）
	if err := m.db.Exec("INSERT INTO backup (id, create_time, update_time) VALUES (?, 0, 0)", m.nextId).Error; err != nil {
		return 0, err
	}
	return m.nextId, nil
}

// newLifecycleTestService 内存库 + 真实仓储 + mock FileMover 的 Service
func newLifecycleTestService(t *testing.T) (*Service, string) {
	t.Helper()
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	if err := db.AutoMigrate(&domain.PersistentStore{}); err != nil {
		t.Fatalf("迁移测试实体失败: %v", err)
	}
	workDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workDir, "store/resource"), 0o755); err != nil {
		t.Fatalf("建 store 目录失败: %v", err)
	}
	repo := NewRepository(db)
	svc := NewService(repo, &mockFileMover{keepDir: t.TempDir(), db: db}, func() string { return workDir })
	return svc, workDir
}

func insertStoreRow(t *testing.T, svc *Service, relPath string) *domain.PersistentStore {
	t.Helper()
	row := domain.NewPersistentStore()
	row.FilePath = sql.NullString{String: relPath, Valid: true}
	row.CompletedAt = 1
	if err := svc.repo.Create(context.Background(), row); err != nil {
		t.Fatalf("插入行失败: %v", err)
	}
	return row
}

func getRowById(t *testing.T, svc *Service, id int64) *domain.PersistentStore {
	t.Helper()
	rows, err := svc.repo.List(context.Background(), &database.QueryOption{
		Conditions:     []clause.Expression{clause.Eq{Column: "id", Value: id}},
		IncludeDeleted: true,
	})
	if err != nil || len(rows) != 1 {
		t.Fatalf("查询行 %d 失败: %v, rows=%d", id, err, len(rows))
	}
	return rows[0]
}

// TestDeleteWithBackupLifecycle 同生共死不变量：移动成功 → 行软删且 backup_id 写入；
// 移动失败 → 返回错误且行保持活行（不软删不写列）；复原 → 双列同清
func TestDeleteWithBackupLifecycle(t *testing.T) {
	svc, workDir := newLifecycleTestService(t)
	ctx := context.Background()
	row := insertStoreRow(t, svc, "store/resource/a.mp4")
	if err := os.WriteFile(filepath.Join(workDir, "store/resource/a.mp4"), []byte("x"), 0o644); err != nil {
		t.Fatalf("造源文件失败: %v", err)
	}

	// 移动失败：中断且行保持活行
	svc.fileMover.(*mockFileMover).failNext = true
	if _, err := svc.DeleteWithBackup(ctx, row.GetID()); err == nil {
		t.Fatalf("移动失败时期望返回错误")
	}
	if got := getRowById(t, svc, row.GetID()); got.DeletedAt > 0 || got.BackupID.Valid {
		t.Fatalf("移动失败后行须保持活行（deleted_at=0, backup_id=NULL），实际 deleted_at=%d backup_id=%+v", got.DeletedAt, got.BackupID)
	}

	// 移动成功：行软删且 backup_id 写入（同生共死）
	svc.fileMover.(*mockFileMover).failNext = false
	backupId, err := svc.DeleteWithBackup(ctx, row.GetID())
	if err != nil || backupId <= 0 {
		t.Fatalf("DeleteWithBackup 成功路径失败: %v, backupId=%d", err, backupId)
	}
	if got := getRowById(t, svc, row.GetID()); got.DeletedAt == 0 || !got.BackupID.Valid || got.BackupID.Int64 != backupId {
		t.Fatalf("成功后行须 deleted_at>0 且 backup_id=%d，实际 deleted_at=%d backup_id=%+v", backupId, got.DeletedAt, got.BackupID)
	}

	// 复原：双列同清（deleted_at=0 且 backup_id=NULL）
	if err := svc.repo.RestoreByIds(ctx, []int64{row.GetID()}); err != nil {
		t.Fatalf("复原失败: %v", err)
	}
	if got := getRowById(t, svc, row.GetID()); got.DeletedAt != 0 || got.BackupID.Valid {
		t.Fatalf("复原后须双列同清，实际 deleted_at=%d backup_id=%+v", got.DeletedAt, got.BackupID)
	}
}

// TestResolveFileStateGenerationOrder /store/ 状态路由代次：活行优先返回活行引用；
// 全删时取最新删代（deleted_at DESC）
func TestResolveFileStateGenerationOrder(t *testing.T) {
	svc, _ := newLifecycleTestService(t)
	ctx := context.Background()
	const relPath = "store/resource/gen.mp4"

	// 逐代「插入→软删」构造两死一活（部分唯一索引约束活行路径唯一，多活行同路径不可直插）。
	// 死代引用的备份清单行须真实存在（persistent_store.backup_id 外键防线）
	if err := svc.repo.(*PersistentStoreRepository).GORM().Exec("INSERT INTO backup (id, create_time, update_time) VALUES (101, 0, 0), (102, 0, 0)").Error; err != nil {
		t.Fatalf("建备份种子失败: %v", err)
	}
	oldRow := insertStoreRow(t, svc, relPath)
	if err := svc.repo.SoftDeleteWithBackup(ctx, oldRow.GetID(), 101); err != nil {
		t.Fatalf("软删旧行失败: %v", err)
	}
	time.Sleep(2 * time.Millisecond) // 相邻软删垫 2ms：同毫秒死代令 deleted_at DESC 排序不稳
	newRow := insertStoreRow(t, svc, relPath)
	if err := svc.repo.SoftDeleteWithBackup(ctx, newRow.GetID(), 102); err != nil {
		t.Fatalf("软删新行失败: %v", err)
	}
	liveRow := insertStoreRow(t, svc, relPath)

	// 活行存在：活行优先（backup_id=0，活行不持有备份引用）
	completed, deleted, backupId := svc.ResolveFileState(ctx, relPath)
	if completed != true || deleted != false || backupId != 0 {
		t.Fatalf("有活行时期望 (true,false,0)，实际 (%v,%v,%d)", completed, deleted, backupId)
	}

	// 活行消失：取最新删代（deleted_at 更大的行，backup_id=102）
	if err := svc.repo.DeleteUnscoped(ctx, liveRow.GetID()); err != nil {
		t.Fatalf("物理删活行失败: %v", err)
	}
	completed, deleted, backupId = svc.ResolveFileState(ctx, relPath)
	if completed != true || deleted != true || backupId != 102 {
		t.Fatalf("全删行时期望最新删代 (true,true,102)，实际 (%v,%v,%d)", completed, deleted, backupId)
	}
}
