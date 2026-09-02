<script setup lang="ts">
import BaseView from '@renderer/views/BaseView.vue'
import SearchTable from '@renderer/components/common/SearchTable.vue'
import { h, onMounted, ref, Ref } from 'vue'
import OperationItem from '@renderer/model/util/OperationItem.ts'
import DialogMode from '@renderer/model/util/DialogMode.ts'
import { Thead } from '@renderer/model/util/Thead.ts'
import ApiUtil from '@renderer/utils/ApiUtil.ts'
import {ElLink, ElMessage} from 'element-plus'
import { Browser } from '@wailsio/runtime'
import DataTableOperationResponse from '@renderer/model/util/DataTableOperationResponse.ts'
import lodash from 'lodash'
import SiteDialog from '@renderer/components/dialogs/SiteDialog.vue'
import { SiteQueryDTO } from '@bindings/github.com/library-squirrel/backend/site/models'
import { SortOrder } from '@bindings/github.com/library-squirrel/backend/base/query/models'
import { siteApi } from '@renderer/apis/http'
import {SiteDTO} from "@bindings/github.com//lvfeng-z/library-squirrel-sdk/dto"
import {Page} from "@bindings/github.com/library-squirrel/backend/base/model"
import { isBlank } from '@renderer/utils/StringUtil.ts'

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
  { label: '编辑', icon: 'edit', code: DialogMode.EDIT }
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
    type: 'text',
    defaultDisabled: true,
    key: 'siteKey',
    title: '站点键',
    hide: false,
    width: 100,
    headerAlign: 'center',
    dataAlign: 'center',
    showOverflowTooltip: true
  }),
  new Thead({
    type: 'custom',
    defaultDisabled: true,
    key: 'homepage',
    title: '首页',
    hide: false,
    headerAlign: 'center',
    dataAlign: 'center',
    showOverflowTooltip: true,
    render: (data) => renderHomepage(data as string | null | undefined)
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
// 分页查询站点（工具栏名称/键搜索条件随每次查询合并进查询参数，均为精确匹配）
async function siteQueryPageFn(page: Page<SiteDTO>): Promise<Page<SiteDTO>> {
  siteQuery.value.siteName = siteSearchParams.value.siteName
  siteQuery.value.siteKey = siteSearchParams.value.siteKey
  const response = await siteApi.siteQueryPage(page, siteQuery.value)
  return response.data
}
// 首页列渲染：空值（local 虚拟站点无首页）显示占位符不出链接；仅 http(s) 前缀的地址放行跳转，
// 点击经 Browser.OpenURL 调系统浏览器打开（WebView 内 window.open 对外部 URL 不可靠）
function renderHomepage(url: string | null | undefined) {
  if (isBlank(url)) {
    return h('span', '-')
  }
  if (!/^https?:\/\//.test(url)) {
    return h('span', url)
  }
  return h(ElLink, { type: 'primary', onClick: () => { Browser.OpenURL(url) } }, { default: () => url })
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
// 处理站点弹窗请求成功事件
function handleSiteDialogRequestSuccess() {
  siteSearchTable.value.doSearch()
}
</script>
<template>
  <base-view>
    <template #default>
      <div class="site-manage-container">
        <search-table
          ref="siteSearchTable"
          v-model:page="sitePage"
          v-model:changed-rows="siteChangedRows"
          class="site-manage-search-table"
          toolbar-radius="var(--app-radius)"
          data-radius="var(--app-radius)"
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
            <el-input
              class="site-manage-search-input"
              v-model="siteSearchParams.siteName.value"
              placeholder="输入站点名称"
              clearable
              @clear="() => siteSearchParams.siteName.value = null"
            />
            <el-input
              class="site-manage-search-input"
              v-model="siteSearchParams.siteKey.value"
              placeholder="输入站点键"
              clearable
              @clear="() => siteSearchParams.siteKey.value = null"
            />
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
  </base-view>
</template>

<style scoped>
.site-manage-container {
  display: flex;
  flex-direction: row;
  justify-content: center;
  align-items: center;
  /* 容器不带底色：一体感由 SearchTable 自身的工具栏面与数据面（含分页面）连成的卡片承担；间距纯 margin（总边距 10px 不变） */
  width: calc(100% - 20px);
  height: calc(100% - 20px);
  margin: 10px;
}

.site-manage-search-table {
  height: 100%;
  width: 100%;
}
.site-manage-search-input {
  width: auto;
  flex-grow: 1;
}
</style>
