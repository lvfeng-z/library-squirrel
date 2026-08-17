package plugin

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	entity "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/base/logger"
)

// TestMain 测试环境挂 nop logger（生产 logger 由应用启动时 Init，测试中为 nil）
func TestMain(m *testing.M) {
	if logger.Log == nil {
		logger.Log = zap.NewNop().Sugar()
	}
	os.Exit(m.Run())
}

// fakeUpgradeRepo 检查更新流分支测试用假仓储：仅实现 GetByPublicId，
// 未实现方法经嵌入的 nil 接口 panic（测试分支不应触达）
type fakeUpgradeRepo struct {
	Repository
	byPublicId map[string]*entity.Plugin
}

func (f *fakeUpgradeRepo) GetByPublicId(ctx context.Context, publicId string) (*entity.Plugin, error) {
	return f.byPublicId[publicId], nil
}

func (f *fakeUpgradeRepo) Save(ctx context.Context, plugin *entity.Plugin) error {
	f.byPublicId[plugin.PublicID.String] = plugin
	return nil
}

// writePluginZip 构造只含 plugin.json 的最小插件包
func writePluginZip(t *testing.T, manifest string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "plugin.zip")
	buf := &bytes.Buffer{}
	w := zip.NewWriter(buf)
	entry, err := w.Create("plugin.json")
	if err != nil {
		t.Fatalf("创建 zip 条目失败: %v", err)
	}
	if _, err := entry.Write([]byte(manifest)); err != nil {
		t.Fatalf("写入 manifest 失败: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("关闭 zip 失败: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("写插件包失败: %v", err)
	}
	return path
}

// bundledExisting 构造已安装的 bundled 来源插件记录
func bundledExisting(publicId, buildID, version string) *entity.Plugin {
	p := entity.NewPlugin()
	p.PublicID = ns(publicId)
	p.Name = ns("已装插件")
	p.Version = ns(version)
	p.BuildID = ns(buildID)
	p.Source = ns(SourceBundled)
	p.Uninstalled = sql.NullBool{Bool: false, Valid: true}
	return p
}

// availableManifest 构造带 buildId 的捆绑包 manifest（version 1.1.0）
func availableManifest(publicId, buildID string) string {
	return `{"id":"` + publicId + `","name":"测试插件","version":"1.1.0","buildId":"` + buildID +
		`","author":"tester","activation":{"type":1},` +
		`"extensions":{"frontendExtensions":[{"id":"v1","name":"视图","kind":"view","content":{"contentType":"code","source":"1"}}]}}`
}

