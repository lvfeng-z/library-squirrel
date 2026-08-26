/**
 * workdirGuard 工作目录防护 HTTP API 包装器
 * 使用 requireResponse + ApiResult 统一响应校验
 */

import { requireResponse, type ApiResult } from '../types'
import { Handler as WorkDirGuardHandler } from '@bindings/github.com/library-squirrel/backend/workdirGuard'
import type { GuardInfoResponse } from '@bindings/github.com/library-squirrel/backend/workdirGuard/models'

// ========== 查询操作 ==========

/**
 * 查询目录保护信息：平台防护机制 + 当前 workDir 可写性探测结果
 * workDir 为空（首次进入设置页未配置目录）时后端跳过探测，仅返回机制信息
 */
export async function workdirGuardGetInfo(workDir: string): Promise<ApiResult<GuardInfoResponse>> {
  return requireResponse(await WorkDirGuardHandler.GetWorkDirGuardInfo(workDir), '查询目录保护')
}
