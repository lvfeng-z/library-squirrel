package backupGovernance

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/library-squirrel/backend/backup"
	"github.com/library-squirrel/backend/base/logger"
	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/persistentStore"
	"github.com/library-squirrel/backend/plugin"
	"github.com/library-squirrel/backend/util"

	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// 治理路径全程记日志，测试进程无 logger.Init——注入 Nop 防全局 logger.Log 为 nil
func TestMain(m *testing.M) {
	logger.Log = zap.NewNop().Sugar()
	os.Exit(m.Run())
}

// stubRetention 固定保留期的设置提供者替身
type stubRetention struct {
	days int
}

func (s *stubRetention) GetBackupGovernanceRetentionDays() int { return s.days }

// failReferencer 引用集查询失败的引用方（正向熔断测试）
type failReferencer struct{}

func (f *failReferencer) Name() string { return "失败方" }
func (f *failReferencer) ListReferencedBackupIDs(ctx context.Context) ([]int64, error) {
	return nil, fmt.Errorf("模拟引用集查询失败")
}
func (f *failReferencer) ClearBackupRefsByBackupIDs(ctx context.Context, ids []int64) error {
	return nil
}

// newGovernanceTestEnv 内存库 + 真实 backup/persistentStore/plugin 服务 + 治理服务。
// persistentStore 注入真实 FileMover（backup.Service），DeleteWithBackup 全链可用
func newGovernanceTestEnv(t *testing.T, retentionDays int, extra ...BackupReferencer) (*Service, *backup.Service, *persistentStore.Service, *gorm.DB, string) {
	t.Helper()
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	if err := db.AutoMigrate(&domain.Backup{}, &domain.PersistentStore{}, &domain.Plugin{}); err != nil {
		t.Fatalf("迁移测试实体失败: %v", err)
	}
	workDir := t.TempDir()
	backupSvc := backup.NewService(backup.NewRepository(db), func() string { return workDir })
	psSvc := persistentStore.NewService(persistentStore.NewRepository(db), backupSvc, func() string { return workDir })
	pluginSvc := plugin.NewService(plugin.NewRepository(db), nil)
	svc := NewService(
		backupSvc,
		append([]BackupReferencer{psSvc, pluginSvc}, extra...),
		&stubRetention{days: retentionDays},
	)
	return svc, backupSvc, psSvc, db, workDir
}

var srcSeq int

// makeBackup 建真实保管清单行（源文件复制入 workDir/backup/…），ageDays>0 时回拨创建时间
func makeBackup(t *testing.T, backupSvc *backup.Service, db *gorm.DB, workDir string, ageDays int64) *domain.Backup {
	t.Helper()
	srcSeq++
	src := filepath.Join(t.TempDir(), fmt.Sprintf("src_%d.bin", srcSeq))
	if err := os.WriteFile(src, []byte("backup-content"), 0o644); err != nil {
		t.Fatalf("造源文件失败: %v", err)
	}
	row, err := backupSvc.CreateBackup(context.Background(), src)
	if err != nil {
		t.Fatalf("建备份失败: %v", err)
	}
	if ageDays > 0 {
		if err := db.Exec("UPDATE backup SET create_time = ? WHERE id = ?",
			util.GetCurrentTimestamp()-ageDays*24*60*60*1000, row.GetID()).Error; err != nil {
			t.Fatalf("回拨创建时间失败: %v", err)
		}
	}
	return row
}

// makeStoreRow 插活行 store 记录
func makeStoreRow(t *testing.T, db *gorm.DB, relPath string) *domain.PersistentStore {
	t.Helper()
	row := domain.NewPersistentStore()
	row.FilePath = sql.NullString{String: relPath, Valid: true}
	row.CompletedAt = 1
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("插 store 行失败: %v", err)
	}
	return row
}

// makePluginRow 插 plugin 记录（backupId>0 时带引用；uninstalled=true 造已卸载行）
func makePluginRow(t *testing.T, db *gorm.DB, backupId int64, uninstalled bool) *domain.Plugin {
	t.Helper()
	row := domain.NewPlugin()
	if backupId > 0 {
		row.BackupID = sql.NullInt64{Int64: backupId, Valid: true}
	}
	if uninstalled {
		row.Uninstalled = sql.NullBool{Bool: true, Valid: true}
	}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("插 plugin 行失败: %v", err)
	}
	return row
}

