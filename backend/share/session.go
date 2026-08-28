package share

// 分享会话运行时：维持到中继的出站隧道（register/bind + 断线重连 + PING 应答），
// 并按 manifest 文件白名单应答收件人的拉取请求。
//
// 流内应用层协议（E2E 加密，中继不可见；收件人侧实现见二期阶段4，契约定稿于本文件）：
//   请求（收件人→分享方，单条记录）： {"type":"manifest"} 或 {"type":"file","path":"works/…","offset":N}
//   应答（分享方→收件人）：
//     首记录 JSON 头 {"ok":true,"kind":"manifest|file","size":N,"offset":M} 或 {"ok":false,"error":"…"}
//     随后内容按记录分块原样承载（明文拼接即完整内容字节），流以 STREAM_CLOSE 收尾。
//   记录 = nonce(12) || AES-256-GCM(明文)，一帧一记录（见 crypto.go）。
//
// 背压策略（PROTOCOL.md §11.3 二选一中的「分块限速」）：请求-响应模式下收件人发送请求后
// 半关闭（§11.2 推荐形态），同流无回向确认通道，故采用单流限速平滑突发，
// 慢消费者由中继 128 帧缓冲满即断流的既有策略兜底（本端随后收到 STREAM_CLOSE 终止发送）。

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/library-squirrel/backend/export"
)

// 会话状态（终态：revoked/expired/failed——不可逆，不再重连）
const (
	stateConnecting   = "connecting"   // 注册中（首次连接）
	stateOnline       = "online"       // 隧道在线（链接可拉取）
	stateReconnecting = "reconnecting" // 断线重连中
	stateRevoked      = "revoked"      // 已撤销（终态）
	stateExpired      = "expired"      // 已过期（终态）
	stateFailed       = "failed"       // 失败（终态：封禁/会话消失/协议缺陷）
)

// errSessionTerminal 读循环返回该哨兵表示会话已达终态（run 循环据此退出、不重连）
var errSessionTerminal = errors.New("会话已终态")

// 流内默认参数
const (
	defaultChunkSize            = 16 * 1024        // 单记录明文分块（帧负载 = 分块 + 28 字节加密开销 < maxPayload）
	defaultMaxStreams           = 8                // 单会话并发流上限（对齐中继 maxStreamsPerSession 默认值）
	defaultStreamRate     int64 = 16 * 1024 * 1024 // 单流限速（字节/秒）
	defaultHandshakeWait        = 15 * time.Second // HELLO→WELCOME 握手超时
	defaultTunnelReadIdle       = 90 * time.Second // 无入帧判定隧道死亡（> 中继 keepaliveTimeout 75s）
	defaultStreamReqWait        = 60 * time.Second // 流打开后等待请求记录的超时
	maxRequestRecordBytes       = 32 * 1024        // 请求记录明文上限（请求为单条小 JSON）
	reconnectMinDelay           = time.Second
	reconnectMaxDelay           = 30 * time.Second
)

// 流内错误码（应答 JSON 头 error 字段）
const (
	streamErrNotFound   = "not_found"   // 请求的包内路径不在白名单
	streamErrMissing    = "missing"     // 在白名单但源文件缺失（manifest 已标 missing）
	streamErrBadRequest = "bad_request" // 请求不可解/非法
	streamErrInternal   = "internal"    // 服务端内部错误
)

// streamRequest 收件人拉取请求
type streamRequest struct {
	Type   string `json:"type"`             // manifest | file
	Path   string `json:"path,omitempty"`   // file：包内路径（manifest.files[].path 白名单键）
	Offset int64  `json:"offset,omitempty"` // file：可选起始偏移（断点续传锚）
}

// streamHeader 应答首记录
type streamHeader struct {
	OK     bool   `json:"ok"`
	Kind   string `json:"kind,omitempty"` // manifest | file
	Size   int64  `json:"size,omitempty"`
	Offset int64  `json:"offset,omitempty"`
	Error  string `json:"error,omitempty"`
}

