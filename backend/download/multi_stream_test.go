package download

// 多轨流管理与下载循环单测（multi_stream_test.go）
//
// 覆盖插件下载执行面的多轨流状态机（暂存写入载体）：
//   - copyLoop 单流分支：downloaded/derived 完成、不完整、读取错误、runCtx 取消保留暂存
//   - handleEOF / handlePause 收口分支：取消/软暂停/正常完成/不完整/哈希校验（不符/未声明）
//   - downloadLoop 多流聚合：全部完成（转提交点前置态）、任一失败、软暂停广播收敛；进度汇总
//   - 软暂停信号消费（含暂停窗口的读取错误形态分流矩阵）：
//     无错误 → 纯排空暂停；传输错误 → 暂停收敛 + Warn 保留细节；EOF → 正常排空不告警；
//     排空超时强制取消 → 有损保留路径
//
// 暂存模式下流写入器为真实暂存文件（t.TempDir() 下 role_seq 键文件），断言落文件与状态；
// 控制面交互经 fakeHandle 模拟（运行 ctx / 软暂停广播通道 / 终态与排空阶段上报记录），
// 不依赖控制面真实实现。

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/taskManager"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// TestMain 全局初始化 no-op logger,使所有测试中的日志调用安全(logger.Log 默认 nil)
func TestMain(m *testing.M) {
	logger.Log = zap.NewNop().Sugar()
	os.Exit(m.Run())
}

// ==== fakes ====

// errorReadCloser 恒定返回 err 的 reader
type errorReadCloser struct {
	err    error
	closed int
}

func (r *errorReadCloser) Read(p []byte) (int, error) { return 0, r.err }
func (r *errorReadCloser) Close() error               { r.closed++; return nil }

// ctxAwareReader 返回 data 后阻塞,直到 ctx 取消再返回错误,让 copyLoop 的读取以错误返回后
// 由 runCtx 取消分支收敛(真实场景中控制面取消后由插件关闭上游使 reader EOF;此处直接绑定 ctx
// 以便测试取消分支)
type ctxAwareReader struct {
	data      []byte
	off       int
	ctx       context.Context
	blockedCh chan struct{}
	closed    int
}

func (r *ctxAwareReader) Read(p []byte) (int, error) {
	// ctx 已取消:直接返回错误(避免 drain 等再次调用时进入下方 close(blockedCh) 分支重复 close)
	select {
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	default:
	}
	if r.off >= len(r.data) {
		close(r.blockedCh)
		<-r.ctx.Done()
		return 0, r.ctx.Err()
	}
	n := copy(p, r.data[r.off:])
	r.off += n
	return n, nil
}
func (r *ctxAwareReader) Close() error { r.closed++; return nil }

// gatedReader 第一次 Read 阻塞等 first 关闭后才返回数据,精确控制 copyLoop 在途往返时序:
// 测试在 close(first) 前广播软暂停,确保 copyLoop Write 落盘后命中软暂停分支。
// 第二次 Read(若发生,意味着软暂停未生效)直接返回 EOF,使失败路径显式可断言。
type gatedReader struct {
	data   []byte
	off    int
	first  chan struct{}
	readN  int
	closed int
}

func (r *gatedReader) Read(p []byte) (int, error) {
	r.readN++
	if r.readN == 1 {
		<-r.first
	}
	if r.off >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.off:])
	r.off += n
	return n, nil
}
func (r *gatedReader) Close() error { r.closed++; return nil }

// softPauseGatedErrReader 第一次 Read 返回全部数据;第二次 Read 发起时先通知测试
// (secondStarted 证明第一轮的软暂停检查已在广播前放行、循环已进入第二次读取),再等软暂停
// 广播到达后返回注入错误(io.EOF 或传输错误)——精确构造「软暂停在第二轮读取挂起期间广播、
// 随后读取返回错误形态」的时序
type softPauseGatedErrReader struct {
	data          []byte
	err           error // 第二次 Read 的返回错误(传输错误或 io.EOF)
	softPause     <-chan struct{}
	secondStarted chan struct{}
	readN         int
	closed        int
}

