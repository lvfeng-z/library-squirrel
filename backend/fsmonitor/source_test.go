package fsmonitor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestFsnotifySource_CreateRemove 验证 fsnotifySource 递归监控子目录，
// 文件创建/删除被转为 ChangeCreate/ChangeRemove，路径为相对 workDir 的正斜杠形式。
func TestFsnotifySource_CreateRemove(t *testing.T) {
	workDir := t.TempDir()
	// 在 Start 前创建子目录，验证 addWatchRecursive 递归覆盖
	subDir := filepath.Join(workDir, "sub")
	if err := os.Mkdir(subDir, 0o755); err != nil {
		t.Fatal(err)
	}

	src, err := NewFsnotifySource(workDir)
	if err != nil {
		t.Fatalf("NewFsnotifySource 失败: %v", err)
	}
	defer src.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events, errs, err := src.Start(ctx)
	if err != nil {
		t.Fatalf("Start 失败: %v", err)
	}

	// 在子目录创建文件(验证递归监控到子目录)
	tmpFile := filepath.Join(subDir, "test.txt")
	if err := os.WriteFile(tmpFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	ev := waitEvent(t, events, 3*time.Second)
	if ev.Kind != ChangeCreate {
		t.Fatalf("期望 ChangeCreate，得到 %v (path=%s)", ev.Kind, ev.Path)
	}
	if !strings.HasSuffix(ev.Path, "sub/test.txt") {
		t.Fatalf("期望 path 以 sub/test.txt 结尾(相对 workDir 正斜杠)，得到 %q", ev.Path)
	}

	// 删除文件
	if err := os.Remove(tmpFile); err != nil {
		t.Fatal(err)
	}
	ev = waitEvent(t, events, 3*time.Second)
	if ev.Kind != ChangeRemove {
		t.Fatalf("期望 ChangeRemove，得到 %v (path=%s)", ev.Kind, ev.Path)
	}

	// errs 不应有错误
	select {
	case err := <-errs:
		t.Fatalf("未期望的错误: %v", err)
	default:
	}
}

// TestFsnotifySource_DynamicSubDir 验证运行期新建子目录被动态补 watch，
// 其后在该子目录创建文件能被监控到。
func TestFsnotifySource_DynamicSubDir(t *testing.T) {
	workDir := t.TempDir()
	src, err := NewFsnotifySource(workDir)
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

	// 运行期新建子目录(触发 Create 事件 + 动态补 watch)
	newDir := filepath.Join(workDir, "dynamic")
	if err := os.Mkdir(newDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ev := waitEvent(t, events, 3*time.Second)
	if ev.Kind != ChangeCreate || !ev.IsDir {
		t.Fatalf("期望目录 ChangeCreate，得到 %+v", ev)
	}

	// 在新子目录创建文件，应被监控(动态补 watch 生效)
	tmpFile := filepath.Join(newDir, "inner.txt")
	if err := os.WriteFile(tmpFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	ev = waitEvent(t, events, 3*time.Second)
	if ev.Kind != ChangeCreate || !strings.HasSuffix(ev.Path, "dynamic/inner.txt") {
		t.Fatalf("期望动态子目录内文件的 ChangeCreate，得到 %+v", ev)
	}
}

// TestFsnotifySource_ConcurrentWritesNoDeadlock 回归锚定：监控树内持续有并发写入时，
// 构造+启动全流程不得死锁。死锁形态：fsnotify 的 AddWith 挂表握手与事件推送共用同一内部协程，
// 消费循环未在线时事件写入方阻塞该协程，递归挂表随之永等。
// 构造一个已存在且被持续写入的热点目录 + 一棵拖长挂表时间的深目录树，
// 全流程经超时看门狗判定成败（死锁表现为 NewFsnotifySource/Start 永不返回）。
func TestFsnotifySource_ConcurrentWritesNoDeadlock(t *testing.T) {
	workDir := t.TempDir()
	hotDir := filepath.Join(workDir, "hot")
	if err := os.Mkdir(hotDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 500; i++ {
		if err := os.MkdirAll(filepath.Join(workDir, fmt.Sprintf("d%03d", i), "leaf"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	stopWrite := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stopWrite:
				return
			default:
			}
			_ = os.WriteFile(filepath.Join(hotDir, fmt.Sprintf("w%05d", i)), []byte("x"), 0o644)
		}
	}()
	defer func() {
		close(stopWrite)
		wg.Wait()
	}()

	type startResult struct {
		src *fsnotifySource
		err error
	}
	done := make(chan startResult, 1)
	go func() {
		src, err := NewFsnotifySource(workDir)
		if err != nil {
			done <- startResult{nil, err}
			return
		}
		_, _, err = src.Start(context.Background())
		done <- startResult{src, err}
	}()

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("构造/启动失败: %v", r.err)
		}
		if r.src != nil {
			r.src.Stop()
		}
	case <-time.After(30 * time.Second):
		t.Fatal("监控树并发写入期间构造+启动超时：挂表与事件推送死锁（消费循环未先于递归挂表启动）")
	}
}

func waitEvent(t *testing.T, events <-chan FileChange, timeout time.Duration) FileChange {
	t.Helper()
	select {
	case ev, ok := <-events:
		if !ok {
			t.Fatal("events channel 已关闭")
		}
		return ev
	case <-time.After(timeout):
		t.Fatal("等待事件超时")
		return FileChange{}
	}
}
