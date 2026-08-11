<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import SegmentedTag from '@renderer/components/common/SegmentedTag.vue'
import SegmentedTagItem from '@renderer/model/util/SegmentedTagItem.ts'
import { BUILTIN_NAMESPACES } from '@renderer/constants/namespace.ts'

// NamespaceTag：包一层 SegmentedTag，把 namespace 作为视觉一体的末段；ns 段点击弹 el-popover 编辑，其余点击透传。
// 可编辑性 = editableNs prop（区域级，TagBox 透传）|| extraData.nsEditable（tag 数据级，兼容）；
// ns 值（extraData.namespace）始终纯 tag 数据，popover 提交时回写。
// 作 TagBox 默认 tagComponent——非编辑态不显示 ns 段、不创建 popover，退化为纯 SegmentedTag。
const props = defineProps<{
  item: SegmentedTagItem
  closeable?: boolean
  variant?: 'block' | 'outlined'
  editableNs?: boolean
}>()

const emits = defineEmits(['clicked', 'mainLabelClicked', 'subLabelClicked', 'close', 'nsEdited'])

// 是否可编辑（显示 ns 选择框）：区域级 editableNs prop 优先，兼容 tag 数据 extraData.nsEditable
const editable = computed(() => props.editableNs || !!props.item.extraData?.nsEditable)
// namespace 本地真源（extraData:any 非深响应，用本地 ref 驱动展示；popover 提交时回写 extraData.namespace）
const editingNs = ref<string>(props.item.extraData?.namespace ?? '')
// ns 段展示文案：有 ns 显示 ns 名（内置用中文 label）；无 ns 时可编辑显示 "+",不可编辑显示空（不显示段）
const nsSegmentText = computed(() => {
  if (editingNs.value) {
    return BUILTIN_NAMESPACES.find((n) => n.value === editingNs.value)?.label ?? editingNs.value
  }
  return editable.value ? '+' : ''
})
// 派生展示 item：浅拷 + 末尾追加 ns 段（仅当有段文案；不 mutate 原始 item.subLabels，避免编辑回路累加）
const displayItem = computed<SegmentedTagItem>(() => {
  const subs = [...(props.item.subLabels ?? [])]
  if (nsSegmentText.value) subs.push(nsSegmentText.value)
  return { ...props.item, subLabels: subs } as SegmentedTagItem
})
// ns 段序号（仅当有段文案时为末段，否则 -1）
const nsIndex = computed(() => (nsSegmentText.value ? (displayItem.value.subLabels?.length ?? 1) - 1 : -1))

const anchorEl = ref<HTMLElement | null>(null)
const popoverVisible = ref(false)

function onSubLabelClicked(index: number, event: MouseEvent) {
  if (editable.value && index === nsIndex.value) {
    event.stopPropagation() // 阻止冒泡到 SegmentedTag 根 @click，避免触发 tag 移动
    anchorEl.value = event.currentTarget as HTMLElement
    // 手动 toggle：trigger="manual" 下 EP 不自动 toggle/不自动 outside-click，避免与 EP 的 visible 控制竞争导致「开后又关」
    popoverVisible.value = !popoverVisible.value
    return
  }
  emits('subLabelClicked', index, event)
}

// editingNs 变更即回写 extraData.namespace（供父组件 confirm 时读取），并通知父组件 ns 被编辑
// （已绑定区编辑后由 ExchangeBox 移至缓冲区，待重新确认走 upsert 更新）
watch(editingNs, (ns) => {
  props.item.extraData = { ...(props.item.extraData ?? {}), namespace: ns }
  emits('nsEdited')
})

// outside-click 关闭：trigger="manual" 下 EP 不自动 outside-click，手动监听 document mousedown——
// target 在 ns 段（virtual-ref）或 popover 内容（el-select 及其 options）内不关，否则关
function handleOutsideClick(event: MouseEvent) {
  if (!popoverVisible.value) return
  const target = event.target as HTMLElement | null
  if (!target) return
  if (anchorEl.value?.contains(target)) return
  if (target.closest('.namespace-tag-popper')) return
  popoverVisible.value = false
}
watch(popoverVisible, (visible) => {
  if (visible) {
    document.addEventListener('mousedown', handleOutsideClick, true)
  } else {
    document.removeEventListener('mousedown', handleOutsideClick, true)
  }
})
onBeforeUnmount(() => {
  document.removeEventListener('mousedown', handleOutsideClick, true)
})
</script>

<template>
  <segmented-tag
    :item="displayItem"
    :closeable="closeable"
    :variant="variant"
    @clicked="emits('clicked')"
    @main-label-clicked="emits('mainLabelClicked')"
    @sub-label-clicked="onSubLabelClicked"
    @close="emits('close')"
  />
  <el-popover
    v-if="editable"
    :virtual-ref="anchorEl"
    virtual-triggering
    v-model:visible="popoverVisible"
    trigger="manual"
    popper-class="namespace-tag-popper"
    placement="bottom"
    :width="220"
  >
    <el-select
      v-model="editingNs"
      filterable
      allow-create
      default-first-option
      clearable
      :teleported="false"
      placeholder="namespace"
      style="width: 100%"
    >
      <el-option
        v-for="ns in BUILTIN_NAMESPACES"
        :key="ns.value"
        :label="ns.label"
        :value="ns.value"
      />
    </el-select>
  </el-popover>
</template>
