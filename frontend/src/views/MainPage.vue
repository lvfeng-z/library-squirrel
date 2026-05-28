<script setup lang="ts">
import { onBeforeUnmount, onMounted, Ref, ref } from 'vue'
import ApiUtil from '@renderer/utils/ApiUtil.js'
import {
  SelectItem,
  WorkFullDTO as BindingsWorkFullDTO,
  WorkSetWithCoverDTO
} from "@bindings/github.com//lvfeng-z/library-squirrel-plugin-sdk/dto"
import SegmentedTagItem from '@renderer/model/util/SegmentedTagItem.js'
import ApiResponse from '@renderer/model/util/ApiResponse.js'
import { arrayNotEmpty, isNullish, notNullish } from '@renderer/utils/CommonUtil.js'
import { setSearchTagColor } from '@renderer/utils/SearchTagColorUtil.js'
import CollapsePanel from '@renderer/components/common/CollapsePanel.vue'
import IPage from '@renderer/model/util/IPage.js'
import AutoLoadTagSelect from '@renderer/components/common/AutoLoadTagSelect.vue'
import { SearchCondition, SearchType } from '@renderer/model/util/SearchCondition.js'
import lodash from 'lodash'
import { CrudOperator } from '@renderer/constants/CrudOperator.js'
import WorkGridForMainPage from '@renderer/components/common/WorkGridForMainPage.vue'
import WorkSetGridForMainPage from '@renderer/components/common/WorkSetGridForMainPage.vue'
import SearchConditionQueryDTO from '@renderer/model/model/queryDTO/SearchConditionQueryDTO.js'
import WorkFullDTO from '@renderer/model/model/dto/WorkFullDTO.js'
import { isNotBlank } from '@renderer/utils/StringUtil.js'
import { searchQuerySearchConditionPage, searchQueryWorkPage, searchQueryWorkSetPage } from '@apis/http/wrappers/search'
import {newPage} from "@renderer/utils/Pager.js";
import {Page} from "@bindings/github.com/library-squirrel/backend/base/model";
import { Events } from '@wailsio/runtime'

// 接口
const apis = {
  searchQuerySearchConditionPage,
  searchQueryWorkPage,
  searchQueryWorkSetPage
}

// main-page-work-space的实例
const workSpace = ref()
// workGrid组件的实例
const workGridRef = ref()
// workSetGrid组件的实例
const workSetGridRef = ref()
// 搜索条件工具栏组件的实例
const searchConditionBar = ref()

const selectedTagList: Ref<SegmentedTagItem[]> = ref([]) // 主搜索栏选中列表
const customTagList: Ref<SegmentedTagItem[]> = ref([]) // 主搜索栏自定义标签列表
const autoLoadInput: Ref<string | undefined> = ref()
const workList: Ref<WorkFullDTO[]> = ref([]) // 需展示的作品列表
// 当前作品的索引
const currentWorkIndex = ref(0)
// 查询参数类型
const searchConditionType: Ref<SearchType[]> = ref([])
// 作品分页
const workPage: Ref<Page<WorkFullDTO>> = ref(new Page<WorkFullDTO>())
// 搜索栏折叠面板开关
const searchBarPanelState: Ref<boolean> = ref(false)
// 加载更多按钮开关
const loadMore: Ref<boolean> = ref(false)
// 作品集视图加载更多按钮开关
const loadMoreWorkSet: Ref<boolean> = ref(false)
// 监听workGrid和workSetGrid组件的高度变化
const resizeObserver = new ResizeObserver((entries) => {
  const entry = entries[0]
  // 判断是作品视图还是作品集视图
  if (workSetView.value) {
    loadMoreWorkSet.value =
      entry.contentRect.height < workSpace.value.clientHeight && workSetPage.value.pageNumber < workSetPage.value.pageCount
  } else {
    loadMore.value = entry.contentRect.height < workSpace.value.clientHeight && workPage.value.pageNumber < workPage.value.pageCount
  }
})
// 作品集视图开关
const workSetView: Ref<boolean> = ref(false)
// 需展示的作品集列表
const workSetList: Ref<WorkSetWithCoverDTO[]> = ref([])
// 当前作品集的索引
const currentWorkSetIndex = ref(0)
// 作品集分页
const workSetPage: Ref<Page<WorkSetWithCoverDTO>> = ref(new Page<WorkSetWithCoverDTO>())

// onMounted
onMounted(() => {
  resizeObserver.observe(workGridRef.value.$el)
  resizeObserver.observe(workSetGridRef.value.$el)
})

// onBeforeUnmount
onBeforeUnmount(() => {
  resizeObserver.disconnect()
})

// 方法
// 查询标签选择列表
async function querySearchItemPage(page: IPage<any>, input?: string): Promise<IPage<any>> {
  const query = new SearchConditionQueryDTO()
  query.nonFieldKeyword = input
  query.types = lodash.cloneDeep(searchConditionType.value)
  let response: ApiResponse
  try {
    response = await apis.searchQuerySearchConditionPage({ pageNumber: page.pageNumber, pageSize: page.pageSize }, query)
  } catch (e) {
    console.log(e)
    return page
  }
  if (ApiUtil.check(response)) {
    const newPage = ApiUtil.data<Page<SelectItem>>(response)
    if (isNullish(newPage)) {
      ApiUtil.msg(response)
      throw new Error(response.msg)
    }
    return newPage
  } else {
    ApiUtil.msg(response)
    throw new Error(response.msg)
  }
}

