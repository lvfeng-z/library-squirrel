<script setup lang="ts">
import BaseView from '@renderer/views/BaseView.vue'
import SearchTable from '@renderer/components/common/SearchTable.vue'
import StatusTag from '@renderer/components/common/StatusTag.vue'
import PluginStatusPanel from '@renderer/components/plugin/PluginStatusPanel.vue'
import {computed, h, onMounted, ref, Ref} from 'vue'
import {useRouter} from 'vue-router'
import OperationItem from '@renderer/model/util/OperationItem.ts'
import DialogMode from '@renderer/model/util/DialogMode.ts'
import {Thead} from '@renderer/model/util/Thead.ts'
import ApiUtil from '@renderer/utils/ApiUtil.ts'
import DataTableOperationResponse from '@renderer/model/util/DataTableOperationResponse.ts'
import {arrayIsEmpty, arrayNotEmpty, isNullish, notNullish} from '@renderer/utils/CommonUtil.ts'
import {ElMessage, ElMessageBox} from 'element-plus'
import PluginDialog from '@renderer/components/dialogs/PluginDialog.vue'
import PluginSettingDialog from '@renderer/components/dialogs/PluginSettingDialog.vue'
import {PluginQueryDTO} from '@bindings/github.com/library-squirrel/backend/plugin/models'
import {Operator, SortOrder} from '@bindings/github.com/library-squirrel/backend/base/query/models'
import {isNotBlank} from '@renderer/utils/StringUtil.ts'
import {fileSysUtilApi, pluginApi, taskApi} from '@renderer/apis/http'
import {PluginDTO, PendingUpgradeDTO} from "@bindings/github.com/library-squirrel/backend/base/model/dto"
import {Page} from "@bindings/github.com/library-squirrel/backend/base/model"
import {usePluginUpdateStore} from '@renderer/store/UsePluginUpdateStore.ts'

const router = useRouter()

// onMounted
onMounted(() => {
  // 使用各字段的 Order 属性进行排序，通过 Priority 控制优先级
  pluginSearchParams.value.updateTime = { value: null, order: SortOrder.OrderDesc, priority: 0 }
  pluginSearchParams.value.createTime = { value: null, order: SortOrder.OrderDesc, priority: 1 }
  pluginSearchTable.value.doSearch()
  // 进入页面即刷新检查更新待办（红点与待更新区块同步）
  pluginUpdateStore.refresh()
})

