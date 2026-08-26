//go:build windows

package workdirGuard

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/library-squirrel/backend/storeRegistry"
)

// TestWindowsGuardProbeSuccess 探针成功：临时目录可写，探针文件不留残留，路径已登记操作抑制
func TestWindowsGuardProbeSuccess(t *testing.T) {
	g := newWindowsGuard()
	workDir := t.TempDir()
	if err := g.Probe(context.Background(), workDir); err != nil {
		t.Fatalf("临时目录探测应成功: %v", err)
	}
	// 探针文件已删除，不留残留
	if _, err := os.Stat(filepath.Join(workDir, probeRelPath)); !os.IsNotExist(err) {
		t.Fatalf("探针文件应已清理，stat 错误: %v", err)
	}
	// 探针路径已登记操作抑制（Suppress 后 defer Release 进入宽限期，IsSuppressed 应命中）
	if !storeRegistry.IsSuppressed(probeRelPath) {
		t.Fatal("探针路径应已登记操作抑制")
	}
}

// TestWindowsGuardProbeFailure 探针失败：目标目录不存在，返回明确错误；失败分支同样完成抑制登记
func TestWindowsGuardProbeFailure(t *testing.T) {
	g := newWindowsGuard()
	// 父目录存在但子目录不存在：写入必失败，返回明确错误
	workDir := filepath.Join(t.TempDir(), "no_such_dir")
	if err := g.Probe(context.Background(), workDir); err == nil {
		t.Fatal("对不存在的目录探测应失败")
	}
	// 失败分支同样完成抑制登记（Suppress 已调用、defer Release 走宽限）
	if !storeRegistry.IsSuppressed(probeRelPath) {
		t.Fatal("探针路径应已登记操作抑制（失败分支）")
	}
}

// TestWindowsGuardProbeEmptyWorkDir 空 workDir 直接报错，不写探针文件
func TestWindowsGuardProbeEmptyWorkDir(t *testing.T) {
	g := newWindowsGuard()
	if err := g.Probe(context.Background(), ""); err == nil {
		t.Fatal("空 workDir 探测应失败")
	}
}
