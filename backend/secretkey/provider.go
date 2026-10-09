// Package secretkey 提供应用加密密钥管理纯能力：32 字节随机钥的装载/生成/持久化
// 与 v2 密文加解密。密钥持久化经平台保护层（Windows 以 DPAPI 按当前用户封装、
// 其余平台明文文件 0600），密钥文件路径由构造方传入，本包不感知应用根目录与
// 业务实体。旧版固定钥密文（无前缀裸 base64）经 legacy 钥回退解密，供存量密文
// 迁移读取；新加密一律产出 v2 格式。
package secretkey

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"golang.org/x/crypto/nacl/secretbox"

	"github.com/library-squirrel/backend/base/logger"
)

// 加密相关错误
var (
	ErrEncryptFailed = errors.New("加密失败")
	ErrDecryptFailed = errors.New("解密失败")
)

const (
	// keySize NaCl secretbox 密钥字节长度
	keySize = 32
	// nonceSize NaCl secretbox nonce 字节长度
	nonceSize = 24
	// v2Prefix v2 密文格式前缀，后接 base64(nonce + secretbox 密文)；旧格式为
	// 无前缀裸 base64，二者以前缀有无判别
	v2Prefix = "v2:"
	// keyFilePerm 密钥文件权限：仅属主可读写（Windows 不识别 POSIX 权限位，
	// 密钥由 DPAPI 当前用户封装保护）
	keyFilePerm = 0600
)

// loadKey/saveKey 密钥装载/持久化平台钩子：密钥从哪来、往哪存。Windows 实现
// 为密钥文件 + DPAPI 封装，其余平台为明文文件；单测经本组钩子注入桩，不触
// 真实平台保护层
var (
	loadKey = platformLoadKey
	saveKey = platformSaveKey
)

// Provider 应用加密提供方：持有本会话密钥，加密一律产出 v2 密文；解密按前缀
// 分流——v2 密文用当前密钥，旧格式密文用 legacy 固定钥回退
type Provider struct {
	key [keySize]byte
}

// NewProvider 装载或生成密钥并构造 Provider，keyPath 为密钥文件落点（由调用方
// 决定）。密钥文件缺失时生成 32 字节随机钥落盘；装载失败（文件损坏、DPAPI
// 解封失败）不阻断——记 Warn 日志后重新生成新钥覆盖落盘。落盘为独占创建：
// 双实例并发启动时仅一方生成写入，落盘竞争失败方转而读取既有文件、采用同一
// 把密钥，避免两把密钥先后落盘互相覆盖。
func NewProvider(keyPath string) (*Provider, error) {
	key, err := loadKey(keyPath)
	if err == nil {
		return &Provider{key: key}, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		logger.Log.Warnw("[secretkey] 密钥文件不可用，重新生成新钥（既有旧密文将无法解密）",
			"path", keyPath, "error", err)
		// 先移除不可用的密钥文件，使下方独占创建可以落新钥
		if rmErr := os.Remove(keyPath); rmErr != nil && !errors.Is(rmErr, fs.ErrNotExist) {
			return nil, fmt.Errorf("移除不可用密钥文件失败: %w", rmErr)
		}
	}
	key, err = generateKey()
	if err != nil {
		return nil, err
	}
	if saveErr := saveKey(keyPath, key); saveErr != nil {
		// 落盘撞上并发对端刚创建的文件：弃用本进程刚生成的钥，改用既有密钥
		if errors.Is(saveErr, fs.ErrExist) {
			existing, loadErr := loadKey(keyPath)
			if loadErr != nil {
				return nil, fmt.Errorf("读取并发落盘的既有密钥失败: %w", loadErr)
			}
			return &Provider{key: existing}, nil
		}
		return nil, fmt.Errorf("密钥落盘失败: %w", saveErr)
	}
	return &Provider{key: key}, nil
}

// Encrypt 以当前密钥加密字符串，返回 v2: + base64(nonce + secretbox 密文)
func (p *Provider) Encrypt(plainText string) (string, error) {
	var nonce [nonceSize]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", ErrEncryptFailed
	}

	sealed := secretbox.Seal(nil, []byte(plainText), &nonce, &p.key)

	combined := make([]byte, nonceSize, nonceSize+len(sealed))
	copy(combined, nonce[:])
	combined = append(combined, sealed...)

	return v2Prefix + base64.StdEncoding.EncodeToString(combined), nil
}

// Decrypt 解密密文：带 v2: 前缀的用当前密钥；无前缀裸 base64 为旧格式密文，
// 用 legacy 固定钥回退解密（迁移期读存量数据）。解不开返回 ErrDecryptFailed
func (p *Provider) Decrypt(cipherText string) (string, error) {
	if encoded, ok := strings.CutPrefix(cipherText, v2Prefix); ok {
		return openSealed(encoded, &p.key)
	}
	legacy := legacyKey()
	return openSealed(cipherText, &legacy)
}

// generateKey 生成 32 字节随机钥（crypto/rand）
func generateKey() ([keySize]byte, error) {
	var key [keySize]byte
	if _, err := rand.Read(key[:]); err != nil {
		return key, fmt.Errorf("生成随机密钥失败: %w", err)
	}
	return key, nil
}

// openSealed 解 base64(nonce + secretbox 密文) 为明文
func openSealed(encryptedBase64 string, key *[keySize]byte) (string, error) {
	encryptedBytes, err := base64.StdEncoding.DecodeString(encryptedBase64)
	if err != nil {
		return "", ErrDecryptFailed
	}

	if len(encryptedBytes) < nonceSize {
		return "", ErrDecryptFailed
	}

	var nonce [nonceSize]byte
	copy(nonce[:], encryptedBytes[:nonceSize])

	decrypted, ok := secretbox.Open(nil, encryptedBytes[nonceSize:], &nonce, key)
	if !ok {
		return "", ErrDecryptFailed
	}

	return string(decrypted), nil
}

// writeKeyFileExclusive 独占创建写入密钥文件：文件已存在时返回包裹 fs.ErrExist
// 的错误，由调用方转入读取既有密钥分支
func writeKeyFileExclusive(keyPath string, data []byte) error {
	f, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, keyFilePerm)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