// 请求作品接口
async function searchWork(page: Page<WorkFullDTO>): Promise<Page<WorkFullDTO>> {
  // 处理搜索框的标签
  const conditions: SearchCondition[] = selectedTagList.value
    .map((searchCondition) => {
      let operator: CrudOperator | undefined = undefined
      if (notNullish(searchCondition.disabled) && searchCondition.disabled) {
        operator = CrudOperator.NOT_EQUAL
      }
      if (notNullish(searchCondition.extraData)) {
        const extraData = searchCondition.extraData as { type: SearchType; id: number }
        return new SearchCondition({ type: extraData.type, value: extraData.id, operator: operator })
      } else {
        return undefined
      }
    })
    .filter(notNullish)
  if (arrayNotEmpty(customTagList.value)) {
    customTagList.value.forEach((tag: SegmentedTagItem) =>
      conditions.push(new SearchCondition({ type: SearchType.WORKS_SITE_NAME, value: tag.value, operator: CrudOperator.LIKE }))
    )
  }
  // 处理搜索框输入的文本
  if (isNotBlank(autoLoadInput.value)) {
    const workName = autoLoadInput.value
    conditions.push(new SearchCondition({ type: SearchType.WORKS_SITE_NAME, value: workName, operator: CrudOperator.LIKE }))
    conditions.push(new SearchCondition({ type: SearchType.WORKS_NICKNAME, value: workName, operator: CrudOperator.LIKE }))
  }

  const response = await apis.searchQueryWorkPage(newPage<BindingsWorkFullDTO>({ pageNumber: page.pageNumber, pageSize: 16 }), conditions)
  if (ApiUtil.check(response)) {
    return response.data
  } else {
    throw new Error(response.msg)
  }
}

// 请求作品集接口
async function searchWorkSet(page: Page<WorkSetWithCoverDTO>): Promise<Page<WorkSetWithCoverDTO>> {
  // 处理搜索框的标签（与 searchWork 相同的条件构建逻辑）
  const conditions: SearchCondition[] = selectedTagList.value
    .map((searchCondition) => {
      let operator: CrudOperator | undefined = undefined
      if (notNullish(searchCondition.disabled) && searchCondition.disabled) {
        operator = CrudOperator.NOT_EQUAL
      }
      if (notNullish(searchCondition.extraData)) {
        const extraData = searchCondition.extraData as { type: SearchType; id: number }
        return new SearchCondition({ type: extraData.type, value: extraData.id, operator: operator })
      } else {
        return undefined
      }
    })
    .filter(notNullish)
  if (arrayNotEmpty(customTagList.value)) {
    customTagList.value.forEach((tag: SegmentedTagItem) =>
      conditions.push(new SearchCondition({ type: SearchType.WORKS_SITE_NAME, value: tag.value, operator: CrudOperator.LIKE }))
    )
  }
  if (isNotBlank(autoLoadInput.value)) {
    const workName = autoLoadInput.value
    conditions.push(new SearchCondition({ type: SearchType.WORKS_SITE_NAME, value: workName, operator: CrudOperator.LIKE }))
    conditions.push(new SearchCondition({ type: SearchType.WORKS_NICKNAME, value: workName, operator: CrudOperator.LIKE }))
  }

  try {
    const result = await apis.searchQueryWorkSetPage(page, conditions)
    return result.data
  } catch (e) {
    console.log(e)
    return page
  }
}

// 加载下一页作品
async function queryWorkPage(next: boolean) {
  // 新查询重置查询条件
  if (!next) {
    workPage.value = newPage<WorkFullDTO>()
    workPage.value.pageSize = 12
    workList.value.length = 0
  }
  //查询
  const tempPage = lodash.cloneDeep(workPage.value)
  tempPage.data = undefined
  const nextPage = await searchWork(tempPage)

  // 没有新数据时，不再增加页码
  if (arrayNotEmpty(nextPage?.data)) {
    workPage.value.pageNumber++
    workPage.value.pageCount = nextPage.pageCount
    workPage.value.dataCount = nextPage.dataCount
    workList.value.push(...nextPage.data)
  }
}

// 加载下一页作品集
async function queryWorkSetPage(next: boolean) {
  // 新查询重置查询条件
  if (!next) {
    workSetPage.value = new Page<WorkSetWithCoverDTO>()
    workSetPage.value.pageSize = 12
    workSetList.value.length = 0
  }
  // 查询
  const tempPage = lodash.cloneDeep(workSetPage.value)
  tempPage.data = undefined
  const nextPage = await searchWorkSet(tempPage)

  // 没有新数据时，不再增加页码
  if (arrayNotEmpty(nextPage?.data)) {
    workSetPage.value.pageNumber++
    workSetPage.value.pageCount = nextPage.pageCount
    workSetPage.value.dataCount = nextPage.dataCount
    workSetList.value.push(...nextPage.data)
  }
}

