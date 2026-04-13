import { getCurrentInstance } from 'vue'
import { ElMessageBox } from 'element-plus'
import GotoPageConfig from '@renderer/model/util/GotoPageConfig.ts'
import { useTourStatesStore } from '@renderer/store/UseTourStatesStore.ts'

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

/**
 * 导航到指定路径
 */
export async function gotoPath(path: string) {
  const router = getRouter()
  if (router) {
    await router.push(path)
  }
}

export function askGotoPage(config: GotoPageConfig) {
  ElMessageBox.alert(config.content, config.title, config.options).then(async () => {
    await gotoPath(config.path)
  })
  switch (config.path) {
    case '/settings':
      if (config.extraData as boolean) {
        useTourStatesStore().tourStates.startWorkdirTour()
      }
  }
}

/**
 * 获取当前路由路径
 */
export function getCurrentPath(): string | null {
  const router = getRouter()
  if (!router) return null

  return router.currentRoute.value.path
}