// TestCompareVersion 点分版本号比较：数字段数值比较、非数字段字符串比较、空串最小
func TestCompareVersion(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.0", "1.1.0", -1},
		{"1.10.0", "1.9.0", 1},  // 数值比较，非字典序
		{"1.0", "1.0.0", -1},    // 缺段视为最小
		{"", "1.0.0", -1},       // 空串最小
		{"1.0.0", "", 1},
		{"v1.0", "1.0", 1},      // 非数字段按字符串比较
		{"1.0.0-beta", "1.0.0-alpha", 1},
	}
	for _, tt := range tests {
		if got := compareVersion(tt.a, tt.b); got != tt.want {
			t.Errorf("compareVersion(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

// TestUpgradeDirection 方向派生（仅展示用）
func TestUpgradeDirection(t *testing.T) {
	tests := []struct {
		installed, target, want string
	}{
		{"1.0.0", "1.1.0", "up"},
		{"1.2.0", "1.1.0", "down"},
		{"1.0.0", "1.0.0", "none"}, // 同号换构建
	}
	for _, tt := range tests {
		if got := upgradeDirection(tt.installed, tt.target); got != tt.want {
			t.Errorf("upgradeDirection(%q, %q) = %q, want %q", tt.installed, tt.target, got, tt.want)
		}
	}
}

// TestPendingListLifecycle 待办列表执行中守卫：claim 置位期间重复 claim 被拦，失败复位可重试，成功移除
func TestPendingListLifecycle(t *testing.T) {
	svc := NewService(nil, nil)
	const pid = "com.test.plugin"
	svc.recordPending(&pendingUpgradeEntry{PublicID: pid, Kind: PendingKindAvailable})

	if _, err := svc.claimPending(pid); err != nil {
		t.Fatalf("首次 claim 失败: %v", err)
	}
	if _, err := svc.claimPending(pid); err != ErrPendingUpgradeExecuting {
		t.Errorf("执行中重复 claim 应返回 ErrPendingUpgradeExecuting, got %v", err)
	}
	// 失败复位：守卫解除，可再次 claim
	svc.finishPending(pid, false)
	if _, err := svc.claimPending(pid); err != nil {
		t.Fatalf("失败复位后 claim 应成功, got %v", err)
	}
	// 成功移除：claim 报不存在
	svc.finishPending(pid, true)
	if _, err := svc.claimPending(pid); err != ErrPendingUpgradeNotFound {
		t.Errorf("成功移除后 claim 应返回 ErrPendingUpgradeNotFound, got %v", err)
	}
}

// TestDeclinePendingUpgradeRequiresBuildID 未打标待办（目标 buildId 为空）拒绝跳过；待办保留未消费
func TestDeclinePendingUpgradeRequiresBuildID(t *testing.T) {
	svc := NewService(nil, nil)
	const pid = "com.test.plugin"
	svc.recordPending(&pendingUpgradeEntry{PublicID: pid, Kind: PendingKindAvailable, TargetBuildID: ""})

	if _, err := svc.DeclinePendingUpgrade(context.Background(), pid); err != ErrDeclineRequiresBuildID {
		t.Fatalf("空 buildId 跳过应返回 ErrDeclineRequiresBuildID, got %v", err)
	}
	// 待办未被消费（守卫已复位）
	pending := svc.GetPendingUpgrades(context.Background())
	if len(pending) != 1 || pending[0].PublicID != pid {
		t.Fatalf("跳过被拒后待办应保留, got %+v", pending)
	}

	// 不存在的待办
	if _, err := svc.DeclinePendingUpgrade(context.Background(), "com.test.other"); err != ErrPendingUpgradeNotFound {
		t.Errorf("不存在待办应返回 ErrPendingUpgradeNotFound, got %v", err)
	}
}

// TestApplyPendingUpgradeRejectsNonAvailable 仅 available 待办可执行换版；forced/error 只读
func TestApplyPendingUpgradeRejectsNonAvailable(t *testing.T) {
	svc := NewService(nil, nil)
	const pid = "com.test.plugin"
	svc.recordPending(&pendingUpgradeEntry{PublicID: pid, Kind: PendingKindForced})

	if _, err := svc.ApplyPendingUpgrade(context.Background(), pid); err == nil {
		t.Fatal("forced 待办执行换版应报错")
	}
	pending := svc.GetPendingUpgrades(context.Background())
	if len(pending) != 1 {
		t.Fatalf("拒绝后待办应保留, got %+v", pending)
	}
}

// TestInstallBundledDetectBranches 检测分支矩阵（不触 installCore 的分支）：
// 判变成立记 available 待办 / 拒绝标记等值静默跳过 / 判变不成立跳过 / 包损坏记 error 待办。
// 强制与未打标分支触达 installCore（文件系统+备份），不在本单测覆盖
func TestInstallBundledDetectBranches(t *testing.T) {
	const pid = "com.test.plugin"
	ctx := context.Background()

	t.Run("判变成立记 available 待办", func(t *testing.T) {
		repo := &fakeUpgradeRepo{byPublicId: map[string]*entity.Plugin{
			pid: bundledExisting(pid, "old-build", "1.0.0"),
		}}
		svc := NewService(repo, nil)
		path := writePluginZip(t, availableManifest(pid, "new-build"))

		plugin, err := svc.InstallBundled(ctx, path)
		if err != nil || plugin != nil {
			t.Fatalf("available 分支应返回 (nil, nil), got (%v, %v)", plugin, err)
		}
		pending := svc.GetPendingUpgrades(ctx)
		if len(pending) != 1 {
			t.Fatalf("应记录 1 条待办, got %+v", pending)
		}
		p := pending[0]
		if p.Kind != PendingKindAvailable || p.PublicID != pid || p.TargetBuildID != "new-build" ||
			p.InstalledVersion != "1.0.0" || p.TargetVersion != "1.1.0" || p.Direction != "up" || p.Source != SourceBundled {
			t.Errorf("待办字段不符: %+v", p)
		}
	})

	t.Run("拒绝标记等值静默跳过", func(t *testing.T) {
		existing := bundledExisting(pid, "old-build", "1.0.0")
		existing.UpgradeDeclinedBuildID = ns("new-build")
		repo := &fakeUpgradeRepo{byPublicId: map[string]*entity.Plugin{pid: existing}}
		svc := NewService(repo, nil)
		path := writePluginZip(t, availableManifest(pid, "new-build"))

		plugin, err := svc.InstallBundled(ctx, path)
		if err != nil || plugin != nil {
			t.Fatalf("拒绝短路应返回 (nil, nil), got (%v, %v)", plugin, err)
		}
		if pending := svc.GetPendingUpgrades(ctx); len(pending) != 0 {
			t.Errorf("拒绝短路不应记待办, got %+v", pending)
		}
	})

	t.Run("判变不成立跳过", func(t *testing.T) {
		repo := &fakeUpgradeRepo{byPublicId: map[string]*entity.Plugin{
			pid: bundledExisting(pid, "same-build", "1.1.0"),
		}}
		svc := NewService(repo, nil)
		path := writePluginZip(t, availableManifest(pid, "same-build"))

		plugin, err := svc.InstallBundled(ctx, path)
		if err != nil || plugin != nil {
			t.Fatalf("判变不成立应返回 (nil, nil), got (%v, %v)", plugin, err)
		}
		if pending := svc.GetPendingUpgrades(ctx); len(pending) != 0 {
			t.Errorf("判变不成立不应记待办, got %+v", pending)
		}
	})

	t.Run("包损坏记 error 待办并上抛", func(t *testing.T) {
		svc := NewService(&fakeUpgradeRepo{byPublicId: map[string]*entity.Plugin{}}, nil)
		path := filepath.Join(t.TempDir(), "broken.zip")
		if err := os.WriteFile(path, []byte("not a zip"), 0o644); err != nil {
			t.Fatalf("写损坏包失败: %v", err)
		}

		if _, err := svc.InstallBundled(ctx, path); err != ErrInvalidPackage {
			t.Fatalf("损坏包应返回 ErrInvalidPackage, got %v", err)
		}
		pending := svc.GetPendingUpgrades(ctx)
		if len(pending) != 1 || pending[0].Kind != PendingKindError || pending[0].PublicID != "" ||
			pending[0].PluginName != "broken.zip" || pending[0].Message == "" {
			t.Errorf("error 待办字段不符: %+v", pending)
		}
	})
}

// TestRestorePendingUpgradeClearsDeclined 反悔入口：清拒绝标记须能写 NULL（Save 全字段替换语义由仓储实现保证，
// 此处验证标记清除后不再构成等值短路——重跑检测判变成立即重建待办）
func TestRestorePendingUpgradeClearsDeclined(t *testing.T) {
	const pid = "com.test.plugin"
	existing := bundledExisting(pid, "old-build", "1.0.0")
	existing.UpgradeDeclinedBuildID = ns("new-build")
	existing.SourceDetail = ns("")

	// SourceDetail 为空（无包路径可重检）时 Restore 仅清标记，不重建待办、不报错
	repo := &fakeUpgradeRepo{byPublicId: map[string]*entity.Plugin{pid: existing}}
	svc := NewService(repo, nil)

	if _, err := svc.RestorePendingUpgrade(context.Background(), pid); err != nil {
		t.Fatalf("Restore 应成功, got %v", err)
	}
	if existing.UpgradeDeclinedBuildID.Valid {
		t.Error("拒绝标记应被清除")
	}
}
