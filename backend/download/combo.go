package download

// 板块组合执行：runMode 三态派生（板块模式唯一源=作品任务领域行）+ 板块组合主体执行 +
// 查重编排（命中冲突经控制面 WaitReplaceConfirm 挂起等待用户整体答复）。查重决策记忆的
// 匹配消费在本侧（冲突作品集合一致才复用决策，跨暂停/恢复不重复弹窗）。

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"strings"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/duplicate"
	"github.com/library-squirrel/backend/settings"
	"github.com/library-squirrel/backend/taskManager"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// storeScopeKind 资源板块选择三态
type storeScopeKind int

const (
	// scopeNone 仅作品信息板块:不拉取资源、不产生任务终态
	scopeNone storeScopeKind = iota
	// scopeAll 全量板块:下载范围取插件 universe(空 universe=插件自决),替换语义覆盖作品全部活行 store
	scopeAll
	// scopeSelected 用户显式指定的板块子集
	scopeSelected
)

// storeScope 资源板块选择(storeRoles+fetchStores 组合两义的显式三态封装)。
// roles 的解释由 kind 决定:Selected 携带用户子集(非空)、All 携带插件 universe(可空,空=插件自决全量)、None 恒空
type storeScope struct {
	kind  storeScopeKind
	roles []string
}

// coversStores 是否拉取资源板块(None=false,All/Selected=true)
func (s storeScope) coversStores() bool { return s.kind != scopeNone }

// runMode 板块执行选择:workInfo 为作品元数据独立板块,storeScope 为资源板块三态选择
type runMode struct {
	workInfo   bool       // 作品元数据板块
	storeScope storeScope // 资源板块选择(三态)
}

func (m runMode) hasWorkInfo() bool { return m.workInfo }

// runModeFromTask 从作品任务领域行持久化字段派生 runMode(三态产出)
// StoreRoles NULL(Start/首次执行,含默认插件)→All,roles 取 universe(空 universe=插件自决全量)
// StoreRoles Valid(重下载已记录)→空串=None(仅作品信息),非空=Selected(用户子集)
// workInfo 统一取 IncludeWorkInfo 字段(首跑由任务启动链路记录为 true)
func runModeFromTask(wt *entity.WorkTask) runMode {
	if !wt.StoreRoles.Valid {
		return runMode{workInfo: wt.IncludeWorkInfo, storeScope: storeScope{kind: scopeAll, roles: parseStoreRoles(wt.InvolvedRoles)}}
	}
	sel := parseStoreRoles(wt.StoreRoles)
	if len(sel) == 0 {
		return runMode{workInfo: wt.IncludeWorkInfo, storeScope: storeScope{kind: scopeNone}}
	}
	return runMode{workInfo: wt.IncludeWorkInfo, storeScope: storeScope{kind: scopeSelected, roles: sel}}
}

// parseStoreRoles 解析逗号分隔的 store_type 字符串为切片
func parseStoreRoles(s sql.NullString) []string {
	if !s.Valid || s.String == "" {
		return nil
	}
	parts := strings.Split(s.String, ",")
	roles := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			roles = append(roles, p)
		}
	}
	return roles
}

// comboResult 板块组合执行的收口分类
type comboResult int

const (
	comboFinished    comboResult = iota // 收口完成（成功/失败/跳过经 handle 上报）
	comboInterrupted                    // 中断未收口（暂停/停止/确认等待取消），交控制面接管
)