func (r *softPauseGatedErrReader) Read(p []byte) (int, error) {
	r.readN++
	if r.readN == 1 {
		n := copy(p, r.data)
		return n, nil
	}
	close(r.secondStarted)
	<-r.softPause
	return 0, r.err
}
func (r *softPauseGatedErrReader) Close() error { r.closed++; return nil }

// fakeHandle 控制面执行句柄测试替身:记录终态/进度/排空阶段上报,软暂停通道可显式 close
// 模拟控制面广播;多流并发上报经 mu 保护
type fakeHandle struct {
	task      *entity.Task
	runCtx    context.Context
	softPause chan struct{}

	mu         sync.Mutex
	finished   bool
	failed     bool
	failMsg    string
	progress   [][2]int64
	drainPhase []bool // 排空阶段上报序列(进入=true、离开=false)
}

func newFakeHandle() (*fakeHandle, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	task := entity.NewTask()
	task.SetID(1)
	return &fakeHandle{task: task, runCtx: ctx, softPause: make(chan struct{})}, cancel
}

var _ taskManager.StrategyHandle = (*fakeHandle)(nil)

func (h *fakeHandle) Task() *entity.Task               { return h.task }
func (h *fakeHandle) RunCtx() context.Context          { return h.runCtx }
func (h *fakeHandle) SoftPauseSignal() <-chan struct{} { return h.softPause }
func (h *fakeHandle) ConfirmMemo() *taskManager.ReplaceConfirmMemo {
	return nil
}
func (h *fakeHandle) SetTerminalRollback(rollback taskManager.TerminalRollback) {}
func (h *fakeHandle) WaitReplaceConfirm(conflicts []taskManager.ConflictInfo) (taskManager.ReplaceDecision, bool) {
	return taskManager.ReplaceDecisionSkip, false
}
func (h *fakeHandle) Skip(errMsg string)    {}
func (h *fakeHandle) ResumeRequested() bool { return false }

func (h *fakeHandle) Finish() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.finished = true
}

func (h *fakeHandle) Fail(errMsg string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.failed = true
	h.failMsg = errMsg
}

func (h *fakeHandle) ReportProgress(total, finished int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.progress = append(h.progress, [2]int64{total, finished})
}

func (h *fakeHandle) MarkDrainPhase(in bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.drainPhase = append(h.drainPhase, in)
}

// newTestSession 构造最小可测执行会话(自建 fakeHandle 提供运行 ctx 与软暂停通道,无依赖注入)
func newTestSession() (*execSession, *fakeHandle, context.CancelFunc) {
	h, cancel := newFakeHandle()
	return newExecSession(nil, h, entity.NewWorkTask(1)), h, cancel
}

// newStream 构造一个单流控制器（真实暂存写入器，落 t.TempDir() 下 role_000 文件）
func newStream(t *testing.T, role, generation string, size int64, reader io.ReadCloser) *streamController {
	t.Helper()
	stagingAbs := filepath.Join(t.TempDir(), fmt.Sprintf("%s_%03d.bin", role, 0))
	w, err := newStagingWriterFresh(stagingAbs, "")
	if err != nil {
		t.Fatalf("打开测试暂存写入器失败: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return newStreamController(&sdkdto.StoreSpec{
		Role: role, Generation: generation, Size: size, Format: "bin", ReadCloser: reader,
	}, 0, w, stagingAbs, "store/resource/test/"+role+"_000.bin", role+"_000.bin")
}

// stagedSize 流暂存文件当前字节数
func stagedSize(t *testing.T, s *streamController) int64 {
	t.Helper()
	info, err := os.Stat(s.stagingAbs)
	if err != nil {
		t.Fatalf("stat 暂存文件失败: %v", err)
	}
	return info.Size()
}

// stagedExists 流暂存文件是否在位（失败保留断言）
func stagedExists(s *streamController) bool {
	_, err := os.Stat(s.stagingAbs)
	return err == nil
}

// observeLogs 切换全局 logger 为可观测实例,返回日志观察器(测试结束后调用方恢复 no-op)
func observeLogs() *observer.ObservedLogs {
	core, logs := observer.New(zapcore.DebugLevel)
	logger.Log = zap.New(core).Sugar()
	return logs
}

