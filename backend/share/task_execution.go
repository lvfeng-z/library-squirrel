package share

// share-receive 任务的执行面策略（taskManager.ExecutionStrategy 实现，按 task_type 注册进
// Manager 策略表，app.go 装配）：
//   - ReceiveExecution：收件人子任务拉取数据流（读本地共享 manifest → 逐文件暂存续传 → ManifestIngestor 回灌导入）
//
// 分享方发布不走任务模块（发布直跑经 Service 内受监督 goroutine 驱动，生命周期落
// share_record——见 service.go/record.go）。

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

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

// ReceiveExecution share-receive（收件人拉取）任务的执行面策略。
type ReceiveExecution struct {
	svc        *Service                   // 提供 workDir / instanceID / 测试可覆写参数
	ingestor   importer.ManifestIngestor  // 回灌导入能力（与 import handler 同一实例，app.go 装配）
	checker    duplicate.DuplicateChecker // 查重判定能力（manifest 作品键 + 板块角色三分类）
	replaceOps resource.ReplaceStoreOps   // 替换链能力（软删替换目标 + 失败回滚复活）
}

// NewReceiveExecution 创建 share-receive 执行面策略
func NewReceiveExecution(svc *Service, ingestor importer.ManifestIngestor,
	checker duplicate.DuplicateChecker, replaceOps resource.ReplaceStoreOps) *ReceiveExecution {
	return &ReceiveExecution{svc: svc, ingestor: ingestor, checker: checker, replaceOps: replaceOps}
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
	client, err := newReceiveClient(payload, e.svc.instanceID, e.svc.opts)
	if err != nil {
		h.Fail(err.Error())
		return
	}
	ctx := h.RunCtx()
	staging := filepath.Join(workDir, receiveStagingRootName, strconv.FormatInt(task.GetID(), 10))
	if err := os.MkdirAll(staging, 0o755); err != nil {
		h.Fail(fmt.Sprintf("创建暂存目录失败: %v", err))
		return
	}

	// 查重 → 确认 → 软删 + 回滚登记（作用域为本作品子集；时序语义同整体路径，见设计七）
	plan, canceled, err := e.planReplace(ctx, sub, h)
	if err != nil {
		reportReceiveError(h, ctx, err)
		return
	}
	if canceled {
		return // 确认被取消（暂停/停止防御性打断）：不上报终态，交控制面接管
	}
	if ctx.Err() != nil {
		return // 软删窗口内暂停/停止：回滚清单已登记（交 setFailed 单点），暂停延续替换
	}

	// 阶段二：逐文件拉取至暂存（只拉本作品引用文件；被裁决跳过作品的文件不拉）
	if err := e.stageFiles(ctx, client, staging, sub, fileSkipSet(sub, plan.skipWorks), h); err != nil {
		reportReceiveError(h, ctx, err)
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
	if _, err := e.ingestor.Ingest(ctx, sub, stagedFileSource(staging), opts); err != nil {
		reportReceiveError(h, ctx, err)
		return
	}
	// 成功：清理本任务暂存（共享 manifest.json 在父任务目录，不动；残留由启动清扫回收）
	_ = os.RemoveAll(staging)
	h.Finish()
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
	h taskManager.StrategyHandle) (*receiveReplacePlan, bool, error) {
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
	results, err := e.checker.Check(ctx, items)
	if err != nil {
		return nil, false, fmt.Errorf("作品查重判定失败: %w", err)
	}

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

	// 冲突作品整体决策（任务粒度）：替换 → 确认替换集；跳过 → 整作品跳过（零交集角色同样不增补）
	if len(conflicts) > 0 {
		decision, canceled := h.WaitReplaceConfirm(conflicts)
		if canceled {
			return nil, true, nil
		}
		switch decision {
		case taskManager.ReplaceDecisionSkip:
			for _, t := range confirmTargets {
				plan.skipWorks[t.manifestID] = struct{}{}
			}
		default:
			for _, t := range confirmTargets {
				plan.confirmedWorks[t.manifestID] = struct{}{}
			}
		}
	}

	// 软删替换全集（确认替换 ∪ 零交集并入）各作品的软删角色并登记回滚清单；
	// 零交集命中作品经此 no-op（活行交集为空），软删后恢复重跑延续同一机制
	softTargets := autoTargets
	if len(plan.confirmedWorks) > 0 {
		softTargets = append(softTargets, confirmTargets...)
	}
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
		if err := stageFile(ctx, client, staging, entry, func(delta int64) {
			done += delta
			report()
		}); err != nil {
			var ae *streamAppError
			if errors.As(err, &ae) && ae.code == streamErrMissing {
				// 分享方现报缺失：置清单缺席标记，导入按挂载缺席降级（其余照常）
				entry.Missing = true
				continue
			}
			return err
		}
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
	delay := receiveBackoffStart
	for attempt := 0; ; attempt++ {
		err := appendFetch(ctx, client, entry, target, offset, onProgress)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !isRelayRetryableErr(err) || attempt >= receiveMaxAttempts-1 {
			return err
		}
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
		head, r, err := client.fetch(ctx, req)
		if err == nil {
			res, berr := body(head, r)
			_ = r.Close()
			if berr == nil {
				return res, nil
			}
			err = berr
		}
		if ctx.Err() != nil {
			return zero, ctx.Err()
		}
		if !isRelayRetryableErr(err) || attempt >= receiveMaxAttempts-1 {
			return zero, err
		}
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
