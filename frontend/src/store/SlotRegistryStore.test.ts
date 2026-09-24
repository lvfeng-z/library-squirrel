import { createPinia, getActivePinia, setActivePinia, type Pinia } from 'pinia'
import { createRouter, createMemoryHistory, type Router, type RouteRecordRaw } from 'vue-router'
import { defineComponent, type DefineComponent } from 'vue'
import { setRouterInstance, getRouterInstance, useSlotRegistryStore } from '@renderer/store/SlotRegistryStore.ts'
import type { ViewSlot, ReplaceViewSlot } from '@renderer/model/slot'

/**
 * SlotRegistryStore 条目级注册/注销与 view·replaceView 动态卸载的 store 层断言集。
 *
 * 项目前端无单测运行器：本模块为零依赖的可执行断言集——经 vue-tsc 做类型检查（开发期
 * 常规验证），断言本体导出单一入口 `runSlotRegistryStoreTests()`，由实机测试（live-test，
 * dev 模式经 Vite 按需加载 /src/store/SlotRegistryStore.test.ts）或未来接入的测试运行器
 * 调用执行。模块导入无副作用，不进入生产 bundle。
 *
 * 测试环境使用独立 pinia 实例 + 内存历史路由（内建路由表以最小桩复刻 MainLayout 骨架，
 * 不导入真实视图组件），执行后恢复原 active pinia 与模块级 router 实例。
 */

interface TestResult {
  name: string
  ok: boolean
  detail?: string
}

export interface SlotRegistryTestReport {
  total: number
  passed: number
  failed: number
  results: TestResult[]
}

// 组件桩：以统一的裸 DefineComponent 类型收敛，避免插槽加载器类型与默认泛型不合
function stubComponent(name: string): DefineComponent {
  return defineComponent({ name, render: () => null }) as unknown as DefineComponent
}

// 内建路由桩：复刻 MainLayout 骨架（主页默认子路由 + 一个被替换目标路由）
const builtinMainPage = stubComponent('mainPage-stub')
const builtinTaskManage = stubComponent('taskManage-stub')

function builtinRoutes(): RouteRecordRaw[] {
  return [
    {
      path: '/',
      name: 'MainLayout',
      component: stubComponent('main-layout-stub'),
      children: [
        { path: '', name: 'mainPage', component: builtinMainPage, meta: { title: '主页', order: 0 } },
        { path: 'taskManage', name: 'taskManage', component: builtinTaskManage, meta: { title: '任务', order: 41 } }
      ]
    }
  ]
}

function pluginViewSlot(pluginPublicId: string, extensionId: string): ViewSlot {
  const loader = (): Promise<DefineComponent> => Promise.resolve(stubComponent(`${extensionId}-view`))
  return {
    slotId: `${pluginPublicId}/${extensionId}`,
    name: extensionId,
    component: loader,
    order: 50,
    isPlugin: true
  }
}

function pluginReplaceViewSlot(pluginPublicId: string, extensionId: string, target: string): ReplaceViewSlot {
  const loader = (): Promise<DefineComponent> => Promise.resolve(stubComponent(`${extensionId}-replace`))
  return {
    slotId: `${pluginPublicId}/${extensionId}`,
    target,
    component: loader
  }
}

function assert(condition: boolean, message: string): void {
  if (!condition) {
    throw new Error(message)
  }
}

// 轮询等待异步导航收敛（注销动作内部的 push 不返回 Promise，此处按条件轮询）
async function waitFor(condition: () => boolean, timeoutMs = 1000): Promise<void> {
  const start = Date.now()
  while (!condition()) {
    if (Date.now() - start > timeoutMs) {
      throw new Error('等待条件超时')
    }
    await new Promise((resolve) => setTimeout(resolve, 10))
  }
}

function routeOf(router: Router, name: string) {
  return router.getRoutes().find((r) => r.name === name)
}

// 路由 component 取值（目标可能不存在或无 component）
function routeComponent(router: Router, name: string): unknown {
  return routeOf(router, name)?.components?.default
}

// ---- 断言用例 ----

// view 注册：复合键路由挂到 MainLayout 下，store 桶就位
async function viewRegisterAddsCompositeRoute(store: ReturnType<typeof useSlotRegistryStore>, router: Router) {
  const slot = pluginViewSlot('com.plugin.a', 'view-1')
  store.registerViewSlot(slot)
  const route = routeOf(router, slot.slotId)
  assert(route !== undefined, 'view 注册后应存在同名路由')
  assert(routeComponent(router, slot.slotId) === slot.component, 'view 路由 component 应为插槽组件加载器')
  assert(routeOf(router, 'MainLayout') !== undefined, 'MainLayout 骨架不应被破坏')
  assert(store.viewSlots.has(slot.slotId), 'viewSlots 桶应含该复合键')
}