// warnMessages 提取全部 Warn 级日志消息
func warnMessages(logs *observer.ObservedLogs) []string {
	var msgs []string
	for _, e := range logs.All() {
		if e.Level == zapcore.WarnLevel {
			msgs = append(msgs, e.Message)
		}
	}
	return msgs
}

// assertDrainPhaseReported 校验下载循环进出时的排空阶段上报序列(进入=true、离开=false 成对)
func assertDrainPhaseReported(t *testing.T, h *fakeHandle, want ...bool) {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.drainPhase) != len(want) {
		t.Fatalf("排空阶段上报序列期望 %v, 实际 %v", want, h.drainPhase)
	}
	for i, w := range want {
		if h.drainPhase[i] != w {
			t.Fatalf("排空阶段上报序列期望 %v, 实际 %v", want, h.drainPhase)
		}
	}
}

// ==== 单流 copyLoop ====

func TestCopyLoop_DownloadedComplete(t *testing.T) {
	data := bytes.Repeat([]byte("a"), 100)
	sess, _, cancel := newTestSession()
	defer cancel()
	s := newStream(t, entity.StoreTypeImage, entity.GenerationDownloaded, int64(len(data)), io.NopCloser(bytes.NewReader(data)))

	res := s.copyLoop(sess)
	if res.kind != resultOK {
		t.Fatalf("期望 resultOK, 实际 %v (msg=%s)", res.kind, res.errMsg)
	}
	if streamState(s.state.Load()) != streamCompleted {
		t.Fatalf("期望流状态 completed")
	}
	if got := stagedSize(t, s); got != int64(len(data)) {
		t.Fatalf("期望暂存写入 %d, 实际 %d", len(data), got)
	}
	// 实测哈希已留存（提交点建行落列）且与内容一致
	if s.actualSha == "" {
		t.Fatal("写满后应留存实测 SHA256")
	}
	sum := sha256.Sum256(data)
	if s.actualSha != hex.EncodeToString(sum[:]) {
		t.Fatalf("实测 SHA256 不符: 期望 %s 实际 %s", hex.EncodeToString(sum[:]), s.actualSha)
	}
}

func TestCopyLoop_DerivedComplete(t *testing.T) {
	data := []byte("thumbdata")
	sess, _, cancel := newTestSession()
	defer cancel()
	// derived: size 未知(0)不校验完整性
	s := newStream(t, entity.StoreTypeThumbnail, entity.GenerationDerived, 0, io.NopCloser(bytes.NewReader(data)))

	res := s.copyLoop(sess)
	if res.kind != resultOK {
		t.Fatalf("期望 resultOK, 实际 %v (msg=%s)", res.kind, res.errMsg)
	}
	if streamState(s.state.Load()) != streamCompleted || stagedSize(t, s) != int64(len(data)) {
		t.Fatalf("derived 完成校验失败: state=%v size=%d", s.state.Load(), stagedSize(t, s))
	}
}

func TestCopyLoop_DownloadedIncomplete(t *testing.T) {
	// size 声明 100,但 reader 只给 30 字节就 EOF → 不完整
	data := bytes.Repeat([]byte("b"), 30)
	sess, _, cancel := newTestSession()
	defer cancel()
	s := newStream(t, entity.StoreTypeImage, entity.GenerationDownloaded, 100, io.NopCloser(bytes.NewReader(data)))

	res := s.copyLoop(sess)
	if res.kind != resultFailed {
		t.Fatalf("期望 resultFailed(不完整), 实际 %v", res.kind)
	}
	// 失败保留暂存（诊断可见，重试重下覆盖），不再走删文件
	if !stagedExists(s) || stagedSize(t, s) != 30 {
		t.Fatalf("不完整期望暂存保留 30 字节, exists=%v size=%d", stagedExists(s), stagedSize(t, s))
	}
}