// runSectionCombo 板块组合执行:严格按 runMode 所选板块执行。
// 含任一资源板块(含全集)时先走查重编排(命中冲突挂起等待用户整体答复)并产生终态;
// 仅 workInfo 为非终态(跳过收口:回执行前状态、不产生终态)
func (sess *execSession) runSectionCombo() comboResult {
	// workdir 检查（资源板块需要，置于查重前避免无效确认）；领域文案保留，同时经统一发射口通知前端
	if sess.mode.storeScope.coversStores() && sess.deps.WorkDirProvider.GetWorkDir() == "" {
		logger.Log.Errorf("[Download] 任务 %d 失败: 未配置资源库目录", sess.taskId)
		settings.NotifyWorkDirUnconfigured("download")
		return sess.comboFail("未配置资源库目录，请先在设置中指定资源库保存位置")
	}

	// 含资源板块：查重（命中冲突经控制面挂起等待用户整体答复，跳过判定=确认替换后的续行）
	skipCheck, interr := sess.checkDuplicate()
	if interr {
		return comboInterrupted
	}
	if skipCheck {
		return comboFinished
	}

	// workId 定位 + 替换判定
	// 含资源板块在已有作品上重执行都视为替换,需备份旧 store
	if sess.mode.storeScope.coversStores() {
		// 查重命中(existingWorkId>0，用户答复替换或无冲突命中)
		if sess.existingWorkId > 0 {
			sess.workId = sess.existingWorkId
			sess.existingWorkId = 0
			sess.isReplace = true
		} else if !sess.mode.hasWorkInfo() {
			// 含资源板块的重执行必须定位到已有作品(否则无处挂载资源)
			logger.Log.Errorf("[Download] 任务 %d 资源重执行未定位到作品", sess.taskId)
			return sess.comboFail("未找到任务对应的作品，无法重新下载资源")
		}
		// 含 workInfo 且查重未命中：workInfo 板块的 SaveWorkInfo 会提供 workId(新作品,非替换)
	}

	// 板块 A：作品信息（CreateWorkInfo + SaveWorkInfo，提供 workId 与文件名模板数据）
	var workResp *sdkdto.WorkResponse
	if sess.mode.hasWorkInfo() {
		var err error
		workResp, err = sess.pluginExec.CreateWorkInfo(sess.runCtx(), sess.task, sess.workTask)
		if err != nil {
			logger.Log.Errorf("[Download] 任务 %d CreateWorkInfo 失败: %v", sess.taskId, err)
			return sess.comboFail(fmt.Sprintf("创建作品信息失败: %v", err))
		}
		savedWorkId, err := sess.deps.WorkInfoSaver.SaveWorkInfo(sess.runCtx(), sess.task, sess.workTask, workResp)
		if err != nil {
			logger.Log.Errorf("[Download] 任务 %d 保存作品信息失败: %v", sess.taskId, err)
			return sess.comboFail(fmt.Sprintf("保存作品信息失败: %v", err))
		}
		sess.workId = savedWorkId
	}

	// 资源板块:coversStores 时 Start 按 scope 携带角色选择性产出(All 空 universe=插件自决全量)
	if sess.mode.storeScope.coversStores() {
		specs, startResp, err := sess.pluginExec.Start(sess.runCtx(), sess.task, sess.workTask, sess.mode.storeScope.roles)
		if err != nil {
			logger.Log.Errorf("[Download] 任务 %d Start 失败: %v", sess.taskId, err)
			return sess.comboFail(fmt.Sprintf("获取资源流集合失败: %v", err))
		}
		// 合并作品元数据到 startResp(供文件名模板使用):
		// 本次跑了作品元数据板块(A)则用其结果;否则(资源板块单独重下)从已有作品加载命名元数据
		sess.mergeWorkMetaForNaming(startResp, workResp)
		selected := sess.filterSpecsByRoles(specs)
		// 防御性排空未选资源角色的 reader:正常情况下插件已按 storeRoles 只产出所选 role(selected==specs,无操作);
		// 若插件多产出了未选 role,其 io.Pipe 无消费者会永久阻塞 demux(多流复用一条 gRPC stream),此处兜底排空
		drainUnselectedReaders(specs, selected)
		if len(selected) == 0 {
			logger.Log.Errorf("[Download] 任务 %d 插件未产出所选资源角色: %v", sess.taskId, sess.mode.storeScope.roles)
			return sess.comboFail("插件未产出所选资源类型")
		}
		return sess.startDownload(selected, startResp)
	}

	// 无资源板块(纯 workInfo):非终态收口——回执行前状态、不产生终态
	sess.handle.Skip("")
	return comboFinished
}

