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
      :close-on-click-modal="props.closeOnClickModal"
      :close-on-press-escape="props.closeOnPressEscape"
      :show-close="props.showClose"
      @open="handleChangeState"
    >
      <template #header>
        <slot name="header" />
      </template>
      <div
        ref="containerRef"
        class="static-height-dialog-container"
      >
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
