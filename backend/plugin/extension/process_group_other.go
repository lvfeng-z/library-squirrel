//go:build !windows

package extension

// ProcessGroup 非 Windows 平台的空实现
type ProcessGroup struct{}

// NewProcessGroup 创建进程分组管理器（非 Windows 平台为空操作）
func NewProcessGroup() (*ProcessGroup, error) {
	return &ProcessGroup{}, nil
}

// Assign 将进程加入分组（非 Windows 平台为空操作）
func (pg *ProcessGroup) Assign(pid int) error {
	return nil
}

// Close 关闭进程分组（非 Windows 平台为空操作）
func (pg *ProcessGroup) Close() {
	// no-op
}
