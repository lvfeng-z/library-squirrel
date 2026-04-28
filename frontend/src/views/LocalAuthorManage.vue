<script setup lang="ts">
import {nextTick, onMounted, Ref, ref} from 'vue'
import BaseSubpage from './BaseSubpage.vue'
import SearchTable from '../components/common/SearchTable.vue'
import ExchangeBox from '../components/common/ExchangeBox.vue'
import LocalAuthorDialog from '../components/dialogs/LocalAuthorDialog.vue'
import lodash from 'lodash'
import ApiUtil from '../utils/ApiUtil.ts'
import ApiResponse from '../model/util/ApiResponse.ts'
import DataTableOperationResponse from '../model/util/DataTableOperationResponse.ts'
import {Thead} from '../model/util/Thead.ts'
import {LocalAuthorDTO, SelectItem, SiteAuthorFullDTO} from "@bindings/github.com/library-squirrel/wails/pkg/model/dto"
import OperationItem from '../model/util/OperationItem.ts'
import DialogMode from '../model/util/DialogMode.ts'
import {Page} from "@bindings/github.com/library-squirrel/wails/pkg/model";
import {arrayNotEmpty, isNullish, notNullish} from '@renderer/utils/CommonUtil.ts'
import {ElMessage} from 'element-plus'
import IPage from '@renderer/model/util/IPage.ts'
import AutoLoadSelect from '@renderer/components/common/AutoLoadSelect.vue'
import {siteQuerySelectItemPageBySiteName} from '@renderer/apis/SiteApi.ts'
import {LocalAuthorQueryDTO} from '@bindings/github.com/library-squirrel/wails/internal/localAuthor/models'
import {Operator, QueryAttribute, SortOrder} from '@bindings/github.com/library-squirrel/wails/pkg/query/models'
import {SiteAuthorQueryDTO} from '@bindings/github.com/library-squirrel/wails/internal/siteAuthor/models'
import {localAuthorApi, siteApi, siteAuthorApi} from '@renderer/apis/http'
import {copyPage} from "@renderer/utils/Pager.ts";
import {isBlank} from "@renderer/utils/StringUtil.ts";

// onMounted
onMounted(() => {
  // 使用各字段的 Order 属性进行排序，通过 Priority 控制优先级
  localAuthorQuery.value.updateTime = { value: null, order: SortOrder.OrderDesc, priority: 0 }
  localAuthorQuery.value.createTime = { value: null, order: SortOrder.OrderDesc, priority: 1 }
  localAuthorSearchTable.value.doSearch()
})

