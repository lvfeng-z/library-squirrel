package recycleBin

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/library-squirrel/backend/base/logger"
	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/persistentStore"
	"github.com/library-squirrel/backend/resource"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// purgeStoreTestEnv 内存库 + 真实 persistentStore 服务（文件条目清理链焦点件）+ 记账 BackupReader
type purgeStoreTestEnv struct {
	svc     *Service
	psSvc   *persistentStore.Service
	backup  *recordingBackupReader
	db      *gorm.DB
	workDir string
}

func newPurgeStoreTestEnv(t *testing.T) *purgeStoreTestEnv {
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
	psSvc := persistentStore.NewService(persistentStore.NewRepository(db), nil, func() string { return workDir })
	backup := &recordingBackupReader{}
	svc := NewService(nil, backup, nil, nil, psSvc, psSvc, nil, nil, func() string { return workDir }, nil, nil,
		&wsTransactor{db: db}, resource.NewResourceStoreRepository(db), nil, nil)
	return &purgeStoreTestEnv{svc: svc, psSvc: psSvc, backup: backup, db: db, workDir: workDir}
}

// insertDeletedStore 插一行并软删（backupId 写行内引用；relPath 为空则不落 file_path）
func (e *purgeStoreTestEnv) insertDeletedStore(t *testing.T, name, relPath string, backupId int64) int64 {
	t.Helper()
	s := domain.NewPersistentStore()
	s.FileName = sql.NullString{String: name, Valid: true}
	if relPath != "" {
		s.FilePath = sql.NullString{String: relPath, Valid: true}
	}
	s.CompletedAt = 1
	if err := e.db.Create(s).Error; err != nil {
		t.Fatalf("插 store 失败: %v", err)
	}
	// 备份清单行种子（persistent_store.backup_id 外键防线——行内引用须指向存在行）
	if backupId > 0 {
		if err := e.db.Exec("INSERT OR IGNORE INTO backup (id, create_time, update_time) VALUES (?, 0, 0)", backupId).Error; err != nil {
			t.Fatalf("建备份行失败: %v", err)
		}
	}
	if err := e.db.Exec("UPDATE persistent_store SET deleted_at = 2000, backup_id = NULLIF(?, 0) WHERE id = ?", backupId, s.GetID()).Error; err != nil {
		t.Fatalf("软删 store 失败: %v", err)
	}
	return s.GetID()
}

// storeRowExists 含已删行判定行是否仍存在
func (e *purgeStoreTestEnv) storeRowExists(t *testing.T, id int64) bool {
	t.Helper()
	var n int64
	if err := e.db.Unscoped().Model(&domain.PersistentStore{}).Where("id = ?", id).Count(&n).Error; err != nil {
		t.Fatalf("统计行失败: %v", err)
	}
	return n > 0
}

// TestPurgeStoreConsumesBackup 彻底删除文件条目：行物理消亡 + 行内引用的备份消费式删除
func TestPurgeStoreConsumesBackup(t *testing.T) {
	env := newPurgeStoreTestEnv(t)
	storeId := env.insertDeletedStore(t, "带备份残迹", "store/resource/作者/带备份残迹.mp4", 701)

	if err := env.svc.PurgeStore(context.Background(), storeId); err != nil {
		t.Fatalf("清理文件条目失败: %v", err)
	}
	if env.storeRowExists(t, storeId) {
		t.Fatalf("清理后 store 行应物理消亡")
	}
	consumed := false
	for _, id := range env.backup.deletedIds {
		if id == 701 {
			consumed = true
		}
	}
	if !consumed {
		t.Fatalf("行内 backup_id=701 应被消费式删除，实际删除 %v", env.backup.deletedIds)
	}
}

// TestPurgeStoreWithoutBackup 无备份行（MarkInvalid 失效形态）清理：仅删行，无备份调用
func TestPurgeStoreWithoutBackup(t *testing.T) {
	env := newPurgeStoreTestEnv(t)
	storeId := env.insertDeletedStore(t, "失效行", "store/resource/作者/失效行.png", 0)

	if err := env.svc.PurgeStore(context.Background(), storeId); err != nil {
		t.Fatalf("清理失效行失败: %v", err)
	}
	if env.storeRowExists(t, storeId) {
		t.Fatalf("清理后失效行应物理消亡")
	}
	if len(env.backup.deletedIds) != 0 {
		t.Fatalf("无备份行不应触发备份删除，实际删除 %v", env.backup.deletedIds)
	}
}

// TestPurgeStoreRemovesResidualFile 尽力删文件：file_path 指向 backup/ 域的实存残迹文件随行清除
// （无保管清单行的散落文件，不删即不可见垃圾）；不存在的路径扑空无害
func TestPurgeStoreRemovesResidualFile(t *testing.T) {
	env := newPurgeStoreTestEnv(t)
	relPath := "backup/2026/08/21/残迹文件.bin"
	absPath := filepath.Join(env.workDir, relPath)
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		t.Fatalf("造目录失败: %v", err)
	}
	if err := os.WriteFile(absPath, []byte("residual"), 0o644); err != nil {
		t.Fatalf("造残迹文件失败: %v", err)
	}
	storeId := env.insertDeletedStore(t, "残迹行", relPath, 0)

	if err := env.svc.PurgeStore(context.Background(), storeId); err != nil {
		t.Fatalf("清理残迹行失败: %v", err)
	}
	if _, err := os.Stat(absPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("残迹文件应随行清除，实际 Stat 结果 err=%v", err)
	}
	// 不存在路径的行：清理扑空无害不报错
	storeId2 := env.insertDeletedStore(t, "路径已失行", "store/resource/作者/不存在.mp4", 0)
	if err := env.svc.PurgeStore(context.Background(), storeId2); err != nil {
		t.Fatalf("路径扑空的清理不应报错: %v", err)
	}
}

