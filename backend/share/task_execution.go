package share

// share-receive 任务的执行面策略（taskManager.ExecutionStrategy 实现，按 task_type 注册进
// Manager 策略表，app.go 装配）：
//   - ReceiveExecution：收件人子任务拉取数据流（读本地共享 manifest → 逐文件暂存续传 → ManifestIngestor 回灌导入）
//
// 分享方发布不走任务模块（发布直跑经 Service 内受监督 goroutine 驱动，生命周期落
// share_record——见 service.go/record.go）。

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/duplicate"
	"github.com/library-squirrel/backend/export"
	importer "github.com/library-squirrel/backend/import"
	"github.com/library-squirrel/backend/resource"
	"github.com/library-squirrel/backend/taskManager"
)

// —— share-receive（收件人拉取）——
//
// 数据流：反解子任务载荷 → 读本地共享 manifest（Receive 预拉落盘父任务目录）→ 过滤本作品
// 子集构造子 manifest → 收件人客户端拨中继逐文件拉取本作品文件至暂存目录（大小对齐即完成；
// 中断后按暂存大小续传，非中止清理）→ ManifestIngestor 回灌导入子 manifest → 成功清理暂存。
// 拉取中断/分享方离线由任务模型承接：暂停/停止保留暂存，重试/恢复从暂存续传；会话终态
// （撤销/过期/不存在）以用户可读文案置失败。过时载荷（ManifestID==0，存量整体任务）显式 Fail。

// receiveStagingRootName workDir 下的收件暂存目录名（任务行一个子目录；不在 store/ 白名单
// 子树内，fsmonitor 不感知）
const receiveStagingRootName = "share-receive"

// 收件拉取退避参数（瞬态错误：网络/分享方离线/中继限流）
const (
	receiveMaxAttempts  = 4           // 单请求最大尝试次数（1 次初始 + 3 次重试）
	receiveBackoffStart = time.Second // 首次退避
	receiveBackoffMax   = 8 * time.Second
)

// StoreMountReader 活行 store 挂载内容载荷批量查询（resource.Service 实现，接口由本模块声明）：
// 按作品批量返回其活行 store 的挂载键（store_type + store_seq）、头部指纹与文件路径，
// 供 planReplace 逐文件内容判定与暂存拷贝使用。StoreMountInfo 域类型定义在 resource 包。
type StoreMountReader interface {
	ListMountsByWorkIds(ctx context.Context, workIds []int64) (map[int64][]resource.StoreMountInfo, error)
}

// ReceiveExecution share-receive（收件人拉取）任务的执行面策略。
type ReceiveExecution struct {
	svc         *Service                   // 提供 workDir / instanceID / 测试可覆写参数
	ingestor    importer.ManifestIngestor  // 回灌导入能力（与 import handler 同一实例，app.go 装配）
	checker     duplicate.DuplicateChecker // 查重判定能力（manifest 作品键 + 板块角色三分类）
	replaceOps  resource.ReplaceStoreOps   // 替换链能力（软删替换目标 + 失败回滚复活）
	mountReader StoreMountReader           // 活行 store 挂载内容载荷查询（逐文件内容判定；nil=不判定走原替换）
}

// NewReceiveExecution 创建 share-receive 执行面策略
func NewReceiveExecution(svc *Service, ingestor importer.ManifestIngestor,
	checker duplicate.DuplicateChecker, replaceOps resource.ReplaceStoreOps,
	mountReader StoreMountReader) *ReceiveExecution {
	return &ReceiveExecution{svc: svc, ingestor: ingestor, checker: checker,
		replaceOps: replaceOps, mountReader: mountReader}
}

