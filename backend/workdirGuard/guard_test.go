package workdirGuard

import (
	"context"
	"errors"
	"runtime"
	"testing"
)

// TestNewPlatformGuardDispatch 平台分支：windows 返回受控文件夹访问实现，其余平台返回 no-op
func TestNewPlatformGuardDispatch(t *testing.T) {
	g := NewPlatformGuard()
	if g == nil {
		t.Fatal("NewPlatformGuard 返回 nil")
	}
	info := g.Info()
	if info.Platform != runtime.GOOS {
		t.Fatalf("Info.Platform 应为 %s，实际 %s", runtime.GOOS, info.Platform)
	}
	switch runtime.GOOS {
	case "windows":
		if !info.Supported {
			t.Fatal("windows 平台应 Supported=true")
		}
		if info.Mechanism != "受控文件夹访问" {
			t.Fatalf("windows 机制名应为「受控文件夹访问」，实际 %q", info.Mechanism)
		}
	default:
		if info.Supported {
			t.Fatal("非 windows 平台应 Supported=false")
		}
		if info.Mechanism != "无内置机制" {
			t.Fatalf("非 windows 机制名应为「无内置机制」，实际 %q", info.Mechanism)
		}
	}
	if info.Guide == "" {
		t.Fatal("Guide 引导文案不应为空")
	}
}

// fakeGuard 可控探针结果与机制信息的测试替身
type fakeGuard struct {
	probeErr error
	info     Info
}

func (f *fakeGuard) Probe(ctx context.Context, workDir string) error { return f.probeErr }
func (f *fakeGuard) Info() Info                                      { return f.info }

// TestHandlerGetWorkDirGuardInfo Handler 探测成功/失败/空 workDir 三分支
func TestHandlerGetWorkDirGuardInfo(t *testing.T) {
	baseInfo := Info{Platform: "windows", Mechanism: "受控文件夹访问", Supported: true, Guide: "guide"}

	// 探测成功分支：ProbeOk=true、ProbeErr 为空、Info 透传
	h := NewHandler(&fakeGuard{info: baseInfo})
	resp := h.GetWorkDirGuardInfo(context.Background(), `E:\workdir`)
	if !resp.Success {
		t.Fatalf("响应应成功: %s", resp.Msg)
	}
	if !resp.Data.ProbeOk {
		t.Fatalf("探测应通过，ProbeErr=%q", resp.Data.ProbeErr)
	}
	if resp.Data.ProbeErr != "" {
		t.Fatalf("探测成功时 ProbeErr 应为空，实际 %q", resp.Data.ProbeErr)
	}
	if resp.Data.Info.Mechanism != "受控文件夹访问" {
		t.Fatal("Info 应透传 Guard.Info 结果")
	}

	// 探测失败分支：ProbeOk=false、ProbeErr 给出原因
	h2 := NewHandler(&fakeGuard{probeErr: errors.New("ACCESS_DENIED"), info: baseInfo})
	resp2 := h2.GetWorkDirGuardInfo(context.Background(), `E:\workdir`)
	if resp2.Data.ProbeOk {
		t.Fatal("探测失败时 ProbeOk 应为 false")
	}
	if resp2.Data.ProbeErr == "" {
		t.Fatal("探测失败应给出 ProbeErr")
	}

	// workDir 为空：跳过探测，仅返回机制信息（ProbeOk=false）
	h3 := NewHandler(&fakeGuard{info: baseInfo})
	resp3 := h3.GetWorkDirGuardInfo(context.Background(), "")
	if resp3.Data.ProbeOk {
		t.Fatal("workDir 为空时不应探测，ProbeOk 应为 false")
	}
	if resp3.Data.Info.Mechanism != "受控文件夹访问" {
		t.Fatal("空 workDir 仍应返回机制信息")
	}
}

// TestHandlerNilGuard guard 缺失时返回失败响应（不 panic）
func TestHandlerNilGuard(t *testing.T) {
	h := NewHandler(nil)
	resp := h.GetWorkDirGuardInfo(context.Background(), `E:\workdir`)
	if resp.Success {
		t.Fatal("guard 为 nil 时响应应为失败")
	}
}
