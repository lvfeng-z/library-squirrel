package externalapi

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/library-squirrel/backend/base/constant"
)

// newTestLifecycle 经生产构造函数构建监听器与生命周期管理器（发现文件写于临时目录）
func newTestLifecycle(t *testing.T) (*Lifecycle, string) {
	t.Helper()
	srv := newTestServer(t)
	dir := t.TempDir()
	t.Cleanup(func() {
		_ = srv.Stop()
	})
	return NewLifecycle(srv, dir), filepath.Join(dir, discoveryFileName)
}

// readDiscoveryFile 读发现文件内容
func readDiscoveryFile(t *testing.T, path string) discoveryFile {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取发现文件失败: %v", err)
	}
	var view discoveryFile
	if err := json.Unmarshal(raw, &view); err != nil {
		t.Fatalf("发现文件不是合法 JSON: %v", err)
	}
	return view
}

// grabFreePort 占一个系统分配端口后立即释放（供固定端口测试用）
func grabFreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("探测空闲端口失败: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

// TestSyncEnabledWritesDiscoveryFile 开启联动：起监听并写发现文件（enabled 恒真、实际
// 监听端口、token、应用版本），探测端点真实可达
func TestSyncEnabledWritesDiscoveryFile(t *testing.T) {
	lc, discoveryPath := newTestLifecycle(t)

	if err := lc.SyncExternalApi(true, 0, testToken); err != nil {
		t.Fatalf("开启同步失败: %v", err)
	}
	view := readDiscoveryFile(t, discoveryPath)
	if !view.Enabled || view.Token != testToken || view.Version != constant.AppVersion {
		t.Fatalf("发现文件内容不符契约，实际 %+v", view)
	}
	if view.Port <= 0 {
		t.Fatalf("发现文件端口期望实际监听端口，实际 %d", view.Port)
	}

	running, addr, lastErr := lc.ExternalApiRuntimeStatus()
	if !running || addr == "" || lastErr != "" {
		t.Fatalf("运行态期望监听中，实际 running=%v addr=%q lastErr=%q", running, addr, lastErr)
	}
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil || portStr != fmt.Sprint(view.Port) {
		t.Fatalf("发现文件端口 %d 与监听地址 %s 不一致", view.Port, addr)
	}
	// 真实监听可达性：免鉴权探测端点应答
	resp, err := http.Get("http://" + addr + healthPath)
	if err != nil {
		t.Fatalf("监听可达性验证失败: %v", err)
	}
	_ = resp.Body.Close()
}

// TestSyncDisabledDeletesDiscoveryFileAndStopsListener 关闭联动：停监听（地址不再可达）
// 并删发现文件；未启用态重复关闭为幂等空操作
func TestSyncDisabledDeletesDiscoveryFileAndStopsListener(t *testing.T) {
	lc, discoveryPath := newTestLifecycle(t)

	if err := lc.SyncExternalApi(true, 0, testToken); err != nil {
		t.Fatalf("开启同步失败: %v", err)
	}
	running, addr, _ := lc.ExternalApiRuntimeStatus()
	if !running {
		t.Fatal("预置开启后期望监听中")
	}

	if err := lc.SyncExternalApi(false, 0, testToken); err != nil {
		t.Fatalf("关闭同步失败: %v", err)
	}
	if _, err := os.Stat(discoveryPath); !os.IsNotExist(err) {
		t.Fatalf("关闭后期望发现文件已删除，实际 stat err=%v", err)
	}
	if resp, err := http.Get("http://" + addr + healthPath); err == nil {
		_ = resp.Body.Close()
		t.Fatal("关闭后期望监听不再可达")
	}
	if running, _, _ = lc.ExternalApiRuntimeStatus(); running {
		t.Fatal("关闭后期望运行态为未监听")
	}

	// 未启用态重复关闭：幂等空操作
	if err := lc.SyncExternalApi(false, 0, testToken); err != nil {
		t.Fatalf("重复关闭期望幂等空操作，实际错误: %v", err)
	}
}

// TestSyncTokenSwapRotatesAuthAndRewritesDiscoveryFile token 变更联动：监听器换钥（旧
// token 立即 401、新 token 放行）并重写发现文件
func TestSyncTokenSwapRotatesAuthAndRewritesDiscoveryFile(t *testing.T) {
	lc, discoveryPath := newTestLifecycle(t)
	if err := lc.SyncExternalApi(true, 0, testToken); err != nil {
		t.Fatalf("开启同步失败: %v", err)
	}
	_, addr, _ := lc.ExternalApiRuntimeStatus()

	const newToken = "rotated-token-456def"
	if err := lc.SyncExternalApi(true, 0, newToken); err != nil {
		t.Fatalf("token 换钥同步失败: %v", err)
	}
	if view := readDiscoveryFile(t, discoveryPath); view.Token != newToken {
		t.Fatalf("发现文件期望随 token 重写为 %q，实际 %q", newToken, view.Token)
	}
	resp, _ := postOperation(t, "http://"+addr, "Bearer "+testToken, "sample.echo", `{}`)
	if resp.StatusCode != 401 {
		t.Fatalf("旧 token 期望 401，实际 %d", resp.StatusCode)
	}
	resp, _ = postOperation(t, "http://"+addr, "Bearer "+newToken, "sample.echo", `{}`)
	if resp.StatusCode != 200 {
		t.Fatalf("新 token 期望 200，实际 %d", resp.StatusCode)
	}
}

// TestSyncSameTargetStateIdempotent 相同目标态重复同步为空操作：地址不变、不换绑
func TestSyncSameTargetStateIdempotent(t *testing.T) {
	lc, _ := newTestLifecycle(t)

	if err := lc.SyncExternalApi(true, 0, testToken); err != nil {
		t.Fatalf("首次开启失败: %v", err)
	}
	_, addr1, _ := lc.ExternalApiRuntimeStatus()
	if err := lc.SyncExternalApi(true, 0, testToken); err != nil {
		t.Fatalf("重复同步期望空操作，实际错误: %v", err)
	}
	_, addr2, _ := lc.ExternalApiRuntimeStatus()
	if addr1 != addr2 {
		t.Fatalf("重复同步期望地址不变，实际 %s → %s", addr1, addr2)
	}
}

// TestSyncPortChangeRebindsListener 固定端口变更且监听中：换绑到新端口，发现文件随新端口重写
func TestSyncPortChangeRebindsListener(t *testing.T) {
	lc, discoveryPath := newTestLifecycle(t)
	port1 := grabFreePort(t)
	port2 := grabFreePort(t)

	if err := lc.SyncExternalApi(true, port1, testToken); err != nil {
		t.Fatalf("固定端口 %d 开启失败: %v", port1, err)
	}
	if view := readDiscoveryFile(t, discoveryPath); view.Port != port1 {
		t.Fatalf("发现文件端口期望 %d，实际 %d", port1, view.Port)
	}

	if err := lc.SyncExternalApi(true, port2, testToken); err != nil {
		t.Fatalf("换绑端口 %d 失败: %v", port2, err)
	}
	running, addr, _ := lc.ExternalApiRuntimeStatus()
	if !running {
		t.Fatal("换绑后期望仍在监听")
	}
	want := fmt.Sprintf("%s:%d", loopbackHost, port2)
	if addr != want {
		t.Fatalf("换绑后期望地址 %s，实际 %s", want, addr)
	}
	if view := readDiscoveryFile(t, discoveryPath); view.Port != port2 {
		t.Fatalf("发现文件端口期望随换绑更新为 %d，实际 %d", port2, view.Port)
	}
}

// TestSyncEnabledWithEmptyTokenRefused 启用态不容忍空 token：拒绝启用（停监听、不写
// 发现文件）并返回错误
func TestSyncEnabledWithEmptyTokenRefused(t *testing.T) {
	lc, discoveryPath := newTestLifecycle(t)

	if err := lc.SyncExternalApi(true, 0, ""); err == nil {
		t.Fatal("空 token 启用期望返回错误")
	}
	if _, err := os.Stat(discoveryPath); !os.IsNotExist(err) {
		t.Fatal("空 token 拒绝启用时发现文件不应存在")
	}
	if running, _, lastErr := lc.ExternalApiRuntimeStatus(); running || lastErr == "" {
		t.Fatalf("空 token 拒绝后期望未监听且记录失败原因，实际 running=%v lastErr=%q", running, lastErr)
	}
}