// checkDuplicate 查重编排：含资源板块且未跳过查重时判定重复。命中冲突先匹配确认决策记忆
// （冲突作品集合一致才复用决策，跨暂停/恢复不重复弹窗），未命中记忆则经控制面挂起等待
// 用户整体答复。返回值：skipTask=跳过收口已完成；interrupted=确认等待被取消或执行已中断，
// 调用方交控制面接管。命中无冲突保留已有作品 ID 供替换定位；未命中走新建路径
func (sess *execSession) checkDuplicate() (skipTask bool, interrupted bool) {
	if !sess.mode.storeScope.coversStores() || sess.skipDuplicateCheck || sess.deps.DuplicateChecker == nil ||
		!sess.workTask.SiteID.Valid || !sess.workTask.SiteWorkID.Valid || sess.workTask.SiteWorkID.String == "" {
		return false, false
	}
	// 查重输入键形态统一：插件任务侧把 task.SiteID 反查站点键（一次查询），
	// 与分享收件/压缩包导入的 manifest 域键（站点键）对齐
	siteKey, ok := sess.resolveSiteKey(sess.runCtx(), sess.workTask.SiteID.Int64)
	if !ok {
		return false, false
	}
	results, err := sess.deps.DuplicateChecker.Check(sess.runCtx(), []duplicate.DuplicateCheckItem{{
		SiteKey: siteKey, SiteWorkID: sess.workTask.SiteWorkID.String, Roles: sess.mode.storeScope.roles,
	}})
	if err != nil || len(results) != 1 {
		return false, false
	}
	res := results[0]
	switch res.Class {
	case duplicate.DuplicateHitNoConflict:
		// 零交集/零行:无覆盖对象,不弹窗,保留已有作品 ID 供替换定位
		sess.existingWorkId = res.WorkID
		return false, false
	case duplicate.DuplicateHitConflict:
	default:
		// 未命中：无处理，走新建路径
		return false, false
	}

	// 行级冲突：先查确认决策记忆命中（冲突本地作品 ID 集一致）复用决策不弹窗；
	// 未命中经控制面挂起等待整体答复（记忆由控制面在答复到达时记录，取消返回时供恢复复用）
	conflicts := []taskManager.ConflictInfo{{
		WorkID:        res.WorkID,
		WorkName:      res.WorkName,
		ConflictRoles: res.ConflictRoles,
	}}
	ids := []int64{res.WorkID}
	var decision taskManager.ReplaceDecision
	if memo := sess.handle.ConfirmMemo(); memo != nil && sameConflictIDSet(memo.ConflictWorkIds, ids) {
		decision = memo.Decision
		logger.Log.Debugf("[Download] 任务 %d 确认决策记忆命中，复用 decision=%d", sess.taskId, memo.Decision)
	} else {
		logger.Log.Infof("[Download] 任务 %d 挂起等待替换确认 已有作品=%d", sess.taskId, res.WorkID)
		var canceled bool
		decision, canceled = sess.handle.WaitReplaceConfirm(conflicts)
		if canceled {
			return false, true
		}
	}
	if decision == taskManager.ReplaceDecisionSkip {
		// 跳过：未实际执行，回执行前状态、不产生终态
		sess.handle.Skip("")
		return true, false
	}
	// 替换：保留已有作品 ID 供替换定位，继续板块组合
	sess.existingWorkId = res.WorkID
	return false, false
}

// sameConflictIDSet 冲突作品 ID 集合相等比对（顺序无关）：确认决策记忆键与当前冲突集
// 一致才复用决策；两次执行的冲突集查重顺序理论可漂移，集合比对消除顺序依赖
func sameConflictIDSet(memoIDs, currentIDs []int64) bool {
	if len(memoIDs) != len(currentIDs) {
		return false
	}
	seen := make(map[int64]struct{}, len(memoIDs))
	for _, id := range memoIDs {
		seen[id] = struct{}{}
	}
	for _, id := range currentIDs {
		if _, ok := seen[id]; !ok {
			return false
		}
	}
	return true
}

