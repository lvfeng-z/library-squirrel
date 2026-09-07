package download

// 入库编排：资源落盘事务（建 store 流 + 挂 resource_store + Resource 保存 + pending_resource_id
// 直写）与替换链前置软删（成功后登记终端回滚清单，失败复活归控制面单点触发）。

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/taskManager"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// softDeleteAndArmRollback 替换前置软删并登记终端回滚：软删作品资源下所选角色的活行 store
// （委派 resource 替换能力，显式角色集合语义），成功后把被软删行清单登记进控制面
// （失败/停止时由控制面失败单点按清单复活）。任一步失败按组合执行失败收口。
// 返回 true 表示执行已中断或失败收口完成，调用方终止板块组合
func (sess *execSession) softDeleteAndArmRollback() bool {
	if sess.deps.ReplaceStoreOps == nil {
		logger.Log.Errorf("[Download] 任务 %d 替换前置软删失败: %v", sess.taskId, fmt.Errorf("替换链能力未注入"))
		sess.comboFail(fmt.Sprintf("替换前置软删旧资源失败: %v", fmt.Errorf("替换链能力未注入")))
		return true
	}
	victims, err := sess.deps.ReplaceStoreOps.SoftDeleteWorkStoreRoles(sess.runCtx(), sess.workId, sess.replaceSoftDeleteRoles())
	if err != nil {
		logger.Log.Errorf("[Download] 任务 %d 替换前置软删失败: %v", sess.taskId, err)
		sess.comboFail(fmt.Sprintf("替换前置软删旧资源失败: %v", err))
		return true
	}
	if len(victims) > 0 {
		sess.handle.SetTerminalRollback(taskManager.TerminalRollback{Victims: victims})
	}
	return false
}

