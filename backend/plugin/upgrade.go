package plugin

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"

	"github.com/library-squirrel/backend/base/logger"
	domain "github.com/library-squirrel/backend/base/model/dto"
	entity2 "github.com/library-squirrel/backend/base/model/entity"
)

// 检查更新流的待办类型（PendingUpgradeDTO.Kind 取值）
const (
	// PendingKindAvailable 可升级：等用户在管理页答复（升级/跳过），计入前端红点
	PendingKindAvailable = "available"
	// PendingKindForced 已因契约不兼容在启动期强制升级：只读告知，供管理页展示
	PendingKindForced = "forced"
	// PendingKindError 捆绑包安装失败（含契约不兼容装不进）：只读告知，供管理页展示
	PendingKindError = "error"
)

// 检查更新流错误定义
var (
	// ErrPendingUpgradeNotFound 待办项不存在（已被消费/移除或从未记录）
	ErrPendingUpgradeNotFound = errors.New("pending upgrade not found")
	// ErrPendingUpgradeExecuting 待办项正在执行换版（执行中守卫）
	ErrPendingUpgradeExecuting = errors.New("pending upgrade is executing")
	// ErrDeclineRequiresBuildID 跳过操作要求目标包带 buildId（未打标包不进检查更新流，无从跳过）
	ErrDeclineRequiresBuildID = errors.New("decline requires a tagged build")
)

// pendingUpgradeEntry 启动期检测出的插件更新待办（内存态，重启重检；落库的只有拒绝标记）。
// 列表键：身份可解析时为 publicId，error 项（包解析失败无身份）为 "pkg:"+包路径
type pendingUpgradeEntry struct {
	PublicID         string
	PluginName       string
	InstalledVersion string
	TargetVersion    string
	TargetBuildID    string // 捆绑包 buildId；未打标包为空（不进检查更新流则不会出现）
	Direction        string // up/down/none
	Kind             string // PendingKindAvailable/PendingKindForced/PendingKindError
	Source           string // 更新来源，当前仅 SourceBundled
	PackagePath      string // 捆绑 zip 绝对路径（服务端权威持有，Apply 时自用，不作前端操作参数）
	Message          string // forced/error 类的说明文案
	Executing        bool   // 执行中守卫：Apply 运行期置位，拦截重复触发与 Decline/Restore 竞态
}

// toDTO 转换为 IPC DTO
func (e *pendingUpgradeEntry) toDTO() *domain.PendingUpgradeDTO {
	return &domain.PendingUpgradeDTO{
		PublicID:         e.PublicID,
		PluginName:       e.PluginName,
		InstalledVersion: e.InstalledVersion,
		TargetVersion:    e.TargetVersion,
		TargetBuildID:    e.TargetBuildID,
		Direction:        e.Direction,
		Kind:             e.Kind,
		Source:           e.Source,
		Message:          e.Message,
	}
}

// recordPending 记录待办项（同键覆盖——同插件重复检测取最新结果）
func (s *Service) recordPending(entry *pendingUpgradeEntry) {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	key := entry.PublicID
	if key == "" {
		key = "pkg:" + entry.PackagePath
	}
	s.pendingUpgrades[key] = entry
}

// removePending 移除待办项（卸载插件时清理可操作项）
func (s *Service) removePending(publicId string) {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	delete(s.pendingUpgrades, publicId)
}

// claimPending 锁内取待办项并置执行中守卫；不存在或执行中均返回错误（守卫以后端为权威，
// 拦截双击重复触发与 Decline/Restore 并发竞态）
func (s *Service) claimPending(publicId string) (*pendingUpgradeEntry, error) {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	entry, ok := s.pendingUpgrades[publicId]
	if !ok {
		return nil, ErrPendingUpgradeNotFound
	}
	if entry.Executing {
		return nil, ErrPendingUpgradeExecuting
	}
	entry.Executing = true
	return entry, nil
}

