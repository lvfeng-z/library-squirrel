/**
 * AuthorInfo HTTP API 包装器
 * 封装 Wails 绑定层响应校验，校验失败时抛出异常，调用方通过 try/catch 捕获
 */

import { Handler as AuthorInfoHandler, SiteAuthorFetchResponse } from '@bindings/github.com/library-squirrel/backend/authorInfo'
import type { ApiResponse as WailsApiResponse } from '@bindings/github.com/library-squirrel/backend/base/model'
import type { SiteAuthorFetchChoice } from '@bindings/github.com/library-squirrel/backend/base/model/dto'
import type { ApiResult } from '@renderer/apis/http/types'
import { requireResponse } from '@renderer/apis/http/types'
import { arrayIsEmpty, notNullish } from '@renderer/utils/CommonUtil.ts'

// ========== 内部工具 ==========

/**
 * 拉取响应收口：候选冲突态由后端以外层失败表达（msg 为引导语，未调用任何插件），
 * 而 requireResponse 遇失败即抛错并丢弃数据载荷，故冲突态把载荷原样交回调用方，
 * 由其弹出插件选择器后带显选键重发；其余形态照旧走 requireResponse。
 */
function resolveFetchResponse(
  response: WailsApiResponse<SiteAuthorFetchResponse | null> | null,
  operation: string
): ApiResult<SiteAuthorFetchResponse> {
  const payload = response?.data
  if (notNullish(payload) && !arrayIsEmpty(payload.conflicts?.filter(notNullish) ?? [])) {
    return { success: true, msg: response?.msg ?? '', data: payload }
  }
  return requireResponse(response, operation)
}

// ========== API 方法 ==========

/**
 * 手动拉取单个站点作者信息（行操作，loading 态由调用方挂起）
 *
 * chosenPlugins 为交互面显选（站点键 → 插件，空 = 未显选）：候选按站点收窄，该站点候选多于一个
 * 且未显选时载荷的 conflicts 非空——至多一组，candidates 为该站点的候选清单（首位即默认选中项，
 * 顺序由后端给定不再重排）
 */
export async function authorInfoFetchSiteAuthorInfo(
  siteAuthorId: number,
  chosenPlugins: SiteAuthorFetchChoice[]
): Promise<ApiResult<SiteAuthorFetchResponse>> {
  return resolveFetchResponse(
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
  return resolveFetchResponse(
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