// TestPurgeStoreRemovesAssociations 软删 store（带 resource_store 关联）→ PurgeStore →
// 关联行归零。外键强制库下清理成功本身即「事务内先摘关联后删行」顺序的证明
// （关联未摘即物理删 store 行直接 FK 违约报错）
func TestPurgeStoreRemovesAssociations(t *testing.T) {
	env := newPurgeStoreTestEnv(t)
	// 挂载链种子（resource_store 的 resource_id/store_id 两面外键防线）
	if err := env.db.Exec("INSERT INTO work (id, create_time, update_time, deleted_at) VALUES (1, 0, 0, 0)").Error; err != nil {
		t.Fatalf("建作品种子失败: %v", err)
	}
	if err := env.db.Exec("INSERT INTO resource (id, create_time, update_time, work_id, resource_type) VALUES (1, 0, 0, 1, 'image')").Error; err != nil {
		t.Fatalf("建资源种子失败: %v", err)
	}
	storeId := env.insertDeletedStore(t, "带关联行", "store/resource/作者/带关联行.png", 0)
	if err := env.db.Exec("INSERT INTO resource_store (resource_id, store_id, store_type, store_seq, create_time, update_time) VALUES (1, ?, 'image', 1, 0, 0)", storeId).Error; err != nil {
		t.Fatalf("建关联种子失败: %v", err)
	}

	if err := env.svc.PurgeStore(context.Background(), storeId); err != nil {
		t.Fatalf("清理带关联条目失败: %v", err)
	}
	if env.storeRowExists(t, storeId) {
		t.Fatalf("清理后 store 行应物理消亡")
	}
	var assocCount int64
	if err := env.db.Model(&domain.ResourceStore{}).Where("store_id = ?", storeId).Count(&assocCount).Error; err != nil {
		t.Fatalf("统计关联行失败: %v", err)
	}
	if assocCount != 0 {
		t.Fatalf("resource_store 关联行应归零，剩余 %d 行", assocCount)
	}
}

// TestPurgeStoreRejectsAliveRow 活行与不存在行拒绝清理（入口校验：仅已删条目）
func TestPurgeStoreRejectsAliveRow(t *testing.T) {
	env := newPurgeStoreTestEnv(t)
	alive := domain.NewPersistentStore()
	if err := env.db.Create(alive).Error; err != nil {
		t.Fatalf("插活行失败: %v", err)
	}
	if err := env.svc.PurgeStore(context.Background(), alive.GetID()); !errors.Is(err, ErrRecycleStoreNotFound) {
		t.Fatalf("活行清理应返回 ErrRecycleStoreNotFound，实际 %v", err)
	}
	if err := env.svc.PurgeStore(context.Background(), 99999); !errors.Is(err, ErrRecycleStoreNotFound) {
		t.Fatalf("不存在行清理应返回 ErrRecycleStoreNotFound，实际 %v", err)
	}
	if !env.storeRowExists(t, alive.GetID()) {
		t.Fatalf("活行不应被误删")
	}
}

// TestPurgeStoreFileFailureRetainsRecord Phase A 文件删除失败（非缺失）→ 返回错误、记录保留，
// 不静默删记录制造「记录消失、文件残留」孤儿。用非空目录制造跨平台 os.Remove 失败
func TestPurgeStoreFileFailureRetainsRecord(t *testing.T) {
	env := newPurgeStoreTestEnv(t)
	rel := "store/resource/作者/删不动"
	if err := os.MkdirAll(filepath.Join(env.workDir, rel, "inner"), 0o755); err != nil {
		t.Fatalf("造非空目录失败: %v", err)
	}
	storeId := env.insertDeletedStore(t, "删除失败残迹", rel, 0)

	if err := env.svc.PurgeStore(context.Background(), storeId); err == nil {
		t.Fatalf("文件删除失败应返回错误，实际 nil")
	}
	if !env.storeRowExists(t, storeId) {
		t.Fatalf("文件删除失败后记录应保留，实际被删")
	}
}

// TestPurgeStoreRecordsKeepsFile 仅删记录（用户对文件失败显式选择）：记录删除、磁盘文件保留
func TestPurgeStoreRecordsKeepsFile(t *testing.T) {
	env := newPurgeStoreTestEnv(t)
	rel := "store/resource/作者/仅删记录.mp4"
	abs := filepath.Join(env.workDir, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("造目录失败: %v", err)
	}
	if err := os.WriteFile(abs, []byte("x"), 0o644); err != nil {
		t.Fatalf("造文件失败: %v", err)
	}
	storeId := env.insertDeletedStore(t, "仅删记录", rel, 0)

	if err := env.svc.PurgeStoreRecords(context.Background(), storeId); err != nil {
		t.Fatalf("仅删记录应成功，实际 %v", err)
	}
	if env.storeRowExists(t, storeId) {
		t.Fatalf("仅删记录后行应删除")
	}
	if _, err := os.Stat(abs); err != nil {
		t.Fatalf("仅删记录不应动磁盘文件，实际 %v", err)
	}
}

// 保障 logger（persistentStore 路径记日志，未初始化会 nil panic）
func init() {
	if logger.Log == nil {
		logger.Log = zap.NewNop().Sugar()
	}
}
