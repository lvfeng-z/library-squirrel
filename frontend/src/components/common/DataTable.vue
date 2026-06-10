<script setup lang="ts" generic="Data, OpParam">
import {computed, onBeforeMount, onMounted, Ref, ref} from 'vue'
import OperationItem from '../../model/util/OperationItem'
import {Thead} from '../../model/util/Thead'
import DataTableOperationResponse from '../../model/util/DataTableOperationResponse'
import PopperInput from './CommentInput/PopperInput.vue'
import CommonInput from '@renderer/components/common/CommentInput/CommonInput.vue'
import {TableColumnCtx, TreeNode} from 'element-plus'
import {arrayNotEmpty} from '@renderer/utils/CommonUtil.ts'
import {getPropByPath, setPropByPath} from '@renderer/utils/ObjectUtil.ts'
import {isBlank} from "@renderer/utils/StringUtil.ts";

// props
const props = withDefaults(
  defineProps<{
    selectable: boolean // 列表是否可选择
    multiSelect: boolean // 列表是否多选
    clickRowSelect?: boolean // 点击行的任意位置进行选中（仅单选生效）
    thead: Thead<Data>[] // 表头信息
    dataKey: string // 数据的唯一标识
    rowClassName?: (data: { row: unknown; rowIndex: number }) => string // 给行添加class的函数
    operationButton?: OperationItem<OpParam>[] // 操作列按钮的文本、图标和代号
    operationWidth?: number // 操作栏宽度
    customOperationButton?: boolean // 是否使用自定义操作栏
    treeData?: boolean // 是否为树形数据
    treeLazy?: boolean // 树形数据是否懒加载
    treeLoad?: (row: unknown, treeNode: TreeNode, resolve: (data: unknown[]) => void) => void // 懒加载处理函数
    border?: boolean
    stripe?: boolean
  }>(),
  { customOperationButton: false, treeData: false, treeLazy: false, border: false, clickRowSelect: false }
)

// onBeforeMount
onBeforeMount(() => {
  initializeThead()
})

// onMounted
onMounted(() => {
  // 获取表格 body 包装器
  const scrollBarWrapper = dataTable.value.$refs.scrollBarRef.wrapRef
  scrollBarWrapper.addEventListener('scroll', () => emits('scroll'))
})

// model
const data = defineModel<unknown[]>('data', { required: true })

// 事件
const emits = defineEmits(['selectionChange', 'buttonClicked', 'rowChanged', 'scroll', 'sortChange'])

// 暴露
defineExpose({
  getVisibleRows,
  getSelectionRows,
  clearSelection,
  toggleRowSelection
})

// 变量
const dataTable = ref() // el-table组件的实例
const currentSelect: Ref<Data[]> = ref([])
const currentSelectKey: Ref<unknown | undefined> = computed(() => {
  if (arrayNotEmpty(currentSelect.value)) {
    return getPropByPath(currentSelect.value[0] as object, props.dataKey)
  } else {
    return undefined
  }
})

