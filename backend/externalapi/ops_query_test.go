package externalapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/library-squirrel/backend/base/model"
	dto "github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/journal"
	"github.com/library-squirrel/backend/plugin"
	"github.com/library-squirrel/backend/storageLocation"
	"github.com/library-squirrel/backend/task"
	"github.com/library-squirrel/backend/taskManager"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// ========== 替身 handler 集：记录收到的入参并返回预置信封 ==========

// fakeSearchQuery 作品搜索替身
type fakeSearchQuery struct {
	gotPageNumber int
	gotPageSize   int
	gotConditions []*dto.SearchCondition
	failMsg       string
}

func (f *fakeSearchQuery) QueryWorkPage(ctx context.Context, pageNumber, pageSize int, conditions []*dto.SearchCondition) *model.ApiResponse[*model.Page[dto.WorkFullDTO]] {
	f.gotPageNumber, f.gotPageSize, f.gotConditions = pageNumber, pageSize, conditions
	if f.failMsg != "" {
		return model.Error[*model.Page[dto.WorkFullDTO]](f.failMsg)
	}
	return model.Success(&model.Page[dto.WorkFullDTO]{
		PageNumber:   pageNumber,
		PageSize:     pageSize,
		PageCount:    1,
		DataCount:    1,
		CurrentCount: 1,
		Data:         []*dto.WorkFullDTO{},
	})
}

// fakeWorkQuery 作品查询替身
type fakeWorkQuery struct {
	gotID   int64
	gotIDs  []int64
	failMsg string
	nilResp bool
}

func (f *fakeWorkQuery) GetById(ctx context.Context, id int64) *model.ApiResponse[*sdkdto.WorkDTO] {
	f.gotID = id
	if f.nilResp {
		return nil
	}
	if f.failMsg != "" {
		return model.Error[*sdkdto.WorkDTO](f.failMsg)
	}
	return model.Success(&sdkdto.WorkDTO{Id: id})
}

func (f *fakeWorkQuery) GetFullWorkInfoByIds(ctx context.Context, ids []int64) *model.ApiResponse[[]*dto.WorkFullDTO] {
	f.gotIDs = ids
	if f.failMsg != "" {
		return model.Error[[]*dto.WorkFullDTO](f.failMsg)
	}
	return model.Success([]*dto.WorkFullDTO{})
}

// fakeTaskQuery 任务查询替身
type fakeTaskQuery struct {
	gotID    int64
	gotIDs   []int64
	gotPage  *model.Page[sdkdto.TaskDTO]
	gotQuery task.TaskQueryDTO
	failMsg  string
}

func (f *fakeTaskQuery) QueryPage(ctx context.Context, page *model.Page[sdkdto.TaskDTO], query task.TaskQueryDTO) *model.ApiResponse[*model.Page[sdkdto.TaskDTO]] {
	f.gotPage, f.gotQuery = page, query
	if f.failMsg != "" {
		return model.Error[*model.Page[sdkdto.TaskDTO]](f.failMsg)
	}
	return model.Success(&model.Page[sdkdto.TaskDTO]{
		PageNumber: 1,
		PageSize:   10,
		PageCount:  1,
		Data:       []*sdkdto.TaskDTO{},
	})
}

func (f *fakeTaskQuery) GetById(ctx context.Context, id int64) *model.ApiResponse[*sdkdto.TaskDTO] {
	f.gotID = id
	if f.failMsg != "" {
		return model.Error[*sdkdto.TaskDTO](f.failMsg)
	}
	return model.Success(&sdkdto.TaskDTO{Id: id})
}

func (f *fakeTaskQuery) ListStatus(ctx context.Context, ids []int64) *model.ApiResponse[[]*dto.TaskProgressDTO] {
	f.gotIDs = ids
	if f.failMsg != "" {
		return model.Error[[]*dto.TaskProgressDTO](f.failMsg)
	}
	return model.Success([]*dto.TaskProgressDTO{})
}

// fakeTaskManagerQuery 任务运行态查询替身
type fakeTaskManagerQuery struct {
	gotTreeStateTaskID int64
	failMsg            string
}

func (f *fakeTaskManagerQuery) GetTaskSnapshot() *model.ApiResponse[*taskManager.TaskSnapshotDTO] {
	if f.failMsg != "" {
		return model.Error[*taskManager.TaskSnapshotDTO](f.failMsg)
	}
	return model.Success(&taskManager.TaskSnapshotDTO{Tasks: []*taskManager.TaskSnapshotItem{}})
}

