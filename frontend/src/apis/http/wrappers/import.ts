/**
 * Import HTTP API 包装器
 * 直接调用 bindings 接口
 */

import { Handler as ImportHandler } from '@bindings/github.com/library-squirrel/backend/import'
import type { StartImportResult } from '@bindings/github.com/library-squirrel/backend/import/models'
import { requireResponse } from '../types'
import type { ApiResult } from '../types'

/**
 * 从导出 ZIP 产物建导入任务树并启动。返回建树摘要（成功提示引导用户去任务面板查看进度）。
 * 版本锚拒绝/空包/超上限等前置校验失败由 requireResponse 抛出 Error，调用方 try/catch 捕获后展示。
 */
export async function importStartImport(zipPath: string): Promise<ApiResult<StartImportResult>> {
  return requireResponse(
    await ImportHandler.StartImport(zipPath),
    '启动导入',
  )
}
