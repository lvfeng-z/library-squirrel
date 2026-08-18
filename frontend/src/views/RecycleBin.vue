<script setup lang="ts">
import BaseView from './BaseView.vue'
import SearchTable from '@renderer/components/common/SearchTable.vue'
import AutoLoadSelect from '@renderer/components/common/AutoLoadSelect.vue'
import { onMounted, Ref, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { recycleBinApi } from '@renderer/apis/http'
import { localAuthorQuerySelectItemPageByName, localTagQuerySelectItemPageByName, siteQuerySelectItemPageBySiteName } from '@renderer/apis/http'
import ApiUtil from '@renderer/utils/ApiUtil.ts'
import { Thead } from '@renderer/model/util/Thead.ts'
import { newPage } from '@renderer/utils/Pager.ts'
import { RecycleQueryDTO, type RecycleItemDTO } from '@bindings/github.com/library-squirrel/backend/recycleBin/models'
import { Operator } from '@bindings/github.com/library-squirrel/backend/base/query/models'
import type { Page } from '@bindings/github.com/library-squirrel/backend/base/model/models'
import { isNullish } from '@renderer/utils/CommonUtil.ts'

// onMounted
onMounted(() => {
  recycleBinSearchTable.value.doSearch()
})

// 变量
// 回收站数据表组件的实例
const recycleBinSearchTable = ref()
// 回收站分页参数
const page: Ref<Page<RecycleItemDTO>> = ref(newPage<RecycleItemDTO>())
// 回收站查询参数（时间范围/站点/作者/标签 + 排序）
const query: Ref<RecycleQueryDTO> = ref(new RecycleQueryDTO())
// 时间范围控件值（Date 二元组，change 时转毫秒时间戳进 query；null = 清空范围）
const deleteTimeRange = ref<[Date, Date] | null>(null)
const workCreateTimeRange = ref<[Date, Date] | null>(null)
// 选择器选中值（SelectItem.value 为 string，提交查询时统一 Number 转换）
const siteIdSelected = ref<string | number | null>(null)
const localAuthorIdSelected = ref<string | number | null>(null)
const localTagIdSelected = ref<string | number | null>(null)
// 回收站的表头
const recycleBinThead: Ref<Thead<RecycleItemDTO>[]> = ref([
  new Thead({
    type: 'text',
    defaultDisabled: true,
    key: 'workName',
    title: '作品名',
    hide: false,
    headerAlign: 'center',
    dataAlign: 'center',
    showOverflowTooltip: true
  }),
  new Thead({
    type: 'text',
    defaultDisabled: true,
    key: 'siteName',
    title: '站点',
    hide: false,
    width: 120,
    headerAlign: 'center',
    dataAlign: 'center',
    showOverflowTooltip: true
  }),
  new Thead({
    type: 'text',
    defaultDisabled: true,
    key: 'authorNames',
    title: '作者',
    hide: false,
    width: 180,
    headerAlign: 'center',
    dataAlign: 'center',
    showOverflowTooltip: true
  }),
  new Thead({
    type: 'datetime',
    defaultDisabled: true,
    key: 'workCreateTime',
    title: '创建时间',
    hide: false,
    width: 200,
    headerAlign: 'center',
    dataAlign: 'center',
    sortable: 'custom',
    showOverflowTooltip: true
  }),
  new Thead({
    type: 'datetime',
    defaultDisabled: true,
    key: 'deleteTime',
    title: '删除时间',
    hide: false,
    width: 200,
    headerAlign: 'center',
    dataAlign: 'center',
    sortable: 'custom',
    showOverflowTooltip: true
  })
])

// 方法
// 分页查询回收站列表（ID 类选择值经 Number 转换后随 query 提交）
async function recycleBinQueryPageFn(p: Page<RecycleItemDTO>): Promise<Page<RecycleItemDTO> | undefined> {
  query.value.siteId.value = isNullish(siteIdSelected.value) ? null : Number(siteIdSelected.value)
  query.value.localAuthorId = isNullish(localAuthorIdSelected.value) ? null : Number(localAuthorIdSelected.value)
  query.value.localTagId = isNullish(localTagIdSelected.value) ? null : Number(localTagIdSelected.value)
  const response = await recycleBinApi.recycleBinPage(p, query.value)
  return response.data
}
// 时间范围控件值转查询属性（起 gte / 止 lte，毫秒时间戳）
function toRangeAttr(time: Date | undefined, operator: Operator): { value: number | null; operator: Operator } {
  return { value: isNullish(time) ? null : time.getTime(), operator }
}
// 处理删除时间范围变化
function handleDeleteTimeRangeChange(range: [Date, Date] | null) {
  deleteTimeRange.value = range
  query.value.deleteTimeStart = toRangeAttr(range?.[0], Operator.OpGte)
  query.value.deleteTimeEnd = toRangeAttr(range?.[1], Operator.OpLte)
}
// 处理创建时间范围变化
function handleWorkCreateTimeRangeChange(range: [Date, Date] | null) {
  workCreateTimeRange.value = range
  query.value.workCreateTimeStart = toRangeAttr(range?.[0], Operator.OpGte)
  query.value.workCreateTimeEnd = toRangeAttr(range?.[1], Operator.OpLte)
}
// 处理列头排序变化（后端排序；取消排序时回落默认 deleteTime DESC）
function handleSortChange(sortData: { column: unknown; prop: string; order: 'ascending' | 'descending' | null }) {
  if (isNullish(sortData.order)) {
    query.value.sortBy = null
    query.value.sortOrder = null
  } else {
    query.value.sortBy = sortData.prop
    query.value.sortOrder = sortData.order === 'ascending' ? 'asc' : 'desc'
  }
  recycleBinSearchTable.value.doSearch()
}
// 复原回收站条目
async function restore(item: RecycleItemDTO) {
  const response = await recycleBinApi.recycleBinRestore(item.id, false)
  if (ApiUtil.check(response)) {
    ElMessage.success('复原成功')
    await recycleBinSearchTable.value.doSearch()
    return
  }
  // 后端仅冲突时返回 ErrRestoreConflict（消息含“冲突”/“已存在”）；其他错误直接展示，不误弹覆盖框
  const failMsg = response?.msg ?? '复原失败'
  if (!failMsg.includes('冲突') && !failMsg.includes('已存在')) {
    ElMessage.error(failMsg)
    return
  }
  try {
    await ElMessageBox.confirm(`${failMsg}\n是否覆盖已存在的作品？`, '复原冲突', {
      confirmButtonText: '覆盖',
      cancelButtonText: '取消',
      type: 'warning'
    })
    const overwriteResponse = await recycleBinApi.recycleBinRestore(item.id, true)
    if (ApiUtil.check(overwriteResponse)) {
      ElMessage.success('复原成功')
      await recycleBinSearchTable.value.doSearch()
    } else {
      ElMessage.error(overwriteResponse?.msg ?? '复原失败')
    }
  } catch {
    // 用户取消
  }
}
// 彻底删除回收站条目
async function purge(item: RecycleItemDTO) {
  try {
    await ElMessageBox.confirm('彻底删除后不可恢复，是否继续？', '彻底删除', {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning'
    })
    const response = await recycleBinApi.recycleBinPurge(item.id)
    if (ApiUtil.check(response)) {
      ElMessage.success('已彻底删除')
      await recycleBinSearchTable.value.doSearch()
    } else {
      ElMessage.error(response?.msg ?? '删除失败')
    }
  } catch {
    // 用户取消
  }
}
</script>

<template>
  <base-view>
    <template #default>
      <div class="recycle-bin-container">
        <search-table
          ref="recycleBinSearchTable"
          v-model:page="page"
          class="recycle-bin-search-table"
          toolbar-radius="var(--app-radius)"
          data-radius="var(--app-radius)"
          data-key="id"
          :thead="recycleBinThead"
          :search="recycleBinQueryPageFn"
          :selectable="false"
          :multi-select="false"
          :custom-operation-button="true"
          :operation-width="200"
          @sort-change="handleSortChange"
        >
          <template #toolbarMain>
            <auto-load-select
              v-model:data="siteIdSelected"
              class="recycle-bin-filter-select"
              :load="siteQuerySelectItemPageBySiteName"
              placeholder="选择站点"
              remote
              filterable
              clearable
            >
              <template #default="{ list }">
                <el-option
                  v-for="item in list"
                  :key="item.value"
                  :value="item.value"
                  :label="item.label"
                />
              </template>
            </auto-load-select>
            <auto-load-select
              v-model:data="localAuthorIdSelected"
              class="recycle-bin-filter-select"
              :load="localAuthorQuerySelectItemPageByName"
              placeholder="选择作者"
              remote
              filterable
              clearable
            >
              <template #default="{ list }">
                <el-option
                  v-for="item in list"
                  :key="item.value"
                  :value="item.value"
                  :label="item.label"
                />
              </template>
            </auto-load-select>
            <auto-load-select
              v-model:data="localTagIdSelected"
              class="recycle-bin-filter-select"
              :load="localTagQuerySelectItemPageByName"
              placeholder="选择标签"
              remote
              filterable
              clearable
            >
              <template #default="{ list }">
                <el-option
                  v-for="item in list"
                  :key="item.value"
                  :value="item.value"
                  :label="item.label"
                />
              </template>
            </auto-load-select>
            <el-date-picker
              v-model="deleteTimeRange"
              class="recycle-bin-filter-range"
              type="datetimerange"
              start-placeholder="删除时间起"
              end-placeholder="删除时间止"
              @change="handleDeleteTimeRangeChange"
            />
            <el-date-picker
              v-model="workCreateTimeRange"
              class="recycle-bin-filter-range"
              type="datetimerange"
              start-placeholder="创建时间起"
              end-placeholder="创建时间止"
              @change="handleWorkCreateTimeRangeChange"
            />
          </template>
          <!-- 破坏性按钮需保真 danger+tone-fail 红色形态，内置 operationButton 下拉项无 danger 样式，故走自定义操作列 -->
          <template #customOperations="{ row }">
            <el-button size="small" type="primary" @click="restore(row as RecycleItemDTO)">复原</el-button>
            <el-button size="small" type="danger" class="tone-fail" @click="purge(row as RecycleItemDTO)">彻底删除</el-button>
          </template>
        </search-table>
      </div>
    </template>
  </base-view>
</template>

<style scoped>
.recycle-bin-container {
  display: flex;
  flex-direction: row;
  justify-content: center;
  align-items: center;
  /* 容器不带底色：一体感由 SearchTable 自身的工具栏面与数据面（含分页面）连成的卡片承担；间距纯 margin（总边距 10px 不变） */
  width: calc(100% - 20px);
  height: calc(100% - 20px);
  margin: 10px;
}

.recycle-bin-search-table {
  height: 100%;
  width: 100%;
}

.recycle-bin-filter-select {
  width: 180px;
  flex-shrink: 0;
}

.recycle-bin-filter-range {
  width: 340px;
  flex-shrink: 0;
}
</style>
