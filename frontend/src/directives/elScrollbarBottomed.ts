import { Directive } from "vue";

const elScrollbarBottomed: Directive = {
  beforeMount(el, binding) {
    const scrollbar = el.querySelector(".el-scrollbar__wrap");
    if (scrollbar) {
      scrollbar.scrollBottomEvent = () => {
        const { scrollTop, scrollHeight, clientHeight } = scrollbar;
        if (scrollHeight - scrollTop - clientHeight < 1) {
          binding.value();
        }
      };
      scrollbar.addEventListener("scroll", scrollbar.scrollBottomEvent);
    }
  },
  unmounted(el) {
    const scrollbar = el.querySelector(".el-scrollbar__wrap");
    if (scrollbar) {
      scrollbar.removeEventListener("scroll", scrollbar.scrollBottomEvent);
    }
  },
};

export default elScrollbarBottomed;
