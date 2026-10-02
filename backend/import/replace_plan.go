package importer

// 回灌前置编排的共享能力（自 share 收件执行器提取，行为保持重构）：分享收件与 zip 导入
// 在「manifest → 查重 → 确认 → 入库」链上需要完全一致的前置编排——查重三分类 → 覆盖确认
// （弹窗+决策记忆复用）→ 逐文件内容判定 → 替换全集软删＋终态回滚登记。按「同一语义单一
// 判据」原则单点实现在本模块（回灌全链归 import，share 专注网络层；依赖方向 share→import）。
// 内容判定与本地拷贝见 content_match.go，子 manifest 构造与跳过集见 manifest_subset.go。

import (
	"context"
	"fmt"
	"time"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/duplicate"
	"github.com/library-squirrel/backend/export"
	"github.com/library-squirrel/backend/resource"
	"github.com/library-squirrel/backend/taskManager"
)

// StoreMountReader 活行 store 挂载内容载荷批量查询（resource.Service 实现，接口由本模块声明）：
// 按作品批量返回其活行 store 的挂载键（store_type + store_seq）、头部指纹与文件路径，
// 供 PlanReplace 逐文件内容判定与暂存拷贝使用。StoreMountInfo 域类型定义在 resource 包。
type StoreMountReader interface {
	ListMountsByWorkIds(ctx context.Context, workIds []int64) (map[int64][]resource.StoreMountInfo, error)
}

// ReplacePlanner 回灌前置编排器：查重三分类 → 确认弹窗（决策记忆复用）→ 逐文件内容判定 →
// 替换全集软删＋终态回滚登记。分享收件执行器与（未来的）zip 导入执行器共用同一实例语义。
type ReplacePlanner struct {
	checker     duplicate.DuplicateChecker // 查重判定能力（manifest 作品键 + 板块角色三分类）
	replaceOps  resource.ReplaceStoreOps   // 替换链能力（软删替换目标 + 失败回滚复活）
	mountReader StoreMountReader           // 活行 store 挂载内容载荷查询（逐文件内容判定；nil=不判定走原替换）
}

// NewReplacePlanner 创建回灌前置编排器。checker/replaceOps 未装配（nil）时 PlanReplace 维持
// 全新建/既有跳过旧语义（异常装配兜底，与提取前收件执行器行为一致）。
func NewReplacePlanner(checker duplicate.DuplicateChecker, replaceOps resource.ReplaceStoreOps,
	mountReader StoreMountReader) *ReplacePlanner {
	return &ReplacePlanner{checker: checker, replaceOps: replaceOps, mountReader: mountReader}
}

// ReplacePlan 查重裁决产物：替换全集（确认替换 ∪ 零交集并入）与用户裁决跳过作品。
// 三集合均以 manifest 作品 ID 为键（与 IngestOptions 替换集的键域一致，Ingest 据此回灌）。
type ReplacePlan struct {
	ConfirmedWorks map[int64]struct{} // 确认替换：冲突交集命中且用户选替换
	AutoMergeWorks map[int64]struct{} // 零交集自动并入：不经确认直接增补挂载（决策5）
	SkipWorks      map[int64]struct{} // 用户裁决跳过：整作品跳过，文件不拉
}

// replaceSoftTarget 需软删的替换目标（确认替换与零交集并入共用）：
// manifest 作品 ID → 本库作品 ID + 软删角色集。
type replaceSoftTarget struct {
	manifestID    int64
	localWorkID   int64
	conflictRoles []string // 交集角色（冲突命中载荷；零交集为空 → 回退 manifest 板块角色）
	manifestRoles []string
}

