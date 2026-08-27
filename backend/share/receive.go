package share

// 收件人侧分享拉取：链接解析（深链 / https 分享链接）→ share-receive 任务创建。
// 拉取数据流主体（经中继拨号 → 流内协议拉 manifest/文件 → 暂存续传 → ManifestIngestor
// 回灌导入）在 task_execution.go 的 ReceiveExecution（任务执行面）与 receive_client.go
// （收件人协议客户端）中实现。

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/library-squirrel/backend/base/logger"
)

// 深链常量（与中继落地页深链格式一致，PROTOCOL.md §6：library-squirrel://share/{relay}/{token}）
const (
	// DeepLinkScheme 分享深链协议 scheme
	DeepLinkScheme = "library-squirrel"
	// DeepLinkHost 深链 authority（路径首段固定为 share，表达「拉取分享」动作）
	DeepLinkHost = "share"
)

// 深链解析错误定义（信息直接面向用户，经对话框展示）
var (
	// ErrShareLinkInvalid 分享链接形态不合法
	ErrShareLinkInvalid = errors.New("分享链接格式不正确")
	// ErrShareLinkNoKey 链接缺少 fragment 密钥（无法解密分享内容）
	ErrShareLinkNoKey = errors.New("分享链接缺少解密密钥（#k=…），请复制完整链接")
	// ErrShareLinkKeyBad 密钥编码不合法或长度不符
	ErrShareLinkKeyBad = errors.New("分享链接中的解密密钥不合法")
)

