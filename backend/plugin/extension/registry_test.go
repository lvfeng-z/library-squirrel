package extension

import (
	"strings"
	"testing"

	domain "github.com/library-squirrel/backend/base"
	"github.com/library-squirrel/backend/base/model"
)

// mockPusher 捕获推送调用的契约测试替身
type mockPusher struct {
	registered       []FrontendExtensionResponse
	unregistered     []FrontendExtensionUnregisterItem
	batchUnregistered [][]FrontendExtensionUnregisterItem
}

func (m *mockPusher) PushRegister(data FrontendExtensionResponse) {
	m.registered = append(m.registered, data)
}

func (m *mockPusher) PushUnregister(item FrontendExtensionUnregisterItem) {
	m.unregistered = append(m.unregistered, item)
}

func (m *mockPusher) PushBatchUnregister(items []FrontendExtensionUnregisterItem) {
	m.batchUnregistered = append(m.batchUnregistered, items)
}

// newTestExtension 构造带元数据的前端扩展（镜像 app.go 声明式注册路径）
func newTestExtension(publicId, extensionId string, kind domain.FrontendExtensionKind) *model.Extension[*domain.FrontendExtensionConfig] {
	cfg := domain.NewFrontendExtensionConfig()
	cfg.Metadata.ID = extensionId
	cfg.Metadata.PluginPublicID = publicId
	cfg.Metadata.Name = "测试扩展"
	cfg.Kind = kind
	return model.NewExtension(*cfg.Metadata, cfg)
}

// TestUnregisterPayloadContract 验证注销事件契约：frontendExtensionId 为 manifest 声明的裸
// extensionId、pluginPublicId 分列——与注册事件 Response.ID 同语义，前端据此派生复合 store 键
func TestUnregisterPayloadContract(t *testing.T) {
	pusher := &mockPusher{}
	r := NewFrontendExtensionRegistry()
	r.SetPusher(pusher)

	const publicId = "com.example.plugin"
	if err := r.Register(newTestExtension(publicId, "article-viewer", domain.FrontendExtensionKindResourceViewer)); err != nil {
		t.Fatalf("注册失败: %v", err)
	}

	// 注册事件 payload：Response.ID 为裸 extensionId
	if len(pusher.registered) != 1 || pusher.registered[0].ID != "article-viewer" || pusher.registered[0].PluginPublicID != publicId {
		t.Fatalf("注册事件 payload 不符: %+v", pusher.registered)
	}

	// 单条注销：frontendExtensionId 必须是裸 extensionId（不得是复合键 publicId/extensionId）
	if err := r.Unregister(publicId, "article-viewer"); err != nil {
		t.Fatalf("注销失败: %v", err)
	}
	if len(pusher.unregistered) != 1 {
		t.Fatal("未收到注销事件")
	}
	item := pusher.unregistered[0]
	if item.ID != "article-viewer" {
		t.Fatalf("注销事件 frontendExtensionId 应为裸 extensionId, 实际: %s", item.ID)
	}
	if item.PluginPublicID != publicId {
		t.Fatalf("注销事件 pluginPublicId 分列不符: %s", item.PluginPublicID)
	}
	if item.Kind != string(domain.FrontendExtensionKindResourceViewer) {
		t.Fatalf("注销事件 kind 不符: %s", item.Kind)
	}
}

// TestBatchUnregisterPayloadContract 验证批量注销（插件整体卸载路径）同样携带裸 extensionId
func TestBatchUnregisterPayloadContract(t *testing.T) {
	pusher := &mockPusher{}
	r := NewFrontendExtensionRegistry()
	r.SetPusher(pusher)

	const publicId = "com.example.plugin"
	for _, id := range []string{"menu-main", "view-browser"} {
		if err := r.Register(newTestExtension(publicId, id, domain.FrontendExtensionKindView)); err != nil {
			t.Fatalf("注册 %s 失败: %v", id, err)
		}
	}

	if err := r.UnregisterAll(publicId); err != nil {
		t.Fatalf("批量注销失败: %v", err)
	}
	if len(pusher.batchUnregistered) != 1 || len(pusher.batchUnregistered[0]) != 2 {
		t.Fatalf("批量注销事件数不符: %+v", pusher.batchUnregistered)
	}
	for _, item := range pusher.batchUnregistered[0] {
		if item.PluginPublicID != publicId {
			t.Fatalf("批量注销项 pluginPublicId 不符: %+v", item)
		}
		// 裸 extensionId 不含分隔符——复合键形态即契约回退
		if item.ID == "" || strings.Contains(item.ID, "/") {
			t.Fatalf("批量注销项 frontendExtensionId 应为裸 extensionId, 实际: %s", item.ID)
		}
	}
}
