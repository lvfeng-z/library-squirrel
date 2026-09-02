/**
 * WorkSet HTTP API 包装器
 * 直接调用 bindings 接口
 */

import type { ApiResponse } from '../types'
import { requireResponse } from '../types'
import { Handler as WorkSetHandler, WorkSetQueryDTO } from '@bindings/github.com/library-squirrel/backend/workSet'
import { Page } from '@bindings/github.com/library-squirrel/backend/base/model/models'
import {
  WorkDTO,
  WorkSetDTO
} from "@bindings/github.com/lvfeng-z/library-squirrel-sdk/dto";
import { WorkSetWithCoverDTO, WorkSetWithWorksResultDTO } from "@bindings/github.com/library-squirrel/backend/base/model/dto";

export interface WorkSetVO {
  id: number
  name: string
  coverId: number
  createTime: number
  updateTime: number
}

// ========== 工具函数 ==========

/**
 * 将 WorkSetResultDTO 转换为 WorkSetVO
 */
function toWorkSetVO(dto: WorkSetDTO): WorkSetVO {
  return {
    id: dto.id ?? 0,
    name: dto.siteWorkSetName ?? '',
    coverId: 0,
    createTime: dto.createTime ?? 0,
    updateTime: dto.updateTime ?? 0
  }
}

// ========== API 方法 ==========

/**
 * 根据作品集ID列表获取作品集及作品信息
 */