// 重新查询搜索条件
async function querySearchCondition() {
  return searchConditionBar.value.newSearch()
}

// test
function handleTest() {
  Events.Emit('plugin:local-import:classify:request', { level: 0, dirName: 'test-author' })
}
</script>

<template>
  <div class="main-page">
    <div class="main-page-topbar z-layer-3">
      <el-radio-group v-model="workSetView" class="topbar-items">
        <el-radio-button label="作品" :value="false" />
        <el-radio-button label="作品集" :value="true" />
      </el-radio-group>
      <div class="main-page-searchbar topbar-items">
        <auto-load-tag-select
          ref="searchConditionBar"
          v-model:selected-data="selectedTagList"
          v-model:custom-data="customTagList"
          v-model:input="autoLoadInput"
          class="main-page-auto-load-tag-select"
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
        <collapse-panel v-model:state="searchBarPanelState" class="z-layer-3" border-radios="10px">
          <div style="padding: 5px; background-color: var(--el-fill-color-blank)">
            <!--TODO在这里实现一个更灵活的组合查询条件的组件，比如拖拽组成AND或OR组合-->
            <el-button @click="handleTest"> test </el-button>
          </div>
        </collapse-panel>
      </div>
      <el-button v-if="!workSetView" type="danger" class="topbar-items" @click="queryWorkPage(false)">搜索</el-button>
      <el-button v-if="workSetView" type="danger" class="topbar-items" @click="queryWorkSetPage(false)">搜索</el-button>
    </div>
    <div ref="workSpace" class="main-page-work-space">
      <div class="view-wrapper">
        <!-- 作品视图 -->
        <div class="view-container" :class="{ 'view-slide-left': workSetView }">
          <el-scrollbar v-el-scrollbar-bottomed="() => queryWorkPage(true)">
            <work-grid-for-main-page
              ref="workGridRef"
              v-model:current-work-index="currentWorkIndex"
              class="main-page-work-grid"
              :work-list="workList"
            />
          </el-scrollbar>
          <span
            ref="loadMoreButton"
            :class="{
              'work-grid-load-more': true,
              'work-grid-show-load-more': loadMore,
              'work-grid-hide-load-more': !loadMore
            }"
            @click="queryWorkPage(true)"
          >
            加载更多...
          </span>
        </div>
        <!-- 作品集视图 -->
        <div class="view-container" :class="{ 'view-slide-right': !workSetView }">
          <el-scrollbar v-el-scrollbar-bottomed="() => queryWorkSetPage(true)">
            <work-set-grid-for-main-page
              ref="workSetGridRef"
              v-model:current-work-set-index="currentWorkSetIndex"
              class="main-page-work-grid"
              :work-set-list="workSetList"
            />
          </el-scrollbar>
          <span
            :class="{
              'work-grid-load-more': true,
              'work-grid-show-load-more': loadMoreWorkSet,
              'work-grid-hide-load-more': !loadMoreWorkSet
            }"
            @click="queryWorkSetPage(true)"
          >
            加载更多...
          </span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.main-page {
  display: flex;
  flex-direction: column;
  height: calc(100% - 3px);
  width: calc(100% - 3px);
  margin-left: 3px;
  margin-top: 3px;
}

.main-page-topbar {
  display: flex;
  height: 33px;
  width: 100%;
}

.main-page-topbar > :deep(.topbar-items) {
  flex-wrap: nowrap;
  margin: 0 5px 0 0;
}

.main-page-searchbar {
  flex-grow: 1;
}

.main-page-work-space {
  position: relative;
  display: flex;
  flex-direction: column;
  height: calc(100% - 33px);
  margin-right: 8px;
}

.main-page-work-grid {
  margin-right: 19px;
}

.work-grid-load-more {
  position: absolute;
  bottom: 0;
  width: 100%;
  transition:
    height 0.3s ease,
    padding 0.3s ease,
    background-color 0.3s ease;
  overflow: hidden;
  color: var(--el-color-info);
  font-weight: bold;
  border-radius: 5px;
  background-color: var(--el-color-info-light-9);
  text-align: center;
  cursor: pointer;
}

.work-grid-load-more:hover {
  background-color: var(--el-color-info-light-7);
}

.work-grid-show-load-more {
  height: 26px;
}

.work-grid-hide-load-more {
  height: 0;
  padding-top: 0;
  padding-bottom: 0;
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

/* 视图容器 - 使用相对定位作为参考 */
.view-wrapper {
  position: relative;
  width: 100%;
  height: 100%;
  overflow: hidden;
}

/* 视图容器 - 使用绝对定位让两个视图重叠 */
.view-container {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  overflow: hidden;
  transition: transform 0.3s ease;
}

/* 作品视图向左移动到左侧不可视区域 */
.view-container.view-slide-left {
  transform: translateX(-100%);
}

/* 作品集视图向右移动到右侧不可视区域 */
.view-container.view-slide-right {
  transform: translateX(100%);
}
</style>
