package externalapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/library-squirrel/backend/base/model"
	dto "github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/journal"
	"github.com/library-squirrel/backend/plugin"
	"github.com/library-squirrel/backend/storageLocation"
	"github.com/library-squirrel/backend/task"
	"github.com/library-squirrel/backend/taskManager"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// 只读操作面：作品搜索、作品详情与批量完整信息、任务分页/详情/状态、任务运行态快照/树状态/
// 空闲判定、插件分页/状态、存储位置、作品资源行、记账账表增量查询。每个操作 = 操作标识 +
// 参数适配 + 目标 handler 方法调用 + 信封解包——过滤语义、分页与缺省一律沿用 handler 现状，
// 本文件不复制业务逻辑、不直访仓储。所需方法面在此按调用方需要声明（各业务模块 handler 以
// 方法集满足），装配期由调用方注入

const (
	// JournalPath 记账账表增量查询端点路径（GET，查询参数 cursor/limit）
	JournalPath = "/api/journal"
	// defaultQueryPageSize 分页读操作的每页行数缺省（与库层分页缺省同一口径：第 1 页、每页 10 行）
	defaultQueryPageSize = 10
)

// 操作标识（统一操作端点 POST /api/op/{标识} 的路径段；能力清单的操作目录以此为准）
const (
	// opSearchWork 作品搜索
	opSearchWork = "search.work"
	// opWorkDetail 作品详情
	opWorkDetail = "work.detail"
	// opWorkFullInfo 作品批量完整信息
	opWorkFullInfo = "work.fullInfo"
	// opTaskPage 任务分页
	opTaskPage = "task.page"
	// opTaskDetail 任务详情
	opTaskDetail = "task.detail"
	// opTaskStatus 任务状态列表
	opTaskStatus = "task.status"
	// opTaskManagerSnapshot 活跃任务完整状态快照
	opTaskManagerSnapshot = "taskManager.snapshot"
	// opTaskManagerTreeState 任务树状态
	opTaskManagerTreeState = "taskManager.treeState"
	// opTaskManagerIdle 任务管理器空闲判定
	opTaskManagerIdle = "taskManager.idle"
	// opPluginPage 插件分页
	opPluginPage = "plugin.page"
	// opPluginStatus 插件状态
	opPluginStatus = "plugin.status"
	// opStorageLocationList 三目录存储位置
	opStorageLocationList = "storageLocation.list"
	// opResourceListByWork 按作品列资源行
	opResourceListByWork = "resource.listByWork"
	// opJournalSince 记账账表增量查询（同时是 GET /api/journal 端点背后的操作）
	opJournalSince = "journal.since"
)

// SearchQueryHandler 作品搜索的查询方法面（*search.Handler 满足）
type SearchQueryHandler interface {
	// QueryWorkPage 按搜索条件分页查询作品完整信息
	QueryWorkPage(ctx context.Context, pageNumber, pageSize int, conditions []*dto.SearchCondition) *model.ApiResponse[*model.Page[dto.WorkFullDTO]]
}

// WorkQueryHandler 作品的查询方法面（*work.Handler 满足）
type WorkQueryHandler interface {
	// GetById 按 ID 取作品
	GetById(ctx context.Context, id int64) *model.ApiResponse[*sdkdto.WorkDTO]
	// GetFullWorkInfoByIds 批量取作品完整信息（资源/作者/标签/站点一次组装）
	GetFullWorkInfoByIds(ctx context.Context, ids []int64) *model.ApiResponse[[]*dto.WorkFullDTO]
}

// TaskQueryHandler 任务的查询方法面（*task.Handler 满足）
type TaskQueryHandler interface {
	// QueryPage 按任务查询条件分页
	QueryPage(ctx context.Context, page *model.Page[sdkdto.TaskDTO], query task.TaskQueryDTO) *model.ApiResponse[*model.Page[sdkdto.TaskDTO]]
	// GetById 按 ID 取任务（核心行 + 作品任务领域行组装）
	GetById(ctx context.Context, id int64) *model.ApiResponse[*sdkdto.TaskDTO]
	// ListStatus 批量取任务进度与状态
	ListStatus(ctx context.Context, ids []int64) *model.ApiResponse[[]*dto.TaskProgressDTO]
}