// sessionRuntimeOptions 会话运行参数（测试可覆写）
type sessionRuntimeOptions struct {
	chunkSize             int
	streamRate            int64
	maxStreams            int
	handshakeWait         time.Duration
	tunnelReadIdle        time.Duration
	streamReqWait         time.Duration
	writeWait             time.Duration
	dialTimeout           time.Duration
	dialFn                func(addr string) (net.Conn, error)
	maxRequestRecordBytes int
}

func defaultRuntimeOptions() sessionRuntimeOptions {
	return sessionRuntimeOptions{
		chunkSize:             defaultChunkSize,
		streamRate:            defaultStreamRate,
		maxStreams:            defaultMaxStreams,
		handshakeWait:         defaultHandshakeWait,
		tunnelReadIdle:        defaultTunnelReadIdle,
		streamReqWait:         defaultStreamReqWait,
		writeWait:             30 * time.Second,
		dialTimeout:           15 * time.Second,
		maxRequestRecordBytes: maxRequestRecordBytes,
		dialFn: func(addr string) (net.Conn, error) {
			return net.DialTimeout("tcp", addr, 15*time.Second)
		},
	}
}

// withOverrides 以非零字段覆写默认参数
func (o sessionRuntimeOptions) withOverrides(ov sessionRuntimeOptions) sessionRuntimeOptions {
	if ov.chunkSize > 0 {
		o.chunkSize = ov.chunkSize
	}
	if ov.streamRate != 0 {
		o.streamRate = ov.streamRate
	}
	if ov.maxStreams > 0 {
		o.maxStreams = ov.maxStreams
	}
	if ov.handshakeWait > 0 {
		o.handshakeWait = ov.handshakeWait
	}
	if ov.tunnelReadIdle > 0 {
		o.tunnelReadIdle = ov.tunnelReadIdle
	}
	if ov.streamReqWait > 0 {
		o.streamReqWait = ov.streamReqWait
	}
	if ov.writeWait > 0 {
		o.writeWait = ov.writeWait
	}
	if ov.dialTimeout > 0 {
		o.dialTimeout = ov.dialTimeout
	}
	if ov.dialFn != nil {
		o.dialFn = ov.dialFn
	}
	if ov.maxRequestRecordBytes > 0 {
		o.maxRequestRecordBytes = ov.maxRequestRecordBytes
	}
	return o
}

// sessionConfig 会话发布参数（发布时一次确定、运行期只读）
type sessionConfig struct {
	id           string
	title        string
	instanceID   string
	relayDial    string // 中继 TCP 拨号地址（host:port）
	relayHost    string // 链接 host（host 或 host:port，链接 https://{relayHost}/s/{token}）
	workDir      string
	key          []byte
	model        *export.ExportModel
	manifestData []byte // 发给收件人的 manifest JSON（收集端序列化产物）
	passwordHash string // 空=无密码
	expireSecs   *int64 // nil=中继默认；0=无限期；>0=自定义秒
	createdAt    int64
	emitter      ShareEventEmitter // nil=不发事件（部分单测场景）
	opts         sessionRuntimeOptions
	// 复原形态（启动自动复原）：凭记录行原 token 经 bind 重绑而非 register（链接不变，
	// 剩余有效期由中继侧会话管理——bind 不重传 expiresAt/passwordHash/元数据）
	seedToken string // 原 token（非空=首次连接即 bind）
	restore   bool   // 复原形态标记：bind WELCOME 即首次在线信号
}

