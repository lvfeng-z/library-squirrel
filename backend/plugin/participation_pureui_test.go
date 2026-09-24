package plugin

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/library-squirrel/backend/base/logger"
	dto "github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/plugin/participation"
	"github.com/library-squirrel/backend/plugin/settingresolver"
	"github.com/library-squirrel/backend/util"
	"go.uber.org/zap"
)

// 纯 UI（无 entryFile）插件的激活+求值零进程路径集成验证：声明集数据源 = 清单（激活相位
// 读盘，不经 loader 进程表），设置落库触发的求值与覆盖表更新全程不依赖任何子进程设施。

// pureUIResolverScript 纯 UI 测试插件的 resolver 脚本：enableParticipation=false 停用前端
// 扩展 v1（带理由），其余取值全参与（快照语义：未列出条目 = 基线参与）
const pureUIResolverScript = `function resolve(input) {
	var s = input && input.settings ? input.settings : {};
	if (String(s.enableParticipation) !== "false") {
		return {version: 1, entries: []};
	}
	return {version: 1, entries: [
		{point: "frontendExtensions", id: "v1", active: false, reason: "设置已关闭参与"}
	]};
}`

// pureUIResolverManifest 纯 UI 插件清单（无 entryFile）：仅前端扩展声明 + 根级 settings +
// settingsResolver 声明
func pureUIResolverManifest(publicId string) string {
	return `{"id":"` + publicId + `","name":"纯 UI resolver 插件","version":"1.0.0","author":"tester",` +
		`"settings":[{"key":"enableParticipation","type":"boolean","title":"参与","default":"true"}],` +
		`"settingsResolver":{"script":"resolver.js","contractVersion":1},` +
		`"extensions":{"frontendExtensions":[{"id":"v1","name":"菜单入口","kind":"menu","content":{"contentType":"code","source":"1"}}]}}`
}

// pureUIParticipant participation.Manager 的生命周期参与者适配（与 app.go 的
// participationParticipant 同构：激活相位末尾登记会话并同步首评，停用清表）
type pureUIParticipant struct {
	mgr *participation.Manager
}

func (p *pureUIParticipant) Activate(ctx context.Context, plugin *entity.Plugin, manifest *dto.PluginManifest) error {
	p.mgr.StartSession(ctx, plugin, manifest)
	return nil
}

func (p *pureUIParticipant) PrepareStop(_ context.Context, _ string, _ PluginStopOp, _ bool) error {
	return nil
}

func (p *pureUIParticipant) OnStopped(_ context.Context, pluginPublicId string) {
	p.mgr.StopSession(pluginPublicId)
}

// waitEntryActive 轮询等待条目有效参与态达到期望值（落库触发的求值异步进行；上限 10s）
func waitEntryActive(t *testing.T, mgr *participation.Manager, publicId string, want bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if active, found := mgr.EntryActive(publicId, settingresolver.PointFrontendExtensions, "v1"); found && active == want {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("等待插件 %s 条目 v1 有效参与态变为 %v 超时", publicId, want)
}

// TestPureUIPluginActivationEvaluatesParticipation 无 entryFile 插件走激活+求值：
// 激活相位以清单为声明集数据源登记会话并同步首评（KV 关闭态 → 条目停用）；设置落库
// 触发的后续求值热生效（零子进程）；停用清空会话
func TestPureUIPluginActivationEvaluatesParticipation(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	if logger.Log == nil {
		logger.Log = zap.NewNop().Sugar()
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	if err := db.AutoMigrate(&entity.Plugin{}); err != nil {
		t.Fatalf("迁移测试实体失败: %v", err)
	}

	const publicId = "com.pureui.resolver"
	repo := NewRepository(db)
	svc := NewService(repo, nil)
	cleanupInstallDir(t, publicId)

	// 参与度真相层 + 真实存储服务（SettingsReader 面）+ 设置服务（落库触发接线）
	storageSvc := NewPluginStorageService(newMockStorageRepo())
	mgr := participation.NewManager(storageSvc, util.RootPath())
	svc.RegisterLifecycleParticipant(&pureUIParticipant{mgr: mgr})
	settingSvc := NewPluginSettingService(repo, storageSvc, util.RootPath(), mgr)

	// 纯 UI 插件落盘（无 entryFile → EntryPath 无效）+ resolver 脚本
	row := plantPluginWithManifestOnDisk(t, svc, publicId, pureUIResolverManifest(publicId))
	if row.EntryPath.Valid && row.EntryPath.String != "" {
		t.Fatalf("前置失败：测试插件应为纯 UI 形态（无 entryFile），实际 EntryPath=%q", row.EntryPath.String)
	}
	absRoot := filepath.Join(util.RootPath(), row.RootPath.String)
	if err := os.WriteFile(filepath.Join(absRoot, "resolver.js"), []byte(pureUIResolverScript), 0o644); err != nil {
		t.Fatalf("写 resolver 脚本失败: %v", err)
	}

	ctx := context.Background()
	// 激活前落库关闭态设置（此时无会话，触发为空操作——下次激活由持久 KV 重算）
	if err := settingSvc.SaveSetting(ctx, publicId, "enableParticipation", "false"); err != nil {
		t.Fatalf("保存设置项失败: %v", err)
	}
	if _, found := mgr.EntryActive(publicId, settingresolver.PointFrontendExtensions, "v1"); found {
		t.Fatal("前置失败：未激活插件不应有参与度会话")
	}

	// 激活（无任何子进程设施参与）：激活完成时首评已就位，关闭态设置生效
	if err := svc.ActivatePlugin(ctx, row); err != nil {
		t.Fatalf("纯 UI 插件应激活成功: %v", err)
	}
	active, found := mgr.EntryActive(publicId, settingresolver.PointFrontendExtensions, "v1")
	if !found || active {
		t.Errorf("激活后 v1 有效参与态 = (%v, %v), 期望 (false, true)——首评由 KV 关闭态重算", active, found)
	}
	entries := mgr.Entries(publicId)
	if len(entries) != 1 || entries[0].Reason != "设置已关闭参与" {
		t.Errorf("激活后条目态 = %+v, 期望单条 v1 停用带理由", entries)
	}
	status := mgr.StatusOf(publicId)
	if status == nil || !status.HasResolver || status.LastFailure != "" {
		t.Errorf("激活后状态面 = %+v, 期望 HasResolver 且首评成功", status)
	}

	// 热路径：设置重开 → 落库触发异步求值 → 条目恢复参与（全程零子进程）
	if err := settingSvc.SaveSetting(ctx, publicId, "enableParticipation", "true"); err != nil {
		t.Fatalf("重开设置项失败: %v", err)
	}
	waitEntryActive(t, mgr, publicId, true)

	// 停用：覆盖表随生命周期参与者清理（停用逆序最先清空）
	if err := svc.lifecycle.deactivate(ctx, publicId, PluginStopOpUntrust, false); err != nil {
		t.Fatalf("停用插件失败: %v", err)
	}
	if entries := mgr.Entries(publicId); entries != nil {
		t.Errorf("停用后条目清单 = %+v, 期望 nil（会话已清）", entries)
	}
	if status := mgr.StatusOf(publicId); status != nil {
		t.Errorf("停用后状态面 = %+v, 期望 nil", status)
	}
}
