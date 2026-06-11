//go:build windows

package extension

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/library-squirrel/backend/base/logger"
)

// ProcessGroup Windows 实现，使用 Job Object 管理子进程
// 设置 KILL_ON_JOB_CLOSE 标志，主进程退出时自动终止所有插件子进程
type ProcessGroup struct {
	job windows.Handle
}

// NewProcessGroup 创建匿名 Job Object 并设置 KILL_ON_JOB_CLOSE 标志
func NewProcessGroup() (*ProcessGroup, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("创建 Job Object 失败: %w", err)
	}

	// 设置 Job Object 限制：主进程退出时自动终止所有子进程
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	_, err = windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	)
	if err != nil {
		windows.CloseHandle(job)
		return nil, fmt.Errorf("设置 Job Object 限制失败: %w", err)
	}

	logger.Log.Infof("Job Object 已创建（KILL_ON_JOB_CLOSE），插件子进程将在主进程退出时自动终止")
	return &ProcessGroup{job: job}, nil
}

// Assign 将指定 PID 的进程加入 Job Object
func (pg *ProcessGroup) Assign(pid int) error {
	handle, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
		false,
		uint32(pid),
	)
	if err != nil {
		return fmt.Errorf("打开进程 %d 句柄失败: %w", pid, err)
	}
	defer windows.CloseHandle(handle)

	err = windows.AssignProcessToJobObject(pg.job, handle)
	if err != nil {
		return fmt.Errorf("将进程 %d 加入 Job Object 失败: %w", pid, err)
	}

	return nil
}

// Close 关闭 Job Object 句柄
func (pg *ProcessGroup) Close() {
	if pg.job != 0 {
		windows.CloseHandle(pg.job)
		pg.job = 0
	}
}
