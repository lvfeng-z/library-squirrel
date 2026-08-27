package share

// 按 PROTOCOL.md 实现的最小中继桩（测试内代码）：覆盖分享方客户端对接所需的协议面——
// 单口线协议接入、HELLO register/bind/recipient dial、隧道流转发（STREAM_OPEN/DATA/
// STREAM_CLOSE 双向）、PING→PONG、REVOKE→RESULT。与真中继（../library-squirrel-relay）
// 行为对齐的关键语义：注册生成 token、bind 替换旧隧道、dial 校验 token/状态/密码/在线、
// 收件人半关闭转发 STREAM_CLOSE 给分享方、REVOKE 应答 RESULT 后断连。

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

// stubSession 桩侧会话账目
type stubSession struct {
	token         string
	passwordHash  string
	expiresAt     int64
	meta          *metaPayload
	state         string // active | revoked | expired
	tun           *stubTunnel
	registerHELLO helloPayload // 注册 HELLO 快照（断言用：元数据/密码摘要/有效期）
}

// stubTunnel 桩侧分享方隧道
type stubTunnel struct {
	stub   *relayStub
	sess   *stubSession
	conn   net.Conn
	wmu    sync.Mutex // 隧道连接写串行化
	mu     sync.Mutex
	nextID uint32
	closed bool
}

// stubStream 桩侧一条虚拟流（绑定一条收件人连接）
type stubStream struct {
	id         uint32
	recipient  net.Conn
	rwmu       sync.Mutex // 收件人连接写串行化
	notifyOnce sync.Once
	done       chan struct{} // 流终结信号（分享方关流/隧道断开；收件人半关闭后等它再收尾）
	doneOnce   sync.Once
}

func (st *stubStream) finish() {
	st.doneOnce.Do(func() { close(st.done) })
}

// relayStub 最小中继桩
type relayStub struct {
	t    *testing.T
	ln   net.Listener
	addr string

	mu             sync.Mutex
	sessions       map[string]*stubSession
	streamTab      map[streamKey]*stubStream
	nextToken      int
	rejectRegister string // 非空=注册被拒（ERROR code，测试注入）
	pongCh         chan struct{}
	revokedCh      chan string
	bindCount      map[string]int
}

// startRelayStub 启动桩中继（监听 127.0.0.1 随机端口）
func startRelayStub(t *testing.T) *relayStub {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("启动中继桩失败: %v", err)
	}
	s := &relayStub{
		t:         t,
		ln:        ln,
		addr:      ln.Addr().String(),
		sessions:  make(map[string]*stubSession),
		streamTab: make(map[streamKey]*stubStream),
		pongCh:    make(chan struct{}, 16),
		revokedCh: make(chan string, 16),
		bindCount: make(map[string]int),
	}
	go s.acceptLoop()
	t.Cleanup(func() { _ = ln.Close() })
	return s
}

func (s *relayStub) acceptLoop() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handleConn(conn)
	}
}

// handleConn 单连接处理：首帧必为 HELLO（帧魔数即单口嗅探，非线协议直接断开）
func (s *relayStub) handleConn(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	_ = conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	fr, err := readFrame(conn, defaultMaxFrame)
	if err != nil || fr.Type != frameHello || fr.StreamID != 0 {
		return // 非 HELLO 首帧（含 HTTP 流量）：断开
	}
	var h helloPayload
	if err := json.Unmarshal(fr.Payload, &h); err != nil {
		s.rejectConn(conn, errCodeMalformed, "HELLO 非法")
		return
	}
	_ = conn.SetReadDeadline(time.Time{})
	switch {
	case h.Role == "sharer" && h.Action == "register":
		s.handleRegister(conn, h)
	case h.Role == "sharer" && h.Action == "bind":
		s.handleBind(conn, h)
	case h.Role == "recipient":
		s.handleDial(conn, h)
	default:
		s.rejectConn(conn, errCodeMalformed, "未知角色/action")
	}
}

