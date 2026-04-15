<script setup lang="ts">
import { onMounted, Ref, ref, UnwrapRef } from 'vue'
import BaseSubpage from './BaseSubpage.vue'
import SearchTable from '../components/common/SearchTable.vue'
import SiteTagDialog from '../components/dialogs/SiteTagDialog.vue'
import lodash from 'lodash'
import ApiUtil from '../utils/ApiUtil.ts'
import DataTableOperationResponse from '../model/util/DataTableOperationResponse.ts'
import { Thead } from '../model/util/Thead.ts'
import OperationItem from '../model/util/OperationItem.ts'
import DialogMode from '../model/util/DialogMode.ts'
import Page from '@renderer/model/util/Page.ts'
import { ElMessage } from 'element-plus'
import { isNullish, arrayNotEmpty } from '@renderer/utils/CommonUtil.ts'
import SelectItem from '@renderer/model/util/SelectItem.ts'
import AutoLoadSelect from '@renderer/components/common/AutoLoadSelect.vue'
import { siteQuerySelectItemPageBySiteName } from '@renderer/apis/SiteApi.ts'
import { localTagQuerySelectItemPageByName } from '@renderer/apis/LocalTagApi.ts'
import SiteTagQueryDTO from '@renderer/model/model/queryDTO/SiteTagQueryDTO.ts'
import SiteTagLocalRelateDTO from '@renderer/model/model/dto/SiteTagLocalRelateDTO.ts'
import type { QuerySortOption } from '@renderer/model/util/QuerySortOption.ts'
import SiteTagFullDTO from '@renderer/model/model/dto/SiteTagFullDTO.ts'
import LocalTag from '@renderer/model/model/entity/LocalTag.ts'
import Site from '@renderer/model/model/entity/Site.ts'
import SiteTag from '@renderer/model/model/entity/SiteTag.ts'
import { localTagApi, siteTagApi } from '@renderer/apis/http'

// onMounted
onMounted(() => {
  if (isNullish(page.value.query)) {
    page.value.query = new SiteTagQueryDTO()
  }
  page.value.query.sort = [
    { key: 'updateTime', asc: false },
    { key: 'createTime', asc: false }
  ]
  siteTagSearchTable.value.doSearch()
})

