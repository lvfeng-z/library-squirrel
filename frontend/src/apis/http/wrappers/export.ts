/**
 * Export HTTP API 包装器
 * 直接调用 bindings 接口
 */

import { Handler as ExportHandler } from '@bindings/github.com/library-squirrel/backend/export'
import { requireResponse } from '../types'
import type { ApiResult } from '../types'

/**
 * 启动导出（异步：立即返回 exportID，进度/完成经 export-events 事件推送）。
 * outputDir 为空时落盘到工作目录根（默认），非空为自选输出目录（前端经文件选择器挑选并持久化）。
 * 空选择/磁盘预检等前置错误由 requireResponse 抛出 Error，调用方 try/catch 捕获。
 */
export async function exportStartExport(workIds: number[], workSetIds: number[], outputDir: string): Promise<ApiResult<string>> {
  return requireResponse(
    await ExportHandler.StartExport(workIds, workSetIds, outputDir),
    '启动导出',
  )
}

/**
 * 取消指定导出（无进行中导出则 no-op）。
 */
export async function exportCancelExport(exportId: string): Promise<void> {
  const result = await ExportHandler.CancelExport(exportId)
  requireResponse(result, '取消导出', false)
}
