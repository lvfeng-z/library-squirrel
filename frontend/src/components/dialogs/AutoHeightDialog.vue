<script setup lang="ts">
import { nextTick, Ref, ref } from 'vue'

// props
// 关闭语义透传三项：显式 default: undefined 阻止 Vue 对缺省 Boolean prop 的 false 强转，
// 缺省保持 undefined 绑定到 el-dialog（等价于不传），Element Plus 走自身默认值
const props = withDefaults(
  defineProps<{
    destroyOnClose?: boolean
    beforeClose?: (done: (shouldCancel?: boolean) => void) => void
    height?: string
    width?: string
    closeOnClickModal?: boolean
    closeOnPressEscape?: boolean
    showClose?: boolean
  }>(),
  {
    height: '90vh',
    destroyOnClose: true,
    closeOnClickModal: undefined,
    closeOnPressEscape: undefined,
    showClose: undefined
  }
)
// model
// 弹窗开关
const state = defineModel<boolean>('state', { required: true })

// 事件
const emits = defineEmits(['open'])

// 变量
// el-scrollbar组件实例
const scrollbarRef = ref()
// el-scrollbar内部容器的高度
const scrollbarWrapHeight: Ref<string> = ref('')

// 方法
// 开启对话框
function handleChangeState() {
  emits('open')
  nextTick(() => {
    const dialog = scrollbarRef.value.$el.parentElement.parentElement
    const headerHeight = dialog.querySelector('.el-dialog__header').clientHeight
    const footerHeight = dialog.querySelector('.el-dialog__footer').clientHeight

    // 减去header+footer+dialog的padding共计4个16px
    scrollbarWrapHeight.value = 'calc(' + props.height + ' - ' + (headerHeight + footerHeight) + 'px - 16px - 16px - 16px - 16px)'
  })
}
</script>

<template>
  <teleport to="#dialog-mount-point">
    <el-dialog
      v-model="state"
      :width="props.width"
      style="margin: auto"
      :destroy-on-close="destroyOnClose"
      :before-close="props.beforeClose"
      :close-on-click-modal="props.closeOnClickModal"
      :close-on-press-escape="props.closeOnPressEscape"
      :show-close="props.showClose"
      @open="handleChangeState"
    >
      <template #header>
        <slot name="header" />
      </template>
      <el-scrollbar
        ref="scrollbarRef"
        class="auto-height-dialog-scrollbar"
      >
        <slot />
      </el-scrollbar>
      <template #footer>
        <slot name="footer" />
      </template>
    </el-dialog>
  </teleport>
</template>

<style scoped>
.auto-height-dialog-scrollbar {
  padding: 0 10px 0 0;
  overflow: hidden;
}
.auto-height-dialog-scrollbar > :deep(.el-scrollbar__wrap) {
  max-height: v-bind(scrollbarWrapHeight);
}
</style>
