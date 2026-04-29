<script setup lang="ts">
import {onMounted, Ref, ref, UnwrapRef} from 'vue'
import BaseSubpage from './BaseSubpage.vue'
import SearchTable from '../components/common/SearchTable.vue'
import SiteTagDialog from '../components/dialogs/SiteTagDialog.vue'
import lodash from 'lodash'
import ApiUtil from '../utils/ApiUtil.ts'
import DataTableOperationResponse from '../model/util/DataTableOperationResponse.ts'
import {Thead} from '../model/util/Thead.ts'
import OperationItem from '../model/util/OperationItem.ts'
import DialogMode from '../model/util/DialogMode.ts'
import {isNullish, notNullish} from '@renderer/utils/CommonUtil.ts'
import {
  LocalTagDTO,
  SelectItem,
  SiteDTO,
  SiteTagDTO,
  SiteTagLocalRelateDTO
} from "@bindings/github.com/library-squirrel/wails/pkg/model/dto"
import AutoLoadSelect from '@renderer/components/common/AutoLoadSelect.vue'
import {siteQuerySelectItemPageBySiteName} from '@renderer/apis/SiteApi.ts'
import {localTagQuerySelectItemPageByName} from '@renderer/apis/http'
import {SiteTagQueryDTO} from '@bindings/github.com/library-squirrel/wails/internal/siteTag'
import {Operator, SortOrder} from '@bindings/github.com/library-squirrel/wails/pkg/query/models'
import {siteTagApi} from '@renderer/apis/http'
import {Page} from "@bindings/github.com/library-squirrel/wails/pkg/model"
import {newPage} from "@renderer/utils/Pager.ts"
import {ElMessage} from 'element-plus'

// onMounted
onMounted(() => {
  // 使用各字段的 Order 属性进行排序，通过 Priority 控制优先级
  siteTagQuery.value.updateTime = { value: null, order: SortOrder.OrderDesc, priority: 0 }
  siteTagQuery.value.createTime = { value: null, order: SortOrder.OrderDesc, priority: 1 }
  siteTagSearchTable.value.doSearch()
})

