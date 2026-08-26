package share

// 中继线协议客户端侧编解码（契约：../library-squirrel-relay/PROTOCOL.md，协议 version 1）。
// 帧布局（多字节整数大端）：
//
//	+---------+---------+--------+----------+--------+-----------------+
//	| magic   | version | type   | streamID | length | payload         |
//	| 2 字节  | 1 字节  | 1 字节 | 4 字节   | 4 字节 | length 字节     |
//	+---------+---------+--------+----------+--------+-----------------+
//
// 分享方（本模块）只使用：HELLO(register/bind)、DATA/STREAM_CLOSE（流）、PING/PONG（保活）、
// REVOKE（撤销）四组帧；WELCOME/ERROR/RESULT/STREAM_OPEN 由中继发来、读循环分发。
// 帧类型为封闭白名单，与中继实现严格一致，未知类型按断连处理。

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// 协议常量（与 PROTOCOL.md §2/§9 对齐）
const (
	protoMagic      = "LS"  // 帧魔数，兼作单口嗅探前缀
	protoVersion    = 1     // 协议版本
	frameHeaderSize = 12    // 帧头字节数
	defaultMaxFrame = 32768 // 单帧负载上限（中继 maxPayload 默认值；客户端分块恒小于它）
)

// 帧类型（封闭白名单）
const (
	frameHello       byte = 0x01 // 客户端→中继：角色与意图声明（连接首帧，streamID=0）
	frameWelcome     byte = 0x02 // 中继→客户端：HELLO 接受应答
	frameError       byte = 0x03 // 拒绝/错误，发送后断连
	frameStreamOpen  byte = 0x04 // 中继→分享方：新虚拟流到达
	frameStreamClose byte = 0x05 // 流正常结束（双向）
	frameData        byte = 0x06 // 流数据（E2E 密文，中继盲转）
	framePing        byte = 0x07 // 保活探测（双向）
	framePong        byte = 0x08 // 保活应答（回显 PING payload）
	frameRevoke      byte = 0x09 // 分享方→中继：撤销会话（仅隧道连接）
	frameResult      byte = 0x0A // 中继→分享方：控制操作应答
)

// 中继 ERROR code 枚举（PROTOCOL.md §4）。终态码不再重试。
const (
	errCodeMalformed   = "malformed"
	errCodeNotFound    = "not_found"
	errCodeExpired     = "expired"
	errCodeRevoked     = "revoked"
	errCodeBadPassword = "bad_password"
	errCodeBanned      = "banned"
	errCodeOffline     = "offline"
	errCodeLimit       = "limit"
	errCodeRateLimited = "rate_limited"
	errCodeServerError = "server_error"
)

// wireErr ERROR 帧载荷
type wireErr struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *wireErr) Error() string { return fmt.Sprintf("中继拒绝: %s: %s", e.Code, e.Message) }

// isTerminalErrCode 判定 ERROR code 是否为终态（PROTOCOL.md §11.5：revoked/expired/banned
// 不要重试；not_found 对 bind 也是终态——会话已不存在）。malformed 属本地构造缺陷，同样终态。
func isTerminalErrCode(code string) bool {
	switch code {
	case errCodeNotFound, errCodeExpired, errCodeRevoked, errCodeBanned, errCodeMalformed:
		return true
	}
	return false
}

// frame 单帧解码形态
type frame struct {
	Type     byte
	StreamID uint32
	Payload  []byte
}

// readFrame 从连接读一帧（帧级校验：魔数/版本/长度上限）
func readFrame(r io.Reader, maxPayload int) (frame, error) {
	if maxPayload <= 0 {
		maxPayload = defaultMaxFrame
	}
	var head [frameHeaderSize]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return frame{}, err
	}
	if string(head[0:2]) != protoMagic {
		return frame{}, fmt.Errorf("帧魔数不符")
	}
	if head[2] != protoVersion {
		return frame{}, fmt.Errorf("协议版本不符: %d", head[2])
	}
	typ := head[3]
	sid := binary.BigEndian.Uint32(head[4:8])
	length := binary.BigEndian.Uint32(head[8:12])
	if int(length) > maxPayload {
		return frame{}, fmt.Errorf("帧负载超长: %d", length)
	}
	payload := make([]byte, length)
	if length > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			return frame{}, err
		}
	}
	return frame{Type: typ, StreamID: sid, Payload: payload}, nil
}

