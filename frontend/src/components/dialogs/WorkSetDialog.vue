<script setup lang="ts">
import {computed, ref, Ref, watch} from 'vue'
import {arrayNotEmpty, isNullish, notNullish} from '@renderer/utils/CommonUtil.ts'
import ApiUtil from '@renderer/utils/ApiUtil.ts'
import {isBlank} from '@renderer/utils/StringUtil.ts'
import StaticHeightDialog from '@renderer/components/dialogs/StaticHeightDialog.vue'
import WorkGridForWorkSet from '@renderer/components/common/WorkGridForWorkSet.vue'
import WorkQueryView from '@renderer/components/common/WorkQueryView.vue'
import WorkSetSelectPanel from '@renderer/components/common/WorkSetSelectPanel.vue'
import TagBox from '@renderer/components/common/TagBox.vue'
import AuthorInfo from '@renderer/components/common/AuthorInfo.vue'
import SegmentedTagItem from '@renderer/model/util/SegmentedTagItem.ts'
import { WorkSetDTO } from "@bindings/github.com//lvfeng-z/library-squirrel-sdk/dto"
import {
  SearchCondition,
  SearchConditionQuery,
  SearchType,
  SelectItem,
  WorkFullDTO,
  WorkSearchOperator,
  WorkSetWithCoverDTO,
  WorkSetWithWorksResultDTO,
  RankedLocalAuthor,
  RankedSiteAuthor
} from "@bindings/github.com/library-squirrel/backend/base/model/dto"
import IPage from '@renderer/model/util/IPage.ts'
import Page from '@renderer/model/util/Page.ts'
import {ArrowLeft, Close, Delete, Document, Download, Edit, Picture, Plus, Sort} from '@element-plus/icons-vue'
import {ElMessage, ElMessageBox} from 'element-plus'
import lodash from 'lodash'
import ApiResponse from '@renderer/model/util/ApiResponse.ts'
import {setSearchTagColor} from '@renderer/utils/SearchTagColorUtil.ts'
import WorkCardItem from '@renderer/model/dto/WorkCardItem.ts'
import {workSetListChildWorkSets, workSetListWorkSetWithWorkByIds, workSetAddChildWorkSet, workSetRemoveChildWorkSet, workSetMergeWorkSetInto, workSetUpdate, workSetApplySiteOrder} from '@renderer/apis/http/wrappers/workSet'
import {
  reWorkWorkSetLinkBatchToWorkSet,
  reWorkWorkSetRemoveBatchFromWorkSet,
  reWorkWorkSetSetCover
} from '@renderer/apis/http/wrappers/reWorkWorkSet'
import {searchQuerySearchConditionPage, searchQueryWorkPage, searchQueryWorkSetPage} from '@renderer/apis/http/wrappers/search'
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
// 视图状态: 'manage' 管理模式, 'select' 选择作品模式, 'selectWorkSet' 选择子作品集模式
const viewMode = ref<'manage' | 'select' | 'selectWorkSet'>('manage')
// 是否启用选择模式
const isCheckable = ref(false)
// 选中的作品id列表
const checkedWorkIds = ref<number[]>([])
// 接口
const apis = {
  workSetListWorkSetWithWorkByIds,
  workSetListChildWorkSets,
  workSetAddChildWorkSet,
  workSetRemoveChildWorkSet,
  workSetMergeWorkSetInto,
  workSetUpdate,
  workSetApplySiteOrder,
  reWorkWorkSetLinkBatchToWorkSet,
  reWorkWorkSetRemoveBatchFromWorkSet,
  reWorkWorkSetSetCover,
  searchQueryWorkPage,
  searchQuerySearchConditionPage,
  searchQueryWorkSetPage
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
// 子作品集列表（当前作品集的直接子集，manage 模式展示）
const childWorkSets = ref<WorkSetDTO[]>([])
// 选择子作品集模式下勾选的作品集 id（要新增为子集）
const selectedChildWorkSetIds = ref<number[]>([])
// 物理纳入模式下勾选的源作品集 id
const selectedMergeSourceIds = ref<number[]>([])
// 当前作品集选择面板的用途：'addChild' 添加子集 / 'mergeSource' 物理纳入源
const selectPurpose = ref<'addChild' | 'mergeSource'>('addChild')
// 已是子集的作品集 id 集合（选择面板排除已加入 + 排除自身，防重复纳入）
const existingChildWorkSetIds = ref<Set<number>>(new Set())
// WorkSetSelectPanel 组件的 ref
const workSetSelectPanelRef = ref()

// 元数据 drawer 开关
const workSetDrawerState = ref(false)
// drawer 内编辑态：名称（nickName）与本地描述（description）
const editNickName = ref('')
const editDescription = ref('')

// 计算属性：选择作品面板是否显示
const isSelectPanelVisible = computed(() => viewMode.value === 'select')
// 计算属性：选择子作品集面板是否显示（添加子集 / 物理纳入源共用）
const isSelectWorkSetPanelVisible = computed(() => viewMode.value === 'selectWorkSet')
// 计算属性：当前选择面板激活的选中 id 列表（按用途路由）
const activeSelectedWorkSetIds = computed(() =>
  selectPurpose.value === 'addChild' ? selectedChildWorkSetIds.value : selectedMergeSourceIds.value
)
// 计算属性：当前选择面板的确认按钮是否可用
const isSelectWorkSetConfirmDisabled = computed(() => activeSelectedWorkSetIds.value.length === 0)

// 元数据 drawer：所属站点（按 site.id 去重）
const aggregatedSites = computed<SegmentedTagItem[]>(() => {
  const seen = new Set<number>()
  const items: SegmentedTagItem[] = []
  for (const w of workList.value) {
    const site = w.site
    if (isNullish(site) || isNullish(site.id)) continue
    if (seen.has(site.id)) continue
    seen.add(site.id)
    items.push(
      new SegmentedTagItem({
        value: site.id,
        label: site.siteName ?? '?',
        disabled: false
      })
    )
  }
  return items
})

// 元数据 drawer：作者聚合（localAuthors + siteAuthors 各按 id 去重，AuthorInfo 内部再做本地/站点代表去重）
const aggregatedLocalAuthors = computed<RankedLocalAuthor[]>(() => {
  const seen = new Set<number>()
  const items: RankedLocalAuthor[] = []
  for (const w of workList.value) {
    for (const la of w.localAuthors ?? []) {
      if (isNullish(la) || isNullish(la.author.id) || seen.has(la.author.id)) continue
      seen.add(la.author.id)
      items.push(la)
    }
  }
  return items
})
const aggregatedSiteAuthors = computed<RankedSiteAuthor[]>(() => {
  const seen = new Set<number>()
  const items: RankedSiteAuthor[] = []
  for (const w of workList.value) {
    for (const sa of w.siteAuthors ?? []) {
      if (isNullish(sa) || isNullish(sa.author.id) || seen.has(sa.author.id)) continue
      seen.add(sa.author.id)
      items.push(sa)
    }
  }
  return items
})

// 元数据 drawer：本地标签（按 localTag.id 去重）
const aggregatedLocalTags = computed<SegmentedTagItem[]>(() => {
  const seen = new Set<number>()
  const items: SegmentedTagItem[] = []
  for (const w of workList.value) {
    for (const lt of w.localTags ?? []) {
      if (isNullish(lt) || isNullish(lt.id) || seen.has(lt.id)) continue
      seen.add(lt.id)
      items.push(
        new SegmentedTagItem({
          value: lt.id,
          label: lt.localTagName ?? '?',
          disabled: false
        })
      )
    }
  }
  return items
})

// 元数据 drawer：站点标签（按 siteTag.id 去重，不读 siteTag.localTag 交叉）
const aggregatedSiteTags = computed<SegmentedTagItem[]>(() => {
  const seen = new Set<number>()
  const items: SegmentedTagItem[] = []
  for (const w of workList.value) {
    for (const st of w.siteTags ?? []) {
      if (isNullish(st) || isNullish(st.siteTag?.id) || seen.has(st.siteTag.id)) continue
      seen.add(st.siteTag.id)
      items.push(
        new SegmentedTagItem({
          value: st.siteTag.id,
          label: st.siteTag.siteTagName ?? '?',
          subLabels: [isBlank(st.site?.siteName) ? '?' : (st.site.siteName ?? '?')],
          disabled: false
        })
      )
    }
  }
  return items
})

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
  // 并行加载直接子作品集（层级管理展示用）
  loadChildWorkSets()
}