// shareSession 单个分享会话
type shareSession struct {
	cfg  sessionConfig
	cip  *e2eCipher
	opts sessionRuntimeOptions // 运行参数（构造时由默认值+覆写合并）

	fileIndex map[string]int // 包内路径 → manifest.Files 索引（拉取白名单）
	// 统计基数（发布时由 manifest 计算，DTO 展示用）
	metaWorkCount int64
	fileCount     int64
	totalBytes    int64
	missingFiles  int64

	mu            sync.Mutex
	token         string
	expiresAt     int64
	link          string
	state         string
	errMsg        string
	streamsServed int64
	bytesServed   int64
	activeStreams int
	curWriter     *frameWriter // 当前隧道写出器（离线为 nil；撤销帧经它发出）

	firstCh chan error    // 首次注册结果（nil=已在线；非 nil=终态失败），缓冲 1、只发一次
	doneCh  chan struct{} // 会话主循环退出信号（终态或宿主 ctx 取消；宿主任务据此判定会话结束）
	streams sync.Map      // streamID → *streamState（读循环与流处理 goroutine 并发访问）
	ctx     context.Context
	cancel  context.CancelFunc
	// firstSignaled 首次在线/失败信号是否已发出（与 firstCh 配对的只发一次标记；
	// 复原形态下 bind WELCOME/被拒也属首次信号，不能只按 token 有无判定）
	firstSignaled bool
}

// newShareSession 创建会话（不启动；由 run 驱动）。密钥在此完成密码学对象构建。
func newShareSession(cfg sessionConfig) (*shareSession, error) {
	cip, err := newE2ECipher(cfg.key)
	if err != nil {
		return nil, err
	}
	s := &shareSession{
		cfg:     cfg,
		cip:     cip,
		firstCh: make(chan error, 1),
		doneCh:  make(chan struct{}),
	}
	s.token = cfg.seedToken // 复原形态：原 token 预置（首次连接即 bind）
	s.opts = defaultRuntimeOptions().withOverrides(cfg.opts)
	s.state = stateConnecting
	// 白名单与统计：包内路径即白名单键——收件人请求的 path 仅作 map 查找，永不进入文件系统
	s.fileIndex = make(map[string]int, len(cfg.model.Manifest.Files))
	for i, f := range cfg.model.Manifest.Files {
		if f.Path == "" {
			continue // 未被任何作品挂载引用（无包内路径），不在白名单
		}
		s.fileIndex[f.Path] = i
		if f.Missing {
			s.missingFiles++
			continue
		}
		s.fileCount++
		s.totalBytes += f.Size
	}
	s.metaWorkCount = int64(len(cfg.model.Manifest.Works))
	if cfg.model.Manifest.Meta.WorkCount > 0 {
		s.metaWorkCount = int64(cfg.model.Manifest.Meta.WorkCount)
	}
	return s, nil
}

// run 会话主循环：连接→注册/重绑→服务，断线按退避重连，终态退出。
// ctx 由发布方持有（撤销/取消即 cancel）；无论何种退出路径均关闭 doneCh 供宿主等待。
func (s *shareSession) run(ctx context.Context) {
	defer close(s.doneCh)
	s.ctx, s.cancel = context.WithCancel(ctx)
	ctx = s.ctx
	delay := reconnectMinDelay
	for {
		_ = s.runOnce(ctx)
		if ctx.Err() != nil {
			return // 主动终止（撤销/取消），状态已由触发方置好
		}
		if s.terminal() {
			s.emitSnapshot()
			return
		}
		// 在线期间复位退避（只在故障窗口内递增）
		if s.currentState() == stateOnline {
			delay = reconnectMinDelay
		}
		// 断线/可重试错误 → 重连（bind），退避递增、成功在线后复位
		s.setState(stateReconnecting, "")
		s.emitSnapshot()
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		delay *= 2
		if delay > reconnectMaxDelay {
			delay = reconnectMaxDelay
		}
	}
}