// TaskManagerQueryHandler 任务运行态的查询方法面（*taskManager.Handler 满足）
type TaskManagerQueryHandler interface {
	// GetTaskSnapshot 取当前活跃任务与移除缓冲区的完整状态快照
	GetTaskSnapshot() *model.ApiResponse[*taskManager.TaskSnapshotDTO]
	// GetTaskTreeState 取任务树状态（父任务聚合、叶子与独立任务取自身）
	GetTaskTreeState(taskId int64) *model.ApiResponse[int]
	// IsIdle 判定任务管理器是否空闲
	IsIdle() *model.ApiResponse[bool]
}

// PluginQueryHandler 插件的查询方法面（*plugin.Handler 满足）
type PluginQueryHandler interface {
	// Page 按插件查询条件分页
	Page(ctx context.Context, page *model.Page[dto.PluginDTO], query plugin.PluginQueryDTO) *model.ApiResponse[*model.Page[dto.PluginDTO]]
	// GetPluginStatus 取插件生命周期与运行时状态
	GetPluginStatus(ctx context.Context, pluginPublicId string) *model.ApiResponse[*plugin.PluginStatusDTO]
}

// StorageLocationQueryHandler 存储位置的查询方法面（*storageLocation.Handler 满足）
type StorageLocationQueryHandler interface {
	// GetStorageLocations 取程序目录/用户数据目录/资源库目录
	GetStorageLocations(ctx context.Context) *model.ApiResponse[*storageLocation.StorageLocations]
}

// ResourceQueryHandler 作品资源行的查询方法面（*resource.Handler 满足）
type ResourceQueryHandler interface {
	// ListByWorkId 按作品 ID 列资源行
	ListByWorkId(ctx context.Context, workId int64) *model.ApiResponse[[]*dto.ResourceDTO]
}

// JournalQueryHandler 记账账表的游标读取面（*journal.Service 满足）
type JournalQueryHandler interface {
	// ListSince 取游标之后按 id 升序的账行、下一游标与续读/过期标记
	ListSince(ctx context.Context, cursor int64, limit int) (*journal.ListResult, error)
}

// QueryDeps 只读操作的目标方法面（装配期注入）。任一项缺失即注册报错——缺实现的操作标识
// 会误导外部调用方，宁缺勿错
type QueryDeps struct {
	Search          SearchQueryHandler
	Work            WorkQueryHandler
	Task            TaskQueryHandler
	TaskManager     TaskManagerQueryHandler
	Plugin          PluginQueryHandler
	StorageLocation StorageLocationQueryHandler
	Resource        ResourceQueryHandler
	Journal         JournalQueryHandler
}

