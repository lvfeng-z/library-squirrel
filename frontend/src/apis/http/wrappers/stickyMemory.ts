/**
 * 粘性记忆 HTTP API 包装器
 * 封装 Wails 绑定层响应校验，校验失败时抛出异常，调用方通过 try/catch 捕获
 */

import { Handler as StickyMemoryHandler } from '@bindings/github.com/library-squirrel/backend/stickymemory'
import { StickyMemoryEntryDTO } from '@bindings/github.com/library-squirrel/backend/base/model/dto'
import type { ApiResult } from '@renderer/apis/http/types'
import { requireResponse } from '@renderer/apis/http/types'

/** 获取粘性记忆管理列表（域/站点域/候选名单/当前选中者均已拆解富化，插件未加载的候选回落原始 id） */
export async function stickyMemoryListMemories(): Promise<ApiResult<StickyMemoryEntryDTO[]>> {
  const result = requireResponse(await StickyMemoryHandler.ListMemories(), '获取记住的选择')
  const data = result.data?.filter((item): item is StickyMemoryEntryDTO => item !== null) ?? []
  return { success: true as const, msg: result.msg, data }
}

/** 删除一条粘性记忆（只删除不编辑；删除后同冲突自然重新询问） */
export async function stickyMemoryDeleteMemory(id: number): Promise<ApiResult<void>> {
  return requireResponse(await StickyMemoryHandler.DeleteMemory(id), '删除记住的选择', false)
}
