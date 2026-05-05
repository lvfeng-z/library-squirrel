package taskManager

import (
	"context"
	"io"

	"github.com/library-squirrel/wails/backend/base/model/dto"
	domain "github.com/library-squirrel/wails/backend/base/model/entity"
)

// TaskExecutorInterface 任务执行器接口
// 由 TaskManager 定义，Plugin 模块实现
// 注意：此接口由 taskManager 模块定义，实现由 plugin 模块提供
type TaskExecutorInterface interface {
	// CreateWorkInfo 创建作品信息
	// ctx: 上下文
	// task: 任务信息
	// 返回作品响应或错误
	CreateWorkInfo(ctx context.Context, task *domain.Task) (*dto.WorkResponse, error)

	// Start 开始任务
	// ctx: 上下文，用于取消和超时控制
	// task: 任务信息
	// workId: 工作ID
	// 返回资源读取器（io.ReadCloser）、WorkResponse 或错误
	// 调用方负责关闭返回的 ReadCloser
	Start(ctx context.Context, task *domain.Task, workId int64) (io.ReadCloser, *dto.WorkResponse, error)

	// Pause 暂停任务
	// ctx: 上下文
	// param: 任务参数，包含任务和资源信息
	// 返回错误（插件可能不支持暂停）
	Pause(ctx context.Context, param *dto.TaskResParam) error

	// Stop 停止任务
	// ctx: 上下文
	// param: 任务参数
	Stop(ctx context.Context, param *dto.TaskResParam) error

	// Resume 恢复任务
	// ctx: 上下文
	// param: 任务参数
	// 返回作品响应或错误
	Resume(ctx context.Context, param *dto.TaskResParam) (*dto.WorkResponse, error)
}
