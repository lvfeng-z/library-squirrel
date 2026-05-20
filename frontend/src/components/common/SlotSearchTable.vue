<script setup lang="ts" generic="Data extends object">
import SearchToolbar from '@renderer/components/common/SearchToolbar.vue'
import { ref } from 'vue'
import { Page } from '@bindings/github.com/library-squirrel/backend/base/model/models.ts'
import lodash from 'lodash'
import { isNullish, notNullish } from '@renderer/utils/CommonUtil.ts'
import { TableColumnCtx, TreeNode as ElTreeNode } from 'element-plus'
import { getPropByPath } from '@renderer/utils/ObjectUtil.ts'
import SlotDataTable from '@renderer/components/common/SlotDataTable.vue'

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
    treeLoad?: (row: Data) => Promise<Data[]>
    border?: boolean
    stripe?: boolean
    search: (page: Page<Data>) => Promise<Page<Data> | undefined>
    createButton?: boolean
    pageSizes?: number[]
    searchButtonDisabled?: boolean
  }>(),
  {
    createButton: false,
    pageSizes: () => [10, 20, 30, 50, 100],
    treeLazy: false,
    border: false
  }
)

// model
const data = defineModel<Data[]>('data', { default: [], required: false })
const page = defineModel<Page<Data>>('page', { required: true })
const toolbarParams = defineModel<object>('toolbarParams', { default: {}, required: false })
const sort = defineModel<{ prop: string; order: 'ascending' | 'descending' | null }>('sort', {
  default: { prop: '', order: null },
  required: false
})

// 事件
const emits = defineEmits([
  'createButtonClicked',
  'selectionChange',
  'pageNumberChanged',
  'pageSizeChanged',
  'query',
  'scroll',
  'sortChange'
])

// 暴露
defineExpose({
  doSearch,
  clearData,
  getVisibleRows,
  getSelectionRows,
  toggleRowSelection
})

// 变量
const dataTableRef = ref()
const layout = ref('sizes, prev, pager, next')
const pagerCount = ref(5)
const treeRefreshMap: Map<number, { treeNode: ElTreeNode; resolve: (data: unknown[]) => void }> = new Map<
  number,
  { treeNode: ElTreeNode; resolve: (data: unknown[]) => void }
>()
const wrappedLoad = isNullish(props.treeLoad)
  ? undefined
  : async (row: unknown, treeNode: ElTreeNode, resolve: (data: unknown[]) => void) => {
      const rowId = Number(getPropByPath(row as object, props.dataKey))
      if (!treeRefreshMap.has(rowId)) {
        treeRefreshMap.set(rowId, { treeNode: treeNode, resolve: resolve })
      }
      if (notNullish(props.treeLoad)) {
        const children = await props.treeLoad(row)
        resolve(children)
      }
    }

// 方法
async function doSearch() {
  dataTableRef.value.clearSelection()
  const tempPage = lodash.cloneDeep(page.value)
  if (!tempPage.pageSize || tempPage.pageSize <= 0) {
    tempPage.pageSize = props.pageSizes[0]
    page.value.pageSize = tempPage.pageSize
  }
  const newPage: Page<Data> | undefined = await props.search(tempPage)
  if (notNullish(newPage)) {
    data.value = newPage.data as Data[]
    page.value.dataCount = newPage.dataCount
    page.value.pageCount = newPage.pageCount
  }
  if (notNullish(props.treeLoad) && notNullish(wrappedLoad)) {
    if (notNullish(data.value)) {
      data.value.forEach((row) => {
        const treeInitItem = treeRefreshMap.get(getPropByPath(row as object, props.dataKey))
        if (notNullish(treeInitItem)) {
          wrappedLoad(row, treeInitItem.treeNode, treeInitItem.resolve)
        }
      })
    }
  }
}
function handleDataTableSelectionChange(selections: []) {
  emits('selectionChange', selections)
}
function handleScroll() {
  emits('scroll')
}
function handleSortChange(sortData: { column: TableColumnCtx; prop: string; order: never }) {
  sort.value = { prop: sortData.prop, order: sortData.order as 'ascending' | 'descending' | null }
  emits('sortChange', sortData)
}
function clearData() {
  data.value.length = 0
}
function handlePageNumberChange() {
  doSearch()
}
function handlePageSizeChange() {
  doSearch()
}
function getVisibleRows(offsetTop?: number, offsetBottom?: number) {
  return dataTableRef.value.getVisibleRows(offsetTop, offsetBottom)
}
function getSelectionRows() {
  return dataTableRef.value.getSelectionRows()
}
function toggleRowSelection(row: Data, selected?: boolean, ignoreSelectable?: boolean) {
  dataTableRef.value.toggleRowSelection(row, selected, ignoreSelectable)
}
</script>

<template>
  <div class="search-table">
    <search-toolbar
      class="search-table-toolbar z-layer-3"
      :search-button-disabled="searchButtonDisabled"
      @search-button-clicked="doSearch"
    >
      <template #main>
        <slot name="toolbarMain" />
      </template>
      <template #dropdown>
        <slot name="toolbarDropdown" />
      </template>
    </search-toolbar>
    <div class="search-table-data">
      <slot-data-table
        ref="dataTableRef"
        v-model:data="data"
        class="search-table-data-table"
        :selectable="selectable"
        :multi-select="multiSelect"
        :click-row-select="clickRowSelect"
        :data-key="dataKey"
        :row-class-name="rowClassName"
        :tree-data="treeData"
        :tree-lazy="props.treeLazy"
        :tree-load="wrappedLoad"
        :border="border"
        :stripe="stripe"
        @selection-change="handleDataTableSelectionChange"
        @scroll="handleScroll"
        @sort-change="handleSortChange"
      >
        <slot />
      </slot-data-table>
      <div class="search-table-data-pagination-scroll-wrapper">
        <el-scrollbar class="search-table-data-pagination-scroll">
          <div class="search-table-data-pagination-wrapper">
            <el-pagination
              v-model:current-page="page.pageNumber"
              v-model:page-size="page.pageSize"
              class="search-table-data-pagination"
              :layout="layout"
              :page-sizes="pageSizes"
              :default-page-size="page.pageSize"
              :pager-count="pagerCount"
              :total="page.dataCount"
              @current-change="handlePageNumberChange"
              @size-change="handlePageSizeChange"
            />
          </div>
        </el-scrollbar>
      </div>
    </div>
  </div>
</template>

<style scoped>
.search-table {
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
}
.search-table-toolbar {
  height: 32px;
  width: 100%;
  background-color: var(--el-fill-color-blank);
  border-top-left-radius: 6px;
  border-top-right-radius: 6px;
}
.search-table-data {
  display: flex;
  flex-direction: column;
  align-items: center;
  height: calc(100% - 37px);
  width: 100%;
  margin-top: 5px;
}
.search-table-data-table {
  flex-grow: 1;
}
.search-table-data-pagination-scroll-wrapper {
  width: 100%;
  height: auto;
}
.search-table-data-pagination-scroll {
  height: auto;
  width: 100%;
}
.search-table-data-pagination-wrapper {
  width: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  background-color: #fdfdfd;
  border-bottom-left-radius: 6px;
  border-bottom-right-radius: 6px;
}
.search-table-data-pagination {
  height: auto;
  width: auto;
}
</style>
