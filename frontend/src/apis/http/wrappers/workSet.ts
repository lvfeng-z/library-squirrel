/**
 * WorkSet HTTP API 包装器
 * 直接调用 bindings 接口
 */

import type { ApiResponse } from '../types'
import { Handler as WorkSetHandler, WorkSetDTO, WorkSetQueryDTO, WorkSetResultDTO } from '@bindings/github.com/library-squirrel/wails/internal/workSet'
import type { Work } from '@bindings/github.com/library-squirrel/wails/internal/model/models'

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
 * 注意：此方法在 bindings 中未实现
 */
export async function workSetListWorkSetWithWorkByIds(
  _workSetIds: number[]
): Promise<ApiResponse<WorkSetWithWorksVO[]>> {
  // TODO: 此接口在 bindings 中未实现 (ListWorkSetWithWorkByIds)
  return { success: false, msg: '此接口未实现：workSetListWorkSetWithWorkByIds' }
}

/**
 * 分页查询作品集（带封面）
 * 注意：此方法在 bindings 中未实现
 */
export async function workSetQueryPageWithCover(_query: {
  page: number
  pageSize: number
}): Promise<ApiResponse<PageResult>> {
  // TODO: 此接口在 bindings 中未实现 (QueryPageWithCover)
  return { success: false, msg: '此接口未实现：workSetQueryPageWithCover' }
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
}): Promise<ApiResponse<PageResult>> {
  const queryDTO = new WorkSetQueryDTO({
    siteWorkSetNameLike: query.query?.name ?? null
  })
  const result = await WorkSetHandler.QueryPage(query.page, query.pageSize, queryDTO)
  if (!result) {
    return { success: false, msg: '查询失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '查询失败' }
  }
  const page = result.data
  if (!page) {
    return { success: true, msg: '', data: { items: [], total: 0, page: query.page, pageSize: query.pageSize } }
  }
  return {
    success: true,
    msg: result.msg ?? '',
    data: {
      items: page.data ? page.data.map(item => item ? toWorkSetVO(item) : null).filter((item): item is WorkSetVO => item !== null) : [],
      total: page.dataCount ?? 0,
      page: page.pageNumber ?? query.page,
      pageSize: page.pageSize ?? query.pageSize
    }
  }
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
 * 批量关联作品到作品集
 * 注意：此方法在 bindings 中未实现
 */
export async function workSetLinkBatch(
  _workSetId: number,
  _workIds: number[]
): Promise<ApiResponse<boolean>> {
  // TODO: 此接口在 bindings 中未实现 (LinkBatch)
  return { success: false, msg: '此接口未实现：workSetLinkBatch' }
}

/**
 * 批量取消作品与作品集的关联
 * 注意：此方法在 bindings 中未实现
 */
export async function workSetRemoveBatch(
  _workSetId: number,
  _workIds: number[]
): Promise<ApiResponse<boolean>> {
  // TODO: 此接口在 bindings 中未实现 (RemoveBatch)
  return { success: false, msg: '此接口未实现：workSetRemoveBatch' }
}

/**
 * 获取作品集下的作品列表
 * 注意：此方法在 bindings 中未实现
 */
export async function workSetGetWorks(
  _workSetId: number
): Promise<ApiResponse<WorkSetVO[]>> {
  // TODO: 此接口在 bindings 中未实现 (GetWorksByWorkSetId)
  return { success: false, msg: '此接口未实现：workSetGetWorks' }
}

/**
 * 设置作品集封面
 * 注意：此方法在 bindings 中未实现
 */
export async function workSetSetCover(
  _workSetId: number,
  _workId: number
): Promise<ApiResponse<boolean>> {
  // TODO: 此接口在 bindings 中未实现 (SetCover)
  return { success: false, msg: '此接口未实现：workSetSetCover' }
}