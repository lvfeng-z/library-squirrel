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

	// 两轨都用 ctxAwareReader:耗尽 data 后阻塞,runCtx 取消后返回错误 → copyLoop 走 runCtx.Done/handlePause
	cr1 := &ctxAwareReader{data: bytes.Repeat([]byte("a"), 10), ctx: m.runCtx, blockedCh: make(chan struct{})}
	cr2 := &ctxAwareReader{data: bytes.Repeat([]byte("b"), 10), ctx: m.runCtx, blockedCh: make(chan struct{})}
	w1, w2 := &fakeStoreWriter{}, &fakeStoreWriter{}
	m.streams = []*streamController{
		newStream(entity.StoreTypeMain, entity.GenerationDownloaded, 100, cr1, w1), // size 100 > 10 → 若不暂停会判不完整
		newStream(entity.StoreTypeVideoTrack, entity.GenerationDownloaded, 100, cr2, w2),
	}

	done := make(chan runResult, 1)
	go func() { done <- m.downloadLoop() }()

	// 等待两轨 reader 都阻塞(已耗尽 data,等待 runCtx 取消)
	<-cr1.blockedCh
	<-cr2.blockedCh

	// 模拟 watcher runCancel:cancel runCtx → copyLoop 走 runCtx.Done → handlePause(保留文件)
	m.cancel()

	res := <-done
	if res != runResultPaused {
		t.Fatalf("期望 runResultPaused, 实际 %v", res)
	}
	if m.GetState() != TaskStatePaused {
		t.Fatalf("期望 Paused, 实际 %s", taskStateName(m.GetState()))
	}
	// 两轨均 Sync+Close(暂停保留,非 Complete/Abort)
	for i, w := range []*fakeStoreWriter{w1, w2} {
		if !w.closed || w.completed || w.aborted {
			t.Fatalf("轨 %d 应为暂停态(closed 非完成/中止): closed=%v completed=%v aborted=%v", i, w.closed, w.completed, w.aborted)
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