// 参数摘要（能力清单的参数 schema 摘要；名称与请求体 JSON 字段名一致）
var (
	// searchWorkParamSpec 作品搜索参数摘要
	searchWorkParamSpec = []OperationParam{
		{Name: "pageNumber", Type: "int", Desc: "页码（从 1 起；非正数取第 1 页）"},
		{Name: "pageSize", Type: "int", Desc: "每页行数（非正数取 10 行）"},
		{Name: "conditions", Type: "array", Desc: "搜索条件数组，元素 {type,value,operator,namespace}；type：1 本地标签 2 站点标签 3 本地作者 4 站点作者 5 站点名称 6 作者昵称 7 上传时间 8 最近浏览 9 媒体类型 10 站点 11 作品集 12 入库时间 13 软删时间；operator：= != > < >= <= LIKE（缺省 =）；namespace 仅标签条件生效"},
	}
	// workDetailParamSpec 作品详情参数摘要
	workDetailParamSpec = []OperationParam{
		{Name: "id", Type: "int64", Required: true, Desc: "作品 ID"},
	}
	// workFullInfoParamSpec 作品批量完整信息参数摘要
	workFullInfoParamSpec = []OperationParam{
		{Name: "ids", Type: "array", Required: true, Desc: "作品 ID 数组（元素为整数）"},
	}
	// taskPageParamSpec 任务分页参数摘要
	taskPageParamSpec = []OperationParam{
		{Name: "page", Type: "object", Desc: "分页对象 {pageNumber,pageSize}；省略或零值由库层取缺省（第 1 页、每页 10 行）"},
		{Name: "query", Type: "object", Desc: "任务查询条件，字段与任务查询 DTO 同名（pid/siteId/siteWorkId/status/taskType/hasChild/pluginPublicId/pluginExtensionId/continuable/taskName/createTime/updateTime），每字段形如 {value,operator,order,priority}；operator 缺省 =，order 指定 asc/desc 即参与排序"},
	}
	// taskDetailParamSpec 任务详情参数摘要
	taskDetailParamSpec = []OperationParam{
		{Name: "id", Type: "int64", Required: true, Desc: "任务 ID"},
	}
	// taskStatusParamSpec 任务状态列表参数摘要
	taskStatusParamSpec = []OperationParam{
		{Name: "ids", Type: "array", Required: true, Desc: "任务 ID 数组（元素为整数）"},
	}
	// taskManagerSnapshotParamSpec 任务快照参数摘要（无参数）
	taskManagerSnapshotParamSpec = []OperationParam{}
	// taskManagerTreeStateParamSpec 任务树状态参数摘要
	taskManagerTreeStateParamSpec = []OperationParam{
		{Name: "taskId", Type: "int64", Required: true, Desc: "任务 ID"},
	}
	// taskManagerIdleParamSpec 任务管理器空闲判定参数摘要（无参数）
	taskManagerIdleParamSpec = []OperationParam{}
	// pluginPageParamSpec 插件分页参数摘要
	pluginPageParamSpec = []OperationParam{
		{Name: "page", Type: "object", Desc: "分页对象 {pageNumber,pageSize}；省略或零值由库层取缺省（第 1 页、每页 10 行）"},
		{Name: "query", Type: "object", Desc: "插件查询条件，字段与插件查询 DTO 同名（publicId/name/author/source/version/activationType/uninstalled/createTime/updateTime），每字段形如 {value,operator,order,priority}；operator 缺省 ="},
	}
	// pluginStatusParamSpec 插件状态参数摘要
	pluginStatusParamSpec = []OperationParam{
		{Name: "pluginPublicId", Type: "string", Required: true, Desc: "插件公开 ID"},
	}
	// storageLocationListParamSpec 存储位置参数摘要（无参数）
	storageLocationListParamSpec = []OperationParam{}
	// resourceListByWorkParamSpec 作品资源行参数摘要
	resourceListByWorkParamSpec = []OperationParam{
		{Name: "workId", Type: "int64", Required: true, Desc: "作品 ID"},
	}
	// journalQueryParamSpec 记账增量查询参数摘要（注册表操作与 GET /api/journal 查询参数共用：
	// 查询参数形态即同名字段，故清单只维护一份摘要）
	journalQueryParamSpec = []OperationParam{
		{Name: "cursor", Type: "int64", Desc: "消费游标：返回 id 严格大于该值的账行，0 = 从头消费"},
		{Name: "limit", Type: "int", Desc: "本批行数上限（非正数取缺省 100 行）"},
	}
)