export async function workSetListWorkSetWithWorkByIds(
  workSetIds: number[]
): Promise<ApiResponse<(WorkSetWithWorksResultDTO | null)[]>> {
  const result = await WorkSetHandler.ListWorkSetWithWorkByIds(workSetIds)
  if (!result) {
    return { success: false, msg: '获取失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '获取失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ?? undefined }
}

/**
 * 分页查询作品集（带封面）
 */
export async function workSetQueryPageWithCover(page: Page<WorkSetWithCoverDTO>, query: WorkSetQueryDTO): Promise<ApiResponse<Page<WorkSetWithCoverDTO>>> {
  const result = await WorkSetHandler.QueryPageWithCover(page, query)
  if (!result) {
    return { success: false, msg: '查询失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '查询失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ?? undefined }
}

export async function workSetGetById(id: number): Promise<ApiResponse<WorkSetVO>> {
  const result = await WorkSetHandler.GetById(id)
  if (!result) {
    return { success: false, msg: '获取失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '获取失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ? toWorkSetVO(result.data) : undefined }
}

export async function workSetQueryPage(page: Page<WorkSetDTO>, query: WorkSetQueryDTO): Promise<ApiResponse<Page<WorkSetDTO>>> {
  const result = await WorkSetHandler.QueryPage(page, query)
  if (!result) {
    return { success: false, msg: '查询失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '查询失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ?? undefined }
}

export async function workSetUpdate(workSet: {
  id: number
  nickName?: string
  description?: string
}): Promise<ApiResponse<null>> {
  // 后端 Update 已改为部分更新（GORM Updates，仅写非零字段），直接传编辑字段即可，未传字段保留原值
  const dto = new WorkSetDTO({
    id: workSet.id,
    nickName: workSet.nickName ?? null,
    description: workSet.description ?? null
  })
  return requireResponse(await WorkSetHandler.Update(dto), '更新作品集', false)
}

/**
 * 手动创建作品集（本地手建集：站点键为空，不占站点业务键；名称落本地昵称）
 */
export async function workSetCreate(workSet: {
  nickName: string
  description?: string
}): Promise<ApiResponse<number>> {
  const dto = new WorkSetDTO({
    nickName: workSet.nickName,
    description: workSet.description ?? null
  })
  return requireResponse(await WorkSetHandler.Save(dto), '创建作品集', false)
}

/**
 * 软删除作品集（移入回收站可复原，关联保留）
 */
export async function workSetSoftDelete(id: number): Promise<ApiResponse<null>> {
  return requireResponse(await WorkSetHandler.SoftDeleteWorkSet(id), '删除作品集', false)
}

/**
 * 获取作品集的直接成员作品ID（不含后代子集成员；封面/移除等直接成员专属操作的判定依据）
 */
export async function workSetGetDirectWorkIds(workSetId: number): Promise<ApiResponse<number[]>> {
  const result = await WorkSetHandler.GetDirectWorkIds(workSetId)
  if (!result) {
    return { success: false, msg: '获取失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '获取失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ?? [] }
}

/**
 * 批量关联作品到作品集
 */
export async function workSetLinkBatch(
  workSetId: number,
  workIds: number[]
): Promise<ApiResponse<boolean>> {
  const result = await WorkSetHandler.LinkBatchToWorkSet(workSetId, workIds)
  if (!result) {
    return { success: false, msg: '操作失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '' }
}

/**
 * 批量取消作品与作品集的关联
 */
export async function workSetRemoveBatch(
  workSetId: number,
  workIds: number[]
): Promise<ApiResponse<boolean>> {
  const result = await WorkSetHandler.RemoveBatchFromWorkSet(workSetId, workIds)
  if (!result) {
    return { success: false, msg: '操作失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '' }
}

/**
 * 获取作品集下的作品列表
 */
export async function workSetGetWorks(
  workSetId: number
): Promise<ApiResponse<WorkDTO[]>> {
  const result = await WorkSetHandler.GetWorksByWorkSetId(workSetId)
  if (!result) {
    return { success: false, msg: '获取失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '获取失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data?.filter((item): item is WorkDTO => item !== null) ?? undefined }
}

/**
 * 设置作品集封面
 */
export async function workSetSetCover(
  workSetId: number,
  workId: number
): Promise<ApiResponse<boolean>> {
  const result = await WorkSetHandler.SetCover(workSetId, workId)
  if (!result) {
    return { success: false, msg: '操作失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '' }
}

/**
 * 获取作品关联的作品集列表
 */
export async function workSetListByWorkId(workId: number): Promise<ApiResponse<(WorkSetDTO | null)[]>> {
  const result = await WorkSetHandler.ListWorkSetsByWorkId(workId)
  if (!result) {
    return { success: false, msg: '获取失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '获取失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data?.filter((item): item is WorkSetDTO => item !== null) ?? undefined }
}

/**
 * 建立作品集父子关系（parent 将 child 纳为子集），后端事务内防环路
 */
export async function workSetAddChildWorkSet(
  parentWorkSetId: number,
  childWorkSetId: number
): Promise<ApiResponse<any>> {
  const result = await WorkSetHandler.AddChildWorkSet(parentWorkSetId, childWorkSetId)
  if (!result) {
    return { success: false, msg: '建立父子关系失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '建立父子关系失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data }
}

/**
 * 解除作品集父子关系
 */
export async function workSetRemoveChildWorkSet(
  parentWorkSetId: number,
  childWorkSetId: number
): Promise<ApiResponse<any>> {
  const result = await WorkSetHandler.RemoveChildWorkSet(parentWorkSetId, childWorkSetId)
  if (!result) {
    return { success: false, msg: '解除父子关系失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '解除父子关系失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data }
}

/**
 * 查询作品集的直接子作品集列表（层级管理展示用）
 */
export async function workSetListChildWorkSets(
  parentWorkSetId: number
): Promise<ApiResponse<(WorkSetDTO | null)[]>> {
  const result = await WorkSetHandler.ListChildWorkSets(parentWorkSetId)
  if (!result) {
    return { success: false, msg: '获取子作品集失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '获取子作品集失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data?.filter((item): item is WorkSetDTO => item !== null) ?? undefined }
}

/**
 * 物理纳入：把源作品集及其后代的作品复制到目标作品集（静态快照，源不变、不可撤回）
 */
export async function workSetMergeWorkSetInto(
  sourceWorkSetId: number,
  targetWorkSetId: number
): Promise<ApiResponse<any>> {
  const result = await WorkSetHandler.MergeWorkSetInto(sourceWorkSetId, targetWorkSetId)
  if (!result) {
    return { success: false, msg: '物理纳入失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '物理纳入失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data }
}

/**
 * 应用原站序：把作品集的原站序(site_sort_order)拷贝到本地序(sort_order)。
 * 应用后本地序即原站序，重载作品列表即按原站顺序展示；site_sort_order 为空的成员保持原本地序。
 */
export async function workSetApplySiteOrder(workSetId: number): Promise<ApiResponse<boolean>> {
  const result = await WorkSetHandler.ApplySiteOrder(workSetId)
  if (!result) {
    return { success: false, msg: '应用原站序失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '' }
}