package taskManager

// 多流单测清单(model_multi_stream_test.go)
//
// 覆盖多轨重构阶段2 最高风险点(taskManager 多流 Pause/Resume/Stop per-stream 状态机 + 资源级聚合)。
//
// A. 纯逻辑(无状态机):
//   - filterSpecsByRoles: 全量 / 子集 / 无命中
//   - uniqueRoles / findSpec / findStoreRow / normalizeExt
//   - mergeWorkInfo
//   - buildThumbnailRelPath / buildThumbnailFileName
//
// B. 单流 streamController(无 setState,不触发 logger):
//   - copyLoop: downloaded 完成 / derived 完成 / downloaded 不完整(EOF 提前) / 读取错误 / ctx 取消
//
// C. handleEOF / handlePause 直接分支:
//   - handleEOF: pausing→暂停 / stopping→取消 / 正常完成 / downloaded 不完整
//   - handlePause: Sync+Close+paused
//
// D. downloadLoop 多流聚合(触发 setState,需 nop logger):
//   - 全部完成 → Finished(已完成轨 writer 保留)
//   - 任一失败 → Failed(已完成轨保留、失败轨 abort)
//   - 全部暂停 → Paused(资源级聚合)
//   - 进度汇总 reportProgress / totalStreamSize

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/entity"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
	"go.uber.org/zap"
)

// TestMain 全局初始化 no-op logger,使所有测试中 setState/handleEOF 等日志调用安全(logger.Log 默认 nil)
func TestMain(m *testing.M) {
	logger.Log = zap.NewNop().Sugar()
	os.Exit(m.Run())
}

// ==== fakes ====

// fakeStoreWriter 内存版 StoreWriter,记录写入字节与生命周期调用
type fakeStoreWriter struct {
	buf       bytes.Buffer
	closed    bool
	completed bool
	aborted   bool
	syncN     int
	writeErr  error // 注入的写入错误
}

func (w *fakeStoreWriter) Write(p []byte) (int, error) {
	if w.writeErr != nil {
		return 0, w.writeErr
	}
	return w.buf.Write(p)
}
func (w *fakeStoreWriter) Sync() error { w.syncN++; return nil }
func (w *fakeStoreWriter) Close() error {
	w.closed = true
	return nil
}
func (w *fakeStoreWriter) Complete() error {
	w.completed = true
	w.closed = true
	return nil
}
func (w *fakeStoreWriter) Abort() error {
	w.aborted = true
	w.closed = true
	return nil
}

// errorReadCloser 恒定返回 err 的 reader
type errorReadCloser struct {
	err    error
	closed int
}

func (r *errorReadCloser) Read(p []byte) (int, error) { return 0, r.err }
func (r *errorReadCloser) Close() error               { r.closed++; return nil }

// ctxAwareReader 返回 data 后阻塞,直到 ctx 取消再返回 (0,nil),让 copyLoop 回到 select 捕获 ctx.Done
// (真实场景中 Stop 取消 ctx 后由插件关闭上游使 reader EOF;此处直接绑定 ctx 以便测试取消分支)
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
// 主测试在 close(first) 前置 softPause,确保 copyLoop Write 落盘后命中 softPause 分支。
// 第二次 Read(若发生,意味着 softPause 未生效)直接返回 EOF,使失败路径显式可断言。
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

// nopLogger 把 logger.Log 设为 no-op,使 setState 的日志调用在测试中安全
func nopLogger() {
	logger.Log = zap.NewNop().Sugar()
}

// newTestManagedTask 构造最小可测 ManagedTask(ctx/pauseCh/cmdCh/state/done/task),不启动 actor(供 copyLoop/downloadLoop 单元测试)
func newTestManagedTask() *ManagedTask {
	ctx, cancel := context.WithCancel(context.Background())
	m := &ManagedTask{
		taskId:    1,
		ctx:       ctx,
		cancel:    cancel,
		runCtx:    ctx, // copyLoop 单元测试用 runCtx(正式环境由 handleRunCmd 派生)
		runCancel: cancel,
		cmdCh:     make(chan taskCmd, 8),
		actorDone: make(chan struct{}),
		done:      make(chan struct{}),
		task:      entity.NewTask(),
	}
	m.state.Store(int32(TaskStateProcessing))
	return m
}

// newStream 构造一个单流控制器
func newStream(role, generation string, size int64, reader io.ReadCloser, writer *fakeStoreWriter) *streamController {
	return &streamController{
		role:       role,
		generation: generation,
		size:       size,
		reader:     reader,
		storeWriter: writer,
	}
}

func mkSpec(role, generation string, size int64, r io.ReadCloser) *sdkdto.StoreSpec {
	return &sdkdto.StoreSpec{Role: role, Generation: generation, Size: size, ReadCloser: r}
}

