<script setup lang="ts" generic="Query extends object">
import { SelectItem } from "@bindings/github.com/library-squirrel/wails/pkg/model/dto"
import { nextTick, Ref, ref, UnwrapRef, watch } from 'vue'
import { arrayNotEmpty, isNullish, notNullish } from '@renderer/utils/CommonUtil.ts'
import SegmentedTag from '@renderer/components/common/SegmentedTag.vue'
import IPage from '@renderer/model/util/IPage.ts'
import Page from '@renderer/model/util/Page.ts'
import lodash from 'lodash'
import SegmentedTagItem from '@renderer/model/util/SegmentedTagItem.ts'

// props
const props = withDefaults(
  defineProps<{
    load?: (page: IPage<SegmentedTagItem, Query>) => Promise<IPage<SegmentedTagItem, Query>>
    tagCloseable?: boolean
    tagsGap?: string
    maxHeight?: string
  }>(),
  {
    tagCloseable: false,
    tagsGap: '5px',
    maxHeight: 'none'
  }
)

// model
// 分页参数
const page = defineModel<IPage<SegmentedTagItem, Query>>('page', { default: new Page<SegmentedTagItem, Query>() })
// 数据列表
const data = defineModel<SegmentedTagItem[]>('data', { default: [] })

// 事件
const emit = defineEmits(['tagClicked', 'tagMainLabelClicked', 'tagSubLabelClicked', 'tagClose'])

// 变量
// el-scrollbar组件的实例
const scrollbar = ref()
// el-row组件的实例
const container = ref()
// 加载按钮的实例
const loadMoreButton = ref()
// loading开关
const loading: Ref<UnwrapRef<boolean>> = ref(false)
// 显示加载按钮
const showLoadButton: Ref<UnwrapRef<boolean>> = ref(false)
// 是否有下一页
const hasNextPage: Ref<UnwrapRef<boolean>> = ref(false)

// 方法
// 处理DataScroll滚动事件
async function handleDataScroll() {
  if (isNullish(props.load)) {
    return
  }
  try {
    loading.value = true
    // 获得滚动条包裹的 ref 对象
    const scrollWrapper = scrollbar.value.wrapRef

    if (scrollWrapper) {
      const scrollHeight = scrollWrapper.scrollHeight
      const scrollTop = scrollWrapper.scrollTop
      const height = scrollWrapper.clientHeight

      // 判断是否滚动到底部
      if (scrollTop + height + 1 >= scrollHeight) {
        await nextPage(false)
      }
    }
  } finally {
    loading.value = false
  }
}
// 处理DataScroll滚动事件
async function nextPage(newSearch: boolean) {
  if (notNullish(props.load)) {
    // 新查询重置查询条件
    if (newSearch) {
      page.value.pageNumber = 1
      data.value.length = 0
    } else {
      page.value.pageNumber++
    }

    nextTick().then(async () => {
      //查询
      const tempPage = lodash.cloneDeep(page.value)
      tempPage.data = []
      const nextPage = await wrappedLoad(tempPage)

      if (notNullish(nextPage)) {
        // 新数据加入到分页数据中
        page.value.pageCount = nextPage.pageCount
        page.value.dataCount = nextPage.dataCount
        if (nextPage.pageNumber <= nextPage.pageCount) {
          hasNextPage.value = nextPage.pageNumber !== nextPage.pageCount
          if (arrayNotEmpty(nextPage.data)) {
            data.value.push(...nextPage.data)
          } else {
            refreshLoadButton()
          }
        } else {
          // 如果当前页超过总页数，当前页设为最大页数
          page.value.pageNumber = nextPage.pageCount <= 0 ? 1 : nextPage.pageCount
          hasNextPage.value = false
          refreshLoadButton()
        }
      }
    })
  }
}
async function newSearch() {
  return nextPage(true)
}
// 处理tag被点击事件
function handleTagClicked(tag: SelectItem) {
  emit('tagClicked', tag)
}
function handleTagMainLabelClicked(tag: SelectItem) {
  emit('tagMainLabelClicked', tag)
}
function handleTagSubLabelClicked(tag: SelectItem, index: number) {
  emit('tagSubLabelClicked', tag, index)
}
function handleTagClose(tag: SelectItem) {
  emit('tagClose', tag)
}
// 刷新加载按钮状态
function refreshLoadButton() {
  let notFull: boolean
  if (notNullish(scrollbar.value) && notNullish(container.value)) {
    const scrollHeight = scrollbar.value.wrapRef.clientHeight
    const containerHeight = container.value.offsetHeight
    const loadMoreButtonHeight = loadMoreButton.value.$el.clientHeight
    notFull = containerHeight <= scrollHeight + loadMoreButtonHeight
  } else {
    notFull = true
  }
  showLoadButton.value = hasNextPage.value && notFull
}
// 包装过的load
function wrappedLoad(page: IPage<SegmentedTagItem, Query>): Promise<IPage<SegmentedTagItem, Query> | undefined> | undefined {
  if (notNullish(props.load)) {
    return props
      .load(page)
      .then((result) => result)
      .catch((error) => {
        console.log(error)
        return undefined
      })
  } else {
    return undefined
  }
}

// watch
// 监听data变化，刷新加载按钮状态
watch(data, () => nextTick(() => refreshLoadButton()), { deep: true })

// 暴露
defineExpose({ scrollbar, newSearch })
</script>

<template>
  <div class="tag-box-wrapper">
    <el-scrollbar ref="scrollbar" v-loading="loading" @scroll="handleDataScroll">
      <div ref="container" class="tag-box-item-container">
        <slot name="head" />
        <segmented-tag
          v-for="item in data"
          :key="item.value"
          :item="item"
          :closeable="tagCloseable"
          @clicked="handleTagClicked(item)"
          @main-label-clicked="handleTagMainLabelClicked(item)"
          @sub-label-clicked="(event) => handleTagSubLabelClicked(item, event)"
          @close="handleTagClose(item)"
        />
        <slot name="tail" />
      </div>
    </el-scrollbar>
    <el-check-tag
      ref="loadMoreButton"
      :class="{
        'tag-box-load-more': true,
        'tag-box-show-load-more': showLoadButton,
        'tag-box-hide-load-more': !showLoadButton
      }"
      @click="nextPage(false)"
    >
      加载更多...
    </el-check-tag>
  </div>
</template>

<style scoped>
.tag-box-wrapper {
  display: flex;
  flex-direction: column;
}
.tag-box-load-more {
  transition:
    height 0.3s ease,
    padding 0.3s ease;
  overflow: hidden;
}
.tag-box-show-load-more {
  padding-top: 6px;
  padding-bottom: 6px;
}
.tag-box-hide-load-more {
  height: 0;
  padding-top: 0;
  padding-bottom: 0;
}
.tag-box-item-container {
  align-items: center;
  display: flex;
  flex: 1;
  flex-wrap: wrap;
  gap: v-bind(tagsGap);
  min-width: 0;
  position: relative;
  margin: 3px;
}
:deep(.el-scrollbar__wrap) {
  max-height: v-bind(maxHeight);
}
</style>