// Execute 收件人子任务拉取主体：读本地共享 manifest → 过滤本作品子集 → 拉取本作品文件至
// 暂存 → 回灌导入 → 清理暂存并置成功；失败置用户可读文案（暂存保留供重试续传）；
// ctx 取消（暂停/停止）不上报终态交控制面接管。过时载荷（ManifestID==0）显式 Fail。
func (e *ReceiveExecution) Execute(h taskManager.StrategyHandle) {
	task := h.Task()
	payload, err := parseShareReceivePayload(task.Payload.String)
	if err != nil {
		h.Fail(err.Error())
		return
	}
	workDir := e.svc.workDir()
	if workDir == "" {
		h.Fail("工作目录未配置，无法接收分享")
		return
	}
	if payload.ManifestID == 0 {
		// 过时载荷（存量整体任务）：新代码不兼容存量，不做迁移或降级
		h.Fail("请删除本任务后重新接收分享")
		return
	}
	// 读本地共享 manifest（Receive 预拉落盘父任务目录，子任务不重复网络拉取）
	manifest, err := readSharedManifest(workDir, payload.ManifestPath)
	if err != nil {
		h.Fail(err.Error())
		return
	}
	if manifest.SchemaVersion != export.SchemaVersion {
		h.Fail(fmt.Sprintf("共享 manifest 版本不支持: %d", manifest.SchemaVersion))
		return
	}
	// 构造只含本作品的子 manifest（按 ManifestID 定位本作品，查重/暂存/导入均收窄到本作品）
	sub, err := buildSubManifest(manifest, payload.ManifestID)
	if err != nil {
		h.Fail(err.Error())
		return
	}
	logger.Log.Infof("[share-recv] 任务 %d 执行开始 manifest=%s", task.GetID(), payload.ManifestPath)
	client, err := newReceiveClient(payload, e.svc.instanceID, e.svc.opts)
	if err != nil {
		h.Fail(err.Error())
		return
	}
	client.taskID = task.GetID()
	ctx := h.RunCtx()
	staging := filepath.Join(workDir, receiveStagingRootName, strconv.FormatInt(task.GetID(), 10))
	if err := os.MkdirAll(staging, 0o755); err != nil {
		h.Fail(fmt.Sprintf("创建暂存目录失败: %v", err))
		return
	}

	// 查重 → 确认 → 逐文件内容判定 → 软删 + 回滚登记（作用域为本作品子集；时序语义同整体路径，见设计七）
	planStart := time.Now()
	plan, canceled, err := e.planReplace(ctx, sub, staging, workDir, h)
	logger.Log.Infof("[share-recv] 任务 %d 查重+确认+内容判定+软删 完成 耗时=%s canceled=%v err=%v", task.GetID(), time.Since(planStart), canceled, err)
	if err != nil {
		reportReceiveError(h, ctx, err)
		return
	}
	if canceled {
		return // 确认被取消（暂停/停止防御性打断）：不上报终态，交控制面接管
	}
	if ctx.Err() != nil {
		logger.Log.Infof("[share-recv] 任务 %d 查重替换阶段被暂停/停止打断", task.GetID())
		return // 软删窗口内暂停/停止：回滚清单已登记（交 setFailed 单点），暂停延续替换
	}

	// 阶段二：逐文件拉取至暂存（只拉本作品引用文件；被裁决跳过作品的文件不拉）
	stageStart := time.Now()
	if err := e.stageFiles(ctx, client, staging, sub, fileSkipSet(sub, plan.skipWorks), h); err != nil {
		logger.Log.Debugf("[share-recv] 任务 %d 拉取阶段失败 耗时=%s err=%v", task.GetID(), time.Since(stageStart), err)
		reportReceiveError(h, ctx, err)
		return
	}
	logger.Log.Infof("[share-recv] 任务 %d 拉取阶段完成 耗时=%s", task.GetID(), time.Since(stageStart))
	if ctx.Err() != nil {
		logger.Log.Infof("[share-recv] 任务 %d 拉取完成后被暂停/停止打断", task.GetID())
		return
	}

	// 阶段三：回灌导入子 manifest（文件源读暂存；入库/查重/落盘全链复用导出回灌能力）。
	// 替换选项：确认替换与零交集并入并入注入；二者皆空（无命中替换）则保持全跳过旧语义
	var opts *importer.IngestOptions
	if len(plan.confirmedWorks) > 0 || len(plan.autoMergeWorks) > 0 {
		opts = &importer.IngestOptions{
			ReplaceWorks:   plan.confirmedWorks,
			AutoMergeWorks: plan.autoMergeWorks,
		}
	}
	ingestStart := time.Now()
	if _, err := e.ingestor.Ingest(ctx, sub, stagedFileSource(staging), opts); err != nil {
		logger.Log.Debugf("[share-recv] 任务 %d 导入失败 耗时=%s err=%v", task.GetID(), time.Since(ingestStart), err)
		reportReceiveError(h, ctx, err)
		return
	}
	logger.Log.Infof("[share-recv] 任务 %d 导入完成 耗时=%s", task.GetID(), time.Since(ingestStart))
	// 成功：清理本任务暂存（共享 manifest.json 在父任务目录，不动；残留由启动清扫回收）
	_ = os.RemoveAll(staging)
	h.Finish()
	logger.Log.Infof("[share-recv] 任务 %d 执行完成", task.GetID())
}

// readSharedManifest 读本地共享 manifest：workDir 相对路径（正斜杠 relPath 域），
// 在 os.ReadFile 调用点现场 join 为绝对路径（absPath 域）。
func readSharedManifest(workDir, relPath string) (*export.Manifest, error) {
	if relPath == "" {
		return nil, errors.New("任务载荷缺少共享 manifest 路径")
	}
	data, err := os.ReadFile(filepath.Join(workDir, relPath))
	if err != nil {
		return nil, fmt.Errorf("读取共享 manifest 失败: %w", err)
	}
	manifest, err := export.Deserialize(data)
	if err != nil {
		return nil, fmt.Errorf("解析共享 manifest 失败: %w", err)
	}
	return manifest, nil
}