func (f *fakeTaskManagerQuery) GetTaskTreeState(taskId int64) *model.ApiResponse[int] {
	f.gotTreeStateTaskID = taskId
	if f.failMsg != "" {
		return model.Error[int](f.failMsg)
	}
	return model.Success(3)
}

func (f *fakeTaskManagerQuery) IsIdle() *model.ApiResponse[bool] {
	if f.failMsg != "" {
		return model.Error[bool](f.failMsg)
	}
	return model.Success(true)
}

// fakePluginQuery 插件查询替身
type fakePluginQuery struct {
	gotPage           *model.Page[dto.PluginDTO]
	gotQuery          plugin.PluginQueryDTO
	gotStatusPublicID string
	failMsg           string
}

func (f *fakePluginQuery) Page(ctx context.Context, page *model.Page[dto.PluginDTO], query plugin.PluginQueryDTO) *model.ApiResponse[*model.Page[dto.PluginDTO]] {
	f.gotPage, f.gotQuery = page, query
	if f.failMsg != "" {
		return model.Error[*model.Page[dto.PluginDTO]](f.failMsg)
	}
	return model.Success(&model.Page[dto.PluginDTO]{
		PageNumber: 1,
		PageSize:   10,
		PageCount:  1,
		Data:       []*dto.PluginDTO{},
	})
}

func (f *fakePluginQuery) GetPluginStatus(ctx context.Context, pluginPublicId string) *model.ApiResponse[*plugin.PluginStatusDTO] {
	f.gotStatusPublicID = pluginPublicId
	if f.failMsg != "" {
		return model.Error[*plugin.PluginStatusDTO](f.failMsg)
	}
	return model.Success(&plugin.PluginStatusDTO{LifecycleState: "active"})
}

// fakeStorageLocationQuery 存储位置查询替身
type fakeStorageLocationQuery struct {
	failMsg string
}

func (f *fakeStorageLocationQuery) GetStorageLocations(ctx context.Context) *model.ApiResponse[*storageLocation.StorageLocations] {
	if f.failMsg != "" {
		return model.Error[*storageLocation.StorageLocations](f.failMsg)
	}
	return model.Success(&storageLocation.StorageLocations{ProgramDir: "program", DataDir: "data", WorkDir: "work"})
}

// fakeResourceQuery 资源行查询替身
type fakeResourceQuery struct {
	gotWorkID int64
	failMsg   string
}

func (f *fakeResourceQuery) ListByWorkId(ctx context.Context, workId int64) *model.ApiResponse[[]*dto.ResourceDTO] {
	f.gotWorkID = workId
	if f.failMsg != "" {
		return model.Error[[]*dto.ResourceDTO](f.failMsg)
	}
	return model.Success([]*dto.ResourceDTO{})
}

// fakeJournalQuery 记账账表读取替身
type fakeJournalQuery struct {
	gotCursor int64
	gotLimit  int
	result    *journal.ListResult
	err       error
}

func (f *fakeJournalQuery) ListSince(ctx context.Context, cursor int64, limit int) (*journal.ListResult, error) {
	f.gotCursor, f.gotLimit = cursor, limit
	if f.err != nil {
		return nil, f.err
	}
	if f.result != nil {
		return f.result, nil
	}
	entry := entity.NewJournalEntry()
	entry.SetID(7)
	entry.Domain = entity.JournalDomainTask
	entry.EventType = "task_terminal"
	entry.SubjectID = 42
	entry.Payload = `{"state":3}`
	return &journal.ListResult{
		Entries:       []*entity.JournalEntry{entry},
		NextCursor:    7,
		HasMore:       true,
		CursorExpired: false,
	}, nil
}

// queryFakes 只读操作替身集（按值持有，deps 取字段地址以便测试读取入参）
type queryFakes struct {
	search          fakeSearchQuery
	work            fakeWorkQuery
	task            fakeTaskQuery
	taskManager     fakeTaskManagerQuery
	plugin          fakePluginQuery
	storageLocation fakeStorageLocationQuery
	resource        fakeResourceQuery
	journal         fakeJournalQuery
}

