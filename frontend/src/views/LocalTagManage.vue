<script setup lang="ts">
import {onMounted, Ref, ref} from 'vue'
import BaseSubpage from './BaseSubpage.vue'
import SearchTable from '../components/common/SearchTable.vue'
import ExchangeBox from '../components/common/ExchangeBox.vue'
import LocalTagDialog from '../components/dialogs/LocalTagDialog.vue'
import ApiUtil from '../utils/ApiUtil.ts'
import DataTableOperationResponse from '../model/util/DataTableOperationResponse.ts'
import {Thead} from '../model/util/Thead.ts'
import {
  LocalTagDTO,
  LocalTagWithBaseTagDTO,
  SelectItem,
  SiteTagFullDTO
} from "@bindings/github.com/library-squirrel/wails/pkg/model/dto"
import OperationItem from '../model/util/OperationItem.ts'
import DialogMode from '../model/util/DialogMode.ts'
import IPage from '@renderer/model/util/IPage.ts'
import {arrayNotEmpty, isNullish, notNullish} from '@renderer/utils/CommonUtil.ts'
import {ElMessage} from 'element-plus'
import AutoLoadSelect from '@renderer/components/common/AutoLoadSelect.vue'
import {siteQuerySelectItemPageBySiteName} from '@renderer/apis/SiteApi.ts'
import {localTagApi, localTagQuerySelectItemPageByName, siteTagApi} from '@renderer/apis/http'
import {LocalTagQueryDTO} from '@bindings/github.com/library-squirrel/wails/internal/localTag/models'
import {Operator, QueryAttribute, SortOrder} from '@bindings/github.com/library-squirrel/wails/pkg/query/models'
import {SiteTagQueryDTO} from '@bindings/github.com/library-squirrel/wails/internal/siteTag/models'
import {copyPage, newPage} from "@renderer/utils/Pager.ts";
import {isBlank} from "@renderer/utils/StringUtil.ts";
import {Page} from "@bindings/github.com/library-squirrel/wails/pkg/model";

// onMounted
onMounted(() => {
  // 使用各字段的 Order 属性进行排序，通过 Priority 控制优先级
  localTagQuery.value.updateTime = { value: null, order: SortOrder.OrderDesc, priority: 0 }
  localTagQuery.value.createTime = { value: null, order: SortOrder.OrderDesc, priority: 1 }
  localTagSearchTable.value.doSearch()
})

