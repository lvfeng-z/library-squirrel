package staging

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap/zapcore"
)

// TestSweepRootRegistryDispatch 根注册表派发：任务属主根按注入谓词判活去留（活=保留、死=回收），
// import/merge 启动一律回收，export 按账本先删目标侧临时文件再回收作用域（账外文件不动——
// 描述即权威清单，不扫描目标目录）。
func TestSweepRootRegistryDispatch(t *testing.T) {
	workDir := t.TempDir()
	ctx := context.Background()

	taskScopes := []struct {
		owner Owner
		key   string
	}{
		{OwnerDownload, "100"},
		{OwnerDownload, "200"},
		{OwnerShareReceive, "300"},
		{OwnerShareReceive, "301"},
	}
	for _, p := range taskScopes {
		if _, err := CreateScope(ctx, workDir, p.owner, p.key, "files"); err != nil {
			t.Fatalf("创建作用域失败: %v", err)
		}
	}
	// 在途暂存内容随回收一并消失
	writeTextT(t, filepath.Join(ScopePath(workDir, OwnerDownload, "200"), "videoTrack_000.mp4.part"), "x")

	importKey, err := MintScopeKey()
	if err != nil {
		t.Fatalf("铸造键失败: %v", err)
	}
	importDir, err := CreateScope(ctx, workDir, OwnerImport, importKey, "import-files")
	if err != nil {
		t.Fatalf("创建 import 作用域失败: %v", err)
	}
	mergeKey, err := MintScopeKey()
	if err != nil {
		t.Fatalf("铸造键失败: %v", err)
	}
	mergeDir, err := CreateScope(ctx, workDir, OwnerMerge, mergeKey, "merge-out")
	if err != nil {
		t.Fatalf("创建 merge 作用域失败: %v", err)
	}

	// 导出：目标目录内一个账内临时文件 + 一个账外文件
	target := t.TempDir()
	inLedger := filepath.Join(target, "library-squirrel-export-a.zip.tmp")
	outLedger := filepath.Join(target, "unrelated.txt")
	writeTextT(t, inLedger, "z")
	writeTextT(t, outLedger, "keep")
	exportKey, err := MintScopeKey()
	if err != nil {
		t.Fatalf("铸造键失败: %v", err)
	}
	exportDir, err := CreateExportScope(ctx, workDir, exportKey, "export-zip", ExportLedger{
		TargetDir: target,
		TempFiles: []string{"library-squirrel-export-a.zip.tmp"},
	})
	if err != nil {
		t.Fatalf("创建导出作用域失败: %v", err)
	}

	if err := SweepAtStartup(ctx, workDir, aliveFor("100", "300")); err != nil {
		t.Fatalf("清扫失败: %v", err)
	}

	for _, kept := range []string{
		ScopePath(workDir, OwnerDownload, "100"),
		ScopePath(workDir, OwnerShareReceive, "300"),
	} {
		if !pathExists(t, kept) {
			t.Fatalf("在途作用域被误回收: %s", kept)
		}
	}
	for _, gone := range []string{
		ScopePath(workDir, OwnerDownload, "200"),
		ScopePath(workDir, OwnerShareReceive, "301"),
		importDir,
		mergeDir,
		exportDir,
	} {
		if pathExists(t, gone) {
			t.Fatalf("应回收作用域仍存在: %s", gone)
		}
	}
	if pathExists(t, inLedger) {
		t.Fatalf("账内导出临时文件未被删除: %s", inLedger)
	}
	if !pathExists(t, outLedger) {
		t.Fatalf("账外文件被误删: %s", outLedger)
	}
}

