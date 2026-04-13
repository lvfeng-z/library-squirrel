// 处理el-scrollbar触底的自定义指令
export default {
  mounted(el, binding) {
    const handleScroll = function (event) {
      const domTarget = event.target
      const scrollTop = domTarget.scrollTop
      const clientHeight = domTarget.clientHeight
      const scrollHeight = domTarget.scrollHeight
      if (scrollHeight - scrollTop <= clientHeight + 1) {
        binding.value()
      }
    }
    const scrollbarWrap = el.children[0]
    if (scrollbarWrap != null) {
      scrollbarWrap.addEventListener('scroll', handleScroll)
    }
  },
  unmounted(el) {
    el.children[0].removeEventListener('scroll', el.__handleScroll)
  }
}
