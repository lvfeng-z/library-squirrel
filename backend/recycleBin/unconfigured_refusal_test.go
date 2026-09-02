package recycleBin

import (
	"context"
	"errors"
	"testing"

	"github.com/library-squirrel/backend/settings"
)

// TestRestoreEntriesRefuseUnconfiguredWorkDir 工作目录未配置（GetWorkDir 空串）时，
// 作品复原与文件条目复原两入口一律返回 ErrWorkDirNotConfigured，判定先于各依赖访问
// （nil 依赖不可达）
func TestRestoreEntriesRefuseUnconfiguredWorkDir(t *testing.T) {
	svc := NewService(nil, nil, nil, nil, nil, nil, nil, nil, func() string { return "" }, nil, nil, nil, nil, nil, nil)
	ctx := context.Background()

	if _, err := svc.RestoreWork(ctx, 1, false); !errors.Is(err, settings.ErrWorkDirNotConfigured) {
		t.Errorf("RestoreWork 期望返回 ErrWorkDirNotConfigured，实际 %v", err)
	}
	if err := svc.RestoreStore(ctx, 1); !errors.Is(err, settings.ErrWorkDirNotConfigured) {
		t.Errorf("RestoreStore 期望返回 ErrWorkDirNotConfigured，实际 %v", err)
	}
}
