package share

// 分享服务：把用户选中的作品/作品集发布为分享会话（复用 export 数据面收集 → 中继注册 →
// 出站隧道常驻），并提供取消/撤销/状态查询与分享记录（share_record）管理。
// 发布直跑（不经任务模块）：Publish 校验通过即以受监督 goroutine 驱动宿主主体
// （收集 → 规划 → 注册 → 隧道维持至会话终态），进度/完成/状态经 share-events 推送，
// 弹窗「取消」经 CancelPublish 直达主体取消；首次在线起生命周期落 share_record 行，
// 跨重启由启动自动复原（RestoreAll：active 行原 token bind 重绑，链接不变）复活。
// 收件拉取（Receive）仍走任务模块（share-receive 任务承载断点续传/暂停恢复语义）。

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/export"
	"github.com/library-squirrel/backend/task"
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
	// ErrSharePayloadInvalid share-receive 任务载荷非法（版本不支持/JSON 不解析）
	ErrSharePayloadInvalid = errors.New("分享任务载荷非法")
	// ErrShareRecordKeyBad 分享记录的 E2E 密钥不合法（非 32 字节 base64url）
	ErrShareRecordKeyBad = errors.New("分享记录密钥不合法")
)

// 内置任务类型（登记于 task.task_type，注册进 taskManager 执行面策略表）
const (
	// TaskTypeReceive 收件方：经中继拉取分享并回灌导入（断点续传/暂停恢复语义归任务建模）
	TaskTypeReceive = "share-receive"
)

// BuiltinTaskControl share-receive 任务创建与启动能力（task.Service 与 taskManager.Manager 经
// app.go 适配器组合装配；taskCtl 经延迟闭包取用，装配时序上 ShareService 先于 taskManager 创建）。
type BuiltinTaskControl interface {
	// CreateBuiltinTask 创建内置类型任务（返回任务 ID）
	CreateBuiltinTask(ctx context.Context, taskType string, taskName string, payload string) (int64, error)
	// StartTasks 启动任务树（传父任务 ID 即整树加载派发）
	StartTasks(ctx context.Context, taskIds []int64) error
	// CreateBuiltinTaskTree 事务原子创建内置任务树：1 父容器 + N 子任务（父 ID 回填子 pid）
	CreateBuiltinTaskTree(ctx context.Context, taskType string, parentName string, children []task.BuiltinTaskChild) (*entity.Task, error)
	// CreateBuiltinTaskParent 创建内置任务树父容器（has_child=true、pid=NULL），供先建父再建子的两段式建树
	CreateBuiltinTaskParent(ctx context.Context, taskType string, parentName string) (*entity.Task, error)
	// CreateBuiltinTaskChildren 在既有父任务下创建内置任务树子任务（pid=parentID、has_child=false）
	CreateBuiltinTaskChildren(ctx context.Context, taskType string, parentID int64, children []task.BuiltinTaskChild) error
	// DeleteTask 批量删除任务（含子任务）；建树失败回滚用
	DeleteTask(ctx context.Context, ids []int64) error
}

// hostParams 宿主主体入参（发布选项经此进入主体；复原时由分享记录行构建——
// 复原走 bind 不涉及密码与有效期重传）
type hostParams struct {
	WorkIDs       []int64
	WorkSetIDs    []int64
	Title         string
	ExpireSeconds int64  // -1=中继默认 / 0=无限期 / >0=自定义秒
	Password      string // 访问密码明文（仅本机使用，线上只走 sha256 摘要；空=无密码）
}

// 默认中继 TCP 端口（中继配置 listenAddr 默认 0.0.0.0:9527）
const defaultRelayPort = "9527"

// instanceIDPattern 中继对 instanceId 的形态约束（[A-Za-z0-9-]{8,128}）
var instanceIDPattern = regexp.MustCompile(`^[A-Za-z0-9-]{8,128}$`)

// lastShareIDTs shareID 时间戳水位（包级原子）：同毫秒并发发布顺延 +1，保 share_id 唯一
var lastShareIDTs int64

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
	// Password 访问密码；空=无密码（明文仅在本机使用，线上只走 sha256 摘要；落记录行的只有有无标记）
	Password string `json:"password"`
}

