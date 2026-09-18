package download

// 入库编排（暂存模式）：执行前规划（最终路径解析+暂存写入器打开）与提交点
// （替换软删 → 暂存 rename 进 store/ → 单事务建行挂载 + 抑制登记与失败补偿逆操作）。
// 替换链坍缩到提交窗口——长下载全程零 DB 副作用，失败补偿为序列内同步逆操作。

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/storeRegistry"
	"github.com/library-squirrel/backend/taskManager"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// softDeleteReplacedStores 提交窗口首步（替换场景）：软删作品资源下所选角色的活行 store
// （委派 resource 替换能力，显式角色集合语义），完成后把被软删行清单登记进控制面——提交
// 序列此后失败时，Fail 上报经 setFailed 单点按清单复活受害者（同一调用栈内同步完成，
// 不跨会话）；提交成功则 Finish 清空登记（软删行进入被替换终态）。软删中途失败的已软删
// 部分清单同样登记（复活覆盖部分软删行）。任一被重执行的板块，只要该类型 store 在已有
// 作品上存在就软删；已完成行移文件入 backup 并写行内 backup_id，未完成行废弃文件，历史
// 残留死行不动；软删行入回收站文件条目由 TTL 收尾，resource_store 关联保留（复活即挂载回位）。
// 长下载期间不软删——失败/暂停/停止于下载窗口时旧 store 一动不动，无回滚需求
func (sess *execSession) softDeleteReplacedStores() error {
	if sess.deps.ReplaceStoreOps == nil {
		return fmt.Errorf("替换链能力未注入")
	}
	// 提交窗口不随执行 ctx 中断：序列一旦进入须走完（成功提交或同步补偿收口），
	// 软删/建行不受暂停/停止打断留下半提交态
	victims, err := sess.deps.ReplaceStoreOps.SoftDeleteWorkStoreRoles(context.Background(), sess.workId, sess.replaceSoftDeleteRoles())
	if len(victims) > 0 {
		sess.handle.SetTerminalRollback(taskManager.TerminalRollback{Victims: victims})
	}
	if err != nil {
		return fmt.Errorf("替换软删旧资源失败: %w", err)
	}
	return nil
}

// openStagingTracks 执行前规划：为全部 specs 解析最终路径、打开暂存写入器。
// 命名解析时点在 Start/Resume 返回后（specs 已具名）——最终名前置解析，document lazy 轨
// （size<=0）同样在此解析（其 spec 已有 role+seq 与命名身份）；暂存文件名按 role_seq 键，
// 与最终名解耦。stagedOffsets 为各轨预置的（role,seq）→已落盘偏移（全新执行为空 map，
// 续传恢复传入暂存枚举结果——非零偏移轨按续传打开并前缀入哈希）
func (sess *execSession) openStagingTracks(specs []*sdkdto.StoreSpec, baseRelPath string,
	stagedOffsets map[storeIdentity]int64, specSeq map[*sdkdto.StoreSpec]int) ([]*streamController, error) {
	workDir := sess.deps.WorkDirProvider.GetWorkDir()
	stagingDir, err := sess.deps.StagingPaths.EnsureStagingScope(sess.runCtx(), workDir, sess.taskId)
	if err != nil {
		return nil, fmt.Errorf("创建暂存作用域失败: %w", err)
	}

	streams := make([]*streamController, 0, len(specs))
	for _, spec := range specs {
		seq := specSeq[spec]
		relPath, fileName, err := resolveStorePath(spec, baseRelPath, seq)
		if err != nil {
			return nil, err
		}
		stagingName := sess.deps.StagingPaths.StagingFileName(spec.Role, seq, normalizeExt(spec.Format))
		stagingAbs := filepath.Join(stagingDir, stagingName)
		expected := ""
		if spec.ExpectedSha256 != nil {
			expected = *spec.ExpectedSha256
		}
		var writer *stagingWriter
		if staged, ok := stagedOffsets[storeIdentity{role: spec.Role, seq: seq}]; ok && staged > 0 &&
			spec.Generation == entity.GenerationDownloaded {
			// 续传打开：插件指定写入偏移优先（插件对续传位置有确切认知），否则用暂存已落盘大小
			writeOffset := staged
			if spec.ResumeWriteOffset != nil && *spec.ResumeWriteOffset >= 0 {
				writeOffset = *spec.ResumeWriteOffset
			}
			// 暂存残留超过声明大小（清单/内容变更过的旧暂存）：截断重下（share 暂存同构处置）
			if spec.Size > 0 && writeOffset > spec.Size {
				writeOffset = 0
			}
			writer, err = newStagingWriterResume(stagingAbs, writeOffset, expected)
			if err == nil {
				sc := newStreamController(spec, seq, writer, stagingAbs, relPath, fileName)
				sc.written = writeOffset
				sc.initialOffset = writeOffset
				logger.Log.Infof("[StagingMount] taskId=%d role=%s seq=%d mode=resume writeOffset=%d staged=%d",
					sess.taskId, spec.Role, seq, writeOffset, staged)
				streams = append(streams, sc)
				continue
			}
		}
		writer, err = newStagingWriterFresh(stagingAbs, expected)
		if err != nil {
			return nil, err
		}
		logger.Log.Infof("[StagingMount] taskId=%d role=%s seq=%d mode=fresh", sess.taskId, spec.Role, seq)
		streams = append(streams, newStreamController(spec, seq, writer, stagingAbs, relPath, fileName))
	}

	return streams, nil
}

