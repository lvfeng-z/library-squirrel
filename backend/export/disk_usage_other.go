//go:build !windows

package export

import (
	"golang.org/x/sys/unix"
)

// diskFreeSpace 返回 dir 所在磁盘的可用字节数（导出前目标盘剩余空间预检用，风险6）。
func diskFreeSpace(dir string) (uint64, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(dir, &st); err != nil {
		return 0, err
	}
	return st.Bavail * uint64(st.Bsize), nil
}