// newQueryFakes 创建全依赖齐备的替身集
func newQueryFakes() *queryFakes {
	return &queryFakes{}
}

// deps 构造注册依赖面（指针指向替身字段，调用入参可被测试观察）
func (f *queryFakes) deps() QueryDeps {
	return QueryDeps{
		Search:          &f.search,
		Work:            &f.work,
		Task:            &f.task,
		TaskManager:     &f.taskManager,
		Plugin:          &f.plugin,
		StorageLocation: &f.storageLocation,
		Resource:        &f.resource,
		Journal:         &f.journal,
	}
}

// newQueryTestServer 经生产注册函数把只读操作装入注册表并构建服务
func newQueryTestServer(t *testing.T) (*Server, *queryFakes) {
	t.Helper()
	fakes := newQueryFakes()
	registry := NewRegistry()
	if err := RegisterQueryOperations(registry, fakes.deps()); err != nil {
		t.Fatalf("注册只读操作失败: %v", err)
	}
	srv, err := NewServer(registry, testToken)
	if err != nil {
		t.Fatalf("构建服务失败: %v", err)
	}
	return srv, fakes
}

// mountedQueryHandler 构建挂载了只读固定端点的路由表：与监听器构建路由时调用同一挂载函数，
// 端点的鉴权与信封语义随之与生产一致
func mountedQueryHandler(srv *Server) http.Handler {
	mux := http.NewServeMux()
	MountQueryRoutes(mux, srv)
	return mux
}

// callQueryOperation 以合法 token 调用统一操作端点并断言成功信封，返回信封视图
func callQueryOperation(t *testing.T, base, operationID, body string) envelopeView {
	t.Helper()
	resp, view := postOperation(t, base, "Bearer "+testToken, operationID, body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("操作 %s 期望 200，实际 %d", operationID, resp.StatusCode)
	}
	if !view.Success {
		t.Fatalf("操作 %s 期望成功信封，实际失败: %s", operationID, view.Msg)
	}
	if string(view.Data) == "null" {
		t.Fatalf("操作 %s 成功信封的 data 不应为 null", operationID)
	}
	return view
}

// getWithAuth 以指定鉴权头 GET 端点，返回原始响应与解码后的信封视图
func getWithAuth(t *testing.T, url, authHeader string) (*http.Response, envelopeView) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("构造请求失败: %v", err)
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("读取响应失败: %v", err)
	}
	var view envelopeView
	if err := json.Unmarshal(raw, &view); err != nil {
		t.Fatalf("响应不是合法 JSON 信封 %q: %v", string(raw), err)
	}
	return resp, view
}

// TestRegisterQueryOperationsRejectsMissingDeps 依赖面缺项属装配期错误：报错且不装入任何
// 操作（缺实现的操作标识会误导外部调用方）
func TestRegisterQueryOperationsRejectsMissingDeps(t *testing.T) {
	if err := RegisterQueryOperations(nil, newQueryFakes().deps()); err == nil {
		t.Fatal("注册表为空期望报错")
	}
	cases := []struct {
		name  string
		clear func(deps *QueryDeps)
	}{
		{"作品搜索", func(deps *QueryDeps) { deps.Search = nil }},
		{"作品", func(deps *QueryDeps) { deps.Work = nil }},
		{"任务", func(deps *QueryDeps) { deps.Task = nil }},
		{"任务运行态", func(deps *QueryDeps) { deps.TaskManager = nil }},
		{"插件", func(deps *QueryDeps) { deps.Plugin = nil }},
		{"存储位置", func(deps *QueryDeps) { deps.StorageLocation = nil }},
		{"资源行", func(deps *QueryDeps) { deps.Resource = nil }},
		{"记账", func(deps *QueryDeps) { deps.Journal = nil }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			deps := newQueryFakes().deps()
			c.clear(&deps)
			registry := NewRegistry()
			if err := RegisterQueryOperations(registry, deps); err == nil {
				t.Fatalf("缺少%s依赖期望注册报错", c.name)
			}
			if registered := registry.List(); len(registered) != 0 {
				t.Fatalf("注册报错时不应装入操作，实际 %d 条", len(registered))
			}
		})
	}
}

