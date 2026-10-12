package externalapi

import (
	"context"
	"errors"

	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/task"
)

// 任务创建与控制操作面（写面）：URL 建任务（消歧冲突载荷透传、带显选键重发）、任务树
// 启动/暂停/恢复/重试/强停、覆盖确认单个与批量。任务状态查询在只读操作面（task.status、
// taskManager.treeState、taskManager.snapshot，见 ops_query.go）。每个操作 = 操作标识 +
// 参数适配 + 目标 handler 方法调用 + 信封解包——确认与消歧全程复用既有「持久状态 + 后续
// 调用解除」状态机（建任务的消歧冲突载荷与覆盖确认等待本就是独立后续调用形态），本文件
// 只是第二入口，不复制业务逻辑、不建阻塞通道。所需方法面在此按调用方需要声明（各业务
// 模块 handler 以方法集满足），装配期由调用方注入

// 操作标识（统一操作端点 POST /api/op/{标识} 的路径段；能力清单的操作目录以此为准）
const (
	// opTaskCreateByURL 按 URL 创建任务
	opTaskCreateByURL = "task.createByURL"
	// opTaskManagerStart 任务树启动
	opTaskManagerStart = "taskManager.start"
	// opTaskManagerPause 任务树暂停
	opTaskManagerPause = "taskManager.pause"
	// opTaskManagerResume 任务树恢复
	opTaskManagerResume = "taskManager.resume"
	// opTaskManagerStop 任务树强停（强制停止任务树）
	opTaskManagerStop = "taskManager.stop"
	// opTaskManagerRetry 任务树重试
	opTaskManagerRetry = "taskManager.retry"
	// opTaskManagerConfirmReplace 覆盖确认（单个任务）
	opTaskManagerConfirmReplace = "taskManager.confirmReplace"
	// opTaskManagerConfirmReplaceBatch 覆盖确认（批量）
	opTaskManagerConfirmReplaceBatch = "taskManager.confirmReplaceBatch"
)

// TaskCreateHandler 建任务的方法面（*task.Handler 满足）
type TaskCreateHandler interface {
	// CreateTaskByURL 按 URL 创建任务：交互面语义——命中多个候选且未显选时返回冲突载荷
	// （未调用任何插件），由调用方带显选键重发；返回的业务结果对象自带 succeed/msg 字段，
	// 冲突与建任务成败均住 success=true 的 data 载荷内
	CreateTaskByURL(ctx context.Context, req *task.CreateTaskByURLRequest) *model.ApiResponse[*task.CreateTaskByURLResponse]
}

// TaskControlHandler 任务树控制的方法面（*taskManager.Handler 满足）
type TaskControlHandler interface {
	// StartTaskTrees 批量启动任务树（板块全量执行）
	StartTaskTrees(ctx context.Context, taskIds []int64) *model.ApiResponse[any]
	// PauseTaskTrees 批量暂停任务树
	PauseTaskTrees(ctx context.Context, taskIds []int64) *model.ApiResponse[any]
	// ResumeTaskTrees 批量恢复任务树
	ResumeTaskTrees(ctx context.Context, taskIds []int64) *model.ApiResponse[any]
	// StopTaskTrees 批量强制停止任务树
	StopTaskTrees(ctx context.Context, taskIds []int64) *model.ApiResponse[any]
	// RetryTaskTrees 批量重试任务树
	RetryTaskTrees(ctx context.Context, taskIds []int64) *model.ApiResponse[any]
	// ConfirmReplace 确认替换或跳过单个任务的重复作品（解除等待确认态）
	ConfirmReplace(ctx context.Context, taskId int64, action string) *model.ApiResponse[any]
	// ConfirmReplaceBatch 批量确认替换或跳过重复作品
	ConfirmReplaceBatch(ctx context.Context, taskIds []int64, action string) *model.ApiResponse[any]
}

// TaskDeps 任务操作的目标方法面（装配期注入）。任一项缺失即注册报错——缺实现的操作标识
// 会误导外部调用方，宁缺勿错
type TaskDeps struct {
	Task        TaskCreateHandler
	TaskManager TaskControlHandler
}

