import { getCurrentInstance } from 'vue'
import { ElMessageBox } from 'element-plus'
import GotoPageConfig from '@renderer/model/util/GotoPageConfig.ts'
import { PageEnum } from '@renderer/model/constant/PageEnum.ts'
import { useTourCenterStore } from '@renderer/store/UseTourCenterStore.ts'

/**
 * 获取 router 实例
 */
function getRouter() {
  // 优先从 globalProperties 获取
  const instance = getCurrentInstance()
  if (instance?.appContext.config.globalProperties.$router) {
    return instance.appContext.config.globalProperties.$router
  }
  // 备用：从 window 获取
  return (window as any).__vueRouter__
}

// PageEnum 到路由 name（viewId）的映射
const PAGE_ROUTE_NAME_MAP: Record<PageEnum, string> = {
  [PageEnum.MainPage]: 'mainPage',
  [PageEnum.SubPage]: '',
  [PageEnum.LocalTagManage]: 'localTagManage',
  [PageEnum.SiteTagManage]: 'siteTagManage',
  [PageEnum.LocalAuthorManage]: 'localAuthorManage',
  [PageEnum.SiteAuthorManage]: 'siteAuthorManage',
  [PageEnum.PluginManage]: 'pluginManage',
  [PageEnum.SiteManage]: 'siteManage',
  [PageEnum.TaskManage]: 'taskManage',
  [PageEnum.Settings]: 'settings',
  [PageEnum.Guide]: 'guide',
  [PageEnum.Developing]: 'developing',
  [PageEnum.Test]: 'test'
}

/**
 * 按 PageEnum 跳转到对应路由
 */
export async function gotoPage(page: PageEnum) {
  const routeName = PAGE_ROUTE_NAME_MAP[page]
  const router = getRouter()
  if (router && routeName) {
    await router.push({ name: routeName })
  }
}

export function askGotoPage(config: GotoPageConfig) {
  ElMessageBox.alert(config.content, config.title, config.options).then(async () => {
    await gotoPage(config.page)
  })
  if (config.page === PageEnum.Settings && (config.extraData as boolean)) {
    void useTourCenterStore().start('first-time')
  }
}