// TestSweepReclaimsScopelessDirectories 无描述（含非空目录）/描述损坏/键名不一致/临时目录残留/
// 未登记根/散落文件/导出缺账本一律回收并记 Warn，有效作用域不受波及。
func TestSweepReclaimsScopelessDirectories(t *testing.T) {
	workDir := t.TempDir()
	ctx := context.Background()

	// 有效作用域（对照，不应被波及）
	okDir, err := CreateScope(ctx, workDir, OwnerDownload, "77", "files")
	if err != nil {
		t.Fatalf("创建作用域失败: %v", err)
	}

	// 非空目录但无描述（谓词即使判活也不救——描述缺失先于策略判定）
	noDesc := mkdirAllT(t, workDir, "staging", "download", "999")
	writeTextT(t, filepath.Join(noDesc, "image_000.jpg.part"), "x")
	mkdirAllT(t, noDesc, "sub")
	writeTextT(t, filepath.Join(noDesc, "sub", "deep.bin"), "x")

	// 描述损坏（坏 JSON）
	corrupt := mkdirAllT(t, workDir, "staging", "download", "998")
	writeTextT(t, filepath.Join(corrupt, scopeDescFileName), "{bad")

	// 描述键与目录名不一致（外部改名形态）
	renamed, err := CreateScope(ctx, workDir, OwnerDownload, "111", "files")
	if err != nil {
		t.Fatalf("创建作用域失败: %v", err)
	}
	if err := os.Rename(renamed, ScopePath(workDir, OwnerDownload, "997")); err != nil {
		t.Fatalf("模拟外部改名失败: %v", err)
	}

	// 创建期临时目录残留（rename 前崩溃形态）
	tempLeftover := mkdirAllT(t, workDir, "staging", "download", ".tmp-123456")

	// 未登记属主根 + 暂存总根下的散落文件
	unregistered := mkdirAllT(t, workDir, "staging", "mystery", "abc")
	stray := filepath.Join(workDir, "staging", "junk.txt")
	writeTextT(t, stray, "x")

	// 导出作用域缺账本（描述合法但无 Export 段）
	noLedger := mkdirAllT(t, workDir, "staging", "export", "nokey")
	writeTextT(t, filepath.Join(noLedger, scopeDescFileName), `{"scopeKey":"nokey","contentShape":"export-zip","createdAt":1}`)

	logs := withLogObserver(t, zapcore.WarnLevel)
	if err := SweepAtStartup(ctx, workDir, aliveFor("77", "999")); err != nil {
		t.Fatalf("清扫失败: %v", err)
	}

	if !pathExists(t, okDir) {
		t.Fatalf("有效作用域被误回收: %s", okDir)
	}
	for _, gone := range []string{
		noDesc, corrupt,
		ScopePath(workDir, OwnerDownload, "997"),
		tempLeftover, unregistered, stray, noLedger,
	} {
		if pathExists(t, gone) {
			t.Fatalf("异常条目未被回收: %s", gone)
		}
	}
	if n := countWarnContaining(logs, "作用域描述缺失"); n == 0 {
		t.Fatalf("描述缺失回收未记 Warn")
	}
	if n := countWarnContaining(logs, "回收异常暂存条目"); n == 0 {
		t.Fatalf("异常回收未记 Warn")
	}
	if n := countWarnContaining(logs, "缺少账本"); n == 0 {
		t.Fatalf("导出缺账本未记 Warn")
	}
}

// TestSweepSingleRemoveFailureContinues 清扫健壮性：单个回收失败不中断其余目录，失败者留待
// 下次启动重试并记 Warn。
func TestSweepSingleRemoveFailureContinues(t *testing.T) {
	workDir := t.TempDir()
	ctx := context.Background()
	for _, key := range []string{"400", "500"} {
		if _, err := CreateScope(ctx, workDir, OwnerDownload, key, "files"); err != nil {
			t.Fatalf("创建作用域失败: %v", err)
		}
	}
	importKey, err := MintScopeKey()
	if err != nil {
		t.Fatalf("铸造键失败: %v", err)
	}
	importDir, err := CreateScope(ctx, workDir, OwnerImport, importKey, "files")
	if err != nil {
		t.Fatalf("创建 import 作用域失败: %v", err)
	}

	// 注入「500 目录回收失败」的桩，其余路径走真实 RemoveAll
	realRemove := removeAll
	removeAll = func(path string) error {
		if filepath.Base(path) == "500" {
			return errors.New("目录被占用")
		}
		return realRemove(path)
	}
	t.Cleanup(func() { removeAll = realRemove })

	logs := withLogObserver(t, zapcore.WarnLevel)
	if err := SweepAtStartup(ctx, workDir, aliveFor()); err != nil {
		t.Fatalf("清扫失败: %v", err)
	}
	if pathExists(t, ScopePath(workDir, OwnerDownload, "400")) {
		t.Fatalf("未注入失败的目录未被回收")
	}
	if !pathExists(t, ScopePath(workDir, OwnerDownload, "500")) {
		t.Fatalf("注入失败的目录不应被回收")
	}
	if pathExists(t, importDir) {
		t.Fatalf("import 作用域未被回收（单败中断了后续目录）")
	}
	if n := countWarnContaining(logs, "回收失败"); n == 0 {
		t.Fatalf("回收失败未记 Warn")
	}
}