// 变量
// 接口
const apis = {
  siteTagCreateAndBindSameNameLocalTag: siteTagApi.siteTagCreateAndBindSameNameLocalTag,
  siteTagDeleteById: siteTagApi.siteTagDeleteById,
  siteTagUpdateById: siteTagApi.siteTagUpdateById,
  siteTagQueryLocalRelateDTOPage: siteTagApi.siteTagQueryLocalRelateDTOPage
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
const siteTagThead: Ref<Thead<SiteTagLocalRelateDTO>[]> = ref([
  new Thead({
    type: 'text',
    defaultDisabled: true,
    dblclickToEdit: true,
    key: 'siteTag.siteTagName',
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
    key: 'siteTag.description',
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
    key: 'siteTag.localTagId',
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
    getCacheData: (rowData: SiteTagLocalRelateDTO) => {
      if (isNullish(rowData.localTag?.id)) {
        return undefined
      }
      return new SelectItem({
        value: rowData.localTag.id,
        label: isNullish(rowData.localTag?.localTagName) ? '' : rowData.localTag.localTagName
      })
    },
    setCacheData: (rowData: SiteTagLocalRelateDTO, data: SelectItem) => {
      if (isNullish(rowData.localTag)) {
        rowData.localTag = new LocalTagDTO()
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
    key: 'siteTag.siteId',
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
    getCacheData: (rowData: SiteTagLocalRelateDTO) => {
      if (isNullish(rowData.site?.id)) {
        return undefined
      }
      return new SelectItem({
        value: rowData.site.id,
        label: isNullish(rowData.site?.siteName) ? '' : rowData.site.siteName
      })
    },
    setCacheData: (rowData: SiteTagLocalRelateDTO, data: SelectItem) => {
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
    key: 'siteTag.updateTime',
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
// 站点标签SearchTable的分页
const page: Ref<UnwrapRef<Page<SiteTagLocalRelateDTO>>> = ref(newPage<SiteTagLocalRelateDTO>())
// 站点标签查询参数
const siteTagQuery: Ref<SiteTagQueryDTO> = ref(new SiteTagQueryDTO())
// 站点标签弹窗的mode
const siteTagDialogMode: Ref<DialogMode> = ref(DialogMode.EDIT)
// 站点标签的对话框开关
const dialogState: Ref<boolean> = ref(false)
// 站点标签对话框的数据
const dialogData: Ref<SiteTagLocalRelateDTO> = ref(new SiteTagLocalRelateDTO({
  siteTag: new SiteTagDTO(),
  site: new SiteDTO()
}))

// 排序字段映射（将嵌套路径映射为查询字段名）
const sortPropMap: Record<string, string> = {
  'siteTag.siteTagName': 'siteTagName',
  'siteTag.siteId': 'siteId',
  'siteTag.updateTime': 'updateTime',
  'siteTag.createTime': 'createTime'
}

// 方法
// 分页查询站点标签的函数
async function siteTagQueryPageFn(
  page: Page<SiteTagLocalRelateDTO>
): Promise<Page<SiteTagLocalRelateDTO>> {
  if (notNullish(siteTagQuery.value.siteTagName.value)) {
    siteTagQuery.value.siteTagName.operator = Operator.OpLike
  }
  // 用户选择的排序优先级最高（priority=-1）
  if (sort.value.prop && sort.value.order) {
    const orderField = (sortPropMap[sort.value.prop] || sort.value.prop) as keyof SiteTagQueryDTO
    ;(siteTagQuery.value as any)[orderField] = {
      value: null,
      order: sort.value.order === 'ascending' ? SortOrder.OrderAsc : SortOrder.OrderDesc,
      priority: -1  // 用户选择优先级最高
    }
  }
  // 设置默认排序（用户选择优先级最高，updateTime 次之，createTime 再次）
  siteTagQuery.value.updateTime = { value: null, order: SortOrder.OrderDesc, priority: 0 }
  siteTagQuery.value.createTime = { value: null, order: SortOrder.OrderDesc, priority: 1 }
  const response = await apis.siteTagQueryLocalRelateDTOPage(page, siteTagQuery.value)
  return response.data
}
// 处理站点标签新增按钮点击事件
async function handleCreateButtonClicked() {
  siteTagDialogMode.value = DialogMode.NEW
  dialogData.value = new SiteTagLocalRelateDTO({
    siteTag: new SiteTagDTO(),
    site: new SiteDTO()
  })
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
      deleteSiteTag(Number(op.id))
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
async function deleteSiteTag(id: number) {
  try {
    const response = await apis.siteTagDeleteById(id)
    ApiUtil.msg(response)
    await siteTagSearchTable.value.doSearch()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}
// 保存行数据编辑
async function saveRowEdit(newData: SiteTagLocalRelateDTO) {
  if (isNullish(newData.siteTag)) {
    return
  }
  const tempData = new SiteTagDTO(newData.siteTag)
  try {
    const response = await apis.siteTagUpdateById(tempData)
    ApiUtil.msg(response)
    const index = changedRows.value.indexOf(newData)
    changedRows.value.splice(index, 1)
    refreshTable()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}
// 创建同名本地标签并绑定
async function creatSameNameLocalTagAndBind(siteTag: SiteTagLocalRelateDTO) {
  if (isNullish(siteTag.siteTag)) {
    return
  }
  const tempData = new SiteTagDTO(siteTag.siteTag)
  try {
    await apis.siteTagCreateAndBindSameNameLocalTag(tempData)
  } catch (e) {
    ElMessage.error((e as Error).message)
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
          v-model:toolbar-params="siteTagQuery"
          v-model:changed-rows="changedRows"
          v-model:sort="sort"
          class="tag-manage-search-table"
          data-key="siteTag.id"
          :operation-button="operationButton"
          :thead="siteTagThead"
          :search="siteTagQueryPageFn"
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
                <el-input v-model="siteTagQuery.siteTagName.value" placeholder="输入标签名称" clearable @clear="() => siteTagQuery.siteTagName.value = null" />
              </el-col>
              <el-col :span="4">
                <auto-load-select
                  v-model:data="siteTagQuery.siteId.value"
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
