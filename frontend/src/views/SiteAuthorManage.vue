<script setup lang="ts">
import {onMounted, Ref, ref} from 'vue'
import BaseSubpage from './BaseSubpage.vue'
import SearchTable from '../components/common/SearchTable.vue'
import ApiUtil from '../utils/ApiUtil.ts'
import {ElMessage} from 'element-plus'
import DataTableOperationResponse from '../model/util/DataTableOperationResponse.ts'
import {Thead} from '../model/util/Thead.ts'
import OperationItem from '../model/util/OperationItem.ts'
import DialogMode from '../model/util/DialogMode.ts'
import {isNullish, notNullish} from '@renderer/utils/CommonUtil.ts'
import SiteAuthorDialog from '@renderer/components/dialogs/SiteAuthorDialog.vue'
import {siteQuerySelectItemPageBySiteName} from '@renderer/apis/http'
import AutoLoadSelect from '@renderer/components/common/AutoLoadSelect.vue'
import {localAuthorQuerySelectItemPageByName, siteAuthorApi, appLauncherApi} from '@renderer/apis/http'
import {
  LocalAuthorDTO,
  SelectItem,
  SiteAuthorDTO,
  SiteAuthorLocalRelateDTO, SiteDTO
} from "@bindings/github.com/library-squirrel/backend/base/model/dto"
import {SiteAuthorQueryDTO} from '@bindings/github.com/library-squirrel/backend/siteAuthor/models'
import {Operator, SortOrder} from '@bindings/github.com/library-squirrel/backend/base/query/models'
import {Page} from "@bindings/github.com/library-squirrel/backend/base/model";
import {newPage} from "@renderer/utils/Pager.ts";

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
  { label: '主页', icon: 'Link', code: 'homepage', rule: (row) => notNullish(row.siteAuthor?.homepage) },
  { label: '查看', icon: 'view', code: DialogMode.VIEW },
  { label: '编辑', icon: 'edit', code: DialogMode.EDIT },
  { label: '删除', icon: 'delete', code: 'delete' }
]
// 站点作者SearchTable的表头
const siteAuthorThead: Ref<Thead<SiteAuthorLocalRelateDTO>[]> = ref([
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
    headerTagType: 'success',
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
    headerTagType: 'success',
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
    headerTagType: 'success',
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
  switch (op.code) {
    case 'create':
      await creatSameNameLocalAuthorAndBind(op.data)
      siteAuthorSearchTable.value.doSearch()
      break
    case 'save':
      saveRowEdit(op.data)
      break
    case 'homepage':
      await appLauncherApi.appLauncherOpenExternal(op.data.siteAuthor.homepage!)
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
</script>

<template>
  <base-subpage>
    <template #default>
      <div class="tag-manage-container">
        <search-table
          ref="siteAuthorSearchTable"
          v-model:page="page"
          v-model:toolbar-params="siteAuthorQuery"
          v-model:changed-rows="changedRows"
          class="tag-manage-search-table"
          data-key="siteAuthor.id"
          :operation-button="operationButton"
          :thead="siteAuthorThead"
          :search="siteAuthorQueryPageFn"
          :multi-select="true"
          :selectable="true"
          :page-sizes="[10, 20, 50, 100, 1000]"
          :operation-width="260"
          @row-button-clicked="handleRowButtonClicked"
        >
          <template #toolbarMain>
            <el-button type="primary" @click="handleCreateButtonClicked">新增</el-button>
            <el-row class="site-author-manage-search-bar">
              <el-col :span="20">
                <el-input v-model="siteAuthorQuery.authorName.value" placeholder="输入作者名称" clearable @clear="() => siteAuthorQuery.authorName.value = null" />
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
      <site-author-dialog
        v-model:form-data="dialogData"
        v-model:state="dialogState"
        :mode="siteAuthorDialogMode"
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
.site-author-manage-search-bar {
  flex-grow: 1;
}
</style>