// ==== A. 纯逻辑 ====

func TestFilterSpecsByRoles(t *testing.T) {
	specs := []*sdkdto.StoreSpec{
		mkSpec(entity.StoreTypeMain, entity.GenerationDownloaded, 10, nil),
		mkSpec(entity.StoreTypeThumbnail, entity.GenerationDerived, 1, nil),
		mkSpec(entity.StoreTypeVideoTrack, entity.GenerationDownloaded, 20, nil),
	}
	// 空 storeRoles = 全量
	m := &ManagedTask{runMode: runMode{storeRoles: nil}}
	if got := m.filterSpecsByRoles(specs); len(got) != 3 {
		t.Fatalf("全量过滤期望 3, 实际 %d", len(got))
	}
	// 子集
	m = &ManagedTask{runMode: runMode{storeRoles: []string{entity.StoreTypeThumbnail}}}
	got := m.filterSpecsByRoles(specs)
	if len(got) != 1 || got[0].Role != entity.StoreTypeThumbnail {
		t.Fatalf("子集过滤期望仅 thumbnail, 实际 %+v", got)
	}
	// 多选子集
	m = &ManagedTask{runMode: runMode{storeRoles: []string{entity.StoreTypeMain, entity.StoreTypeVideoTrack}}}
	if got := m.filterSpecsByRoles(specs); len(got) != 2 {
		t.Fatalf("多选过滤期望 2, 实际 %d", len(got))
	}
	// 无命中
	m = &ManagedTask{runMode: runMode{storeRoles: []string{"unknown"}}}
	if got := m.filterSpecsByRoles(specs); len(got) != 0 {
		t.Fatalf("无命中期望 0, 实际 %d", len(got))
	}
}

func TestNormalizeExt(t *testing.T) {
	if got := normalizeExt("mp4"); got != ".mp4" {
		t.Fatalf("mp4 期望 .mp4, 实际 %s", got)
	}
	if got := normalizeExt(".mp4"); got != ".mp4" {
		t.Fatalf(".mp4 期望 .mp4, 实际 %s", got)
	}
	if got := normalizeExt(""); got != "" {
		t.Fatalf("空期望空, 实际 %s", got)
	}
}

func TestBuildThumbnailRelPath(t *testing.T) {
	got := buildThumbnailRelPath("store/resource/作者/video.mp4", "jpg")
	want := "store/thumbnail/作者/video_thumbnail.jpg"
	if got != want {
		t.Fatalf("期望 %s, 实际 %s", want, got)
	}
}

// TestRunModeFromTask 锁定板块派生契约(StartTaskTree 重置 StoreRoles=NULL 依赖此返回全量)
// 回归:上一次 Redownload 持久化的子集 StoreRoles 不应泄漏到后续全量执行
func TestRunModeFromTask(t *testing.T) {
	// StoreRoles=NULL(首次执行 / StartTaskTree 重置后)= 全量
	t1 := entity.NewTask()
	mode := runModeFromTask(t1)
	if !mode.hasWorkInfo() || !mode.hasStore(entity.StoreTypeMain) || !mode.hasStore(entity.StoreTypeThumbnail) {
		t.Fatalf("NULL StoreRoles 期望全量, 实际 %+v", mode)
	}

	// StoreRoles="thumbnail"(Redownload 子集)= 仅缩略图
	t2 := entity.NewTask()
	t2.StoreRoles = sql.NullString{String: entity.StoreTypeThumbnail, Valid: true}
	t2.IncludeWorkInfo = false
	mode = runModeFromTask(t2)
	if mode.hasStore(entity.StoreTypeMain) || !mode.hasStore(entity.StoreTypeThumbnail) {
		t.Fatalf("thumbnail 子集期望仅缩略图, 实际 %+v", mode)
	}

	// StoreRoles="main,thumbnail" + includeWorkInfo=true = 全量显式
	t3 := entity.NewTask()
	t3.StoreRoles = sql.NullString{String: entity.StoreTypeMain + "," + entity.StoreTypeThumbnail, Valid: true}
	t3.IncludeWorkInfo = true
	mode = runModeFromTask(t3)
	if !mode.hasWorkInfo() || !mode.hasStore(entity.StoreTypeMain) || !mode.hasStore(entity.StoreTypeThumbnail) {
		t.Fatalf("显式全量期望 main+thumbnail+workInfo, 实际 %+v", mode)
	}
}

