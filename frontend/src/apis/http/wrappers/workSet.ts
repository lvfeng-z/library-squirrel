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
  WorkSetDTO, WorkSetWithCoverDTO,
  WorkSetWithWorksResultDTO
} from "@bindings/github.com/lvfeng-z/library-squirrel-sdk/dto";

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
    id: dto.id,
    name: dto.siteWorkSetName ?? '',
    coverId: 0,
    createTime: dto.createTime,
    updateTime: dto.updateTime
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

export async function workSetSave(workSet: {
  name?: string
  coverId?: number
}): Promise<ApiResponse<WorkSetVO>> {
  const dto = new WorkSetDTO({
    siteWorkSetName: workSet.name ?? null
  })
  const result = await WorkSetHandler.Save(dto)
  if (!result) {
    return { success: false, msg: '保存失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '保存失败' }
  }
  return { success: true, msg: result.msg ?? '', data: { id: result.data ?? 0, name: '', coverId: 0, createTime: 0, updateTime: 0 } }
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

export async function workSetDelete(id: number): Promise<ApiResponse<null>> {
  const result = await WorkSetHandler.Delete(id)
  if (!result) {
    return { success: false, msg: '删除失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '' }
}

/**
 * 根据站点作品集ID和站点名称获取作品集
 */
export async function workSetGetBySiteWorkSetIdAndSiteName(
  siteWorkSetId: string,
  siteName: string
): Promise<ApiResponse<WorkSetVO>> {
  const result = await WorkSetHandler.GetBySiteWorkSetIdAndSiteName(siteWorkSetId, siteName)
  if (!result) {
    return { success: false, msg: '获取失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '获取失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ? toWorkSetVO(result.data) : undefined }
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