// 参数摘要（能力清单的参数 schema 摘要；名称与请求体 JSON 字段名一致）
var (
	// taskCreateByURLParamSpec URL 建任务参数摘要
	taskCreateByURLParamSpec = []OperationParam{
		{Name: "url", Type: "string", Required: true, Desc: "作品页地址；命中多个可处理候选且未显选时响应为冲突载荷（conflict=true + conflictCandidates 候选清单，首位即默认选中项），识别后带显选键重发同一操作"},
		{Name: "chosenPluginPublicId", Type: "string", Desc: "显选键之一：候选插件公开 ID（与 chosenExtensionId 联合定位一个候选；两键均空 = 未显选）"},
		{Name: "chosenExtensionId", Type: "string", Desc: "显选键之二：候选扩展点 ID（与 chosenPluginPublicId 联合定位一个候选）"},
		{Name: "remember", Type: "bool", Desc: "显选附带「记住此选择」勾选态：true 时路由成功后落消歧记忆，同站点同候选组合的后续建任务静默直路由；缺省 false"},
	}
	// taskTreeActionParamSpec 任务树控制参数摘要（启动/暂停/恢复/重试/强停共用形态）
	taskTreeActionParamSpec = []OperationParam{
		{Name: "taskIds", Type: "array", Required: true, Desc: "任务 ID 数组（元素为整数）；父任务覆盖整树，叶子与独立任务只作用自身"},
	}
	// confirmReplaceParamSpec 单个覆盖确认参数摘要
	confirmReplaceParamSpec = []OperationParam{
		{Name: "taskId", Type: "int64", Required: true, Desc: "任务 ID（须处于等待确认态，否则报任务不在等待确认表）"},
		{Name: "action", Type: "string", Required: true, Desc: "确认动作：replace = 替换重复作品继续执行（涉及作品被分享拉取持有时被拒，需解锁后重发）；skip = 跳过重复作品"},
	}
	// confirmReplaceBatchParamSpec 批量覆盖确认参数摘要
	confirmReplaceBatchParamSpec = []OperationParam{
		{Name: "taskIds", Type: "array", Required: true, Desc: "任务 ID 数组（不在等待确认表的任务被静默跳过）"},
		{Name: "action", Type: "string", Required: true, Desc: "确认动作：replace = 替换重复作品继续执行（任一涉及作品被锁则整体不投递、全部留在等待确认表）；skip = 跳过重复作品"},
	}
)

