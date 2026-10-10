package plugin

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/util"
)

// plantSettingsPagePluginOnDisk 预置带 settingsPage 声明的插件行与安装目录清单（复用清单校验
// 测试的清单构造器），并登记安装目录清理
func plantSettingsPagePluginOnDisk(t *testing.T, svc *Service, publicId string) *entity.Plugin {
	t.Helper()
	row := plantPluginWithManifestOnDisk(t, svc, publicId, settingsPageManifest(publicId, "v1"))
	t.Cleanup(func() {
		_ = os.RemoveAll(filepath.Join(util.RootPath(), PluginPackageRoot, publicId))
	})
	return row
}

// TestGetPluginStatusReturnsSettingsPageExtensionId settingsPageExtensionId 为清单静态声明面：
// 已激活与未激活插件均返回清单声明的条目 id（与激活态无关）；旧清单无字段返回空串
func TestGetPluginStatusReturnsSettingsPageExtensionId(t *testing.T) {
	svc, _ := newCleanupTestService(t)
	ctx := context.Background()

	// 已激活插件
	const activeId = "com.status.settingspage.active"
	activeRow := plantSettingsPagePluginOnDisk(t, svc, activeId)
	if err := svc.ActivatePlugin(ctx, activeRow); err != nil {
		t.Fatalf("激活插件失败: %v", err)
	}
	status, err := svc.GetPluginStatus(ctx, activeId)
	if err != nil {
		t.Fatalf("获取插件状态失败: %v", err)
	}
	if status.LifecycleState != lifecycleStateActive {
		t.Errorf("已激活插件生命周期态应为 active, 实际: %s", status.LifecycleState)
	}
	if status.SettingsPageExtensionId != "v1" {
		t.Errorf("已激活插件应返回清单声明的 settingsPageExtensionId, 实际: %q", status.SettingsPageExtensionId)
	}

	// 未激活插件：静态面与激活态无关，同样返回（未激活时前端扩展注册表为空，判据只能取清单）
	const inactiveId = "com.status.settingspage.inactive"
	plantSettingsPagePluginOnDisk(t, svc, inactiveId)
	status, err = svc.GetPluginStatus(ctx, inactiveId)
	if err != nil {
		t.Fatalf("获取插件状态失败: %v", err)
	}
	if status.LifecycleState != lifecycleStateInactive {
		t.Errorf("未激活插件生命周期态应为 inactive, 实际: %s", status.LifecycleState)
	}
	if status.SettingsPageExtensionId != "v1" {
		t.Errorf("未激活插件同样应返回 settingsPageExtensionId（静态面）, 实际: %q", status.SettingsPageExtensionId)
	}

	// 旧清单（无 settingsPage 字段）：零值兼容返回空串
	const legacyId = "com.status.settingspage.legacy"
	plantPluginWithManifestOnDisk(t, svc, legacyId, settingsPageManifest(legacyId, ""))
	t.Cleanup(func() {
		_ = os.RemoveAll(filepath.Join(util.RootPath(), PluginPackageRoot, legacyId))
	})
	status, err = svc.GetPluginStatus(ctx, legacyId)
	if err != nil {
		t.Fatalf("获取插件状态失败: %v", err)
	}
	if status.SettingsPageExtensionId != "" {
		t.Errorf("旧清单无 settingsPage 字段应返回空串, 实际: %q", status.SettingsPageExtensionId)
	}
}

// TestHandlerActivatePluginTrustGateAndIdempotency 设置入口激活通路：未信任插件被信任门控拒绝
// 且失败信封携带可读原因；已运行插件重复激活幂等成功（不报错不回状态）
func TestHandlerActivatePluginTrustGateAndIdempotency(t *testing.T) {
	svc, _ := newCleanupTestService(t)
	h := NewHandler(svc)
	ctx := context.Background()

	// 未信任插件：激活被拒，失败信封 Msg 含未信任语义（可读原因，前端透传展示）
	const untrustedId = "com.activate.handler.untrusted"
	untrustedRow := plantSettingsPagePluginOnDisk(t, svc, untrustedId)
	untrustedRow.Trusted = sql.NullBool{Bool: false, Valid: true}
	if err := svc.repo.Save(ctx, untrustedRow); err != nil {
		t.Fatalf("改写信任标记失败: %v", err)
	}
	resp := h.ActivatePlugin(ctx, untrustedId)
	if resp.Success {
		t.Fatal("未信任插件的激活应被信任门控拒绝")
	}
	if !strings.Contains(resp.Msg, "未信任") {
		t.Errorf("失败原因应含未信任语义, 实际: %q", resp.Msg)
	}
	if state, _ := svc.lifecycle.lifecycleStatusOf(untrustedId); state != lifecycleStateInactive {
		t.Errorf("被拒后插件应保持未激活, 实际: %s", state)
	}

	// 已运行插件：handler 重复激活幂等成功
	const runningId = "com.activate.handler.idempotent"
	runningRow := plantSettingsPagePluginOnDisk(t, svc, runningId)
	if err := svc.ActivatePlugin(ctx, runningRow); err != nil {
		t.Fatalf("预激活插件失败: %v", err)
	}
	resp = h.ActivatePlugin(ctx, runningId)
	if !resp.Success {
		t.Errorf("已运行插件重复激活应幂等成功, 实际失败: %q", resp.Msg)
	}
	if state, _ := svc.lifecycle.lifecycleStatusOf(runningId); state != lifecycleStateActive {
		t.Errorf("重复激活后插件应保持运行中, 实际: %s", state)
	}
}
