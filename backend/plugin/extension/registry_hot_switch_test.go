package extension

import (
	"errors"
	"testing"

	domain "github.com/library-squirrel/backend/base"
)

// TestEntryLevelHotSwitchCycle 条目级热切换闭环（设置驱动参与度翻转的后端链路）：
// 注册 → 单条注销 → 重新注册，每次操作各自下发一条单条事件，重注册不因同键历史残留
// 被拒——重新启用即重新下发，前端最近一次注册接管呈现
func TestEntryLevelHotSwitchCycle(t *testing.T) {
	pusher := &mockPusher{}
	r := NewFrontendExtensionRegistry()
	r.SetPusher(pusher)

	const publicId = "com.example.plugin"
	const entryId = "menu-entry"
	register := func() {
		if err := r.Register(newTestExtension(publicId, entryId, domain.FrontendExtensionKindMenu)); err != nil {
			t.Fatalf("注册失败: %v", err)
		}
	}

	// 首次注册：单条注册事件
	register()
	if len(pusher.registered) != 1 || pusher.registered[0].ID != entryId {
		t.Fatalf("首次注册事件不符: %+v", pusher.registered)
	}

	// 停用方向：单条注销事件（携带 kind 供前端分桶卸载）
	if err := r.Unregister(publicId, entryId); err != nil {
		t.Fatalf("注销失败: %v", err)
	}
	if len(pusher.unregistered) != 1 || pusher.unregistered[0].ID != entryId ||
		pusher.unregistered[0].Kind != string(domain.FrontendExtensionKindMenu) {
		t.Fatalf("注销事件不符: %+v", pusher.unregistered)
	}
	if _, err := r.Get(publicId, entryId); !errors.Is(err, ErrExtensionNotFound) {
		t.Fatalf("注销后查询应报不存在, 实际: %v", err)
	}

	// 参与方向重注册：同键历史已随注销摘除，注册成功且再次下发注册事件
	register()
	if len(pusher.registered) != 2 || pusher.registered[1].ID != entryId {
		t.Fatalf("重注册事件不符: %+v", pusher.registered)
	}
	if _, err := r.Get(publicId, entryId); err != nil {
		t.Fatalf("重注册后查询失败: %v", err)
	}
}

// TestRegisterDuplicateRejected 同键在场时重复注册被拒且不产生第二条事件——
// 热切换重注册只应在条目已注销后发生，重复注册属上游时序异常
func TestRegisterDuplicateRejected(t *testing.T) {
	pusher := &mockPusher{}
	r := NewFrontendExtensionRegistry()
	r.SetPusher(pusher)

	const publicId = "com.example.plugin"
	if err := r.Register(newTestExtension(publicId, "entry", domain.FrontendExtensionKindView)); err != nil {
		t.Fatalf("首次注册失败: %v", err)
	}
	if err := r.Register(newTestExtension(publicId, "entry", domain.FrontendExtensionKindView)); !errors.Is(err, ErrExtensionAlreadyExists) {
		t.Fatalf("重复注册应报 ErrExtensionAlreadyExists, 实际: %v", err)
	}
	if len(pusher.registered) != 1 {
		t.Fatalf("重复注册不得追加事件: %+v", pusher.registered)
	}
}

// TestUnregisterUnknownEntrySkipped 条目不在场时注销报不存在且不产生事件——
// 激活期解析失败的条目从未注册，参与度停用回调对其注销属幂等空操作（仅记日志）
func TestUnregisterUnknownEntrySkipped(t *testing.T) {
	pusher := &mockPusher{}
	r := NewFrontendExtensionRegistry()
	r.SetPusher(pusher)

	if err := r.Unregister("com.example.plugin", "never-registered"); !errors.Is(err, ErrExtensionNotFound) {
		t.Fatalf("未知条目注销应报 ErrExtensionNotFound, 实际: %v", err)
	}
	if len(pusher.unregistered) != 0 {
		t.Fatalf("未知条目注销不得产生事件: %+v", pusher.unregistered)
	}
}

// TestUnregisterKeepsSiblingEntries 同插件多条目下单条注销只摘目标条目，
// 其余条目保持注册（跨插件键与同插件兄弟条目均不受影响）
func TestUnregisterKeepsSiblingEntries(t *testing.T) {
	pusher := &mockPusher{}
	r := NewFrontendExtensionRegistry()
	r.SetPusher(pusher)

	const publicId = "com.example.plugin"
	for _, id := range []string{"menu-main", "view-browser", "dialog-quick"} {
		if err := r.Register(newTestExtension(publicId, id, domain.FrontendExtensionKindView)); err != nil {
			t.Fatalf("注册 %s 失败: %v", id, err)
		}
	}
	if err := r.Register(newTestExtension("com.example.other", "view-browser", domain.FrontendExtensionKindView)); err != nil {
		t.Fatalf("注册他插件同裸 id 失败: %v", err)
	}

	if err := r.Unregister(publicId, "view-browser"); err != nil {
		t.Fatalf("注销失败: %v", err)
	}
	for _, id := range []string{"menu-main", "dialog-quick"} {
		if _, err := r.Get(publicId, id); err != nil {
			t.Fatalf("兄弟条目 %s 不应被株连: %v", id, err)
		}
	}
	// 他插件同裸 extensionId 条目不受影响（注册中心键为复合键）
	if _, err := r.Get("com.example.other", "view-browser"); err != nil {
		t.Fatalf("他插件同裸 id 条目不应被株连: %v", err)
	}

	// 后续整体注销仅剩两条（view-browser 已不在批量事件中）
	if err := r.UnregisterAll(publicId); err != nil {
		t.Fatalf("批量注销失败: %v", err)
	}
	if len(pusher.batchUnregistered) != 1 || len(pusher.batchUnregistered[0]) != 2 {
		t.Fatalf("批量注销项数不符: %+v", pusher.batchUnregistered)
	}
}