// comboFail 组合执行失败处理。
// 执行已中断（暂停/停止）则不收口交控制面；含资源板块为终态失败（先关闭流写入句柄释放
// 文件锁——提交窗口软删的受害者复活由控制面 setFailed 单点按登记清单同步执行，下载窗口
// 失败则旧 store 未动、无回滚需求）；无资源板块为非终态收口（跳过上报携带错误说明：
// 回执行前状态、推送错误通知、不产生终态）
func (sess *execSession) comboFail(errMsg string) comboResult {
	if sess.runAborted() {
		return comboInterrupted
	}
	if sess.mode.storeScope.coversStores() {
		sess.closeStreamWriters()
		sess.failTerminal(errMsg)
		return comboFinished
	}
	sess.handle.Skip(errMsg)
	return comboFinished
}

// resolveSiteKey 站点 ID → 站点键（查重输入键形态统一：把任务领域行的站点 ID 反查站点键）。
// 站点行缺失/查询失败返回 ok=false，调用方按未命中处理（站点不存在则无作品可引用它）
func (sess *execSession) resolveSiteKey(ctx context.Context, siteId int64) (string, bool) {
	if sess.deps.SiteKeyResolver == nil {
		return "", false
	}
	sites, err := sess.deps.SiteKeyResolver.ListByIds(ctx, []int64{siteId})
	if err != nil || len(sites) == 0 || sites[0].SiteKey == "" {
		return "", false
	}
	return sites[0].SiteKey, true
}

// replaceSoftDeleteRoles 替换软删与失败回滚的生效角色集,从 storeScope 三态派生:
// Selected 用用户子集;All 用 store_type 封闭枚举全集(=作品全部活行);None 不涉及资源板块、
// 无软删对象。resource 替换能力只认显式角色集合(空集=不软删任何行),「空=全量」的展开归发起方
func (sess *execSession) replaceSoftDeleteRoles() []string {
	switch sess.mode.storeScope.kind {
	case scopeSelected:
		return sess.mode.storeScope.roles
	case scopeAll:
		return entity.AllStoreTypes()
	default:
		return nil
	}
}

// filterSpecsByRoles 按 storeScope 携带的角色集过滤 spec(All 空 universe=插件自决,不过滤;
// Selected 按 user 子集过滤;None 不到达——调用点在 coversStores 门槛内)
func (sess *execSession) filterSpecsByRoles(specs []*sdkdto.StoreSpec) []*sdkdto.StoreSpec {
	if len(sess.mode.storeScope.roles) == 0 {
		out := make([]*sdkdto.StoreSpec, len(specs))
		copy(out, specs)
		return out
	}
	roleSet := make(map[string]struct{}, len(sess.mode.storeScope.roles))
	for _, r := range sess.mode.storeScope.roles {
		roleSet[r] = struct{}{}
	}
	var out []*sdkdto.StoreSpec
	for _, s := range specs {
		if s == nil {
			continue
		}
		if _, ok := roleSet[s.Role]; ok {
			out = append(out, s)
		}
	}
	return out
}

// drainUnselectedReaders 排空 all 中未被 selected 选中的 spec reader(读到 EOF 后关闭)
// 用于过滤后丢弃的 role:多流 gRPC demux 会向其 io.Pipe 写数据,无人消费会永久阻塞 demux
func drainUnselectedReaders(all, selected []*sdkdto.StoreSpec) {
	if len(all) == len(selected) {
		return
	}
	selectedSet := make(map[*sdkdto.StoreSpec]struct{}, len(selected))
	for _, s := range selected {
		selectedSet[s] = struct{}{}
	}
	for _, sp := range all {
		if sp == nil || sp.ReadCloser == nil {
			continue
		}
		if _, ok := selectedSet[sp]; ok {
			continue
		}
		reader := sp.ReadCloser
		go func() {
			_, _ = io.Copy(io.Discard, reader)
			_ = reader.Close()
		}()
	}
}
