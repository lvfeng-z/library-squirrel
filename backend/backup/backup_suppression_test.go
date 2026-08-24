package backup

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/storeRegistry"

	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// 文件操作失败路径记日志，测试进程无 logger.Init——注入 Nop 防全局 logger.Log 为 nil
func TestMain(m *testing.M) {
	logger.Log = zap.NewNop().Sugar()
	os.Exit(m.Run())
}

// newBackupTestEnv 内存库 + 真实 backup 服务
func newBackupTestEnv(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	if err := db.AutoMigrate(&entity.Backup{}); err != nil {
		t.Fatalf("迁移测试实体失败: %v", err)
	}
	workDir := t.TempDir()
	return NewService(NewRepository(db), func() string { return workDir }), db
}

// writeSource 在 workDir 外写一个源文件（CreateBackup 复制模式，不产生源端事件）
func writeSource(t *testing.T, name string) string {
	t.Helper()
	src := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(src, []byte("content-"+name), 0o644); err != nil {
		t.Fatal(err)
	}
	return src
}

// TestBackupQueryFaces 查询面：GetByFilePath 精确命中/未命中、ListByPathPrefix 前缀圈行、
// ListAllInWorkDir 全量并排除旧工作目录行（工作目录迁移前的行不在当前监控树）、UpdateFilePath 生效
func TestBackupQueryFaces(t *testing.T) {
	svc, db := newBackupTestEnv(t)
	ctx := context.Background()

	row, err := svc.CreateBackup(ctx, writeSource(t, "a.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	relPath := row.FilePath.String

	if hit, err := svc.GetByFilePath(ctx, relPath); err != nil || hit == nil || hit.GetID() != row.GetID() {
		t.Fatalf("GetByFilePath 应精确命中新建行，hit=%v err=%v", hit, err)
	}
	if hit, _ := svc.GetByFilePath(ctx, "backup/2099/01/01/none.mp4"); hit != nil {
		t.Fatalf("未命中路径应返回 nil，实际 %+v", hit)
	}

	// 旧工作目录行：同路径字符串不同 workdir，不进当前工作目录的任何查询面
	oldRow := entity.NewBackup()
	oldRow.FileName = row.FileName
	oldRow.FilePath = row.FilePath
	oldRow.Workdir.Valid = true
	oldRow.Workdir.String = `Z:\old\workdir`
	if err := db.Create(oldRow).Error; err != nil {
		t.Fatal(err)
	}
	if hits, _ := svc.ListByPathPrefix(ctx, "backup"); len(hits) != 1 || hits[0].GetID() != row.GetID() {
		t.Fatalf("前缀圈行应仅含当前工作目录行，实际 %+v", hits)
	}
	if all, _ := svc.ListAllInWorkDir(ctx); len(all) != 1 || all[0].GetID() != row.GetID() {
		t.Fatalf("全量投影应仅含当前工作目录行，实际 %+v", all)
	}

	if err := svc.UpdateFilePath(ctx, row.GetID(), "backup/2026/08/24/moved.mp4"); err != nil {
		t.Fatal(err)
	}
	if hit, _ := svc.GetByFilePath(ctx, "backup/2026/08/24/moved.mp4"); hit == nil || hit.GetID() != row.GetID() {
		t.Fatal("UpdateFilePath 后应按新路径命中")
	}
}

// TestNormalizeFilePaths 分隔符规范化：历史反斜杠行修正为正斜杠（fsmonitor backup 域对账的
// 磁盘键/事件路径均为正斜杠，反斜杠行恒判缺失）；正斜杠行不受影响、幂等
func TestNormalizeFilePaths(t *testing.T) {
	svc, db := newBackupTestEnv(t)
	ctx := context.Background()

	row, err := svc.CreateBackup(ctx, writeSource(t, "n.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	// 手造历史反斜杠行（规范正斜杠路径的反斜杠镜像）
	bsRow := entity.NewBackup()
	bsRow.FileName = row.FileName
	bsRow.FilePath.Valid = true
	bsRow.FilePath.String = `backup\2026\08\16\legacy.zip`
	bsRow.Workdir = row.Workdir
	if err := db.Create(bsRow).Error; err != nil {
		t.Fatal(err)
	}

	n, err := svc.NormalizeFilePaths(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("应修正 1 行反斜杠行，实际 %d", n)
	}
	fixed, err := svc.GetByFilePath(ctx, "backup/2026/08/16/legacy.zip")
	if err != nil || fixed == nil || fixed.GetID() != bsRow.GetID() {
		t.Fatalf("规范化后应按正斜杠路径命中，fixed=%v err=%v", fixed, err)
	}
	// 幂等：全表已规范后零修正
	if n, _ := svc.NormalizeFilePaths(ctx); n != 0 {
		t.Fatalf("二次规范化应零修正，实际 %d", n)
	}
}

// TestBackupSelfOpSuppression 自操作抑制登记：DeleteBackup/RestoreFile 在宽限窗口内
// 对操作的 backup 文件相对路径保持抑制（Release 宽限 3s 覆盖 fsnotify 异步延迟，
// 调用返回后立即断言即为窗口内）；DeleteBackup 对不存在的行幂等返回 nil
func TestBackupSelfOpSuppression(t *testing.T) {
	svc, _ := newBackupTestEnv(t)
	ctx := context.Background()

	// RestoreFile：源端（backup/ 域）登记
	row, err := svc.CreateBackup(ctx, writeSource(t, "restore-me.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	backupRel := row.FilePath.String
	target := filepath.Join(t.TempDir(), "restored.mp4")
	if err := svc.RestoreFile(ctx, svc.GetBackupPath(row), target); err != nil {
		t.Fatal(err)
	}
	if !storeRegistry.IsSuppressed(backupRel) {
		t.Fatalf("RestoreFile 后源端 %s 应处于抑制窗口内", backupRel)
	}

	// DeleteBackup：文件端登记 + 缺失行幂等
	row2, err := svc.CreateBackup(ctx, writeSource(t, "delete-me.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteBackup(ctx, row2.GetID()); err != nil {
		t.Fatal(err)
	}
	if !storeRegistry.IsSuppressed(row2.FilePath.String) {
		t.Fatalf("DeleteBackup 后文件 %s 应处于抑制窗口内", row2.FilePath.String)
	}
	if err := svc.DeleteBackup(ctx, 99999); err != nil {
		t.Fatalf("删除不存在的清单行应幂等返回 nil，实际 %v", err)
	}
}