// TestManifestCoversRegisteredOperations 清单覆盖断言：注册表内每个操作在清单中都有一条含
// 参数摘要的条目（无参数的操作参数为空列表而非缺失），描述与注册表一致，且首批只读操作
// 全部在册；注册表外的固定端点（启用态探测、能力清单、记账增量查询）单列在册
func TestManifestCoversRegisteredOperations(t *testing.T) {
	srv, _ := newQueryTestServer(t)
	manifest := BuildManifest(srv.Registry())

	if manifest.Version != ManifestVersion {
		t.Fatalf("清单版本期望 %s，实际 %s", ManifestVersion, manifest.Version)
	}
	if manifest.AppVersion == "" {
		t.Fatal("清单应用版本不应为空")
	}
	if manifest.OperationEndpoint != opPathPrefix+"{operationId}" {
		t.Fatalf("操作端点模板期望 %s{operationId}，实际 %s", opPathPrefix, manifest.OperationEndpoint)
	}

	registered := srv.Registry().List()
	if len(manifest.Operations) != len(registered) {
		t.Fatalf("清单操作数期望与注册表一致（%d），实际 %d", len(registered), len(manifest.Operations))
	}
	byID := make(map[string]ManifestOperation, len(manifest.Operations))
	for _, operation := range manifest.Operations {
		byID[operation.ID] = operation
	}
	for _, op := range registered {
		entry, ok := byID[op.ID]
		if !ok {
			t.Fatalf("清单缺少已注册操作 %s", op.ID)
		}
		if entry.Desc != op.Desc {
			t.Fatalf("操作 %s 描述与注册表不一致：%q / %q", op.ID, entry.Desc, op.Desc)
		}
		if entry.Params == nil {
			t.Fatalf("操作 %s 缺少参数摘要", op.ID)
		}
		for _, param := range entry.Params {
			if param.Name == "" || param.Type == "" || param.Desc == "" {
				t.Fatalf("操作 %s 参数摘要不完整: %+v", op.ID, param)
			}
		}
	}

	wantIDs := []string{
		opSearchWork, opWorkDetail, opWorkFullInfo,
		opTaskPage, opTaskDetail, opTaskStatus,
		opTaskManagerSnapshot, opTaskManagerTreeState, opTaskManagerIdle,
		opPluginPage, opPluginStatus,
		opStorageLocationList, opResourceListByWork, opJournalSince,
	}
	for _, id := range wantIDs {
		if _, ok := byID[id]; !ok {
			t.Fatalf("清单缺少首批只读操作 %s", id)
		}
	}
	if len(wantIDs) != len(registered) {
		t.Fatalf("注册操作数期望 %d 条，实际 %d 条", len(wantIDs), len(registered))
	}
	if params := byID[opJournalSince].Params; len(params) != 2 {
		t.Fatalf("记账增量查询期望 2 个参数（cursor/limit），实际 %d 个", len(params))
	}
	if params := byID[opTaskManagerIdle].Params; len(params) != 0 {
		t.Fatalf("无参数操作的参数摘要期望空列表，实际 %d 个", len(params))
	}

	endpointPaths := make(map[string]ManifestEndpoint, len(manifest.Endpoints))
	for _, endpoint := range manifest.Endpoints {
		endpointPaths[endpoint.Path] = endpoint
	}
	for _, path := range []string{healthPath, ManifestPath, JournalPath} {
		if _, ok := endpointPaths[path]; !ok {
			t.Fatalf("清单缺少固定端点 %s", path)
		}
	}
	if endpointPaths[healthPath].Auth {
		t.Fatal("启用态探测端点应为免鉴权")
	}
	if !endpointPaths[JournalPath].Auth {
		t.Fatal("记账增量查询端点应需鉴权")
	}
	if len(endpointPaths[JournalPath].Params) != len(journalQueryParamSpec) {
		t.Fatalf("记账端点参数摘要期望 %d 个，实际 %d 个", len(journalQueryParamSpec), len(endpointPaths[JournalPath].Params))
	}
}

