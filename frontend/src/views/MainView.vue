<script setup lang="ts">
import {onBeforeUnmount, onMounted, Ref, ref, toRaw} from 'vue'
import ApiUtil from '@renderer/utils/ApiUtil.js'
import {
  SearchCondition,
  SearchConditionQuery, SearchType,
  SelectItem,
  WorkFullDTO, WorkSearchOperator,
  WorkSetWithCoverDTO
} from "@bindings/github.com/library-squirrel/backend/base/model/dto"
import SegmentedTagItem from '@renderer/model/util/SegmentedTagItem.js'
import ApiResponse from '@renderer/model/util/ApiResponse.js'
import {arrayNotEmpty, isNullish, notNullish} from '@renderer/utils/CommonUtil.js'
import {setSearchTagColor} from '@renderer/utils/SearchTagColorUtil.js'
import CollapsePanel from '@renderer/components/common/CollapsePanel.vue'
import IPage from '@renderer/model/util/IPage.js'
import AutoLoadTagSelect from '@renderer/components/common/AutoLoadTagSelect.vue'
import lodash from 'lodash'
import WorkGridForMainPage from '@renderer/components/common/WorkGridForMainPage.vue'
import WorkSetGridForMainPage from '@renderer/components/common/WorkSetGridForMainPage.vue'
import WorkSetCreateDialog from '@renderer/components/dialogs/WorkSetCreateDialog.vue'
import ExportProgressDialog from '@renderer/components/dialogs/ExportProgressDialog.vue'
import {isNotBlank} from '@renderer/utils/StringUtil.js'
import {searchQuerySearchConditionPage, searchQueryWorkPage, searchQueryWorkSetPage} from '@apis/http/wrappers/search'
import {settingsGetSettings, settingsSaveSettings} from '@renderer/apis/http/wrappers/settings'
import {newPage} from "@renderer/utils/Pager.js";
import {Page} from "@bindings/github.com/library-squirrel/backend/base/model";
import {Events} from '@wailsio/runtime'
import EmbedSlotRenderer from '@renderer/components/slot/EmbedSlotRenderer.vue'
import {useWorkSelectionStore} from '@renderer/store/UseWorkSelectionStore.ts'

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
const workPage: Ref<Page<WorkFullDTO>> = ref(newPage<WorkFullDTO>())
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
// 主页作品/作品集多选选择集（跨页保持；操作栏展示与导出数据源）
const workSelectionStore = useWorkSelectionStore()
// 主页多选模式开关（默认关；持久化到 settings.appearance.multiSelectEnabled，重启保持）
const multiSelectEnabled: Ref<boolean> = ref(false)

// onMounted
onMounted(() => {
  resizeObserver.observe(workGridRef.value.$el)
  resizeObserver.observe(workSetGridRef.value.$el)
  void loadMultiSelectSetting()
})

// onBeforeUnmount
onBeforeUnmount(() => {
  resizeObserver.disconnect()
})

