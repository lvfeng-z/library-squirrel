package task

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/library-squirrel/wails/internal/database"
	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/internal/pluginTaskUrlListener"
	"github.com/library-squirrel/wails/internal/site"
	"github.com/library-squirrel/wails/pkg/logger"
	pkgModel "github.com/library-squirrel/wails/pkg/model"

	"gorm.io/gorm/clause"
)

// ========== 查询 DTO ==========

// TaskQueryDTO 任务查询条件
type TaskQueryDTO struct {
	// 精确查询
	ID                   *int64  `json:"-"`                    // 任务ID（程序设置，不从JSON解析）
	Pid                  *int64  `json:"pid"`                  // 父任务ID
	SiteID               *int    `json:"siteId"`               // 站点ID
	SiteWorkID           *string `json:"siteWorkId"`           // 站点作品ID
	Status               *int    `json:"status"`               // 任务状态
	IsCollection         *int    `json:"isCollection"`         // 是否为合集（0=否，1=是）
	PluginPublicID       *string `json:"pluginPublicId"`       // 插件公开ID
	PluginContributionID *string `json:"pluginContributionId"` // 插件贡献ID
	Continuable          *int    `json:"continuable"`          // 是否可继续（0=否，1=是）
	// 模糊查询
	TaskNameLike *string `json:"taskNameLike"` // 任务名称（模糊匹配）
	// 排序字段：create_time, update_time, task_name
	OrderBy   string `json:"orderBy"`   // 排序字段
	OrderDesc bool   `json:"orderDesc"` // 是否降序
}

// BuildOrderBy 根据查询DTO构建排序条件
func (dto *TaskQueryDTO) BuildOrderBy() clause.Expression {
	column := "id"
	if dto.OrderBy != "" {
		// 支持的排序字段映射
		orderByMap := map[string]string{
			"create_time": "create_time",
			"update_time": "update_time",
			"task_name":   "task_name",
		}
		if v, ok := orderByMap[dto.OrderBy]; ok {
			column = v
		}
	}
	return clause.OrderBy{Columns: []clause.OrderByColumn{{Column: clause.Column{Name: column}, Desc: dto.OrderDesc}}}
}

// 错误定义
var (
	ErrUrlNotSupported   = &BusinessError{Code: 400, Message: "url不受支持"}
	ErrNoPluginFound     = &BusinessError{Code: 500, Message: "尝试了所有插件均未成功"}
	ErrSiteNameRequired  = &BusinessError{Code: 400, Message: "创建任务失败，插件返回的任务信息中缺少站点名称"}
	ErrSiteNotFound      = &BusinessError{Code: 400, Message: "创建任务失败，没有找到站点对应的信息"}
	ErrPluginDataInvalid = &BusinessError{Code: 500, Message: "序列化插件保存的pluginData失败"}
	ErrTaskHandlerFailed = &BusinessError{Code: 500, Message: "插件创建任务失败"}
)

// BusinessError 业务错误
type BusinessError struct {
	Code    int
	Message string
}

func (e *BusinessError) Error() string {
	return e.Message
}

// TaskStatusEnum 任务状态枚举
type TaskStatusEnum int

const (
	TaskStatusCreated          TaskStatusEnum = 0
	TaskStatusProcessing       TaskStatusEnum = 1
	TaskStatusWaiting          TaskStatusEnum = 2
	TaskStatusPause            TaskStatusEnum = 3
	TaskStatusFinished         TaskStatusEnum = 4
	TaskStatusPartlyFinished   TaskStatusEnum = 5
	TaskStatusFailed           TaskStatusEnum = 6
	TaskStatusWaitingUserInput TaskStatusEnum = 7
)

