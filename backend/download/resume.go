package download

// 跨重启续传：按已持久化的 pending_resource_id 定位资源，据 resource_store 各轨 store 状态
// 推导续传偏移，调插件 Resume 取新流集合，续接/重建 store 后进入下载循环。资源缺失时降级
// 为完整重新执行（板块组合重走）。

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/plugin/extension"
	"github.com/library-squirrel/backend/settings"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// resumeFromPersistedState 跨重启续传主体。
// 任务在之前的运行中已暂停,pending_resource_id 已持久化。本方法跳过 CreateWorkInfo/SaveWorkInfo/Start,
// 按 resource_store 各轨 PersistentStore 状态计算续传偏移,调用插件 Resume 取新流集合,续接/重建 store 后进入下载循环
func (sess *execSession) resumeFromPersistedState() comboResult {
	defer func() {
		if r := recover(); r != nil {
			logger.Log.Errorf("[Download] 任务 %d resumeFromPersistedState panic: %v", sess.taskId, r)
			sess.handle.Fail(fmt.Sprintf("跨重启续传 panic: %v", r))
		}
	}()

	// 1. 通过 pending_resource_id 加载 Resource 实体（资源缺失/无 pending 时降级完整重新执行）
	if !sess.workTask.PendingResourceID.Valid {
		logger.Log.Warnf("[Download] 任务 %d 无有效的 pending_resource_id，降级为完整重新执行", sess.taskId)
		return sess.runSectionCombo()
	}
	resource, err := sess.deps.ResourceReader.GetById(sess.runCtx(), sess.workTask.PendingResourceID.Int64)
	if err != nil || resource == nil {
		logger.Log.Warnf("[Download] 任务 %d 加载 Resource(id=%d) 失败: %v，降级为完整重新执行", sess.taskId, sess.workTask.PendingResourceID.Int64, err)
		return sess.runSectionCombo()
	}

	workDir := sess.deps.WorkDirProvider.GetWorkDir()
	if workDir == "" {
		logger.Log.Errorf("[Download] 任务 %d 失败: 未配置资源库目录", sess.taskId)
		settings.NotifyWorkDirUnconfigured("download")
		sess.failTerminal("未配置资源库目录，请先在设置中指定资源库保存位置")
		return comboFinished
	}

	// 2. 读 resource_store 各轨 store 关联
	if sess.deps.ResourceStoreReader == nil {
		logger.Log.Warnf("[Download] 任务 %d 未配置 ResourceStoreReader，降级为完整重新执行", sess.taskId)
		return sess.runSectionCombo()
	}
	storeRows, err := sess.deps.ResourceStoreReader.ListByResourceId(sess.runCtx(), resource.GetID())
	if err != nil {
		logger.Log.Warnf("[Download] 任务 %d 查询 resource_store 失败: %v，降级为完整重新执行", sess.taskId, err)
		return sess.runSectionCombo()
	}
	// 活性过滤：关联保留形态下软删行（替换 victim/外部裁决失效行）的关联也在列，但死行不是续传对象
	// （被误判"store 记录丢失"触发整轨重下、身份匹配错位）——按行活性过滤后再进续传判定
	storeRows = sess.filterAliveAssocs(sess.runCtx(), storeRows)
	if len(storeRows) == 0 {
		logger.Log.Warnf("[Download] 任务 %d Resource 无活行 store 关联，降级为完整重新执行", sess.taskId)
		return sess.runSectionCombo()
	}

	sess.workId = resource.WorkID
	// 回填本次执行产出的 Resource ID：downloadLoop 完成路径的完整度重算以该字段定位资源，
	// 恢复会话的资源来自 pending 定位而非本次新建，不回填则重算被零值守卫跳过（完整性标志不落）
	sess.currentResourceId = resource.GetID()

	// 3. 计算各 downloaded 轨续传偏移 + 收集未完成 derived 轨(整轨重产)
	// 已完成(状态 Complete 且文件存在)的轨道跳过;downloaded 未完成按文件大小算偏移;derived 未完成收集到 incompleteDerivedRoles
	streamOffsets := make([]*sdkdto.StoreResumeOffset, 0, len(storeRows))
	completedSet := make(map[storeIdentity]struct{}, len(storeRows))
	var incompleteDerivedRoles []string
	for _, row := range storeRows {
		ident := storeIdentity{role: row.StoreType, seq: row.StoreSeq}
		store, storeErr := sess.deps.StoreReader.GetById(sess.runCtx(), row.StoreID)
		if storeErr != nil || store == nil {
			// store 记录丢失:downloaded 整轨重下(offset=0),derived 整轨重产
			if row.Generation == entity.GenerationDerived {
				incompleteDerivedRoles = append(incompleteDerivedRoles, row.StoreType)
			} else {
				streamOffsets = append(streamOffsets, &sdkdto.StoreResumeOffset{Role: row.StoreType, StoreSeq: int32(row.StoreSeq), Offset: 0})
			}
			continue
		}
		absPath := sess.deps.StoreReader.GetAbsPath(store)
		info, statErr := os.Stat(absPath)
		if store.CompletedAt > 0 && statErr == nil {
			// 该 store 已完成:按身份记录,不进入 Resume/重产(同 role 多 store 各自独立判定)
			completedSet[ident] = struct{}{}
			continue
		}
		// 未完成:downloaded 按偏移续传,derived 整轨重产
		if row.Generation == entity.GenerationDerived {
			incompleteDerivedRoles = append(incompleteDerivedRoles, row.StoreType)
		} else {
			var offset int64
			if statErr == nil {
				offset = info.Size()
			}
			streamOffsets = append(streamOffsets, &sdkdto.StoreResumeOffset{Role: row.StoreType, StoreSeq: int32(row.StoreSeq), Offset: offset})
		}
	}

	logger.Log.Infof("[Download] 任务 %d 跨重启续传: resourceID=%d, offsets=%v, completed=%v, regenDerived=%v", sess.taskId, resource.GetID(), streamOffsets, completedSet, incompleteDerivedRoles)

	// 4. 调用插件 Resume(按 StreamOffsets 续传未完成 downloaded store,身份化 role+store_seq)
	param := &sdkdto.TaskResumeParam{
		Task:          dto.AssembleTaskDTO(sess.task, sess.workTask, nil),
		StreamOffsets: streamOffsets,
	}
	specs, newResp, err := sess.pluginExec.Resume(sess.runCtx(), param)
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
	if newResp == nil {
		newResp = &sdkdto.WorkResponse{}
	}
	// 续传同样合并作品命名元数据(与板块组合一致)，避免重建路径落 unknownAuthor
	sess.mergeWorkMetaForNaming(newResp, nil)
	sess.workResp = newResp

	// 未完成的 derived 轨由 Start 重新生成(Resume 只续传 downloaded;derived 一次性产物未完成须整轨重产)
	if len(incompleteDerivedRoles) > 0 {
		derivedSpecs, _, startErr := sess.pluginExec.Start(sess.runCtx(), sess.task, sess.workTask, incompleteDerivedRoles)
		if startErr != nil {
			logger.Log.Errorf("[Download] 任务 %d 重产 derived 轨 %v 失败: %v", sess.taskId, incompleteDerivedRoles, startErr)
			// Pause 在 derived 重产进行中取消 ctx:视为暂停,不置失败
			if sess.runAborted() {
				return comboInterrupted
			}
			sess.failTerminal(fmt.Sprintf("重产资源失败: %v", startErr))
			return comboFinished
		}
		specs = append(specs, derivedSpecs...)
	}

	if len(specs) == 0 {
		// 无未完成轨道需续传/重产:任务直接完成
		logger.Log.Infof("[Download][resume] taskId=%d resumeFromPersistedState 无未完成轨道,直接 Finished(streamOffsets=%v regenDerived=%v)", sess.taskId, streamOffsets, incompleteDerivedRoles)
		sess.markResourceComplete(sess.runCtx(), resource.GetID())
		sess.clearPendingResourceID()
		sess.handle.Finish()
		return comboFinished
	}

	// 5. 为每个返回的 spec 续接(continuable downloaded)或重建 store,构建 streamController
	// 解析 bas 基准名与目录(与 startDownload 一致)
	baseRelPath, bas := sess.resolveBaseName(newResp)
	// 多 store 判定基于资源全局 store 总数:resume 的 specs 是未完成子集(已完成 store 不在其中),
	// 不能用 len(specs)——否则部分完成时判定翻转→文件名漂移→续传/重建到错误路径
	multiStore := len(storeRows) > 1
	// 解析每个 spec 的全局 store_seq(specs 是未完成子集,同 role 部分完成时 specs 内重计会与全局 store_seq
	// 错位 → findStoreRowByIdentity 匹配已完成行 → 续传覆盖;须按 streamOffsets/storeRows 取全局 seq)
	specSeq := resumeSpecSeq(specs, streamOffsets, storeRows, completedSet)

	streams := make([]*streamController, 0, len(specs))
	txErr := sess.deps.Transactor.ExecInTransaction(context.Background(), func(txCtx context.Context) error {
		// 未完成 store 的续传/重建:按 spec 处理,记录 (role,seq)→storeId 供全量重挂组装
		storeIdByIdentity := make(map[storeIdentity]int64, len(specs))
		for _, spec := range specs {
			sameRoleSeq := specSeq[spec]
			relPath, fileName := sess.resolveStorePath(spec, baseRelPath, bas, sameRoleSeq, multiStore)
			// 身份匹配:同 role 内按 store_seq(sameRoleSeq)精确定位已有行(替代 role 首匹配,支持 N-同 role)
			existingRow := findStoreRowByIdentity(storeRows, spec.Role, sameRoleSeq)
			offset, hasOffset := findResumeOffset(streamOffsets, spec.Role, sameRoleSeq)
			// continuable 的 downloaded store 且有正偏移:用已有 storeId + ResumeStream 续传
			// 写入偏移:插件指定(spec.ResumeWriteOffset)优先,否则用主程序 stat 的 offset
			if spec.Generation == entity.GenerationDownloaded && existingRow != nil && hasOffset && offset > 0 {
				writeOffset := offset
				if spec.ResumeWriteOffset != nil {
					writeOffset = *spec.ResumeWriteOffset
				}
				writer, resumeErr := sess.deps.StoreStreamer.ResumeStream(txCtx, existingRow.StoreID, writeOffset)
				if resumeErr != nil {
					return resumeErr
				}
				logger.Log.Infof("[ResumeMount] taskId=%d role=%s seq=%d mode=ResumeStream storeId=%d writeOffset=%d streamOffset=%d",
					sess.taskId, spec.Role, sameRoleSeq, existingRow.StoreID, writeOffset, offset)
				sc := newStreamController(spec, existingRow.StoreID, writer, relPath)
				sc.written = writeOffset
				sc.initialOffset = writeOffset
				streams = append(streams, sc)
				storeIdByIdentity[storeIdentity{spec.Role, sameRoleSeq}] = existingRow.StoreID
			} else {
				// derived 或 offset=0 的 downloaded:StoreStream 重建
				storeId, writer, storeErr := sess.deps.StoreStreamer.StoreStream(txCtx, relPath, fileName)
				if storeErr != nil {
					return storeErr
				}
				logger.Log.Infof("[ResumeMount] taskId=%d role=%s seq=%d mode=StoreStream storeId=%d writeOffset=0 streamOffset=%d",
					sess.taskId, spec.Role, sameRoleSeq, storeId, offset)
				streams = append(streams, newStreamController(spec, storeId, writer, relPath))
				storeIdByIdentity[storeIdentity{spec.Role, sameRoleSeq}] = storeId
			}
		}
		// 全量重挂:按 storeRows 顺序(保持 store_seq 稳定)组装已完成 + 本次续传/重建的 store。
		// 已完成用原 storeId(不重下),未完成用 storeIdByIdentity;避免 mountResourceStores 批删丢已完成同 role 关联。
		mounts := make([]pendingMount, 0, len(storeRows))
		for _, row := range storeRows {
			storeId := row.StoreID
			if newId, ok := storeIdByIdentity[storeIdentity{row.StoreType, row.StoreSeq}]; ok {
				storeId = newId
			}
			mounts = append(mounts, pendingMount{role: row.StoreType, generation: row.Generation, storeId: storeId})
		}
		if err := sess.mountResourceStores(txCtx, resource.GetID(), mounts); err != nil {
			return err
		}
		return nil
	})
	if txErr != nil {
		for _, s := range streams {
			if s.storeWriter != nil {
				s.storeWriter.Close()
			}
			sess.deps.StoreFileCleaner.CleanupFile(s.relPath)
		}
		streams = nil
		logger.Log.Errorf("[Download] 任务 %d 跨重启续传事务失败: %v", sess.taskId, txErr)
		if sess.runAborted() {
			return comboInterrupted
		}
		sess.failTerminal(fmt.Sprintf("跨重启续传创建存储失败: %v", txErr))
		return comboFinished
	}

	// 时序不变量:downloadLoop 须在上方续传落盘事务提交后执行。插件 pull chunk 时可能经
	// GetStoreRelPath 查询 resource_store 路径(如 document 引用兄弟 image 文件名),该查询走独立
	// DB 连接,仅事务提交后 resource_store 行与任务 PendingResourceID 才对其可见。事务回滚/暂停路径上方已提前 return。
	sess.streams = streams
	// 续接的既有行与前会话创建时已登记的行经控制面合并去重，本会话重建的行新登记——
	// 恢复后失败的回滚须覆盖两段会话累计的全部新建行
	sess.registerCreatedStores(streams)
	switch sess.downloadLoop() {
	case loopDone:
		return comboFinished
	default:
		return comboInterrupted
	}
}

