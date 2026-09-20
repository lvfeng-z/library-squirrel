package storeRegistry

import (
	"errors"
	"slices"
	"testing"
)

// canonicalDirs 生产装配面注册的三条存储目录（与 app.go 装配注册同值），
// 供白名单谓词类用例以生产口径运行。
var canonicalDirs = []StoreDir{
	{Path: "store/work", Owner: "resource"},
	{Path: "store/avatar/local", Owner: "author"},
	{Path: "store/avatar/site", Owner: "author"},
}

// resetRegistry 清空注册表。注册只发生在装配期（单一窗口），生产无重置入口；
// 包内测试用本函数隔离各用例的注册状态。
func resetRegistry() {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry = nil
}

// registerCanonical 注册生产三条目，测试结束时清空还原。
func registerCanonical(t *testing.T) {
	t.Helper()
	resetRegistry()
	for _, d := range canonicalDirs {
		if err := Register(d); err != nil {
			t.Fatalf("注册生产存储目录 %q 失败: %v", d.Path, err)
		}
	}
	t.Cleanup(resetRegistry)
}

// TestRegisterUniqueness 注册唯一性：同路径重复注册、父子前缀（子树重叠）均拒绝；
// 段边界外（store/workX）不与 store/work 冲突，可注册。
func TestRegisterUniqueness(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)
	if err := Register(StoreDir{Path: "store/work", Owner: "resource"}); err != nil {
		t.Fatalf("首次注册失败: %v", err)
	}
	if err := Register(StoreDir{Path: "store/work", Owner: "download"}); !errors.Is(err, ErrDuplicateDir) {
		t.Fatalf("同路径重复注册期望 ErrDuplicateDir，实际: %v", err)
	}
	// 已注册目录的子目录：子树重叠（对账扫描重复遍历、域判定口径分叉）
	if err := Register(StoreDir{Path: "store/work/sub", Owner: "other"}); !errors.Is(err, ErrPrefixConflict) {
		t.Fatalf("子前缀注册期望 ErrPrefixConflict，实际: %v", err)
	}
	// 已注册目录的父目录：同样子树重叠
	if err := Register(StoreDir{Path: "store", Owner: "other"}); !errors.Is(err, ErrPrefixConflict) {
		t.Fatalf("父前缀注册期望 ErrPrefixConflict，实际: %v", err)
	}
	if err := Register(StoreDir{Path: "store/workX", Owner: "other"}); err != nil {
		t.Fatalf("段边界外路径注册失败: %v", err)
	}
	if got := len(RegisteredDirs()); got != 2 {
		t.Fatalf("注册成功 + 段边界外一笔后条目数 = %d, want 2", got)
	}
}

// TestRegisterInvalidPath 路径与条目非法：空串、反斜杠、绝对路径、非规范形、越出 workDir、
// Owner 为空均拒绝；全部拒绝后注册表保持为空。
func TestRegisterInvalidPath(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)
	bad := []struct {
		dir  StoreDir
		want error
	}{
		{StoreDir{Path: "", Owner: "m"}, ErrInvalidDirPath},
		{StoreDir{Path: `store\work`, Owner: "m"}, ErrInvalidDirPath},  // 反斜杠（relPath 域规范为正斜杠）
		{StoreDir{Path: "/store/work", Owner: "m"}, ErrInvalidDirPath}, // 绝对路径
		{StoreDir{Path: "store/work/", Owner: "m"}, ErrInvalidDirPath}, // 尾斜杠非规范形
		{StoreDir{Path: "store//work", Owner: "m"}, ErrInvalidDirPath}, // 冗余分隔符
		{StoreDir{Path: "./store/work", Owner: "m"}, ErrInvalidDirPath},
		{StoreDir{Path: ".", Owner: "m"}, ErrInvalidDirPath},    // workDir 根自身
		{StoreDir{Path: "..", Owner: "m"}, ErrInvalidDirPath},   // 越出 workDir
		{StoreDir{Path: "../x", Owner: "m"}, ErrInvalidDirPath}, // 越出 workDir
		{StoreDir{Path: "store/work", Owner: ""}, ErrEmptyOwner},
	}
	for _, c := range bad {
		err := Register(c.dir)
		if !errors.Is(err, c.want) {
			t.Errorf("Register(%q, owner=%q) 期望 %v，实际: %v", c.dir.Path, c.dir.Owner, c.want, err)
		}
	}
	if got := RegisteredDirs(); len(got) != 0 {
		t.Fatalf("全部拒绝后注册表应仍为空，实际: %v", got)
	}
}