// 变量
// 插件数据表组件的实例
const pluginSearchTable = ref()
// 插件分页参数
const pluginPage: Ref<Page<PluginDTO>> = ref(new Page<PluginDTO>())
// 列表当前行（queryPage 每次查询刷新；SearchTable 行数据留在组件内部，v-model:page 仅回写分页元数据）
const tableRows: Ref<PluginDTO[]> = ref([])
// 插件操作栏按钮（首个 rule 命中的为主按钮，其余进下拉；升级/跳过仅对有可升级待办的行显示）
const pluginOperationButton: OperationItem<PluginDTO>[] = [
  { label: '升级', icon: 'Upload', code: 'upgrade', buttonType: 'primary', rule: (row) => notNullish(findAvailableByPublicId(String(row.publicId))) },
  { label: '设置', icon: 'Setting', code: 'settings' },
  { label: '查看', icon: 'View', code: DialogMode.VIEW },
  { label: '信任', icon: 'Check', code: 'trust' },
  { label: '修复', icon: 'Refresh', code: 'reinstall' },
  { label: '状态', icon: 'Monitor', code: 'status' },
  { label: '跳过此构建', icon: 'Close', code: 'decline', rule: (row) => notNullish(findAvailableByPublicId(String(row.publicId))) },
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
    type: 'custom',
    defaultDisabled: true,
    key: 'source',
    title: '来源',
    hide: false,
    width: 90,
    headerAlign: 'center',
    dataAlign: 'center',
    render: (data, row) => h(StatusTag, { status: buildSourceStatusKey(row) })
  }),
  new Thead({
    type: 'custom',
    defaultDisabled: true,
    key: 'trusted',
    title: '信任',
    hide: false,
    width: 90,
    headerAlign: 'center',
    dataAlign: 'center',
    render: (data) => h(StatusTag, { status: data === true ? 'plugin-trusted' : 'plugin-unverified' })
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
// 来源状态 key：官方身份优先（内容摘要命中官方指纹名单→plugin-official 绿），未证实按安装渠道 plugin-{source}（bundled/local/url/marketplace）
function buildSourceStatusKey(plugin: PluginDTO | null | undefined): string {
  return plugin?.official === true ? 'plugin-official' : `plugin-${plugin?.source ?? ''}`
}
// 插件的查询参数
const pluginSearchParams: Ref<PluginQueryDTO> = ref<PluginQueryDTO>(new PluginQueryDTO())
// 被选中的插件（多选，供批量升级）
const selectedPlugins: Ref<PluginDTO[]> = ref([])
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

// 检查更新待办（红点数据源同 store；本页承担答复：升级/跳过/重新提示）
const pluginUpdateStore = usePluginUpdateStore()
// 升级执行中的待办 publicId（按钮 loading 与重复点击拦截；后端执行中守卫为最权威兜底）
const applyingIds: Ref<string[]> = ref([])

// 已跳过更新的 bundled 插件（来自本页插件列表数据，PluginDTO 透传拒绝标记）
const declinedPlugins = computed<PluginDTO[]>(() => {
  return tableRows.value.filter(
    (plugin) => plugin?.source === 'bundled' && isNotBlank(plugin.upgradeDeclinedBuildId ?? undefined)
  ) as PluginDTO[]
})

// 列表是否存在非官方插件（第三方免责提示仅此时显示，纯官方库下省略；仅 true 视为官方，false/NULL 一律非官方——保守方向）
const hasThirdPartyPlugin = computed(() => {
  return tableRows.value.some((plugin) => plugin?.official !== true)
})

// 待更新区块是否可见（告知类任一非空；可升级项经行内按钮与批量升级答复，不占区块）
const hasPendingEntry = computed(() => {
  return (
    arrayNotEmpty(pluginUpdateStore.forcedList) ||
    arrayNotEmpty(pluginUpdateStore.errorList) ||
    arrayNotEmpty(declinedPlugins.value)
  )
})

// 按插件身份键查其可升级待办
function findAvailableByPublicId(publicId: string): PendingUpgradeDTO | undefined {
  return pluginUpdateStore.availableList.find((item) => item.publicId === publicId)
}

// 选中插件中的可升级待办（批量升级目标）
const selectedAvailableItems = computed<PendingUpgradeDTO[]>(() => {
  const selectedIds = new Set(selectedPlugins.value.map((plugin) => String(plugin.publicId)))
  return pluginUpdateStore.availableList.filter((item) => selectedIds.has(item.publicId))
})

// 批量升级按钮禁用态（未选中任何可升级插件）
const batchUpgradeDisabled = computed(() => arrayIsEmpty(selectedAvailableItems.value))

// 批量卸载按钮禁用态（未选中任何插件）
const batchUninstallDisabled = computed(() => arrayIsEmpty(selectedPlugins.value))

// 版本展示文案（含降级标注）
function versionText(item: PendingUpgradeDTO): string {
  const direction = item.direction === 'down' ? '（降级）' : ''
  return `v${item.installedVersion} → v${item.targetVersion}${direction}`
}

// 点击升级：先按运行中任务数分流——N>0 引导先去暂停（服务端必否决，不提供「仍要尝试」），N=0 直接确认执行
async function handleUpgradeClicked(item: PendingUpgradeDTO) {
  const activeCount = (await taskApi.taskGetActiveCountByPlugin(item.publicId)).data ?? 0
  if (activeCount > 0) {
    ElMessageBox.confirm(
      `该插件有 ${activeCount} 个运行中任务，升级前需先暂停（已暂停任务不受影响）。`,
      '插件升级',
      { confirmButtonText: '去任务页', cancelButtonText: '取消', type: 'warning' }
    )
      .then(() => {
        router.push({ name: 'taskManage' })
      })
      .catch(() => {})
    return
  }
  ElMessageBox.confirm(`将【${item.pluginName}】升级：${versionText(item)}`, '插件升级', {
    confirmButtonText: '升级', cancelButtonText: '取消'
  })
    .then(() => applyPendingUpgrade(item))
    .catch(() => {})
}

// 执行换版核心（无提示；运行期热重载，被运行中任务否决时返回 false）。单条与批量共用
async function doApplyUpgrade(item: PendingUpgradeDTO): Promise<boolean> {
  if (applyingIds.value.includes(item.publicId)) {
    return false
  }
  applyingIds.value.push(item.publicId)
  try {
    await pluginApi.pluginApplyPendingUpgrade(item.publicId)
    return true
  } catch {
    return false
  } finally {
    applyingIds.value = applyingIds.value.filter((id) => id !== item.publicId)
  }
}

// 单条升级（行内按钮路径）：成败即时提示并刷新
async function applyPendingUpgrade(item: PendingUpgradeDTO) {
  const success = await doApplyUpgrade(item)
  if (success) {
    ElMessage.success(`【${item.pluginName}】已升级到 v${item.targetVersion}`)
  } else {
    ElMessage.error(`升级失败：${item.pluginName}（存在运行中任务或执行中，请暂停任务后重试）`)
  }
  await pluginUpdateStore.refresh()
  pluginSearchTable.value.doSearch()
}

// 批量升级：顺序升级选中插件中的可升级项（逐项走服务端否决），汇总结果
function handleBatchUpgradeClicked() {
  const targets = selectedAvailableItems.value
  const skipped = selectedPlugins.value.length - targets.length
  const skipText = skipped > 0 ? `，${skipped} 个选中项无可用更新将跳过` : ''
  ElMessageBox.confirm(
    `将升级 ${targets.length} 个插件${skipText}。存在运行中任务的插件的升级将被阻止。`,
    '批量升级',
    { confirmButtonText: '升级', cancelButtonText: '取消', type: 'warning' }
  )
    .then(async () => {
      let successCount = 0
      const failures: string[] = []
      for (const item of targets) {
        if (await doApplyUpgrade(item)) {
          successCount++
        } else {
          failures.push(item.pluginName)
        }
      }
      if (arrayIsEmpty(failures)) {
        ElMessage.success(`批量升级完成：${successCount} 个成功`)
      } else {
        ElMessage.warning(`批量升级完成：${successCount} 个成功，${failures.length} 个失败（${failures.join('、')}）`)
      }
      await pluginUpdateStore.refresh()
      pluginSearchTable.value.doSearch()
    })
    .catch(() => {})
}

// 批量卸载：顺序卸载选中插件（逐项走服务端否决——运行中任务阻止卸载），汇总结果
function handleBatchUninstallClicked() {
  const targets = selectedPlugins.value
  ElMessageBox.confirm(
    `确认卸载选中的 ${targets.length} 个插件？卸载后其扩展功能将不可用；存在运行中任务的插件的卸载将被阻止。`,
    '批量卸载',
    { confirmButtonText: '卸载', cancelButtonText: '取消', type: 'warning' }
  )
    .then(async () => {
      let successCount = 0
      const failures: string[] = []
      for (const plugin of targets) {
        try {
          await pluginApi.pluginUnInstall(String(plugin.publicId))
          successCount++
        } catch {
          failures.push(String(plugin.name))
        }
      }
      if (arrayIsEmpty(failures)) {
        ElMessage.success(`批量卸载完成：${successCount} 个成功`)
      } else {
        ElMessage.warning(`批量卸载完成：${successCount} 个成功，${failures.length} 个失败（${failures.join('、')}）`)
      }
      await pluginUpdateStore.refresh()
      pluginSearchTable.value.doSearch()
    })
    .catch(() => {})
}

// 跳过此构建：持久化拒绝标记，下次启动对等值 buildId 静默跳过，直到新构建出现
function handleDeclineClicked(item: PendingUpgradeDTO) {
  ElMessageBox.confirm(
    `跳过【${item.pluginName}】的此构建（v${item.targetVersion}）？下次启动不再提示，直到新构建出现。`,
    '跳过此构建',
    { confirmButtonText: '跳过', cancelButtonText: '取消', type: 'warning' }
  )
    .then(async () => {
      try {
        await pluginApi.pluginDeclinePendingUpgrade(item.publicId)
        ElMessage.success('已跳过此构建')
      } catch (e) {
        ElMessage.error((e as Error).message)
      } finally {
        await pluginUpdateStore.refresh()
        pluginSearchTable.value.doSearch()
      }
    })
    .catch(() => {})
}

// 重新提示（反悔入口）：清除拒绝标记并立即重跑检测重建待办
async function handleRestoreClicked(plugin: PluginDTO) {
  try {
    await pluginApi.pluginRestorePendingUpgrade(String(plugin.publicId))
    ElMessage.success('已恢复更新提示')
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    await pluginUpdateStore.refresh()
    pluginSearchTable.value.doSearch()
  }
}

// 方法
// 分页查询插件
async function queryPage(page: Page<PluginDTO>): Promise<Page<PluginDTO>> {
  pluginSearchParams.value.name.operator = Operator.OpLike
  pluginSearchParams.value.author.operator = Operator.OpLike
  const response = await pluginApi.pluginQueryPage(page, pluginSearchParams.value)
  tableRows.value = response.data?.data ?? []
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
    case 'trust':
      handleTrust(op.data)
      break
    case 'reinstall':
      beforeReInstall(String(op.data.publicId))
      break
    case 'status':
      statusPublicId.value = String(op.data.publicId)
      drawerVisible.value = true
      break
    case 'upgrade': {
      const item = findAvailableByPublicId(String(op.data.publicId))
      if (isNullish(item)) {
        ElMessage.info('该插件无可用更新')
      } else {
        handleUpgradeClicked(item)
      }
      break
    }
    case 'decline': {
      const item = findAvailableByPublicId(String(op.data.publicId))
      if (isNullish(item)) {
        ElMessage.info('该插件无可用更新')
      } else {
        handleDeclineClicked(item)
      }
      break
    }
    case 'uninstall':
      unInstall(String(op.data.publicId))
      break
    default:
      break
  }
}
// 处理被选中的插件改变的事件（多选；选中集供批量升级使用）
function handleSelectionChange(selections: PluginDTO[]) {
  selectedPlugins.value = selections
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
    await pluginApi.pluginReinstall(pluginPublicId, true)
    pluginSearchTable.value.doSearch()
    ElMessage({ type: 'success', message: '修复完成' })
  } catch (e) {
    pluginSearchTable.value.doSearch()
    ElMessage({ type: 'error', message: `修复失败，${(e as Error).message}` })
  }
}
// 卸载
async function unInstall(pluginPublicId: string) {
  ElMessageBox.confirm('确认卸载该插件？卸载后其扩展功能将不可用。', '卸载插件', {
    confirmButtonText: '卸载', cancelButtonText: '取消', type: 'warning'
  })
    .then(async () => {
      try {
        await pluginApi.pluginUnInstall(pluginPublicId)
        pluginSearchTable.value.doSearch()
        ElMessage({ type: 'success', message: '已卸载' })
      } catch (e) {
        pluginSearchTable.value.doSearch()
        ElMessage({ type: 'error', message: `卸载失败，${(e as Error).message}` })
      }
    })
    .catch(() => {})
}
// 设置插件信任状态（手动信任/取消信任）：取消信任即时停用运行时——确认框按运行中任务数明示代价（决策6）
async function handleTrust(plugin: PluginDTO) {
  const publicId = String(plugin.publicId)
  if (plugin.trusted === true) {
    const activeCount = (await taskApi.taskGetActiveCountByPlugin(publicId)).data ?? 0
    const message = activeCount > 0
      ? `取消信任将立即停用该插件，其 ${activeCount} 个运行中任务将失败终止。确认取消信任？`
      : '取消信任将立即停用该插件。确认取消信任？'
    ElMessageBox.confirm(message, '取消信任', {
      confirmButtonText: '取消信任', cancelButtonText: '保留', type: 'warning'
    })
      .then(async () => {
        try {
          await pluginApi.pluginSetTrusted(publicId, false, true)
          pluginSearchTable.value.doSearch()
          ElMessage.success('已取消信任并停用')
        } catch (e) {
          ElMessage.error((e as Error).message)
        }
      })
      .catch(() => {})
  } else {
    try {
      await pluginApi.pluginSetTrusted(publicId, true)
      pluginSearchTable.value.doSearch()
      ElMessage.success('已信任并激活')
    } catch (e) {
      ElMessage.error((e as Error).message)
    }
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
    // 手动安装（用户选择的本地包）知情同意：告知完整宿主能力风险，确认后传 trusted=true；取消则不安装。
    // 文案中性化——不断言包的发布者身份，官方判定由 host 指纹名单在安装后给出（列表绿「官方」标呈现）
    ElMessageBox.confirm(
      '此插件非随主程序捆绑分发，安装运行后将获得宿主完整权限，包括：读写你的全部资源库数据、创建下载任务、发起任意网络请求、打开原生窗口、执行任意代码。<br><br>注意：插件作者身份无法验证，运行期间造成的数据外泄或损坏将不可逆。<br><br>请仅在你了解并信任该插件及其作者时确认安装。',
      '安装插件',
      { confirmButtonText: '确认安装', cancelButtonText: '取消', type: 'warning', dangerouslyUseHTMLString: true }
    )
      .then(() => installFromPath(packagePath, true))
      .catch(() => {})
  }
}
// 通过安装包路径安装插件。trusted 透传知情同意结果（true=已确认信任；false=未信任，装后需手动信任）
async function installFromPath(packagePath: string, trusted: boolean = false) {
  try {
    const result = await pluginApi.pluginInstallFromPath(packagePath, trusted)
    ApiUtil.msg(result)
    pluginSearchTable.value.doSearch()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}
// 通过安装包路径重新安装插件
async function reInstallFromPath(publicPublicId: string, packagePath: string) {
  try {
    const result = await pluginApi.pluginReinstallFromPath(publicPublicId, packagePath, true)
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
        <!-- 检查更新告知区块：可升级项经行内按钮与批量升级答复，此处仅保留告知与反悔（已跳过/已自动升级/安装失败） -->
        <div
          v-if="hasPendingEntry"
          class="plugin-pending-panel"
        >
          <div
            v-if="arrayNotEmpty(declinedPlugins)"
            class="plugin-pending-group"
          >
            <div class="plugin-pending-group-title">已跳过</div>
            <div
              v-for="plugin in declinedPlugins"
              :key="plugin.id"
              class="plugin-pending-row"
            >
              <span class="plugin-pending-name">{{ plugin.name }}</span>
              <span class="plugin-pending-version">已跳过构建 {{ plugin.upgradeDeclinedBuildId }}</span>
              <el-button
                size="small"
                @click="handleRestoreClicked(plugin)"
              >
                重新提示
              </el-button>
            </div>
          </div>
          <div
            v-if="arrayNotEmpty(pluginUpdateStore.forcedList)"
            class="plugin-pending-group"
          >
            <div class="plugin-pending-group-title">已自动升级</div>
            <div
              v-for="item in pluginUpdateStore.forcedList"
              :key="item.publicId"
              class="plugin-pending-row"
            >
              <span class="plugin-pending-name">{{ item.pluginName }}</span>
              <span class="plugin-pending-version">{{ versionText(item) }}（已装版本契约不兼容，已强制升级）</span>
            </div>
          </div>
          <div
            v-if="arrayNotEmpty(pluginUpdateStore.errorList)"
            class="plugin-pending-group"
          >
            <div class="plugin-pending-group-title">捆绑包安装失败</div>
            <div
              v-for="(item, index) in pluginUpdateStore.errorList"
              :key="index"
              class="plugin-pending-row"
            >
              <span class="plugin-pending-name">{{ item.pluginName }}</span>
              <span class="plugin-pending-message">{{ item.message }}</span>
            </div>
          </div>
        </div>
        <!-- 第三方免责提示：仅列表存在非官方插件时显示 -->
        <div
          v-if="hasThirdPartyPlugin"
          class="plugin-disclaimer"
        >
          来源标记区分官方与第三方插件；第三方插件由其作者独立维护，相关问题请咨询插件作者。
        </div>
        <search-table
          ref="pluginSearchTable"
          v-model:page="pluginPage"
          class="plugin-manage-left-search-table"
          data-key="id"
          :operation-button="pluginOperationButton"
          :thead="pluginThead"
          :search="queryPage"
          :multi-select="true"
          :selectable="true"
          :page-sizes="[10, 20, 50, 100]"
          :operation-width="280"
          toolbar-radius="var(--app-radius)"
          data-radius="var(--app-radius)"
          @row-button-clicked="handleRowButtonClicked"
          @selection-change="handleSelectionChange"
        >
          <template #toolbarMain>
            <!-- 第一行：按钮类操作（批量卸载为破坏性操作：danger + tone-fail） -->
            <el-button
              :disabled="batchUpgradeDisabled"
              @click="handleBatchUpgradeClicked"
            >
              批量升级
            </el-button>
              <el-button
                type="danger"
                class="tone-fail"
                :disabled="batchUninstallDisabled"
                @click="handleBatchUninstallClicked"
              >
                批量卸载
              </el-button>
              <el-button
                type="primary"
                @click="handleInstallClicked"
              >
                安装
              </el-button>
              <!-- 第二行：输入类筛选（全宽元素强制分行——工具栏显式分行由消费者负责） -->
              <div class="plugin-toolbar-filter-row">
                <el-input
                  v-model="pluginSearchParams.name.value"
                  class="plugin-name-search-input"
                  placeholder="输入名称"
                  clearable
                  @clear="pluginSearchParams.name.value = null"
                />
                <el-input
                  v-model="pluginSearchParams.author.value"
                  class="plugin-author-search-input"
                  placeholder="输入作者"
                  clearable
                  @clear="pluginSearchParams.author.value = null"
                />
                <el-select
                  v-model="pluginSearchParams.source.value"
                  class="plugin-source-search-select"
                  placeholder="选择来源"
                  clearable
                  @clear="pluginSearchParams.source.value = null"
                >
                  <el-option
                    label="捆绑"
                    value="bundled"
                  />
                  <el-option
                    label="本地"
                    value="local"
                  />
                  <el-option
                    label="网络"
                    value="url"
                  />
                  <el-option
                    label="市场"
                    value="marketplace"
                  />
                </el-select>
              </div>
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
  flex-direction: column;
  justify-content: center;
  align-items: center;
  /* 容器不带底色：一体感由 SearchTable 自身的工具栏面与数据面（含分页面）连成的卡片承担；
     间距纯 margin（原 padding+margin 各 5px 中的 padding 用于把底色铺到内部元素之外，随底色移除退役，总边距 10px 不变） */
  width: calc(100% - 20px);
  height: calc(100% - 20px);
  margin: 10px;
}
.plugin-manage-left-search-table {
  flex: 1;
  min-height: 0;
  width: 100%;
}
/* 工具栏第二行：输入类筛选容器（全宽强制分行，内部可放多个输入控件） */
.plugin-toolbar-filter-row {
  width: 100%;
  display: flex;
  gap: 8px;
}
/* 工具栏折行后筛选控件的宽度约束（无约束的输入框折行点不可控） */
.plugin-name-search-input {
  width: 220px;
}
.plugin-author-search-input {
  width: 180px;
}
.plugin-source-search-select {
  width: 140px;
}
/* 第三方免责提示行（插件列表上方 muted 小字） */
.plugin-disclaimer {
  flex-shrink: 0;
  width: 100%;
  color: var(--app-text-secondary);
  font-size: 12px;
  margin-bottom: 5px;
  box-sizing: border-box;
}
/* 检查更新待办区块（有待办时显示于表格上方） */
.plugin-pending-panel {
  flex-shrink: 0;
  width: 100%;
  max-height: 30%;
  overflow-y: auto;
  /* 容器去底色后自带卡片面，与下方 SearchTable 卡片各自成块 */
  background: var(--app-bg-surface);
  border-radius: var(--app-radius);
  margin-bottom: 5px;
  padding: 5px 10px;
  box-sizing: border-box;
}
.plugin-pending-group-title {
  color: var(--app-text-secondary);
  font-size: 12px;
  margin: 5px 0;
}
.plugin-pending-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 4px 0;
}
.plugin-pending-name {
  color: var(--app-text-primary);
  min-width: 140px;
}
.plugin-pending-version {
  color: var(--app-text-regular);
  flex: 1;
}
.plugin-pending-message {
  color: var(--app-text-regular);
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