// softDeleteStoreWithRef 走生产路径软删 store 行并写备份引用（回收站条目的真实形态）
func softDeleteStoreWithRef(t *testing.T, db *gorm.DB, storeId, backupId int64) {
	t.Helper()
	if err := persistentStore.NewRepository(db).SoftDeleteWithBackup(context.Background(), storeId, backupId); err != nil {
		t.Fatalf("软删 store 行失败: %v", err)
	}
}

// readStoreRow 直读 store 行（Unscoped 含已删行）
func readStoreRow(t *testing.T, db *gorm.DB, id int64) *domain.PersistentStore {
	t.Helper()
	var row domain.PersistentStore
	if err := db.Unscoped().First(&row, "id = ?", id).Error; err != nil {
		t.Fatalf("查 store 行 %d 失败: %v", id, err)
	}
	return &row
}

// readPluginRow 直读 plugin 行
func readPluginRow(t *testing.T, db *gorm.DB, id int64) *domain.Plugin {
	t.Helper()
	var row domain.Plugin
	if err := db.First(&row, "id = ?", id).Error; err != nil {
		t.Fatalf("查 plugin 行 %d 失败: %v", id, err)
	}
	return &row
}

func backupExists(t *testing.T, db *gorm.DB, id int64) bool {
	t.Helper()
	var count int64
	if err := db.Model(&domain.Backup{}).Where("id = ?", id).Count(&count).Error; err != nil {
		t.Fatalf("查 backup 行 %d 失败: %v", id, err)
	}
	return count > 0
}

// TestRunOnceOrphanCleanup 正向无主清理：无主超期→清文件与清单行；未超期保留；
// 软删 store 行引用的超期备份保留（核心回归——软删行是合法引用者，引用集查询
// 漏 Unscoped 时该备份会被误清、回收站复原不可用）
func TestRunOnceOrphanCleanup(t *testing.T) {
	svc, backupSvc, _, db, workDir := newGovernanceTestEnv(t, 7)
	ctx := context.Background()

	orphan := makeBackup(t, backupSvc, db, workDir, 8)     // 无主超期 → 清
	fresh := makeBackup(t, backupSvc, db, workDir, 0)      // 无主未超期 → 保留
	referenced := makeBackup(t, backupSvc, db, workDir, 8) // 软删行引用 + 超期 → 保留
	store := makeStoreRow(t, db, "store/resource/c.mp4")
	softDeleteStoreWithRef(t, db, store.GetID(), referenced.GetID())

	svc.RunOnce(ctx)

	if backupExists(t, db, orphan.GetID()) {
		t.Fatalf("无主超期备份 %d 应被清理", orphan.GetID())
	}
	if absPath := backupSvc.GetBackupPath(orphan); util.FileExists(absPath) {
		t.Fatalf("无主备份文件应被删除: %s", absPath)
	}
	if !backupExists(t, db, fresh.GetID()) {
		t.Fatalf("未超期备份 %d 不应被清理", fresh.GetID())
	}
	if !backupExists(t, db, referenced.GetID()) {
		t.Fatalf("软删行引用的备份 %d 被误清（引用集查询漏含已删行）", referenced.GetID())
	}
	if absPath := backupSvc.GetBackupPath(referenced); !util.FileExists(absPath) {
		t.Fatalf("被引用备份文件不应被删除: %s", absPath)
	}
	if got := readStoreRow(t, db, store.GetID()).BackupID; got != referenced.GetID() {
		t.Fatalf("软删行引用被误清: backup_id=%d 期望 %d", got, referenced.GetID())
	}
}

