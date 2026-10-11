package extension

import (
	"encoding/json"
	"strings"
	"testing"

	domain "github.com/library-squirrel/backend/base"
)

// TestRegisterEventPayloadCarriesViewParent 注册事件载荷透传 view 条目 parent：同插件指向
// 与宿主路由名两形态的声明值原样下行；未声明（空值）时载荷省略该键（旧清单下行零差异，
// 前端按缺省一级页面处理）
func TestRegisterEventPayloadCarriesViewParent(t *testing.T) {
	pusher := &mockPusher{}
	r := NewFrontendExtensionRegistry()
	r.SetPusher(pusher)

	const publicId = "com.example.viewparent"
	samePluginSub := newTestExtension(publicId, "sub-view", domain.FrontendExtensionKindView)
	samePluginSub.Instance.Parent = "main-view"
	if err := r.Register(samePluginSub); err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	hostRouteSub := newTestExtension(publicId, "host-sub-view", domain.FrontendExtensionKindView)
	hostRouteSub.Instance.Parent = "pluginManage"
	if err := r.Register(hostRouteSub); err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	topLevel := newTestExtension(publicId, "top-view", domain.FrontendExtensionKindView)
	if err := r.Register(topLevel); err != nil {
		t.Fatalf("注册失败: %v", err)
	}

	if len(pusher.registered) != 3 {
		t.Fatalf("注册事件数不符: %d", len(pusher.registered))
	}
	if got := pusher.registered[0].Parent; got != "main-view" {
		t.Errorf("注册事件 payload.parent 应透传同插件指向值, 实际: %q", got)
	}
	if got := pusher.registered[1].Parent; got != "pluginManage" {
		t.Errorf("注册事件 payload.parent 应透传宿主路由名值, 实际: %q", got)
	}
	raw, err := json.Marshal(pusher.registered[2])
	if err != nil {
		t.Fatalf("序列化响应失败: %v", err)
	}
	if strings.Contains(string(raw), `"parent"`) {
		t.Errorf("未声明 parent 的条目载荷应省略该键, 实际: %s", raw)
	}
}
