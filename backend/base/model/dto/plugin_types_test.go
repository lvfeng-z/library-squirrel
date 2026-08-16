package dto

import "testing"

// TestValidatePluginID 验证插件 id（= publicId 身份键）安装期校验：
// 须为反向域名格式，且不得残留旧身份的 UUID 后缀（防新旧格式混存产生双身份记录）
func TestValidatePluginID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"反向域名", "com.lvfeng.pixivSuite", false},
		{"本名含连字符", "com.lvfeng.test-plugin", false},
		{"多级域名", "com.example.corp.somePlugin", false},
		{"单段无点", "pixivSuite", true},
		{"含斜杠", "lvfeng/com.lvfeng.pixivSuite", true},
		{"含空格", "com.lvfeng.pixiv Suite", true},
		{"旧身份 UUID 后缀残留", "com.lvfeng.pixivSuite_001594e7-dea2-5774-854f-7ac9950e04a0", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePluginID(tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePluginID(%q) error = %v, wantErr %v", tt.id, err, tt.wantErr)
			}
		})
	}
}

// TestStripLegacyUUIDSuffix 验证旧身份 UUID 后缀剥离（无后缀原样返回，供存量迁移派生用）
func TestStripLegacyUUIDSuffix(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"剥离标准 UUID 后缀", "com.lvfeng.pixivSuite_001594e7-dea2-5774-854f-7ac9950e04a0", "com.lvfeng.pixivSuite"},
		{"无后缀原样返回", "com.lvfeng.pixivSuite", "com.lvfeng.pixivSuite"},
		{"本名含连字符不误剥", "com.lvfeng.test-plugin_00000000-0000-0000-0000-000000000001", "com.lvfeng.test-plugin"},
		{"非 UUID 尾段不动", "com.example.plugin_notauuid", "com.example.plugin_notauuid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StripLegacyUUIDSuffix(tt.in); got != tt.want {
				t.Errorf("StripLegacyUUIDSuffix(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
