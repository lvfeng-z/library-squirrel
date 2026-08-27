package share

// 分享服务：把用户选中的作品/作品集发布为分享会话（复用 export 数据面收集 → 中继注册 →
// 出站隧道常驻），并提供撤销/状态查询。
// 二期任务化（share-host）：发布入口创建 share-host 任务并交 taskManager 执行
// （具备任务标准能力：状态机/进度/暂停恢复/停止/重试，重启不自动重建、按任务标准语义停留）；
// 会话宿主主体在 HostSession（由 HostExecution 策略驱动），会话运行态仍为进程内注册表
// （App 退出即隧道断、链接失效属设计语义；任务行持久化承载重试/恢复语义）。

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/export"
	"github.com/library-squirrel/backend/util"
)

// 发布错误定义
var (
	// ErrShareEmptySelection 分享选择为空
	ErrShareEmptySelection = errors.New("请至少选择一个作品或作品集")
	// ErrShareRelayNotConfigured 未配置分享中继地址
	ErrShareRelayNotConfigured = errors.New("未配置分享中继地址，请先在设置中配置")
	// ErrShareWorkDirEmpty 工作目录未配置
	ErrShareWorkDirEmpty = errors.New("工作目录未配置，无法分享")
	// ErrShareNotFound 分享会话不存在
	ErrShareNotFound = errors.New("分享会话不存在")
	// ErrShareTaskControlNil 未注入任务控制能力（装配缺失）
	ErrShareTaskControlNil = errors.New("分享任务控制能力未装配")
	// ErrSharePayloadInvalid share-host 任务载荷非法（版本不支持/JSON 不解析）
	ErrSharePayloadInvalid = errors.New("分享任务载荷非法")
)

// 内置任务类型（登记于 task.task_type，注册进 taskManager 执行面策略表）
const (
	// TaskTypeHost 分享方：宿主分享会话（收集 → 注册中继 → 隧道维持至终态）
	TaskTypeHost = "share-host"
	// TaskTypeReceive 收件方：经中继拉取分享并回灌导入（数据流归下阶段，仅类型与执行器骨架）
	TaskTypeReceive = "share-receive"
)

// shareHostPayloadSchemaVersion share-host 任务载荷格式版本
const shareHostPayloadSchemaVersion = 1

// shareHostPayload share-host 任务载荷（创建时序列化入 task.payload，恢复/重试重执行时反解）
type shareHostPayload struct {
	SchemaVersion int     `json:"schemaVersion"` // 载荷格式版本（高于自身支持即失败，防静默数据损坏）
	WorkIDs       []int64 `json:"workIds"`
	WorkSetIDs    []int64 `json:"workSetIds"`
	Title         string  `json:"title"`
	ExpireSeconds int64   `json:"expireSeconds"` // -1=中继默认 / 0=无限期 / >0=自定义秒
	PasswordHash  string  `json:"passwordHash"`  // sha256 hex；访问密码明文不落库，重注册直接复用摘要
}

// newShareHostPayload 构建并序列化任务载荷（明文密码在此转为摘要）
func newShareHostPayload(workIDs, workSetIDs []int64, options SharePublishOptions) (string, error) {
	hash := ""
	if options.Password != "" {
		hash = PasswordHashHex(options.Password)
	}
	b, err := json.Marshal(&shareHostPayload{
		SchemaVersion: shareHostPayloadSchemaVersion,
		WorkIDs:       workIDs,
		WorkSetIDs:    workSetIDs,
		Title:         options.Title,
		ExpireSeconds: options.ExpireSeconds,
		PasswordHash:  hash,
	})
	if err != nil {
		return "", fmt.Errorf("%w：序列化失败 %v", ErrSharePayloadInvalid, err)
	}
	return string(b), nil
}

// parseShareHostPayload 反解任务载荷（版本锚校验，高于支持版本 fail-fast）
func parseShareHostPayload(raw string) (*shareHostPayload, error) {
	if raw == "" {
		return nil, fmt.Errorf("%w：载荷为空", ErrSharePayloadInvalid)
	}
	var p shareHostPayload
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return nil, fmt.Errorf("%w：%v", ErrSharePayloadInvalid, err)
	}
	if p.SchemaVersion > shareHostPayloadSchemaVersion {
		return nil, fmt.Errorf("%w：载荷版本 %d 高于支持的 %d", ErrSharePayloadInvalid, p.SchemaVersion, shareHostPayloadSchemaVersion)
	}
	return &p, nil
}

