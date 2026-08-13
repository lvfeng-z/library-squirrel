//go:build !windows

// 非 Windows 平台占位：USN 离线追溯仅 Windows（NTFS 卷级 journal）可用。
// deps.go 的 windows 分支虽在本平台不执行但仍参与编译，故 NewUsnProvider 须有非 Windows 定义（返回 nil）。
// 非 Windows 平台 OfflineChangeProvider 恒为 nil，离线走全量对账。
package fsmonitor

// NewUsnProvider 非 Windows 平台无 USN 能力，返回 nil。
func NewUsnProvider(workDir string, cursors CursorStore) OfflineChangeProvider {
	return nil
}
