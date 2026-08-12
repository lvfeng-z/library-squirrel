//go:build !windows

package fsmonitor

// isElevated 非 Windows 平台无 USN journal 能力，恒 false。
func isElevated() bool {
	return false
}
