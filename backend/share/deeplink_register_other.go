//go:build !windows

package share

// 深链协议注册（非 Windows 占位）：协议注册由各平台构建资产承担（macOS Info.plist
// CFBundleURLTypes / Linux desktop MimeType），运行时无需自注册，查询恒为未注册、
// 注册/取消为 no-op。深链拉起链路本身（单实例转发 + URL 事件 + argv 兜底）平台无关。

// ShareProtocolRegStatus 深链协议注册状态（与 Windows 版同构）
type ShareProtocolRegStatus struct {
	Registered bool   `json:"registered"`
	Command    string `json:"command"`
	CurrentExe bool   `json:"currentExe"`
}

// EnsureShareProtocolRegistered 非 Windows：协议注册归构建资产，no-op
func EnsureShareProtocolRegistered() error { return nil }

// QueryShareProtocolRegStatus 非 Windows：恒为未注册
func QueryShareProtocolRegStatus() ShareProtocolRegStatus { return ShareProtocolRegStatus{} }

// UnregisterShareProtocol 非 Windows：no-op
func UnregisterShareProtocol() error { return nil }
