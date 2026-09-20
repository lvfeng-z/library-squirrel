/**
 * AuthorInfo HTTP API 包装器
 * 封装 Wails 绑定层响应校验，校验失败时抛出异常，调用方通过 try/catch 捕获
 */

import { Handler as AuthorInfoHandler, SiteAuthorFetchItemResult } from '@bindings/github.com/library-squirrel/backend/authorInfo'
import type { ApiResult } from '@renderer/apis/http/types'
import { requireResponse } from '@renderer/apis/http/types'

// ========== API 方法 ==========

/** 手动拉取单个站点作者信息（行操作，loading 态由调用方挂起） */
export async function authorInfoFetchSiteAuthorInfo(siteAuthorId: number): Promise<ApiResult<any>> {
  return requireResponse(await AuthorInfoHandler.FetchSiteAuthorInfo(siteAuthorId), '拉取站点作者信息', false)
}

/** 手动批量拉取站点作者信息（逐条结果清单与入参顺序一致） */
export async function authorInfoFetchSiteAuthorsInfo(siteAuthorIds: number[]): Promise<ApiResult<(SiteAuthorFetchItemResult | null)[]>> {
  return requireResponse(await AuthorInfoHandler.FetchSiteAuthorsInfo(siteAuthorIds), '批量拉取站点作者信息')
}

/** 为本地作者设置头像（源文件为文件对话框选取的绝对路径；换头像形态先删旧） */
export async function authorInfoSetLocalAuthorAvatar(localAuthorId: number, sourceAbsPath: string): Promise<ApiResult<any>> {
  return requireResponse(await AuthorInfoHandler.SetLocalAuthorAvatar(localAuthorId, sourceAbsPath), '设置本地作者头像', false)
}

/** 移除本地作者头像（显式破坏操作，调用方二次确认） */
export async function authorInfoRemoveLocalAuthorAvatar(localAuthorId: number): Promise<ApiResult<any>> {
  return requireResponse(await AuthorInfoHandler.RemoveLocalAuthorAvatar(localAuthorId), '移除本地作者头像', false)
}