// finishPending 结束执行：成功移除待办项，失败复位守卫供重试
func (s *Service) finishPending(publicId string, success bool) {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	if success {
		delete(s.pendingUpgrades, publicId)
	} else if entry, ok := s.pendingUpgrades[publicId]; ok {
		entry.Executing = false
	}
}

// GetPendingUpgrades 返回检查更新待办项（available/forced/error 混合；available 数即前端红点计数）
func (s *Service) GetPendingUpgrades(ctx context.Context) []*domain.PendingUpgradeDTO {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	result := make([]*domain.PendingUpgradeDTO, 0, len(s.pendingUpgrades))
	for _, entry := range s.pendingUpgrades {
		result = append(result, entry.toDTO())
	}
	return result
}

// ApplyPendingUpgrade 答复「升级」：对 available 待办执行运行期换版（当次会话生效）。
// 走 ReinstallFromPath 内部链（参与者否决 → 停进程 → 删文件 → 重装并激活），zip 路径由服务端
// 从待办项自持（检测期记录），不经前端回传；运行中任务由参与者否决（Paused 不拦）
func (s *Service) ApplyPendingUpgrade(ctx context.Context, publicId string) (*entity2.Plugin, error) {
	entry, err := s.claimPending(publicId)
	if err != nil {
		return nil, err
	}
	if entry.Kind != PendingKindAvailable {
		s.finishPending(publicId, false)
		return nil, errors.New("only available pending upgrade can be applied")
	}

	plugin, err := s.ReinstallFromPath(ctx, publicId, entry.PackagePath, true)
	s.finishPending(publicId, err == nil)
	if err != nil {
		return nil, err
	}
	logger.Log.Infof("插件更新待办已执行换版: %s -> v%s (buildId=%s)", publicId, entry.TargetVersion, entry.TargetBuildID)
	return plugin, nil
}

// DeclinePendingUpgrade 答复「跳过此构建」：把目标 buildId 写入拒绝标记并移除待办。
// 下次启动检测对等值 buildId 静默跳过；新 buildId 到来自动失效；重装全字段覆盖自然清零
func (s *Service) DeclinePendingUpgrade(ctx context.Context, publicId string) (*entity2.Plugin, error) {
	entry, err := s.claimPending(publicId)
	if err != nil {
		return nil, err
	}
	if entry.Kind != PendingKindAvailable {
		s.finishPending(publicId, false)
		return nil, errors.New("only available pending upgrade can be declined")
	}
	if entry.TargetBuildID == "" {
		s.finishPending(publicId, false)
		return nil, ErrDeclineRequiresBuildID
	}

	plugin, err := s.repo.GetByPublicId(ctx, publicId)
	if err != nil {
		s.finishPending(publicId, false)
		return nil, err
	}
	if plugin == nil {
		s.finishPending(publicId, false)
		return nil, ErrPluginNotFound
	}

	plugin.UpgradeDeclinedBuildID = sql.NullString{String: entry.TargetBuildID, Valid: true}
	if err := s.repo.Updates(ctx, plugin); err != nil {
		s.finishPending(publicId, false)
		return nil, err
	}
	s.finishPending(publicId, true)
	logger.Log.Infof("插件更新待办已跳过此构建(buildId=%s): %s", entry.TargetBuildID, publicId)
	return plugin, nil
}