// view 动态注销：用户当前停留在被注销页面时跳默认页（路由守卫断言），路由与 store 桶清除
async function viewUnregisterRedirectsWhenCurrent(store: ReturnType<typeof useSlotRegistryStore>, router: Router) {
  const slot = pluginViewSlot('com.plugin.a', 'view-2')
  store.registerViewSlot(slot)
  await router.push(`/${slot.slotId}`)
  assert(router.currentRoute.value.name === slot.slotId, '前置条件：当前位于插件 view 页面')

  store.switchView(slot.slotId)
  store.unregisterViewSlot(slot.slotId)
  await waitFor(() => router.currentRoute.value.name === 'mainPage')
  assert(routeOf(router, slot.slotId) === undefined, '注销后复合键路由应被移除')
  assert(!store.viewSlots.has(slot.slotId), '注销后 viewSlots 桶应清除该复合键')
  assert(store.activeViewId === null, '注销激活中的视图应清空 activeViewId')
}

// view 动态注销：用户停留在其他页面时不发生跳转
async function viewUnregisterKeepsCurrentWhenOtherRoute(
  store: ReturnType<typeof useSlotRegistryStore>,
  router: Router
) {
  const slot = pluginViewSlot('com.plugin.a', 'view-3')
  store.registerViewSlot(slot)
  await router.push('/')
  store.unregisterViewSlot(slot.slotId)
  await new Promise((resolve) => setTimeout(resolve, 30))
  assert(router.currentRoute.value.name === 'mainPage', '非当前页面注销不应触发跳转')
  assert(routeOf(router, slot.slotId) === undefined, '注销后路由应被移除')
}

// replaceView 注册：覆盖目标路由 component，原始路由（component + meta）被记录供恢复
async function replaceViewRegisterOverridesAndRecordsOriginal(
  store: ReturnType<typeof useSlotRegistryStore>,
  router: Router
) {
  const slot = pluginReplaceViewSlot('com.plugin.a', 'replace-1', 'taskManage')
  store.registerReplaceViewSlot(slot)
  const route = routeOf(router, 'taskManage')
  assert(route !== undefined, '替换后目标路由应存在')
  assert(routeComponent(router, 'taskManage') === slot.component, '目标路由 component 应为替换组件加载器')
  assert(route?.meta?.replaced === true, '替换路由 meta 应带 replaced 标记')
  assert(route?.meta?.title === '任务', '替换路由应保留原 meta 标题')
  const original = store.originalRouteRecords.get('taskManage')
  assert(original !== undefined, '首次覆盖应记录原始路由')
  assert(original?.component === builtinTaskManage, '原始记录应为内建组件')
  assert(original?.meta?.title === '任务', '原始记录应含内建 meta')
  // 用例收尾清理：注销本用例的覆盖者，恢复内建路由——后续用例（注销恢复语义）以
  // 「无存活覆盖者」为前置，残留覆盖者会按最近启用动作胜语义接管目标路由
  store.unregisterReplaceViewSlot(slot.slotId)
}

// replaceView 动态注销：恢复主程序原路由（component 与 meta 均复原）
async function replaceViewUnregisterRestoresOriginalRoute(
  store: ReturnType<typeof useSlotRegistryStore>,
  router: Router
) {
  const slot = pluginReplaceViewSlot('com.plugin.a', 'replace-2', 'taskManage')
  store.registerReplaceViewSlot(slot)
  store.unregisterReplaceViewSlot(slot.slotId)
  const route = routeOf(router, 'taskManage')
  assert(route !== undefined, '注销后目标路由应仍存在（恢复原路由而非删除）')
  assert(routeComponent(router, 'taskManage') === builtinTaskManage, '注销后应恢复内建 component')
  assert(route?.meta?.title === '任务', '注销后应恢复内建 meta')
  assert(route?.meta?.replaced === undefined, '注销后不应残留 replaced 标记')
  assert(!store.originalRouteRecords.has('taskManage'), '恢复后原始记录应清除')
  assert(!store.replaceViewSlots.has(slot.slotId), '注销后 replaceViewSlots 桶应清除该复合键')
}