// TestQueryOperationsOverHTTP 每个读操作经统一操作端点走通：合法 token 调用返回同构成功
// 信封，且入参按操作声明送达目标 handler 方法
func TestQueryOperationsOverHTTP(t *testing.T) {
	cases := []struct {
		id     string
		body   string
		verify func(t *testing.T, fakes *queryFakes)
	}{
		{
			id:   opSearchWork,
			body: `{"pageNumber":2,"pageSize":5,"conditions":[{"type":1,"value":"猫"}]}`,
			verify: func(t *testing.T, fakes *queryFakes) {
				if fakes.search.gotPageNumber != 2 || fakes.search.gotPageSize != 5 {
					t.Fatalf("分页期望 2/5，实际 %d/%d", fakes.search.gotPageNumber, fakes.search.gotPageSize)
				}
				if len(fakes.search.gotConditions) != 1 {
					t.Fatalf("搜索条件期望 1 条，实际 %d 条", len(fakes.search.gotConditions))
				}
				condition := fakes.search.gotConditions[0]
				if condition.Type != dto.LocalTag || condition.Value != "猫" {
					t.Fatalf("搜索条件期望本地标签「猫」，实际 %+v", condition)
				}
			},
		},
		{
			id:   opWorkDetail,
			body: `{"id":3}`,
			verify: func(t *testing.T, fakes *queryFakes) {
				if fakes.work.gotID != 3 {
					t.Fatalf("作品 ID 期望 3，实际 %d", fakes.work.gotID)
				}
			},
		},
		{
			id:   opWorkFullInfo,
			body: `{"ids":[4,5]}`,
			verify: func(t *testing.T, fakes *queryFakes) {
				if len(fakes.work.gotIDs) != 2 || fakes.work.gotIDs[0] != 4 || fakes.work.gotIDs[1] != 5 {
					t.Fatalf("作品 ID 数组期望 [4 5]，实际 %v", fakes.work.gotIDs)
				}
			},
		},
		{
			id:   opTaskPage,
			body: `{"page":{"pageNumber":2,"pageSize":5},"query":{"status":{"value":3}}}`,
			verify: func(t *testing.T, fakes *queryFakes) {
				if fakes.task.gotPage == nil || fakes.task.gotPage.PageNumber != 2 || fakes.task.gotPage.PageSize != 5 {
					t.Fatalf("分页对象期望 2/5，实际 %+v", fakes.task.gotPage)
				}
				if fakes.task.gotQuery.Status.Value == nil || *fakes.task.gotQuery.Status.Value != 3 {
					t.Fatalf("任务查询条件期望 status=3，实际 %+v", fakes.task.gotQuery.Status.Value)
				}
			},
		},
		{
			id:   opTaskDetail,
			body: `{"id":8}`,
			verify: func(t *testing.T, fakes *queryFakes) {
				if fakes.task.gotID != 8 {
					t.Fatalf("任务 ID 期望 8，实际 %d", fakes.task.gotID)
				}
			},
		},
		{
			id:   opTaskStatus,
			body: `{"ids":[1,2,3]}`,
			verify: func(t *testing.T, fakes *queryFakes) {
				if len(fakes.task.gotIDs) != 3 {
					t.Fatalf("任务 ID 数组期望 3 个，实际 %v", fakes.task.gotIDs)
				}
			},
		},
		{
			id:   opTaskManagerSnapshot,
			body: `{}`,
			verify: func(t *testing.T, fakes *queryFakes) {
			},
		},
		{
			id:   opTaskManagerTreeState,
			body: `{"taskId":9}`,
			verify: func(t *testing.T, fakes *queryFakes) {
				if fakes.taskManager.gotTreeStateTaskID != 9 {
					t.Fatalf("任务树状态入参期望 9，实际 %d", fakes.taskManager.gotTreeStateTaskID)
				}
			},
		},
		{
			id:   opTaskManagerIdle,
			body: `{}`,
			verify: func(t *testing.T, fakes *queryFakes) {
			},
		},
		{
			id:   opPluginPage,
			body: `{"page":{"pageNumber":1,"pageSize":20},"query":{"name":{"value":"pixiv"}}}`,
			verify: func(t *testing.T, fakes *queryFakes) {
				if fakes.plugin.gotPage == nil || fakes.plugin.gotPage.PageSize != 20 {
					t.Fatalf("分页对象期望每页 20，实际 %+v", fakes.plugin.gotPage)
				}
				if fakes.plugin.gotQuery.Name.Value == nil || *fakes.plugin.gotQuery.Name.Value != "pixiv" {
					t.Fatalf("插件查询条件期望 name=pixiv，实际 %+v", fakes.plugin.gotQuery.Name.Value)
				}
			},
		},
		{
			id:   opPluginStatus,
			body: `{"pluginPublicId":"pixivSuite"}`,
			verify: func(t *testing.T, fakes *queryFakes) {
				if fakes.plugin.gotStatusPublicID != "pixivSuite" {
					t.Fatalf("插件公开 ID 期望 pixivSuite，实际 %q", fakes.plugin.gotStatusPublicID)
				}
			},
		},
		{
			id:   opStorageLocationList,
			body: `{}`,
			verify: func(t *testing.T, fakes *queryFakes) {
			},
		},
		{
			id:   opResourceListByWork,
			body: `{"workId":12}`,
			verify: func(t *testing.T, fakes *queryFakes) {
				if fakes.resource.gotWorkID != 12 {
					t.Fatalf("作品 ID 期望 12，实际 %d", fakes.resource.gotWorkID)
				}
			},
		},
		{
			id:   opJournalSince,
			body: `{"cursor":5,"limit":2}`,
			verify: func(t *testing.T, fakes *queryFakes) {
				if fakes.journal.gotCursor != 5 || fakes.journal.gotLimit != 2 {
					t.Fatalf("游标入参期望 5/2，实际 %d/%d", fakes.journal.gotCursor, fakes.journal.gotLimit)
				}
			},
		},
	}

	for _, c := range cases {
		t.Run(c.id, func(t *testing.T) {
			srv, fakes := newQueryTestServer(t)
			ts := httptest.NewServer(srv.buildHandler())
			defer ts.Close()
			callQueryOperation(t, ts.URL, c.id, c.body)
			c.verify(t, fakes)
		})
	}
}

