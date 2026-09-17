/**
 * TagNamespace HTTP API 包装器
 * tag 关联级 namespace 维度清单（选择器候选）的拉取面
 */

import { Handler as TagNamespaceHandler } from '@bindings/github.com/library-squirrel/backend/tagNamespace'
import type { TagNamespace } from '@bindings/github.com/library-squirrel/backend/base/model/entity/models'
import type { ApiResult } from '../types'
import { requireResponse } from '../types'

// ========== API 方法 ==========

// 拉取全量 ns 清单（value/label/origin/lastUse 全字段，value 升序）；分组（内置/用户/插件）与 last_use 排序归前端
export async function tagNamespaceList(): Promise<ApiResult<(TagNamespace | null)[]>> {
  return requireResponse(await TagNamespaceHandler.List(), '查询 namespace 清单')
}