func TestCopyLoop_ReadError(t *testing.T) {
	sess, _, cancel := newTestSession()
	defer cancel()
	s := newStream(t, entity.StoreTypeImage, entity.GenerationDownloaded, 100, &errorReadCloser{err: errors.New("net boom")})

	res := s.copyLoop(sess)
	if res.kind != resultFailed {
		t.Fatalf("期望 resultFailed, 实际 %v", res.kind)
	}
	if res.errMsg == "" {
		t.Fatalf("期望错误信息非空")
	}
	if !stagedExists(s) {
		t.Fatalf("读取错误期望暂存保留")
	}
}

func TestCopyLoop_Cancel(t *testing.T) {
	sess, h, cancel := newTestSession()
	defer cancel()
	// runCtx 取消(控制面暂停/停止)统一走 handlePause 保留暂存。
	// 停止后的暂存清理由控制面按任务状态处置,copyLoop 不删文件
	cr := &ctxAwareReader{data: bytes.Repeat([]byte("c"), 50), ctx: h.runCtx, blockedCh: make(chan struct{})}
	s := newStream(t, entity.StoreTypeImage, entity.GenerationDownloaded, 100, cr)

	done := make(chan streamResult, 1)
	go func() { done <- s.copyLoop(sess) }()

	<-cr.blockedCh
	cancel()

	res := <-done
	if res.kind != resultPaused {
		t.Fatalf("期望 resultPaused(runCtx 取消统一保留暂存), 实际 %v", res.kind)
	}
	if !stagedExists(s) {
		t.Fatalf("runCtx 取消应保留暂存文件")
	}
	if !s.writer.closed {
		t.Fatalf("runCtx 取消应 Sync+Close 保留文件")
	}
}

// TestCopyLoop_PauseCancelPreservesFile 回归:runCtx 取消时 copyLoop 应 Sync+Close 保留暂存(不删),
// 否则下次续传 offset=0 → 进度倒退
func TestCopyLoop_PauseCancelPreservesFile(t *testing.T) {
	sess, h, cancel := newTestSession()
	defer cancel()
	cr := &ctxAwareReader{data: bytes.Repeat([]byte("c"), 50), ctx: h.runCtx, blockedCh: make(chan struct{})}
	s := newStream(t, entity.StoreTypeImage, entity.GenerationDownloaded, 100, cr)

	done := make(chan streamResult, 1)
	go func() { done <- s.copyLoop(sess) }()

	<-cr.blockedCh
	cancel()

	res := <-done
	if res.kind != resultPaused {
		t.Fatalf("期望 resultPaused, 实际 %v", res.kind)
	}
	if !stagedExists(s) {
		t.Fatalf("runCtx 取消不应删除暂存(应保留文件防进度倒退)")
	}
	if !s.writer.closed {
		t.Fatalf("runCtx 取消应 Sync+Close 保留文件")
	}
}

// ==== handleEOF / handlePause 分支 ====

func TestHandleEOF_RunCtxCanceled(t *testing.T) {
	// runCtx 取消(停止/排空超时强制取消)导致上游关闭产生 EOF:视为中断,保留暂存
	sess, _, cancel := newTestSession()
	defer cancel()
	cancel()
	s := newStream(t, entity.StoreTypeImage, entity.GenerationDownloaded, 10, io.NopCloser(bytes.NewReader([]byte("x"))))

	res := s.handleEOF(sess)
	if res.kind != resultPaused {
		t.Fatalf("runCtx 取消时 EOF 期望 resultPaused, 实际 %v", res.kind)
	}
	if !s.writer.closed {
		t.Fatalf("期望 Sync+Close(非完成收尾)")
	}
	if streamState(s.state.Load()) != streamPaused {
		t.Fatalf("期望流状态 paused")
	}
}

func TestHandleEOF_SoftPause(t *testing.T) {
	// 软暂停进行中导致上游关闭产生 EOF:置 paused,不判完成
	sess, h, cancel := newTestSession()
	defer cancel()
	close(h.softPause)
	s := newStream(t, entity.StoreTypeImage, entity.GenerationDownloaded, 10, io.NopCloser(bytes.NewReader([]byte("x"))))

	res := s.handleEOF(sess)
	if res.kind != resultPaused {
		t.Fatalf("软暂停进行中 EOF 期望 resultPaused, 实际 %v", res.kind)
	}
	if !s.writer.closed {
		t.Fatalf("期望 Sync+Close(非完成收尾)")
	}
	if streamState(s.state.Load()) != streamPaused {
		t.Fatalf("期望流状态 paused")
	}
}

