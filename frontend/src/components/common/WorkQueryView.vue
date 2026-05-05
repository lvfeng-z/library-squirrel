<script setup lang="ts">
import AutoLoadTagSelect from '@renderer/components/common/AutoLoadTagSelect.vue'
import WorkGrid from '@renderer/components/common/WorkGrid.vue'
import SegmentedTagItem from '@renderer/model/util/SegmentedTagItem.ts'
import { SelectItem } from "@bindings/github.com/library-squirrel/wails/backend/base/model/dto"
import Page from '@renderer/model/util/Page.ts'
import IPage from '@renderer/model/util/IPage.ts'
import { ref, UnwrapRef, Ref, watch, onMounted, onUnmounted } from 'vue'
import lodash from 'lodash'
import { notNullish, arrayNotEmpty } from '@renderer/utils/CommonUtil.ts'
import { SearchCondition, SearchType } from '@renderer/model/util/SearchCondition.ts'
import { CrudOperator } from '@renderer/constants/CrudOperator.ts'
import WorkCardItem from '@renderer/model/model/dto/WorkCardItem.ts'

// props
const props = withDefaults(
  defineProps<{
    /** 查询标签选择列表的加载函数 */
    loadSearchItemPage: (page: IPage<SelectItem>, input?: string) => Promise<IPage<SelectItem>>
    /** 作品查询函数 */
    fetchWorkPage: (page: Page<WorkCardItem>, conditions: SearchCondition[]) => Promise<Page<WorkCardItem>>
    /** 可选的搜索条件类型列表 */
    searchTypes?: SearchType[]
    /** 标签颜色解析器 */
    colorResolver?: (segmentedTagItem: SegmentedTagItem) => void
    /** 作品分页大小 */
    workPageSize?: number
    /** 标签选择器分页大小 */
    tagSelectPageSize?: number
    /** 标签选择器最大高度 */
    tagSelectMaxHeight?: string
    /** 标签选择器最小高度 */
    tagSelectMinHeight?: string
    /** 标签选择器标签间距 */
    tagSelectTagsGap?: string
    /** 是否可选中 */
    checkable?: boolean
    /** 选中的作品id列表 */
    checkedWorkIds?: number[]
    /** 是否在选中标签变化时自动搜索 */
    autoSearchOnTagChange?: boolean
    /** 是否在输入变化时自动搜索（防抖500ms） */
    autoSearchOnInputChange?: boolean
  }>(),
  {
    searchTypes: () => [SearchType.LOCAL_TAG, SearchType.SITE_TAG, SearchType.LOCAL_AUTHOR, SearchType.SITE_AUTHOR],
    workPageSize: 16,
    tagSelectPageSize: 40,
    tagSelectMaxHeight: '300px',
    tagSelectMinHeight: '33px',
    tagSelectTagsGap: '10px',
    checkable: false,
    checkedWorkIds: () => [],
    autoSearchOnTagChange: true,
    autoSearchOnInputChange: false
  }
)

// events
const emits = defineEmits<{
  /** 作品点击事件 */
  workClicked: [work: WorkCardItem]
  /** 选中作品变化事件（当checkable为true时） */
  checkedChange: [workIds: number[]]
}>()

// model
const selectedTagList: Ref<UnwrapRef<SegmentedTagItem[]>> = ref([])
const customTagList: Ref<UnwrapRef<SegmentedTagItem[]>> = ref([])
const autoLoadInput: Ref<UnwrapRef<string | undefined>> = ref(undefined)
// 查询参数类型
const searchConditionType: Ref<SearchType[]> = defineModel<SearchType[]>('searchConditionType', { required: false, default: [] })

// 变量
const workList: Ref<UnwrapRef<WorkCardItem[]>> = ref([])
const workPage: Ref<UnwrapRef<Page<WorkCardItem>>> = ref(new Page<WorkCardItem>())
const loadMoreVisible: Ref<UnwrapRef<boolean>> = ref(false)
const loading: Ref<UnwrapRef<boolean>> = ref(false)
const searchConditionBar = ref()
const workSpace = ref()

// 监听 workGrid 容器的高度变化
const resizeObserver = new ResizeObserver((entries) => {
  if (workSpace.value && entries[0]) {
    const workGridElement = entries[0].target
    loadMoreVisible.value =
      workGridElement.clientHeight < workSpace.value.clientHeight && workPage.value.pageNumber < workPage.value.pageCount
  }
})

// 在挂载时开始观察
onMounted(() => {
  if (workSpace.value) {
    resizeObserver.observe(workSpace.value)
  }
})

// 在卸载时停止观察
onUnmounted(() => {
  resizeObserver.disconnect()
})

// 监听选中标签变化，自动搜索
watch(
  selectedTagList,
  () => {
    if (props.autoSearchOnTagChange) {
      search()
    }
  },
  { deep: true }
)

