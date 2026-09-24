/**
 * AuthorInfo HTTP API 包装器
 * 封装 Wails 绑定层响应校验，校验失败时抛出异常，调用方通过 try/catch 捕获
 */

import { Handler as AuthorInfoHandler, SiteAuthorFetchResponse } from '@bindings/github.com/library-squirrel/backend/authorInfo'
import type { SiteAuthorFetchChoice } from '@bindings/github.com/library-squirrel/backend/base/model/dto'
import type { ApiResult } from '@renderer/apis/http/types'
import { requireResponse } from '@renderer/apis/http/types'

// ========== API 方法 ==========

/**
 * 手动拉取单个站点作者信息（行操作，loading 态由调用方挂起）
 *
 * chosenPlugins 为交互面显选（站点键 → 插件与条目两键联合定位一个候选条目，空 = 未显选）：
 * 候选按站点收窄，该站点候选多于一个且未显选时响应为冲突态（外层成功，载荷的 conflicts 非空，
 * 未调用任何插件）——至多一组，candidates 为该站点的候选清单（插件 × 条目对，展示名为后端拼好的
 * 「插件名 · 条目名」全键展示名；首位即默认选中项，顺序由后端给定不再重排），由调用方读载荷分流
 */
export async function authorInfoFetchSiteAuthorInfo(
  siteAuthorId: number,
  chosenPlugins: SiteAuthorFetchChoice[]
): Promise<ApiResult<SiteAuthorFetchResponse>> {
  return requireResponse(
    await AuthorInfoHandler.FetchSiteAuthorInfo(siteAuthorId, chosenPlugins),
    '拉取站点作者信息'
  )
}

/**
 * 手动批量拉取站点作者信息：冲突按站点分组整批前置返回（同一站点只一组、只问一次），
 * 否则载荷的 items 为与入参顺序一致的逐条结果清单
 */
export async function authorInfoFetchSiteAuthorsInfo(
  siteAuthorIds: number[],
  chosenPlugins: SiteAuthorFetchChoice[]
): Promise<ApiResult<SiteAuthorFetchResponse>> {
  return requireResponse(
    await AuthorInfoHandler.FetchSiteAuthorsInfo(siteAuthorIds, chosenPlugins),
    '批量拉取站点作者信息'
  )
}

/** 为本地作者设置头像（源文件为文件对话框选取的绝对路径；换头像形态先删旧） */
export async function authorInfoSetLocalAuthorAvatar(localAuthorId: number, sourceAbsPath: string): Promise<ApiResult<any>> {
  return requireResponse(await AuthorInfoHandler.SetLocalAuthorAvatar(localAuthorId, sourceAbsPath), '设置本地作者头像', false)
}

/** 移除本地作者头像（显式破坏操作，调用方二次确认） */
export async function authorInfoRemoveLocalAuthorAvatar(localAuthorId: number): Promise<ApiResult<any>> {
  return requireResponse(await AuthorInfoHandler.RemoveLocalAuthorAvatar(localAuthorId), '移除本地作者头像', false)
}