// TestRunOnceUninstalledPluginRefKept plugin 已卸载行引用不清（卸载行持有重装能力引用，合法有主）
func TestRunOnceUninstalledPluginRefKept(t *testing.T) {
	svc, backupSvc, _, db, workDir := newGovernanceTestEnv(t, 7)
	ctx := context.Background()

	backupRow := makeBackup(t, backupSvc, db, workDir, 8)
	makePluginRow(t, db, backupRow.GetID(), true)

	svc.RunOnce(ctx)

	if !backupExists(t, db, backupRow.GetID()) {
		t.Fatalf("已卸载插件行引用的备份 %d 被误清", backupRow.GetID())
	}
}

// TestRunOnceDanglingRefsCleared 反向悬空清列：引用的清单行不存在时，
// persistent_store 置 0（含已删行）、plugin 置 NULL
func TestRunOnceDanglingRefsCleared(t *testing.T) {
	svc, _, _, db, _ := newGovernanceTestEnv(t, 7)
	ctx := context.Background()

	store := makeStoreRow(t, db, "store/resource/dangling.mp4")
	softDeleteStoreWithRef(t, db, store.GetID(), 999) // 999 无对应清单行
	pRow := makePluginRow(t, db, 888, false)          // 888 无对应清单行

	svc.RunOnce(ctx)

	if got := readStoreRow(t, db, store.GetID()).BackupID; got != 0 {
		t.Fatalf("已删 store 行悬空引用未清: backup_id=%d", got)
	}
	p := readPluginRow(t, db, pRow.GetID())
	if p.BackupID.Valid {
		t.Fatalf("plugin 悬空引用未置 NULL: %d", p.BackupID.Int64)
	}
}

// TestRunOnceIllegalAliveRefsCleared 防御清列：活行携带 backup_id>0 构造上不可达，
// 对账检出即清（引用的清单行本轮在引用集内，不受正向清理影响）
func TestRunOnceIllegalAliveRefsCleared(t *testing.T) {
	svc, backupSvc, _, db, workDir := newGovernanceTestEnv(t, 7)
	ctx := context.Background()

	backupRow := makeBackup(t, backupSvc, db, workDir, 0)
	store := makeStoreRow(t, db, "store/resource/alive.mp4")
	// 直改 DB 造非法态（活行带引用）
	if err := db.Exec("UPDATE persistent_store SET backup_id = ? WHERE id = ?", backupRow.GetID(), store.GetID()).Error; err != nil {
		t.Fatalf("造非法态失败: %v", err)
	}

	svc.RunOnce(ctx)

	if got := readStoreRow(t, db, store.GetID()); got.DeletedAt != 0 || got.BackupID != 0 {
		t.Fatalf("活行非法引用未清: deleted_at=%d backup_id=%d", got.DeletedAt, got.BackupID)
	}
	if !backupExists(t, db, backupRow.GetID()) {
		t.Fatalf("被引用的清单行 %d 不应被清理", backupRow.GetID())
	}
}

// TestComputeReferencerStats 监视哨统计：按引用方分组正确计算数量与最老引用年龄
func TestComputeReferencerStats(t *testing.T) {
	svc, backupSvc, _, db, workDir := newGovernanceTestEnv(t, 7)
	ctx := context.Background()

	oldBackup := makeBackup(t, backupSvc, db, workDir, 100)
	newBackup := makeBackup(t, backupSvc, db, workDir, 10)
	store := makeStoreRow(t, db, "store/resource/old.mp4")
	softDeleteStoreWithRef(t, db, store.GetID(), oldBackup.GetID())
	makePluginRow(t, db, newBackup.GetID(), false)

	refs, _ := svc.collectReferenced(ctx)
	stats, err := svc.computeReferencerStats(ctx, refs)
	if err != nil {
		t.Fatalf("统计失败: %v", err)
	}
	byName := make(map[string]ReferencerStats, len(stats))
	for _, st := range stats {
		byName[st.Name] = st
	}
	psStat, ok := byName["作品存储"]
	if !ok || psStat.Count != 1 {
		t.Fatalf("作品存储统计缺失或数量错误: %+v", psStat)
	}
	if psStat.OldestAgeDays < 100 || psStat.OldestAgeDays > 101 {
		t.Fatalf("作品存储最老引用年龄应约 100 天，实际 %d", psStat.OldestAgeDays)
	}
	plStat, ok := byName["插件"]
	if !ok || plStat.Count != 1 {
		t.Fatalf("插件统计缺失或数量错误: %+v", plStat)
	}
	if plStat.OldestAgeDays < 10 || plStat.OldestAgeDays > 11 {
		t.Fatalf("插件最老引用年龄应约 10 天，实际 %d", plStat.OldestAgeDays)
	}
}

