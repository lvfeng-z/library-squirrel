package share

// E2E 加密与工具函数单元测试（决策14 正确性硬验收的单元层）

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2ECipher_RoundtripAndWrongKey(t *testing.T) {
	key, err := GenerateShareKey()
	if err != nil {
		t.Fatalf("生成密钥失败: %v", err)
	}
	if len(key) != shareKeyLen {
		t.Fatalf("密钥长度 = %d, want %d", len(key), shareKeyLen)
	}
	cip, err := newE2ECipher(key)
	if err != nil {
		t.Fatalf("构造 cipher 失败: %v", err)
	}
	plaintext := []byte("测试明文内容 Test plaintext 0123456789 \x00\x01\xff")
	record, err := cip.sealRecord(plaintext)
	if err != nil {
		t.Fatalf("加密失败: %v", err)
	}
	// ① 线路字节 ≠ 明文
	if bytes.Contains(record, plaintext) {
		t.Fatal("密文记录包含明文片段")
	}
	if bytes.Equal(record, plaintext) {
		t.Fatal("密文与明文相同")
	}
	// ② 同密钥解密逐字节还原
	got, err := cip.openRecord(record)
	if err != nil {
		t.Fatalf("解密失败: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("解密结果与明文不一致: got %q want %q", got, plaintext)
	}
	// 异密钥解密必须失败（认证标签校验）
	wrongKey := make([]byte, shareKeyLen)
	copy(wrongKey, key)
	wrongKey[0] ^= 0xff
	wrongCip, _ := newE2ECipher(wrongKey)
	if _, err := wrongCip.openRecord(record); err == nil {
		t.Fatal("错误密钥解密不应成功")
	}
	// 记录截断/短记录必须失败
	if _, err := cip.openRecord(record[:10]); err == nil {
		t.Fatal("短记录解密不应成功")
	}
}

func TestBuildShareLink(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	link := BuildShareLink("relay.example.com", "stub-token-abc", key)
	wantPrefix := "https://relay.example.com/s/stub-token-abc#k="
	if !strings.HasPrefix(link, wantPrefix) {
		t.Fatalf("链接形态不符: %s", link)
	}
	frag := strings.TrimPrefix(link, wantPrefix)
	got, err := base64.RawURLEncoding.DecodeString(frag)
	if err != nil {
		t.Fatalf("fragment 非法 base64url: %v", err)
	}
	if !bytes.Equal(got, key) {
		t.Fatal("fragment 未携带原始密钥")
	}
	// 密钥只出现在 fragment：链接的请求部分（# 前）不含密钥及其编码
	reqPart := link[:strings.Index(link, "#")]
	if strings.Contains(reqPart, frag) {
		t.Fatal("密钥编码泄露到链接请求部分（fragment 之外）")
	}
}

func TestPasswordHashHex(t *testing.T) {
	// 已知向量：sha256("abc")
	want := sha256.Sum256([]byte("abc"))
	if got := PasswordHashHex("abc"); got != byteHex(want[:]) {
		t.Fatalf("密码摘要不符: got %s", got)
	}
	if len(PasswordHashHex("x")) != 64 {
		t.Fatal("摘要应为 64 位 hex")
	}
}

func byteHex(b []byte) string {
	const hexDigits = "0123456789abcdef"
	out := make([]byte, 0, len(b)*2)
	for _, v := range b {
		out = append(out, hexDigits[v>>4], hexDigits[v&0x0f])
	}
	return string(out)
}

func TestSanitizeMetaText(t *testing.T) {
	if got := SanitizeMetaText("a\r b\n c\t d\x07", 100); got != "a b c d" {
		t.Fatalf("控制字符未剔除: %q", got)
	}
	long := strings.Repeat("测", 300)
	if got := SanitizeMetaText(long, 200); len([]rune(got)) != 200 {
		t.Fatalf("rune 截断失败: %d", len([]rune(got)))
	}
}

func TestNormalizeRelayAddress(t *testing.T) {
	cases := []struct {
		in, dial, host string
	}{
		{"relay.example.com", "relay.example.com:9527", "relay.example.com"},
		{"https://relay.example.com", "relay.example.com:9527", "relay.example.com"},
		{"relay.example.com:9000", "relay.example.com:9000", "relay.example.com:9000"},
		{" 127.0.0.1:9527 ", "127.0.0.1:9527", "127.0.0.1:9527"},
	}
	for _, c := range cases {
		dial, host, err := normalizeRelayAddress(c.in)
		if err != nil {
			t.Fatalf("地址 %q 解析失败: %v", c.in, err)
		}
		if dial != c.dial || host != c.host {
			t.Fatalf("地址 %q 解析 = (%q,%q), want (%q,%q)", c.in, dial, host, c.dial, c.host)
		}
	}
	if _, _, err := normalizeRelayAddress("a b"); err == nil {
		t.Fatal("含空格地址应被拒绝")
	}
}

func TestMapExpireSeconds(t *testing.T) {
	if v := mapExpireSeconds(-1); v != nil {
		t.Fatal("-1 应映射为 nil（中继默认）")
	}
	if v := mapExpireSeconds(0); v == nil || *v != 0 {
		t.Fatal("0 应映射为 &0（无限期）")
	}
	if v := mapExpireSeconds(3600); v == nil || *v != 3600 {
		t.Fatal(">0 应映射为自定义秒数")
	}
}

func TestLoadOrCreateInstanceID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "share-instance-id")
	id1, err := LoadOrCreateInstanceID(path)
	if err != nil {
		t.Fatalf("创建实例 ID 失败: %v", err)
	}
	if !instanceIDPattern.MatchString(id1) {
		t.Fatalf("实例 ID 形态非法: %q", id1)
	}
	// 重读返回同一 ID（设备绑定持久性）
	id2, err := LoadOrCreateInstanceID(path)
	if err != nil || id1 != id2 {
		t.Fatalf("重读实例 ID 不一致: %q vs %q (err=%v)", id1, id2, err)
	}
	// 文件损坏（非法字符）时重建
	if err := os.WriteFile(path, []byte("非法ID!!"), 0o644); err != nil {
		t.Fatal(err)
	}
	id3, err := LoadOrCreateInstanceID(path)
	if err != nil || id3 == id1 {
		t.Fatalf("损坏文件应重建新实例 ID: %q (err=%v)", id3, err)
	}
}
