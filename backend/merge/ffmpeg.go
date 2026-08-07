package merge

import (
	"os"
	"fmt"
	"time"
	"bytes"
	"errors"
	"context"
	"strconv"
	"strings"
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
// onProgress 在合并过程中被回调上报百分比（0~99，nil 表示不上报），合并成功后回调 100；
// 无总时长可解析时不回调（调用方维持不定态展示）。视频轨或音频轨无效、ffmpeg 失败、或合并被中断
//（超时/取消），均返回携带诊断信息的错误；任何失败路径都会清理 outPath 处的残留产物。
func (m *FFmpegMuxer) MergeRemux(ctx context.Context, videoPath, audioPath, outPath string, onProgress func(percent int)) error {
	// 合并超时与调用方 ctx 共同决定子进程生命周期：先到期者触发中断
	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, m.binaryPath,
		"-nostats",            // 关闭默认 stats 行，改用 -progress 的结构化进度块
		"-progress", "pipe:2", // 进度块(key=value)写 stderr，供解析百分比
		"-i", videoPath,
		"-i", audioPath,
		"-c", "copy",
		"-movflags", "+faststart",
		"-y", outPath,
	)

	// stderr 双用：全量累积到 sink.buf（失败诊断取末尾）+ 按行解析进度。
	// sink.Write 由 os/exec 内部 copy goroutine 驱动（与下方 cmd.Run 并发）；失败诊断的读取发生在
	// cmd.Run 返回后（Run 会 join 内部 copy goroutine），故 buf 无并发读写。onProgress 仅在解析进度时回调。
	sink := &stderrSink{}
	if onProgress != nil {
		sink.onLine = (&ffmpegProgressParser{onPercent: onProgress}).parse
	}
	cmd.Stderr = sink

	if err := cmd.Run(); err != nil {
		cleanupPartialOutput(outPath)
		// ctx 到期或被调用方取消时归为"被中断"，区别于 ffmpeg 自身失败
		if ctx.Err() != nil {
			return fmt.Errorf("合并被中断：%w", ctx.Err())
		}
		return fmt.Errorf("合并失败：%s", ffmpegFailureDetail(err, sink.buf.Bytes()))
	}

	// 产物校验：remux 成功应得到非空文件；ffmpeg 退出码为 0 却无产物视作异常
	info, err := os.Stat(outPath)
	if err != nil || info.Size() == 0 {
		cleanupPartialOutput(outPath)
		return fmt.Errorf("%w：%s", ErrMergeOutputInvalid, outPath)
	}
	if onProgress != nil {
		onProgress(100)
	}
	return nil
}

// stderrSink 收集 ffmpeg stderr：全量累积到 buf（失败诊断取末尾）+ 切完整行回调 onLine（进度解析）。
// 实现 io.Writer，由 os/exec 内部 copy goroutine 驱动（与 cmd.Run 所在 goroutine 并发）。
type stderrSink struct {
	buf     bytes.Buffer
	onLine  func(line string)
	partial []byte // 跨 Write 的未凑成行尾部
}

// Write 追加字节、累积全量，并把凑成的完整行（去 \r）交给 onLine；未成行的尾部留待下次。
func (s *stderrSink) Write(p []byte) (int, error) {
	s.buf.Write(p)
	if s.onLine == nil {
		return len(p), nil
	}
	s.partial = append(s.partial, p...)
	for {
		nl := bytes.IndexByte(s.partial, '\n')
		if nl < 0 {
			break
		}
		line := strings.TrimRight(string(s.partial[:nl]), "\r")
		s.partial = s.partial[nl+1:]
		s.onLine(line)
	}
	return len(p), nil
}

// ffmpegProgressParser 解析 ffmpeg stderr 行计算合并进度百分比。
// totalSec 取自 banner 的 Duration（各输入取最大值；remux 无 -shortest，输出≈最长输入）；
// out_time= 行给出已完成时长，percent = clamp(outSec/totalSec*100, 0, 99)。
type ffmpegProgressParser struct {
	totalSec  float64
	onPercent func(int)
}

// parse 处理一行 stderr：banner 提取总时长、进度块提取已完成时长算百分比。
func (p *ffmpegProgressParser) parse(line string) {
	if dur, ok := parseDurationLine(line); ok {
		if dur > p.totalSec {
			p.totalSec = dur
		}
		return
	}
	if p.totalSec <= 0 {
		return // 尚未解析到总时长，无法算百分比（调用方维持不定态）
	}
	if secs, ok := parseOutTimeLine(line); ok {
		pct := int(secs / p.totalSec * 100)
		if pct < 0 {
			pct = 0
		} else if pct > 99 {
			pct = 99
		}
		p.onPercent(pct)
	}
}

// parseDurationLine 从 ffmpeg banner 行（形如 "  Duration: 00:01:23.45, ..."）提取时长秒数。
func parseDurationLine(line string) (float64, bool) {
	const prefix = "Duration:"
	idx := strings.Index(line, prefix)
	if idx < 0 {
		return 0, false
	}
	rest := strings.TrimSpace(line[idx+len(prefix):])
	if comma := strings.IndexByte(rest, ','); comma >= 0 {
		rest = rest[:comma]
	}
	return parseFFmpegTime(strings.TrimSpace(rest))
}

// parseOutTimeLine 从进度块行（形如 "out_time=00:00:12.345678"）提取已完成时长秒数。
func parseOutTimeLine(line string) (float64, bool) {
	const prefix = "out_time="
	if !strings.HasPrefix(line, prefix) {
		return 0, false
	}
	return parseFFmpegTime(strings.TrimSpace(line[len(prefix):]))
}

// parseFFmpegTime 解析 ffmpeg 时长格式 HH:MM:SS[.ffffff] 为秒数。
func parseFFmpegTime(s string) (float64, bool) {
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0, false
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	sec, err3 := strconv.ParseFloat(parts[2], 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, false
	}
	return float64(h)*3600 + float64(m)*60 + sec, true
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
