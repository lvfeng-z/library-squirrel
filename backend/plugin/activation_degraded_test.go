package plugin

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// 激活失败降级透传测试：注入 Activate 必失败的参与者替身，经 InstallFromPath/SetTrusted
// 走真实安装链（OpenTestDB），断言操作主体成功（err==nil、DB 行已落）且 degraded 携带降级文案；
// 反向用例（无失败参与者/未信任门控拦截）断言 degraded 为空。
// 复用 backup_cleanup_test.go 的 newCleanupTestService、plugin_upgrade_test.go 的
// writePluginZip/availableManifest、lifecycle_test.go 的 recordingParticipant 替身。

// TestInstallFromPathDegradedOnActivateFailure 已信任安装遇参与者激活失败：安装主体成功落库、
// err==nil、degraded 携带「已安装（已信任）但激活失败」与底层失败原因
func TestInstallFromPathDegradedOnActivateFailure(t *testing.T) {
	svc, _ := newCleanupTestService(t)
	ctx := context.Background()
	svc.lifecycle.registerParticipant(&recordingParticipant{
		name: "fail", activateErr: errors.New("模拟注册失败"), events: &[]string{},
	})
	const pid = "com.degraded.install"

	plugin, degraded, err := svc.InstallFromPath(ctx, writePluginZip(t, availableManifest(pid, "b1")), true)
	if err != nil {
		t.Fatalf("激活失败不应令安装整体报错: %v", err)
	}
	if degraded == "" {
		t.Fatal("已信任安装激活失败时 degraded 应携带降级文案")
	}
	if !strings.Contains(degraded, "已安装（已信任）但激活失败") || !strings.Contains(degraded, "模拟注册失败") {
		t.Fatalf("降级文案应含统一前缀与底层失败原因, 实际: %q", degraded)
	}
	if !plugin.Trusted.Valid || !plugin.Trusted.Bool {
		t.Fatalf("安装主体应按已信任落库, 实际 %+v", plugin.Trusted)
	}

	row, rerr := svc.repo.GetByPublicId(ctx, pid)
	if rerr != nil || row == nil {
		t.Fatalf("激活失败后安装行应已落库（不回滚）: row=%v err=%v", row, rerr)
	}
}

// TestInstallFromPathNoDegradedWhenActivateSucceeds 反向：参与者激活成功时 degraded 为空
func TestInstallFromPathNoDegradedWhenActivateSucceeds(t *testing.T) {
	svc, _ := newCleanupTestService(t)
	ctx := context.Background()
	svc.lifecycle.registerParticipant(&recordingParticipant{name: "ok", events: &[]string{}})

	_, degraded, err := svc.InstallFromPath(ctx, writePluginZip(t, availableManifest("com.degraded.ok", "b1")), true)
	if err != nil {
		t.Fatalf("安装失败: %v", err)
	}
	if degraded != "" {
		t.Fatalf("激活成功不应携带降级文案, 实际: %q", degraded)
	}
}

// TestSetTrustedDegradedOnActivateFailure 信任链降级：未信任安装被信任门控拦下属设计内状态
// 不产生降级；显式信任后激活失败时 err==nil、degraded 非空、信任标记已落库
func TestSetTrustedDegradedOnActivateFailure(t *testing.T) {
	svc, _ := newCleanupTestService(t)
	ctx := context.Background()
	svc.lifecycle.registerParticipant(&recordingParticipant{
		name: "fail", activateErr: errors.New("模拟注册失败"), events: &[]string{},
	})
	const pid = "com.degraded.trust"

	// 未信任安装：激活被信任门控拦截（待用户显式信任），不属降级
	_, degraded, err := svc.InstallFromPath(ctx, writePluginZip(t, availableManifest(pid, "b1")), false)
	if err != nil {
		t.Fatalf("未信任安装应成功: %v", err)
	}
	if degraded != "" {
		t.Fatalf("未信任安装被门控拦截不应产生降级文案, 实际: %q", degraded)
	}

	// 显式信任：激活失败降级透传，信任标记不回滚
	plugin, degraded, err := svc.SetTrusted(ctx, pid, true, false)
	if err != nil {
		t.Fatalf("信任后激活失败不应令信任操作报错: %v", err)
	}
	if degraded == "" || !strings.Contains(degraded, "已安装（已信任）但激活失败") {
		t.Fatalf("信任后激活失败应携带降级文案, 实际: %q", degraded)
	}
	if !plugin.Trusted.Valid || !plugin.Trusted.Bool {
		t.Fatalf("信任标记应已写入返回值, 实际 %+v", plugin.Trusted)
	}
	row, rerr := svc.repo.GetByPublicId(ctx, pid)
	if rerr != nil || row == nil || !row.Trusted.Valid || !row.Trusted.Bool {
		t.Fatalf("信任标记应已落库（不回滚）: row=%v err=%v", row, rerr)
	}
}

// TestHandlerInstallMsgCarriesDegraded handler 层透传：激活失败时响应仍成功（Success=true）且
// Msg 携带降级文案；无降级时 Msg 为默认成功文案 "success"
func TestHandlerInstallMsgCarriesDegraded(t *testing.T) {
	ctx := context.Background()

	svc, _ := newCleanupTestService(t)
	svc.lifecycle.registerParticipant(&recordingParticipant{
		name: "fail", activateErr: errors.New("模拟注册失败"), events: &[]string{},
	})
	resp := NewHandler(svc).InstallFromPath(ctx, writePluginZip(t, availableManifest("com.degraded.handler", "b1")), true)
	if !resp.Success {
		t.Fatalf("激活失败时响应仍应成功, msg=%q", resp.Msg)
	}
	if resp.Msg == "" || resp.Msg == "success" || !strings.Contains(resp.Msg, "已安装（已信任）但激活失败") {
		t.Fatalf("Msg 应携带降级文案, 实际: %q", resp.Msg)
	}

	svc2, _ := newCleanupTestService(t)
	svc2.lifecycle.registerParticipant(&recordingParticipant{name: "ok", events: &[]string{}})
	resp2 := NewHandler(svc2).InstallFromPath(ctx, writePluginZip(t, availableManifest("com.degraded.handler2", "b1")), true)
	if !resp2.Success || resp2.Msg != "success" {
		t.Fatalf("无降级时 Msg 应为默认成功文案, 实际: success=%v msg=%q", resp2.Success, resp2.Msg)
	}
}
