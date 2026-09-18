package staging

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestMintScopeKeyUnique 铸造键唯一性：多次铸造不撞键，且形态过键校验（hex、32 字符）——
// 计数器或秒级时间键在同批快速铸造下会撞键，本用例同时排除这两类退化。
func TestMintScopeKeyUnique(t *testing.T) {
	seen := make(map[string]bool, 64)
	for i := 0; i < 64; i++ {
		key, err := MintScopeKey()
		if err != nil {
			t.Fatalf("铸造作用域键失败: %v", err)
		}
		if len(key) != 32 {
			t.Fatalf("铸造键长度非 32: %q", key)
		}
		if err := validateScopeKey(key); err != nil {
			t.Fatalf("铸造键未过键校验: %v", err)
		}
		if seen[key] {
			t.Fatalf("两次铸造撞键: %s", key)
		}
		seen[key] = true
	}
}

// TestCreateScopeWritesLegalDescription 创建原子性：经包入口创建的目录必含合法描述（键/形态/
// 创建时刻），且属主根内无临时目录残留。
func TestCreateScopeWritesLegalDescription(t *testing.T) {
	workDir := t.TempDir()
	before := time.Now().UnixMilli()

	dir, err := CreateScope(context.Background(), workDir, OwnerDownload, "123", "role-seq-files")
	if err != nil {
		t.Fatalf("创建作用域失败: %v", err)
	}
	if want := filepath.Join(workDir, "staging", "download", "123"); dir != want {
		t.Fatalf("作用域路径不符: got %s want %s", dir, want)
	}
	desc, err := readScopeDescription(dir)
	if err != nil {
		t.Fatalf("读描述失败: %v", err)
	}
	if desc.ScopeKey != "123" || desc.ContentShape != "role-seq-files" {
		t.Fatalf("描述字段不符: %+v", desc)
	}
	if desc.CreatedAt < before || desc.CreatedAt > time.Now().UnixMilli() {
		t.Fatalf("创建时刻越界: %d", desc.CreatedAt)
	}
	if desc.Export != nil {
		t.Fatalf("非导出属主的描述不应含账本: %+v", desc.Export)
	}
	entries, err := os.ReadDir(filepath.Join(workDir, "staging", "download"))
	if err != nil {
		t.Fatalf("读属主根失败: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "123" {
		t.Fatalf("属主根存在临时目录残留: %v", entries)
	}
}

// TestCreateScopeValidation 创建入口的参数校验。
func TestCreateScopeValidation(t *testing.T) {
	workDir := t.TempDir()
	ctx := context.Background()
	if _, err := CreateScope(ctx, "", OwnerDownload, "1", "files"); !errors.Is(err, ErrWorkDirEmpty) {
		t.Fatalf("空 workDir 未被拒绝: %v", err)
	}
	cases := []struct {
		name    string
		owner   Owner
		key     string
		shape   string
		wantErr error
	}{
		{"未登记属主", Owner("mystery"), "1", "files", ErrOwnerNotRegistered},
		{"空键", OwnerDownload, "", "files", ErrInvalidScopeKey},
		{"路径分隔符", OwnerDownload, "a/b", "files", ErrInvalidScopeKey},
		{"目录游标", OwnerDownload, "..", "files", ErrInvalidScopeKey},
		{"点号前缀", OwnerDownload, ".hidden", "files", ErrInvalidScopeKey},
		{"超长键", OwnerDownload, strings.Repeat("a", 129), "files", ErrInvalidScopeKey},
		{"空内容形态", OwnerDownload, "1", "", ErrEmptyContentShape},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := CreateScope(ctx, workDir, tc.owner, tc.key, tc.shape); !errors.Is(err, tc.wantErr) {
				t.Fatalf("错误不符: got %v want %v", err, tc.wantErr)
			}
		})
	}
}

// TestCreateScopeDuplicateRejected 同键作用域不可重复创建：既有作用域与外部遗留目录均返回
// ErrScopeExists，不覆盖既有内容。
func TestCreateScopeDuplicateRejected(t *testing.T) {
	workDir := t.TempDir()
	ctx := context.Background()
	if _, err := CreateScope(ctx, workDir, OwnerDownload, "42", "files"); err != nil {
		t.Fatalf("首次创建失败: %v", err)
	}
	if _, err := CreateScope(ctx, workDir, OwnerDownload, "42", "files"); !errors.Is(err, ErrScopeExists) {
		t.Fatalf("重复创建未拒绝: %v", err)
	}
	// 外部遗留目录（无描述）同样拒绝，须先回收再重建
	mkdirAllT(t, ScopePath(workDir, OwnerDownload, "43"))
	if _, err := CreateScope(ctx, workDir, OwnerDownload, "43", "files"); !errors.Is(err, ErrScopeExists) {
		t.Fatalf("既有目录未拒绝: %v", err)
	}
}