// RestorePendingUpgrade 「重新提示」反悔入口：清除拒绝标记，并立即重跑该插件的捆绑包检测，
// 判变成立则重建 available 待办（红点复亮），不必等下次启动。检测读包无副作用；
// 已装契约不兼容时不在此强制换版（运行期换版必须走 Apply 的参与者否决链），仅记 available 待办
func (s *Service) RestorePendingUpgrade(ctx context.Context, publicId string) (*entity2.Plugin, error) {
	plugin, err := s.repo.GetByPublicId(ctx, publicId)
	if err != nil {
		return nil, err
	}
	if plugin == nil {
		return nil, ErrPluginNotFound
	}

	// 清拒绝标记须用 Save 全字段替换——Updates 跳过零值，NullString{Valid:false} 写不进 NULL
	plugin.UpgradeDeclinedBuildID = sql.NullString{}
	if err := s.repo.Save(ctx, plugin); err != nil {
		return nil, err
	}

	// 立即重跑检测：捆绑 zip 路径取自 SourceDetail（installCore 落库的安装包路径）
	if plugin.Source.Valid && plugin.Source.String == SourceBundled && plugin.SourceDetail.Valid {
		s.redetectBundled(ctx, plugin, plugin.SourceDetail.String)
	}
	logger.Log.Infof("插件更新拒绝标记已清除: %s", publicId)
	return plugin, nil
}

// redetectBundled 对单个插件重跑捆绑包判变检测：判变成立记 available 待办（跳过强制/拒绝短路——
// 前者不适用运行期，后者刚被清除）
func (s *Service) redetectBundled(ctx context.Context, existing *entity2.Plugin, packagePath string) {
	installDTO, err := s.loadPluginPackage(packagePath)
	if err != nil {
		logger.Log.Warnf("重新检测捆绑包失败: %s, %v", packagePath, err)
		return
	}
	if installDTO.PublicID != existing.PublicID.String {
		logger.Log.Warnf("重新检测捆绑包身份不匹配: %s != %s", installDTO.PublicID, existing.PublicID.String)
		return
	}
	if !needBundledUpgrade(existing, installDTO) {
		return
	}
	name := installDTO.Name
	if existing.Name.Valid {
		name = existing.Name.String
	}
	s.recordPending(&pendingUpgradeEntry{
		PublicID:         installDTO.PublicID,
		PluginName:       name,
		InstalledVersion: versionString(existing.Version),
		TargetVersion:    installDTO.Version,
		TargetBuildID:    installDTO.BuildID,
		Direction:        upgradeDirection(versionString(existing.Version), installDTO.Version),
		Kind:             PendingKindAvailable,
		Source:           SourceBundled,
		PackagePath:      packagePath,
	})
	logger.Log.Infof("重新检测到可用更新，待办已重建: %s (buildId=%s)", installDTO.PublicID, installDTO.BuildID)
}

// versionString NullString 安全取串
func versionString(v sql.NullString) string {
	if v.Valid {
		return v.String
	}
	return ""
}

// upgradeDirection 版本方向（up=升级/down=降级/none=同号换构建），仅展示用
func upgradeDirection(installed, target string) string {
	switch compareVersion(installed, target) {
	case -1:
		return "up"
	case 1:
		return "down"
	default:
		return "none"
	}
}

// compareVersion 比较点分版本号：数字段逐段数值比较，非数字段按字符串比较；空串视为最小。
// 返回 -1（a<b）/0（相等）/1（a>b）
func compareVersion(a, b string) int {
	if a == b {
		return 0
	}
	segsA, segsB := strings.Split(a, "."), strings.Split(b, ".")
	n := len(segsA)
	if len(segsB) > n {
		n = len(segsB)
	}
	for i := 0; i < n; i++ {
		sa, sb := "", ""
		if i < len(segsA) {
			sa = segsA[i]
		}
		if i < len(segsB) {
			sb = segsB[i]
		}
		na, ea := strconv.Atoi(sa)
		nb, eb := strconv.Atoi(sb)
		switch {
		case ea != nil || eb != nil:
			// 任一段非纯数字：按字符串比较（空串最小）
			if c := strings.Compare(sa, sb); c != 0 {
				return c
			}
		case na != nb:
			if na < nb {
				return -1
			}
			return 1
		}
	}
	return 0
}

// contractVersionOf 取插件记录的契约版本（NULL/0 = 未声明，校验时视为当前契约放行，与加载终检同语义）
func contractVersionOf(p *entity2.Plugin) int {
	if p.ContractVersion.Valid {
		return int(p.ContractVersion.Int64)
	}
	return 0
}
