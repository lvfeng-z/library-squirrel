import { describe, it, expect } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, getActivePinia, setActivePinia, type Pinia } from 'pinia'
import { createMemoryHistory, createRouter, RouterView, type RouteRecordRaw, type Router } from 'vue-router'
import {
  KeepAlive,
  cloneVNode,
  defineComponent,
  getCurrentInstance,
  h,
  onActivated,
  onDeactivated,
  onUnmounted,
  ref,
  type Component,
  type DefineComponent,
  type VNode
} from 'vue'
import {
  setRouterInstance,
  getRouterInstance,
  useSlotRegistryStore,
  pluginViewBoundaryName
} from '@renderer/store/SlotRegistryStore'
import type { ViewSlot } from '@renderer/model/slot'
import MainView from '@renderer/views/MainView.vue'
import { routes as appRoutes } from './router/routes'

/**
 * MainLayout 视图宿主的 keep-alive 渲染形态组件级验证。
 *
 * 被测组合（宿主视图保活机制的首验点）：keep-alive include 按组件名匹配 × 插件 view 经
 * 异步懒加载路由组件（loader 每次调用产出新建的 wrapper 组件对象）。断言走真实 RouterView +
 * 真实 KeepAlive 渲染链，include 白名单取真实 store 派生（插件声明源 + 宿主路由 meta 静态源
 * 双源合流），wrapper 名与 include 派生共用真实 pluginViewBoundaryName——字符串匹配路径
 * 全程真实，非类型检查。
 *
 * 渲染形态以最小夹具复刻 MainLayout.vue 的视图宿主区域（router-view 插槽 + 常驻 keep-alive +
 * include 绑定 store getter）：真实 MainLayout 依赖 Element Plus 全家与多个 store 的挂载期
 * 副作用，挂载真实组件的桩面噪声远超收益，而本用例要证的是渲染组合语义本身。
 */

type SlotRegistryStore = ReturnType<typeof useSlotRegistryStore>

// 状态化视图探针：记录挂载次数、实例 uid 与生命周期事件序列；本地状态（计数）经点击自增
function statefulView(name: string) {
  const events: string[] = []
  const uids: number[] = []
  let mountCount = 0
  const component = defineComponent({
    name,
    setup() {
      const uid = getCurrentInstance()!.uid
      const count = ref(0)
      uids.push(uid)
      mountCount++
      events.push(`mounted:${uid}`)
      onActivated(() => events.push(`activated:${uid}`))
      onDeactivated(() => events.push(`deactivated:${uid}`))
      onUnmounted(() => events.push(`unmounted:${uid}`))
      return () =>
        h('div', {
          class: `probe-${name}`,
          'data-count': String(count.value),
          'data-uid': String(uid),
          onClick: () => {
            count.value++
          }
        })
    }
  })
  return { component, events, uids: () => [...uids], mountCount: () => mountCount }
}

function countOf(events: string[], prefix: 'mounted' | 'activated' | 'deactivated' | 'unmounted'): number {
  return events.filter((entry) => entry.startsWith(prefix)).length
}

// 插件 view 组件加载器夹具：复刻 useSlotSyncListener.wrapWithBoundary 的产物形态——
// 异步 loader 每次调用都新建名为 PluginViewBoundary:<slotId> 的 wrapper 组件对象
// （不记忆化，保持与真实产物同构：缓存键稳定性交由路由与 keep-alive 的真实行为裁决）
function boundaryWrappedLoader(slotId: string, inner: Component): () => Promise<DefineComponent> {
  return async () => {
    await Promise.resolve()
    return defineComponent({
      name: pluginViewBoundaryName(slotId),
      setup: () => () => h(inner)
    }) as DefineComponent
  }
}

function pluginViewSlot(slotId: string, inner: Component, keepAlive?: boolean): ViewSlot {
  return {
    slotId,
    name: slotId,
    component: boundaryWrappedLoader(slotId, inner),
    order: 50,
    isPlugin: true,
    keepAlive
  }
}

