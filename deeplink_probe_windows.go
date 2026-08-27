package main

// 深链二启早退探针（Windows）：深链唤起的第二实例若完整走 NewApp（DB/迁移/捆绑插件安装）
// 才在 wails 单实例检测处退出，重复重初始化且与运行中实例并发写库——main() 早期以与
// wails 同名的互斥体探测已有实例，确认后走仅含单实例配置的极简应用实例完成 argv 转发
// 即退出。首实例冷启动（互斥体不存在）不走本路径，防自占锁误判 already-running。

import (
	"errors"
	"os"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/sys/windows"
)

// secondInstanceMutexName wails 单实例互斥体名（与 wails single_instance_windows.go 一致）
func secondInstanceMutexName() string {
	return "wails-app-" + singleInstanceUniqueID + "-sim"
}

// anotherInstanceRunning 探测已有应用实例是否持有单实例互斥体（探测后立即释放自身句柄）
func anotherInstanceRunning() bool {
	name, err := windows.UTF16PtrFromString(secondInstanceMutexName())
	if err != nil {
		return false
	}
	handle, err := windows.CreateMutex(nil, false, name)
	if err != nil {
		// 已存在（首实例持有）：CreateMutex 仍返回有效句柄，须关闭后再返回判定结果
		if handle != 0 {
			_ = windows.CloseHandle(handle)
		}
		return errors.Is(err, windows.ERROR_ALREADY_EXISTS)
	}
	if handle != 0 {
		_ = windows.CloseHandle(handle)
	}
	return false
}

// forwardDeepLinkToRunningInstance 深链二启早退：极简应用实例内 wails 检测到已运行即把
// argv（含深链 URL）经 WM_COPYDATA 转发首实例并退出；未在 New 内退出属探针竞态误报
// （首实例恰在探测后退出），放弃本次转发由用户重击链接。
func forwardDeepLinkToRunningInstance() {
	_ = application.New(application.Options{
		Name: "library-squirrel",
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: singleInstanceUniqueID,
		},
	})
	logger.Log.Warn("[main] 深链转发未按预期退出（首实例可能已退出），请重新打开分享链接")
	os.Exit(1)
}
