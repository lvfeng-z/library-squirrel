package download

// 插件下载执行会话：承载一次执行的执行面状态（作品任务领域行快照、流集合、软暂停信号
// 消费、替换/查重决策位）。会话随每次执行构建、随执行结束消亡，策略自身无跨执行可变状态；
// 与控制面的交互全部经 StrategyHandle（运行 ctx/进度/终态上报/软暂停广播）单向进行。

import (
	"context"
	"database/sql"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/taskManager"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// execSession 插件下载任务的执行会话：任务核心行经 handle 获取，作品任务领域行为执行入口
// 按 taskId 查得的行快照，流集合与各决策位为执行期可变状态（仅执行 goroutine 访问，
// 流级字段由 streamController 自带锁保护）
type execSession struct {
	deps   *Deps
	handle taskManager.StrategyHandle

	taskId   int64            // 任务 id（=领域行共享主键）
	task     *entity.Task     // 任务核心行快照
	workTask *entity.WorkTask // 作品任务领域行快照
	workId   int64            // 作品 ID：执行初始=任务 id，作品信息保存后为实际作品 id

	// 本次执行的板块组合（执行入口从领域行持久化字段派生，板块模式唯一源=work_task 行）
	mode runMode
	// 插件任务执行器（执行入口按领域行的插件公开 ID 获取）
	pluginExec PluginExecutor

	// 替换场景（已有作品上重执行资源板块），前置软删旧 store 的决策位
	isReplace bool
	// 跳过查重（覆盖确认答复替换后的续行）
	skipDuplicateCheck bool
	// 已有作品 ID（查重命中时记录，替换定位用）
	existingWorkId int64
	// 本次执行产出的 Resource ID（资源完整度重算用）
	currentResourceId int64
	// 作品信息响应（Start/Resume 返回，供文件名模板 token 数据）
	workResp *sdkdto.WorkResponse

	// 多流控制器集合（按本次所选 storeRoles 过滤后的 spec 构建）
	streams []*streamController

	// 软暂停广播通道（构建时自 handle 获取缓存；控制面进入软暂停时 close，
	// 下载循环以非阻塞探测消费，作为收尾退出依据）
	softPauseCh <-chan struct{}
}

// newExecSession 构建执行会话：workTask 为执行入口按 taskId 查得的领域行
func newExecSession(deps *Deps, handle taskManager.StrategyHandle, workTask *entity.WorkTask) *execSession {
	task := handle.Task()
	var taskId int64
	if task != nil {
		taskId = task.GetID()
	}
	return &execSession{
		deps:        deps,
		handle:      handle,
		taskId:      taskId,
		task:        task,
		workTask:    workTask,
		workId:      taskId,
		softPauseCh: handle.SoftPauseSignal(),
	}
}

// runCtx 本次执行的 ctx（暂停/停止时由控制面取消）
func (sess *execSession) runCtx() context.Context {
	return sess.handle.RunCtx()
}

// softPauseReceived 是否已收到软暂停广播（通道 close 后恒真）
func (sess *execSession) softPauseReceived() bool {
	select {
	case <-sess.softPauseCh:
		return true
	default:
		return false
	}
}

// markResourceComplete 计算资源完整度并持久化（下载完成/续传完成时调用）。
// 委派共享重算能力：按活行 store 角色计数——关联保留形态下软删行关联不计入
func (sess *execSession) markResourceComplete(ctx context.Context, resourceId int64) {
	if resourceId == 0 || sess.deps == nil || sess.deps.ResourceRecomputer == nil {
		return
	}
	sess.deps.ResourceRecomputer.RecomputeResourceComplete(ctx, resourceId)
}

// failTerminal 失败终态收口：失败任务不续传，先清 pending_resource_id（残留值在作品/资源被
// 外部删除或还原后会指向失效 resource/store），再经 handle 上报失败。暂停保留 pending 供恢复
// 续传定位，不走此路径
func (sess *execSession) failTerminal(errMsg string) {
	if sess.workTask.PendingResourceID.Valid {
		sess.clearPendingResourceID()
	}
	sess.handle.Fail(errMsg)
}

// clearPendingResourceID 清除任务的 pending_resource_id 并直写领域行（成功/失败终态时调用；
// 暂停保留该值供恢复续传定位）
func (sess *execSession) clearPendingResourceID() {
	sess.workTask.PendingResourceID = sql.NullInt64{}
	if sess.deps == nil || sess.deps.PendingResourceUpdater == nil {
		return
	}
	if err := sess.deps.PendingResourceUpdater.UpdatePendingResourceID(context.Background(), sess.taskId, sess.workTask.PendingResourceID); err != nil {
		logger.Log.Warnf("[Download] 任务 %d 清除 pending_resource_id 失败: %v", sess.taskId, err)
	}
}
