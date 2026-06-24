package util

import (
	"strings"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	tests := []string{
		"hello world",
		"",
		"中文测试 🎉",
		strings.Repeat("a", 1000),
		"accessToken: abc123xyz",
	}
	for _, plain := range tests {
		encrypted, err := Encrypt(plain)
		if err != nil {
			t.Fatalf("Encrypt(%q) 错误: %v", plain, err)
		}
		decrypted, err := Decrypt(encrypted)
		if err != nil {
			t.Fatalf("Decrypt 错误: %v", err)
		}
		if decrypted != plain {
			t.Errorf("往返不一致: got %q, want %q", decrypted, plain)
		}
	}
}

func TestEncryptProducesDifferentCiphertext(t *testing.T) {
	plain := "same-secret"
	a, _ := Encrypt(plain)
	b, _ := Encrypt(plain)
	if a == b {
		t.Error("同一明文两次加密应产生不同密文（随机 nonce）")
	}
}

func TestDecryptInvalidBase64(t *testing.T) {
	_, err := Decrypt("!!!not-base64!!!")
	if err != ErrDecryptFailed {
		t.Errorf("非法 base64 应返回 ErrDecryptFailed, got %v", err)
	}
}

func TestDecryptTooShort(t *testing.T) {
	// 合法 base64 但解码后不足 24 字节（nonce 长度）
	_, err := Decrypt("AQID")
	if err != ErrDecryptFailed {
		t.Errorf("过短密文应返回 ErrDecryptFailed, got %v", err)
	}
}
