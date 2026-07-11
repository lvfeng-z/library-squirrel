package merge

import (
	"os"
	"fmt"
	"time"
	"bytes"
	"errors"
	"context"
	"os/exec"
)

// 错误定义
var (
	// ErrFFmpegNotFound 系统 PATH 上找不到 ffmpeg；合并是用户主动触发的能力，需明确提示安装。
	ErrFFmpegNotFound = errors.New("未在系统 PATH 中找到 ffmpeg，请先安装 ffmpeg 后重试")
	// ErrMergeOutputInvalid ffmpeg 退出码为 0 但产物缺失或为空，通常意味着容器写入异常。
	ErrMergeOutputInvalid = errors.New("合并完成但产物异常")
)

// defaultMergeTimeout 单次合并超时：remux 通常秒级，此处为大文件兜底。
const defaultMergeTimeout = 10 * time.Minute

// stderrTailBytes 失败诊断时从 stderr 截取的末尾最大字节数（ffmpeg 错误总结通常位于末尾）。
const stderrTailBytes = 512

// FFmpegMuxer 封装对 ffmpeg 的调用，把独立的视频轨与音频轨无重编码合并为单个媒体文件。
// 它是纯工具层：不访问数据库、不注册 IPC handler，由上层 merge 业务注入使用。
type FFmpegMuxer struct {
	binaryPath string        // ffmpeg 可执行文件路径（构造时由 LookPath 确定）
	timeout    time.Duration // 单次合并超时
}

// NewFFmpegMuxer 在系统 PATH 上查找 ffmpeg；找不到返回 ErrFFmpegNotFound。
func NewFFmpegMuxer() (*FFmpegMuxer, error) {
	p, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, ErrFFmpegNotFound
	}
	return &FFmpegMuxer{
		binaryPath: p,
		timeout:    defaultMergeTimeout,
	}, nil
}

// WithTimeout 覆盖单次合并超时，返回 muxer 自身以便链式配置。
func (m *FFmpegMuxer) WithTimeout(d time.Duration) *FFmpegMuxer {
	m.timeout = d
	return m
}

// MergeRemux 对两个完整的本地媒体文件做无重编码合并（remux），
// 产物写到 outPath（调用方负责 outPath 唯一性与父目录存在）。
// 视频轨或音频轨无效、ffmpeg 失败、或合并被中断（超时/取消），均返回携带诊断信息的错误；
// 任何失败路径都会清理 outPath 处的残留产物。
func (m *FFmpegMuxer) MergeRemux(ctx context.Context, videoPath, audioPath, outPath string) error {
	// 合并超时与调用方 ctx 共同决定子进程生命周期：先到期者触发中断
	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, m.binaryPath,
		"-i", videoPath,
		"-i", audioPath,
		"-c", "copy",
		"-movflags", "+faststart",
		"-y", outPath,
	)

	// ffmpeg 的进度与错误信息均写 stderr，捕获末尾用于失败诊断
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	if err := cmd.Run(); err != nil {
		cleanupPartialOutput(outPath)
		// ctx 到期或被调用方取消时归为"被中断"，区别于 ffmpeg 自身失败
		if ctx.Err() != nil {
			return fmt.Errorf("合并被中断：%w", ctx.Err())
		}
		return fmt.Errorf("合并失败：%s", ffmpegFailureDetail(err, stderrBuf.Bytes()))
	}

	// 产物校验：remux 成功应得到非空文件；ffmpeg 退出码为 0 却无产物视作异常
	info, err := os.Stat(outPath)
	if err != nil || info.Size() == 0 {
		cleanupPartialOutput(outPath)
		return fmt.Errorf("%w：%s", ErrMergeOutputInvalid, outPath)
	}
	return nil
}

// ffmpegFailureDetail 从 stderr 末尾提取可读片段作为失败原因；stderr 为空时回退到运行错误本身。
func ffmpegFailureDetail(runErr error, stderr []byte) string {
	tail := bytes.TrimSpace(tailBytes(stderr, stderrTailBytes))
	if len(tail) == 0 {
		return runErr.Error()
	}
	return string(tail)
}

// tailBytes 返回 b 的末尾最多 n 字节。
func tailBytes(b []byte, n int) []byte {
	if len(b) <= n {
		return b
	}
	return b[len(b)-n:]
}

// cleanupPartialOutput 删除合并失败/中断遗留的半成品产物，避免占位文件干扰后续调用与外部打开。
func cleanupPartialOutput(outPath string) {
	if outPath == "" {
		return
	}
	// 清理失败不影响主错误；产物可能本就不存在，忽略 NotExist
	_ = os.Remove(outPath)
}