// PlanReplace 查重 → 确认 → 软删与回滚登记（回灌前置编排主体）：
//   - manifest 作品键（站点键 + 站点侧作品 ID）+ 板块角色集合三分类（DuplicateChecker.Check）
//   - 命中冲突非空 → WaitReplaceConfirm 整体决策（任务粒度，复用 ConfirmReplace 答复）；
//     取消返回 canceled=true，调用方不上报终态交控制面接管
//   - 零交集命中作品自动并入 AutoMergeWorks（查重命中即挂已有作品，弹窗与否只决定确认）
//   - 替换全集（确认替换 ∪ 零交集并入）按各作品「交集角色」（零交集/保守弹窗回退 manifest
//     板块角色全集）软删——冲突交集替换与零交集 no-op 两语义天然统一（活行交集仅冲突角色）；
//     软删成功后立即经 SetTerminalRollback 登记回滚清单（失败/停止由控制面 setFailed 单点复活，
//     多作品清单合并登记，软删中断窗口三态收口见分享方案「设计六」）
func (p *ReplacePlanner) PlanReplace(ctx context.Context, manifest *export.Manifest,
	staging, workDir string, h taskManager.StrategyHandle) (*ReplacePlan, bool, error) {
	taskID := h.Task().GetID()
	plan := &ReplacePlan{
		ConfirmedWorks: make(map[int64]struct{}),
		AutoMergeWorks: make(map[int64]struct{}),
		SkipWorks:      make(map[int64]struct{}),
	}
	if p.checker == nil || p.replaceOps == nil {
		// 查重/替换能力未装配（异常装配兜底）：维持全新建/既有跳过旧语义
		return plan, false, nil
	}

	// 反解 manifest 作品键与板块角色集合（站点键取 manifest.Sites 映射；无站点身份作品按未命中处理）
	siteKeyByID := make(map[int64]string, len(manifest.Sites))
	for i := range manifest.Sites {
		if s := manifest.Sites[i]; s.SiteKey != "" {
			siteKeyByID[s.ID] = s.SiteKey
		}
	}
	items := make([]duplicate.DuplicateCheckItem, 0, len(manifest.Works))
	rolesByWork := make(map[int64][]string, len(manifest.Works))
	for i := range manifest.Works {
		w := &manifest.Works[i]
		roles := manifestWorkRoles(w)
		rolesByWork[w.ID] = roles
		var siteKey, siteWorkID string
		if w.SiteID != nil {
			siteKey = siteKeyByID[*w.SiteID]
		}
		if w.SiteWorkID != nil {
			siteWorkID = *w.SiteWorkID
		}
		items = append(items, duplicate.DuplicateCheckItem{
			SiteKey:    siteKey,
			SiteWorkID: siteWorkID,
			Roles:      roles,
		})
	}
	checkStart := time.Now()
	results, err := p.checker.Check(ctx, items)
	if err != nil {
		return nil, false, fmt.Errorf("作品查重判定失败: %w", err)
	}
	logger.Log.Debugf("[replace-plan] 任务 %d 作品查重完成 耗时=%s 条目=%d", taskID, time.Since(checkStart), len(items))

	// 三分类分流：冲突作品收集确认输入，零交集作品并入自动增补（未命中作品交 ingest 既有创建）
	var conflicts []taskManager.ConflictInfo
	var confirmTargets, autoTargets []replaceSoftTarget
	for i, res := range results {
		w := &manifest.Works[i]
		switch res.Class {
		case duplicate.DuplicateHitConflict:
			conflicts = append(conflicts, taskManager.ConflictInfo{
				WorkID:        res.WorkID,
				WorkName:      res.WorkName,
				ConflictRoles: res.ConflictRoles,
			})
			confirmTargets = append(confirmTargets, replaceSoftTarget{
				manifestID:    w.ID,
				localWorkID:   res.WorkID,
				conflictRoles: res.ConflictRoles,
				manifestRoles: rolesByWork[w.ID],
			})
		case duplicate.DuplicateHitNoConflict:
			plan.AutoMergeWorks[w.ID] = struct{}{}
			autoTargets = append(autoTargets, replaceSoftTarget{
				manifestID:    w.ID,
				localWorkID:   res.WorkID,
				manifestRoles: rolesByWork[w.ID],
			})
		}
	}

	// 冲突作品整体决策（任务粒度）：先查确认决策记忆命中（冲突本地作品 ID 集集合相等）复用决策
	// 不弹窗（单次会话内确认保留）；未命中弹窗等待整体答复（WaitReplaceConfirm 内部已记
	// 记忆，取消返回时供恢复复用）
	if len(conflicts) > 0 {
		if memo := h.ConfirmMemo(); memo != nil && sameIDSet(memo.ConflictWorkIds, ConflictWorkIDsOf(conflicts)) {
			// 记忆命中：复用既有整体决策，不重复弹窗
			logger.Log.Debugf("[replace-plan] 任务 %d 确认决策记忆命中，复用 decision=%d", taskID, memo.Decision)
			applyConfirmDecision(plan, confirmTargets, memo.Decision)
		} else {
			logger.Log.Infof("[replace-plan] 任务 %d 弹窗等待替换确认 冲突数=%d", taskID, len(conflicts))
			confirmStart := time.Now()
			decision, canceled := h.WaitReplaceConfirm(conflicts)
			logger.Log.Infof("[replace-plan] 任务 %d 替换确认返回 耗时=%s canceled=%v decision=%d", taskID, time.Since(confirmStart), canceled, decision)
			if canceled {
				return nil, true, nil // 记忆已由 WaitReplaceConfirm 记录，恢复复用
			}
			applyConfirmDecision(plan, confirmTargets, decision)
		}
	}

	// 逐文件内容判定：确认替换作品按 manifest 文件条目与本地活行 store 内容比对——
	// 全部匹配的整作品跳过（不软删/不拉取/不导入），部分匹配的匹配文件由本地活文件
	// 拷入暂存免重复获取。判定仅作用于确认替换目标（confirmTargets），零交集自动增补不受影响。
	if p.mountReader != nil {
		contentStart := time.Now()
		if err := p.applyContentMatches(ctx, manifest, staging, workDir, plan, &confirmTargets); err != nil {
			return nil, false, fmt.Errorf("逐文件内容判定失败: %w", err)
		}
		logger.Log.Debugf("[replace-plan] 任务 %d 逐文件内容判定完成 耗时=%s 跳过=%d 确认替换=%d", taskID, time.Since(contentStart), len(plan.SkipWorks), len(plan.ConfirmedWorks))
	}

	// 软删替换全集（确认替换 ∪ 零交集并入）各作品的软删角色并登记回滚清单；
	// 零交集命中作品经此 no-op（活行交集为空），软删后恢复重跑延续同一机制
	softTargets := autoTargets
	if len(plan.ConfirmedWorks) > 0 {
		softTargets = append(softTargets, confirmTargets...)
	}
	logger.Log.Debugf("[replace-plan] 任务 %d 软删替换目标 数量=%d", taskID, len(softTargets))
	for _, t := range softTargets {
		roles := t.conflictRoles
		if len(roles) == 0 {
			roles = t.manifestRoles
		}
		refs, serr := p.replaceOps.SoftDeleteWorkStoreRoles(ctx, t.localWorkID, roles)
		if serr != nil {
			// 部分清单也可回滚：已软删行先登记（即使错误），再上抛交 Fail 单点复活
			if len(refs) > 0 {
				h.SetTerminalRollback(taskManager.TerminalRollback{Victims: refs})
			}
			return nil, false, fmt.Errorf("软删替换目标作品(id=%d)失败: %w", t.localWorkID, serr)
		}
		if len(refs) > 0 {
			h.SetTerminalRollback(taskManager.TerminalRollback{Victims: refs})
		}
	}
	return plan, false, nil
}

