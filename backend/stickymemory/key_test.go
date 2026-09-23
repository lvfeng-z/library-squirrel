package stickymemory

import "testing"

// TestBuildContextKeyCandidateOrderStable 候选序稳定：乱序输入与排序输入产出同一
// context_key（候选按全键字典序），且调用方切片不被排序改写
func TestBuildContextKeyCandidateOrderStable(t *testing.T) {
	sorted := []string{"alpha\x00ext-1", "beta\x00ext-10", "beta\x00ext-2"}
	shuffled := []string{"beta\x00ext-2", "alpha\x00ext-1", "beta\x00ext-10"}

	got := BuildContextKey("pixiv", shuffled)
	if want := BuildContextKey("pixiv", sorted); got != want {
		t.Fatalf("乱序输入应产出与排序输入相同的键:\n got=%q\nwant=%q", got, want)
	}
	// 字典序逐段锚定："beta\x00ext-10" < "beta\x00ext-2"（前缀更短者小）
	if want := "pixiv\x02alpha\x00ext-1\x01beta\x00ext-10\x01beta\x00ext-2"; got != want {
		t.Fatalf("键编码不符:\n got=%q\nwant=%q", got, want)
	}
	if shuffled[0] != "beta\x00ext-2" {
		t.Fatalf("调用方切片不应被排序改写，实际 %q", shuffled)
	}
}

// TestContextKeySiteDomainSeparatorNoAmbiguity 站点域含候选列表分隔符（\x01）时无拆解
// 歧义、无键碰撞：域边界由 \x02 独占——嵌 \x01 的域与「短域 + 恰当首候选」的另一种
// 组合产出不同键，且各自拆解精确还原
func TestContextKeySiteDomainSeparatorNoAmbiguity(t *testing.T) {
	oddDomain := "site\x01odd"
	key := BuildContextKey(oddDomain, []string{"a\x00e1", "b\x00e2"})
	gotDomain, gotKeys := ParseContextKey(key)
	if gotDomain != oddDomain {
		t.Fatalf("嵌 \\x01 的站点域应精确还原: got=%q want=%q", gotDomain, oddDomain)
	}
	if len(gotKeys) != 2 || gotKeys[0] != "a\x00e1" || gotKeys[1] != "b\x00e2" {
		t.Fatalf("候选列表应精确还原: %q", gotKeys)
	}

	// 对照组合：域不含 \x01、首候选携带同一文本段——两组合产出不同键（\x01 vs \x02 在
	// 同一位置分界），不互串；域边界仍由首个 \x02 正确切出
	other := BuildContextKey("site", []string{"odd\x01a\x00e1", "b\x00e2"})
	if key == other {
		t.Fatalf("不同 (站点域, 候选组合) 对不应撞键: %q", key)
	}
	if otherDomain, _ := ParseContextKey(other); otherDomain != "site" {
		t.Fatalf("对照组合的域边界应按首个 \\x02 正确切出: %q", otherDomain)
	}
}

// TestParseContextKeyRoundTrip 拆解 round-trip：键 → (站点域, 候选全键列表) 精确还原，
// 拆解产物（已序）重组回同键；候选全键自身拆解为 (插件 PublicID, 扩展点 ID)
func TestParseContextKeyRoundTrip(t *testing.T) {
	domain := "pixiv"
	keys := []string{"plug-b\x00ext-9", "plug-a\x00ext-1"}
	key := BuildContextKey(domain, keys)

	gotDomain, gotKeys := ParseContextKey(key)
	if gotDomain != domain {
		t.Fatalf("站点域还原失败: got=%q want=%q", gotDomain, domain)
	}
	if len(gotKeys) != 2 || gotKeys[0] != "plug-a\x00ext-1" || gotKeys[1] != "plug-b\x00ext-9" {
		t.Fatalf("候选列表应按字典序还原: %q", gotKeys)
	}
	if rebuild := BuildContextKey(gotDomain, gotKeys); rebuild != key {
		t.Fatalf("拆解产物重组应得同键:\n got=%q\nwant=%q", rebuild, key)
	}

	pid, ext := SplitCandidateFullKey("plug-a\x00ext-1")
	if pid != "plug-a" || ext != "ext-1" {
		t.Fatalf("候选全键拆解失败: pid=%q ext=%q", pid, ext)
	}
	if pid2, ext2 := SplitCandidateFullKey(CandidateFullKey("plug-a", "ext-1")); pid2 != pid || ext2 != ext {
		t.Fatalf("CandidateFullKey 产物应可拆解还原: pid=%q ext=%q", pid2, ext2)
	}
}