// RegisterQueryOperations 把只读操作装入注册表并登记各操作的参数摘要（能力清单数据源）。
// 依赖面缺项即报错且不装入任何操作；操作标识重复等注册表错误原样上抛（装配期错误）
func RegisterQueryOperations(r *Registry, deps QueryDeps) error {
	if r == nil {
		return errors.New("外部操作注册表为空")
	}
	switch {
	case deps.Search == nil:
		return errors.New("作品搜索查询面未注入")
	case deps.Work == nil:
		return errors.New("作品查询面未注入")
	case deps.Task == nil:
		return errors.New("任务查询面未注入")
	case deps.TaskManager == nil:
		return errors.New("任务运行态查询面未注入")
	case deps.Plugin == nil:
		return errors.New("插件查询面未注入")
	case deps.StorageLocation == nil:
		return errors.New("存储位置查询面未注入")
	case deps.Resource == nil:
		return errors.New("资源行查询面未注入")
	case deps.Journal == nil:
		return errors.New("记账账表读取面未注入")
	}

	// 作品搜索：分页值为整数对且库层该路径按偏移直算，零值会退化为空页，故在操作入口补缺省
	if err := registerOperation(r, opSearchWork, "作品搜索：按搜索条件筛选作品并按页返回完整信息（资源/作者/标签/站点）", searchWorkParamSpec,
		func(ctx context.Context, p searchWorkParams) (any, error) {
			return unwrapResponse(deps.Search.QueryWorkPage(ctx, normalizeQueryPageNumber(p.PageNumber), normalizeQueryPageSize(p.PageSize), p.Conditions))
		}); err != nil {
		return err
	}

	// 作品详情
	if err := registerOperation(r, opWorkDetail, "作品详情：按作品 ID 取作品核心信息", workDetailParamSpec,
		func(ctx context.Context, p workDetailParams) (any, error) {
			return unwrapResponse(deps.Work.GetById(ctx, p.ID))
		}); err != nil {
		return err
	}

	// 作品批量完整信息
	if err := registerOperation(r, opWorkFullInfo, "作品批量完整信息：按作品 ID 数组一次取回资源/作者/标签/站点", workFullInfoParamSpec,
		func(ctx context.Context, p workFullInfoParams) (any, error) {
			return unwrapResponse(deps.Work.GetFullWorkInfoByIds(ctx, p.IDs))
		}); err != nil {
		return err
	}

	// 任务分页：分页对象按入参原样交给 handler（nil 与零值由 handler 与库层兜底）
	if err := registerOperation(r, opTaskPage, "任务分页查询：按任务查询条件筛选并按页返回任务（含作品任务领域行）", taskPageParamSpec,
		func(ctx context.Context, p taskPageParams) (any, error) {
			return unwrapResponse(deps.Task.QueryPage(ctx, p.Page, p.Query))
		}); err != nil {
		return err
	}

	// 任务详情
	if err := registerOperation(r, opTaskDetail, "任务详情：按任务 ID 取任务核心行与作品任务领域行", taskDetailParamSpec,
		func(ctx context.Context, p taskDetailParams) (any, error) {
			return unwrapResponse(deps.Task.GetById(ctx, p.ID))
		}); err != nil {
		return err
	}

	// 任务状态
	if err := registerOperation(r, opTaskStatus, "任务状态：按任务 ID 数组取进度与状态", taskStatusParamSpec,
		func(ctx context.Context, p taskStatusParams) (any, error) {
			return unwrapResponse(deps.Task.ListStatus(ctx, p.IDs))
		}); err != nil {
		return err
	}

	// 任务快照
	if err := registerOperation(r, opTaskManagerSnapshot, "任务运行态快照：当前活跃任务与被移除任务的完整状态", taskManagerSnapshotParamSpec,
		func(ctx context.Context, _ noParams) (any, error) {
			return unwrapResponse(deps.TaskManager.GetTaskSnapshot())
		}); err != nil {
		return err
	}

	// 任务树状态
	if err := registerOperation(r, opTaskManagerTreeState, "任务树状态：父任务取聚合状态、叶子与独立任务取自身状态", taskManagerTreeStateParamSpec,
		func(ctx context.Context, p taskManagerTreeStateParams) (any, error) {
			return unwrapResponse(deps.TaskManager.GetTaskTreeState(p.TaskID))
		}); err != nil {
		return err
	}

	// 任务管理器空闲判定
	if err := registerOperation(r, opTaskManagerIdle, "任务管理器空闲判定：是否无活跃任务（供外部侧决定何时收尾）", taskManagerIdleParamSpec,
		func(ctx context.Context, _ noParams) (any, error) {
			return unwrapResponse(deps.TaskManager.IsIdle())
		}); err != nil {
		return err
	}

	// 插件分页
	if err := registerOperation(r, opPluginPage, "插件分页查询：按插件查询条件筛选并按页返回插件", pluginPageParamSpec,
		func(ctx context.Context, p pluginPageParams) (any, error) {
			return unwrapResponse(deps.Plugin.Page(ctx, p.Page, p.Query))
		}); err != nil {
		return err
	}

	// 插件状态
	if err := registerOperation(r, opPluginStatus, "插件状态：生命周期状态、运行时信息、扩展点与参与度概要", pluginStatusParamSpec,
		func(ctx context.Context, p pluginStatusParams) (any, error) {
			return unwrapResponse(deps.Plugin.GetPluginStatus(ctx, p.PluginPublicID))
		}); err != nil {
		return err
	}

	// 存储位置
	if err := registerOperation(r, opStorageLocationList, "存储位置：程序目录、用户数据目录与资源库目录的绝对路径（未配置资源库目录时为空串）", storageLocationListParamSpec,
		func(ctx context.Context, _ noParams) (any, error) {
			return unwrapResponse(deps.StorageLocation.GetStorageLocations(ctx))
		}); err != nil {
		return err
	}

	// 作品资源行
	if err := registerOperation(r, opResourceListByWork, "作品资源行：按作品 ID 列出资源（含类型与各角色 store）", resourceListByWorkParamSpec,
		func(ctx context.Context, p resourceListByWorkParams) (any, error) {
			return unwrapResponse(deps.Resource.ListByWorkId(ctx, p.WorkID))
		}); err != nil {
		return err
	}

	// 记账增量查询（返回形态本身即游标契约，不经 handler 信封）
	if err := registerOperation(r, opJournalSince, "事件记账增量查询：返回游标之后的账行、下一游标与续读/过期标记", journalQueryParamSpec,
		func(ctx context.Context, p journalSinceParams) (any, error) {
			result, err := deps.Journal.ListSince(ctx, p.Cursor, p.Limit)
			if err != nil {
				return nil, err
			}
			return result, nil
		}); err != nil {
		return err
	}

	return nil
}

