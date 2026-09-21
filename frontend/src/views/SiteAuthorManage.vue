<script setup lang="ts">
import {h, onMounted, Ref, ref} from 'vue'
import BaseView from './BaseView.vue'
import SearchTable from '../components/common/SearchTable.vue'
import ApiUtil from '../utils/ApiUtil.ts'
import {ElMessage} from 'element-plus'
import DataTableOperationResponse from '../model/util/DataTableOperationResponse.ts'
import {Thead} from '../model/util/Thead.ts'
import OperationItem from '../model/util/OperationItem.ts'
import DialogMode from '../model/util/DialogMode.ts'
import {arrayIsEmpty, isNullish, notNullish} from '@renderer/utils/CommonUtil.ts'
import SiteAuthorDialog from '@renderer/components/dialogs/SiteAuthorDialog.vue'
import PluginCandidateSelectDialog from '@renderer/components/dialogs/PluginCandidateSelectDialog.vue'
import AvatarThumb from '@renderer/components/common/AvatarThumb.vue'
import {siteQuerySelectItemPageBySiteName} from '@renderer/apis/http'
import AutoLoadSelect from '@renderer/components/common/AutoLoadSelect.vue'
import {authorInfoApi, localAuthorQuerySelectItemPageByName, siteAuthorApi, appLauncherApi} from '@renderer/apis/http'
import {useNotificationStore} from '@renderer/store/UseNotificationStore.ts'
import {
  LocalAuthorDTO,
  SiteDTO
} from "@bindings/github.com//lvfeng-z/library-squirrel-sdk/dto"
import {
  SelectItem,
  PluginCandidate,
  SiteAuthorDTO,
  SiteAuthorLocalRelateDTO
} from "@bindings/github.com/library-squirrel/backend/base/model/dto"
import {SiteAuthorFetchResponse} from "@bindings/github.com/library-squirrel/backend/authorInfo"
import {SiteAuthorQueryDTO} from '@bindings/github.com/library-squirrel/backend/siteAuthor/models'
import {Operator, SortOrder} from '@bindings/github.com/library-squirrel/backend/base/query/models'
import {Page} from "@bindings/github.com/library-squirrel/backend/base/model";
import {newPage} from "@renderer/utils/Pager.ts";
import {isBlank} from "@renderer/utils/StringUtil.ts";

// onMounted
onMounted(() => {
  // 使用各字段的 Order 属性进行排序，通过 Priority 控制优先级
  siteAuthorQuery.value.updateTime = { value: null, order: SortOrder.OrderDesc, priority: 0 }
  siteAuthorQuery.value.createTime = { value: null, order: SortOrder.OrderDesc, priority: 1 }
  siteAuthorSearchTable.value.doSearch()
})

