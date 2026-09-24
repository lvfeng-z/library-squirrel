package participation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	pluginsdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/plugin/settingresolver"
)

// 求值编排层失败分类（settingresolver.FailureKind 之外的宿主侧失败；串值进状态面，
// 供管理页降级标注）
const (
	FailureKindScriptLoad   = "script_load"   // resolver 装载失败：脚本缺失/不可读/契约版本不受支持
	FailureKindSettingsRead = "settings_read" // 全量设置读取失败（KV 读取/解密失败）
	FailureKindCanceled     = "canceled"      // 激活上下文取消，求值被放弃
	FailureKindInternal     = "internal"      // 其余宿主侧编排错误
)

// SettingsReader 插件全量设置读取面（宿主侧直读插件 KV，加密项已解密）——
// *plugin.PluginStorageService 结构性实现
type SettingsReader interface {
	GetAllValues(ctx context.Context, pluginID int64) (map[string]*pluginsdkdto.StorageValue, error)
}

// ChangeHandler 条目级参与度变更通知回调（下游派生面联动消费）。回调在求值 goroutine
// （落库触发路径）或激活相位 goroutine 上串行调用：须非阻塞、不得回调本层方法
type ChangeHandler func(change Change)

// Change 条目级参与度变更（覆盖表快照替换后的差异）：携带条目 point/id 与变更后方向
// （Active=false 停用方向、true 参与方向）及停用理由
type Change struct {
	PluginPublicId string
	Point          settingresolver.Point
	ID             string
	Active         bool
	Reason         string
}

// EntryState 单个声明条目的当前参与态（管理页「声明 × 状态」展示数据面）
type EntryState struct {
	Point  settingresolver.Point
	ID     string
	Active bool
	Reason string // 覆盖停用时 resolver 给出的理由（参与态或无覆盖为空）
}

// Status 插件参与度会话的求值状态快照：真相层内存可查的降级态数据面
type Status struct {
	HasResolver    bool      // 清单是否声明 settingsResolver
	LastEvalAt     time.Time // 最近一次求值完成时间（零值 = 从未求值）
	LastFailure    string    // 最近一次求值失败分类（空 = 最近一次成功或从未求值）
	LastFailureMsg string    // 最近一次失败的人读信息
	LastRejected   int       // 最近一次求值输出被单条拒收的条目数（含声明集外条目）
}

// Manager 插件条目参与度真相层：per 插件会话（声明集 + 覆盖表 + 求值状态），承担求值
// 编排与变更通知。会话随激活相位登记（StartSession）、随停用清理（StopSession）；
// 未激活插件无会话——设置落库触发为空操作（下次激活由持久 KV 重算重建）
type Manager struct {
	settings SettingsReader
	runner   *settingresolver.Runner
	rootPath string // 应用根目录（resolver 脚本路径 = rootPath/插件根/脚本名）

	// mu 保护 sessions 与 subscriptions，及各会话的覆盖表/状态/在途标记；
	// 求值与设置读取在锁外进行，结果应用时经会话现势检查后在锁内提交
	mu            sync.Mutex
	sessions      map[string]*session
	subscriptions []*changeSubscription
}

// NewManager 创建参与度真相层。settings 为插件全量设置读取面（加密项解密由其承担），
// rootPath 为应用根目录
func NewManager(settings SettingsReader, rootPath string) *Manager {
	return &Manager{
		settings: settings,
		runner:   settingresolver.NewRunner(),
		rootPath: rootPath,
		sessions: make(map[string]*session),
	}
}

// session 单插件参与度会话：激活相位登记、停用清理。resolver 为 nil = 清单未声明
// resolver（不求值、无覆盖，查询面退化为纯声明集）
type session struct {
	publicId string
	pluginID int64
	resolver *resolverSpec
	declared map[entryKey]struct{}
	defaults map[string]string

	// 以下字段由 Manager.mu 保护
	overrides   map[entryKey]settingresolver.Entry
	status      Status
	evalRunning bool // 一轮求值在途（同步首评或后台循环占用）
	evalPending bool // 在途期间有触发合并进来，待跑（合并标记——中间触发跳过，只跑最新输入）
}

// resolverSpec 清单声明的 resolver 装载产物。loadFailure 非 nil 时（脚本缺失/不可读/
// 契约版本不受支持）每轮求值直接落败
type resolverSpec struct {
	scriptName  string
	script      string
	loadFailure *evalFailure
}

// evalFailure 一次求值失败的分类记录（供状态面与日志）
type evalFailure struct {
	Kind    string
	Message string
}