// 变量
// 接口
const apis = {
  localAuthorDeleteById: localAuthorApi.localAuthorDeleteById,
  localAuthorUpdateById: localAuthorApi.localAuthorUpdateById,
  localAuthorQueryPage: localAuthorApi.localAuthorQueryPage,
  siteAuthorUpdateBindLocalAuthor: siteAuthorApi.siteAuthorUpdateBindLocalAuthor,
  siteQuerySelectItemPage: siteApi.siteQuerySelectItemPage,
  siteAuthorQueryBoundOrUnboundInLocalAuthorPage: siteAuthorApi.siteAuthorQueryBoundOrUnboundInLocalAuthorPage
}
// localAuthorSearchTable的组件实例
const localAuthorSearchTable = ref()
// siteAuthorExchangeBox的组件实例
const siteAuthorExchangeBox = ref()
// 本地作者SearchTable的分页
const page: Ref<Page<LocalAuthorDTO>> = ref(new Page<LocalAuthorDTO>())
// 本地作者查询参数
const localAuthorQuery: Ref<LocalAuthorQueryDTO> = ref(new LocalAuthorQueryDTO())
// 被改变的数据行
const changedRows: Ref<LocalAuthorDTO[]> = ref([])
// 被选中的本地作者
const localAuthorSelected: Ref<LocalAuthorDTO> = ref(new LocalAuthorDTO())
// 本地作者SearchTable的operationButton
const operationButton: OperationItem<LocalAuthorDTO>[] = [
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
// 本地作者SearchTable的表头
const localAuthorThead: Ref<Thead<LocalAuthorDTO>[]> = ref([
  new Thead({
    type: 'text',
    defaultDisabled: true,
    dblclickToEdit: true,
    key: 'authorName',
    title: '名称',
    hide: false,
    width: 150,
    headerAlign: 'center',
    dataAlign: 'center',
    showOverflowTooltip: true
  }),
  new Thead({
    type: 'text',
    defaultDisabled: true,
    dblclickToEdit: true,
    key: 'introduce',
    title: '介绍',
    hide: false,
    width: 150,
    headerAlign: 'center',
    dataAlign: 'center',
    showOverflowTooltip: true
  }),
  new Thead({
    type: 'datetime',
    defaultDisabled: true,
    key: 'updateTime',
    title: '修改时间',
    hide: false,
    width: 200,
    headerAlign: 'center',
    headerTagType: 'success',
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
    headerTagType: 'success',
    dataAlign: 'center',
    showOverflowTooltip: true
  })
])
// 本地作者弹窗的mode
const localAuthorDialogMode: Ref<DialogMode> = ref(DialogMode.EDIT)
// 本地作者的对话框开关
const dialogState: Ref<boolean> = ref(false)
// 本地作者对话框的数据
const dialogData: Ref<LocalAuthorDTO> = ref(new LocalAuthorDTO())
// 站点作者ExchangeBox的upper的查询参数
const exchangeBoxUpperSearchParams: Ref<SiteAuthorQueryDTO> = ref(new SiteAuthorQueryDTO())
// 站点作者ExchangeBox的lower的查询参数
const exchangeBoxLowerSearchParams: Ref<SiteAuthorQueryDTO> = ref(new SiteAuthorQueryDTO())
// 是否禁用ExchangeBox的搜索按钮
const disableExcSearchButton: Ref<boolean> = ref(false)

// 方法
// 分页查询本地作者的函数
async function localAuthorQueryPageFn(
  page: Page<LocalAuthorDTO>
): Promise<Page<LocalAuthorDTO>> {
  const response = await apis.localAuthorQueryPage(page, localAuthorQuery.value)
  if (ApiUtil.check(response)) {
    const result = ApiUtil.data<Page<LocalAuthorDTO>>(response)
    if (isNullish(result)) {
      throw new Error('localAuthorQueryPage未返回数据')
    }
    return result
  } else {
    ApiUtil.msg(response)
    throw new Error(response.msg)
  }
}
// 处理本地作者新增按钮点击事件
async function handleCreateButtonClicked() {
  localAuthorDialogMode.value = DialogMode.NEW
  dialogData.value = new LocalAuthorDTO()
  dialogState.value = true
}
// 处理本地作者数据行按钮点击事件
function handleRowButtonClicked(op: DataTableOperationResponse<LocalAuthorDTO>) {
  switch (op.code) {
    case 'save':
      saveRowEdit(op.data)
      break
    case DialogMode.VIEW:
      localAuthorDialogMode.value = DialogMode.VIEW
      dialogData.value = op.data
      dialogState.value = true
      break
    case DialogMode.EDIT:
      localAuthorDialogMode.value = DialogMode.EDIT
      dialogData.value = op.data
      dialogState.value = true
      break
    case 'delete':
      deleteLocalAuthor(Number(op.id))
      break
    default:
      break
  }
}
// 处理被选中的本地作者改变的事件
async function handleLocalAuthorSelectionChange(selections: LocalAuthorDTO[]) {
  if (selections.length > 0) {
    disableExcSearchButton.value = false
    localAuthorSelected.value = selections[0]
    // 不等待DOM更新完成会导致ExchangeBox总是使用更新之前的值查询
    await nextTick()
    siteAuthorExchangeBox.value.refreshData()
  }
}
// 处理本地作者弹窗请求成功事件
function refreshTable() {
  localAuthorSearchTable.value.doSearch()
}
// 保存行数据编辑
async function saveRowEdit(newData: LocalAuthorDTO) {
  const tempData = lodash.cloneDeep(newData)

  const response = await apis.localAuthorUpdateById(tempData)
  ApiUtil.msg(response)
  if (ApiUtil.check(response)) {
    const index = changedRows.value.indexOf(newData)
    changedRows.value.splice(index, 1)
    refreshTable()
  }
}
// 删除本地作者
async function deleteLocalAuthor(id: number) {
  const response = await apis.localAuthorDeleteById(id)
  ApiUtil.msg(response)
  if (ApiUtil.check(response)) {
    await localAuthorSearchTable.value.doSearch()
  }
}
// 处理站点作者ExchangeBox确认交换的事件
async function handleExchangeBoxConfirm(isUpper: boolean | undefined, upper: SelectItem[], lower: SelectItem[]) {
  if (isNullish(localAuthorSelected.value)) {
    ElMessage({
      message: '确认修改时必须选中一个本地作者',
      type: 'warning'
    })
    return
  }

  if (isNullish(isUpper) ? true : isUpper) {
    let upperResponse: ApiResponse
    if (arrayNotEmpty(upper)) {
      const boundIds = upper.map((item) => Number(item.value))
      upperResponse = await apis.siteAuthorUpdateBindLocalAuthor(localAuthorSelected.value.id, boundIds)
    } else {
      upperResponse = { success: true, msg: '', data: undefined }
    }
    ApiUtil.failedMsg(upperResponse)
  }
  if (isNullish(isUpper) ? true : !isUpper) {
    let lowerResponse: ApiResponse
    if (arrayNotEmpty(lower)) {
      const unBoundIds = lower.map((item) => Number(item.value))
      lowerResponse = await apis.siteAuthorUpdateBindLocalAuthor(null, unBoundIds)
    } else {
      lowerResponse = { success: true, msg: '', data: undefined }
    }
    ApiUtil.failedMsg(lowerResponse)
  }
  siteAuthorExchangeBox.value.refreshData(isUpper)
}
// 请求站点作者分页选择列表的函数
async function requestSiteAuthorSelectItemPage(page: IPage<SelectItem>, bounded: boolean): Promise<Page<SelectItem>> {
  const query = bounded ? exchangeBoxUpperSearchParams.value : exchangeBoxLowerSearchParams.value
  query.authorName.operator = Operator.OpLike
  query.localAuthorId = new QueryAttribute({value: localAuthorSelected.value.id})
  query.boundOnLocalAuthorId = new QueryAttribute({value: bounded})
  const tempPage = copyPage<SiteAuthorFullDTO>(page)
  const response = await apis.siteAuthorQueryBoundOrUnboundInLocalAuthorPage(tempPage, query)
  if (ApiUtil.check(response)) {
    const responsePage: Page<SiteAuthorFullDTO> | undefined = ApiUtil.data(response)
    if (isNullish(responsePage)) {
      throw new Error('siteAuthorQueryBoundOrUnboundInLocalAuthorPage未返回数据')
    }
    const resultPage = copyPage<SelectItem>(responsePage)
    resultPage.data = responsePage.data.filter(notNullish).map(data => {
      const authorName = data.authorName
      const siteName = data.site?.siteName
      return new SelectItem({
        value: String(data.id),
        label: isBlank(authorName) ? '?' : authorName,
        subLabels: [isBlank(siteName) ? '?' : siteName],
        extraData: undefined
      })
    })
    return resultPage
  } else {
    throw new Error(response.msg)
  }
}
</script>

<template>
  <base-subpage>
    <template #default>
      <div class="local-author-manage-container">
        <div class="local-author-manage-left">
          <search-table
            ref="localAuthorSearchTable"
            v-model:page="page"
            v-model:toolbar-params="localAuthorQuery"
            v-model:changed-rows="changedRows"
            class="local-author-manage-left-search-table"
            data-key="id"
            :operation-button="operationButton"
            :thead="localAuthorThead"
            :search="localAuthorQueryPageFn"
            :multi-select="false"
            :selectable="true"
            @row-button-clicked="handleRowButtonClicked"
            @selection-change="handleLocalAuthorSelectionChange"
          >
            <template #toolbarMain>
              <el-button type="primary" @click="handleCreateButtonClicked">新增</el-button>
              <el-input v-model="localAuthorQuery.authorName.value" placeholder="输入作者名称" clearable @clear="() => localAuthorQuery.authorName.value = null" />
            </template>
          </search-table>
        </div>
        <div class="local-author-manage-right">
          <exchange-box
            ref="siteAuthorExchangeBox"
            v-model:upper-search-params="exchangeBoxUpperSearchParams"
            v-model:lower-search-params="exchangeBoxLowerSearchParams"
            :upper-load="(_page) => requestSiteAuthorSelectItemPage(_page, true)"
            :lower-load="(_page) => requestSiteAuthorSelectItemPage(_page, false)"
            :search-button-disabled="disableExcSearchButton"
            tags-gap="10px"
            @upper-confirm="(upper, lower) => handleExchangeBoxConfirm(true, upper, lower)"
            @lower-confirm="(upper, lower) => handleExchangeBoxConfirm(false, upper, lower)"
            @all-confirm="(upper, lower) => handleExchangeBoxConfirm(undefined, upper, lower)"
          >
            <template #upperToolbarMain>
              <el-row class="local-author-manage-search-bar">
                <el-col :span="18">
                  <el-input v-model="exchangeBoxUpperSearchParams.authorName.value" placeholder="输入站点作者名称" clearable @clear="() => exchangeBoxUpperSearchParams.authorName.value = null" />
                </el-col>
                <el-col :span="6">
                  <auto-load-select
                    v-model="exchangeBoxUpperSearchParams.siteId.value"
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
              <el-row class="local-author-manage-search-bar">
                <el-col :span="18">
                  <el-input v-model="exchangeBoxLowerSearchParams.authorName.value" placeholder="输入站点作者名称" clearable @clear="() => exchangeBoxLowerSearchParams.authorName.value = null" />
                </el-col>
                <el-col :span="6">
                  <auto-load-select
                    v-model="exchangeBoxLowerSearchParams.siteId.value"
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
              <div class="local-author-manage-site-author-title">
                <span class="local-author-manage-site-author-title-text">已绑定站点作者</span>
              </div>
            </template>
            <template #lowerTitle>
              <div class="local-author-manage-site-author-title">
                <span class="local-author-manage-site-author-title-text">未绑定站点作者</span>
              </div>
            </template>
          </exchange-box>
        </div>
      </div>
    </template>
    <template #dialog>
      <local-author-dialog
        v-model:form-data="dialogData"
        v-model:state="dialogState"
        :mode="localAuthorDialogMode"
        @request-success="refreshTable"
      />
    </template>
  </base-subpage>
</template>

<style>
.local-author-manage-container {
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

.local-author-manage-left {
  width: calc(50% - 5px);
  height: 100%;
  margin-right: 5px;
}
.local-author-manage-left-search-table {
  height: 100%;
  width: 100%;
}
.local-author-manage-right {
  width: calc(50% - 5px);
  height: 100%;
  margin-left: 5px;
}
.local-author-manage-site-author-title {
  display: flex;
  width: 100%;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--el-border-color);
  border-radius: 5px;
  background-color: var(--el-fill-color-blank);
}
.local-author-manage-site-author-title-text {
  text-align: center;
  writing-mode: vertical-lr;
  color: var(--el-text-color-regular);
}
.local-author-manage-search-bar {
  flex-grow: 1;
}
</style>