// Registry 返回监听器持有的操作注册表：装配期在 handler 构造完成后据此注册操作（注册表由
// 监听器持有，进入运行期后即为只读目录）
func (s *Server) Registry() *Registry {
	return s.registry
}

// MountQueryRoutes 把注册表外的只读固定端点挂到路由表：能力清单、记账账表增量查询与
// 外部操作记录增量查询（免鉴权的启用态探测端点在监听器路由表内声明）。由监听器构建
// 路由时调用，每个路由表调用一次
func MountQueryRoutes(mux *http.ServeMux, s *Server) {
	mux.HandleFunc("GET "+ManifestPath, s.authenticated(s.handleManifest))
	mux.HandleFunc("GET "+JournalPath, s.authenticated(s.handleJournal))
	mux.HandleFunc("GET "+OplogPath, s.authenticated(s.handleOplog))
}

// handleJournal 记账账表增量查询端点：把查询参数翻译为该操作的入参后按同一操作执行路径调用
// （与 POST /api/op/journal.since 同一实现、同一信封语义、同一并发上限——查询参数形态只是
// 注册表操作的投影，不另立一套读取）。查询参数非法属操作失败：200 + 纯失败信封，不进入
// 业务调用
func (s *Server) handleJournal(w http.ResponseWriter, r *http.Request) {
	if !s.tryAcquireDispatchSlot() {
		writeJSON(w, http.StatusServiceUnavailable, model.Error[any]("外部接口并发已达上限，请稍后重试"))
		return
	}
	defer s.releaseDispatchSlot()

	params := journalSinceParams{}
	query := r.URL.Query()
	if raw := query.Get("cursor"); raw != "" {
		cursor, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusOK, model.Error[any]("cursor 参数非法：需为整数"))
			return
		}
		params.Cursor = cursor
	}
	if raw := query.Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil {
			writeJSON(w, http.StatusOK, model.Error[any]("limit 参数非法：需为整数"))
			return
		}
		params.Limit = limit
	}
	body, err := json.Marshal(params)
	if err != nil {
		writeJSON(w, http.StatusOK, model.Error[any](fmt.Sprintf("记账查询参数构造失败: %v", err)))
		return
	}
	op, ok := s.registry.Lookup(opJournalSince)
	if !ok {
		writeJSON(w, http.StatusServiceUnavailable, model.Error[any]("事件记账增量查询未接入"))
		return
	}
	started := time.Now()
	result, invokeErr := op.Invoke(r.Context(), json.RawMessage(body))
	s.recordInvocation(r.Context(), opJournalSince, body, invokeErr == nil, r.Header.Get(ClientLabelHeader), time.Since(started))
	if invokeErr != nil {
		writeJSON(w, http.StatusOK, model.Error[any](invokeErr.Error()))
		return
	}
	writeJSON(w, http.StatusOK, model.Success(result))
}

