package task

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
	querypkg "github.com/library-squirrel/backend/base/query"
	"github.com/library-squirrel/backend/database"
	pkgerr "github.com/library-squirrel/backend/error"
	"github.com/library-squirrel/backend/pluginTaskUrlListener"
	"github.com/library-squirrel/backend/site"
	"github.com/library-squirrel/backend/util"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
	"gorm.io/gorm/clause"
)

// 错误定义
var (
	ErrUrlNotSupported   = &pkgerr.BusinessError{Code: 400, Message: "url不受支持"}
	ErrNoPluginFound     = &pkgerr.BusinessError{Code: 500, Message: "尝试了所有插件均未成功"}
	ErrSiteNameRequired  = &pkgerr.BusinessError{Code: 400, Message: "创建任务失败，插件返回的任务信息中缺少站点名称"}
	ErrSiteNotFound      = &pkgerr.BusinessError{Code: 400, Message: "创建任务失败，没有找到站点对应的信息"}
	ErrPluginDataInvalid = &pkgerr.BusinessError{Code: 500, Message: "序列化插件保存的pluginData失败"}
	ErrTaskHandlerFailed = &pkgerr.BusinessError{Code: 500, Message: "插件创建任务失败"}
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
	// Create 新建
	Create(ctx context.Context, task *entity.Task) error
	// CreateBatch 批量新建
	CreateBatch(ctx context.Context, tasks []*entity.Task) error
	// Updates 更新
	Updates(ctx context.Context, task *entity.Task) error
	// GetById 根据ID获取
	GetById(ctx context.Context, id int64) (*entity.Task, error)
	// List 查询列表
	List(ctx context.Context, opt *database.QueryOption) ([]*entity.Task, error)
	// Count 统计数量
	Count(ctx context.Context, opt *database.QueryOption) (int64, error)
	// Delete 删除
	Delete(ctx context.Context, id int64) error
	// Page 分页查询
	Page(ctx context.Context, opt *database.PageOption) (*model.Page[entity.Task], error)
	// QueryParentPage 分页查询父任务
	QueryParentPage(ctx context.Context, opt *database.PageOption) (*model.Page[entity.Task], error)
	// RefreshTaskStatus 刷新任务状态
	RefreshTaskStatus(ctx context.Context, taskId int64) (int64, error)
	// ListTaskTree 获取任务树列表
	ListTaskTree(ctx context.Context, taskIds []int64, includeStatus ...TaskStatusEnum) ([]*entity.Task, error)
	// SetTaskTreeStatus 设置任务树状态
	SetTaskTreeStatus(ctx context.Context, taskIds []int64, status TaskStatusEnum, includeStatus ...TaskStatusEnum) (int64, error)
	// ListStatus 查询状态列表
	ListStatus(ctx context.Context, ids []int64) ([]*entity.Task, error)
	// CreateTask 创建任务
	CreateTask(ctx context.Context, task *entity.Task) error
	// ListChildrenTask 查询子任务列表
	ListChildrenTask(ctx context.Context, pid int64) ([]*entity.Task, error)
	// QueryChildrenTaskPage 查询子任务分页
	QueryChildrenTaskPage(ctx context.Context, opt *database.PageOption) (*model.Page[entity.Task], error)
	// ListSchedule 查询任务进度列表
	ListSchedule(ctx context.Context, ids []int64) ([]*entity.Task, error)
	// DeleteTask 删除任务（包含子任务）- 批量删除
	DeleteTask(ctx context.Context, ids []int64) error
	// ClearResourceTaskId 批量清空资源行对任务及其子任务的 task_id 引用（删除链前置步）
	ClearResourceTaskId(ctx context.Context, ids []int64) error
	// BatchSetStatus 批量设置任务状态（同时更新 error_message）
	BatchSetStatus(ctx context.Context, statuses map[int64]StatusUpdate) error
	// UpdatePendingResourceID 更新任务的 pending_resource_id
	UpdatePendingResourceID(ctx context.Context, taskId int64, resourceID sql.NullInt64) error
	// ListBySiteAndSiteWorkID 根据站点和站点作品ID查询关联任务列表
	ListBySiteAndSiteWorkID(ctx context.Context, siteId int64, siteWorkId string) ([]*entity.Task, error)
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

// buildTaskProgressTree 将任务实体列表构建为 TaskProgressTreeDTO 树形结构
func buildTaskProgressTree(tasks []*entity.Task) []*dto.TaskProgressTreeDTO {
	if len(tasks) == 0 {
		return nil
	}
	dtos := make([]*dto.TaskProgressTreeDTO, len(tasks))
	for i, task := range tasks {
		dtos[i] = dto.NewTaskProgressTreeDTO(dto.NewTaskDTO(task))
	}
	return taskProgressTreeBuilder.BuildTree(dtos, setTaskProgressTreeChildren)
}

// TaskHandlerProvider 任务处理器提供者接口
// 用于获取插件的任务处理器，解耦 task 模块对 plugin 模块的直接依赖
type TaskHandlerProvider interface {
	// GetTaskHandler 获取任务处理器
	GetTaskHandler(pluginPublicId, extensionId string) (sdkdto.TaskHandler, error)
}

// Transactor 事务执行器接口
type Transactor interface {
	ExecInTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

// Service 任务服务
type Service struct {
	repo              Repository
	transactor        Transactor
	taskHandlerGetter TaskHandlerProvider
	urlListener       *pluginTaskUrlListener.Service
	siteSvc           *site.Service
	memoryProvider    MemoryStateProvider
}

// NewService 创建任务服务
func NewService(repo Repository, transactor Transactor, taskHandlerGetter TaskHandlerProvider, urlListener *pluginTaskUrlListener.Service, siteSvc *site.Service) *Service {
	return &Service{
		repo:              repo,
		transactor:        transactor,
		taskHandlerGetter: taskHandlerGetter,
		urlListener:       urlListener,
		siteSvc:           siteSvc,
	}
}

// SetMemoryProvider 设置内存任务状态提供者（延迟注入，解决初始化顺序问题）
func (s *Service) SetMemoryProvider(provider MemoryStateProvider) {
	s.memoryProvider = provider
}

// buildPageOptionWithMemory 构建 PageOption，综合内存中的任务状态调整查询条件
// 瞬态状态：从内存收集匹配 ID → 清除 Status 条件 → 添加 id IN (匹配IDs)
// 稳态状态：从内存收集不匹配 ID → 保留 Status 条件 → 追加 id NOT IN (不匹配IDs)
func (s *Service) buildPageOptionWithMemory(query TaskQueryDTO, page, pageSize int) (*database.PageOption, error) {
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

// GetById 根据ID获取
func (s *Service) GetById(ctx context.Context, id int64) (*entity.Task, error) {
	return s.repo.GetById(ctx, id)
}

// Save 保存任务
func (s *Service) Save(ctx context.Context, task *entity.Task) error {
	return s.repo.Create(ctx, task)
}

// SaveBatch 批量保存任务
func (s *Service) SaveBatch(ctx context.Context, tasks []*entity.Task) error {
	return s.repo.CreateBatch(ctx, tasks)
}

// Update 更新任务
func (s *Service) Update(ctx context.Context, task *entity.Task) error {
	return s.repo.Updates(ctx, task)
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

// Page 分页查询
func (s *Service) Page(ctx context.Context, page *model.Page[entity.Task], query TaskQueryDTO) (*model.Page[entity.Task], error) {
	opt, err := s.buildPageOptionWithMemory(query, page.PageNumber, page.PageSize)
	if err != nil {
		return nil, err
	}
	result, err := s.repo.Page(ctx, opt)
	if err != nil {
		return nil, err
	}
	s.overlayMemoryStates(result.Data)
	return result, nil
}

// QueryParentPage 分页查询父任务
func (s *Service) QueryParentPage(ctx context.Context, page *model.Page[entity.Task], query TaskQueryDTO) (*model.Page[entity.Task], error) {
	opt, err := s.buildPageOptionWithMemory(query, page.PageNumber, page.PageSize)
	if err != nil {
		return nil, err
	}
	result, err := s.repo.QueryParentPage(ctx, opt)
	if err != nil {
		return nil, err
	}
	s.overlayMemoryStates(result.Data)
	return result, nil
}

// RefreshTaskStatus 刷新任务状态
func (s *Service) RefreshTaskStatus(ctx context.Context, taskId int64) (int64, error) {
	return s.repo.RefreshTaskStatus(ctx, taskId)
}

// SetTreeStatus 设置任务树状态
func (s *Service) SetTreeStatus(ctx context.Context, taskIds []int64, status TaskStatusEnum, includeStatus ...TaskStatusEnum) (int64, error) {
	return s.repo.SetTaskTreeStatus(ctx, taskIds, status, includeStatus...)
}

// ListTaskTree 获取任务树列表
func (s *Service) ListTaskTree(ctx context.Context, taskIds []int64, includeStatus ...TaskStatusEnum) ([]*entity.Task, error) {
	return s.repo.ListTaskTree(ctx, taskIds, includeStatus...)
}

// ListStatus 查询状态列表
func (s *Service) ListStatus(ctx context.Context, ids []int64) ([]*dto.TaskProgressDTO, error) {
	tasks, err := s.repo.ListStatus(ctx, ids)
	if err != nil {
		return nil, err
	}
	result := make([]*dto.TaskProgressDTO, len(tasks))
	for i, task := range tasks {
		taskDTO := dto.NewTaskDTO(task)
		progressDTO := dto.NewTaskProgressDTO(taskDTO)
		if task.Status == int(TaskStatusFinished) {
			progressDTO.Schedule = new(100)
		}
		result[i] = progressDTO
	}
	return result, nil
}

// CreateTask 创建任务
func (s *Service) CreateTask(ctx context.Context, req *dto.CreateTaskRequest) (*entity.Task, error) {
	task := &entity.Task{
		BaseEntity: &model.BaseEntity{},
		// pid 外键引用 task.id（无 id=0 行）：req.Pid=0 → NULL=根级任务
		Pid:               sql.NullInt64{Int64: req.Pid, Valid: req.Pid != 0},
		TaskName:          sql.NullString{String: req.TaskName, Valid: true},
		SiteID:            sql.NullInt64{Int64: int64(req.SiteID), Valid: true},
		SiteWorkID:        sql.NullString{String: req.SiteWorkID, Valid: true},
		URL:               sql.NullString{String: req.URL, Valid: true},
		HasChild:          sql.NullBool{Bool: req.HasChild, Valid: true},
		Status:            int(TaskStatusCreated),
		PluginPublicID:    sql.NullString{String: req.PluginPublicID, Valid: true},
		PluginExtensionID: sql.NullString{String: req.PluginExtensionID, Valid: true},
		PluginData:        sql.NullString{String: req.PluginData, Valid: true},
	}
	if err := s.repo.CreateTask(ctx, task); err != nil {
		return nil, err
	}
	return task, nil
}

// DeleteTask 删除任务（包含子任务）- 批量删除
// 事务内先清 resource.task_id 引用再删任务行：外键强制下引用未清即删行被拒（NULL=非任务产）
func (s *Service) DeleteTask(ctx context.Context, ids []int64) error {
	return s.transactor.ExecInTransaction(ctx, func(txCtx context.Context) error {
		if err := s.repo.ClearResourceTaskId(txCtx, ids); err != nil {
			return err
		}
		return s.repo.DeleteTask(txCtx, ids)
	})
}

// QueryTreeDataPage 查询任务树数据分页
func (s *Service) QueryTreeDataPage(ctx context.Context, page, pageSize int, queryDTO *TaskQueryDTO) (*dto.TreeDataPageDTO, error) {
	conv := querypkg.NewConverter(entity.Task{})
	opt, err := conv.ToPageOption(queryDTO, page, pageSize, nil)
	if err != nil {
		return nil, err
	}

	// 分页查询父任务（has_child=1 OR pid IS NULL，根级任务 pid=NULL）
	resultPage, err := s.repo.QueryParentPage(ctx, opt)
	if err != nil {
		return nil, err
	}

	// 将分页数据构建为 TaskProgressTreeDTO 树
	tree := buildTaskProgressTree(resultPage.Data)

	// 获取 TreeID 和 TreeName（从分页数据中获取）
	var treeID int64
	var treeName string
	if len(resultPage.Data) > 0 {
		treeID = resultPage.Data[0].GetID()
		treeName = resultPage.Data[0].TaskName.String
	}

	return &dto.TreeDataPageDTO{
		TreeID:   treeID,
		TreeName: treeName,
		Total:    resultPage.DataCount,
		Tasks:    tree,
	}, nil
}

// ListChildrenTask 查询子任务列表
func (s *Service) ListChildrenTask(ctx context.Context, pid int64) ([]*entity.Task, error) {
	return s.repo.ListChildrenTask(ctx, pid)
}

// ListBySiteAndSiteWorkID 根据站点和站点作品ID查询关联任务列表
func (s *Service) ListBySiteAndSiteWorkID(ctx context.Context, siteId int64, siteWorkId string) ([]*entity.Task, error) {
	return s.repo.ListBySiteAndSiteWorkID(ctx, siteId, siteWorkId)
}

// QueryChildrenTaskPage 查询子任务分页
func (s *Service) QueryChildrenTaskPage(ctx context.Context, page *model.Page[entity.Task], query TaskQueryDTO) (*model.Page[entity.Task], error) {
	opt, err := s.buildPageOptionWithMemory(query, page.PageNumber, page.PageSize)
	if err != nil {
		return nil, err
	}
	result, err := s.repo.QueryChildrenTaskPage(ctx, opt)
	if err != nil {
		return nil, err
	}
	s.overlayMemoryStates(result.Data)
	return result, nil
}

// EnrichTaskProgressTreePage 将 Task 实体分页丰富为 TaskProgressTreeDTO 分页
// 批量查询站点名称并注入，同时填充树形结构字段（hasChildren、children、isLeaf）
func (s *Service) EnrichTaskProgressTreePage(ctx context.Context, rawPage *model.Page[entity.Task]) (*model.Page[dto.TaskProgressTreeDTO], error) {
	tasks := rawPage.Data
	if len(tasks) == 0 {
		return model.NewPage[dto.TaskProgressTreeDTO](nil, rawPage.DataCount, rawPage.PageNumber, rawPage.PageSize), nil
	}

	// 1. 收集 siteIds（去重）
	siteIdSet := make(map[int64]struct{})
	for _, task := range tasks {
		if task.SiteID.Valid && task.SiteID.Int64 > 0 {
			siteIdSet[task.SiteID.Int64] = struct{}{}
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
	data := make([]*dto.TaskProgressTreeDTO, 0, len(tasks))
	for _, task := range tasks {
		taskDTO := dto.NewTaskDTO(task)
		treeDTO := dto.NewTaskProgressTreeDTO(taskDTO)
		// 注入站点名称
		if task.SiteID.Valid {
			if siteName, ok := siteNameMap[task.SiteID.Int64]; ok {
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
}

// CreateTaskByURLResponse 根据URL创建任务的响应
type CreateTaskByURLResponse struct {
	Succeed       bool   `json:"succeed"`
	AddedQuantity int    `json:"addedQuantity"`
	Msg           string `json:"msg"`
}

// CreateTaskByURL 根据传入的url创建任务
// 通过 URL 监听器发现能处理此 URL 的插件，调用插件的 create 方法创建任务
func (s *Service) CreateTaskByURL(ctx context.Context, url string) (*CreateTaskByURLResponse, error) {
	// 1. 查询监听此url的插件
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

	// 2. 按照排序尝试每个插件
	for _, listener := range listeners {
		if !listener.PublicID.Valid || listener.PublicID.String == "" {
			logger.Log.Warnf("URL监听器缺少插件 PublicID，跳过 (extensionId=%s)", listener.ExtensionID)
			continue
		}
		pluginPublicId := listener.PublicID.String

		// 获取任务处理器
		taskHandler, err := s.taskHandlerGetter.GetTaskHandler(pluginPublicId, listener.ExtensionID)
		if err != nil {
			logger.Log.Warnf("获取任务处理器失败 (plugin=%s, extensionId=%s): %v", pluginPublicId, listener.ExtensionID, err)
			continue
		}

		// 3. 调用插件的 create 方法
		result, err := taskHandler.Create(url)
		if err != nil {
			logger.Log.Errorf("插件创建任务失败 (plugin=%s): %v", pluginPublicId, err)
			continue
		}

		// 4. 处理插件返回
		var count int
		if result.IsStream() {
			streamCh, streamErr := s.handleCreateTaskStream(ctx, result.Stream(), listener, 100)
			if streamErr != nil {
				logger.Log.Errorf("处理流式任务失败 (plugin=%s): %v", pluginPublicId, streamErr)
				continue
			}
			// 计数叶子级单元：leaf 与 child（Task 项），parent 容器（Parent 项）不计
			for item := range streamCh {
				if item.Task != nil {
					count++
				}
			}
		} else {
			responses := result.Array()
			if len(responses) > 0 {
				count, err = s.handleCreateTaskArray(ctx, responses, listener)
				if err != nil {
					logger.Log.Errorf("处理插件返回数据失败 (plugin=%s): %v", pluginPublicId, err)
					continue
				}
			}
		}

		if count > 0 {
			return &CreateTaskByURLResponse{
				Succeed:       true,
				AddedQuantity: count,
				Msg:           "创建成功",
			}, nil
		}
	}

	// 未能在循环中返回，则返回失败
	return &CreateTaskByURLResponse{
		Succeed:       false,
		AddedQuantity: 0,
		Msg:           fmt.Sprintf("尝试了所有插件均未成功，url: %s", url),
	}, nil
}

// createPlan 一个 TaskCreateResponse 经单点判定后的创建计划。
// leaf 与 parent 互斥：无 Children 时 leaf 非空（独立任务）；有 Children 时 parent+children。
// children 的 Pid 待调用方落盘 parent 后回填。
type createPlan struct {
	leaf     *entity.Task
	parent   *entity.Task
	children []*entity.Task
}

// count 此计划贡献的叶子级任务计数（leaf=1；parent+N=N，parent 容器不计）。
func (p *createPlan) count() int {
	if p.leaf != nil {
		return 1
	}
	return len(p.children)
}

// childToResponse 把子响应适配为 TaskCreateResponse，复用 fillTaskFromResponse 的统一字段映射。
func childToResponse(c *sdkdto.TaskCreateChildResponse) *sdkdto.TaskCreateResponse {
	return &sdkdto.TaskCreateResponse{
		TaskName:      c.TaskName,
		SiteWorkId:    c.SiteWorkId,
		Url:           c.Url,
		SiteName:      c.SiteName,
		PluginData:    c.PluginData,
		InvolvedRoles: c.InvolvedRoles,
		ResourceType:  c.ResourceType,
	}
}

// fillTaskFromResponse 把响应字段填入一个已分配的 Task（leaf/parent/child 通用，双路径共用）。
// pid：父任务 ID（child 传 parent.id；leaf/parent 传 0）。pid=0 写 NULL=根级任务（外键引用 task.id，无 id=0 行）。
// hasChild：是否父任务（容器，不带 SiteWorkID/PluginData）。
// siteCache：站点名→ID 缓存（调用方持有，跨任务复用，避免重复查库）。
// SiteName 为空时返回 ErrSiteNameRequired——leaf/parent/child 均须归属站点。
func (s *Service) fillTaskFromResponse(ctx context.Context, task *entity.Task, resp *sdkdto.TaskCreateResponse, listener *pluginTaskUrlListener.PluginWithExtension, pid int64, hasChild bool, siteCache map[string]int) error {
	task.TaskName = sql.NullString{String: resp.TaskName, Valid: true}
	task.URL = sql.NullString{String: resp.Url, Valid: true}
	task.Status = int(TaskStatusCreated)
	task.HasChild = sql.NullBool{Bool: hasChild, Valid: true}
	// pid=0 → NULL=根级任务（外键引用 task.id，无 id=0 行，写 0 必违约）；child 落盘前由调用方回填父 ID
	task.Pid = sql.NullInt64{Int64: pid, Valid: pid != 0}
	task.PluginPublicID = listener.PublicID
	task.PluginExtensionID = sql.NullString{String: listener.ExtensionID, Valid: true}

	if resp.SiteName == "" {
		return errors.Join(ErrSiteNameRequired, errors.New("siteName is empty"))
	}
	siteId, ok := siteCache[resp.SiteName]
	if !ok {
		site, err := s.siteSvc.GetByName(ctx, resp.SiteName)
		if err != nil || site == nil {
			return errors.Join(ErrSiteNotFound, errors.New(resp.SiteName))
		}
		siteId = int(site.ID)
		siteCache[resp.SiteName] = siteId
	}
	task.SiteID = sql.NullInt64{Int64: int64(siteId), Valid: true}

	// 身份字段：leaf/child 带 SiteWorkID/PluginData；parent 容器不带
	if !hasChild {
		task.SiteWorkID = sql.NullString{String: resp.SiteWorkId, Valid: true}
		if resp.PluginData != "" {
			task.PluginData = sql.NullString{String: resp.PluginData, Valid: true}
		}
	}

	// involvedRoles:创建期声明的涉及板块(universe),逗号join;空=NULL(未确定/默认)
	if len(resp.InvolvedRoles) > 0 {
		task.InvolvedRoles = sql.NullString{String: strings.Join(resp.InvolvedRoles, ","), Valid: true}
	}

	// resourceType:创建期声明的资源类型(预定义值);空=NULL(未声明);有 children 时由各 child 声明
	if resp.ResourceType != "" {
		task.ResourceType = sql.NullString{String: resp.ResourceType, Valid: true}
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
		// 无 Children：独立 leaf（如 local 单文件导入），pid=NULL（根级）、HasChild=false
		leaf := &entity.Task{BaseEntity: &model.BaseEntity{}}
		if err := s.fillTaskFromResponse(ctx, leaf, taskResp, listener, 0, false, siteCache); err != nil {
			return nil, err
		}
		return &createPlan{leaf: leaf}, nil
	}

	// 有 Children：parent + 每个 child，不折叠
	parent := &entity.Task{BaseEntity: &model.BaseEntity{}}
	if err := s.fillTaskFromResponse(ctx, parent, taskResp, listener, 0, true, siteCache); err != nil {
		return nil, err
	}
	children := make([]*entity.Task, 0, len(taskResp.Children))
	for _, childResp := range taskResp.Children {
		child := &entity.Task{BaseEntity: &model.BaseEntity{}}
		if err := s.fillTaskFromResponse(ctx, child, childToResponse(childResp), listener, 0, false, siteCache); err != nil {
			return nil, err
		}
		children = append(children, child)
	}
	return &createPlan{parent: parent, children: children}, nil
}

// handleCreateTaskArray 处理插件返回的任务数组。
// 经 planCreateResponse 单点判定：无 Children→独立 leaf；有 Children→parent+children（不折叠）。
// 返回叶子级任务计数（leaf=1、parent+N=N，parent 容器不计）。
func (s *Service) handleCreateTaskArray(ctx context.Context, pluginResponses []*sdkdto.TaskCreateResponse, listener *pluginTaskUrlListener.PluginWithExtension) (int, error) {
	if len(pluginResponses) == 0 {
		return 0, nil
	}

	count := 0
	siteCache := make(map[string]int) // siteName -> siteId 缓存

	for _, resp := range pluginResponses {
		plan, err := s.planCreateResponse(ctx, resp, listener, siteCache)
		if err != nil {
			// 字段填充失败（如 SiteName 缺失）：跳过此响应
			continue
		}

		if plan.leaf != nil {
			// 独立 leaf：直接落盘
			if err := s.repo.CreateTask(ctx, plan.leaf); err != nil {
				continue
			}
			count += plan.count()
			continue
		}

		// parent + children：事务内落盘 parent → 回填 children.Pid → 落盘 children
		err = s.transactor.ExecInTransaction(ctx, func(txCtx context.Context) error {
			if err := s.repo.CreateTask(txCtx, plan.parent); err != nil {
				return err
			}
			parentId := plan.parent.GetID()
			for _, child := range plan.children {
				child.Pid = sql.NullInt64{Int64: parentId, Valid: true}
				if err := s.repo.CreateTask(txCtx, child); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			logger.Log.Errorf("[Task] 创建任务组失败: %v", err)
			continue
		}
		count += plan.count()
	}

	return count, nil
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
		batch := make([]*entity.Task, 0, batchSize)
		// 当前 work 的父任务与其 PluginTaskId；同 PluginTaskId 的后续响应归入同一父（合并续传），
		// 不同 PluginTaskId 或空值则建新父——以 PluginTaskId（插件稳定 work 标识）为合并键。
		var currentParent *entity.Task
		var currentPluginTaskId string

		// 批量保存缓存中的 leaf/child
		flushBatch := func() {
			if len(batch) > 0 {
				if err := s.repo.CreateBatch(ctx, batch); err != nil {
					for range batch {
						outChan <- &CreateTaskStreamChan{Error: err}
					}
				}
				batch = batch[:0]
			}
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
				outChan <- &CreateTaskStreamChan{Task: plan.leaf}
				continue
			}

			// parent + children：同 PluginTaskId 复用现有 parent（合并续传），否则建新 parent
			if currentParent == nil || taskResp.PluginTaskId == "" || taskResp.PluginTaskId != currentPluginTaskId {
				if err := s.repo.CreateTask(ctx, plan.parent); err != nil {
					outChan <- &CreateTaskStreamChan{Error: err}
					continue
				}
				outChan <- &CreateTaskStreamChan{Parent: plan.parent}
				currentParent = plan.parent
				currentPluginTaskId = taskResp.PluginTaskId
			}
			parentId := currentParent.GetID()
			for _, child := range plan.children {
				child.Pid = sql.NullInt64{Int64: parentId, Valid: true}
				batch = append(batch, child)
				if len(batch) >= batchSize {
					flushBatch()
				}
				outChan <- &CreateTaskStreamChan{Task: child}
			}
		}

		flushBatch()
	}()

	return outChan, nil
}
