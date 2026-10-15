import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter, type Router, type RouteRecordRaw } from 'vue-router'
import { defineComponent, h, type Component } from 'vue'
import DynamicSideMenu from './DynamicSideMenu.vue'
import { useSlotRegistryStore } from '@renderer/store/SlotRegistryStore'
import type { MenuSlotItem } from '@renderer/store/SlotRegistryStore'

// SideMenu 替身：只保留默认插槽出口与 activeIndex 透出（真实滚动容器/折叠交互与定位逻辑无关，
// 且其依赖的 el-scrollbar/el-tooltip/v-click-out-side 指令在 jsdom 测试环境未安装）
vi.mock('@renderer/components/oneOff/SideMenu.vue', async () => {
  const { defineComponent, h } = await import('vue')
  return {
    default: defineComponent({
      name: 'SideMenuStub',
      props: {
        width: String,
        foldWidth: String,
        defaultActive: Array,
        activeIndex: String,
        backgroundColor: String
      },
      setup(props, { slots }) {
        return () => h('div', { class: 'side-menu-stub', 'data-active-index': props.activeIndex }, slots.default?.())
      }
    })
  }
})

// —— Element Plus 组件替身（渲染函数实现，不依赖运行时模板编译器）——

// el-menu-item 替身：根元素 li；未声明为 prop 的 attrs（含 data-menu-slot 定位钩子）
// 经 Vue 默认透传落根元素——本替身同时充当「透传机制成立」的断言载体
const ElMenuItemStub: Component = defineComponent({
  props: { index: String },
  setup(_props, { slots }) {
    return () => h('li', slots.default?.())
  }
})

// el-sub-menu 替身：title 插槽与子项插槽串行渲染，根元素 li
const ElSubMenuStub: Component = defineComponent({
  props: { index: String },
  setup(_props, { slots }) {
    return () => h('li', [slots.title?.(), slots.default?.()])
  }
})

const ElBadgeStub: Component = defineComponent({
  props: { value: [Number, String], max: Number, offset: Array, hidden: Boolean },
  setup(_props, { slots }) {
    return () => h('span', { class: 'menu-badge-stub' }, slots.default?.())
  }
})

const ElIconStub: Component = defineComponent({
  setup(_props, { slots }) {
    return () => h('i', slots.default?.())
  }
})

// el-image 替身：AppIcon 渲染期即解析该组件（无图标时虽不创建仍需可解析），注册以消除解析告警
const ElImageStub: Component = defineComponent({
  props: { src: String, fit: String },
  setup(_props, { slots }) {
    return () => h('span', { class: 'el-image-stub' }, slots.default?.())
  }
})

// 空路由组件：定位测试只关心路由名与 meta.parent，不渲染页面内容
const EmptyView: Component = defineComponent({
  setup() {
    return () => h('div')
  }
})

// 路由表：复刻宿主一级页（无 meta.parent、直配菜单项）+ 插件两级页（二级带 meta.parent）+
// 无菜单项可达的孤立页
const routes: RouteRecordRaw[] = [
  { path: '/', name: 'mainPage', component: EmptyView, meta: {} },
  { path: '/settings', name: 'settings', component: EmptyView, meta: {} },
  { path: '/com.plugin.a/workbench', name: 'com.plugin.a/workbench', component: EmptyView, meta: {} },
  {
    path: '/com.plugin.a/workbench-parse',
    name: 'com.plugin.a/workbench-parse',
    component: EmptyView,
    meta: { parent: 'com.plugin.a/workbench' }
  },
  { path: '/no-menu-page', name: 'no-menu-page', component: EmptyView, meta: {} }
]

// 测试菜单树：内置分组（子项挂宿主一级路由名）+ 插件顶层项（viewId 为复合路由名）
function builtinMenuSlots(): MenuSlotItem[] {
  return [
    {
      slotId: 'builtin-group',
      index: 'builtin-group',
      label: '内置分组',
      children: [
        { slotId: 'builtin-group/mainPage', index: 'builtin-group/mainPage', label: '主页', viewId: 'mainPage' },
        { slotId: 'builtin-group/settings', index: 'builtin-group/settings', label: '设置', viewId: 'settings' }
      ]
    },
    {
      slotId: 'com.plugin.a/workbench',
      index: 'com.plugin.a/workbench',
      label: '插件工作台',
      viewId: 'com.plugin.a/workbench'
    }
  ]
}

