package download

// 多轨流管理与下载循环：单流控制器（read→暂存写→累计）+ 多流并发聚合 + 软暂停/取消响应。
// 软暂停以信号通道消费（控制面进入软暂停时 close 广播，下载循环非阻塞探测）；进入/离开
// 下载循环经 MarkDrainPhase 上报，控制面命令监听据此分流暂停处置（可排空阶段走软暂停，
// 其余阶段立即取消）；排空超时的强制取消归控制面，本侧只消费取消结果。
// 暂存模式：流写入器为暂存文件（download 直接管理），全程零 DB 副作用；全部轨道写满后由
// 调用方执行提交点（rename 进 store/ + 建行挂载事务）。失败轨暂存文件保留（诊断可见、
// 重试重下覆盖），不再走「删文件+删行」的 Abort。

import (
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/entity"

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
	loopDone   loopResult = iota // 传输收口（成功转提交点/失败已上报终态）
	loopPaused                   // 暂停中断（未上报终态，交控制面按暂停收敛）
)

// streamController 管理单个轨道的传输(downloaded/derived 通用:reader→暂存文件拷贝)
type streamController struct {
	role        string // store_type(main/thumbnail/videoTrack/...)
	generation  string // downloaded | derived
	format      string // 文件扩展名
	size        int64  // 远程大小;-1/0 未知(document lazy 产物生成前大小未知)
	suggestName string // 插件建议文件名
	continuable bool   // 是否支持续传(derived 恒为 false)
	seq         int    // 同 role 内 0-based 序号(= store_seq；暂存文件名键/续传身份)

	expectedSha   string // 来源声明的期望 SHA256（空=未声明，跳过校验）
	reader        io.ReadCloser
	writer        *stagingWriter // 暂存文件写入器（含全量 sha256 流式哈希）
	stagingAbs    string         // 暂存文件绝对路径（absPath 域，仅 os.* 调用点；提交点 rename 源）
	finalRel      string         // 最终落盘 relPath（执行前解析派生；提交点 rename 目标与建行 file_path）
	finalName     string         // 最终文件名（提交点建行 file_name）
	actualSha     string         // 写满后实测 SHA256 hex（finalize 产出，提交点建行落列）
	written       int64          // 已写入字节数(mu 保护；续传恢复时=写入偏移起点，含前会话已落盘部分)
	initialOffset int64          // 续传初始偏移(恢复时 = writeOffset;新建轨为 0),进度分母补全完整大小
	state         atomic.Int32   // streamState
	mu            sync.Mutex     // 保护 written 与 drain 期间的 reader/writer 访问
}

// newStreamController 构建单流控制器（spec 身份字段 + 暂存/最终路径与期望哈希）
func newStreamController(spec *sdkdto.StoreSpec, seq int, writer *stagingWriter, stagingAbs, finalRel, finalName string) *streamController {
	sc := &streamController{
		role:        spec.Role,
		generation:  spec.Generation,
		format:      spec.Format,
		size:        spec.Size,
		suggestName: spec.SuggestName,
		seq:         seq,
		writer:      writer,
		stagingAbs:  stagingAbs,
		finalRel:    finalRel,
		finalName:   finalName,
		reader:      spec.ReadCloser,
	}
	if spec.ExpectedSha256 != nil {
		sc.expectedSha = *spec.ExpectedSha256
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

// downloadLoop 多流并发下载循环:每条流一个 goroutine 跑 read→暂存写→累计。
// 任一失败 → 关闭流写入句柄后经 handle 上报失败终态（暂存保留，无 DB 副作用可回滚）;
// 任一流暂停 → 不上报终态返回,交控制面按暂停收敛;全部完成 → 返回 loopDone 由调用方执行
// 提交点（rename + 建行事务 + 终态收口）
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
		// 失败收口：关闭流写入句柄释放文件锁（暂存保留供诊断与重试覆盖；替换场景旧 store 的
		// 复活与新建处置统一由控制面 setFailed 单点按登记清单执行）；失败终态清 pending
		// (failTerminal 内)后上报失败
		sess.closeStreamWriters()
		sess.failTerminal(msg)
		return loopDone
	}
	// 全部完成:交调用方执行提交点（rename + 建行事务 + 完整度重算 + 清 pending + 成功终态）
	return loopDone
}