// 变量
// 接口
const apis = {
  localTagQuerySelectItemPage: localTagApi.localTagQuerySelectItemPage,
  siteTagCreateAndBindSameNameLocalTag: async (siteTag: any) => {
    ElMessage.error('此功能暂未实现：siteTagCreateAndBindSameNameLocalTag')
    return { success: false, msg: '此功能暂未实现' }
  },
  siteTagDeleteById: siteTagApi.siteTagDeleteById,
  siteTagUpdateById: siteTagApi.siteTagUpdateById,
  siteTagQueryLocalRelateDTOPage: async (page: any) => {
    ElMessage.error('此功能暂未实现：siteTagQueryLocalRelateDTOPage')
    return { success: false, msg: '此功能暂未实现', data: page }
  }
}
// siteTagSearchTable的组件实例
const siteTagSearchTable = ref()
// 被改变的数据行
const changedRows: Ref<SiteTagLocalRelateDTO[]> = ref([])
// 排序配置
const sort: Ref<{ prop: string; order: 'ascending' | 'descending' | null }> = ref({ prop: '', order: null })
// 站点标签SearchTable的operationButton
const operationButton: OperationItem<SiteTagLocalRelateDTO>[] = [
  {
    label: '保存',
    icon: 'Checked',
    buttonType: 'primary',
    code: 'save',
    rule: (row) => changedRows.value.includes(row)
  },
  {
    label: '创建同名本地标签',
    icon: 'CirclePlusFilled',
    buttonType: 'primary',
    code: 'create',
    rule: (row) => !row.hasSameNameLocalTag
  },
  { label: '查看', icon: 'view', code: DialogMode.VIEW },
  { label: '编辑', icon: 'edit', code: DialogMode.EDIT },
  { label: '删除', icon: 'delete', code: 'delete' }
]
// 站点标签SearchTable的表头
const siteTagThead: Ref<Thead<SiteTagFullDTO>[]> = ref([
  new Thead({
    type: 'text',
    defaultDisabled: true,
    dblclickToEdit: true,
    key: 'siteTagName',
    title: '名称',
    hide: false,
    width: 250,
    headerAlign: 'center',
    dataAlign: 'center',
    showOverflowTooltip: true,
    sortable: 'custom'
  }),
  new Thead({
    type: 'textarea',
    defaultDisabled: true,
    dblclickToEdit: true,
    key: 'description',
    title: '详情',
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
    key: 'localTagId',
    title: '本地标签',
    hide: false,
    width: 150,
    headerAlign: 'center',
    headerTagType: 'success',
    dataAlign: 'center',
    showOverflowTooltip: true,
    remote: true,
    remotePaging: true,
    remotePageMethod: localTagQuerySelectItemPageByName,
    getCacheData: (rowData: SiteTagFullDTO) => {
      if (isNullish(rowData.localTag?.id)) {
        return undefined
      }
      return new SelectItem({
        value: rowData.localTag.id,
        label: isNullish(rowData.localTag?.localTagName) ? '' : rowData.localTag.localTagName
      })
    },
    setCacheData: (rowData: SiteTagFullDTO, data: SelectItem) => {
      if (isNullish(rowData.localTag)) {
        rowData.localTag = new LocalTag()
      }
      rowData.localTag.id = Number(data.value)
      rowData.localTag.localTagName = data.label
    }
  }),
  new Thead({
    type: 'autoLoadSelect',
    editMethod: 'replace',
    defaultDisabled: true,
    dblclickToEdit: true,
    key: 'siteId',
    title: '站点',
    hide: false,
    width: 150,
    headerAlign: 'center',
    headerTagType: 'success',
    dataAlign: 'center',
    showOverflowTooltip: true,
    sortable: 'custom',
    remote: true,
    remotePaging: true,
    remotePageMethod: siteQuerySelectItemPageBySiteName,
    getCacheData: (rowData: SiteTagFullDTO) => {
      if (isNullish(rowData.site?.id)) {
        return undefined
      }
      return new SelectItem({
        value: rowData.site.id,
        label: isNullish(rowData.site?.siteName) ? '' : rowData.site.siteName
      })
    },
    setCacheData: (rowData: SiteTagFullDTO, data: SelectItem) => {
      if (isNullish(rowData.site)) {
        rowData.site = new Site()
      }
      rowData.site.id = Number(data.value)
      rowData.site.siteName = data.label
    }
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
    showOverflowTooltip: true,
    sortable: 'custom'
  })
])
// 站点标签SearchTable的查询参数
const siteTagSearchParams: Ref<SiteTagQueryDTO> = ref(new SiteTagQueryDTO())
// 站点标签SearchTable的分页
const page: Ref<UnwrapRef<Page<SiteTagQueryDTO, SiteTagLocalRelateDTO>>> = ref(new Page<SiteTagQueryDTO, SiteTagLocalRelateDTO>())
// 站点标签弹窗的mode
const siteTagDialogMode: Ref<DialogMode> = ref(DialogMode.EDIT)
// 站点标签的对话框开关
const dialogState: Ref<boolean> = ref(false)
// 站点标签对话框的数据
const dialogData: Ref<SiteTagLocalRelateDTO> = ref(new SiteTagLocalRelateDTO())

