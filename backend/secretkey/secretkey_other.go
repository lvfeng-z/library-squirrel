//go:build !windows

package secretkey

import (
	"fmt"
	"os"
)

// platformLoadKey 非 Windows 密钥装载：读取明文密钥文件并校验 32 字节长度。
// 文件不存在时返回包裹 fs.ErrNotExist 的错误
func platformLoadKey(keyPath string) ([keySize]byte, error) {
	var key [keySize]byte
	data, err := os.ReadFile(keyPath)
	if err != nil {
		return key, err
	}
	if len(data) != keySize {
		return key, fmt.Errorf("密钥文件内容非法: 期望 %d 字节, 实得 %d 字节", keySize, len(data))
	}
	copy(key[:], data)
	return key, nil
}

// platformSaveKey 非 Windows 密钥持久化：明文独占创建写入密钥文件（0600 限
// 属主读写）。文件已存在时返回包裹 fs.ErrExist 的错误
func platformSaveKey(keyPath string, key [keySize]byte) error {
	return writeKeyFileExclusive(keyPath, key[:])
}
