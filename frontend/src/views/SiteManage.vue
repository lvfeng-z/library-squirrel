<script setup lang="ts">
import BaseSubpage from '@renderer/views/BaseSubpage.vue'
import SearchTable from '@renderer/components/common/SearchTable.vue'
import { onMounted, ref, Ref, UnwrapRef } from 'vue'
import OperationItem from '@renderer/model/util/OperationItem.ts'
import DialogMode from '@renderer/model/util/DialogMode.ts'
import { Thead } from '@renderer/model/util/Thead.ts'
import ApiUtil from '@renderer/utils/ApiUtil.ts'
import {ElMessage} from 'element-plus'
import DataTableOperationResponse from '@renderer/model/util/DataTableOperationResponse.ts'
import lodash from 'lodash'
import SiteDialog from '@renderer/components/dialogs/SiteDialog.vue'
import { SiteQueryDTO } from '@bindings/github.com/library-squirrel/backend/site/models'
import { SortOrder } from '@bindings/github.com/library-squirrel/backend/base/query/models'
import { siteApi } from '@renderer/apis/http'
import {SiteDTO} from "@bindings/github.com/library-squirrel/backend/base/model/dto"
import {Page} from "@bindings/github.com/library-squirrel/backend/base/model"

// onMounted
onMounted(() => {
  // 使用各字段的 Order 属性进行排序，通过 Priority 控制优先级
  siteQuery.value.updateTime = { value: null, order: SortOrder.OrderDesc, priority: 0 }
  siteQuery.value.createTime = { value: null, order: SortOrder.OrderDesc, priority: 1 }
  siteSearchTable.value.doSearch()
})

// 变量
// 站点数据表组件的实例
const siteSearchTable = ref()
// 是否调转站点和域名
const reversed: Ref<boolean> = ref(false)
// 站点分页参数
const sitePage: Ref<Page<SiteDTO>> = ref(new Page<SiteDTO>())
// 站点查询参数
const siteQuery: Ref<SiteQueryDTO> = ref(new SiteQueryDTO())
// 站点被修改的行
const siteChangedRows: Ref<SiteDTO[]> = ref([])
// 站点操作栏按钮
const siteOperationButton: OperationItem<SiteDTO>[] = [
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
const siteThead: Ref<Thead<SiteDTO>[]> = ref([
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
const siteDialogMode: Ref<DialogMode> = ref(DialogMode.EDIT)
// 站点对话框开关
const siteDialogState: Ref<boolean> = ref(false)
// 站点对话框的数据
const siteDialogData: Ref<SiteDTO> = ref(new SiteDTO())
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
async function siteQueryPageFn(page: Page<SiteDTO>): Promise<Page<SiteDTO>> {
  const response = await siteApi.siteQueryPage(page, siteQuery.value)
  return response.data
}
// 处理站点新增按钮点击事件
async function handleSiteCreateButtonClicked() {
  siteDialogMode.value = DialogMode.NEW
  siteDialogData.value = new SiteDTO()
  siteDialogState.value = true
}
// 处理站点数据行按钮点击事件
function handleSiteRowButtonClicked(op: DataTableOperationResponse<SiteDTO>) {
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
      deleteSite(Number(op.id))
      break
    default:
      break
  }
}
// 保存站点行数据编辑
async function saveSiteRowEdit(newData: SiteDTO) {
  const tempData = lodash.cloneDeep(newData)
  try {
    const response = await siteApi.siteUpdateById(tempData)
    ApiUtil.msg(response)
    const index = siteChangedRows.value.indexOf(newData)
    siteChangedRows.value.splice(index, 1)
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}
// 删除站点
async function deleteSite(id: number) {
  try {
    const response = await siteApi.siteDeleteById(id)
    ApiUtil.msg(response)
    await siteSearchTable.value.doSearch()
  } catch (e) {
    ElMessage.error((e as Error).message)
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
          :search="siteQueryPageFn"
          :selectable="true"
          :multi-select="reversed"
          :page-sizes="[10, 20, 50, 100]"
          @row-button-clicked="handleSiteRowButtonClicked"
        >
          <template #toolbarMain>
            <el-button type="primary" @click="handleSiteCreateButtonClicked">新增</el-button>
            <el-input v-model="siteSearchParams.siteName.value" placeholder="输入站点名称" clearable @clear="() => siteSearchParams.siteName.value = null" />
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
