import { createPinia, getActivePinia, setActivePinia, type Pinia } from 'pinia'
import { createRouter, createMemoryHistory, type Router, type RouteRecordRaw } from 'vue-router'
import { defineComponent, type DefineComponent } from 'vue'
import { setRouterInstance, getRouterInstance, useSlotRegistryStore } from '@renderer/store/SlotRegistryStore.ts'
import type { ViewSlot, ReplaceViewSlot } from '@renderer/model/slot'

/**
 * SlotRegistryStore 条目级注册/注销、view·replaceView 动态卸载与页面层级（meta.parent）的 store 层断言集。
 *
 * 双运行形态：①vitest 用例（文末 VITEST 环境下注册，每用例独立 pinia + 内存路由）；
 * ②实机测试（live-test，dev 模式经 Vite 按需动态 import，调用导出入口
 * `runSlotRegistryStoreTests()` 采集结构化报告）。模块顶层静态 import 'vitest' 会把测试
 * 运行器拉进浏览器上下文，故 vitest 挂接仅在其环境下动态注册。
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

type SlotRegistryStore = ReturnType<typeof useSlotRegistryStore>

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

function pluginViewSlot(pluginPublicId: string, extensionId: string, parent?: string): ViewSlot {
  const loader = (): Promise<DefineComponent> => Promise.resolve(stubComponent(`${extensionId}-view`))
  return {
    slotId: `${pluginPublicId}/${extensionId}`,
    name: extensionId,
    component: loader,
    order: 50,
    isPlugin: true,
    parent
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

// 捕获 console.warn 输出（降级告警断言用；用毕须 restore 恢复原函数）
function captureConsoleWarn(): { warnings: string[]; restore: () => void } {
  const warnings: string[] = []
  const originalWarn = console.warn
  console.warn = (...args: unknown[]) => {
    warnings.push(args.map(String).join(' '))
  }
  return { warnings, restore: () => (console.warn = originalWarn) }
}

// 独立测试环境：专属 pinia + 内存路由（复刻 MainLayout 骨架），执行后由 releaseTestEnv 恢复现场
interface TestEnv {
  store: SlotRegistryStore
  router: Router
  previousPinia: Pinia | undefined
  previousRouter: Router | null
}

async function createTestEnv(): Promise<TestEnv> {
  const previousPinia = getActivePinia()
  const previousRouter = getRouterInstance()
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
  return { store: useSlotRegistryStore(), router, previousPinia, previousRouter }
}

function releaseTestEnv(env: TestEnv): void {
  setRouterInstance(env.previousRouter)
  if (env.previousPinia) {
    setActivePinia(env.previousPinia)
  }
}

// ---- 断言用例 ----

// view 注册：复合键路由挂到 MainLayout 下，store 桶就位
async function viewRegisterAddsCompositeRoute(store: SlotRegistryStore, router: Router) {
  const slot = pluginViewSlot('com.plugin.a', 'view-1')
  store.registerViewSlot(slot)
  const route = routeOf(router, slot.slotId)
  assert(route !== undefined, 'view 注册后应存在同名路由')
  assert(routeComponent(router, slot.slotId) === slot.component, 'view 路由 component 应为插槽组件加载器')
  assert(routeOf(router, 'MainLayout') !== undefined, 'MainLayout 骨架不应被破坏')
  assert(store.viewSlots.has(slot.slotId), 'viewSlots 桶应含该复合键')
}

// view 动态注销：用户当前停留在被注销页面时跳默认页（路由守卫断言），路由与 store 桶清除
async function viewUnregisterRedirectsWhenCurrent(store: SlotRegistryStore, router: Router) {
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
  store: SlotRegistryStore,
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

// view 注册：parent 声明同插件裸 extensionId（父页面先在场），注册时解析为同插件复合路由名写入 meta.parent
async function parentWrittenAsSamePluginCompositeName(store: SlotRegistryStore, router: Router) {
  const parent = pluginViewSlot('com.plugin.h', 'parent-view')
  const child = pluginViewSlot('com.plugin.h', 'child-view', 'parent-view')
  store.registerViewSlot(parent)
  store.registerViewSlot(child)
  assert(
    routeOf(router, child.slotId)?.meta?.parent === 'com.plugin.h/parent-view',
    '同插件父页面声明应解析为复合路由名写入 meta.parent'
  )
}

// view 注册：parent 声明宿主路由名，注册时原样写入 meta.parent
async function parentWrittenAsHostRouteName(store: SlotRegistryStore, router: Router) {
  const child = pluginViewSlot('com.plugin.h', 'host-child', 'taskManage')
  store.registerViewSlot(child)
  assert(
    routeOf(router, child.slotId)?.meta?.parent === 'taskManage',
    '宿主路由名形态的 parent 应原样写入 meta.parent'
  )
}

// view 注册：parent 与宿主路由名同名重叠（本插件恰有同名 view 条目）时，同插件复合名优先（确定顺序）
async function parentSamePluginCompositeWinsOverHostRoute(store: SlotRegistryStore, router: Router) {
  store.registerViewSlot(pluginViewSlot('com.plugin.h', 'taskManage'))
  const child = pluginViewSlot('com.plugin.h', 'overlap-child', 'taskManage')
  store.registerViewSlot(child)
  assert(
    routeOf(router, child.slotId)?.meta?.parent === 'com.plugin.h/taskManage',
    '同名重叠时同插件复合名应优先于宿主路由名'
  )
}

// view 注册：注册顺序不定（后端注册中心按 map 遍历推送扩展，同插件父页面可能晚于子页面到达）——
// 子页注册时两形态均未命中而降级，同插件父页面到场后补写其 meta.parent
async function lateParentRegistrationBackfillsHierarchy(store: SlotRegistryStore, router: Router) {
  const child = pluginViewSlot('com.plugin.h', 'late-child', 'late-parent')
  store.registerViewSlot(child)
  assert(routeOf(router, child.slotId)?.meta?.parent === undefined, '父页面未在场时子页应降级为无层级注册')

  store.registerViewSlot(pluginViewSlot('com.plugin.h', 'late-parent'))
  assert(
    routeOf(router, child.slotId)?.meta?.parent === 'com.plugin.h/late-parent',
    '同插件父页面到场后应补写子页 meta.parent'
  )
}

// view 注册：parent 宿主名不可解析 → 降级为无层级注册（页面照常可用：路由与桶就位）+ console.warn 告警
async function unresolvableParentDegradesAndStillRegisters(store: SlotRegistryStore, router: Router) {
  const slot = pluginViewSlot('com.plugin.h', 'orphan-child', 'no-such-host-route')
  const capture = captureConsoleWarn()
  try {
    store.registerViewSlot(slot)
  } finally {
    capture.restore()
  }
  const route = routeOf(router, slot.slotId)
  assert(route !== undefined, '降级页面的路由应照常注册')
  assert(routeComponent(router, slot.slotId) === slot.component, '降级页面路由 component 应为插槽组件加载器')
  assert(route?.meta?.parent === undefined, '不可解析的 parent 不得写入 meta.parent')
  assert(store.viewSlots.has(slot.slotId), '降级页面应留在 viewSlots 桶')
  assert(capture.warnings.some((warning) => warning.includes(slot.slotId)), '降级注册应产出 console.warn 告警')
}

// replaceView 注册：覆盖目标路由 component，原始路由（component + meta）被记录供恢复
async function replaceViewRegisterOverridesAndRecordsOriginal(
  store: SlotRegistryStore,
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
  store: SlotRegistryStore,
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
  store: SlotRegistryStore,
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
  store: SlotRegistryStore,
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

// 注销级联：正停在孙页面时整批注销孙/子两级（子页面 parent 指向宿主 taskManage），
// 同批消亡的子页面跳过，落第一个幸存祖先（宿主页面）。批量到达顺序不定（后端 map 遍历），
// 两种顺序均须落幸存者
async function unregisterCascadeSkipsDyingAncestors(store: SlotRegistryStore, router: Router) {
  const batchOrders = [
    ['com.plugin.h/grandchild-view', 'com.plugin.h/mid-view'],
    ['com.plugin.h/mid-view', 'com.plugin.h/grandchild-view']
  ]
  for (const batchOrder of batchOrders) {
    const mid = pluginViewSlot('com.plugin.h', 'mid-view', 'taskManage')
    const grandchild = pluginViewSlot('com.plugin.h', 'grandchild-view', 'mid-view')
    store.registerViewSlot(mid)
    store.registerViewSlot(grandchild)
    await router.push(`/${grandchild.slotId}`)
    assert(router.currentRoute.value.name === grandchild.slotId, '前置条件：当前位于孙页面')

    store.unregisterViewSlots(batchOrder)
    await waitFor(() => router.currentRoute.value.name === 'taskManage')
    assert(routeOf(router, grandchild.slotId) === undefined, '批量注销后孙页面路由应被移除')
    assert(routeOf(router, mid.slotId) === undefined, '批量注销后子页面路由应被移除')
    assert(!store.viewSlots.has(grandchild.slotId) && !store.viewSlots.has(mid.slotId), '批量注销后桶应清除两级复合键')
  }
}

// 注销级联：链上无幸存祖先（两级同批消亡且父页面无上级）→ 落主页
async function unregisterCascadeAllDeadFallsHome(store: SlotRegistryStore, router: Router) {
  const root = pluginViewSlot('com.plugin.h', 'root-view')
  const leaf = pluginViewSlot('com.plugin.h', 'leaf-view', 'root-view')
  store.registerViewSlot(root)
  store.registerViewSlot(leaf)
  await router.push(`/${leaf.slotId}`)

  store.unregisterViewSlots([leaf.slotId, root.slotId])
  await waitFor(() => router.currentRoute.value.name === 'mainPage')
  assert(routeOf(router, leaf.slotId) === undefined && routeOf(router, root.slotId) === undefined, '全灭批次路由均应移除')
}

// 注销级联（单条注销，设置驱动的条目级热切换）：正停在被注销页面 → 落紧邻父页面（宿主父页面存活）
async function unregisterSingleLandsOnSurvivingParent(store: SlotRegistryStore, router: Router) {
  const child = pluginViewSlot('com.plugin.h', 'single-child', 'taskManage')
  store.registerViewSlot(child)
  await router.push(`/${child.slotId}`)

  store.unregisterViewSlot(child.slotId)
  await waitFor(() => router.currentRoute.value.name === 'taskManage')
  assert(routeOf(router, child.slotId) === undefined, '注销后路由应被移除')
}

// ---- 用例登记 ----

const testCases: Array<[string, (store: SlotRegistryStore, router: Router) => Promise<void>]> = [
  ['view 注册：复合键路由挂 MainLayout 下', viewRegisterAddsCompositeRoute],
  ['view 动态注销：当前页跳默认页 + 路由与桶清除', viewUnregisterRedirectsWhenCurrent],
  ['view 动态注销：非当前页不跳转', viewUnregisterKeepsCurrentWhenOtherRoute],
  ['meta.parent 写入：同插件复合名形态', parentWrittenAsSamePluginCompositeName],
  ['meta.parent 写入：宿主路由名形态', parentWrittenAsHostRouteName],
  ['meta.parent 写入：同名重叠时同插件复合名优先', parentSamePluginCompositeWinsOverHostRoute],
  ['meta.parent 补写：同插件父页面晚于子页面到达', lateParentRegistrationBackfillsHierarchy],
  ['meta.parent 降级：宿主名不可解析时无层级且页面照常注册', unresolvableParentDegradesAndStillRegisters],
  ['replaceView 注册：覆盖路由并记录原始路由', replaceViewRegisterOverridesAndRecordsOriginal],
  ['replaceView 动态注销：恢复原 component 与 meta', replaceViewUnregisterRestoresOriginalRoute],
  ['同 target 多插件：最近启用动作胜 + 逐层回落', replaceViewSameTargetLatestRegistrationWins],
  ['热切换重启用：重新注册即接管', replaceViewReRegisterTakesOverAgain],
  ['注销级联：同批消亡祖先跳过落幸存者（两种到达顺序）', unregisterCascadeSkipsDyingAncestors],
  ['注销级联：链上全灭落主页', unregisterCascadeAllDeadFallsHome],
  ['注销级联：单条注销落紧邻父页面', unregisterSingleLandsOnSurvivingParent]
]

// ---- 运行入口 ----

/**
 * 执行全部断言并返回结构化报告（JSON 可序列化，供实机测试采集）。
 * 在独立 pinia + 内存路由上运行，结束后恢复原 active pinia 与模块级 router 实例
 */
export async function runSlotRegistryStoreTests(): Promise<SlotRegistryTestReport> {
  const report: SlotRegistryTestReport = { total: 0, passed: 0, failed: 0, results: [] }
  const env = await createTestEnv()

  try {
    for (const [name, testCase] of testCases) {
      report.total++
      try {
        await testCase(env.store, env.router)
        report.passed++
        report.results.push({ name, ok: true })
      } catch (error) {
        report.failed++
        report.results.push({ name, ok: false, detail: error instanceof Error ? error.message : String(error) })
      }
    }
  } finally {
    releaseTestEnv(env)
  }

  return report
}

// vitest 运行器挂接：每用例独立环境执行同一断言集（live-test 走上述导出入口，两形态共用用例）
if (import.meta.env.VITEST) {
  const { describe, it } = await import('vitest')
  describe('SlotRegistryStore 条目注册/注销与页面层级', () => {
    for (const [name, testCase] of testCases) {
      it(name, async () => {
        const env = await createTestEnv()
        try {
          await testCase(env.store, env.router)
        } finally {
          releaseTestEnv(env)
        }
      })
    }
  })
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
