package merge

import (
	"os"
	"time"
	"context"
	"os/exec"
	"strings"
	"testing"
	"path/filepath"
)

// skipIfNoFFmpeg 返回 ffmpeg 路径；系统未安装时跳过当前测试。
func skipIfNoFFmpeg(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg 未安装，跳过合并测试")
	}
	return p
}

// mustNewMuxer 构造 FFmpegMuxer，构造失败即终止当前测试。
func mustNewMuxer(t *testing.T) *FFmpegMuxer {
	t.Helper()
	m, err := NewFFmpegMuxer()
	if err != nil {
		t.Fatalf("NewFFmpegMuxer 失败：%v", err)
	}
	return m
}

// generateTestMedia 用 ffmpeg lavfi 在临时目录生成 1 秒测试视频与音频。
// 产物不进仓库，随 t.TempDir() 自动清理；选用 mpeg4/aac 编码器以避开 GPL 的 libx264 依赖。
// 生成失败（如发行版缺编码器）时跳过而非失败，保证测试可移植。
func generateTestMedia(t *testing.T, ffmpegBin string) (video, audio string) {
	t.Helper()
	dir := t.TempDir()
	video = filepath.Join(dir, "video.mp4")
	audio = filepath.Join(dir, "audio.m4a")

	if out, err := exec.Command(ffmpegBin,
		"-f", "lavfi", "-i", "testsrc=duration=1:size=160x120:rate=10",
		"-c:v", "mpeg4", "-y", video,
	).CombinedOutput(); err != nil {
		t.Skipf("无法用 ffmpeg 生成测试视频，跳过：%v\n%s", err, out)
	}
	if out, err := exec.Command(ffmpegBin,
		"-f", "lavfi", "-i", "anullsrc=channel_layout=stereo:sample_rate=44100",
		"-t", "1", "-c:a", "aac", "-y", audio,
	).CombinedOutput(); err != nil {
		t.Skipf("无法用 ffmpeg 生成测试音频，跳过：%v\n%s", err, out)
	}
	return video, audio
}

// TestMergeRemux_Success 验证有效视频轨+音频轨能合并出非空产物。
func TestMergeRemux_Success(t *testing.T) {
	bin := skipIfNoFFmpeg(t)
	muxer := mustNewMuxer(t)

	video, audio := generateTestMedia(t, bin)
	out := filepath.Join(t.TempDir(), "merged.mp4")

	if err := muxer.MergeRemux(context.Background(), video, audio, out); err != nil {
		t.Fatalf("合并失败：%v", err)
	}
	info, err := os.Stat(out)
	if err != nil {
		t.Fatalf("产物不可读：%v", err)
	}
	if info.Size() == 0 {
		t.Fatal("产物为空")
	}
}

// TestMergeRemux_InvalidInput 验证非媒体输入时返回携带 stderr 的失败错误，并清理残留产物。
func TestMergeRemux_InvalidInput(t *testing.T) {
	skipIfNoFFmpeg(t)
	muxer := mustNewMuxer(t)

	dir := t.TempDir()
	bad := filepath.Join(dir, "not-media.mp4")
	if err := os.WriteFile(bad, []byte("not a media file"), 0o644); err != nil {
		t.Fatalf("写坏输入文件失败：%v", err)
	}
	_, audio := generateTestMedia(t, muxer.binaryPath)
	out := filepath.Join(dir, "merged.mp4")

	err := muxer.MergeRemux(context.Background(), bad, audio, out)
	if err == nil {
		t.Fatal("期望合并失败，实际成功")
	}
	if !strings.Contains(err.Error(), "合并失败") {
		t.Fatalf("错误信息缺少\"合并失败\"前缀：%v", err)
	}
	if _, statErr := os.Stat(out); statErr == nil {
		t.Fatal("失败后产物未被清理")
	}
}

// TestMergeRemux_Timeout 验证超时路径：极短超时使合并被中断并返回错误。
func TestMergeRemux_Timeout(t *testing.T) {
	bin := skipIfNoFFmpeg(t)
	muxer := mustNewMuxer(t).WithTimeout(time.Nanosecond)

	video, audio := generateTestMedia(t, bin)
	out := filepath.Join(t.TempDir(), "merged.mp4")

	err := muxer.MergeRemux(context.Background(), video, audio, out)
	if err == nil {
		t.Fatal("期望因超时失败，实际成功")
	}
	if !strings.Contains(err.Error(), "被中断") {
		t.Fatalf("错误信息缺少\"被中断\"：%v", err)
	}
}