// runOnce 一次完整的「连接→握手→隧道服务」；返回后由 run 决定重连或退出
func (s *shareSession) runOnce(ctx context.Context) error {
	conn, err := s.opts.dialFn(s.cfg.relayDial)
	if err != nil {
		return fmt.Errorf("连接中继失败: %w", err)
	}
	defer func() { _ = conn.Close() }()

	w := newFrameWriter(conn, s.opts.writeWait)
	if err := w.write(frameHello, 0, marshalHello(s.buildHello())); err != nil {
		return fmt.Errorf("发送 HELLO 失败: %w", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(s.opts.handshakeWait))
	fr, err := readFrame(conn, defaultMaxFrame)
	if err != nil {
		return fmt.Errorf("读取 WELCOME 失败: %w", err)
	}
	switch fr.Type {
	case frameWelcome:
		var wp welcomePayload
		if err := json.Unmarshal(fr.Payload, &wp); err != nil {
			return fmt.Errorf("解析 WELCOME 失败: %w", err)
		}
		firstOnline := s.acceptWelcome(wp)
		s.emitSnapshot()
		if firstOnline {
			s.signalFirst(nil)
		}
	case frameError:
		we, perr := parseWireErr(fr.Payload)
		if perr != nil {
			return fmt.Errorf("解析 ERROR 失败: %w", perr)
		}
		s.applyRelayRejection(we)
		if s.terminal() {
			return errSessionTerminal
		}
		return we // 可重试（rate_limited/limit/server_error）→ run 退避重连
	default:
		return fmt.Errorf("握手期收到非预期帧 0x%02x", fr.Type)
	}
	return s.serveTunnel(ctx, conn, w)
}

// buildHello 按当前阶段构造 HELLO：无 token=register（含元数据/密码/有效期），有 token=bind
func (s *shareSession) buildHello() *helloPayload {
	h := &helloPayload{Role: "sharer", InstanceID: s.cfg.instanceID}
	s.mu.Lock()
	token := s.token
	s.mu.Unlock()
	if token == "" {
		h.Action = "register"
		h.PasswordHash = s.cfg.passwordHash
		h.ExpireSeconds = s.cfg.expireSecs
		h.Meta = &metaPayload{Title: s.cfg.title, WorkCount: s.metaWorkCount, Source: s.metaSource(), WorksName: s.metaWorksName()}
	} else {
		h.Action = "bind"
		h.Token = token
	}
	return h
}

// metaSource 落地页来源声明：manifest 站点名去重拼接；无站点（纯本地）回落「本地资源库」
func (s *shareSession) metaSource() string {
	names := make([]string, 0, 4)
	seen := make(map[string]struct{})
	for _, site := range s.cfg.model.Manifest.Sites {
		name := ""
		if site.SiteName != nil {
			name = *site.SiteName
		}
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
		if len(names) >= 3 {
			break
		}
	}
	if len(names) == 0 {
		return "本地资源库"
	}
	src := names[0]
	for _, n := range names[1:] {
		src += "/" + n
	}
	return SanitizeMetaText(src, 100)
}

// sanitizedWorkName 作品名净化（与 title/source 同款）：site_work_name 优先、次 nick_name、
// 全空以「作品 {ID}」占位，净化控制字符并截断到 200 rune。收件侧子任务命名与落地页 worksName
// 三处共用同一净化后作品名；relay 侧对 worksName 单名校验 ≤200 rune 且禁控制字符，不净化会被
// 中继以 malformed 拒绝（跨仓契约，见方案风险8）。
func sanitizedWorkName(w *export.WorkRecord) string {
	name := ""
	if w.SiteWorkName != nil {
		name = *w.SiteWorkName
	}
	if name == "" && w.NickName != nil {
		name = *w.NickName
	}
	if name == "" {
		name = fmt.Sprintf("作品 %d", w.ID)
	}
	return SanitizeMetaText(name, 200)
}

// metaWorksName 落地页作品名列表：按 manifest.Works 顺序取净化后作品名（与收件侧子任务命名一致）。
// 仅 register 上传，bind 复原不携带。
func (s *shareSession) metaWorksName() []string {
	works := s.cfg.model.Manifest.Works
	names := make([]string, 0, len(works))
	for i := range works {
		names = append(names, sanitizedWorkName(&works[i]))
	}
	return names
}

// acceptWelcome 记录 WELCOME；返回是否为「首次在线」（首次时置 online 并复位退避由 run 处理）。
// 首次信号只发一次：注册形态以 WELCOME 带 token 为准（重连 bind 无 token 不重复触发）；
// 复原形态以首个 bind WELCOME 为准。
func (s *shareSession) acceptWelcome(wp welcomePayload) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if wp.Token != "" {
		s.token = wp.Token // register 应答：中继生成的新 token
	}
	s.expiresAt = wp.ExpiresAt
	wasOnline := s.state == stateOnline
	if !wasOnline {
		s.state = stateOnline
		s.errMsg = ""
	}
	if !wasOnline && !s.firstSignaled && (wp.Token != "" || s.cfg.restore) {
		s.firstSignaled = true
		return true
	}
	return false
}