func (s *relayStub) rejectConn(conn net.Conn, code, msg string) {
	b, _ := json.Marshal(wireErr{Code: code, Message: msg})
	_ = newFrameWriter(conn, 5*time.Second).write(frameError, 0, b)
}

// handleRegister 注册：生成 token、建会话、本连接转隧道
func (s *relayStub) handleRegister(conn net.Conn, h helloPayload) {
	s.mu.Lock()
	if s.rejectRegister != "" {
		code := s.rejectRegister
		s.mu.Unlock()
		s.rejectConn(conn, code, "测试注入拒绝")
		return
	}
	s.nextToken++
	// token 形态对齐真中继（22 字符 [A-Za-z0-9_-]）：9 字符前缀 + 13 位零填充序号
	token := fmt.Sprintf("stubtoken%013d", s.nextToken)
	var expiresAt int64
	now := time.Now()
	switch {
	case h.ExpireSeconds == nil:
		expiresAt = now.Add(7 * 24 * time.Hour).UnixMilli()
	case *h.ExpireSeconds == 0:
		expiresAt = 0
	default:
		expiresAt = now.Add(time.Duration(*h.ExpireSeconds) * time.Second).UnixMilli()
	}
	sess := &stubSession{token: token, passwordHash: h.PasswordHash, expiresAt: expiresAt, meta: h.Meta, state: "active", registerHELLO: h}
	s.sessions[token] = sess
	s.mu.Unlock()

	tun := &stubTunnel{stub: s, sess: sess, conn: conn}
	s.mu.Lock()
	sess.tun = tun
	s.mu.Unlock()

	wp, _ := json.Marshal(welcomePayload{Token: token, ExpiresAt: expiresAt})
	if err := tun.writeFrame(frameWelcome, 0, wp); err != nil {
		return
	}
	tun.serve()
}

// handleBind 重绑：会话校验通过即替换旧隧道
func (s *relayStub) handleBind(conn net.Conn, h helloPayload) {
	s.mu.Lock()
	sess, ok := s.sessions[h.Token]
	if !ok {
		s.mu.Unlock()
		s.rejectConn(conn, errCodeNotFound, "分享不存在")
		return
	}
	if sess.state != "active" {
		code := errCodeRevoked
		msg := "分享已撤销"
		if sess.state == "expired" {
			code = errCodeExpired
			msg = "分享已过期"
		}
		s.mu.Unlock()
		s.rejectConn(conn, code, msg)
		return
	}
	s.bindCount[h.Token]++
	old := sess.tun
	tun := &stubTunnel{stub: s, sess: sess, conn: conn}
	sess.tun = tun
	expiresAt := sess.expiresAt
	s.mu.Unlock()

	if old != nil {
		old.closeTunnel() // 替换语义：旧隧道立即断开
	}
	wp, _ := json.Marshal(welcomePayload{ExpiresAt: expiresAt})
	if err := tun.writeFrame(frameWelcome, 0, wp); err != nil {
		return
	}
	tun.serve()
}

// handleDial 收件人拨号：token/状态/密码/在线校验 → WELCOME + 隧道开流
func (s *relayStub) handleDial(conn net.Conn, h helloPayload) {
	s.mu.Lock()
	sess, ok := s.sessions[h.Token]
	if !ok {
		s.mu.Unlock()
		s.rejectConn(conn, errCodeNotFound, "分享不存在")
		return
	}
	if sess.state != "active" {
		code := errCodeRevoked
		msg := "分享已撤销"
		if sess.state == "expired" {
			code = errCodeExpired
			msg = "分享已过期"
		}
		s.mu.Unlock()
		s.rejectConn(conn, code, msg)
		return
	}
	if sess.passwordHash != "" && h.PasswordHash != sess.passwordHash {
		s.mu.Unlock()
		s.rejectConn(conn, errCodeBadPassword, "访问密码错误")
		return
	}
	tun := sess.tun
	if tun == nil {
		s.mu.Unlock()
		s.rejectConn(conn, errCodeOffline, "分享方不在线")
		return
	}
	s.mu.Unlock()
	tun.mu.Lock()
	if tun.closed {
		tun.mu.Unlock()
		s.rejectConn(conn, errCodeOffline, "分享方不在线")
		return
	}
	tun.nextID++
	id := tun.nextID
	st := &stubStream{id: id, recipient: conn, done: make(chan struct{})}
	tun.mu.Unlock()
	s.mu.Lock()
	s.streamTab[streamKey{tun, id}] = st
	s.mu.Unlock()

	if err := tun.writeFrame(frameStreamOpen, id, nil); err != nil {
		return
	}
	if err := newFrameWriter(conn, 5*time.Second).write(frameWelcome, 0, []byte("{}")); err != nil {
		return
	}
	// 内联服务（连接生命周期与流一致——handleConn 的 defer close 在流结束后才生效）
	tun.serveRecipient(st)
}