// 变量
// siteAuthorSearchTable的组件实例
const siteAuthorSearchTable = ref()
// 被改变的数据行
const changedRows: Ref<SiteAuthorLocalRelateDTO[]> = ref([])
// 站点作者SearchTable的operationButton
const operationButton: OperationItem<SiteAuthorLocalRelateDTO>[] = [
  {
    label: '保存',
    icon: 'Checked',
    buttonType: 'primary',
    code: 'save',
    rule: (row) => changedRows.value.includes(row)
  },
  {
    label: '创建同名本地作者',
    icon: 'CirclePlusFilled',
    buttonType: 'primary',
    code: 'create',
    rule: (row) => !row.hasSameNameLocalAuthor
  },
  { label: '拉取信息', icon: 'Download', code: 'fetchInfo' },
  { label: '主页', icon: 'Link', code: 'homepage', rule: (row) => notNullish(row.siteAuthor?.homepage) },
  { label: '查看', icon: 'view', code: DialogMode.VIEW },
  { label: '编辑', icon: 'edit', code: DialogMode.EDIT },
  { label: '删除', icon: 'delete', code: 'delete' }
]
// 站点作者SearchTable的表头
const siteAuthorThead: Ref<Thead<SiteAuthorLocalRelateDTO>[]> = ref([
  new Thead({
    type: 'custom',
    defaultDisabled: true,
    key: 'avatarFilePath',
    title: '头像',
    hide: false,
    width: 70,
    headerAlign: 'center',
    dataAlign: 'center',
    // 头像列小图（32px）：无头像/加载失败由 AvatarThumb 统一降级为占位图标
    render: (data) => h(AvatarThumb, { filePath: data as string | null | undefined, size: 32 })
  }),
  new Thead({
    type: 'text',
    defaultDisabled: true,
    dblclickToEdit: true,
    key: 'siteAuthor.authorName',
    title: '名称',
    hide: false,
    width: 250,
    headerAlign: 'center',
    dataAlign: 'center',
    showOverflowTooltip: true
  }),
  new Thead({
    type: 'textarea',
    defaultDisabled: true,
    dblclickToEdit: true,
    key: 'siteAuthor.introduce',
    title: '介绍',
    hide: false,
    width: 400,
    headerAlign: 'center',
    dataAlign: 'center',
    showOverflowTooltip: true
  }),
  new Thead({
    type: 'autoLoadSelect',
    editMethod: 'replace',
    defaultDisabled: true,
    dblclickToEdit: true,
    key: 'siteAuthor.localAuthorId',
    title: '本地作者',
    hide: false,
    width: 150,
    headerAlign: 'center',
    dataAlign: 'center',
    showOverflowTooltip: true,
    remote: true,
    remotePaging: true,
    remotePageMethod: localAuthorQuerySelectItemPageByName,
    getCacheData: (rowData: SiteAuthorLocalRelateDTO) => {
      if (isNullish(rowData.localAuthor?.id)) {
        return undefined
      }
      return new SelectItem({
        value: rowData.localAuthor.id,
        label: isNullish(rowData.localAuthor?.authorName) ? '' : rowData.localAuthor.authorName
      })
    },
    setCacheData: (rowData: SiteAuthorLocalRelateDTO, data: SelectItem) => {
      if (isNullish(rowData.localAuthor)) {
        rowData.localAuthor = new LocalAuthorDTO()
      }
      rowData.localAuthor.id = Number(data.value)
      rowData.localAuthor.authorName = data.label
    }
  }),
  new Thead({
    type: 'autoLoadSelect',
    editMethod: 'replace',
    defaultDisabled: true,
    dblclickToEdit: true,
    key: 'siteAuthor.siteId',
    title: '站点',
    hide: false,
    width: 150,
    headerAlign: 'center',
    dataAlign: 'center',
    showOverflowTooltip: true,
    remote: true,
    remotePaging: true,
    remotePageMethod: siteQuerySelectItemPageBySiteName,
    getCacheData: (rowData: SiteAuthorLocalRelateDTO) => {
      if (isNullish(rowData.site?.id)) {
        return undefined
      }
      return new SelectItem({
        value: rowData.site.id,
        label: isNullish(rowData.site?.siteName) ? '' : rowData.site.siteName
      })
    },
    setCacheData: (rowData: SiteAuthorLocalRelateDTO, data: SelectItem) => {
      if (isNullish(rowData.site)) {
        rowData.site = new SiteDTO()
      }
      rowData.site.id = Number(data.value)
      rowData.site.siteName = data.label
    }
  }),
  new Thead({
    type: 'datetime',
    defaultDisabled: true,
    dblclickToEdit: true,
    key: 'siteAuthor.updateTime',
    title: '修改时间',
    hide: false,
    width: 200,
    headerAlign: 'center',
    dataAlign: 'center',
    showOverflowTooltip: true
  })
])
// 站点作者SearchTable的分页
const page: Ref<Page<SiteAuthorLocalRelateDTO>> = ref(newPage<SiteAuthorLocalRelateDTO>())
// 站点作者查询参数
const siteAuthorQuery: Ref<SiteAuthorQueryDTO> = ref(new SiteAuthorQueryDTO())
// 站点作者弹窗的mode
const siteAuthorDialogMode: Ref<DialogMode> = ref(DialogMode.EDIT)
// 站点作者的对话框开关
const dialogState: Ref<boolean> = ref(false)
// 站点作者对话框的数据
const dialogData: Ref<SiteAuthorLocalRelateDTO> = ref(new SiteAuthorLocalRelateDTO())
// 表格当前多选选中的站点作者行（批量拉取入口的输入）
const selectedRows: Ref<SiteAuthorLocalRelateDTO[]> = ref([])
// 单行「拉取信息」进行中（表格区挂 loading）
const fetchInfoLoading: Ref<boolean> = ref(false)
// 批量拉取进行中（按钮 loading + 表格区挂 loading）
const batchFetchLoading: Ref<boolean> = ref(false)
// 插件候选选择器开关与其候选清单（候选多于一个且未显选时弹出）
const pluginSelectState: Ref<boolean> = ref(false)
const pluginSelectCandidates: Ref<PluginCandidate[]> = ref([])
// 冲突挂起的拉取：等待用户选择期间持有待拉取作者标识、展示名索引与单/批形态，选择后据此重发
let pendingFetchIds: number[] = []
let pendingFetchNameById: Map<number, string> = new Map()
let pendingFetchSingle = true

