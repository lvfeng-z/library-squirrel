<script setup lang="ts">
import { computed, defineAsyncComponent, watch } from 'vue'
import { useSlotRegistryStore } from '@renderer/store/SlotRegistryStore'
import AutoHeightDialog from '@renderer/components/dialogs/AutoHeightDialog.vue'

/**
 * 插件自定义设置页弹窗壳（dialog 呈现）：宿主自持开关状态，插件组件是被动内容——
 * 打开时按复合键从扩展注册表取组件加载器动态挂载（加载器按条目内容形态加载
 * precompiled/vueSource 产物并带插件边界包裹），关闭即卸载
 */

// props
// 弹窗目标复合键的两段：插件公共 Id + 设置页扩展条目 Id
const props = defineProps<{
  publicId: string
  extensionId: string
}>()

// model
// 弹窗开关（宿主本地响应式变量，打开控制全在宿主）
const state = defineModel<boolean>('state', { required: true })

// 变量
// 扩展注册表：dialog 呈现的设置页 view 条目只入注册表不挂路由，本壳按复合键取件
const slotRegistryStore = useSlotRegistryStore()

// 注册表条目复合键（与注册面的 publicId/extensionId 拼法同构）
const slotId = computed(() => `${props.publicId}/${props.extensionId}`)

// 弹窗开着遇注册表条目被移除（参与度停用等移除链）时自动关闭，不留空壳
watch(
  () => slotRegistryStore.viewSlots.get(slotId.value),
  (slot) => {
    if (state.value && !slot) {
      state.value = false
    }
  },
  { immediate: true }
)

// 内容组件：仅弹窗开启且注册表条目在场时解析，每次开壳重建异步组件、
// 关闭置空即卸载——条目不在场渲染空（开壳入口已校验在场，此处兜底开关与条目的竞态）
const contentComponent = computed(() => {
  if (!state.value) {
    return null
  }
  const slot = slotRegistryStore.viewSlots.get(slotId.value)
  if (!slot) {
    return null
  }
  return defineAsyncComponent({
    loader: () => slot.component()
  })
})

// 方法
// 关闭弹窗（宿主侧关闭入口，与右上角关闭等价；操作按钮由插件内容自带）
function handleCloseClicked() {
  state.value = false
}
</script>

<template>
  <auto-height-dialog
    v-model:state="state"
    width="min(560px, 90vw)"
    :close-on-click-modal="false"
  >
    <template #header>
      <span class="plugin-custom-setting-dialog-title">插件设置</span>
    </template>
    <component
      :is="contentComponent"
      v-if="contentComponent"
    />
    <template #footer>
      <el-button @click="handleCloseClicked">
        关闭
      </el-button>
    </template>
  </auto-height-dialog>
</template>

<style scoped>
.plugin-custom-setting-dialog-title {
  font-size: 20px;
  color: var(--app-text-primary);
}
</style>
