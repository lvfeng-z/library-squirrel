//go:build windows

package sysproxy

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

// readWindowsSystemProxy 读取 Windows 系统代理（注册表 Internet Settings）。
// 返回带 scheme 的代理地址（如 http://127.0.0.1:7890）；未启用或读取失败时 ok=false。
// 不处理 ProxyOverride：其条目通常为本地地址（localhost 等），插件出网请求始终为
// 远程，无功能交集。
func readWindowsSystemProxy() (proxy string, ok bool) {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
		registry.QUERY_VALUE)
	if err != nil {
		return "", false
	}
	defer k.Close()

	enable, _, err := k.GetIntegerValue("ProxyEnable")
	if err != nil || enable == 0 {
		return "", false
	}
	server, _, err := k.GetStringValue("ProxyServer")
	if err != nil || strings.TrimSpace(server) == "" {
		return "", false
	}
	return normalizeProxyServer(server), true
}
