/**
 * ExternalApi HTTP API 包装器
 * 直接调用 bindings 接口
 */

import { requireResponse, type ApiResult } from '../types'
import { Handler as ExternalApiHandler } from '@bindings/github.com/library-squirrel/backend/externalapi'
import { Page } from '@bindings/github.com/library-squirrel/backend/base/model/models'
import type { ExternalApiOpRecord } from '@bindings/github.com/library-squirrel/backend/base/model/entity/models'

/**
 * 分页列出外部操作记录（最新调用在前；设置页只读列表数据源）。
 * 记录行只读——参数摘要已在落库时脱敏（URL 查询串不落库），老化行由后端轮转自动裁剪
 */
export async function externalApiListOpRecords(pageNumber: number, pageSize: number): Promise<ApiResult<Page<ExternalApiOpRecord>>> {
  return requireResponse(await ExternalApiHandler.ListOpRecords(pageNumber, pageSize), '查询外部操作记录')
}
