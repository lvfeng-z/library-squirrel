package download

// 多轨流管理与下载循环：单流控制器（read→write→累计）+ 多流并发聚合 + 软暂停/取消响应。
// 软暂停以信号通道消费（控制面进入软暂停时 close 广播，下载循环非阻塞探测）；进入/离开
// 下载循环经 MarkDrainPhase 上报，控制面命令监听据此分流暂停处置（可排空阶段走软暂停，
// 其余阶段立即取消）；排空超时的强制取消归控制面，本侧只消费取消结果。

import (
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/persistentStore"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// streamState 单条流的执行状态
type streamState int32

const (
	streamDownloading streamState = iota
	streamCompleted
	streamPaused
	streamFailed
	streamCanceled
)

// streamResultKind 单条流 copyLoop 的结果分类
type streamResultKind int

const (
	resultOK streamResultKind = iota // 完成
	resultPaused
	resultCanceled
	resultFailed
)

// streamResult 单条流 copyLoop 的结果
type streamResult struct {
	kind   streamResultKind
	errMsg string
}

// loopResult 下载循环的收口分类
type loopResult int

const (
	loopDone   loopResult = iota // 终态已定（成功/失败经 handle 上报）
	loopPaused                   // 暂停中断（未上报终态，交控制面按暂停收敛）
)

// streamController 管理单个 store 的传输(downloaded/derived 通用:reader→store 拷贝)
type streamController struct {
	role        string // store_type(main/thumbnail/videoTrack/...)
	generation  string // downloaded | derived
	format      string // 文件扩展名
	size        int64  // 远程大小;-1 未知
	suggestName string // 插件建议文件名
	continuable bool   // 是否支持续传(derived 恒为 false)

	reader        io.ReadCloser               // 资源数据流(由调用方关闭)
	storeWriter   persistentStore.StoreWriter // 当前写入的 StoreWriter
	storeId       int64                       // PersistentStore 记录 ID
	relPath       string                      // StoreStream 的相对路径(事务回滚/清理用)
	written       int64                       // 已写入字节数(mu 保护)
	initialOffset int64                       // 续传初始偏移(恢复时 = writeOffset;新建轨为 0),进度分母补全完整大小
	state         atomic.Int32                // streamState
	mu            sync.Mutex                  // 保护 written 与 drain 期间的 reader/storeWriter 访问
}

// newStreamController 构建单流控制器
func newStreamController(spec *sdkdto.StoreSpec, storeId int64, writer persistentStore.StoreWriter, relPath string) *streamController {
	sc := &streamController{
		role:        spec.Role,
		generation:  spec.Generation,
		format:      spec.Format,
		size:        spec.Size,
		suggestName: spec.SuggestName,
		storeWriter: writer,
		storeId:     storeId,
		relPath:     relPath,
		reader:      spec.ReadCloser,
	}
	if spec.Continuable != nil {
		sc.continuable = *spec.Continuable
	}
	return sc
}

// closeStreamReaders 关闭全部流的 reader
func (sess *execSession) closeStreamReaders() {
	for _, s := range sess.streams {
		if s.reader != nil {
			s.reader.Close()
		}
	}
}

// anyStreamPaused 是否有任意流进入 paused 状态
func (sess *execSession) anyStreamPaused() bool {
	for _, s := range sess.streams {
		if streamState(s.state.Load()) == streamPaused {
			return true
		}
	}
	return false
}

// totalStreamSize 全部流的远程大小之和(进度总量)
func (sess *execSession) totalStreamSize() int64 {
	var total int64
	for _, s := range sess.streams {
		if s.size > 0 {
			total += s.size
		}
	}
	return total
}

// reportProgress 汇总全部流进度并经 handle 上报
func (sess *execSession) reportProgress() {
	var total, finished int64
	for _, s := range sess.streams {
		// size<=0 的轨(如 document lazy 产物,生成前大小未知)不参与进度核算:
		// 既不计入 total 也不计入 finished,避免 finished 超 total 导致进度 >100%
		if s.size <= 0 {
			continue
		}
		total += s.size // spec.Size 为完整大小(retry_reader 206 据 Content-Range 还原)
		s.mu.Lock()
		finished += s.written
		s.mu.Unlock()
	}
	sess.handle.ReportProgress(total, finished)
}

// runAborted 本次执行是否已被取消(暂停/停止均经 runCtx 取消表达;控制面命令监听在长任务
// 执行期间只取消 runCtx 并暂存命令,状态处置在执行返回后进行)
func (sess *execSession) runAborted() bool {
	return sess.runCtx().Err() != nil
}

// downloadLoop 多流并发下载循环:每条流一个 goroutine 跑 read→write→累计。
// 全部完成 → 重算资源完整度、清 pending 后经 handle 上报成功终态;任一失败 → 清 pending 后
// 经 handle 上报失败终态(保留已完成轨的 store);任一流暂停 → 不上报终态返回,交控制面按暂停收敛
func (sess *execSession) downloadLoop() loopResult {
	// 上报进入可排空阶段:控制面命令监听据此让暂停走软暂停(排空在途再停),离开时上报退出
	sess.handle.MarkDrainPhase(true)
	defer sess.handle.MarkDrainPhase(false)
	// 关闭 reader(各 spec reader 由本方法负责)
	defer sess.closeStreamReaders()

	downloadStart := time.Now()
	totalSize := sess.totalStreamSize()

	var wg sync.WaitGroup
	var failedMsg atomic.Value // string
	var hasFailed atomic.Bool
	var hasCanceled atomic.Bool

	for _, s := range sess.streams {
		wg.Add(1)
		go func(s *streamController) {
			defer wg.Done()
			res := s.copyLoop(sess)
			switch res.kind {
			case resultOK:
				// 正常完成
			case resultPaused:
				// 由 downloadLoop 统一判定
			case resultCanceled:
				hasCanceled.Store(true)
			case resultFailed:
				hasFailed.Store(true)
				if res.errMsg != "" {
					failedMsg.Store(res.errMsg)
				}
			}
		}(s)
	}
	wg.Wait()

	logger.Log.Infof("[Download] downloadLoop 结束: taskId=%d, totalSize=%d, elapsed=%v", sess.taskId, totalSize, time.Since(downloadStart))

	// 暂停优先(流任一进入 paused 即任务级暂停):不上报终态,交控制面置 Paused/通知插件
	if sess.anyStreamPaused() {
		return loopPaused
	}
	if hasCanceled.Load() {
		// 流被取消且 runCtx 已取消:视为中断,不上报终态交控制面(停止的终态收口由
		// 控制面停止命令处置完成)
		if sess.runAborted() {
			return loopPaused
		}
		return loopDone
	}
	if hasFailed.Load() {
		msg := "下载失败"
		if v := failedMsg.Load(); v != nil {
			if s, ok := v.(string); ok && s != "" {
				msg = s
			}
		}
		// 失败收口：先关闭流写入句柄释放文件锁，新建 store 的丢弃与被软删旧行的复活统一由
		// 控制面 setFailed 单点按登记清单执行（替换场景丢弃新建行，非替换保留已下载成果）；
		// 失败终态清 pending(failTerminal 内)后上报失败
		sess.closeStreamWriters()
		sess.failTerminal(msg)
		return loopDone
	}
	// 全部完成:先计算并持久化资源完整度(此刻所有 store 已 Complete),再清 pending + 上报成功终态
	sess.markResourceComplete(sess.runCtx(), sess.currentResourceId)
	sess.clearPendingResourceID()
	sess.handle.Finish()
	return loopDone
}

// copyLoop 单流读取循环:read→write→累计,响应暂停/取消/EOF
func (s *streamController) copyLoop(sess *execSession) streamResult {
	buf := make([]byte, 32*1024)
	for {
		select {
		case <-sess.runCtx().Done():
			// runCtx 取消(控制面暂停/停止时取消在途 reader):统一保留文件。
			// 停止的文件删除由控制面停止命令对流集合的后置清理处理
			return s.handlePause(buf)
		default:
		}

		n, readErr := s.reader.Read(buf)
		if n > 0 {
			written, writeErr := s.storeWriter.Write(buf[:n])
			if written > 0 {
				s.mu.Lock()
				s.written += int64(written)
				s.mu.Unlock()
			}
			if writeErr != nil {
				logger.Log.Errorf("[Download] 任务 %d 写入文件失败(role=%s): %v", sess.taskId, s.role, writeErr)
				s.abort()
				s.state.Store(int32(streamFailed))
				return streamResult{kind: resultFailed, errMsg: fmt.Sprintf("写入文件失败: %v", writeErr)}
			}
			sess.reportProgress()
		}
		// 优雅暂停:软暂停广播已到达且本轮 Read 的数据已落盘,退出收尾。传 nil 跳过 handlePause 的 drain——
		// pull 模型下 drain 会再 Read(即发起新 PullRequest)拉取新数据,违背"暂停只阻止新数据发起"。
		// runCtx 未取消守卫区分正常排空与超时兜底:控制面排空超时已强制取消时,由下方 readErr 分支
		// 走有损保留路径。在途读取的错误形态分流:无错误与 EOF 均视为正常排空直接暂停;其余传输错误
		// 仍按暂停收敛(用户暂停意图优先),Warn 保留错误细节——暂停窗口插件死亡可诊断
		if sess.softPauseReceived() && sess.runCtx().Err() == nil {
			if readErr != nil && readErr != io.EOF {
				logger.Log.Warnf("[Download] 任务 %d 软暂停窗口出现传输错误(role=%s),按暂停收敛: %v", sess.taskId, s.role, readErr)
			}
			return s.handlePause(nil)
		}
		if readErr != nil {
			if readErr == io.EOF {
				return s.handleEOF(sess)
			}
			// runCtx 取消导致的读取错误(gRPC stream cancel):视为中断,保留文件,不 Failed
			if sess.runCtx().Err() != nil {
				return s.handlePause(buf)
			}
			// 非 EOF 非 runCtx 取消:真正的读取失败
			logger.Log.Errorf("[Download] 任务 %d 下载读取失败(role=%s): %v", sess.taskId, s.role, readErr)
			s.abort()
			s.state.Store(int32(streamFailed))
			return streamResult{kind: resultFailed, errMsg: fmt.Sprintf("下载读取失败: %v", readErr)}
		}
	}
}

// handleEOF 处理 reader EOF:runCtx 取消或软暂停进行中导致的 EOF 走暂停路径(保留文件),
// 否则校验完整性并完成
func (s *streamController) handleEOF(sess *execSession) streamResult {
	// runCtx 取消(控制面暂停/停止)导致上游关闭产生 EOF:视为中断,保留文件
	if sess.runCtx().Err() != nil {
		return s.handlePause(nil)
	}
	// 软暂停进行中导致上游关闭产生的 EOF:置 paused(暂停窗口的 EOF 属正常排空,不判完成)
	if sess.softPauseReceived() {
		return s.handlePause(nil)
	}
	// 完整性校验:downloaded 轨
	s.mu.Lock()
	written := s.written
	s.mu.Unlock()
	if s.generation == entity.GenerationDownloaded {
		switch {
		case s.size > 0 && written < s.size:
			// 已知预期大小且未下完
			logger.Log.Errorf("[Download] 任务 %d 下载不完整(role=%s): 已下载 %d / 预期 %d", sess.taskId, s.role, written, s.size)
			s.abort()
			s.state.Store(int32(streamFailed))
			return streamResult{kind: resultFailed, errMsg: fmt.Sprintf("%s 下载不完整: 已下载 %d / 预期 %d", s.role, written, s.size)}
		case written == 0:
			// 预期大小未知(spec.Size<=0)但一字节未写:空产物,判定不完整
			logger.Log.Errorf("[Download] 任务 %d 下载为空(role=%s): written=0", sess.taskId, s.role)
			s.abort()
			s.state.Store(int32(streamFailed))
			return streamResult{kind: resultFailed, errMsg: fmt.Sprintf("%s 下载为空(written=0)", s.role)}
		}
	}
	if err := s.storeWriter.Complete(); err != nil {
		logger.Log.Errorf("[Download] 任务 %d Complete 失败(role=%s): %v", sess.taskId, s.role, err)
		s.state.Store(int32(streamFailed))
		return streamResult{kind: resultFailed, errMsg: fmt.Sprintf("完成存储失败: %v", err)}
	}
	s.state.Store(int32(streamCompleted))
	return streamResult{kind: resultOK}
}

// handlePause 排空缓冲区、同步并关闭写入器、置 paused
func (s *streamController) handlePause(buf []byte) streamResult {
	if buf != nil {
		s.drain(buf)
	}
	s.storeWriter.Sync()
	s.storeWriter.Close()
	s.state.Store(int32(streamPaused))
	return streamResult{kind: resultPaused}
}

// drain 排空 reader 中所有已发送数据并写入文件,直到 reader 返回错误或 EOF
func (s *streamController) drain(buf []byte) {
	for {
		n, err := s.reader.Read(buf)
		if n > 0 {
			if written, writeErr := s.storeWriter.Write(buf[:n]); writeErr == nil && written > 0 {
				s.mu.Lock()
				s.written += int64(written)
				s.mu.Unlock()
			}
		}
		if err != nil {
			return
		}
	}
}

// abort 放弃写入:关闭句柄 + 删除文件 + 删 DB 记录
func (s *streamController) abort() {
	if s.storeWriter != nil {
		s.storeWriter.Abort()
	}
}