// 被测布局层：与 MainLayout.vue 视图宿主同形态——router-view 插槽 + 常驻 keep-alive
// （不按路由条件包裹）+ include 绑定 store 派生白名单；名单外组件经 keep-alive 透传。
// router-view 的插槽产出 Component 为 vnode，模板形态 <component :is="Component"> 在
// 运行期对其做克隆（type 保持原组件对象），此处以 cloneVNode 等价复刻
const keepAliveLayout = defineComponent({
  name: 'KeepAliveLayout',
  setup() {
    const slotRegistry = useSlotRegistryStore()
    return () =>
      h(RouterView, null, {
        default: ({ Component }: { Component?: VNode }) =>
          Component === undefined
            ? []
            : [h(KeepAlive, { include: slotRegistry.keepAliveViewNames }, () => [cloneVNode(Component)])]
      })
  }
})

// 应用外壳：根 RouterView 渲染布局层记录（对应真实 App.vue → MainLayout 两级结构）
const appShell = defineComponent({
  name: 'AppShell',
  setup: () => () => h(RouterView)
})

interface KeepAliveTestEnv {
  wrapper: VueWrapper
  router: Router
  store: SlotRegistryStore
  previousPinia: Pinia | undefined
  previousRouter: Router | null
}

// 独立环境：真实 pinia + 内存路由 + 真实 RouterView/KeepAlive 渲染链；hostExtraChildren
// 注入用例专属的宿主静态路由（含/不含 meta.keepAlive 两形态），执行后恢复现场
async function mountKeepAliveApp(hostExtraChildren: RouteRecordRaw[] = []): Promise<KeepAliveTestEnv> {
  const previousPinia = getActivePinia()
  const previousRouter = getRouterInstance()
  const pinia = createPinia()
  setActivePinia(pinia)
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      {
        path: '/',
        name: 'MainLayout',
        component: keepAliveLayout,
        children: [
          { path: '', name: 'mainPage', component: statefulView('mainPage').component },
          ...hostExtraChildren
        ]
      }
    ]
  })
  // registerViewSlot 经模块级 router 实例挂动态路由，须先就位
  setRouterInstance(router)
  const wrapper = mount(appShell, { global: { plugins: [pinia, router] } })
  await router.isReady()
  await settle()
  return { wrapper, router, store: useSlotRegistryStore(), previousPinia, previousRouter }
}

function releaseKeepAliveApp(env: KeepAliveTestEnv): void {
  env.wrapper.unmount()
  setRouterInstance(env.previousRouter)
  if (env.previousPinia) {
    setActivePinia(env.previousPinia)
  }
}

// 等待渲染链收敛：微任务（导航/组件解析）→ 宏任务一拍（keep-alive 缓存搬运）→ 再微任务
async function settle(): Promise<void> {
  await flushPromises()
  await new Promise((resolve) => setTimeout(resolve, 0))
  await flushPromises()
}

// —— 假 ResizeObserver（jsdom 无原生实现）——
// 记录每个实例的观察/断开次数；回调不自动触发，用于区分「观察器自动回传」与「MainView
// activated 主动补测」两条路径——本用例中可见性刷新只能来自后者
interface FakeResizeObserver {
  callback: ResizeObserverCallback
  targets: Set<Element>
  observeCount: number
  disconnectCount: number
}

function installFakeResizeObserver(): { instances: FakeResizeObserver[]; restore: () => void } {
  const instances: FakeResizeObserver[] = []
  const original = globalThis.ResizeObserver
  class FakeResizeObserverImplementation {
    callback: ResizeObserverCallback
    targets = new Set<Element>()
    observeCount = 0
    disconnectCount = 0

    constructor(callback: ResizeObserverCallback) {
      this.callback = callback
      instances.push(this)
    }

    observe(target: Element): void {
      this.observeCount++
      this.targets.add(target)
    }

    unobserve(target: Element): void {
      this.targets.delete(target)
    }

    disconnect(): void {
      this.disconnectCount++
      this.targets.clear()
    }
  }
  globalThis.ResizeObserver = FakeResizeObserverImplementation as unknown as typeof ResizeObserver
  return {
    instances,
    restore: () => {
      globalThis.ResizeObserver = original
    }
  }
}

// Element Plus 组件替身（渲染函数实现）：MainView 模板经全局解析 el-*，由全局组件兜住；
// 需透传默认插槽（el-scrollbar/el-button 内嵌的网格与文案仍要渲染出 ref 目标与文案）
function passthrough(name: string): Component {
  return defineComponent({
    name,
    setup: (_, { slots }) => () => h('div', { class: `${name}-stub` }, slots.default?.())
  })
}

