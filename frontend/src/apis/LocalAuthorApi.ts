import ApiUtil from '@renderer/utils/ApiUtil.ts'
import IPage from '@renderer/model/util/IPage.ts'
import SelectItem from '@renderer/model/util/SelectItem.ts'
import Page from '@renderer/model/util/Page.ts'
import { isNullish } from '@renderer/utils/CommonUtil.ts'
import { ElMessage } from 'element-plus'
import { localAuthorApi } from '@renderer/apis/http'

/**
 * 分页查询站点作者选择列表
 * @param page
 * @param authorName
 */
export async function localAuthorQuerySelectItemPageByName(
  page: IPage<unknown, SelectItem>,
  authorName: string
): Promise<IPage<unknown, SelectItem>> {
  page.query = { authorName }
  return localAuthorQuerySelectItemPage(page)
}

/**
 * 分页查询站点作者选择列表
 * @param page
 */
export async function localAuthorQuerySelectItemPage(page: IPage<unknown, SelectItem>): Promise<IPage<unknown, SelectItem>> {
  const response = await localAuthorApi.localAuthorQuerySelectItemPage({
    page: page.pageNumber,
    pageSize: page.pageSize,
    query: page.query as Record<string, unknown> | undefined
  })

  // 解析响应值
  if (ApiUtil.check(response)) {
    const newPage = ApiUtil.data<Page<unknown, SelectItem>>(response)
    if (isNullish(newPage)) {
      const msg = '分页查询站点作者选择列表，没有返回分页数据'
      ElMessage({ message: msg, type: 'error' })
      throw new Error(msg)
    }
    return newPage
  } else {
    ApiUtil.failedMsg(response)
    throw new Error()
  }
}