// TestCreateExportScopeLedgerRoundTrip 导出形态描述：账本字段完整落盘往返。
func TestCreateExportScopeLedgerRoundTrip(t *testing.T) {
	workDir := t.TempDir()
	target := t.TempDir()
	ledger := ExportLedger{TargetDir: target, TempFiles: []string{"library-squirrel-export-a.zip.tmp"}}
	dir, err := CreateExportScope(context.Background(), workDir, "exp-key-1", "export-zip", ledger)
	if err != nil {
		t.Fatalf("创建导出作用域失败: %v", err)
	}
	if want := ScopePath(workDir, OwnerExport, "exp-key-1"); dir != want {
		t.Fatalf("作用域路径不符: got %s want %s", dir, want)
	}
	desc, err := readScopeDescription(dir)
	if err != nil {
		t.Fatalf("读描述失败: %v", err)
	}
	if desc.Export == nil || desc.Export.TargetDir != target || len(desc.Export.TempFiles) != 1 ||
		desc.Export.TempFiles[0] != "library-squirrel-export-a.zip.tmp" {
		t.Fatalf("账本往返不符: %+v", desc.Export)
	}
}

// TestCreateExportScopeValidation 账本校验：缺目标目录、临时文件名带路径段、空 workDir 均拒绝。
func TestCreateExportScopeValidation(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	if _, err := CreateExportScope(ctx, workDir, "k1", "export-zip", ExportLedger{TempFiles: []string{"a.tmp"}}); !errors.Is(err, ErrEmptyExportTargetDir) {
		t.Fatalf("缺目标目录未拒绝: %v", err)
	}
	if _, err := CreateExportScope(ctx, workDir, "k2", "export-zip", ExportLedger{TargetDir: "x", TempFiles: []string{"sub/a.tmp"}}); !errors.Is(err, ErrInvalidTempFileName) {
		t.Fatalf("带路径段文件名未拒绝: %v", err)
	}
	if _, err := CreateExportScope(ctx, "", "k3", "export-zip", ExportLedger{TargetDir: "x"}); !errors.Is(err, ErrWorkDirEmpty) {
		t.Fatalf("空 workDir 未拒绝: %v", err)
	}
}

// TestReadScopeDescriptionAnomalyForms 描述异常形态识别：缺失/坏 JSON/字段校验不过分别报错。
func TestReadScopeDescriptionAnomalyForms(t *testing.T) {
	workDir := t.TempDir()
	missing := mkdirAllT(t, workDir, "staging", "download", "100")
	if _, err := readScopeDescription(missing); !errors.Is(err, ErrScopeDescMissing) {
		t.Fatalf("描述缺失未识别: %v", err)
	}
	corrupt := mkdirAllT(t, workDir, "staging", "download", "101")
	writeTextT(t, filepath.Join(corrupt, scopeDescFileName), "{not-json")
	if _, err := readScopeDescription(corrupt); !errors.Is(err, ErrScopeDescCorrupt) {
		t.Fatalf("坏 JSON 未识别: %v", err)
	}
	noKey := mkdirAllT(t, workDir, "staging", "download", "102")
	writeTextT(t, filepath.Join(noKey, scopeDescFileName), `{"contentShape":"x","createdAt":1}`)
	if _, err := readScopeDescription(noKey); !errors.Is(err, ErrScopeDescCorrupt) {
		t.Fatalf("缺稳定键未识别: %v", err)
	}
	noShape := mkdirAllT(t, workDir, "staging", "download", "103")
	writeTextT(t, filepath.Join(noShape, scopeDescFileName), `{"scopeKey":"103","createdAt":1}`)
	if _, err := readScopeDescription(noShape); !errors.Is(err, ErrScopeDescCorrupt) {
		t.Fatalf("缺内容形态未识别: %v", err)
	}
}

// TestRemoveScope 单作用域回收：存在即删、不存在容忍、空 workDir 无操作。
func TestRemoveScope(t *testing.T) {
	workDir := t.TempDir()
	ctx := context.Background()
	dir, err := CreateScope(ctx, workDir, OwnerMerge, "m1", "files")
	if err != nil {
		t.Fatalf("创建作用域失败: %v", err)
	}
	if err := RemoveScope(ctx, workDir, OwnerMerge, "m1"); err != nil {
		t.Fatalf("回收失败: %v", err)
	}
	if pathExists(t, dir) {
		t.Fatalf("作用域未被回收: %s", dir)
	}
	if err := RemoveScope(ctx, workDir, OwnerMerge, "m1"); err != nil {
		t.Fatalf("不存在应容忍: %v", err)
	}
	if err := RemoveScope(ctx, "", OwnerMerge, "m1"); err != nil {
		t.Fatalf("空 workDir 应无操作: %v", err)
	}
}
