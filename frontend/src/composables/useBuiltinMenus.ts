import { useSlotRegistryStore } from '@renderer/store/SlotRegistryStore'
import { HomeFilled, Discount, User, Star, List, Link, TakeawayBox, Setting, Guide, Coordinate, Delete } from '@element-plus/icons-vue'
import { markRaw } from 'vue'

/**
 * 初始化内置菜单项
 * 在应用启动时调用（路由已在 routes.ts 静态定义，此处只注册菜单）
 */
export function initBuiltinMenus() {
  const store = useSlotRegistryStore()

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
      slotId: 'builtin-slotTask',
      index: 'slotTask',
      icon: markRaw(List),
      label: '任务(Slot)',
      order: 41,
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
      slotId: 'builtin-recycleBin',
      index: 'recycleBin',
      icon: markRaw(Delete),
      label: '回收站',
      order: 90,
      viewId: 'recycleBin'
    },
    {
      slotId: 'builtin-test',
      index: 'test',
      icon: markRaw(Coordinate),
      label: '测试按钮',
      order: 100,
      viewId: 'test'
    }
  ])
}
