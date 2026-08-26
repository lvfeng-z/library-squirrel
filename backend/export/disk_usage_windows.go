//go:build windows

package export

import (
	"golang.org/x/sys/windows"
)

// diskFreeSpace 返回 dir 所在磁盘的可用字节数（导出前目标盘剩余空间预检用，风险6）。
// 取"可供调用者使用的可用空间"（freeBytesAvailableToCaller，含配额感知），而非卷级总剩余。
func diskFreeSpace(dir string) (uint64, error) {
	pathPtr, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return 0, err
	}
	var freeBytesAvailableToCaller, totalBytes, totalFreeBytes uint64
	err = windows.GetDiskFreeSpaceEx(pathPtr, &freeBytesAvailableToCaller, &totalBytes, &totalFreeBytes)
	return freeBytesAvailableToCaller, err
}
