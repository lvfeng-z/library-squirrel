import type { RouteRecordRaw } from 'vue-router'

export const routes: RouteRecordRaw[] = [
  {
    path: '/',
    name: 'MainLayout',
    component: () => import('@renderer/MainLayout.vue'),
    children: [
      {
        path: '',
        name: 'mainPage',
        component: () => import('@renderer/views/MainView.vue'),
        meta: { title: '主页', icon: 'HomeFilled', order: 0 }
      },
      {
        path: 'localTagManage',
        name: 'localTagManage',
        component: () => import('@renderer/views/LocalTagManage.vue'),
        meta: { title: '本地标签', order: 10 }
      },
      {
        path: 'siteTagManage',
        name: 'siteTagManage',
        component: () => import('@renderer/views/SiteTagManage.vue'),
        meta: { title: '站点标签', order: 11 }
      },
      {
        path: 'localAuthorManage',
        name: 'localAuthorManage',
        component: () => import('@renderer/views/LocalAuthorManage.vue'),
        meta: { title: '本地作者', order: 20 }
      },
      {
        path: 'siteAuthorManage',
        name: 'siteAuthorManage',
        component: () => import('@renderer/views/SiteAuthorManage.vue'),
        meta: { title: '站点作者', order: 21 }
      },
      {
        path: 'developing',
        name: 'developing',
        component: () => import('@renderer/views/Developing.vue'),
        meta: { title: '收藏', order: 30 }
      },
      {
        path: 'taskManage',
        name: 'taskManage',
        component: () => import('@renderer/views/TaskManage.vue'),
        meta: { title: '任务', order: 41 }
      },
      {
        path: 'siteManage',
        name: 'siteManage',
        component: () => import('@renderer/views/SiteManage.vue'),
        meta: { title: '站点管理', order: 50 }
      },
      {
        path: 'siteBrowserManage',
        name: 'siteBrowserManage',
        component: () => import('@renderer/views/SiteBrowserManage.vue'),
        meta: { title: '站点浏览', order: 51 }
      },
      {
        path: 'pluginManage',
        name: 'pluginManage',
        component: () => import('@renderer/views/PluginManage.vue'),
        meta: { title: '插件', order: 60 }
      },
      {
        path: 'settings',
        name: 'settings',
        component: () => import('@renderer/views/Settings.vue'),
        meta: { title: '设置', order: 70 }
      },
      {
        path: 'guide',
        name: 'guide',
        component: () => import('@renderer/views/Guide.vue'),
        meta: { title: '向导', order: 80 }
      },
      {
        path: 'recycleBin',
        name: 'recycleBin',
        component: () => import('@renderer/views/RecycleBin.vue'),
        meta: { title: '回收站', order: 90 }
      },
      {
        path: 'test',
        name: 'test',
        component: () => import('@renderer/views/Test.vue'),
        meta: { title: '测试按钮', order: 100 }
      }
    ]
  }
]