// copyLoop 单流读取循环:read→暂存写→累计,响应暂停/取消/EOF
func (s *streamController) copyLoop(sess *execSession) streamResult {
	buf := make([]byte, 32*1024)
	for {
		select {
		case <-sess.runCtx().Done():
			// runCtx 取消(控制面暂停/停止时取消在途 reader):统一保留暂存文件。
			// 停止的暂存清理由控制面按任务状态处置（任务行在即保留给恢复判定）
			return s.handlePause(buf)
		default:
		}

		n, readErr := s.reader.Read(buf)
		if n > 0 {
			written, writeErr := s.writer.Write(buf[:n])
			if written > 0 {
				s.mu.Lock()
				s.written += int64(written)
				s.mu.Unlock()
			}
			if writeErr != nil {
				logger.Log.Errorf("[Download] 任务 %d 写入暂存失败(role=%s): %v", sess.taskId, s.role, writeErr)
				s.closeWriter()
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
			// runCtx 取消导致的读取错误(gRPC stream cancel):视为中断,保留暂存,不 Failed
			if sess.runCtx().Err() != nil {
				return s.handlePause(buf)
			}
			// 非 EOF 非 runCtx 取消:真正的读取失败
			logger.Log.Errorf("[Download] 任务 %d 下载读取失败(role=%s): %v", sess.taskId, s.role, readErr)
			s.closeWriter()
			s.state.Store(int32(streamFailed))
			return streamResult{kind: resultFailed, errMsg: fmt.Sprintf("下载读取失败: %v", readErr)}
		}
	}
}

// handleEOF 处理 reader EOF:runCtx 取消或软暂停进行中导致的 EOF 走暂停路径(保留暂存),
// 否则校验完整性并收尾暂存（写满即「待提交」，DB 行由提交点统一建）
func (s *streamController) handleEOF(sess *execSession) streamResult {
	// runCtx 取消(控制面暂停/停止)导致上游关闭产生 EOF:视为中断,保留暂存
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
			s.closeWriter()
			s.state.Store(int32(streamFailed))
			return streamResult{kind: resultFailed, errMsg: fmt.Sprintf("%s 下载不完整: 已下载 %d / 预期 %d", s.role, written, s.size)}
		case written == 0:
			// 预期大小未知(spec.Size<=0)但一字节未写:空产物,判定不完整
			logger.Log.Errorf("[Download] 任务 %d 下载为空(role=%s): written=0", sess.taskId, s.role)
			s.closeWriter()
			s.state.Store(int32(streamFailed))
			return streamResult{kind: resultFailed, errMsg: fmt.Sprintf("%s 下载为空(written=0)", s.role)}
		}
	}
	// 暂存写满收尾：Sync+Close 后比对来源声明的完整性哈希。不符按任务失败（暂存保留供诊断，
	// 重试重下覆盖）；实测哈希留存供提交点建行落 actual_sha256 列
	actualSha, err := s.writer.finalize()
	if err != nil {
		logger.Log.Errorf("[Download] 任务 %d 资源完整性校验失败(role=%s): %v", sess.taskId, s.role, err)
		s.state.Store(int32(streamFailed))
		return streamResult{kind: resultFailed, errMsg: fmt.Sprintf("资源完整性校验失败（%s）：%s", s.role, err)}
	}
	s.actualSha = actualSha
	s.state.Store(int32(streamCompleted))
	return streamResult{kind: resultOK}
}

// handlePause 排空缓冲区、同步并关闭暂存写入器、置 paused
func (s *streamController) handlePause(buf []byte) streamResult {
	if buf != nil {
		s.drain(buf)
	}
	s.writer.Sync()
	s.writer.Close()
	s.state.Store(int32(streamPaused))
	return streamResult{kind: resultPaused}
}

// drain 排空 reader 中所有已发送数据并写入暂存,直到 reader 返回错误或 EOF
func (s *streamController) drain(buf []byte) {
	for {
		n, err := s.reader.Read(buf)
		if n > 0 {
			if written, writeErr := s.writer.Write(buf[:n]); writeErr == nil && written > 0 {
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

// closeWriter 失败路径关闭暂存句柄（暂存文件保留——诊断可见，重试重下覆盖）
func (s *streamController) closeWriter() {
	if s.writer != nil {
		s.writer.Close()
	}
}
