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
// container组件实例
const containerRef = ref()
// 弹窗开关
const state = defineModel<boolean>('state', { required: true })
const containerHeight: Ref<string> = ref('')

// 事件
const emits = defineEmits(['open'])

// 变量

// 方法
// 开启对话框
function handleChangeState() {
  emits('open')
  nextTick(() => {
    const dialog = containerRef.value.parentElement.parentElement
    const headerHeight = dialog.querySelector('.el-dialog__header').clientHeight
    const footerHeight = dialog.querySelector('.el-dialog__footer').clientHeight

    // 减去header+footer+dialog的padding共计4个16px
    containerHeight.value = 'calc(' + props.height + ' - ' + (headerHeight + footerHeight) + 'px - 16px - 16px - 16px - 16px)'
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
      <div ref="containerRef" class="static-height-dialog-container">
        <slot />
      </div>
      <template #footer>
        <slot name="footer" />
      </template>
    </el-dialog>
  </teleport>
</template>

<style scoped>
.static-height-dialog-container {
  height: v-bind(containerHeight);
}
</style>