// Repository 任务仓储接口（由 service 定义需要的数据库操作方法）
// 注意：只定义 service 真正需要的方法，遵循最小依赖原则
type Repository interface {
	// Save 保存
	Save(ctx context.Context, task *domain.Task) error
	// SaveBatch 批量保存
	SaveBatch(ctx context.Context, tasks []*domain.Task) error
	// Update 更新
	Update(ctx context.Context, task *domain.Task) error
	// GetById 根据ID获取
	GetById(ctx context.Context, id int64) (*domain.Task, error)
	// List 查询列表
	List(ctx context.Context, opt *database.QueryOption) ([]*domain.Task, error)
	// Count 统计数量
	Count(ctx context.Context, opt *database.QueryOption) (int64, error)
	// Delete 删除
	Delete(ctx context.Context, id int64) error
	// Page 分页查询
	Page(ctx context.Context, opt *database.PageOption) (*pkgModel.Page[domain.Task], error)
	// QueryParentPage 分页查询父任务
	QueryParentPage(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression) (*pkgModel.Page[domain.Task], error)
	// RefreshTaskStatus 刷新任务状态
	RefreshTaskStatus(ctx context.Context, taskId int64) (int64, error)
	// ListTaskTree 获取任务树列表
	ListTaskTree(ctx context.Context, taskIds []int64, includeStatus ...TaskStatusEnum) ([]*domain.Task, error)
	// SetTaskTreeStatus 设置任务树状态
	SetTaskTreeStatus(ctx context.Context, taskIds []int64, status TaskStatusEnum, includeStatus ...TaskStatusEnum) (int64, error)
	// ListStatus 查询状态列表
	ListStatus(ctx context.Context, ids []int64) ([]*TaskScheduleDTO, error)
	// CreateTask 创建任务
	CreateTask(ctx context.Context, task *domain.Task) error
	// ListChildrenTask 查询子任务列表
	ListChildrenTask(ctx context.Context, pid int64) ([]*domain.Task, error)
	// QueryChildrenTaskPage 查询子任务分页
	QueryChildrenTaskPage(ctx context.Context, pid int64, page, pageSize int, where clause.Expression, order clause.Expression) (*pkgModel.Page[domain.Task], error)
	// ListSchedule 查询任务进度列表
	ListSchedule(ctx context.Context, ids []int64) ([]*TaskScheduleDTO, error)
	// DeleteTask 删除任务（包含子任务）
	DeleteTask(ctx context.Context, id int64) error
}

// TaskScheduleDTO 任务进度DTO
type TaskScheduleDTO struct {
	ID       int64 `json:"id"`
	Pid      int64 `json:"pid"`
	Status   int   `json:"status"`
	Schedule int   `json:"schedule"`
}

// CreateTaskRequest 创建任务请求
type CreateTaskRequest struct {
	Pid                  int64  `json:"pid"`
	TaskName             string `json:"taskName"`
	SiteID               int    `json:"siteId"`
	SiteWorkID           string `json:"siteWorkId"`
	URL                  string `json:"url"`
	IsCollection         int    `json:"isCollection"`
	PluginPublicID       string `json:"pluginPublicId"`
	PluginContributionID string `json:"pluginContributionId"`
	PluginData           string `json:"pluginData"`
}

// TreeDataPageRequest 任务树数据分页请求
type TreeDataPageRequest struct {
	TreeID int64 `json:"treeId"`
}

// TreeDataPageDTO 任务树数据分页DTO
type TreeDataPageDTO struct {
	TreeID   int64          `json:"treeId"`
	TreeName string         `json:"treeName"`
	Total    int64          `json:"total"`
	Tasks    []*domain.Task `json:"tasks"`
}

// WorkSaver 作品保存接口
type WorkSaver interface {
	// Save 保存作品
	Save(ctx context.Context, work *domain.Work) error
}

// ResourceSaver 资源保存接口
type ResourceSaver interface {
	// Save 保存资源
	Save(ctx context.Context, resource *domain.Resource) error
}

// TaskHandlerProvider 任务处理器提供者接口
// 用于获取插件的任务处理器，解耦 task 模块对 plugin 模块的直接依赖
type TaskHandlerProvider interface {
	// GetTaskHandler 获取任务处理器
	GetTaskHandler(pluginPublicId, contributionId string) (domain.TaskHandler, error)
}

// Service 任务服务
type Service struct {
	repo              Repository
	workSaver         WorkSaver
	resourceSaver     ResourceSaver
	taskHandlerGetter TaskHandlerProvider
	urlListener       *pluginTaskUrlListener.Service
	siteSvc           *site.Service
}