// startDownload 为每个 spec 打开暂存写入器、进入多流下载循环，全部写满后执行提交点
func (sess *execSession) startDownload(specs []*sdkdto.StoreSpec) comboResult {
	// 解析作品落盘目录（站点复合键身份派生，详见 resolveStoreDir）
	baseRelPath, err := sess.resolveStoreDir(sess.runCtx())
	if err != nil {
		logger.Log.Errorf("[Download] 任务 %d 解析落盘目录失败: %v", sess.taskId, err)
		if sess.runAborted() {
			return comboInterrupted
		}
		return sess.comboFail(fmt.Sprintf("解析落盘目录失败: %v", err))
	}
	// 同 role 内 seq 按 specs 顺序分配（Start 全量返回，specs 内重计即全局序）
	roleCounters := make(map[string]int, len(specs))
	specSeq := make(map[*sdkdto.StoreSpec]int, len(specs))
	for _, spec := range specs {
		specSeq[spec] = roleCounters[spec.Role]
		roleCounters[spec.Role]++
	}

	streams, err := sess.openStagingTracks(specs, baseRelPath, nil, specSeq)
	if err != nil {
		logger.Log.Errorf("[Download] 任务 %d 打开暂存失败: %v", sess.taskId, err)
		if sess.runAborted() {
			return comboInterrupted
		}
		return sess.comboFail(fmt.Sprintf("创建暂存失败: %v", err))
	}

	// setup 阶段暂停在暂存打开后命中:关闭句柄返回暂停,避免带着已取消的 ctx 进入 downloadLoop
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
		// 失败终态已在循环内上报；成功转提交点（提交点内部完成收口）
		if sess.allStreamsCompleted() {
			return sess.commitAndFinish()
		}
		return comboFinished
	default:
		return comboInterrupted
	}
}

// allStreamsCompleted 全部流写满收尾（提交点的前置判定；失败路径的流为 failed 态）
func (sess *execSession) allStreamsCompleted() bool {
	for _, s := range sess.streams {
		if streamState(s.state.Load()) != streamCompleted {
			return false
		}
	}
	return true
}

// commitAndFinish 提交点+成功收口：全部暂存写满后单点执行（详见 commitStaged），随后重算
// 资源完整度、上报成功终态（成功即软删受害者进入被替换终态，Finish 清回滚登记）。
// 提交失败按任务失败收口：暂存已由序列内补偿回退，软删受害者经 Fail 上报由控制面
// setFailed 单点复活（同步触发，与失败收口同一调用栈完成）
func (sess *execSession) commitAndFinish() comboResult {
	if err := sess.commitStaged(); err != nil {
		logger.Log.Errorf("[Download] 任务 %d 提交资源失败: %v", sess.taskId, err)
		return sess.comboFail(fmt.Sprintf("提交资源失败: %v", err))
	}
	sess.markResourceComplete(sess.runCtx(), sess.currentResourceId)
	sess.handle.Finish()
	return comboFinished
}

