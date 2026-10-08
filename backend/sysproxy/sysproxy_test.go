package sysproxy

import "testing"

// stubSystemProxy 替换系统代理层读取钩子（注册表读取在 windows 分文件实现，
// 本测试经钩子注入桩驱动三级链），返回还原函数
func stubSystemProxy(proxy string, ok bool) func() {
	orig := readSystemProxy
	readSystemProxy = func() (string, bool) { return proxy, ok }
	return func() { readSystemProxy = orig }
}

// clearProxyEnv 清空代理相关环境变量，保证 env 层判定确定（httpproxy 视空串为未设置）
func clearProxyEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy", "NO_PROXY", "no_proxy"} {
		t.Setenv(key, "")
	}
}

// TestNormalizeProxyServer 注册表 ProxyServer 值的多协议格式分支：多协议取 https、
// 无 https 退 http、无 http/https 条目得空、单地址补 scheme、空值
func TestNormalizeProxyServer(t *testing.T) {
	tests := []struct {
		name   string
		server string
		want   string
	}{
		{name: "空值", server: "", want: ""},
		{name: "仅空白", server: "   ", want: ""},
		{name: "单地址补 scheme", server: "127.0.0.1:7890", want: "http://127.0.0.1:7890"},
		{name: "单地址含首尾空白", server: " 127.0.0.1:7890 ", want: "http://127.0.0.1:7890"},
		{name: "已含 scheme 原样返回", server: "socks5://127.0.0.1:1080", want: "socks5://127.0.0.1:1080"},
		{name: "多协议取 https 条目", server: "http=10.0.0.1:80;https=10.0.0.2:443;ftp=10.0.0.3:21", want: "http://10.0.0.2:443"},
		{name: "多协议无 https 退 http", server: "http=10.0.0.1:80;ftp=10.0.0.3:21", want: "http://10.0.0.1:80"},
		{name: "多协议仅 https 单条目", server: "https=proxy.local:3128", want: "http://proxy.local:3128"},
		{name: "无 http/https 条目得空", server: "ftp=10.0.0.3:21;socks=10.0.0.4:1080", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeProxyServer(tt.server); got != tt.want {
				t.Errorf("normalizeProxyServer(%q) = %q, 期望 %q", tt.server, got, tt.want)
			}
		})
	}
}

// TestResolveChainOrder 三级链优先序四例：显式 > 系统代理 > 环境变量 > 直连。
// 系统层经钩子注入，显式与 env 用真实入参驱动
func TestResolveChainOrder(t *testing.T) {
	t.Run("显式代理压过系统代理与环境变量", func(t *testing.T) {
		defer stubSystemProxy("http://sys:1", true)()
		clearProxyEnv(t)
		t.Setenv("HTTPS_PROXY", "http://env:2")
		gotURL, gotSource := Resolve(" http://explicit:3 ", "https://www.example.com/a")
		if gotURL != "http://explicit:3" || gotSource != SourceExplicit {
			t.Errorf("显式层决议 = (%q, %q), 期望 (http://explicit:3, explicit)", gotURL, gotSource)
		}
	})
	t.Run("无显式时系统代理压过环境变量", func(t *testing.T) {
		defer stubSystemProxy("http://sys:1", true)()
		clearProxyEnv(t)
		t.Setenv("HTTPS_PROXY", "http://env:2")
		gotURL, gotSource := Resolve("", "https://www.example.com/a")
		if gotURL != "http://sys:1" || gotSource != SourceSystem {
			t.Errorf("系统层决议 = (%q, %q), 期望 (http://sys:1, system)", gotURL, gotSource)
		}
	})
	t.Run("无显式无系统时环境变量兜底", func(t *testing.T) {
		defer stubSystemProxy("", false)()
		clearProxyEnv(t)
		t.Setenv("HTTPS_PROXY", "http://env:2")
		gotURL, gotSource := Resolve("", "https://www.example.com/a")
		if gotURL != "http://env:2" || gotSource != SourceEnv {
			t.Errorf("env 层决议 = (%q, %q), 期望 (http://env:2, env)", gotURL, gotSource)
		}
	})
	t.Run("三层皆无直连", func(t *testing.T) {
		defer stubSystemProxy("", false)()
		clearProxyEnv(t)
		gotURL, gotSource := Resolve("", "https://www.example.com/a")
		if gotURL != "" || gotSource != SourceNone {
			t.Errorf("无代理决议 = (%q, %q), 期望 (\"\", none)", gotURL, gotSource)
		}
	})
}

// TestResolveEnvMatching env 层按请求目标地址匹配：scheme 区分与 NO_PROXY 例外
func TestResolveEnvMatching(t *testing.T) {
	defer stubSystemProxy("", false)()
	clearProxyEnv(t)

	t.Run("https 目标走 HTTPS_PROXY", func(t *testing.T) {
		t.Setenv("HTTPS_PROXY", "http://env-https:1")
		gotURL, gotSource := Resolve("", "https://www.example.com/a")
		if gotURL != "http://env-https:1" || gotSource != SourceEnv {
			t.Errorf("https 目标决议 = (%q, %q), 期望 (http://env-https:1, env)", gotURL, gotSource)
		}
	})
	t.Run("http 目标走 HTTP_PROXY", func(t *testing.T) {
		t.Setenv("HTTP_PROXY", "http://env-http:1")
		gotURL, gotSource := Resolve("", "http://www.example.com/a")
		if gotURL != "http://env-http:1" || gotSource != SourceEnv {
			t.Errorf("http 目标决议 = (%q, %q), 期望 (http://env-http:1, env)", gotURL, gotSource)
		}
	})
	t.Run("NO_PROXY 命中目标主机直连", func(t *testing.T) {
		t.Setenv("HTTPS_PROXY", "http://env:2")
		t.Setenv("NO_PROXY", "www.example.com")
		gotURL, gotSource := Resolve("", "https://www.example.com/a")
		if gotURL != "" || gotSource != SourceNone {
			t.Errorf("NO_PROXY 命中决议 = (%q, %q), 期望 (\"\", none)", gotURL, gotSource)
		}
	})
	t.Run("scheme 与环境变量不对应时直连", func(t *testing.T) {
		t.Setenv("HTTPS_PROXY", "http://env-https:1")
		gotURL, gotSource := Resolve("", "http://www.example.com/a")
		if gotURL != "" || gotSource != SourceNone {
			t.Errorf("scheme 不对应决议 = (%q, %q), 期望 (\"\", none)", gotURL, gotSource)
		}
	})
	t.Run("空目标地址直连", func(t *testing.T) {
		t.Setenv("HTTPS_PROXY", "http://env-https:1")
		gotURL, gotSource := Resolve("", "")
		if gotURL != "" || gotSource != SourceNone {
			t.Errorf("空目标决议 = (%q, %q), 期望 (\"\", none)", gotURL, gotSource)
		}
	})
}
