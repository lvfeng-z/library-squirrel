package settings

import (
	"path/filepath"
	"testing"
)

// TestExportFileNameFormatDefaults 导出文件名模板默认值：双源构造（NewSettings 与
// defaultSettings）同值锚定 + 服务读取兜底（空值回退默认模板）。
// koanf 按零值合并 struct 默认层，任一构造源缺失该默认即整链退化为空模板。
func TestExportFileNameFormatDefaults(t *testing.T) {
	a := NewSettings().ExportSettings.FileNameFormat
	b := defaultSettings().ExportSettings.FileNameFormat
	if a != b || a != DefaultFileNameFormat {
		t.Fatalf("默认值双源不一致: NewSettings=%q defaultSettings=%q 常量=%q", a, b, DefaultFileNameFormat)
	}
	if DefaultFileNameFormat == "" {
		t.Fatal("默认模板不得为空（空模板渲染不出可用的文件主名）")
	}

	svc := NewService(filepath.Join(t.TempDir(), "settings.json"))
	if got := svc.GetFileNameFormat(); got != DefaultFileNameFormat {
		t.Fatalf("无配置文件时应读默认 %q，实际 %q", DefaultFileNameFormat, got)
	}

	// 显式配置生效
	if err := svc.SaveSettings([]SettingChange{{Path: "exportSettings.fileNameFormat", Value: "[${author}]"}}); err != nil {
		t.Fatalf("保存设置失败: %v", err)
	}
	if got := svc.GetFileNameFormat(); got != "[${author}]" {
		t.Fatalf("显式配置未生效，实际 %q", got)
	}

	// 清空回退默认
	if err := svc.SaveSettings([]SettingChange{{Path: "exportSettings.fileNameFormat", Value: ""}}); err != nil {
		t.Fatalf("保存设置失败: %v", err)
	}
	if got := svc.GetFileNameFormat(); got != DefaultFileNameFormat {
		t.Fatalf("清空后应回退默认 %q，实际 %q", DefaultFileNameFormat, got)
	}
}