// writeFrame 隧道侧写帧（串行化）
func (t *stubTunnel) writeFrame(typ byte, streamID uint32, payload []byte) error {
	t.wmu.Lock()
	defer t.wmu.Unlock()
	_ = t.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return encodeFrameTo(t.conn, typ, streamID, payload)
}

// encodeFrameTo 编码并写一帧（测试辅助：桩侧裸写）
func encodeFrameTo(w io.Writer, typ byte, streamID uint32, payload []byte) error {
	return writeFrameTo(w, typ, streamID, payload)
}

// serve 隧道读循环（分享方方向）
func (t *stubTunnel) serve() {
	defer t.detach()
	for {
		_ = t.conn.SetReadDeadline(time.Now().Add(75 * time.Second))
		fr, err := readFrame(t.conn, defaultMaxFrame)
		if err != nil {
			return
		}
		switch fr.Type {
		case frameData:
			if st := t.streamByID(fr.StreamID); st != nil {
				if err := st.writeToRecipient(frameData, 1, fr.Payload); err != nil {
					t.abortStream(st)
				}
			}
		case frameStreamClose:
			if st := t.streamByID(fr.StreamID); st != nil {
				t.removeStream(st)
				_ = st.writeToRecipient(frameStreamClose, 1, nil)
				_ = st.recipient.Close()
				st.finish()
			}
		case framePing:
			_ = t.writeFrame(framePong, 0, fr.Payload)
		case framePong:
			select {
			case t.stub.pongCh <- struct{}{}:
			default:
			}
		case frameRevoke:
			t.stub.markRevoked(t.sess.token)
			b, _ := json.Marshal(resultPayload{OK: true, Action: "revoke"})
			_ = t.writeFrame(frameResult, 0, b)
			return
		default:
			return
		}
	}
}

// serveRecipient 收件人侧读循环：DATA(sid=1)→隧道、STREAM_CLOSE→半关闭通知分享方
func (t *stubTunnel) serveRecipient(st *stubStream) {
	for {
		_ = st.recipient.SetReadDeadline(time.Now().Add(75 * time.Second))
		fr, err := readFrame(st.recipient, defaultMaxFrame)
		if err != nil {
			t.abortStream(st)
			return
		}
		switch fr.Type {
		case frameData:
			if fr.StreamID != 1 {
				t.abortStream(st)
				return
			}
			if err := t.writeFrame(frameData, st.id, fr.Payload); err != nil {
				t.abortStream(st)
				return
			}
		case frameStreamClose:
			if fr.StreamID != 1 {
				t.abortStream(st)
				return
			}
			// 半关闭：通知分享方后等待分享方收尾（写方向保持可用——对齐真中继半关闭语义）
			st.notifyOnce.Do(func() { _ = t.writeFrame(frameStreamClose, st.id, nil) })
			select {
			case <-st.done:
			case <-time.After(15 * time.Second):
			}
			return
		case framePing:
			_ = st.writeToRecipient(framePong, 0, nil)
		default:
			t.abortStream(st)
			return
		}
	}
}

func (t *stubTunnel) streamByID(id uint32) *stubStream {
	// 桩的流表挂在 stubSession 上按 tunnel 维护：为简化，用 nextID 单调性 + 映射表
	return t.stub.lookupStream(t, id)
}

