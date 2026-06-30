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

// pausableReader 先返回 data,耗尽后阻塞直到 pauseCh 关闭再返回 EOF(模拟插件暂停关闭上游)
type pausableReader struct {
	data      []byte
	off       int
	pauseCh   chan struct{}
	blockedCh chan struct{} // reader 耗尽阻塞时关闭,供测试同步
	closed    int
}

func (r *pausableReader) Read(p []byte) (int, error) {
	select {
	case <-r.pauseCh:
		return 0, io.EOF
	default:
	}
	if r.off >= len(r.data) {
		close(r.blockedCh)
		<-r.pauseCh // 阻塞等待暂停信号
		return 0, io.EOF
	}
	n := copy(p, r.data[r.off:])
	r.off += n
	return n, nil
}
func (r *pausableReader) Close() error { r.closed++; return nil }

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
	if r.off >= len(r.data) {
		close(r.blockedCh)
		<-r.ctx.Done()
		return 0, nil // 不报错,交给 copyLoop 的 select 处理 ctx.Done
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

// newTestManagedTask 构造最小可测 ManagedTask(ctx/pauseCh/state/done/task)
func newTestManagedTask() *ManagedTask {
	ctx, cancel := context.WithCancel(context.Background())
	m := &ManagedTask{
		taskId: 1,
		ctx:    ctx,
		cancel: cancel,
		pauseCh: make(chan struct{}),
		done:   make(chan struct{}),
		task:   entity.NewTask(),
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
	// 模拟 Stop:ctx 取消时 isStopping()=true → Abort(删文件)
	m.state.Store(int32(TaskStateStopping))
	w := &fakeStoreWriter{}
	cr := &ctxAwareReader{data: bytes.Repeat([]byte("c"), 50), ctx: m.ctx, blockedCh: make(chan struct{})}
	s := newStream(entity.StoreTypeMain, entity.GenerationDownloaded, 100, cr, w)

	done := make(chan streamResult, 1)
	go func() { done <- s.copyLoop(m) }()

	<-cr.blockedCh
	m.cancel()

	res := <-done
	if res.kind != resultCanceled {
		t.Fatalf("期望 resultCanceled, 实际 %v", res.kind)
	}
	if !w.aborted {
		t.Fatalf("Stop 取消期望 writer.Aborted")
	}
}

// TestCopyLoop_PauseCancelPreservesFile 回归:setup 阶段 pause 取消 ctx 时,
// copyLoop 应 Sync+Close 保留文件(不 Abort),否则下次 resume offset=0 → 进度倒退
func TestCopyLoop_PauseCancelPreservesFile(t *testing.T) {
	m := newTestManagedTask()
	// pause 取消 ctx:state=Processing(非 Stopping) → 应保留文件
	w := &fakeStoreWriter{}
	cr := &ctxAwareReader{data: bytes.Repeat([]byte("c"), 50), ctx: m.ctx, blockedCh: make(chan struct{})}
	s := newStream(entity.StoreTypeMain, entity.GenerationDownloaded, 100, cr, w)

	done := make(chan streamResult, 1)
	go func() { done <- s.copyLoop(m) }()

	<-cr.blockedCh
	m.cancel()

	res := <-done
	if res.kind != resultCanceled {
		t.Fatalf("期望 resultCanceled, 实际 %v", res.kind)
	}
	if w.aborted {
		t.Fatalf("pause 取消不应 Abort(应保留文件防进度倒退)")
	}
	if !w.closed {
		t.Fatalf("pause 取消应 Sync+Close 保留文件")
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

	// 两轨都用 pausableReader:耗尽后阻塞,关闭 pauseCh 后返回 EOF → 暂停
	pr1 := &pausableReader{data: bytes.Repeat([]byte("a"), 10), pauseCh: make(chan struct{}), blockedCh: make(chan struct{})}
	pr2 := &pausableReader{data: bytes.Repeat([]byte("b"), 10), pauseCh: make(chan struct{}), blockedCh: make(chan struct{})}
	w1, w2 := &fakeStoreWriter{}, &fakeStoreWriter{}
	m.streams = []*streamController{
		newStream(entity.StoreTypeMain, entity.GenerationDownloaded, 100, pr1, w1), // size 100 > 10 → 若不暂停会判不完整
		newStream(entity.StoreTypeVideoTrack, entity.GenerationDownloaded, 100, pr2, w2),
	}

	done := make(chan runResult, 1)
	go func() { done <- m.downloadLoop() }()

	// 等待两轨 reader 都阻塞(已耗尽 data,等待暂停信号)
	<-pr1.blockedCh
	<-pr2.blockedCh

	// 模拟 Pause():置 Pausing + 关闭 pauseCh 广播 + 各 reader 的 pauseCh
	m.state.Store(int32(TaskStatePausing))
	close(m.pauseCh)
	close(pr1.pauseCh)
	close(pr2.pauseCh)

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

// TestPrepareForResume_WaitsForGoroutineExit 回归:prepareForResume 必须等待旧 executeTask goroutine
// 退出后才改写共享状态,否则频繁启停下旧 goroutine 与新 goroutine 竞争 streams/ctx → 并行度下降/状态错乱
func TestPrepareForResume_WaitsForGoroutineExit(t *testing.T) {
	m := newTestManagedTask()
	// 模拟一个仍在运行的旧 goroutine:runExited 为开通道
	running := make(chan struct{})
	m.runExited = running

	proceeded := make(chan struct{})
	go func() {
		m.prepareForResume()
		close(proceeded)
	}()

	// 旧 goroutine 未退出时,prepareForResume 应阻塞
	select {
	case <-proceeded:
		t.Fatal("prepareForResume 不应在旧 goroutine 退出前返回")
	case <-time.After(50 * time.Millisecond):
	}

	// 旧 goroutine 退出
	close(running)

	select {
	case <-proceeded:
	case <-time.After(2 * time.Second):
		t.Fatal("旧 goroutine 退出后 prepareForResume 应及时返回")
	}
}