// applyRelayRejection 按 ERROR code 落终态或保持可重试
func (s *shareSession) applyRelayRejection(we *wireErr) {
	if !isTerminalErrCode(we.Code) {
		return // 可重试：状态维持 reconnecting，交 run 退避
	}
	msg := we.Error()
	state := stateFailed
	switch we.Code {
	case errCodeRevoked:
		state = stateRevoked
	case errCodeExpired:
		state = stateExpired
	}
	s.mu.Lock()
	if !s.isTerminalLocked() {
		s.state = state
		s.errMsg = msg
	}
	first := !s.firstSignaled
	if first {
		s.firstSignaled = true
	}
	s.mu.Unlock()
	if first {
		s.signalFirst(we) // 首次连接即被拒（注册或复原 bind）：发布/复原流程收终态失败
	}
}

// serveTunnel 隧道读循环：分发 STREAM_OPEN/DATA/STREAM_CLOSE/PING/RESULT，直至断连/终态
func (s *shareSession) serveTunnel(ctx context.Context, conn net.Conn, w *frameWriter) error {
	s.setWriter(w)
	defer s.setWriter(nil)
	for {
		_ = conn.SetReadDeadline(time.Now().Add(s.opts.tunnelReadIdle))
		fr, err := readFrame(conn, defaultMaxFrame)
		if err != nil {
			return fmt.Errorf("隧道读取失败: %w", err) // 断连/保活超时 → 重连
		}
		switch fr.Type {
		case framePing:
			if err := w.write(framePong, 0, fr.Payload); err != nil {
				return err
			}
		case frameStreamOpen:
			s.handleStreamOpen(ctx, w, fr.StreamID)
		case frameData:
			s.deliverStreamData(fr.StreamID, fr.Payload)
		case frameStreamClose:
			s.closeStreamInbound(fr.StreamID)
		case frameResult:
			var rp resultPayload
			if err := json.Unmarshal(fr.Payload, &rp); err == nil && rp.OK && rp.Action == "revoke" {
				s.setState(stateRevoked, "")
				return errSessionTerminal
			}
		case frameError:
			we, perr := parseWireErr(fr.Payload)
			if perr != nil {
				return perr
			}
			s.applyRelayRejection(we)
			if s.terminal() {
				return errSessionTerminal
			}
			return we
		case framePong:
			// 当前客户端不主动发 PING；防御性忽略
		default:
			return fmt.Errorf("隧道期收到非预期帧 0x%02x", fr.Type)
		}
	}
}

// setWriter 记录/清除当前隧道写出器（撤销帧的发送通道）
func (s *shareSession) setWriter(w *frameWriter) {
	s.mu.Lock()
	s.curWriter = w
	s.mu.Unlock()
}

// parseWireErr 解析 ERROR 帧载荷
func parseWireErr(payload []byte) (*wireErr, error) {
	var we wireErr
	if err := json.Unmarshal(payload, &we); err != nil {
		return nil, err
	}
	if we.Code == "" {
		return nil, fmt.Errorf("ERROR 载荷缺 code")
	}
	return &we, nil
}

// signalFirst 发送首次注册结果（只发一次）
func (s *shareSession) signalFirst(err error) {
	select {
	case s.firstCh <- err:
	default:
	}
}

// —— 流处理 ——

