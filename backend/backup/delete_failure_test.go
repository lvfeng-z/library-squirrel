package backup

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestDeleteBackupFileFailure 删除备份文件失败（非缺失）时：DeleteBackup 不删记录并返回错误，
// DeleteBackupRecord 仅删记录、磁盘文件保留。
// 用「备份路径替换为非空目录」制造跨平台可移植的 os.Remove 失败（目录非空恒不可删），
// 模拟文件被占用/只读等真实删除失败
func TestDeleteBackupFileFailure(t *testing.T) {
	svc, _ := newBackupTestEnv(t)
	ctx := context.Background()

	row, err := svc.CreateBackup(ctx, writeSource(t, "locked.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	abs := svc.GetBackupPath(row)
	if err := os.Remove(abs); err != nil {
		t.Fatalf("清理测试文件失败: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(abs, "inner"), 0o755); err != nil {
		t.Fatalf("创建非空目录失败: %v", err)
	}

	// DeleteBackup：文件删除失败 → 返回错误、记录保留（不得静默删记录制造孤儿）
	if err := svc.DeleteBackup(ctx, row.GetID()); err == nil {
		t.Fatalf("非空目录删除应失败并返回错误，实际返回 nil")
	}
	kept, err := svc.GetById(ctx, row.GetID())
	if err != nil || kept == nil {
		t.Fatalf("文件删除失败后记录应保留，实际 err=%v kept=%v", err, kept)
	}

	// DeleteBackupRecord：仅删记录、不动磁盘文件（用户对失败显式选择「仅删记录」）
	if err := svc.DeleteBackupRecord(ctx, row.GetID()); err != nil {
		t.Fatalf("仅删记录应成功，实际 %v", err)
	}
	gone, err := svc.GetById(ctx, row.GetID())
	if err == nil && gone != nil {
		t.Fatalf("仅删记录后记录应删除，实际仍存在")
	}
	if _, err := os.Stat(abs); err != nil {
		t.Fatalf("仅删记录不应动磁盘文件（目录应保留），实际 %v", err)
	}
}