// buildSubManifest 构造只含本作品的子 manifest（纯数据组装，不改 ManifestIngestor 接口）：
// Works 仅保留 manifestID 匹配的作品，Files 仅保留本作品 Stores[].StoreID 引用的条目；
// 站点/作者/标签/作品集保留全集（find-or-create 幂等，多子任务并发导入同一主数据无冲突）。
// Meta 计数按子 manifest 实际内容更新（仅展示用途，不参与导入判定）。
func buildSubManifest(src *export.Manifest, manifestID int64) (*export.Manifest, error) {
	var work *export.WorkRecord
	for i := range src.Works {
		if src.Works[i].ID == manifestID {
			work = &src.Works[i]
			break
		}
	}
	if work == nil {
		return nil, fmt.Errorf("共享 manifest 中不存在作品 ID %d", manifestID)
	}
	storeIDs := make(map[int64]struct{})
	for i := range work.Resources {
		for _, s := range work.Resources[i].Stores {
			storeIDs[s.StoreID] = struct{}{}
		}
	}
	sub := &export.Manifest{
		SchemaVersion: src.SchemaVersion,
		Meta:          src.Meta,
		Sites:         src.Sites,
		LocalAuthors:  src.LocalAuthors,
		SiteAuthors:   src.SiteAuthors,
		LocalTags:     src.LocalTags,
		SiteTags:      src.SiteTags,
		WorkSets:      src.WorkSets,
		Works:         []export.WorkRecord{*work},
	}
	for i := range src.Files {
		if _, ok := storeIDs[src.Files[i].StoreID]; ok {
			sub.Files = append(sub.Files, src.Files[i])
		}
	}
	sub.Meta.WorkCount = 1
	sub.Meta.FileCount = len(sub.Files)
	return sub, nil
}

// reportReceiveError 统一的错误收口：ctx 已取消（暂停/停止）不上报终态交控制面接管，
// 其余按分类转用户可读文案置失败。
func reportReceiveError(h taskManager.StrategyHandle, ctx context.Context, err error) {
	if ctx.Err() != nil {
		return
	}
	h.Fail(receiveUserMessage(err))
}

// receiveUserMessage 错误 → 用户可读文案（中继拨号分类/流内错误/截断/透传）
func receiveUserMessage(err error) string {
	var de *relayDialError
	if errors.As(err, &de) {
		if msg, _ := relayDialFailMessage(err); msg != "" {
			return msg
		}
		return de.Error()
	}
	var ae *streamAppError
	if errors.As(err, &ae) {
		switch ae.code {
		case streamErrNotFound:
			return "分享方数据与清单不一致，无法完成拉取"
		case streamErrMissing:
			return "分享方源文件缺失，无法完成拉取"
		case streamErrBadRequest:
			return "拉取请求被分享方拒绝"
		default:
			return fmt.Sprintf("分享方内部错误（%s）", ae.code)
		}
	}
	if errors.Is(err, errStreamTruncated) {
		return "拉取数据不完整（传输中断），可在任务面板重试续传"
	}
	msg := err.Error()
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return msg
}

// receiveReplacePlan 查重裁决产物：替换全集（确认替换 ∪ 零交集并入）与用户裁决跳过作品。
// 三集合均以 manifest 作品 ID 为键（与 IngestOptions 替换集的键域一致，ingest 据此回灌）。
type receiveReplacePlan struct {
	confirmedWorks map[int64]struct{} // 确认替换：冲突交集命中且用户选替换
	autoMergeWorks map[int64]struct{} // 零交集自动并入：不经确认直接增补挂载（决策5）
	skipWorks      map[int64]struct{} // 用户裁决跳过：整作品跳过，文件不拉
}

// replaceSoftTarget 需软删的替换目标（确认替换与零交集并入共用）：
// manifest 作品 ID → 本库作品 ID + 软删角色集。
type replaceSoftTarget struct {
	manifestID    int64
	localWorkID   int64
	conflictRoles []string // 交集角色（冲突命中载荷；零交集为空 → 回退 manifest 板块角色）
	manifestRoles []string
}

