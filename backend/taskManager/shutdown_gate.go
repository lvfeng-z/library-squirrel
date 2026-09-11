package taskManager

import (
	"context"
	"errors"
	"sync"

	"github.com/library-squirrel/backend/base/logger"
)

// ShutdownGate 程序退出前的阻断点统一封装：模块把「退出前必须完成的自持收尾」注册为阻断项，
// app 在窗口销毁后、进程退出前调用 WaitAll 有界等待全部完成。任务模块是首个注册方（优雅关闭：
// 暂停全部瞬态任务、等待稳态并终刷落盘）；其他模块将来需要阻断程序关闭时走同一注册，不各自
// 在窗口事件或退出序列里另写等待。
type ShutdownGate struct {
	mu       sync.Mutex
	blockers []shutdownBlocker
}

// shutdownBlocker 单个退出阻断项：name 供日志定位；wait 须自持有界（尊重传入 ctx 的取消），
// 完成返回 nil，超时/失败返回错误由 WaitAll 汇总上报
type shutdownBlocker struct {
	name string
	wait func(ctx context.Context) error
}

// NewShutdownGate 创建退出阻断点（无注册项时 WaitAll 直通返回）
func NewShutdownGate() *ShutdownGate {
	return &ShutdownGate{}
}

// Register 注册一个退出阻断项（按注册顺序等待；同名重复注册各自独立执行）
func (g *ShutdownGate) Register(name string, wait func(ctx context.Context) error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.blockers = append(g.blockers, shutdownBlocker{name: name, wait: wait})
}

// WaitAll 顺序等待全部阻断项完成。单项错误记录后继续其余项（退出收尾互不依赖，一项失败
// 不掩盖其余项的完成情况），返回聚合错误；整体有界由调用方经 ctx 控制
func (g *ShutdownGate) WaitAll(ctx context.Context) error {
	g.mu.Lock()
	blockers := append([]shutdownBlocker(nil), g.blockers...)
	g.mu.Unlock()

	var errs []error
	for _, b := range blockers {
		if err := b.wait(ctx); err != nil {
			logger.Log.Warnf("[ShutdownGate] 退出阻断项 %s 未正常完成: %v", b.name, err)
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
