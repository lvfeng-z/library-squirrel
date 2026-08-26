package share

// 分享服务：把用户选中的作品/作品集发布为分享会话（复用 export 数据面收集 → 中继注册 →
// 出站隧道常驻），并提供撤销/状态查询。发布沿用导出的异步轻量壳（后台 goroutine +
// 进度事件 + 可取消）；会话为进程内运行态（不落库——App 退出即隧道断、链接失效属设计语义，
// 二期任务模块接入后再承接持久化/恢复）。

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
)

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
	// Password 访问密码；空=无密码（明文仅在本机使用，线上只走 sha256 摘要）
	Password string `json:"password"`
}

// publishJob 一次进行中的发布：持有脱离 IPC handler ctx 的独立 ctx，供取消
type publishJob struct {
	cancel context.CancelFunc
}

// Service 分享服务
type Service struct {
	collector  ExportCollector
	planner    ExportPlanner
	relayAddr  func() string // 中继地址（settings.shareSettings.relayAddress）
	workDir    func() string
	instanceID string                // 设备绑定实例 ID（溯源锚点，持久化于程序根 config/）
	emitter    ShareEventEmitter     // nil=不发事件（单测场景）
	opts       sessionRuntimeOptions // 测试覆写（零值=默认）

	mu         sync.Mutex
	publishing map[string]*publishJob   // shareId → 进行中发布
	sessions   map[string]*shareSession // shareId → 会话（含终态，列表展示用）
}

// NewService 创建分享服务。
// instanceID 为设备绑定实例 ID（LoadOrCreateInstanceID 产物，App 生命周期内恒定）。
func NewService(collector ExportCollector, planner ExportPlanner, relayAddr func() string,
	workDir func() string, instanceID string, emitter ShareEventEmitter) *Service {
	return &Service{
		collector:  collector,
		planner:    planner,
		relayAddr:  relayAddr,
		workDir:    workDir,
		instanceID: instanceID,
		emitter:    emitter,
		publishing: make(map[string]*publishJob),
		sessions:   make(map[string]*shareSession),
	}
}

// setTunables 覆写会话运行参数（仅测试使用）
func (s *Service) setTunables(opts sessionRuntimeOptions) {
	s.opts = opts
}

// Publish 启动异步分享发布：前置校验通过即注册 job 并起独立 goroutine，立即返回 shareID。
// 进度/完成/会话状态经 share-events 事件推送。
func (s *Service) Publish(ctx context.Context, workIDs []int64, workSetIDs []int64, options SharePublishOptions) (string, error) {
	if len(workIDs) == 0 && len(workSetIDs) == 0 {
		return "", ErrShareEmptySelection
	}
	relay := strings.TrimSpace(s.relayAddr())
	if relay == "" {
		return "", ErrShareRelayNotConfigured
	}
	if s.workDir() == "" {
		return "", ErrShareWorkDirEmpty
	}
	shareID := fmt.Sprintf("share-%d", time.Now().UnixNano())
	runCtx, cancel := context.WithCancel(context.Background()) // detached：handler 返回后发布仍跑
	s.mu.Lock()
	s.publishing[shareID] = &publishJob{cancel: cancel}
	s.mu.Unlock()
	go s.runPublish(runCtx, shareID, workIDs, workSetIDs, options, relay)
	return shareID, nil
}

// CancelPublish 取消进行中的发布（已在线的会话撤销走 Revoke；无进行中发布则 no-op）
func (s *Service) CancelPublish(shareID string) {
	s.mu.Lock()
	job, ok := s.publishing[shareID]
	s.mu.Unlock()
	if ok {
		job.cancel()
	}
}

// Revoke 撤销分享会话（在线：REVOKE 帧直达中继即时生效；离线：本地终止）
func (s *Service) Revoke(ctx context.Context, shareID string) error {
	s.mu.Lock()
	sess, ok := s.sessions[shareID]
	job, publishing := s.publishing[shareID]
	s.mu.Unlock()
	if !ok {
		if publishing {
			// 尚在发布中：按取消处理
			job.cancel()
			return nil
		}
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

// runPublish 发布主流程：收集 → 规划（包内路径/白名单）→ 生成密钥 → 注册会话 → 首次在线即完成。
// 任何失败路径（含取消）推送 complete(success=false)；成功路径推送 complete + 会话留驻。
func (s *Service) runPublish(ctx context.Context, shareID string, workIDs, workSetIDs []int64,
	options SharePublishOptions, relay string) {
	fail := func(errMsg string) {
		if s.emitter != nil {
			s.emitter.PushComplete(ShareCompleteData{ShareID: shareID, Success: false, ErrMsg: errMsg})
		}
		s.removePublishing(shareID)
		s.removeSession(shareID)
	}
	if s.emitter != nil {
		s.emitter.PushProgress(shareID, "collecting")
	}

	model, err := s.collector.Collect(ctx, workIDs, workSetIDs)
	if err != nil {
		fail(cancelAwareMsg(ctx, err))
		return
	}
	workDir := s.workDir()
	if workDir == "" {
		fail(ErrShareWorkDirEmpty.Error())
		return
	}
	if _, err := s.planner.Plan(ctx, workDir, model); err != nil {
		fail(cancelAwareMsg(ctx, err))
		return
	}

	dialAddr, relayHost, err := normalizeRelayAddress(relay)
	if err != nil {
		fail(err.Error())
		return
	}

	// 发给收件人的 manifest JSON（含包内路径/大小/缺失标记；sha256 不预计算——见 README 边界）
	manifestData, err := model.Manifest.Serialize()
	if err != nil {
		fail(fmt.Sprintf("序列化 manifest 失败: %v", err))
		return
	}

	key, err := GenerateShareKey()
	if err != nil {
		fail(err.Error())
		return
	}

	passwordHash := ""
	if options.Password != "" {
		passwordHash = PasswordHashHex(options.Password)
	}
	expireSecs := mapExpireSeconds(options.ExpireSeconds)

	cfg := sessionConfig{
		id:           shareID,
		title:        SanitizeMetaText(defaultTitle(len(workIDs), len(workSetIDs), options.Title), 200),
		instanceID:   s.instanceID,
		relayDial:    dialAddr,
		relayHost:    relayHost,
		workDir:      workDir,
		key:          key,
		model:        model,
		manifestData: manifestData,
		passwordHash: passwordHash,
		expireSecs:   expireSecs,
		createdAt:    util.GetCurrentTimestamp(),
		emitter:      s.emitter,
		opts:         s.opts,
	}
	sess, err := newShareSession(cfg)
	if err != nil {
		fail(err.Error())
		return
	}
	s.mu.Lock()
	s.sessions[shareID] = sess
	s.mu.Unlock()

	if s.emitter != nil {
		s.emitter.PushProgress(shareID, "registering")
	}
	go sess.run(ctx)

	select {
	case firstErr := <-sess.firstCh:
		if firstErr != nil {
			fail(firstErr.Error())
			return
		}
	case <-ctx.Done():
		fail("已取消")
		return
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
	s.removePublishing(shareID)
	logger.Log.Infof("[share] 分享发布成功 shareId=%s token=%s relay=%s", shareID, sess.tokenOf(), relayHost)
}

// removePublishing 从进行中注册表删除（发布结束时调用）
func (s *Service) removePublishing(shareID string) {
	s.mu.Lock()
	delete(s.publishing, shareID)
	s.mu.Unlock()
}

// removeSession 移除会话（仅失败/取消路径：未在线的会话不留列表）
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
