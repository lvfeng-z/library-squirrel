<script setup lang="ts">
import BaseSubpage from '@renderer/views/BaseSubpage.vue'
import SearchTable from '@renderer/components/common/SearchTable.vue'
import { onMounted, ref, Ref, UnwrapRef } from 'vue'
import Page from '@renderer/model/util/Page.ts'
import OperationItem from '@renderer/model/util/OperationItem.ts'
import DialogMode from '@renderer/model/util/DialogMode.ts'
import { Thead } from '@renderer/model/util/Thead.ts'
import ApiUtil from '@renderer/utils/ApiUtil.ts'
import DataTableOperationResponse from '@renderer/model/util/DataTableOperationResponse.ts'
import lodash from 'lodash'
import { isNullish } from '@renderer/utils/CommonUtil.ts'
import SiteDialog from '@renderer/components/dialogs/SiteDialog.vue'
import SiteQueryDTO from '@renderer/model/model/queryDTO/SiteQueryDTO.ts'
import Site from '@renderer/model/model/entity/Site.ts'
import { siteApi } from '@renderer/apis/http'

// onMounted
onMounted(() => {
  if (isNullish(sitePage.value.query)) {
    sitePage.value.query = new SiteQueryDTO()
  }
  sitePage.value.query.sort = [
    { key: 'updateTime', asc: false },
    { key: 'createTime', asc: false }
  ]
  siteSearchTable.value.doSearch()
})

// 变量
const apis = {
  siteDeleteById: siteApi.siteDeleteById,
  siteQueryPage: siteApi.siteQueryPage,
  siteUpdateById: siteApi.siteUpdateById
}
// 站点数据表组件的实例
const siteSearchTable = ref()
// 是否调转站点和域名
const reversed: Ref<boolean> = ref(false)
// 站点分页参数
const sitePage: Ref<Page<SiteQueryDTO, Site>> = ref(new Page<SiteQueryDTO, Site>())
// 站点被修改的行
const siteChangedRows: Ref<Site[]> = ref([])
// 站点操作栏按钮
const siteOperationButton: OperationItem<Site>[] = [
  {
    label: '保存',
    icon: 'Checked',
    buttonType: 'primary',
    code: 'save',
    rule: (row) => siteChangedRows.value.includes(row)
  },
  { label: '查看', icon: 'view', code: DialogMode.VIEW },
  { label: '编辑', icon: 'edit', code: DialogMode.EDIT },
  { label: '删除', icon: 'delete', code: 'delete' }
]
// 站点的表头
const siteThead: Ref<Thead<Site>[]> = ref([
  new Thead({
    type: 'text',
    defaultDisabled: true,
    dblclickToEdit: true,
    key: 'siteName',
    title: '名称',
    hide: false,
    width: 100,
    headerAlign: 'center',
    dataAlign: 'center',
    showOverflowTooltip: true
  }),
  new Thead({
    type: 'textarea',
    defaultDisabled: true,
    dblclickToEdit: true,
    key: 'siteDescription',
    title: '描述',
    hide: false,
    headerAlign: 'center',
    headerTagType: 'success',
    dataAlign: 'center',
    showOverflowTooltip: true
  }),
  new Thead({
    type: 'datetime',
    defaultDisabled: true,
    dblclickToEdit: true,
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
    dblclickToEdit: true,
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
// 站点的查询参数
const siteSearchParams: Ref<SiteQueryDTO> = ref(new SiteQueryDTO())
// 站点弹窗的模式
const siteDialogMode: Ref<UnwrapRef<DialogMode>> = ref(DialogMode.EDIT)
// 站点对话框开关
const siteDialogState: Ref<boolean> = ref(false)
// 站点对话框的数据
const siteDialogData: Ref<UnwrapRef<Site>> = ref(new Site())
// // 被选中的站点
// const siteSelected: Ref<Site | undefined> = computed(() => {
//   if (IsNullish(siteSearchTable.value)) {
//     return undefined
//   }
//   const temp = siteSearchTable.value.getSelectionRows()
//   if (ArrayNotEmpty(temp)) {
//     return temp[0] as Site
//   } else {
//     return undefined
//   }
// })

// 方法
// 分页查询站点
async function siteQueryPage(page: Page<SiteQueryDTO, Site>): Promise<Page<SiteQueryDTO, Site> | undefined> {
  if (isNullish(page.query)) {
    page.query = new SiteQueryDTO()
  }
  const response = await apis.siteQueryPage(page)
  if (ApiUtil.check(response)) {
    return ApiUtil.data<Page<SiteQueryDTO, Site>>(response)
  } else {
    ApiUtil.msg(response)
    return undefined
  }
}
// 处理站点新增按钮点击事件
async function handleSiteCreateButtonClicked() {
  siteDialogMode.value = DialogMode.NEW
  siteDialogData.value = new Site()
  siteDialogState.value = true
}
// 处理站点数据行按钮点击事件
function handleSiteRowButtonClicked(op: DataTableOperationResponse<Site>) {
  switch (op.code) {
    case 'save':
      saveSiteRowEdit(op.data)
      break
    case DialogMode.VIEW:
      siteDialogMode.value = DialogMode.VIEW
      siteDialogData.value = op.data
      siteDialogState.value = true
      break
    case DialogMode.EDIT:
      siteDialogMode.value = DialogMode.EDIT
      siteDialogData.value = op.data
      siteDialogState.value = true
      break
    case 'delete':
      deleteSite(op.id)
      break
    default:
      break
  }
}
// 保存站点行数据编辑
async function saveSiteRowEdit(newData: Site) {
  const tempData = lodash.cloneDeep(newData)

  const response = await apis.siteUpdateById(tempData)
  ApiUtil.msg(response)
  if (ApiUtil.check(response)) {
    const index = siteChangedRows.value.indexOf(newData)
    siteChangedRows.value.splice(index, 1)
  }
}
// 删除站点
async function deleteSite(id: string) {
  const response = await apis.siteDeleteById(id)
  ApiUtil.msg(response)
  if (ApiUtil.check(response)) {
    await siteSearchTable.value.doSearch()
  }
}
// 处理站点弹窗请求成功事件
function handleSiteDialogRequestSuccess() {
  siteSearchTable.value.doSearch()
}
</script>
<template>
  <base-subpage>
    <template #default>
      <div class="site-manage-container">
        <search-table
          ref="siteSearchTable"
          v-model:page="sitePage"
          v-model:toolbar-params="siteSearchParams"
          v-model:changed-rows="siteChangedRows"
          class="site-manage-search-table"
          data-key="id"
          :operation-button="siteOperationButton"
          :operation-width="140"
          :thead="siteThead"
          :search="siteQueryPage"
          :selectable="true"
          :multi-select="reversed"
          :page-sizes="[10, 20, 50, 100]"
          @row-button-clicked="handleSiteRowButtonClicked"
        >
          <template #toolbarMain>
            <el-button type="primary" @click="handleSiteCreateButtonClicked">新增</el-button>
            <el-input v-model="siteSearchParams.siteName" placeholder="输入站点名称" clearable />
          </template>
        </search-table>
      </div>
    </template>
    <template #dialog>
      <site-dialog
        v-model:form-data="siteDialogData"
        v-model:state="siteDialogState"
        :mode="siteDialogMode"
        @request-success="handleSiteDialogRequestSuccess"
      />
    </template>
  </base-subpage>
</template>

<style scoped>
.site-manage-container {
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

.site-manage-search-table {
  height: 100%;
  width: 100%;
}
</style>
