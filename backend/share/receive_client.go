package share

// 收件人侧中继拨号客户端：按 PROTOCOL.md §3.3（recipient dial）与 §11.2（请求-响应 +
// 半关闭）实现——每次拉取一条 TCP 连接（= 隧道内一条虚拟流，streamID 恒为 1）：
// 发单条加密请求记录 → STREAM_CLOSE 半关闭 → 读加密应答记录至对端关流。
// 流内应用层协议与 session.go 头注契约定稿一致（首记录 JSON 头 + 内容分块记录）。

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// 收件人侧默认参数
const (
	defaultRecipientReadIdle = 120 * time.Second // 单帧读取空闲超时（数据持续流动，远小于中继 600s 空闲断开）
	// receiveMaxHeaderBytes 应答首记录（JSON 头）明文上限
	receiveMaxHeaderBytes = 4 * 1024
)

// relayDialError 中继拨号被拒（ERROR 帧 code 语义，分类见 relayDialFailMessage）
type relayDialError struct {
	code string
	msg  string
}

func (e *relayDialError) Error() string {
	return fmt.Sprintf("中继拒绝: %s: %s", e.code, e.msg)
}

// streamAppError 流内应用层错误（分享方应答 JSON 头 ok=false 的 error 码）
type streamAppError struct {
	code string
}

func (e *streamAppError) Error() string {
	return fmt.Sprintf("分享方返回错误: %s", e.code)
}

// errStreamTruncated 应答字节数与 JSON 头 size 不符（截断/发送侧中途失败）
var errStreamTruncated = errors.New("拉取数据不完整（与声明大小不符）")

// relayDialFailMessage 中继 ERROR code →（用户可读失败文案, 是否终态不可重试）。
// 终态判定对齐 PROTOCOL.md §11.5：revoked/expired/banned/not_found/malformed 不重试；
// offline 属分享方暂时离线、limit/rate_limited/server_error 属瞬态——退避重试耗尽后以该文案失败。
func relayDialFailMessage(err error) (string, bool) {
	var de *relayDialError
	if !errors.As(err, &de) {
		return "", false
	}
	switch de.code {
	case errCodeNotFound:
		return "分享不存在或已被撤销（分享已失效）", true
	case errCodeExpired:
		return "分享已过期（分享已失效）", true
	case errCodeRevoked:
		return "分享已被撤销（分享已失效）", true
	case errCodeBanned:
		return "本设备已被中继封禁，无法拉取", true
	case errCodeBadPassword:
		return "访问密码错误（请删除本任务后携带正确密码重新拉取）", true
	case errCodeMalformed:
		return "中继判定请求非法（客户端缺陷，分享已失效）", true
	case errCodeOffline:
		return "分享方不在线（分享方需保持应用运行），可稍后在任务面板重试", false
	case errCodeLimit, errCodeRateLimited, errCodeServerError:
		return "中继暂不可用（" + de.code + "），可稍后在任务面板重试", false
	}
	return de.Error(), false
}

// isRelayRetryableErr 判定拨号错误是否可退避重试：网络错误（连接失败/重置/超时）与
// 可重试中继 code 按瞬态处理；流内应用错误仅 internal（分享方 IO 瞬态）可重试，
// not_found/missing/bad_request 属确定性拒绝，重试无意义。
func isRelayRetryableErr(err error) bool {
	var de *relayDialError
	if errors.As(err, &de) {
		_, terminal := relayDialFailMessage(err)
		return !terminal
	}
	var ae *streamAppError
	if errors.As(err, &ae) {
		return ae.code == streamErrInternal
	}
	return true
}

// receiveClient 收件人拉取客户端（一次任务执行一个实例；逐文件/逐请求拨号）
type receiveClient struct {
	dialAddr      string
	token         string
	instanceID    string
	passwordHash  string
	cip           *e2eCipher
	opts          sessionRuntimeOptions
	readIdle      time.Duration
	handshakeWait time.Duration
}