// 同 target 多插件热切换接管 = 最近启用动作胜：后注册者接管；最近者注销后由剩余覆盖者中
// 最近注册者接管；全部注销后恢复原路由
async function replaceViewSameTargetLatestRegistrationWins(
  store: ReturnType<typeof useSlotRegistryStore>,
  router: Router
) {
  const slotA = pluginReplaceViewSlot('com.plugin.a', 'replace-a', 'taskManage')
  const slotB = pluginReplaceViewSlot('com.plugin.b', 'replace-b', 'taskManage')
  store.registerReplaceViewSlot(slotA)
  store.registerReplaceViewSlot(slotB)
  assert(routeComponent(router, 'taskManage') === slotB.component, '后注册者 B 应接管目标路由')

  store.unregisterReplaceViewSlot(slotB.slotId)
  assert(
    routeComponent(router, 'taskManage') === slotA.component,
    'B 注销后应由剩余覆盖者中最近注册的 A 接管，而非直接回原路由'
  )
  assert(store.originalRouteRecords.has('taskManage'), 'A 仍在覆盖时原始记录应保留')

  store.unregisterReplaceViewSlot(slotA.slotId)
  assert(routeComponent(router, 'taskManage') === builtinTaskManage, '全部覆盖者注销后应恢复原路由')
  assert(!store.originalRouteRecords.has('taskManage'), '全部注销后原始记录应清除')
}

// 热切换重启用：注销后重新注册即重新接管（最近启用动作胜），原始路由记录重建正确
async function replaceViewReRegisterTakesOverAgain(
  store: ReturnType<typeof useSlotRegistryStore>,
  router: Router
) {
  const slot = pluginReplaceViewSlot('com.plugin.a', 'replace-3', 'taskManage')
  store.registerReplaceViewSlot(slot)
  store.unregisterReplaceViewSlot(slot.slotId)
  assert(routeComponent(router, 'taskManage') === builtinTaskManage, '前置条件：已恢复原路由')

  store.registerReplaceViewSlot(slot)
  assert(routeComponent(router, 'taskManage') === slot.component, '重新注册应重新接管')
  const original = store.originalRouteRecords.get('taskManage')
  assert(original?.component === builtinTaskManage, '重接管时原始记录应重建为内建组件（非前任插件组件）')
}

// ---- 运行入口 ----

/**
 * 执行全部断言并返回结构化报告（JSON 可序列化，供实机测试采集）。
 * 在独立 pinia + 内存路由上运行，结束后恢复原 active pinia 与模块级 router 实例
 */
export async function runSlotRegistryStoreTests(): Promise<SlotRegistryTestReport> {
  const previousPinia = getActivePinia()
  const previousRouter = getRouterInstance()
  const report: SlotRegistryTestReport = { total: 0, passed: 0, failed: 0, results: [] }

  const testPinia: Pinia = createPinia()
  setActivePinia(testPinia)
  const router: Router = createRouter({
    history: createMemoryHistory(),
    routes: builtinRoutes()
  })
  setRouterInstance(router)
  // vue-router 5：未安装进应用的 router 初始导航惰性启动，须显式 push 触发后 isReady 才收敛
  // （单等 isReady 永远挂起——v4 语义在此版本不成立）
  await router.push('/')
  await router.isReady()

  try {
    const store = useSlotRegistryStore()
    const cases: Array<[string, (store: ReturnType<typeof useSlotRegistryStore>, router: Router) => Promise<void>]> = [
      ['view 注册：复合键路由挂 MainLayout 下', viewRegisterAddsCompositeRoute],
      ['view 动态注销：当前页跳默认页 + 路由与桶清除', viewUnregisterRedirectsWhenCurrent],
      ['view 动态注销：非当前页不跳转', viewUnregisterKeepsCurrentWhenOtherRoute],
      ['replaceView 注册：覆盖路由并记录原始路由', replaceViewRegisterOverridesAndRecordsOriginal],
      ['replaceView 动态注销：恢复原 component 与 meta', replaceViewUnregisterRestoresOriginalRoute],
      ['同 target 多插件：最近启用动作胜 + 逐层回落', replaceViewSameTargetLatestRegistrationWins],
      ['热切换重启用：重新注册即接管', replaceViewReRegisterTakesOverAgain]
    ]
    for (const [name, testCase] of cases) {
      report.total++
      try {
        await testCase(store, router)
        report.passed++
        report.results.push({ name, ok: true })
      } catch (error) {
        report.failed++
        report.results.push({ name, ok: false, detail: error instanceof Error ? error.message : String(error) })
      }
    }
  } finally {
    setRouterInstance(previousRouter)
    if (previousPinia) {
      setActivePinia(previousPinia)
    }
  }

  return report
}

// 供浏览器控制台手工执行（live-test 亦经动态 import 调用 runSlotRegistryStoreTests）
declare global {
  interface Window {
    __runSlotRegistryStoreTests?: () => Promise<SlotRegistryTestReport>
  }
}

if (typeof window !== 'undefined') {
  window.__runSlotRegistryStoreTests = runSlotRegistryStoreTests
}
