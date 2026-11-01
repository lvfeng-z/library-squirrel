import { describe, it, expect, vi } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { defineComponent, h, withDirectives, type Directive } from 'vue'
import elScrollbarBottomed from '@renderer/directives/elScrollbarBottomed'

/**
 * elScrollbarBottomed 指令语义验证：触底判定 + 高度塌陷防护。
 *
 * 防护针对的缺陷形态（D2，视图保活）：keep-alive 视图切回时 DOM 子树重插入瞬间以塌陷
 * 高度布局，浏览器钳制 scrollTop 派发的 scroll 事件伪装成「已触底」——事件几何特征为
 * scrollTop 回退或 scrollHeight 收缩；真实用户触底滚动方向恒向下且内容高度不收缩。
 */

// 复刻 el-scrollbar 的 DOM 形态：指令宿主的 children[0] 即滚动容器 wrap
function mountScrollbarHost(onBottomed: () => void): VueWrapper {
  const Host = defineComponent({
    name: 'ScrollbarHost',
    props: { bottomed: { type: Function, required: true } },
    setup(props) {
      return () =>
        withDirectives(h('div', { class: 'sb-host' }, [h('div', { class: 'sb-wrap' })]), [
          [elScrollbarBottomed as unknown as Directive, props.bottomed as () => void]
        ])
    }
  })
  return mount(Host, { props: { bottomed: onBottomed } })
}

// jsdom 无布局：几何量直接注入 wrap（指令同步读取三者，defineProperty 可控）
function setWrapGeometry(wrap: HTMLElement, geo: { scrollTop: number; scrollHeight: number; clientHeight: number }): void {
  Object.defineProperty(wrap, 'scrollTop', { configurable: true, writable: true, value: geo.scrollTop })
  Object.defineProperty(wrap, 'scrollHeight', { configurable: true, value: geo.scrollHeight })
  Object.defineProperty(wrap, 'clientHeight', { configurable: true, value: geo.clientHeight })
}

function fireScroll(wrap: HTMLElement): void {
  wrap.dispatchEvent(new Event('scroll'))
}

describe('elScrollbarBottomed 触底指令', () => {
  it('向下滚动到达底部触发 binding（正常触底不受防护影响）', () => {
    const bottomed = vi.fn()
    const wrapper = mountScrollbarHost(bottomed)
    const wrap = wrapper.find('.sb-wrap').element as HTMLElement
    // 中段：远离底部，不触发
    setWrapGeometry(wrap, { scrollTop: 500, scrollHeight: 2000, clientHeight: 600 })
    fireScroll(wrap)
    expect(bottomed).not.toHaveBeenCalled()
    // 滚到底部：2000 - 1400 = 600 <= 600 + 1 触发
    setWrapGeometry(wrap, { scrollTop: 1400, scrollHeight: 2000, clientHeight: 600 })
    fireScroll(wrap)
    expect(bottomed).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('高度塌陷钳制事件（scrollTop 回退，实机形态 3000→680/scrollHeight 塌陷）不触发 binding', () => {
    const bottomed = vi.fn()
    const wrapper = mountScrollbarHost(bottomed)
    const wrap = wrapper.find('.sb-wrap').element as HTMLElement
    // 切走前的稳定态：非零滚动位
    setWrapGeometry(wrap, { scrollTop: 3000, scrollHeight: 7809, clientHeight: 624 })
    fireScroll(wrap)
    expect(bottomed).not.toHaveBeenCalled()
    // 切回瞬间的钳制事件：scrollHeight 塌陷至 1305、scrollTop 被钳到 1305-624（底部成立），
    // 但 scrollTop 回退 + scrollHeight 收缩 → 伪触底，跳过
    setWrapGeometry(wrap, { scrollTop: 681, scrollHeight: 1305, clientHeight: 624 })
    fireScroll(wrap)
    expect(bottomed).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('scrollHeight 收缩（scrollTop 持平）的伪底部不触发；后续真实触底仍正常触发', () => {
    const bottomed = vi.fn()
    const wrapper = mountScrollbarHost(bottomed)
    const wrap = wrapper.find('.sb-wrap').element as HTMLElement
    setWrapGeometry(wrap, { scrollTop: 500, scrollHeight: 2000, clientHeight: 600 })
    fireScroll(wrap)
    // 内容高度收缩至底部之下（900-500=400 <= 601 伪成立），scrollTop 持平 → 跳过
    setWrapGeometry(wrap, { scrollTop: 500, scrollHeight: 900, clientHeight: 600 })
    fireScroll(wrap)
    expect(bottomed).not.toHaveBeenCalled()
    // 塌陷恢复后用户继续下滚到底（2000-1400=600 <= 601）→ 正常触发（防护不闭锁）
    setWrapGeometry(wrap, { scrollTop: 1400, scrollHeight: 2000, clientHeight: 600 })
    fireScroll(wrap)
    expect(bottomed).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('卸载反注册滚动监听（unmounted 钩子修复面）：卸载后 scroll 事件不再到达 binding', () => {
    const bottomed = vi.fn()
    const wrapper = mountScrollbarHost(bottomed)
    const wrap = wrapper.find('.sb-wrap').element as HTMLElement
    setWrapGeometry(wrap, { scrollTop: 1400, scrollHeight: 2000, clientHeight: 600 })
    fireScroll(wrap)
    expect(bottomed).toHaveBeenCalledTimes(1)
    wrapper.unmount()
    // 卸载后继续派发：监听已随 unmounted 移除（旧实现 el.__handleScroll 从未赋值、移除无效）
    fireScroll(wrap)
    expect(bottomed).toHaveBeenCalledTimes(1)
  })
})