// RegisterTaskOperations 把任务创建与控制操作装入注册表并登记各操作的参数摘要（能力清单
// 数据源）。依赖面缺项即报错且不装入任何操作；操作标识重复等注册表错误原样上抛（装配期
// 错误）
func RegisterTaskOperations(r *Registry, deps TaskDeps) error {
	if r == nil {
		return errors.New("外部操作注册表为空")
	}
	switch {
	case deps.Task == nil:
		return errors.New("建任务面未注入")
	case deps.TaskManager == nil:
		return errors.New("任务树控制面未注入")
	}

	// URL 建任务：消歧冲突载荷与建任务成败均住 data 载荷内（业务结果对象自带 succeed/msg），
	// 参数原样透传给 handler——显选键、remember 语义由任务模块既有状态机解释
	if err := registerOperation(r, opTaskCreateByURL, "按 URL 创建任务：路由到能处理该 URL 的插件扩展点建任务；多候选未显选时返回冲突载荷，带显选键重发即消歧；建成的任务可撞覆盖确认等待态（经任务状态查询感知，由覆盖确认操作解除）", taskCreateByURLParamSpec,
		func(ctx context.Context, p taskCreateByURLParams) (any, error) {
			return unwrapResponse(deps.Task.CreateTaskByURL(ctx, &task.CreateTaskByURLRequest{
				URL:                  p.URL,
				ChosenPluginPublicId: p.ChosenPluginPublicId,
				ChosenExtensionId:    p.ChosenExtensionId,
				Remember:             p.Remember,
			}))
		}); err != nil {
		return err
	}

	// 任务树启动
	if err := registerOperation(r, opTaskManagerStart, "启动任务树：板块全量执行（各任务按整树启动，重试/恢复不经此操作）", taskTreeActionParamSpec,
		func(ctx context.Context, p taskTreeActionParams) (any, error) {
			return unwrapResponse(deps.TaskManager.StartTaskTrees(ctx, p.TaskIDs))
		}); err != nil {
		return err
	}

	// 任务树暂停
	if err := registerOperation(r, opTaskManagerPause, "暂停任务树", taskTreeActionParamSpec,
		func(ctx context.Context, p taskTreeActionParams) (any, error) {
			return unwrapResponse(deps.TaskManager.PauseTaskTrees(ctx, p.TaskIDs))
		}); err != nil {
		return err
	}

	// 任务树恢复
	if err := registerOperation(r, opTaskManagerResume, "恢复任务树", taskTreeActionParamSpec,
		func(ctx context.Context, p taskTreeActionParams) (any, error) {
			return unwrapResponse(deps.TaskManager.ResumeTaskTrees(ctx, p.TaskIDs))
		}); err != nil {
		return err
	}

	// 任务树强停（破坏性收尾：强制停止任务树）
	if err := registerOperation(r, opTaskManagerStop, "强制停止任务树：破坏性收尾（强制停止任务树的执行），用于卡死任务的了结", taskTreeActionParamSpec,
		func(ctx context.Context, p taskTreeActionParams) (any, error) {
			return unwrapResponse(deps.TaskManager.StopTaskTrees(ctx, p.TaskIDs))
		}); err != nil {
		return err
	}

	// 任务树重试
	if err := registerOperation(r, opTaskManagerRetry, "重试任务树：按各任务已记录的板块模式重新执行", taskTreeActionParamSpec,
		func(ctx context.Context, p taskTreeActionParams) (any, error) {
			return unwrapResponse(deps.TaskManager.RetryTaskTrees(ctx, p.TaskIDs))
		}); err != nil {
		return err
	}

	// 覆盖确认（单个）：等待确认态的解除入口，与 GUI 确认弹窗等效（汇入同一确认状态机）
	if err := registerOperation(r, opTaskManagerConfirmReplace, "覆盖确认（单个任务）：对等待确认态的任务答复替换或跳过重复作品，与图形界面的确认弹窗等效", confirmReplaceParamSpec,
		func(ctx context.Context, p confirmReplaceParams) (any, error) {
			return unwrapResponse(deps.TaskManager.ConfirmReplace(ctx, p.TaskID, p.Action))
		}); err != nil {
		return err
	}

	// 覆盖确认（批量）
	if err := registerOperation(r, opTaskManagerConfirmReplaceBatch, "覆盖确认（批量）：对多个等待确认态的任务一并答复替换或跳过", confirmReplaceBatchParamSpec,
		func(ctx context.Context, p confirmReplaceBatchParams) (any, error) {
			return unwrapResponse(deps.TaskManager.ConfirmReplaceBatch(ctx, p.TaskIDs, p.Action))
		}); err != nil {
		return err
	}

	return nil
}

// taskCreateByURLParams URL 建任务入参（对齐建任务请求 DTO 的字段）
type taskCreateByURLParams struct {
	URL                  string `json:"url"`
	ChosenPluginPublicId string `json:"chosenPluginPublicId"`
	ChosenExtensionId    string `json:"chosenExtensionId"`
	Remember             bool   `json:"remember"`
}

// taskTreeActionParams 任务树控制入参（启动/暂停/恢复/重试/强停共用形态）
type taskTreeActionParams struct {
	TaskIDs []int64 `json:"taskIds"`
}

// confirmReplaceParams 单个覆盖确认入参
type confirmReplaceParams struct {
	TaskID int64  `json:"taskId"`
	Action string `json:"action"`
}

// confirmReplaceBatchParams 批量覆盖确认入参
type confirmReplaceBatchParams struct {
	TaskIDs []int64 `json:"taskIds"`
	Action  string  `json:"action"`
}