func (st *stubStream) writeToRecipient(typ byte, streamID uint32, payload []byte) error {
	st.rwmu.Lock()
	defer st.rwmu.Unlock()
	_ = st.recipient.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return encodeFrameTo(st.recipient, typ, streamID, payload)
}

// abortStream 异常终止流：关收件人连接并通知分享方
func (t *stubTunnel) abortStream(st *stubStream) {
	t.removeStream(st)
	st.notifyOnce.Do(func() { _ = t.writeFrame(frameStreamClose, st.id, nil) })
	_ = st.recipient.Close()
	st.finish()
}

func (t *stubTunnel) removeStream(st *stubStream) {
	t.stub.removeStreamRef(t, st.id)
}

// closeTunnel 断开隧道（bind 替换/测试注入）
func (t *stubTunnel) closeTunnel() {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return
	}
	t.closed = true
	t.mu.Unlock()
	_ = t.conn.Close()
}

// detach 隧道断开时摘除会话关联
func (t *stubTunnel) detach() {
	t.stub.detachTunnel(t)
}

// —— 桩全局索引（流表：tunnel+streamID → 流）——

type streamKey struct {
	tun *stubTunnel
	id  uint32
}

func (s *relayStub) lookupStream(tun *stubTunnel, id uint32) *stubStream {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.streamTab[streamKey{tun, id}]
}

func (s *relayStub) removeStreamRef(tun *stubTunnel, id uint32) {
	s.mu.Lock()
	delete(s.streamTab, streamKey{tun, id})
	s.mu.Unlock()
}

func (s *relayStub) detachTunnel(tun *stubTunnel) {
	s.mu.Lock()
	if tun.sess.tun == tun {
		tun.sess.tun = nil
	}
	for k := range s.streamTab {
		if k.tun == tun {
			delete(s.streamTab, k)
		}
	}
	s.mu.Unlock()
}

func (s *relayStub) markRevoked(token string) {
	s.mu.Lock()
	if sess, ok := s.sessions[token]; ok {
		sess.state = "revoked"
	}
	s.mu.Unlock()
	select {
	case s.revokedCh <- token:
	default:
	}
}

// —— 测试注入/查询辅助 ——

// setRejectRegister 注入注册拒绝
func (s *relayStub) setRejectRegister(code string) {
	s.mu.Lock()
	s.rejectRegister = code
	s.mu.Unlock()
}

// dropTunnel 服务端强制断开指定会话隧道（模拟网络断连）
func (s *relayStub) dropTunnel(token string) {
	s.mu.Lock()
	tun := s.sessions[token].tun
	s.mu.Unlock()
	if tun != nil {
		tun.closeTunnel()
	}
}

// pushTunnelError 向分享方隧道推 ERROR 帧（模拟中继终态推送：expired 等；
// 分享方读循环据此落终态）
func (s *relayStub) pushTunnelError(token, code string) error {
	tun := s.tunnelOf(token)
	if tun == nil {
		return fmt.Errorf("隧道不在线")
	}
	b, _ := json.Marshal(wireErr{Code: code, Message: "测试注入终态"})
	return tun.writeFrame(frameError, 0, b)
}

// tunnelOf 查询会话当前隧道是否在线
func (s *relayStub) tunnelOf(token string) *stubTunnel {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.sessions[token]; ok {
		return sess.tun
	}
	return nil
}

// sessionOf 查询会话
func (s *relayStub) sessionOf(token string) *stubSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessions[token]
}

// sendPingToTunnel 向分享方隧道发 PING（保活应答测试）
func (s *relayStub) sendPingToTunnel(token string) error {
	tun := s.tunnelOf(token)
	if tun == nil {
		return fmt.Errorf("隧道不在线")
	}
	return tun.writeFrame(framePing, 0, []byte("ka"))
}

// bindCountOf 查询重绑次数
func (s *relayStub) bindCountOf(token string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bindCount[token]
}