// 变量
// 接口
const apis = {
  localTagDeleteById: localTagApi.localTagDeleteById,
  localTagUpdateById: localTagApi.localTagUpdateById,
  localTagQueryWithBaseTagPage: localTagApi.localTagQueryWithBaseTagPage,
  localTagListSelectItems: localTagApi.localTagListSelectItems,
  localTagQuerySelectItemPage: localTagApi.localTagQuerySelectItemPage,
  localTagGetTree: localTagApi.localTagGetTree,
  siteTagUpdateBindLocalTag: siteTagApi.siteTagUpdateBindLocalTag,
  siteTagQueryBoundOrUnboundToLocalTagPage: siteTagApi.siteTagQueryBoundOrUnboundToLocalTagPage
}
// localTagSearchTable的组件实例
const localTagSearchTable = ref()
// siteTagExchangeBox的组件实例
const siteTagExchangeBox = ref()
// 被改变的数据行
const changedRows: Ref<LocalTagWithBaseTagDTO[]> = ref([])
// 被选中的本地标签
const localTagSelected: Ref<LocalTagDTO> = ref(new LocalTagDTO())
// 本地标签SearchTable的operationButton
const operationButton: OperationItem<LocalTagWithBaseTagDTO>[] = [
  {
    label: '保存',
    icon: 'Checked',
    buttonType: 'primary',
    code: 'save',
    rule: (row) => changedRows.value.includes(row)
  },
  { label: '查看', icon: 'view', code: DialogMode.VIEW },
  { label: '编辑', icon: 'edit', code: DialogMode.EDIT },
  { label: '删除', icon: 'delete', code: 'delete' }
]
// 本地标签SearchTable的表头
const localTagThead: Ref<Thead<LocalTagWithBaseTagDTO>[]> = ref([
  new Thead({
    type: 'text',
    defaultDisabled: true,
    dblclickToEdit: true,
    key: 'localTag.localTagName',
    title: '名称',
    hide: false,
    width: 150,
    headerAlign: 'center',
    dataAlign: 'center',
    showOverflowTooltip: true
  }),
  new Thead({
    type: 'autoLoadSelect',
    defaultDisabled: true,
    dblclickToEdit: true,
    key: 'localTag.baseLocalTagId',
    title: '上级标签',
    hide: false,
    width: 150,
    headerAlign: 'center',
    headerTagType: 'success',
    dataAlign: 'center',
    showOverflowTooltip: true,
    remote: true,
    remotePaging: true,
    remotePageMethod: localTagQuerySelectItemPageByName,
    getCacheData: (rowData: LocalTagWithBaseTagDTO) => {
      if (isNullish(rowData.baseTag?.id)) {
        return undefined
      }
      return new SelectItem({
        value: rowData.baseTag.id,
        label: isNullish(rowData.baseTag?.localTagName) ? '' : rowData.baseTag.localTagName
      })
    },
    setCacheData: (rowData: LocalTagWithBaseTagDTO, data: SelectItem) => {
      if (isNullish(rowData.baseTag)) {
        rowData.baseTag = new LocalTagDTO()
      }
      rowData.baseTag.id = Number(data.value)
      rowData.baseTag.localTagName = data.label
    }
  }),
  new Thead({
    type: 'datetime',
    defaultDisabled: true,
    dblclickToEdit: true,
    key: 'localTag.updateTime',
    title: '修改时间',
    hide: false,
    width: 200,
    headerAlign: 'center',
    headerTagType: 'success',
    dataAlign: 'center',
    showOverflowTooltip: true
  })
])
// 本地标签SearchTable的分页
const page: Ref<Page<LocalTagWithBaseTagDTO>> = ref(newPage<LocalTagWithBaseTagDTO>())
// 本地标签查询参数
const localTagQuery: Ref<LocalTagQueryDTO> = ref(new LocalTagQueryDTO())
// 本地标签弹窗的mode
const localTagDialogMode: Ref<DialogMode> = ref(DialogMode.EDIT)
// 本地标签的对话框开关
const dialogState: Ref<boolean> = ref(false)
// 本地标签对话框的数据
const dialogData: Ref<LocalTagWithBaseTagDTO> = ref(new LocalTagWithBaseTagDTO())
// 站点标签ExchangeBox的upper的查询参数
const exchangeBoxUpperSearchParams: Ref<SiteTagQueryDTO> = ref(new SiteTagQueryDTO())
// 站点标签ExchangeBox的lower的查询参数
const exchangeBoxLowerSearchParams: Ref<SiteTagQueryDTO> = ref(new SiteTagQueryDTO())
// 是否禁用ExchangeBox的搜索按钮
const disableExcSearchButton: Ref<boolean> = ref(false)

