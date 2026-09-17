/**
 * AuthorRole HTTP API 包装器
 * author 关联级 role 维度清单（选择器候选）的拉取面
 */

import { Handler as AuthorRoleHandler } from '@bindings/github.com/library-squirrel/backend/authorRole'
import type { AuthorRole } from '@bindings/github.com/library-squirrel/backend/base/model/entity/models'
import type { ApiResult } from '../types'
import { requireResponse } from '../types'

// ========== API 方法 ==========

// 拉取全量 role 清单（value/label/origin/lastUse 全字段，value 升序）；分组（内置/用户/插件）与 last_use 排序归前端
export async function authorRoleList(): Promise<ApiResult<(AuthorRole | null)[]>> {
  return requireResponse(await AuthorRoleHandler.List(), '查询 role 清单')
}
