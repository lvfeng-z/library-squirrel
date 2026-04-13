import App from './App.vue'
import { createApp } from 'vue'
import * as Vue from 'vue'
import { createPinia } from 'pinia'
import Element from 'element-plus'
import { elementIconRegister } from './plugins/elementIcon'
import router from './router'
import 'element-plus/dist/index.css'
import './styles/el-tag-mimic.css'
import './styles/rounded-borders.css'
import './styles/scroll-text-left.css'
import './styles/scroll-text-center.css'
import './styles/z-axis-layers.css'
import clickOutSide from './directives/clickOutSide.ts'
import elSelectBottomed from './directives/elSelectBottomed.ts'
import elScrollbarBottomed from './directives/elScrollbarBottomed.ts'
import { iniListener } from '@renderer/MainIpcListener.ts'
import { initBuiltinMenus } from './composables/useBuiltinMenus.ts'
import { setRouterInstance } from './store/SlotRegistryStore.ts'
import lodash from 'lodash'

const app = createApp(App)
const pinia = createPinia()
app.use(Element)
app.use(pinia)
app.use(router)
// 全局注册 el-icon
elementIconRegister(app)
// 注册点击外部事件的自定义指令
app.directive('clickOutSide', clickOutSide)
// 注册el-select触底的的自定义指令
app.directive('elSelectBottomed', elSelectBottomed)
// 注册el-scrollbar触底的自定义指令
app.directive('elScrollbarBottomed', elScrollbarBottomed)

// 暴露 router 实例到全局（在 initBuiltinMenus 之前）
app.config.globalProperties.$router = router
window['__vueRouter__'] = router

// 设置 router 实例到 store（在 router 初始化后）
setRouterInstance(router)

// 构建插件的上下文对象
const pluginContext = {
  // --- Vue Core ---
  vue: Vue,

  // --- Globals (从 app 实例提取) ---
  globals: {
    // 直接从 globalProperties 拿，这是最稳妥的
    $message: app.config.globalProperties.$message,
    $notify: app.config.globalProperties.$notify,
    $confirm: app.config.globalProperties.$confirm, // 注意：ElementPlus 默认可能是 $alert 或 ElMessageBox，需确认
    $alert: app.config.globalProperties.$alert,

    // 路由
    $router: router,
    // 注意：$route 是动态的，通常插件内部用 useRoute() 获取，这里可以不放，或者放个 getter
    // $route: router.currentRoute, // 不推荐直接放静态引用

    // 状态管理
    $store: pinia // 如果是 Pinia
  },

  // --- Third-party Libs ---
  libs: {
    lodash: lodash
  },

  // --- Custom Business Logic ---
  custom: {}
}

// 插件的上下文暴露到 window
// 使用深冻结 (Optional) 防止插件意外修改主程序的核心引用
// Object.freeze(pluginContext.vue)
window['__PLUGIN_CTX__'] = pluginContext

app.mount('#app')

// 初始化内置菜单（在 pinia store 初始化之后）
initBuiltinMenus()

iniListener()
