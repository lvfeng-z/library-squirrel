package share

// 收件人侧分享拉取：链接解析（深链 / https 分享链接）→ share-receive 任务创建。
// 拉取数据流主体（读本地共享 manifest → 经中继拨号逐文件拉取 → 暂存续传 → ManifestIngestor
// 回灌导入）在 task_execution.go 的 ReceiveExecution（任务执行面）与 receive_client.go
// （收件人协议客户端）中实现。

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/export"
	"github.com/library-squirrel/backend/task"
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

	// 子任务字段（父+子任务树形态，每作品一个子任务）：ManifestPath 为共享 manifest 的
	// workDir 相对路径（正斜杠 relPath 域），ManifestID 为本任务负责的 manifest 作品 ID。
	// 加可选字段不递增 schemaVersion——旧载荷新代码可解析（新字段零值 → ManifestID==0），
	// 执行面按过时载荷显式 Fail（决策2 不兼容存量）。
	ManifestPath string `json:"manifestPath"`
	ManifestID   int64  `json:"manifestID"`
}

// newShareReceivePayload 构建并序列化任务载荷（明文密码在此转为摘要；不含子任务定位字段）
func newShareReceivePayload(target *ReceiveTarget, password string) (string, error) {
	return buildShareReceivePayload(target, password, "", 0)
}

// newShareReceiveChildPayload 构建子任务载荷：基础连接参数 + 共享 manifest 定位与作品过滤。
// 子任务只存连接参数 + 清单定位 + 作品过滤，不重复存 manifest 内容（manifest 落盘共享文件）。
func newShareReceiveChildPayload(target *ReceiveTarget, password string, manifestPath string, manifestID int64) (string, error) {
	return buildShareReceivePayload(target, password, manifestPath, manifestID)
}

func buildShareReceivePayload(target *ReceiveTarget, password string, manifestPath string, manifestID int64) (string, error) {
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
		ManifestPath:  manifestPath,
		ManifestID:    manifestID,
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

// fetchManifest 同步拉取分享 manifest：复用执行面拉取壳（fetchWithRetry 瞬态退避 + readAllBody
// 应答全量读入），解序列化后返回。供 Receive 建树前预拉（决策1），与子任务执行读本地共享清单互补。
func fetchManifest(ctx context.Context, client *receiveClient) (*export.Manifest, error) {
	body, err := fetchWithRetry(ctx, client, &streamRequest{Type: "manifest"}, readAllBody)
	if err != nil {
		return nil, err
	}
	manifest, err := export.Deserialize(body)
	if err != nil {
		return nil, fmt.Errorf("解析分享清单失败: %w", err)
	}
	return manifest, nil
}

// writeSharedManifestFile 将 manifest 序列化落盘到 workDir 相对路径（relPath 域正斜杠；
// absPath 域仅存在于 os 调用点现场 join）。落盘失败由调用方按决策5 回滚删树。
func writeSharedManifestFile(workDir, relPath string, manifest *export.Manifest) error {
	if workDir == "" {
		return errors.New("工作目录未配置，无法保存分享清单")
	}
	data, err := manifest.Serialize()
	if err != nil {
		return fmt.Errorf("序列化分享清单失败: %w", err)
	}
	abs := filepath.Join(workDir, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	return os.WriteFile(abs, data, 0o644)
}

// Receive 启动收件拉取：解析链接（深链或 https 分享链接）→ 同步预拉 manifest（决策1）→
// 建父子任务树（父「拉取分享（{host}）」容器 + 每作品一子任务）→ 共享 manifest 落盘父任务目录 →
// 整树启动 → 返回 {parentTaskId, workCount, workNames}（作品名列表供收件侧展示，决策4 之①）。
// password 为分享设有访问密码时的明文（仅本机使用，落任务载荷的只有 sha256 摘要）。
// 失败语义（决策5）：manifest 拉取失败不建任何任务；父任务创建后、子任务全部建成前任一步失败
// （manifest 落盘失败/建子任务中断）显式删除已建任务树（DeleteTask 含子任务），不留孤儿任务。
func (s *Service) Receive(ctx context.Context, link string, password string) (*ShareReceiveResult, error) {
	target, err := ParseShareLink(link)
	if err != nil {
		return nil, err
	}
	if s.taskCtl == nil {
		return nil, ErrShareTaskControlNil
	}
	// 收件人客户端（连接参数载荷 + 拨号）——预拉 manifest 与子任务后续拉取共用同形态连接参数
	payloadJSON, err := newShareReceivePayload(target, password)
	if err != nil {
		return nil, err
	}
	connPayload, err := parseShareReceivePayload(payloadJSON)
	if err != nil {
		return nil, err
	}
	client, err := newReceiveClient(connPayload, s.instanceID, s.opts)
	if err != nil {
		return nil, err
	}
	// 同步预拉 manifest（建树前唯一网络往返；失败无任务可重试，返回用户可读文案，修正后重新接收）
	manifest, err := fetchManifest(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("拉取分享清单失败: %s", receiveUserMessage(err))
	}
	if manifest.SchemaVersion != export.SchemaVersion {
		return nil, fmt.Errorf("分享清单版本不支持: %d", manifest.SchemaVersion)
	}
	if len(manifest.Works) == 0 {
		return nil, errors.New("分享清单中没有作品，无法接收")
	}
	// 作品名（净化后）为子任务命名与返回 DTO workNames 的共同来源（与落地页 worksName 三处一致）
	names := make([]string, 0, len(manifest.Works))
	for i := range manifest.Works {
		names = append(names, sanitizedWorkName(&manifest.Works[i]))
	}

	// 两段式建树：先建父容器拿 parentID（子任务载荷的 ManifestPath 依赖父目录路径），
	// 落盘共享 manifest 到父任务目录后补建子任务
	parent, err := s.taskCtl.CreateBuiltinTaskParent(ctx, TaskTypeReceive,
		fmt.Sprintf("拉取分享（%s）", target.RelayHost))
	if err != nil {
		return nil, err
	}
	parentID := parent.GetID()
	manifestRel := path.Join(receiveStagingRootName, strconv.FormatInt(parentID, 10), "manifest.json")
	if err := writeSharedManifestFile(s.workDir(), manifestRel, manifest); err != nil {
		_ = s.taskCtl.DeleteTask(ctx, []int64{parentID})
		return nil, fmt.Errorf("保存分享清单失败: %v", err)
	}
	children := make([]task.BuiltinTaskChild, 0, len(manifest.Works))
	for i := range manifest.Works {
		childPayload, err := newShareReceiveChildPayload(target, password, manifestRel, manifest.Works[i].ID)
		if err != nil {
			_ = s.taskCtl.DeleteTask(ctx, []int64{parentID})
			return nil, err
		}
		children = append(children, task.BuiltinTaskChild{TaskName: names[i], Payload: childPayload})
	}
	if err := s.taskCtl.CreateBuiltinTaskChildren(ctx, TaskTypeReceive, parentID, children); err != nil {
		_ = s.taskCtl.DeleteTask(ctx, []int64{parentID})
		return nil, err
	}
	// 整树启动（taskManager 按父 ID 加载整树并派发全部子任务）
	if err := s.taskCtl.StartTasks(ctx, []int64{parentID}); err != nil {
		return nil, err
	}
	return &ShareReceiveResult{ParentTaskID: parentID, WorkCount: len(manifest.Works), WorkNames: names}, nil
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
