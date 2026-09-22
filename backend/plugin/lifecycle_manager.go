package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/library-squirrel/backend/base/logger"
	dto "github.com/library-squirrel/backend/base/model/dto"
	entity2 "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/util"
)

// ErrPluginLifecycleBusy 生命周期操作与该插件进行中的瞬态（激活中/停用中）交错，本次操作被拒绝
var ErrPluginLifecycleBusy = errors.New("plugin lifecycle busy")

// pluginLifecycleState 生命周期状态取值（「未激活」不占状态表项，查不到即未激活）
type pluginLifecycleState int

const (
	// lifecycleActivating 激活中（瞬态：参与者激活相位执行期间）
	lifecycleActivating pluginLifecycleState = iota + 1
	// lifecycleActive 运行中（激活全相位成功的稳定态）
	lifecycleActive
	// lifecycleStopping 停用中（瞬态：否决检查与清理执行期间）
	lifecycleStopping
)

// lifecycleManager 插件生命周期状态机：以 publicId 为键跟踪激活/停用，按参与者相位编排
// 痕迹注册与清理。状态表项随激活产生、随停用/崩溃消解，不随插件安装记录累积陈旧项。
// 互斥锁在参与者相位回调执行期间保持持有——生命周期操作全程串行，参与者实现禁止回调
// 生命周期方法（重入死锁，约束见 LifecycleParticipant 接口注释）
type lifecycleManager struct {
	mu sync.Mutex

	participants []LifecycleParticipant

	// states publicId -> 生命周期状态（查不到即未激活）
	states map[string]pluginLifecycleState
	// lastActivateErr publicId -> 最近一次激活失败原因（下次激活成功时清除）
	lastActivateErr map[string]error

	// readManifest 激活时读取插件 manifest（默认从插件安装目录读 plugin.json；测试注入替身）
	readManifest func(plugin *entity2.Plugin) (*dto.PluginManifest, error)
}

// 生命周期状态对外字符串取值（状态面板 DTO 序列化值）
const (
	lifecycleStateInactive   = "inactive"
	lifecycleStateActivating = "activating"
	lifecycleStateActive     = "active"
	lifecycleStateStopping   = "stopping"
)

// lifecycleStatusOf 查询插件生命周期状态与最近一次激活失败原因的只读快照（状态面板取值）。
// 未激活无状态表项返回 inactive；lastActivateError 在最近一次激活失败后非空、重试成功时
// 由激活路径清除——激活中瞬态读到的是上一次失败的原因，属如实快照
func (m *lifecycleManager) lifecycleStatusOf(publicId string) (state string, lastActivateError error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state = lifecycleStateInactive
	switch m.states[publicId] {
	case lifecycleActivating:
		state = lifecycleStateActivating
	case lifecycleActive:
		state = lifecycleStateActive
	case lifecycleStopping:
		state = lifecycleStateStopping
	}
	return state, m.lastActivateErr[publicId]
}

// newLifecycleManager 创建生命周期状态机
func newLifecycleManager() *lifecycleManager {
	return &lifecycleManager{
		states:          make(map[string]pluginLifecycleState),
		lastActivateErr: make(map[string]error),
		readManifest:    readPluginManifest,
	}
}

// registerParticipant 注册生命周期参与者（注册顺序即激活相位顺序，停用按逆序清理）
func (m *lifecycleManager) registerParticipant(p LifecycleParticipant) {
	m.participants = append(m.participants, p)
}

// readPluginManifest 从插件安装目录读取并解析 plugin.json
func readPluginManifest(plugin *entity2.Plugin) (*dto.PluginManifest, error) {
	manifestBytes, err := readPluginManifestBytes(plugin)
	if err != nil {
		return nil, err
	}
	return parsePluginManifest(plugin.PublicID.String, manifestBytes)
}

// readPluginManifestBytes 读取插件安装目录下的 plugin.json 原文（声明面校验就原文探测顶层键，
// 故需要原文而非仅解析结果；激活与状态展示等多路径共用，不落激活语境日志）
func readPluginManifestBytes(plugin *entity2.Plugin) ([]byte, error) {
	publicId := plugin.PublicID.String
	pluginRootDir := filepath.Join(util.RootPath(), plugin.RootPath.String)
	manifestBytes, err := os.ReadFile(filepath.Join(pluginRootDir, "plugin.json"))
	if err != nil {
		return nil, fmt.Errorf("读取 plugin.json 失败 %s: %w", publicId, err)
	}
	return manifestBytes, nil
}

