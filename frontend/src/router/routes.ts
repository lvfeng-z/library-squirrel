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
        component: () => import('@renderer/views/MainView.vue'),
        meta: {
          title: '主页',
          icon: 'HomeFilled',
          order: 0
        }
      },
        // initBuiltinMenus注册其他内置路由
    ],
  }
]