// startDownload 为每个 spec 建存储、挂 resource_store、进入多流下载循环
func (sess *execSession) startDownload(specs []*sdkdto.StoreSpec, workResp *sdkdto.WorkResponse) comboResult {
	sess.workResp = workResp

	// 解析 bas 基准名与目录(所有 store 文件名共用;bas 由模板+作品元数据生成,不依赖具体 spec)
	baseRelPath, bas := sess.resolveBaseName(workResp)
	roleCounters := make(map[string]int, len(specs))
	// 多 store 判定(资源级):资源 store 总数>1 则全部带 role+seq;单 store 用 <bas>.<ext>
	multiStore := len(specs) > 1

	// 事务:为每个 spec 建 StoreStream + 挂 resource_store + Resource Save + PendingResourceID 更新
	streams := make([]*streamController, 0, len(specs))
	txErr := sess.deps.Transactor.ExecInTransaction(context.Background(), func(txCtx context.Context) error {
		mounts := make([]pendingMount, 0, len(specs))
		for _, spec := range specs {
			sameRoleSeq := roleCounters[spec.Role]
			roleCounters[spec.Role]++
			relPath, fileName := sess.resolveStorePath(spec, baseRelPath, bas, sameRoleSeq, multiStore)
			storeId, writer, storeErr := sess.deps.StoreStreamer.StoreStream(txCtx, relPath, fileName)
			if storeErr != nil {
				return storeErr
			}
			streams = append(streams, newStreamController(spec, storeId, writer, relPath))
			mounts = append(mounts, pendingMount{role: spec.Role, generation: spec.Generation, storeId: storeId})
		}

		// 保存 Resource(替换场景更新 / 新建场景创建) + 挂 resource_store
		resourceId, resourceErr := sess.saveResource(txCtx, sess.workId, mounts)
		if resourceErr != nil {
			return resourceErr
		}
		sess.currentResourceId = resourceId

		// 同步更新 pending_resource_id（事务内直接写 DB，作品任务领域行与内存对象同步）
		sess.workTask.PendingResourceID = sql.NullInt64{Int64: resourceId, Valid: true}
		return sess.deps.PendingResourceUpdater.UpdatePendingResourceID(txCtx, sess.taskId, sess.workTask.PendingResourceID)
	})
	if txErr != nil {
		// 事务回滚：DB 记录已全部回滚，需显式关闭句柄并清理文件
		for _, s := range streams {
			if s.storeWriter != nil {
				s.storeWriter.Close()
			}
			sess.deps.StoreFileCleaner.CleanupFile(s.relPath)
		}
		streams = nil
		logger.Log.Errorf("[Download] 任务 %d 创建资源事务失败: %v", sess.taskId, txErr)
		if sess.runAborted() {
			return comboInterrupted
		}
		sess.failTerminal(fmt.Sprintf("创建资源失败: %v", txErr))
		return comboFinished
	}

	// 新建行清单登记进终态回滚载荷（事务提交后、任何中断返回路径前）：停止/暂停恢复后失败
	// 等中断路径不经会话收口，控制面回滚复活旧代前按此清单丢弃新建行
	sess.registerCreatedStores(streams)

	// setup 阶段暂停在事务窗口内命中:暂停此时 len(streams)==0 走 cancel 路径,
	// 事务用 context.Background 不受影响仍提交;此处清理已建句柄并返回暂停,避免带着已取消的 ctx 进入 downloadLoop
	if sess.runAborted() {
		for _, s := range streams {
			if s.storeWriter != nil {
				s.storeWriter.Sync()
				s.storeWriter.Close()
			}
			if s.reader != nil {
				s.reader.Close()
			}
		}
		sess.streams = nil
		return comboInterrupted
	}

	// 时序不变量:downloadLoop 须在上方 startDownload 事务提交后执行。插件 pull chunk 时可能经
	// GetStoreRelPath 查询 resource_store 路径(如 document 引用兄弟 image 文件名),该查询走独立
	// DB 连接,仅事务提交后 resource_store 行与任务 PendingResourceID 才对其可见。事务回滚/暂停路径上方已提前 return。
	sess.streams = streams
	switch sess.downloadLoop() {
	case loopDone:
		return comboFinished
	default:
		return comboInterrupted
	}
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
	roleSeq := make(map[string]int, len(mounts)) // 同 role 内序号:store 稳定身份(与续传身份匹配、文件名消歧统一)
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

// filterAliveAssocs 过滤出指向活行 store 的关联（批量判活，无 N+1；行缺失的关联一并剔除）
func (sess *execSession) filterAliveAssocs(ctx context.Context, rows []*entity.ResourceStore) []*entity.ResourceStore {
	if len(rows) == 0 || sess.deps.StoreBackupReader == nil {
		return rows
	}
	ids := make([]int64, 0, len(rows))
	for _, rs := range rows {
		if rs.StoreID > 0 {
			ids = append(ids, rs.StoreID)
		}
	}
	stores := sess.deps.StoreBackupReader.ListByIdsIncludeDeleted(ctx, ids)
	alive := make(map[int64]struct{}, len(stores))
	for _, st := range stores {
		if st.DeletedAt == 0 {
			alive[st.GetID()] = struct{}{}
		}
	}
	result := make([]*entity.ResourceStore, 0, len(rows))
	for _, rs := range rows {
		if _, ok := alive[rs.StoreID]; ok {
			result = append(result, rs)
		}
	}
	return result
}

// registerCreatedStores 把本会话新建/续接的 store 行清单登记进终态回滚载荷（控制面按
// store ID 去重合并，跨执行轮次并集保留）。行创建事务提交后调用——登记先于一切中断
// 返回路径（停止/暂停不经会话收口，控制面回滚丢弃新建行、复活旧代均按登记清单执行）
func (sess *execSession) registerCreatedStores(streams []*streamController) {
	if len(streams) == 0 {
		return
	}
	ids := make([]int64, 0, len(streams))
	for _, s := range streams {
		ids = append(ids, s.storeId)
	}
	sess.handle.SetTerminalRollback(taskManager.TerminalRollback{CreatedStoreIDs: ids})
}

// closeStreamWriters 关闭全部流的写入句柄（失败收口前调用，释放文件句柄——Windows 文件锁
// 会阻碍控制面回滚物理删文件；各流自身收尾路径已关闭的为幂等兜底）
func (sess *execSession) closeStreamWriters() {
	for _, s := range sess.streams {
		if s.storeWriter != nil {
			s.storeWriter.Close()
		}
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