// parsePluginManifest 解析 plugin.json 原文
func parsePluginManifest(publicId string, manifestBytes []byte) (*dto.PluginManifest, error) {
	var manifest dto.PluginManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, fmt.Errorf("解析 plugin.json 失败 %s: %w", publicId, err)
	}
	return &manifest, nil
}

// activate 激活插件：状态守卫（已运行幂等跳过、瞬态交错拒绝）→ 读 manifest → 参与者按
// 注册顺序正向执行 Activate。任一相位失败即全体参与者逆序 OnStopped 统一回滚（幂等清理），
// 状态回未激活并记录失败原因；下次激活成功时清除失败记录
func (m *lifecycleManager) activate(ctx context.Context, plugin *entity2.Plugin) error {
	publicId := plugin.PublicID.String

	m.mu.Lock()
	defer m.mu.Unlock()

	switch m.states[publicId] {
	case lifecycleActive:
		return nil
	case lifecycleActivating, lifecycleStopping:
		return fmt.Errorf("%w: %s", ErrPluginLifecycleBusy, publicId)
	}
	m.states[publicId] = lifecycleActivating

	logger.Log.Infof("正在激活插件: %s (root=%s)", publicId, filepath.Join(util.RootPath(), plugin.RootPath.String))
	manifest, err := m.readManifest(plugin)
	if err != nil {
		delete(m.states, publicId)
		m.lastActivateErr[publicId] = err
		return err
	}

	// manifest 无扩展点（安装校验后插件目录内容漂移）：无痕迹可注册，不进相位，视为激活成功
	if manifest.Extensions == nil {
		logger.Log.Warnf("插件 %s 无扩展点，跳过", publicId)
		delete(m.lastActivateErr, publicId)
		m.states[publicId] = lifecycleActive
		return nil
	}

	for _, p := range m.participants {
		if err := p.Activate(ctx, plugin, manifest); err != nil {
			for i := len(m.participants) - 1; i >= 0; i-- {
				m.participants[i].OnStopped(ctx, publicId)
			}
			delete(m.states, publicId)
			m.lastActivateErr[publicId] = err
			return fmt.Errorf("激活参与者相位失败 %s: %w", publicId, err)
		}
	}

	delete(m.lastActivateErr, publicId)
	m.states[publicId] = lifecycleActive
	return nil
}

// deactivate 停用插件：全体参与者 PrepareStop 否决检查（force=true 跳过）→ 按注册逆序
// OnStopped 清理（后注册的进程域先停，先清痕迹）。未激活时无痕迹可清、幂等返回；
// 否决时状态回运行中（插件保持运行）
func (m *lifecycleManager) deactivate(ctx context.Context, publicId string, op PluginStopOp, force bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch m.states[publicId] {
	case lifecycleActivating, lifecycleStopping:
		return fmt.Errorf("%w: %s", ErrPluginLifecycleBusy, publicId)
	case lifecycleActive:
		// 运行中：进入停用相位
	default:
		// 未激活：无运行痕迹，停用幂等
		return nil
	}

	m.states[publicId] = lifecycleStopping
	if !force {
		for _, p := range m.participants {
			if err := p.PrepareStop(ctx, publicId, op, force); err != nil {
				m.states[publicId] = lifecycleActive
				return fmt.Errorf("停用被参与者否决: %w", err)
			}
		}
	}
	for i := len(m.participants) - 1; i >= 0; i-- {
		m.participants[i].OnStopped(ctx, publicId)
	}
	delete(m.states, publicId)
	return nil
}

// notifyCrashed 插件进程崩溃收敛：状态为运行中时按注册逆序执行参与者清理并回未激活。
// 幂等——非运行中状态的崩溃通知不重复清理；不执行 PrepareStop（崩溃无法否决），
// 进程表条目由 loader 在触发前已摘除
func (m *lifecycleManager) notifyCrashed(ctx context.Context, publicId string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.states[publicId] != lifecycleActive {
		return
	}
	logger.Log.Warnf("插件进程崩溃，执行参与者痕迹清理: %s", publicId)
	for i := len(m.participants) - 1; i >= 0; i-- {
		m.participants[i].OnStopped(ctx, publicId)
	}
	delete(m.states, publicId)
}
