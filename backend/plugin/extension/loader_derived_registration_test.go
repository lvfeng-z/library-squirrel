package extension

import (
	"errors"
	"testing"

	"github.com/library-squirrel/backend/base/model"
)

// activatedPluginInfo 按生产路径（json 解析 → ApplyManifestDeclarations）装载全能力包清单的插件信息，
// 派生注册以其 WorkFetch/SiteBrowsers 声明为输入
func activatedPluginInfo(t *testing.T) *PluginInfo {
	t.Helper()
	info := &PluginInfo{ID: 7, PublicID: "com.example.plugin_a"}
	ApplyManifestDeclarations(info, parseManifest(t, manifestWithAllPackages))
	return info
}

// TestRegisterDeclaredExtensionsBuildsProxies 激活期按清单条目建代理注册进两注册表：
// 键 = 插件公开 ID + 条目 id，元数据 name/description 取清单条目，实例为无状态代理
func TestRegisterDeclaredExtensionsBuildsProxies(t *testing.T) {
	loader := NewLoader(NewWorkFetchRegistry(), NewSiteBrowserRegistry())
	info := activatedPluginInfo(t)

	if err := loader.registerDeclaredExtensions(info); err != nil {
		t.Fatalf("派生注册失败: %v", err)
	}

	handlerExt, err := loader.workFetchRegistry.Get(info.PublicID, "main")
	if err != nil {
		t.Fatalf("作品拉取条目未注册: %v", err)
	}
	if handlerExt.Metadata.Type != model.ExtensionTypeWorkFetch ||
		handlerExt.Metadata.ID != "main" ||
		handlerExt.Metadata.PluginID != info.ID ||
		handlerExt.Metadata.PluginPublicID != info.PublicID ||
		handlerExt.Metadata.Name != "主处理器" {
		t.Errorf("作品拉取元数据 = %+v, 期望 name 取清单、归属字段取插件信息", handlerExt.Metadata)
	}
	if _, ok := handlerExt.Instance.(*WorkFetchProxy); !ok {
		t.Errorf("作品拉取实例应为无状态代理, 实际 %T", handlerExt.Instance)
	}

	browserExt, err := loader.siteBrowserRegistry.Get(info.PublicID, "main")
	if err != nil {
		t.Fatalf("站点浏览器条目未注册: %v", err)
	}
	if browserExt.Metadata.Type != model.ExtensionTypeSiteBrowser ||
		browserExt.Metadata.ID != "main" ||
		browserExt.Metadata.PluginID != info.ID ||
		browserExt.Metadata.PluginPublicID != info.PublicID ||
		browserExt.Metadata.Name != "站点浏览器" ||
		browserExt.Metadata.Description != "条目描述" {
		t.Errorf("站点浏览器元数据 = %+v, 期望 name/description 取清单、归属字段取插件信息", browserExt.Metadata)
	}
	if _, ok := browserExt.Instance.(*SiteBrowserProxy); !ok {
		t.Errorf("站点浏览器实例应为无状态代理, 实际 %T", browserExt.Instance)
	}
}

// TestRegisterDeclaredExtensionsDuplicateKeyFails 同键条目已存在（上次激活异常退出残留）时
// 再次派生注册整体失败，错误含 ErrExtensionAlreadyExists
func TestRegisterDeclaredExtensionsDuplicateKeyFails(t *testing.T) {
	loader := NewLoader(NewWorkFetchRegistry(), NewSiteBrowserRegistry())
	info := activatedPluginInfo(t)

	if err := loader.registerDeclaredExtensions(info); err != nil {
		t.Fatalf("首次派生注册失败: %v", err)
	}
	if err := loader.registerDeclaredExtensions(info); !errors.Is(err, ErrExtensionAlreadyExists) {
		t.Fatalf("同键残留时再注册应报 ErrExtensionAlreadyExists, 实际 %v", err)
	}
}

// TestUnloadPluginClearsDerivedEntries 停用/崩溃清理等价：UnloadPlugin（显式停用与崩溃路径
// 共用的清理入口）后两注册表该插件条目全清
func TestUnloadPluginClearsDerivedEntries(t *testing.T) {
	loader := NewLoader(NewWorkFetchRegistry(), NewSiteBrowserRegistry())
	info := activatedPluginInfo(t)
	if err := loader.registerDeclaredExtensions(info); err != nil {
		t.Fatalf("派生注册失败: %v", err)
	}

	if err := loader.UnloadPlugin(info.PublicID); err != nil {
		t.Fatalf("卸载失败: %v", err)
	}

	if _, err := loader.workFetchRegistry.Get(info.PublicID, "main"); !errors.Is(err, ErrExtensionNotFound) {
		t.Errorf("停用后作品拉取条目应已注销, 实际 %v", err)
	}
	if _, err := loader.siteBrowserRegistry.Get(info.PublicID, "main"); !errors.Is(err, ErrExtensionNotFound) {
		t.Errorf("停用后站点浏览器条目应已注销, 实际 %v", err)
	}
}
