package backup

import (
	"context"
	"errors"
	"testing"

	"github.com/library-squirrel/backend/settings"
)

// TestServiceRefusesUnconfiguredWorkDir 工作目录未配置（GetWorkDir 空串）时，
// 备份收存（storeFile 汇口，覆盖 CreateBackup/MoveToBackup 两入口）与还原入口一律
// 返回 ErrWorkDirNotConfigured，判定先于仓储与磁盘访问（nil 仓储不可达），
// 清单行 Workdir 不再落空串
func TestServiceRefusesUnconfiguredWorkDir(t *testing.T) {
	svc := NewService(nil, func() string { return "" })
	ctx := context.Background()

	if _, err := svc.CreateBackup(ctx, "C:/src.mp4"); !errors.Is(err, settings.ErrWorkDirNotConfigured) {
		t.Errorf("CreateBackup 期望返回 ErrWorkDirNotConfigured，实际 %v", err)
	}
	if _, err := svc.MoveToBackup(ctx, "C:/src.mp4"); !errors.Is(err, settings.ErrWorkDirNotConfigured) {
		t.Errorf("MoveToBackup 期望返回 ErrWorkDirNotConfigured，实际 %v", err)
	}
	if err := svc.RestoreFile(ctx, "C:/backup/2026/09/02/x.mp4", "C:/store/resource/a/x.mp4"); !errors.Is(err, settings.ErrWorkDirNotConfigured) {
		t.Errorf("RestoreFile 期望返回 ErrWorkDirNotConfigured，实际 %v", err)
	}
}
