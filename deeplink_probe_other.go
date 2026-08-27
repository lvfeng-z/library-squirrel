//go:build !windows

package main

// 深链二启早退探针（非 Windows 占位）：单实例互斥体探测是 Windows 专属优化（wails
// Windows 实现的命名互斥体）；其余平台的单实例检测在完整 application.New 内完成，
// 深链二启承担一次完整初始化后转发退出的成本，链路行为不受影响。

// anotherInstanceRunning 非 Windows：不做早期探测（恒 false，走完整启动路径）
func anotherInstanceRunning() bool { return false }