// TestQueryOperationPageFormAndDefault 分页参数形态与缺省：分页对象形态的操作按入参原样透传
// （省略时以 nil 交给 handler，零值缺省由 handler 与库层取第 1 页、每页 10 行）；作品搜索的
// 分页为整数对、库层该路径按偏移直算，故缺省在操作入口补齐
func TestQueryOperationPageFormAndDefault(t *testing.T) {
	srv, fakes := newQueryTestServer(t)
	ts := httptest.NewServer(srv.buildHandler())
	defer ts.Close()

	callQueryOperation(t, ts.URL, opTaskPage, `{"page":{"pageNumber":3,"pageSize":25}}`)
	if fakes.task.gotPage == nil || fakes.task.gotPage.PageNumber != 3 || fakes.task.gotPage.PageSize != 25 {
		t.Fatalf("任务分页对象期望原样透传 3/25，实际 %+v", fakes.task.gotPage)
	}
	callQueryOperation(t, ts.URL, opTaskPage, `{}`)
	if fakes.task.gotPage != nil {
		t.Fatalf("任务分页对象省略时期望透传 nil（缺省由 handler 与库层兜底），实际 %+v", fakes.task.gotPage)
	}

	callQueryOperation(t, ts.URL, opPluginPage, `{"page":{"pageNumber":2,"pageSize":5}}`)
	if fakes.plugin.gotPage == nil || fakes.plugin.gotPage.PageNumber != 2 || fakes.plugin.gotPage.PageSize != 5 {
		t.Fatalf("插件分页对象期望原样透传 2/5，实际 %+v", fakes.plugin.gotPage)
	}
	callQueryOperation(t, ts.URL, opPluginPage, `{}`)
	if fakes.plugin.gotPage != nil {
		t.Fatalf("插件分页对象省略时期望透传 nil（缺省由 handler 与库层兜底），实际 %+v", fakes.plugin.gotPage)
	}

	callQueryOperation(t, ts.URL, opSearchWork, `{}`)
	if fakes.search.gotPageNumber != 1 || fakes.search.gotPageSize != 10 {
		t.Fatalf("作品搜索分页缺省期望 1/10，实际 %d/%d", fakes.search.gotPageNumber, fakes.search.gotPageSize)
	}
	callQueryOperation(t, ts.URL, opSearchWork, `{"pageNumber":-1,"pageSize":0}`)
	if fakes.search.gotPageNumber != 1 || fakes.search.gotPageSize != 10 {
		t.Fatalf("作品搜索分页非正数期望取缺省 1/10，实际 %d/%d", fakes.search.gotPageNumber, fakes.search.gotPageSize)
	}
}

