package dto

import "testing"

// TestValidatePluginID 验证插件 id（= publicId 身份键）安装期校验：须为反向域名格式
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
		{"含下划线", "com.lvfeng.pixivSuite_001594e7-dea2-5774-854f-7ac9950e04a0", true},
		{"含斜杠", "lvfeng/com.lvfeng.pixivSuite", true},
		{"含空格", "com.lvfeng.pixiv Suite", true},
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