func TestHandleEOF_Complete(t *testing.T) {
	sess, _, cancel := newTestSession()
	defer cancel()
	s := newStream(t, entity.StoreTypeImage, entity.GenerationDownloaded, 10, nil)
	s.written = 10 // 已写满

	res := s.handleEOF(sess)
	if res.kind != resultOK {
		t.Fatalf("期望 resultOK, 实际 %v", res.kind)
	}
	if streamState(s.state.Load()) != streamCompleted {
		t.Fatalf("期望流状态 completed")
	}
	if s.actualSha == "" {
		t.Fatalf("写满后应留存实测 SHA256")
	}
}

func TestHandleEOF_Incomplete(t *testing.T) {
	sess, _, cancel := newTestSession()
	defer cancel()
	s := newStream(t, entity.StoreTypeImage, entity.GenerationDownloaded, 100, nil)
	s.written = 30 // 不足

	res := s.handleEOF(sess)
	if res.kind != resultFailed {
		t.Fatalf("期望 resultFailed(不完整), 实际 %v", res.kind)
	}
	if !stagedExists(s) {
		t.Fatalf("不完整期望暂存保留")
	}
}

func TestHandlePause(t *testing.T) {
	s := newStream(t, entity.StoreTypeImage, entity.GenerationDownloaded, 10, io.NopCloser(bytes.NewReader([]byte{})))

	res := s.handlePause(nil)
	if res.kind != resultPaused {
		t.Fatalf("期望 resultPaused, 实际 %v", res.kind)
	}
	if !s.writer.closed {
		t.Fatalf("期望 Sync+Close")
	}
	if streamState(s.state.Load()) != streamPaused {
		t.Fatalf("期望流状态 paused")
	}
}

// ==== downloadLoop 多流聚合 ====

func TestDownloadLoop_AllComplete(t *testing.T) {
	h, cancel := newFakeHandle()
	defer cancel()
	sess := newExecSession(nil, h, entity.NewWorkTask(1))

	data := bytes.Repeat([]byte("a"), 50)
	s1 := newStream(t, entity.StoreTypeImage, entity.GenerationDownloaded, 50, io.NopCloser(bytes.NewReader(data)))
	s2 := newStream(t, entity.StoreTypeThumbnail, entity.GenerationDerived, 0, io.NopCloser(bytes.NewReader([]byte("thumb"))))
	sess.streams = []*streamController{s1, s2}

	res := sess.downloadLoop()
	if res != loopDone {
		t.Fatalf("期望 loopDone, 实际 %v", res)
	}
	// 全部完成转提交点：终态由提交点收口（downloadLoop 只收失败终态）
	h.mu.Lock()
	finished, failed := h.finished, h.failed
	h.mu.Unlock()
	if finished || failed {
		t.Fatalf("全部完成不应在循环内上报终态(归提交点): finished=%v failed=%v", finished, failed)
	}
	if streamState(s1.state.Load()) != streamCompleted || streamState(s2.state.Load()) != streamCompleted {
		t.Fatalf("期望两轨均 completed: s1=%v s2=%v", s1.state.Load(), s2.state.Load())
	}
	assertDrainPhaseReported(t, h, true, false)
}

