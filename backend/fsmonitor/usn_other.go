//go:build !windows

// USN Journal 离线追溯仅 Windows（NTFS 卷级持久变更日志）可用。
// 本文件为非 Windows 平台占位：USN 解析符号均在 usn_windows.go（//go:build windows），
// 此处无可引用对象，仅保证包在 Linux/macOS 下可编译。
// 非 Windows 平台 OfflineChangeProvider 恒为 nil，离线走全量对账（见 deps.go NewPlatformDeps）。
package fsmonitor