// TestRunOnceReferencerFailureFuse 引用集查询失败熔断：失败方存在时本轮正向清理整体跳过
// （该方引用呈现为零引用，进候选即误清活备份）
func TestRunOnceReferencerFailureFuse(t *testing.T) {
	svc, backupSvc, _, db, workDir := newGovernanceTestEnv(t, 7, &failReferencer{})
	ctx := context.Background()

	orphan := makeBackup(t, backupSvc, db, workDir, 8) // 无主超期，但存在失败方

	svc.RunOnce(ctx)

	if !backupExists(t, db, orphan.GetID()) {
		t.Fatalf("存在引用集查询失败方时正向清理应熔断，备份 %d 被误清", orphan.GetID())
	}
}

// TestEndToEndRestoreAfterGovernance 端到端总验证（误清防护）：作品软删 → 备份超龄 →
// 治理巡检 → 软删行引用仍在 → 复原全链可用（文件取回 + 行复活）
func TestEndToEndRestoreAfterGovernance(t *testing.T) {
	svc, backupSvc, psSvc, db, workDir := newGovernanceTestEnv(t, 7)
	ctx := context.Background()

	// 1. 造真实文件与记录，走生产软删链（文件移入 backup、行软删带 backup_id）
	const relPath = "store/resource/e2e.mp4"
	absPath := filepath.Join(workDir, relPath)
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		t.Fatalf("建目录失败: %v", err)
	}
	if err := os.WriteFile(absPath, []byte("e2e-content"), 0o644); err != nil {
		t.Fatalf("造文件失败: %v", err)
	}
	store := makeStoreRow(t, db, relPath)
	backupId, err := psSvc.DeleteWithBackup(ctx, store.GetID())
	if err != nil || backupId <= 0 {
		t.Fatalf("软删链失败: %v, backupId=%d", err, backupId)
	}
	if util.FileExists(absPath) {
		t.Fatalf("软删后源文件应已移入 backup")
	}

	// 2. 备份超龄（8 天 > 保留期 7 天）后治理巡检
	if err := db.Exec("UPDATE backup SET create_time = ? WHERE id = ?",
		util.GetCurrentTimestamp()-8*24*60*60*1000, backupId).Error; err != nil {
		t.Fatalf("回拨创建时间失败: %v", err)
	}
	svc.RunOnce(ctx)

	// 3. 软删行引用保护了备份：清单行在、引用在
	if !backupExists(t, db, backupId) {
		t.Fatalf("软删行引用的备份被治理误清，复原将不可用")
	}
	if got := readStoreRow(t, db, store.GetID()).BackupID; got != backupId {
		t.Fatalf("软删行引用丢失: backup_id=%d 期望 %d", got, backupId)
	}

	// 4. 复原全链：按行内 backup_id 定位备份 → 文件还原回原路径 → 行复活（引用双清）
	backupAbs := backupSvc.ResolveBackupPathById(ctx, backupId)
	if backupAbs == "" || !util.FileExists(backupAbs) {
		t.Fatalf("备份文件应存在: %q", backupAbs)
	}
	if err := backupSvc.RestoreFile(ctx, backupAbs, absPath); err != nil {
		t.Fatalf("还原文件失败: %v", err)
	}
	if !util.FileExists(absPath) {
		t.Fatalf("还原后源文件应回到原路径")
	}
	if err := persistentStore.NewRepository(db).RestoreByIds(ctx, []int64{store.GetID()}); err != nil {
		t.Fatalf("复活 store 行失败: %v", err)
	}
	if got := readStoreRow(t, db, store.GetID()); got.DeletedAt != 0 || got.BackupID != 0 {
		t.Fatalf("复原后行应复活且引用双清: deleted_at=%d backup_id=%d", got.DeletedAt, got.BackupID)
	}
}