// commitStaged 提交点序列：替换软删（首步，腾空 rename 目标并登记回滚清单）→ 暂存 rename
// 到最终路径（同卷原子；替换场景目标已无活文件——软删分流保证，已完成行移入 backup、
// 未完成行废弃；非替换场景同键活行不存在——查重确认环保证）→ 单事务建 persistent_store 行
// （必然完整，completed_at 即时置位+宽高/头指纹/哈希双列）+ resource_store 挂载 + Resource
// Save（find-or-create）。rename 是 store/ 白名单内文件操作，download 为操作
// 属主须登记抑制（rename 前登记、事务提交后统一 Release）——否则
// rename→建行窗口内 fsmonitor 收 Create 事件查无 DB 行，落入误裁决。
// 失败补偿=序列内逆操作（已 rename 轨逆 rename 回退暂存；软删受害者复活经 Fail 上报由
// setFailed 单点同步触发；建行段由事务原子性兜底），同步完成不跨会话。序列不随执行 ctx
// 中断——暂停/停止落进窗口时序列走完，不留半提交态
func (sess *execSession) commitStaged() error {
	if sess.deps.StoreCommitter == nil {
		return fmt.Errorf("提交点建行能力未注入")
	}
	workDir := sess.deps.WorkDirProvider.GetWorkDir()

	// 替换场景首步：软删所选板块对应的旧 store（下载窗口零 DB 副作用的坍缩落点——
	// 软删延后至此，全部暂存写满、提交开始才触碰旧 store）
	if sess.isReplace && sess.mode.storeScope.coversStores() {
		if err := sess.softDeleteReplacedStores(); err != nil {
			return err
		}
	}

	// 抑制键登记面：rename 前登记、序列结束（含补偿）后统一 Release（宽限期覆盖 fsnotify 延迟；
	// 逆 rename 的 Remove 事件同样落在登记窗口内）
	suppressed := make([]string, 0, len(sess.streams))
	defer func() {
		for _, key := range suppressed {
			storeRegistry.Release(key)
		}
	}()

	// 逐轨 rename：暂存（staging/，白名单外零登记）→ 最终路径（store/ 白名单内）
	renamed := make([]*streamController, 0, len(sess.streams))
	compensate := func() {
		for i := len(renamed) - 1; i >= 0; i-- {
			s := renamed[i]
			finalAbs := filepath.Join(workDir, s.finalRel)
			if rerr := os.Rename(finalAbs, s.stagingAbs); rerr != nil {
				logger.Log.Errorf("[Download] 任务 %d 提交补偿回退暂存失败(final=%s): %v", sess.taskId, s.finalRel, rerr)
			}
		}
	}
	for _, s := range sess.streams {
		s.closeWriter() // 防御：Windows 句柄未关会令 rename 失败（正常路径 finalize 已关）
		finalAbs := filepath.Join(workDir, s.finalRel)
		storeRegistry.Suppress(s.finalRel)
		suppressed = append(suppressed, s.finalRel)
		if err := os.MkdirAll(filepath.Dir(finalAbs), 0o755); err != nil {
			compensate()
			return fmt.Errorf("创建最终目录失败: %w", err)
		}
		if err := os.Rename(s.stagingAbs, finalAbs); err != nil {
			compensate()
			return fmt.Errorf("暂存移入最终路径失败: %w", err)
		}
		renamed = append(renamed, s)
	}

	// 单事务：建行 + 挂载 + Resource Save（事务内 repository 方法经 dbFromCtx
	// 走事务连接，见 database 规则）
	var resourceId int64
	txErr := sess.deps.Transactor.ExecInTransaction(context.Background(), func(txCtx context.Context) error {
		mounts := make([]pendingMount, 0, len(sess.streams))
		for _, s := range sess.streams {
			expectedSha := sql.NullString{}
			if s.expectedSha != "" {
				expectedSha = sql.NullString{String: s.expectedSha, Valid: true}
			}
			actualSha := sql.NullString{}
			if s.actualSha != "" {
				actualSha = sql.NullString{String: s.actualSha, Valid: true}
			}
			storeId, err := sess.deps.StoreCommitter.CommitStore(txCtx, s.finalRel, s.finalName, expectedSha, actualSha)
			if err != nil {
				return fmt.Errorf("建 store 行失败(%s): %w", s.finalRel, err)
			}
			mounts = append(mounts, pendingMount{role: s.role, generation: s.generation, storeId: storeId})
		}

		// 保存 Resource(替换场景更新 / 新建场景创建) + 挂 resource_store
		rid, resourceErr := sess.saveResource(txCtx, sess.workId, mounts)
		if resourceErr != nil {
			return resourceErr
		}
		resourceId = rid
		return nil
	})
	if txErr != nil {
		// 建行段由事务原子性兜底（行全回滚）；文件段逆 rename 回退暂存（暂存保留，
		// 恢复/重试可续传或重下）
		compensate()
		return txErr
	}
	sess.currentResourceId = resourceId

	// 暂存目录收尾：全部轨道已 rename 消费，目录（含未被认领的残留文件）一并回收
	stagingDir := sess.deps.StagingPaths.StagingPath(workDir, sess.taskId)
	if err := os.RemoveAll(stagingDir); err != nil {
		logger.Log.Warnf("[Download] 任务 %d 清理暂存目录失败: %v", sess.taskId, err)
	}
	return nil
}

// pendingMount saveResource 挂载单个 store 的中间结构
type pendingMount struct {
	role       string
	generation string
	storeId    int64
}