// streamState 一条入站虚拟流的接收侧状态
type streamState struct {
	sid       uint32
	records   chan []byte // 入站记录（DATA 帧负载）；缓冲满即丢弃并终止流（请求为单条小记录，不该积压）
	halfClose chan struct{}
	halfOnce  sync.Once
}

// handleStreamOpen 中继通知新流：并发上限内建立接收状态并起处理 goroutine，超限直接关流拒绝
func (s *shareSession) handleStreamOpen(ctx context.Context, w *frameWriter, sid uint32) {
	s.mu.Lock()
	if s.activeStreams >= s.opts.maxStreams {
		s.mu.Unlock()
		_ = w.write(frameStreamClose, sid, nil)
		return
	}
	s.activeStreams++
	s.mu.Unlock()

	st := &streamState{
		sid:       sid,
		records:   make(chan []byte, 8),
		halfClose: make(chan struct{}),
	}
	s.streams.Store(sid, st)
	go func() {
		defer s.releaseStream(sid)
		s.serveStream(ctx, w, st)
	}()
}

// deliverStreamData 投递流数据帧；未知流（已结束/迟到帧）静默丢弃（对齐 PROTOCOL.md §5）
func (s *shareSession) deliverStreamData(sid uint32, record []byte) {
	v, ok := s.streams.Load(sid)
	if !ok {
		return
	}
	st := v.(*streamState)
	select {
	case st.records <- record:
	default:
		// 请求侧积压异常：终止该流（防御路径，正常请求只有一条记录）
		s.streams.Delete(sid)
		st.halfOnce.Do(func() { close(st.halfClose) })
	}
}

// closeStreamInbound 收件人半关闭（请求发完信号）
func (s *shareSession) closeStreamInbound(sid uint32) {
	if v, ok := s.streams.Load(sid); ok {
		st := v.(*streamState)
		st.halfOnce.Do(func() { close(st.halfClose) })
	}
}

// releaseStream 流收尾：摘表 + 并发计数递减
func (s *shareSession) releaseStream(sid uint32) {
	s.streams.Delete(sid)
	s.mu.Lock()
	if s.activeStreams > 0 {
		s.activeStreams--
	}
	s.mu.Unlock()
}

// serveStream 单流处理：读请求 → 白名单判定 → 加密分块应答 → 关流
func (s *shareSession) serveStream(ctx context.Context, w *frameWriter, st *streamState) {
	var record []byte
	timer := time.NewTimer(s.opts.streamReqWait)
	defer timer.Stop()
	select {
	case record = <-st.records:
	case <-st.halfClose:
		// 半关闭与请求记录同时就绪时仍优先取记录（select 随机性对冲）
		select {
		case record = <-st.records:
		default:
			s.finishStreamWithError(w, st, streamErrBadRequest)
			return
		}
	case <-timer.C:
		// 迟迟无请求：直接关流（不发错误记录，对端已按空闲超时处理）
		_ = w.write(frameStreamClose, st.sid, nil)
		return
	case <-ctx.Done():
		_ = w.write(frameStreamClose, st.sid, nil)
		return
	}
	// 后续半关闭不再等待；多余记录忽略（请求契约=单条记录）
	plaintext, err := s.cip.openRecord(record)
	if err != nil || len(plaintext) > s.opts.maxRequestRecordBytes {
		s.finishStreamWithError(w, st, streamErrBadRequest)
		return
	}
	var req streamRequest
	if err := json.Unmarshal(plaintext, &req); err != nil {
		s.finishStreamWithError(w, st, streamErrBadRequest)
		return
	}
	switch req.Type {
	case "manifest":
		s.serveBytes(w, st, "manifest", 0, int64(len(s.cfg.manifestData)), bytes.NewReader(s.cfg.manifestData))
	case "file":
		s.serveFile(w, st, &req)
	default:
		s.finishStreamWithError(w, st, streamErrBadRequest)
	}
}

