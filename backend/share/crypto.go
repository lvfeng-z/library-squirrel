package share

// 分享端到端加密（决策14：正确性为硬验收项）。
//
// 密钥形态：分享方生成 32 字节随机密钥（AES-256），**不经中继**——只放分享链接的 URL
// fragment（`https://{relay}/s/{token}#k=<base64url>`）；fragment 不会出现在发往中继的
// 任何 HTTP/线协议请求中（浏览器与客户端均不发送 fragment）。
//
// 记录形态（流内应用层）：每条记录 = `nonce(12) || AES-256-GCM(密文||tag(16))`，
// 记录与 DATA 帧一对一（一帧一完整记录）；随机 96-bit nonce，单会话记录数远低于
// NIST 随机 nonce 上限（2^32）。

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
)

// shareKeyLen E2E 密钥长度（AES-256）
const shareKeyLen = 32

// nonceLen AES-GCM 标准 nonce 长度
const nonceLen = 12

// GenerateShareKey 生成随机会话密钥（crypto/rand 32 字节）
func GenerateShareKey() ([]byte, error) {
	key := make([]byte, shareKeyLen)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("生成分享密钥失败: %w", err)
	}
	return key, nil
}

// e2eCipher 流内记录加解密器（AES-256-GCM）
type e2eCipher struct {
	aead cipher.AEAD
}

// newE2ECipher 以会话密钥构造记录加解密器
func newE2ECipher(key []byte) (*e2eCipher, error) {
	if len(key) != shareKeyLen {
		return nil, fmt.Errorf("分享密钥长度非法: %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &e2eCipher{aead: aead}, nil
}

// sealRecord 加密一条明文记录：随机 nonce || GCM(明文)。
// 返回值即一个完整 DATA 帧负载，可原样发往中继（中继只见密文）。
func (c *e2eCipher) sealRecord(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, nonceLen)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("生成 nonce 失败: %w", err)
	}
	return c.aead.Seal(nonce, nonce, plaintext, nil), nil
}

// openRecord 解密一条记录（认证失败即错误——密文被篡改/密钥不符均在此暴露）
func (c *e2eCipher) openRecord(record []byte) ([]byte, error) {
	if len(record) <= nonceLen {
		return nil, fmt.Errorf("记录过短: %d", len(record))
	}
	return c.aead.Open(nil, record[:nonceLen], record[nonceLen:], nil)
}

// BuildShareLink 构造分享链接：`https://{relayHost}/s/{token}#k={base64url(密钥)}`。
// 密钥只进 fragment——URL fragment 不随请求发往中继。
func BuildShareLink(relayHost, token string, key []byte) string {
	return fmt.Sprintf("https://%s/s/%s#k=%s", relayHost, token, base64.RawURLEncoding.EncodeToString(key))
}

// PasswordHashHex 访问密码摘要：hex(sha256(明文))——明文密码永不在线路上出现（PROTOCOL.md §3）
func PasswordHashHex(password string) string {
	sum := sha256.Sum256([]byte(password))
	return hex.EncodeToString(sum[:])
}

// SanitizeMetaText 净化落地页元数据文本：剔除控制字符（含 \r\n\t）并按 rune 截断到上限
// （中继对 title/source 有长度与控制字符校验，客户端先行净化避免注册被 malformed 拒绝）
func SanitizeMetaText(s string, maxRunes int) string {
	var b strings.Builder
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	runes := []rune(b.String())
	if len(runes) > maxRunes {
		runes = runes[:maxRunes]
	}
	return string(runes)
}
