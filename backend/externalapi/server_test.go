package externalapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/library-squirrel/backend/base/logger"
)

// TestMain 测试环境挂 nop logger（生产 logger 由应用启动时 Init，测试中为 nil）
func TestMain(m *testing.M) {
	if logger.Log == nil {
		logger.Log = zap.NewNop().Sugar()
	}
	os.Exit(m.Run())
}

// testToken 测试用固定 token
const testToken = "test-token-0123456789abcdef"

// newTestServer 经生产构造函数构建带样例操作的服务
func newTestServer(t *testing.T) *Server {
	t.Helper()
	registry := NewRegistry()
	if err := RegisterSampleOperation(registry); err != nil {
		t.Fatalf("注册样例操作失败: %v", err)
	}
	srv, err := NewServer(registry, testToken)
	if err != nil {
		t.Fatalf("构建服务失败: %v", err)
	}
	return srv
}

// envelopeView 信封断言视图（与前端 ApiResponse 同构：success/msg/data 三字段）
type envelopeView struct {
	Success bool            `json:"success"`
	Msg     string          `json:"msg"`
	Data    json.RawMessage `json:"data"`
}

// postOperation 以指定鉴权头 POST 调用统一操作端点，返回原始响应与解码后的信封视图
func postOperation(t *testing.T, base, authHeader, operationID, body string) (*http.Response, envelopeView) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, base+"/api/op/"+operationID, strings.NewReader(body))
	if err != nil {
		t.Fatalf("构造请求失败: %v", err)
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("读取响应失败: %v", err)
	}
	var view envelopeView
	if err := json.Unmarshal(raw, &view); err != nil {
		t.Fatalf("响应不是合法 JSON 信封 %q: %v", string(raw), err)
	}
	return resp, view
}

// TestOperationRejectsMissingOrFakeToken 鉴权断言：无 token 与伪 token 均 401
func TestOperationRejectsMissingOrFakeToken(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.buildHandler())
	defer ts.Close()

	resp, _ := postOperation(t, ts.URL, "", "sample.echo", `{"message":"hi"}`)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("无 token 期望 401，实际 %d", resp.StatusCode)
	}

	resp, _ = postOperation(t, ts.URL, "Bearer fake-token", "sample.echo", `{"message":"hi"}`)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("伪 token 期望 401，实际 %d", resp.StatusCode)
	}
}

// TestSampleOperationEnvelope 合法 token 命中样例操作：200 + 同构信封
// （success=true、msg=success、data 回显入参与服务器时间）
func TestSampleOperationEnvelope(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.buildHandler())
	defer ts.Close()

	resp, view := postOperation(t, ts.URL, "Bearer "+testToken, "sample.echo", `{"message":"你好"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("合法 token 期望 200，实际 %d", resp.StatusCode)
	}
	if !view.Success {
		t.Fatalf("样例操作期望成功信封，实际失败: %s", view.Msg)
	}
	if view.Msg != "success" {
		t.Fatalf("成功信封 msg 期望 success，实际 %q", view.Msg)
	}
	var data struct {
		Echo       string `json:"echo"`
		ServerTime string `json:"serverTime"`
	}
	if err := json.Unmarshal(view.Data, &data); err != nil {
		t.Fatalf("data 载荷解码失败 %q: %v", string(view.Data), err)
	}
	if data.Echo != "你好" {
		t.Fatalf("回显消息期望 你好，实际 %q", data.Echo)
	}
	if data.ServerTime == "" {
		t.Fatal("服务器时间为空")
	}
}

// TestHealthWithoutAuth 探测端点免鉴权可达
func TestHealthWithoutAuth(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.buildHandler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/health")
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health 期望 200，实际 %d", resp.StatusCode)
	}
}

// TestUnregisteredOperation 未注册操作 404
func TestUnregisteredOperation(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.buildHandler())
	defer ts.Close()

	resp, _ := postOperation(t, ts.URL, "Bearer "+testToken, "no.such.operation", `{}`)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("未注册操作期望 404，实际 %d", resp.StatusCode)
	}
}

// TestOperationParamDecodeFailure 参数解码失败属操作失败：200 + 纯失败信封（不进入
// 业务调用），非失败分支不携带 data 载荷
func TestOperationParamDecodeFailure(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.buildHandler())
	defer ts.Close()

	resp, view := postOperation(t, ts.URL, "Bearer "+testToken, "sample.echo", `{"message":123}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("解码失败期望 200 由信封表达，实际 %d", resp.StatusCode)
	}
	if view.Success {
		t.Fatal("解码失败期望失败信封")
	}
	if view.Msg == "" {
		t.Fatal("失败信封 msg 不应为空")
	}
}

// TestTokenResetInvalidatesOldToken token 更换即旧 token 失效（内存单点比对）
func TestTokenResetInvalidatesOldToken(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.buildHandler())
	defer ts.Close()

	resp, _ := postOperation(t, ts.URL, "Bearer "+testToken, "sample.echo", `{}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("原 token 期望 200，实际 %d", resp.StatusCode)
	}

	srv.auth.SetToken("new-token-xyz")
	resp, _ = postOperation(t, ts.URL, "Bearer "+testToken, "sample.echo", `{}`)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("旧 token 期望 401，实际 %d", resp.StatusCode)
	}
	resp, _ = postOperation(t, ts.URL, "Bearer new-token-xyz", "sample.echo", `{}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("新 token 期望 200，实际 %d", resp.StatusCode)
	}
}

// TestNewServerGeneratesRandomToken token 缺省时随机生成
func TestNewServerGeneratesRandomToken(t *testing.T) {
	srv, err := NewServer(NewRegistry(), "")
	if err != nil {
		t.Fatalf("构建服务失败: %v", err)
	}
	if srv.Token() == "" {
		t.Fatal("缺省 token 期望随机生成非空值")
	}
}

// TestStartStopIdempotent 监听器起停幂等：重复 Start 为空操作且返回同一地址；重复
// Stop 为空操作；停止后可再次启动（随设置开关起停的运行时切换基础）
func TestStartStopIdempotent(t *testing.T) {
	srv := newTestServer(t)

	addr1, err := srv.Start(0)
	if err != nil {
		t.Fatalf("首次启动失败: %v", err)
	}
	// 真实监听可达性：免鉴权探测端点应答
	healthResp, err := http.Get("http://" + addr1 + "/api/health")
	if err != nil {
		t.Fatalf("启动后请求失败: %v", err)
	}
	_ = healthResp.Body.Close()
	if healthResp.StatusCode != http.StatusOK {
		t.Fatalf("启动后 health 期望 200，实际 %d", healthResp.StatusCode)
	}

	addr2, err := srv.Start(0)
	if err != nil {
		t.Fatalf("重复启动期望幂等空操作，实际错误: %v", err)
	}
	if addr2 != addr1 {
		t.Fatalf("重复启动期望返回同一地址 %s，实际 %s", addr1, addr2)
	}

	if err := srv.Stop(); err != nil {
		t.Fatalf("停止失败: %v", err)
	}
	if err := srv.Stop(); err != nil {
		t.Fatalf("重复停止期望幂等空操作，实际错误: %v", err)
	}

	addr3, err := srv.Start(0)
	if err != nil {
		t.Fatalf("停止后再次启动失败: %v", err)
	}
	if addr3 == "" {
		t.Fatal("再次启动未返回地址")
	}
	if err := srv.Stop(); err != nil {
		t.Fatalf("收尾停止失败: %v", err)
	}
}
