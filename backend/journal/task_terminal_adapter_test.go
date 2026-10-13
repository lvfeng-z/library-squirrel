package journal

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/taskManager"
)

// terminalEmitRecord 一次 Wails 发射的留痕（字段导出以便整体 JSON 化比对发射签名）
type terminalEmitRecord struct {
	Name string `json:"Name"`
	Data []any  `json:"Data"`
}

// terminalRecordingEmitter 记录型 Wails 事件发射器（taskManager.WailsEventEmitter 的测试替身；
// 发射器接口全为导出类型，包外可实现——被推送器接口里未导出的 taskScheduleDTO 形参不受影响）
type terminalRecordingEmitter struct {
	mu     sync.Mutex
	events []terminalEmitRecord
}

func (e *terminalRecordingEmitter) Emit(eventName string, data ...any) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, terminalEmitRecord{Name: eventName, Data: data})
	return true
}

func (e *terminalRecordingEmitter) reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = nil
}

func (e *terminalRecordingEmitter) snapshot() []terminalEmitRecord {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]terminalEmitRecord, len(e.events))
	copy(out, e.events)
	return out
}

// terminalEnvelope 统一 topic 信封（taskManager.ipcEvent）的 JSON 形态：未导出类型不能类型断言，
// 经 JSON 比对/解析核验「主题 + 事件类型」两者语义
type terminalEnvelope struct {
	Name string `json:"Name"`
	Data []struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	} `json:"Data"`
}

func (r terminalEmitRecord) signature(t *testing.T) string {
	t.Helper()
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("发射记录 JSON 化失败: %v", err)
	}
	return string(b)
}

func (r terminalEmitRecord) envelope(t *testing.T) terminalEnvelope {
	t.Helper()
	var env terminalEnvelope
	if err := json.Unmarshal([]byte(r.signature(t)), &env); err != nil {
		t.Fatalf("信封解析失败: %v", err)
	}
	return env
}

// listTerminalEntries 按 id 升序返回全部账行
func listTerminalEntries(t *testing.T, repo *JournalRepository) []*entity.JournalEntry {
	t.Helper()
	var rows []*entity.JournalEntry
	if err := repo.GORM().Order("id ASC").Find(&rows).Error; err != nil {
		t.Fatalf("回查账行失败: %v", err)
	}
	return rows
}

