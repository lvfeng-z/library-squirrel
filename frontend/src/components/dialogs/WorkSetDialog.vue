<script setup lang="ts">
import {computed, ref, Ref, watch} from 'vue'
import {arrayNotEmpty, isNullish, notNullish} from '@renderer/utils/CommonUtil.ts'
import ApiUtil from '@renderer/utils/ApiUtil.ts'
import StaticHeightDialog from '@renderer/components/dialogs/StaticHeightDialog.vue'
import WorkGridForWorkSet from '@renderer/components/common/WorkGridForWorkSet.vue'
import WorkQueryView from '@renderer/components/common/WorkQueryView.vue'
import {
  SearchCondition,
  SearchConditionQuery,
  SearchType,
  SelectItem,
  WorkFullDTO,
  WorkSearchOperator,
  WorkSetDTO,
  WorkSetWithWorksResultDTO
} from "@bindings/github.com//lvfeng-z/library-squirrel-sdk/dto"
import IPage from '@renderer/model/util/IPage.ts'
import Page from '@renderer/model/util/Page.ts'
import {ArrowLeft, Close, Delete, Edit, Picture, Plus} from '@element-plus/icons-vue'
import {ElMessage, ElMessageBox} from 'element-plus'
import lodash from 'lodash'
import ApiResponse from '@renderer/model/util/ApiResponse.ts'
import {setSearchTagColor} from '@renderer/utils/SearchTagColorUtil.ts'
import WorkCardItem from '@renderer/model/dto/WorkCardItem.ts'
import {workSetListWorkSetWithWorkByIds} from '@renderer/apis/http/wrappers/workSet'
import {
  reWorkWorkSetLinkBatchToWorkSet,
  reWorkWorkSetRemoveBatchFromWorkSet,
  reWorkWorkSetSetCover
} from '@renderer/apis/http/wrappers/reWorkWorkSet'
import {searchQuerySearchConditionPage, searchQueryWorkPage} from '@renderer/apis/http/wrappers/search'
import {newPage} from "@renderer/utils/Pager.ts";

// props
const props = defineProps<{
  width?: string
}>()

// model
// 弹窗开关
const state = defineModel<boolean>('state', { required: true })
const currentWorkSetId = defineModel<number>('currentWorkSetId', { required: true })

// 变量
// 视图状态: 'manage' 管理模式, 'select' 选择作品模式
const viewMode = ref<'manage' | 'select'>('manage')
// 是否启用选择模式
const isCheckable = ref(false)
// 选中的作品id列表
const checkedWorkIds = ref<number[]>([])
// 接口
const apis = {
  workSetListWorkSetWithWorkByIds,
  reWorkWorkSetLinkBatchToWorkSet,
  reWorkWorkSetRemoveBatchFromWorkSet,
  reWorkWorkSetSetCover,
  searchQueryWorkPage,
  searchQuerySearchConditionPage
}
// 当前作品集
const currentWorkSet = ref<WorkSetDTO | null>(null)
// 作品列表（bindings WorkFullDTO 嵌套结构）
const workList: Ref<WorkFullDTO[]> = ref([])
// 当前作品的索引
const currentWorkIndex = ref(0)
// 选择作品组件相关
const isSelectingWork = ref(false)
const selectedWorkIdsForAdd = ref<number[]>([])
// WorkQueryView 组件的 ref
const workQueryViewRef = ref()
// 添加作品页面使用的搜索项类型
const searchConditionType = ref<SearchType[]>()

// 计算属性：选择作品面板是否显示
const isSelectPanelVisible = computed(() => viewMode.value === 'select')

// 方法
async function loadWorkList() {
  if (isNullish(currentWorkSetId.value)) {
    return
  }
  const workSetId = currentWorkSetId.value
  const response = await apis.workSetListWorkSetWithWorkByIds([workSetId])
  if (ApiUtil.check(response)) {
    const workSetList = ApiUtil.data<(WorkSetWithWorksResultDTO | null)[]>(response)
    if (arrayNotEmpty(workSetList) && notNullish(workSetList[0])) {
      const result = workSetList[0]
      currentWorkSet.value = result.workSet
      workList.value = (result.works ?? []).filter(notNullish) as WorkFullDTO[]
      currentWorkIndex.value = 0
    }
  }
}

// 移除按钮点击处理
async function handleDelete() {
  if (checkedWorkIds.value.length === 0) {
    ElMessage({
      type: 'warning',
      message: '请先选择要移除的作品'
    })
    return
  }

  try {
    // 确认对话框
    await ElMessageBox.confirm(`确定要从作品集中移除选中的 ${checkedWorkIds.value.length} 个作品吗？`, '确认移除', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning'
    })

    const workIds = [...checkedWorkIds.value] // 创建数组副本避免传递响应式对象
    const workSetId = currentWorkSetId.value
    const response = await apis.reWorkWorkSetRemoveBatchFromWorkSet(workSetId, workIds)

    if (ApiUtil.check(response)) {
      const deletedCount = workIds.length
      ElMessage({
        type: 'success',
        message: `成功从作品集中移除 ${deletedCount} 个作品`
      })
      // 重新加载作品列表
      await loadWorkList()
      // 清空选中状态
      checkedWorkIds.value = []
    } else {
      ElMessage({
        type: 'error',
        message: `移除作品失败: ${response.msg || '未知错误'}`
      })
    }
  } catch (error) {
    // 用户取消移除
    if (error === 'cancel' || error === 'close') {
      return
    }
    ElMessage({
      type: 'error',
      message: `移除作品失败: ${error}`
    })
  }
}