// ShareIDFromTaskID 由任务 ID 推导分享会话 ID（"share-{taskId}"，创建即定、重启不变，
// 会话注册表与任务行双向可推导）
func ShareIDFromTaskID(taskID int64) string {
	return fmt.Sprintf("share-%d", taskID)
}

// TaskIDFromShareID 反解任务 ID（格式不符返回 false）
func TaskIDFromShareID(shareID string) (int64, bool) {
	s, ok := strings.CutPrefix(shareID, "share-")
	if !ok {
		return 0, false
	}
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

// BuiltinTaskControl share-host 任务生命周期控制能力（task.Service.CreateBuiltinTask 与
// taskManager.Manager 的启停经 app.go 适配器组合装配；taskCtl 经延迟闭包取用，装配时序上
// ShareService 先于 taskManager 创建）。
type BuiltinTaskControl interface {
	// CreateBuiltinTask 创建内置类型任务（返回任务 ID）
	CreateBuiltinTask(ctx context.Context, taskType string, taskName string, payload string) (int64, error)
	// StartTasks 启动任务树
	StartTasks(ctx context.Context, taskIds []int64) error
	// StopTasks 停止任务树
	StopTasks(ctx context.Context, taskIds []int64) error
}

// HostOutcome share-host 任务主体一次执行的结果分类（HostExecution 据此映射任务终态）
type HostOutcome int

const (
	// HostFinished 会话终态自然结束（撤销/过期）→ 任务成功终态
	HostFinished HostOutcome = iota
	// HostFailed 主体失败（发布失败/会话失败终态）→ 任务失败终态
	HostFailed
	// HostInterrupted ctx 取消（任务暂停/停止）→ 不上报终态，控制面接管
	HostInterrupted
)

// ProgressReporter 任务进度上报（total/finished 阶段步进；nil=不上报）
type ProgressReporter func(total, finished int64)

// 默认中继 TCP 端口（中继配置 listenAddr 默认 0.0.0.0:9527）
const defaultRelayPort = "9527"

// instanceIDPattern 中继对 instanceId 的形态约束（[A-Za-z0-9-]{8,128}）
var instanceIDPattern = regexp.MustCompile(`^[A-Za-z0-9-]{8,128}$`)

// ExportCollector 分享数据面依赖：复用导出收集能力（发起方定义接口，export.Service 实现）
type ExportCollector interface {
	Collect(ctx context.Context, workIDs []int64, workSetIDs []int64) (*export.ExportModel, error)
}

// ExportPlanner 分享数据面依赖：复用导出打包规划（填充包内路径/大小/缺失标记，不写盘）
type ExportPlanner interface {
	Plan(ctx context.Context, workDir string, model *export.ExportModel) (*export.PackStats, error)
}

// SharePublishOptions 发布选项（IPC 入参）
type SharePublishOptions struct {
	// Title 落地页标题；空=默认标题（已选 N 项分享）
	Title string `json:"title"`
	// ExpireSeconds 有效期秒数：-1=中继默认(7 天)；0=无限期；>0=自定义秒数
	ExpireSeconds int64 `json:"expireSeconds"`
	// Password 访问密码；空=无密码（明文仅在本机使用，线上只走 sha256 摘要；落任务载荷的只有摘要）
	Password string `json:"password"`
}

// Service 分享服务
type Service struct {
	collector  ExportCollector
	planner    ExportPlanner
	relayAddr  func() string // 中继地址（settings.shareSettings.relayAddress）
	workDir    func() string
	instanceID string                // 设备绑定实例 ID（溯源锚点，持久化于程序根 config/）
	emitter    ShareEventEmitter     // nil=不发事件（单测场景）
	taskCtl    BuiltinTaskControl    // share-host 任务创建/启停能力（app.go 装配）
	opts       sessionRuntimeOptions // 测试覆写（零值=默认）

	mu       sync.Mutex
	sessions map[string]*shareSession // shareId → 会话（含终态，列表展示用）
	// 深链到达缓存（冷启动衔接：事件先于前端就绪时暂存，前端启动后 ConsumeIncomingLink 取走）
	pendingIncomingLink string
	lastIncomingLink    string
	lastIncomingLinkAt  time.Time
}

// NewService 创建分享服务。
// instanceID 为设备绑定实例 ID（LoadOrCreateInstanceID 产物，App 生命周期内恒定）；
// taskCtl 为 share-host 任务控制能力（延迟闭包适配器，运行期取用）。
func NewService(collector ExportCollector, planner ExportPlanner, relayAddr func() string,
	workDir func() string, instanceID string, emitter ShareEventEmitter, taskCtl BuiltinTaskControl) *Service {
	return &Service{
		collector:  collector,
		planner:    planner,
		relayAddr:  relayAddr,
		workDir:    workDir,
		instanceID: instanceID,
		emitter:    emitter,
		taskCtl:    taskCtl,
		sessions:   make(map[string]*shareSession),
	}
}

// setTunables 覆写会话运行参数（仅测试使用）
func (s *Service) setTunables(opts sessionRuntimeOptions) {
	s.opts = opts
}

// Publish 启动任务化分享发布：创建 share-host 任务并立即启动（用户点「开始分享」即显式触发），
// 返回 shareID（"share-{taskId}"）。进度/完成/会话状态经 share-events 推送，任务运行态
// （暂停/停止/重试/失败信息）由任务面板标准能力承载。
func (s *Service) Publish(ctx context.Context, workIDs []int64, workSetIDs []int64, options SharePublishOptions) (string, error) {
	if len(workIDs) == 0 && len(workSetIDs) == 0 {
		return "", ErrShareEmptySelection
	}
	if strings.TrimSpace(s.relayAddr()) == "" {
		return "", ErrShareRelayNotConfigured
	}
	if s.workDir() == "" {
		return "", ErrShareWorkDirEmpty
	}
	if s.taskCtl == nil {
		return "", ErrShareTaskControlNil
	}
	payload, err := newShareHostPayload(workIDs, workSetIDs, options)
	if err != nil {
		return "", err
	}
	taskID, err := s.taskCtl.CreateBuiltinTask(ctx, TaskTypeHost,
		defaultTitle(len(workIDs), len(workSetIDs), options.Title), payload)
	if err != nil {
		return "", err
	}
	if err := s.taskCtl.StartTasks(ctx, []int64{taskID}); err != nil {
		return "", err
	}
	return ShareIDFromTaskID(taskID), nil
}

// CancelPublish 取消分享（停止 share-host 任务；任务停止经标准语义置 Failed）。
// 已在线会话的撤销走 Revoke（REVOKE 帧直达中继即时生效）。
func (s *Service) CancelPublish(ctx context.Context, shareID string) {
	taskID, ok := TaskIDFromShareID(shareID)
	if !ok || s.taskCtl == nil {
		return
	}
	if err := s.taskCtl.StopTasks(ctx, []int64{taskID}); err != nil {
		logger.Log.Warnf("[share] 停止分享任务失败 shareId=%s: %v", shareID, err)
	}
}

// Revoke 撤销分享会话（在线：REVOKE 帧直达中继即时生效；离线：本地终止）。
// 发布中（尚未注册在线）的分享不在会话注册表内，取消走 CancelPublish。
func (s *Service) Revoke(ctx context.Context, shareID string) error {
	s.mu.Lock()
	sess, ok := s.sessions[shareID]
	s.mu.Unlock()
	if !ok {
		return ErrShareNotFound
	}
	sess.Revoke()
	return nil
}

// Sessions 全部会话快照（含终态会话；按创建时间升序）
func (s *Service) Sessions(ctx context.Context) []*ShareSessionDTO {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*ShareSessionDTO, 0, len(s.sessions))
	for _, sess := range s.sessions {
		out = append(out, sess.snapshot())
	}
	// 创建时间升序（map 遍历序不定，展示需稳定）
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].CreatedAt < out[j-1].CreatedAt; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// HostSession 执行分享宿主主体（share-host 任务的执行面，阻塞直至终态或 ctx 取消）：
// 收集 → 规划（包内路径/白名单）→ 生成密钥 → 注册会话 → 首次在线 → 隧道维持至会话终态。
// 阶段进度经 prog 上报（步进 total=3）；发布失败/成功仍经 share-events 推 complete。
// ctx 取消（任务暂停/停止）时返回 HostInterrupted：会话本地终止并移出注册表，终态由任务控制面接管。
func (s *Service) HostSession(ctx context.Context, shareID string, payload *shareHostPayload, prog ProgressReporter) (HostOutcome, string) {
	fail := func(errMsg string) (HostOutcome, string) {
		if s.emitter != nil {
			s.emitter.PushComplete(ShareCompleteData{ShareID: shareID, Success: false, ErrMsg: errMsg})
		}
		s.removeSession(shareID)
		return HostFailed, errMsg
	}
	interrupted := func() (HostOutcome, string) {
		// 首次在线前中止（暂停/停止）：推送取消终态供发布弹窗收尾，任务终态交控制面
		if s.emitter != nil {
			s.emitter.PushComplete(ShareCompleteData{ShareID: shareID, Success: false, ErrMsg: "已取消"})
		}
		s.removeSession(shareID)
		return HostInterrupted, ""
	}
	phase := func(finished int64, name string) {
		if s.emitter != nil {
			s.emitter.PushProgress(shareID, name)
		}
		if prog != nil {
			prog(3, finished)
		}
	}

	phase(1, "collecting")
	model, err := s.collector.Collect(ctx, payload.WorkIDs, payload.WorkSetIDs)
	if err != nil {
		if ctx.Err() != nil {
			return interrupted()
		}
		return fail(cancelAwareMsg(ctx, err))
	}
	workDir := s.workDir()
	if workDir == "" {
		return fail(ErrShareWorkDirEmpty.Error())
	}
	if _, err := s.planner.Plan(ctx, workDir, model); err != nil {
		if ctx.Err() != nil {
			return interrupted()
		}
		return fail(cancelAwareMsg(ctx, err))
	}

	dialAddr, relayHost, err := normalizeRelayAddress(strings.TrimSpace(s.relayAddr()))
	if err != nil {
		return fail(err.Error())
	}

	// 发给收件人的 manifest JSON（含包内路径/大小/缺失标记；sha256 不预计算——见 README 边界）
	manifestData, err := model.Manifest.Serialize()
	if err != nil {
		return fail(fmt.Sprintf("序列化 manifest 失败: %v", err))
	}

	key, err := GenerateShareKey()
	if err != nil {
		return fail(err.Error())
	}

	cfg := sessionConfig{
		id:           shareID,
		title:        SanitizeMetaText(defaultTitle(len(payload.WorkIDs), len(payload.WorkSetIDs), payload.Title), 200),
		instanceID:   s.instanceID,
		relayDial:    dialAddr,
		relayHost:    relayHost,
		workDir:      workDir,
		key:          key,
		model:        model,
		manifestData: manifestData,
		passwordHash: payload.PasswordHash,
		expireSecs:   mapExpireSeconds(payload.ExpireSeconds),
		createdAt:    util.GetCurrentTimestamp(),
		emitter:      s.emitter,
		opts:         s.opts,
	}
	sess, err := newShareSession(cfg)
	if err != nil {
		return fail(err.Error())
	}
	s.mu.Lock()
	s.sessions[shareID] = sess
	s.mu.Unlock()

	phase(2, "registering")
	go sess.run(ctx)

	select {
	case firstErr := <-sess.firstCh:
		if firstErr != nil {
			return fail(firstErr.Error())
		}
	case <-ctx.Done():
		return interrupted()
	}

	link := BuildShareLink(relayHost, sess.tokenOf(), key)
	sess.setLink(link)
	sess.emitSnapshot()
	if s.emitter != nil {
		s.emitter.PushComplete(ShareCompleteData{
			ShareID: shareID,
			Success: true,
			Link:    link,
			Session: sess.snapshot(),
		})
	}
	if prog != nil {
		prog(3, 3)
	}
	logger.Log.Infof("[share] 分享发布成功 shareId=%s token=%s relay=%s", shareID, sess.tokenOf(), relayHost)

	// 隧道维持：等待会话终态或任务中断（暂停/停止）
	select {
	case <-sess.doneCh:
	case <-ctx.Done():
	}
	if ctx.Err() != nil && !sess.terminal() {
		// 中断：会话本地终止（中继侧存续至有效期到期，与撤销离线路径同语义），移出注册表；
		// 恢复/重试将重新注册（新 token/密钥/链接，与重启不自动重建的设计语义一致）
		s.removeSession(shareID)
		return HostInterrupted, ""
	}
	switch sess.currentState() {
	case stateRevoked, stateExpired:
		// 会话终态自然结束：宿主职责完成
		return HostFinished, ""
	case stateFailed:
		return HostFailed, sess.snapshot().ErrMsg
	default:
		// doneCh 已关但非终态且 ctx 未取消：会话主循环异常退出的防御路径
		s.removeSession(shareID)
		return HostInterrupted, ""
	}
}