// frameWriter 帧写出器：写互斥锁串行化（多流并发写同一连接时保证单帧原子性，PROTOCOL.md §11.1）
// + 每次写设超时（对齐中继 writeTimeoutSec 默认 30s）。
type frameWriter struct {
	conn      net.Conn
	writeMu   sync.Mutex
	writeWait time.Duration
}

func newFrameWriter(conn net.Conn, writeWait time.Duration) *frameWriter {
	if writeWait <= 0 {
		writeWait = 30 * time.Second
	}
	return &frameWriter{conn: conn, writeWait: writeWait}
}

// write 写出一帧（互斥串行化 + 写超时）
func (w *frameWriter) write(typ byte, streamID uint32, payload []byte) error {
	w.writeMu.Lock()
	defer w.writeMu.Unlock()
	_ = w.conn.SetWriteDeadline(time.Now().Add(w.writeWait))
	return writeFrameTo(w.conn, typ, streamID, payload)
}

// writeFrameTo 编码并写出一帧；单次 Write 保证帧原子性（并发写同一连接须由调用方串行化）
func writeFrameTo(w io.Writer, typ byte, streamID uint32, payload []byte) error {
	buf := make([]byte, frameHeaderSize+len(payload))
	buf[0] = protoMagic[0]
	buf[1] = protoMagic[1]
	buf[2] = protoVersion
	buf[3] = typ
	binary.BigEndian.PutUint32(buf[4:8], streamID)
	binary.BigEndian.PutUint32(buf[8:12], uint32(len(payload)))
	copy(buf[frameHeaderSize:], payload)
	_, err := w.Write(buf)
	return err
}

// helloPayload HELLO 载荷（字段集与中继 decodeHello 严格一致——中继 DisallowUnknownFields，
// 客户端不得携带契约外字段）。指针字段 nil 即省略。
type helloPayload struct {
	Role           string       `json:"role"`                     // sharer
	Action         string       `json:"action,omitempty"`         // register | bind
	Token          string       `json:"token,omitempty"`          // bind 必填
	InstanceID     string       `json:"instanceId"`               // 设备绑定实例 ID
	PasswordHash   string       `json:"passwordHash,omitempty"`   // hex(sha256(访问密码))，仅 register
	ExpireSeconds  *int64       `json:"expireSeconds,omitempty"`  // 仅 register：nil=中继默认；0=无限期；>0=自定义秒
	Meta           *metaPayload `json:"meta,omitempty"`           // 仅 register：落地页文字元数据
	CandidateAddrs []string     `json:"candidateAddrs,omitempty"` // V2 直连预留位，本客户端不携带
}

// metaPayload 落地页文字元数据（预览最小化：仅文字，无任何图像字段）
type metaPayload struct {
	Title     string `json:"title"`     // ≤200 字符、无控制字符
	WorkCount int64  `json:"workCount"` // 0..1e9
	Source    string `json:"source"`    // ≤100 字符（来源站点，落地页强制展示）
}

// welcomePayload WELCOME 载荷（register 应答含新 token；bind 应答仅 expiresAt）
type welcomePayload struct {
	Token     string `json:"token,omitempty"`
	ExpiresAt int64  `json:"expiresAt"` // unix 毫秒，0=无限期
}

// resultPayload RESULT 载荷（REVOKE 应答）
type resultPayload struct {
	OK     bool   `json:"ok"`
	Action string `json:"action"`
}

// marshalHello 序列化 HELLO 载荷（静态结构，序列化失败属程序缺陷）
func marshalHello(h *helloPayload) []byte {
	b, err := json.Marshal(h)
	if err != nil {
		panic(fmt.Sprintf("HELLO 载荷序列化失败: %v", err))
	}
	return b
}
