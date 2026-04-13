import type { RouteRecordRaw } from 'vue-router'

export const routes: RouteRecordRaw[] = [
  {
    path: '/',
    name: 'MainLayout',
    component: () => import('@renderer/MainLayout.vue'),
    children: [
      {
        path: '',
        name: 'Home',
        component: () => import('@renderer/components/oneOff/MainPageWrapper.vue'),
        meta: {
          title: '主页',
          icon: 'HomeFilled',
          order: 0
        }
      },
      // 标签分组
      {
        path: 'tag',
        name: 'TagGroup',
        redirect: { name: 'LocalTagManage' },
        meta: {
          title: '标签',
          icon: 'Discount',
          order: 10,
          isGroup: true
        },
        children: [
          {
            path: 'local-tag',
            name: 'LocalTagManage',
            component: () => import('@renderer/views/LocalTagManage.vue'),
            meta: {
              title: '本地标签',
              icon: 'Discount',
              order: 11
            }
          },
          {
            path: 'site-tag',
            name: 'SiteTagManage',
            component: () => import('@renderer/views/SiteTagManage.vue'),
            meta: {
              title: '站点标签',
              icon: 'Discount',
              order: 12
            }
          }
        ]
      },
      // 作者分组
      {
        path: 'author',
        name: 'AuthorGroup',
        redirect: { name: 'LocalAuthorManage' },
        meta: {
          title: '作者',
          icon: 'User',
          order: 20,
          isGroup: true
        },
        children: [
          {
            path: 'local-author',
            name: 'LocalAuthorManage',
            component: () => import('@renderer/views/LocalAuthorManage.vue'),
            meta: {
              title: '本地作者',
              icon: 'User',
              order: 21
            }
          },
          {
            path: 'site-author',
            name: 'SiteAuthorManage',
            component: () => import('@renderer/views/SiteAuthorManage.vue'),
            meta: {
              title: '站点作者',
              icon: 'User',
              order: 22
            }
          }
        ]
      },
      {
        path: 'favorite',
        name: 'Favorite',
        component: () => import('@renderer/views/Developing.vue'),
        meta: {
          title: '收藏',
          icon: 'Star',
          order: 30
        }
      },
      {
        path: 'task',
        name: 'TaskManage',
        component: () => import('@renderer/views/TaskManage.vue'),
        meta: {
          title: '任务',
          icon: 'List',
          order: 40
        }
      },
      // 站点分组
      {
        path: 'site',
        name: 'SiteGroup',
        redirect: { name: 'SiteManage' },
        meta: {
          title: '站点',
          icon: 'Link',
          order: 50,
          isGroup: true
        },
        children: [
          {
            path: 'site-manage',
            name: 'SiteManage',
            component: () => import('@renderer/views/SiteManage.vue'),
            meta: {
              title: '站点管理',
              icon: 'Link',
              order: 51
            }
          },
          {
            path: 'site-browser',
            name: 'SiteBrowserManage',
            component: () => import('@renderer/views/SiteBrowserManage.vue'),
            meta: {
              title: '站点浏览',
              icon: 'Link',
              order: 52
            }
          }
        ]
      },
      {
        path: 'plugin',
        name: 'PluginManage',
        component: () => import('@renderer/views/PluginManage.vue'),
        meta: {
          title: '插件',
          icon: 'TakeawayBox',
          order: 60
        }
      },
      {
        path: 'settings',
        name: 'Settings',
        component: () => import('@renderer/views/Settings.vue'),
        meta: {
          title: '设置',
          icon: 'Setting',
          order: 70
        }
      },
      {
        path: 'guide',
        name: 'Guide',
        component: () => import('@renderer/views/Guide.vue'),
        meta: {
          title: '向导',
          icon: 'Guide',
          order: 80
        }
      },
      {
        path: 'test',
        name: 'Test',
        component: () => import('@renderer/views/Test.vue'),
        meta: {
          title: '测试按钮',
          icon: 'Coordinate',
          order: 90
        }
      }
    ]
  }
]