// NewService 创建任务服务
func NewService(repo Repository, workSaver WorkSaver, resourceSaver ResourceSaver, taskHandlerGetter TaskHandlerProvider, urlListener *pluginTaskUrlListener.Service, siteSvc *site.Service) *Service {
	return &Service{
		repo:              repo,
		workSaver:         workSaver,
		resourceSaver:     resourceSaver,
		taskHandlerGetter: taskHandlerGetter,
		urlListener:       urlListener,
		siteSvc:           siteSvc,
	}
}

// GetById 根据ID获取
func (s *Service) GetById(ctx context.Context, id int64) (*domain.Task, error) {
	return s.repo.GetById(ctx, id)
}

// Save 保存任务
func (s *Service) Save(ctx context.Context, task *domain.Task) error {
	return s.repo.Save(ctx, task)
}

// SaveBatch 批量保存任务
func (s *Service) SaveBatch(ctx context.Context, tasks []*domain.Task) error {
	return s.repo.SaveBatch(ctx, tasks)
}

// Update 更新任务
func (s *Service) Update(ctx context.Context, task *domain.Task) error {
	return s.repo.Update(ctx, task)
}

// Delete 删除任务
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// List 查询列表
func (s *Service) List(ctx context.Context, opt *database.QueryOption) ([]*domain.Task, error) {
	return s.repo.List(ctx, opt)
}

// Count 统计数量
func (s *Service) Count(ctx context.Context, opt *database.QueryOption) (int64, error) {
	return s.repo.Count(ctx, opt)
}

// Page 分页查询
func (s *Service) Page(ctx context.Context, opt *database.PageOption) (*pkgModel.Page[domain.Task], error) {
	return s.repo.Page(ctx, opt)
}

// PageByDTO 分页查询（基于 QueryDTO）
func (s *Service) PageByDTO(ctx context.Context, page, pageSize int, queryDTO TaskQueryDTO) (*pkgModel.Page[domain.Task], error) {
	conditions := buildConditionsFromDTO(&queryDTO)
	orderBy := queryDTO.BuildOrderBy()
	opt := &database.PageOption{
		QueryOption: database.QueryOption{
			Conditions: conditions,
			OrderBy:    []clause.Expression{orderBy},
		},
		Page:     page,
		PageSize: pageSize,
	}
	return s.repo.Page(ctx, opt)
}

// QueryParentPage 分页查询父任务
func (s *Service) QueryParentPage(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression) (*pkgModel.Page[domain.Task], error) {
	return s.repo.QueryParentPage(ctx, page, pageSize, where, order)
}

