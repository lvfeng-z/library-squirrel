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
	"github.com/library-squirrel/wails/pkg/query"

	"gorm.io/gorm/clause"
)

// ========== 查询 DTO ==========

// TaskQueryDTO 任务查询条件
type TaskQueryDTO struct {
	ID                   query.QueryAttribute `json:"-" query:"id"`                                        // 任务ID（程序设置，不从JSON解析）
	Pid                  query.QueryAttribute `json:"pid" query:"pid"`                                     // 父任务ID
	SiteID               query.QueryAttribute `json:"siteId" query:"site_id"`                              // 站点ID
	SiteWorkID           query.QueryAttribute `json:"siteWorkId" query:"site_work_id"`                     // 站点作品ID
	Status               query.QueryAttribute `json:"status" query:"status"`                               // 任务状态
	IsCollection         query.QueryAttribute `json:"isCollection" query:"is_collection"`                  // 是否为合集（0=否，1=是）
	PluginPublicID       query.QueryAttribute `json:"pluginPublicId" query:"plugin_public_id"`             // 插件公开ID
	PluginContributionID query.QueryAttribute `json:"pluginContributionId" query:"plugin_contribution_id"` // 插件贡献ID
	Continuable          query.QueryAttribute `json:"continuable" query:"continuable"`                     // 是否可继续（0=否，1=是）
	TaskName             query.QueryAttribute `json:"taskName" query:"task_name"`                          // 任务名称（模糊匹配）
	CreateTime           query.QueryAttribute `json:"createTime" query:"create_time"`                      // 创建时间（可用于排序）
	UpdateTime           query.QueryAttribute `json:"updateTime" query:"update_time"`                      // 更新时间（可用于排序）
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
	Page(ctx context.Context, opt *database.PageOption) (*pkgModel.Page[domain.Task, TaskQueryDTO], error)
	// QueryParentPage 分页查询父任务
	QueryParentPage(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression) (*pkgModel.Page[domain.Task, TaskQueryDTO], error)
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
	QueryChildrenTaskPage(ctx context.Context, pid int64, page, pageSize int, where clause.Expression, order clause.Expression) (*pkgModel.Page[domain.Task, TaskQueryDTO], error)
	// ListSchedule 查询任务进度列表
	ListSchedule(ctx context.Context, ids []int64) ([]*TaskScheduleDTO, error)
	// DeleteTask 删除任务（包含子任务）- 批量删除
	DeleteTask(ctx context.Context, ids []int64) error
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

// TaskTreeDTO 任务树DTO
type TaskTreeDTO struct {
	ID                   int64          `json:"id"`
	Pid                  int64          `json:"pid"`
	TaskName             string         `json:"taskName"`
	SiteID               int            `json:"siteId"`
	SiteWorkID           string         `json:"siteWorkId"`
	URL                  string         `json:"url"`
	Status               int            `json:"status"`
	IsCollection         int            `json:"isCollection"`
	PluginPublicID       string         `json:"pluginPublicId"`
	PluginContributionID string         `json:"pluginContributionId"`
	PluginData           string         `json:"pluginData"`
	ErrorMessage         string         `json:"errorMessage"`
	Children             []*TaskTreeDTO `json:"children,omitempty"`
}

// ToTaskTreeDTO 将 domain.Task 转换为 TaskTreeDTO
func ToTaskTreeDTO(task *domain.Task) *TaskTreeDTO {
	if task == nil {
		return nil
	}
	return &TaskTreeDTO{
		ID:                   task.GetID(),
		Pid:                  nullInt64ToInt64(task.Pid),
		TaskName:             task.TaskName.String,
		SiteID:               int(nullInt64ToInt64(task.SiteID)),
		SiteWorkID:           task.SiteWorkID.String,
		URL:                  task.URL.String,
		Status:               task.Status,
		IsCollection:         int(nullInt64ToInt64(task.IsCollection)),
		PluginPublicID:       task.PluginPublicID.String,
		PluginContributionID: task.PluginContributionID.String,
		PluginData:           task.PluginData.String,
		ErrorMessage:         task.ErrorMessage.String,
	}
}

// nullInt64ToInt64 将 sql.NullInt64 转换为 int64
func nullInt64ToInt64(ni sql.NullInt64) int64 {
	if ni.Valid {
		return ni.Int64
	}
	return 0
}

// buildTaskTree 将任务列表构建为树形结构
func buildTaskTree(tasks []*domain.Task) []*TaskTreeDTO {
	if len(tasks) == 0 {
		return nil
	}

	// 转换为 DTO
	dtos := make([]*TaskTreeDTO, len(tasks))
	for i, task := range tasks {
		dtos[i] = ToTaskTreeDTO(task)
	}

	// 构建树形结构
	tree := make([]*TaskTreeDTO, 0)
	nodeMap := make(map[int64]*TaskTreeDTO)

	// 先按 pid 分组
	for _, dto := range dtos {
		nodeMap[dto.ID] = dto
	}

	// 遍历找到根节点（pid 为 0 或 nil）
	for _, dto := range dtos {
		if dto.Pid == 0 {
			tree = append(tree, dto)
		}
	}

	// 递归构建子树
	var buildChildren func(dto *TaskTreeDTO)
	buildChildren = func(dto *TaskTreeDTO) {
		dto.Children = make([]*TaskTreeDTO, 0)
		for _, node := range dtos {
			if node.Pid == dto.ID {
				dto.Children = append(dto.Children, node)
				buildChildren(node)
			}
		}
	}

	for _, root := range tree {
		buildChildren(root)
	}

	return tree
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
	Tasks    []*TaskTreeDTO `json:"tasks"`
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
func (s *Service) Page(ctx context.Context, opt *database.PageOption) (*pkgModel.Page[domain.Task, TaskQueryDTO], error) {
	return s.repo.Page(ctx, opt)
}

// PageByDTO 分页查询（基于 QueryDTO）
func (s *Service) PageByDTO(ctx context.Context, page, pageSize int, queryDTO *TaskQueryDTO) (*pkgModel.Page[domain.Task, TaskQueryDTO], error) {
	conv := query.NewConverter(domain.Task{})
	opt, err := conv.ToPageOption(queryDTO, page, pageSize)
	if err != nil {
		return nil, err
	}
	return s.repo.Page(ctx, opt)
}

// QueryParentPage 分页查询父任务
func (s *Service) QueryParentPage(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression) (*pkgModel.Page[domain.Task, TaskQueryDTO], error) {
	return s.repo.QueryParentPage(ctx, page, pageSize, where, order)
}

// QueryParentPageByDTO 分页查询父任务（基于 QueryDTO）
func (s *Service) QueryParentPageByDTO(ctx context.Context, page, pageSize int, queryDTO *TaskQueryDTO) (*pkgModel.Page[domain.Task, TaskQueryDTO], error) {
	conv := query.NewConverter(domain.Task{})
	queryOpt, err := conv.ToQueryOption(queryDTO)
	if err != nil {
		return nil, err
	}
	var where clause.Expression
	if len(queryOpt.Conditions) > 0 {
		where = queryOpt.Conditions[0]
	}
	var order clause.Expression
	if len(queryOpt.OrderBy) > 0 {
		order = queryOpt.OrderBy[0]
	}
	return s.repo.QueryParentPage(ctx, page, pageSize, where, order)
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

// DeleteTask 删除任务（包含子任务）- 批量删除
func (s *Service) DeleteTask(ctx context.Context, ids []int64) error {
	return s.repo.DeleteTask(ctx, ids)
}

// QueryTreeDataPage 查询任务树数据分页
func (s *Service) QueryTreeDataPage(ctx context.Context, page, pageSize int, queryDTO *TaskQueryDTO) (*TreeDataPageDTO, error) {
	conv := query.NewConverter(domain.Task{})
	queryOpt, err := conv.ToQueryOption(queryDTO)
	if err != nil {
		return nil, err
	}
	var where clause.Expression
	if len(queryOpt.Conditions) > 0 {
		where = queryOpt.Conditions[0]
	}
	var order clause.Expression
	if len(queryOpt.OrderBy) > 0 {
		order = queryOpt.OrderBy[0]
	}

	// 分页查询父任务（is_collection=1 OR pid IS NULL OR pid=0）
	resultPage, err := s.repo.QueryParentPage(ctx, page, pageSize, where, order)
	if err != nil {
		return nil, err
	}

	// 将分页数据转换为 TaskTreeDTO
	treeDTOS := make([]*TaskTreeDTO, len(resultPage.Data))
	for i, task := range resultPage.Data {
		treeDTOS[i] = ToTaskTreeDTO(task)
	}

	// 构建树形结构
	tree := buildTaskTreeByDTO(treeDTOS)

	// 获取 TreeID 和 TreeName（从分页数据中获取）
	var treeID int64
	var treeName string
	if len(resultPage.Data) > 0 {
		treeID = resultPage.Data[0].GetID()
		treeName = resultPage.Data[0].TaskName.String
	}

	return &TreeDataPageDTO{
		TreeID:   treeID,
		TreeName: treeName,
		Total:    resultPage.DataCount,
		Tasks:    tree,
	}, nil
}

// buildTaskTreeByDTO 将 TaskTreeDTO 列表构建为树形结构
func buildTaskTreeByDTO(dtos []*TaskTreeDTO) []*TaskTreeDTO {
	if len(dtos) == 0 {
		return nil
	}

	tree := make([]*TaskTreeDTO, 0)
	nodeMap := make(map[int64]*TaskTreeDTO)

	// 建立 ID 到节点的映射，并收集根节点
	for _, dto := range dtos {
		nodeMap[dto.ID] = dto
		if dto.Pid == 0 {
			tree = append(tree, dto)
		}
	}

	// 递归构建子树
	var buildChildren func(dto *TaskTreeDTO)
	buildChildren = func(dto *TaskTreeDTO) {
		dto.Children = make([]*TaskTreeDTO, 0)
		for _, node := range dtos {
			if node.Pid == dto.ID {
				dto.Children = append(dto.Children, node)
				buildChildren(node)
			}
		}
	}

	for _, root := range tree {
		buildChildren(root)
	}

	return tree
}

// ListChildrenTask 查询子任务列表
func (s *Service) ListChildrenTask(ctx context.Context, pid int64) ([]*domain.Task, error) {
	return s.repo.ListChildrenTask(ctx, pid)
}

// QueryChildrenTaskPage 查询子任务分页
func (s *Service) QueryChildrenTaskPage(ctx context.Context, pid int64, page, pageSize int, where clause.Expression, order clause.Expression) (*pkgModel.Page[domain.Task, TaskQueryDTO], error) {
	return s.repo.QueryChildrenTaskPage(ctx, pid, page, pageSize, where, order)
}

// QueryChildrenTaskPageByDTO 查询子任务分页（基于 QueryDTO）
func (s *Service) QueryChildrenTaskPageByDTO(ctx context.Context, pid int64, page, pageSize int, queryDTO *TaskQueryDTO) (*pkgModel.Page[domain.Task, TaskQueryDTO], error) {
	conv := query.NewConverter(domain.Task{})
	queryOpt, err := conv.ToQueryOption(queryDTO)
	if err != nil {
		return nil, err
	}
	var where clause.Expression
	if len(queryOpt.Conditions) > 0 {
		where = queryOpt.Conditions[0]
	}
	var order clause.Expression
	if len(queryOpt.OrderBy) > 0 {
		order = queryOpt.OrderBy[0]
	}
	return s.repo.QueryChildrenTaskPage(ctx, pid, page, pageSize, where, order)
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
