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

func TestNormalizeRelayAddress(t *testing.T) {
	// 地址语义判定表逐行覆盖 + 前缀覆盖/网段边界/书写形态
	cases := []struct {
		name     string
		in       string
		wantDial string
		wantTLS  bool
		wantHost string
	}{
		{"公网字面量缺省端口", "relay.example.com", "relay.example.com:443", true, "relay.example.com"},
		{"公网字面量显式端口", "relay.example.com:9000", "relay.example.com:9000", true, "relay.example.com:9000"},
		{"公网裸 IP 字面量不豁免", "1.2.3.4", "1.2.3.4:443", true, "1.2.3.4"},
		{"https 前缀缺省端口", "https://relay.example.com", "relay.example.com:443", true, "relay.example.com"},
		{"https 前缀显式端口", "https://relay.example.com:8443", "relay.example.com:8443", true, "relay.example.com:8443"},
		{"https 前缀覆盖回环豁免", "https://127.0.0.1:9527", "127.0.0.1:9527", true, "127.0.0.1:9527"},
		{"https 前缀覆盖 localhost 豁免", "https://localhost", "localhost:443", true, "localhost"},
		{"http 逃生口缺省端口", "http://relay.example.com", "relay.example.com:9527", false, "relay.example.com"},
		{"http 逃生口显式端口", "http://relay.example.com:9000", "relay.example.com:9000", false, "relay.example.com:9000"},
		{"回环 IPv4 缺省端口", "127.0.0.1", "127.0.0.1:9527", false, "127.0.0.1"},
		{"回环段内非 .1 地址", "127.200.0.9", "127.200.0.9:9527", false, "127.200.0.9"},
		{"localhost 字面量", "localhost", "localhost:9527", false, "localhost"},
		{"回环 IPv6 字面量", "::1", "[::1]:9527", false, "[::1]"},
		{"回环 IPv6 方括号带端口", "[::1]:9527", "[::1]:9527", false, "[::1]:9527"},
		{"私网 10/8", "10.0.0.5", "10.0.0.5:9527", false, "10.0.0.5"},
		{"私网 172.16/12 段内上界", "172.31.255.1", "172.31.255.1:9527", false, "172.31.255.1"},
		{"172.32 在 172.16/12 之外走 TLS", "172.32.0.1", "172.32.0.1:443", true, "172.32.0.1"},
		{"私网 192.168/16", "192.168.1.100", "192.168.1.100:9527", false, "192.168.1.100"},
		{"书写空白与尾斜杠容忍", "  127.0.0.1:9527/ ", "127.0.0.1:9527", false, "127.0.0.1:9527"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ep, host, err := normalizeRelayAddress(c.in)
			if err != nil {
				t.Fatalf("地址 %q 解析失败: %v", c.in, err)
			}
			if ep.Addr != c.wantDial || ep.TLS != c.wantTLS || host != c.wantHost {
				t.Fatalf("地址 %q = (addr %q, tls %v, host %q), want (addr %q, tls %v, host %q)",
					c.in, ep.Addr, ep.TLS, host, c.wantDial, c.wantTLS, c.wantHost)
			}
		})
	}
	for _, bad := range []string{
		"tcp://relay.example.com", // tcp:// 前缀报错：明文逃生口统一为 http:// 前缀
		"a b",
		"  ",
		"https://",
	} {
		if _, _, err := normalizeRelayAddress(bad); err == nil {
			t.Fatalf("地址 %q 应被拒绝", bad)
		}
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
