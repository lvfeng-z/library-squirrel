package util

import (
	"crypto/rand"
	"encoding/base64"
	"errors"

	"golang.org/x/crypto/nacl/secretbox"
)

// 加密相关错误
var (
	ErrEncryptFailed = errors.New("加密失败")
	ErrDecryptFailed = errors.New("解密失败")
)

// encryptionKey 32 字节密钥，用于 NaCl secretbox
// TODO 生产环境应从安全密钥存储获取，当前为固定密钥（行为类似系统级密钥）
var encryptionKey [32]byte

func init() {
	copy(encryptionKey[:], []byte("github.com/library-squirrel/wails-secure-key-32byte!"))
}

// Encrypt 加密字符串，返回 base64 编码的密文
func Encrypt(plainText string) (string, error) {
	var nonce [24]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", ErrEncryptFailed
	}

	encrypted := secretbox.Seal(nil, []byte(plainText), &nonce, &encryptionKey)

	// 组合 nonce + encrypted 并进行 base64 编码
	result := make([]byte, 24+len(encrypted))
	copy(result[:24], nonce[:])
	copy(result[24:], encrypted)

	return base64.StdEncoding.EncodeToString(result), nil
}

// Decrypt 解密 base64 编码的密文
func Decrypt(encryptedBase64 string) (string, error) {
	encryptedBytes, err := base64.StdEncoding.DecodeString(encryptedBase64)
	if err != nil {
		return "", ErrDecryptFailed
	}

	if len(encryptedBytes) < 24 {
		return "", ErrDecryptFailed
	}

	var nonce [24]byte
	copy(nonce[:], encryptedBytes[:24])
	encrypted := encryptedBytes[24:]

	decrypted, ok := secretbox.Open(nil, encrypted, &nonce, &encryptionKey)
	if !ok {
		return "", ErrDecryptFailed
	}

	return string(decrypted), nil
}
