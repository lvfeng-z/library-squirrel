package externalapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/library-squirrel/backend/base/model"
	dto "github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/task"
	"github.com/library-squirrel/backend/taskManager"
	"github.com/library-squirrel/backend/util"
)

// ========== 替身：建任务与任务树控制（含等待确认小状态机） ==========

// fakeTaskCreate 建任务替身：未携带显选键时回消歧冲突载荷（候选清单透传、不触任何后续
// 状态）；携带显选键时建成任务并经 control 替身登记等待确认态（模拟撞上覆盖确认）
type fakeTaskCreate struct {
	gotReqs []*task.CreateTaskByURLRequest
	// control 建成任务后登记等待确认态的联合替身（nil = 只回成功不入状态机）
	control *fakeTaskManagerControl
	failMsg string
}

func (f *fakeTaskCreate) CreateTaskByURL(ctx context.Context, req *task.CreateTaskByURLRequest) *model.ApiResponse[*task.CreateTaskByURLResponse] {
	f.gotReqs = append(f.gotReqs, req)
	if f.failMsg != "" {
		return model.Error[*task.CreateTaskByURLResponse](f.failMsg)
	}
	if req.ChosenPluginPublicId == "" && req.ChosenExtensionId == "" {
		return model.Success(&task.CreateTaskByURLResponse{
			Succeed:  false,
			Msg:      "该链接可由多个插件处理，请选择插件",
			Conflict: true,
			ConflictCandidates: []*dto.PluginCandidate{
				{PluginPublicId: "pub-a", PluginName: "插件甲", ExtensionId: "ext-a"},
				{PluginPublicId: "pub-b", PluginName: "插件乙", ExtensionId: "ext-b"},
			},
		})
	}
	if f.control != nil {
		f.control.waitingForInput[f.control.nextTaskID] = true
	}
	return model.Success(&task.CreateTaskByURLResponse{
		Succeed:       true,
		AddedQuantity: 1,
		Msg:           "已添加 1 个任务",
	})
}

// fakeTaskManagerControl 任务运行态查询 + 任务树控制联合替身：持有「等待确认表」小状态
// 机——建任务替身登记等待确认态，ConfirmReplace / ConfirmReplaceBatch 解除；各控制方法
// 记录入参即回成功（与真实状态机同为「持久状态 + 后续调用解除」形态）
type fakeTaskManagerControl struct {
	gotStartIDs        []int64
	gotPauseIDs        []int64
	gotResumeIDs       []int64
	gotStopIDs         []int64
	gotRetryIDs        []int64
	gotConfirmTaskID   int64
	gotConfirmAction   string
	gotConfirmBatchIDs []int64
	gotConfirmBatchAct string
	gotTreeStateTaskID int64
	waitingForInput    map[int64]bool
	nextTaskID         int64
	failMsg            string
}