func TestMergeWorkInfo(t *testing.T) {
	to := &sdkdto.WorkResponse{}
	workName := "x"
	from := &sdkdto.WorkResponse{
		Work:         &sdkdto.WorkDTO{SiteWorkName: &workName},
		SiteAuthors:  []*sdkdto.TaskSiteAuthorDTO{{SiteAuthorID: "a1"}},
		LocalAuthors: []*sdkdto.LocalAuthorDTO{{ID: 1}},
	}
	mergeWorkInfo(to, from)
	if to.Work == nil || to.Work.SiteWorkName == nil {
		t.Fatalf("Work 未合并")
	}
	if len(to.SiteAuthors) != 1 || len(to.LocalAuthors) != 1 {
		t.Fatalf("作者未合并: %+v", to)
	}
	// nil 安全
	mergeWorkInfo(nil, from)
	mergeWorkInfo(to, nil)
}

// ==== B. 单流 copyLoop ====

func TestCopyLoop_DownloadedComplete(t *testing.T) {
	data := bytes.Repeat([]byte("a"), 100)
	m := newTestManagedTask()
	w := &fakeStoreWriter{}
	s := newStream(entity.StoreTypeMain, entity.GenerationDownloaded, int64(len(data)), io.NopCloser(bytes.NewReader(data)), w)

	res := s.copyLoop(m)
	if res.kind != resultOK {
		t.Fatalf("期望 resultOK, 实际 %v (msg=%s)", res.kind, res.errMsg)
	}
	if !w.completed {
		t.Fatalf("期望 writer.Completed")
	}
	if w.buf.Len() != len(data) {
		t.Fatalf("期望写入 %d, 实际 %d", len(data), w.buf.Len())
	}
}

func TestCopyLoop_DerivedComplete(t *testing.T) {
	data := []byte("thumbdata")
	m := newTestManagedTask()
	w := &fakeStoreWriter{}
	// derived: size 未知(0)不校验完整性
	s := newStream(entity.StoreTypeThumbnail, entity.GenerationDerived, 0, io.NopCloser(bytes.NewReader(data)), w)

	res := s.copyLoop(m)
	if res.kind != resultOK {
		t.Fatalf("期望 resultOK, 实际 %v", res.kind)
	}
	if !w.completed || w.buf.Len() != len(data) {
		t.Fatalf("derived 完成校验失败: completed=%v len=%d", w.completed, w.buf.Len())
	}
}

func TestCopyLoop_DownloadedIncomplete(t *testing.T) {
	// size 声明 100,但 reader 只给 30 字节就 EOF → 不完整
	data := bytes.Repeat([]byte("b"), 30)
	m := newTestManagedTask()
	w := &fakeStoreWriter{}
	s := newStream(entity.StoreTypeMain, entity.GenerationDownloaded, 100, io.NopCloser(bytes.NewReader(data)), w)

	res := s.copyLoop(m)
	if res.kind != resultFailed {
		t.Fatalf("期望 resultFailed(不完整), 实际 %v", res.kind)
	}
	if !w.aborted {
		t.Fatalf("不完整期望 writer.Aborted")
	}
}

func TestCopyLoop_ReadError(t *testing.T) {
	m := newTestManagedTask()
	w := &fakeStoreWriter{}
	s := newStream(entity.StoreTypeMain, entity.GenerationDownloaded, 100, &errorReadCloser{err: errors.New("net boom")}, w)

	res := s.copyLoop(m)
	if res.kind != resultFailed {
		t.Fatalf("期望 resultFailed, 实际 %v", res.kind)
	}
	if res.errMsg == "" {
		t.Fatalf("期望错误信息非空")
	}
	if !w.aborted {
		t.Fatalf("读取错误期望 writer.Aborted")
	}
}

func TestCopyLoop_Cancel(t *testing.T) {
	m := newTestManagedTask()
	// A 阶段:runCtx 取消(Pause/Stop 经 watcher)统一走 handlePause 保留文件。
	// Stop 的文件删除由 handleStopCmd 的 streams abort 处理(actor 内),copyLoop 不再 abort。
	w := &fakeStoreWriter{}
	cr := &ctxAwareReader{data: bytes.Repeat([]byte("c"), 50), ctx: m.ctx, blockedCh: make(chan struct{})}
	s := newStream(entity.StoreTypeMain, entity.GenerationDownloaded, 100, cr, w)

	done := make(chan streamResult, 1)
	go func() { done <- s.copyLoop(m) }()

	<-cr.blockedCh
	m.cancel()

	res := <-done
	if res.kind != resultPaused {
		t.Fatalf("期望 resultPaused(runCtx 取消统一保留文件), 实际 %v", res.kind)
	}
	if w.aborted {
		t.Fatalf("runCtx 取消不应 Abort(Stop 的删除由 handleStopCmd 处理)")
	}
	if !w.closed {
		t.Fatalf("runCtx 取消应 Sync+Close 保留文件")
	}
}