// serveFile 文件拉取：path 仅作白名单键查 manifest 索引，实际读文件只经 manifest 声明的
// storePath（DB 收集产物）——请求内容永不进入文件系统，路径穿越/任意读在此结构性排除。
func (s *shareSession) serveFile(w *frameWriter, st *streamState, req *streamRequest) {
	idx, ok := s.fileIndex[req.Path]
	if !ok {
		s.finishStreamWithError(w, st, streamErrNotFound)
		return
	}
	entry := s.cfg.model.Manifest.Files[idx]
	if entry.Missing {
		s.finishStreamWithError(w, st, streamErrMissing)
		return
	}
	if req.Offset < 0 || (entry.Size > 0 && req.Offset >= entry.Size) {
		s.finishStreamWithError(w, st, streamErrBadRequest)
		return
	}
	// absPath 域：仅 os.* 调用点现场构造（PATH_SEPARATOR_DISCIPLINE）
	f, err := os.Open(filepath.Join(s.cfg.workDir, entry.StorePath))
	if err != nil {
		s.finishStreamWithError(w, st, streamErrInternal)
		return
	}
	defer func() { _ = f.Close() }()
	size := entry.Size - req.Offset
	s.serveBytes(w, st, "file", req.Offset, size, io.NewSectionReader(f, req.Offset, size))
}

// serveBytes 统一的字节流应答：JSON 头记录 + 内容分块记录 + STREAM_CLOSE 收尾。
// rate 限速在此生效（背压策略：分块限速）。
func (s *shareSession) serveBytes(w *frameWriter, st *streamState, kind string, offset, size int64, r io.Reader) {
	pacer := newStreamPacer(s.opts.streamRate)
	var served int64
	defer func() {
		s.mu.Lock()
		s.streamsServed++
		s.bytesServed += served
		s.mu.Unlock()
		s.emitSnapshot()
		_ = w.write(frameStreamClose, st.sid, nil) // 幂等收尾：无论成败都关流（对端按 size 判完整性）
	}()
	head, err := json.Marshal(streamHeader{OK: true, Kind: kind, Size: size, Offset: offset})
	if err != nil {
		return
	}
	if err := s.writeRecord(w, st.sid, pacer, head, &served); err != nil {
		return
	}
	buf := make([]byte, s.opts.chunkSize)
	for {
		n, rerr := r.Read(buf)
		if n > 0 {
			if err := s.writeRecord(w, st.sid, pacer, buf[:n], &served); err != nil {
				return
			}
		}
		if rerr == io.EOF {
			return
		}
		if rerr != nil {
			return // 中途读失败：已发头记录无法收回，直接关流（对端按 size 不匹配判截断）
		}
	}
}

// finishStreamWithError 错误应答：单条错误头记录 + 关流
func (s *shareSession) finishStreamWithError(w *frameWriter, st *streamState, code string) {
	defer func() {
		s.mu.Lock()
		s.streamsServed++
		s.mu.Unlock()
		s.emitSnapshot()
		_ = w.write(frameStreamClose, st.sid, nil)
	}()
	head, _ := json.Marshal(streamHeader{OK: false, Error: code})
	_ = s.writePlainRecord(w, st.sid, head)
}

// writeRecord 加密并写一条内容记录（含限速等待）
func (s *shareSession) writeRecord(w *frameWriter, sid uint32, pacer *streamPacer, plaintext []byte, served *int64) error {
	pacer.wait(int64(len(plaintext)))
	record, err := s.cip.sealRecord(plaintext)
	if err != nil {
		return err
	}
	if err := w.write(frameData, sid, record); err != nil {
		return err
	}
	*served += int64(len(plaintext))
	return nil
}

// writePlainRecord 写一条不计数的内容记录（错误头记录不计入服务字节）
func (s *shareSession) writePlainRecord(w *frameWriter, sid uint32, plaintext []byte) error {
	record, err := s.cip.sealRecord(plaintext)
	if err != nil {
		return err
	}
	return w.write(frameData, sid, record)
}

// streamPacer 单流线性限速器：累计字节按速率换算目标时刻，超前则等待。
// 平滑突发，避免瞬时灌满中继每流 128 帧缓冲触发慢消费者断流。
type streamPacer struct {
	rate  int64
	start time.Time
	sent  int64
}