// 加载当前作品集的直接子作品集
async function loadChildWorkSets() {
  if (isNullish(currentWorkSetId.value)) {
    return
  }
  const response = await apis.workSetListChildWorkSets(currentWorkSetId.value)
  if (ApiUtil.check(response)) {
    const list = ApiUtil.data<(WorkSetDTO | null)[]>(response)
    childWorkSets.value = (list ?? []).filter(notNullish) as WorkSetDTO[]
    existingChildWorkSetIds.value = new Set(childWorkSets.value.map((ws) => ws.id))
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

// 应用原站序：把原站序(site_sort_order)拷贝到本地序(sort_order)，重载作品列表即按原站顺序展示
async function handleApplySiteOrder() {
  try {
    await ElMessageBox.confirm('将按原站顺序重新排列作品集内作品（覆盖当前本地排序）。确认？', '应用原站序', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning'
    })
  } catch {
    return
  }
  const response = await apis.workSetApplySiteOrder(currentWorkSetId.value)
  if (ApiUtil.check(response)) {
    ElMessage({ type: 'success', message: '已应用原站序' })
    await loadWorkList()
  } else {
    ElMessage({ type: 'error', message: `应用原站序失败: ${response.msg || '未知错误'}` })
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
    // 后端返回 WorkFullDTO（id/nickName 嵌套在 .work 下），CardGrid/WorkInfo 期望 WorkCardItem（顶层字段），
    // 需经 WorkCardItem 适配层转换，否则 getId 取不到 id（勾选失效）、WorkInfo 取不到 nickName（显示"?"）
    resultPage.data = resultPage.data?.filter((origin): origin is WorkFullDTO => notNullish(origin)).map((origin) => new WorkCardItem(new WorkFullDTO(origin)))
    return resultPage as unknown as Page<WorkCardItem>
  }
  return new Page<WorkCardItem>()
}

// 点击选择面板的取消按钮
function handleSelectCancel() {
  viewMode.value = 'manage'
  isSelectingWork.value = false
}

// 点击「添加子作品集」按钮，切换到选择子作品集模式（用途：addChild）
function handleAddChildWorkSet() {
  selectPurpose.value = 'addChild'
  viewMode.value = 'selectWorkSet'
  selectedChildWorkSetIds.value = []
  workSetSelectPanelRef.value?.clearKeyword()
  workSetSelectPanelRef.value?.queryWorkSets()
}

// 点击「物理纳入」按钮，切换到选择源作品集模式（用途：mergeSource）
function handleMergeWorkSet() {
  selectPurpose.value = 'mergeSource'
  viewMode.value = 'selectWorkSet'
  selectedMergeSourceIds.value = []
  workSetSelectPanelRef.value?.clearKeyword()
  workSetSelectPanelRef.value?.queryWorkSets()
}

// 选择子作品集面板的查询函数（排除自身；按用途排除已加入子集）
async function fetchWorkSetPageForAdd(page: Page<WorkSetWithCoverDTO>, keyword?: string): Promise<Page<WorkSetWithCoverDTO>> {
  const response = await apis.searchQueryWorkSetPage(
    newPage<WorkSetWithCoverDTO>({ pageNumber: page.pageNumber, pageSize: page.pageSize, pageCount: page.pageCount }),
    []
  )
  if (ApiUtil.check(response)) {
    const resultPage = ApiUtil.data<Page<WorkSetWithCoverDTO>>(response)
    if (isNullish(resultPage)) {
      return new Page<WorkSetWithCoverDTO>()
    }
    // 前端过滤：排除自身、按用途排除已加入子集、按 keyword 名称过滤
    const selfId = currentWorkSetId.value
    resultPage.data = (resultPage.data ?? []).filter(notNullish).filter((ws) => {
      const id = ws.workSet?.id
      if (isNullish(id) || id === selfId) return false
      // 添加子集模式排除已是子集（避免重复纳入）；物理纳入模式不排除（允许纳入已关联的作品集）
      if (selectPurpose.value === 'addChild' && existingChildWorkSetIds.value.has(id)) return false
      if (notNullish(keyword) && keyword.trim()) {
        const name = (ws.workSet?.nickName ?? ws.workSet?.siteWorkSetName ?? '').toLowerCase()
        if (!name.includes(keyword.trim().toLowerCase())) return false
      }
      return true
    })
    return resultPage
  }
  return new Page<WorkSetWithCoverDTO>()
}

// 选择子作品集面板选中变化（按用途路由到对应选中列表）
function handleSelectWorkSetCheckedChange(ids: number[]) {
  if (selectPurpose.value === 'addChild') {
    selectedChildWorkSetIds.value = ids
  } else {
    selectedMergeSourceIds.value = ids
  }
}

// 选择子作品集面板确定：按用途分流
async function handleSelectWorkSetConfirm() {
  if (selectPurpose.value === 'addChild') {
    await doAddChildWorkSets()
  } else {
    await doMergeWorkSets()
  }
}

// 执行添加子作品集（逐个建立父子关系，后端事务内防环路）
async function doAddChildWorkSets() {
  if (selectedChildWorkSetIds.value.length === 0) {
    ElMessage({ type: 'warning', message: '请先选择要添加的子作品集' })
    return
  }
  const parentId = currentWorkSetId.value
  let successCount = 0
  let cycleDetected = false
  for (const childId of [...selectedChildWorkSetIds.value]) {
    const response = await apis.workSetAddChildWorkSet(parentId, childId)
    if (ApiUtil.check(response)) {
      successCount++
    } else {
      // 环路错误（后端 ErrWorkSetCycleDetected 文案）单独提示，不中断后续
      if ((response.msg ?? '').includes('环路')) {
        cycleDetected = true
      }
    }
  }
  if (successCount > 0) {
    ElMessage({ type: 'success', message: `成功添加 ${successCount} 个子作品集` })
  }
  if (cycleDetected) {
    ElMessage({ type: 'warning', message: '部分作品集会形成环路，已跳过' })
  }
  await loadChildWorkSets()
  await loadWorkList()
  viewMode.value = 'manage'
  selectedChildWorkSetIds.value = []
}

// 执行物理纳入（复制源及作品集的后代作品到当前作品集，静态快照、不可撤回）
async function doMergeWorkSets() {
  if (selectedMergeSourceIds.value.length === 0) {
    ElMessage({ type: 'warning', message: '请先选择要导入的源作品集' })
    return
  }
  const sources = [...selectedMergeSourceIds.value]
  // 二次确认：物理纳入是静态快照，不记录来源、不可撤回
  try {
    await ElMessageBox.confirm(
      `将把选中的 ${sources.length} 个作品集（及其子作品集）的作品复制到当前作品集。\n这是一份静态快照：之后源作品集变化不会同步，且无法一键撤回。确认导入？`,
      '确认导入',
      {
        confirmButtonText: '确认导入',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )
  } catch (error) {
    // 用户取消
    return
  }
  let successCount = 0
  for (const sourceId of sources) {
    const response = await apis.workSetMergeWorkSetInto(sourceId, currentWorkSetId.value)
    if (ApiUtil.check(response)) {
      successCount++
    } else {
      ElMessage({ type: 'error', message: `导入失败: ${response.msg || '未知错误'}` })
    }
  }
  if (successCount > 0) {
    ElMessage({ type: 'success', message: `成功导入 ${successCount} 个作品集的作品` })
  }
  await loadWorkList()
  viewMode.value = 'manage'
  selectedMergeSourceIds.value = []
}

// 移除子作品集（解除父子关系）
async function handleRemoveChildWorkSet(childId: number) {
  try {
    await ElMessageBox.confirm('确定解除该子作品集关系吗？', '确认解除', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning'
    })
    const response = await apis.workSetRemoveChildWorkSet(currentWorkSetId.value, childId)
    if (ApiUtil.check(response)) {
      ElMessage({ type: 'success', message: '已解除子作品集关系' })
      await loadChildWorkSets()
      await loadWorkList()
    } else {
      ElMessage({ type: 'error', message: `解除失败: ${response.msg || '未知错误'}` })
    }
  } catch (error) {
    if (error === 'cancel' || error === 'close') return
    ElMessage({ type: 'error', message: `解除失败: ${error}` })
  }
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
      // 后端 LinkBatchToWorkSet 为 void 语义（data=null），添加数量取本地选中数
      const addedCount = selectedWorkIdsForAdd.value.length
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
  workSetDrawerState.value = false
  done()
}

// 打开元数据 drawer：用当前作品集的 nickName/description 初始化编辑态
function openWorkSetDrawer() {
  editNickName.value = currentWorkSet.value?.nickName ?? ''
  editDescription.value = currentWorkSet.value?.description ?? ''
  workSetDrawerState.value = true
}

// 保存作品集元数据（名称 + 本地描述），成功后重新加载作品列表
async function handleSaveWorkSetMeta() {
  if (isNullish(currentWorkSet.value)) return
  const id = currentWorkSet.value.id
  try {
    const response = await apis.workSetUpdate({
      id,
      nickName: editNickName.value,
      description: editDescription.value
    })
    if (ApiUtil.check(response)) {
      ElMessage({ type: 'success', message: '保存成功' })
      workSetDrawerState.value = false
      await loadWorkList()
    } else {
      ElMessage({ type: 'error', message: `保存失败: ${response.msg || '未知错误'}` })
    }
  } catch (error) {
    ElMessage({ type: 'error', message: `保存失败: ${error}` })
  }
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
        <span>{{ isBlank(currentWorkSet?.nickName) ? currentWorkSet?.siteWorkSetName : currentWorkSet?.nickName }}</span>
        <div v-if="!isCheckable">
          <el-button
            type="primary"
            :plain="true"
            @click="openWorkSetDrawer"
          >
            <el-icon><Document /></el-icon>
            详情
          </el-button>
          <el-button
            type="primary"
            :plain="true"
            @click="isCheckable = true"
          >
            <el-icon><Edit /></el-icon>
            管理
          </el-button>
          <el-button
            type="primary"
            :plain="true"
            @click="handleApplySiteOrder"
          >
            <el-icon><Sort /></el-icon>
            原站序
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
            添加作品
          </el-button>
          <el-button
            type="danger"
            class="tone-fail"
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
          <el-button
              type="primary"
              @click="handleAddChildWorkSet"
          >
            <el-icon><Plus /></el-icon>
            添加子集
          </el-button>
          <el-button
              type="primary"
              @click="handleMergeWorkSet"
          >
            <el-icon><Download /></el-icon>
            导入
          </el-button>
          <el-button @click="isCheckable = false">
            <el-icon><Close /></el-icon>
            取消
          </el-button>
        </div>
      </div>
      <div
        v-else-if="viewMode === 'select'"
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
      <div
        v-else
        class="work-set-header"
      >
        <div class="work-set-header-back">
          <el-button @click="handleSelectCancel">
            <el-icon><ArrowLeft /></el-icon>
          </el-button>
          <span>{{ selectPurpose === 'addChild' ? '添加子作品集' : '导入作品集' }}</span>
        </div>
        <div class="work-set-header-actions">
          <el-button
            type="primary"
            :disabled="isSelectWorkSetConfirmDisabled"
            @click="handleSelectWorkSetConfirm"
          >
            <el-icon><Plus /></el-icon>
            {{ selectPurpose === 'addChild' ? '确定添加' : '确定导入' }}
          </el-button>
          <el-button @click="handleSelectCancel">
            <el-icon><Close /></el-icon>
            取消
          </el-button>
        </div>
      </div>
    </template>

    <div class="work-set-dialog-main-container">
      <div :class="{ 'work-set-main-left': true, 'main-left-visible': !isSelectPanelVisible && !isSelectWorkSetPanelVisible }">
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

      <!-- 选择子作品集面板（仿选择作品面板，滑入覆盖） -->
      <div :class="{ 'work-set-select-panel': true, 'z-layer-2': true, 'select-panel-visible': isSelectWorkSetPanelVisible }">
        <work-set-select-panel
          ref="workSetSelectPanelRef"
          :fetch-work-set-page="fetchWorkSetPageForAdd"
          :checkable="true"
          :checked-work-set-ids="activeSelectedWorkSetIds"
          @checked-change="handleSelectWorkSetCheckedChange"
        />
      </div>
    </div>

    <!-- 元数据 drawer：作品集完整元数据展示与编辑（名称/本地描述可编辑，其余只读聚合；子集 chips 迁入） -->
    <el-drawer
      v-model="workSetDrawerState"
      size="40%"
      :with-header="false"
    >
      <el-scrollbar class="work-set-drawer-scrollbar">
        <el-descriptions
          direction="horizontal"
          :column="1"
          border
        >
          <el-descriptions-item label="名称">
            <el-input v-model="editNickName" placeholder="请输入作品集名称" />
          </el-descriptions-item>
          <el-descriptions-item label="本地描述">
            <el-input
              v-model="editDescription"
              type="textarea"
              :rows="3"
              placeholder="请输入本地描述（与站点简介分离，重新抓取不会被覆盖）"
            />
          </el-descriptions-item>
          <el-descriptions-item label="站点简介">
            <span>{{ isBlank(currentWorkSet?.siteWorkSetDescription) ? '无' : currentWorkSet?.siteWorkSetDescription }}</span>
          </el-descriptions-item>
          <el-descriptions-item label="所属站点">
            <tag-box :data="aggregatedSites" />
          </el-descriptions-item>
          <el-descriptions-item label="作者">
            <author-info
              :local-authors="aggregatedLocalAuthors"
              :site-authors="aggregatedSiteAuthors"
            />
          </el-descriptions-item>
          <el-descriptions-item label="本地标签">
            <tag-box :data="aggregatedLocalTags" />
          </el-descriptions-item>
          <el-descriptions-item label="站点标签">
            <tag-box :data="aggregatedSiteTags" />
          </el-descriptions-item>
          <el-descriptions-item label="子作品集">
            <div
              v-if="childWorkSets.length > 0"
              class="work-set-drawer-children"
            >
              <el-tag
                v-for="cws in childWorkSets"
                :key="cws.id"
                class="work-set-drawer-child-chip"
                closable
                @close="handleRemoveChildWorkSet(cws.id)"
              >
                {{ cws.nickName ?? cws.siteWorkSetName ?? '未命名' }}
              </el-tag>
            </div>
            <span v-else class="work-set-drawer-empty">无</span>
          </el-descriptions-item>
        </el-descriptions>
        <div class="work-set-drawer-footer">
          <el-button
            type="primary"
            @click="handleSaveWorkSetMeta"
          >
            保存
          </el-button>
        </div>
      </el-scrollbar>
    </el-drawer>
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

/* 元数据 drawer */
.work-set-drawer-scrollbar {
  height: 100%;
}
.work-set-drawer-children {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.work-set-drawer-child-chip {
  max-width: 200px;
}
.work-set-drawer-empty {
  color: var(--app-text-secondary);
}
.work-set-drawer-footer {
  display: flex;
  justify-content: flex-end;
  padding: 16px 12px;
}
</style>