// TestCopyLoop_PauseCancelPreservesFile 回归:runCtx 取消时 copyLoop 应 Sync+Close 保留文件(不 Abort),
// 否则下次 resume offset=0 → 进度倒退
func TestCopyLoop_PauseCancelPreservesFile(t *testing.T) {
	m := newTestManagedTask()
	// runCtx 取消(经 watcher):应保留文件
	w := &fakeStoreWriter{}
	cr := &ctxAwareReader{data: bytes.Repeat([]byte("c"), 50), ctx: m.ctx, blockedCh: make(chan struct{})}
	s := newStream(entity.StoreTypeMain, entity.GenerationDownloaded, 100, cr, w)

	done := make(chan streamResult, 1)
	go func() { done <- s.copyLoop(m) }()

	<-cr.blockedCh
	m.cancel()

	res := <-done
	if res.kind != resultPaused {
		t.Fatalf("期望 resultPaused, 实际 %v", res.kind)
	}
	if w.aborted {
		t.Fatalf("runCtx 取消不应 Abort(应保留文件防进度倒退)")
	}
	if !w.closed {
		t.Fatalf("runCtx 取消应 Sync+Close 保留文件")
	}
}

// ==== C. handleEOF / handlePause 分支 ====

func TestHandleEOF_Pausing(t *testing.T) {
	m := newTestManagedTask()
	m.state.Store(int32(TaskStatePausing))
	w := &fakeStoreWriter{}
	s := newStream(entity.StoreTypeMain, entity.GenerationDownloaded, 10, io.NopCloser(bytes.NewReader([]byte("x"))), w)

	res := s.handleEOF(m)
	if res.kind != resultPaused {
		t.Fatalf("pausing 时 EOF 期望 resultPaused, 实际 %v", res.kind)
	}
	if !w.closed || w.completed {
		t.Fatalf("期望 Sync+Close(非 Complete): closed=%v completed=%v", w.closed, w.completed)
	}
	if streamState(s.state.Load()) != streamPaused {
		t.Fatalf("期望 stream 状态 paused")
	}
}

func TestHandleEOF_Stopping(t *testing.T) {
	m := newTestManagedTask()
	m.state.Store(int32(TaskStateStopping))
	w := &fakeStoreWriter{}
	s := newStream(entity.StoreTypeMain, entity.GenerationDownloaded, 10, io.NopCloser(bytes.NewReader([]byte("x"))), w)

	res := s.handleEOF(m)
	if res.kind != resultCanceled {
		t.Fatalf("stopping 时 EOF 期望 resultCanceled, 实际 %v", res.kind)
	}
	if !w.aborted {
		t.Fatalf("stopping 期望 writer.Aborted")
	}
}

func TestHandleEOF_Complete(t *testing.T) {
	m := newTestManagedTask()
	w := &fakeStoreWriter{}
	s := newStream(entity.StoreTypeMain, entity.GenerationDownloaded, 10, nil, w)
	s.written = 10 // 已写满

	res := s.handleEOF(m)
	if res.kind != resultOK {
		t.Fatalf("期望 resultOK, 实际 %v", res.kind)
	}
	if !w.completed {
		t.Fatalf("期望 writer.Completed")
	}
}

func TestHandleEOF_Incomplete(t *testing.T) {
	m := newTestManagedTask()
	w := &fakeStoreWriter{}
	s := newStream(entity.StoreTypeMain, entity.GenerationDownloaded, 100, nil, w)
	s.written = 30 // 不足

	res := s.handleEOF(m)
	if res.kind != resultFailed {
		t.Fatalf("期望 resultFailed(不完整), 实际 %v", res.kind)
	}
	if !w.aborted {
		t.Fatalf("不完整期望 writer.Aborted")
	}
}

func TestHandlePause(t *testing.T) {
	w := &fakeStoreWriter{}
	s := newStream(entity.StoreTypeMain, entity.GenerationDownloaded, 10, io.NopCloser(bytes.NewReader([]byte{})), w)

	res := s.handlePause(nil)
	if res.kind != resultPaused {
		t.Fatalf("期望 resultPaused, 实际 %v", res.kind)
	}
	if w.syncN == 0 || !w.closed {
		t.Fatalf("期望 Sync+Close: syncN=%d closed=%v", w.syncN, w.closed)
	}
	if streamState(s.state.Load()) != streamPaused {
		t.Fatalf("期望 stream 状态 paused")
	}
}

// ==== D. downloadLoop 多流聚合 ====

