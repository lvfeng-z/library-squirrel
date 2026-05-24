package task

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
	pkgerr "github.com/library-squirrel/backend/error"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model"
	querypkg "github.com/library-squirrel/backend/base/query"
	"github.com/library-squirrel/backend/database"
	"github.com/library-squirrel/backend/pluginTaskUrlListener"
	"github.com/library-squirrel/backend/site"
	"github.com/library-squirrel/backend/util"
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

// Repository 任务仓储接口（由 service 定义需要的数据库操作方法）
type Repository interface {
	// Save 保存
	Save(ctx context.Context, task *entity.Task) error
	// SaveBatch 批量保存
	SaveBatch(ctx context.Context, tasks []*entity.Task) error
	// Update 更新
	Update(ctx context.Context, task *entity.Task) error
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
		// SetStatus 设置指定任务的状态（不级联）
		SetStatus(ctx context.Context, taskId int64, status TaskStatusEnum) error
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
	// BatchSetStatus 批量设置任务状态
	BatchSetStatus(ctx context.Context, statuses map[int64]TaskStatusEnum) error
}

// taskProgressTreeBuilder 任务进度树构建器，复用通用 TreeBuilder
var taskProgressTreeBuilder = util.NewTreeBuilder[*dto.TaskProgressTreeDTO](
	func(node *dto.TaskProgressTreeDTO) int64 { return node.TaskProgress.Task.ID },
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

// WorkSaver 作品保存接口
type WorkSaver interface {
	// Save 保存作品
	Save(ctx context.Context, work *entity.Work) error
}

// ResourceSaver 资源保存接口
type ResourceSaver interface {
	// Save 保存资源
	Save(ctx context.Context, resource *entity.Resource) error
}

// TaskHandlerProvider 任务处理器提供者接口
// 用于获取插件的任务处理器，解耦 task 模块对 plugin 模块的直接依赖
type TaskHandlerProvider interface {
	// GetTaskHandler 获取任务处理器
	GetTaskHandler(pluginPublicId, contributionId string) (dto.TaskHandler, error)
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
func (s *Service) GetById(ctx context.Context, id int64) (*entity.Task, error) {
	return s.repo.GetById(ctx, id)
}

// Save 保存任务
func (s *Service) Save(ctx context.Context, task *entity.Task) error {
	return s.repo.Save(ctx, task)
}

// SaveBatch 批量保存任务
func (s *Service) SaveBatch(ctx context.Context, tasks []*entity.Task) error {
	return s.repo.SaveBatch(ctx, tasks)
}

// Update 更新任务
func (s *Service) Update(ctx context.Context, task *entity.Task) error {
	return s.repo.Update(ctx, task)
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
	conv := querypkg.NewConverter(entity.Task{})
	opt, err := conv.ToPageOption(query, page.PageNumber, page.PageSize, nil)
	if err != nil {
		return nil, err
	}
	return s.repo.Page(ctx, opt)
}

// QueryParentPage 分页查询父任务
func (s *Service) QueryParentPage(ctx context.Context, page *model.Page[entity.Task], query TaskQueryDTO) (*model.Page[entity.Task], error) {
	conv := querypkg.NewConverter(entity.Task{})
	opt, err := conv.ToPageOption(query, page.PageNumber, page.PageSize, nil)
	if err != nil {
		return nil, err
	}
	return s.repo.QueryParentPage(ctx, opt)
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
		BaseEntity:           &model.BaseEntity{},
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
func (s *Service) QueryTreeDataPage(ctx context.Context, page, pageSize int, queryDTO *TaskQueryDTO) (*dto.TreeDataPageDTO, error) {
	conv := querypkg.NewConverter(entity.Task{})
	opt, err := conv.ToPageOption(queryDTO, page, pageSize, nil)
	if err != nil {
		return nil, err
	}

	// 分页查询父任务（is_collection=1 OR pid IS NULL OR pid=0）
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

// QueryChildrenTaskPage 查询子任务分页
func (s *Service) QueryChildrenTaskPage(ctx context.Context, page *model.Page[entity.Task], query TaskQueryDTO) (*model.Page[entity.Task], error) {
	conv := querypkg.NewConverter(entity.Task{})
	opt, err := conv.ToPageOption(query, page.PageNumber, page.PageSize, nil)
	if err != nil {
		return nil, err
	}
	return s.repo.QueryChildrenTaskPage(ctx, opt)
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

// SaveWorkInfo 保存作品信息
func (s *Service) SaveWorkInfo(ctx context.Context, work *entity.Work, resources []*entity.Resource) error {
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
	task, err := s.repo.GetById(ctx, taskId)
	if err != nil {
		return err
	}

	if task.Pid.Valid && task.Pid.Int64 != 0 {
		_, err := s.repo.RefreshTaskStatus(ctx, task.Pid.Int64)
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
		return &CreateTaskByURLResponse{
			Succeed:       false,
			AddedQuantity: 0,
			Msg:           fmt.Sprintf("url不受支持，url: %s", url),
		}, nil
	}

	// 2. 按照排序尝试每个插件
	for _, listener := range listeners {
		if !listener.PublicID.Valid || listener.PublicID.String == "" {
			logger.Log.Warnf("URL监听器缺少插件 PublicID，跳过 (contributionId=%s)", listener.ContributionID)
			continue
		}
		pluginPublicId := listener.PublicID.String

		// 获取任务处理器
		taskHandler, err := s.taskHandlerGetter.GetTaskHandler(pluginPublicId, listener.ContributionID)
		if err != nil {
			logger.Log.Warnf("获取任务处理器失败 (plugin=%s, contributionId=%s): %v", pluginPublicId, listener.ContributionID, err)
			continue
		}

		// 3. 调用插件的 create 方法
		pluginResponses, err := taskHandler.Create(url)
		if err != nil {
			logger.Log.Errorf("插件创建任务失败 (plugin=%s): %v", pluginPublicId, err)
			continue
		}

		// 4. 处理插件返回
		if len(pluginResponses) > 0 {
			count, err := s.handleCreateTaskArray(ctx, pluginResponses, url, listener)
			if err != nil {
				logger.Log.Errorf("处理插件返回数据失败 (plugin=%s): %v", pluginPublicId, err)
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
func (s *Service) handleCreateTaskArray(ctx context.Context, pluginResponses []*dto.TaskCreateResponse, url string, listener *pluginTaskUrlListener.PluginWithContribution) (int, error) {
	if len(pluginResponses) == 0 {
		return 0, nil
	}

	childrenCount := 0
	siteCache := make(map[string]int) // siteName -> siteId 缓存

	// 给任务赋值的函数
	assignTask := func(task *entity.Task, taskResp *dto.TaskCreateResponse, pid int64) error {
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
			task := &entity.Task{
				BaseEntity: &model.BaseEntity{},
			}
			childResp := children[0]
			if err := assignTask(task, &dto.TaskCreateResponse{
				TaskName:   childResp.TaskName,
				SiteWorkID: childResp.SiteWorkID,
				URL:        childResp.URL,
				SiteName:   childResp.SiteName,
				PluginData: childResp.PluginData,
			}, 0); err != nil {
				continue
			}
			if err := s.repo.CreateTask(ctx, task); err != nil {
				continue
			}
			childrenCount++
			continue
		}

		// 多个子任务：先创建父任务
		parentTask := &entity.Task{
			BaseEntity: &model.BaseEntity{},
		}
		if err := assignTask(parentTask, &dto.TaskCreateResponse{
			TaskName: parentResp.TaskName,
			URL:      parentResp.URL,
			SiteName: parentResp.SiteName,
		}, 0); err != nil {
			continue
		}
		parentTask.IsCollection = sql.NullInt64{Int64: 1, Valid: true} // 集合任务
		if err := s.repo.CreateTask(ctx, parentTask); err != nil {
			continue
		}
		parentId := parentTask.GetID()

		// 创建子任务
		for _, childResp := range children {
			childTask := &entity.Task{
				BaseEntity: &model.BaseEntity{},
			}
			if err := assignTask(childTask, &dto.TaskCreateResponse{
				TaskName:   childResp.TaskName,
				SiteWorkID: childResp.SiteWorkID,
				URL:        childResp.URL,
				SiteName:   childResp.SiteName,
				PluginData: childResp.PluginData,
			}, parentId); err != nil {
				continue
			}
			if err := s.repo.CreateTask(ctx, childTask); err != nil {
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
	Task   *entity.Task
	Parent *entity.Task // 父任务（如果是子任务的话）
	Error  error
}

// handleCreateTaskStream 处理插件返回的流式任务（使用 Go channel）
// 该方法会启动一个 goroutine 来读取任务流，并通过 channel 返回结果
func (s *Service) handleCreateTaskStream(ctx context.Context, taskChan <-chan *dto.TaskCreateResponse, listener *pluginTaskUrlListener.PluginWithContribution, batchSize int) (<-chan *CreateTaskStreamChan, error) {
	outChan := make(chan *CreateTaskStreamChan)

	go func() {
		defer close(outChan)

		var parentTask *entity.Task
		siteCache := make(map[string]int)
		batch := make([]*entity.Task, 0, batchSize)

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

				parentTask = &entity.Task{
					BaseEntity: &model.BaseEntity{},
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
				childTask := &entity.Task{
					BaseEntity: &model.BaseEntity{},
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