// 设为封面按钮点击处理
async function handleSetCover() {
  if (checkedWorkIds.value.length === 0) {
    ElMessage({
      type: 'warning',
      message: '请先选择要设为封面的作品'
    })
    return
  }

  if (checkedWorkIds.value.length > 1) {
    ElMessage({
      type: 'warning',
      message: '只能选择一个作品设为封面'
    })
    return
  }

  try {
    const workId = checkedWorkIds.value[0]
    const workSetId = currentWorkSetId.value
    const response = await apis.reWorkWorkSetSetCover(workSetId, workId)

    if (ApiUtil.check(response)) {
      ElMessage({
        type: 'success',
        message: '封面设置成功'
      })
      // 清空选中状态
      checkedWorkIds.value = []
    } else {
      ElMessage({
        type: 'error',
        message: `设置封面失败: ${response.msg || '未知错误'}`
      })
    }
  } catch (error) {
    ElMessage({
      type: 'error',
      message: `设置封面失败: ${error}`
    })
  }
}

// 加载搜索条件选项
async function loadSearchItemPage(page: IPage<SelectItem>, input?: string): Promise<IPage<SelectItem>> {
  const query = new SearchConditionQuery()
  query.keyword = input ?? undefined
  query.types = lodash.cloneDeep(searchConditionType.value)
  let response: ApiResponse
  try {
    response = await apis.searchQuerySearchConditionPage(newPage({ pageNumber: page.pageNumber, pageSize: page.pageSize }), query)
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

// 点击添加按钮，切换到选择作品模式
function handleAdd() {
  viewMode.value = 'select'
  // 重置选择状态
  selectedWorkIdsForAdd.value = []
  // 重置 WorkQueryView 并加载作品列表
  workQueryViewRef.value?.clearConditions()
  workQueryViewRef.value?.queryWork()
}

// 作品查询函数 - 支持排除当前作品集的作品
async function fetchWorkPageForAdd(page: Page<WorkCardItem>, conditions: SearchCondition[]): Promise<Page<WorkCardItem>> {
  // 使用 WORK_SET 类型的 SearchCondition 排除当前作品集的作品
  conditions.push(
    new SearchCondition({
      type: SearchType.WorkSet,
      value: currentWorkSetId.value,
      operator: WorkSearchOperator.NotEqual
    })
  )

  // 调用原始 API
  const response = await apis.searchQueryWorkPage(
      newPage<WorkFullDTO>({ pageNumber: page.pageNumber, pageSize: page.pageSize, pageCount: page.pageCount }),
      conditions
  )
  if (ApiUtil.check(response)) {
    const resultPage = ApiUtil.data<Page<WorkFullDTO>>(response)
    if (isNullish(resultPage)) {
      return new Page<WorkCardItem>()
    }
    resultPage.data = resultPage.data?.filter((origin): origin is WorkFullDTO => notNullish(origin)).map((origin) => new WorkFullDTO(origin))
    return resultPage as unknown as Page<WorkCardItem>
  }
  return new Page<WorkCardItem>()
}

// 点击选择面板的取消按钮
function handleSelectCancel() {
  viewMode.value = 'manage'
  isSelectingWork.value = false
}

// 点击选择面板的确定按钮
async function handleSelectConfirm() {
  if (selectedWorkIdsForAdd.value.length === 0) {
    ElMessage({
      type: 'warning',
      message: '请先选择要添加的作品'
    })
    return
  }

  try {
    const response = await apis.reWorkWorkSetLinkBatchToWorkSet(
      currentWorkSetId.value,
      [...selectedWorkIdsForAdd.value]
    )

    if (ApiUtil.check(response)) {
      const addedCount = ApiUtil.data<number>(response)
      ElMessage({
        type: 'success',
        message: `成功添加 ${addedCount} 个作品到作品集`
      })
      // 重新加载作品列表
      await loadWorkList()
      // 返回管理视图
      viewMode.value = 'manage'
      isSelectingWork.value = false
      selectedWorkIdsForAdd.value = []
    } else {
      ElMessage({
        type: 'error',
        message: `添加作品失败: ${response.msg || '未知错误'}`
      })
    }
  } catch (error) {
    ElMessage({
      type: 'error',
      message: `添加作品失败: ${error}`
    })
  }
}

// 处理选中状态变化
function handleCheckedChange(ids: number[]) {
  checkedWorkIds.value = ids
}

// 处理选中作品变化
function handleSelectWorkCheckedChange(ids: number[]) {
  selectedWorkIdsForAdd.value = ids
}

// 关闭弹窗的回调
function beforeDialogClose(done: (shouldCancel?: boolean) => void) {
  viewMode.value = 'manage'
  isCheckable.value = false
  checkedWorkIds.value = []
  done()
}

// watch
watch(currentWorkSetId, () => loadWorkList())
// 监听对话框打开，重新加载数据以获取最新排序
watch(state, (newValue) => {
  if (newValue && currentWorkSetId.value) {
    loadWorkList()
  }
})
watch(isCheckable, (newValue) => {
  if (!newValue) {
    // 退出管理模式时清空选中状态
    checkedWorkIds.value = []
  }
})
</script>

<template>
  <static-height-dialog
    v-model:state="state"
    :width="props.width"
    :before-close="beforeDialogClose"
  >
    <template #header>
      <div
        v-if="viewMode === 'manage'"
        class="work-set-header"
      >
        <span>{{ currentWorkSet?.siteWorkSetName }}</span>
        <div v-if="!isCheckable">
          <el-button
            type="primary"
            :plain="true"
            @click="isCheckable = true"
          >
            <el-icon><Edit /></el-icon>
            管理
          </el-button>
        </div>
        <div
          v-else
          class="work-set-header-actions"
        >
          <el-button
            type="primary"
            @click="handleAdd"
          >
            <el-icon><Plus /></el-icon>
            添加
          </el-button>
          <el-button
            type="danger"
            @click="handleDelete"
          >
            <el-icon><Delete /></el-icon>
            移除
          </el-button>
          <el-button
            type="success"
            :disabled="checkedWorkIds.length !== 1"
            @click="handleSetCover"
          >
            <el-icon><Picture /></el-icon>
            设为封面
          </el-button>
          <el-button @click="isCheckable = false">
            <el-icon><Close /></el-icon>
            取消
          </el-button>
        </div>
      </div>
      <div
        v-else
        class="work-set-header"
      >
        <div class="work-set-header-back">
          <el-button @click="handleSelectCancel">
            <el-icon><ArrowLeft /></el-icon>
          </el-button>
          <span>从作品库添加作品</span>
        </div>
        <div class="work-set-header-actions">
          <el-button
            type="primary"
            :disabled="selectedWorkIdsForAdd.length === 0"
            @click="handleSelectConfirm"
          >
            <el-icon><Plus /></el-icon>
            确定添加
          </el-button>
          <el-button @click="handleSelectCancel">
            <el-icon><Close /></el-icon>
            取消
          </el-button>
        </div>
      </div>
    </template>

    <div class="work-set-dialog-main-container">
      <div :class="{ 'work-set-main-left': true, 'main-left-visible': !isSelectPanelVisible }">
        <el-scrollbar>
          <work-grid-for-work-set
            v-model:current-work-set-id="currentWorkSetId"
            v-model:current-work-index="currentWorkIndex"
            :work-list="workList"
            :checkable="isCheckable"
            :checked-work-ids="checkedWorkIds"
            @checked-change="handleCheckedChange"
          />
        </el-scrollbar>
      </div>

      <div :class="{ 'work-set-select-panel': true, 'z-layer-2': true, 'select-panel-visible': isSelectPanelVisible }">
        <work-query-view
          ref="workQueryViewRef"
          v-model:search-condition-type="searchConditionType"
          :load-search-item-page="loadSearchItemPage"
          :fetch-work-page="fetchWorkPageForAdd"
          :color-resolver="setSearchTagColor"
          :checkable="true"
          :checked-work-ids="selectedWorkIdsForAdd"
          :auto-search-on-input-change="false"
          :auto-search-on-tag-change="false"
          tag-select-tags-gap="8px"
          tag-select-max-height="200px"
          tag-select-min-height="36px"
          @checked-change="handleSelectWorkCheckedChange"
        />
      </div>
    </div>
  </static-height-dialog>
</template>

<style scoped>
.work-set-dialog-main-container {
  display: flex;
  flex-direction: row;
  height: 100%;
  width: 100%;
  overflow: hidden;
  position: relative;
}

/* 左侧区域 - 已有作品 */
.work-set-main-left {
  display: flex;
  flex-direction: column;
  height: 100%;
  width: 100%;
  transition: transform 0.3s ease;
}

.main-left-visible {
  transform: translateX(0);
}

/* 选择作品面板 */
.work-set-select-panel {
  position: absolute;
  right: 0;
  top: 0;
  height: 100%;
  width: 100%;
  background-color: var(--app-bg-surface);
  display: flex;
  flex-direction: column;
  padding-right: 3px;
  transition: transform 0.3s ease;
  transform: translateX(100%);
}
.select-panel-visible {
  transform: translateX(3px);
}

/* 头部样式 */
.work-set-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  width: 100%;
}

.work-set-header-back {
  display: flex;
  gap: 10px;
}

.work-set-header-actions {
  display: flex;
  gap: 8px;
}
</style>
