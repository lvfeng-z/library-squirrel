package settings

import (
	"path/filepath"
	"testing"
)

// TestAppearanceViewCloseButtonEnabledDefault 视图关闭按钮开关默认值：双源构造（NewSettings 与
// defaultSettings）同值锚定（两处不同步会让默认值随加载路径漂移）+ 服务读取与显式开启生效
func TestAppearanceViewCloseButtonEnabledDefault(t *testing.T) {
	a := NewSettings().Appearance.ViewCloseButtonEnabled
	b := defaultSettings().Appearance.ViewCloseButtonEnabled
	if a || b {
		t.Fatalf("默认值双源应一致且为关: NewSettings=%v defaultSettings=%v", a, b)
	}

	svc := NewService(filepath.Join(t.TempDir(), "settings.json"))
	if svc.GetSettings().Appearance.ViewCloseButtonEnabled {
		t.Fatal("无配置文件时应读默认关")
	}
	if err := svc.SaveSettings([]SettingChange{{Path: "appearance.viewCloseButtonEnabled", Value: true}}); err != nil {
		t.Fatalf("保存设置失败: %v", err)
	}
	if !svc.GetSettings().Appearance.ViewCloseButtonEnabled {
		t.Fatal("显式开启应生效")
	}
}
