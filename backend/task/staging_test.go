package task

// 暂存基建测试：目录派生、暂存文件命名、启动清扫（孤儿回收/活任务保留）、
// fsmonitor 零感知锚定（暂存子树不在监控白名单与 backup 域内）。

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/library-squirrel/backend/storeRegistry"
)

// TestStagingPathDerivation 暂存目录派生单点：{workDir}/task-staging/{taskID}/ 结构
func TestStagingPathDerivation(t *testing.T) {
	workDir := filepath.Join("w", "workdir")
	got := StagingPath(workDir, 42)
	want := filepath.Join(workDir, StagingRootName, "42")
	if got != want {
		t.Fatalf("StagingPath = %q, want %q", got, want)
	}
}

// TestStagingFileName 暂存文件名 role_seq 派生键：三位零填充、扩展名补点、空扩展无段
func TestStagingFileName(t *testing.T) {
	cases := []struct {
		role string
		seq  int
		ext  string
		want string
	}{
		{"videoTrack", 0, "mp4", "videoTrack_000.mp4"},
		{"image", 12, ".png", "image_012.png"},
		{"document", 3, "", "document_003"},
		{"audioMain", 123, "flac", "audioMain_123.flac"},
	}
	for _, c := range cases {
		if got := StagingFileName(c.role, c.seq, c.ext); got != c.want {
			t.Fatalf("StagingFileName(%q, %d, %q) = %q, want %q", c.role, c.seq, c.ext, got, c.want)
		}
	}
}

// TestStagingOutsideMonitorScope fsmonitor 零感知锚定：暂存子树路径（目录与文件两级）不在
// store/ 白名单与 backup/ 域——fsnotify 事件过滤与 USN 路径过滤共用这两个谓词，命中即丢弃
func TestStagingOutsideMonitorScope(t *testing.T) {
	rels := []string{
		StagingRootName,
		StagingRootName + "/123",
		StagingRootName + "/123/" + StagingFileName("videoTrack", 0, "mp4"),
	}
	for _, rel := range rels {
		if storeRegistry.InScanDirs(rel) {
			t.Fatalf("暂存路径 %q 不应命中 store/ 白名单（InScanDirs）", rel)
		}
		if storeRegistry.InBackupDir(rel) {
			t.Fatalf("暂存路径 %q 不应命中 backup/ 域（InBackupDir）", rel)
		}
	}
}

// TestCleanupOrphanStaging 启动清扫：任务行已删=孤儿回收（目录含暂存文件一并移除）、
// 任务行在=保留、非目录条目与非数字名目录跳过不动、根缺失与空 workDir 静默返回
func TestCleanupOrphanStaging(t *testing.T) {
	workDir := t.TempDir()
	aliveDir := StagingPath(workDir, 1)  // 任务行在 → 保留
	orphanDir := StagingPath(workDir, 2) // 任务行删 → 回收
	for _, d := range []string{aliveDir, orphanDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("建暂存目录失败: %v", err)
		}
	}
	// 孤儿目录内暂存文件占位（回收判定按目录归属，与内容无关）
	if err := os.WriteFile(filepath.Join(orphanDir, StagingFileName("image", 0, "jpg")), []byte("x"), 0o644); err != nil {
		t.Fatalf("写暂存文件失败: %v", err)
	}
	// 非目录条目与非数字名目录：清扫职责外，跳过不动
	junkFile := filepath.Join(workDir, StagingRootName, "junk.txt")
	if err := os.WriteFile(junkFile, []byte("x"), 0o644); err != nil {
		t.Fatalf("写噪声文件失败: %v", err)
	}
	junkDir := filepath.Join(workDir, StagingRootName, "not-a-task-id")
	if err := os.MkdirAll(junkDir, 0o755); err != nil {
		t.Fatalf("建噪声目录失败: %v", err)
	}

	exists := func(id int64) bool { return id == 1 }
	if err := CleanupOrphanStaging(workDir, exists); err != nil {
		t.Fatalf("清扫失败: %v", err)
	}
	if _, err := os.Stat(orphanDir); !os.IsNotExist(err) {
		t.Fatalf("孤儿暂存目录应被回收, stat err=%v", err)
	}
	if _, err := os.Stat(aliveDir); err != nil {
		t.Fatalf("活任务暂存目录应保留: %v", err)
	}
	if _, err := os.Stat(junkFile); err != nil {
		t.Fatalf("非目录条目应跳过不动: %v", err)
	}
	if _, err := os.Stat(junkDir); err != nil {
		t.Fatalf("非数字名目录应跳过不动: %v", err)
	}

	// 根缺失（全新库无暂存目录）与空 workDir（未配置态）：静默返回不报错
	if err := CleanupOrphanStaging(filepath.Join(workDir, "no-such-root"), exists); err != nil {
		t.Fatalf("根缺失应静默返回: %v", err)
	}
	if err := CleanupOrphanStaging("", exists); err != nil {
		t.Fatalf("空 workDir 应静默返回: %v", err)
	}
}