// 真实 MainView 挂载配置：本地导入的子组件全部桩化，不进入子组件实现与副作用
const MAIN_VIEW_PASSTHROUGH_COMPONENTS: Record<string, Component> = {
  ElButton: passthrough('ElButton'),
  ElRadioGroup: passthrough('ElRadioGroup'),
  ElRadioButton: passthrough('ElRadioButton'),
  ElSwitch: passthrough('ElSwitch'),
  ElCheckboxGroup: passthrough('ElCheckboxGroup'),
  ElCheckbox: passthrough('ElCheckbox'),
  ElScrollbar: passthrough('ElScrollbar')
}

const MAIN_VIEW_CHILD_STUBS: Record<string, boolean> = {
  CollapsePanel: true,
  AutoLoadTagSelect: true,
  WorkGridForMainPage: true,
  WorkSetGridForMainPage: true,
  WorkSetCreateDialog: true,
  ExportProgressDialog: true,
  SharePublishDialog: true,
  EmbedSlotRenderer: true
}

describe('MainLayout 视图保活（keep-alive include × 异步懒加载 view）', () => {
  it('声明保活的插件 view：切走 deactivated 不卸载，切回 activated 实例同一且本地状态保持', async () => {
    const probe = statefulView('KeepPluginInner')
    const env = await mountKeepAliveApp()
    try {
      const slotId = 'com.plugin.a/keep-view'
      env.store.registerViewSlot(pluginViewSlot(slotId, probe.component, true))
      // include 字符串路径：白名单含该条目的唯一 wrapper 名
      expect(env.store.keepAliveViewNames).toContain(pluginViewBoundaryName(slotId))

      await env.router.push(`/${slotId}`)
      await settle()
      expect(probe.mountCount()).toBe(1)
      // 渲染出的 wrapper 组件名与 include 白名单条目一致（名字匹配的前提）
      expect(env.wrapper.findComponent({ name: pluginViewBoundaryName(slotId) }).exists()).toBe(true)
      expect(countOf(probe.events, 'mounted')).toBe(1)
      expect(countOf(probe.events, 'activated')).toBe(1)

      // 本地状态变更
      await env.wrapper.find('.probe-KeepPluginInner').trigger('click')
      await settle()
      expect(env.wrapper.find('.probe-KeepPluginInner').attributes('data-count')).toBe('1')

      // 切走：deactivated 而非 unmounted（实例驻留缓存）
      await env.router.push('/')
      await settle()
      expect(countOf(probe.events, 'deactivated')).toBe(1)
      expect(countOf(probe.events, 'unmounted')).toBe(0)

      // 切回：activated、无重挂载（实例引用同一）、本地状态保持
      await env.router.push(`/${slotId}`)
      await settle()
      expect(countOf(probe.events, 'mounted')).toBe(1)
      expect(countOf(probe.events, 'activated')).toBe(2)
      expect(probe.uids()).toHaveLength(1)
      expect(env.wrapper.find('.probe-KeepPluginInner').attributes('data-count')).toBe('1')
    } finally {
      releaseKeepAliveApp(env)
    }
  })

  it('未声明保活的插件 view：切走即卸载，切回重挂载、本地状态归零', async () => {
    const probe = statefulView('PlainPluginInner')
    const env = await mountKeepAliveApp()
    try {
      const slotId = 'com.plugin.a/plain-view'
      env.store.registerViewSlot(pluginViewSlot(slotId, probe.component))
      expect(env.store.keepAliveViewNames).not.toContain(pluginViewBoundaryName(slotId))

      await env.router.push(`/${slotId}`)
      await settle()
      expect(probe.mountCount()).toBe(1)
      await env.wrapper.find('.probe-PlainPluginInner').trigger('click')
      await settle()
      expect(env.wrapper.find('.probe-PlainPluginInner').attributes('data-count')).toBe('1')

      // 切走即卸载（不进缓存）
      await env.router.push('/')
      await settle()
      expect(countOf(probe.events, 'unmounted')).toBe(1)

      // 切回重挂载：新实例、状态归零
      await env.router.push(`/${slotId}`)
      await settle()
      expect(countOf(probe.events, 'mounted')).toBe(2)
      expect(probe.uids()).toHaveLength(2)
      expect(env.wrapper.find('.probe-PlainPluginInner').attributes('data-count')).toBe('0')
    } finally {
      releaseKeepAliveApp(env)
    }
  })

  it('宿主静态声明页（meta.keepAlive + keepAliveName）与插件源双源合流，均经 include 缓存复用', async () => {
    const hostProbe = statefulView('HostKeepView')
    const pluginProbe = statefulView('DualPluginInner')
    const env = await mountKeepAliveApp([
      {
        path: 'hostKeepView',
        name: 'hostKeepView',
        component: hostProbe.component,
        meta: { keepAlive: true, keepAliveName: 'HostKeepView' }
      }
    ])
    try {
      const slotId = 'com.plugin.b/dual-view'
      env.store.registerViewSlot(pluginViewSlot(slotId, pluginProbe.component, true))
      // 双源合流：插件 wrapper 名与宿主组件显式名同在一份 include 白名单内
      expect(env.store.keepAliveViewNames).toEqual(
        expect.arrayContaining(['HostKeepView', pluginViewBoundaryName(slotId)])
      )

      // 宿主静态源条目同样缓存复用
      await env.router.push('/hostKeepView')
      await settle()
      expect(countOf(hostProbe.events, 'mounted')).toBe(1)
      await env.wrapper.find('.probe-HostKeepView').trigger('click')
      await settle()
      expect(env.wrapper.find('.probe-HostKeepView').attributes('data-count')).toBe('1')

      await env.router.push('/')
      await settle()
      expect(countOf(hostProbe.events, 'unmounted')).toBe(0)

      await env.router.push('/hostKeepView')
      await settle()
      expect(countOf(hostProbe.events, 'mounted')).toBe(1)
      expect(countOf(hostProbe.events, 'activated')).toBe(2)
      expect(env.wrapper.find('.probe-HostKeepView').attributes('data-count')).toBe('1')

      // 插件源条目在同环境下同样复用（两源在同一个 keep-alive 内并存）
      await env.router.push(`/${slotId}`)
      await settle()
      expect(countOf(pluginProbe.events, 'mounted')).toBe(1)
      await env.router.push('/')
      await settle()
      await env.router.push(`/${slotId}`)
      await settle()
      expect(countOf(pluginProbe.events, 'mounted')).toBe(1)
      expect(countOf(pluginProbe.events, 'activated')).toBe(2)
    } finally {
      releaseKeepAliveApp(env)
    }
  })

  it('未声明的宿主页经 keep-alive 透传：切走即卸载、无 deactivated（与无缓存现状一致）', async () => {
    const probe = statefulView('HostPlainView')
    const env = await mountKeepAliveApp([
      { path: 'hostPlainView', name: 'hostPlainView', component: probe.component }
    ])
    try {
      await env.router.push('/hostPlainView')
      await settle()
      expect(countOf(probe.events, 'mounted')).toBe(1)

      await env.router.push('/')
      await settle()
      expect(countOf(probe.events, 'unmounted')).toBe(1)
      expect(countOf(probe.events, 'deactivated')).toBe(0)
    } finally {
      releaseKeepAliveApp(env)
    }
  })

  it('主视图（真实 routes.ts 声明 × MainView 显式命名）：切走 deactivated、切回 activated 实例同一且状态保持，观察器重挂并立即补测', async () => {
    const mainPageRecord = (appRoutes[0].children ?? []).find((route) => route.name === 'mainPage')
    expect(mainPageRecord).toBeDefined()
    // 契约两半：路由静态声明（keepAliveName）与组件显式命名须同名，否则 include 名字匹配落空
    expect(mainPageRecord!.meta).toMatchObject({ keepAlive: true, keepAliveName: 'MainView' })
    expect((MainView as unknown as { name?: string }).name).toBe('MainView')

    const fakeResizeObserver = installFakeResizeObserver()
    const previousPinia = getActivePinia()
    const previousRouter = getRouterInstance()
    const pinia = createPinia()
    setActivePinia(pinia)
    const plainProbe = statefulView('MainPagePlainView')
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        {
          path: '/',
          name: 'MainLayout',
          component: keepAliveLayout,
          children: [
            mainPageRecord!,
            { path: 'plain', name: 'plain', component: plainProbe.component }
          ]
        }
      ]
    })
    setRouterInstance(router)
    const wrapper = mount(appShell, {
      global: {
        plugins: [pinia, router],
        components: MAIN_VIEW_PASSTHROUGH_COMPONENTS,
        stubs: MAIN_VIEW_CHILD_STUBS,
        directives: {
          // 滚到底指令宿主容器已由桩层虚拟化，本用例不触发其滚动语义，仅需可解析
          elScrollbarBottomed: { mounted() {} }
        }
      }
    })
    try {
      await router.isReady()
      await settle()

      // 真实路由 meta + 显式命名共同产出 include 条目；主视图首挂即 activated（keep-alive 语义）
      expect(useSlotRegistryStore().keepAliveViewNames).toContain('MainView')
      const firstVm = wrapper.findComponent(MainView).vm
      // setupState 属组件内部非公开 API：类型上经受控断言桥接，运行期访问形态保持不变
      const setupState = (
        firstVm.$ as unknown as {
          setupState: {
            selectedTagList: unknown[]
            workList: unknown[]
            searchBarPanelState: boolean
            workSpace: HTMLElement
            workGridRef: { $el: HTMLElement }
            workPage: { pageNumber: number; pageCount: number }
            loadMore: boolean
          }
        }
      ).setupState
      const firstUid = firstVm.$.uid
      const observer = fakeResizeObserver.instances[0]
      expect(observer).toBeDefined()
      expect(observer.observeCount).toBe(2)
      // 首挂补测的初始判定：空库无下一页，按钮不可见
      expect(setupState.loadMore).toBe(false)
      expect(wrapper.findAll('.work-grid-show-load-more')).toHaveLength(0)

      // 用户态探针：筛选标签 + 已加载列表 + 折叠面板（不经 API，直接以实例状态作保持性断言）
      setupState.selectedTagList.push({})
      setupState.workList.push({}, {})
      setupState.searchBarPanelState = true
      // 保活期几何变化：网格矮于可视区且仍有下一页 → 切回立即补测应让「加载更多」转为可见
      Object.defineProperty(setupState.workSpace, 'clientHeight', { value: 500, configurable: true })
      Object.defineProperty(setupState.workGridRef.$el, 'getBoundingClientRect', {
        configurable: true,
        value: () => ({ height: 400 })
      })
      setupState.workPage.pageCount = 3

      // 切走：deactivated 而非卸载，观察器断开一次
      await router.push('/plain')
      await settle()
      expect(firstVm.$.isDeactivated).toBe(true)
      expect(firstVm.$.isUnmounted).toBe(false)
      expect(observer.disconnectCount).toBe(1)
      expect(wrapper.findComponent(MainView).exists()).toBe(false)

      // 切回：同一实例、状态保持、观察器重挂（首挂 2 次 + 切回 2 次）
      await router.push('/')
      await settle()
      const secondVm = wrapper.findComponent(MainView).vm
      expect(secondVm.$.uid).toBe(firstUid)
      expect(observer.disconnectCount).toBe(1)
      expect(observer.observeCount).toBe(4)
      expect(setupState.selectedTagList).toHaveLength(1)
      expect(setupState.workList).toHaveLength(2)
      expect(setupState.searchBarPanelState).toBe(true)

      // 观察器回调在本用例从未触发，`加载更多` 由不可见转为可见只能来自 activated 的立即补测
      expect(setupState.loadMore).toBe(true)
      expect(wrapper.findAll('.work-grid-show-load-more')).toHaveLength(1)

      // 对照：同一渲染链上无 keepAlive 标记的宿主页仍切走即卸载、再次进入重挂载
      await router.push('/plain')
      await settle()
      expect(plainProbe.mountCount()).toBe(2)
      expect(countOf(plainProbe.events, 'unmounted')).toBe(1)
    } finally {
      wrapper.unmount()
      setRouterInstance(previousRouter)
      if (previousPinia) {
        setActivePinia(previousPinia)
      }
      fakeResizeObserver.restore()
    }
  })
})
