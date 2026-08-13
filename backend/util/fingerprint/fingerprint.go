package fingerprint

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// headFingerprintBytes 头部指纹读取上限：只读文件前 64KB 算哈希，避免对大文件全量哈希
const headFingerprintBytes = 64 * 1024

// Fingerprint 内容指纹：文件的内容身份证，与路径无关。
// 用于变更关联匹配——外部移动/重命名后路径变化，靠指纹判定仍是同一文件。
// Size 为文件字节数，Digest 为 "size:头部SHA256hex" 格式串（落库与匹配口径，不可变）。
type Fingerprint struct {
	Size   int64
	Digest string
}

// Computer 内容指纹计算能力。
// 落盘时算并落库，供文件移动/重命名的内容关联匹配；nil 表示不计算（降级）。
type Computer interface {
	// Fingerprint 计算给定绝对路径文件的内容指纹
	Fingerprint(ctx context.Context, absPath string) (Fingerprint, error)
}

// headComputer 基于 size + 头部 64KB SHA256 的 Computer 实现。
// 只读文件头部，几毫秒完成，相比下载/拷贝耗时可忽略，故同步调用。
// 精度：检测 size 变化或头部字节变化；中间字节改且 size 不变的极端情况漏检。
type headComputer struct{}

// NewHeadComputer 创建头部哈希指纹计算器
func NewHeadComputer() Computer {
	return &headComputer{}
}

// Fingerprint 计算给定绝对路径文件的内容指纹（size + 头部 64KB SHA256）
func (h *headComputer) Fingerprint(ctx context.Context, absPath string) (Fingerprint, error) {
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
