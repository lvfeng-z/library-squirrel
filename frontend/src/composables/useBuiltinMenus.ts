import { useSlotRegistryStore, setRouterInstance, getRouterInstance } from '@renderer/store/SlotRegistryStore'
import { HomeFilled, Discount, User, Star, List, Link, TakeawayBox, Setting, Guide, Coordinate } from '@element-plus/icons-vue'
import { markRaw } from 'vue'
import type { Router } from 'vue-router'
import { ViewSlot } from '@renderer/model/slot'

/**
 * 设置 Router 实例
 * 在应用启动时调用
 */
export function initRouterInstance(router: Router) {
  setRouterInstance(router)
}

/**
 * 获取 Router 实例
 */
export function useRouterInstance(): Router | null {
  return getRouterInstance()
}

/**
 * 初始化内置菜单项
 * 在应用启动时调用
 */
export function initBuiltinMenus() {
  const store = useSlotRegistryStore()

  // 注册内置视图（主页和所有子页面）
  const builtinViews: ViewSlot[] = [
    {
      slotId: 'mainPage',
      name: '主页',
      component: () => import('@renderer/components/oneOff/MainPageWrapper.vue'),
      order: 0,
      isBuiltin: true
    },
    {
      slotId: 'localTagManage',
      name: '本地标签',
      component: () => import('@renderer/views/LocalTagManage.vue'),
      order: 10,
      isBuiltin: true
    },
    {
      slotId: 'siteTagManage',
      name: '站点标签',
      component: () => import('@renderer/views/SiteTagManage.vue'),
      order: 11,
      isBuiltin: true
    },
    {
      slotId: 'localAuthorManage',
      name: '本地作者',
      component: () => import('@renderer/views/LocalAuthorManage.vue'),
      order: 20,
      isBuiltin: true
    },
    {
      slotId: 'siteAuthorManage',
      name: '站点作者',
      component: () => import('@renderer/views/SiteAuthorManage.vue'),
      order: 21,
      isBuiltin: true
    },
    {
      slotId: 'developing',
      name: '收藏',
      component: () => import('@renderer/views/Developing.vue'),
      order: 30,
      isBuiltin: true
    },
    {
      slotId: 'taskManage',
      name: '任务',
      component: () => import('@renderer/views/TaskManage.vue'),
      order: 40,
      isBuiltin: true
    },
    {
      slotId: 'siteManage',
      name: '站点管理',
      component: () => import('@renderer/views/SiteManage.vue'),
      order: 50,
      isBuiltin: true
    },
    {
      slotId: 'siteBrowserManage',
      name: '站点浏览',
      component: () => import('@renderer/views/SiteBrowserManage.vue'),
      order: 51,
      isBuiltin: true
    },
    {
      slotId: 'pluginManage',
      name: '插件',
      component: () => import('@renderer/views/PluginManage.vue'),
      order: 60,
      isBuiltin: true
    },
    {
      slotId: 'settings',
      name: '设置',
      component: () => import('@renderer/views/Settings.vue'),
      order: 70,
      isBuiltin: true
    },
    {
      slotId: 'guide',
      name: '向导',
      component: () => import('@renderer/views/Guide.vue'),
      order: 80,
      isBuiltin: true
    },
    {
      slotId: 'test',
      name: '测试按钮',
      component: () => import('@renderer/views/Test.vue'),
      order: 90,
      isBuiltin: true
    }
  ]

  // 注册视图（使用 registerViewSlotWithRoute 确保路由被添加）
  builtinViews.forEach((view) => store.registerViewSlotWithRoute(view))

  // 批量注册内置菜单（icon 使用 markRaw 避免 reactive 警告）
  store.registerMenuSlots([
    {
      slotId: 'builtin-main',
      index: 'main',
      icon: markRaw(HomeFilled),
      label: '主页',
      order: 0,
      viewId: 'mainPage'
    },
    {
      slotId: 'builtin-tag',
      index: 'tag',
      icon: markRaw(Discount),
      label: '标签',
      order: 10,
      children: [
        {
          slotId: 'builtin-localTag',
          index: 'localTag',
          icon: markRaw(Discount),
          label: '本地标签',
          order: 11,
          viewId: 'localTagManage'
        },
        {
          slotId: 'builtin-siteTag',
          index: 'siteTag',
          icon: markRaw(Discount),
          label: '站点标签',
          order: 12,
          viewId: 'siteTagManage'
        }
      ]
    },
    {
      slotId: 'builtin-author',
      index: 'author',
      icon: markRaw(User),
      label: '作者',
      order: 20,
      children: [
        {
          slotId: 'builtin-localAuthor',
          index: 'localAuthor',
          icon: markRaw(User),
          label: '本地作者',
          order: 21,
          viewId: 'localAuthorManage'
        },
        {
          slotId: 'builtin-siteAuthor',
          index: 'siteAuthor',
          icon: markRaw(User),
          label: '站点作者',
          order: 22,
          viewId: 'siteAuthorManage'
        }
      ]
    },
    {
      slotId: 'builtin-favorite',
      index: 'favorite',
      icon: markRaw(Star),
      label: '收藏',
      order: 30,
      viewId: 'developing'
    },
    {
      slotId: 'builtin-task',
      index: 'task',
      icon: markRaw(List),
      label: '任务',
      order: 40,
      viewId: 'taskManage'
    },
    {
      slotId: 'builtin-site',
      index: 'site',
      icon: markRaw(Link),
      label: '站点',
      order: 50,
      children: [
        {
          slotId: 'builtin-siteManage',
          index: 'siteManage',
          icon: markRaw(Link),
          label: '站点管理',
          order: 51,
          viewId: 'siteManage'
        },
        {
          slotId: 'builtin-siteBrowser',
          index: 'siteBrowser',
          icon: markRaw(Link),
          label: '站点浏览',
          order: 52,
          viewId: 'siteBrowserManage'
        }
      ]
    },
    {
      slotId: 'builtin-plugin',
      index: 'plugin',
      icon: markRaw(TakeawayBox),
      label: '插件',
      order: 60,
      viewId: 'pluginManage'
    },
    {
      slotId: 'builtin-settings',
      index: 'settings',
      icon: markRaw(Setting),
      label: '设置',
      order: 70,
      viewId: 'settings'
    },
    {
      slotId: 'builtin-guide',
      index: 'guide',
      icon: markRaw(Guide),
      label: '向导',
      order: 80,
      viewId: 'guide'
    },
    {
      slotId: 'builtin-test',
      index: 'test',
      icon: markRaw(Coordinate),
      label: '测试按钮',
      order: 90,
      viewId: 'test'
    }
  ])
}