// changeSubscription 变更通知订阅（active=false 已退订，派发时跳过）
type changeSubscription struct {
	handler ChangeHandler
	active  bool
}

// StartSession 登记插件参与度会话并同步完成首次求值（生命周期参与者激活相位末尾调用，
// 晚于基线注册与进程 Activate RPC 的 KV 自迁移）：声明集取自清单——不经 loader 进程表
// 读取，纯 UI 插件（无 entryFile、无进程）同经此路径。首次求值同步执行，激活完成时
// 覆盖表已就位；求值失败不构成激活失败（保留语义：无覆盖 = 全基线，降级态入状态面）。
// 重复登记同 publicId 时旧会话被替换，旧会话的在途求值结果经现势检查丢弃
func (m *Manager) StartSession(ctx context.Context, plugin *entity.Plugin, manifest *dto.PluginManifest) {
	if plugin == nil || manifest == nil || !plugin.PublicID.Valid || plugin.PublicID.String == "" {
		return
	}
	s := m.buildSession(plugin, manifest)

	m.mu.Lock()
	m.sessions[s.publicId] = s
	s.evalRunning = true // 同步首评占用在途位：窗口内的落库触发只置合并标记
	m.mu.Unlock()

	m.evaluateOnce(ctx, s)

	m.mu.Lock()
	stillCurrent := m.sessions[s.publicId] == s
	if !stillCurrent || !s.evalPending {
		s.evalRunning = false
		s.evalPending = false
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()
	go m.evalLoop(s) // 首评期间有落库触发合并进来：转交后台循环消化（循环出口自清在途位）
}

// StopSession 清空该插件参与度会话（生命周期参与者 OnStopped 调用；幂等——会话未必
// 在场）。在途求值的结果经现势检查丢弃，不复活已清空的覆盖表
func (m *Manager) StopSession(pluginPublicId string) {
	m.mu.Lock()
	delete(m.sessions, pluginPublicId)
	m.mu.Unlock()
}

// TriggerEvaluation 设置落库后的异步求值触发（保存/重置两落库点分别调用）：立即返回，
// 求值经独立 goroutine 串行进行；在途时只置合并标记（在途完成后以最新输入重跑，中间
// 触发合并跳过）。无会话（插件未激活）或清单未声明 resolver 时空操作
func (m *Manager) TriggerEvaluation(pluginPublicId string) {
	m.mu.Lock()
	s, ok := m.sessions[pluginPublicId]
	if !ok || s.resolver == nil {
		m.mu.Unlock()
		return
	}
	if s.evalRunning {
		s.evalPending = true
		m.mu.Unlock()
		return
	}
	s.evalRunning = true
	s.evalPending = true // 本轮触发即待跑工作；evalLoop 消化后按合并标记续跑
	m.mu.Unlock()
	go m.evalLoop(s)
}

// EntryActive 查询插件声明条目的当前参与态（声明集 ⊕ 覆盖表：无覆盖 = 基线参与）。
// found=false：插件无会话（未激活/已停用）或条目未在清单声明
func (m *Manager) EntryActive(pluginPublicId string, point settingresolver.Point, id string) (active, found bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.sessions[pluginPublicId]
	if s == nil {
		return false, false
	}
	key := entryKey{Point: point, ID: id}
	if _, ok := s.declared[key]; !ok {
		return false, false
	}
	return entryEffective(key, s.overrides), true
}

// Entries 列出插件全部声明条目的当前参与态（point、id 字典序）。nil = 无会话
func (m *Manager) Entries(pluginPublicId string) []EntryState {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.sessions[pluginPublicId]
	if s == nil {
		return nil
	}
	states := make([]EntryState, 0, len(s.declared))
	for key := range s.declared {
		state := EntryState{Point: key.Point, ID: key.ID, Active: entryEffective(key, s.overrides)}
		if !state.Active {
			state.Reason = s.overrides[key].Reason
		}
		states = append(states, state)
	}
	sort.Slice(states, func(i, j int) bool {
		if states[i].Point != states[j].Point {
			return states[i].Point < states[j].Point
		}
		return states[i].ID < states[j].ID
	})
	return states
}

// StatusOf 查询插件求值状态快照（降级态数据面）。nil = 无会话
func (m *Manager) StatusOf(pluginPublicId string) *Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.sessions[pluginPublicId]
	if s == nil {
		return nil
	}
	status := s.status
	return &status
}