// TestRegisteredDirsSnapshot 快照稳定性：返回切片为独立拷贝，调用方修改不回流注册表；
// RegisteredPaths 与 RegisteredDirs 取值一致。
func TestRegisteredDirsSnapshot(t *testing.T) {
	registerCanonical(t)
	s1 := RegisteredDirs()
	s1[0].Owner = "篡改"
	s1 = append(s1[:0], StoreDir{Path: "store/evil", Owner: "x"}) // 复用返回切片底层数组写脏
	s2 := RegisteredDirs()
	if len(s2) != len(canonicalDirs) {
		t.Fatalf("取快照后条目数 = %d, want %d", len(s2), len(canonicalDirs))
	}
	for i, d := range s2 {
		if d != canonicalDirs[i] {
			t.Fatalf("快照条目[%d] = %+v, want %+v", i, d, canonicalDirs[i])
		}
	}
	paths := RegisteredPaths()
	want := []string{"store/work", "store/avatar/local", "store/avatar/site"}
	if len(paths) != len(want) {
		t.Fatalf("RegisteredPaths 长度 = %d, want %d", len(paths), len(want))
	}
	for i, p := range paths {
		if p != want[i] {
			t.Fatalf("RegisteredPaths[%d] = %q, want %q", i, p, want[i])
		}
	}
}

// TestInScanDirs 验证白名单谓词：命中已注册子树为 true，外部目录为 false。
func TestInScanDirs(t *testing.T) {
	registerCanonical(t)
	cases := []struct {
		rel  string
		want bool
	}{
		{"store/work/作者/x.jpg", true},
		{"store/work", true},             // 子树根自身
		{"store/thumbnail/t.jpg", false}, // 已退役目录（缩略图统一进 store/work，不再独立子目录）
		{"store/avatar/local/a.png", true},
		{"store/avatar/site/b.png", true},
		{"store/avatar", false}, // 仅 store/avatar 不在白名单（只有 local/site）
		{"backup/2026/x.mp4", false},
		{".git/config", false},
		{"log/server.log", false},
		{"", false},
		{".", false},
		{"store/workX/y.jpg", false}, // 前缀串匹配须按分隔符，非 store/workX
	}
	for _, c := range cases {
		if got := InScanDirs(c.rel); got != c.want {
			t.Fatalf("InScanDirs(%q) = %v want %v", c.rel, got, c.want)
		}
	}
}

// TestInBackupDir 验证备份根谓词：backup 子树（含根）为 true，store 子树与外部为 false。
func TestInBackupDir(t *testing.T) {
	cases := []struct {
		rel  string
		want bool
	}{
		{"backup", true}, // 子树根自身
		{"backup/2026/08/23/x.mp4", true},
		{"backupX/y.mp4", false}, // 前缀串匹配须按分隔符
		{"store/work/x.jpg", false},
		{"", false},
		{".", false},
	}
	for _, c := range cases {
		if got := InBackupDir(c.rel); got != c.want {
			t.Fatalf("InBackupDir(%q) = %v want %v", c.rel, got, c.want)
		}
	}
}

