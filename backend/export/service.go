package export

import (
	"context"
	"errors"

	"github.com/library-squirrel/backend/base/model/entity"
)

// TaskTypeExport 导出任务类型（登记于 task.task_type，注册进 taskManager 执行面策略表）
const TaskTypeExport = "export"

// TaskControl 导出任务创建与启动能力（task.Service 与 taskManager.Manager 经 app.go 适配器
// 组合装配；适配器经延迟闭包取用——装配时序上 ExportService 先于 TaskManager 创建）。
// 任务核心行不含领域载荷：导出选择参数在 export_task 领域行（建任务后由 Service 补写）。
type TaskControl interface {
	// CreateBuiltinTask 创建内置类型任务（返回任务 ID）
	CreateBuiltinTask(ctx context.Context, taskType string, taskName string) (int64, error)
	// StartTasks 启动任务树
	StartTasks(ctx context.Context, taskIds []int64) error
	// DeleteTask 批量删除任务（含子任务与领域行）；建任务失败回滚用
	DeleteTask(ctx context.Context, ids []int64) error
}

// ErrTaskControlNil 导出任务创建能力未装配
var ErrTaskControlNil = errors.New("导出任务创建能力未装配")

// ExportTaskResult StartExport 返回载荷：新建导出任务 ID（执行进度与终态经任务面板统一承载）。
type ExportTaskResult struct {
	TaskID int64 `json:"taskId"`
}

// Service 导出服务：对外提供导出数据收集与导出任务的两步创建编排（建任务核心行 +
// export_task 领域行 → 启动）；打包执行归 ExportExecution 执行面策略（经 taskManager 调度）。
type Service struct {
	collector   *Collector            // 导出数据面（分享发布复用）
	exportTasks *ExportTaskRepository // 导出任务领域行仓储
	taskCtl     TaskControl           // 导出任务创建/启动能力（app.go 装配；nil=不可建导出任务）
	workDir     func() string         // 工作目录（源文件根 + 缺省输出根；读取时取，避免持有过期值）
}

// NewService 创建导出服务。
// repo 为导出数据面查询仓储；exportTasks 为导出任务领域行仓储；versionProvider 提供来源
// app 版本（写入 manifest.meta.appVersion）；workDirProvider 提供当前工作目录。
func NewService(repo Repository, exportTasks *ExportTaskRepository, versionProvider func() string, workDirProvider func() string) *Service {
	return &Service{
		collector:   NewCollector(repo, versionProvider),
		exportTasks: exportTasks,
		workDir:     workDirProvider,
	}
}

// SetTaskControl 设置导出任务创建/启动能力（app.go 装配：适配器经延迟闭包取用运行期就绪的
// TaskService/TaskManager，注入时点不受装配顺序约束）
func (s *Service) SetTaskControl(ctl TaskControl) {
	s.taskCtl = ctl
}

// Collect 按选择 id 列表收集导出数据模型：后端完成作品集闭包、成员关系裁剪与全部关联数据
// 收集，产出内存态导出模型（分享发布复用同一数据面）。
func (s *Service) Collect(ctx context.Context, workIDs []int64, workSetIDs []int64) (*ExportModel, error) {
	return s.collector.Collect(ctx, workIDs, workSetIDs)
}

// StartExport 创建导出任务并启动（两步建任务）：前置校验选择非空（失败不建任务行）→ 建任务
// 核心行 → 补写 export_task 领域行 → 启动执行；任一步失败显式删除任务行回滚（不留孤儿任务）。
// 返回新建任务 ID。outputDir 为空时沿用工作目录作落盘根，非空为自选输出目录。
func (s *Service) StartExport(ctx context.Context, workIDs []int64, workSetIDs []int64, outputDir string) (*ExportTaskResult, error) {
	if s.taskCtl == nil {
		return nil, ErrTaskControlNil
	}
	if len(workIDs) == 0 && len(workSetIDs) == 0 {
		return nil, ErrExportEmptySelection
	}
	workIDsJSON, err := marshalIDList(workIDs)
	if err != nil {
		return nil, err
	}
	workSetIDsJSON, err := marshalIDList(workSetIDs)
	if err != nil {
		return nil, err
	}
	taskID, err := s.taskCtl.CreateBuiltinTask(ctx, TaskTypeExport, exportTaskName(len(workIDs)+len(workSetIDs)))
	if err != nil {
		return nil, err
	}
	et := entity.NewExportTask(taskID)
	et.WorkIDs = workIDsJSON
	et.WorkSetIDs = workSetIDsJSON
	et.OutputDir = outputDir
	if err := s.exportTasks.CreateForTask(ctx, taskID, et); err != nil {
		_ = s.taskCtl.DeleteTask(ctx, []int64{taskID})
		return nil, err
	}
	if err := s.taskCtl.StartTasks(ctx, []int64{taskID}); err != nil {
		_ = s.taskCtl.DeleteTask(ctx, []int64{taskID})
		return nil, err
	}
	return &ExportTaskResult{TaskID: taskID}, nil
}
