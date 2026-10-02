package importer

import (
	"strings"
	"testing"
)

// TestSanitizeMetaText 自 share 迁移（行为保持，断言原样）：控制字符剔除与 rune 截断。
func TestSanitizeMetaText(t *testing.T) {
	if got := SanitizeMetaText("a\r b\n c\t d\x07", 100); got != "a b c d" {
		t.Fatalf("控制字符未剔除: %q", got)
	}
	long := strings.Repeat("测", 300)
	if got := SanitizeMetaText(long, 200); len([]rune(got)) != 200 {
		t.Fatalf("rune 截断失败: %d", len([]rune(got)))
	}
}
