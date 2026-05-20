<script setup lang="ts" generic="Data">
import {computed, onMounted, Ref, ref} from 'vue'
import {TableColumnCtx, TreeNode} from 'element-plus'
import {getPropByPath} from '@renderer/utils/ObjectUtil.ts'

// props
const props = withDefaults(
  defineProps<{
    selectable: boolean
    multiSelect: boolean
    clickRowSelect?: boolean
    dataKey: string
    rowClassName?: (data: { row: unknown; rowIndex: number }) => string
    treeData?: boolean
    treeLazy?: boolean
    treeLoad?: (row: unknown, treeNode: TreeNode, resolve: (data: unknown[]) => void) => void
    border?: boolean
    stripe?: boolean
  }>(),
  { treeData: false, treeLazy: false, border: false, clickRowSelect: false }
)

// onMounted
onMounted(() => {
  const scrollBarWrapper = dataTable.value.$refs.scrollBarRef.wrapRef
  scrollBarWrapper.addEventListener('scroll', () => emits('scroll'))
})

// model
const data = defineModel<unknown[]>('data', { required: true })

// 事件
const emits = defineEmits(['selectionChange', 'scroll', 'sortChange'])

// 暴露
defineExpose({
  getVisibleRows,
  getSelectionRows,
  clearSelection,
  toggleRowSelection
})

// 变量
const dataTable = ref()
const currentSelect: Ref<Data[]> = ref([])
const currentSelectKey: Ref<unknown | undefined> = computed(() => {
  if (Array.isArray(currentSelect.value) && currentSelect.value.length > 0) {
    return getPropByPath(currentSelect.value[0] as object, props.dataKey)
  } else {
    return undefined
  }
})

// 方法
function handleSelectionChange(event: Data[]) {
  currentSelect.value = event
  emits('selectionChange', currentSelect.value)
}
function handleSortChange(column: TableColumnCtx, prop: string, order: never) {
  emits('sortChange', column, prop, order)
}
function getSelectionRows() {
  return currentSelect.value
}
function clearSelection() {
  currentSelect.value.length = 0
  dataTable.value.clearSelection()
}
function toggleRowSelection(row: Data, selected?: boolean, ignoreSelectable?: boolean) {
  if (props.multiSelect) {
    dataTable.value.toggleRowSelection(row, selected, ignoreSelectable)
  }
  handleSelectionChange([row])
}
function getVisibleRows(offsetTop?: number, offsetBottom?: number) {
  const tableBodyWrapper = dataTable.value.$el.querySelector('.el-table__body-wrapper') as Element
  const rowElements = tableBodyWrapper.querySelectorAll('.row-key-col')
  return Array.from(rowElements)
    .filter((row) => {
      const rowTop = row.getBoundingClientRect().top
      const rowBottom = row.getBoundingClientRect().bottom
      offsetTop = offsetTop || 0
      offsetBottom = offsetBottom || 0
      return (
        rowTop < tableBodyWrapper.getBoundingClientRect().bottom + offsetBottom &&
        rowBottom > tableBodyWrapper.getBoundingClientRect().top - offsetTop
      )
    })
    .map((rowElement) => {
      try {
        return rowElement.attributes['row-key']['value']
      } catch (error) {
        console.warn(error)
        return undefined
      }
    })
}
</script>

<template>
  <el-table
    ref="dataTable"
    class="data-table"
    :lazy="props.treeLazy"
    :load="props.treeLoad"
    :data="data"
    :row-key="dataKey"
    :row-class-name="rowClassName"
    :tree-props="treeData ? { hasChildren: 'hasChildren', children: 'children' } : undefined"
    :selectable="props.selectable"
    :border="props.border"
    :stripe="props.stripe"
    :header-cell-class-name="'data-table-header-cell'"
    @current-change="(current: Data) => (clickRowSelect ? handleSelectionChange([current]) : undefined)"
    @selection-change="handleSelectionChange"
    @sort-change="handleSortChange"
  >
    <el-table-column v-if="props.selectable && props.multiSelect" :fixed="true" type="selection" width="35" :reserve-selection="props.multiSelect" />
    <el-table-column v-if="props.selectable && !props.multiSelect" :fixed="true" width="35">
      <template #default="{ row }">
        <el-radio v-model="currentSelectKey" :value="getPropByPath(row, dataKey)" @click="handleSelectionChange([row])" />
      </template>
    </el-table-column>
    <slot />
    <el-table-column :hidden="true" :width="1">
      <template #default="scope">
        <div :row-key="getPropByPath(scope.row, dataKey)" class="row-key-col" style="width: 0; height: 0; position: absolute" />
      </template>
    </el-table-column>
  </el-table>
</template>

<style scoped>
:deep(.data-table-header-cell .cell) {
  display: flex;
  justify-content: center;
  align-items: center;
  gap: 1px;
}
</style>