// scrollIntoView 替身：jsdom 未实现该方法，直接挂到 Element.prototype 供组件调用并记录
const scrollIntoViewMock = vi.fn()

// 挂载被测组件：真实 router（内存历史）+ 真实 pinia（菜单树直写 store 桶），等待初始导航收敛
async function mountMenu(): Promise<{ wrapper: VueWrapper; router: Router }> {
  const pinia = createPinia()
  setActivePinia(pinia)
  const slotStore = useSlotRegistryStore()
  builtinMenuSlots().forEach((slot) => slotStore.menuSlots.set(slot.slotId, slot))

  const router = createRouter({ history: createMemoryHistory(), routes })
  const wrapper = mount(DynamicSideMenu, {
    global: {
      plugins: [pinia, router],
      components: {
        'el-menu-item': ElMenuItemStub,
        'el-sub-menu': ElSubMenuStub,
        'el-badge': ElBadgeStub,
        'el-icon': ElIconStub,
        'el-image': ElImageStub
      }
    }
  })
  await router.isReady()
  return { wrapper, router }
}

// 等待定位链路收敛：watch post 队列（微任务）→ nextTick → rAF 替身（宏任务 setTimeout 0）
async function settle(): Promise<void> {
  await flushPromises()
  await new Promise((resolve) => setTimeout(resolve, 0))
  await flushPromises()
}

beforeEach(() => {
  scrollIntoViewMock.mockClear()
  Element.prototype.scrollIntoView = scrollIntoViewMock
  // rAF 替身落到宏任务队列：jsdom 环境的 requestAnimationFrame 可用性不作假设，统一走 setTimeout
  vi.stubGlobal(
    'requestAnimationFrame',
    (callback: FrameRequestCallback): number => setTimeout(() => callback(0), 0)
  )
})

afterEach(() => {
  vi.unstubAllGlobals()
  delete (Element.prototype as { scrollIntoView?: unknown }).scrollIntoView
})

function targetElementOf(wrapper: VueWrapper, slotId: string): HTMLElement {
  const target = wrapper.find(`[data-menu-slot="${slotId}"]`)
  expect(target.exists()).toBe(true)
  return target.element as HTMLElement
}