// planReplace 查重 → 确认 → 软删与回滚登记（时序改造核心）：
//   - manifest 作品键（站点名 + 站点侧作品 ID）+ 板块角色集合三分类（DuplicateChecker.Check）
//   - 命中冲突非空 → WaitReplaceConfirm 整体决策（任务粒度，复用 ConfirmReplace 答复）；
//     取消返回 canceled=true，Execute 不上报终态交控制面接管
//   - 零交集命中作品自动并入 autoMergeWorks（查重命中即挂已有作品，弹窗与否只决定确认）
//   - 替换全集（确认替换 ∪ 零交集并入）按各作品「交集角色」（零交集/保守弹窗回退 manifest
//     板块角色全集）软删——冲突交集替换与零交集 no-op 两语义天然统一（活行交集仅冲突角色）；
//     软删成功后立即经 SetTerminalRollback 登记回滚清单（失败/停止由控制面 setFailed 单点复活，
//     多作品清单合并登记，软删中断窗口三态收口见方案「设计六」）
func (e *ReceiveExecution) planReplace(ctx context.Context, manifest *export.Manifest,
	staging, workDir string, h taskManager.StrategyHandle) (*receiveReplacePlan, bool, error) {
	taskID := h.Task().GetID()
	plan := &receiveReplacePlan{
		confirmedWorks: make(map[int64]struct{}),
		autoMergeWorks: make(map[int64]struct{}),
		skipWorks:      make(map[int64]struct{}),
	}
	if e.checker == nil || e.replaceOps == nil {
		// 查重/替换能力未装配（异常装配兜底）：维持全新建/既有跳过旧语义
		return plan, false, nil
	}

	// 反解 manifest 作品键与板块角色集合（站点名取 manifest.Sites 映射；无站点身份作品按未命中处理）
	siteNameByID := make(map[int64]string, len(manifest.Sites))
	for i := range manifest.Sites {
		if s := manifest.Sites[i]; s.SiteName != nil {
			siteNameByID[s.ID] = *s.SiteName
		}
	}
	items := make([]duplicate.DuplicateCheckItem, 0, len(manifest.Works))
	rolesByWork := make(map[int64][]string, len(manifest.Works))
	for i := range manifest.Works {
		w := &manifest.Works[i]
		roles := manifestWorkRoles(w)
		rolesByWork[w.ID] = roles
		var siteName, siteWorkID string
		if w.SiteID != nil {
			siteName = siteNameByID[*w.SiteID]
		}
		if w.SiteWorkID != nil {
			siteWorkID = *w.SiteWorkID
		}
		items = append(items, duplicate.DuplicateCheckItem{
			SiteName:   siteName,
			SiteWorkID: siteWorkID,
			Roles:      roles,
		})
	}
	checkStart := time.Now()
	results, err := e.checker.Check(ctx, items)
	if err != nil {
		return nil, false, fmt.Errorf("作品查重判定失败: %w", err)
	}
	logger.Log.Debugf("[share-recv] 任务 %d 作品查重完成 耗时=%s 条目=%d", taskID, time.Since(checkStart), len(items))

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
			plan.autoMergeWorks[w.ID] = struct{}{}
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
		if memo := h.ConfirmMemo(); memo != nil && sameIDSet(memo.ConflictWorkIds, conflictWorkIDsOf(conflicts)) {
			// 记忆命中：复用既有整体决策，不重复弹窗
			logger.Log.Debugf("[share-recv] 任务 %d 确认决策记忆命中，复用 decision=%d", taskID, memo.Decision)
			applyConfirmDecision(plan, confirmTargets, memo.Decision)
		} else {
			logger.Log.Infof("[share-recv] 任务 %d 弹窗等待替换确认 冲突数=%d", taskID, len(conflicts))
			confirmStart := time.Now()
			decision, canceled := h.WaitReplaceConfirm(conflicts)
			logger.Log.Infof("[share-recv] 任务 %d 替换确认返回 耗时=%s canceled=%v decision=%d", taskID, time.Since(confirmStart), canceled, decision)
			if canceled {
				return nil, true, nil // 记忆已由 WaitReplaceConfirm 记录，恢复复用
			}
			applyConfirmDecision(plan, confirmTargets, decision)
		}
	}

	// 逐文件内容判定：确认替换作品按 manifest 文件条目与本地活行 store 内容比对——
	// 全部匹配的整作品跳过（不软删/不拉取/不导入），部分匹配的匹配文件由本地活文件
	// 拷入暂存免网络重拉。判定仅作用于确认替换目标（confirmTargets），零交集自动增补不受影响。
	if e.mountReader != nil {
		contentStart := time.Now()
		if err := e.applyContentMatches(ctx, manifest, staging, workDir, plan, &confirmTargets); err != nil {
			return nil, false, fmt.Errorf("逐文件内容判定失败: %w", err)
		}
		logger.Log.Debugf("[share-recv] 任务 %d 逐文件内容判定完成 耗时=%s 跳过=%d 确认替换=%d", taskID, time.Since(contentStart), len(plan.skipWorks), len(plan.confirmedWorks))
	}

	// 软删替换全集（确认替换 ∪ 零交集并入）各作品的软删角色并登记回滚清单；
	// 零交集命中作品经此 no-op（活行交集为空），软删后恢复重跑延续同一机制
	softTargets := autoTargets
	if len(plan.confirmedWorks) > 0 {
		softTargets = append(softTargets, confirmTargets...)
	}
	logger.Log.Debugf("[share-recv] 任务 %d 软删替换目标 数量=%d", taskID, len(softTargets))
	for _, t := range softTargets {
		roles := t.conflictRoles
		if len(roles) == 0 {
			roles = t.manifestRoles
		}
		refs, serr := e.replaceOps.SoftDeleteWorkStoreRoles(ctx, t.localWorkID, roles)
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
// （零交集角色同样不增补）。planReplace 的记忆复用与弹窗两路径共用同一落位。
func applyConfirmDecision(plan *receiveReplacePlan, targets []replaceSoftTarget, decision taskManager.ReplaceDecision) {
	switch decision {
	case taskManager.ReplaceDecisionSkip:
		for _, t := range targets {
			plan.skipWorks[t.manifestID] = struct{}{}
		}
	default:
		for _, t := range targets {
			plan.confirmedWorks[t.manifestID] = struct{}{}
		}
	}
}

// mountKey 挂载键（store_type + store_seq）：manifest 文件条目与本地活行 store 按此配对判定
type mountKey struct {
	storeType string
	storeSeq  int64
}

// applyContentMatches 对确认替换作品做逐文件内容判定：
//   - 全部文件与本地活行 store 内容一致（头部指纹快路径 + 全量 sha256 强校验）→ 整作品跳过：
//     从确认替换集移除并加入整作品跳过集——不软删/不拉取/不导入，保留现状（confirmTargets 同步过滤）。
//   - 部分匹配 → 作品保留确认替换（整作品替换收尾），匹配文件由本地活文件拷入暂存免网络重拉
//     （stageFiles 见满尺寸暂存即跳过）。
//   - 无匹配/缺指纹/无本地对应 store/文件数 0 → 原样整作品替换（安全回退，不误跳过）。
//
// 判定仅作用于确认替换目标；裁决跳过作品已入 skipWorks、零交集自动增补不涉及覆盖，均不受影响。
func (e *ReceiveExecution) applyContentMatches(ctx context.Context, manifest *export.Manifest,
	staging, workDir string, plan *receiveReplacePlan, targets *[]replaceSoftTarget) error {
	workIds := make([]int64, 0, len(*targets))
	for _, t := range *targets {
		if _, ok := plan.confirmedWorks[t.manifestID]; ok {
			workIds = append(workIds, t.localWorkID)
		}
	}
	localMounts, err := e.mountReader.ListMountsByWorkIds(ctx, workIds)
	if err != nil {
		return err
	}
	entryByStoreID := make(map[int64]*export.FileEntry, len(manifest.Files))
	for i := range manifest.Files {
		entryByStoreID[manifest.Files[i].StoreID] = &manifest.Files[i]
	}
	for i := len(*targets) - 1; i >= 0; i-- {
		t := (*targets)[i]
		if _, ok := plan.confirmedWorks[t.manifestID]; !ok {
			continue // 裁决跳过作品已入 skipWorks，不参与内容判定
		}
		work := findManifestWork(manifest, t.manifestID)
		if work == nil {
			continue
		}
		fileByKey := make(map[mountKey]*export.FileEntry)
		for j := range work.Resources {
			for _, s := range work.Resources[j].Stores {
				fileByKey[mountKey{s.StoreType, int64(s.StoreSeq)}] = entryByStoreID[s.StoreID]
			}
		}
		localByKey := make(map[mountKey]resource.StoreMountInfo)
		for _, m := range localMounts[t.localWorkID] {
			localByKey[mountKey{m.StoreType, m.StoreSeq}] = m
		}
		var matched []resource.StoreMountInfo
		total := 0
		for key, entry := range fileByKey {
			if entry == nil {
				continue // 挂载无文件条目（异常态）：不计入文件数，不影响判定
			}
			total++
			if entry.Missing || entry.Path == "" {
				continue // 缺席/无包内路径文件不参与匹配（也不拉取），同时阻断整作品跳过
			}
			local, ok := localByKey[key]
			if !ok {
				continue // 无本地对应活行 store → 不匹配
			}
			if e.fileContentMatches(ctx, workDir, local, entry) {
				matched = append(matched, local)
			}
		}
		if total > 0 && len(matched) == total {
			// 全部匹配：内容一致 → 整作品跳过（保留现状，不软删/不拉取/不导入）
			delete(plan.confirmedWorks, t.manifestID)
			plan.skipWorks[t.manifestID] = struct{}{}
			*targets = append((*targets)[:i], (*targets)[i+1:]...)
			continue
		}
		if len(matched) == 0 {
			continue // 无匹配 → 原样整作品替换（安全回退）
		}
		// 部分匹配：匹配文件拷入暂存（免网络重拉）；不匹配文件留待 stageFiles 拉取
		for _, local := range matched {
			key := mountKey{local.StoreType, local.StoreSeq}
			if err := copyLocalToStaging(ctx, staging, workDir, local, fileByKey[key]); err != nil {
				return err
			}
		}
	}
	return nil
}

// fileContentMatches 判定本地活文件与宿主文件条目内容一致：快路径头部指纹直比（零读盘），
// 匹配再读本地文件全量 sha256 与 manifest Sha256 比对（全量强校验）。
// 任一指纹缺失/本地路径为空/本地文件不可读 → 不匹配（安全回退，不误跳过）。
func (e *ReceiveExecution) fileContentMatches(ctx context.Context, workDir string,
	local resource.StoreMountInfo, entry *export.FileEntry) bool {
	if entry.ContentFingerprint == "" || entry.Sha256 == "" || local.ContentFingerprint == "" || local.StorePath == "" {
		return false
	}
	if entry.ContentFingerprint != local.ContentFingerprint {
		return false
	}
	sum, err := sha256File(ctx, filepath.Join(workDir, filepath.FromSlash(local.StorePath)))
	if err != nil {
		return false
	}
	return sum == entry.Sha256
}

// copyLocalToStaging 把本地活文件拷入暂存（满尺寸；stageFiles 见「暂存==声明」即跳过拉取）。
// 拷贝读流 tee 全量 sha256 二次确认——源文件在判定与拷贝之间被改动则放弃拷贝改网络拉取。
// 所有失败路径移除半成品暂存后回落拉取（不阻断整作品替换主流程，真实错误由 stageFiles 报出）。
func copyLocalToStaging(ctx context.Context, staging, workDir string,
	local resource.StoreMountInfo, entry *export.FileEntry) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	target := stagingPath(staging, entry.Path)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return nil // 暂存不可写由 stageFiles 报错，此处回落拉取
	}
	src, err := os.Open(filepath.Join(workDir, filepath.FromSlash(local.StorePath)))
	if err != nil {
		return nil // 本地文件已不可读 → 回落网络拉取
	}
	defer func() { _ = src.Close() }()
	dst, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		_ = os.Remove(target)
		return nil
	}
	hasher := sha256.New()
	written, copyErr := io.Copy(dst, io.TeeReader(src, hasher))
	closeErr := dst.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(target)
		return nil
	}
	if written != entry.Size || hex.EncodeToString(hasher.Sum(nil)) != entry.Sha256 {
		// 源文件内容在判定后漂移（size 或全量哈希不一致）：放弃拷贝，改由 stageFiles 网络拉取
		_ = os.Remove(target)
		return nil
	}
	return nil
}

