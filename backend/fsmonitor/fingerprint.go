package fsmonitor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// 头部指纹读取上限：只读文件前 64KB 算哈希，避免对大文件全量哈希
const headFingerprintBytes = 64 * 1024

// headFingerprinter 基于 size + 头部 64KB SHA256 的 ContentFingerprinter 实现。
// 只读文件头部，几毫秒完成，相比下载/拷贝耗时可忽略，故同步调用。
// 精度：检测 size 变化或头部字节变化；中间字节改且 size 不变的极端情况漏检。
type headFingerprinter struct{}

// NewHeadFingerprinter 创建头部哈希指纹计算器
func NewHeadFingerprinter() ContentFingerprinter {
	return &headFingerprinter{}
}

// Fingerprint 计算给定绝对路径文件的内容指纹（size + 头部 64KB SHA256）
func (h *headFingerprinter) Fingerprint(ctx context.Context, absPath string) (Fingerprint, error) {
	info, err := os.Stat(absPath)
	if err != nil {
		return Fingerprint{}, fmt.Errorf("获取文件信息失败: %w", err)
	}
	size := info.Size()

	file, err := os.Open(absPath)
	if err != nil {
		return Fingerprint{}, fmt.Errorf("打开文件失败: %w", err)
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.CopyN(hasher, file, headFingerprintBytes); err != nil && err != io.EOF {
		return Fingerprint{}, fmt.Errorf("读取文件头部失败: %w", err)
	}
	digest := fmt.Sprintf("%d:%s", size, hex.EncodeToString(hasher.Sum(nil)))
	return Fingerprint{Size: size, Digest: digest}, nil
}

// ComputeFingerprint 便捷函数：计算绝对路径指纹，返回可直接落库的字符串（失败返回空串 + 错误）
func ComputeFingerprint(absPath string) (string, error) {
	fp, err := (&headFingerprinter{}).Fingerprint(context.Background(), absPath)
	if err != nil {
		return "", err
	}
	return fp.Digest, nil
}
