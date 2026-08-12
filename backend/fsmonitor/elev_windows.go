//go:build windows

package fsmonitor

import "golang.org/x/sys/windows"

// isElevated 当前进程是否以管理员（提升）权限运行。
// USN 卷级 journal 读取需管理员（R2 实测），注入 USN provider 前据此门控。
func isElevated() bool {
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return false
	}
	defer token.Close()
	return token.IsElevated()
}
