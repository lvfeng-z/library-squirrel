package storeRegistry

import (
	"testing"
)

// TestInScanDirs 验证白名单谓词：命中 store/* 子树为 true，外部目录为 false。
// 从 fsmonitor/usn_windows_test.go 迁入并改为跨平台（无 windows build tag）。
func TestInScanDirs(t *testing.T) {
	cases := []struct {
		rel  string
		want bool
	}{
		{"store/resource/作者/x.jpg", true},
		{"store/resource", true},         // 子树根自身
		{"store/thumbnail/t.jpg", false}, // 已退役目录（缩略图统一进 store/resource，不再独立子目录）
		{"store/avatar/local/a.png", true},
		{"store/avatar/site/b.png", true},
		{"store/avatar", false}, // 仅 store/avatar 不在白名单（只有 local/site）
		{"backup/2026/x.mp4", false},
		{".git/config", false},
		{"log/server.log", false},
		{"", false},
		{".", false},
		{"store/resourceX/y.jpg", false}, // 前缀串匹配须按分隔符，非 store/resourceX
	}
	for _, c := range cases {
		if got := InScanDirs(c.rel); got != c.want {
			t.Fatalf("InScanDirs(%q) = %v want %v", c.rel, got, c.want)
		}
	}
}

// TestValidatePath 验证落盘前路径校验：白名单内放行、白名单外拒绝、反斜杠归一。
func TestValidatePath(t *testing.T) {
	ok := []string{
		"store/resource/作者/video.mp4",
		"store/resource",
		"store/avatar/local/1.png",
		"store/avatar/site/2.png",
	}
	for _, p := range ok {
		if err := ValidatePath(p); err != nil {
			t.Errorf("ValidatePath(%q) 期望通过，实际错误: %v", p, err)
		}
	}
	bad := []string{
		"backup/2026/x.mp4",
		"store/thumbnail/x.jpg", // 已退役目录，不再放行
		"store/avatar",          // 仅 store/avatar，未注册子目录
		"store/resourceX/y.jpg", // 前缀串匹配按分隔符，不误命中
		".git/config",
	}
	for _, p := range bad {
		if err := ValidatePath(p); err == nil {
			t.Errorf("ValidatePath(%q) 期望拒绝，实际通过", p)
		}
	}
	// Windows 反斜杠路径也须放行（ToSlash 归一）
	if err := ValidatePath(`store\resource\作者\v.mp4`); err != nil {
		t.Errorf("ValidatePath(反斜杠) 期望通过，实际错误: %v", err)
	}
}
