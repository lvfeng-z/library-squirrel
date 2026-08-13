//go:build windows

package fsmonitor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/storeRegistry"
	"go.uber.org/zap"
)

// TestResolve_HitAndMiss 验证父目录命中缓存时拼出全路径，未命中（workDir 外）返回 ok=false。
func TestResolve_HitAndMiss(t *testing.T) {
	c := newFrnPathCache("X:/wd")
	c.frnToRel[100] = "store/resource"
	got, ok := c.Resolve(100, "a.jpg")
	if !ok || got != "store/resource/a.jpg" {
		t.Fatalf("Resolve(100,a.jpg) = %q,%v want store/resource/a.jpg,true", got, ok)
	}
	// 中文文件名拼接
	if got, ok := c.Resolve(100, "作品_001.jpg"); !ok || got != "store/resource/作品_001.jpg" {
		t.Fatalf("中文文件名解析 = %q,%v", got, ok)
	}
	if _, ok := c.Resolve(999, "x.jpg"); ok {
		t.Fatal("外部 ParentFRN 应 ok=false")
	}
}

// TestOnDirCreate_InSubtree 验证父目录已缓存时新目录加入缓存，进而可解析其下文件。
func TestOnDirCreate_InSubtree(t *testing.T) {
	c := newFrnPathCache("X:/wd")
	c.frnToRel[100] = "store/resource"
	c.OnDirCreate(usnRecord{FRN: 200, ParentFRN: 100, FileName: "新作者"})
	if got, ok := c.Resolve(200, "x.jpg"); !ok || got != "store/resource/新作者/x.jpg" {
		t.Fatalf("新建目录后解析其下文件 = %q,%v", got, ok)
	}
}

// TestOnDirCreate_ExternalParent 验证父目录未缓存时新目录不加入（子树外新建）。
func TestOnDirCreate_ExternalParent(t *testing.T) {
	c := newFrnPathCache("X:/wd")
	c.OnDirCreate(usnRecord{FRN: 200, ParentFRN: 999, FileName: "x"})
	if _, ok := c.frnToRel[200]; ok {
		t.Fatal("父目录未缓存时新目录不应入缓存")
	}
}

// TestOnDirDelete_SubtreeCascade 验证目录删除移除自身及所有下级条目，兄弟与父级保留。
func TestOnDirDelete_SubtreeCascade(t *testing.T) {
	c := newFrnPathCache("X:/wd")
	c.frnToRel[100] = "store/resource"
	c.frnToRel[200] = "store/resource/A"
	c.frnToRel[300] = "store/resource/A/B"
	c.frnToRel[400] = "store/resource/C"
	c.OnDirDelete(usnRecord{FRN: 200})
	if _, ok := c.frnToRel[200]; ok {
		t.Fatal("删除目录自身应移除")
	}
	if _, ok := c.frnToRel[300]; ok {
		t.Fatal("删除目录下级应级联移除")
	}
	if _, ok := c.frnToRel[400]; !ok {
		t.Fatal("删除目录兄弟应保留")
	}
	if _, ok := c.frnToRel[100]; !ok {
		t.Fatal("删除目录父级应保留")
	}
}

// TestOnDirDelete_NotInCache 验证删除不在缓存的目录为无害空操作。
func TestOnDirDelete_NotInCache(t *testing.T) {
	c := newFrnPathCache("X:/wd")
	c.frnToRel[100] = "store/resource"
	c.OnDirDelete(usnRecord{FRN: 999})
	if _, ok := c.frnToRel[100]; !ok {
		t.Fatal("删除外部目录不应影响缓存")
	}
}

// TestOnDirRename_InSubtreeMove 验证子树内移动：整棵迁移到新父目录+新名（含下级前缀更新）。
func TestOnDirRename_InSubtreeMove(t *testing.T) {
	c := newFrnPathCache("X:/wd")
	c.frnToRel[10] = "store/resource"
	c.frnToRel[11] = "store/thumbnail"
	c.frnToRel[200] = "store/resource/oldDir"
	c.frnToRel[300] = "store/resource/oldDir/sub"
	// oldDir 从 store/resource 移到 store/thumbnail 并改名 newDir
	c.OnDirRename(
		usnRecord{FRN: 200, ParentFRN: 10, FileName: "oldDir"},
		usnRecord{FRN: 200, ParentFRN: 11, FileName: "newDir"},
	)
	if got := c.frnToRel[200]; got != "store/thumbnail/newDir" {
		t.Fatalf("移动后目录路径 = %q want store/thumbnail/newDir", got)
	}
	if got := c.frnToRel[300]; got != "store/thumbnail/newDir/sub" {
		t.Fatalf("移动后下级路径 = %q want store/thumbnail/newDir/sub", got)
	}
}

// TestOnDirRename_MoveOut 验证移出 workDir 子树：新父目录未缓存→整棵移除。
func TestOnDirRename_MoveOut(t *testing.T) {
	c := newFrnPathCache("X:/wd")
	c.frnToRel[10] = "store/resource"
	c.frnToRel[200] = "store/resource/D"
	c.frnToRel[300] = "store/resource/D/sub"
	c.OnDirRename(
		usnRecord{FRN: 200, ParentFRN: 10, FileName: "D"},
		usnRecord{FRN: 200, ParentFRN: 999, FileName: "D"},
	)
	if _, ok := c.frnToRel[200]; ok {
		t.Fatal("移出子树的目录应被移除")
	}
	if _, ok := c.frnToRel[300]; ok {
		t.Fatal("移出子树的下级应级联移除")
	}
}