// 方法
// 分页查询站点作者的函数
async function siteAuthorQueryPageFn(
  page: Page<SiteAuthorLocalRelateDTO>
): Promise<Page<SiteAuthorLocalRelateDTO>> {
  siteAuthorQuery.value.authorName.operator = Operator.OpLike
  const response = await siteAuthorApi.siteAuthorQueryLocalRelateDTOPage(page, siteAuthorQuery.value)
  return response.data
}
// 处理站点作者新增按钮点击事件
async function handleCreateButtonClicked() {
  siteAuthorDialogMode.value = DialogMode.NEW
  dialogData.value = new SiteAuthorLocalRelateDTO()
  dialogData.value.siteAuthor = new SiteAuthorDTO()
  dialogState.value = true
}
// 处理站点作者数据行按钮点击事件
async function handleRowButtonClicked(op: DataTableOperationResponse<SiteAuthorLocalRelateDTO>) {
  const homepage = op.data.siteAuthor?.homepage
  switch (op.code) {
    case 'create':
      await creatSameNameLocalAuthorAndBind(op.data)
      siteAuthorSearchTable.value.doSearch()
      break
    case 'fetchInfo':
      await fetchSiteAuthorInfo(op.data)
      break
    case 'save':
      saveRowEdit(op.data)
      break
    case 'homepage':
      if (isBlank(homepage)) {
        ElMessage.error('无法打开作者主页，作者信息缺少主页信息')
        break
      }
      await appLauncherApi.appLauncherOpenExternal(homepage)
      break
    case DialogMode.VIEW:
      siteAuthorDialogMode.value = DialogMode.VIEW
      dialogData.value = op.data
      dialogState.value = true
      break
    case DialogMode.EDIT:
      siteAuthorDialogMode.value = DialogMode.EDIT
      dialogData.value = op.data
      dialogState.value = true
      break
    case 'delete':
      deleteSiteAuthor(Number(op.id))
      break
    default:
      break
  }
}
// 处理站点作者弹窗请求成功事件
function refreshTable() {
  siteAuthorSearchTable.value.doSearch()
}
// 删除站点作者
async function deleteSiteAuthor(id: number) {
  try {
    const response = await siteAuthorApi.siteAuthorDeleteById(id)
    ApiUtil.msg(response)
    await siteAuthorSearchTable.value.doSearch()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}
// 保存行数据编辑
async function saveRowEdit(newData: SiteAuthorLocalRelateDTO) {
  const authorDTO = new SiteAuthorDTO({
    id: newData.siteAuthor?.id,
    authorName: newData.siteAuthor?.authorName || null,
    introduce: newData.siteAuthor?.introduce || null,
    localAuthorId: newData.siteAuthor?.localAuthorId || null,
    siteId: newData.siteAuthor?.siteId || null,
    fixedAuthorName: newData.siteAuthor?.fixedAuthorName || null
  })
  try {
    const response = await siteAuthorApi.siteAuthorUpdateById(authorDTO)
    ApiUtil.msg(response)
    const index = changedRows.value.indexOf(newData)
    changedRows.value.splice(index, 1)
    refreshTable()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}
// 创建同名本地作者并绑定
async function creatSameNameLocalAuthorAndBind(relateData: SiteAuthorLocalRelateDTO) {
  const authorDTO = new SiteAuthorDTO({
    id: relateData.siteAuthor?.id,
    authorName: relateData.siteAuthor?.authorName || null,
    introduce: relateData.siteAuthor?.introduce || null
  })
  try {
    await siteAuthorApi.siteAuthorCreateAndBindSameNameLocalAuthor(authorDTO)
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}
// 处理站点作者表格选中项变化事件
function handleSelectionChange(selections: SiteAuthorLocalRelateDTO[]) {
  selectedRows.value = selections
}
// 冲突候选清单：载荷处于「候选多于一个且未显选」态时返回候选清单（首位即默认选中项），否则返回 null
function conflictCandidatesOf(payload: SiteAuthorFetchResponse): PluginCandidate[] | null {
  const conflict = payload.conflict
  if (isNullish(conflict) || !conflict.conflict) {
    return null
  }
  return conflict.candidates.filter(notNullish)
}
// 挂起本次拉取并弹出插件选择器：确认后带显选键重发，取消则丢弃（冲突态未调用任何插件，无副作用需回滚）
function holdFetchConflict(candidates: PluginCandidate[], ids: number[], nameById: Map<number, string>, single: boolean) {
  pendingFetchIds = ids
  pendingFetchNameById = nameById
  pendingFetchSingle = single
  pluginSelectCandidates.value = candidates
  pluginSelectState.value = true
}
// 选择器确认：带插件显选键重发本次拉取（批量整批同选，重发时整批仍只问这一次）
function handlePluginChosen(candidate: PluginCandidate) {
  if (arrayIsEmpty(pendingFetchIds)) {
    return
  }
  const ids = pendingFetchIds
  const nameById = pendingFetchNameById
  const single = pendingFetchSingle
  pendingFetchIds = []
  pendingFetchNameById = new Map()
  if (single) {
    void fetchSiteAuthorInfoById(ids[0], nameById, candidate.pluginPublicId)
  } else {
    void batchFetchSiteAuthorsInfo(ids, nameById, candidate.pluginPublicId)
  }
}
// 选择器取消：不重发（不算拉取失败）
function handlePluginChooseCanceled() {
  pendingFetchIds = []
  pendingFetchNameById = new Map()
}
// 行操作「拉取信息」：从来源站点拉取该作者的最新介绍与头像，拉取期间表格区挂 loading
async function fetchSiteAuthorInfo(row: SiteAuthorLocalRelateDTO) {
  const id = row.siteAuthor?.id
  if (isNullish(id)) {
    ElMessage.error('拉取作者信息失败：行数据缺少作者标识')
    return
  }
  const authorName = row.siteAuthor?.authorName
  const nameById = new Map<number, string>([[id, isBlank(authorName) ? String(id) : authorName]])
  await fetchSiteAuthorInfoById(id, nameById, '')
}
// 单作者拉取：显选键为空 = 首次触发；命中冲突转插件选择器，由用户点名后带键重发
async function fetchSiteAuthorInfoById(id: number, nameById: Map<number, string>, chosenPluginPublicId: string) {
  fetchInfoLoading.value = true
  try {
    const response = await authorInfoApi.authorInfoFetchSiteAuthorInfo(id, chosenPluginPublicId)
    const candidates = conflictCandidatesOf(response.data)
    if (notNullish(candidates)) {
      holdFetchConflict(candidates, [id], nameById, true)
      return
    }
    ElMessage.success(`已拉取「${nameById.get(id) ?? id}」的作者信息`)
    // 介绍/头像可能已更新，刷新表格与已打开的对话框数据源
    refreshTable()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    fetchInfoLoading.value = false
  }
}
// 工具栏「批量拉取信息」：对多选行逐条拉取；逐条结果反馈——汇总一条 ElMessage，
// 存在失败时另在通知中心留一条含逐条明细的终态通知供回看
async function handleBatchFetchClicked() {
  const ids = selectedRows.value
    .map((row) => row.siteAuthor?.id)
    .filter((id): id is number => notNullish(id))
  if (arrayIsEmpty(ids)) {
    ElMessage.warning('请先勾选要拉取的站点作者')
    return
  }
  // 作者标识 → 展示名索引（失败明细按行数据取名，取名不到回落 id）
  const nameById = new Map<number, string>()
  for (const row of selectedRows.value) {
    const id = row.siteAuthor?.id
    const authorName = row.siteAuthor?.authorName
    if (notNullish(id)) {
      nameById.set(id, isBlank(authorName) ? String(id) : authorName)
    }
  }
  await batchFetchSiteAuthorsInfo(ids, nameById, '')
}
// 批量拉取：整批一次触发（候选冲突为整批前置返回，问一次）；逐条结果反馈——汇总一条 ElMessage，
// 存在失败时另在通知中心留一条含逐条明细的终态通知供回看
async function batchFetchSiteAuthorsInfo(ids: number[], nameById: Map<number, string>, chosenPluginPublicId: string) {
  batchFetchLoading.value = true
  try {
    const response = await authorInfoApi.authorInfoFetchSiteAuthorsInfo(ids, chosenPluginPublicId)
    const candidates = conflictCandidatesOf(response.data)
    if (notNullish(candidates)) {
      holdFetchConflict(candidates, ids, nameById, false)
      return
    }
    const results = response.data.items?.filter(notNullish) ?? []
    const failures = results.filter((item) => !item.success)
    if (arrayIsEmpty(failures)) {
      ElMessage.success(`批量拉取完成：成功 ${results.length} 条`)
    } else {
      ElMessage.warning(`批量拉取完成：成功 ${results.length - failures.length} 条，失败 ${failures.length} 条（明细见通知中心）`)
      useNotificationStore().add({
        level: 'warning',
        category: 'authorInfo',
        title: '批量拉取作者信息',
        statusText: `${failures.length} 条失败`,
        terminal: true,
        render: () => h(
          'div',
          { style: 'display: flex; flex-direction: column; gap: 2px; font-size: 12px; color: var(--app-text-regular);' },
          failures.map((item) => h(
            'span',
            { title: item.message },
            `${nameById.get(item.siteAuthorId) ?? item.siteAuthorId}：${isBlank(item.message) ? '未知原因' : item.message}`
          ))
        )
      })
    }
    refreshTable()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    batchFetchLoading.value = false
  }
}
</script>

<template>
  <base-view>
    <template #default>
      <div
        v-loading="fetchInfoLoading || batchFetchLoading"
        :element-loading-text="batchFetchLoading ? '批量拉取作者信息中...' : '拉取作者信息中...'"
        class="tag-manage-container"
      >
        <search-table
          ref="siteAuthorSearchTable"
          v-model:page="page"
          v-model:changed-rows="changedRows"
          class="tag-manage-search-table"
          toolbar-radius="var(--app-radius)"
          data-radius="var(--app-radius)"
          data-key="siteAuthor.id"
          :operation-button="operationButton"
          :thead="siteAuthorThead"
          :search="siteAuthorQueryPageFn"
          :multi-select="true"
          :selectable="true"
          :page-sizes="[10, 20, 50, 100, 1000]"
          :operation-width="260"
          :search-button-disabled="fetchInfoLoading || batchFetchLoading"
          @row-button-clicked="handleRowButtonClicked"
          @selection-change="handleSelectionChange"
        >
          <template #toolbarMain>
            <el-button
              type="primary"
              @click="handleCreateButtonClicked"
            >
              新增
            </el-button>
            <el-button
              type="primary"
              plain
              icon="Download"
              :loading="batchFetchLoading"
              :disabled="arrayIsEmpty(selectedRows) || fetchInfoLoading"
              @click="handleBatchFetchClicked"
            >
              批量拉取信息
            </el-button>
            <el-row class="site-author-manage-search-bar">
              <el-col :span="20">
                <el-input
                  v-model="siteAuthorQuery.authorName.value"
                  placeholder="输入作者名称"
                  clearable
                  @clear="() => siteAuthorQuery.authorName.value = null"
                />
              </el-col>
              <el-col :span="4">
                <auto-load-select
                  v-model="siteAuthorQuery.siteId.value"
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
              </el-col>
            </el-row>
          </template>
        </search-table>
      </div>
    </template>
    <template #dialog>
      <site-author-dialog
        v-model:form-data="dialogData"
        v-model:state="dialogState"
        :mode="siteAuthorDialogMode"
        @request-success="refreshTable"
      />
      <!-- 插件候选选择器：拉取信息命中多个候选插件时由拉取流程唤起（批量整批问一次） -->
      <plugin-candidate-select-dialog
        v-model:state="pluginSelectState"
        :candidates="pluginSelectCandidates"
        @confirm="handlePluginChosen"
        @cancel="handlePluginChooseCanceled"
      />
    </template>
  </base-view>
</template>

<style>
.tag-manage-container {
  display: flex;
  width: calc(100% - 20px);
  height: calc(100% - 20px);
  /* 容器不带底色：一体感由 SearchTable 自身的工具栏面与数据面（含分页面）连成的卡片承担；间距纯 margin（总边距 10px 不变） */
  margin: 10px;
  flex-direction: row;
  justify-content: center;
  align-items: center;
}
.tag-manage-search-table {
  height: 100%;
  width: 100%;
}
.site-author-manage-search-bar {
  flex-grow: 1;
}
</style>