// 监听自定义标签变化，自动搜索
watch(
  customTagList,
  () => {
    if (props.autoSearchOnTagChange) {
      search()
    }
  },
  { deep: true }
)

// 监听输入变化，自动搜索（防抖）
let inputChangeTimeout: ReturnType<typeof setTimeout> | null = null
watch(autoLoadInput, () => {
  if (props.autoSearchOnInputChange) {
    if (inputChangeTimeout) {
      clearTimeout(inputChangeTimeout)
    }
    inputChangeTimeout = setTimeout(() => {
      search()
    }, 500)
  }
})

// 方法
/** 查询标签选择列表 */
async function querySearchItemPage(page: IPage<SelectItem>, input?: string): Promise<IPage<SelectItem>> {
  return props.loadSearchItemPage(page, input)
}

/** 构建查询条件 */
async function buildSearchConditions(): Promise<SearchCondition[]> {
  const conditions: SearchCondition[] = []

  // 处理选中的标签
  selectedTagList.value.forEach((searchCondition) => {
    let operator: CrudOperator | undefined = undefined
    if (notNullish(searchCondition.disabled) && searchCondition.disabled) {
      operator = CrudOperator.NOT_EQUAL
    }
    if (notNullish(searchCondition.extraData)) {
      const extraData = searchCondition.extraData as { type: SearchType; id: number }
      conditions.push(new SearchCondition({ type: extraData.type, value: extraData.id, operator: operator }))
    }
  })

  // 处理自定义标签
  if (arrayNotEmpty(customTagList.value)) {
    customTagList.value.forEach((tag) => {
      conditions.push(new SearchCondition({ type: SearchType.WORKS_SITE_NAME, value: tag.value, operator: CrudOperator.LIKE }))
    })
  }

  // 处理搜索框输入的文本
  if (notNullish(autoLoadInput.value) && autoLoadInput.value.trim()) {
    const workName = autoLoadInput.value.trim()
    conditions.push(new SearchCondition({ type: SearchType.WORKS_SITE_NAME, value: workName, operator: CrudOperator.LIKE }))
    conditions.push(new SearchCondition({ type: SearchType.WORKS_NICKNAME, value: workName, operator: CrudOperator.LIKE }))
  }

  return conditions
}

/** 执行作品查询 */
async function doFetchWorkPage(page: Page<WorkCardItem>): Promise<Page<WorkCardItem>> {
  const conditions = await buildSearchConditions()
  page.pageSize = props.workPageSize
  return props.fetchWorkPage(page, conditions)
}

/** 执行查询（首次查询或重新查询） */
async function queryWork(reset: boolean = true): Promise<void> {
  if (loading.value) return
  loading.value = true

  try {
    // 重置查询条件
    if (reset) {
      workPage.value = new Page<WorkCardItem>()
      workList.value = []
    }

    // 构建查询参数
    const tempPage = lodash.cloneDeep(workPage.value)
    tempPage.data = undefined
    const nextPage = await doFetchWorkPage(tempPage)

    // 没有新数据时，不再增加页码
    if (arrayNotEmpty(nextPage.data)) {
      workPage.value.pageNumber++
      workPage.value.pageCount = nextPage.pageCount
      workPage.value.dataCount = nextPage.dataCount
      workList.value.push(...nextPage.data)
    }
  } finally {
    loading.value = false
  }
}

/** 加载更多 */
async function loadMore(): Promise<void> {
  if (loading.value) return
  await queryWork(false)
}

/** 重新查询搜索条件 */
function refreshSearchCondition(): Promise<void> {
  return searchConditionBar.value?.newSearch()
}

/** 处理作品点击 */
function handleWorkClicked(work: WorkCardItem): void {
  emits('workClicked', work)
}

/** 处理选中变化 */
function handleCheckedChange(workIds: number[]): void {
  emits('checkedChange', workIds)
}

/** 清空所有查询条件 */
function clearConditions(): void {
  selectedTagList.value = []
  customTagList.value = []
  autoLoadInput.value = undefined
}

/** 主动触发搜索 */
function search(): void {
  queryWork(true)
}

/** 处理输入变化 */
function handleInputChange(): void {
  if (props.autoSearchOnInputChange) {
    if (inputChangeTimeout) {
      clearTimeout(inputChangeTimeout)
    }
    inputChangeTimeout = setTimeout(() => {
      search()
    }, 500)
  }
}
/** 重新查询搜索条件 */
async function querySearchCondition() {
  return searchConditionBar.value.newSearch()
}

// 暴露方法
defineExpose({
  queryWork: search,
  loadMore,
  clearConditions,
  refreshSearchCondition
})
</script>