// QueryParentPageByDTO 分页查询父任务（基于 QueryDTO）
func (s *Service) QueryParentPageByDTO(ctx context.Context, page, pageSize int, queryDTO TaskQueryDTO) (*pkgModel.Page[domain.Task], error) {
	conditions := buildConditionsFromDTO(&queryDTO)
	where := combineConditions(conditions)
	orderBy := queryDTO.BuildOrderBy()
	return s.repo.QueryParentPage(ctx, page, pageSize, where, orderBy)
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
func (s *Service) ListTaskTree(ctx context.Context, taskIds []int64, includeStatus ...TaskStatusEnum) ([]*domain.Task, error) {
	return s.repo.ListTaskTree(ctx, taskIds, includeStatus...)
}

// ListStatus 查询状态列表
func (s *Service) ListStatus(ctx context.Context, ids []int64) ([]*TaskScheduleDTO, error) {
	return s.repo.ListStatus(ctx, ids)
}

// CreateTask 创建任务
func (s *Service) CreateTask(ctx context.Context, req *CreateTaskRequest) (*domain.Task, error) {
	task := &domain.Task{
		BaseEntity:           &pkgModel.BaseEntity{},
		Pid:                  sql.NullInt64{Int64: req.Pid, Valid: true},
		TaskName:             sql.NullString{String: req.TaskName, Valid: true},
		SiteID:               sql.NullInt64{Int64: int64(req.SiteID), Valid: true},
		SiteWorkID:           sql.NullString{String: req.SiteWorkID, Valid: true},
		URL:                  sql.NullString{String: req.URL, Valid: true},
		IsCollection:         sql.NullInt64{Int64: int64(req.IsCollection), Valid: true},
		Status:               int(TaskStatusCreated),
		PluginPublicID:       sql.NullString{String: req.PluginPublicID, Valid: true},
		PluginContributionID: sql.NullString{String: req.PluginContributionID, Valid: true},
		PluginData:           sql.NullString{String: req.PluginData, Valid: true},
	}
	if err := s.repo.CreateTask(ctx, task); err != nil {
		return nil, err
	}
	return task, nil
}

// DeleteTask 删除任务（包含子任务）
func (s *Service) DeleteTask(ctx context.Context, id int64) error {
	return s.repo.DeleteTask(ctx, id)
}

// QueryTreeDataPage 查询任务树数据分页
func (s *Service) QueryTreeDataPage(ctx context.Context, treeId int64) (*TreeDataPageDTO, error) {
	// 获取根任务
	rootTask, err := s.repo.GetById(ctx, treeId)
	if err != nil {
		return nil, err
	}

	// 获取任务树下的所有任务
	tasks, err := s.repo.ListTaskTree(ctx, []int64{treeId})
	if err != nil {
		return nil, err
	}

	return &TreeDataPageDTO{
		TreeID:   treeId,
		TreeName: rootTask.TaskName.String,
		Total:    int64(len(tasks)),
		Tasks:    tasks,
	}, nil
}

// ListChildrenTask 查询子任务列表
func (s *Service) ListChildrenTask(ctx context.Context, pid int64) ([]*domain.Task, error) {
	return s.repo.ListChildrenTask(ctx, pid)
}

// QueryChildrenTaskPage 查询子任务分页
func (s *Service) QueryChildrenTaskPage(ctx context.Context, pid int64, page, pageSize int, where clause.Expression, order clause.Expression) (*pkgModel.Page[domain.Task], error) {
	return s.repo.QueryChildrenTaskPage(ctx, pid, page, pageSize, where, order)
}

// QueryChildrenTaskPageByDTO 查询子任务分页（基于 QueryDTO）
func (s *Service) QueryChildrenTaskPageByDTO(ctx context.Context, pid int64, page, pageSize int, queryDTO TaskQueryDTO) (*pkgModel.Page[domain.Task], error) {
	conditions := buildConditionsFromDTO(&queryDTO)
	where := combineConditions(conditions)
	orderBy := queryDTO.BuildOrderBy()
	return s.repo.QueryChildrenTaskPage(ctx, pid, page, pageSize, where, orderBy)
}

// buildConditionsFromDTO 根据查询DTO构建查询条件
func buildConditionsFromDTO(dto *TaskQueryDTO) []clause.Expression {
	var conditions []clause.Expression

	if dto.ID != nil {
		conditions = append(conditions, clause.Eq{Column: "id", Value: *dto.ID})
	}
	if dto.Pid != nil {
		conditions = append(conditions, clause.Eq{Column: "pid", Value: *dto.Pid})
	}
	if dto.SiteID != nil {
		conditions = append(conditions, clause.Eq{Column: "site_id", Value: *dto.SiteID})
	}
	if dto.SiteWorkID != nil {
		conditions = append(conditions, clause.Eq{Column: "site_work_id", Value: *dto.SiteWorkID})
	}
	if dto.Status != nil {
		conditions = append(conditions, clause.Eq{Column: "status", Value: *dto.Status})
	}
	if dto.IsCollection != nil {
		conditions = append(conditions, clause.Eq{Column: "is_collection", Value: *dto.IsCollection})
	}
	if dto.PluginPublicID != nil {
		conditions = append(conditions, clause.Eq{Column: "plugin_public_id", Value: *dto.PluginPublicID})
	}
	if dto.PluginContributionID != nil {
		conditions = append(conditions, clause.Eq{Column: "plugin_contribution_id", Value: *dto.PluginContributionID})
	}
	if dto.Continuable != nil {
		conditions = append(conditions, clause.Eq{Column: "continuable", Value: *dto.Continuable})
	}
	if dto.TaskNameLike != nil {
		conditions = append(conditions, clause.Like{Column: "task_name", Value: *dto.TaskNameLike})
	}

	return conditions
}

// combineConditions 将多个条件组合成单个表达式
func combineConditions(conditions []clause.Expression) clause.Expression {
	if len(conditions) == 0 {
		return nil
	}
	if len(conditions) == 1 {
		return conditions[0]
	}
	result := clause.AndConditions{}
	for _, cond := range conditions {
		result.Exprs = append(result.Exprs, cond)
	}
	return result
}

// ListSchedule 查询任务进度列表
func (s *Service) ListSchedule(ctx context.Context, ids []int64) ([]*TaskScheduleDTO, error) {
	return s.repo.ListSchedule(ctx, ids)
}

// SaveWorkInfo 保存作品信息
func (s *Service) SaveWorkInfo(ctx context.Context, work *domain.Work, resources []*domain.Resource) error {
	// 保存作品信息
	if err := s.workSaver.Save(ctx, work); err != nil {
		return err
	}

	// 保存资源信息
	for _, resource := range resources {
		if err := s.resourceSaver.Save(ctx, resource); err != nil {
			return err
		}
	}

	return nil
}

// RefreshParentStatus 根据子任务状态刷新父任务状态
func (s *Service) RefreshParentStatus(ctx context.Context, taskId int64) error {
	// 获取任务
	task, err := s.repo.GetById(ctx, taskId)
	if err != nil {
		return err
	}

	// 如果是子任务，刷新父任务状态
	if task.Pid.Valid && task.Pid.Int64 != 0 {
		_, err := s.repo.SetTaskTreeStatus(ctx, []int64{task.Pid.Int64}, TaskStatusProcessing)
		return err
	}

	return nil
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
	if len(listeners) == 0 {
		logger.Log.Info("CreateTaskByURL: no listener for url", zap.String("url", url))
		return &CreateTaskByURLResponse{
			Succeed:       false,
			AddedQuantity: 0,
			Msg:           fmt.Sprintf("url不受支持，url: %s", url),
		}, nil
	}

	// 2. 按照排序尝试每个插件
	for _, listener := range listeners {
		if !listener.PublicID.Valid || listener.PublicID.String == "" {
			logger.Log.Error("CreateTaskByURL: plugin publicId is empty", zap.Any("listener", listener))
			continue
		}
		pluginPublicId := listener.PublicID.String

		// 获取任务处理器
		taskHandler, err := s.taskHandlerGetter.GetTaskHandler(pluginPublicId, listener.ContributionID)
		if err != nil {
			logger.Log.Error("CreateTaskByURL: failed to get task handler",
				zap.String("pluginPublicId", pluginPublicId),
				zap.String("contributionId", listener.ContributionID),
				zap.Error(err))
			continue
		}

		// 3. 调用插件的 create 方法
		pluginResponses, err := taskHandler.Create(url)
		if err != nil {
			logger.Log.Error("CreateTaskByURL: plugin create failed",
				zap.String("url", url),
				zap.String("pluginPublicId", pluginPublicId),
				zap.Error(err))
			continue
		}

		// 4. 处理插件返回
		if len(pluginResponses) > 0 {
			count, err := s.handleCreateTaskArray(ctx, pluginResponses, url, listener)
			if err != nil {
				logger.Log.Error("CreateTaskByURL: handle create task array failed",
					zap.String("url", url),
					zap.Error(err))
				continue
			}
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

// handleCreateTaskArray 处理插件返回的任务数组
func (s *Service) handleCreateTaskArray(ctx context.Context, pluginResponses []*domain.TaskCreateResponse, url string, listener *pluginTaskUrlListener.PluginWithContribution) (int, error) {
	if len(pluginResponses) == 0 {
		return 0, nil
	}

	childrenCount := 0
	siteCache := make(map[string]int) // siteName -> siteId 缓存

	// 给任务赋值的函数
	assignTask := func(task *domain.Task, taskResp *domain.TaskCreateResponse, pid int64) error {
		task.TaskName = sql.NullString{String: taskResp.TaskName, Valid: true}
		task.SiteWorkID = sql.NullString{String: taskResp.SiteWorkID, Valid: true}
		task.URL = sql.NullString{String: taskResp.URL, Valid: true}
		task.Status = int(TaskStatusCreated)
		task.IsCollection = sql.NullInt64{Int64: 0, Valid: true}
		task.Pid = sql.NullInt64{Int64: pid, Valid: true}
		task.PluginPublicID = listener.PublicID
		task.PluginContributionID = sql.NullString{String: listener.ContributionID, Valid: true}

		// 根据站点名称获取站点id
		if taskResp.SiteName == "" {
			return errors.Join(ErrSiteNameRequired, errors.New("siteName is empty"))
		}

		siteId, ok := siteCache[taskResp.SiteName]
		if !ok {
			site, err := s.siteSvc.GetByName(ctx, taskResp.SiteName)
			if err != nil || site == nil {
				return errors.Join(ErrSiteNotFound, errors.New(taskResp.SiteName))
			}
			siteId = int(site.ID)
			siteCache[taskResp.SiteName] = siteId
		}
		task.SiteID = sql.NullInt64{Int64: int64(siteId), Valid: true}

		// 处理 pluginData
		if taskResp.PluginData != "" {
			task.PluginData = sql.NullString{String: taskResp.PluginData, Valid: true}
		}

		return nil
	}

	// 处理每个父任务响应
	for _, parentResp := range pluginResponses {
		children := parentResp.Children
		if len(children) == 0 {
			continue
		}

		// 单个任务不创建父任务，只创建子任务
		if len(children) == 1 {
			task := &domain.Task{
				BaseEntity: &pkgModel.BaseEntity{},
			}
			childResp := children[0]
			if err := assignTask(task, &domain.TaskCreateResponse{
				TaskName:   childResp.TaskName,
				SiteWorkID: childResp.SiteWorkID,
				URL:        childResp.URL,
				SiteName:   childResp.SiteName,
				PluginData: childResp.PluginData,
			}, 0); err != nil {
				logger.Log.Error("handleCreateTaskArray: failed to assign single task", zap.Error(err))
				continue
			}
			if err := s.repo.CreateTask(ctx, task); err != nil {
				logger.Log.Error("handleCreateTaskArray: failed to save single task", zap.Error(err))
				continue
			}
			childrenCount++
			continue
		}

		// 多个子任务：先创建父任务
		parentTask := &domain.Task{
			BaseEntity: &pkgModel.BaseEntity{},
		}
		if err := assignTask(parentTask, &domain.TaskCreateResponse{
			TaskName: parentResp.TaskName,
			URL:      parentResp.URL,
			SiteName: parentResp.SiteName,
		}, 0); err != nil {
			logger.Log.Error("handleCreateTaskArray: failed to assign parent task", zap.Error(err))
			continue
		}
		parentTask.IsCollection = sql.NullInt64{Int64: 1, Valid: true} // 集合任务
		if err := s.repo.CreateTask(ctx, parentTask); err != nil {
			logger.Log.Error("handleCreateTaskArray: failed to save parent task", zap.Error(err))
			continue
		}
		parentId := parentTask.GetID()

		// 创建子任务
		for _, childResp := range children {
			childTask := &domain.Task{
				BaseEntity: &pkgModel.BaseEntity{},
			}
			if err := assignTask(childTask, &domain.TaskCreateResponse{
				TaskName:   childResp.TaskName,
				SiteWorkID: childResp.SiteWorkID,
				URL:        childResp.URL,
				SiteName:   childResp.SiteName,
				PluginData: childResp.PluginData,
			}, parentId); err != nil {
				logger.Log.Error("handleCreateTaskArray: failed to assign child task", zap.Error(err))
				continue
			}
			if err := s.repo.CreateTask(ctx, childTask); err != nil {
				logger.Log.Error("handleCreateTaskArray: failed to save child task", zap.Error(err))
				continue
			}
			childrenCount++
		}
	}

	return childrenCount, nil
}

// CreateTaskStreamChan Go 风格的流式任务创建通道
// 用于异步流式处理插件返回的任务
type CreateTaskStreamChan struct {
	Task   *domain.Task
	Parent *domain.Task // 父任务（如果是子任务的话）
	Error  error
}

// handleCreateTaskStream 处理插件返回的流式任务（使用 Go channel）
// 该方法会启动一个 goroutine 来读取任务流，并通过 channel 返回结果
func (s *Service) handleCreateTaskStream(ctx context.Context, taskChan <-chan *domain.TaskCreateResponse, listener *pluginTaskUrlListener.PluginWithContribution, batchSize int) (<-chan *CreateTaskStreamChan, error) {
	outChan := make(chan *CreateTaskStreamChan)

	go func() {
		defer close(outChan)

		var parentTask *domain.Task
		siteCache := make(map[string]int)
		batch := make([]*domain.Task, 0, batchSize)

		// 确保批量保存
		flushBatch := func() {
			if len(batch) > 0 {
				if err := s.repo.SaveBatch(ctx, batch); err != nil {
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

			children := taskResp.Children
			if len(children) == 0 {
				continue
			}

			// 处理父任务信息
			if parentTask == nil || (parentTask.TaskName.Valid && parentTask.TaskName.String != taskResp.TaskName) {
				flushBatch()

				parentTask = &domain.Task{
					BaseEntity: &pkgModel.BaseEntity{},
				}
				parentTask.TaskName = sql.NullString{String: taskResp.TaskName, Valid: true}
				parentTask.URL = sql.NullString{String: taskResp.URL, Valid: true}
				parentTask.Status = int(TaskStatusCreated)
				parentTask.IsCollection = sql.NullInt64{Int64: 1, Valid: true}
				parentTask.PluginPublicID = listener.PublicID
				parentTask.PluginContributionID = sql.NullString{String: listener.ContributionID, Valid: true}

				// 获取站点id
				if taskResp.SiteName != "" {
					siteId, ok := siteCache[taskResp.SiteName]
					if !ok {
						if site, err := s.siteSvc.GetByName(ctx, taskResp.SiteName); err == nil && site != nil {
							siteId = int(site.ID)
							siteCache[taskResp.SiteName] = siteId
						}
					}
					parentTask.SiteID = sql.NullInt64{Int64: int64(siteId), Valid: true}
				}

				if err := s.repo.CreateTask(ctx, parentTask); err != nil {
					outChan <- &CreateTaskStreamChan{Error: err}
					parentTask = nil
					continue
				}
				outChan <- &CreateTaskStreamChan{Parent: parentTask}
			}

			// 处理子任务
			for _, childResp := range children {
				childTask := &domain.Task{
					BaseEntity: &pkgModel.BaseEntity{},
				}
				childTask.Pid = sql.NullInt64{Int64: parentTask.GetID(), Valid: true}
				childTask.TaskName = sql.NullString{String: childResp.TaskName, Valid: true}
				childTask.SiteWorkID = sql.NullString{String: childResp.SiteWorkID, Valid: true}
				childTask.URL = sql.NullString{String: childResp.URL, Valid: true}
				childTask.Status = int(TaskStatusCreated)
				childTask.IsCollection = sql.NullInt64{Int64: 0, Valid: true}
				childTask.PluginPublicID = listener.PublicID
				childTask.PluginContributionID = sql.NullString{String: listener.ContributionID, Valid: true}

				// 获取站点id
				if childResp.SiteName != "" {
					siteId, ok := siteCache[childResp.SiteName]
					if !ok {
						if site, err := s.siteSvc.GetByName(ctx, childResp.SiteName); err == nil && site != nil {
							siteId = int(site.ID)
							siteCache[childResp.SiteName] = siteId
						}
					}
					childTask.SiteID = sql.NullInt64{Int64: int64(siteId), Valid: true}
				}

				// 处理 pluginData
				if childResp.PluginData != "" {
					childTask.PluginData = sql.NullString{String: childResp.PluginData, Valid: true}
				}

				batch = append(batch, childTask)

				// 达到批量大小时保存
				if len(batch) >= batchSize {
					flushBatch()
				}

				outChan <- &CreateTaskStreamChan{Task: childTask}
			}
		}

		flushBatch()
	}()

	return outChan, nil
}