// newReceiveClient 构建收件人客户端（构建 E2E 密码学对象；密钥非法即失败）
func newReceiveClient(p *shareReceivePayload, instanceID string, opts sessionRuntimeOptions) (*receiveClient, error) {
	key, err := decodeShareKeyB64(p.KeyB64)
	if err != nil {
		return nil, err
	}
	cip, err := newE2ECipher(key)
	if err != nil {
		return nil, err
	}
	merged := defaultRuntimeOptions().withOverrides(opts)
	readIdle := defaultRecipientReadIdle
	if opts.tunnelReadIdle > 0 {
		readIdle = opts.tunnelReadIdle // 测试覆写通道（生产用收件人默认值）
	}
	return &receiveClient{
		dialAddr:      p.RelayDial,
		token:         p.Token,
		instanceID:    instanceID,
		passwordHash:  p.PasswordHash,
		cip:           cip,
		opts:          merged,
		readIdle:      readIdle,
		handshakeWait: merged.handshakeWait,
	}, nil
}

// decodeShareKeyB64 解码任务载荷中的 E2E 密钥（base64url → 32 字节）
func decodeShareKeyB64(b64 string) ([]byte, error) {
	key, err := base64.RawURLEncoding.DecodeString(b64)
	if err != nil || len(key) != shareKeyLen {
		return nil, fmt.Errorf("%w：任务载荷密钥不合法", ErrShareLinkKeyBad)
	}
	return key, nil
}

// fetch 发起一次拉取请求（拨号 → 请求 → 半关闭 → 应答首记录）：成功返回 JSON 头与其后
// 的明文内容读取器（读取器在 EOF 处校验字节数与头 size 一致）。每次调用独立连接；
// ctx 取消时关闭底层连接令阻塞读立即中断（调用方据此尽快返回，暂停/停止语义）。
func (c *receiveClient) fetch(ctx context.Context, req *streamRequest) (*streamHeader, *streamContentReader, error) {
	conn, err := c.opts.dialFn(c.dialAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("连接中继失败: %w", err)
	}
	r := &streamContentReader{conn: conn, cip: c.cip, readIdle: c.readIdle}
	r.startCtxWatch(ctx)
	fail := func(err error) (*streamHeader, *streamContentReader, error) {
		_ = r.Close()
		return nil, nil, err
	}
	w := newFrameWriter(conn, c.opts.writeWait)
	// 拨号握手（HELLO role=recipient；action 省略，PROTOCOL.md §3）
	hello := &helloPayload{Role: "recipient", Token: c.token, InstanceID: c.instanceID, PasswordHash: c.passwordHash}
	if err := w.write(frameHello, 0, marshalHello(hello)); err != nil {
		return fail(fmt.Errorf("发送 HELLO 失败: %w", err))
	}
	_ = conn.SetReadDeadline(time.Now().Add(c.handshakeWait))
	fr, err := readFrame(conn, defaultMaxFrame)
	if err != nil {
		return fail(fmt.Errorf("读取 WELCOME 失败: %w", err))
	}
	switch fr.Type {
	case frameWelcome:
		// dial 应答为空载荷，连接即一条虚拟流
	case frameError:
		we, perr := parseWireErr(fr.Payload)
		if perr != nil {
			return fail(perr)
		}
		return fail(&relayDialError{code: we.Code, msg: we.Message})
	default:
		return fail(fmt.Errorf("握手期收到非预期帧 0x%02x", fr.Type))
	}

	// 单条加密请求记录 + 半关闭（请求-响应模式，PROTOCOL.md §11.2）
	plaintext, err := json.Marshal(req)
	if err != nil {
		return fail(err)
	}
	record, err := c.cip.sealRecord(plaintext)
	if err != nil {
		return fail(err)
	}
	if err := w.write(frameData, 1, record); err != nil {
		return fail(fmt.Errorf("发送请求失败: %w", err))
	}
	if err := w.write(frameStreamClose, 1, nil); err != nil {
		return fail(fmt.Errorf("半关闭失败: %w", err))
	}

	headRecord, err := r.nextRecord()
	if err != nil {
		return fail(err)
	}
	if len(headRecord) > receiveMaxHeaderBytes {
		return fail(fmt.Errorf("应答头记录超长: %d", len(headRecord)))
	}
	var head streamHeader
	if err := json.Unmarshal(headRecord, &head); err != nil {
		return fail(fmt.Errorf("解析应答头失败: %w", err))
	}
	if !head.OK {
		return fail(&streamAppError{code: head.Error})
	}
	r.expect = head.Size
	return &head, r, nil
}