func newStreamPacer(rate int64) *streamPacer {
	return &streamPacer{rate: rate, start: time.Now()}
}

func (p *streamPacer) wait(n int64) {
	if p.rate <= 0 {
		return
	}
	p.sent += n
	// 浮点换算目标时刻：整型「字节 × 秒」在超大文件下溢出 int64
	target := p.start.Add(time.Duration(float64(p.sent) / float64(p.rate) * float64(time.Second)))
	if d := time.Until(target); d > 0 {
		time.Sleep(d)
	}
}

// —— 状态与生命周期 ——

// setState 置状态（终态不可逆：终态后不再变更）
func (s *shareSession) setState(state, errMsg string) {
	s.mu.Lock()
	if !s.isTerminalLocked() {
		s.state = state
		s.errMsg = errMsg
	}
	s.mu.Unlock()
}

func (s *shareSession) isTerminalLocked() bool {
	switch s.state {
	case stateRevoked, stateExpired, stateFailed:
		return true
	}
	return false
}

func (s *shareSession) terminal() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isTerminalLocked()
}

// currentState 读取当前状态
func (s *shareSession) currentState() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// setLink 发布成功后记录完整链接（含 fragment 密钥；仅本机内存，随 DTO 供前端展示/复制）
func (s *shareSession) setLink(link string) {
	s.mu.Lock()
	s.link = link
	s.mu.Unlock()
}

// tokenOf 读取当前 token
func (s *shareSession) tokenOf() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.token
}

// Revoke 撤销会话：在线时发 REVOKE 帧等 RESULT 应答（读循环落终态），离线时本地终止
// （中继侧会话存续至到期——撤销是隧道帧，离线无法送达，见 README 边界）。
func (s *shareSession) Revoke() {
	s.mu.Lock()
	if s.isTerminalLocked() {
		s.mu.Unlock()
		return
	}
	w := s.curWriter
	s.mu.Unlock()
	if w != nil {
		if err := w.write(frameRevoke, 0, nil); err != nil {
			s.setState(stateRevoked, "")
			s.emitSnapshot()
			s.cancelCtx()
			return
		}
		// RESULT 由读循环处理；兜底：应答超时仍未终态则本地置撤销并终止
		time.AfterFunc(3*time.Second, func() {
			if !s.terminal() {
				s.setState(stateRevoked, "")
				s.emitSnapshot()
				s.cancelCtx()
			}
		})
		return
	}
	s.setState(stateRevoked, "")
	s.emitSnapshot()
	s.cancelCtx()
}

// cancelCtx 终止 run 主循环（幂等）
func (s *shareSession) cancelCtx() {
	s.mu.Lock()
	c := s.cancel
	s.mu.Unlock()
	if c != nil {
		c()
	}
}

// emitSnapshot 推送会话状态快照事件（emitter 未注入时静默）
func (s *shareSession) emitSnapshot() {
	if s.cfg.emitter == nil {
		return
	}
	s.cfg.emitter.PushState(s.snapshot())
}

// snapshot 构建会话 DTO 快照
func (s *shareSession) snapshot() *ShareSessionDTO {
	s.mu.Lock()
	defer s.mu.Unlock()
	return &ShareSessionDTO{
		ShareID:           s.cfg.id,
		Token:             s.token,
		Link:              s.link,
		Title:             s.cfg.title,
		WorkCount:         s.metaWorkCount,
		FileCount:         s.fileCount,
		TotalBytes:        s.totalBytes,
		MissingFiles:      s.missingFiles,
		ExpiresAt:         s.expiresAt,
		PasswordProtected: s.cfg.passwordHash != "",
		State:             s.state,
		StreamsServed:     s.streamsServed,
		BytesServed:       s.bytesServed,
		CreatedAt:         s.cfg.createdAt,
		RelayAddress:      s.cfg.relayHost,
		ErrMsg:            s.errMsg,
	}
}
