//go:build !windows

package sysproxy

// readWindowsSystemProxy 非 Windows 平台无 Windows 系统代理可读，三级链回落
// 环境变量层。
func readWindowsSystemProxy() (proxy string, ok bool) {
	return "", false
}
