package download

// 跨重启续传（暂存模式）：按 taskId 派生暂存目录 → 枚举 role_seq 键文件 → os.Stat 得各轨
// 偏移 → 调插件 Resume 取流集合 → 暂存续接/重建后进入下载循环（与全新执行共用提交点）。
// 暂存模式下暂停任务零 DB 足迹（资源行在提交点才建），恢复不读未完成行；任务所属作品按
// 领域复合键 (site, site_work_id) 从 work_task 行定位。暂存为空（升级前旧暂停任务/全新执行）
// 或作品定位失败时降级为完整重新执行（板块组合重走）。

import (
	"errors"
	"fmt"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/plugin/extension"
	"github.com/library-squirrel/backend/settings"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// stagingHasFiles 任务暂存目录是否有轨道文件（执行入口的续传分叉判据：恢复信号 + 暂存
// 非空才走续传；旧模式暂停任务无暂存锚，降级全新重下）
func (sess *execSession) stagingHasFiles() bool {
	workDir := sess.deps.WorkDirProvider.GetWorkDir()
	if workDir == "" {
		return false
	}
	entries, err := enumerateStaging(sess.taskId, sess.deps.StagingPaths.StagingPath(workDir, sess.taskId))
	if err != nil {
		logger.Log.Warnf("[Download] 任务 %d 枚举暂存目录失败: %v", sess.taskId, err)
		return false
	}
	return len(entries) > 0
}

// resumeFromPersistedState 跨重启续传主体：暂存枚举推导各轨偏移，调插件 Resume 续传未完成
// downloaded 轨；Resume 未认领的暂存轨（derived 一次性产物未完成须整轨重产，及插件判定
// 无需续传的轨）经 Start 重产；全部写满后走提交点（与全新执行共用）
func (sess *execSession) resumeFromPersistedState() comboResult {
	defer func() {
		if r := recover(); r != nil {
			logger.Log.Errorf("[Download] 任务 %d resumeFromPersistedState panic: %v", sess.taskId, r)
			sess.handle.Fail(fmt.Sprintf("跨重启续传 panic: %v", r))
		}
	}()

	workDir := sess.deps.WorkDirProvider.GetWorkDir()
	if workDir == "" {
		logger.Log.Errorf("[Download] 任务 %d 失败: 未配置资源库目录", sess.taskId)
		settings.NotifyWorkDirUnconfigured("download")
		sess.failTerminal("未配置资源库目录，请先在设置中指定资源库保存位置")
		return comboFinished
	}

	// 1. 枚举暂存轨道（role_seq 键文件 + 已落盘字节数=续传锚，文件名扩展名段=轨级实际
	// 扩展名的唯一载体）
	entries, err := enumerateStaging(sess.taskId, sess.deps.StagingPaths.StagingPath(workDir, sess.taskId))
	if err != nil {
		logger.Log.Errorf("[Download] 任务 %d 枚举暂存目录失败: %v", sess.taskId, err)
		sess.failTerminal(fmt.Sprintf("枚举下载暂存失败: %v", err))
		return comboFinished
	}
	if len(entries) == 0 {
		logger.Log.Warnf("[Download] 任务 %d 暂存目录无轨道文件，降级为完整重新执行", sess.taskId)
		return sess.runSectionCombo()
	}

	// 2. 定位任务所属作品（暂存模式暂停任务无 pending/资源行，按领域复合键回填 workId；
	// 供提交点 find-or-create 与命名元数据加载）。作品已删除等定位失败降级完整重新执行
	if sess.deps.WorkLocator == nil || !sess.workTask.SiteID.Valid || !sess.workTask.SiteWorkID.Valid || sess.workTask.SiteWorkID.String == "" {
		logger.Log.Warnf("[Download] 任务 %d 缺少站点复合键，降级为完整重新执行", sess.taskId)
		return sess.runSectionCombo()
	}
	work, err := sess.deps.WorkLocator.GetBySiteAndSiteWorkID(sess.runCtx(), sess.workTask.SiteID.Int64, sess.workTask.SiteWorkID.String)
	if err != nil || work == nil {
		logger.Log.Warnf("[Download] 任务 %d 定位所属作品失败(site=%d siteWorkId=%s): %v，降级为完整重新执行",
			sess.taskId, sess.workTask.SiteID.Int64, sess.workTask.SiteWorkID.String, err)
		return sess.runSectionCombo()
	}
	sess.workId = work.GetID()
	// 已有作品上的资源重执行即替换：暂停期间软删尚未发生（软删在提交窗口），恢复会话的
	// 提交点须补位软删——旧 store 让位与回滚登记与全新执行同构
	sess.isReplace = true

	// 3. 调用插件 Resume：全部暂存轨按当前已落盘偏移下发（含已写满轨——多轨并发下小轨先
	// 写满、等待其余轨道写满一起提交是常态），身份化 role+store_seq。单轨完成状态的确认是
	// 插件兼容职责（站点对越界 Range 的响应形态各异，本地偏移与来源现值的比对只有插件能做）：
	// 满轨由插件以立即 EOF 流（Size=偏移，主程序续传打开即写满完成）或空过（主程序经 Start
	// 重产）表达；来源现值与偏移不符时插件返回以现值为 Size 的完整流，主程序按声明大小截断
	// 整轨重下
	streamOffsets := make([]*sdkdto.StoreResumeOffset, 0, len(entries))
	for _, ent := range entries {
		streamOffsets = append(streamOffsets, &sdkdto.StoreResumeOffset{
			Role: ent.role, StoreSeq: int32(ent.seq), Offset: ent.size,
		})
	}
	param := &sdkdto.TaskResumeParam{
		Task:          dto.AssembleTaskDTO(sess.task, sess.workTask, nil),
		StreamOffsets: streamOffsets,
	}
	specs, _, err := sess.pluginExec.Resume(sess.runCtx(), param)
	if err != nil {
		// Pause 在 Resume 进行中取消 ctx(stream ctx 继承任务 ctx):视为暂停,不置失败
		if sess.runAborted() {
			logger.Log.Infof("[Download] 任务 %d Resume 被暂停打断(RPC 已取消): %v", sess.taskId, err)
			return comboInterrupted
		}
		logger.Log.Errorf("[Download] 任务 %d 跨重启 Resume 失败: %v", sess.taskId, err)
		// 插件已停用/崩溃时执行器路由不可达：翻译为可操作的引导文案，其余保持泛化文案
		msg := fmt.Sprintf("跨重启续传失败: %v", err)
		if errors.Is(err, extension.ErrExtensionNotFound) {
			msg = "插件已停止运行，请确认插件已启用后重试"
		}
		sess.failTerminal(msg)
		return comboFinished
	}

	// 4. Resume 认领配对：返回 spec 按角色从暂存轨队列消费全局 seq（同 role 多轨按序对齐，
	// 与下发的同 role 偏移顺序一致）。未被认领的暂存轨交 Start 重产（derived 一次性产物
	// 未完成须整轨重产；插件对无需续传的 downloaded 轨经重产覆盖）
	seqBySpec, uncovered := pairResumeSpecs(entries, specs)
	// 续接偏移表仅收 Resume 认领的轨：未认领轨（含经 Start 重产的 downloaded 轨）全新写，
	// 旧暂存前缀截断丢弃——新旧内容来源不同，续接会产生拼接产物
	uncoveredSet := make(map[storeIdentity]struct{}, len(uncovered))
	for _, ent := range uncovered {
		uncoveredSet[storeIdentity{role: ent.role, seq: ent.seq}] = struct{}{}
	}
	staged := make(map[storeIdentity]int64, len(entries))
	for _, ent := range entries {
		if _, ok := uncoveredSet[storeIdentity{role: ent.role, seq: ent.seq}]; ok {
			continue
		}
		staged[storeIdentity{role: ent.role, seq: ent.seq}] = ent.size
	}
	var regenSpecs []*sdkdto.StoreSpec
	if len(uncovered) > 0 {
		regenRoles := uniqueStagingRoles(uncovered)
		rs, _, startErr := sess.pluginExec.Start(sess.runCtx(), sess.task, sess.workTask, regenRoles)
		if startErr != nil {
			logger.Log.Errorf("[Download] 任务 %d 重产资源轨 %v 失败: %v", sess.taskId, regenRoles, startErr)
			// Pause 在重产进行中取消 ctx:视为暂停,不置失败
			if sess.runAborted() {
				return comboInterrupted
			}
			sess.failTerminal(fmt.Sprintf("重产资源失败: %v", startErr))
			return comboFinished
		}
		reSeq := pairRegenSpecs(uncovered, rs)
		for spec, seq := range reSeq {
			seqBySpec[spec] = seq
		}
		regenSpecs = rs
	}
	specs = append(specs, regenSpecs...)

	if len(specs) == 0 {
		// 暂存非空但插件对全部轨道既不续传也不重产：暂存产物无提交元数据，显式失败保留
		// 暂存（重试/重新执行可覆盖），不静默丢失
		logger.Log.Errorf("[Download] 任务 %d 恢复时插件未认领任何暂存轨道(offsets=%v)", sess.taskId, streamOffsets)
		sess.failTerminal("插件未返回待续传资源，暂存已保留，请重试或重新执行任务")
		return comboFinished
	}

	// 5. 规划+打开暂存写入器（认领的 downloaded 轨按偏移续接；重产/derived 轨全新写），
	// 进入下载循环，全部写满后走提交点。落盘目录按站点复合键身份派生（与全新执行同源）
	baseRelPath, err := sess.resolveStoreDir(sess.runCtx())
	if err != nil {
		logger.Log.Errorf("[Download] 任务 %d 解析落盘目录失败: %v", sess.taskId, err)
		sess.failTerminal(fmt.Sprintf("解析落盘目录失败: %v", err))
		return comboFinished
	}
	streams, err := sess.openStagingTracks(specs, baseRelPath, staged, seqBySpec)
	if err != nil {
		logger.Log.Errorf("[Download] 任务 %d 恢复打开暂存失败: %v", sess.taskId, err)
		if sess.runAborted() {
			return comboInterrupted
		}
		sess.failTerminal(fmt.Sprintf("跨重启续传创建存储失败: %v", err))
		return comboFinished
	}
	if sess.runAborted() {
		for _, s := range streams {
			s.closeWriter()
			if s.reader != nil {
				s.reader.Close()
			}
		}
		return comboInterrupted
	}

	sess.streams = streams
	switch sess.downloadLoop() {
	case loopDone:
		if sess.allStreamsCompleted() {
			return sess.commitAndFinish()
		}
		return comboFinished
	default:
		return comboInterrupted
	}
}

// pairResumeSpecs Resume 认领配对：返回 spec 按角色消费暂存轨队列的 (role,seq) 全局序，
// 未被认领的暂存轨按序返回（供重产）。同 role 多轨按暂存枚举序与 spec 返回序对齐
// （依赖插件按下发顺序返回 specs 的契约）；某 role 的 spec 数超出暂存轨数时按该 role
// 现有最大序递增分配（插件在恢复轮新增轨道的场景，首个新轨从 0 起）。
// 认领轨的扩展名以暂存文件名所载的创建期声明为准——恢复轮当次响应的 Format 不复现创建期
// 声明（如 416 转译空 Header 得空值），按当次响应派生暂存名会脱离磁盘上的既有文件；未经
// 认领的 spec（新增轨）保持当次声明（创建行为，声明本来就是新的）
func pairResumeSpecs(entries []stagingEntry, specs []*sdkdto.StoreSpec) (map[*sdkdto.StoreSpec]int, []stagingEntry) {
	queues, maxSeq := stagingRoleQueues(entries)
	claimed := make(map[storeIdentity]struct{}, len(specs))
	out := make(map[*sdkdto.StoreSpec]int, len(specs))
	for _, spec := range specs {
		if spec == nil {
			continue
		}
		if q := queues[spec.Role]; len(q) > 0 {
			ent := q[0]
			queues[spec.Role] = q[1:]
			out[spec] = ent.seq
			spec.Format = ent.ext
			claimed[storeIdentity{role: spec.Role, seq: ent.seq}] = struct{}{}
		} else {
			out[spec] = nextSeqBeyond(maxSeq, spec.Role)
		}
	}
	var uncovered []stagingEntry
	for _, ent := range entries {
		if _, ok := claimed[storeIdentity{role: ent.role, seq: ent.seq}]; !ok {
			uncovered = append(uncovered, ent)
		}
	}
	return out, uncovered
}

// pairRegenSpecs 重产 spec 配对：重产轨从未认领暂存队列按角色消费序号（同 role 多轨
// 按序对齐）；超出队列的按最大序递增（该 role 无未认领轨时从 0 起）。重产为整轨全新写，
// 扩展名保持当次声明（旧暂存文件成为残留，随暂存目录在提交点一并回收）
func pairRegenSpecs(uncovered []stagingEntry, specs []*sdkdto.StoreSpec) map[*sdkdto.StoreSpec]int {
	queues, maxSeq := stagingRoleQueues(uncovered)
	out := make(map[*sdkdto.StoreSpec]int, len(specs))
	for _, spec := range specs {
		if spec == nil {
			continue
		}
		if q := queues[spec.Role]; len(q) > 0 {
			out[spec] = q[0].seq
			queues[spec.Role] = q[1:]
		} else {
			out[spec] = nextSeqBeyond(maxSeq, spec.Role)
		}
	}
	return out
}

// nextSeqBeyond 按 role 已有最大序递增取下一序号；该 role 尚无任何暂存轨时从 0 起
func nextSeqBeyond(maxSeq map[string]int, role string) int {
	if _, ok := maxSeq[role]; ok {
		maxSeq[role]++
	} else {
		maxSeq[role] = 0
	}
	return maxSeq[role]
}

// stagingRoleQueues 暂存条目按角色组建消费队列（条目已按 (role,seq) 有序），并返回各 role
// 的最大序号（超出队列时递增分配用）
func stagingRoleQueues(entries []stagingEntry) (queues map[string][]stagingEntry, maxSeq map[string]int) {
	queues = make(map[string][]stagingEntry, len(entries))
	maxSeq = make(map[string]int, len(entries))
	for _, ent := range entries {
		queues[ent.role] = append(queues[ent.role], ent)
		if ent.seq > maxSeq[ent.role] {
			maxSeq[ent.role] = ent.seq
		}
	}
	return
}

// uniqueStagingRoles 提取暂存条目去重后的 role 列表（保持出现顺序）
func uniqueStagingRoles(entries []stagingEntry) []string {
	seen := make(map[string]struct{}, len(entries))
	roles := make([]string, 0, len(entries))
	for _, ent := range entries {
		if _, ok := seen[ent.role]; ok {
			continue
		}
		seen[ent.role] = struct{}{}
		roles = append(roles, ent.role)
	}
	return roles
}
