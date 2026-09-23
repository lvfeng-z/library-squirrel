package extension

import (
	"database/sql"
	"testing"

	dto "github.com/library-squirrel/backend/base/model/dto"
	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/pluginTaskUrlListener"
)

// urlListenerFixture 组装 URL 监听派生索引与 Loader 清理链的最小装置：
// cleaner 接线与 app.go 装配一致（整插件注销其监听条目）
func urlListenerFixture() (*Loader, *pluginTaskUrlListener.Service) {
	loader := NewLoader(NewWorkFetchRegistry(), NewSiteBrowserRegistry())
	svc := pluginTaskUrlListener.NewService(pluginTaskUrlListener.NewManager())
	loader.SetUrlListenerCleaner(func(pluginPublicId string) {
		svc.Unregister(pluginPublicId, "")
	})
	return loader, svc
}

// registerUrlListenerEntry 按生产输入形态（清单作品拉取条目声明）登记一个监听条目
func registerUrlListenerEntry(t *testing.T, svc *pluginTaskUrlListener.Service, publicId string) {
	t.Helper()
	plugin := domain.NewPlugin()
	plugin.PublicID = sql.NullString{String: publicId, Valid: true}
	plugin.Name = sql.NullString{String: "插件" + publicId, Valid: true}
	svc.RegisterDeclared(plugin, []dto.WorkFetchDeclaration{
		{ID: "main", Name: "主处理器", UrlPatterns: []string{`^https://x\.com/`}},
	})
}

// TestUnloadPluginClearsDerivedUrlListenerIndex 停用/崩溃清理等价：UnloadPlugin（显式停用与
// 崩溃路径共用的清理入口）后派生索引该插件条目全清，其他插件条目不受影响
func TestUnloadPluginClearsDerivedUrlListenerIndex(t *testing.T) {
	loader, svc := urlListenerFixture()
	registerUrlListenerEntry(t, svc, "com.example.a")
	registerUrlListenerEntry(t, svc, "com.example.b")

	if got := svc.ListListener("https://x.com/1"); len(got) != 2 {
		t.Fatalf("登记后两插件条目应命中, 实际 %d 个", len(got))
	}

	if err := loader.UnloadPlugin("com.example.a"); err != nil {
		t.Fatalf("卸载失败: %v", err)
	}

	got := svc.ListListener("https://x.com/1")
	if len(got) != 1 || got[0].PublicID.String != "com.example.b" {
		t.Fatalf("卸载后应只剩插件B条目, 实际 %+v", got)
	}
}

// TestCrashDropsDerivedUrlListenerIndex 插件崩溃后派生索引随进程表条目消失而清理。
// 崩溃链：GetServices 检测进程退出 → handlePluginCrash 摘进程表条目并回调 crashNotifier →
// 产线经 NotifyPluginCrashed 按参与者逆序 OnStopped 收敛到 UnloadPlugin（本测试直连等价模拟）
func TestCrashDropsDerivedUrlListenerIndex(t *testing.T) {
	loader, svc := urlListenerFixture()
	loader.SetCrashNotifier(func(pluginPublicId string) {
		if err := loader.UnloadPlugin(pluginPublicId); err != nil {
			t.Errorf("崩溃清理卸载失败: %v", err)
		}
	})

	// 植入进程表条目：handlePluginCrash 只摘条目与通知，不触碰子进程句柄，裸条目即可
	loader.mu.Lock()
	loader.processes["com.example.a"] = &pluginEntry{}
	loader.mu.Unlock()

	registerUrlListenerEntry(t, svc, "com.example.a")
	if got := svc.ListListener("https://x.com/1"); len(got) != 1 {
		t.Fatalf("崩溃前派生索引条目应命中, 实际 %d 个", len(got))
	}

	loader.handlePluginCrash("com.example.a")

	if _, ok := loader.GetServices("com.example.a"); ok {
		t.Fatal("崩溃后进程表条目应已摘除")
	}
	if got := svc.ListListener("https://x.com/1"); got != nil {
		t.Fatalf("崩溃清理后派生索引应已清空, 实际 %+v", got)
	}
}