// streamContentReader 应答内容读取器：逐帧读 DATA → 解密记录 → 明文透出，
// STREAM_CLOSE 即 EOF（校验累计字节与头 size 一致）；PING 回 PONG（长应答保活）。
type streamContentReader struct {
	conn      net.Conn
	cip       *e2eCipher
	readIdle  time.Duration
	buf       []byte // 当前记录未消费明文
	expect    int64  // 头声明的总字节数（EOF 校验）
	got       int64  // 已消费字节数
	closed    bool
	stopWatch chan struct{} // ctx 取消 watcher 停止信号（Close 收尾）
	stopOnce  sync.Once
}

// startCtxWatch 启动 ctx 取消 watcher：ctx 取消即关连接中断阻塞读（握手/应答期均生效，
// 暂停/停止尽快返回）；watcher 生命周期与读取器一致（Close 停止）。
func (r *streamContentReader) startCtxWatch(ctx context.Context) {
	stop := make(chan struct{})
	r.stopWatch = stop
	go func() {
		select {
		case <-ctx.Done():
			_ = r.conn.Close()
		case <-stop:
		}
	}()
}

// nextRecord 读取并解密下一条应答记录（缓冲耗尽时）
func (r *streamContentReader) nextRecord() ([]byte, error) {
	for {
		_ = r.conn.SetReadDeadline(time.Now().Add(r.readIdle))
		fr, err := readFrame(r.conn, defaultMaxFrame)
		if err != nil {
			return nil, err
		}
		switch fr.Type {
		case frameData:
			if fr.StreamID != 1 {
				return nil, fmt.Errorf("非预期流 ID %d", fr.StreamID)
			}
			plaintext, err := r.cip.openRecord(fr.Payload)
			if err != nil {
				return nil, fmt.Errorf("解密记录失败（密钥不符或数据损坏）: %w", err)
			}
			return plaintext, nil
		case framePing:
			w := newFrameWriter(r.conn, 30*time.Second)
			if err := w.write(framePong, 0, fr.Payload); err != nil {
				return nil, err
			}
		case framePong:
			// 保活应答：忽略
		case frameStreamClose:
			return nil, io.EOF
		case frameError:
			we, perr := parseWireErr(fr.Payload)
			if perr != nil {
				return nil, perr
			}
			return nil, &relayDialError{code: we.Code, msg: we.Message}
		default:
			return nil, fmt.Errorf("应答期收到非预期帧 0x%02x", fr.Type)
		}
	}
}

// Read 实现 io.Reader（跨记录连续读取）
func (r *streamContentReader) Read(p []byte) (int, error) {
	for len(r.buf) == 0 {
		rec, err := r.nextRecord()
		if err != nil {
			if errors.Is(err, io.EOF) {
				if r.expect > 0 && r.got != r.expect {
					return 0, errStreamTruncated
				}
				return 0, io.EOF
			}
			return 0, err
		}
		r.buf = rec
	}
	n := copy(p, r.buf)
	r.buf = r.buf[n:]
	r.got += int64(n)
	return n, nil
}

// Close 停止 ctx watcher 并关闭底层连接（幂等）
func (r *streamContentReader) Close() error {
	r.stopOnce.Do(func() {
		if r.stopWatch != nil {
			close(r.stopWatch)
		}
	})
	if r.closed {
		return nil
	}
	r.closed = true
	return r.conn.Close()
}
