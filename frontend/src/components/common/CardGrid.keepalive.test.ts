import { describe, it, expect, vi, beforeAll, afterEach } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { defineComponent, h, KeepAlive, nextTick, ref } from 'vue'
import CardGrid from '@renderer/components/common/CardGrid.vue'

/**
 * CardGrid 瀑布流的保活形态验证（D2 根因面）。
 *
 * 根因（实机断言）：keep-alive 切走时 Chromium 给 ResizeObserver 投递宽度归零项
 * （内容盒消失），容器宽度防抖计时器（100ms）在 DOM 脱离文档后触发全量重排，以脱附
 * 垃圾几何（clientWidth=0 → 列宽为负）重写卡片布局；切回重插入瞬间塌陷布局先生效，
 * 滚动位被钳制并误触触底指令。修复面：deactivated 拆观察 + 清计时/rAF（移出期布局
 * 静止）、activated 按当前宽度补排 + 重挂观察。
 */

interface FakeResizeObserver {
  callback: ResizeObserverCallback
  targets: Set<Element>
  observeCount: number
  disconnectCount: number
  fire: (target: Element, width: number) => void
}

const fakeInstances: FakeResizeObserver[] = []

function installFakeResizeObserver(): void {
  class FakeResizeObserverImplementation {
    callback: ResizeObserverCallback
    targets = new Set<Element>()
    observeCount = 0
    disconnectCount = 0

    constructor(callback: ResizeObserverCallback) {
      this.callback = callback
      const self = this as unknown as FakeResizeObserver
      self.fire = (target: Element, width: number) => {
        // ResizeObserverCallback 形参为 (entries, observer)：第二参按签名补传观察器本身（组件侧回调只读 entries）
        this.callback([{ target, contentRect: { width } } as unknown as ResizeObserverEntry], self as unknown as ResizeObserver)
      }
      fakeInstances.push(self)
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
  vi.stubGlobal('ResizeObserver', FakeResizeObserverImplementation as unknown as typeof ResizeObserver)
}

// jsdom rAF 兜底（vitest jsdom 环境通常自带；缺省时以 setTimeout 近似，计时器语义不变）
beforeAll(() => {
  if (!globalThis.requestAnimationFrame) {
    // Node 侧 setTimeout 返回 Timeout，DOM 侧 rAF 约定返回 number：仅桥接类型形态，计时语义不变
    globalThis.requestAnimationFrame = (cb: FrameRequestCallback) =>
      setTimeout(() => cb(performance.now()), 16) as unknown as number
    globalThis.cancelAnimationFrame = (id: number) => clearTimeout(id)
  }
})

afterEach(() => {
  vi.unstubAllGlobals()
  fakeInstances.length = 0
})

const wait = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms))

// clientWidth 以 getter 注入并联动 isConnected：忠实复刻 Chromium「脱附元素 clientWidth=0」
function injectConnectedClientWidth(el: HTMLElement, widthRef: { value: number }): void {
  Object.defineProperty(el, 'clientWidth', {
    configurable: true,
    get: () => (el.isConnected ? widthRef.value : 0)
  })
}

// 卡片元素取自已捕获的 gridEl 引用（同一节点跨 keep-alive 搬移存活；切走期 wrapper 树内不可见）
function cardEl(gridEl: HTMLElement, index: number): HTMLElement {
  return gridEl.children[index] as HTMLElement
}

describe('CardGrid 瀑布流保活（keep-alive 移出期布局静止）', () => {
  it('切走前挂起的宽度防抖在移出期被取消：容器高度/卡片坐标不被脱附垃圾几何重写，切回重挂观察', async () => {
    installFakeResizeObserver()
    const showGrid = ref(true)
    const widthRef = { value: 1267 }
    const GridPage = defineComponent({
      name: 'GridPage',
      setup() {
        return () =>
          h(CardGrid, {
            items: [{ id: 1 }, { id: 2 }, { id: 3 }, { id: 4 }],
            checkable: false,
            // 泛型组件在 h() 处 T 推为 unknown：以显式函数类型标注桥接，取 id 的运行期行为不变
            getId: ((item: { id: number }) => item.id) as (item: unknown) => number,
            getDimension: () => ({ width: 100, height: 100 })
          })
      }
    })
    const PageB = defineComponent({
      name: 'PageB',
      setup: () => () => h('div', { class: 'page-b' })
    })
    const Host = defineComponent({
      name: 'KaHost',
      setup: () => () => h(KeepAlive, null, { default: () => [h(showGrid.value ? GridPage : PageB)] })
    })
    const wrapper = mount(Host, { attachTo: document.body })
    await nextTick()
    const gridEl = wrapper.find('.card-grid').element as HTMLElement
    expect(gridEl).toBeDefined()
    injectConnectedClientWidth(gridEl, widthRef)

    // 基线：以注入宽度投递一次容器宽度变化 → 防抖到期全量重排（列宽 (1267-18)/4=312.25，
    // 单列单卡：卡高 304.25+54+6=364.25）
    const containerObserver = fakeInstances[0]
    containerObserver.fire(gridEl, 1267)
    await wait(600) // 100ms 防抖 + 350ms 动画窗拆除
    expect(gridEl.style.height).toBe('364.25px')
    expect(cardEl(gridEl, 1).style.transform).toBe('translate(318.25px, 0px)')

    // 缺陷形态注入：切走瞬间 Chromium 投递宽度归零项 → 防抖计时器挂起（100ms 后触发）
    containerObserver.fire(gridEl, 0)
    // 切走（deactivated）：拆观察 + 清计时器
    showGrid.value = false
    await nextTick()
    await wait(50)
    expect(wrapper.find('.page-b').exists()).toBe(true)
    expect(containerObserver.disconnectCount).toBeGreaterThanOrEqual(1)
    // 移出期度过整个防抖+动画窗窗口：若计时器未清，将按脱附 clientWidth=0 重排出垃圾布局
    await wait(600)
    expect(gridEl.style.height).toBe('364.25px')
    expect(cardEl(gridEl, 1).style.transform).toBe('translate(318.25px, 0px)')
    expect(gridEl.classList.contains('animating')).toBe(false)

    // 切回（activated）：重挂容器观察（新实例）、宽度未变不触发逐卡重写，几何原样
    showGrid.value = true
    await nextTick()
    await wait(50)
    expect(wrapper.find('.card-grid').exists()).toBe(true)
    const reattachedObserver = fakeInstances[fakeInstances.length - 1]
    expect(fakeInstances.indexOf(reattachedObserver)).toBeGreaterThan(0)
    expect(reattachedObserver.targets.has(gridEl)).toBe(true)
    expect(reattachedObserver.observeCount).toBeGreaterThanOrEqual(1)
    expect(gridEl.style.height).toBe('364.25px')
    expect(cardEl(gridEl, 1).style.transform).toBe('translate(318.25px, 0px)')

    // 对照：在场状态的真实宽度变化仍走正常防抖重排（修复未破坏常规路径）
    widthRef.value = 1567
    reattachedObserver.fire(gridEl, 1567)
    await wait(600)
    const colWidth = (1567 - 18) / 4
    expect(cardEl(gridEl, 1).style.transform).toBe(`translate(${colWidth + 6}px, 0px)`)
    expect(gridEl.style.height).toBe(`${colWidth - 8 + 54 + 6}px`)
    wrapper.unmount()
  })
})