// SubscribeChanges 订阅条目级参与度变更通知，返回退订函数。通知携带条目 point/id 与
// 变更后方向；停用清表（StopSession）不发通知——停用路径的痕迹清理由各派生面参与者
// 自行承担
func (m *Manager) SubscribeChanges(handler ChangeHandler) (unsubscribe func()) {
	sub := &changeSubscription{handler: handler, active: true}
	m.mu.Lock()
	m.subscriptions = append(m.subscriptions, sub)
	m.mu.Unlock()
	return func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		sub.active = false
	}
}

// evalLoop 会话后台求值循环（落库触发路径）：消化合并标记，每轮以最新输入求值
// （求值开始时现读 KV）；无待跑或会话已消解（停用清表/重激活换代）时退出并清在途
// 标记（含待跑——消解路径的合并标记无消费方，悬置会阻碍会话静默观察）
func (m *Manager) evalLoop(s *session) {
	for {
		m.mu.Lock()
		if m.sessions[s.publicId] != s || !s.evalPending {
			s.evalRunning = false
			s.evalPending = false
			m.mu.Unlock()
			return
		}
		s.evalPending = false
		m.mu.Unlock()

		m.evaluateOnce(context.Background(), s)
	}
}

// evaluateOnce 执行一轮求值编排：读全量设置（声明默认值 ← KV 存储值）→ resolver 运行器
// 求值（含输出 shape 校验）→ 结果/失败应用。设置读取与脚本求值在锁外进行
func (m *Manager) evaluateOnce(ctx context.Context, s *session) {
	if s.resolver == nil {
		return
	}
	if s.resolver.loadFailure != nil {
		m.applyFailure(s, *s.resolver.loadFailure)
		return
	}
	settings, err := m.readEvalSettings(ctx, s)
	if err != nil {
		m.applyFailure(s, evalFailure{Kind: FailureKindSettingsRead, Message: err.Error()})
		return
	}
	result, err := m.runner.Evaluate(ctx, s.resolver.script, settingresolver.Input{Settings: settings})
	if err != nil {
		m.applyFailure(s, classifyFailure(err))
		return
	}
	m.applyResult(s, result)
}

// readEvalSettings 构造求值输入设置：声明键全集，KV 存储值（已解密）覆盖声明默认值，
// KV 缺键回落默认值——与设置页读取同构。KV 中不在声明集内的残留键不进输入
func (m *Manager) readEvalSettings(ctx context.Context, s *session) (map[string]interface{}, error) {
	stored, err := m.settings.GetAllValues(ctx, s.pluginID)
	if err != nil {
		return nil, err
	}
	settings := make(map[string]interface{}, len(s.defaults))
	for key, def := range s.defaults {
		if v, ok := stored[key]; ok && v != nil {
			settings[key] = v.Value
		} else {
			settings[key] = def
		}
	}
	return settings, nil
}

// applyResult 应用一次成功求值：声明集外条目单条拒收（记日志）后按快照整体替换覆盖表
// （每次求值输出 = 当前完整意愿），对有效参与态发生变化的声明条目在锁外发变更通知。
// 会话已消解（停用/重激活换代）时丢弃结果
func (m *Manager) applyResult(s *session, result *settingresolver.Result) {
	undeclared := 0
	newOverrides := make(map[entryKey]settingresolver.Entry, len(result.Entries))
	for _, e := range result.Entries {
		key := entryKey{Point: e.Point, ID: e.ID}
		if _, ok := s.declared[key]; !ok {
			undeclared++
			logger.Log.Warnf("插件 %s resolver 输出条目 %s/%s 不在清单声明集内，单条拒收", s.publicId, e.Point, e.ID)
			continue
		}
		newOverrides[key] = e
	}
	for _, r := range result.Rejected {
		logger.Log.Warnf("插件 %s resolver 输出第 %d 条被拒收: %s", s.publicId, r.Index, r.Reason)
	}

	m.mu.Lock()
	if m.sessions[s.publicId] != s {
		m.mu.Unlock()
		return
	}
	changes := diffEffective(s.declared, s.overrides, newOverrides, s.publicId)
	s.overrides = newOverrides
	s.status = Status{
		HasResolver:  true,
		LastEvalAt:   time.Now(),
		LastRejected: len(result.Rejected) + undeclared,
	}
	subs := m.snapshotSubscriptionsLocked()
	m.mu.Unlock()

	emitChanges(subs, changes)
}