// sha256File 计算文件全量 SHA256（hex；读本地活文件作内容身份强校验）
func sha256File(ctx context.Context, absPath string) (string, error) {
	f, err := os.Open(absPath)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// findManifestWork 按 manifest 作品 ID 定位作品记录（子 manifest 通常单作品，通用化供多作品遍历）
func findManifestWork(manifest *export.Manifest, manifestID int64) *export.WorkRecord {
	for i := range manifest.Works {
		if manifest.Works[i].ID == manifestID {
			return &manifest.Works[i]
		}
	}
	return nil
}

// conflictWorkIDsOf 提取冲突作品的本地 ID 集（保序去重；确认决策记忆键，
// 对齐 taskManager.conflictWorkIds 语义——记忆记录与比对共用同一形态）
func conflictWorkIDsOf(conflicts []taskManager.ConflictInfo) []int64 {
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

// manifestWorkRoles 作品在 manifest 中声明的板块角色集合（资源挂载去重并集）：
// 作查重输入的期望板块与软删角色集（零交集命中时对活行 no-op）
func manifestWorkRoles(w *export.WorkRecord) []string {
	seen := make(map[string]struct{})
	var roles []string
	for i := range w.Resources {
		for _, s := range w.Resources[i].Stores {
			if s.StoreType == "" {
				continue
			}
			if _, dup := seen[s.StoreType]; dup {
				continue
			}
			seen[s.StoreType] = struct{}{}
			roles = append(roles, s.StoreType)
		}
	}
	return roles
}

// fileSkipSet 被裁决跳过作品的文件条目 StoreID 集：文件可能被多作品引用，
// 仅当引用它的全部作品都被跳过时才不拉取（共享文件不因单作品跳过而丢失）
func fileSkipSet(manifest *export.Manifest, skipWorks map[int64]struct{}) map[int64]struct{} {
	if len(skipWorks) == 0 {
		return nil
	}
	workIDsByFile := make(map[int64]map[int64]struct{})
	for i := range manifest.Works {
		w := &manifest.Works[i]
		for j := range w.Resources {
			for _, s := range w.Resources[j].Stores {
				set := workIDsByFile[s.StoreID]
				if set == nil {
					set = make(map[int64]struct{})
					workIDsByFile[s.StoreID] = set
				}
				set[w.ID] = struct{}{}
			}
		}
	}
	skip := make(map[int64]struct{})
	for storeID, refs := range workIDsByFile {
		allSkipped := true
		for wid := range refs {
			if _, ok := skipWorks[wid]; !ok {
				allSkipped = false
				break
			}
		}
		if allSkipped {
			skip[storeID] = struct{}{}
		}
	}
	return skip
}

func (e *ReceiveExecution) stageFiles(ctx context.Context, client *receiveClient, staging string,
	manifest *export.Manifest, skipFiles map[int64]struct{}, h taskManager.StrategyHandle) error {
	taskID := h.Task().GetID()
	// 待拉条目判定：缺包内路径/源缺失标记/被裁决跳过作品的文件 一律跳过
	pull := func(entry *export.FileEntry) bool {
		if entry.Path == "" || entry.Missing {
			return false
		}
		_, skip := skipFiles[entry.StoreID]
		return !skip
	}
	// 进度基数：全部待拉条目的声明字节（已完成条目也计入基数，finished 经暂存盘点补齐）
	var total int64
	for i := range manifest.Files {
		entry := &manifest.Files[i]
		if !pull(entry) {
			continue
		}
		total += entry.Size
	}
	var done int64
	report := func() {
		if total > 0 {
			h.ReportProgress(total, done)
			logger.Log.Debugf("[share-recv] 任务 %d 进度推进 total=%d done=%d", taskID, total, done)
		}
	}
	// 初始进度：已暂存字节（重试/恢复任务的即时段位）
	for i := range manifest.Files {
		entry := &manifest.Files[i]
		if !pull(entry) {
			continue
		}
		if sz := stagedSize(stagingPath(staging, entry.Path)); sz > 0 && sz <= entry.Size {
			done += sz
		}
	}
	report()
	for i := range manifest.Files {
		entry := &manifest.Files[i]
		if !pull(entry) {
			continue
		}
		fileStart := time.Now()
		if err := stageFile(ctx, client, staging, entry, func(delta int64) {
			done += delta
			report()
		}); err != nil {
			var ae *streamAppError
			if errors.As(err, &ae) && ae.code == streamErrMissing {
				// 分享方现报缺失：置清单缺席标记，导入按挂载缺席降级（其余照常）
				logger.Log.Debugf("[share-recv] 任务 %d 文件 %s 分享方现报缺失", taskID, entry.Path)
				entry.Missing = true
				continue
			}
			logger.Log.Debugf("[share-recv] 任务 %d 文件 %s 拉取失败 耗时=%s err=%v", taskID, entry.Path, time.Since(fileStart), err)
			return err
		}
		logger.Log.Debugf("[share-recv] 任务 %d 文件 %s 拉取完成 耗时=%s", taskID, entry.Path, time.Since(fileStart))
	}
	report()
	return nil
}

// stagingPath 暂存内绝对路径（包内路径正斜杠 → 平台分隔符；absPath 域，仅 os 调用点使用）
func stagingPath(staging, entryPath string) string {
	return filepath.Join(staging, filepath.FromSlash(entryPath))
}

// stagedSize 暂存文件已落盘字节数（不存在为 0）
func stagedSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// stageFile 拉取单文件至暂存（含瞬态退避重试与断点续传）：
//   - 暂存大小 == 声明大小：跳过（上次已完成）
//   - 0 < 暂存大小 < 声明大小：从该偏移续传（对齐流内协议 file 请求 offset 锚）
//   - 暂存大小异常（> 声明/0 字节残留）：重建（截断重拉）
func stageFile(ctx context.Context, client *receiveClient, staging string, entry *export.FileEntry,
	onProgress func(delta int64)) error {
	target := stagingPath(staging, entry.Path)
	if entry.Size > 0 && stagedSize(target) == entry.Size {
		return nil // 上次已完整拉取
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("创建暂存子目录失败: %w", err)
	}
	var offset int64
	if sz := stagedSize(target); sz > 0 && sz < entry.Size {
		offset = sz
	} else if sz > entry.Size {
		// 残留超过声明（清单变更过的旧暂存）：截断重拉
		if err := os.Truncate(target, 0); err != nil {
			return fmt.Errorf("重置暂存文件失败: %w", err)
		}
	}
	logger.Log.Debugf("[share-recv] 任务 %d 文件 %s 拉取准备 暂存=%d 声明=%d 续传offset=%d", client.taskID, entry.Path, stagedSize(target), entry.Size, offset)
	delay := receiveBackoffStart
	for attempt := 0; ; attempt++ {
		err := appendFetch(ctx, client, entry, target, offset, onProgress)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			logger.Log.Debugf("[share-recv] 任务 %d 文件 %s 拉取被取消(attempt=%d) err=%v", client.taskID, entry.Path, attempt, err)
			return ctx.Err()
		}
		if !isRelayRetryableErr(err) || attempt >= receiveMaxAttempts-1 {
			return err
		}
		logger.Log.Debugf("[share-recv] 任务 %d 文件 %s 拉取瞬态错误重试 attempt=%d err=%v 退避=%s", client.taskID, entry.Path, attempt, err, delay)
		// 重试前按实际落盘字节重算续传锚（appendFetch 失败点即上次落盘点）
		offset = stagedSize(target)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		if delay < receiveBackoffMax {
			delay *= 2
		}
	}
}

// appendFetch 一次 file 请求的拉取落盘：从 offset 起追加写暂存文件，校验应答头声明与
// 请求锚一致、终态字节数与声明一致。
func appendFetch(ctx context.Context, client *receiveClient, entry *export.FileEntry,
	target string, offset int64, onProgress func(delta int64)) error {
	_, err := fetchWithRetry(ctx, client, &streamRequest{Type: "file", Path: entry.Path, Offset: offset},
		func(head *streamHeader, r *streamContentReader) (bool, error) {
			if head.Kind != "file" || head.Offset != offset || head.Size != entry.Size-offset {
				return false, fmt.Errorf("%w：应答头与请求不匹配（offset=%d size=%d，期望 offset=%d size=%d）",
					errStreamTruncated, head.Offset, head.Size, offset, entry.Size-offset)
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
			if err != nil {
				return false, fmt.Errorf("打开暂存文件失败: %w", err)
			}
			defer func() { _ = f.Close() }()
			buf := make([]byte, 32*1024)
			var written int64
			for {
				n, rerr := r.Read(buf)
				if n > 0 {
					if _, werr := f.Write(buf[:n]); werr != nil {
						return false, fmt.Errorf("写暂存文件失败: %w", werr)
					}
					written += int64(n)
					onProgress(int64(n))
				}
				if rerr == io.EOF {
					if written != head.Size {
						return false, fmt.Errorf("%w：实收 %d 字节，声明 %d", errStreamTruncated, written, head.Size)
					}
					return true, nil
				}
				if rerr != nil {
					return false, rerr
				}
			}
		})
	return err
}

// readAllBody 应答体全量读入（manifest 拉取用）
func readAllBody(head *streamHeader, r *streamContentReader) (body []byte, err error) {
	defer func() { _ = r.Close() }()
	if head.Kind != "manifest" {
		return nil, fmt.Errorf("manifest 应答 kind 异常: %s", head.Kind)
	}
	body, err = io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if head.Size > 0 && int64(len(body)) != head.Size {
		return nil, errStreamTruncated
	}
	return body, nil
}

// fetchWithRetry 单请求瞬态退避重试壳：非瞬态错误（中继终态拒绝/流内应用错误）与重试
// 耗尽原样上抛；body 消费中途失败同样按瞬态重试（文件场景由暂存大小自然续传）。
func fetchWithRetry[T any](ctx context.Context, client *receiveClient, req *streamRequest,
	body func(head *streamHeader, r *streamContentReader) (T, error)) (T, error) {
	var zero T
	delay := receiveBackoffStart
	for attempt := 0; ; attempt++ {
		fetchStart := time.Now()
		head, r, err := client.fetch(ctx, req)
		logger.Log.Debugf("[share-recv] task=%d fetch 尝试#%d path=%s 拨号+应答耗%s err=%v", client.taskID, attempt, req.Path, time.Since(fetchStart), err)
		if err == nil {
			res, berr := body(head, r)
			partial := r.got > 0
			_ = r.Close()
			if berr == nil {
				return res, nil
			}
			err = berr
			logger.Log.Debugf("[share-recv] task=%d fetch path=%s 内容消费失败 got=%d err=%v", client.taskID, req.Path, r.got, err)
			// 内容已部分消费后失败：本次尝试已把部分字节写入暂存（O_APPEND 追加），同 offset
			// 重试会叠加重复字节——文件超长损坏且进度双计（实测：传输中中断即现）。交外层
			// 续传锚重算（stageFile 按实际落盘字节重设 offset）处理，不再内层重复追加。
			if partial {
				return zero, err
			}
		}
		if ctx.Err() != nil {
			return zero, ctx.Err()
		}
		if !isRelayRetryableErr(err) || attempt >= receiveMaxAttempts-1 {
			logger.Log.Debugf("[share-recv] task=%d fetch path=%s 判定终态错误 attempt=%d err=%v", client.taskID, req.Path, attempt, err)
			return zero, err
		}
		logger.Log.Debugf("[share-recv] task=%d fetch path=%s 重试退避 attempt=%d delay=%s err=%v", client.taskID, req.Path, attempt, delay, err)
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(delay):
		}
		if delay < receiveBackoffMax {
			delay *= 2
		}
	}
}

// stagedFileSource 构建暂存目录 → 导入文件源（FileSource 的暂存实现）：包内路径做
// 白名单校验（禁绝对路径/反斜杠/穿越段），仅允许读暂存内既有文件。
func stagedFileSource(staging string) importer.FileSource {
	return func(entryPath string) (io.ReadCloser, error) {
		if !safeEntryPath(entryPath) {
			return nil, fmt.Errorf("包内路径不合法: %s", entryPath)
		}
		f, err := os.Open(stagingPath(staging, entryPath))
		if err != nil {
			return nil, fmt.Errorf("%w：%s", importer.ErrPackageFileMissing, entryPath)
		}
		return f, nil
	}
}

// safeEntryPath 包内路径白名单校验：非空、正斜杠相对路径、无穿越段
func safeEntryPath(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, "\\") {
		return false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
	}
	return true
}

// CleanupOrphanReceiveStaging 启动清扫：回收任务行已不存在的收件暂存目录（任务删除后
// 其暂存随之失去归属；成功任务的暂存已在执行尾清理，此处兜底崩溃残留与已删任务残留）。
// 收件任务为父子树形态：父目录 {parentID}/ 含共享 manifest.json，子目录为各子任务文件暂存，
// 三者均为任务 ID 命名的平级子目录——清扫按任务行存在性逐目录独立判定：父行删除回收父目录
// （含 manifest）、子行删除回收子目录，互不影响。exists 由调用方提供任务行存在性查询。
func CleanupOrphanReceiveStaging(workDir string, exists func(id int64) bool) error {
	if workDir == "" {
		return nil
	}
	root := filepath.Join(workDir, receiveStagingRootName)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		id, perr := strconv.ParseInt(ent.Name(), 10, 64)
		if perr != nil || id <= 0 {
			continue
		}
		if !exists(id) {
			if rerr := os.RemoveAll(filepath.Join(root, ent.Name())); rerr != nil {
				return rerr
			}
		}
	}
	return nil
}