<template>
  <div class="work-query-view">
    <!-- 顶部查询工具栏 -->
    <div class="work-query-view-toolbar">
      <div class="work-query-view-auto-load-tag-select">
        <auto-load-tag-select
          ref="searchConditionBar"
          v-model:selected-data="selectedTagList"
          v-model:custom-data="customTagList"
          v-model:input="autoLoadInput"
          placement="bottom"
          :load="querySearchItemPage"
          :page-size="tagSelectPageSize"
          :color-resolver="colorResolver"
          :tags-gap="tagSelectTagsGap"
          :max-height="tagSelectMaxHeight"
          :min-height="tagSelectMinHeight"
          @input="handleInputChange"
        >
          <template #left>
            <el-checkbox-group
              v-model="searchConditionType"
              class="main-page-auto-load-tag-select-tag-type-checkbox-group"
              @change="querySearchCondition"
            >
              <el-checkbox :value="SearchType.LOCAL_TAG">
                <span
                  class="main-page-auto-load-tag-select-tag-type-checkbox main-page-auto-load-tag-select-tag-type-checkbox-local-tag"
                >
                  本地标签
                </span>
              </el-checkbox>
              <el-checkbox :value="SearchType.SITE_TAG">
                <span
                  class="main-page-auto-load-tag-select-tag-type-checkbox main-page-auto-load-tag-select-tag-type-checkbox-site-tag"
                >
                  站点标签
                </span>
              </el-checkbox>
              <el-checkbox :value="SearchType.LOCAL_AUTHOR">
                <span
                  class="main-page-auto-load-tag-select-tag-type-checkbox main-page-auto-load-tag-select-tag-type-checkbox-local-author"
                >
                  本地作者
                </span>
              </el-checkbox>
              <el-checkbox :value="SearchType.SITE_AUTHOR">
                <span
                  class="main-page-auto-load-tag-select-tag-type-checkbox main-page-auto-load-tag-select-tag-type-checkbox-site-author"
                >
                  站点作者
                </span>
              </el-checkbox>
            </el-checkbox-group>
          </template>
        </auto-load-tag-select>
      </div>
      <el-button type="primary" :loading="loading" @click="search">搜索</el-button>
    </div>

    <!-- 作品展示区域 -->
    <div ref="workSpace" class="work-query-view-work-space">
      <el-scrollbar>
        <work-grid
          :work-list="workList"
          :checkable="checkable"
          :checked-work-ids="checkedWorkIds"
          class="work-query-view-work-grid"
          @image-clicked="handleWorkClicked"
          @checked-change="handleCheckedChange"
        />
      </el-scrollbar>
      <!-- 加载更多按钮 -->
      <div
        v-show="loadMoreVisible"
        :class="{
          'work-query-view-load-more': true,
          'work-query-view-load-more-visible': loadMoreVisible,
          'work-query-view-load-more-hidden': !loadMoreVisible
        }"
        @click="loadMore"
      >
        <span v-if="loading">加载中...</span>
        <span v-else>加载更多...</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.work-query-view {
  display: flex;
  flex-direction: column;
  height: 100%;
  width: 100%;
}

.work-query-view-toolbar {
  display: flex;
  flex-wrap: nowrap;
  gap: 8px;
  margin-bottom: 8px;
}

.work-query-view-auto-load-tag-select {
  flex-grow: 1;
  min-width: 200px;
}

.main-page-auto-load-tag-select-tag-type-checkbox-group {
  display: flex;
  flex-direction: column;
}
.main-page-auto-load-tag-select-tag-type-checkbox {
  padding: 0 7px 0 6px;
  border-radius: 15px;
}
.main-page-auto-load-tag-select-tag-type-checkbox-local-tag {
  background-color: rgb(133.4, 206.2, 97.4, 30%);
  color: rgb(78.1, 141.8, 46.6, 75%);
}
.main-page-auto-load-tag-select-tag-type-checkbox-site-tag {
  background-color: rgb(64, 158, 255, 25%);
  color: rgb(64, 158, 255, 85%);
}
.main-page-auto-load-tag-select-tag-type-checkbox-local-author {
  background-color: rgb(245, 108, 108, 25%);
  color: rgb(245, 108, 108, 75%);
}
.main-page-auto-load-tag-select-tag-type-checkbox-site-author {
  background-color: rgb(164, 158, 255, 25%);
  color: rgb(164, 158, 255, 95%);
}

.work-query-view-work-space {
  flex-grow: 1;
  position: relative;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.work-query-view-work-grid {
  margin-right: 19px;
}

.work-query-view-load-more {
  position: absolute;
  bottom: 0;
  width: 100%;
  height: 26px;
  padding: 5px 0;
  color: var(--el-color-info);
  font-weight: bold;
  border-radius: 5px;
  background-color: var(--el-color-info-light-9);
  text-align: center;
  cursor: pointer;
  transition:
    height 0.3s ease,
    padding 0.3s ease,
    background-color 0.3s ease;
  overflow: hidden;
}

.work-query-view-load-more:hover {
  background-color: var(--el-color-info-light-7);
}

.work-query-view-load-more-visible {
  height: 36px;
}

.work-query-view-load-more-hidden {
  height: 0;
  padding-top: 0;
  padding-bottom: 0;
}
</style>