// applyFailure 应用一次求值失败：保留上一次覆盖表（首次失败 = 无覆盖 = 全基线），失败
// 分类与时间入状态面。会话已消解时丢弃
func (m *Manager) applyFailure(s *session, f evalFailure) {
	logger.Log.Warnf("插件 %s resolver 求值失败（%s），保留上一次覆盖表: %s", s.publicId, f.Kind, f.Message)
	m.mu.Lock()
	if m.sessions[s.publicId] != s {
		m.mu.Unlock()
		return
	}
	s.status.LastEvalAt = time.Now()
	s.status.LastFailure = f.Kind
	s.status.LastFailureMsg = f.Message
	m.mu.Unlock()
}

// snapshotSubscriptionsLocked 拍订阅快照（调用方持 Manager.mu），供锁外派发
func (m *Manager) snapshotSubscriptionsLocked() []*changeSubscription {
	subs := make([]*changeSubscription, len(m.subscriptions))
	copy(subs, m.subscriptions)
	return subs
}

// emitChanges 锁外串行派发变更通知；单个回调 panic 不波及其他回调与宿主
func emitChanges(subs []*changeSubscription, changes []Change) {
	for _, change := range changes {
		for _, sub := range subs {
			if !sub.active {
				continue
			}
			invokeChangeHandler(sub.handler, change)
		}
	}
}

func invokeChangeHandler(handler ChangeHandler, change Change) {
	defer func() {
		if r := recover(); r != nil {
			logger.Log.Errorf("参与度变更通知回调 panic（插件 %s 条目 %s/%s）: %v",
				change.PluginPublicId, change.Point, change.ID, r)
		}
	}()
	handler(change)
}

// diffEffective 计算声明条目的有效参与态差异（旧覆盖表 → 新覆盖表）：基线参与（无覆盖
// 或覆盖 active=true）与停用（覆盖 active=false）间的翻转各成一条变更
func diffEffective(declared map[entryKey]struct{}, oldOverrides, newOverrides map[entryKey]settingresolver.Entry, publicId string) []Change {
	var changes []Change
	for key := range declared {
		before := entryEffective(key, oldOverrides)
		after := entryEffective(key, newOverrides)
		if before == after {
			continue
		}
		change := Change{PluginPublicId: publicId, Point: key.Point, ID: key.ID, Active: after}
		if !after {
			change.Reason = newOverrides[key].Reason
		}
		changes = append(changes, change)
	}
	return changes
}

// entryEffective 条目有效参与态：无覆盖 = 基线参与
func entryEffective(key entryKey, overrides map[entryKey]settingresolver.Entry) bool {
	if e, ok := overrides[key]; ok {
		return e.Active
	}
	return true
}

// classifyFailure 将求值错误归入失败分类：settingresolver 失败取其分类；上下文取消
// 归 canceled；其余归 internal
func classifyFailure(err error) evalFailure {
	var evalErr *settingresolver.EvalError
	if errors.As(err, &evalErr) {
		return evalFailure{Kind: string(evalErr.Kind), Message: evalErr.Message}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return evalFailure{Kind: FailureKindCanceled, Message: err.Error()}
	}
	return evalFailure{Kind: FailureKindInternal, Message: err.Error()}
}

// buildSession 从插件实体与清单构建会话：声明集、设置默认值与 resolver 装载（脚本全文
// 此时读入——包内只读工件；装载失败预分类，每轮求值直接落败）
func (m *Manager) buildSession(plugin *entity.Plugin, manifest *dto.PluginManifest) *session {
	s := &session{
		publicId: plugin.PublicID.String,
		pluginID: plugin.GetID(),
		declared: declaredEntries(manifest),
		defaults: settingDefaults(manifest),
		status:   Status{HasResolver: manifest.SettingsResolver != nil},
	}
	if decl := manifest.SettingsResolver; decl != nil {
		spec := &resolverSpec{scriptName: decl.Script}
		s.resolver = spec
		switch {
		case decl.ContractVersion != resolverContractVersion:
			spec.loadFailure = &evalFailure{
				Kind:    FailureKindScriptLoad,
				Message: fmt.Sprintf("contractVersion %d 不受支持（唯一受支持值 %d）", decl.ContractVersion, resolverContractVersion),
			}
		default:
			scriptPath := filepath.Join(m.rootPath, plugin.RootPath.String, decl.Script)
			content, err := os.ReadFile(scriptPath)
			if err != nil {
				spec.loadFailure = &evalFailure{
					Kind:    FailureKindScriptLoad,
					Message: fmt.Sprintf("读取脚本文件失败 %s: %v", scriptPath, err),
				}
			} else {
				spec.script = string(content)
			}
		}
	}
	return s
}
