//go:build windows

package fsmonitor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/library-squirrel/backend/base/logger"
	"go.uber.org/zap"
)

// urec 构造一条 usnRecord 供 pairAndResolve 测试（reason 决定分类，attrs 决定 IsDir）。
func urec(frn, parentFRN uint64, reason uint32, isDir bool, name string) usnRecord {
	attrs := uint32(0)
	if isDir {
		attrs = fileAttributeDirectory
	}
	return usnRecord{
		FRN:            frn,
		ParentFRN:      parentFRN,
		Reason:         reason,
		FileAttributes: attrs,
		FileName:       name,
		TimeStamp:      133600000000000000, // 合法 Windows FILETIME（~2024），供 ts() 转换
	}
}

// newCache 构造预填目录映射的 frnPathCache（直接填 frnToRel，跳过 Build 的卷 IO）。
func newCache(entries map[uint64]string) *frnPathCache {
	c := newFrnPathCache("X:/wd")
	for frn, rel := range entries {
		c.frnToRel[frn] = rel
	}
	return c
}

// TestPairAndResolve_FileRename 文件 rename OLD/NEW 同 FRN → 单条 ChangeMove（旧→新）。
func TestPairAndResolve_FileRename(t *testing.T) {
	cache := newCache(map[uint64]string{100: "store/resource"})
	out := pairAndResolve([]usnRecord{
		urec(1, 100, usnReasonRenameOld, false, "old.jpg"),
		urec(1, 100, usnReasonRenameNew, false, "new.jpg"),
	}, cache)
	if len(out) != 1 || out[0].Kind != ChangeMove {
		t.Fatalf("期望 1 条 ChangeMove，got %+v", out)
	}
	if out[0].Path != "store/resource/old.jpg" || out[0].ToPath != "store/resource/new.jpg" || out[0].IsDir {
		t.Fatalf("ChangeMove 字段错: %+v", out[0])
	}
}

// TestPairAndResolve_NonAdjacentRename OLD 与 NEW 非相邻（中间夹无关记录），仍按 FRN 配对（R7）。
func TestPairAndResolve_NonAdjacentRename(t *testing.T) {
	cache := newCache(map[uint64]string{100: "store/resource"})
	out := pairAndResolve([]usnRecord{
		urec(1, 100, usnReasonRenameOld, false, "old.jpg"),     // OLD（FRN=1）
		urec(2, 100, usnReasonFileCreate, false, "other.jpg"),  // 无关 create（FRN=2）
		urec(1, 100, usnReasonRenameNew, false, "new.jpg"),     // NEW（FRN=1，与 OLD 非相邻）
	}, cache)
	if len(out) != 2 {
		t.Fatalf("期望 2 条（Move + Create），got %d: %+v", len(out), out)
	}
	var move, create *FileChange
	for i := range out {
		if out[i].Kind == ChangeMove {
			move = &out[i]
		} else if out[i].Kind == ChangeCreate {
			create = &out[i]
		}
	}
	if move == nil || move.Path != "store/resource/old.jpg" || move.ToPath != "store/resource/new.jpg" {
		t.Fatalf("非相邻 rename 应配对为 ChangeMove，got %+v", move)
	}
	if create == nil || create.Path != "store/resource/other.jpg" {
		t.Fatalf("夹杂的无关 create 应独立产出，got %+v", create)
	}
}

// TestPairAndResolve_UnpairedOldFallback 未配上 NEW 的 OLD → 兜底 ChangeRemove。
func TestPairAndResolve_UnpairedOldFallback(t *testing.T) {
	cache := newCache(map[uint64]string{100: "store/resource"})
	out := pairAndResolve([]usnRecord{
		urec(1, 100, usnReasonRenameOld, false, "x.jpg"),
	}, cache)
	if len(out) != 1 || out[0].Kind != ChangeRemove || out[0].Path != "store/resource/x.jpg" {
		t.Fatalf("未配 OLD 应兜底 ChangeRemove，got %+v", out)
	}
}

// TestPairAndResolve_UnpairedNewFallback 无 OLD 配对的 NEW → 兜底 ChangeCreate。
func TestPairAndResolve_UnpairedNewFallback(t *testing.T) {
	cache := newCache(map[uint64]string{100: "store/resource"})
	out := pairAndResolve([]usnRecord{
		urec(1, 100, usnReasonRenameNew, false, "x.jpg"),
	}, cache)
	if len(out) != 1 || out[0].Kind != ChangeCreate || out[0].Path != "store/resource/x.jpg" {
		t.Fatalf("无 OLD 的 NEW 应兜底 ChangeCreate，got %+v", out)
	}
}

