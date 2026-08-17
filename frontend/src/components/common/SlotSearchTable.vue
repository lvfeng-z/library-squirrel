<script setup lang="ts" generic="Data extends object">
import SearchToolbar from '@renderer/components/common/SearchToolbar.vue'
import { computed, ref } from 'vue'
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
    toolbarBackground?: string // 工具栏块底色（CSS color 值）
    dataBackground?: string // 数据栏块底色（默认透明：表体与分页壳各自承担底色）
    toolbarDataGap?: string // 工具栏与数据栏之间的间隔高度
    toolbarRadius?: string // 工具栏块圆角（默认上圆角下直角，与历史视觉一致）
    dataRadius?: string // 数据栏块圆角（默认无：底色透明时不可见）
  }>(),
  {
    createButton: false,
    pageSizes: () => [10, 20, 30, 50, 100],
    treeLazy: false,
    border: false,
    toolbarBackground: 'var(--app-bg-surface)',
    dataBackground: 'transparent',
    toolbarDataGap: '5px',
    toolbarRadius: 'var(--app-radius) var(--app-radius) 0 0',
    dataRadius: '0'
  }
)

// 数据栏底色是否为自定义（非透明默认）：自定义时表体/分页壳整体让位透明，放行容器底色
const isDataBgCustom = computed(() => props.dataBackground !== 'transparent')

// model
const data = defineModel<Data[]>('data', { default: [], required: false })
const page = defineModel<Page<Data>>('page', { required: true })
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
  : async (row: Data, treeNode: ElTreeNode, resolve: (data: unknown[]) => void) => {
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
  // 搜索后重新建立 dataList.value[i].children 与 el-table 内部子节点之间的引用关联
  if (notNullish(props.treeLoad) && notNullish(wrappedLoad)) {
    if (notNullish(data.value)) {
      data.value.forEach((row) => {
        const taskId = getPropByPath(row as object, props.dataKey) as number
        const treeInitItem = treeRefreshMap.get(taskId)
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
    <div :class="{ 'search-table-data': true, 'search-table-data-custom-bg': isDataBgCustom }">
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
/* 工具栏/数据栏分离视觉的皮肤参数（props 经 v-bind 注入，与 SearchTable 同构，默认值与历史视觉一致） */
.search-table-toolbar {
  width: 100%;
  background-color: v-bind('props.toolbarBackground');
  border-radius: v-bind('props.toolbarRadius');
}
.search-table-data {
  display: flex;
  flex-direction: column;
  align-items: center;
  /* 工具栏多行自适应后高度由 flex 分配（原 calc(100% - 37px) 按 32px 工具栏硬补偿） */
  flex: 1;
  min-height: 0;
  width: 100%;
  /* 圆角裁剪子元素（表体/分页面方角），否则容器圆角被越出绘制盖掉 */
  overflow: hidden;
  background-color: v-bind('props.dataBackground');
  border-radius: v-bind('props.dataRadius');
  margin-top: v-bind('props.toolbarDataGap');
}
/* 数据栏自定义底色时放行容器底色：el-table 底色走 CSS 变量级覆盖（EP 内部全部级联），
   默认不命中本规则，EP 原生观感零改动 */
.search-table-data-custom-bg :deep(.el-table) {
  --el-table-bg-color: transparent;
  --el-table-tr-bg-color: transparent;
  --el-table-header-bg-color: transparent;
  --el-table-expanded-cell-bg-color: transparent;
}
.search-table-data-custom-bg .search-table-data-pagination-wrapper {
  background-color: transparent;
}
.search-table-data-custom-bg .search-table-data-pagination-scroll {
  background-color: transparent;
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
  background-color: var(--app-bg-surface);
  border-bottom-left-radius: 6px;
  border-bottom-right-radius: 6px;
}
.search-table-data-pagination {
  height: auto;
  width: auto;
}
</style>
