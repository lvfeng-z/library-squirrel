package plugin

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/plugin/participation"
	"github.com/library-squirrel/backend/util"
	pluginsdktransport "github.com/lvfeng-z/library-squirrel-sdk/transport"
)

// recordingEvalTrigger 求值触发记录替身：记录 TriggerEvaluation 收到的 publicId 序列
type recordingEvalTrigger struct {
	mu      sync.Mutex
	trigger []string
}

func (r *recordingEvalTrigger) TriggerEvaluation(pluginPublicId string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.trigger = append(r.trigger, pluginPublicId)
}

func (r *recordingEvalTrigger) calls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.trigger...)
}

// newSettingTestServiceWithTrigger 设置服务测试装配（带求值触发替身；其余与
// newSettingTestService 同构）
func newSettingTestServiceWithTrigger(t *testing.T, trigger ParticipationEvalTrigger) (*PluginSettingService, *Service) {
	t.Helper()
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	if err := db.AutoMigrate(&entity.Plugin{}); err != nil {
		t.Fatalf("迁移测试实体失败: %v", err)
	}
	repo := NewRepository(db)
	svc := NewService(repo, nil)
	return NewPluginSettingService(repo, NewPluginStorageService(newMockStorageRepo()), util.RootPath(), trigger), svc
}

// resolverManifestWith 带根级 settingsResolver 声明的可安装清单（纯 UI 形态：仅前端扩展，
// 无 entryFile）
func resolverManifestWith(publicId, resolverField string) string {
	return `{"id":"` + publicId + `","name":"resolver 插件","version":"1.0.0","author":"tester",` +
		fmt.Sprintf(`"contractVersion":%d,`, pluginsdktransport.ContractVersion) +
		`"settings":[{"key":"toggle","type":"string","title":"开关","default":"on"}],` +
		resolverField +
		`"extensions":{` + frontendExtensionsField + `}}`
}

// validResolverScript 安装闸门集成测试用合格脚本（默认值输入输出空清单 = 全基线）
const validResolverScript = `function resolve(input) {
	return {version: 1, entries: []};
}`