func TestDownloadLoop_OneFails(t *testing.T) {
	h, cancel := newFakeHandle()
	defer cancel()
	sess := newExecSession(nil, h, entity.NewWorkTask(1))

	sMain := newStream(t, entity.StoreTypeImage, entity.GenerationDownloaded, 50, io.NopCloser(bytes.NewReader(bytes.Repeat([]byte("a"), 50))))
	sFail := newStream(t, entity.StoreTypeThumbnail, entity.GenerationDerived, 0, &errorReadCloser{err: errors.New("thumb gen failed")})
	sess.streams = []*streamController{sMain, sFail}

	res := sess.downloadLoop()
	if res != loopDone {
		t.Fatalf("期望 loopDone, 实际 %v", res)
	}
	h.mu.Lock()
	failed, failMsg := h.failed, h.failMsg
	h.mu.Unlock()
	if !failed {
		t.Fatalf("期望经 handle 上报失败终态")
	}
	if failMsg == "" {
		t.Fatalf("期望失败信息非空")
	}
	// 已完成轨保留(completed),失败轨暂存保留(诊断可见)
	if streamState(sMain.state.Load()) != streamCompleted {
		t.Fatalf("主轨应保留完成态: %v", sMain.state.Load())
	}
	if streamState(sFail.state.Load()) != streamFailed || !stagedExists(sFail) {
		t.Fatalf("失败轨应为 failed 且暂存保留: state=%v exists=%v", sFail.state.Load(), stagedExists(sFail))
	}
}