// Service 分享服务
type Service struct {
	repo       *Repository           // 分享记录仓储（nil=不落记录，单测场景）
	collector  ExportCollector       // 复用 export 收集能力
	planner    ExportPlanner         // 复用 export 规划能力
	relayAddr  func() string         // 中继地址（settings.shareSettings.relayAddress）
	workDir    func() string         // 工作目录（settings）
	instanceID string                // 设备绑定实例 ID（溯源锚点，持久化于程序根 config/）
	emitter    ShareEventEmitter     // nil=不发事件（单测场景）
	taskCtl    BuiltinTaskControl    // share-receive 任务创建/启动能力（app.go 装配）
	opts       sessionRuntimeOptions // 测试覆写（零值=默认）

	mu                  sync.Mutex
	sessions            map[string]*shareSession      // shareId → 会话（含终态，列表展示用）
	hostCancels         map[string]context.CancelFunc // shareId → 宿主主体 goroutine 取消（发布弹窗「取消」直达）
	pendingIncomingLink string                        // 深链到达缓存（冷启动衔接，前端启动后 ConsumeIncomingLink 取走）
	lastIncomingLink    string
	lastIncomingLinkAt  time.Time
}

// NewService 创建分享服务。
// repo 为分享记录仓储（nil=不落记录）；instanceID 为设备绑定实例 ID（LoadOrCreateInstanceID
// 产物，App 生命周期内恒定）；taskCtl 为 share-receive 任务控制能力（延迟闭包适配器，运行期取用）。
func NewService(repo *Repository, collector ExportCollector, planner ExportPlanner, relayAddr func() string,
	workDir func() string, instanceID string, emitter ShareEventEmitter, taskCtl BuiltinTaskControl) *Service {
	return &Service{
		repo:       repo,
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

// Publish 发布分享（直跑，不经任务模块）：前置校验通过即以受监督 goroutine 驱动宿主主体，
// 立即返回 shareID。进度/完成/会话状态经 share-events 推送（发布弹窗消费）；
// 「取消」经 CancelPublish；首次在线起生命周期落 share_record。
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
	shareID := nextShareID()
	s.startHostSupervised(shareID, hostParams{
		WorkIDs:       workIDs,
		WorkSetIDs:    workSetIDs,
		Title:         options.Title,
		ExpireSeconds: options.ExpireSeconds,
		Password:      options.Password,
	}, nil)
	return shareID, nil
}

// CancelPublish 取消分享发布（发布弹窗「取消」，不经任务控制面）：直接取消宿主主体——
// 会话本地终止；未到首次在线不产生记录行；已在线则会话移出注册表（中继侧存续至有效期
// 到期——在线撤销走 Revoke）。
func (s *Service) CancelPublish(ctx context.Context, shareID string) {
	s.mu.Lock()
	cancel := s.hostCancels[shareID]
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Revoke 撤销分享会话。在线：REVOKE 帧直达中继即时生效，终态事件驱动落记录行；
// 离线（会话不在注册表，如启动后未复原/复原重连中）：记录行直接落 revoked——本地终态，
// 中继侧会话存续至到期（撤销是隧道帧，离线无法送达，见 README 边界）。
func (s *Service) Revoke(ctx context.Context, shareID string) error {
	s.mu.Lock()
	sess, ok := s.sessions[shareID]
	s.mu.Unlock()
	if ok {
		sess.Revoke()
		return nil
	}
	if s.repo != nil {
		rec, err := s.repo.GetByShareID(ctx, shareID)
		if err != nil {
			return err
		}
		if rec != nil && rec.State == RecordStateActive {
			return s.repo.UpdateTerminal(ctx, rec.GetID(), RecordStateRevoked, "", util.GetCurrentTimestamp())
		}
	}
	return ErrShareNotFound
}

// DeleteRecord 删除分享记录（物理删行）：在驻会话先撤销（REVOKE 帧直达中继，链接即时失效），
// 主体尚在收集/注册中则先取消；随后删除记录行。离线 active 记录仅本地删行
// （中继侧会话存续至到期，与离线撤销同边界）。
func (s *Service) DeleteRecord(ctx context.Context, shareID string) error {
	if s.repo == nil {
		return ErrShareNotFound
	}
	s.mu.Lock()
	sess := s.sessions[shareID]
	cancel := s.hostCancels[shareID]
	s.mu.Unlock()
	if sess != nil {
		sess.Revoke()
	} else if cancel != nil {
		cancel()
	}
	rec, err := s.repo.GetByShareID(ctx, shareID)
	if err != nil {
		return err
	}
	if rec == nil {
		if sess != nil || cancel != nil {
			return nil // 发布/复原主体在驻但未达首次在线（无记录行）：主体已终止，视为删净
		}
		return ErrShareNotFound
	}
	return s.repo.DeleteUnscoped(ctx, rec.GetID())
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

// Records 全部分享记录（create_time 倒序，最新分享在前；视图按 share_id 关联会话运行态）
func (s *Service) Records(ctx context.Context) ([]*ShareRecordDTO, error) {
	if s.repo == nil {
		return []*ShareRecordDTO{}, nil
	}
	recs, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*ShareRecordDTO, 0, len(recs))
	for _, rec := range recs {
		out = append(out, toShareRecordDTO(rec))
	}
	return out, nil
}

// RestoreAll 启动自动复原：对 share_record 全部 active 行逐条独立 goroutine 复原
// （重新收集 → 规划 → 原 token bind 重绑；链接不变，剩余有效期由中继侧会话管理——
// bind 不重传 expiresAt）。复原失败按原因落终态：收集失败（作品已删）/中继 not_found →
// failed 记原因；中继不可达 → 会话重连循环（运行态不入表，下次启动重试）。
func (s *Service) RestoreAll(ctx context.Context) {
	if s.repo == nil {
		return
	}
	recs, err := s.repo.ListByState(ctx, RecordStateActive)
	if err != nil {
		logger.Log.Warnf("[share] 读取待复原分享记录失败: %v", err)
		return
	}
	for _, rec := range recs {
		workIDs, werr := unmarshalInt64s(rec.WorkIDs)
		workSetIDs, serr := unmarshalInt64s(rec.WorkSetIDs)
		if werr != nil || serr != nil {
			s.markRecordFailed(rec, "分享记录数据损坏（分享对象清单不可解析）")
			continue
		}
		logger.Log.Infof("[share] 复原分享 shareId=%s token=%s", rec.ShareID, rec.Token)
		s.startHostSupervised(rec.ShareID, hostParams{
			WorkIDs:       workIDs,
			WorkSetIDs:    workSetIDs,
			Title:         rec.Title,
			ExpireSeconds: rec.ExpireSeconds,
		}, rec)
	}
}

// startHostSupervised 启动受监督的宿主主体 goroutine（发布/复原共用）：panic 经 recover
// 收尾（发布：complete 失败事件终止弹窗等待；复原：记录落 failed），主体异常不外溢进程。
// 取消函数登记进 hostCancels（CancelPublish/DeleteRecord 直达），主体退出时自摘。
func (s *Service) startHostSupervised(shareID string, p hostParams, rec *entity.ShareRecord) {
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	if s.hostCancels == nil {
		s.hostCancels = make(map[string]context.CancelFunc)
	}
	s.hostCancels[shareID] = cancel
	s.mu.Unlock()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Log.Errorf("[share] 宿主主体 panic shareId=%s: %v\n%s", shareID, r, debug.Stack())
				if rec != nil {
					s.markRecordFailed(rec, "分享服务内部错误")
				} else if s.emitter != nil {
					s.emitter.PushComplete(ShareCompleteData{ShareID: shareID, Success: false, ErrMsg: "分享服务内部错误"})
				}
				s.removeSession(shareID)
			}
			s.mu.Lock()
			delete(s.hostCancels, shareID)
			s.mu.Unlock()
			cancel()
		}()
		s.hostSessionBody(ctx, shareID, p, rec)
	}()
}

