import { Directive } from "vue";

const elSelectBottomed: Directive = {
  beforeMount(el, binding) {
    el.scrollBottomEvent = () => {
      const { scrollTop, scrollHeight, clientHeight } = el;
      if (scrollHeight - scrollTop - clientHeight < 1) {
        binding.value();
      }
    };
    el.addEventListener("scroll", el.scrollBottomEvent);
  },
  unmounted(el) {
    el.removeEventListener("scroll", el.scrollBottomEvent);
  },
};

export default elSelectBottomed;
