package taskManager

import (
	"context"

	domain "github.com/library-squirrel/backend/base/model/entity"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// TaskExecutorInterface 任务执行器接口
// 由 TaskManager 定义，Plugin 模块实现
// 注意：此接口由 taskManager 模块定义，实现由 plugin 模块提供
type TaskExecutorInterface interface {
	// CreateWorkInfo 创建作品信息
	// ctx: 上下文
	// task: 任务信息
	// 返回作品响应或错误
	CreateWorkInfo(ctx context.Context, task *domain.Task) (*sdkdto.WorkResponse, error)

	// Start 开始任务
	// ctx: 上下文，用于取消和超时控制
	// task: 任务信息
	// storeRoles: 本次执行所选 store_type 子集(空=全量),插件据此选择性产出
	// 返回 StoreSpec 流集合(含 downloaded 与 derived)、WorkResponse 或错误
	// 调用方负责关闭各 StoreSpec.ReadCloser
	Start(ctx context.Context, task *domain.Task, storeRoles []string) ([]*sdkdto.StoreSpec, *sdkdto.WorkResponse, error)

	// Pause 暂停任务（任务级，广播到全部 stream）
	// ctx: 上下文
	// param: 任务参数，包含任务和资源信息
	// 返回错误（插件可能不支持暂停）
	Pause(ctx context.Context, param *sdkdto.TaskResParam) error

	// Stop 停止任务（任务级）
	// ctx: 上下文
	// param: 任务参数
	Stop(ctx context.Context, param *sdkdto.TaskResParam) error

	// Resume 恢复任务
	// ctx: 上下文
	// param: 续传参数（StreamOffsets 为各 downloaded 轨已写入字节数）
	// 返回 StoreSpec 流集合、WorkResponse 或错误
	// 调用方负责关闭各 StoreSpec.ReadCloser
	Resume(ctx context.Context, param *sdkdto.TaskResumeParam) ([]*sdkdto.StoreSpec, *sdkdto.WorkResponse, error)
}
