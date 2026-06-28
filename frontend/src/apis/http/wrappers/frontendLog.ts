/**
 * FrontendLog HTTP API 包装器
 * 前端 console 日志批量上报，落盘到独立 frontend.log
 */

import { Handler as FrontendLogHandler } from '@bindings/github.com/library-squirrel/backend/frontendLog'
import type { FrontendLogEntry } from '@bindings/github.com/library-squirrel/backend/frontendLog'
import type { ApiResult } from '@renderer/apis/http/types'
import { requireResponse } from '@renderer/apis/http/types'

export type { FrontendLogEntry }

/** 批量上报前端日志到 frontend.log（变更类，不校验 data） */
export async function frontendLogWrite(entries: FrontendLogEntry[]): Promise<ApiResult<any>> {
  return requireResponse(await FrontendLogHandler.Write(entries), '上报前端日志', false)
}
