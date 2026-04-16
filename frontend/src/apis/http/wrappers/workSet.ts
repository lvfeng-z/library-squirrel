/**
 * WorkSet HTTP API 包装器
 * 直接调用 bindings 接口
 */

import type { ApiResponse } from '../types'
import { Handler as WorkSetHandler, WorkSetDTO, WorkSetQueryDTO, WorkSetResultDTO } from '@bindings/github.com/library-squirrel/wails/internal/workSet'
import type { Work } from '@bindings/github.com/library-squirrel/wails/internal/model/models'
import type { Page } from '@bindings/github.com/library-squirrel/wails/pkg/model/models'

export interface WorkSetVO {
  id: number
  name: string
  coverId: number
  createTime: number
  updateTime: number
}

export interface WorkSetWithWorksVO {
  workSet: WorkSetVO
  works: WorkSetVO[]
}

export interface WorkSetWithCoverVO {
  workSet: WorkSetVO
  coverWork?: WorkSetVO
}

export interface PageResult {
  items: WorkSetVO[]
  total: number
  page: number
  pageSize: number
}

// ========== 工具函数 ==========

/**
 * 将 WorkSetResultDTO 转换为 WorkSetVO
 */
function toWorkSetVO(dto: WorkSetResultDTO): WorkSetVO {
  return {
    id: dto.id,
    name: dto.siteWorkSetName ?? '',
    coverId: 0,
    createTime: dto.createTime,
    updateTime: dto.updateTime
  }
}

/**
 * 将 Work 转换为 WorkSetVO
 */
function workToWorkSetVO(work: Work): WorkSetVO {
  return {
    id: work.id,
    name: work.siteWorkName?.toString() ?? '',
    coverId: 0,
    createTime: work.createTime,
    updateTime: work.updateTime
  }
}

// ========== API 方法 ==========

/**
 * 根据作品集ID列表获取作品集及作品信息
 */
export async function workSetListWorkSetWithWorkByIds(
  workSetIds: number[]
): Promise<ApiResponse<WorkSetWithWorksVO[]>> {
  const result = await WorkSetHandler.ListWorkSetWithWorkByIds(workSetIds)
  if (!result) {
    return { success: false, msg: '获取失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '获取失败' }
  }
  return { success: true, msg: result.msg ?? '', data: undefined }
}

/**
 * 分页查询作品集（带封面）
 */
export async function workSetQueryPageWithCover(query: {
  page: number
  pageSize: number
}): Promise<ApiResponse<Page<any>>> {
  const queryDTO = new WorkSetQueryDTO({})
  const result = await WorkSetHandler.QueryPageWithCover(query.page, query.pageSize, queryDTO)
  if (!result) {
    return { success: false, msg: '查询失败：接口返回为空' }
  }
  return result
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

export async function workSetQueryPage(query: {
  page: number
  pageSize: number
  query?: { name?: string }
}): Promise<ApiResponse<Page<WorkSetResultDTO>>> {
  const queryDTO = new WorkSetQueryDTO({
    siteWorkSetNameLike: query.query?.name ?? null
  })
  const result = await WorkSetHandler.QueryPage(query.page, query.pageSize, queryDTO)
  if (!result) {
    return { success: false, msg: '查询失败：接口返回为空' }
  }
  return result
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
  name?: string
  coverId?: number
}): Promise<ApiResponse<WorkSetVO>> {
  const dto = new WorkSetDTO({
    id: workSet.id,
    siteWorkSetName: workSet.name ?? null
  })
  const result = await WorkSetHandler.Update(dto)
  if (!result) {
    return { success: false, msg: '更新失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '' }
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
): Promise<ApiResponse<Work[]>> {
  const result = await WorkSetHandler.GetWorksByWorkSetId(workSetId)
  if (!result) {
    return { success: false, msg: '获取失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '获取失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ?? undefined }
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