// saveResource 保存 Resource(事务内调用)并挂 resource_store 行。
// 始终按 workId 查找已有 Resource:找到则更新(避免频繁启停暂停后恢复导致重复创建),
// 未找到则创建新 Resource。isReplace 标志仅用于组合执行的备份决策,不影响此处。
// store 关联只写 resource_store 行
func (sess *execSession) saveResource(ctx context.Context, workId int64, mounts []pendingMount) (int64, error) {
	var resourceId int64

	// 始终查找已有 Resource(不依赖 isReplace),防止重复创建
	existing := sess.findReplaceResource(ctx, workId)
	if existing != nil {
		existing.ResourceComplete = sql.NullInt64{Int64: 0, Valid: true}
		if err := sess.deps.ResourceUpdater.Updates(ctx, existing); err != nil {
			return 0, fmt.Errorf("更新 Resource 失败: %w", err)
		}
		resourceId = existing.GetID()
	}

	if resourceId == 0 {
		// 无已有 Resource:创建新 Resource
		resource := entity.NewResource()
		resource.WorkID = workId
		resource.TaskID = sql.NullInt64{Int64: sess.task.GetID(), Valid: true}
		resource.ResourceComplete = sql.NullInt64{Int64: 0, Valid: true} // 下载未完成
		// 创建期声明的资源类型;严格识别——空值或非预定义值在写入前抛错,不兜底
		resourceType := sess.workTask.ResourceType.String
		if err := entity.ValidateResourceType(resourceType); err != nil {
			return 0, fmt.Errorf("资源类型声明无效: %w", err)
		}
		resource.ResourceType = resourceType

		var err error
		resourceId, err = sess.deps.ResourceSaver.Save(ctx, resource)
		if err != nil {
			return 0, fmt.Errorf("保存资源到数据库失败: %w", err)
		}
	}

	// 挂 resource_store 行:先清同 role 旧关联,再插入本次产出
	if err := sess.mountResourceStores(ctx, resourceId, mounts); err != nil {
		return 0, fmt.Errorf("挂载 resource_store 失败: %w", err)
	}

	return resourceId, nil
}

// findReplaceResource 替换场景定位已有 Resource(按 workId 查询,取首个)
func (sess *execSession) findReplaceResource(ctx context.Context, workId int64) *entity.Resource {
	resources, queryErr := sess.deps.ResourceReader.ListByWorkId(ctx, workId)
	if queryErr != nil {
		logger.Log.Warnf("[Download] 查询作品 %d 资源失败: %v", workId, queryErr)
		return nil
	}
	if len(resources) > 0 {
		return resources[0]
	}
	return nil
}

// mountResourceStores 写入 resource_store 行(替换本次产出 role 的旧关联后再插入)
func (sess *execSession) mountResourceStores(ctx context.Context, resourceId int64, mounts []pendingMount) error {
	if sess.deps.ResourceStoreWriter == nil {
		return nil
	}
	roles := uniqueRoles(mounts)
	if err := sess.deps.ResourceStoreWriter.DeleteByResourceIdAndTypes(ctx, resourceId, roles); err != nil {
		return err
	}
	if len(mounts) == 0 {
		return nil
	}
	stores := make([]*entity.ResourceStore, 0, len(mounts))
	roleSeq := make(map[string]int, len(mounts)) // 同 role 内序号:store 稳定身份(与暂存键/续传配对同维度)
	for _, mt := range mounts {
		// 严格识别 store_type:非预定义角色抛错,不兜底
		if err := entity.ValidateStoreType(mt.role); err != nil {
			return fmt.Errorf("store_type 非法(%s): %w", mt.role, err)
		}
		s := entity.NewResourceStore()
		s.ResourceID = resourceId
		s.StoreType = mt.role
		s.Generation = mt.generation
		s.StoreID = mt.storeId
		s.StoreSeq = roleSeq[mt.role]
		roleSeq[mt.role]++
		stores = append(stores, s)
	}
	return sess.deps.ResourceStoreWriter.CreateBatch(ctx, stores)
}

// closeStreamWriters 关闭全部流的写入句柄（失败收口前调用，释放文件句柄——Windows 文件锁
// 会阻碍控制面回滚物理删文件；各流自身收尾路径已关闭的为幂等兜底）
func (sess *execSession) closeStreamWriters() {
	for _, s := range sess.streams {
		s.closeWriter()
	}
}

// uniqueRoles 提取 mounts 中去重后的 role 列表
func uniqueRoles(mounts []pendingMount) []string {
	seen := make(map[string]struct{}, len(mounts))
	roles := make([]string, 0, len(mounts))
	for _, mt := range mounts {
		if _, ok := seen[mt.role]; ok {
			continue
		}
		seen[mt.role] = struct{}{}
		roles = append(roles, mt.role)
	}
	return roles
}