// hostSessionBody 分享宿主主体（发布/复原共用，goroutine 内执行，阻塞直至会话终态或取消）：
// 收集 → 规划（包内路径/白名单）→ 建会话（发布：生成新密钥 register；复原：记录行原
// token/key bind 重绑）→ 首次在线 → 隧道维持至会话终态（终态同步落记录行）。
// 发布形态（rec=nil）：进度/完成经 share-events 推送；未达首次在线即终止不产生记录行。
// 复原形态（rec 非 nil）：记录行已存在保持 active；失败按原因落终态，中继不可达则会话
// 重连循环不落表（下次启动重试）。
func (s *Service) hostSessionBody(ctx context.Context, shareID string, p hostParams, rec *entity.ShareRecord) {
	fail := func(errMsg string) {
		if rec == nil {
			if s.emitter != nil {
				s.emitter.PushComplete(ShareCompleteData{ShareID: shareID, Success: false, ErrMsg: errMsg})
			}
		} else {
			// 复原主体失败（收集失败/密钥损坏等无会话终态场景）：记录行落 failed 记原因
			s.markRecordFailed(rec, errMsg)
		}
		s.removeSession(shareID)
	}
	interrupted := func() {
		// 发布：推「已取消」终态收尾弹窗（记录行不存在）；复原：静默终止（记录保持 active，下次启动重试）
		if rec == nil && s.emitter != nil {
			s.emitter.PushComplete(ShareCompleteData{ShareID: shareID, Success: false, ErrMsg: "已取消"})
		}
		s.removeSession(shareID)
	}
	phase := func(name string) {
		if s.emitter != nil {
			s.emitter.PushProgress(shareID, name)
		}
	}

	phase("collecting")
	model, err := s.collector.Collect(ctx, p.WorkIDs, p.WorkSetIDs)
	if err != nil {
		if ctx.Err() != nil {
			interrupted()
			return
		}
		fail(cancelAwareMsg(ctx, err))
		return
	}
	// 复原形态：分享对象必须全部仍可收集——对象删除后经活作品查询静默消失（Collect 不报错），
	// 此处比对记录清单与收集结果，缺失即落 failed（不复活内容残缺的会话）
	if rec != nil {
		if missing := missingShareTargets(model, p.WorkIDs, p.WorkSetIDs); len(missing) > 0 {
			fail(fmt.Sprintf("分享对象已删除: %s", strings.Join(missing, ", ")))
			return
		}
	}
	workDir := s.workDir()
	if workDir == "" {
		fail(ErrShareWorkDirEmpty.Error())
		return
	}
	if _, err := s.planner.Plan(ctx, workDir, model); err != nil {
		if ctx.Err() != nil {
			interrupted()
			return
		}
		fail(cancelAwareMsg(ctx, err))
		return
	}

	// 中继地址：发布取当前配置；复原取记录行（链接所指中继，不受当前配置切换影响）
	relayCfg := strings.TrimSpace(s.relayAddr())
	if rec != nil {
		relayCfg = rec.RelayAddress
	}
	dialAddr, relayHost, err := normalizeRelayAddress(relayCfg)
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

	var key []byte
	createdAt := util.GetCurrentTimestamp()
	if rec != nil {
		// 复原：密钥必须与原链接一致（收件人链接 fragment 不变）
		key, err = base64.RawURLEncoding.DecodeString(rec.KeyB64)
		if err != nil || len(key) != shareKeyLen {
			fail(fmt.Errorf("%w：长度不符", ErrShareRecordKeyBad).Error())
			return
		}
		createdAt = rec.GetCreateTime() // 列表展示的创建时间跨重启稳定
	} else {
		key, err = GenerateShareKey()
		if err != nil {
			fail(err.Error())
			return
		}
	}

	passwordHash := ""
	if p.Password != "" {
		passwordHash = PasswordHashHex(p.Password)
	}

	cfg := sessionConfig{
		id:           shareID,
		title:        SanitizeMetaText(defaultTitle(len(p.WorkIDs), len(p.WorkSetIDs), p.Title), 200),
		instanceID:   s.instanceID,
		relayDial:    dialAddr,
		relayHost:    relayHost,
		workDir:      workDir,
		key:          key,
		model:        model,
		manifestData: manifestData,
		passwordHash: passwordHash,
		expireSecs:   mapExpireSeconds(p.ExpireSeconds),
		createdAt:    createdAt,
		emitter:      s.emitter,
		opts:         s.opts,
	}
	if rec != nil {
		cfg.seedToken = rec.Token
		cfg.restore = true
	}
	sess, err := newShareSession(cfg)
	if err != nil {
		fail(err.Error())
		return
	}
	s.mu.Lock()
	s.sessions[shareID] = sess
	s.mu.Unlock()

	phase("registering")
	go sess.run(ctx)

	// 首次连接结果（发布：注册；复原：bind 重绑）
	var firstErr error
	select {
	case firstErr = <-sess.firstCh:
	case <-ctx.Done():
		interrupted()
		return
	case <-sess.doneCh:
		// 首次结果未发而主循环已退出（在线前撤销等防御路径）：优先取已缓冲的首次结果，
		// 无则按会话当前终态折算
		select {
		case firstErr = <-sess.firstCh:
		default:
			firstErr = fmt.Errorf("会话在首次在线前终止（%s）", sess.currentState())
		}
	}
	if firstErr != nil {
		if rec != nil {
			// 复原 bind 被拒：按会话终态映射记录（revoked/expired 保持原语义，其余落 failed）
			s.applyRecordTerminal(rec, sess.currentState(), firstErr.Error())
		} else if s.emitter != nil {
			s.emitter.PushComplete(ShareCompleteData{ShareID: shareID, Success: false, ErrMsg: firstErr.Error()})
		}
		s.removeSession(shareID)
		return
	}

	link := BuildShareLink(relayHost, sess.tokenOf(), key)
	sess.setLink(link)
	sess.emitSnapshot()
	if rec == nil {
		if s.emitter != nil {
			s.emitter.PushComplete(ShareCompleteData{
				ShareID: shareID,
				Success: true,
				Link:    link,
				Session: sess.snapshot(),
			})
		}
		// 首次在线落 active 记录行（发布形态的写入点）；后续终态更新以此行锚定
		rec = s.persistActiveRecord(shareID, p, relayHost, key, sess)
	} else {
		s.refreshRestoredRecord(rec, sess)
	}
	logger.Log.Infof("[share] 分享在线 shareId=%s token=%s relay=%s 复原=%v", shareID, sess.tokenOf(), relayHost, cfg.restore)

	// 隧道维持：等待会话终态或主体取消
	select {
	case <-sess.doneCh:
	case <-ctx.Done():
	}
	if ctx.Err() != nil && !sess.terminal() {
		// 进程内终止（取消发布/复原主体终止）：发布无记录行；复原记录保持 active（下次启动再复原）
		s.removeSession(shareID)
		return
	}
	if sess.terminal() {
		// 会话终态（撤销/过期/失败）→ 记录行终态
		s.applyRecordTerminal(rec, sess.currentState(), sess.snapshot().ErrMsg)
		return
	}
	// doneCh 已关但非终态且 ctx 未取消：会话主循环异常退出的防御路径
	s.removeSession(shareID)
}

