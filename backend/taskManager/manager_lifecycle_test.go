package taskManager

import (
	"database/sql"
	"testing"
)

// TestCountActiveByPlugin 验证插件停用拦截判据：仅统计该插件名下运行中任务
// （Processing/Pausing/Stopping/WaitingForInput），Created/Paused/终态与其他插件不计
func TestCountActiveByPlugin(t *testing.T) {
	mgr := NewManager(2, nil, nil, nil, nil, nil)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()

	const target = "com.example.plugin"
	mk := func(id int64, plugin string, state TaskState) *ManagedTask {
		mt := newTestManagedTask()
		mt.taskId = id
		mt.task.PluginPublicID = sql.NullString{String: plugin, Valid: true}
		mt.setState(state)
		return mt
	}

	mgr.mu.Lock()
	mgr.taskMap[1] = mk(1, target, TaskStateProcessing)
	mgr.taskMap[2] = mk(2, target, TaskStatePausing)
	mgr.taskMap[3] = mk(3, target, TaskStateStopping)
	mgr.taskMap[4] = mk(4, target, TaskStateWaitingForInput)
	mgr.taskMap[5] = mk(5, target, TaskStatePaused)
	mgr.taskMap[6] = mk(6, target, TaskStateCreated)
	mgr.taskMap[7] = mk(7, target, TaskStateFinished)
	mgr.taskMap[8] = mk(8, "com.other.plugin", TaskStateProcessing)
	mgr.mu.Unlock()

	if n := mgr.CountActiveByPlugin(target); n != 4 {
		t.Fatalf("运行中任务计数不符: 期望 4, 实际 %d", n)
	}
	if n := mgr.CountActiveByPlugin("com.other.plugin"); n != 1 {
		t.Fatalf("其他插件计数不符: 期望 1, 实际 %d", n)
	}
	if n := mgr.CountActiveByPlugin("com.absent.plugin"); n != 0 {
		t.Fatalf("无任务插件计数应为 0, 实际 %d", n)
	}
}
