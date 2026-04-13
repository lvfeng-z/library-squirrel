// 处理el-select触底的的自定义指令
export default {
  mounted(el, binding) {
    const handleScroll = function (event) {
      const domTarget = event.target
      const scrollTop = domTarget.scrollTop
      const clientHeight = domTarget.clientHeight
      const scrollHeight = domTarget.scrollHeight
      // 内容没满一屏就不触发，可防止内容变少导致的触底
      if (scrollHeight <= clientHeight) {
        return
      }
      if (scrollHeight - scrollTop <= clientHeight) {
        // 把input元素的value属性（即输入的文本）传递过去
        binding.value(el.firstElementChild.firstElementChild.firstElementChild.firstElementChild.value)
      }
    }
    // 获取 el-select__input 携带的 aria-controls 属性
    const child = el.querySelector('.el-select__input')
    const id = child.getAttribute('aria-controls')
    // 通过 aria-controls 找到 popper 元素
    const popper = document.getElementById(id)
    if (popper != null) {
      const selectWrapper = popper.parentElement

      if (selectWrapper !== null) {
        selectWrapper.addEventListener('scroll', handleScroll)
        el.__ls_handleScroll = handleScroll
        el.__ls_scrollDom = selectWrapper
      }
    }
  },
  unmounted(el) {
    el.__ls_scrollDom.removeEventListener('scroll', el.__ls_handleScroll)
  }
}