describe('DynamicSideMenu 跳转定位提示', () => {
  it('初次加载不提示：初始导航落定后无脉冲、无滚动', async () => {
    const { wrapper } = await mountMenu()
    await settle()

    expect(wrapper.find('.menu-attention-pulse').exists()).toBe(false)
    expect(scrollIntoViewMock).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('程序化跳转（二级页沿 meta.parent 上溯）：祖先项平滑滚动 + 脉冲挂载并于 animationend 移除，持久选中跟随上溯项', async () => {
    const { wrapper, router } = await mountMenu()
    await settle()

    await router.push('/com.plugin.a/workbench-parse?root=abc')
    await settle()

    const target = targetElementOf(wrapper, 'com.plugin.a/workbench')
    // 脉冲类经 data-menu-slot 钩子挂到祖先菜单项根元素
    expect(target.classList.contains('menu-attention-pulse')).toBe(true)
    // 平滑滚动调用参数与调用主体
    expect(scrollIntoViewMock).toHaveBeenCalledTimes(1)
    expect(scrollIntoViewMock).toHaveBeenCalledWith({ behavior: 'smooth', block: 'nearest' })
    expect(scrollIntoViewMock.mock.contexts[0]).toBe(target)
    // 持久选中：上溯命中的祖先项 index 作为活动项传给侧栏壳
    expect(wrapper.find('.side-menu-stub').attributes('data-active-index')).toBe('com.plugin.a/workbench')

    // jsdom 无动画引擎：主动派发 animationend，断言脉冲类被移除
    target.dispatchEvent(new Event('animationend'))
    await settle()
    expect(target.classList.contains('menu-attention-pulse')).toBe(false)
    wrapper.unmount()
  })

  it('程序化跳转（一级页精确命中）：目标菜单项自身脉冲并于 animationend 移除', async () => {
    const { wrapper, router } = await mountMenu()
    await settle()

    await router.push('/settings')
    await settle()

    const target = targetElementOf(wrapper, 'builtin-group/settings')
    expect(target.classList.contains('menu-attention-pulse')).toBe(true)
    expect(scrollIntoViewMock).toHaveBeenCalledWith({ behavior: 'smooth', block: 'nearest' })

    target.dispatchEvent(new Event('animationend'))
    await settle()
    expect(target.classList.contains('menu-attention-pulse')).toBe(false)
    wrapper.unmount()
  })

  it('animationend 不触发时按定时兜底移除脉冲类', async () => {
    // 仅伪造 setTimeout/clearTimeout：vitest 默认会把 requestAnimationFrame 一并伪掉
    //（覆盖 beforeEach 的 rAF 替身），rAF 替身自身依赖 setTimeout，保留其可用性
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    try {
      const { wrapper, router } = await mountMenu()
      await flushPromises()

      await router.push('/com.plugin.a/workbench-parse')
      await flushPromises()
      // rAF 替身的宏任务（setTimeout 0）由假时钟推进触发
      await vi.advanceTimersByTimeAsync(0)

      const target = targetElementOf(wrapper, 'com.plugin.a/workbench')
      expect(target.classList.contains('menu-attention-pulse')).toBe(true)

      // 不派发 animationend（模拟动画事件丢失），推进到兜底时点后类被移除
      await vi.advanceTimersByTimeAsync(2000)
      expect(target.classList.contains('menu-attention-pulse')).toBe(false)
      wrapper.unmount()
    } finally {
      vi.useRealTimers()
    }
  })

  it('菜单项自身点击引发的跳转不提示：不脉冲、不滚动', async () => {
    const { wrapper, router } = await mountMenu()
    await settle()

    await wrapper.find('[data-menu-slot="com.plugin.a/workbench"]').trigger('click')
    await settle()

    // 跳转确实发生（排除「未导航导致无脉冲」的假通过）
    expect(router.currentRoute.value.name).toBe('com.plugin.a/workbench')
    expect(targetElementOf(wrapper, 'com.plugin.a/workbench').classList.contains('menu-attention-pulse')).toBe(false)
    expect(scrollIntoViewMock).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('点击当前页菜单项（同名重复导航）后，后续程序化跳转照常提示', async () => {
    const { wrapper, router } = await mountMenu()
    await settle()

    // 程序化落到插件工作台并完结脉冲，清零滚动计数
    await router.push('/com.plugin.a/workbench')
    await settle()
    const target = targetElementOf(wrapper, 'com.plugin.a/workbench')
    target.dispatchEvent(new Event('animationend'))
    await settle()
    scrollIntoViewMock.mockClear()

    // 点击当前页菜单项：route.name 不变，自点击不提示
    await wrapper.find('[data-menu-slot="com.plugin.a/workbench"]').trigger('click')
    await settle()
    expect(scrollIntoViewMock).not.toHaveBeenCalled()

    // 自点击标志未残留：下一次程序化跳转照常脉冲
    await router.push('/settings')
    await settle()
    expect(targetElementOf(wrapper, 'builtin-group/settings').classList.contains('menu-attention-pulse')).toBe(true)
    wrapper.unmount()
  })

  it('纯 query 变化不重触发：同路由名换 query 不再脉冲滚动', async () => {
    const { wrapper, router } = await mountMenu()
    await settle()

    await router.push('/com.plugin.a/workbench-parse?root=a')
    await settle()
    const target = targetElementOf(wrapper, 'com.plugin.a/workbench')
    target.dispatchEvent(new Event('animationend'))
    await settle()
    scrollIntoViewMock.mockClear()

    await router.push('/com.plugin.a/workbench-parse?root=b')
    await settle()

    expect(scrollIntoViewMock).not.toHaveBeenCalled()
    expect(target.classList.contains('menu-attention-pulse')).toBe(false)
    wrapper.unmount()
  })

  it('无菜单项可达的跳转静默：不动作不报错', async () => {
    const { wrapper, router } = await mountMenu()
    await settle()

    await router.push('/no-menu-page')
    await settle()

    expect(scrollIntoViewMock).not.toHaveBeenCalled()
    expect(wrapper.find('.menu-attention-pulse').exists()).toBe(false)
    wrapper.unmount()
  })
})
