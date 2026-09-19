package fsmonitor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestFsnotifySource_ExcludedSubtreeNoWatchHandle 回归锚定：排除子树（暂存总根）整体免挂
// watch。机制背景：监控树内「新建目录随即 rename 定名」（staging 作用域原子创建的
// MkdirTemp→写描述→rename 序列）与对新目录的动态补 watch 构成句柄竞态，Windows 上 rename
// 随机失败为 sharing violation（实测同构序列 300 次迭代 7 次失败、错误文本与生产事故一致；
// 子树排除后 0 次）。三断言：①排除子树内作用域创建序列循环全数成功；②排除子树内零 watch
// （覆盖初始 walk 与动态补挂两个挂来源）；③排除子树内变更零事件、监控能力对照正常。
func TestFsnotifySource_ExcludedSubtreeNoWatchHandle(t *testing.T) {
	workDir := t.TempDir()
	stagingRoot := filepath.Join(workDir, "staging")
	ownerRoot := filepath.Join(stagingRoot, "download")
	if err := os.MkdirAll(ownerRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	src, err := NewFsnotifySource(workDir, stagingRoot)
	if err != nil {
		t.Fatalf("NewFsnotifySource 失败: %v", err)
	}
	defer src.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events, _, err := src.Start(ctx)
	if err != nil {
		t.Fatalf("Start 失败: %v", err)
	}

	// ① 作用域创建序列循环（staging.materializeScope 同构）：rename 不得因 watch 句柄竞态失败
	for i := 0; i < 300; i++ {
		tempDir, err := os.MkdirTemp(ownerRoot, ".tmp-*")
		if err != nil {
			t.Fatalf("iter %d MkdirTemp 失败: %v", i, err)
		}
		if err := os.WriteFile(filepath.Join(tempDir, "scope.json"), []byte(`{"scopeKey":"x"}`), 0o644); err != nil {
			t.Fatalf("iter %d WriteFile 失败: %v", i, err)
		}
		final := filepath.Join(ownerRoot, fmt.Sprintf("s%d", i))
		if err := os.Rename(tempDir, final); err != nil {
			t.Fatalf("iter %d rename 失败（排除子树内不得存在 watch 句柄竞态）: %v", i, err)
		}
		if err := os.RemoveAll(final); err != nil {
			t.Fatalf("iter %d 清理失败: %v", i, err)
		}
	}

	// ② 排除子树内零 watch（初始 walk 与动态补挂两来源一并覆盖）
	src.mu.Lock()
	watches := make(map[string]bool, len(src.watches))
	for path, watched := range src.watches {
		watches[path] = watched
	}
	src.mu.Unlock()
	if !watches[filepath.Clean(workDir)] {
		t.Fatal("workDir 根目录应有 watch（排除只应作用于暂存子树）")
	}
	for path := range watches {
		if strings.HasPrefix(filepath.Clean(path), stagingRoot) {
			t.Fatalf("排除子树内存在 watch: %s", path)
		}
	}

	// ③a 排除子树内高频变更后，事件通道应为空（无 watch 即无事件源）
	select {
	case ev := <-events:
		t.Fatalf("排除子树内变更不应产生事件，收到: %+v", ev)
	default:
	}

	// ③b 对照：workDir 下普通目录仍被动态补挂并产出事件（监控能力未受损）
	ctrlDir := filepath.Join(workDir, "control")
	if err := os.Mkdir(ctrlDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ev := waitEvent(t, events, 3*time.Second)
	if ev.Kind != ChangeCreate || !ev.IsDir || !strings.HasSuffix(ev.Path, "control") {
		t.Fatalf("期望对照目录的 ChangeCreate 事件，得到 %+v", ev)
	}
	if err := os.WriteFile(filepath.Join(ctrlDir, "probe.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	ev = waitEvent(t, events, 3*time.Second)
	if ev.Kind != ChangeCreate || !strings.HasSuffix(ev.Path, "control/probe.txt") {
		t.Fatalf("期望对照目录内文件的 ChangeCreate 事件，得到 %+v", ev)
	}
}