// applyConfirmDecision 将整体决策应用到确认目标集（任务粒度）：替换 → 确认替换集；跳过 → 整作品跳过
// （零交集角色同样不增补）。PlanReplace 的记忆复用与弹窗两路径共用同一落位。
func applyConfirmDecision(plan *ReplacePlan, targets []replaceSoftTarget, decision taskManager.ReplaceDecision) {
	switch decision {
	case taskManager.ReplaceDecisionSkip:
		for _, t := range targets {
			plan.SkipWorks[t.manifestID] = struct{}{}
		}
	default:
		for _, t := range targets {
			plan.ConfirmedWorks[t.manifestID] = struct{}{}
		}
	}
}

// ConflictWorkIDsOf 提取冲突作品的本地 ID 集（保序去重；确认决策记忆键，
// 对齐 taskManager.conflictWorkIds 语义——记忆记录与比对共用同一形态）
func ConflictWorkIDsOf(conflicts []taskManager.ConflictInfo) []int64 {
	ids := make([]int64, 0, len(conflicts))
	seen := make(map[int64]struct{}, len(conflicts))
	for _, c := range conflicts {
		if _, ok := seen[c.WorkID]; ok {
			continue
		}
		seen[c.WorkID] = struct{}{}
		ids = append(ids, c.WorkID)
	}
	return ids
}

// sameIDSet 集合相等比对（顺序无关）：确认记忆键与当前冲突 ID 集一致才复用决策。
// 两次执行的冲突集查重顺序理论可漂移，集合比对消除顺序依赖。
func sameIDSet(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[int64]struct{}, len(a))
	for _, id := range a {
		seen[id] = struct{}{}
	}
	for _, id := range b {
		if _, ok := seen[id]; !ok {
			return false
		}
	}
	return true
}
