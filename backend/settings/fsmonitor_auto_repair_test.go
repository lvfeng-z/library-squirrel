package settings

import (
	"path/filepath"
	"reflect"
	"testing"
)

// TestFsmonitorAutoRepairDefaults 自动修复设置默认值双源锚定：NewSettings 与 defaultSettings 同值、
// 开关默认关；服务无配置时读默认；显式配置开关与策略覆盖表生效并持久化（含 map 值读写）
func TestFsmonitorAutoRepairDefaults(t *testing.T) {
	a := NewSettings().FsmonitorSettings
	b := defaultSettings().FsmonitorSettings
	if a.AutoRepairEnabled != b.AutoRepairEnabled || a.AutoRepairEnabled {
		t.Fatalf("默认开关双源不一致或非关: NewSettings=%v defaultSettings=%v", a.AutoRepairEnabled, b.AutoRepairEnabled)
	}
	if !reflect.DeepEqual(a.AutoRepairPolicies, b.AutoRepairPolicies) {
		t.Fatalf("默认策略表双源不一致: NewSettings=%v defaultSettings=%v", a.AutoRepairPolicies, b.AutoRepairPolicies)
	}

	svc := NewService(filepath.Join(t.TempDir(), "settings.json"))
	fs := svc.GetSettings().FsmonitorSettings
	if fs.AutoRepairEnabled {
		t.Fatal("无配置文件时自动修复开关应默认关")
	}
	if len(fs.AutoRepairPolicies) != 0 {
		t.Fatalf("无配置文件时策略覆盖表应空，实际 %v", fs.AutoRepairPolicies)
	}

	// 显式配置生效（开关 + 策略覆盖表；map 经 koanf.Set 落盘后读回）
	if err := svc.SaveSettings([]SettingChange{
		{Path: "fsmonitor.autoRepairEnabled", Value: true},
		{Path: "fsmonitor.autoRepairPolicies", Value: map[string]string{"store:Move": "restore", "store:Delete": "ack"}},
	}); err != nil {
		t.Fatalf("保存设置失败: %v", err)
	}
	fs2 := svc.GetSettings().FsmonitorSettings
	if !fs2.AutoRepairEnabled {
		t.Fatal("显式开启自动修复未生效")
	}
	if fs2.AutoRepairPolicies["store:Move"] != "restore" || fs2.AutoRepairPolicies["store:Delete"] != "ack" {
		t.Fatalf("策略覆盖表未生效: %v", fs2.AutoRepairPolicies)
	}
}
