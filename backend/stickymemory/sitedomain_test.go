package stickymemory

import "testing"

// TestResolveTaskSiteDomainUnanimousSiteKey 候选 siteKey 全体一致且非空：取该声明值，
// URL 不参与判定（声明命中时配无法解析的 URL 仍取声明值）
func TestResolveTaskSiteDomainUnanimousSiteKey(t *testing.T) {
	got, ok := ResolveTaskSiteDomain([]string{"pixiv", "pixiv"}, "https://www.pixiv.net/artworks/123")
	if !ok || got != "pixiv" {
		t.Fatalf("一致 siteKey 应取声明值: got=%q ok=%v, 期望 pixiv true", got, ok)
	}

	// 声明一致即定站点域，URL 只在兜底轨才解析
	got, ok = ResolveTaskSiteDomain([]string{"bilibili"}, "://missing-scheme")
	if !ok || got != "bilibili" {
		t.Fatalf("一致 siteKey 时 URL 不应参与判定: got=%q ok=%v, 期望 bilibili true", got, ok)
	}

	// 首尾空白归一后一致，视为同一声明值
	got, ok = ResolveTaskSiteDomain([]string{" pixiv ", "pixiv"}, "https://www.pixiv.net/")
	if !ok || got != "pixiv" {
		t.Fatalf("空白归一后一致应取声明值: got=%q ok=%v, 期望 pixiv true", got, ok)
	}
}

// TestResolveTaskSiteDomainFallsBackToHost 混合声明（不一致或含空）与全空（含空列表）
// 均退任务 URL host 兜底
func TestResolveTaskSiteDomainFallsBackToHost(t *testing.T) {
	const rawURL = "https://www.pixiv.net/artworks/123"
	cases := [][]string{
		{"pixiv", "bilibili"}, // 候选间不一致
		{"pixiv", ""},         // 含空声明
		{"", ""},              // 全空
		nil,                   // 无候选（空列表）
	}
	for _, keys := range cases {
		got, ok := ResolveTaskSiteDomain(keys, rawURL)
		if !ok || got != "www.pixiv.net" {
			t.Errorf("候选 %v 应退 host 兜底: got=%q ok=%v, 期望 www.pixiv.net true", keys, got, ok)
		}
	}
}

// TestResolveTaskSiteDomainInvalidURL host 解析失败或为空：ok=false——本轮不查不写记忆，
// 冲突照常交用户显选
func TestResolveTaskSiteDomainInvalidURL(t *testing.T) {
	// 无一致声明 + URL 本身解析失败
	if got, ok := ResolveTaskSiteDomain([]string{"pixiv", "bilibili"}, "://missing-scheme"); ok {
		t.Fatalf("解析失败的 URL 应 ok=false: got=%q", got)
	}
	// URL 可解析但无 host（无 scheme 纯文本 / 空 URL），Hostname() 为空
	for _, rawURL := range []string{"no-scheme-plain-text", ""} {
		if got, ok := ResolveTaskSiteDomain(nil, rawURL); ok {
			t.Fatalf("无 host 的 URL %q 应 ok=false: got=%q", rawURL, got)
		}
	}
	// host 兜底轨带端口时取纯主机名
	if got, ok := ResolveTaskSiteDomain(nil, "https://www.example.com:8443/x"); !ok || got != "www.example.com" {
		t.Fatalf("带端口 URL 应取纯主机名: got=%q ok=%v, 期望 www.example.com true", got, ok)
	}
}
