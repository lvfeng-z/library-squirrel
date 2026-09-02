package resource

import (
	"context"
	"errors"
	"testing"

	"github.com/library-squirrel/backend/settings"
)

// TestRestoreReplacedStoresRefusesUnconfiguredWorkDir 工作目录未配置（GetWorkDir 空串）时，
// 替换回滚入口返回 ErrWorkDirNotConfigured，判定先于各依赖访问（nil 依赖不可达）
func TestRestoreReplacedStoresRefusesUnconfiguredWorkDir(t *testing.T) {
	svc := NewReplacementService(nil, nil, nil, nil, nil, nil, nil, replaceWorkDir{workDir: ""}, nil)

	err := svc.RestoreReplacedStores(context.Background(), RestoreScope{WorkID: 1, Roles: []string{"image"}})
	if !errors.Is(err, settings.ErrWorkDirNotConfigured) {
		t.Errorf("RestoreReplacedStores 期望返回 ErrWorkDirNotConfigured，实际 %v", err)
	}
}
