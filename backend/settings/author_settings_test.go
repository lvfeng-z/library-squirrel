package settings

import (
	"path/filepath"
	"testing"
)

// TestAuthorAutoFetchInfoDefault 作者自动拉取开关默认值：双源构造（NewSettings 与
// defaultSettings）同值锚定（漏一侧 koanf 以零值 false 覆盖默认开的既有事故形态）+ 服务
// 读取与显式关闭生效
func TestAuthorAutoFetchInfoDefault(t *testing.T) {
	a := NewSettings().AuthorSettings.AutoFetchInfo
	b := defaultSettings().AuthorSettings.AutoFetchInfo
	if !a || !b {
		t.Fatalf("默认值双源应一致且为开: NewSettings=%v defaultSettings=%v", a, b)
	}

	svc := NewService(filepath.Join(t.TempDir(), "settings.json"))
	if !svc.AuthorAutoFetchInfoEnabled() {
		t.Fatal("无配置文件时应读默认开")
	}
	if err := svc.SaveSettings([]SettingChange{{Path: "authorSettings.autoFetchInfo", Value: false}}); err != nil {
		t.Fatalf("保存设置失败: %v", err)
	}
	if svc.AuthorAutoFetchInfoEnabled() {
		t.Fatal("显式关闭应生效")
	}
}