// storeIdentity resource_store 行的身份键:同 role 内 store_seq 唯一定位一个 store(N-同 role 多 store 支持)
type storeIdentity struct {
	role string
	seq  int
}

// findStoreRowByIdentity 按 (role, store_seq) 身份在 resource_store 行中精确匹配(替代 role 首匹配,避免同 role 歧义)
func findStoreRowByIdentity(rows []*entity.ResourceStore, role string, storeSeq int) *entity.ResourceStore {
	for _, r := range rows {
		if r != nil && r.StoreType == role && r.StoreSeq == storeSeq {
			return r
		}
	}
	return nil
}

// findResumeOffset 在续传偏移列表中按 (role, store_seq) 查找;未命中返回 found=false
func findResumeOffset(offsets []*sdkdto.StoreResumeOffset, role string, storeSeq int) (offset int64, found bool) {
	for _, o := range offsets {
		if o != nil && o.Role == role && int(o.StoreSeq) == storeSeq {
			return o.Offset, true
		}
	}
	return 0, false
}

// resumeSpecSeq 解析 resume 返回的每个 spec 对应的全局 store_seq。
// specs 是未完成子集(已完成 store 不在其中),若按 specs 内 roleCounters 重计 seq,同 role 部分完成时会与
// 全局 store_seq 错位 → findStoreRowByIdentity/findResumeOffset 匹配到已完成行 → 续传覆盖已完成 store。
// 配对:downloaded specs 按 Resume 返回顺序与 streamOffsets 配对(streamOffsets 由主程序按 storeRows 未完成
// downloaded 顺序构造,携带全局 StoreSeq);derived specs 按 role 从 storeRows 未完成 derived 行查(同 role 单例)。
// 依赖插件 Resume/Start 按传入顺序返回 specs 的契约
func resumeSpecSeq(specs []*sdkdto.StoreSpec, streamOffsets []*sdkdto.StoreResumeOffset, storeRows []*entity.ResourceStore, completed map[storeIdentity]struct{}) map[*sdkdto.StoreSpec]int {
	out := make(map[*sdkdto.StoreSpec]int, len(specs))
	dlIdx := 0
	derivedSeq := make(map[string]int)
	for _, spec := range specs {
		if spec == nil {
			continue
		}
		if spec.Generation == entity.GenerationDownloaded {
			if dlIdx < len(streamOffsets) {
				out[spec] = int(streamOffsets[dlIdx].StoreSeq)
				dlIdx++
			}
			continue
		}
		if seq, ok := derivedSeq[spec.Role]; ok {
			out[spec] = seq
			continue
		}
		for _, row := range storeRows {
			if row == nil || row.StoreType != spec.Role || row.Generation != entity.GenerationDerived {
				continue
			}
			if _, complete := completed[storeIdentity{row.StoreType, row.StoreSeq}]; complete {
				continue
			}
			derivedSeq[spec.Role] = row.StoreSeq
			out[spec] = row.StoreSeq
			break
		}
	}
	return out
}
