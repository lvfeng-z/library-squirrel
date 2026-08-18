<script setup lang="ts">
import BaseView from './BaseView.vue'
import SearchTable from '@renderer/components/common/SearchTable.vue'
import AutoLoadSelect from '@renderer/components/common/AutoLoadSelect.vue'
import AutoLoadTagSelect from '@renderer/components/common/AutoLoadTagSelect.vue'
import { onMounted, Ref, ref, h } from 'vue'
import { ElMessage, ElMessageBox, ElPopover, ElImage } from 'element-plus'
import lodash from 'lodash'
import { recycleBinApi, siteQuerySelectItemPageBySiteName } from '@renderer/apis/http'
import { searchQuerySearchConditionPage } from '@renderer/apis/http/wrappers/search'
import { setSearchTagColor } from '@renderer/utils/SearchTagColorUtil.js'
import { buildStoreUrl } from '@renderer/utils/UrlUtil.ts'
import { Thead } from '@renderer/model/util/Thead.ts'
import { newPage } from '@renderer/utils/Pager.ts'
import SegmentedTagItem from '@renderer/model/util/SegmentedTagItem.ts'
import IPage from '@renderer/model/util/IPage.ts'
import { SearchCondition, SearchType, WorkSearchOperator, RecycleWorkDTO } from '@bindings/github.com/library-squirrel/backend/base/model/dto'
import { SearchConditionQuery } from '@bindings/github.com/library-squirrel/backend/base/model/dto'
import { SelectItem } from '@bindings/github.com/library-squirrel/backend/base/model/dto'
import { RecyclePageQuery } from '@bindings/github.com/library-squirrel/backend/recycleBin/models'
import type { Page } from '@bindings/github.com/library-squirrel/backend/base/model/models'
import { isBlank, isNotBlank } from '@renderer/utils/StringUtil.ts'
import { arrayNotEmpty, isNullish, notNullish } from '@renderer/utils/CommonUtil.ts'
import ApiUtil from '@renderer/utils/ApiUtil.ts'

// onMounted
onMounted(() => {
  recycleBinSearchTable.value.doSearch()
})

// 变量
// 回收站数据表组件的实例
const recycleBinSearchTable = ref()
// 标签选择器（AutoLoadTagSelect）实例
const searchConditionBar = ref()
// 回收站分页参数
const page: Ref<Page<RecycleWorkDTO>> = ref(newPage<RecycleWorkDTO>())
// 回收站查询参数（SearchCondition 条件体系 + 排序）
const query: Ref<RecyclePageQuery> = ref(new RecyclePageQuery({ conditions: [] }))
// 标签条已选条目（extraData 携带 type/id/namespace；disabled 态 = 排除该条件）
const selectedTagList: Ref<SegmentedTagItem[]> = ref([])
// 标签条自定义文本条目（作品名关键词标签）
const customTagList: Ref<SegmentedTagItem[]> = ref([])
// 标签条输入框文本（回车后进 customTagList）
const autoLoadInput: Ref<string | undefined> = ref(undefined)
// 候选条件的类型开关（空 = 全部四类，与作品搜索一致）
const searchConditionType: Ref<SearchType[]> = ref([])
// 时间范围控件值（Date 二元组，查询时转毫秒时间戳条件；null = 清空范围）
const deleteTimeRange = ref<[Date, Date] | null>(null)
const workCreateTimeRange = ref<[Date, Date] | null>(null)
const uploadTimeRange = ref<[Date, Date] | null>(null)
// 站点选择（SelectItem.value 为 string，组装条件时 Number 转换）
const siteIdSelected = ref<string | number | null>(null)
// 回收站的表头
const recycleBinThead: Ref<Thead<RecycleWorkDTO>[]> = ref([
  new Thead({
    type: 'custom',
    defaultDisabled: true,
    key: 'workName',
    title: '作品名',
    hide: false,
    headerAlign: 'center',
    dataAlign: 'center',
    // 悬停弹出预览图（图片选择与作品卡片同优先级：缩略图优先、图片资源主图回退，由后端投影 previewPath 决定）；
    // show-after 与单元格溢出 tooltip 错峰，避免双气泡叠加
    render: (data, extraData) => {
      const row = extraData as RecycleWorkDTO
      const name = (data as string | null) ?? ''
      if (isNullish(row) || isBlank(row.previewPath)) {
        return h('span', name)
      }
      return h(
        ElPopover,
        { trigger: 'hover', placement: 'top', width: 'auto', showAfter: 400 },
        {
          reference: () => h('span', name),
          default: () =>
            h(ElImage, {
              src: buildStoreUrl(row.previewPath),
              fit: 'contain',
              style: 'max-width: 360px; max-height: 240px;'
            })
        }
      )
    }
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
    key: 'createTime',
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
  }),
  new Thead({
    type: 'text',
    defaultDisabled: true,
    key: 'expireDaysLeft',
    title: '剩余天数',
    hide: false,
    width: 100,
    headerAlign: 'center',
    dataAlign: 'center',
    showOverflowTooltip: true
  })
])

