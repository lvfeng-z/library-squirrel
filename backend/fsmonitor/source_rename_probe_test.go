package fsmonitor

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

// TestFsnotifyRenameEventShape 观察探针：同目录改名与跨目录移动时 fsnotify 实际投递的事件形态。
// 用于锚定「旧路径以何种 Op 到达」——Rename 映射与平台相关，此测试固化当前依赖版本（v1.10.1）的实测行为
func TestFsnotifyRenameEventShape(t *testing.T) {
	dir := t.TempDir()
	sub1 := filepath.Join(dir, "d1")
	sub2 := filepath.Join(dir, "d2")
	for _, d := range []string{sub1, sub2} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	src := filepath.Join(sub1, "a.txt")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		t.Skipf("fsnotify 不可用: %v", err)
	}
	defer w.Close()
	for _, d := range []string{sub1, sub2} {
		if err := w.Add(d); err != nil {
			t.Fatal(err)
		}
	}

	collect := func(action func()) []string {
		var got []string
		deadline := time.Now().Add(3 * time.Second)
		action()
		for time.Now().Before(deadline) {
			select {
			case ev := <-w.Events:
				rel, err := filepath.Rel(dir, ev.Name)
				if err != nil {
					continue
				}
				got = append(got, ev.Op.String()+" "+filepath.ToSlash(rel))
			case err := <-w.Errors:
				t.Fatalf("fsnotify 错误: %v", err)
			case <-time.After(300 * time.Millisecond):
				if len(got) > 0 {
					return got
				}
			}
		}
		return got
	}

	sameDir := collect(func() {
		if err := os.Rename(src, filepath.Join(sub1, "b.txt")); err != nil {
			t.Fatal(err)
		}
	})
	sort.Strings(sameDir)
	t.Logf("同目录改名事件: %v", sameDir)

	crossDir := collect(func() {
		if err := os.Rename(filepath.Join(sub1, "b.txt"), filepath.Join(sub2, "b.txt")); err != nil {
			t.Fatal(err)
		}
	})
	sort.Strings(crossDir)
	t.Logf("跨目录移动事件: %v", crossDir)

	hasOp := func(events []string, op, path string) bool {
		for _, e := range events {
			if strings.Contains(e, op+" ") && strings.HasSuffix(e, path) {
				return true
			}
		}
		return false
	}
	// 行为锚定（fsnotify v1.10.1 Windows 实测）：同目录改名旧名腿=Rename Op（无 Remove）、
	// 新名腿=Create；跨目录移动旧名腿=Remove、新名腿=Create。
	// source.go 首版不转发 Rename Op——同目录改名的旧路径消失在事件源层即被丢弃
	if !hasOp(sameDir, "RENAME", "d1/a.txt") || !hasOp(sameDir, "CREATE", "d1/b.txt") || hasOp(sameDir, "REMOVE", "d1/a.txt") {
		t.Fatalf("同目录改名事件形态与锚定不符: %v", sameDir)
	}
	if !hasOp(crossDir, "REMOVE", "d1/b.txt") || !hasOp(crossDir, "CREATE", "d2/b.txt") {
		t.Fatalf("跨目录移动事件形态与锚定不符: %v", crossDir)
	}
}
