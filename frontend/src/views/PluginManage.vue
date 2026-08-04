<script setup lang="ts">
import BaseView from '@renderer/views/BaseView.vue'
import SearchTable from '@renderer/components/common/SearchTable.vue'
import PluginStatusPanel from '@renderer/components/plugin/PluginStatusPanel.vue'
import {onMounted, ref, Ref} from 'vue'
import OperationItem from '@renderer/model/util/OperationItem.ts'
import DialogMode from '@renderer/model/util/DialogMode.ts'
import {Thead} from '@renderer/model/util/Thead.ts'
import ApiUtil from '@renderer/utils/ApiUtil.ts'
import DataTableOperationResponse from '@renderer/model/util/DataTableOperationResponse.ts'
import {arrayNotEmpty} from '@renderer/utils/CommonUtil.ts'
import {ElMessage, ElMessageBox} from 'element-plus'
import PluginDialog from '@renderer/components/dialogs/PluginDialog.vue'
import PluginSettingDialog from '@renderer/components/dialogs/PluginSettingDialog.vue'
import {PluginQueryDTO} from '@bindings/github.com/library-squirrel/backend/plugin/models'
import {Operator, SortOrder} from '@bindings/github.com/library-squirrel/backend/base/query/models'
import {isNotBlank} from '@renderer/utils/StringUtil.ts'
import {fileSysUtilApi, pluginApi} from '@renderer/apis/http'
import {PluginDTO} from "@bindings/github.com/library-squirrel/backend/base/model/dto"
import {Page} from "@bindings/github.com/library-squirrel/backend/base/model"

// onMounted
onMounted(() => {
  // 使用各字段的 Order 属性进行排序，通过 Priority 控制优先级
  pluginSearchParams.value.updateTime = { value: null, order: SortOrder.OrderDesc, priority: 0 }
  pluginSearchParams.value.createTime = { value: null, order: SortOrder.OrderDesc, priority: 1 }
  pluginSearchTable.value.doSearch()
})

// 变量
// 插件数据表组件的实例
const pluginSearchTable = ref()
// 插件分页参数
const pluginPage: Ref<Page<PluginDTO>> = ref(new Page<PluginDTO>())
// 插件操作栏按钮
const pluginOperationButton: OperationItem<PluginDTO>[] = [
  { label: '设置', icon: 'Setting', code: 'settings' },
  { label: '查看', icon: 'View', code: DialogMode.VIEW },
  { label: '修复', icon: 'Refresh', code: 'reinstall' },
  { label: '卸载', icon: 'delete', code: 'uninstall' }
]
// 插件的表头
const pluginThead: Ref<Thead<PluginDTO>[]> = ref([
  new Thead({
    type: 'text',
    defaultDisabled: true,
    key: 'name',
    title: '名称',
    hide: false,
    headerAlign: 'center',
    dataAlign: 'center',
    showOverflowTooltip: true
  }),
  new Thead({
    type: 'text',
    defaultDisabled: true,
    key: 'author',
    title: '作者',
    hide: false,
    headerAlign: 'center',
    dataAlign: 'center',
    showOverflowTooltip: true
  }),
  new Thead({
    type: 'text',
    defaultDisabled: true,
    key: 'version',
    title: '版本号',
    hide: false,
    width: 100,
    headerAlign: 'center',
    dataAlign: 'center',
    showOverflowTooltip: true
  }),
  new Thead({
    type: 'datetime',
    defaultDisabled: true,
    key: 'createTime',
    title: '安装时间',
    hide: false,
    width: 180,
    headerAlign: 'center',
    dataAlign: 'center',
    showOverflowTooltip: true
  })
])
// 插件的查询参数
const pluginSearchParams: Ref<PluginQueryDTO> = ref<PluginQueryDTO>(new PluginQueryDTO())
// 被选中的插件
const pluginSelected: Ref<PluginDTO> = ref(new PluginDTO())
// 对话框开关
const dialogState: Ref<boolean> = ref(false)
// 对话框的数据
const dialogData: Ref<PluginDTO> = ref(new PluginDTO())
// 状态抽屉开关
const drawerVisible: Ref<boolean> = ref(false)
// 状态抽屉对应的插件 publicId
const statusPublicId: Ref<string> = ref('')
// 设置对话框开关
const settingDialogState: Ref<boolean> = ref(false)
// 设置对话框对应的插件 publicId
const settingPublicId: Ref<string> = ref('')

