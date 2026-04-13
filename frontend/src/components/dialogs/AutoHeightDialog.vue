<script setup lang="ts">
import { nextTick, Ref, ref } from 'vue'

// props
const props = withDefaults(
  defineProps<{
    destroyOnClose?: boolean
    beforeClose?: (done: (shouldCancel?: boolean) => void) => void
    height?: string
    width?: string
  }>(),
  {
    height: '90vh',
    destroyOnClose: true
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
      @open="handleChangeState"
    >
      <template #header>
        <slot name="header" />
      </template>
      <el-scrollbar ref="scrollbarRef" class="auto-height-dialog-scrollbar">
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
