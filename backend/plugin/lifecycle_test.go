package plugin

import (
	"context"
	"errors"
	"testing"
)

// recordingParticipant 按调用顺序记录相位事件的参与者测试替身；vetoErr 非 nil 时 PrepareStop 返回否决
type recordingParticipant struct {
	name    string
	vetoErr error
	events  *[]string
}

func (p *recordingParticipant) PrepareStop(ctx context.Context, pluginPublicId string, op PluginStopOp, force bool) error {
	*p.events = append(*p.events, p.name+".PrepareStop")
	return p.vetoErr
}

func (p *recordingParticipant) OnStopped(ctx context.Context, pluginPublicId string) {
	*p.events = append(*p.events, p.name+".OnStopped")
}

// TestStopRuntimePhaseOrder 验证停用相位顺序：全部参与者否决检查 → 停进程 → 参与者清理，
// 顺序错乱会令清理先于进程停止或否决失效
func TestStopRuntimePhaseOrder(t *testing.T) {
	var events []string
	stopped := false
	svc := &Service{
		participants: []LifecycleParticipant{
			&recordingParticipant{name: "p1", events: &events},
			&recordingParticipant{name: "p2", events: &events},
		},
		runtimeStopper: func(pluginPublicId string) error {
			stopped = true
			events = append(events, "stopper")
			return nil
		},
	}

	if err := svc.stopRuntime(context.Background(), "com.example.plugin", PluginStopOpUninstall, false); err != nil {
		t.Fatalf("stopRuntime 失败: %v", err)
	}

	want := []string{"p1.PrepareStop", "p2.PrepareStop", "stopper", "p1.OnStopped", "p2.OnStopped"}
	if len(events) != len(want) {
		t.Fatalf("相位事件数不符: %v", events)
	}
	for i, e := range want {
		if events[i] != e {
			t.Fatalf("相位顺序不符: 期望 %v, 实际 %v", want, events)
		}
	}
	if !stopped {
		t.Fatal("运行时停止器未被调用")
	}
}

// TestStopRuntimeVetoAborts 验证否决中止：任一参与者 PrepareStop 报错时停进程与清理均不执行
func TestStopRuntimeVetoAborts(t *testing.T) {
	var events []string
	stopped := false
	svc := &Service{
		participants: []LifecycleParticipant{
			&recordingParticipant{name: "p1", events: &events},
			&recordingParticipant{name: "p2", vetoErr: errors.New("有运行中任务"), events: &events},
		},
		runtimeStopper: func(pluginPublicId string) error {
			stopped = true
			events = append(events, "stopper")
			return nil
		},
	}

	err := svc.stopRuntime(context.Background(), "com.example.plugin", PluginStopOpUpdate, false)
	if err == nil {
		t.Fatal("否决未生效：stopRuntime 应返回错误")
	}
	if stopped {
		t.Fatal("否决后进程不应被停止")
	}
	if len(events) != 2 { // 仅 p1/p2 的 PrepareStop，无 stopper 与 OnStopped
		t.Fatalf("否决后不应有停进程与清理事件: %v", events)
	}
}

// TestStopRuntimeForceSkipsVeto 验证 force 跳过否决（用户确认后强制停路径）
func TestStopRuntimeForceSkipsVeto(t *testing.T) {
	var events []string
	svc := &Service{
		participants: []LifecycleParticipant{
			&recordingParticipant{name: "p1", vetoErr: errors.New("有运行中任务"), events: &events},
		},
		runtimeStopper: func(pluginPublicId string) error {
			events = append(events, "stopper")
			return nil
		},
	}

	if err := svc.stopRuntime(context.Background(), "com.example.plugin", PluginStopOpUntrust, true); err != nil {
		t.Fatalf("force 停用失败: %v", err)
	}
	if len(events) != 2 || events[0] != "stopper" || events[1] != "p1.OnStopped" {
		t.Fatalf("force 应跳过否决直达停进程: %v", events)
	}
}