// 方法
// 查询标签/作者候选列表（类型开关经 types 下发，空 = 全部）
async function querySearchItemPage(p: IPage<SelectItem>, input?: string): Promise<IPage<SelectItem>> {
  const conditionQuery = new SearchConditionQuery({ keyword: isBlank(input) ? undefined : input })
  conditionQuery.types = lodash.cloneDeep(searchConditionType.value)
  const response = await searchQuerySearchConditionPage(newPage<SelectItem>({ pageNumber: p.pageNumber, pageSize: p.pageSize }), conditionQuery)
  const data = ApiUtil.data<Page<SelectItem>>(response)
  if (isNullish(data)) {
    ApiUtil.msg(response)
    throw new Error(response?.msg ?? '查询搜索条件失败')
  }
  return data
}
// 类型开关变化后重新加载候选
function handleConditionTypeChange() {
  searchConditionBar.value.newSearch()
}
// namespace 由已选 tag 的 ns 段编辑写入 extraData.namespace（local=用户点 ns 段设搜索 ns、site=站点自带固定 ns）；author 不带
// 空串视作未设：DB 空串落 NULL，无命中
function resolveSearchNamespace(extraData: { type: SearchType; namespace?: string }): string | undefined {
  if (extraData.type === SearchType.LocalTag || extraData.type === SearchType.SiteTag) {
    return extraData.namespace || undefined
  }
  return undefined
}
// 组装查询条件（与作品搜索同构：多标签组合 + 排除 + namespace + 关键词 + 站点 + 三组时间范围）
function buildConditions(): SearchCondition[] {
  const conditions: SearchCondition[] = []
  // 标签/作者多选条（disabled 态转 NotEqual 排除）
  selectedTagList.value.forEach((tag) => {
    if (isNullish(tag.extraData)) {
      return
    }
    const extraData = tag.extraData as { type: SearchType; id: number; namespace?: string }
    const operator = notNullish(tag.disabled) && tag.disabled ? WorkSearchOperator.NotEqual : undefined
    conditions.push(new SearchCondition({ type: extraData.type, value: extraData.id, operator: operator, namespace: resolveSearchNamespace(extraData) }))
  })
  // 自定义文本标签 → 作品名 LIKE
  customTagList.value.forEach((tag) => {
    conditions.push(new SearchCondition({ type: SearchType.WorksSiteName, value: tag.value, operator: WorkSearchOperator.Like }))
  })
  // 输入框关键词 → 作品名 LIKE（不并推昵称条件：双 AND 会滤掉无昵称作品）
  if (isNotBlank(autoLoadInput.value)) {
    conditions.push(new SearchCondition({ type: SearchType.WorksSiteName, value: autoLoadInput.value, operator: WorkSearchOperator.Like }))
  }
  // 站点
  if (!isNullish(siteIdSelected.value)) {
    conditions.push(new SearchCondition({ type: SearchType.Site, value: Number(siteIdSelected.value) }))
  }
  // 三组时间范围（起 gte / 止 lte 成对）
  if (!isNullish(deleteTimeRange.value)) {
    conditions.push(new SearchCondition({ type: SearchType.WorksDeleteTime, value: deleteTimeRange.value[0].getTime(), operator: WorkSearchOperator.GreaterOrEqual }))
    conditions.push(new SearchCondition({ type: SearchType.WorksDeleteTime, value: deleteTimeRange.value[1].getTime(), operator: WorkSearchOperator.LessOrEqual }))
  }
  if (!isNullish(workCreateTimeRange.value)) {
    conditions.push(new SearchCondition({ type: SearchType.WorksCreateTime, value: workCreateTimeRange.value[0].getTime(), operator: WorkSearchOperator.GreaterOrEqual }))
    conditions.push(new SearchCondition({ type: SearchType.WorksCreateTime, value: workCreateTimeRange.value[1].getTime(), operator: WorkSearchOperator.LessOrEqual }))
  }
  if (!isNullish(uploadTimeRange.value)) {
    conditions.push(new SearchCondition({ type: SearchType.WorksUploadTime, value: uploadTimeRange.value[0].getTime(), operator: WorkSearchOperator.GreaterOrEqual }))
    conditions.push(new SearchCondition({ type: SearchType.WorksUploadTime, value: uploadTimeRange.value[1].getTime(), operator: WorkSearchOperator.LessOrEqual }))
  }
  return conditions
}
// 分页查询回收站列表
async function recycleBinQueryPageFn(p: Page<RecycleWorkDTO>): Promise<Page<RecycleWorkDTO> | undefined> {
  query.value.conditions = buildConditions()
  const response = await recycleBinApi.recycleBinPage(p, query.value)
  return response.data
}
// 处理列头排序变化（后端排序；取消排序时回落默认 deleteTime DESC）
function handleSortChange(sortData: { column: unknown; prop: string; order: 'ascending' | 'descending' | null }) {
  if (isNullish(sortData.order)) {
    query.value.sortBy = null
    query.value.sortOrder = null
  } else {
    query.value.sortBy = sortData.prop === 'createTime' ? 'createTime' : 'deleteTime'
    query.value.sortOrder = sortData.order === 'ascending' ? 'asc' : 'desc'
  }
  recycleBinSearchTable.value.doSearch()
}
// 复原回收站条目（失败消息含「冲突/已存在」时弹覆盖确认，确认后 overwrite=true 重试）
async function restore(item: RecycleWorkDTO) {
  try {
    await recycleBinApi.recycleBinRestore(item.id, false)
    ElMessage.success('复原成功')
    await recycleBinSearchTable.value.doSearch()
  } catch (e) {
    const failMsg = (e as Error).message ?? '复原失败'
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
      await recycleBinApi.recycleBinRestore(item.id, true)
      ElMessage.success('复原成功')
      await recycleBinSearchTable.value.doSearch()
    } catch (e2) {
      // ElMessageBox 取消为字符串 reject，静默；二次失败为 Error，展示
      if (e2 instanceof Error) {
        ElMessage.error(e2.message)
      }
    }
  }
}
// 彻底删除回收站条目
async function purge(item: RecycleWorkDTO) {
  try {
    await ElMessageBox.confirm('彻底删除后不可恢复，是否继续？', '彻底删除', {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning'
    })
    await recycleBinApi.recycleBinPurge(item.id)
    ElMessage.success('已彻底删除')
    await recycleBinSearchTable.value.doSearch()
  } catch (e) {
    // 确认框取消为字符串 reject，静默；接口失败为 Error，展示
    if (e instanceof Error) {
      ElMessage.error(e.message)
    }
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
            <div class="recycle-bin-tag-select">
              <auto-load-tag-select
                ref="searchConditionBar"
                v-model:selected-data="selectedTagList"
                v-model:custom-data="customTagList"
                v-model:input="autoLoadInput"
                :load="querySearchItemPage"
                :page-size="40"
                :color-resolver="setSearchTagColor"
                tags-gap="10px"
                max-height="300px"
                min-height="33px"
              >
                <template #left>
                  <el-checkbox-group
                    v-model="searchConditionType"
                    class="recycle-bin-tag-type-checkbox-group"
                    @change="handleConditionTypeChange"
                  >
                    <el-checkbox :value="SearchType.LocalTag">本地标签</el-checkbox>
                    <el-checkbox :value="SearchType.SiteTag">站点标签</el-checkbox>
                    <el-checkbox :value="SearchType.LocalAuthor">本地作者</el-checkbox>
                    <el-checkbox :value="SearchType.SiteAuthor">站点作者</el-checkbox>
                  </el-checkbox-group>
                </template>
              </auto-load-tag-select>
            </div>
          </template>
          <!-- 次要筛选收进搜索按钮「更多选项」折叠面板，工具栏只保留标签条 -->
          <template #toolbarDropdown>
            <div class="recycle-bin-advanced-filter">
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
              <el-date-picker
                v-model="deleteTimeRange"
                class="recycle-bin-filter-range"
                type="datetimerange"
                start-placeholder="删除时间起"
                end-placeholder="删除时间止"
              />
              <el-date-picker
                v-model="workCreateTimeRange"
                class="recycle-bin-filter-range"
                type="datetimerange"
                start-placeholder="创建时间起"
                end-placeholder="创建时间止"
              />
              <el-date-picker
                v-model="uploadTimeRange"
                class="recycle-bin-filter-range"
                type="datetimerange"
                start-placeholder="上传时间起"
                end-placeholder="上传时间止"
              />
            </div>
          </template>
          <!-- 破坏性按钮需保真 danger+tone-fail 红色形态，内置 operationButton 下拉项无 danger 样式，故走自定义操作列 -->
          <template #customOperations="{ row }">
            <el-button size="small" type="primary" @click="restore(row as RecycleWorkDTO)">复原</el-button>
            <el-button size="small" type="danger" class="tone-fail" @click="purge(row as RecycleWorkDTO)">彻底删除</el-button>
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

/* 占满工具栏内容列整行；组件内部 wrapper 经 :deep 撑满包裹 div，
   气泡宽度（el-popover :width 由 resizeObserver 实测 wrapper）随之满宽 */
.recycle-bin-tag-select {
  width: 100%;
}
.recycle-bin-tag-select :deep(.auto-load-tag-select-main) {
  width: 100%;
}

.recycle-bin-tag-type-checkbox-group {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

/* 「更多选项」折叠面板内的次要筛选区：自由折行，超出走面板滚动条 */
.recycle-bin-advanced-filter {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  padding: 8px;
}

.recycle-bin-filter-select {
  width: 200px;
  flex-shrink: 0;
}

.recycle-bin-filter-range {
  width: 350px;
  flex-shrink: 0;
}
</style>
