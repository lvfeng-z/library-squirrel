package appLauncher

import (
	"errors"
	"testing"

	"github.com/library-squirrel/backend/settings"
)

// stubWorkDirProvider 测试用工作目录提供者
type stubWorkDirProvider struct{ workDir string }

func (p stubWorkDirProvider) GetWorkDir() string { return p.workDir }

// TestOpenPathRefusesUnconfiguredWorkDir 工作目录未配置（GetWorkDir 空串）时，
// OpenPath（含其委托方 OpenImage）返回 ErrWorkDirNotConfigured，不发起系统打开命令
func TestOpenPathRefusesUnconfiguredWorkDir(t *testing.T) {
	svc := NewService(stubWorkDirProvider{workDir: ""})

	if err := svc.OpenPath("store/resource/a/1.jpg"); !errors.Is(err, settings.ErrWorkDirNotConfigured) {
		t.Errorf("OpenPath 期望返回 ErrWorkDirNotConfigured，实际 %v", err)
	}
	if err := svc.OpenImage("store/resource/a/1.jpg"); !errors.Is(err, settings.ErrWorkDirNotConfigured) {
		t.Errorf("OpenImage 期望返回 ErrWorkDirNotConfigured，实际 %v", err)
	}
}
