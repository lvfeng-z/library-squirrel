package task

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
	querypkg "github.com/library-squirrel/backend/base/query"
	"github.com/library-squirrel/backend/database"
	pkgerr "github.com/library-squirrel/backend/error"
	"github.com/library-squirrel/backend/pluginTaskUrlListener"
	"github.com/library-squirrel/backend/route"
	"github.com/library-squirrel/backend/site"
	"github.com/library-squirrel/backend/util"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 错误定义
var (
	ErrUrlNotSupported   = &pkgerr.BusinessError{Code: 400, Message: "url不受支持"}
	ErrNoPluginFound     = &pkgerr.BusinessError{Code: 500, Message: "尝试了所有插件均未成功"}
	ErrSiteKeyRequired   = &pkgerr.BusinessError{Code: 400, Message: "创建任务失败，插件返回的任务信息中缺少站点键"}
	ErrSiteNotFound      = &pkgerr.BusinessError{Code: 400, Message: "创建任务失败，没有找到站点对应的信息"}
	ErrPluginDataInvalid = &pkgerr.BusinessError{Code: 500, Message: "序列化插件保存的pluginData失败"}
	ErrTaskHandlerFailed = &pkgerr.BusinessError{Code: 500, Message: "插件创建任务失败"}
	// ErrChosenCandidateInvalid 交互面显选键未命中该 URL 的候选集（两键联合定位一个扩展点候选）
	ErrChosenCandidateInvalid = &pkgerr.BusinessError{Code: 400, Message: "所选的插件扩展点不在该链接的候选集内"}
)

// StatusUpdate 待持久化的状态变更（包含状态和错误信息）
type StatusUpdate struct {
	Status       TaskStatusEnum
	ErrorMessage sql.NullString
}

// TaskStatusEnum 任务状态枚举，与 taskManager.TaskState 保持一致
type TaskStatusEnum int

const (
	TaskStatusCreated        TaskStatusEnum = 0
	TaskStatusWaiting        TaskStatusEnum = 1
	TaskStatusProcessing     TaskStatusEnum = 2
	TaskStatusPausing        TaskStatusEnum = 3
	TaskStatusPaused         TaskStatusEnum = 4
	TaskStatusStopping       TaskStatusEnum = 5
	TaskStatusFinished       TaskStatusEnum = 6
	TaskStatusFailed         TaskStatusEnum = 7
	TaskStatusPartlyFinished TaskStatusEnum = 8
)

// MemoryStateProvider 内存任务状态提供者接口
// 由 taskManager.Manager 实现，用于查询时综合内存中的实时状态
type MemoryStateProvider interface {
	// GetTaskStates 获取所有内存中任务的当前状态快照
	// 返回 map[taskId]status，包含父任务和子任务
	GetTaskStates() map[int64]int
}

// isTransientStatus 判断状态是否为瞬态（不会出现在数据库中）
func isTransientStatus(status int) bool {
	switch TaskStatusEnum(status) {
	case TaskStatusCreated, TaskStatusWaiting, TaskStatusProcessing,
		TaskStatusPausing, TaskStatusStopping:
		return true
	default:
		return false
	}
}

// Repository 任务仓储接口（由 service 定义需要的数据库操作方法）
type Repository interface {
	// Create 新建（核心行）
	Create(ctx context.Context, task *entity.Task) error
	// CreateBatch 批量新建（核心行）
	CreateBatch(ctx context.Context, tasks []*entity.Task) error
	// Updates 更新（核心行）
	Updates(ctx context.Context, task *entity.Task) error
	// GetById 根据ID获取（核心行）
	GetById(ctx context.Context, id int64) (*entity.Task, error)
	// List 查询列表
	List(ctx context.Context, opt *database.QueryOption) ([]*entity.Task, error)
	// Count 统计数量
	Count(ctx context.Context, opt *database.QueryOption) (int64, error)
	// Delete 删除
	Delete(ctx context.Context, id int64) error
	// Page 分页查询
	Page(ctx context.Context, opt *database.PageOption) (*model.Page[entity.Task], error)
	// QueryParentPage 分页查询父任务（挂作品任务领域表左连接，支持领域列过滤）
	QueryParentPage(ctx context.Context, opt *database.PageOption) (*model.Page[entity.Task], error)
	// RefreshTaskStatus 刷新任务状态
	RefreshTaskStatus(ctx context.Context, taskId int64) (int64, error)
	// ListTaskTree 获取任务树列表（核心行圈定 + 领域行双查组装）
	ListTaskTree(ctx context.Context, taskIds []int64, includeStatus ...TaskStatusEnum) (*TaskTreeRows, error)
	// SetTaskTreeStatus 设置任务树状态
	SetTaskTreeStatus(ctx context.Context, taskIds []int64, status TaskStatusEnum, includeStatus ...TaskStatusEnum) (int64, error)
	// ListStatus 查询状态列表
	ListStatus(ctx context.Context, ids []int64) ([]*entity.Task, error)
	// CreateTask 创建任务核心行
	CreateTask(ctx context.Context, task *entity.Task) error
	// CreateWorkTaskForTask 为已落库核心行建作品任务领域行（主键覆写为 taskID）
	CreateWorkTaskForTask(ctx context.Context, taskID int64, wt *entity.WorkTask) error
	// CreateWorkTaskBatch 批量建作品任务领域行（各领域行已持核心行共享主键）
	CreateWorkTaskBatch(ctx context.Context, wts []*entity.WorkTask) error
	// SaveWorkTaskForTask 全字段 UPSERT 作品任务领域行（通用编辑端点用）
	SaveWorkTaskForTask(ctx context.Context, taskID int64, wt *entity.WorkTask) error
	// GetWorkTaskById 按共享主键（=所属任务 id）查询作品任务领域行
	GetWorkTaskById(ctx context.Context, taskID int64) (*entity.WorkTask, error)
	// ListWorkTasksByIds 按共享主键集合批量查询作品任务领域行
	ListWorkTasksByIds(ctx context.Context, ids []int64) (map[int64]*entity.WorkTask, error)
	// ListChildrenTask 查询子任务列表
	ListChildrenTask(ctx context.Context, pid int64) ([]*entity.Task, error)
	// QueryChildrenTaskPage 查询子任务分页（挂作品任务领域表左连接）
	QueryChildrenTaskPage(ctx context.Context, opt *database.PageOption) (*model.Page[entity.Task], error)
	// ListSchedule 查询任务进度列表
	ListSchedule(ctx context.Context, ids []int64) ([]*entity.Task, error)
	// DeleteTask 删除任务（包含子任务：领域行先于核心行）- 批量删除，返回全量被删任务 ID 集
	DeleteTask(ctx context.Context, ids []int64) ([]int64, error)
	// ClearResourceTaskId 批量清空资源行对任务及其子任务的 task_id 引用（删除链前置步）
	ClearResourceTaskId(ctx context.Context, ids []int64) error
	// BatchSetStatus 批量设置任务状态（同时更新 error_message）
	BatchSetStatus(ctx context.Context, statuses map[int64]StatusUpdate) error
	// ListBySiteAndSiteWorkID 根据站点和站点作品ID查询关联任务列表
	ListBySiteAndSiteWorkID(ctx context.Context, siteId int64, siteWorkId string) ([]*entity.Task, error)
}

// TaskWithWorkTask 任务核心行与其作品任务领域行的成对载体：创建计划的成员与
// 分页/树查询双查后的组装单元（WorkTask 可为 nil——内置类型任务无作品领域行）
type TaskWithWorkTask struct {
	Task     *entity.Task
	WorkTask *entity.WorkTask
}

// taskProgressTreeBuilder 任务进度树构建器，复用通用 TreeBuilder
var taskProgressTreeBuilder = util.NewTreeBuilder[*dto.TaskProgressTreeDTO](
	func(node *dto.TaskProgressTreeDTO) int64 { return node.TaskProgress.Task.Id },
	func(node *dto.TaskProgressTreeDTO) int64 {
		if node.TaskProgress.Task.Pid != nil {
			return *node.TaskProgress.Task.Pid
		}
		return 0
	},
	0,
)

func setTaskProgressTreeChildren(node *dto.TaskProgressTreeDTO, children []*dto.TaskProgressTreeDTO) {
	node.Children = children
}

// buildTaskProgressTree 将任务成对载体列表构建为 TaskProgressTreeDTO 树形结构
func buildTaskProgressTree(pairs []*TaskWithWorkTask) []*dto.TaskProgressTreeDTO {
	if len(pairs) == 0 {
		return nil
	}
	dtos := make([]*dto.TaskProgressTreeDTO, len(pairs))
	for i, pair := range pairs {
		dtos[i] = dto.NewTaskProgressTreeDTO(dto.AssembleTaskDTO(pair.Task, pair.WorkTask, nil))
	}
	return taskProgressTreeBuilder.BuildTree(dtos, setTaskProgressTreeChildren)
}

