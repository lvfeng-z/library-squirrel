package settings

import "errors"

// ErrWorkDirNotConfigured 工作目录未配置（GetWorkDir 返回空串）。
// 消费方以 errors.Is 判定，文案引导用户前往设置页完成配置
var ErrWorkDirNotConfigured = errors.New("工作目录未配置，请先在设置页配置工作目录")

// EventEmitter 前端事件发射器（由 Wails EventManager 实现）。
// 本地最小接口定义，避免 settings 包依赖 taskManager 等业务包（与 fsmonitor/resource 的本地事件接口同模式）
type EventEmitter interface {
	Emit(eventName string, data ...any) bool
}

// unconfiguredEmitterFunc 未配置通知的发射器闭包；接线前为 nil（应用启动早期发射器尚未就绪）
var unconfiguredEmitterFunc func() EventEmitter

// SetUnconfiguredEmitter 接线未配置通知的发射器闭包（app.SetEventEmitter 中调用）。
// 闭包延迟读取发射器本身，规避「settings 服务构造早于 Wails 发射器就绪」的初始化时序问题
func SetUnconfiguredEmitter(fn func() EventEmitter) {
	unconfiguredEmitterFunc = fn
}

// NotifyWorkDirUnconfigured 经统一发射口通知前端「工作目录未配置」，payload 携触发来源
// （source=发现未配置的模块名）。发射口未接线或发射器未就绪时静默跳过
func NotifyWorkDirUnconfigured(source string) {
	if unconfiguredEmitterFunc == nil {
		return
	}
	if em := unconfiguredEmitterFunc(); em != nil {
		em.Emit("workdir:unconfigured", map[string]any{"source": source})
	}
}

// RefuseIfUnconfigured 请求期未配置拒绝的收口入口：workdir 为空串（=未配置）时经统一发射口
// 通知前端并返回 ErrWorkDirNotConfigured，已配置返回 nil。拒绝点一行调用，
// 判定、通知与错误返回不散落多处
func RefuseIfUnconfigured(workdir, source string) error {
	if workdir == "" {
		NotifyWorkDirUnconfigured(source)
		return ErrWorkDirNotConfigured
	}
	return nil
}