func TestDownloadLoop_PauseBroadcast(t *testing.T) {
	h, cancel := newFakeHandle()
	defer cancel()
	sess := newExecSession(nil, h, entity.NewWorkTask(1))

	// 两轨用 gatedReader:第一次 Read 阻塞等 first,精确卡软暂停时序
	gr1 := &gatedReader{data: bytes.Repeat([]byte("a"), 10), first: make(chan struct{})}
	gr2 := &gatedReader{data: bytes.Repeat([]byte("b"), 10), first: make(chan struct{})}
	s1 := newStream(t, entity.StoreTypeImage, entity.GenerationDownloaded, 100, gr1)
	s2 := newStream(t, entity.StoreTypeVideoTrack, entity.GenerationDownloaded, 100, gr2)
	sess.streams = []*streamController{s1, s2}

	done := make(chan loopResult, 1)
	go func() { done <- sess.downloadLoop() }()

	// 模拟控制面进入软暂停:close 广播(不取消 runCtx),再许可两轨在途 Read 返回
	close(h.softPause)
	close(gr1.first)
	close(gr2.first)

	select {
	case res := <-done:
		if res != loopPaused {
			t.Fatalf("期望 loopPaused, 实际 %v", res)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("downloadLoop 未退出(软暂停路径未生效)")
	}
	h.mu.Lock()
	finished, failed := h.finished, h.failed
	h.mu.Unlock()
	// 暂停收敛不上报终态,交控制面置 Paused
	if finished || failed {
		t.Fatalf("软暂停不应上报终态: finished=%v failed=%v", finished, failed)
	}
	// 优雅暂停不取消 runCtx(与立即切断路径的区别)
	if h.runCtx.Err() != nil {
		t.Fatalf("优雅暂停不应取消 runCtx")
	}
	// 两轨在途数据落盘 + Sync+Close(非完成收尾),暂存保留
	for i, s := range []*streamController{s1, s2} {
		if !s.writer.closed || streamState(s.state.Load()) != streamPaused {
			t.Fatalf("轨 %d 应为暂停态: closed=%v state=%v", i, s.writer.closed, s.state.Load())
		}
		if got := stagedSize(t, s); got != 10 {
			t.Fatalf("轨 %d 期望在途 10 字节落盘, 实际 %d", i, got)
		}
	}
	assertDrainPhaseReported(t, h, true, false)
}

func TestProgressAggregation(t *testing.T) {
	sess, h, cancel := newTestSession()
	defer cancel()

	s1 := newStream(t, entity.StoreTypeImage, entity.GenerationDownloaded, 100, nil)
	s2 := newStream(t, entity.StoreTypeThumbnail, entity.GenerationDerived, 5, nil)
	s1.written = 60
	s2.written = 5
	sess.streams = []*streamController{s1, s2}

	// totalStreamSize 只计 size>0 的轨(derived size=5 计入;若 derived size=0 不计)
	if got := sess.totalStreamSize(); got != 105 {
		t.Fatalf("totalStreamSize 期望 105, 实际 %d", got)
	}
	sess.reportProgress()
	h.mu.Lock()
	progress := append([][2]int64(nil), h.progress...)
	h.mu.Unlock()
	if len(progress) != 1 {
		t.Fatalf("期望一次进度上报, 实际 %d", len(progress))
	}
	if progress[0] != [2]int64{105, 65} {
		t.Fatalf("reportProgress 期望 total=105 finished=65, 实际 total=%d finished=%d", progress[0][0], progress[0][1])
	}
}

// ==== 软暂停信号消费 ====

// TestCopyLoop_SoftPause_DrainsInflight 验证优雅暂停核心(在途读取无错误形态):本轮 Read 的
// 在途数据先落暂存,copyLoop 随后退出,且不再 Read(不发起新 PullRequest 拉取新数据)。
func TestCopyLoop_SoftPause_DrainsInflight(t *testing.T) {
	sess, h, cancel := newTestSession()
	defer cancel()
	// 在途数据 50 字节;size=100 → 若软暂停未生效会继续读到 EOF 判不完整 Failed
	gr := &gatedReader{data: bytes.Repeat([]byte("x"), 50), first: make(chan struct{})}
	s := newStream(t, entity.StoreTypeImage, entity.GenerationDownloaded, 100, gr)

	done := make(chan streamResult, 1)
	go func() { done <- s.copyLoop(sess) }()

	// 广播软暂停(模拟控制面进入软暂停),再许可在途 Read 返回。
	// close 经 channel happens-before 传播到 copyLoop 的非阻塞探测
	close(h.softPause)
	close(gr.first)

	select {
	case res := <-done:
		if res.kind != resultPaused {
			t.Fatalf("期望 resultPaused(在途落盘后退出), 实际 %v (msg=%s)", res.kind, res.errMsg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("copyLoop 未退出(软暂停路径未生效)")
	}
	if got := stagedSize(t, s); got != 50 {
		t.Fatalf("期望在途 50 字节落暂存, 实际 %d", got)
	}
	if !s.writer.closed {
		t.Fatalf("软暂停应 Sync+Close")
	}
	// 仅 Read 一次:在途往返已落盘即退出,未发起新 PullRequest
	if gr.readN != 1 {
		t.Fatalf("期望仅 Read 1 次(在途), 实际 %d(发了新 PullRequest)", gr.readN)
	}
}

// TestCopyLoop_SoftPause_TransferErrorConvergesPauseWithDetail 软暂停窗口出现传输错误(非 EOF):
// 仍按暂停收敛(用户暂停意图优先、保留暂存供续传),但 Warn 日志保留传输错误细节供诊断
func TestCopyLoop_SoftPause_TransferErrorConvergesPauseWithDetail(t *testing.T) {
	logs := observeLogs()
	defer func() { logger.Log = zap.NewNop().Sugar() }()

	sess, h, cancel := newTestSession()
	defer cancel()
	boom := errors.New("插件传输中断: 连接已重置")
	// 第二次 Read 先等软暂停广播到达再返回传输错误——第一轮落盘后的软暂停检查
	// 在广播前已放行进入第二轮读取,确保错误发生在暂停窗口内
	gr := &softPauseGatedErrReader{data: bytes.Repeat([]byte("x"), 50), err: boom, softPause: h.softPause, secondStarted: make(chan struct{})}
	s := newStream(t, entity.StoreTypeImage, entity.GenerationDownloaded, 100, gr)

	done := make(chan streamResult, 1)
	go func() { done <- s.copyLoop(sess) }()

	<-gr.secondStarted // 等循环进入第二次读取(第一轮软暂停检查已放行)
	close(h.softPause) // 广播软暂停,许可第二次读取以传输错误返回

	select {
	case res := <-done:
		if res.kind != resultPaused {
			t.Fatalf("期望 resultPaused(用户暂停意图优先), 实际 %v (msg=%s)", res.kind, res.errMsg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("copyLoop 未退出(软暂停窗口传输错误路径未生效)")
	}
	if got := stagedSize(t, s); got != 50 {
		t.Fatalf("期望在途 50 字节落暂存, 实际 %d", got)
	}
	if !s.writer.closed {
		t.Fatalf("软暂停窗口传输错误应 Sync+Close")
	}
	// Warn 保留传输错误细节(含 role 与错误文本)
	var found bool
	for _, msg := range warnMessages(logs) {
		if strings.Contains(msg, boom.Error()) && strings.Contains(msg, entity.StoreTypeImage) {
			found = true
		}
	}
	if !found {
		t.Fatalf("期望 Warn 日志保留传输错误细节, 实际 Warn 列表: %v", warnMessages(logs))
	}
}

// TestCopyLoop_SoftPause_EOFIsNormalDrain 软暂停窗口的 EOF 视为正常排空:按暂停收敛,
// 不产生传输错误告警
func TestCopyLoop_SoftPause_EOFIsNormalDrain(t *testing.T) {
	logs := observeLogs()
	defer func() { logger.Log = zap.NewNop().Sugar() }()

	sess, h, cancel := newTestSession()
	defer cancel()
	gr := &softPauseGatedErrReader{data: bytes.Repeat([]byte("x"), 50), err: io.EOF, softPause: h.softPause, secondStarted: make(chan struct{})}
	s := newStream(t, entity.StoreTypeImage, entity.GenerationDownloaded, 100, gr)

	done := make(chan streamResult, 1)
	go func() { done <- s.copyLoop(sess) }()

	<-gr.secondStarted // 等循环进入第二次读取(第一轮软暂停检查已放行)
	close(h.softPause) // 广播软暂停,许可第二次读取以 EOF 返回

	select {
	case res := <-done:
		if res.kind != resultPaused {
			t.Fatalf("期望 resultPaused(EOF 正常排空按暂停收敛), 实际 %v (msg=%s)", res.kind, res.errMsg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("copyLoop 未退出(软暂停窗口 EOF 路径未生效)")
	}
	if got := stagedSize(t, s); got != 50 {
		t.Fatalf("期望在途 50 字节落暂存, 实际 %d", got)
	}
	if !s.writer.closed {
		t.Fatalf("期望 Sync+Close 保留暂存")
	}
	if warns := warnMessages(logs); len(warns) != 0 {
		t.Fatalf("软暂停窗口的 EOF 属正常排空,不应告警, 实际 Warn 列表: %v", warns)
	}
}

// TestCopyLoop_SoftPauseDrainTimeoutForceCancel 排空超时兜底:软暂停已广播、在途读取迟迟
// 不返回时,控制面强制取消 runCtx,读取以错误返回后走有损保留路径(取消属控制面显式决策,
// 不产生传输错误告警)
func TestCopyLoop_SoftPauseDrainTimeoutForceCancel(t *testing.T) {
	logs := observeLogs()
	defer func() { logger.Log = zap.NewNop().Sugar() }()

	sess, h, cancel := newTestSession()
	defer cancel()
	// data 为空:第一次 Read 即阻塞等 runCtx,模拟插件卡死/在途不完成
	cr := &ctxAwareReader{ctx: h.runCtx, blockedCh: make(chan struct{})}
	s := newStream(t, entity.StoreTypeImage, entity.GenerationDownloaded, 100, cr)

	close(h.softPause) // 软暂停已广播,转入排空等待

	done := make(chan streamResult, 1)
	go func() { done <- s.copyLoop(sess) }()

	<-cr.blockedCh // copyLoop 阻塞在在途 Read

	// 模拟控制面排空超时兜底:强制取消 runCtx
	cancel()

	select {
	case res := <-done:
		// runCtx 取消 → Read 返回错误 → 有损路径 handlePause(保留暂存)
		if res.kind != resultPaused {
			t.Fatalf("期望 resultPaused(超时兜底有损路径), 实际 %v", res.kind)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("copyLoop 未退出(排空超时兜底未生效)")
	}
	if !s.writer.closed {
		t.Fatal("兜底路径应 Sync+Close 保留暂存")
	}
	if warns := warnMessages(logs); len(warns) != 0 {
		t.Fatalf("超时兜底的强制取消属控制面显式决策,不应告警, 实际 Warn 列表: %v", warns)
	}
}