func (f *fakeTaskManagerControl) GetTaskSnapshot() *model.ApiResponse[*taskManager.TaskSnapshotDTO] {
	if f.failMsg != "" {
		return model.Error[*taskManager.TaskSnapshotDTO](f.failMsg)
	}
	ids := make([]int64, 0, len(f.waitingForInput))
	for id := range f.waitingForInput {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	tasks := make([]*taskManager.TaskSnapshotItem, 0, len(ids))
	for _, id := range ids {
		tasks = append(tasks, &taskManager.TaskSnapshotItem{
			ID:       id,
			TaskName: "示例任务",
			Status:   int(taskManager.TaskStateWaitingForInput),
		})
	}
	return model.Success(&taskManager.TaskSnapshotDTO{Tasks: tasks})
}

func (f *fakeTaskManagerControl) GetTaskTreeState(taskId int64) *model.ApiResponse[int] {
	f.gotTreeStateTaskID = taskId
	if f.failMsg != "" {
		return model.Error[int](f.failMsg)
	}
	if f.waitingForInput[taskId] {
		return model.Success(int(taskManager.TaskStateWaitingForInput))
	}
	return model.Success(int(taskManager.TaskStateFinished))
}

func (f *fakeTaskManagerControl) IsIdle() *model.ApiResponse[bool] {
	return model.Success(len(f.waitingForInput) == 0)
}

func (f *fakeTaskManagerControl) StartTaskTrees(ctx context.Context, taskIds []int64) *model.ApiResponse[any] {
	f.gotStartIDs = taskIds
	return model.Success[any](nil)
}

func (f *fakeTaskManagerControl) PauseTaskTrees(ctx context.Context, taskIds []int64) *model.ApiResponse[any] {
	f.gotPauseIDs = taskIds
	return model.Success[any](nil)
}

func (f *fakeTaskManagerControl) ResumeTaskTrees(ctx context.Context, taskIds []int64) *model.ApiResponse[any] {
	f.gotResumeIDs = taskIds
	return model.Success[any](nil)
}

func (f *fakeTaskManagerControl) StopTaskTrees(ctx context.Context, taskIds []int64) *model.ApiResponse[any] {
	f.gotStopIDs = taskIds
	return model.Success[any](nil)
}

func (f *fakeTaskManagerControl) RetryTaskTrees(ctx context.Context, taskIds []int64) *model.ApiResponse[any] {
	f.gotRetryIDs = taskIds
	return model.Success[any](nil)
}

func (f *fakeTaskManagerControl) ConfirmReplace(ctx context.Context, taskId int64, action string) *model.ApiResponse[any] {
	f.gotConfirmTaskID, f.gotConfirmAction = taskId, action
	if f.failMsg != "" {
		return model.Error[any](f.failMsg)
	}
	delete(f.waitingForInput, taskId)
	return model.Success[any](nil)
}

func (f *fakeTaskManagerControl) ConfirmReplaceBatch(ctx context.Context, taskIds []int64, action string) *model.ApiResponse[any] {
	f.gotConfirmBatchIDs, f.gotConfirmBatchAct = taskIds, action
	if f.failMsg != "" {
		return model.Error[any](f.failMsg)
	}
	for _, id := range taskIds {
		delete(f.waitingForInput, id)
	}
	return model.Success[any](nil)
}

// ========== 测试基建 ==========

// newTaskOnlyTestServer 仅注册任务操作的服务（无数据库、无记录面）
func newTaskOnlyTestServer(t *testing.T, create *fakeTaskCreate, control *fakeTaskManagerControl) *Server {
	t.Helper()
	registry := NewRegistry()
	if err := RegisterTaskOperations(registry, TaskDeps{Task: create, TaskManager: control}); err != nil {
		t.Fatalf("注册任务操作失败: %v", err)
	}
	srv, err := NewServer(registry, testToken)
	if err != nil {
		t.Fatalf("构建服务失败: %v", err)
	}
	return srv
}

// newFullChainTestServer 注册只读 + 任务操作并接入真实记录账表（内存库完整迁移）的服务
func newFullChainTestServer(t *testing.T) (*Server, *fakeTaskCreate, *fakeTaskManagerControl) {
	t.Helper()
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Fatalf("打开测试库失败: %v", err)
	}
	control := &fakeTaskManagerControl{waitingForInput: map[int64]bool{}, nextTaskID: 42}
	create := &fakeTaskCreate{control: control}
	// 只读操作面同样接入（任务状态查询走它），任务运行态查询面换成联合替身以反映状态机
	deps := newQueryFakes().deps()
	deps.TaskManager = control
	registry := NewRegistry()
	if err := RegisterQueryOperations(registry, deps); err != nil {
		t.Fatalf("注册只读操作失败: %v", err)
	}
	if err := RegisterTaskOperations(registry, TaskDeps{Task: create, TaskManager: control}); err != nil {
		t.Fatalf("注册任务操作失败: %v", err)
	}
	srv, err := NewServer(registry, testToken)
	if err != nil {
		t.Fatalf("构建服务失败: %v", err)
	}
	srv.SetOpRecordStore(NewOpRecordService(NewOpRecordRepository(db)))
	return srv, create, control
}

