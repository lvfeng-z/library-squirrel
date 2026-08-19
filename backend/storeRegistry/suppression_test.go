package storeRegistry

import (
	"path/filepath"
	"sync"
	"testing"
)

// clearSuppress 测试间清空抑制集（避免用例间污染）
func clearSuppress() {
	suppressMu.Lock()
	suppressSet = make(map[string]suppressEntry)
	suppressMu.Unlock()
}

// TestSuppressAndHit 登记后精确命中
func TestSuppressAndHit(t *testing.T) {
	clearSuppress()
	Suppress("store/resource/作者/x.jpg")
	if !IsSuppressed("store/resource/作者/x.jpg") {
		t.Fatal("登记后精确路径应命中")
	}
}

// TestReleaseGrace Release 后宽限期内仍命中（expiry 刷新到 now+grace）
func TestReleaseGrace(t *testing.T) {
	clearSuppress()
	Suppress("store/resource/a.mp4")
	Release("store/resource/a.mp4")
	if !IsSuppressed("store/resource/a.mp4") {
		t.Fatal("Release 后宽限期内应仍命中")
	}
}

// TestReleaseUnregistered 未登记项 Release 不续命
func TestReleaseUnregistered(t *testing.T) {
	clearSuppress()
	Release("store/resource/never.jpg") // 未登记，应无副作用
	if IsSuppressed("store/resource/never.jpg") {
		t.Fatal("未登记项 Release 后不应命中")
	}
}

// TestPrefixMatch 登记目录命中下级文件（祖先前缀匹配，含分隔符边界）
func TestPrefixMatch(t *testing.T) {
	clearSuppress()
	Suppress("store/resource/作者")
	cases := []struct {
		path string
		want bool
	}{
		{"store/resource/作者/x.jpg", true},     // 下级文件
		{"store/resource/作者/sub/y.jpg", true}, // 多级下级
		{"store/resource/作者", true},           // 目录自身
		{"store/resource/作者X/z.jpg", false},   // 同前缀串但非祖先（分隔符边界）
		{"store/resource/其他/w.jpg", false},    // 兄弟目录
		{"store/thumbnail/t.jpg", false},      // 无关
	}
	for _, c := range cases {
		if got := IsSuppressed(c.path); got != c.want {
			t.Errorf("IsSuppressed(%q)=%v want %v", c.path, got, c.want)
		}
	}
}

// TestDescendantMatch 登记文件命中其祖先目录查询（文件登记覆盖父目录 Create 事件，如 StoreStream 的 MkdirAll）
func TestDescendantMatch(t *testing.T) {
	clearSuppress()
	Suppress("store/resource/作者/x.jpg")
	cases := []struct {
		path string
		want bool
	}{
		{"store/resource/作者", true},  // 父目录（后代匹配）
		{"store/resource", true},     // 祖父目录（后代匹配）
		{"store", true},              // 根（后代匹配）
		{"store/resource/其他", false}, // 兄弟（无后代关系）
		{"store/thumbnail", false},   // 无关
	}
	for _, c := range cases {
		if got := IsSuppressed(c.path); got != c.want {
			t.Errorf("IsSuppressed(%q)=%v want %v", c.path, got, c.want)
		}
	}
}

// TestNotRegistered 未登记路径不命中
func TestNotRegistered(t *testing.T) {
	clearSuppress()
	if IsSuppressed("store/resource/x.jpg") {
		t.Fatal("未登记路径不应命中")
	}
}

// TestKeyNormalize 平台分隔符登记与正斜杠查询等价（归一）
func TestKeyNormalize(t *testing.T) {
	clearSuppress()
	Suppress(filepath.Join("store", "resource", "a.jpg")) // Windows 下 Join 产反斜杠
	if !IsSuppressed("store/resource/a.jpg") {
		t.Fatal("平台分隔符登记应与正斜杠查询等价")
	}
}

// TestExpiry 过期项不命中（手动构造过期项，免实际等待）
func TestExpiry(t *testing.T) {
	clearSuppress()
	suppressMu.Lock()
	suppressSet["store/resource/old.jpg"] = suppressEntry{expiry: nowMs() - 1000} // 已过期
	suppressMu.Unlock()
	if IsSuppressed("store/resource/old.jpg") {
		t.Fatal("过期项不应命中")
	}
}

// TestSetSuppressEnabled D7 开关：关掉后 IsSuppressed 恒 false
func TestSetSuppressEnabled(t *testing.T) {
	clearSuppress()
	defer SetSuppressEnabled(true) // 恢复默认，避免污染其他用例
	Suppress("store/resource/a.jpg")
	SetSuppressEnabled(false)
	if IsSuppressed("store/resource/a.jpg") {
		t.Fatal("suppressEnabled=false 时不应命中")
	}
}

// TestConcurrent 并发登记/释放/查询无 race（须 go test -race）
func TestConcurrent(t *testing.T) {
	clearSuppress()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(3)
		go func() { defer wg.Done(); Suppress("store/resource/a.jpg") }()
		go func() { defer wg.Done(); Release("store/resource/a.jpg") }()
		go func() { defer wg.Done(); IsSuppressed("store/resource/a.jpg") }()
	}
	wg.Wait()
}
