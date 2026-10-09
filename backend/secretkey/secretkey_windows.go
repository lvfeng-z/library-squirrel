//go:build windows

package secretkey

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// platformLoadKey Windows 密钥装载：读取密钥文件，经 DPAPI（当前用户作用域）
// 解封后校验 32 字节长度。文件不存在时返回包裹 fs.ErrNotExist 的错误
func platformLoadKey(keyPath string) ([keySize]byte, error) {
	var key [keySize]byte
	blob, err := os.ReadFile(keyPath)
	if err != nil {
		return key, err
	}
	if len(blob) == 0 {
		return key, fmt.Errorf("密钥文件为空: %s", keyPath)
	}
	plain, err := cryptUnprotect(blob)
	if err != nil {
		return key, fmt.Errorf("DPAPI 解封密钥失败: %w", err)
	}
	if len(plain) != keySize {
		return key, fmt.Errorf("密钥长度非法: 期望 %d 字节, 实得 %d 字节", keySize, len(plain))
	}
	copy(key[:], plain)
	return key, nil
}

// platformSaveKey Windows 密钥持久化：经 DPAPI（当前用户作用域）封装后独占
// 创建写入密钥文件。文件已存在时返回包裹 fs.ErrExist 的错误
func platformSaveKey(keyPath string, key [keySize]byte) error {
	blob, err := cryptProtect(key[:])
	if err != nil {
		return fmt.Errorf("DPAPI 封装密钥失败: %w", err)
	}
	return writeKeyFileExclusive(keyPath, blob)
}

// cryptProtect 以 DPAPI 封装数据：CRYPTPROTECT_UI_FORBIDDEN 静默执行（不弹
// 系统保护提示窗），输出密文仅当前 Windows 用户可解
func cryptProtect(data []byte) ([]byte, error) {
	in := &windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
	var out windows.DataBlob
	if err := windows.CryptProtectData(in, nil, nil, 0, nil,
		windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, err
	}
	return dpapiOutput(out), nil
}

// cryptUnprotect 以 DPAPI 解封 cryptProtect 的输出
func cryptUnprotect(data []byte) ([]byte, error) {
	in := &windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(in, nil, nil, 0, nil,
		windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, err
	}
	return dpapiOutput(out), nil
}

// dpapiOutput 拷贝 DPAPI 输出缓冲并释放其内存：DataBlob.Data 指向系统堆
// （LocalAlloc 分配），调用方不可直接持有，须先拷贝再 LocalFree 归还
func dpapiOutput(out windows.DataBlob) []byte {
	defer func() {
		_, _ = windows.LocalFree(windows.Handle(uintptr(unsafe.Pointer(out.Data))))
	}()
	buf := make([]byte, int(out.Size))
	copy(buf, unsafe.Slice(out.Data, int(out.Size)))
	return buf
}