func TestDownloadLoop_AllComplete(t *testing.T) {
	nopLogger()
	m := newTestManagedTask()
	m.task.PendingResourceID = sql.NullInt64{Int64: 99, Valid: true}
	var resourceCleared bool
	m.onResourceIDUpdate = func(_ int64, id sql.NullInt64) {
		if !id.Valid {
			resourceCleared = true
		}
	}

	data := bytes.Repeat([]byte("a"), 50)
	w1, w2 := &fakeStoreWriter{}, &fakeStoreWriter{}
	m.streams = []*streamController{
		newStream(entity.StoreTypeMain, entity.GenerationDownloaded, 50, io.NopCloser(bytes.NewReader(data)), w1),
		newStream(entity.StoreTypeThumbnail, entity.GenerationDerived, 0, io.NopCloser(bytes.NewReader([]byte("thumb"))), w2),
	}

	res := m.downloadLoop()
	if res != runResultDone {
		t.Fatalf("期望 runResultDone, 实际 %v", res)
	}
	if m.GetState() != TaskStateFinished {
		t.Fatalf("期望 Finished, 实际 %s", taskStateName(m.GetState()))
	}
	if !w1.completed || !w2.completed {
		t.Fatalf("期望两轨均 Completed: w1=%v w2=%v", w1.completed, w2.completed)
	}
	if !resourceCleared {
		t.Fatalf("期望完成后清空 PendingResourceID")
	}
}

func TestDownloadLoop_OneFails(t *testing.T) {
	nopLogger()
	m := newTestManagedTask()
	m.task.PendingResourceID = sql.NullInt64{Int64: 99, Valid: true}
	m.onResourceIDUpdate = func(_ int64, _ sql.NullInt64) {}

	wMain, wFail := &fakeStoreWriter{}, &fakeStoreWriter{}
	m.streams = []*streamController{
		newStream(entity.StoreTypeMain, entity.GenerationDownloaded, 50, io.NopCloser(bytes.NewReader(bytes.Repeat([]byte("a"), 50))), wMain),
		newStream(entity.StoreTypeThumbnail, entity.GenerationDerived, 0, &errorReadCloser{err: errors.New("thumb gen failed")}, wFail),
	}

	res := m.downloadLoop()
	if res != runResultDone {
		t.Fatalf("期望 runResultDone, 实际 %v", res)
	}
	if m.GetState() != TaskStateFailed {
		t.Fatalf("期望 Failed, 实际 %s", taskStateName(m.GetState()))
	}
	// 已完成轨保留(Completed 非 Aborted),失败轨 Aborted
	if !wMain.completed || wMain.aborted {
		t.Fatalf("主轨应保留: completed=%v aborted=%v", wMain.completed, wMain.aborted)
	}
	if !wFail.aborted {
		t.Fatalf("失败轨应 Aborted")
	}
}