// TestQueryOperationFailureIsPureFailure 信封语义：业务失败经外部面仍是纯失败——200 由信封
// 表达成败、success=false、msg 为用户可读原因、data 恒 null（不携带业务载荷）；参数解码失败
// 同属操作失败且不进入业务调用
func TestQueryOperationFailureIsPureFailure(t *testing.T) {
	srv, fakes := newQueryTestServer(t)
	fakes.work.failMsg = "作品不存在"
	ts := httptest.NewServer(srv.buildHandler())
	defer ts.Close()

	resp, view := postOperation(t, ts.URL, "Bearer "+testToken, opWorkDetail, `{"id":404}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("业务失败期望 200 由信封表达，实际 %d", resp.StatusCode)
	}
	if view.Success {
		t.Fatal("业务失败期望失败信封")
	}
	if view.Msg != "作品不存在" {
		t.Fatalf("失败原因期望原样透传「作品不存在」，实际 %q", view.Msg)
	}
	if string(view.Data) != "null" {
		t.Fatalf("纯失败信封 data 期望 null，实际 %s", view.Data)
	}

	srvNil, fakesNil := newQueryTestServer(t)
	fakesNil.work.nilResp = true
	tsNil := httptest.NewServer(srvNil.buildHandler())
	defer tsNil.Close()
	resp, view = postOperation(t, tsNil.URL, "Bearer "+testToken, opWorkDetail, `{"id":1}`)
	if resp.StatusCode != http.StatusOK || view.Success {
		t.Fatalf("业务未返回响应期望 200 + 失败信封，实际 %d/%v", resp.StatusCode, view.Success)
	}
	if view.Msg == "" {
		t.Fatal("失败信封 msg 不应为空")
	}

	srvDecode, fakesDecode := newQueryTestServer(t)
	tsDecode := httptest.NewServer(srvDecode.buildHandler())
	defer tsDecode.Close()
	resp, view = postOperation(t, tsDecode.URL, "Bearer "+testToken, opWorkDetail, `{"id":"abc"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("参数解码失败期望 200 由信封表达，实际 %d", resp.StatusCode)
	}
	if view.Success {
		t.Fatal("参数解码失败期望失败信封")
	}
	if !strings.Contains(view.Msg, "参数解码失败") {
		t.Fatalf("失败原因期望说明参数解码失败，实际 %q", view.Msg)
	}
	if fakesDecode.work.gotID != 0 {
		t.Fatalf("参数解码失败不应进入业务调用，实际收到作品 ID %d", fakesDecode.work.gotID)
	}
}

// TestManifestEndpoint 能力清单端点：免鉴权匿名请求 401；合法 token 返回同构信封且 data 为
// 按注册表生成的清单
func TestManifestEndpoint(t *testing.T) {
	srv, _ := newQueryTestServer(t)
	ts := httptest.NewServer(mountedQueryHandler(srv))
	defer ts.Close()

	resp, _ := getWithAuth(t, ts.URL+ManifestPath, "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("无 token 期望 401，实际 %d", resp.StatusCode)
	}

	resp, view := getWithAuth(t, ts.URL+ManifestPath, "Bearer "+testToken)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("合法 token 期望 200，实际 %d", resp.StatusCode)
	}
	if !view.Success {
		t.Fatalf("能力清单期望成功信封，实际失败: %s", view.Msg)
	}
	var manifest Manifest
	if err := json.Unmarshal(view.Data, &manifest); err != nil {
		t.Fatalf("清单载荷解码失败 %q: %v", string(view.Data), err)
	}
	if len(manifest.Operations) != len(srv.Registry().List()) {
		t.Fatalf("清单操作数期望 %d，实际 %d", len(srv.Registry().List()), len(manifest.Operations))
	}
}

