/**
 * WorkSet Wails 绑定包装器
 */

import { Handler } from '@bindings/github.com/library-squirrel/wails/internal/workSet'
import type { ApiResponse } from '@apis/http'

// ========== API 方法 ==========

/**
 * 保存作品集
 */
export async function workSetSave(workSet: any): Promise<ApiResponse<void>> {
  return Handler.Save(workSet)
}

/**
 * 更新作品集
 */
export async function workSetUpdate(workSet: any): Promise<ApiResponse<void>> {
  return Handler.Update(workSet)
}

/**
 * 删除作品集
 */
export async function workSetDelete(id: number): Promise<ApiResponse<void>> {
  return Handler.Delete(id)
}

/**
 * 获取单个作品集
 */
export async function workSetGetById(id: number): Promise<ApiResponse<any>> {
  return Handler.GetById(id)
}

/**
 * 分页查询作品集
 */
export async function workSetQueryPage(query: any): Promise<ApiResponse<any>> {
  return Handler.QueryPage(query.page ?? 1, query.pageSize ?? 10, query.query)
}

/**
 * 分页查询作品集（带封面）
 */
export async function workSetQueryPageWithCover(query: any): Promise<ApiResponse<any>> {
  return Handler.QueryPage(query.page ?? 1, query.pageSize ?? 10, query.query)
}

/**
 * 获取作品集中的作品列表
 */
export async function workSetGetWorks(id: number): Promise<ApiResponse<any[]>> {
  return Handler.GetWorksByWorkSetId(id)
}

/**
 * 批量关联作品到作品集
 */
export async function workSetLinkBatch(id: number, workIds: number[]): Promise<ApiResponse<void>> {
  // 批量关联通过循环单条关联实现
  const promises = workIds.map(workId => Handler.LinkWorkToWorkSet(workId, id))
  await Promise.all(promises)
  return { success: true, data: undefined } as ApiResponse<void>
}

/**
 * 批量从作品集移除作品
 */
export async function workSetRemoveBatch(id: number, workIds: number[]): Promise<ApiResponse<void>> {
  const promises = workIds.map(workId => Handler.UnlinkWorkFromWorkSet(workId, id))
  await Promise.all(promises)
  return { success: true, data: undefined } as ApiResponse<void>
}

/**
 * 设置作品集封面
 */
export async function workSetSetCover(id: number, workId: number): Promise<ApiResponse<void>> {
  // Handler 不支持设置封面，需要后端扩展
  return Handler.LinkWorkToWorkSet(workId, id) as ApiResponse<void>
}

/**
 * 取消设置作品集封面
 */
export async function workSetUnsetCover(id: number, workId: number): Promise<ApiResponse<void>> {
  return Handler.UnlinkWorkFromWorkSet(workId, id)
}

/**
 * 获取作品集封面作品ID
 * @deprecated Handler 不支持，需要后端扩展
 */
export async function workSetGetCoverWorkId(id: number): Promise<ApiResponse<number>> {
  // 临时实现：返回第一个作品作为封面
  const works = await Handler.GetWorksByWorkSetId(id)
  return works as ApiResponse<number>
}

/**
 * 根据ID列表获取作品集及作品
 */
export async function workSetListWorkSetWithWorkByIds(ids: number[]): Promise<ApiResponse<any[]>> {
  const promises = ids.map(id => Handler.GetById(id))
  const results = await Promise.all(promises)
  return results as ApiResponse<any[]>
}
