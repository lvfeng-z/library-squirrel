//go:build windows

package window

import (
	"errors"
	"syscall"
	"unsafe"
)

var (
	dwmapi                    = syscall.NewLazyDLL("dwmapi.dll")
	procDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")
)

// DWM 窗口属性常量
const (
	dwmwaUseImmersiveDarkMode = 20 // 暗色标题栏开关（Windows 10 19041+ / Windows 11）
	dwmwaCaptionColor         = 35 // 标题栏背景色（Windows 11 22000+）
	dwmwaTextColor            = 36 // 标题文字色（Windows 11 22000+）
)

// setTitleColor 设置窗口标题栏背景色与文字色
// bgHex/textHex 为 #RRGGBB 格式
func setTitleColor(hwnd uintptr, bgHex, textHex string) error {
	if hwnd == 0 {
		return errors.New("窗口句柄为空")
	}
	bg, err := hexToBGR(bgHex)
	if err != nil {
		return err
	}
	text, err := hexToBGR(textHex)
	if err != nil {
		return err
	}
	if err := setDwmColor(hwnd, dwmwaCaptionColor, bg); err != nil {
		return err
	}
	if err := setDwmColor(hwnd, dwmwaTextColor, text); err != nil {
		return err
	}
	// 浅色标题栏：关闭暗色模式（Windows 10 兜底为亮色标题栏）
	_ = setDwmBool(hwnd, dwmwaUseImmersiveDarkMode, false)
	return nil
}

// setDwmColor 设置 DWM 颜色属性（值为 0x00BBGGRR）
func setDwmColor(hwnd uintptr, attr uint32, color uint32) error {
	r1, _, e1 := procDwmSetWindowAttribute.Call(
		hwnd,
		uintptr(attr),
		uintptr(unsafe.Pointer(&color)),
		unsafe.Sizeof(color),
	)
	if r1 != 0 {
		return e1
	}
	return nil
}

// setDwmBool 设置 DWM 布尔属性
func setDwmBool(hwnd uintptr, attr uint32, value bool) error {
	v := int32(0)
	if value {
		v = 1
	}
	r1, _, e1 := procDwmSetWindowAttribute.Call(
		hwnd,
		uintptr(attr),
		uintptr(unsafe.Pointer(&v)),
		unsafe.Sizeof(v),
	)
	if r1 != 0 {
		return e1
	}
	return nil
}

// hexToBGR 将 #RRGGBB 转为 DWM 使用的 0x00BBGGRR
func hexToBGR(hex string) (uint32, error) {
	if len(hex) != 7 || hex[0] != '#' {
		return 0, errors.New("颜色格式应为 #RRGGBB")
	}
	r, err := parseHex(hex[1:3])
	if err != nil {
		return 0, err
	}
	g, err := parseHex(hex[3:5])
	if err != nil {
		return 0, err
	}
	b, err := parseHex(hex[5:7])
	if err != nil {
		return 0, err
	}
	return uint32(b)<<16 | uint32(g)<<8 | uint32(r), nil
}

// parseHex 解析两位十六进制字符串
func parseHex(s string) (uint8, error) {
	var v uint8
	for i := 0; i < len(s); i++ {
		c := s[i]
		var d uint8
		switch {
		case c >= '0' && c <= '9':
			d = c - '0'
		case c >= 'a' && c <= 'f':
			d = c - 'a' + 10
		case c >= 'A' && c <= 'F':
			d = c - 'A' + 10
		default:
			return 0, errors.New("无效的十六进制字符: " + string(c))
		}
		v = v<<4 | d
	}
	return v, nil
}