func boolToInt64(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// TestTaskTerminalPusherFanOutPassthroughAndTerminalOnly 组合推送器两路 fan-out：
// Wails 一路纯透传（发射签名与原推送器逐字节相同、无增发事件），记账一路仅终态落账——
// 非终态状态迁移（含稳定态 Paused）、进度类、移除类、查重类一律零落账（方案风险3）
func TestTaskTerminalPusherFanOutPassthroughAndTerminalOnly(t *testing.T) {
	svc, repo := newTestService(t)

	emitter := &terminalRecordingEmitter{}
	rawPusher := taskManager.NewWailsTaskEventPusher(emitter)
	composite := NewTaskTerminalPusher(rawPusher, NewTaskTerminalRecorder(svc))

	// 基线：原推送器的发射签名
	emitter.reset()
	rawPusher.PushStateChange(11, "task-11", taskManager.TaskStateFinished)
	rawEvents := emitter.snapshot()
	if len(rawEvents) != 1 {
		t.Fatalf("原推送器应发射 1 条事件，实际 %d", len(rawEvents))
	}

	// 子任务终态：Wails 一路收到且仅一条、签名逐字节相同；记账一路落一行终态账
	emitter.reset()
	composite.PushStateChange(11, "task-11", taskManager.TaskStateFinished)
	compositeEvents := emitter.snapshot()
	if len(compositeEvents) != 1 {
		t.Fatalf("组合推送器应仅透传一条 Wails 事件，实际 %d", len(compositeEvents))
	}
	if got, want := compositeEvents[0].signature(t), rawEvents[0].signature(t); got != want {
		t.Fatalf("Wails 路语义应零变化：\n组合=%s\n原生=%s", got, want)
	}
	if env := compositeEvents[0].envelope(t); env.Name != "task-events" || env.Data[0].Type != "updateTask" {
		t.Fatalf("Wails 路主题/事件类型不符: %+v", env)
	}

	entries := listTerminalEntries(t, repo)
	if len(entries) != 1 {
		t.Fatalf("终态应落 1 行账，实际 %d 行", len(entries))
	}
	terminal := entries[0]
	if terminal.Domain != entity.JournalDomainTask || terminal.EventType != EventTypeTaskTerminal {
		t.Fatalf("账行域/事件类型不符: domain=%s eventType=%s", terminal.Domain, terminal.EventType)
	}
	if terminal.SubjectID != 11 {
		t.Fatalf("账行主体引用应为任务 ID 11，实际 %d", terminal.SubjectID)
	}
	var payload taskTerminalPayload
	if err := json.Unmarshal([]byte(terminal.Payload), &payload); err != nil {
		t.Fatalf("账行载荷非 JSON 摘要: %v", err)
	}
	if payload.TaskID != 11 || payload.State != int(taskManager.TaskStateFinished) ||
		payload.StateName != "Finished" || payload.TaskName != "task-11" || payload.Parent {
		t.Fatalf("终态载荷不符: %+v", payload)
	}

	// 非终态状态迁移（处理中/暂停/等待输入）：Wails 一路照常透传，记账零落账
	for _, state := range []taskManager.TaskState{
		taskManager.TaskStateProcessing, taskManager.TaskStatePaused, taskManager.TaskStateWaitingForInput,
	} {
		emitter.reset()
		composite.PushStateChange(12, "task-12", state)
		if len(emitter.snapshot()) != 1 {
			t.Fatalf("非终态 %d 推送仍须透传 Wails 一路", int(state))
		}
		if got := countRows(t, repo); got != 1 {
			t.Fatalf("非终态 %d 状态迁移应零落账，实际账行 %d", int(state), got)
		}
	}

	// 进度类（高风险写账风暴面，风险3）：Wails 一路透传到原主题，记账零落账；
	// PushProgressBatch 形参为 taskManager 未导出类型，包外无法调用——其经内嵌接口直接转发原推送器、
	// 不经记账适配器，无落账路径（同族进度方法已在此覆盖）
	emitter.reset()
	composite.PushProgress(11, 100, 40)
	composite.PushParentProgress(11, 5, 2)
	progressEvents := emitter.snapshot()
	if len(progressEvents) != 2 {
		t.Fatalf("进度推送应透传 2 条 Wails 事件，实际 %d", len(progressEvents))
	}
	if env := progressEvents[0].envelope(t); env.Name != "task-events" || env.Data[0].Type != "updateSchedule" {
		t.Fatalf("子任务进度主题/事件类型不符: %+v", env)
	}
	if env := progressEvents[1].envelope(t); env.Name != "parent-events" || env.Data[0].Type != "updateParentSchedule" {
		t.Fatalf("父任务进度主题/事件类型不符: %+v", env)
	}
	if got := countRows(t, repo); got != 1 {
		t.Fatalf("进度类推送应零落账，实际账行 %d", got)
	}

	// 父任务终态：两路同款（父任务同为任务行，subject = 父任务 ID，载荷 parent=true）
	emitter.reset()
	composite.PushParentStateChange(21, "parent-21", taskManager.TaskStatePartlyFinished)
	parentEvents := emitter.snapshot()
	if len(parentEvents) != 1 {
		t.Fatalf("父任务终态应仅透传一条 Wails 事件，实际 %d", len(parentEvents))
	}
	if env := parentEvents[0].envelope(t); env.Name != "parent-events" || env.Data[0].Type != "updateParentTask" {
		t.Fatalf("父任务终态主题/事件类型不符: %+v", env)
	}
	entries = listTerminalEntries(t, repo)
	if len(entries) != 2 {
		t.Fatalf("父任务终态应再落 1 行账，实际 %d 行", len(entries))
	}
	parentEntry := entries[1]
	if parentEntry.SubjectID != 21 {
		t.Fatalf("父任务账行主体引用应为 21，实际 %d", parentEntry.SubjectID)
	}
	payload = taskTerminalPayload{}
	if err := json.Unmarshal([]byte(parentEntry.Payload), &payload); err != nil {
		t.Fatalf("父任务账行载荷非 JSON 摘要: %v", err)
	}
	if !payload.Parent || payload.State != int(taskManager.TaskStatePartlyFinished) || payload.StateName != "PartlyFinished" {
		t.Fatalf("父任务终态载荷不符: %+v", payload)
	}

	// 父任务非终态：透传照旧、零落账
	emitter.reset()
	composite.PushParentStateChange(22, "parent-22", taskManager.TaskStateProcessing)
	if len(emitter.snapshot()) != 1 {
		t.Fatalf("父任务非终态推送仍须透传 Wails 一路")
	}
	if got := countRows(t, repo); got != 2 {
		t.Fatalf("父任务非终态应零落账，实际账行 %d", got)
	}

	// 非状态类推送（移除/查重）：透传照旧、零落账
	emitter.reset()
	composite.PushTaskRemove([]int64{11})
	composite.PushDuplicateDetected(11, "task-11", 7, "work-7", []string{"image"})
	otherEvents := emitter.snapshot()
	if len(otherEvents) != 2 {
		t.Fatalf("移除/查重推送应各透传 1 条 Wails 事件，实际 %d", len(otherEvents))
	}
	if env := otherEvents[0].envelope(t); env.Name != "task-events" || env.Data[0].Type != "removeTask" {
		t.Fatalf("移除任务主题/事件类型不符: %+v", env)
	}
	if otherEvents[1].Name != "taskStatus-duplicateDetected" {
		t.Fatalf("查重推送主题不符: %s", otherEvents[1].Name)
	}
	if got := countRows(t, repo); got != 2 {
		t.Fatalf("非状态类推送应零落账，实际账行 %d", got)
	}
}

// TestTaskTerminalFilterCoversAllStates 终态过滤边界：遍历 taskManager.TaskState 全量枚举值
// （taskManager/model.go:23-34），仅 {Finished, Failed, PartlyFinished} 落账——判定集与
// taskManager/manager.go:1424-1426 同集；Paused 是稳定态但非终态，不落账
func TestTaskTerminalFilterCoversAllStates(t *testing.T) {
	svc, repo := newTestService(t)
	ctx := context.Background()
	recorder := NewTaskTerminalRecorder(svc)

	allStates := []taskManager.TaskState{
		taskManager.TaskStateCreated, taskManager.TaskStateWaiting, taskManager.TaskStateProcessing,
		taskManager.TaskStatePausing, taskManager.TaskStatePaused, taskManager.TaskStateStopping,
		taskManager.TaskStateFinished, taskManager.TaskStateFailed, taskManager.TaskStatePartlyFinished,
		taskManager.TaskStateWaitingForInput,
	}
	terminalSet := map[taskManager.TaskState]bool{
		taskManager.TaskStateFinished:       true,
		taskManager.TaskStateFailed:         true,
		taskManager.TaskStatePartlyFinished: true,
	}

	var expectedRows int64
	for i, state := range allStates {
		isTerminal := terminalSet[state]
		if got := isTerminalTaskState(state); got != isTerminal {
			t.Fatalf("状态 %d 终态判定应为 %t，实际 %t", int(state), isTerminal, got)
		}
		before := countRows(t, repo)
		if recorded := recorder.RecordTerminalState(ctx, int64(100+i), "task", state, false); recorded != isTerminal {
			t.Fatalf("状态 %d RecordTerminalState 应返回 %t，实际 %t", int(state), isTerminal, recorded)
		}
		expectedRows += boolToInt64(isTerminal)
		if got, want := countRows(t, repo), before+boolToInt64(isTerminal); got != want {
			t.Fatalf("状态 %d 落账行数不符：期望 %d，实际 %d", int(state), want, got)
		}
	}
	if got := countRows(t, repo); got != expectedRows {
		t.Fatalf("全状态遍历后应共 %d 行终态账，实际 %d", expectedRows, got)
	}
}

// TestTaskTerminalPusherConcurrentAccounting 并发终态迁移（-race）：多任务并发终态推送两路互不
// 干扰——Wails 一路事件数与推送数守恒，记账一路行数守恒且游标 id 严格升序（无丢账/无重复）
func TestTaskTerminalPusherConcurrentAccounting(t *testing.T) {
	svc, repo := newTestService(t)

	emitter := &terminalRecordingEmitter{}
	composite := NewTaskTerminalPusher(taskManager.NewWailsTaskEventPusher(emitter), NewTaskTerminalRecorder(svc))

	const workers, perWorker = 8, 10
	states := []taskManager.TaskState{
		taskManager.TaskStateFinished, taskManager.TaskStateFailed, taskManager.TaskStatePartlyFinished,
	}

	var wg sync.WaitGroup
	for w := range workers {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := range perWorker {
				// 终态与非终态交替：仅终态落账、非终态只走 Wails 一路
				state := states[i%len(states)]
				if i%2 == 1 {
					state = taskManager.TaskStateProcessing
				}
				composite.PushStateChange(int64(w*1000+i+1), "task", state)
			}
		}(w)
	}
	wg.Wait()

	if got, want := len(emitter.snapshot()), workers*perWorker; got != want {
		t.Fatalf("Wails 一路事件数应守恒 %d，实际 %d", want, got)
	}
	entries := listTerminalEntries(t, repo)
	if got, want := len(entries), workers*perWorker/2; got != want {
		t.Fatalf("并发终态应落 %d 行账，实际 %d", want, got)
	}
	prev := int64(0)
	for _, e := range entries {
		if id := e.GetID(); id <= prev {
			t.Fatalf("账行 id 应严格升序（自增游标保序）：prev=%d id=%d", prev, id)
		} else {
			prev = id
		}
		if e.Domain != entity.JournalDomainTask || e.EventType != EventTypeTaskTerminal {
			t.Fatalf("账行域/事件类型不符: domain=%s eventType=%s", e.Domain, e.EventType)
		}
	}
}