// 方法
// 初始化表头
async function initializeThead() {
  for (const item of props.thead) {
    // 给selectData赋值
    if (item.remote) {
      item.refreshSelectData()
    }
  }
}
// 处理选中事件
function handleSelectionChange(event: Data[]) {
  currentSelect.value = event
  emits('selectionChange', currentSelect.value)
}
// 处理排序变化事件
function handleSortChange(column: TableColumnCtx, prop: string, order: never) {
  emits('sortChange', column, prop, order)
}
// 获取当前选中
function getSelectionRows() {
  return currentSelect.value
}
// 取消所有选中
function clearSelection() {
  currentSelect.value.length = 0
  dataTable.value.clearSelection()
}
// 切换选中行
function toggleRowSelection(row: Data, selected?: boolean, ignoreSelectable?: boolean) {
  if (props.multiSelect) {
    dataTable.value.toggleRowSelection(row, selected, ignoreSelectable)
  }
  handleSelectionChange([row])
}
// 处理操作按钮点击事件
function handleRowButtonClicked(operation: { row: Data; operationItem: OperationItem<OpParam> }) {
  const id = getPropByPath<string>(operation.row as object, props.dataKey)
  if (isBlank(id)) {
    throw new Error('操作失败：无法获取行数据的唯一标识，请检查表格组件的dataKey属性配置是否正确')
  }
  const operationResponse: DataTableOperationResponse<Data> = {
    id: id,
    code: operation.operationItem.code,
    data: operation.row
  }
  emits('buttonClicked', operationResponse)
}
// 处理行数据变化
function handleRowChange(row: object) {
  emits('rowChanged', row)
}
// 获取操作栏按钮
function getOperateButton(row): OperationItem<OpParam> {
  if (props.operationButton !== undefined && props.operationButton.length > 0) {
    return props.operationButton.filter((item) => (item.rule === undefined ? true : item.rule(row)))[0]
  } else {
    return { code: '', label: '', icon: '' }
  }
}
// 获取操作栏下拉按钮
function getOperateDropDownButton(row): OperationItem<OpParam>[] {
  if (props.operationButton !== undefined && props.operationButton.length > 0) {
    return props.operationButton.filter((item) => (item.rule === undefined ? true : item.rule(row))).slice(1)
  } else {
    return [{ code: '', label: '', icon: '' }]
  }
}
// 获取可视范围内的行数据
function getVisibleRows(offsetTop?: number, offsetBottom?: number) {
  // 获取表格 body 包装器
  const tableBodyWrapper = dataTable.value.$el.querySelector('.el-table__body-wrapper') as Element

  // 获取所有行中class为row-key-col的元素（添加在每一行末尾的隐藏列）
  const rowElements = tableBodyWrapper.querySelectorAll('.row-key-col')

  // 返回可视区域内的行的键
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
    <el-table-column
      v-if="props.selectable && props.multiSelect"
      :fixed="true"
      type="selection"
      width="35"
      :reserve-selection="props.multiSelect"
    />
    <el-table-column
      v-if="props.selectable && !props.multiSelect"
      :fixed="true"
      width="35"
    >
      <template #default="{ row }">
        <el-radio
          v-model="currentSelectKey"
          :value="getPropByPath(row, dataKey)"
          @click="handleSelectionChange([row])"
        />
      </template>
    </el-table-column>
    <template v-for="(item, index) in props.thead">
      <template v-if="!item.hide">
        <el-table-column
          :key="index"
          :prop="item.key"
          :label="item.title"
          :width="item.width"
          :min-width="item.minWidth"
          :align="item.dataAlign"
          :fixed="item.fixed"
          :sortable="item.sortable"
          :sort-method="item.sortMethod"
          :sort-by="item.sortBy"
          :show-overflow-tooltip="item.showOverflowTooltip"
        >
          <template #header>
            <div :style="{ textAlign: item.headerAlign }">
              <el-tag
                size="default"
                :type="item.headerTagType"
              >
                {{ item.title }}
              </el-tag>
            </div>
          </template>
          <template #default="scope">
            <component
              :is="item.editMethod === 'popper' ? PopperInput : CommonInput"
              class="data-table-data-cell"
              :data="getPropByPath(scope.row, item.key)"
              :config="item"
              :cache-data="item.getCacheData(scope.row)"
              :extra-data="scope.row"
              @data-changed="handleRowChange(scope.row)"
              @update:data="(newValue: unknown) => setPropByPath(scope.row, item.key, newValue)"
              @update:cache-data="(newData) => item.setCacheData(scope.row, newData)"
            />
          </template>
        </el-table-column>
      </template>
    </template>
    <el-table-column
      v-if="(props.operationButton !== undefined && props.operationButton.length > 0) || props.customOperationButton"
      fixed="right"
      align="center"
      :width="operationWidth"
      :min-width="104"
    >
      <template #header>
        <el-tag
          size="default"
          type="warning"
        >
          {{ '操作' }}
        </el-tag>
      </template>
      <template #default="{ row }">
        <el-dropdown v-if="!customOperationButton">
          <el-button
            :type="getOperateButton(row).buttonType"
            :icon="getOperateButton(row).icon"
            @click="handleRowButtonClicked({ row: row, operationItem: getOperateButton(row) })"
          >
            {{ getOperateButton(row).label }}
          </el-button>
          <template #dropdown>
            <el-dropdown-menu>
              <template
                v-for="item in getOperateDropDownButton(row)"
                :key="item.code"
              >
                <el-dropdown-item
                  :icon="item.icon"
                  @click="handleRowButtonClicked({ row: row, operationItem: item })"
                >
                  {{ item.label }}
                </el-dropdown-item>
              </template>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
        <slot
          v-if="customOperationButton"
          name="customOperations"
          :row="row"
        />
      </template>
    </el-table-column>
    <el-table-column
      :hidden="true"
      :width="1"
    >
      <template #default="scope">
        <div
          :row-key="getPropByPath(scope.row, dataKey)"
          class="row-key-col"
          style="width: 0; height: 0; position: absolute"
        />
      </template>
    </el-table-column>
  </el-table>
</template>

<style scoped>
:deep(.data-table-header-cell .cell) {
  display: flex;
  justify-content: center; /* 水平居中，根据需求可改为 space-between 或 flex-start */
  align-items: center; /* 垂直居中 */
  gap: 1px; /* 两个元素之间的间距 */
}
.data-table-data-cell {
  display: inline-block;
}
</style>
