package plugin

import (
	"context"
	"errors"
	"testing"

	dto "github.com/library-squirrel/backend/base/model/dto"
	entity2 "github.com/library-squirrel/backend/base/model/entity"
)

// recordingParticipant 按调用顺序记录相位事件的参与者测试替身；
// activateErr 非 nil 时 Activate 返回失败；vetoErr 非 nil 时 PrepareStop 返回否决
type recordingParticipant struct {
	name        string
	activateErr error
	vetoErr     error
	events      *[]string
}

func (p *recordingParticipant) Activate(ctx context.Context, plugin *entity2.Plugin, manifest *dto.PluginManifest) error {
	*p.events = append(*p.events, p.name+".Activate")
	return p.activateErr
}

func (p *recordingParticipant) PrepareStop(ctx context.Context, pluginPublicId string, op PluginStopOp, force bool) error {
	*p.events = append(*p.events, p.name+".PrepareStop")
	return p.vetoErr
}

func (p *recordingParticipant) OnStopped(ctx context.Context, pluginPublicId string) {
	*p.events = append(*p.events, p.name+".OnStopped")
}

// newActiveManager 构造带记录参与者的状态机（注册顺序即相位顺序）并置插件为运行中，
// 供停用相位测试使用
func newActiveManager(events *[]string, names ...string) *lifecycleManager {
	m := newLifecycleManager()
	for _, name := range names {
		m.registerParticipant(&recordingParticipant{name: name, events: events})
	}
	m.states["com.example.plugin"] = lifecycleActive
	return m
}

// TestDeactivatePhaseOrder 验证停用相位顺序：全部参与者否决检查 → 按注册逆序清理
//（后注册的进程域先清，痕迹域后清），顺序错乱会令痕迹清理先于进程停止或否决失效
func TestDeactivatePhaseOrder(t *testing.T) {
	var events []string
	m := newActiveManager(&events, "static", "frontend", "proc")

	if err := m.deactivate(context.Background(), "com.example.plugin", PluginStopOpUninstall, false); err != nil {
		t.Fatalf("deactivate 失败: %v", err)
	}

	want := []string{
		"static.PrepareStop", "frontend.PrepareStop", "proc.PrepareStop",
		"proc.OnStopped", "frontend.OnStopped", "static.OnStopped",
	}
	if len(events) != len(want) {
		t.Fatalf("相位事件数不符: %v", events)
	}
	for i, e := range want {
		if events[i] != e {
			t.Fatalf("相位顺序不符: 期望 %v, 实际 %v", want, events)
		}
	}
	if _, still := m.states["com.example.plugin"]; still {
		t.Fatal("停用后状态应回未激活（表项移除）")
	}
}

// TestDeactivateVetoAborts 验证否决中止：任一参与者 PrepareStop 报错时不执行任何清理，
// 状态保持运行中（插件继续运行）
func TestDeactivateVetoAborts(t *testing.T) {
	var events []string
	m := newLifecycleManager()
	m.registerParticipant(&recordingParticipant{name: "static", events: &events})
	m.registerParticipant(&recordingParticipant{name: "proc", vetoErr: errors.New("有运行中任务"), events: &events})
	m.states["com.example.plugin"] = lifecycleActive

	err := m.deactivate(context.Background(), "com.example.plugin", PluginStopOpUpdate, false)
	if err == nil {
		t.Fatal("否决未生效：deactivate 应返回错误")
	}
	if len(events) != 2 { // 仅 static/proc 的 PrepareStop，无 OnStopped
		t.Fatalf("否决后不应有清理事件: %v", events)
	}
	if m.states["com.example.plugin"] != lifecycleActive {
		t.Fatal("否决后状态应保持运行中")
	}
}

// TestDeactivateForceSkipsVeto 验证 force 跳过否决（用户确认后强制停路径）
func TestDeactivateForceSkipsVeto(t *testing.T) {
	var events []string
	m := newLifecycleManager()
	m.registerParticipant(&recordingParticipant{name: "proc", vetoErr: errors.New("有运行中任务"), events: &events})
	m.states["com.example.plugin"] = lifecycleActive

	if err := m.deactivate(context.Background(), "com.example.plugin", PluginStopOpUntrust, true); err != nil {
		t.Fatalf("force 停用失败: %v", err)
	}
	if len(events) != 1 || events[0] != "proc.OnStopped" {
		t.Fatalf("force 应跳过否决直达清理: %v", events)
	}
}