// persistActiveRecord 首次在线落 active 记录行（发布形态的写入点）；失败返回 nil（终态更新跳过）
func (s *Service) persistActiveRecord(shareID string, p hostParams, relayHost string, key []byte, sess *shareSession) *entity.ShareRecord {
	if s.repo == nil {
		return nil
	}
	snap := sess.snapshot()
	rec := entity.NewShareRecord()
	rec.ShareID = shareID
	rec.Token = snap.Token
	rec.Title = snap.Title
	rec.WorkIDs = marshalInt64s(p.WorkIDs)
	rec.WorkSetIDs = marshalInt64s(p.WorkSetIDs)
	rec.RelayAddress = relayHost
	rec.KeyB64 = base64.RawURLEncoding.EncodeToString(key)
	rec.PasswordProtected = p.Password != ""
	rec.ExpireSeconds = p.ExpireSeconds
	rec.ExpiresAt = snap.ExpiresAt
	rec.FileCount = snap.FileCount
	rec.TotalBytes = snap.TotalBytes
	rec.MissingFiles = snap.MissingFiles
	rec.State = RecordStateActive
	if err := s.repo.Create(context.Background(), rec); err != nil {
		logger.Log.Errorf("[share] 落分享记录失败 shareId=%s: %v", shareID, err)
		return nil
	}
	return rec
}