// removeSession 移除会话（中止/失败路径：不再对外提供列表展示）
func (s *Service) removeSession(shareID string) {
	s.mu.Lock()
	delete(s.sessions, shareID)
	s.mu.Unlock()
}

// mapExpireSeconds 选项映射 HELLO expireSeconds：-1→nil（中继默认）；0→&0（无限期）；>0→&n
func mapExpireSeconds(opt int64) *int64 {
	switch {
	case opt < 0:
		return nil
	case opt == 0:
		zero := int64(0)
		return &zero
	default:
		v := opt
		return &v
	}
}

// defaultTitle 落地页标题：用户提供则用之，否则「分享 N 个作品/M 个作品集」
func defaultTitle(workCount, workSetCount int, userTitle string) string {
	if strings.TrimSpace(userTitle) != "" {
		return userTitle
	}
	switch {
	case workCount > 0 && workSetCount > 0:
		return fmt.Sprintf("分享 %d 个作品与 %d 个作品集", workCount, workSetCount)
	case workSetCount > 0:
		return fmt.Sprintf("分享 %d 个作品集", workSetCount)
	default:
		return fmt.Sprintf("分享 %d 个作品", workCount)
	}
}

// normalizeRelayAddress 规范化中继地址：剥离 scheme；无端口补默认 9527（拨号用）；
// 链接 host 保留用户书写形态（host 或 host:port）。
func normalizeRelayAddress(addr string) (dialAddr, host string, err error) {
	a := strings.TrimSpace(addr)
	a = strings.TrimPrefix(a, "https://")
	a = strings.TrimPrefix(a, "http://")
	a = strings.TrimPrefix(a, "tcp://")
	a = strings.TrimSuffix(a, "/")
	if a == "" || strings.ContainsAny(a, " /\\") {
		return "", "", fmt.Errorf("分享中继地址非法: %q", addr)
	}
	host = a
	dialAddr = a
	if !strings.Contains(a, ":") {
		dialAddr = a + ":" + defaultRelayPort
	}
	return dialAddr, host, nil
}