// 方法
// 分页查询插件
async function queryPage(page: Page<PluginDTO>): Promise<Page<PluginDTO>> {
  pluginSearchParams.value.name.operator = Operator.OpLike
  const response = await pluginApi.pluginQueryPage(page, pluginSearchParams.value)
  return response.data
}
// 处理插件数据行按钮点击事件
function handleRowButtonClicked(op: DataTableOperationResponse<PluginDTO>) {
  switch (op.code) {
    case DialogMode.VIEW:
      dialogData.value = op.data
      dialogState.value = true
      break
    case 'settings':
      settingPublicId.value = String(op.data.publicId)
      settingDialogState.value = true
      break
    case 'reinstall':
      beforeReInstall(String(op.data.publicId))
      break
    case 'uninstall':
      unInstall(String(op.data.publicId))
      break
    default:
      break
  }
}
// 处理被选中的插件改变的事件
async function handleSelectionChange(selections: PluginDTO[]) {
  if (selections.length > 0) {
    pluginSelected.value = selections[0]
    statusPublicId.value = String(selections[0].publicId)
    drawerVisible.value = true
  } else {
    drawerVisible.value = false
  }
}
// 重新安装前询问安装来源
async function beforeReInstall(pluginPublicId: string) {
  ElMessageBox.confirm('请选择修复方式', '', {
    confirmButtonText: '自动修复',
    cancelButtonText: '选择安装包修复',
    type: 'warning',
    distinguishCancelAndClose: true
  })
    .then(() => reInstall(pluginPublicId))
    .catch(async (action: 'cancel' | 'close') => {
      if (action === 'cancel') {
        const packagePath = await selectPackage()
        if (isNotBlank(packagePath)) {
          return reInstallFromPath(pluginPublicId, packagePath)
        }
      }
    })
}
// 重新安装
async function reInstall(pluginPublicId: string) {
  try {
    await pluginApi.pluginReinstall(pluginPublicId)
    pluginSearchTable.value.doSearch()
    ElMessage({ type: 'success', message: '修复完成' })
  } catch (e) {
    pluginSearchTable.value.doSearch()
    ElMessage({ type: 'error', message: `修复失败，${(e as Error).message}` })
  }
}
// 卸载
async function unInstall(pluginPublicId: string) {
  try {
    await pluginApi.pluginUnInstall(pluginPublicId)
    pluginSearchTable.value.doSearch()
    ElMessage({ type: 'success', message: '已卸载' })
  } catch (e) {
    pluginSearchTable.value.doSearch()
    ElMessage({ type: 'error', message: `卸载失败，${(e as Error).message}` })
  }
}
// 选择安装包
async function selectPackage(): Promise<string | undefined> {
  const response = await fileSysUtilApi.fileSysUtilSelectFile('选择插件安装包', undefined, [
    { DisplayName: '插件包', Pattern: '*.zip' }
  ])
  if (ApiUtil.check(response)) {
    const dirSelectResult = ApiUtil.data(response) as { canceled: boolean; filePaths: string[] }
    if (!dirSelectResult.canceled && arrayNotEmpty(dirSelectResult.filePaths)) {
      return dirSelectResult.filePaths[0]
    }
  }
  return undefined
}
async function handleInstallClicked() {
  const packagePath = await selectPackage()
  if (isNotBlank(packagePath)) {
    return installFromPath(packagePath)
  }
}
// 通过安装包路径安装插件
async function installFromPath(packagePath: string) {
  try {
    const result = await pluginApi.pluginInstallFromPath(packagePath)
    ApiUtil.msg(result)
    pluginSearchTable.value.doSearch()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}
// 通过安装包路径重新安装插件
async function reInstallFromPath(publicPublicId: string, packagePath: string) {
  try {
    const result = await pluginApi.pluginReinstallFromPath(publicPublicId, packagePath)
    ApiUtil.msg(result)
    pluginSearchTable.value.doSearch()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}
</script>
<template>
  <base-view>
    <template #default>
      <div class="plugin-manage-container">
        <search-table
          ref="pluginSearchTable"
          v-model:page="pluginPage"
          class="plugin-manage-left-search-table"
          data-key="id"
          :operation-button="pluginOperationButton"
          :thead="pluginThead"
          :search="queryPage"
          :multi-select="false"
          :selectable="true"
          :page-sizes="[10, 20, 50, 100]"
          :operation-width="220"
          @row-button-clicked="handleRowButtonClicked"
          @selection-change="handleSelectionChange"
        >
          <template #toolbarMain>
            <el-button
              type="primary"
              @click="handleInstallClicked"
            >
              安装
            </el-button>
            <el-input
              v-model="pluginSearchParams.name.value"
              placeholder="输入名称"
              clearable
            />
          </template>
          <template #toolbarDropdown>
            <el-button />
          </template>
        </search-table>
      </div>

      <el-drawer
        v-model="drawerVisible"
        title="插件状态"
        direction="rtl"
        size="450px"
        :destroy-on-close="false"
      >
        <PluginStatusPanel
          v-if="isNotBlank(statusPublicId)"
          :public-id="statusPublicId"
        />
      </el-drawer>
    </template>
    <template #dialog>
      <plugin-dialog
        v-model:form-data="dialogData"
        v-model:state="dialogState"
        :mode="DialogMode.VIEW"
      />
      <plugin-setting-dialog
        v-if="isNotBlank(settingPublicId)"
        v-model:state="settingDialogState"
        :public-id="settingPublicId"
      />
    </template>
  </base-view>
</template>

<style scoped>
.plugin-manage-container {
  display: flex;
  flex-direction: row;
  justify-content: center;
  align-items: center;
  background: var(--app-bg-surface);
  border-radius: var(--app-radius);
  width: calc(100% - 20px);
  height: calc(100% - 20px);
  padding: 5px;
  margin: 5px;
}
.plugin-manage-left-search-table {
  height: 100%;
  width: 100%;
}
</style>