// TestPairAndResolve_DirRename 目录 rename → OnDirRename 迁移缓存（含下级）+ 发 ChangeCreate(IsDir)，不发 ChangeMove(IsDir)。
func TestPairAndResolve_DirRename(t *testing.T) {
	cache := newCache(map[uint64]string{
		100: "store/resource",
		200: "store/resource/olddir",
		300: "store/resource/olddir/sub",
	})
	out := pairAndResolve([]usnRecord{
		urec(200, 100, usnReasonRenameOld, true, "olddir"),
		urec(200, 100, usnReasonRenameNew, true, "newdir"),
	}, cache)
	if len(out) != 1 || out[0].Kind != ChangeCreate || !out[0].IsDir || out[0].Path != "store/resource/newdir" {
		t.Fatalf("目录 rename 应发 ChangeCreate(IsDir)（走 processDirCreate），got %+v", out)
	}
	// OnDirRename 应迁移目录及下级
	if cache.frnToRel[200] != "store/resource/newdir" {
		t.Fatalf("OnDirRename 未迁移目录自身: got %q", cache.frnToRel[200])
	}
	if cache.frnToRel[300] != "store/resource/newdir/sub" {
		t.Fatalf("OnDirRename 未迁移下级: got %q", cache.frnToRel[300])
	}
}

// TestPairAndResolve_ExternalParentDropped 父目录不在缓存（workDir 子树外）→ 丢弃。
func TestPairAndResolve_ExternalParentDropped(t *testing.T) {
	cache := newCache(map[uint64]string{100: "store/resource"}) // 仅 100 在缓存
	out := pairAndResolve([]usnRecord{
		urec(1, 999, usnReasonFileCreate, false, "x.jpg"), // ParentFRN=999 不在缓存
	}, cache)
	if len(out) != 0 {
		t.Fatalf("外部父目录的记录应丢弃，got %+v", out)
	}
}

// TestPairAndResolve_WhitelistFilter 解析出白名单外路径（缓存手动填了非 store/* 路径）→ emit 丢弃。
func TestPairAndResolve_WhitelistFilter(t *testing.T) {
	cache := newCache(map[uint64]string{100: "backup/x"}) // backup/ 不在 scanDirs
	out := pairAndResolve([]usnRecord{
		urec(1, 100, usnReasonFileCreate, false, "y.jpg"),
	}, cache)
	if len(out) != 0 {
		t.Fatalf("白名单外路径应丢弃（got %q）", out[0].Path)
	}
}

// --- 集成测试（需管理员 + 真实 NTFS 卷）---

// mockCursorStore 内存 CursorStore（免 SQLite/cgo，供 provider 集成测试）。
type mockCursorStore struct {
	mu sync.Mutex
	m  map[string]*Cursor
}

func newMockCursorStore() *mockCursorStore { return &mockCursorStore{m: make(map[string]*Cursor)} }
func (s *mockCursorStore) key(j uint64, w string) string {
	return fmt.Sprintf("%d|%s", j, w)
}
func (s *mockCursorStore) Get(_ context.Context, j uint64, w string) (*Cursor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, ok := s.m[s.key(j, w)]; ok {
		c2 := *c
		return &c2, nil
	}
	return nil, nil
}
func (s *mockCursorStore) Save(_ context.Context, c Cursor) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c2 := c
	s.m[s.key(c.JournalID, c.WorkDir)] = &c2
	return nil
}

// TestUsnProvider_Integration 轻量集成：管理员下打开真实卷、QUERY、首次 D5 存游标、二次续读无错。
// 不造文件、避 USN 时序噪声；断言无错 + 返回路径命中白名单 + 游标前进（无重复）。
func TestUsnProvider_Integration(t *testing.T) {
	if !isElevated() {
		t.Skip("需管理员权限（USN 卷级读取 R2）")
	}
	logger.Log = zap.NewNop().Sugar() // 测试期 logger 未初始化，置 nop 防 ChangesSince 日志调用 panic
	workDir := t.TempDir()            // 系统盘 NTFS 临时目录
	for _, sub := range scanDirs {
		if err := os.MkdirAll(filepath.Join(workDir, filepath.FromSlash(sub)), 0o755); err != nil {
			t.Fatalf("建白名单子树失败 %s: %v", sub, err)
		}
	}
	p := NewUsnProvider(workDir, newMockCursorStore())

	// 首次：D5，存游标（NextUsn），返回空（不追溯历史）
	ch1, _, err := p.ChangesSince(context.Background(), nil)
	if err != nil {
		t.Fatalf("首次 ChangesSince 失败（openVolume/QUERY/游标）: %v", err)
	}

	// 二次：从已存游标续读，游标已前进 → 无重复
	ch2, _, err := p.ChangesSince(context.Background(), nil)
	if err != nil {
		t.Fatalf("二次 ChangesSince 失败: %v", err)
	}

	// 断言所有返回路径命中白名单（store/* 子树）
	for _, c := range append(append([]FileChange{}, ch1...), ch2...) {
		switch c.Kind {
		case ChangeMove:
			if !inScanDirs(c.Path) && !inScanDirs(c.ToPath) {
				t.Errorf("ChangeMove 路径未命中白名单: %+v", c)
			}
		default:
			if !inScanDirs(c.Path) {
				t.Errorf("变更路径未命中白名单: %+v", c)
			}
		}
	}
	t.Logf("集成通过：首次 %d 条、二次 %d 条（首次预期 0=D5）", len(ch1), len(ch2))
}
