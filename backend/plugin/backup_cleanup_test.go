package plugin

import (
	"archive/zip"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	domain "github.com/library-squirrel/backend/base/model/dto"
	entity "github.com/library-squirrel/backend/base/model/entity"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// fakeBackupProvider BackupProvider 测试替身：内存清单（自增 ID 记录 Create/Delete 调用）；
// failCreate/failDelete 注入对应步骤失败
type fakeBackupProvider struct {
	failCreate bool
	failDelete bool
	nextId     int64
	rows       map[int64]*entity.Backup
	deleted    []int64
}

func (f *fakeBackupProvider) CreateBackup(ctx context.Context, sourcePath string) (*entity.Backup, error) {
	if f.failCreate {
		return nil, fmt.Errorf("模拟创建备份失败")
	}
	f.nextId++
	row := entity.NewBackup()
	row.SetID(f.nextId)
	row.FileName = sql.NullString{String: sourcePath, Valid: true}
	f.rows[row.GetID()] = row
	return row, nil
}

func (f *fakeBackupProvider) GetById(ctx context.Context, id int64) (*entity.Backup, error) {
	row, ok := f.rows[id]
	if !ok {
		return nil, nil
	}
	return row, nil
}

func (f *fakeBackupProvider) DeleteBackup(ctx context.Context, id int64) error {
	if f.failDelete {
		return fmt.Errorf("模拟删除备份失败")
	}
	f.deleted = append(f.deleted, id)
	delete(f.rows, id)
	return nil
}

// markFailRepo 仓储装饰器：全方法委托真实仓储，MarkUninstalledAndClearBackup 注入失败
// （模拟标记步骤 DB 错误，验证卸载整体中止且备份未动）
type markFailRepo struct {
	Repository
}

func (r *markFailRepo) MarkUninstalledAndClearBackup(ctx context.Context, publicId string, expectedBackupId int64) error {
	return fmt.Errorf("模拟标记失败")
}

// newCleanupTestService 内存库 + 真实仓储 + fake BackupProvider 的 Service（生产构造函数装配）
func newCleanupTestService(t *testing.T) (*Service, *fakeBackupProvider) {
	t.Helper()
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	if err := db.AutoMigrate(&entity.Plugin{}); err != nil {
		t.Fatalf("迁移测试实体失败: %v", err)
	}
	provider := &fakeBackupProvider{rows: map[int64]*entity.Backup{}}
	svc := NewService(NewRepository(db), provider)
	return svc, provider
}

// plantPluginRow 预置插件行（RootPath 指向不存在目录：卸载链 removeFiles 仅 Warn 不影响流程）
func plantPluginRow(t *testing.T, svc *Service, publicId string, backupId int64) {
	t.Helper()
	row := entity.NewPlugin()
	row.PublicID = sql.NullString{String: publicId, Valid: true}
	row.Name = sql.NullString{String: "fake", Valid: true}
	row.Version = sql.NullString{String: "1.0.0", Valid: true}
	if backupId > 0 {
		row.BackupID = sql.NullInt64{Int64: backupId, Valid: true}
	}
	row.Uninstalled = sql.NullBool{Bool: false, Valid: true}
	row.RootPath = sql.NullString{String: "plugin/package/nonexistent/0.0.0", Valid: true}
	if err := svc.repo.Create(context.Background(), row); err != nil {
		t.Fatalf("预置插件行失败: %v", err)
	}
}

// makePackageZip 造一个仅含占位 plugin.json 的安装包 zip（installCore 的 ExtractZip 数据源）
func makePackageZip(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pkg.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("造包文件失败: %v", err)
	}
	defer f.Close()
	w := zip.NewWriter(f)
	entry, err := w.Create("plugin.json")
	if err != nil {
		t.Fatalf("造包条目失败: %v", err)
	}
	if _, err := entry.Write([]byte("{}")); err != nil {
		t.Fatalf("写包条目失败: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("闭合包失败: %v", err)
	}
	return path
}

func newInstallDTO(publicId, version, packagePath string) *domain.PluginInstallDTO {
	return &domain.PluginInstallDTO{
		PublicID:    publicId,
		Author:      "tester",
		Name:        "fake",
		Version:     version,
		EntryFile:   "entry.exe",
		Activation:  domain.PluginActivation{Type: 1},
		PackagePath: packagePath,
	}
}

// referencedContains 引用集包含判定
func referencedContains(ctx context.Context, t *testing.T, svc *Service, id int64) bool {
	t.Helper()
	ids, err := svc.ListReferencedBackupIDs(ctx)
	if err != nil {
		t.Fatalf("查询引用集失败: %v", err)
	}
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

// TestUninstallClearsBackup 卸载清备份：行标记已卸载且清空引用、备份被消费式删除、引用集收缩
func TestUninstallClearsBackup(t *testing.T) {
	svc, provider := newCleanupTestService(t)
	ctx := context.Background()
	provider.rows[5] = entity.NewBackup()
	plantPluginRow(t, svc, "com.example.fake", 5)

	if err := svc.Uninstall(ctx, "com.example.fake"); err != nil {
		t.Fatalf("卸载失败: %v", err)
	}

	row, err := svc.repo.GetByPublicId(ctx, "com.example.fake")
	if err != nil || row == nil {
		t.Fatalf("查询行失败: %v, row=%v", err, row)
	}
	if !row.Uninstalled.Valid || !row.Uninstalled.Bool {
		t.Fatalf("卸载后行须 uninstalled=true，实际 %+v", row.Uninstalled)
	}
	if row.BackupID.Valid {
		t.Fatalf("卸载后行须清空备份引用，实际 backup_id=%d", row.BackupID.Int64)
	}
	if len(provider.deleted) != 1 || provider.deleted[0] != 5 {
		t.Fatalf("卸载须消费式删除备份 5，实际 %v", provider.deleted)
	}
	if _, ok := provider.rows[5]; ok {
		t.Fatal("备份清单行 5 应已删除")
	}
	if referencedContains(ctx, t, svc, 5) {
		t.Fatal("卸载后引用集不应再含备份 5")
	}
}

// TestUninstallBackupDeleteFailureTolerated 卸载删备份失败容错：行已清列、卸载整体成功（Warn 路径留无主）
func TestUninstallBackupDeleteFailureTolerated(t *testing.T) {
	svc, provider := newCleanupTestService(t)
	ctx := context.Background()
	provider.rows[5] = entity.NewBackup()
	plantPluginRow(t, svc, "com.example.fake", 5)
	provider.failDelete = true

	if err := svc.Uninstall(ctx, "com.example.fake"); err != nil {
		t.Fatalf("删备份失败不应令卸载整体失败: %v", err)
	}

	row, _ := svc.repo.GetByPublicId(ctx, "com.example.fake")
	if !row.Uninstalled.Bool || row.BackupID.Valid {
		t.Fatalf("容错路径行仍须标记+清列，实际 uninstalled=%+v backup_id=%+v", row.Uninstalled, row.BackupID)
	}
	if len(provider.deleted) != 0 {
		t.Fatalf("删除失败不应记入已删，实际 %v", provider.deleted)
	}
	if _, ok := provider.rows[5]; !ok {
		t.Fatal("删除失败时备份清单行应保留（成无主，治理兜底）")
	}
}

// TestUninstallMarkFailureAborts 卸载标记失败：整体报错可重试，备份未动
func TestUninstallMarkFailureAborts(t *testing.T) {
	svc, provider := newCleanupTestService(t)
	ctx := context.Background()
	provider.rows[5] = entity.NewBackup()
	plantPluginRow(t, svc, "com.example.fake", 5)
	// 换装标记失败装饰器（Service 持接口，测试同包可替换）
	svc.repo = &markFailRepo{Repository: svc.repo}

	if err := svc.Uninstall(ctx, "com.example.fake"); err == nil {
		t.Fatal("标记失败应令卸载整体报错")
	}

	svc.repo = svc.repo.(*markFailRepo).Repository // 还原真实仓储后核验
	row, _ := svc.repo.GetByPublicId(ctx, "com.example.fake")
	if row.Uninstalled.Bool {
		t.Fatal("标记失败后行不应变为已卸载")
	}
	if !row.BackupID.Valid || row.BackupID.Int64 != 5 {
		t.Fatalf("标记失败后备份引用须原样保留，实际 %+v", row.BackupID)
	}
	if len(provider.deleted) != 0 {
		t.Fatalf("标记失败后不应删除备份，实际 %v", provider.deleted)
	}
}

// TestInstallCoreReinstallCleansPrevBackup 换版清旧备份：重装成功后旧备份直清、新备份保留且行内引用切换
func TestInstallCoreReinstallCleansPrevBackup(t *testing.T) {
	svc, provider := newCleanupTestService(t)
	ctx := context.Background()
	provider.rows[5] = entity.NewBackup()
	plantPluginRow(t, svc, "com.example.fake", 5)

	plugin, err := svc.installCore(ctx, newInstallDTO("com.example.fake", "1.0.1", makePackageZip(t)), true, installContext{Source: SourceLocal, Trusted: true})
	if err != nil {
		t.Fatalf("重装失败: %v", err)
	}

	if !plugin.BackupID.Valid || plugin.BackupID.Int64 == 0 {
		t.Fatalf("重装后行须引用新备份，实际 %+v", plugin.BackupID)
	}
	newId := plugin.BackupID.Int64
	if newId == 5 {
		t.Fatal("新备份 ID 不应与旧备份相同")
	}
	if len(provider.deleted) != 1 || provider.deleted[0] != 5 {
		t.Fatalf("换版须直清旧备份 5，实际 %v", provider.deleted)
	}
	if _, ok := provider.rows[newId]; !ok {
		t.Fatal("新备份清单行应保留")
	}
	if referencedContains(ctx, t, svc, 5) {
		t.Fatal("换版后引用集不应再含旧备份 5")
	}
	if !referencedContains(ctx, t, svc, newId) {
		t.Fatal("换版后引用集应含新备份")
	}
}

// TestInstallCoreReinstallWithoutPrevBackup 重装无旧备份（引用 NULL）：不触发直清
func TestInstallCoreReinstallWithoutPrevBackup(t *testing.T) {
	svc, provider := newCleanupTestService(t)
	ctx := context.Background()
	plantPluginRow(t, svc, "com.example.fake", 0)

	if _, err := svc.installCore(ctx, newInstallDTO("com.example.fake", "1.0.1", makePackageZip(t)), true, installContext{Source: SourceLocal, Trusted: true}); err != nil {
		t.Fatalf("重装失败: %v", err)
	}
	if len(provider.deleted) != 0 {
		t.Fatalf("无旧备份不应触发直清，实际 %v", provider.deleted)
	}
}

// TestInstallCoreFreshInstallNoCleanup 全新安装（非重装）：不触发直清
func TestInstallCoreFreshInstallNoCleanup(t *testing.T) {
	svc, provider := newCleanupTestService(t)
	ctx := context.Background()

	plugin, err := svc.installCore(ctx, newInstallDTO("com.example.fake", "1.0.0", makePackageZip(t)), false, installContext{Source: SourceLocal, Trusted: true})
	if err != nil {
		t.Fatalf("新装失败: %v", err)
	}
	if !plugin.BackupID.Valid || plugin.BackupID.Int64 == 0 {
		t.Fatalf("新装后行须引用新备份，实际 %+v", plugin.BackupID)
	}
	if len(provider.deleted) != 0 {
		t.Fatalf("新装不应触发直清，实际 %v", provider.deleted)
	}
}

// TestInstallCoreCreateFailureWindow 换版失败窗口：备份创建失败——行内引用已空（Save 覆盖）、
// 旧备份不补偿不删除（成无主由治理兜底）
func TestInstallCoreCreateFailureWindow(t *testing.T) {
	svc, provider := newCleanupTestService(t)
	ctx := context.Background()
	provider.rows[5] = entity.NewBackup()
	plantPluginRow(t, svc, "com.example.fake", 5)
	provider.failCreate = true

	if _, err := svc.installCore(ctx, newInstallDTO("com.example.fake", "1.0.1", makePackageZip(t)), true, installContext{Source: SourceLocal, Trusted: true}); err == nil {
		t.Fatal("创建备份失败应令重装报错")
	}

	row, _ := svc.repo.GetByPublicId(ctx, "com.example.fake")
	if row.BackupID.Valid {
		t.Fatalf("失败窗口行内引用应为 NULL（Save 覆盖所致），实际 %d", row.BackupID.Int64)
	}
	if len(provider.deleted) != 0 {
		t.Fatalf("失败窗口不应删除旧备份，实际 %v", provider.deleted)
	}
	if _, ok := provider.rows[5]; !ok {
		t.Fatal("旧备份清单行应保留（成无主，治理兜底）")
	}
}

// TestMarkUninstalledAndClearBackupCAS 并发守卫：期望引用与行内不符（重装链并发改写）时
// 0 行受影响报状态变化错误且行原样；命中时标记+清列；期望 0 与 NULL 行为 null-safe 匹配
func TestMarkUninstalledAndClearBackupCAS(t *testing.T) {
	svc, _ := newCleanupTestService(t)
	ctx := context.Background()
	plantPluginRow(t, svc, "com.example.fake", 200)
	plantPluginRow(t, svc, "com.example.other", 0) // backup_id NULL 行

	// 期望值与行内不符：中止且行原样
	if err := svc.repo.MarkUninstalledAndClearBackup(ctx, "com.example.fake", 300); !errors.Is(err, ErrPluginStateChanged) {
		t.Fatalf("引用不符应返回 ErrPluginStateChanged，实际 %v", err)
	}
	row, _ := svc.repo.GetByPublicId(ctx, "com.example.fake")
	if row.Uninstalled.Bool || !row.BackupID.Valid || row.BackupID.Int64 != 200 {
		t.Fatalf("守卫命中后行应原样，实际 uninstalled=%v backup_id=%+v", row.Uninstalled.Bool, row.BackupID)
	}

	// 期望值命中：标记+清列
	if err := svc.repo.MarkUninstalledAndClearBackup(ctx, "com.example.fake", 200); err != nil {
		t.Fatalf("命中时应成功: %v", err)
	}
	row, _ = svc.repo.GetByPublicId(ctx, "com.example.fake")
	if !row.Uninstalled.Bool || row.BackupID.Valid {
		t.Fatalf("命中后行须标记+清列，实际 uninstalled=%v backup_id=%+v", row.Uninstalled.Bool, row.BackupID)
	}

	// 期望 0（无备份）与 NULL 行 null-safe 匹配
	if err := svc.repo.MarkUninstalledAndClearBackup(ctx, "com.example.other", 0); err != nil {
		t.Fatalf("NULL 行与期望 0 应匹配: %v", err)
	}
}