// refreshRestoredRecord 复原在线后刷新记录行：到期时刻按中继 WELCOME 回填（bind 不重传
// expiresAt，剩余有效期以中继侧会话为准）、规划统计按重新收集值刷新
func (s *Service) refreshRestoredRecord(rec *entity.ShareRecord, sess *shareSession) {
	if s.repo == nil {
		return
	}
	snap := sess.snapshot()
	if err := s.repo.RefreshOnline(context.Background(), rec.GetID(), snap.ExpiresAt, snap.FileCount, snap.TotalBytes, snap.MissingFiles); err != nil {
		logger.Log.Warnf("[share] 刷新复原分享记录失败 shareId=%s: %v", rec.ShareID, err)
	}
}

// applyRecordTerminal 会话终态 → 记录行终态（revoked 记撤销时刻；failed 记原因；
// expired 无附加信息）。state 入参为会话状态（stateRevoked/stateExpired/stateFailed 等）。
func (s *Service) applyRecordTerminal(rec *entity.ShareRecord, state, errMsg string) {
	if s.repo == nil || rec == nil {
		return
	}
	recState := RecordStateFailed
	var revokedAt int64
	switch state {
	case stateRevoked:
		recState = RecordStateRevoked
		revokedAt = util.GetCurrentTimestamp()
		errMsg = ""
	case stateExpired:
		recState = RecordStateExpired
		errMsg = ""
	}
	if err := s.repo.UpdateTerminal(context.Background(), rec.GetID(), recState, errMsg, revokedAt); err != nil {
		logger.Log.Warnf("[share] 落分享记录终态失败 shareId=%s state=%s: %v", rec.ShareID, recState, err)
	}
}