// 方法
// 分页查询本地标签的函数
async function localTagQueryPageFn(page: Page<LocalTagWithBaseTagDTO>): Promise<Page<LocalTagWithBaseTagDTO>> {
  localTagQuery.value.localTagName.operator = Operator.OpLike
  const response = await apis.localTagQueryWithBaseTagPage(page, localTagQuery.value)
  return response.data
}
// 处理本地标签新增按钮点击事件
async function handleCreateButtonClicked() {
  localTagDialogMode.value = DialogMode.NEW
  dialogData.value = new LocalTagWithBaseTagDTO({ localTag: new LocalTagDTO() })
  dialogState.value = true
}
// 处理本地标签数据行按钮点击事件
function handleRowButtonClicked(op: DataTableOperationResponse<LocalTagWithBaseTagDTO>) {
  switch (op.code) {
    case 'save':
      saveRowEdit(op.data)
      break
    case DialogMode.VIEW:
      localTagDialogMode.value = DialogMode.VIEW
      dialogData.value = op.data
      dialogState.value = true
      break
    case DialogMode.EDIT:
      localTagDialogMode.value = DialogMode.EDIT
      dialogData.value = op.data
      dialogState.value = true
      break
    case 'delete':
      deleteLocalTag(op.id)
      break
    default:
      break
  }
}
// 处理被选中的本地标签改变的事件
async function handleLocalTagSelectionChange(selections: LocalTagWithBaseTagDTO[]) {
  if (selections.length > 0) {
    disableExcSearchButton.value = false
    localTagSelected.value = selections[0].localTag ?? new LocalTagDTO()
    siteTagExchangeBox.value.refreshData()
  }
}
// 处理本地标签弹窗请求成功事件
function refreshTable() {
  localTagSearchTable.value.doSearch()
}
// 保存行数据编辑
async function saveRowEdit(newData: LocalTagWithBaseTagDTO) {
  try {
    const response = await apis.localTagUpdateById(newData.localTag ?? new LocalTagDTO())
    ApiUtil.msg(response)
    const index = changedRows.value.indexOf(newData)
    changedRows.value.splice(index, 1)
    refreshTable()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}
// 删除本地标签
async function deleteLocalTag(id: string) {
  try {
    const response = await apis.localTagDeleteById(Number(id))
    ApiUtil.msg(response)
    await localTagSearchTable.value.doSearch()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}
// 处理站点标签ExchangeBox确认交换的事件
async function handleExchangeBoxConfirm(isUpper: boolean | undefined, upper: SelectItem[], lower: SelectItem[]) {
  if (isNullish(localTagSelected.value)) {
    ElMessage({
      message: '确认修改时必须选中一个本地标签',
      type: 'warning'
    })
    return
  }

  if (isNullish(isUpper) ? true : isUpper && arrayNotEmpty(upper)) {
    const boundIds = upper.map((item) => Number(item.value))
    await apis.siteTagUpdateBindLocalTag(localTagSelected.value.id, boundIds)
  }
  if (isNullish(isUpper) ? true : !isUpper && arrayNotEmpty(lower)) {
    const unBoundIds = lower.map((item) => Number(item.value))
    await apis.siteTagUpdateBindLocalTag(null, unBoundIds)
  }
  siteTagExchangeBox.value.refreshData(isUpper)
}
// 请求站点标签分页选择列表的函数
async function requestSiteTagSelectItemPage(
  page: IPage<SelectItem>,
  bounded: boolean
): Promise<IPage<SelectItem>> {
  const queryPage = copyPage<SiteTagFullDTO>(page)
  // 固定参数
  exchangeBoxLowerSearchParams.value.localTagId = new QueryAttribute({ value: localTagSelected.value.id })
  exchangeBoxLowerSearchParams.value.boundOnLocalTagId = new QueryAttribute({ value: bounded })
  // 用户输入的参数
  exchangeBoxLowerSearchParams.value.siteTagName.operator = Operator.OpLike
  const response = await apis.siteTagQueryBoundOrUnboundToLocalTagPage(queryPage, exchangeBoxLowerSearchParams.value)
  const responsePage = response.data
  const result = copyPage<SelectItem>(responsePage)
  result.data = responsePage.data.filter(notNullish).map(data => {
    const siteTagName = data.siteTag?.siteTagName
    const baseSiteTagId = data.siteTag?.baseSiteTagId
    const siteName = data.site?.siteName
    return new SelectItem({
      value: String(data.siteTag?.id),
      label: isBlank(siteTagName) ? '?' : siteTagName,
      rootId: isBlank(baseSiteTagId) ? '?' : baseSiteTagId,
      subLabels: [isBlank(siteName) ? '?' : siteName],
      extraData: undefined
    })
  })
  return result
}
</script>

<template>
  <base-subpage>
    <template #default>
      <div class="tag-manage-container">
        <div class="tag-manage-left">
          <search-table
            ref="localTagSearchTable"
            v-model:page="page"
            v-model:toolbar-params="localTagQuery"
            v-model:changed-rows="changedRows"
            class="tag-manage-left-search-table"
            data-key="localTag.id"
            :operation-button="operationButton"
            :thead="localTagThead"
            :search="localTagQueryPageFn"
            :multi-select="false"
            :selectable="true"
            :page-sizes="[10, 20, 50, 100, 1000]"
            @row-button-clicked="handleRowButtonClicked"
            @selection-change="handleLocalTagSelectionChange"
          >
            <template #toolbarMain>
              <el-button type="primary" @click="handleCreateButtonClicked">新增</el-button>
              <el-row class="local-tag-manage-search-bar">
                <el-col :span="16">
                  <el-input v-model="localTagQuery.localTagName.value" placeholder="输入标签名称" clearable @clear="() => localTagQuery.localTagName.value = null" />
                </el-col>
                <el-col :span="8">
                  <auto-load-select
                    v-model:data="localTagQuery.baseLocalTagId.value"
                    :load="localTagQuerySelectItemPageByName"
                    placeholder="选择上级标签"
                    remote
                    filterable
                    clearable
                  >
                    <template #default="{ list }">
                      <el-option v-for="item in list" :key="item.value" :value="item.value" :label="item.label" />
                    </template>
                  </auto-load-select>
                </el-col>
              </el-row>
            </template>
          </search-table>
        </div>
        <div class="tag-manage-right">
          <exchange-box
            ref="siteTagExchangeBox"
            v-model:upper-search-params="exchangeBoxUpperSearchParams"
            v-model:lower-search-params="exchangeBoxLowerSearchParams"
            :upper-load="(temp: IPage<SelectItem>) => requestSiteTagSelectItemPage(temp, true)"
            :lower-load="(temp: IPage<SelectItem>) => requestSiteTagSelectItemPage(temp, false)"
            :search-button-disabled="disableExcSearchButton"
            tags-gap="10px"
            @upper-confirm="(upper, lower) => handleExchangeBoxConfirm(true, upper, lower)"
            @lower-confirm="(upper, lower) => handleExchangeBoxConfirm(false, upper, lower)"
            @all-confirm="(upper, lower) => handleExchangeBoxConfirm(undefined, upper, lower)"
          >
            <template #upperToolbarMain>
              <el-row class="local-tag-manage-search-bar">
                <el-col :span="18">
                  <el-input v-model="exchangeBoxUpperSearchParams.siteTagName.value" placeholder="输入站点标签名称" clearable />
                </el-col>
                <el-col :span="6">
                  <auto-load-select
                    v-model:data="exchangeBoxUpperSearchParams.siteId.value"
                    :load="siteQuerySelectItemPageBySiteName"
                    placeholder="选择站点"
                    remote
                    filterable
                    clearable
                  >
                    <template #default="{ list }">
                      <el-option v-for="item in list" :key="item.value" :value="item.value" :label="item.label" />
                    </template>
                  </auto-load-select>
                </el-col>
              </el-row>
            </template>
            <template #lowerToolbarMain>
              <el-row class="local-tag-manage-search-bar">
                <el-col :span="18">
                  <el-input v-model="exchangeBoxLowerSearchParams.siteTagName.value" placeholder="输入站点标签名称" clearable />
                </el-col>
                <el-col :span="6">
                  <auto-load-select
                    v-model:data="exchangeBoxLowerSearchParams.siteId.value"
                    :load="siteQuerySelectItemPageBySiteName"
                    placeholder="选择站点"
                    remote
                    filterable
                    clearable
                  >
                    <template #default="{ list }">
                      <el-option v-for="item in list" :key="item.value" :value="item.value" :label="item.label" />
                    </template>
                  </auto-load-select>
                </el-col>
              </el-row>
            </template>
            <template #upperTitle>
              <div class="local-tag-manage-site-author-title">
                <span class="local-tag-manage-site-author-title-text">已绑定站点标签</span>
              </div>
            </template>
            <template #lowerTitle>
              <div class="local-tag-manage-site-author-title">
                <span class="local-tag-manage-site-author-title-text">未绑定站点标签</span>
              </div>
            </template>
          </exchange-box>
        </div>
      </div>
    </template>
    <template #dialog>
      <local-tag-dialog
        v-model:form-data="dialogData"
        v-model:state="dialogState"
        :mode="localTagDialogMode"
        @request-success="refreshTable"
      />
    </template>
  </base-subpage>
</template>

<style>
.tag-manage-container {
  display: flex;
  flex-direction: row;
  justify-content: center;
  align-items: center;
  background: #f4f4f4;
  border-radius: 6px;
  width: calc(100% - 20px);
  height: calc(100% - 20px);
  padding: 5px;
  margin: 5px;
}

.tag-manage-left {
  width: calc(50% - 5px);
  height: 100%;
  margin-right: 5px;
}
.tag-manage-left-search-table {
  height: 100%;
  width: 100%;
}
.tag-manage-right {
  width: calc(50% - 5px);
  height: 100%;
  margin-left: 5px;
}
.local-tag-manage-site-author-title {
  display: flex;
  width: 100%;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--el-border-color);
  border-radius: 5px;
  background-color: var(--el-fill-color-blank);
}
.local-tag-manage-site-author-title-text {
  text-align: center;
  writing-mode: vertical-lr;
  color: var(--el-text-color-regular);
}
.local-tag-manage-search-bar {
  flex-grow: 1;
}
</style>