func TestDownloadLoop_PauseBroadcast(t *testing.T) {
	nopLogger()
	m := newTestManagedTask()
	m.task.PendingResourceID = sql.NullInt64{Int64: 99, Valid: true}
	m.onResourceIDUpdate = func(_ int64, _ sql.NullInt64) {}

	// 两轨用 gatedReader:第一次 Read 阻塞等 first,精确卡 softPause 时序
	gr1 := &gatedReader{data: bytes.Repeat([]byte("a"), 10), first: make(chan struct{})}
	gr2 := &gatedReader{data: bytes.Repeat([]byte("b"), 10), first: make(chan struct{})}
	w1, w2 := &fakeStoreWriter{}, &fakeStoreWriter{}
	m.streams = []*streamController{
		newStream(entity.StoreTypeMain, entity.GenerationDownloaded, 100, gr1, w1),
		newStream(entity.StoreTypeVideoTrack, entity.GenerationDownloaded, 100, gr2, w2),
	}

	done := make(chan runResult, 1)
	go func() { done <- m.downloadLoop() }()

	// 模拟 cmdWatcher 收到 cmdPause:置 softPause(不取消 runCtx),再许可两轨在途 Read 返回
	m.softPause.Store(true)
	close(gr1.first)
	close(gr2.first)

	select {
	case res := <-done:
		if res != runResultPaused {
			t.Fatalf("期望 runResultPaused, 实际 %v", res)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("downloadLoop 未退出(softPause 路径未生效)")
	}
	if m.GetState() != TaskStatePaused {
		t.Fatalf("期望 Paused, 实际 %s", taskStateName(m.GetState()))
	}
	// 优雅暂停不取消 runCtx(与立即切断路径的区别)
	if m.runCtx.Err() != nil {
		t.Fatalf("优雅暂停不应取消 runCtx")
	}
	// 两轨在途数据落盘 + Sync+Close(非 Complete/Abort)
	for i, w := range []*fakeStoreWriter{w1, w2} {
		if !w.closed || w.completed || w.aborted {
			t.Fatalf("轨 %d 应为暂停态: closed=%v completed=%v aborted=%v", i, w.closed, w.completed, w.aborted)
		}
		if w.buf.Len() != 10 {
			t.Fatalf("轨 %d 期望在途 10 字节落盘, 实际 %d", i, w.buf.Len())
		}
	}
}

func TestProgressAggregation(t *testing.T) {
	m := newTestManagedTask()
	var total, finished int64
	m.onProgress = func(_ int64, tt, f int64) { total, finished = tt, f }

	s1 := newStream(entity.StoreTypeMain, entity.GenerationDownloaded, 100, nil, &fakeStoreWriter{})
	s2 := newStream(entity.StoreTypeThumbnail, entity.GenerationDerived, 5, nil, &fakeStoreWriter{})
	s1.written = 60
	s2.written = 5
	m.streams = []*streamController{s1, s2}

	// totalStreamSize 只计 size>0 的轨(derived size=5 计入;若 derived size=0 不计)
	if got := m.totalStreamSize(); got != 105 {
		t.Fatalf("totalStreamSize 期望 105, 实际 %d", got)
	}
	m.reportProgress()
	if total != 105 || finished != 65 {
		t.Fatalf("reportProgress 期望 total=105 finished=65, 实际 total=%d finished=%d", total, finished)
	}
}

// TestDrainUnselectedReaders_NoDeadlock 回归:Redownload 单独 role 时,插件 Start 返回全部 spec,
// 过滤掉的 role 的 io.Pipe 若无人消费会永久阻塞 demux(多流复用一条 gRPC stream),拖死全部轨道。
// drainUnselectedReaders 排空未选 reader,demux 必须能完成。
func TestDrainUnselectedReaders_NoDeadlock(t *testing.T) {
	mainPr, mainPw := io.Pipe()
	thumbPr, thumbPw := io.Pipe()
	all := []*sdkdto.StoreSpec{
		{Role: entity.StoreTypeMain, ReadCloser: mainPr},
		{Role: entity.StoreTypeThumbnail, ReadCloser: thumbPr},
	}
	selected := []*sdkdto.StoreSpec{all[0]} // 仅选 main,thumbnail 未选

	m := &ManagedTask{}

	// demux 写入:先写未选的 thumbnail(回归 bug 触发点),再写 main,各关闭
	demuxDone := make(chan struct{})
	go func() {
		defer close(demuxDone)
		_, _ = thumbPw.Write([]byte("thumb-data"))
		_ = thumbPw.Close()
		_, _ = mainPw.Write([]byte("main-data"))
		_ = mainPw.Close()
	}()

	// 排空未选 reader(thumbnail)——修复的核心
	m.drainUnselectedReaders(all, selected)

	// 消费已选 main
	mainBuf, _ := io.ReadAll(mainPr)

	// demux 必须在超时前完成(无 drain 则 thumbnail pipe 写入永久阻塞)
	select {
	case <-demuxDone:
	case <-time.After(2 * time.Second):
		t.Fatal("demux 阻塞:未选 role 的 io.Pipe 无人消费(回归 bug 未修复)")
	}

	if string(mainBuf) != "main-data" {
		t.Fatalf("main 数据期望 main-data, 实际 %q", mainBuf)
	}
}

// 防止编译器误报未使用(atomic 在 newTestManagedTask 间接用,这里显式引用以稳定导入)
var _ = atomic.Int32{}
var _ = sync.Mutex{}

// TestPrepareForResume_NoBlockAndResetsFields 回归:prepareForResume 必须立即返回(不阻塞),并完整重置执行期可变状态(streams/resumeFromDB)。
// actor 模型下 m.ctx 是 actor 主 ctx(一生一灭,不重建),runCtx 由 handleRunCmd 派生;此处只验证执行期可变字段重置。
func TestPrepareForResume_NoBlockAndResetsFields(t *testing.T) {
	m := newTestManagedTask()
	m.streams = []*streamController{{role: "stale"}}
	m.task.PendingResourceID = sql.NullInt64{Int64: 99, Valid: true}

	// 应立即返回,不阻塞
	done := make(chan struct{})
	go func() {
		m.prepareForResume()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("prepareForResume 不应阻塞")
	}

	if m.streams != nil {
		t.Fatal("streams 应置 nil")
	}
	if !m.resumeFromDB {
		t.Fatal("PendingResourceID 有效时应置 resumeFromDB=true")
	}
}

// TestDispatch_Exclusive 回归:dispatch 的 actorStarted CAS 保证"一任务一 actor"。
// 首次 dispatch 成功(actorStarted false→true + 投 cmdStart);重复 dispatch 幂等返回 false。
func TestDispatch_Exclusive(t *testing.T) {
	mgr := NewManager(2, nil, nil, nil, nil)
	defer func() {
		close(mgr.closeCh)
		<-mgr.flushDone
	}()

	task := newTestManagedTask()
	// 不启动 actor:本测试只验证 actorStarted CAS 幂等,不实际消费 cmdCh

	if !mgr.dispatch(task) {
		t.Fatal("首次 dispatch 应 claim 成功")
	}
	if mgr.dispatch(task) {
		t.Fatal("已 dispatched(actorStarted=true 且非 Paused/Pausing),重复 dispatch 应返回 false(幂等)")
	}
}

// TestNewManagedTask_ActorStartedZero 回归:NewManagedTask 不得对 actorStarted 赋值。
// actorStarted 是 dispatch 的 CAS(false→true) 首派守卫(初值须为 false);创建期 Store(true)
// 会使守卫永远失败、cmdStart 不投递,未命中查重的新任务卡死在 Created 永不执行。
// 本测试经 NewManagedTask 构造(生产路径),区别于 newTestManagedTask 的字面量构造——
// 后者绕过 NewManagedTask,无法捕获创建期的错误赋值(正是此前 bug 的潜伏原因)。
func TestNewManagedTask_ActorStartedZero(t *testing.T) {
	mgr := NewManager(2, nil, nil, nil, nil)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()

	task := entity.NewTask()
	task.PluginPublicID = sql.NullString{String: "test-plugin", Valid: true}
	task.TaskName = sql.NullString{String: "t", Valid: true}

	mt := NewManagedTask(1, 0, task, nil, nil, mgr, make(chan struct{}, 1))
	mt.cancel()
	<-mt.actorDone

	if mt.actorStarted.Load() {
		t.Fatal("NewManagedTask 后 actorStarted 必须为 false(零值):它是 dispatch 的 CAS 首派守卫,创建期赋 true 会令新任务永不启动")
	}
}

// TestPauseTaskTree_PostsCmdPause 回归:PauseTaskTree 对非终态子任务投 cmdPause(actor 命令队列保证 pause 覆盖陈旧 resume)。
// actor 模型下不再用 pendingResume 标志;本测试验证 cmdPause 被投递到子任务 cmdCh。
func TestPauseTaskTree_PostsCmdPause(t *testing.T) {
	mgr := NewManager(2, nil, nil, nil, nil)
	defer func() {
		close(mgr.closeCh)
		<-mgr.flushDone
	}()

	child := newTestManagedTask()
	child.setState(TaskStatePaused)

	parent := NewParentTask(254, "parent")
	parent.AddChild(child)
	mgr.mu.Lock()
	mgr.parentMap[254] = parent
	mgr.mu.Unlock()

	if err := mgr.PauseTaskTree(context.Background(), 254, false); err != nil {
		t.Fatalf("PauseTaskTree 失败: %v", err)
	}

	// cmdPause 应被投递到 child.cmdCh(不启动 actor,直接读 channel)
	select {
	case cmd := <-child.cmdCh:
		if cmd.kind != cmdPause {
			t.Fatalf("期望 cmdPause, 实际 %d", cmd.kind)
		}
	default:
		t.Fatal("PauseTaskTree 应投递 cmdPause 到子任务")
	}
}

// ==== 优雅暂停(softPause + drain 超时兜底)====

// TestCopyLoop_SoftPause_DrainsInflight 验证优雅暂停核心:本轮 Read 的在途数据先落盘,
// copyLoop 随后退出,且不再 Read(不发起新 PullRequest 拉取新数据)。
func TestCopyLoop_SoftPause_DrainsInflight(t *testing.T) {
	m := newTestManagedTask()
	w := &fakeStoreWriter{}
	// 在途数据 50 字节;size=100 → 若 softPause 未生效会继续读到 EOF 判不完整 Failed
	gr := &gatedReader{data: bytes.Repeat([]byte("x"), 50), first: make(chan struct{})}
	s := newStream(entity.StoreTypeMain, entity.GenerationDownloaded, 100, gr, w)

	done := make(chan streamResult, 1)
	go func() { done <- s.copyLoop(m) }()

	// 置 softPause(模拟 cmdWatcher 收到 cmdPause),再许可在途 Read 返回。
	// Store 在 close 前,经 channel happens-before 传播到 copyLoop 的 softPause.Load
	m.softPause.Store(true)
	close(gr.first)

	select {
	case res := <-done:
		if res.kind != resultPaused {
			t.Fatalf("期望 resultPaused(在途落盘后退出), 实际 %v (msg=%s)", res.kind, res.errMsg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("copyLoop 未退出(softPause 路径未生效)")
	}
	if w.buf.Len() != 50 {
		t.Fatalf("期望在途 50 字节落盘, 实际 %d", w.buf.Len())
	}
	if w.aborted {
		t.Fatalf("softPause 不应 Abort")
	}
	if !w.closed {
		t.Fatalf("softPause 应 Sync+Close")
	}
	// 仅 Read 一次:在途往返已落盘即退出,未发起新 PullRequest
	if gr.readN != 1 {
		t.Fatalf("期望仅 Read 1 次(在途), 实际 %d(发了新 PullRequest)", gr.readN)
	}
}

// TestCmdWatcher_PauseStageAware 验证 cmdWatcher 对 cmdPause 的阶段感知分流:
// downloadLoop 阶段(inDownload=true)走优雅暂停(softPause+drainTimer,不取消 runCtx);
// setup 阶段(inDownload=false)立即 runCancel(无在途 chunk,快速中断)。cmdStop 不论阶段都立即取消。
func TestCmdWatcher_PauseStageAware(t *testing.T) {
	// downloadLoop 阶段:cmdPause 走优雅暂停——置 softPause + drainTimer,不取消 runCtx
	m := newTestManagedTask()
	m.inDownload.Store(true) // 模拟已进入 downloadLoop
	stop := make(chan struct{})
	go m.cmdWatcher(stop)

	m.cmdCh <- taskCmd{kind: cmdPause}
	time.Sleep(50 * time.Millisecond) // 等 watcher 处理命令

	if !m.softPause.Load() {
		t.Fatal("downloadLoop 阶段 cmdPause 应置 softPause=true")
	}
	if m.runCtx.Err() != nil {
		t.Fatal("downloadLoop 阶段 cmdPause(优雅暂停)不应立即取消 runCtx")
	}
	if m.drainTimer == nil {
		t.Fatal("downloadLoop 阶段 cmdPause 应启动 drainTimer")
	}
	m.drainTimer.Stop() // 防 2s 后触发干扰后续断言
	close(stop)

	// setup 阶段:cmdPause 立即 runCancel(无在途 chunk,快速中断)
	m2 := newTestManagedTask() // inDownload 保持 false(setup)
	stop2 := make(chan struct{})
	go m2.cmdWatcher(stop2)
	m2.cmdCh <- taskCmd{kind: cmdPause}
	time.Sleep(50 * time.Millisecond)
	if m2.runCtx.Err() == nil {
		t.Fatal("setup 阶段 cmdPause 应立即取消 runCtx(快速中断)")
	}
	if m2.softPause.Load() {
		t.Fatal("setup 阶段 cmdPause 不应置 softPause(走 runCancel,非 drain)")
	}
	close(stop2)

	// cmdStop:不论阶段都立即取消 runCtx(watcher 随前述 return,用新 task)
	m3 := newTestManagedTask()
	m3.inDownload.Store(true) // 即使 downloadLoop 阶段,cmdStop 也立即取消
	stop3 := make(chan struct{})
	go m3.cmdWatcher(stop3)
	m3.cmdCh <- taskCmd{kind: cmdStop}
	time.Sleep(50 * time.Millisecond)
	if m3.runCtx.Err() == nil {
		t.Fatal("cmdStop 应立即取消 runCtx(不论阶段)")
	}
	close(stop3)
}

// TestDrainTimeout_ForceCancel 验证 drain 超时兜底:在途 Read 阻塞不完成时,强制 runCancel(等价 drainTimer 到期)
// 使 copyLoop 走 runCtx 取消的有损路径退出。真实定时器为标准库 time.AfterFunc(2s),此处手动触发以避免测试等待。
func TestDrainTimeout_ForceCancel(t *testing.T) {
	m := newTestManagedTask()
	w := &fakeStoreWriter{}
	// data 为空:第一次 Read 即阻塞等 runCtx,模拟插件卡死/在途不完成
	cr := &ctxAwareReader{ctx: m.runCtx, blockedCh: make(chan struct{})}
	s := newStream(entity.StoreTypeMain, entity.GenerationDownloaded, 100, cr, w)

	m.softPause.Store(true) // 优雅暂停已发起,转入 drain 等待

	done := make(chan streamResult, 1)
	go func() { done <- s.copyLoop(m) }()

	<-cr.blockedCh // copyLoop 阻塞在在途 Read

	// 模拟 drainTimer 到期:强制取消 runCtx
	m.runCancel()

	select {
	case res := <-done:
		// runCtx 取消 → Read 返回 error → 有损路径 handlePause(保留文件)
		if res.kind != resultPaused {
			t.Fatalf("期望 resultPaused(超时兜底有损路径), 实际 %v", res.kind)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("copyLoop 未退出(drain 超时兜底未生效)")
	}
	if !w.closed {
		t.Fatal("兜底路径应 Sync+Close 保留文件")
	}
}

// TestPrepareForResume_ResetsSoftPause 验证 prepareForResume 清除上一轮暂停标志,
// 防 Resume 后 copyLoop 误命中 softPause 立即退出。
func TestPrepareForResume_ResetsSoftPause(t *testing.T) {
	m := newTestManagedTask()
	m.softPause.Store(true)
	m.drainTimer = time.AfterFunc(time.Hour, func() {})

	m.prepareForResume()

	if m.softPause.Load() {
		t.Fatal("prepareForResume 应重置 softPause=false")
	}
	if m.drainTimer != nil {
		t.Fatal("prepareForResume 应清空 drainTimer")
	}
}