// markRecordFailed 复原失败落 failed 终态并记原因（记录行损坏/主体 panic 等无会话终态场景）
func (s *Service) markRecordFailed(rec *entity.ShareRecord, errMsg string) {
	if s.repo == nil || rec == nil {
		return
	}
	if err := s.repo.UpdateTerminal(context.Background(), rec.GetID(), RecordStateFailed, errMsg, 0); err != nil {
		logger.Log.Warnf("[share] 落分享记录失败终态失败 shareId=%s: %v", rec.ShareID, err)
	}
}

// removeSession 移除会话（中止/失败路径：不再对外提供列表展示）
func (s *Service) removeSession(shareID string) {
	s.mu.Lock()
	delete(s.sessions, shareID)
	s.mu.Unlock()
}

// missingShareTargets 比对分享对象清单与收集结果，返回收集缺失的对象（「作品#175」形态）。
func missingShareTargets(model *export.ExportModel, workIDs []int64, workSetIDs []int64) []string {
	collectedWorks := make(map[int64]struct{}, len(model.Manifest.Works))
	for _, w := range model.Manifest.Works {
		collectedWorks[w.ID] = struct{}{}
	}
	collectedSets := make(map[int64]struct{}, len(model.Manifest.WorkSets))
	for _, ws := range model.Manifest.WorkSets {
		collectedSets[ws.ID] = struct{}{}
	}
	var missing []string
	for _, id := range workIDs {
		if id > 0 {
			if _, ok := collectedWorks[id]; !ok {
				missing = append(missing, fmt.Sprintf("作品#%d", id))
			}
		}
	}
	for _, id := range workSetIDs {
		if id > 0 {
			if _, ok := collectedSets[id]; !ok {
				missing = append(missing, fmt.Sprintf("作品集#%d", id))
			}
		}
	}
	return missing
}

// nextShareID 分配分享会话 ID（"share-{unixMilli}"；同毫秒顺延 +1 保唯一）
func nextShareID() string {
	for {
		old := atomic.LoadInt64(&lastShareIDTs)
		ts := util.GetCurrentTimestamp()
		if ts <= old {
			ts = old + 1
		}
		if atomic.CompareAndSwapInt64(&lastShareIDTs, old, ts) {
			return fmt.Sprintf("share-%d", ts)
		}
	}
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