// TestSweepTimingContract 清扫时序契约：启动清扫先行、服务在其后创建作用域——同会话内再清扫
// 不误回收在途作用域；属主消亡后的清扫才回收。
func TestSweepTimingContract(t *testing.T) {
	workDir := t.TempDir()
	ctx := context.Background()
	// 启动清扫先行（此刻无任何作用域）
	if err := SweepAtStartup(ctx, workDir, aliveFor("777")); err != nil {
		t.Fatalf("清扫失败: %v", err)
	}
	// 清扫之后服务才创建作用域并写入在途内容
	dir, err := CreateScope(ctx, workDir, OwnerDownload, "777", "files")
	if err != nil {
		t.Fatalf("创建作用域失败: %v", err)
	}
	writeTextT(t, filepath.Join(dir, "image_000.jpg.part"), "x")
	// 同一会话内再清扫（属主仍在）：不误回收
	if err := SweepAtStartup(ctx, workDir, aliveFor("777")); err != nil {
		t.Fatalf("清扫失败: %v", err)
	}
	if !pathExists(t, dir) {
		t.Fatalf("在途作用域被误回收: %s", dir)
	}
	if _, err := readScopeDescription(dir); err != nil {
		t.Fatalf("描述受损: %v", err)
	}
	// 属主消亡（下次启动形态）：回收
	if err := SweepAtStartup(ctx, workDir, aliveFor()); err != nil {
		t.Fatalf("清扫失败: %v", err)
	}
	if pathExists(t, dir) {
		t.Fatalf("属主消亡后作用域未被回收: %s", dir)
	}
}

// TestSweepOwnerAliveNilKeepsScopes 归属谓词未注入（装配缺陷防御）：任务属主根下作用域
// 宁留勿毁，记 Warn。
func TestSweepOwnerAliveNilKeepsScopes(t *testing.T) {
	workDir := t.TempDir()
	ctx := context.Background()
	dir, err := CreateScope(ctx, workDir, OwnerDownload, "888", "files")
	if err != nil {
		t.Fatalf("创建作用域失败: %v", err)
	}
	logs := withLogObserver(t, zapcore.WarnLevel)
	if err := SweepAtStartup(ctx, workDir, nil); err != nil {
		t.Fatalf("清扫失败: %v", err)
	}
	if !pathExists(t, dir) {
		t.Fatalf("谓词缺失时作用域被误回收: %s", dir)
	}
	if n := countWarnContaining(logs, "归属谓词未注入"); n == 0 {
		t.Fatalf("谓词缺失未记 Warn")
	}
}

// TestCleanExportLedger 账本清理单元：账内文件删除、非法文件名跳过并记 Warn。
func TestCleanExportLedger(t *testing.T) {
	target := t.TempDir()
	writeTextT(t, filepath.Join(target, "a.zip.tmp"), "a")
	writeTextT(t, filepath.Join(target, "b.zip.tmp"), "b")
	desc := &ScopeDescription{
		ScopeKey:     "k",
		ContentShape: "export-zip",
		Export: &ExportLedger{
			TargetDir: target,
			TempFiles: []string{"a.zip.tmp", "sub/b.zip.tmp", "b.zip.tmp"},
		},
	}
	logs := withLogObserver(t, zapcore.WarnLevel)
	cleanExportLedger(desc)
	if pathExists(t, filepath.Join(target, "a.zip.tmp")) || pathExists(t, filepath.Join(target, "b.zip.tmp")) {
		t.Fatalf("账内临时文件未被删除")
	}
	if n := countWarnContaining(logs, "文件名非法"); n == 0 {
		t.Fatalf("非法文件名未记 Warn")
	}
}

// TestSweepExportTargetUnreachableTolerated 目标目录不存在：目标侧本无临时文件（IsNotExist
// 按已清理落定），清扫不报错、作用域照常回收。
func TestSweepExportTargetUnreachableTolerated(t *testing.T) {
	workDir := t.TempDir()
	ctx := context.Background()
	exportDir, err := CreateExportScope(ctx, workDir, "gone-key", "export-zip", ExportLedger{
		TargetDir: filepath.Join(workDir, "不存在的目标目录"),
		TempFiles: []string{"x.zip.tmp"},
	})
	if err != nil {
		t.Fatalf("创建导出作用域失败: %v", err)
	}
	if err := SweepAtStartup(ctx, workDir, aliveFor()); err != nil {
		t.Fatalf("目标不可达不应令清扫报错: %v", err)
	}
	if pathExists(t, exportDir) {
		t.Fatalf("导出作用域未被回收: %s", exportDir)
	}
}

// TestSweepGuards 空串 workDir 与暂存总根缺失均为无操作。
func TestSweepGuards(t *testing.T) {
	ctx := context.Background()
	if err := SweepAtStartup(ctx, "", aliveFor()); err != nil {
		t.Fatalf("空 workDir 应无操作: %v", err)
	}
	if err := SweepAtStartup(ctx, t.TempDir(), aliveFor()); err != nil {
		t.Fatalf("无暂存根应无操作: %v", err)
	}
}