// postOperationWithLabel 以合法 token 与自报客户端标签头 POST 调用统一操作端点
func postOperationWithLabel(t *testing.T, base, operationID, body, clientLabel string) (*http.Response, envelopeView) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, base+"/api/op/"+operationID, strings.NewReader(body))
	if err != nil {
		t.Fatalf("构造请求失败: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+testToken)
	if clientLabel != "" {
		req.Header.Set(ClientLabelHeader, clientLabel)
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

// callVoidOperation 以合法 token 调用统一操作端点并断言成功信封（无返回载荷的操作 data
// 允许为 null），返回信封视图
func callVoidOperation(t *testing.T, base, operationID, body string) envelopeView {
	t.Helper()
	resp, view := postOperation(t, base, "Bearer "+testToken, operationID, body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("操作 %s 期望 200，实际 %d", operationID, resp.StatusCode)
	}
	if !view.Success {
		t.Fatalf("操作 %s 期望成功信封，实际失败: %s", operationID, view.Msg)
	}
	return view
}

// createResultView 建任务响应的断言视图（业务结果对象自带成败字段）
type createResultView struct {
	Succeed            bool   `json:"succeed"`
	AddedQuantity      int    `json:"addedQuantity"`
	Msg                string `json:"msg"`
	Conflict           bool   `json:"conflict"`
	ConflictCandidates []struct {
		PluginPublicId string `json:"pluginPublicId"`
		ExtensionId    string `json:"extensionId"`
	} `json:"conflictCandidates"`
}

// oplogRowView 记录行的断言视图
type oplogRowView struct {
	ID          int64  `json:"id"`
	CreateTime  int64  `json:"createTime"`
	Operation   string `json:"operation"`
	Params      string `json:"params"`
	Success     bool   `json:"success"`
	ClientLabel string `json:"clientLabel"`
	DurationMs  int64  `json:"durationMs"`
}

// oplogResultView 记录游标契约的断言视图
type oplogResultView struct {
	Entries       []oplogRowView `json:"entries"`
	NextCursor    int64          `json:"nextCursor"`
	HasMore       bool           `json:"hasMore"`
	CursorExpired bool           `json:"cursorExpired"`
}

// fetchOplog 以合法 token 调记录查询端点并断言成功，返回游标契约视图
func fetchOplog(t *testing.T, base, query string) oplogResultView {
	t.Helper()
	resp, view := getWithAuth(t, base+OplogPath+"?"+query, "Bearer "+testToken)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("记录查询期望 200，实际 %d", resp.StatusCode)
	}
	if !view.Success {
		t.Fatalf("记录查询期望成功信封，实际失败: %s", view.Msg)
	}
	var result oplogResultView
	if err := json.Unmarshal(view.Data, &result); err != nil {
		t.Fatalf("记录载荷解码失败 %q: %v", string(view.Data), err)
	}
	return result
}

// ========== 注册面 ==========

// TestRegisterTaskOperationsRejectsMissingDeps 依赖面缺项属装配期错误：报错且不装入任何操作
func TestRegisterTaskOperationsRejectsMissingDeps(t *testing.T) {
	if err := RegisterTaskOperations(nil, TaskDeps{}); err == nil {
		t.Fatal("注册表为空期望报错")
	}
	cases := []struct {
		name  string
		clear func(deps *TaskDeps)
	}{
		{"建任务", func(deps *TaskDeps) { deps.Task = nil }},
		{"任务树控制", func(deps *TaskDeps) { deps.TaskManager = nil }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			deps := TaskDeps{Task: &fakeTaskCreate{}, TaskManager: &fakeTaskManagerControl{}}
			c.clear(&deps)
			registry := NewRegistry()
			if err := RegisterTaskOperations(registry, deps); err == nil {
				t.Fatalf("缺少%s依赖期望注册报错", c.name)
			}
			if registered := registry.List(); len(registered) != 0 {
				t.Fatalf("注册报错时不应装入操作，实际 %d 条", len(registered))
			}
		})
	}
}

// ========== 任务操作经统一操作端点走通 ==========

// TestTaskControlOperationsOverHTTP 各控制操作经统一操作端点走通：入参按操作声明送达目标
// handler 方法，返回同构成功信封
func TestTaskControlOperationsOverHTTP(t *testing.T) {
	create := &fakeTaskCreate{}
	control := &fakeTaskManagerControl{waitingForInput: map[int64]bool{}}
	srv := newTaskOnlyTestServer(t, create, control)
	ts := httptest.NewServer(srv.buildHandler())
	defer ts.Close()

	cases := []struct {
		id     string
		body   string
		verify func(t *testing.T)
	}{
		{opTaskManagerStart, `{"taskIds":[5,6]}`, func(t *testing.T) {
			if len(control.gotStartIDs) != 2 || control.gotStartIDs[0] != 5 || control.gotStartIDs[1] != 6 {
				t.Fatalf("启动入参期望 [5 6]，实际 %v", control.gotStartIDs)
			}
		}},
		{opTaskManagerPause, `{"taskIds":[7]}`, func(t *testing.T) {
			if len(control.gotPauseIDs) != 1 || control.gotPauseIDs[0] != 7 {
				t.Fatalf("暂停入参期望 [7]，实际 %v", control.gotPauseIDs)
			}
		}},
		{opTaskManagerResume, `{"taskIds":[8]}`, func(t *testing.T) {
			if len(control.gotResumeIDs) != 1 || control.gotResumeIDs[0] != 8 {
				t.Fatalf("恢复入参期望 [8]，实际 %v", control.gotResumeIDs)
			}
		}},
		{opTaskManagerStop, `{"taskIds":[9,10]}`, func(t *testing.T) {
			if len(control.gotStopIDs) != 2 || control.gotStopIDs[0] != 9 || control.gotStopIDs[1] != 10 {
				t.Fatalf("强停入参期望 [9 10]，实际 %v", control.gotStopIDs)
			}
		}},
		{opTaskManagerRetry, `{"taskIds":[11]}`, func(t *testing.T) {
			if len(control.gotRetryIDs) != 1 || control.gotRetryIDs[0] != 11 {
				t.Fatalf("重试入参期望 [11]，实际 %v", control.gotRetryIDs)
			}
		}},
		{opTaskManagerConfirmReplace, `{"taskId":12,"action":"skip"}`, func(t *testing.T) {
			if control.gotConfirmTaskID != 12 || control.gotConfirmAction != "skip" {
				t.Fatalf("单个确认入参期望 12/skip，实际 %d/%s", control.gotConfirmTaskID, control.gotConfirmAction)
			}
		}},
		{opTaskManagerConfirmReplaceBatch, `{"taskIds":[13,14],"action":"replace"}`, func(t *testing.T) {
			if len(control.gotConfirmBatchIDs) != 2 || control.gotConfirmBatchAct != "replace" {
				t.Fatalf("批量确认入参期望 [13 14]/replace，实际 %v/%s", control.gotConfirmBatchIDs, control.gotConfirmBatchAct)
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.id, func(t *testing.T) {
			callVoidOperation(t, ts.URL, c.id, c.body)
			c.verify(t)
		})
	}
}

// TestTaskOperationFailureIsPureFailure 业务失败经外部面仍是纯失败：200 由信封表达成败、
// data 恒 null；建任务面带冲突载荷的响应不属失败（业务分支态住 data 载荷，见全链测试）
func TestTaskOperationFailureIsPureFailure(t *testing.T) {
	create := &fakeTaskCreate{failMsg: "url不受支持"}
	srv := newTaskOnlyTestServer(t, create, &fakeTaskManagerControl{waitingForInput: map[int64]bool{}})
	ts := httptest.NewServer(srv.buildHandler())
	defer ts.Close()

	resp, view := postOperation(t, ts.URL, "Bearer "+testToken, opTaskCreateByURL, `{"url":"https://h.ex/x"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("业务失败期望 200 由信封表达，实际 %d", resp.StatusCode)
	}
	if view.Success || view.Msg != "url不受支持" {
		t.Fatalf("业务失败期望纯失败信封且原因透传，实际 %v/%q", view.Success, view.Msg)
	}
	if string(view.Data) != "null" {
		t.Fatalf("纯失败信封 data 期望 null，实际 %s", view.Data)
	}
}

// ========== 全链：建任务 → 冲突 → 显选重发 → 待确认 → 确认解除 + 记录账 ==========

// TestTaskCreateConfirmFullChainWithRecords 退出标准全链断言：建任务未显选回冲突载荷 →
// 带显选键重发建成 → 任务落等待确认态（快照与树状态可见）→ 覆盖确认解除；每次调用产生
// 记录行且游标分页正确，URL 查询串不落记录、客户端标签如实入账
func TestTaskCreateConfirmFullChainWithRecords(t *testing.T) {
	srv, create, control := newFullChainTestServer(t)
	ts := httptest.NewServer(srv.buildHandler())
	defer ts.Close()
	const sampleURL = "https://www.pixiv.net/artworks/123456?php=SECRETPHP&sk=tok123"

	// 1. 建任务：未显选 → 冲突载荷（success=true 的 data 载荷带候选清单，首位即默认选中）
	resp, view := postOperation(t, ts.URL, "Bearer "+testToken, opTaskCreateByURL, fmt.Sprintf(`{"url":%q}`, sampleURL))
	if resp.StatusCode != http.StatusOK || !view.Success {
		t.Fatalf("建任务期望成功信封，实际 %d/%v（%s）", resp.StatusCode, view.Success, view.Msg)
	}
	var conflict createResultView
	if err := json.Unmarshal(view.Data, &conflict); err != nil {
		t.Fatalf("冲突载荷解码失败 %q: %v", string(view.Data), err)
	}
	if !conflict.Conflict || len(conflict.ConflictCandidates) != 2 {
		t.Fatalf("期望冲突载荷与 2 个候选，实际 %+v", conflict)
	}
	if conflict.ConflictCandidates[0].PluginPublicId != "pub-a" {
		t.Fatalf("候选首位期望默认选中项 pub-a，实际 %s", conflict.ConflictCandidates[0].PluginPublicId)
	}
	if len(create.gotReqs) != 1 || create.gotReqs[0].URL != sampleURL {
		t.Fatalf("建任务入参期望原样透传，实际 %+v", create.gotReqs)
	}

	// 2. 带显选键重发（自报客户端标签）→ 建成
	resp, view = postOperationWithLabel(t, ts.URL, opTaskCreateByURL,
		fmt.Sprintf(`{"url":%q,"chosenPluginPublicId":"pub-b","chosenExtensionId":"ext-b"}`, sampleURL), "dsh-agent")
	if resp.StatusCode != http.StatusOK || !view.Success {
		t.Fatalf("显选重发期望成功信封，实际 %d/%v（%s）", resp.StatusCode, view.Success, view.Msg)
	}
	var created createResultView
	if err := json.Unmarshal(view.Data, &created); err != nil {
		t.Fatalf("建任务载荷解码失败 %q: %v", string(view.Data), err)
	}
	if !created.Succeed || created.AddedQuantity != 1 {
		t.Fatalf("显选重发期望建成 1 个任务，实际 %+v", created)
	}
	if len(create.gotReqs) != 2 || create.gotReqs[1].ChosenPluginPublicId != "pub-b" || create.gotReqs[1].ChosenExtensionId != "ext-b" {
		t.Fatalf("显选键期望送达建任务面，实际 %+v", create.gotReqs[1])
	}

	// 3. 任务落等待确认态：快照可见等待确认任务，取其 ID
	snapshotView := callQueryOperation(t, ts.URL, opTaskManagerSnapshot, `{}`)
	var snapshot struct {
		Tasks []struct {
			ID     int64 `json:"id"`
			Status int   `json:"status"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(snapshotView.Data, &snapshot); err != nil {
		t.Fatalf("快照载荷解码失败 %q: %v", string(snapshotView.Data), err)
	}
	if len(snapshot.Tasks) != 1 || snapshot.Tasks[0].ID != 42 {
		t.Fatalf("快照期望 1 个等待确认任务（ID 42），实际 %+v", snapshot.Tasks)
	}
	if snapshot.Tasks[0].Status != int(taskManager.TaskStateWaitingForInput) {
		t.Fatalf("快照任务状态期望等待用户确认，实际 %d", snapshot.Tasks[0].Status)
	}

	// 4. 树状态查询 → 等待确认
	stateView := callQueryOperation(t, ts.URL, opTaskManagerTreeState, `{"taskId":42}`)
	var state int
	if err := json.Unmarshal(stateView.Data, &state); err != nil {
		t.Fatalf("树状态载荷解码失败 %q: %v", string(stateView.Data), err)
	}
	if state != int(taskManager.TaskStateWaitingForInput) {
		t.Fatalf("树状态期望等待用户确认，实际 %d", state)
	}
	if control.gotTreeStateTaskID != 42 {
		t.Fatalf("树状态入参期望 42，实际 %d", control.gotTreeStateTaskID)
	}

	// 5. 覆盖确认解除（与 GUI 确认弹窗等效的第二入口；无返回载荷的操作 data 合法为 null）
	callVoidOperation(t, ts.URL, opTaskManagerConfirmReplace, `{"taskId":42,"action":"replace"}`)
	if control.gotConfirmTaskID != 42 || control.gotConfirmAction != "replace" {
		t.Fatalf("确认入参期望 42/replace，实际 %d/%s", control.gotConfirmTaskID, control.gotConfirmAction)
	}

	// 6. 再查状态 → 确认已解除（不再等待确认）
	stateView = callQueryOperation(t, ts.URL, opTaskManagerTreeState, `{"taskId":42}`)
	if err := json.Unmarshal(stateView.Data, &state); err != nil {
		t.Fatalf("树状态载荷解码失败 %q: %v", string(stateView.Data), err)
	}
	if state == int(taskManager.TaskStateWaitingForInput) {
		t.Fatal("确认后期望脱离等待确认态")
	}
	if !control.IsIdle().Data {
		t.Fatal("确认后期望无等待确认任务")
	}

	// 7. 记录账：调用序列 = 建任务、建任务（带标签）、快照、树状态、确认、树状态，共 6 行；
	//    游标分页：limit=4 首批 + cursor 续读到尾
	first := fetchOplog(t, ts.URL, "cursor=0&limit=4")
	if len(first.Entries) != 4 || first.NextCursor != 4 || !first.HasMore || first.CursorExpired {
		t.Fatalf("首批期望 4 行/游标 4/可续读/未过期，实际 %+v", first)
	}
	second := fetchOplog(t, ts.URL, fmt.Sprintf("cursor=%d&limit=10", first.NextCursor))
	if len(second.Entries) != 2 || second.HasMore {
		t.Fatalf("续读期望 2 行且读尽，实际 %d 行/hasMore=%v", len(second.Entries), second.HasMore)
	}
	all := append(first.Entries, second.Entries...)
	wantOps := []string{opTaskCreateByURL, opTaskCreateByURL, opTaskManagerSnapshot, opTaskManagerTreeState, opTaskManagerConfirmReplace, opTaskManagerTreeState}
	if len(all) != len(wantOps) {
		t.Fatalf("记录行期望 %d 行，实际 %d 行", len(wantOps), len(all))
	}
	for i, want := range wantOps {
		if all[i].Operation != want {
			t.Fatalf("第 %d 行操作期望 %s，实际 %s", i+1, want, all[i].Operation)
		}
		if !all[i].Success {
			t.Fatalf("第 %d 行期望成功行（全链均为成功信封）", i+1)
		}
		if all[i].ID != int64(i+1) || all[i].CreateTime <= 0 || all[i].DurationMs < 0 {
			t.Fatalf("第 %d 行游标/时间/耗时不合法: %+v", i+1, all[i])
		}
	}
	// URL 脱敏：查询串不落记录（防凭据类参数入账），主机与路径保留
	if !strings.Contains(all[0].Params, "https://www.pixiv.net/artworks/123456") {
		t.Fatalf("参数摘要期望保留协议+主机+路径，实际 %q", all[0].Params)
	}
	for _, secret := range []string{"SECRETPHP", "tok123"} {
		if strings.Contains(all[0].Params, secret) {
			t.Fatalf("参数摘要不应含查询串内容 %s，实际 %q", secret, all[0].Params)
		}
	}
	if !strings.Contains(all[0].Params, "查询串") {
		t.Fatalf("参数摘要期望记查询串存在标记，实际 %q", all[0].Params)
	}
	// 客户端标签：自报行如实入账，未自报行为空
	if all[1].ClientLabel != "dsh-agent" {
		t.Fatalf("自报标签期望入账，实际 %q", all[1].ClientLabel)
	}
	if all[0].ClientLabel != "" {
		t.Fatalf("未自报行标签期望为空，实际 %q", all[0].ClientLabel)
	}
}

// ========== 记录查询端点 ==========

// TestOplogEndpointAuthAndParams 记录查询端点：无 token 401；非法查询参数属查询失败
// （200 + 纯失败信封）；未装配记录面时 503 快速失败
func TestOplogEndpointAuthAndParams(t *testing.T) {
	// 未装配记录面：不以空结果冒充成功
	unwired, err := NewServer(NewRegistry(), testToken)
	if err != nil {
		t.Fatalf("构建服务失败: %v", err)
	}
	unwiredTS := httptest.NewServer(mountedQueryHandler(unwired))
	defer unwiredTS.Close()
	resp, _ := getWithAuth(t, unwiredTS.URL+OplogPath, "Bearer "+testToken)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("未接入期望 503，实际 %d", resp.StatusCode)
	}

	srv, _, _ := newFullChainTestServer(t)
	ts := httptest.NewServer(mountedQueryHandler(srv))
	defer ts.Close()

	resp, _ = getWithAuth(t, ts.URL+OplogPath, "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("无 token 期望 401，实际 %d", resp.StatusCode)
	}

	resp, view := getWithAuth(t, ts.URL+OplogPath+"?cursor=abc", "Bearer "+testToken)
	if resp.StatusCode != http.StatusOK || view.Success {
		t.Fatalf("非法 cursor 期望 200 + 失败信封，实际 %d/%v", resp.StatusCode, view.Success)
	}
	if string(view.Data) != "null" {
		t.Fatalf("纯失败信封 data 期望 null，实际 %s", view.Data)
	}
	resp, view = getWithAuth(t, ts.URL+OplogPath+"?limit=abc", "Bearer "+testToken)
	if resp.StatusCode != http.StatusOK || view.Success {
		t.Fatalf("非法 limit 期望 200 + 失败信封，实际 %d/%v", resp.StatusCode, view.Success)
	}

	// 空账表：cursor=0 从头消费，空行集 + 游标不动 + 未过期
	empty := fetchOplog(t, ts.URL, "cursor=0&limit=5")
	if len(empty.Entries) != 0 || empty.NextCursor != 0 || empty.HasMore || empty.CursorExpired {
		t.Fatalf("空账表期望空行集/游标不动/未过期，实际 %+v", empty)
	}
}

// ========== 记录服务：轮转 / 游标 / 分页 ==========

// TestOpRecordRotationAndCursor 轮转两轨（容量裁最旧、TTL 裁超龄）与游标过期标记、
// 设置页分页读取（最新在前）
func TestOpRecordRotationAndCursor(t *testing.T) {
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Fatalf("打开测试库失败: %v", err)
	}
	ctx := context.Background()
	repo := NewOpRecordRepository(db)
	svc := NewOpRecordService(repo)

	// 容量轨：3 行裁到最新 1 行（裁最旧 2 行）
	for i := range 3 {
		if err := svc.Record(ctx, fmt.Sprintf("op.%d", i), "{}", true, "", 1); err != nil {
			t.Fatalf("写入记录失败: %v", err)
		}
	}
	deleted, err := repo.RotateOldest(ctx, 0, 1)
	if err != nil {
		t.Fatalf("容量轮转失败: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("容量轨期望裁 2 行，实际 %d", deleted)
	}

	// TTL 轨：把存活行拨回超龄，Record 顺带轮转裁掉旧行、新行留存
	aged := util.GetCurrentTimestamp() - int64((opRecordRotationTTL + time.Hour).Milliseconds())
	if err := db.Exec("UPDATE external_api_op_record SET create_time = ?", aged).Error; err != nil {
		t.Fatalf("拨回行龄失败: %v", err)
	}
	if err := svc.Record(ctx, "op.new", "{}", false, "aged-trim", 2); err != nil {
		t.Fatalf("写入记录失败: %v", err)
	}
	agedCount := int64(0)
	if err := db.Raw("SELECT COUNT(*) FROM external_api_op_record WHERE create_time < ?", util.GetCurrentTimestamp()-opRecordRotationTTL.Milliseconds()).Scan(&agedCount).Error; err != nil {
		t.Fatalf("统计超龄行失败: %v", err)
	}
	if agedCount != 0 {
		t.Fatalf("TTL 轨期望超龄行全裁，实际剩 %d 行", agedCount)
	}

	// 游标过期：被裁游标早于最旧存活行 → 过期标记；cursor=0 不判过期
	result, err := svc.ListSince(ctx, 2, 0)
	if err != nil {
		t.Fatalf("游标读取失败: %v", err)
	}
	if !result.CursorExpired {
		t.Fatal("指向已裁行的游标期望过期标记")
	}
	result, err = svc.ListSince(ctx, 0, 0)
	if err != nil {
		t.Fatalf("游标读取失败: %v", err)
	}
	if result.CursorExpired {
		t.Fatal("从头消费不判过期")
	}

	// 设置页分页：最新在前
	for i := range 2 {
		if err := svc.Record(ctx, fmt.Sprintf("op.tail.%d", i), "{}", true, "", 1); err != nil {
			t.Fatalf("写入记录失败: %v", err)
		}
	}
	page, err := svc.PageRecent(ctx, 1, 2)
	if err != nil {
		t.Fatalf("分页读取失败: %v", err)
	}
	if len(page.Data) != 2 || page.DataCount != 3 || page.PageCount != 2 {
		t.Fatalf("分页期望首页 2 行/共 3 行/2 页，实际行 %d/总 %d/页 %d", len(page.Data), page.DataCount, page.PageCount)
	}
	if page.Data[0].Operation != "op.tail.1" || page.Data[1].Operation != "op.tail.0" {
		t.Fatalf("分页期望最新在前，实际 %s、%s", page.Data[0].Operation, page.Data[1].Operation)
	}
}

// ========== 参数摘要脱敏 ==========

// TestSummarizeOperationParams 参数摘要：URL 值仅落协议+主机+路径（查询串只记存在与字符
// 数）、嵌套结构全走脱敏、非 JSON 请求体只记字节数、超长截断
func TestSummarizeOperationParams(t *testing.T) {
	if got := summarizeOperationParams(nil); got != "" {
		t.Fatalf("空请求体摘要期望空串，实际 %q", got)
	}

	got := summarizeOperationParams([]byte(`{"url":"https://h.example.net/a/b?x=1&y=2","name":"猫"}`))
	if !strings.Contains(got, `"https://h.example.net/a/b`) {
		t.Fatalf("摘要期望保留协议+主机+路径，实际 %q", got)
	}
	if strings.Contains(got, "x=1") || strings.Contains(got, "y=2") {
		t.Fatalf("摘要不应含查询串内容，实际 %q", got)
	}
	if !strings.Contains(got, "查询串 7 字符") {
		t.Fatalf("摘要期望记查询串存在与字符数，实际 %q", got)
	}
	if !strings.Contains(got, "猫") {
		t.Fatalf("非 URL 字符串值期望原样保留，实际 %q", got)
	}

	got = summarizeOperationParams([]byte(`{"urls":["http://h2.example.org/p?q=secret"],"ids":[1,2]}`))
	if strings.Contains(got, "secret") {
		t.Fatalf("嵌套数组内的 URL 查询串不应入摘要，实际 %q", got)
	}
	if !strings.Contains(got, "http://h2.example.org/p") {
		t.Fatalf("嵌套数组内的 URL 期望保留主机与路径，实际 %q", got)
	}

	got = summarizeOperationParams([]byte(`{"path":"C:\\dir\\file.txt","label":"普通标识"}`))
	if !strings.Contains(got, `C:\\dir\\file.txt`) || !strings.Contains(got, "普通标识") {
		t.Fatalf("非 URL 形字符串期望原样保留，实际 %q", got)
	}

	got = summarizeOperationParams([]byte(`not json at all`))
	if !strings.Contains(got, "非 JSON 请求体") || !strings.Contains(got, "15 字节") {
		t.Fatalf("非 JSON 请求体期望只记字节数，实际 %q", got)
	}

	long := strings.Repeat("a", opRecordMaxParamsLength+100)
	got = summarizeOperationParams([]byte(`{"blob":"` + long + `"}`))
	if !strings.HasSuffix(got, "…（截断）") {
		t.Fatalf("超长摘要期望截断标记结尾，实际 %q…", got[:50])
	}
	if got := summarizeOperationParams([]byte(`{"blob":"` + long + `"}`)); len([]rune(got)) > opRecordMaxParamsLength+len("…（截断）") {
		t.Fatalf("截断后摘要不应超出上限 + 截断标记，实际 %d 字符", len([]rune(got)))
	}
}

// ========== 落账尽力而为 ==========

// failingOpRecordStore 落账恒失败的记录面替身
type failingOpRecordStore struct{}

func (failingOpRecordStore) Record(ctx context.Context, operation, paramsSummary string, success bool, clientLabel string, durationMs int64) error {
	return errors.New("落账失败")
}

func (failingOpRecordStore) ListSince(ctx context.Context, cursor int64, limit int) (*OpRecordListResult, error) {
	return nil, errors.New("记录不可读")
}

func (failingOpRecordStore) PageRecent(ctx context.Context, pageNumber, pageSize int) (*model.Page[entity.ExternalApiOpRecord], error) {
	return nil, errors.New("记录不可读")
}

// TestRecordWriteFailureKeepsEnvelope 落账失败不影响操作响应语义：操作仍按自身成败回信封
func TestRecordWriteFailureKeepsEnvelope(t *testing.T) {
	registry := NewRegistry()
	if err := RegisterSampleOperation(registry); err != nil {
		t.Fatalf("注册样例操作失败: %v", err)
	}
	srv, err := NewServer(registry, testToken)
	if err != nil {
		t.Fatalf("构建服务失败: %v", err)
	}
	srv.SetOpRecordStore(failingOpRecordStore{})
	ts := httptest.NewServer(srv.buildHandler())
	defer ts.Close()

	resp, view := postOperation(t, ts.URL, "Bearer "+testToken, "sample.echo", `{"message":"hi"}`)
	if resp.StatusCode != http.StatusOK || !view.Success {
		t.Fatalf("落账失败不影响操作响应：期望 200 + 成功信封，实际 %d/%v（%s）", resp.StatusCode, view.Success, view.Msg)
	}
}
