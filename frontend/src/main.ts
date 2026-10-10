import App from './App.vue'
import { createApp } from 'vue'
import * as Vue from 'vue'
import { createPinia } from 'pinia'
import Element from 'element-plus'
import { elementIconRegister } from './plugins/elementIcon'
import router from './router'
import BaseView from './views/BaseView.vue'
import 'element-plus/dist/index.css'
import './styles/el-tag-mimic.css'
import './styles/rounded-borders.css'
import './styles/scroll-text-left.css'
import './styles/scroll-text-center.css'
import './styles/z-axis-layers.css'
import './styles/theme/index.css'
import './styles/tone-button.css'
import clickOutSide from './directives/clickOutSide.ts'
import elSelectBottomed from './directives/elSelectBottomed.ts'
import elScrollbarBottomed from './directives/elScrollbarBottomed.ts'
import { iniListener } from '@renderer/MainIpcListener.ts'
import { initBuiltinMenus } from './composables/useBuiltinMenus.ts'
import { setRouterInstance } from './store/SlotRegistryStore.ts'
import { useTourCenterStore } from '@renderer/store/UseTourCenterStore.ts'
import { useThemeStore } from '@renderer/store/UseThemeStore.ts'
import { useViewCloseButtonStore } from '@renderer/store/UseViewCloseButtonStore.ts'
import { registerBuiltinTours } from '@renderer/tour/definitions'
import lodash from 'lodash'
import * as apis from './apis/http'
import { setupConsoleForward } from '@renderer/utils/ConsoleForward.ts'

// 开发环境：劫持 console 全级别 + 未处理异常，转发到后端 frontend.log
// 生产构建 import.meta.env.DEV 为 false，不启用（frontend.log 惰性创建，生产无此文件）
if (import.meta.env.DEV) {
  setupConsoleForward()
}

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
// 全局注册宿主视图外壳 BaseView：用途是让插件前端扩展（precompiled 产物的 SFC 模板）内写
// <BaseView> 时零 import 直接可用——模板编译产物里的 resolveComponent("BaseView") 在宿主
// 应用上下文命中本全局注册表，与 el-* 的解析同机制、同契约层（见 doc/plugin-dev-guide.md 6.3.1）。
// 组件工厂签名 (Vue, WailsRuntime) 两参不变；BaseView 提供 100% 尺寸外壳、默认插槽内容区
// 与 #dialog 具名插槽（绝对定位弹层），不采用时插件自负布局纪律。
app.component('BaseView', BaseView)

// 暴露 router 实例到全局（在 initBuiltinMenus 之前）
app.config.globalProperties.$router = router
window['__vueRouter__'] = router

// 全局兜底：捕获未被 PluginBoundary 等局部错误边界拦截的渲染错误，
// 记录日志并阻止其冒泡为未处理异常导致整页白屏（插件故障隔离的最后防线）
app.config.errorHandler = (err, _instance, info) => {
  const e = err as Error | undefined
  console.error('[GlobalErrorHandler] 未捕获的渲染错误', { info, msg: e?.message, stack: e?.stack })
}

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

  // --- 主题（供插件读取当前主题 id，延迟读取避免初始化顺序问题） ---
  theme: {
    getCurrent: () => useThemeStore().currentThemeId
  },

  // --- Custom Business Logic ---
  custom: { apis }
}

// 插件的上下文暴露到 window
// 使用深冻结 (Optional) 防止插件意外修改主程序的核心引用
// Object.freeze(pluginContext.vue)
window['__PLUGIN_CTX__'] = pluginContext

app.mount('#app')

// 初始化内置菜单（在 app.mount 之后，确保 Pinia store active；注册路由后 replace 重新匹配当前 URL）
initBuiltinMenus()

// 注册内置向导并加载已完成状态
const tourCenterStore = useTourCenterStore()
registerBuiltinTours((def) => tourCenterStore.registerTour(def))
void tourCenterStore.loadCompleted()

// 应用持久化的主题（首屏由 tokens.css 的 :root fallback 兜底，避免闪烁）
const themeStore = useThemeStore()
void themeStore.load()

// 加载视图关闭按钮开关（默认关；控制应用外壳左上角非主页视图的关闭按钮是否显示）
void useViewCloseButtonStore().load()

iniListener()