// 方法
// 查询标签选择列表
async function querySearchItemPage(page: IPage<any>, input?: string): Promise<IPage<any>> {
  const query = new SearchConditionQuery()
  query.keyword = input ?? undefined
  query.types = lodash.cloneDeep(searchConditionType.value)
  let response: ApiResponse
  try {
    response = await apis.searchQuerySearchConditionPage(newPage<SelectItem>({ pageNumber: page.pageNumber, pageSize: page.pageSize }), query)
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

// namespace 由已选 tag 的 ns 段编辑写入 extraData.namespace（local=用户点 ns 段设搜索 ns、site=站点自带固定 ns）；author 不带
// 空串视作未设（undefined）：local 标签清空 ns 段后不应按 namespace='' 过滤（DB 空串落 NULL，无命中）
function resolveSearchNamespace(extraData: { type: SearchType; namespace?: string }): string | undefined {
  if (extraData.type === SearchType.LocalTag || extraData.type === SearchType.SiteTag) {
    return extraData.namespace || undefined
  }
  return undefined
}

// 请求作品接口
async function searchWork(page: Page<WorkFullDTO>): Promise<Page<WorkFullDTO>> {
  // 处理搜索框的标签
  const conditions: SearchCondition[] = selectedTagList.value
    .map((searchCondition) => {
      let operator: WorkSearchOperator | undefined = undefined
      if (notNullish(searchCondition.disabled) && searchCondition.disabled) {
        operator = WorkSearchOperator.NotEqual
      }
      if (notNullish(searchCondition.extraData)) {
        const extraData = searchCondition.extraData as { type: SearchType; id: number; namespace?: string }
        return new SearchCondition({ type: extraData.type, value: extraData.id, operator: operator, namespace: resolveSearchNamespace(extraData) })
      } else {
        return undefined
      }
    })
    .filter(notNullish)
  if (arrayNotEmpty(customTagList.value)) {
    customTagList.value.forEach((tag: SegmentedTagItem) =>
      conditions.push(new SearchCondition({ type: SearchType.WorksSiteName, value: tag.value, operator: WorkSearchOperator.Like }))
    )
  }
  // 处理搜索框输入的文本
  if (isNotBlank(autoLoadInput.value)) {
    const workName = autoLoadInput.value
    conditions.push(new SearchCondition({ type: SearchType.WorksSiteName, value: workName, operator: WorkSearchOperator.Like }))
    conditions.push(new SearchCondition({ type: SearchType.WorksNickname, value: workName, operator: WorkSearchOperator.Like }))
  }

  const response = await apis.searchQueryWorkPage(newPage<WorkFullDTO>({ pageNumber: page.pageNumber, pageSize: 16 }), conditions)
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
      let operator: WorkSearchOperator | undefined = undefined
      if (notNullish(searchCondition.disabled) && searchCondition.disabled) {
        operator = WorkSearchOperator.NotEqual
      }
      if (notNullish(searchCondition.extraData)) {
        const extraData = searchCondition.extraData as { type: SearchType; id: number; namespace?: string }
        return new SearchCondition({ type: extraData.type, value: extraData.id, operator: operator, namespace: resolveSearchNamespace(extraData) })
      } else {
        return undefined
      }
    })
    .filter(notNullish)
  if (arrayNotEmpty(customTagList.value)) {
    customTagList.value.forEach((tag: SegmentedTagItem) =>
      conditions.push(new SearchCondition({ type: SearchType.WorksSiteName, value: tag.value, operator: WorkSearchOperator.Like }))
    )
  }
  if (isNotBlank(autoLoadInput.value)) {
    const workName = autoLoadInput.value
    conditions.push(new SearchCondition({ type: SearchType.WorksSiteName, value: workName, operator: WorkSearchOperator.Like }))
    conditions.push(new SearchCondition({ type: SearchType.WorksNickname, value: workName, operator: WorkSearchOperator.Like }))
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
  const tempPage = toRaw(workPage.value)
  tempPage.data = []
  const nextPage = await searchWork(tempPage)

  // 没有新数据时，不再增加页码
  if (arrayNotEmpty(nextPage?.data)) {
    workPage.value.pageNumber++
    workPage.value.pageCount = nextPage.pageCount
    workPage.value.dataCount = nextPage.dataCount
    workList.value.push(...nextPage.data.filter(notNullish))
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
  const tempPage = toRaw(workSetPage.value)
  tempPage.data = []
  const nextPage = await searchWorkSet(tempPage)

  // 没有新数据时，不再增加页码
  if (arrayNotEmpty(nextPage?.data)) {
    workSetPage.value.pageNumber++
    workSetPage.value.pageCount = nextPage.pageCount
    workSetPage.value.dataCount = nextPage.dataCount
    workSetList.value.push(...nextPage.data.filter(notNullish))
  }
}

// 作品集已软删除（弹窗删除入口上抛）：刷新作品集列表，已删集从列表消失
function handleWorkSetDeleted() {
  queryWorkSetPage(false)
}

// 新建作品集弹窗开关
const workSetCreateDialogState = ref(false)

// 作品集已创建：刷新作品集列表
function handleWorkSetCreated() {
  queryWorkSetPage(false)
}

// 重新查询搜索条件
async function querySearchCondition() {
  return searchConditionBar.value.newSearch()
}

// 作品勾选变化 → 选择集 store 同步（携带当前已加载 id 集，保留不在可见集的已选项）
function handleWorkCheckedChange(workIds: number[]): void {
  const visibleWorkIds = workList.value.map((work) => work.work?.id).filter(notNullish)
  workSelectionStore.syncWorkIds(workIds, visibleWorkIds)
}

// 作品集勾选变化 → 选择集 store 同步（语义同作品）
function handleWorkSetCheckedChange(workSetIds: number[]): void {
  const visibleWorkSetIds = workSetList.value.map((workSet) => workSet.workSet?.id).filter(notNullish)
  workSelectionStore.syncWorkSetIds(workSetIds, visibleWorkSetIds)
}

// 导出弹窗开关（打开后先在弹窗内确认输出目录，再触发后端异步导出）
const exportDialogState = ref(false)

// 导出：把选择集 store 中的选中作品/作品集 id 列表传给后端收集打包（决策5），
// 弹窗展示进度条与 [取消]，完成后展示产物路径
function handleExport(): void {
  exportDialogState.value = true
}

// 清除选择：清空选择集 store，网格经 checkedIds 置空联动取消勾选
function handleClearSelection(): void {
  workSelectionStore.clear()
}

// 读取设置中的多选开关初始状态（设置缺失/读取失败回落默认关，不阻塞主页渲染）
async function loadMultiSelectSetting(): Promise<void> {
  try {
    const res = await settingsGetSettings()
    if (!res.success || !res.data) return
    const appearance = res.data.appearance as { multiSelectEnabled?: boolean } | undefined
    multiSelectEnabled.value = appearance?.multiSelectEnabled ?? false
  } catch (e) {
    console.warn('读取多选开关设置失败', e)
  }
}

// 多选开关切换：关闭时清空选择集（操作栏隐藏、勾选态联动取消）；状态持久化到设置
function handleMultiSelectChange(): void {
  if (!multiSelectEnabled.value) {
    workSelectionStore.clear()
  }
  void settingsSaveSettings([{path: 'appearance.multiSelectEnabled', value: multiSelectEnabled.value}])
}

// test
function handleTest() {
  Events.Emit('plugin:local-import:classify:request', { level: 0, dirName: 'test-author' })
}
</script>

<template>
  <div class="main-page">
    <div class="main-page-topbar z-layer-3">
      <el-radio-group
        v-model="workSetView"
        class="topbar-items"
      >
        <el-radio-button
          label="作品"
          :value="false"
        />
        <el-radio-button
          label="作品集"
          :value="true"
        />
      </el-radio-group>
      <div class="main-page-multiselect-toggle topbar-items">
        <span class="main-page-multiselect-toggle-label">多选</span>
        <el-switch
          v-model="multiSelectEnabled"
          @change="handleMultiSelectChange"
        />
      </div>
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
            <div class="main-page-namespace-filter" style="display: flex; flex-direction: column; gap: 8px;">
            <el-checkbox-group
              v-model="searchConditionType"
              class="main-page-auto-load-tag-select-tag-type-checkbox-group"
              @change="querySearchCondition"
            >
              <el-checkbox :value="SearchType.LocalTag">
                <span
                  class="main-page-auto-load-tag-select-tag-type-checkbox main-page-auto-load-tag-select-tag-type-checkbox-local-tag"
                >
                  本地标签
                </span>
              </el-checkbox>
              <el-checkbox :value="SearchType.SiteTag">
                <span
                  class="main-page-auto-load-tag-select-tag-type-checkbox main-page-auto-load-tag-select-tag-type-checkbox-site-tag"
                >
                  站点标签
                </span>
              </el-checkbox>
              <el-checkbox :value="SearchType.LocalAuthor">
                <span
                  class="main-page-auto-load-tag-select-tag-type-checkbox main-page-auto-load-tag-select-tag-type-checkbox-local-author"
                >
                  本地作者
                </span>
              </el-checkbox>
              <el-checkbox :value="SearchType.SiteAuthor">
                <span
                  class="main-page-auto-load-tag-select-tag-type-checkbox main-page-auto-load-tag-select-tag-type-checkbox-site-author"
                >
                  站点作者
                </span>
              </el-checkbox>
            </el-checkbox-group>
            </div>
          </template>
        </auto-load-tag-select>
        <collapse-panel
          v-model:state="searchBarPanelState"
          class="z-layer-3"
          border-radios="10px"
        >
          <div style="padding: 5px; background-color: var(--app-bg-surface)">
            <!--TODO在这里实现一个更灵活的组合查询条件的组件，比如拖拽组成AND或OR组合-->
            <el-button @click="handleTest">
              test
            </el-button>
            <!--测试 embed 插槽位：有插件声明 position="test.embed" 则渲染插件组件，否则显示默认内容-->
            <EmbedSlotRenderer position="test.embed">
              <span style="color: var(--app-text-secondary); font-size: 12px;">[embed 插槽位 test.embed：无插件插入]</span>
            </EmbedSlotRenderer>
          </div>
        </collapse-panel>
      </div>
      <el-button
        v-if="!workSetView"
        type="primary"
        class="topbar-items"
        @click="queryWorkPage(false)"
      >
        搜索
      </el-button>
      <el-button
        v-if="workSetView"
        type="primary"
        plain
        class="topbar-items"
        @click="workSetCreateDialogState = true"
      >
        新建作品集
      </el-button>
      <el-button
          v-if="workSetView"
          type="primary"
          class="topbar-items"
          @click="queryWorkSetPage(false)"
      >
        搜索
      </el-button>
    </div>
    <div
      ref="workSpace"
      class="main-page-work-space"
    >
      <div class="view-wrapper">
        <!-- 作品视图 -->
        <div
          class="view-container"
          :class="{ 'view-slide-left': workSetView }"
        >
          <el-scrollbar v-el-scrollbar-bottomed="() => queryWorkPage(true)">
            <work-grid-for-main-page
              ref="workGridRef"
              v-model:current-work-index="currentWorkIndex"
              class="main-page-work-grid"
              :work-list="workList"
              :checkable="multiSelectEnabled"
              :checked-work-ids="workSelectionStore.workIds"
              @checked-change="handleWorkCheckedChange"
              @work-set-deleted="handleWorkSetDeleted"
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
        <div
          class="view-container"
          :class="{ 'view-slide-right': !workSetView }"
        >
          <el-scrollbar v-el-scrollbar-bottomed="() => queryWorkSetPage(true)">
            <work-set-grid-for-main-page
              ref="workSetGridRef"
              v-model:current-work-set-index="currentWorkSetIndex"
              class="main-page-work-grid"
              :work-set-list="workSetList"
              :checkable="multiSelectEnabled"
              :checked-work-set-ids="workSelectionStore.workSetIds"
              @checked-change="handleWorkSetCheckedChange"
              @work-set-deleted="handleWorkSetDeleted"
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
    <!-- 浮动操作栏：多选开启且选择非空时浮出；[导出] 为首个消费者，未来批量操作位（删除/加入作品集等）在此追加 -->
    <div
      v-if="multiSelectEnabled && workSelectionStore.hasSelection"
      class="main-page-selection-bar z-layer-5"
    >
      <span class="main-page-selection-bar-count">
        已选 {{ workSelectionStore.totalCount }} 项
      </span>
      <el-button
        type="primary"
        @click="handleExport"
      >
        导出
      </el-button>
      <el-button @click="handleClearSelection">
        清除选择
      </el-button>
    </div>
    <!-- 新建作品集弹窗（创建成功后刷新作品集列表） -->
    <work-set-create-dialog
      v-model:state="workSetCreateDialogState"
      @created="handleWorkSetCreated"
    />
    <!-- 导出进度弹窗（打开即携带选中 id 列表触发异步导出，进度/取消/完成展示） -->
    <export-progress-dialog
      v-model:state="exportDialogState"
      :work-ids="workSelectionStore.workIds"
      :work-set-ids="workSelectionStore.workSetIds"
    />
  </div>
</template>

<style scoped>
.main-page {
  display: flex;
  flex-direction: column;
  height: calc(100% - 12px);
  width: calc(100% - 12px);
  padding: 6px;
  background-color: var(--app-bg-page);
  position: relative;
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

/* 多选开关：顶栏 radio 组旁，垂直居中与相邻控件对齐 */
.main-page-multiselect-toggle {
  display: flex;
  align-items: center;
  gap: 4px;
}

.main-page-multiselect-toggle-label {
  font-size: 13px;
  color: var(--app-text-secondary);
  white-space: nowrap;
}

.main-page-searchbar {
  flex-grow: 1;
}

.main-page-work-space {
  position: relative;
  display: flex;
  flex-direction: column;
  height: calc(100% - 33px);
  margin-top: 3px;
  border-radius: var(--app-radius);
  overflow: hidden;
  padding: 6px;
  background-color: var(--app-bg-surface);
}

/* 浮动操作栏：底部居中悬浮，覆盖内容但不遮挡顶部栏 */
.main-page-selection-bar {
  position: absolute;
  bottom: 24px;
  left: 50%;
  transform: translateX(-50%);
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 16px;
  background-color: var(--app-bg-surface);
  border: 1px solid var(--app-border-color);
  border-radius: var(--app-radius-lg);
  box-shadow: var(--app-shadow);
}

.main-page-selection-bar-count {
  color: var(--app-text-primary);
  font-weight: bold;
  white-space: nowrap;
}

.main-page-work-grid {
  margin-right: 10px;
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
  color: var(--app-color-info);
  font-weight: bold;
  border-radius: var(--app-radius);
  background-color: var(--app-color-info-light-9);
  text-align: center;
  cursor: pointer;
}

.work-grid-load-more:hover {
  background-color: var(--app-color-info-light-7);
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
  background-color: var(--app-tag-green-bg);
  color: var(--app-tag-green-text);
}

.main-page-auto-load-tag-select-tag-type-checkbox-site-tag {
  background-color: var(--app-tag-blue-bg);
  color: var(--app-tag-blue-text);
}

.main-page-auto-load-tag-select-tag-type-checkbox-local-author {
  background-color: var(--app-tag-red-bg);
  color: var(--app-tag-red-text);
}

.main-page-auto-load-tag-select-tag-type-checkbox-site-author {
  background-color: var(--app-tag-purple-bg);
  color: var(--app-tag-purple-text);
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