// writePluginZipWithFiles 构造含额外文件的插件包（plugin.json + files 各一条）
func writePluginZipWithFiles(t *testing.T, manifest string, files map[string]string) string {
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
	for name, content := range files {
		fileEntry, err := w.Create(name)
		if err != nil {
			t.Fatalf("创建 zip 条目 %s 失败: %v", name, err)
		}
		if _, err := fileEntry.Write([]byte(content)); err != nil {
			t.Fatalf("写入 %s 失败: %v", name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("关闭 zip 失败: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("写插件包失败: %v", err)
	}
	return path
}

// TestSaveAndResetSettingTriggerParticipationEval 触发点②③接线：SaveSetting 与
// ResetSetting 落库成功后各自触发该插件的参与度求值（ResetSetting 不经 SaveSetting，
// 两处独立挂接）；落库失败的路径不触发
func TestSaveAndResetSettingTriggerParticipationEval(t *testing.T) {
	trigger := &recordingEvalTrigger{}
	settingSvc, svc := newSettingTestServiceWithTrigger(t, trigger)
	const publicId = "com.settings.trigger"
	manifest := resolverManifestWith(publicId, "") // 无 resolver 声明：触发面接线与声明无关
	plantPluginWithManifestOnDisk(t, svc, publicId, manifest)

	ctx := context.Background()
	if err := settingSvc.SaveSetting(ctx, publicId, "toggle", "off"); err != nil {
		t.Fatalf("保存设置项失败: %v", err)
	}
	if err := settingSvc.SaveSetting(ctx, publicId, "unknownKey", "x"); err == nil {
		t.Fatal("未知设置项应保存失败")
	}
	if err := settingSvc.ResetSetting(ctx, publicId, "toggle"); err != nil {
		t.Fatalf("重置设置项失败: %v", err)
	}

	calls := trigger.calls()
	if len(calls) != 2 || calls[0] != publicId || calls[1] != publicId {
		t.Errorf("落库后触发序列 = %v, 期望 [%s %s]（保存与重置各一次，失败路径不触发）", calls, publicId, publicId)
	}
}

// cleanupInstallDir 安装集成用例的落盘目录清理（安装解压发生在真实应用根目录的插件包目录）
func cleanupInstallDir(t *testing.T, publicId string) {
	t.Helper()
	t.Cleanup(func() {
		_ = os.RemoveAll(filepath.Join(util.RootPath(), PluginPackageRoot, publicId))
	})
}

// TestInstallRejectsResolverScriptMissing 安装闸门集成：清单声明 settingsResolver 而包内
// 缺脚本文件 → 拒收安装且不落库
func TestInstallRejectsResolverScriptMissing(t *testing.T) {
	svc, _ := newCleanupTestService(t)
	const publicId = "com.install.resolver.missing"
	cleanupInstallDir(t, publicId)
	manifest := resolverManifestWith(publicId, `"settingsResolver":{"script":"resolver.js","contractVersion":1},`)

	_, _, err := svc.InstallFromPath(context.Background(),
		writePluginZipWithFiles(t, manifest, nil), true)
	if err == nil {
		t.Fatal("缺 resolver 脚本的插件包应拒收安装")
	}
	if !errors.Is(err, participation.ErrResolverInvalid) {
		t.Errorf("拒收原因应携带哨兵错误 %v, 实际: %v", participation.ErrResolverInvalid, err)
	}
	if !strings.Contains(err.Error(), "缺少 resolver 脚本文件") {
		t.Errorf("拒收原因应点名缺文件, 实际: %v", err)
	}
	row, rerr := svc.repo.GetByPublicId(context.Background(), publicId)
	if rerr != nil || row != nil {
		t.Errorf("拒收后不应留下安装行: row=%v err=%v", row, rerr)
	}
}

// TestInstallRejectsBadResolverScript 安装闸门集成：脚本 dry-run 输出不合格（声明集外条目）
// → 拒收安装
func TestInstallRejectsBadResolverScript(t *testing.T) {
	svc, _ := newCleanupTestService(t)
	const publicId = "com.install.resolver.bad"
	cleanupInstallDir(t, publicId)
	manifest := resolverManifestWith(publicId, `"settingsResolver":{"script":"resolver.js","contractVersion":1},`)
	badScript := `function resolve(input) {
	return {version: 1, entries: [{point: "frontendExtensions", id: "ghost", active: false}]};
}`

	_, _, err := svc.InstallFromPath(context.Background(),
		writePluginZipWithFiles(t, manifest, map[string]string{"resolver.js": badScript}), true)
	if err == nil {
		t.Fatal("dry-run 输出声明集外条目的插件包应拒收安装")
	}
	if !errors.Is(err, participation.ErrResolverInvalid) {
		t.Errorf("拒收原因应携带哨兵错误 %v, 实际: %v", participation.ErrResolverInvalid, err)
	}
}

// TestInstallAcceptsValidResolver 安装闸门集成：声明与脚本合格 → 照常安装，脚本随包落盘
// （激活期装载读取）
func TestInstallAcceptsValidResolver(t *testing.T) {
	svc, _ := newCleanupTestService(t)
	const publicId = "com.install.resolver.ok"
	cleanupInstallDir(t, publicId)
	manifest := resolverManifestWith(publicId, `"settingsResolver":{"script":"resolver.js","contractVersion":1},`)

	plugin, _, err := svc.InstallFromPath(context.Background(),
		writePluginZipWithFiles(t, manifest, map[string]string{"resolver.js": validResolverScript}), true)
	if err != nil {
		t.Fatalf("合格 resolver 插件包应安装成功: %v", err)
	}
	scriptPath := filepath.Join(util.RootPath(), plugin.RootPath.String, "resolver.js")
	if _, err := os.Stat(scriptPath); err != nil {
		t.Errorf("resolver 脚本应随包落盘供激活装载: %v", err)
	}
}

// TestInstallWithoutResolverDeclarationUnchanged 反向控制：不带 settingsResolver 声明的
// 插件包安装行为不变（闸门零介入）
func TestInstallWithoutResolverDeclarationUnchanged(t *testing.T) {
	svc, _ := newCleanupTestService(t)
	const publicId = "com.install.resolver.absent"
	cleanupInstallDir(t, publicId)
	manifest := resolverManifestWith(publicId, "")

	if _, _, err := svc.InstallFromPath(context.Background(),
		writePluginZipWithFiles(t, manifest, nil), true); err != nil {
		t.Fatalf("不带 resolver 声明的插件包应照常安装: %v", err)
	}
}
