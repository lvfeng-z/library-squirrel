package staging

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"go.uber.org/zap/zapcore"
)

// TestRetireLegacyRoots 旧版暂存根整体回收：task-staging/ 与 share-receive/（含其内的任务
// 子目录与暂存文件）一并移除并记 Info；workDir 其余条目不动。
func TestRetireLegacyRoots(t *testing.T) {
	workDir := t.TempDir()
	oldUnified := mkdirAllT(t, workDir, "task-staging", "42")
	writeTextT(t, filepath.Join(oldUnified, "image_000.jpg.part"), "x")
	oldReceive := mkdirAllT(t, workDir, "share-receive", "999")
	writeTextT(t, filepath.Join(oldReceive, "manifest.json"), "{}")
	bystander := mkdirAllT(t, workDir, "store", "resource")

	logs := withLogObserver(t, zapcore.InfoLevel)
	if err := RetireLegacyRoots(context.Background(), workDir); err != nil {
		t.Fatalf("旧根回收失败: %v", err)
	}
	for _, gone := range []string{oldUnified, oldReceive} {
		if pathExists(t, gone) {
			t.Fatalf("旧版暂存根未被整体回收: %s", gone)
		}
	}
	if !pathExists(t, bystander) {
		t.Fatalf("workDir 其余目录不应受波及: %s", bystander)
	}
	if n := countInfoContaining(logs, "回收旧版暂存根"); n < 2 {
		t.Fatalf("两处旧根回收均应记 Info，实际 %d 条", n)
	}
}

// TestRetireLegacyRootsIdempotent 重复启动幂等：旧根已回收后再次执行为静默 no-op，不再记
// 回收日志、无副作用。
func TestRetireLegacyRootsIdempotent(t *testing.T) {
	workDir := t.TempDir()
	mkdirAllT(t, workDir, "task-staging", "42")
	mkdirAllT(t, workDir, "share-receive", "999")

	if err := RetireLegacyRoots(context.Background(), workDir); err != nil {
		t.Fatalf("首次回收失败: %v", err)
	}
	logs := withLogObserver(t, zapcore.InfoLevel)
	if err := RetireLegacyRoots(context.Background(), workDir); err != nil {
		t.Fatalf("重复执行应无错误: %v", err)
	}
	if n := countInfoContaining(logs, "回收旧版暂存根"); n != 0 {
		t.Fatalf("根已不存在时不应再记回收日志，实际 %d 条", n)
	}
}

// TestRetireLegacyRootsSingleFailureRetried 单目录回收失败不中断其余回收；失败目录留待下次
// 启动重试（恢复真实回收后再次执行即清偿）。
func TestRetireLegacyRootsSingleFailureRetried(t *testing.T) {
	workDir := t.TempDir()
	taskStaging := mkdirAllT(t, workDir, "task-staging", "42")
	shareReceive := mkdirAllT(t, workDir, "share-receive", "999")

	realRemove := removeAll
	removeAll = func(path string) error {
		if filepath.Base(path) == "task-staging" {
			return errors.New("目录被占用")
		}
		return realRemove(path)
	}
	logs := withLogObserver(t, zapcore.WarnLevel)
	if err := RetireLegacyRoots(context.Background(), workDir); err != nil {
		t.Fatalf("单目录失败不应外抛错误: %v", err)
	}
	if !pathExists(t, taskStaging) {
		t.Fatalf("注入失败的旧根不应被回收")
	}
	if pathExists(t, shareReceive) {
		t.Fatalf("未注入失败的旧根应照常回收（单败中断了其余）")
	}
	if n := countWarnContaining(logs, "回收旧版暂存根失败"); n == 0 {
		t.Fatalf("回收失败未记 Warn")
	}
	removeAll = realRemove

	// 下次启动重试：失败目录清偿
	if err := RetireLegacyRoots(context.Background(), workDir); err != nil {
		t.Fatalf("重试回收失败: %v", err)
	}
	if pathExists(t, taskStaging) {
		t.Fatalf("重试后旧根应被回收")
	}
}

// TestRetireLegacyRootsGuards 空 workDir 为无操作；根缺失的全新库为静默 no-op。
func TestRetireLegacyRootsGuards(t *testing.T) {
	if err := RetireLegacyRoots(context.Background(), ""); err != nil {
		t.Fatalf("空 workDir 应无操作: %v", err)
	}
	if err := RetireLegacyRoots(context.Background(), t.TempDir()); err != nil {
		t.Fatalf("无旧根应无操作: %v", err)
	}
}