// TestJournalEndpoint 记账增量查询端点：查询参数翻译为操作入参后按同一操作执行路径调用，
// 返回游标契约字段（Entries/NextCursor/HasMore/CursorExpired）
func TestJournalEndpoint(t *testing.T) {
	srv, fakes := newQueryTestServer(t)
	ts := httptest.NewServer(mountedQueryHandler(srv))
	defer ts.Close()

	resp, view := getWithAuth(t, ts.URL+JournalPath+"?cursor=5&limit=2", "Bearer "+testToken)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("合法 token 期望 200，实际 %d", resp.StatusCode)
	}
	if !view.Success {
		t.Fatalf("记账增量查询期望成功信封，实际失败: %s", view.Msg)
	}
	if fakes.journal.gotCursor != 5 || fakes.journal.gotLimit != 2 {
		t.Fatalf("查询参数期望翻译为 5/2，实际 %d/%d", fakes.journal.gotCursor, fakes.journal.gotLimit)
	}
	var result struct {
		Entries       []map[string]any `json:"entries"`
		NextCursor    int64            `json:"nextCursor"`
		HasMore       bool             `json:"hasMore"`
		CursorExpired bool             `json:"cursorExpired"`
	}
	if err := json.Unmarshal(view.Data, &result); err != nil {
		t.Fatalf("记账载荷解码失败 %q: %v", string(view.Data), err)
	}
	if len(result.Entries) != 1 || result.NextCursor != 7 || !result.HasMore || result.CursorExpired {
		t.Fatalf("游标契约字段期望 1 行/游标 7/可续读/未过期，实际 %+v", result)
	}

	resp, _ = getWithAuth(t, ts.URL+JournalPath, "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("无 token 期望 401，实际 %d", resp.StatusCode)
	}

	// 省略查询参数：零值入参（0 = 从头消费，行数上限取缺省）
	fakes.journal.gotCursor, fakes.journal.gotLimit = -1, -1
	if _, view = getWithAuth(t, ts.URL+JournalPath, "Bearer "+testToken); !view.Success {
		t.Fatalf("省略查询参数期望成功信封，实际失败: %s", view.Msg)
	}
	if fakes.journal.gotCursor != 0 || fakes.journal.gotLimit != 0 {
		t.Fatalf("省略查询参数期望零值入参，实际 %d/%d", fakes.journal.gotCursor, fakes.journal.gotLimit)
	}

	// 非整数查询参数属操作失败：200 + 纯失败信封，且不进入业务调用
	fakes.journal.gotCursor, fakes.journal.gotLimit = -1, -1
	resp, view = getWithAuth(t, ts.URL+JournalPath+"?cursor=abc", "Bearer "+testToken)
	if resp.StatusCode != http.StatusOK || view.Success {
		t.Fatalf("非法 cursor 期望 200 + 失败信封，实际 %d/%v", resp.StatusCode, view.Success)
	}
	if string(view.Data) != "null" {
		t.Fatalf("纯失败信封 data 期望 null，实际 %s", view.Data)
	}
	resp, view = getWithAuth(t, ts.URL+JournalPath+"?limit=abc", "Bearer "+testToken)
	if resp.StatusCode != http.StatusOK || view.Success {
		t.Fatalf("非法 limit 期望 200 + 失败信封，实际 %d/%v", resp.StatusCode, view.Success)
	}
	if fakes.journal.gotCursor != -1 || fakes.journal.gotLimit != -1 {
		t.Fatalf("非法查询参数不应进入业务调用，实际入参 %d/%d", fakes.journal.gotCursor, fakes.journal.gotLimit)
	}

	// 记账读取面读库失败：已调用的失败一律 200 + 纯失败信封
	fakes.journal.err = errors.New("读取记账账行失败")
	resp, view = getWithAuth(t, ts.URL+JournalPath, "Bearer "+testToken)
	if resp.StatusCode != http.StatusOK || view.Success {
		t.Fatalf("读库失败期望 200 + 失败信封，实际 %d/%v", resp.StatusCode, view.Success)
	}
	if view.Msg != "读取记账账行失败" {
		t.Fatalf("读库失败原因期望原样透传，实际 %q", view.Msg)
	}
	fakes.journal.err = nil
}

// TestJournalEndpointUnwired 记账读取面未接入（注册表无对应操作）时端点快速失败：不得以空
// 结果冒充成功
func TestJournalEndpointUnwired(t *testing.T) {
	srv, err := NewServer(NewRegistry(), testToken)
	if err != nil {
		t.Fatalf("构建服务失败: %v", err)
	}
	ts := httptest.NewServer(mountedQueryHandler(srv))
	defer ts.Close()

	resp, view := getWithAuth(t, ts.URL+JournalPath, "Bearer "+testToken)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("未接入期望 503，实际 %d", resp.StatusCode)
	}
	if view.Success || view.Msg == "" {
		t.Fatalf("未接入期望失败信封且带原因，实际 %+v", view)
	}
}
