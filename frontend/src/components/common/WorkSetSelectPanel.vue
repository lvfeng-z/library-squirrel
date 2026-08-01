<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted } from 'vue'
import CardGrid from '@renderer/components/common/CardGrid.vue'
import WorkSetCard from '@renderer/components/common/WorkSetCard.vue'
import { WorkSetWithCoverDTO } from '@bindings/github.com//lvfeng-z/library-squirrel-sdk/dto'
import Page from '@renderer/model/util/Page.ts'
import IPage from '@renderer/model/util/IPage.ts'
import { arrayNotEmpty, isNullish, notNullish } from '@renderer/utils/CommonUtil.ts'
import { getWorkSetCardDimension } from '@renderer/utils/ImageDimension.ts'

// props
const props = withDefaults(
  defineProps<{
    /** 作品集查询函数（page 含分页信息，返回填充后的 page） */
    fetchWorkSetPage: (page: Page<WorkSetWithCoverDTO>, keyword?: string) => Promise<Page<WorkSetWithCoverDTO>>
    /** 是否可选中 */
    checkable?: boolean
    /** 选中的作品集id列表 */
    checkedWorkSetIds?: number[]
    /** 分页大小 */
    pageSize?: number
  }>(),
  {
    checkable: false,
    checkedWorkSetIds: () => [],
    pageSize: 16
  }
)

// events
const emits = defineEmits<{
  /** 选中作品集变化事件（checkable 为 true 时） */
  checkedChange: [workSetIds: number[]]
}>()

// 变量
const keyword = ref<string | undefined>(undefined)
const workSetList = ref<WorkSetWithCoverDTO[]>([])
const workSetPage = ref<Page<WorkSetWithCoverDTO>>(new Page<WorkSetWithCoverDTO>())
const loadMoreVisible = ref(false)
const loading = ref(false)
const workSpace = ref()

// 监听作品集网格容器高度，决定是否展示"加载更多"
const resizeObserver = new ResizeObserver((entries) => {
  if (workSpace.value && entries[0]) {
    const gridElement = entries[0].target
    loadMoreVisible.value =
      gridElement.clientHeight < workSpace.value.clientHeight && workSetPage.value.pageNumber < workSetPage.value.pageCount
  }
})

onMounted(() => {
  if (workSpace.value) {
    resizeObserver.observe(workSpace.value)
  }
})

onUnmounted(() => {
  resizeObserver.disconnect()
})

// 首次查询或重新查询
async function queryWorkSets(reset: boolean = true): Promise<void> {
  if (loading.value) return
  loading.value = true
  try {
    if (reset) {
      workSetPage.value = new Page<WorkSetWithCoverDTO>()
      workSetList.value = []
    }
    const tempPage: IPage<WorkSetWithCoverDTO> = {
      pageNumber: workSetPage.value.pageNumber,
      pageSize: props.pageSize,
      pageCount: workSetPage.value.pageCount,
      dataCount: workSetPage.value.dataCount,
      currentCount: workSetPage.value.currentCount,
      data: []
    }
    const nextPage = await props.fetchWorkSetPage(tempPage as Page<WorkSetWithCoverDTO>, keyword.value)
    if (arrayNotEmpty(nextPage.data)) {
      workSetPage.value.pageNumber++
      workSetPage.value.pageCount = nextPage.pageCount
      workSetPage.value.dataCount = nextPage.dataCount
      workSetList.value.push(...nextPage.data.filter(notNullish))
    }
  } finally {
    loading.value = false
  }
}

// 加载更多
async function loadMore(): Promise<void> {
  if (loading.value) return
  await queryWorkSets(false)
}

// 处理选中变化
function handleCheckedChange(ids: number[]): void {
  emits('checkedChange', ids)
}

// 暴露方法
defineExpose({
  queryWorkSets,
  clearKeyword(): void {
    keyword.value = undefined
  }
})
</script>

<template>
  <div class="work-set-select-panel-view">
    <!-- 顶部搜索工具栏 -->
    <div class="work-set-select-panel-toolbar">
      <el-input
        v-model="keyword"
        placeholder="按作品集名称搜索"
        clearable
        class="work-set-select-panel-keyword"
        @keyup.enter="queryWorkSets(true)"
        @clear="queryWorkSets(true)"
      />
      <el-button
        type="primary"
        :loading="loading"
        @click="queryWorkSets(true)"
      >
        搜索
      </el-button>
    </div>

    <!-- 作品集展示区域 -->
    <div
      ref="workSpace"
      class="work-set-select-panel-work-space"
    >
      <el-scrollbar v-el-scrollbar-bottomed="() => queryWorkSets(false)">
        <card-grid
          :items="workSetList"
          :checkable="isNullish(checkable) ? false : checkable"
          :checked-ids="checkedWorkSetIds"
          :get-id="(workSet: WorkSetWithCoverDTO) => workSet.workSet?.id"
          :get-dimension="getWorkSetCardDimension"
          class="work-set-select-panel-grid"
          @checked-change="handleCheckedChange"
        >
          <template #card="{ item, checked, onUpdateChecked }">
            <work-set-card
              :checked="checked"
              :work-set="item"
              :max-height="500"
              :max-width="500"
              :checkable="isNullish(checkable) ? false : checkable"
              @update:checked="onUpdateChecked"
            />
          </template>
        </card-grid>
      </el-scrollbar>
      <!-- 加载更多按钮 -->
      <div
        v-show="loadMoreVisible"
        :class="{
          'work-set-select-panel-load-more': true,
          'work-set-select-panel-load-more-visible': loadMoreVisible,
          'work-set-select-panel-load-more-hidden': !loadMoreVisible
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
.work-set-select-panel-view {
  display: flex;
  flex-direction: column;
  height: 100%;
  width: 100%;
}

.work-set-select-panel-toolbar {
  display: flex;
  flex-wrap: nowrap;
  gap: 8px;
  margin-bottom: 8px;
}

.work-set-select-panel-keyword {
  flex-grow: 1;
  min-width: 200px;
}

.work-set-select-panel-work-space {
  flex-grow: 1;
  position: relative;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.work-set-select-panel-grid {
  margin-right: 19px;
}

.work-set-select-panel-load-more {
  position: absolute;
  bottom: 0;
  width: 100%;
  height: 26px;
  padding: 5px 0;
  color: var(--app-color-info);
  font-weight: bold;
  border-radius: var(--app-radius);
  background-color: var(--app-color-info-light-9);
  text-align: center;
  cursor: pointer;
  transition:
    height 0.3s ease,
    padding 0.3s ease,
    background-color 0.3s ease;
  overflow: hidden;
}

.work-set-select-panel-load-more:hover {
  background-color: var(--app-color-info-light-7);
}

.work-set-select-panel-load-more-visible {
  height: 36px;
}

.work-set-select-panel-load-more-hidden {
  height: 0;
  padding-top: 0;
  padding-bottom: 0;
}
</style>