// TestOnDirRename_MoveIn 验证外部移入：FRN 原不在缓存，新父目录已缓存→作为新建加入。
func TestOnDirRename_MoveIn(t *testing.T) {
	c := newFrnPathCache("X:/wd")
	c.frnToRel[10] = "store/resource"
	c.OnDirRename(
		usnRecord{FRN: 200, ParentFRN: 999, FileName: "extDir"},
		usnRecord{FRN: 200, ParentFRN: 10, FileName: "inDir"},
	)
	if got := c.frnToRel[200]; got != "store/resource/inDir" {
		t.Fatalf("移入后目录路径 = %q want store/resource/inDir", got)
	}
}

// TestOnDirRename_SameNameNoOp 验证改名前后路径不变（同名）时无副作用。
func TestOnDirRename_SameNameNoOp(t *testing.T) {
	c := newFrnPathCache("X:/wd")
	c.frnToRel[10] = "store/resource"
	c.frnToRel[200] = "store/resource/D"
	c.OnDirRename(
		usnRecord{FRN: 200, ParentFRN: 10, FileName: "D"},
		usnRecord{FRN: 200, ParentFRN: 10, FileName: "D"},
	)
	if got := c.frnToRel[200]; got != "store/resource/D" {
		t.Fatalf("同名 rename 后路径应不变 = %q", got)
	}
}

// TestOnDirRename_ChainedConvergence 验证连读多条重命名记录后缓存收敛到最终位置
// （S1 构建限制的核心保障：以缓存当前路径为迁移源，逐条重放使中间状态正确）。
func TestOnDirRename_ChainedConvergence(t *testing.T) {
	c := newFrnPathCache("X:/wd")
	c.frnToRel[10] = "store/resource"
	// 模拟 S1 构建后的最终位置：D 当前在 "store/resource/c"
	c.frnToRel[200] = "store/resource/c"
	c.frnToRel[300] = "store/resource/c/sub"
	// 重放离线期记录：a→b，然后 b→c（最终与 S1 一致）
	c.OnDirRename(
		usnRecord{FRN: 200, ParentFRN: 10, FileName: "a"},
		usnRecord{FRN: 200, ParentFRN: 10, FileName: "b"},
	)
	// 第一条重命名后，缓存应以「当前路径」为源迁移到 b（而非从 a 找不到条目）
	if got := c.frnToRel[200]; got != "store/resource/b" {
		t.Fatalf("a→b 后目录应在 b 位置 = %q", got)
	}
	if got := c.frnToRel[300]; got != "store/resource/b/sub" {
		t.Fatalf("a→b 后下级应跟随 = %q", got)
	}
	c.OnDirRename(
		usnRecord{FRN: 200, ParentFRN: 10, FileName: "b"},
		usnRecord{FRN: 200, ParentFRN: 10, FileName: "c"},
	)
	if got := c.frnToRel[200]; got != "store/resource/c" {
		t.Fatalf("b→c 后应收敛到 c = %q", got)
	}
	if got := c.frnToRel[300]; got != "store/resource/c/sub" {
		t.Fatalf("b→c 后下级应收敛 = %q", got)
	}
}

// TestFrnPathCache_Build 集成测试：在临时 workDir 建白名单子树，Build 后验证
// readFRN（FSCTL_READ_FILE_USN_DATA）非管理员下可读目录 FRN（R2 对照结论），缓存非空且多级命中。
func TestFrnPathCache_Build(t *testing.T) {
	logger.Log = zap.NewNop().Sugar() // 测试期 logger 未初始化，置 nop 防日志调用 panic
	workDir := t.TempDir()
	for _, sub := range storeRegistry.RegisteredPaths {
		if err := os.MkdirAll(filepath.Join(workDir, filepath.FromSlash(sub)), 0o755); err != nil {
			t.Fatalf("建目录失败 %s: %v", sub, err)
		}
	}
	// store/resource 下建一个中文子目录，验证多级目录与中文路径都被缓存
	if err := os.MkdirAll(filepath.Join(workDir, "store/resource/作者"), 0o755); err != nil {
		t.Fatalf("建子目录失败: %v", err)
	}
	// readFRN 探针：直接读一个目录的 FRN，定位真实错误（权限/参数）
	probe := filepath.Join(workDir, "store/resource")
	if frn, err := readFRN(probe); err != nil {
		t.Logf("readFRN 探针失败 %s: %v", probe, err)
	} else {
		t.Logf("readFRN 探针成功 %s: FRN=%d", probe, frn)
	}

	c := newFrnPathCache(workDir)
	if err := c.Build(context.Background()); err != nil {
		t.Fatalf("Build 失败（readFRN 不可用？卷非 NTFS？）: %v", err)
	}
	if len(c.frnToRel) == 0 {
		t.Fatal("Build 后缓存为空")
	}
	// 至少缓存各白名单子树根 + 作者 子目录
	if len(c.frnToRel) < len(storeRegistry.RegisteredPaths) {
		t.Fatalf("缓存目录数 %d < 白名单根数 %d", len(c.frnToRel), len(storeRegistry.RegisteredPaths))
	}
	// 验证多级中文目录命中缓存
	found := false
	for _, rel := range c.frnToRel {
		if rel == "store/resource/作者" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("期望缓存含 store/resource/作者，实际: %v", c.frnToRel)
	}
}
