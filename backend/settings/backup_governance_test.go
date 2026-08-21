package settings

import (
	"path/filepath"
	"testing"
)

// TestBackupGovernanceRetentionDefaults 备份治理保留期默认值：双源构造（NewSettings 与
// defaultSettings）同值锚定 + 服务读取兜底（零值/负值不被接受为合法保留期，回退默认）
func TestBackupGovernanceRetentionDefaults(t *testing.T) {
	a := NewSettings().BackupGovernance.RetentionDays
	b := defaultSettings().BackupGovernance.RetentionDays
	if a != b || a != DefaultBackupGovernanceRetentionDays {
		t.Fatalf("默认值双源不一致: NewSettings=%d defaultSettings=%d 常量=%d", a, b, DefaultBackupGovernanceRetentionDays)
	}

	svc := NewService(filepath.Join(t.TempDir(), "settings.json"))
	if got := svc.GetBackupGovernanceRetentionDays(); got != DefaultBackupGovernanceRetentionDays {
		t.Fatalf("无配置文件时应读默认 %d，实际 %d", DefaultBackupGovernanceRetentionDays, got)
	}

	// 显式配置生效
	if err := svc.SaveSettings([]SettingChange{{Path: "backupGovernance.retentionDays", Value: 30}}); err != nil {
		t.Fatalf("保存设置失败: %v", err)
	}
	if got := svc.GetBackupGovernanceRetentionDays(); got != 30 {
		t.Fatalf("显式配置 30 未生效，实际 %d", got)
	}

	// 零值回退默认（0 作为"立即清空"不被接受——保留期是替换在途窗口的正确性参数）
	if err := svc.SaveSettings([]SettingChange{{Path: "backupGovernance.retentionDays", Value: 0}}); err != nil {
		t.Fatalf("保存设置失败: %v", err)
	}
	if got := svc.GetBackupGovernanceRetentionDays(); got != DefaultBackupGovernanceRetentionDays {
		t.Fatalf("零值应回退默认 %d，实际 %d", DefaultBackupGovernanceRetentionDays, got)
	}
}
