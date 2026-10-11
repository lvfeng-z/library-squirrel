/**
 * storageLocation 存储位置 HTTP API 包装器
 * 使用 requireResponse + ApiResult 统一响应校验
 */

import { requireResponse, type ApiResult } from '../types'
import { Handler as StorageLocationHandler } from '@bindings/github.com/library-squirrel/backend/storageLocation'
import type { StorageLocations } from '@bindings/github.com/library-squirrel/backend/storageLocation/models'

// ========== 查询操作 ==========

/**
 * 查询三目录绝对路径（程序目录/用户数据目录/资源库目录）
 * 资源库目录实时读 settings 的 workdir，未配置时为空串
 */
export async function storageLocationGetLocations(): Promise<ApiResult<StorageLocations>> {
  return requireResponse(await StorageLocationHandler.GetStorageLocations(), '查询存储位置')
}
