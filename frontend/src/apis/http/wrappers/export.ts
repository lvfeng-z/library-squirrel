/**
 * Export HTTP API 包装器
 * 直接调用 bindings 接口
 */

import { Handler as ExportHandler } from '@bindings/github.com/library-squirrel/backend/export'
import type { ExportTaskResult } from '@bindings/github.com/library-squirrel/backend/export/models'
import { requireResponse } from '../types'
import type { ApiResult } from '../types'

/**
 * 创建导出任务并启动（两步建任务）。返回新建任务 ID——执行进度与终态经导出任务视图统一承载。
 * outputDir 为空时落盘到工作目录根（默认），非空为自选输出目录（前端经文件选择器挑选并持久化）。
 * 空选择等前置错误由 requireResponse 抛出 Error，调用方 try/catch 捕获。
 */
export async function exportStartExport(workIds: number[], workSetIds: number[], outputDir: string): Promise<ApiResult<ExportTaskResult>> {
  return requireResponse(
    await ExportHandler.StartExport(workIds, workSetIds, outputDir),
    '启动导出',
  )
}
