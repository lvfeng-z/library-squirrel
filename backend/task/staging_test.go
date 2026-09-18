package task

// 暂存基建测试：目录派生（两属主根）、作用域确保（原子创建/既有复用）、按任务 ID 清理
// （两根各回收）、归属判活谓词、fsmonitor 零感知锚定（暂存子树不在监控白名单与 backup 域内）。

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/staging"
	"github.com/library-squirrel/backend/storeRegistry"
)

// TestStagingPathDerivation 两属主根的目录派生单点：staging/download/{taskID}/ 与
// staging/share-receive/{taskID}/；收件共享清单相对路径随父任务作用域派生（正斜杠）
func TestStagingPathDerivation(t *testing.T) {
	workDir := filepath.Join("w", "workdir")
	if got := DownloadStagingPath(workDir, 42); got != filepath.Join(workDir, "staging", "download", "42") {
		t.Fatalf("DownloadStagingPath = %q", got)
	}
	if got := ReceiveStagingPath(workDir, 7); got != filepath.Join(workDir, "staging", "share-receive", "7") {
		t.Fatalf("ReceiveStagingPath = %q", got)
	}
	if got := ReceiveManifestRelPath(999); got != "staging/share-receive/999/manifest.json" {
		t.Fatalf("ReceiveManifestRelPath = %q", got)
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

// TestStagingOutsideMonitorScope fsmonitor 零感知锚定：暂存子树路径（根/作用域/文件三级）不在
// store/ 白名单与 backup/ 域——fsnotify 事件过滤与 USN 路径过滤共用这两个谓词，命中即丢弃
func TestStagingOutsideMonitorScope(t *testing.T) {
	rels := []string{
		staging.RootName,
		"staging/download/123",
		"staging/download/123/" + StagingFileName("videoTrack", 0, "mp4"),
		"staging/share-receive/999/manifest.json",
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

// TestEnsureTaskScope 作用域确保：首建落自证描述（scope.json 承载作用域键与内容形态）、
// 既有目录（恢复场景）复用不报错、空 workDir 显式拒绝
func TestEnsureTaskScope(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()

	dlDir, err := EnsureDownloadScope(ctx, workDir, 42)
	if err != nil {
		t.Fatalf("创建下载作用域失败: %v", err)
	}
	if dlDir != DownloadStagingPath(workDir, 42) {
		t.Fatalf("返回目录与派生单点不一致: %q", dlDir)
	}
	assertScopeDesc(t, dlDir, "42", downloadScopeContentShape)

	recvDir, err := EnsureReceiveScope(ctx, workDir, 999)
	if err != nil {
		t.Fatalf("创建收件作用域失败: %v", err)
	}
	assertScopeDesc(t, recvDir, "999", receiveScopeContentShape)

	// 既有目录（暂停/崩溃后恢复）复用：返回同路径、描述原样
	again, err := EnsureDownloadScope(ctx, workDir, 42)
	if err != nil {
		t.Fatalf("既有作用域复用失败: %v", err)
	}
	if again != dlDir {
		t.Fatalf("复用应返回同目录: %q != %q", again, dlDir)
	}
	assertScopeDesc(t, dlDir, "42", downloadScopeContentShape)

	if _, err := EnsureDownloadScope(ctx, "", 42); !errors.Is(err, staging.ErrWorkDirEmpty) {
		t.Fatalf("空 workDir 应返回 ErrWorkDirEmpty, got %v", err)
	}
}

// assertScopeDesc 断言作用域目录内自证描述可解析且键与内容形态正确
func assertScopeDesc(t *testing.T, scopeDir, wantKey, wantShape string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(scopeDir, "scope.json"))
	if err != nil {
		t.Fatalf("读作用域描述失败: %v", err)
	}
	var desc staging.ScopeDescription
	if err := json.Unmarshal(data, &desc); err != nil {
		t.Fatalf("作用域描述不可解析: %v", err)
	}
	if desc.ScopeKey != wantKey || desc.ContentShape != wantShape {
		t.Fatalf("作用域描述键/形态不符: key=%q shape=%q", desc.ScopeKey, desc.ContentShape)
	}
}

// TestCleanupStagingByTaskIds 按任务 ID 清理两属主根：被删 ID 的下载与收件作用域均回收、
// 未涉及 ID 不受影响、空 workDir 与空集合静默返回
func TestCleanupStagingByTaskIds(t *testing.T) {
	workDir := t.TempDir()
	for _, id := range []int64{1, 2, 3} {
		for _, dir := range []string{DownloadStagingPath(workDir, id), ReceiveStagingPath(workDir, id)} {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatalf("建暂存目录失败: %v", err)
			}
		}
	}
	if err := os.WriteFile(filepath.Join(ReceiveStagingPath(workDir, 1), "manifest.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("写清单失败: %v", err)
	}

	if err := CleanupStagingByTaskIds(workDir, []int64{1, 2}); err != nil {
		t.Fatalf("清理失败: %v", err)
	}
	for _, id := range []int64{1, 2} {
		for _, dir := range []string{DownloadStagingPath(workDir, id), ReceiveStagingPath(workDir, id)} {
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Fatalf("被删任务 %d 的暂存目录应回收: %s err=%v", id, dir, err)
			}
		}
	}
	for _, dir := range []string{DownloadStagingPath(workDir, 3), ReceiveStagingPath(workDir, 3)} {
		if _, err := os.Stat(dir); err != nil {
			t.Fatalf("未涉及任务的暂存目录不应受影响: %s: %v", dir, err)
		}
	}

	if err := CleanupStagingByTaskIds(workDir, nil); err != nil {
		t.Fatalf("空集合应静默返回: %v", err)
	}
	if err := CleanupStagingByTaskIds("", []int64{1}); err != nil {
		t.Fatalf("空 workDir 应静默返回: %v", err)
	}
}

// TestNewStagingOwnerAlive 归属判活谓词：装载的 ID 集合判活、集合外判死、非数字键（任务属主根
// 下目录名恒为任务 ID）判死、装载失败返回错误
func TestNewStagingOwnerAlive(t *testing.T) {
	alive, err := NewStagingOwnerAlive(context.Background(), fakeIdLister{ids: []int64{1, 2}})
	if err != nil {
		t.Fatalf("构造谓词失败: %v", err)
	}
	if !alive("1") || !alive("2") {
		t.Fatalf("装载集合内的键应判活")
	}
	if alive("3") {
		t.Fatalf("集合外的键应判死")
	}
	if alive("not-a-task-id") {
		t.Fatalf("非数字键应判死")
	}

	if _, err := NewStagingOwnerAlive(context.Background(), fakeIdLister{err: errors.New("查询失败")}); err == nil {
		t.Fatalf("装载失败应返回错误")
	}
}

// fakeIdLister 任务行 ID 装载桩
type fakeIdLister struct {
	ids []int64
	err error
}

func (f fakeIdLister) ListAllIds(ctx context.Context) ([]int64, error) {
	return f.ids, f.err
}

// TestListAllIds 仓储全量 ID 装载（外键强制测试库）：插三行收回三元
func TestListAllIds(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	repo := NewRepository(db, nil, nil)
	for i := 0; i < 3; i++ {
		tk := entity.NewTask()
		if err := db.Create(tk).Error; err != nil {
			t.Fatalf("插任务失败: %v", err)
		}
	}
	ids, err := repo.ListAllIds(context.Background())
	if err != nil {
		t.Fatalf("装载失败: %v", err)
	}
	if len(ids) != 3 {
		t.Fatalf("应装载 3 个 ID, got %d", len(ids))
	}
}