// TestValidatePath 验证落盘前路径校验：白名单内放行、白名单外拒绝、反斜杠归一。
func TestValidatePath(t *testing.T) {
	registerCanonical(t)
	ok := []string{
		"store/work/作者/video.mp4",
		"store/work",
		"store/avatar/local/1.png",
		"store/avatar/site/2.png",
	}
	for _, p := range ok {
		if err := ValidatePath(p); err != nil {
			t.Errorf("ValidatePath(%q) 期望通过，实际错误: %v", p, err)
		}
	}
	bad := []string{
		"backup/2026/x.mp4",
		"store/thumbnail/x.jpg", // 已退役目录，不再放行
		"store/avatar",          // 仅 store/avatar，未注册子目录
		"store/workX/y.jpg",     // 前缀串匹配按分隔符，不误命中
		".git/config",
	}
	for _, p := range bad {
		if err := ValidatePath(p); err == nil {
			t.Errorf("ValidatePath(%q) 期望拒绝，实际通过", p)
		}
	}
	// Windows 反斜杠路径也须放行（ToSlash 归一）
	if err := ValidatePath(`store\work\作者\v.mp4`); err != nil {
		t.Errorf("ValidatePath(反斜杠) 期望通过，实际错误: %v", err)
	}
}

// TestStartupSnapshotTakesPostRegistrationState 注册时序守卫：fsmonitor 启动期消费的注册快照
// 取自注册完成后的注册表状态——离线对账扫描根（fsmonitor/scanner.go collectDiskFiles，消费
// RegisteredDirs）与 USN 缓存种子（fsmonitor/frn_cache_windows.go Build，消费 RegisteredPaths）
// 在监控启动时取快照遍历一次。模拟启动序列：init 期（零注册）先取一次读点，装配注册后再取
// 启动期快照——须含全部注册目录、不含未注册目录。RegisteredPaths/RegisteredDirs 若回退为包级
// init 求值或首次调用即缓存（init 期恒空、装配注册对既取快照不可见），本用例失败。
func TestStartupSnapshotTakesPostRegistrationState(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)
	// init 期读点：装配注册尚未发生，注册表为空
	if got := RegisteredPaths(); len(got) != 0 {
		t.Fatalf("零注册状态 RegisteredPaths = %v, want 空", got)
	}
	// 装配注册：生产三条目 + 一个新增域目录（模拟后续业务域装配期登记）
	toRegister := append(append([]StoreDir{}, canonicalDirs...),
		StoreDir{Path: "store/derived", Owner: "derived"})
	for _, d := range toRegister {
		if err := Register(d); err != nil {
			t.Fatalf("注册 %q 失败: %v", d.Path, err)
		}
	}
	// 启动期快照取值点（监控启动时一次）：须含全部注册目录
	startupPaths := RegisteredPaths()
	startupDirs := RegisteredDirs()
	if len(startupPaths) != len(toRegister) || len(startupDirs) != len(toRegister) {
		t.Fatalf("启动期快照条目数 paths=%d dirs=%d, want %d",
			len(startupPaths), len(startupDirs), len(toRegister))
	}
	for _, d := range toRegister {
		if !slices.Contains(startupPaths, d.Path) {
			t.Fatalf("启动期快照 RegisteredPaths 缺注册目录 %q: %v", d.Path, startupPaths)
		}
		if !slices.ContainsFunc(startupDirs, func(x StoreDir) bool { return x == d }) {
			t.Fatalf("启动期快照 RegisteredDirs 缺注册条目 %+v: %v", d, startupDirs)
		}
	}
	// 未注册目录不得进入快照，白名单谓词随注册内容派生（注册目录子树放行、未注册目录拒绝）
	if slices.Contains(startupPaths, "store/unregistered") {
		t.Fatalf("启动期快照含未注册目录 store/unregistered: %v", startupPaths)
	}
	if !InScanDirs("store/derived/x.bin") {
		t.Fatal("InScanDirs(注册目录子树) 期望 true")
	}
	if err := ValidatePath("store/derived/x.bin"); err != nil {
		t.Fatalf("ValidatePath(注册目录子树) 期望通过，实际错误: %v", err)
	}
	if err := ValidatePath("store/unregistered/x.bin"); err == nil {
		t.Fatal("ValidatePath(未注册目录) 期望拒绝，实际通过")
	}
}
