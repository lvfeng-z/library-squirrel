package migration

import "testing"

// TestDeriveNewPublicId 验证旧格式 publicId（author/id[_uuid]）到新身份键的派生规则：
// 去 author 段 + 剥离 id 末尾的旧身份 UUID 后缀；无法派生时返回空串（迁移据此跳过记录）
func TestDeriveNewPublicId(t *testing.T) {
	tests := []struct {
		name string
		old  string
		want string
	}{
		{"标准旧格式 author/id_uuid", "lvfeng/com.lvfeng.pixivSuite_001594e7-dea2-5774-854f-7ac9950e04a0", "com.lvfeng.pixivSuite"},
		{"旧格式无 UUID 后缀仅去 author 段", "someone/com.example.plugin", "com.example.plugin"},
		{"id 本名含连字符不被误剥", "lvfeng/com.lvfeng.test-plugin_00000000-0000-0000-0000-000000000001", "com.lvfeng.test-plugin"},
		{"大写 UUID 同样剥离", "a/com.b_001594E7-DEA2-5774-854F-7AC9950E04A0", "com.b"},
		{"尾部非完整 UUID 不剥离", "someone/com.example.plugin_notauuidx", "com.example.plugin_notauuidx"},
		{"新格式（无 / 分隔）不派生", "com.lvfeng.pixivSuite", ""},
		{"author 段后为空", "lvfeng/", ""},
		{"多斜杠异常数据不派生（结果含 / 非法）", "lvfeng/com.example.plugin/1.0.0", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deriveNewPublicId(tt.old); got != tt.want {
				t.Errorf("deriveNewPublicId(%q) = %q, want %q", tt.old, got, tt.want)
			}
		})
	}
}
