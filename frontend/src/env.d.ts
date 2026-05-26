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

declare global {
  interface Window {
    __PLUGIN_CTX__: PluginContext
  }
}