// TaskHandlerProvider 任务处理器提供者接口
// 用于获取插件的任务处理器，解耦 task 模块对 plugin 模块的直接依赖
type TaskHandlerProvider interface {
	// GetTaskHandler 获取任务处理器
	GetTaskHandler(pluginPublicId, extensionId string) (sdkdto.TaskHandler, error)
}

// ctxAwareTaskCreator 支持以调用方 ctx 为基创建任务的处理器扩展（TaskHandlerProxy 实现）。
// SDK TaskHandler 接口的 Create 无 ctx 参数，调用方取消语义经此主程序内部接口传递
type ctxAwareTaskCreator interface {
	CreateWithContext(ctx context.Context, url string) (*sdkdto.TaskCreateResult, error)
}

// createTaskWithContext 优先经 ctx 感知通道以调用方 ctx 为基创建任务（取消即终结插件流与
// 接收泵），处理器不支持时回落 SDK 接口的无 ctx Create
func createTaskWithContext(ctx context.Context, handler sdkdto.TaskHandler, url string) (*sdkdto.TaskCreateResult, error) {
	if creator, ok := handler.(ctxAwareTaskCreator); ok {
		return creator.CreateWithContext(ctx, url)
	}
	return handler.Create(url)
}

// Transactor 事务执行器接口
type Transactor interface {
	ExecInTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

// TaskTypeRegistry 已知任务类型提供者（taskManager 持执行面策略表实现；延迟注入解决
// 装配时序——TaskService 先于 TaskManager 创建）。nil 时不校验成员资格（仅非空校验）
type TaskTypeRegistry interface {
	// IsKnownTaskType 任务类型是否已注册（执行面策略表成员 ∪ 插件下载类型）
	IsKnownTaskType(taskType string) bool
}

// RunningStopper 运行态任务停止器（taskManager.Manager 实现；延迟注入解决装配时序——
// TaskService 先于 TaskManager 创建）。任务删除链「先停后删」编排依赖
type RunningStopper interface {
	// StopAndWaitTerminal 停止任务树并等待全部内存目标离开运行态（非运行任务快速直通）；
	// timeout 内未完成返回错误（调用方拒绝本次删除，不强行删除——删行但执行继续属不可预期态）
	StopAndWaitTerminal(ctx context.Context, taskIds []int64, timeout time.Duration) error
}

// deleteStopWaitTimeout 删除链「先停后删」的停止等待上限（与控制命令 ack 有界等待同量级：
// 任务主体逐文件检查点退出 + 终态即时落盘，正常路径远低于此值）
const deleteStopWaitTimeout = 35 * time.Second

// Service 任务服务
type Service struct {
	repo              Repository
	transactor        Transactor
	taskHandlerGetter TaskHandlerProvider
	urlListener       *pluginTaskUrlListener.Service
	siteSvc           *site.Service
	memoryProvider    MemoryStateProvider
	taskTypeRegistry  TaskTypeRegistry
	runningStopper    RunningStopper
	workDirGetter     func() string
}

// NewService 创建任务服务。workDirGetter 供删除链清理下载暂存目录取 workDir
// （空串=未配置，清理函数容忍跳过）
func NewService(repo Repository, transactor Transactor, taskHandlerGetter TaskHandlerProvider, urlListener *pluginTaskUrlListener.Service, siteSvc *site.Service, workDirGetter func() string) *Service {
	return &Service{
		repo:              repo,
		transactor:        transactor,
		taskHandlerGetter: taskHandlerGetter,
		urlListener:       urlListener,
		siteSvc:           siteSvc,
		workDirGetter:     workDirGetter,
	}
}

// SetMemoryProvider 设置内存任务状态提供者（延迟注入，解决初始化顺序问题）
func (s *Service) SetMemoryProvider(provider MemoryStateProvider) {
	s.memoryProvider = provider
}

// SetTaskTypeRegistry 设置已知任务类型提供者（延迟注入：TaskManager 创建后回填）
func (s *Service) SetTaskTypeRegistry(reg TaskTypeRegistry) {
	s.taskTypeRegistry = reg
}

// SetRunningStopper 设置运行态任务停止器（延迟注入：TaskManager 创建后回填）
func (s *Service) SetRunningStopper(stopper RunningStopper) {
	s.runningStopper = stopper
}

// buildPageOptionWithMemory 构建 PageOption，综合内存中的任务状态调整查询条件
// 并挂作品任务领域表左连接（领域列过滤经全限定列名引用）
// 瞬态状态：从内存收集匹配 ID → 清除 Status 条件 → 添加 id IN (匹配IDs)
// 稳态状态：从内存收集不匹配 ID → 保留 Status 条件 → 追加 id NOT IN (不匹配IDs)
func (s *Service) buildPageOptionWithMemory(query TaskQueryDTO, page, pageSize int) (*database.PageOption, error) {
	opt, err := s.buildPageOptionCore(query, page, pageSize)
	if err != nil {
		return nil, err
	}
	opt.Joins = append(opt.Joins, workTaskLeftJoin())
	return opt, nil
}

// buildPageOptionCore 构建 PageOption 的内存状态综合部分（连接子句由外层追加）
func (s *Service) buildPageOptionCore(query TaskQueryDTO, page, pageSize int) (*database.PageOption, error) {
	// 无状态过滤或无内存提供者：标准转换
	if query.Status.Value == nil || s.memoryProvider == nil {
		conv := querypkg.NewConverter(entity.Task{})
		return conv.ToPageOption(query, page, pageSize, nil)
	}

	targetStatus := int(*query.Status.Value)
	states := s.memoryProvider.GetTaskStates()

	if isTransientStatus(targetStatus) {
		// 瞬态：收集内存中匹配的 ID
		var matchingIDs []int64
		for id, state := range states {
			if state == targetStatus {
				matchingIDs = append(matchingIDs, id)
			}
		}

		// 清除 Status 条件（DB 中不存在瞬态）
		query.Status.Value = nil

		conv := querypkg.NewConverter(entity.Task{})
		opt, err := conv.ToPageOption(query, page, pageSize, nil)
		if err != nil {
			return nil, err
		}

		if len(matchingIDs) > 0 {
			vals := make([]interface{}, len(matchingIDs))
			for i, id := range matchingIDs {
				vals[i] = id
			}
			opt.Conditions = append(opt.Conditions, clause.IN{
				Column: clause.Column{Name: "id"},
				Values: vals,
			})
		} else {
			// 无匹配任务，返回永假条件
			opt.Conditions = append(opt.Conditions, clause.Eq{
				Column: clause.Column{Name: "id"}, Value: int64(-1),
			})
		}
		return opt, nil
	}

	// 稳态：收集内存中状态不同的 ID（需排除，防止 DB 旧状态干扰）
	var excludeIDs []interface{}
	for id, state := range states {
		if state != targetStatus {
			excludeIDs = append(excludeIDs, id)
		}
	}

	conv := querypkg.NewConverter(entity.Task{})
	opt, err := conv.ToPageOption(query, page, pageSize, nil)
	if err != nil {
		return nil, err
	}

	if len(excludeIDs) > 0 {
		opt.Conditions = append(opt.Conditions, clause.Not(clause.IN{
			Column: clause.Column{Name: "id"},
			Values: excludeIDs,
		}))
	}
	return opt, nil
}

// overlayMemoryStates 用内存中的实时状态覆写查询结果中的状态
// 使首次加载即显示正确状态，而非等待推送更新
func (s *Service) overlayMemoryStates(tasks []*entity.Task) {
	if s.memoryProvider == nil || len(tasks) == 0 {
		return
	}
	states := s.memoryProvider.GetTaskStates()
	for _, task := range tasks {
		if state, ok := states[task.GetID()]; ok {
			task.Status = state
		}
	}
}

// GetById 根据ID获取：核心行 + 作品任务领域行成对返回（内置类型无作品领域行，WorkTask 为 nil）
func (s *Service) GetById(ctx context.Context, id int64) (*entity.Task, *entity.WorkTask, error) {
	task, err := s.repo.GetById(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	workTask, err := s.repo.GetWorkTaskById(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return task, nil, nil
		}
		return nil, nil, err
	}
	return task, workTask, nil
}

// Save 保存任务：核心行与作品任务领域行成对创建（workTask 为 nil 时仅建核心行）
func (s *Service) Save(ctx context.Context, task *entity.Task, workTask *entity.WorkTask) error {
	return s.transactor.ExecInTransaction(ctx, func(txCtx context.Context) error {
		if err := s.repo.Create(txCtx, task); err != nil {
			return err
		}
		if workTask == nil {
			return nil
		}
		return s.repo.CreateWorkTaskForTask(txCtx, task.GetID(), workTask)
	})
}

// SaveBatch 批量保存任务核心行
func (s *Service) SaveBatch(ctx context.Context, tasks []*entity.Task) error {
	return s.repo.CreateBatch(ctx, tasks)
}

// Update 更新任务：核心行走部分更新；作品任务领域行全字段 UPSERT（为 nil 时仅更新核心行）
func (s *Service) Update(ctx context.Context, task *entity.Task, workTask *entity.WorkTask) error {
	if err := s.repo.Updates(ctx, task); err != nil {
		return err
	}
	if workTask == nil {
		return nil
	}
	return s.repo.SaveWorkTaskForTask(ctx, task.GetID(), workTask)
}

// Delete 删除任务
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// List 查询列表
func (s *Service) List(ctx context.Context, opt *database.QueryOption) ([]*entity.Task, error) {
	return s.repo.List(ctx, opt)
}

// Count 统计数量
func (s *Service) Count(ctx context.Context, opt *database.QueryOption) (int64, error) {
	return s.repo.Count(ctx, opt)
}

// Page 分页查询：核心行分页 + 作品任务领域行批量装配
func (s *Service) Page(ctx context.Context, page *model.Page[entity.Task], query TaskQueryDTO) (*model.Page[TaskWithWorkTask], error) {
	opt, err := s.buildPageOptionWithMemory(query, page.PageNumber, page.PageSize)
	if err != nil {
		return nil, err
	}
	result, err := s.repo.Page(ctx, opt)
	if err != nil {
		return nil, err
	}
	s.overlayMemoryStates(result.Data)
	return s.pairWithWorkTasks(ctx, result)
}

// QueryParentPage 分页查询父任务：核心行分页 + 作品任务领域行批量装配
func (s *Service) QueryParentPage(ctx context.Context, page *model.Page[entity.Task], query TaskQueryDTO) (*model.Page[TaskWithWorkTask], error) {
	opt, err := s.buildPageOptionWithMemory(query, page.PageNumber, page.PageSize)
	if err != nil {
		return nil, err
	}
	result, err := s.repo.QueryParentPage(ctx, opt)
	if err != nil {
		return nil, err
	}
	s.overlayMemoryStates(result.Data)
	return s.pairWithWorkTasks(ctx, result)
}

// pairWithWorkTasks 核心行分页批量装配作品任务领域行（双查组装，无领域行处 nil）
func (s *Service) pairWithWorkTasks(ctx context.Context, page *model.Page[entity.Task]) (*model.Page[TaskWithWorkTask], error) {
	if len(page.Data) == 0 {
		return model.NewPage[TaskWithWorkTask](nil, page.DataCount, page.PageNumber, page.PageSize), nil
	}
	ids := make([]int64, 0, len(page.Data))
	for _, t := range page.Data {
		ids = append(ids, t.GetID())
	}
	workTasks, err := s.repo.ListWorkTasksByIds(ctx, ids)
	if err != nil {
		return nil, err
	}
	data := make([]*TaskWithWorkTask, 0, len(page.Data))
	for _, t := range page.Data {
		data = append(data, &TaskWithWorkTask{Task: t, WorkTask: workTasks[t.GetID()]})
	}
	return model.NewPage[TaskWithWorkTask](data, page.DataCount, page.PageNumber, page.PageSize), nil
}

// RefreshTaskStatus 刷新任务状态
func (s *Service) RefreshTaskStatus(ctx context.Context, taskId int64) (int64, error) {
	return s.repo.RefreshTaskStatus(ctx, taskId)
}

// SetTreeStatus 设置任务树状态
func (s *Service) SetTreeStatus(ctx context.Context, taskIds []int64, status TaskStatusEnum, includeStatus ...TaskStatusEnum) (int64, error) {
	return s.repo.SetTaskTreeStatus(ctx, taskIds, status, includeStatus...)
}

// ListTaskTree 获取任务树列表（核心行 + 领域行双查组装）
func (s *Service) ListTaskTree(ctx context.Context, taskIds []int64, includeStatus ...TaskStatusEnum) (*TaskTreeRows, error) {
	return s.repo.ListTaskTree(ctx, taskIds, includeStatus...)
}

// ListStatus 查询状态列表：核心行批量查 + 作品任务领域行装配为进度 DTO
func (s *Service) ListStatus(ctx context.Context, ids []int64) ([]*dto.TaskProgressDTO, error) {
	tasks, err := s.repo.ListStatus(ctx, ids)
	if err != nil {
		return nil, err
	}
	workTasks, err := s.repo.ListWorkTasksByIds(ctx, ids)
	if err != nil {
		return nil, err
	}
	result := make([]*dto.TaskProgressDTO, len(tasks))
	for i, task := range tasks {
		taskDTO := dto.AssembleTaskDTO(task, workTasks[task.GetID()], nil)
		progressDTO := dto.NewTaskProgressDTO(taskDTO)
		if task.Status == int(TaskStatusFinished) {
			progressDTO.Schedule = new(100)
		}
		result[i] = progressDTO
	}
	return result, nil
}

// CreateTask 创建任务（IPC 入口）：核心行写显式插件下载类型，与作品任务领域行事务内成对落库
func (s *Service) CreateTask(ctx context.Context, req *dto.CreateTaskRequest) (*entity.Task, error) {
	task := &entity.Task{
		BaseEntity: &model.BaseEntity{},
		// pid 外键引用 task.id（无 id=0 行）：req.Pid=0 → NULL=根级任务
		Pid:      sql.NullInt64{Int64: req.Pid, Valid: req.Pid != 0},
		TaskName: sql.NullString{String: req.TaskName, Valid: true},
		HasChild: sql.NullBool{Bool: req.HasChild, Valid: true},
		Status:   int(TaskStatusCreated),
		TaskType: sql.NullString{String: entity.TaskTypePluginDownload, Valid: true},
	}
	// site_id 外键引用 site.id（无 id=0 行）：req.SiteID=0 → NULL=未关联站点。
	// 领域行主键未绑定（落库口 CreateForTask 覆写为核心行 id）
	wt := &entity.WorkTask{BaseEntity: &model.BaseEntity{},
		SiteID:            sql.NullInt64{Int64: int64(req.SiteID), Valid: req.SiteID != 0},
		SiteWorkID:        sql.NullString{String: req.SiteWorkID, Valid: true},
		URL:               sql.NullString{String: req.URL, Valid: true},
		PluginPublicID:    sql.NullString{String: req.PluginPublicID, Valid: true},
		PluginExtensionID: sql.NullString{String: req.PluginExtensionID, Valid: true},
		PluginData:        sql.NullString{String: req.PluginData, Valid: true},
	}
	err := s.transactor.ExecInTransaction(ctx, func(txCtx context.Context) error {
		if err := s.repo.CreateTask(txCtx, task); err != nil {
			return err
		}
		return s.repo.CreateWorkTaskForTask(txCtx, task.GetID(), wt)
	})
	if err != nil {
		return nil, err
	}
	return task, nil
}

// ErrTaskTypeEmpty 创建内置任务时任务类型为空
var ErrTaskTypeEmpty = &pkgerr.BusinessError{Code: 400, Message: "任务类型为空"}

// ErrTaskTypeUnknown 创建内置任务时任务类型未注册（既非执行面策略类型也非插件下载类型）
var ErrTaskTypeUnknown = &pkgerr.BusinessError{Code: 400, Message: "未知的任务类型"}

// validateBuiltinTaskType 校验内置任务类型：非空 + 成员资格（任务类型注册器已注入时；
// nil 注册器跳过成员校验——装配早期/最小测试装配场景）
func (s *Service) validateBuiltinTaskType(taskType string) error {
	if taskType == "" {
		return ErrTaskTypeEmpty
	}
	if s.taskTypeRegistry != nil && !s.taskTypeRegistry.IsKnownTaskType(taskType) {
		return ErrTaskTypeUnknown
	}
	return nil
}

// CreateBuiltinTask 创建内置类型任务（task_type 非插件下载类型，非插件执行；领域载荷由
// 该类型归属方写自有领域表，任务核心行不承载）。taskName 供任务面板展示。
// 创建后停留 Created，启动/暂停/停止等运行控制与插件任务一致（经 taskManager）。
func (s *Service) CreateBuiltinTask(ctx context.Context, taskType string, taskName string) (*entity.Task, error) {
	taskType = strings.TrimSpace(taskType)
	if err := s.validateBuiltinTaskType(taskType); err != nil {
		return nil, err
	}
	task := &entity.Task{
		BaseEntity: &model.BaseEntity{},
		TaskName:   sql.NullString{String: taskName, Valid: true},
		Status:     int(TaskStatusCreated),
		TaskType:   sql.NullString{String: taskType, Valid: true},
		// 内置任务恒为独立叶子任务：has_child 须落 0 而非 NULL——任务树查询以
		// has_child = 0/1 二值圈定 children/parent，NULL 与两分支皆不匹配（行从树查询消失）
		HasChild: sql.NullBool{Bool: false, Valid: true},
	}
	if err := s.repo.CreateTask(ctx, task); err != nil {
		return nil, err
	}
	return task, nil
}

// BuiltinTaskChild 内置任务树子任务入参：任务名。
// children 顺序即子任务展示顺序（任务树查询按创建序返回子任务）；领域载荷由建树调用方
// 持子任务 ID 后写入自有领域表
type BuiltinTaskChild struct {
	TaskName string
}

// ErrBuiltinTaskNoChildren 创建内置任务树时子任务为空
var ErrBuiltinTaskNoChildren = &pkgerr.BusinessError{Code: 400, Message: "子任务为空"}

// ErrBuiltinTaskChildrenNoParent 创建内置任务树子任务时父任务 ID 无效
var ErrBuiltinTaskChildrenNoParent = &pkgerr.BusinessError{Code: 400, Message: "父任务 ID 无效"}

// newBuiltinTaskParent 构造内置任务树父容器实体：has_child=1（Valid=true）、pid=NULL、task_type 落值。
// has_child 落 1 而非 NULL——任务树查询以 has_child=0/1 二值圈定 children/parent，NULL 与两分支
// 皆不匹配（行从树查询消失），同 CreateBuiltinTask 的叶子二值落法。
func newBuiltinTaskParent(taskType string, parentName string) *entity.Task {
	return &entity.Task{
		BaseEntity: &model.BaseEntity{},
		TaskName:   sql.NullString{String: parentName, Valid: true},
		Status:     int(TaskStatusCreated),
		TaskType:   sql.NullString{String: taskType, Valid: true},
		HasChild:   sql.NullBool{Bool: true, Valid: true},
	}
}

// newBuiltinTaskChild 构造内置任务树子任务实体：pid=parentID、has_child=false、task_type 落值。
func newBuiltinTaskChild(taskType string, parentID int64, c BuiltinTaskChild) *entity.Task {
	return &entity.Task{
		BaseEntity: &model.BaseEntity{},
		Pid:        sql.NullInt64{Int64: parentID, Valid: true},
		TaskName:   sql.NullString{String: c.TaskName, Valid: true},
		Status:     int(TaskStatusCreated),
		TaskType:   sql.NullString{String: taskType, Valid: true},
		HasChild:   sql.NullBool{Bool: false, Valid: true},
	}
}

// CreateBuiltinTaskTree 创建内置任务树：1 个父容器（task_type=taskType、has_child=true、pid=NULL）
// + N 个子任务（pid=父任务ID、has_child=false、task_type=taskType）。
// 事务内建父得 ID → 回填子 pid；任一步失败整体回滚。返回父任务（含 ID）。
// 父容器为纯聚合节点（has_child=1），无执行面；子任务各自独立执行。
func (s *Service) CreateBuiltinTaskTree(ctx context.Context, taskType string, parentName string, children []BuiltinTaskChild) (*entity.Task, error) {
	taskType = strings.TrimSpace(taskType)
	if err := s.validateBuiltinTaskType(taskType); err != nil {
		return nil, err
	}
	if len(children) == 0 {
		return nil, ErrBuiltinTaskNoChildren
	}
	parent := newBuiltinTaskParent(taskType, parentName)
	err := s.transactor.ExecInTransaction(ctx, func(txCtx context.Context) error {
		if err := s.repo.CreateTask(txCtx, parent); err != nil {
			return err
		}
		parentID := parent.GetID()
		for _, c := range children {
			if err := s.repo.CreateTask(txCtx, newBuiltinTaskChild(taskType, parentID, c)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return parent, nil
}

// CreateBuiltinTaskParent 创建内置任务树父容器（has_child=true、pid=NULL、task_type 落值）。
// 单独建父供调用方在「子任务入参依赖父任务 ID」的场景——如收件建树先建父任务、落盘共享清单
// 到父任务目录、再建子任务（子任务领域行经父目录路径定位清单）；失败回滚由调用方显式 DeleteTask 整树。
func (s *Service) CreateBuiltinTaskParent(ctx context.Context, taskType string, parentName string) (*entity.Task, error) {
	taskType = strings.TrimSpace(taskType)
	if err := s.validateBuiltinTaskType(taskType); err != nil {
		return nil, err
	}
	parent := newBuiltinTaskParent(taskType, parentName)
	if err := s.repo.CreateTask(ctx, parent); err != nil {
		return nil, err
	}
	return parent, nil
}

// CreateBuiltinTaskChildren 在既有父任务下创建内置任务树子任务（pid=parentID、has_child=false、
// task_type 落值），返回创建的子任务（含 ID，供调用方写各自领域行）。children 顺序即子任务展示顺序。
// 非事务——调用方（share Receive 建树）在建子失败时自行 DeleteTask 回滚整树（显式删树语义）。
func (s *Service) CreateBuiltinTaskChildren(ctx context.Context, taskType string, parentID int64, children []BuiltinTaskChild) ([]*entity.Task, error) {
	taskType = strings.TrimSpace(taskType)
	if err := s.validateBuiltinTaskType(taskType); err != nil {
		return nil, err
	}
	if parentID <= 0 {
		return nil, ErrBuiltinTaskChildrenNoParent
	}
	if len(children) == 0 {
		return nil, ErrBuiltinTaskNoChildren
	}
	created := make([]*entity.Task, 0, len(children))
	for _, c := range children {
		child := newBuiltinTaskChild(taskType, parentID, c)
		if err := s.repo.CreateTask(ctx, child); err != nil {
			return nil, err
		}
		created = append(created, child)
	}
	return created, nil
}

// DeleteTask 删除任务（包含子任务）- 批量删除
// 删除即放弃执行：无条件先经停止器「停止 + 有界等待终态」再进删除链（暂存即时清理等收尾
// 自然落在停止完成后）——运行态是瞬态、内存权威、不落库，运行判定由停止器按内存目标集
// 自查，非运行任务（不在内存或已终态，含建树回滚删 Created 态树）在其内部快速直通；等待
// 超时拒绝删除（提示稍后重试，不强行删除——删行但执行继续属不可预期态）。
// 事务内先清 resource.task_id 引用再删任务行：外键强制下引用未清即删行被拒（NULL=非任务产）。
// 提交后清理被删任务（含子任务）的下载暂存目录——暂存目录按任务 ID 派生，生命周期与任务行一致，
// 任务行消亡即失去归属；删除时即时清理，不等启动清扫兜底
func (s *Service) DeleteTask(ctx context.Context, ids []int64) error {
	if s.runningStopper != nil {
		if err := s.runningStopper.StopAndWaitTerminal(ctx, ids, deleteStopWaitTimeout); err != nil {
			return err
		}
	}
	var deletedIds []int64
	if err := s.transactor.ExecInTransaction(ctx, func(txCtx context.Context) error {
		if err := s.repo.ClearResourceTaskId(txCtx, ids); err != nil {
			return err
		}
		deleted, err := s.repo.DeleteTask(txCtx, ids)
		if err != nil {
			return err
		}
		deletedIds = deleted
		return nil
	}); err != nil {
		return err
	}
	// 暂存清理是文件系统副作用，置于事务提交后：事务回滚时任务行仍在，暂存须留给恢复判定。
	// 清理失败不阻断删除——任务行已消亡，残留暂存由启动清扫按任务行缺失回收
	if s.workDirGetter != nil {
		if cerr := CleanupStagingByTaskIds(s.workDirGetter(), deletedIds); cerr != nil {
			logger.Log.Warnf("清理被删任务下载暂存目录失败: %v", cerr)
		}
	}
	return nil
}

// QueryTreeDataPage 查询任务树数据分页
func (s *Service) QueryTreeDataPage(ctx context.Context, page, pageSize int, queryDTO *TaskQueryDTO) (*dto.TreeDataPageDTO, error) {
	conv := querypkg.NewConverter(entity.Task{})
	opt, err := conv.ToPageOption(queryDTO, page, pageSize, nil)
	if err != nil {
		return nil, err
	}
	opt.Joins = append(opt.Joins, workTaskLeftJoin())

	// 分页查询父任务（has_child=1 OR pid IS NULL，根级任务 pid=NULL）+ 领域行装配
	resultPage, err := s.repo.QueryParentPage(ctx, opt)
	if err != nil {
		return nil, err
	}
	pairPage, err := s.pairWithWorkTasks(ctx, resultPage)
	if err != nil {
		return nil, err
	}

	// 将分页数据构建为 TaskProgressTreeDTO 树
	tree := buildTaskProgressTree(pairPage.Data)

	// 获取 TreeID 和 TreeName（从分页数据中获取）
	var treeID int64
	var treeName string
	if len(pairPage.Data) > 0 {
		treeID = pairPage.Data[0].Task.GetID()
		treeName = pairPage.Data[0].Task.TaskName.String
	}

	return &dto.TreeDataPageDTO{
		TreeID:   treeID,
		TreeName: treeName,
		Total:    pairPage.DataCount,
		Tasks:    tree,
	}, nil
}

// ListChildrenTask 查询子任务列表：核心行 + 作品任务领域行成对装配
func (s *Service) ListChildrenTask(ctx context.Context, pid int64) ([]*TaskWithWorkTask, error) {
	tasks, err := s.repo.ListChildrenTask(ctx, pid)
	if err != nil {
		return nil, err
	}
	return s.attachWorkTasks(ctx, tasks)
}

// ListBySiteAndSiteWorkID 根据站点和站点作品ID查询关联任务列表（核心行 + 领域行成对装配）
func (s *Service) ListBySiteAndSiteWorkID(ctx context.Context, siteId int64, siteWorkId string) ([]*TaskWithWorkTask, error) {
	tasks, err := s.repo.ListBySiteAndSiteWorkID(ctx, siteId, siteWorkId)
	if err != nil {
		return nil, err
	}
	return s.attachWorkTasks(ctx, tasks)
}

// attachWorkTasks 为核心行列表批量装配作品任务领域行（无领域行处 nil）
func (s *Service) attachWorkTasks(ctx context.Context, tasks []*entity.Task) ([]*TaskWithWorkTask, error) {
	if len(tasks) == 0 {
		return make([]*TaskWithWorkTask, 0), nil
	}
	ids := make([]int64, 0, len(tasks))
	for _, t := range tasks {
		ids = append(ids, t.GetID())
	}
	workTasks, err := s.repo.ListWorkTasksByIds(ctx, ids)
	if err != nil {
		return nil, err
	}
	pairs := make([]*TaskWithWorkTask, 0, len(tasks))
	for _, t := range tasks {
		pairs = append(pairs, &TaskWithWorkTask{Task: t, WorkTask: workTasks[t.GetID()]})
	}
	return pairs, nil
}

// QueryChildrenTaskPage 查询子任务分页：核心行分页 + 作品任务领域行批量装配
func (s *Service) QueryChildrenTaskPage(ctx context.Context, page *model.Page[entity.Task], query TaskQueryDTO) (*model.Page[TaskWithWorkTask], error) {
	opt, err := s.buildPageOptionWithMemory(query, page.PageNumber, page.PageSize)
	if err != nil {
		return nil, err
	}
	result, err := s.repo.QueryChildrenTaskPage(ctx, opt)
	if err != nil {
		return nil, err
	}
	s.overlayMemoryStates(result.Data)
	return s.pairWithWorkTasks(ctx, result)
}

// EnrichTaskProgressTreePage 将核心行+领域行成对分页丰富为 TaskProgressTreeDTO 分页
// 批量查询站点名称并注入，同时填充树形结构字段（hasChildren、children、isLeaf）
func (s *Service) EnrichTaskProgressTreePage(ctx context.Context, rawPage *model.Page[TaskWithWorkTask]) (*model.Page[dto.TaskProgressTreeDTO], error) {
	pairs := rawPage.Data
	if len(pairs) == 0 {
		return model.NewPage[dto.TaskProgressTreeDTO](nil, rawPage.DataCount, rawPage.PageNumber, rawPage.PageSize), nil
	}

	// 1. 收集 siteIds（去重）——站点身份在作品任务领域行
	siteIdSet := make(map[int64]struct{})
	for _, pair := range pairs {
		if pair.WorkTask != nil && pair.WorkTask.SiteID.Valid && pair.WorkTask.SiteID.Int64 > 0 {
			siteIdSet[pair.WorkTask.SiteID.Int64] = struct{}{}
		}
	}

	// 2. 批量查询站点名称，构建 id→siteName 映射
	siteNameMap := make(map[int64]string)
	if len(siteIdSet) > 0 {
		siteIds := make([]int64, 0, len(siteIdSet))
		for id := range siteIdSet {
			siteIds = append(siteIds, id)
		}
		sites, err := s.siteSvc.ListByIds(ctx, siteIds)
		if err != nil {
			return nil, err
		}
		for _, site := range sites {
			if site.SiteName.Valid {
				siteNameMap[site.GetID()] = site.SiteName.String
			}
		}
	}

	// 3. 转换并丰富
	data := make([]*dto.TaskProgressTreeDTO, 0, len(pairs))
	for _, pair := range pairs {
		taskDTO := dto.AssembleTaskDTO(pair.Task, pair.WorkTask, nil)
		treeDTO := dto.NewTaskProgressTreeDTO(taskDTO)
		// 注入站点名称
		if pair.WorkTask != nil && pair.WorkTask.SiteID.Valid {
			if siteName, ok := siteNameMap[pair.WorkTask.SiteID.Int64]; ok {
				treeDTO.TaskProgress.SiteName = &siteName
			}
		}
		data = append(data, treeDTO)
	}

	return model.NewPage[dto.TaskProgressTreeDTO](data, rawPage.DataCount, rawPage.PageNumber, rawPage.PageSize), nil
}

// ListSchedule 查询任务进度列表
func (s *Service) ListSchedule(ctx context.Context, ids []int64) ([]*dto.TaskProgressDTO, error) {
	return s.ListStatus(ctx, ids)
}

// CreateTaskByURLRequest 根据URL创建任务的请求
type CreateTaskByURLRequest struct {
	URL string `json:"url" binding:"required"`
	// ChosenPluginPublicId 与 ChosenExtensionId 为交互面显选键：两键联合定位一个扩展点候选，
	// 均空 = 未显选（程序化入口恒为此态）
	ChosenPluginPublicId string `json:"chosenPluginPublicId"`
	ChosenExtensionId    string `json:"chosenExtensionId"`
}

// CreateTaskByURLResponse 根据URL创建任务的响应
type CreateTaskByURLResponse struct {
	Succeed       bool   `json:"succeed"`
	AddedQuantity int    `json:"addedQuantity"`
	Msg           string `json:"msg"`
	// Conflict 为真表示该 URL 命中了多个候选扩展点且本次触发未显选：未调用任何插件，
	// 由调用方发起选择后带显选键重发；ConflictCandidates 为候选清单，首位即默认选中项
	Conflict           bool                   `json:"conflict"`
	ConflictCandidates []*dto.PluginCandidate `json:"conflictCandidates"`
}

// taskEntry 建任务触发的形态：交互面可在多候选未显选时发起显选，程序化入口无此通道。
type taskEntry int

const (
	// taskEntryProgrammatic 程序化触发（宿主 CreateTask、插件经宿主建任务）：恒不问，按候选全键字典序尝试
	taskEntryProgrammatic taskEntry = iota
	// taskEntryInteractive 交互触发（前端手动建任务）：多候选且未显选时返回冲突载荷，由调用方选择后带键重发
	taskEntryInteractive
)

// taskChoice 交互面的显选键：两键联合定位候选集内的一个扩展点候选
type taskChoice struct {
	PluginPublicId string
	ExtensionId    string
}

// hasSelection 本次触发是否携带显选键（两键均空 = 未显选）
func (c taskChoice) hasSelection() bool {
	return c.PluginPublicId != "" || c.ExtensionId != ""
}

// CreateTaskByURL 根据传入的url创建任务（程序化入口：宿主 CreateTask、插件经宿主建任务）。
// 通过 URL 监听器发现能处理此 URL 的扩展点候选（粒度 = 插件 × extensionId），按候选全键字典序
// 逐个尝试，调用插件的 create 方法创建任务。任一候选出现失败信号（处理器不可用/插件错误返回/零任务）
// 即终止并返回原因提示，不再尝试后续候选；监听器缺插件 PublicID 属注册数据缺陷而非插件运行结果，
// 跳过后仍继续。
func (s *Service) CreateTaskByURL(ctx context.Context, url string) (*CreateTaskByURLResponse, error) {
	return s.createTaskByURL(ctx, url, taskEntryProgrammatic, taskChoice{})
}

// CreateTaskByURLWithChoice 交互触发（前端手动建任务）的建任务入口：携带显选键时该扩展点候选置于
// 路由首位；未携带且候选多于一个时返回冲突载荷（Conflict=true + 候选清单）且不调用任何插件。
func (s *Service) CreateTaskByURLWithChoice(ctx context.Context, url, chosenPluginPublicId, chosenExtensionId string) (*CreateTaskByURLResponse, error) {
	choice := taskChoice{PluginPublicId: chosenPluginPublicId, ExtensionId: chosenExtensionId}
	return s.createTaskByURL(ctx, url, taskEntryInteractive, choice)
}

// createTaskByURL 两入口共用的执行核：发现候选 → 交互面显选校验与冲突收口 → 经路由基座按序尝试。
func (s *Service) createTaskByURL(ctx context.Context, url string, entry taskEntry, choice taskChoice) (*CreateTaskByURLResponse, error) {
	listeners := s.urlListener.ListListener(url)
	logger.Log.Infof("[CreateTaskByURL] url=%s 匹配监听器 %d 个", url, len(listeners))
	if len(listeners) == 0 {
		logger.Log.Warnf("[CreateTaskByURL] 无监听器匹配(插件未注册该 URL 类型 或 插件未激活): %s", url)
		return &CreateTaskByURLResponse{
			Succeed:       false,
			AddedQuantity: 0,
			Msg:           fmt.Sprintf("url不受支持，url: %s", url),
		}, nil
	}

	candidates := orderedRoutableCandidates(listeners)

	if entry == taskEntryInteractive {
		switch {
		case choice.hasSelection():
			if !containsCandidate(candidates, choice) {
				return nil, errors.Join(ErrChosenCandidateInvalid,
					fmt.Errorf("plugin=%s extensionId=%s", choice.PluginPublicId, choice.ExtensionId))
			}
		case len(candidates) >= 2:
			return &CreateTaskByURLResponse{
				Succeed:            false,
				AddedQuantity:      0,
				Msg:                "该链接可由多个插件处理，请选择插件",
				Conflict:           true,
				ConflictCandidates: toPluginCandidates(candidates),
			}, nil
		}
	}

	resp, err := route.Route(ctx, &taskRouteAdapter{service: s, url: url, candidates: candidates, chosen: choice})
	if err != nil {
		var failure *candidateFailure
		if errors.As(err, &failure) {
			return failure.resp, nil
		}
		logger.Log.Warnf("[CreateTaskByURL] 无候选可路由 url=%s: %v", url, err)
		return &CreateTaskByURLResponse{
			Succeed:       false,
			AddedQuantity: 0,
			Msg:           fmt.Sprintf("尝试了所有插件均未成功，url: %s", url),
		}, nil
	}
	return resp, nil
}

// orderedRoutableCandidates 筛出可路由候选（扩展点粒度）并按候选全键字典序排列。缺插件 PublicID 的
// 监听器条目无法定位插件与任务处理器，属注册数据缺陷而非插件运行结果，剔除。
// 该序即冲突载荷的默认序，与基座路由序同源（同用 candidateOrderKey）。
func orderedRoutableCandidates(listeners []*pluginTaskUrlListener.PluginWithExtension) []*pluginTaskUrlListener.PluginWithExtension {
	candidates := make([]*pluginTaskUrlListener.PluginWithExtension, 0, len(listeners))
	for _, listener := range listeners {
		if !listener.PublicID.Valid || listener.PublicID.String == "" {
			logger.Log.Warnf("URL监听器缺少插件 PublicID，跳过 (extensionId=%s)", listener.ExtensionID)
			continue
		}
		candidates = append(candidates, listener)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidateOrderKey(candidates[i].PublicID.String, candidates[i].ExtensionID) <
			candidateOrderKey(candidates[j].PublicID.String, candidates[j].ExtensionID)
	})
	return candidates
}

// candidateOrderKey 候选全键：插件 ID 与扩展点 ID 以 NUL 拼接。NUL 不在标识符取值域内，
// ("a","bc") 与 ("ab","c") 不会撞键。
func candidateOrderKey(pluginPublicId, extensionId string) string {
	return pluginPublicId + "\x00" + extensionId
}

// containsCandidate 显选键是否命中候选集（两键联合匹配一个扩展点候选）
func containsCandidate(candidates []*pluginTaskUrlListener.PluginWithExtension, choice taskChoice) bool {
	for _, candidate := range candidates {
		if candidate.PublicID.String == choice.PluginPublicId && candidate.ExtensionID == choice.ExtensionId {
			return true
		}
	}
	return false
}

// toPluginCandidates 候选清单转为共享 DTO（按候选全键字典序，首位即默认选中项）
func toPluginCandidates(candidates []*pluginTaskUrlListener.PluginWithExtension) []*dto.PluginCandidate {
	result := make([]*dto.PluginCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		result = append(result, &dto.PluginCandidate{
			PluginPublicId: candidate.PublicID.String,
			PluginName:     listenerPluginName(candidate),
			ExtensionId:    candidate.ExtensionID,
		})
	}
	return result
}

// taskRouteAdapter 任务创建面的路由接入：URL 监听器即候选发现，调用插件 create 并消费其返回落库
// （成功后的字段填充与落库链属本消费面内部事务）。该消费面没有协议级不适配信号（正则命中即预期处理），
// 故一切失败皆判为真失败。
type taskRouteAdapter struct {
	service    *Service
	url        string
	candidates []*pluginTaskUrlListener.PluginWithExtension
	chosen     taskChoice
}

func (a *taskRouteAdapter) Candidates(_ context.Context) ([]*pluginTaskUrlListener.PluginWithExtension, error) {
	return a.candidates, nil
}

// OrderKey 候选排序键：显选候选前缀 "0"、其余前缀 "1"，令显选项排在其前。前缀定长且施加于全体候选，
// 组内相对序仍为候选全键字典序。
func (a *taskRouteAdapter) OrderKey(c *pluginTaskUrlListener.PluginWithExtension) string {
	if a.chosen.hasSelection() && c.PublicID.String == a.chosen.PluginPublicId && c.ExtensionID == a.chosen.ExtensionId {
		return "0" + candidateOrderKey(c.PublicID.String, c.ExtensionID)
	}
	return "1" + candidateOrderKey(c.PublicID.String, c.ExtensionID)
}

// Describe 候选点名：插件展示名 + 扩展点 ID，定位一个路由原子
func (a *taskRouteAdapter) Describe(c *pluginTaskUrlListener.PluginWithExtension) string {
	return fmt.Sprintf("%s/%s", listenerPluginName(c), c.ExtensionID)
}

// Invoke 取候选的任务处理器并消费其返回：流与数组两路径统一落库，产出任务的候选即路由终点。
// 处理器不可用、传输层错误、消费失败与零任务均为真失败，响应文案经 candidateFailure 原样交回调用方。
func (a *taskRouteAdapter) Invoke(ctx context.Context, c *pluginTaskUrlListener.PluginWithExtension) (*CreateTaskByURLResponse, route.Outcome, error) {
	pluginPublicId := c.PublicID.String
	pluginName := listenerPluginName(c)

	// 获取任务处理器：失败即插件不可运行，终止
	taskHandler, err := a.service.taskHandlerGetter.GetTaskHandler(pluginPublicId, c.ExtensionID)
	if err != nil {
		logger.Log.Warnf("获取任务处理器失败 (plugin=%s, extensionId=%s): %v", pluginPublicId, c.ExtensionID, err)
		return nil, route.Failed, failedCandidate(fmt.Sprintf("插件 %s 未激活或任务处理器不可用", pluginName))
	}

	// 调用插件的 create 方法。gRPC 层错误代表基础设施故障（进程崩溃/连接中断/传输异常）；
	// 插件业务失败原因经结果对象的 reason 承载，不表现为 err。
	// 经 ctx 感知通道继承调用方 ctx：取消可终结建流与接收泵
	result, err := createTaskWithContext(ctx, taskHandler, a.url)
	if err != nil {
		logger.Log.Errorf("插件创建任务失败 (plugin=%s): %v", pluginPublicId, err)
		return nil, route.Failed, failedCandidate(fmt.Sprintf("插件 %s 创建任务失败（异常退出或连接中断）：%v", pluginName, err))
	}

	// 处理插件返回，统计成功落库的叶子单元数与失败项数
	var count, failed int
	if result.IsStream() {
		streamCh, streamErr := a.service.handleCreateTaskStream(ctx, result.Stream(), c, 100)
		if streamErr != nil {
			logger.Log.Errorf("处理流式任务失败 (plugin=%s): %v", pluginPublicId, streamErr)
			return nil, route.Failed, failedCandidate(fmt.Sprintf("插件 %s 创建任务失败：%v", pluginName, streamErr))
		}
		// 计数叶子级单元：leaf 与 child（Task 项），parent 容器（Parent 项）不计；
		// 单项字段填充或落库失败以 Error 项计入失败数
		for item := range streamCh {
			if item.Task != nil {
				count++
			} else if item.Error != nil {
				failed++
			}
		}
	} else {
		responses := result.Array()
		if len(responses) > 0 {
			var arrayErr error
			count, failed, arrayErr = a.service.handleCreateTaskArray(ctx, responses, c)
			if arrayErr != nil {
				logger.Log.Errorf("处理插件返回数据失败 (plugin=%s): %v", pluginPublicId, arrayErr)
				return nil, route.Failed, failedCandidate(fmt.Sprintf("插件 %s 创建任务失败：%v", pluginName, arrayErr))
			}
		}
	}

	// reason（插件声明的业务原因）须在流消费完毕（channel close）之后读取
	reason := result.Reason()

	if count > 0 {
		// 有任务落库即成功；失败数为落库维度、插件报告的原因为业务维度，分层表述
		msg := fmt.Sprintf("成功创建 %d 个任务", count)
		if failed > 0 {
			msg = fmt.Sprintf("成功创建 %d 个任务，%d 个失败（详见日志）", count, failed)
			if reason != "" {
				msg += fmt.Sprintf("；插件报告：%s", reason)
			}
		}
		return &CreateTaskByURLResponse{Succeed: true, AddedQuantity: count, Msg: msg}, route.Succeeded, nil
	}

	if reason != "" {
		return nil, route.Failed, failedCandidate(fmt.Sprintf("插件 %s 未创建任务：%s", pluginName, reason))
	}
	return nil, route.Failed, failedCandidate(fmt.Sprintf("插件 %s 未返回任务，也未说明原因", pluginName))
}

// candidateFailure 候选失败的收口载体：resp 即面向调用方的最终响应（Succeed=false + 文案）。
// 基座把失败原因包进自身的候选点名文案，故须经本类型把响应原样交回，文案不带基座前缀。
type candidateFailure struct{ resp *CreateTaskByURLResponse }

func (e *candidateFailure) Error() string { return e.resp.Msg }

// failedCandidate 以面向调用方的失败文案构造候选失败
func failedCandidate(msg string) *candidateFailure {
	return &candidateFailure{resp: &CreateTaskByURLResponse{Succeed: false, AddedQuantity: 0, Msg: msg}}
}

// listenerPluginName URL 监听器条目的插件展示名，用于提示文案点名插件；插件未设置名时回退 publicId。
func listenerPluginName(listener *pluginTaskUrlListener.PluginWithExtension) string {
	if name := listener.Plugin.Name.String; name != "" {
		return name
	}
	return listener.PublicID.String
}

// createPlan 一个 TaskCreateResponse 经单点判定后的创建计划（成员均为核心行+作品领域行成对载体）。
// leaf 与 parent 互斥：无 Children 时 leaf 非空（独立任务）；有 Children 时 parent+children。
// children 的 Pid 待调用方落盘 parent 后回填。
type createPlan struct {
	leaf     *TaskWithWorkTask
	parent   *TaskWithWorkTask
	children []*TaskWithWorkTask
}

// count 此计划贡献的叶子级任务计数（leaf=1；parent+N=N，parent 容器不计）。
func (p *createPlan) count() int {
	if p.leaf != nil {
		return 1
	}
	return len(p.children)
}

// childToResponse 把子响应适配为 TaskCreateResponse，复用 fillTaskFromResponse 的统一字段映射。
// 子应答契约无 siteKey 字段，站点归属继承父应答的键——同一父任务的子任务必属同站点。
func childToResponse(c *sdkdto.TaskCreateChildResponse, parentSiteKey string) *sdkdto.TaskCreateResponse {
	return &sdkdto.TaskCreateResponse{
		TaskName:      c.TaskName,
		SiteWorkId:    c.SiteWorkId,
		Url:           c.Url,
		SiteName:      c.SiteName,
		SiteKey:       parentSiteKey,
		PluginData:    c.PluginData,
		InvolvedRoles: c.InvolvedRoles,
		ResourceType:  c.ResourceType,
	}
}

// fillTaskFromResponse 把响应字段填入一个成对载体（leaf/parent/child 通用，双路径共用）：
// 控制字段落核心行（task_type=插件下载类型），领域字段落作品任务领域行（主键在落库口绑定核心行 id）。
// pid：父任务 ID（child 传 parent.id；leaf/parent 传 0）。pid=0 写 NULL=根级任务（外键引用 task.id，无 id=0 行）。
// hasChild：是否父任务（容器，不带 SiteWorkID/PluginData）。
// siteCache：站点键→ID 缓存（调用方持有，跨任务复用，避免重复查库）。
// SiteKey 为空时返回 ErrSiteKeyRequired——leaf/parent/child 均须归属站点（child 的键由
// childToResponse 继承父应答）。
func (s *Service) fillTaskFromResponse(ctx context.Context, pair *TaskWithWorkTask, resp *sdkdto.TaskCreateResponse, listener *pluginTaskUrlListener.PluginWithExtension, pid int64, hasChild bool, siteCache map[string]int) error {
	task, wt := pair.Task, pair.WorkTask
	task.TaskName = sql.NullString{String: resp.TaskName, Valid: true}
	task.Status = int(TaskStatusCreated)
	task.HasChild = sql.NullBool{Bool: hasChild, Valid: true}
	task.TaskType = sql.NullString{String: entity.TaskTypePluginDownload, Valid: true}
	// pid=0 → NULL=根级任务（外键引用 task.id，无 id=0 行，写 0 必违约）；child 落盘前由调用方回填父 ID
	task.Pid = sql.NullInt64{Int64: pid, Valid: pid != 0}
	wt.URL = sql.NullString{String: resp.Url, Valid: true}
	wt.PluginPublicID = listener.PublicID
	wt.PluginExtensionID = sql.NullString{String: listener.ExtensionID, Valid: true}

	if resp.SiteKey == "" {
		return errors.Join(ErrSiteKeyRequired, errors.New("siteKey is empty"))
	}
	siteId, ok := siteCache[resp.SiteKey]
	if !ok {
		site, err := s.siteSvc.GetByKey(ctx, resp.SiteKey)
		if err != nil || site == nil {
			return errors.Join(ErrSiteNotFound, errors.New(resp.SiteKey))
		}
		siteId = int(site.ID)
		siteCache[resp.SiteKey] = siteId
	}
	wt.SiteID = sql.NullInt64{Int64: int64(siteId), Valid: true}

	// 身份字段：leaf/child 带 SiteWorkID/PluginData；parent 容器不带
	if !hasChild {
		wt.SiteWorkID = sql.NullString{String: resp.SiteWorkId, Valid: true}
		if resp.PluginData != "" {
			wt.PluginData = sql.NullString{String: resp.PluginData, Valid: true}
		}
	}

	// involvedRoles:创建期声明的涉及板块(universe),逗号join;空=NULL(未确定/默认)
	if len(resp.InvolvedRoles) > 0 {
		wt.InvolvedRoles = sql.NullString{String: strings.Join(resp.InvolvedRoles, ","), Valid: true}
	}

	// resourceType:创建期声明的资源类型(预定义值);空=NULL(未声明);有 children 时由各 child 声明
	if resp.ResourceType != "" {
		wt.ResourceType = sql.NullString{String: resp.ResourceType, Valid: true}
	}

	return nil
}

// planCreateResponse 把一个插件响应单点判定为 leaf 或 parent+children 并填好字段（children 的 Pid 除外）。
// stream 与 array 共用此方法——leaf/parent/child 三态在此唯一实现，消除双路径不对称：
// 无 Children → 独立 leaf（pid=NULL 根级）；有 Children → parent+children，不折叠（Children=[1] 也建 parent+child）。
// 不变量：改此函数须保 leaf(pid=NULL 根级) 路径不被遗漏/折叠，参见 memory leaf-task-regression-hotspot。
// 通信契约详见 doc/plugin-dev-guide.md「Create 返回的任务结构契约」。
func (s *Service) planCreateResponse(ctx context.Context, taskResp *sdkdto.TaskCreateResponse, listener *pluginTaskUrlListener.PluginWithExtension, siteCache map[string]int) (*createPlan, error) {
	if len(taskResp.Children) == 0 {
		// 无 Children：独立 leaf（如 local 单文件导入），pid=NULL（根级）、HasChild=false。
		// 领域行主键未绑定（落库口 CreateForTask 覆写为核心行 id），BaseEntity 须显式初始化
		leaf := &TaskWithWorkTask{
			Task:     &entity.Task{BaseEntity: &model.BaseEntity{}},
			WorkTask: &entity.WorkTask{BaseEntity: &model.BaseEntity{}},
		}
		if err := s.fillTaskFromResponse(ctx, leaf, taskResp, listener, 0, false, siteCache); err != nil {
			return nil, err
		}
		return &createPlan{leaf: leaf}, nil
	}

	// 有 Children：parent + 每个 child，不折叠
	parent := &TaskWithWorkTask{
		Task:     &entity.Task{BaseEntity: &model.BaseEntity{}},
		WorkTask: &entity.WorkTask{BaseEntity: &model.BaseEntity{}},
	}
	if err := s.fillTaskFromResponse(ctx, parent, taskResp, listener, 0, true, siteCache); err != nil {
		return nil, err
	}
	children := make([]*TaskWithWorkTask, 0, len(taskResp.Children))
	for _, childResp := range taskResp.Children {
		child := &TaskWithWorkTask{
			Task:     &entity.Task{BaseEntity: &model.BaseEntity{}},
			WorkTask: &entity.WorkTask{BaseEntity: &model.BaseEntity{}},
		}
		if err := s.fillTaskFromResponse(ctx, child, childToResponse(childResp, taskResp.SiteKey), listener, 0, false, siteCache); err != nil {
			return nil, err
		}
		children = append(children, child)
	}
	return &createPlan{parent: parent, children: children}, nil
}

// persistPlanPair 事务内成对落库一个计划成员：核心行拿 id → 领域行绑定同值共享主键
func (s *Service) persistPlanPair(ctx context.Context, pair *TaskWithWorkTask) error {
	return s.transactor.ExecInTransaction(ctx, func(txCtx context.Context) error {
		if err := s.repo.CreateTask(txCtx, pair.Task); err != nil {
			return err
		}
		return s.repo.CreateWorkTaskForTask(txCtx, pair.Task.GetID(), pair.WorkTask)
	})
}

// handleCreateTaskArray 处理插件返回的任务数组。
// 经 planCreateResponse 单点判定：无 Children→独立 leaf；有 Children→parent+children（不折叠）。
// 返回成功落库的叶子级任务计数（leaf=1、parent+N=N，parent 容器不计）与失败项计数
// （字段填充失败或落库失败的响应，按叶子单元口径折算，见 responseLeafUnits）。
func (s *Service) handleCreateTaskArray(ctx context.Context, pluginResponses []*sdkdto.TaskCreateResponse, listener *pluginTaskUrlListener.PluginWithExtension) (int, int, error) {
	if len(pluginResponses) == 0 {
		return 0, 0, nil
	}

	count := 0
	failed := 0
	siteCache := make(map[string]int) // siteKey -> siteId 缓存

	for _, resp := range pluginResponses {
		plan, err := s.planCreateResponse(ctx, resp, listener, siteCache)
		if err != nil {
			// 字段填充失败（如 SiteKey 缺失/未注册）：此响应的全部叶子单元计为失败
			logger.Log.Errorf("[Task] 插件响应字段填充失败，跳过 (plugin=%s, taskName=%s): %v", listener.PublicID.String, resp.TaskName, err)
			failed += responseLeafUnits(resp)
			continue
		}

		if plan.leaf != nil {
			// 独立 leaf：事务内成对落盘
			if err := s.persistPlanPair(ctx, plan.leaf); err != nil {
				logger.Log.Errorf("[Task] 创建独立任务失败 (plugin=%s, taskName=%s): %v", listener.PublicID.String, plan.leaf.Task.TaskName.String, err)
				failed += plan.count()
				continue
			}
			count += plan.count()
			continue
		}

		// parent + children：事务内落盘 parent → 回填 children.Pid → 成对落盘 children
		err = s.transactor.ExecInTransaction(ctx, func(txCtx context.Context) error {
			if err := s.repo.CreateTask(txCtx, plan.parent.Task); err != nil {
				return err
			}
			parentId := plan.parent.Task.GetID()
			if err := s.repo.CreateWorkTaskForTask(txCtx, parentId, plan.parent.WorkTask); err != nil {
				return err
			}
			for _, child := range plan.children {
				child.Task.Pid = sql.NullInt64{Int64: parentId, Valid: true}
				if err := s.repo.CreateTask(txCtx, child.Task); err != nil {
					return err
				}
				if err := s.repo.CreateWorkTaskForTask(txCtx, child.Task.GetID(), child.WorkTask); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			logger.Log.Errorf("[Task] 创建任务组失败: %v", err)
			failed += plan.count()
			continue
		}
		count += plan.count()
	}

	return count, failed, nil
}

// responseLeafUnits 一个插件响应对应的叶子单元数（无 Children 记 1，有 Children 记 len(Children)），
// 与 createPlan.count 口径一致，用于响应整体失败时的失败项折算。
func responseLeafUnits(resp *sdkdto.TaskCreateResponse) int {
	if len(resp.Children) == 0 {
		return 1
	}
	return len(resp.Children)
}

// CreateTaskStreamChan Go 风格的流式任务创建通道
// 用于异步流式处理插件返回的任务
type CreateTaskStreamChan struct {
	Task   *entity.Task
	Parent *entity.Task // 父任务（如果是子任务的话）
	Error  error
}

// handleCreateTaskStream 处理插件返回的流式任务（使用 Go channel）。
// 经 planCreateResponse 单点判定（与 array 路径一致）：无 Children→独立 leaf；有 Children→parent+children（不折叠）。
// 同 PluginTaskId 的多响应归入同一 parent（合并），让插件可把一个超大 work 拆成多响应流式发
// （如 local 扫描含大量文件的目录：边扫边发、复用同一 PluginTaskId）。
// parent 即时落盘（CreateTask），leaf/child 进批量缓存经 CreateBatch 落盘；通过 channel 返回结果。
func (s *Service) handleCreateTaskStream(ctx context.Context, taskChan <-chan *sdkdto.TaskCreateResponse, listener *pluginTaskUrlListener.PluginWithExtension, batchSize int) (<-chan *CreateTaskStreamChan, error) {
	outChan := make(chan *CreateTaskStreamChan)

	go func() {
		defer close(outChan)

		siteCache := make(map[string]int)
		batch := make([]*TaskWithWorkTask, 0, batchSize)
		// 当前 work 的父任务与其 PluginTaskId；同 PluginTaskId 的后续响应归入同一父（合并续传），
		// 不同 PluginTaskId 或空值则建新父——以 PluginTaskId（插件稳定 work 标识）为合并键。
		var currentParent *TaskWithWorkTask
		var currentPluginTaskId string

		// 批量保存缓存中的 leaf/child：核心行批量落库拿 id → 领域行批量绑定共享主键（同一事务）
		flushBatch := func() {
			if len(batch) == 0 {
				return
			}
			pending := batch
			err := s.transactor.ExecInTransaction(ctx, func(txCtx context.Context) error {
				coreBatch := make([]*entity.Task, 0, len(pending))
				for _, pair := range pending {
					coreBatch = append(coreBatch, pair.Task)
				}
				if err := s.repo.CreateBatch(txCtx, coreBatch); err != nil {
					return err
				}
				workBatch := make([]*entity.WorkTask, 0, len(pending))
				for _, pair := range pending {
					pair.WorkTask.SetID(pair.Task.GetID())
					workBatch = append(workBatch, pair.WorkTask)
				}
				return s.repo.CreateWorkTaskBatch(txCtx, workBatch)
			})
			if err != nil {
				for range pending {
					outChan <- &CreateTaskStreamChan{Error: err}
				}
			}
			batch = batch[:0]
		}

		for taskResp := range taskChan {
			select {
			case <-ctx.Done():
				return
			default:
			}

			plan, err := s.planCreateResponse(ctx, taskResp, listener, siteCache)
			if err != nil {
				outChan <- &CreateTaskStreamChan{Error: err}
				continue
			}

			if plan.leaf != nil {
				// 独立 leaf：进批量缓存
				batch = append(batch, plan.leaf)
				if len(batch) >= batchSize {
					flushBatch()
				}
				outChan <- &CreateTaskStreamChan{Task: plan.leaf.Task}
				continue
			}

			// parent + children：同 PluginTaskId 复用现有 parent（合并续传），否则建新 parent
			if currentParent == nil || taskResp.PluginTaskId == "" || taskResp.PluginTaskId != currentPluginTaskId {
				if err := s.persistPlanPair(ctx, plan.parent); err != nil {
					outChan <- &CreateTaskStreamChan{Error: err}
					continue
				}
				outChan <- &CreateTaskStreamChan{Parent: plan.parent.Task}
				currentParent = plan.parent
				currentPluginTaskId = taskResp.PluginTaskId
			}
			parentId := currentParent.Task.GetID()
			for _, child := range plan.children {
				child.Task.Pid = sql.NullInt64{Int64: parentId, Valid: true}
				batch = append(batch, child)
				if len(batch) >= batchSize {
					flushBatch()
				}
				outChan <- &CreateTaskStreamChan{Task: child.Task}
			}
		}

		flushBatch()
	}()

	return outChan, nil
}
