// Package sysproxy 提供宿主侧出网代理解析的纯能力：三级检测链（显式代理 >
// Windows 系统代理 > 环境变量）逐请求现查、无缓存，供插件代理解析 RPC
// （HostService.ResolveProxy）委托
package sysproxy

import (
	"net/url"
	"strings"

	"golang.org/x/net/http/httpproxy"
)

// 代理解析来源标签（与 SDK dto.ProxyResolveProvider 契约一致），仅日志排障用、
// 不驱动分支
const (
	SourceExplicit = "explicit"
	SourceSystem   = "system"
	SourceEnv      = "env"
	SourceNone     = "none"
)

// readSystemProxy 系统代理层读取入口，Windows 平台读注册表、其余平台恒无；
// 单测经本钩子注入桩以驱动三级链
var readSystemProxy = readWindowsSystemProxy

// Resolve 三级链决议单个请求的出网代理：显式代理 > Windows 系统代理 > 环境变量。
// explicitURL 为调用方显式代理（插件设置透传，空 = 自动检测），裁剪首尾空白后
// 原样返回、不做解析（地址解析与缺 scheme 补全由调用方承载）；requestURL 为本次
// 请求目标地址，env 层按其 scheme 与 NO_PROXY 例外表匹配。返回代理地址（空 = 直连）
// 与来源标签。逐请求现查无缓存，代理中途开关对下一请求即时生效。
func Resolve(explicitURL, requestURL string) (proxyURL, source string) {
	explicitURL = strings.TrimSpace(explicitURL)
	if explicitURL != "" {
		return explicitURL, SourceExplicit
	}
	if sys, ok := readSystemProxy(); ok {
		return sys, SourceSystem
	}
	return resolveFromEnv(requestURL)
}

// resolveFromEnv 环境变量层：按请求目标地址匹配 HTTP_PROXY/HTTPS_PROXY/NO_PROXY
// （x/net/http/httpproxy，标准库 ProxyFromEnvironment 同源实现）。每次调用现读
// 环境变量，与其余两层「逐请求现查」语义一致；目标地址不可解析、环境值非法或
// 无匹配时直连——请求不因环境值畸形而失败。
func resolveFromEnv(requestURL string) (string, string) {
	u, err := url.Parse(requestURL)
	if err != nil {
		return "", SourceNone
	}
	proxy, err := httpproxy.FromEnvironment().ProxyFunc()(u)
	if err != nil || proxy == nil {
		return "", SourceNone
	}
	return proxy.String(), SourceEnv
}

// normalizeProxyServer 规范化注册表 ProxyServer 值为单一代理地址。
// ProxyServer 可能是 "host:port" 或 "http=h:p;https=h:p;ftp=h:p" 多协议格式：
// 多协议时优先取 https、其次 http 条目（远程站点走 HTTPS）；统一补全 http://
// scheme（注册表值不含 scheme，代理自身通常以 HTTP CONNECT 方式工作）。
func normalizeProxyServer(server string) string {
	server = strings.TrimSpace(server)
	if server == "" {
		return ""
	}
	if strings.Contains(server, "=") {
		var picked string
		for _, part := range strings.Split(server, ";") {
			kv := strings.SplitN(part, "=", 2)
			if len(kv) != 2 {
				continue
			}
			scheme, addr := strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1])
			if scheme != "https" && scheme != "http" {
				continue
			}
			picked = withHTTPScheme(addr)
			if scheme == "https" {
				break
			}
		}
		return picked
	}
	return withHTTPScheme(server)
}

// withHTTPScheme 为地址补全 http:// scheme（已含 :// 则原样返回）
func withHTTPScheme(addr string) string {
	if strings.Contains(addr, "://") {
		return addr
	}
	return "http://" + addr
}
