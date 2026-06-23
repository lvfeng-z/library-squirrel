//go:build !windows

package window

import "errors"

// setTitleColor 非 Windows 平台无操作
// macOS 已隐藏原生标题栏（MacTitleBarHiddenInset），Linux 标题栏由桌面环境控制
func setTitleColor(hwnd uintptr, bgHex, textHex string) error {
	return errors.New("当前平台不支持标题栏颜色控制")
}