// cancelAwareMsg ctx 已取消（用户主动中止）时报「已取消」，否则透传原始错误
func cancelAwareMsg(ctx context.Context, err error) string {
	if ctx.Err() != nil {
		return "已取消"
	}
	return err.Error()
}

// LoadOrCreateInstanceID 读取或创建设备绑定实例 ID（中继溯源锚点）。
// 持久化为程序根 config/ 下小文件；文件缺失/非法时生成 16 字节随机 hex（32 字符，满足
// [A-Za-z0-9-]{8,128}）。设备换机/重装即新实例（溯源语义即「设备维度」，非账号维度）。
func LoadOrCreateInstanceID(path string) (string, error) {
	if b, err := os.ReadFile(path); err == nil {
		id := strings.TrimSpace(string(b))
		if instanceIDPattern.MatchString(id) {
			return id, nil
		}
	} else if !os.IsNotExist(err) {
		logger.Log.Warnf("[share] 读取实例 ID 文件失败: %v", err)
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("生成实例 ID 失败: %w", err)
	}
	id := fmt.Sprintf("%x", buf)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("创建配置目录失败: %w", err)
	}
	if err := os.WriteFile(path, []byte(id), 0o644); err != nil {
		return "", fmt.Errorf("写入实例 ID 失败: %w", err)
	}
	return id, nil
}