// 方法
// 分页查询站点标签的函数
async function siteTagQueryPage(
  page: Page<SiteTagQueryDTO, object>
): Promise<Page<SiteTagQueryDTO, SiteTagLocalRelateDTO> | undefined> {
  // 根据用户选择的排序构建排序配置
  const userSort: QuerySortOption[] = []
  // 添加用户选择的排序（如果存在）
  if (sort.value.prop && sort.value.order) {
    userSort.push({
      key: sort.value.prop,
      asc: sort.value.order === 'ascending'
    })
  }
  // 添加默认排序（按修改时间和创建时间降序）
  if (isNullish(page.query)) {
    page.query = new SiteTagQueryDTO()
  }
  if (arrayNotEmpty(page.query.sort)) {
    page.query.sort = [...userSort, ...page.query.sort]
  } else {
    page.query.sort = [...userSort, { key: 'updateTime', asc: false }, { key: 'createTime', asc: false }]
  }
  const response = await apis.siteTagQueryLocalRelateDTOPage(page)
  if (ApiUtil.check(response)) {
    let responsePage = ApiUtil.data<Page<SiteTagQueryDTO, SiteTagLocalRelateDTO>>(response)
    if (isNullish(responsePage)) {
      return undefined
    }
    responsePage = new Page(responsePage)
    return responsePage
  } else {
    ApiUtil.msg(response)
    return undefined
  }
}
// 处理站点标签新增按钮点击事件
async function handleCreateButtonClicked() {
  siteTagDialogMode.value = DialogMode.NEW
  dialogData.value = new SiteTagLocalRelateDTO()
  dialogState.value = true
}
// 处理站点标签数据行按钮点击事件
async function handleRowButtonClicked(op: DataTableOperationResponse<SiteTagLocalRelateDTO>) {
  switch (op.code) {
    case 'create':
      await creatSameNameLocalTagAndBind(lodash.cloneDeep(op.data))
      siteTagSearchTable.value.doSearch()
      break
    case 'save':
      saveRowEdit(op.data)
      break
    case DialogMode.VIEW:
      siteTagDialogMode.value = DialogMode.VIEW
      dialogData.value = op.data
      dialogState.value = true
      break
    case DialogMode.EDIT:
      siteTagDialogMode.value = DialogMode.EDIT
      dialogData.value = op.data
      dialogState.value = true
      break
    case 'delete':
      deleteSiteTag(op.id)
      break
    default:
      break
  }
}
// 处理站点标签弹窗请求成功事件
function refreshTable() {
  siteTagSearchTable.value.doSearch()
}
// 删除站点标签
async function deleteSiteTag(id: string) {
  const response = await apis.siteTagDeleteById(id)
  ApiUtil.msg(response)
  if (ApiUtil.check(response)) {
    await siteTagSearchTable.value.doSearch()
  }
}
// 保存行数据编辑
async function saveRowEdit(newData: SiteTagLocalRelateDTO) {
  const tempData = lodash.cloneDeep(newData)

  const response = await apis.siteTagUpdateById(tempData)
  ApiUtil.msg(response)
  if (ApiUtil.check(response)) {
    const index = changedRows.value.indexOf(newData)
    changedRows.value.splice(index, 1)
    refreshTable()
  }
}
// 创建同名本地标签并绑定
async function creatSameNameLocalTagAndBind(siteTag: SiteTag) {
  const response = await apis.siteTagCreateAndBindSameNameLocalTag(siteTag)
  if (!ApiUtil.check(response)) {
    ApiUtil.msg(response)
  }
}
</script>

<template>
  <base-subpage>
    <template #default>
      <div class="tag-manage-container">
        <search-table
          ref="siteTagSearchTable"
          v-model:page="page"
          v-model:toolbar-params="siteTagSearchParams"
          v-model:changed-rows="changedRows"
          v-model:sort="sort"
          class="tag-manage-search-table"
          data-key="id"
          :operation-button="operationButton"
          :thead="siteTagThead"
          :search="siteTagQueryPage"
          :multi-select="true"
          :selectable="true"
          :page-sizes="[10, 20, 50, 100, 1000]"
          :operation-width="205"
          @row-button-clicked="handleRowButtonClicked"
          @sort-change="siteTagSearchTable.doSearch()"
        >
          <template #toolbarMain>
            <el-button type="primary" @click="handleCreateButtonClicked">新增</el-button>
            <el-row class="site-tag-manage-search-bar">
              <el-col :span="20">
                <el-input v-model="siteTagSearchParams.siteTagName" placeholder="输入标签名称" clearable />
              </el-col>
              <el-col :span="4">
                <auto-load-select
                  v-model="siteTagSearchParams.siteId"
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
        </search-table>
      </div>
    </template>
    <template #dialog>
      <site-tag-dialog
        v-model:form-data="dialogData"
        v-model:state="dialogState"
        :mode="siteTagDialogMode"
        @request-success="refreshTable"
      />
    </template>
  </base-subpage>
</template>

<style>
.tag-manage-container {
  background: #f4f4f4;
  border-radius: 6px;
  display: flex;
  width: calc(100% - 20px);
  height: calc(100% - 20px);
  padding: 5px;
  margin: 5px;
  flex-direction: row;
  justify-content: center;
  align-items: center;
}
.tag-manage-search-table {
  height: 100%;
  width: 100%;
}
.site-tag-manage-search-bar {
  flex-grow: 1;
}
</style>
