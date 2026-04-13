export default {
  mounted(el) {
    const label = el.querySelector('.scrollable-check')
    const selfWidth = el.offsetWidth // 使用标签自身的实际宽度

    const shouldScroll = label.offsetWidth > selfWidth
    if (shouldScroll) {
      label.classList.add('scroll-text')
    } else {
      label.classList.remove('scroll-text')
    }
  }
}
