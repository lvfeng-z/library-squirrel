package download

// PluginDownloadStrategy 插件下载任务的执行面策略（task_type = "plugin-download"）：
// 板块组合执行 + 多轨下载/续传 + 替换链的领域主体，经 StrategyHandle 上报终态/进度/覆盖确认/跳过收口。
// 暂停/停止命令处理时的插件 RPC 转发经 InterruptNotifier 提供（控制面对策略按类型断言调用）。

import (
	"context"
	"fmt"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/taskManager"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// PluginDownloadStrategy 插件下载执行面策略。每次 Execute 自建执行会话（work_task 行快照/
// 流集合/软暂停信号消费等会话态），无跨执行可变状态
type PluginDownloadStrategy struct {
	deps *Deps
}

// NewPluginDownloadStrategy 构造插件下载执行面策略
func NewPluginDownloadStrategy(deps *Deps) *PluginDownloadStrategy {
	return &PluginDownloadStrategy{deps: deps}
}

// Execute 插件下载任务主体执行入口：按 taskId 查作品任务领域行（行缺失即失败收口），
// 按领域行的插件身份取执行器，据恢复信号分叉——恢复且暂存目录有轨道文件时走跨重启续传
// （暂存枚举推导偏移；暂存为空/作品定位失败时续传主体内部降级完整重新执行），其余走板块
// 组合执行（重走查重/板块选择/替换链）。中断（暂停/停止）不上报终态交控制面接管
func (s *PluginDownloadStrategy) Execute(handle taskManager.StrategyHandle) {
	task := handle.Task()
	var taskId int64
	if task != nil {
		taskId = task.GetID()
	}
	wt, err := s.deps.WorkTasks.GetById(handle.RunCtx(), taskId)
	if err != nil || wt == nil {
		// 领域行缺失（建树链异常窗口遗留）：按领域数据缺失显式失败
		handle.Fail("任务缺少作品领域数据")
		return
	}
	exec, execErr := s.deps.PluginExecFactory.Executor(wt.PluginPublicID.String)
	if execErr != nil {
		handle.Fail(fmt.Sprintf("获取插件执行器失败: %v", execErr))
		return
	}
	sess := newExecSession(s.deps, handle, wt)
	sess.pluginExec = exec
	sess.mode = runModeFromTask(wt)
	if handle.ResumeRequested() && sess.stagingHasFiles() {
		sess.resumeFromPersistedState()
		return
	}
	sess.runSectionCombo()
}

// NotifyInterrupt 暂停/停止命令处理时的插件 RPC 转发：自查任务核心行与作品任务领域行组装
// 参数，stop=true 走 Stop（放弃语义）、否则走 Pause（保留已落盘字节供续传）。RPC 失败仅
// 告警——中断的主体信号是运行 ctx 取消，插件侧通知为尽力而为
func (s *PluginDownloadStrategy) NotifyInterrupt(ctx context.Context, taskID int64, stop bool) {
	if s.deps == nil || s.deps.TaskCoreReader == nil || s.deps.WorkTasks == nil || s.deps.PluginExecFactory == nil {
		return
	}
	task, err := s.deps.TaskCoreReader.GetById(ctx, taskID)
	if err != nil || task == nil {
		logger.Log.Warnf("[Download] 任务 %d 中断通知组装失败: 查询任务核心行失败 %v", taskID, err)
		return
	}
	wt, err := s.deps.WorkTasks.GetById(ctx, taskID)
	if err != nil || wt == nil {
		logger.Log.Warnf("[Download] 任务 %d 中断通知组装失败: 查询作品任务领域行失败 %v", taskID, err)
		return
	}
	exec, err := s.deps.PluginExecFactory.Executor(wt.PluginPublicID.String)
	if err != nil {
		logger.Log.Warnf("[Download] 任务 %d 中断通知获取执行器失败: %v", taskID, err)
		return
	}
	param := &sdkdto.TaskResParam{
		Task: dto.AssembleTaskDTO(task, wt, nil),
	}
	if stop {
		if err := exec.Stop(ctx, param); err != nil {
			logger.Log.Errorf("[Download] 任务 %d 插件 Stop 失败: %v", taskID, err)
		}
		return
	}
	if err := exec.Pause(ctx, param); err != nil {
		logger.Log.Warnf("[Download] 任务 %d 插件 Pause 失败: %v", taskID, err)
	}
}

// 编译期接口断言：暂停/停止命令处理的执行面通知能力（控制面按类型断言调用）
var _ taskManager.InterruptNotifier = (*PluginDownloadStrategy)(nil)
