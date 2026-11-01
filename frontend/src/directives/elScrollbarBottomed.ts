// 处理el-scrollbar触底的自定义指令
// 触底判定带高度塌陷防护：keep-alive 视图切回等场景下，DOM 子树重插入瞬间可能先以
// 塌陷高度布局，浏览器钳制 scrollTop 派发的 scroll 事件会伪装成「已触底」而误触发
// 加载下一页。此类事件的几何特征是 scrollTop 回退或 scrollHeight 收缩——真实用户触底
// 滚动方向恒为向下（scrollTop 不减）且内容高度不收缩，据此跳过判定
interface ScrollbarBottomedHost extends HTMLElement {
  // 挂载期记录的滚动处理器（unmounted 反注册用；记录在指令宿主元素上）
  __elScrollbarBottomedHandler?: ((event: Event) => void) | null
}

export default {
  mounted(el: ScrollbarBottomedHost, binding: { value: () => void }) {
    let lastScrollTop = 0
    let lastScrollHeight = 0
    const handleScroll = function (event: Event) {
      const domTarget = event.target as HTMLElement
      const scrollTop = domTarget.scrollTop
      const clientHeight = domTarget.clientHeight
      const scrollHeight = domTarget.scrollHeight
      // 钳制伪触底识别：scrollTop 回退（浏览器钳制/用户上滚，均非触底趋近）或
      // scrollHeight 收缩（内容高度塌陷，此刻的「底部」是伪底部）；0.5px 容差吸收
      // 缩放/亚像素抖动
      const clampArtifact = scrollTop < lastScrollTop - 0.5 || scrollHeight < lastScrollHeight - 0.5
      lastScrollTop = scrollTop
      lastScrollHeight = scrollHeight
      if (clampArtifact) {
        return
      }
      if (scrollHeight - scrollTop <= clientHeight + 1) {
        binding.value()
      }
    }
    const scrollbarWrap = el.children[0]
    if (scrollbarWrap != null) {
      scrollbarWrap.addEventListener('scroll', handleScroll)
      el.__elScrollbarBottomedHandler = handleScroll
    }
  },
  unmounted(el: ScrollbarBottomedHost) {
    const scrollbarWrap = el.children[0]
    if (scrollbarWrap != null && el.__elScrollbarBottomedHandler != null) {
      scrollbarWrap.removeEventListener('scroll', el.__elScrollbarBottomedHandler)
      el.__elScrollbarBottomedHandler = null
    }
  }
}
