//go:build !windows

package workdirGuard

import (
	"context"
	"testing"
)

// TestOtherGuardNoop no-op 实现：Probe 恒成功、Info 声明无内置机制
func TestOtherGuardNoop(t *testing.T) {
	g := newOtherGuard()
	if err := g.Probe(context.Background(), t.TempDir()); err != nil {
		t.Fatalf("no-op Probe 应恒成功: %v", err)
	}
	info := g.Info()
	if info.Supported {
		t.Fatal("no-op 实现应 Supported=false")
	}
	if info.Mechanism != "无内置机制" {
		t.Fatalf("机制名应为「无内置机制」，实际 %q", info.Mechanism)
	}
	if info.Guide == "" {
		t.Fatal("Guide 不应为空")
	}
}