// shareTokenPattern 中继 token 形态（PROTOCOL.md §7：22 字符 base64url，128 bit 熵）
var shareTokenPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{22}$`)

// ReceiveTarget 一次收件拉取的目标参数（链接解析产物，进任务载荷）
type ReceiveTarget struct {
	RelayDial string // 中继 TCP 拨号地址（host:port，无端口已补默认 9527）
	RelayHost string // 链接 host 形态（host 或 host:port，展示用）
	Token     string // 中继会话 token（访问凭证）
	Key       []byte // E2E 密钥（AES-256，32 字节；只存在于分享双方，不经中继）
}

// ParseShareLink 解析分享链接，接受两种形态：
//   - 深链：library-squirrel://share/{relay}/{token}#k={base64url 密钥}
//   - 落地页链接：https://{relay}/s/{token}#k={base64url 密钥}
//
// 校验：token 字符集（22 字符 base64url）、中继地址字符集（复用发布侧规范化）、
// 密钥 base64url 解码后恰为 32 字节。恶意深链是新输入面（scheme/host/token/密钥全字段白名单校验）。
func ParseShareLink(link string) (*ReceiveTarget, error) {
	s := strings.TrimSpace(link)
	if s == "" || len(s) > 2048 || strings.ContainsAny(s, " \t\r\n") {
		return nil, fmt.Errorf("%w：链接为空、过长或含空白字符", ErrShareLinkInvalid)
	}
	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("%w：%v", ErrShareLinkInvalid, err)
	}
	var relayHost, token string
	switch strings.ToLower(u.Scheme) {
	case DeepLinkScheme:
		if !strings.EqualFold(u.Host, DeepLinkHost) {
			return nil, fmt.Errorf("%w：深链 host 须为 %s", ErrShareLinkInvalid, DeepLinkHost)
		}
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("%w：深链路径须为 /{中继地址}/{token}", ErrShareLinkInvalid)
		}
		relayHost, token = parts[0], parts[1]
	case "https", "http":
		pathParts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(pathParts) != 2 || pathParts[0] != "s" || pathParts[1] == "" {
			return nil, fmt.Errorf("%w：分享链接路径须为 /s/{token}", ErrShareLinkInvalid)
		}
		relayHost, token = u.Host, pathParts[1]
	default:
		return nil, fmt.Errorf("%w：不支持的 scheme %q", ErrShareLinkInvalid, u.Scheme)
	}
	if !shareTokenPattern.MatchString(token) {
		return nil, fmt.Errorf("%w：token 形态不合法", ErrShareLinkInvalid)
	}
	dialAddr, host, err := normalizeRelayAddress(relayHost)
	if err != nil {
		return nil, fmt.Errorf("%w：中继地址不合法", ErrShareLinkInvalid)
	}
	key, err := parseShareLinkKey(u)
	if err != nil {
		return nil, err
	}
	return &ReceiveTarget{RelayDial: dialAddr, RelayHost: host, Token: token, Key: key}, nil
}

// parseShareLinkKey 提取并校验链接携带的 E2E 密钥（fragment 优先，兼容 query 形态）。
func parseShareLinkKey(u *url.URL) ([]byte, error) {
	kv := u.Fragment
	if kv == "" {
		kv = u.RawQuery
	}
	v, ok := strings.CutPrefix(kv, "k=")
	if !ok || v == "" {
		return nil, ErrShareLinkNoKey
	}
	key, err := base64.RawURLEncoding.DecodeString(v)
	if err != nil {
		return nil, fmt.Errorf("%w：%v", ErrShareLinkKeyBad, err)
	}
	if len(key) != shareKeyLen {
		return nil, fmt.Errorf("%w：长度 %d 非 32 字节", ErrShareLinkKeyBad, len(key))
	}
	return key, nil
}

// —— share-receive 任务载荷 ——

// shareReceivePayloadSchemaVersion share-receive 任务载荷格式版本
const shareReceivePayloadSchemaVersion = 1

// shareReceivePayload share-receive 任务载荷（创建时序列化入 task.payload，
// 暂停恢复/重试/跨重启重执行时反解；E2E 密钥与密码摘要落本机任务行，属收件人自有数据）
type shareReceivePayload struct {
	SchemaVersion int    `json:"schemaVersion"` // 载荷格式版本（高于自身支持即失败，防静默数据损坏）
	RelayDial     string `json:"relayDial"`     // 中继 TCP 拨号地址（host:port）
	RelayHost     string `json:"relayHost"`     // 中继展示地址
	Token         string `json:"token"`         // 会话 token
	KeyB64        string `json:"keyB64"`        // E2E 密钥（base64url）
	PasswordHash  string `json:"passwordHash"`  // 访问密码摘要（sha256 hex；空=无密码）
}

// newShareReceivePayload 构建并序列化任务载荷（明文密码在此转为摘要）
func newShareReceivePayload(target *ReceiveTarget, password string) (string, error) {
	hash := ""
	if password != "" {
		hash = PasswordHashHex(password)
	}
	b, err := json.Marshal(&shareReceivePayload{
		SchemaVersion: shareReceivePayloadSchemaVersion,
		RelayDial:     target.RelayDial,
		RelayHost:     target.RelayHost,
		Token:         target.Token,
		KeyB64:        base64.RawURLEncoding.EncodeToString(target.Key),
		PasswordHash:  hash,
	})
	if err != nil {
		return "", fmt.Errorf("%w：序列化失败 %v", ErrSharePayloadInvalid, err)
	}
	return string(b), nil
}

// parseShareReceivePayload 反解任务载荷（版本锚校验，高于支持版本 fail-fast）
func parseShareReceivePayload(raw string) (*shareReceivePayload, error) {
	if raw == "" {
		return nil, fmt.Errorf("%w：载荷为空", ErrSharePayloadInvalid)
	}
	var p shareReceivePayload
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return nil, fmt.Errorf("%w：%v", ErrSharePayloadInvalid, err)
	}
	if p.SchemaVersion > shareReceivePayloadSchemaVersion {
		return nil, fmt.Errorf("%w：载荷版本 %d 高于支持的 %d", ErrSharePayloadInvalid, p.SchemaVersion, shareReceivePayloadSchemaVersion)
	}
	return &p, nil
}

// Receive 启动收件拉取：解析链接（深链或 https 分享链接）→ 创建 share-receive 任务并立即
// 启动（拉取主体经任务执行面 ReceiveExecution），返回任务 ID（进度/终态由任务面板承载）。
// password 为分享设有访问密码时的明文（仅本机使用，落任务载荷的只有 sha256 摘要）。
func (s *Service) Receive(ctx context.Context, link string, password string) (int64, error) {
	target, err := ParseShareLink(link)
	if err != nil {
		return 0, err
	}
	if s.taskCtl == nil {
		return 0, ErrShareTaskControlNil
	}
	payload, err := newShareReceivePayload(target, password)
	if err != nil {
		return 0, err
	}
	taskID, err := s.taskCtl.CreateBuiltinTask(ctx, TaskTypeReceive,
		fmt.Sprintf("拉取分享（%s）", target.RelayHost), payload)
	if err != nil {
		return 0, err
	}
	if err := s.taskCtl.StartTasks(ctx, []int64{taskID}); err != nil {
		return 0, err
	}
	return taskID, nil
}

// —— 深链到达入口（main.go 三通道汇入：单实例二启转发 / URL 事件 / argv 兜底）——

// incomingLinkDedupeWindow 同一深链的去重窗口（冷启动 URL 事件与 argv 兜底双通道同源）
const incomingLinkDedupeWindow = 3 * time.Second

// FindShareDeepLinkArg 从进程参数中扫描深链 URL（argv 兜底通道：URL 事件判定要求
// len(args)==2 且含 "://"，dev 模式附加参数不满足——前缀扫描补位）
func FindShareDeepLinkArg(args []string) string {
	prefix := DeepLinkScheme + "://"
	for _, a := range args {
		if len(a) > len(prefix) && strings.HasPrefix(strings.ToLower(a), prefix) &&
			!strings.ContainsAny(a, " \t\r\n") {
			return a
		}
	}
	return ""
}

// NotifyIncomingLink 深链到达：轻校验（scheme/host 前缀，完整校验在 Receive）后缓存
// 待前端消费 + 推 receive-link 事件。事件可能先于前端就绪（冷启动）而丢失，由
// ConsumeIncomingLink 消费式拉取兜底；窗口期内重复同链去重。
// 返回是否接受（非法形态记日志丢弃——恶意深链是新输入面）。
func (s *Service) NotifyIncomingLink(raw string) bool {
	if !looksLikeShareDeepLink(raw) {
		logger.Log.Warnf("[share] 忽略非法深链: %.200q", raw)
		return false
	}
	s.mu.Lock()
	sameLink := raw == s.lastIncomingLink && time.Since(s.lastIncomingLinkAt) < incomingLinkDedupeWindow
	s.lastIncomingLink = raw
	s.lastIncomingLinkAt = time.Now()
	if !sameLink {
		s.pendingIncomingLink = raw
	}
	s.mu.Unlock()
	if sameLink {
		return true
	}
	logger.Log.Infof("[share] 分享深链到达: %s", raw)
	if s.emitter != nil {
		s.emitter.PushReceiveLink(raw)
	}
	return true
}

// ConsumeIncomingLink 取走并清空缓存的待处理深链（前端启动衔接：注册事件监听后拉取，
// 冷启动期先于前端就绪到达的深链由此进入接收对话框）
func (s *Service) ConsumeIncomingLink() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	link := s.pendingIncomingLink
	s.pendingIncomingLink = ""
	return link
}

// looksLikeShareDeepLink 深链轻校验：scheme+host 前缀匹配、无空白、长度受限
// （完整校验——token/密钥/中继地址白名单——在 ParseShareLink）
func looksLikeShareDeepLink(raw string) bool {
	if raw == "" || len(raw) > 2048 || strings.ContainsAny(raw, " \t\r\n") {
		return false
	}
	prefix := DeepLinkScheme + "://" + DeepLinkHost + "/"
	return strings.HasPrefix(strings.ToLower(raw), prefix)
}
