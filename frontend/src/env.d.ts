/// <reference types="vite/client" />

declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<object, object, unknown>
  export default component
}

import type * as apis from './apis/http'

interface PluginContext {
  vue: typeof import('vue')
  globals: {
    $message: any
    $notify: any
    $confirm: any
    $alert: any
    $router: any
    $store: any
  }
  libs: { lodash: any }
  custom: {
    apis: typeof apis
  }
}

declare module 'vue-router' {
  interface RouteMeta {
    // 父页面路由名（页面层级单一事实源）：宿主子页在 routes.ts 静态声明，
    // 插件子页由 SlotRegistryStore 注册路由时解析声明值写入；
    // 面包屑条、后退快捷键、注销级联均沿此字段上溯
    parent?: string
  }
}

declare global {
  interface Window {
    __PLUGIN_CTX__: PluginContext
  }
}