// tryAcquireDispatchSlot 非阻塞获取分发层并发名额（超限即返回 false，调用方快速失败）。
// 名额与统一操作端点共用同一信号量，故全部 HTTP 入口受同一并发上限约束
func (s *Server) tryAcquireDispatchSlot() bool {
	select {
	case s.dispatchSem <- struct{}{}:
		return true
	default:
		return false
	}
}

// releaseDispatchSlot 归还分发层并发名额
func (s *Server) releaseDispatchSlot() {
	<-s.dispatchSem
}

// registerOperation 注册操作并随注册登记参数摘要：注册表项与能力清单的参数摘要同批登记，
// 避免清单与注册表各维护一份操作表。泛型参数解码沿用注册表的注册辅助（解码失败即操作失败）
func registerOperation[P any](r *Registry, id, desc string, params []OperationParam, fn func(context.Context, P) (any, error)) error {
	if err := RegisterHandler(r, id, desc, fn); err != nil {
		return err
	}
	recordOperationParams(id, params)
	return nil
}

// unwrapResponse 把 handler 的同构信封转换为操作注册表的（载荷, 错误）形态：内外信封不嵌套，
// 分发层按同一 model.ApiResponse 序列化。失败信封只取 msg 作为失败原因，不携带业务载荷
func unwrapResponse[T any](resp *model.ApiResponse[T]) (any, error) {
	if resp == nil {
		return nil, errors.New("业务处理未返回响应")
	}
	if !resp.Success {
		msg := resp.Msg
		if msg == "" {
			msg = "操作未达成（业务处理未给出原因）"
		}
		return nil, errors.New(msg)
	}
	return resp.Data, nil
}

// normalizeQueryPageNumber 页码缺省：非正数取第 1 页
func normalizeQueryPageNumber(pageNumber int) int {
	if pageNumber <= 0 {
		return 1
	}
	return pageNumber
}

// normalizeQueryPageSize 每页行数缺省：非正数取 defaultQueryPageSize
func normalizeQueryPageSize(pageSize int) int {
	if pageSize <= 0 {
		return defaultQueryPageSize
	}
	return pageSize
}

// noParams 无参数操作的入参占位（请求体即使非空也不影响执行）
type noParams struct{}

// searchWorkParams 作品搜索入参（对齐作品搜索 handler 的形参）
type searchWorkParams struct {
	PageNumber int                    `json:"pageNumber"`
	PageSize   int                    `json:"pageSize"`
	Conditions []*dto.SearchCondition `json:"conditions"`
}

// workDetailParams 作品详情入参
type workDetailParams struct {
	ID int64 `json:"id"`
}

// workFullInfoParams 作品批量完整信息入参
type workFullInfoParams struct {
	IDs []int64 `json:"ids"`
}

// taskPageParams 任务分页入参（分页对象形态与 handler 形参一致）
type taskPageParams struct {
	Page  *model.Page[sdkdto.TaskDTO] `json:"page"`
	Query task.TaskQueryDTO           `json:"query"`
}

// taskDetailParams 任务详情入参
type taskDetailParams struct {
	ID int64 `json:"id"`
}

// taskStatusParams 任务状态入参
type taskStatusParams struct {
	IDs []int64 `json:"ids"`
}

// taskManagerTreeStateParams 任务树状态入参
type taskManagerTreeStateParams struct {
	TaskID int64 `json:"taskId"`
}

// pluginPageParams 插件分页入参（分页对象形态与 handler 形参一致）
type pluginPageParams struct {
	Page  *model.Page[dto.PluginDTO] `json:"page"`
	Query plugin.PluginQueryDTO      `json:"query"`
}

// pluginStatusParams 插件状态入参
type pluginStatusParams struct {
	PluginPublicID string `json:"pluginPublicId"`
}

// resourceListByWorkParams 作品资源行入参
type resourceListByWorkParams struct {
	WorkID int64 `json:"workId"`
}

// journalSinceParams 记账增量查询入参（游标契约的入参面）
type journalSinceParams struct {
	Cursor int64 `json:"cursor"`
	Limit  int   `json:"limit"`
}
