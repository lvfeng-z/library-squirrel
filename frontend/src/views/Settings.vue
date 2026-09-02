<script setup lang="ts">
import BaseView from './BaseView.vue'
import { computed, nextTick, onBeforeMount, onBeforeUnmount, Ref, ref } from 'vue'
import { useRouter } from 'vue-router'
import lodash from 'lodash'
import {Settings} from "@bindings/github.com/library-squirrel/backend/settings";
import ApiUtil from '@renderer/utils/ApiUtil.ts'
import { arrayNotEmpty, isNullish, notNullish } from '@renderer/utils/CommonUtil.ts'
import { ElMessage, ElMessageBox } from 'element-plus'
import ResFileNameFormatEnum from '@renderer/constants/ResFileNameFormatEnum.ts'
import { useTourTargets } from '@renderer/composables/useTourTargets'
import { useTourCenterStore } from '@renderer/store/UseTourCenterStore'
import { settingsApi, fileSysUtilApi, fsmonitorApi, workdirGuardApi } from '@renderer/apis/http'
import { shareProtocolStatus, shareUnregisterProtocol } from '@renderer/apis/http/wrappers/share'
import type { ShareProtocolRegStatus } from '@bindings/github.com/library-squirrel/backend/share/models'
import {emptySettings} from "@renderer/model/util/Settings.js";
import { useThemeStore } from '@renderer/store/UseThemeStore.ts'
import { useWorkdirStatusStore } from '@renderer/store/UseWorkdirStatusStore.ts'
import { isBlank, isNotBlank } from '@renderer/utils/StringUtil.ts'
import type { ThemeId } from '@renderer/theme/themes'
import type { AutoRepairPolicyDTO } from '@bindings/github.com/library-squirrel/backend/fsmonitor/models'
import type { GuardInfoResponse } from '@bindings/github.com/library-squirrel/backend/workdirGuard/models'

// onBeforeMount
onBeforeMount(() => {
  loadSettings()
  void loadProtocolStatus()
})

// 变量
const apis = {
  fileSysUtilSelectDirectory: fileSysUtilApi.fileSysUtilSelectDirectory,
  settingsGetSettings: settingsApi.settingsGetSettings,
  settingsSaveSettings: settingsApi.settingsSaveSettings,
  settingsResetSettings: settingsApi.settingsResetSettings,
  fsmonitorGetAutoRepairPolicySchema: fsmonitorApi.fsmonitorGetAutoRepairPolicySchema,
  workdirGuardGetInfo: workdirGuardApi.workdirGuardGetInfo
} // 接口
// 工作目录输入组件实例
const workdirInput = ref()
// 向导目标注册
const { register: registerTourTarget } = useTourTargets()
registerTourTarget('settings.workdirInput', workdirInput)
// 主要容器的实例
const containerRef = ref()
// 作品文件名称命名格式输入组件实例
const workSettingsFileNameFormatInput = ref()
// 作品文件名称命名格式输入弹窗里的组件实例
const workSettingsFileNameFormatDialogInput = ref()
// 设置
const settings: Ref<Settings> = ref(emptySettings)
let oldSettings: Settings = emptySettings // 原设置
// 主题（即时生效，独立于设置的保存流程，切换时由 store 自行持久化）
const themeStore = useThemeStore()
// 自动修复策略 schema（可选项由后端 apply 能力约束，前端不写死）
const policySchema = ref<AutoRepairPolicyDTO[]>([])
// 有选择空间的策略组合（options 多于一项）才渲染下拉；Delete 单选项与 Untracked 不可配置不渲染
const configurablePolicies = computed(() => policySchema.value.filter((p) => p.options.length > 1))
// 目录保护探测结果（平台机制信息 + 当前 workDir 可写性）
const guardInfo = ref<GuardInfoResponse | null>(null)
const guardLoading = ref(false)
// 修复动作可读名（与确认弹窗文案对齐）
const REPAIR_ACTION_LABEL: Record<string, string> = {
  sync: '同步路径',
  restore: '复原',
  ack: '确认失效'
}
async function handleSelectTheme(id: ThemeId) {
  await themeStore.setTheme(id)
}
// 深链协议注册状态（便携版运行时自注册视图；安装版由安装/卸载器管理 HKLM 键）
const protocolStatus = ref<ShareProtocolRegStatus | null>(null)
// 拉取深链协议注册状态（失败静默——非核心功能不阻塞设置页）
async function loadProtocolStatus(): Promise<void> {
  try {
    protocolStatus.value = await shareProtocolStatus()
  } catch (e) {
    console.warn('查询深链协议注册状态失败', e)
  }
}
// 取消深链协议注册（便携版无卸载器的清理入口；应用每次启动会重新自注册）
async function handleUnregisterProtocol(): Promise<void> {
  const confirm = await ElMessageBox.confirm(
    '取消后浏览器中的分享链接将不再唤起本应用（下次启动应用时会重新注册）。',
    '取消深链协议注册',
    { confirmButtonText: '取消注册', cancelButtonText: '保留', type: 'warning' }
  ).then(() => true).catch(() => false)
  if (!confirm) return
  try {
    await shareUnregisterProtocol()
    ElMessage.success('已取消深链协议注册')
    await loadProtocolStatus()
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '取消注册失败')
  }
}
// 作品文件名称命名格式对话框开关
const workSettingsFileNameFormatDialogState: Ref<boolean> = ref(false)
// 路由实例
const router = useRouter()

// 方法
// autoRepairPolicies 为 map：递归 diff 只遍历旧键，新键不产出变更——整表比较判定变更
function hasPolicyChanges(): boolean {
  const policies = settings.value.fsmonitor.autoRepairPolicies ?? {}
  const oldPolicies = oldSettings.fsmonitor.autoRepairPolicies ?? {}
  return !lodash.isEqual(policies, oldPolicies)
}
// 检查设置是否有未保存的更改
function hasUnsavedChanges(): boolean {
  const changed = getChangedProperties(settings.value, oldSettings)
  return arrayNotEmpty(changed) || hasPolicyChanges()
}

// 路由守卫 - 离开前检查未保存的更改
async function handleBeforeRouteLeave() {
  if (hasUnsavedChanges()) {
    const confirm = await askBeforeSave()
    if (confirm) {
      // 用户选择保存
      await saveSettings()
      return true
    }
    // 用户选择不保存
    return true
  }
  return true
}

// 离开前询问是否保存
async function askBeforeSave(): Promise<boolean> {
  return new Promise<boolean>((resolve) => {
    ElMessageBox.confirm('是否保存更改?', '更改未保存', {
      confirmButtonText: '是',
      cancelButtonText: '否',
      type: 'warning'
    })
      .then(() => {
        resolve(true)
      })
      .catch(() => {
        resolve(false)
      })
  })
}

// 注册路由守卫
const leaveGuard = router.beforeEach(handleBeforeRouteLeave)

// 组件卸载前移除路由守卫
onBeforeUnmount(() => {
  leaveGuard()
})
// 加载设置
async function loadSettings() {
  const response = await apis.settingsGetSettings()
  if (ApiUtil.check(response)) {
    const data = ApiUtil.data<Settings>(response)
    settings.value = isNullish(data) ? emptySettings : data
    oldSettings = lodash.cloneDeep(settings.value)
  } else {
    ElMessage({
      message: '获取设置失败',
      type: 'error'
    })
  }
  // 设置就绪后同步加载策略 schema 与目录保护探测（workDir 变更保存后经此刷新）
  await loadPolicySchema()
  await probeWorkDirGuard()
}
// 加载自动修复策略 schema
async function loadPolicySchema() {
  try {
    const response = await apis.fsmonitorGetAutoRepairPolicySchema()
    policySchema.value = response.data
  } catch (e: any) {
    ElMessage.error(e.message)
  }
}
// 探测目录保护（读当前 workdir；workdir 为空时后端跳过探测仅返回机制信息）
async function probeWorkDirGuard() {
  guardLoading.value = true
  try {
    const response = await apis.workdirGuardGetInfo(settings.value.workdir ?? '')
    guardInfo.value = response.data
  } catch (e: any) {
    ElMessage.error(e.message)
  } finally {
    guardLoading.value = false
  }
}
// 策略下拉变更写入（写入 autoRepairPolicies 覆盖表）
function setAutoRepairPolicy(key: string, value: string) {
  settings.value.fsmonitor.autoRepairPolicies[key] = value
}
// 修复动作可读名（未登记回退原值）
function actionLabel(action: string): string {
  return REPAIR_ACTION_LABEL[action] ?? action
}
// 保存设置
async function saveSettings() {
  const changed = getChangedProperties(settings.value, oldSettings)
  // autoRepairPolicies 是 map：递归 diff 仅遍历旧键，新键不产出变更路径——整表比较后整体提交
  const changedWithoutPolicies = changed.filter((c) => !c.path.startsWith('fsmonitor.autoRepairPolicies'))
  if (hasPolicyChanges()) {
    changedWithoutPolicies.push({ path: 'fsmonitor.autoRepairPolicies', value: settings.value.fsmonitor.autoRepairPolicies ?? {} })
  }
  // 保存前的旧工作目录快照（loadSettings 重载会覆盖 oldSettings，转换判定须先取）
  const previousWorkdir = oldSettings.workdir
  const response = await apis.settingsSaveSettings(changedWithoutPolicies)
  await loadSettings()
  if (ApiUtil.check(response)) {
    const succeed = ApiUtil.data<boolean>(response)
    if (succeed) {
      ElMessage({
        message: '修改成功',
        type: 'success'
      })
      // 同步工作目录未配置状态：已配置收起常驻横幅；清空则升横幅
      void useWorkdirStatusStore().refresh()
      // 未配置→已配置：文件监控与回收站自动清理不随保存启动（转换按重启生效落地），提示重启
      if (isBlank(previousWorkdir) && isNotBlank(settings.value.workdir)) {
        ElMessage({
          message: '工作目录已配置，文件监控与回收站自动清理将在重启应用后生效',
          type: 'info',
          duration: 6000
        })
      }
    } else {
      ElMessage({
        message: '修改失败',
        type: 'error'
      })
    }
  }
}
// 所有设置重置为默认
async function resetSettings() {
  const confirm = await askBeforeReset()
  if (confirm) {
    const response = await apis.settingsResetSettings()
    await loadSettings()
    // 设置重置后同步主题状态（appearance.theme 回到默认）
    await themeStore.load()
    if (ApiUtil.check(response)) {
      const succeed = ApiUtil.data<boolean>(response)
      if (succeed) {
        ElMessage({
          message: '重置成功',
          type: 'success'
        })
        // 重置清空工作目录：同步未配置状态（升常驻横幅）
        void useWorkdirStatusStore().refresh()
      } else {
        ElMessage({
          message: '重置失败',
          type: 'error'
        })
      }
    }
  }
}
// 递归获取已更改的设置
function getChangedProperties(newVal: object, oldVal: object, root?: string) {
  let changedProperties: { path: string; value: unknown }[] = []
  for (const key of Object.keys(oldVal)) {
    const newRoot = root === undefined ? key : root + '.' + key
    if (Object.prototype.hasOwnProperty.call(newVal, key)) {
      if (typeof oldVal[key] === 'object') {
        const children = getChangedProperties(newVal[key], oldVal[key], newRoot)
        changedProperties = [...changedProperties, ...children]
      } else {
        if (oldVal[key] !== newVal[key]) {
          changedProperties.push({ path: newRoot, value: newVal[key] })
        }
      }
    } else {
      changedProperties.push({ path: newRoot, value: undefined })
    }
  }

  return changedProperties
}
// 选择目录
async function selectDir() {
  const response = await apis.fileSysUtilSelectDirectory('选择工作目录')
  if (ApiUtil.check(response)) {
    const dirSelectResult = ApiUtil.data(response) as { canceled: boolean; filePaths: string[] }
    if (!dirSelectResult.canceled && arrayNotEmpty(dirSelectResult.filePaths) && notNullish(settings.value)) {
      settings.value.workdir = dirSelectResult.filePaths[0]
    }
  }
}
// 选择导出默认目录（持久化随设置整体保存流程）
async function selectExportDir() {
  const response = await apis.fileSysUtilSelectDirectory('选择导出默认目录')
  if (ApiUtil.check(response)) {
    const dirSelectResult = ApiUtil.data(response) as { canceled: boolean; filePaths: string[] }
    if (!dirSelectResult.canceled && arrayNotEmpty(dirSelectResult.filePaths) && notNullish(settings.value)) {
      settings.value.exportSettings.outputDir = dirSelectResult.filePaths[0]
    }
  }
}
// 重置导出默认目录（回退：不设置即使用工作目录）
function resetExportDir() {
  if (notNullish(settings.value)) {
    settings.value.exportSettings.outputDir = ''
  }
}
// 重置前询问
async function askBeforeReset(): Promise<boolean> {
  return new Promise<boolean>((resolve) => {
    ElMessageBox.confirm('所有设置将重置到默认状态', '是否重置?', {
      confirmButtonText: '是',
      cancelButtonText: '否',
      type: 'warning'
    })
      .then(() => {
        resolve(true)
      })
      .catch(() => {
        ElMessage({
          message: '取消重置',
          type: 'warning'
        })
        resolve(false)
      })
  })
}
// 作品-添加命名标识符
function insertFormatToken(element: ResFileNameFormatEnum, isDialog: boolean) {
  let inputElement: HTMLInputElement
  if (isDialog) {
    inputElement = workSettingsFileNameFormatDialogInput.value.textarea
  } else {
    inputElement = workSettingsFileNameFormatInput.value.input
  }
  if (inputElement) {
    const startPos = inputElement.selectionStart // 光标起始位置
    const endPos = inputElement.selectionEnd // 光标结束位置

    if (notNullish(startPos) && notNullish(endPos)) {
      // 插入字符串到光标位置
      settings.value.workSettings.fileNameFormat =
        settings.value.workSettings.fileNameFormat.slice(0, startPos) +
        element.token +
        settings.value.workSettings.fileNameFormat.slice(endPos)

      // 设置新的光标位置
      const newCursorPos = startPos + element.token.length
      inputElement.focus() // 确保输入框保持焦点
      nextTick(() => inputElement.setSelectionRange(newCursorPos, newCursorPos))
    }
  }
}
</script>

<template>
  <base-view>
    <template #default>
      <el-container class="settings-container">
        <el-main style="display: flex; flex-direction: row; padding: 0">
          <el-anchor
            :container="containerRef?.parentElement?.parentElement"
            direction="vertical"
            type="default"
            :offset="30"
            @click="(e: MouseEvent) => e.preventDefault()"
          >
            <el-anchor-link
              href="#basicSettings"
              title="基本设置"
            />
            <el-anchor-link
              href="#appearanceSettings"
              title="外观"
            />
            <el-anchor-link
              href="#downloadSettings"
              title="下载"
            />
            <el-anchor-link
              href="#workSettings"
              title="作品"
            />
            <el-anchor-link
              href="#pluginSettings"
              title="插件"
            />
            <el-anchor-link
              href="#recycleBinSettings"
              title="回收站"
            />
            <el-anchor-link
              href="#backupGovernanceSettings"
              title="备份治理"
            />
            <el-anchor-link
              href="#otherSettings"
              title="其他"
            />
          </el-anchor>
          <el-scrollbar class="settings-scrollbar">
            <div
              ref="containerRef"
              class="settings-scrollbar-container"
            >
              <div id="basicSettings">
                <el-text
                  class="mx-1"
                  size="large"
                  tag="b"
                >
                  基本设置
                </el-text>
                <el-divider
                  content-position="left"
                  border-style="dotted"
                >
                  <el-text>工作目录</el-text>
                </el-divider>
                <el-tooltip
                  placement="top"
                  effect="customized"
                  content="LibrarySquirrel所管理的所有资源都会被保存到这个目录下，请确保这个目录有足够的空间，并且非必要的情况下不要更改此项"
                >
                  <el-row>
                    <el-col :span="22">
                      <el-input
                        ref="workdirInput"
                        v-model="settings.workdir"
                      />
                    </el-col>
                    <el-col :span="1">
                      <el-button
                        icon="FolderOpened"
                        @click="selectDir"
                      />
                    </el-col>
                    <el-col :span="1">
                      <el-button
                        type="danger"
                        class="tone-fail"
                        icon="RefreshLeft"
                        @click="settings.workdir = oldSettings.workdir"
                      />
                    </el-col>
                  </el-row>
                </el-tooltip>
                <el-card
                  shadow="never"
                  class="settings-guard-card"
                >
                  <template #header>
                    <div class="settings-guard-card-header">
                      <span>目录保护</span>
                      <el-button
                        size="small"
                        :loading="guardLoading"
                        @click="probeWorkDirGuard"
                      >
                        重新检测
                      </el-button>
                    </div>
                  </template>
                  <template v-if="notNullish(guardInfo)">
                    <div class="settings-guard-mechanism">
                      <span class="settings-guard-mechanism-name">防护机制：{{ guardInfo.info.mechanism }}</span>
                      <el-tag
                        size="small"
                        :type="guardInfo.info.supported ? 'success' : 'info'"
                      >
                        {{ guardInfo.info.supported ? '受支持' : '不支持' }}
                      </el-tag>
                    </div>
                    <el-alert
                      v-if="guardInfo.probeOk"
                      class="settings-guard-alert"
                      title="探测通过：工作目录当前可写，未被系统保护机制拦截"
                      type="success"
                      :closable="false"
                      show-icon
                    />
                    <el-alert
                      v-else-if="guardInfo.probeErr"
                      class="settings-guard-alert"
                      :title="`探测失败：${guardInfo.probeErr}`"
                      type="warning"
                      :closable="false"
                      show-icon
                    />
                    <el-alert
                      v-else
                      class="settings-guard-alert"
                      title="工作目录未配置，跳过探测"
                      type="info"
                      :closable="false"
                      show-icon
                    />
                    <div
                      v-if="guardInfo.info.guide"
                      class="settings-guard-guide"
                    >
                      {{ guardInfo.info.guide }}
                    </div>
                  </template>
                </el-card>
                <el-divider
                  content-position="left"
                  border-style="dotted"
                >
                  <el-text>导出默认目录</el-text>
                </el-divider>
                <el-tooltip
                  placement="top"
                  effect="customized"
                  content="导出作品时 ZIP 默认保存到该目录；不设置则使用工作目录。导出弹窗内仍可临时改为其他目录（仅本次有效，不改变此处默认值）。"
                >
                  <el-row>
                    <el-col :span="22">
                      <el-input
                        v-model="settings.exportSettings.outputDir"
                        placeholder="默认：工作目录"
                      />
                    </el-col>
                    <el-col :span="1">
                      <el-button
                        icon="FolderOpened"
                        @click="selectExportDir"
                      />
                    </el-col>
                    <el-col :span="1">
                      <el-button
                        type="danger"
                        class="tone-fail"
                        icon="RefreshLeft"
                        @click="resetExportDir"
                      />
                    </el-col>
                  </el-row>
                </el-tooltip>
                <el-divider
                  content-position="left"
                  border-style="dotted"
                >
                  <el-text>分享中继地址</el-text>
                </el-divider>
                <el-tooltip
                  placement="top"
                  effect="customized"
                  content="分享功能的盲转中继地址（host 或 host:port，可带 https:// 前缀；未写端口默认 9527）。官方中继为默认值，可改为自建中继。"
                >
                  <el-input
                    v-model="settings.shareSettings.relayAddress"
                    placeholder="relay.example.com"
                    clearable
                  />
                </el-tooltip>
                <el-divider
                  content-position="left"
                  border-style="dotted"
                >
                  <el-text>深链协议注册</el-text>
                </el-divider>
                <el-tooltip
                  placement="top"
                  effect="customized"
                  content="library-squirrel:// 分享链接的唤起注册：安装版随安装器写入（卸载时清理），便携版由应用启动时自注册。此入口仅取消便携自注册（HKCU），下次启动会重新注册。"
                >
                  <div class="settings-protocol-row">
                    <el-text
                      size="small"
                      :type="protocolStatus?.registered ? 'success' : 'info'"
                    >
                      {{ protocolStatus?.registered
                        ? (protocolStatus.currentExe ? '已注册（当前程序）' : '已注册（其他程序路径）')
                        : '未注册（非 Windows 平台或未自注册）' }}
                    </el-text>
                    <el-button
                      size="small"
                      type="danger"
                      class="tone-fail"
                      :disabled="!protocolStatus?.registered"
                      @click="handleUnregisterProtocol"
                    >
                      取消注册
                    </el-button>
                  </div>
                </el-tooltip>
                <el-divider
                  content-position="left"
                  border-style="dotted"
                >
                  <el-text>USN 离线追溯（实验性）</el-text>
                  <el-switch
                    v-model="settings.fsmonitor.usnEnabled"
                    class="settings-element-in-divider"
                    inline-prompt
                    size="large"
                    active-text="开"
                    inactive-text="关"
                  />
                </el-divider>
                <el-tooltip
                  placement="top"
                  effect="customized"
                >
                  <template #content>
                    开启后，软件未运行期间对工作目录的文件操作将通过 Windows USN Journal 精确追溯（区别于默认的全量对账，能区分"本次离线变的"与"历史遗留不一致"）。<br>
                    <b>需以管理员身份运行</b>，非管理员运行时自动降级为全量对账。仅 Windows 支持。
                  </template>
                  <el-text
                    type="info"
                    size="small"
                  >
                    什么是 USN 离线追溯？
                  </el-text>
                </el-tooltip>
                <el-divider
                  content-position="left"
                  border-style="dotted"
                >
                  <el-text>操作抑制</el-text>
                  <el-switch
                    v-model="settings.fsmonitor.suppressEnabled"
                    class="settings-element-in-divider"
                    inline-prompt
                    size="large"
                    active-text="开"
                    inactive-text="关"
                  />
                </el-divider>
                <el-tooltip
                  placement="top"
                  effect="customized"
                >
                  <template #content>
                    开启后，软件自身的 store/ 写入（下载落盘、合并、还原等）不会被监控误报为外部变更。仅在排查误报/漏报问题时关闭——关闭后内部写入会重新触发"外部新增"误报（对账兜底，不致数据损坏）。
                  </template>
                  <el-text
                    type="info"
                    size="small"
                  >
                    什么是操作抑制？
                  </el-text>
                </el-tooltip>
                <el-divider
                  content-position="left"
                  border-style="dotted"
                >
                  <el-text>自动修复</el-text>
                  <el-switch
                    v-model="settings.fsmonitor.autoRepairEnabled"
                    class="settings-element-in-divider"
                    inline-prompt
                    size="large"
                    active-text="开"
                    inactive-text="关"
                  />
                </el-divider>
                <el-tooltip
                  placement="top"
                  effect="customized"
                >
                  <template #content>
                    开启后，软件运行期间检测到的路径层面外部变更（资源文件/目录移动、备份文件移动）将按下方策略自动处理，不再逐条弹窗确认；自动执行失败会降级为人工确认。离线对账发现的变更始终保留人工确认。
                  </template>
                  <el-text
                    type="info"
                    size="small"
                  >
                    什么是自动修复？
                  </el-text>
                </el-tooltip>
                <div
                  v-if="settings.fsmonitor.autoRepairEnabled"
                  class="settings-auto-repair-policies"
                >
                  <div
                    v-for="p in configurablePolicies"
                    :key="p.key"
                    class="settings-auto-repair-policy-row"
                  >
                    <span class="settings-auto-repair-policy-label">{{ p.label }}</span>
                    <el-select
                      :model-value="settings.fsmonitor.autoRepairPolicies[p.key] ?? p.default"
                      class="settings-auto-repair-policy-select"
                      size="small"
                      @update:model-value="(v: string) => setAutoRepairPolicy(p.key, v)"
                    >
                      <el-option
                        v-for="opt in p.options"
                        :key="opt"
                        :label="actionLabel(opt)"
                        :value="opt"
                      />
                    </el-select>
                  </div>
                </div>
                <el-divider />
              </div>
              <div id="appearanceSettings">
                <el-text
                  class="mx-1"
                  size="large"
                  tag="b"
                >
                  外观
                </el-text>
                <el-divider
                  content-position="left"
                  border-style="dotted"
                >
                  <el-text>主题</el-text>
                </el-divider>
                <div class="appearance-theme-list">
                  <div
                    v-for="theme in themeStore.themeList"
                    :key="theme.id"
                    :class="['appearance-theme-card', { 'appearance-theme-card-active': theme.id === themeStore.currentThemeId }]"
                    @click="handleSelectTheme(theme.id)"
                  >
                    <div
                      class="appearance-theme-swatch"
                      :style="{ backgroundColor: theme.swatch.bg }"
                    >
                      <div
                        class="appearance-theme-swatch-surface"
                        :style="{ backgroundColor: theme.swatch.surface }"
                      >
                        <div
                          class="appearance-theme-swatch-primary"
                          :style="{ backgroundColor: theme.swatch.primary }"
                        />
                      </div>
                    </div>
                    <el-text>{{ theme.name }}</el-text>
                  </div>
                </div>
                <el-divider />
              </div>
              <div id="downloadSettings">
                <el-text
                  class="mx-1"
                  size="large"
                  tag="b"
                >
                  下载
                </el-text>
                <el-divider
                  content-position="left"
                  border-style="dotted"
                >
                  <el-text>并行下载数</el-text>
                  <el-input-number
                      v-model="settings.importSettings.maxParallelImport"
                      :max="20"
                      :min="1"
                      controls-position="right"
                      class="settings-element-in-divider"
                  />
                </el-divider>
                <el-divider
                  content-position="left"
                  border-style="dotted"
                  class="settings-consecutive-divider"
                >
                  <el-text>重新下载时是否更新作品信息</el-text>
                  <el-switch
                    v-model="settings.importSettings.updateWorkInfoWhenImport"
                    class="settings-element-in-divider"
                    inline-prompt
                    size="large"
                    active-text="是"
                    inactive-text="否"
                  />
                </el-divider>
              </div>
              <div id="workSettings">
                <el-text size="large" tag="b">
                  作品
                </el-text>
                <el-divider
                  content-position="left"
                  border-style="dotted"
                >
                  <el-text>作品的文件命名格式</el-text>
                </el-divider>
                <el-row class="work-settings-file-name-format-button">
                  <el-button @click="insertFormatToken(ResFileNameFormatEnum.AUTHOR, false)">
                    {{ ResFileNameFormatEnum.AUTHOR.name }}
                  </el-button>
                  <el-button @click="insertFormatToken(ResFileNameFormatEnum.LOCAL_AUTHOR_NAME, false)">
                    {{ ResFileNameFormatEnum.LOCAL_AUTHOR_NAME.name }}
                  </el-button>
                  <el-button @click="insertFormatToken(ResFileNameFormatEnum.SITE_AUTHOR_NAME, false)">
                    {{ ResFileNameFormatEnum.SITE_AUTHOR_NAME.name }}
                  </el-button>
                  <el-button @click="insertFormatToken(ResFileNameFormatEnum.SITE_AUTHOR_ID, false)">
                    {{ ResFileNameFormatEnum.SITE_AUTHOR_ID.name }}
                  </el-button>
                  <el-button @click="insertFormatToken(ResFileNameFormatEnum.SITE_WORK_NAME, false)">
                    {{ ResFileNameFormatEnum.SITE_WORK_NAME.name }}
                  </el-button>
                  <el-button @click="insertFormatToken(ResFileNameFormatEnum.SITE_WORK_ID, false)">
                    {{ ResFileNameFormatEnum.SITE_WORK_ID.name }}
                  </el-button>
                  <el-button @click="insertFormatToken(ResFileNameFormatEnum.DESCRIPTION, false)">
                    {{ ResFileNameFormatEnum.DESCRIPTION.name }}
                  </el-button>
                  <el-tooltip :show-after="850">
                    <template #content>
                      查看更多选项
                    </template>
                    <el-button @click="workSettingsFileNameFormatDialogState = true">
                      ...
                    </el-button>
                  </el-tooltip>
                </el-row>
                <el-input
                  ref="workSettingsFileNameFormatInput"
                  v-model="settings.workSettings.fileNameFormat"
                />
                <el-divider />
              </div>
              <div id="pluginSettings">
                <el-text size="large" tag="b">
                  插件
                </el-text>
                <el-divider
                  content-position="left"
                  border-style="dotted"
                >
                  <el-text>运行时编译</el-text>
                  <el-switch
                    v-model="settings.pluginSettings.allowUnsafeEval"
                    class="settings-element-in-divider"
                    inline-prompt
                    size="large"
                    active-text="开"
                    inactive-text="关"
                  />
                </el-divider>
                <el-tooltip
                  placement="top"
                  effect="customized"
                >
                  <template #content>
                    开启此选项可以解决某些插件页面无法正常显示的问题。<br>这会降低应用的安全性，建议仅在遇到页面加载异常时临时开启。
                  </template>
                  <el-text
                    type="info"
                    size="small"
                  >
                    什么是运行时编译？
                  </el-text>
                </el-tooltip>
                <el-divider
                  content-position="left"
                  border-style="dotted"
                >
                  <el-text>受限模式</el-text>
                  <el-switch
                    v-model="settings.pluginSettings.restrictedMode"
                    class="settings-element-in-divider"
                    inline-prompt
                    size="large"
                    active-text="开"
                    inactive-text="关"
                  />
                </el-divider>
                <el-tooltip
                  placement="top"
                  effect="customized"
                >
                  <template #content>
                    开启后，应用启动时仅激活官方捆绑插件，跳过所有第三方插件。<br>用于排查第三方插件问题时的安全启动；重启应用后生效。
                  </template>
                  <el-text
                    type="info"
                    size="small"
                  >
                    什么是受限模式？
                  </el-text>
                </el-tooltip>
                <el-divider />
              </div>
              <div id="recycleBinSettings">
                <el-text size="large" tag="b">
                  回收站
                </el-text>
                <el-divider
                  content-position="left"
                  border-style="dotted"
                >
                  <el-text>自动清理</el-text>
                  <el-switch
                    v-model="settings.recycleBin.autoCleanupEnabled"
                    class="settings-element-in-divider"
                    inline-prompt
                    size="large"
                    active-text="开"
                    inactive-text="关"
                  />
                </el-divider>
                <el-divider
                  content-position="left"
                  border-style="dotted"
                  class="settings-consecutive-divider"
                >
                  <el-text>保留天数</el-text>
                  <el-input-number
                    v-model="settings.recycleBin.retentionDays"
                    :min="1"
                    controls-position="right"
                    class="settings-element-in-divider"
                  />
                </el-divider>
                <el-tooltip
                  placement="top"
                  effect="customized"
                >
                  <template #content>
                    回收站中的作品超过保留天数后将自动彻底删除（不可恢复）。<br>应用启动时检查一次，之后每 24 小时检查一次。
                  </template>
                  <el-text
                    type="info"
                    size="small"
                  >
                    自动清理规则
                  </el-text>
                </el-tooltip>
                <el-divider />
              </div>
              <div id="backupGovernanceSettings">
                <el-text size="large" tag="b">
                  备份治理
                </el-text>
                <el-divider
                  content-position="left"
                  border-style="dotted"
                >
                  <el-text>无主备份保留天数</el-text>
                  <el-input-number
                    v-model="settings.backupGovernance.retentionDays"
                    :min="1"
                    controls-position="right"
                    class="settings-element-in-divider"
                  />
                </el-divider>
                <el-tooltip
                  placement="top"
                  effect="customized"
                >
                  <template #content>
                    不再被任何作品/插件引用的备份超过保留天数后将自动删除（不可恢复）。<br>应用启动时检查一次，之后每 24 小时检查一次；替换任务在途期间其还原点备份受引用保护，不受保留期影响。
                  </template>
                  <el-text
                    type="info"
                    size="small"
                  >
                    清理规则
                  </el-text>
                </el-tooltip>
                <el-divider />
              </div>
              <div id="otherSettings">
                <el-text
                  class="mx-1"
                  size="large"
                  tag="b"
                >
                  其他
                </el-text>
                <el-divider
                  content-position="left"
                  border-style="dotted"
                >
                  <el-text>向导</el-text>
                </el-divider>
                <el-button @click="useTourCenterStore().resetAllCompleted()">
                  重置向导
                </el-button>
                <el-divider />
              </div>
            </div>
          </el-scrollbar>
        </el-main>
        <el-footer height="32px">
          <el-row>
            <el-col :span="6">
              <el-button
                type="primary"
                @click="saveSettings"
              >
                保存
              </el-button>
              <el-button
                type="danger"
                class="tone-fail"
                @click="resetSettings"
              >
                默认设置
              </el-button>
            </el-col>
          </el-row>
        </el-footer>
      </el-container>
    </template>
    <template #dialog>
      <el-dialog
        v-model="workSettingsFileNameFormatDialogState"
        center
        align-center
      >
        <el-scrollbar class="settings-work-settings-file-name-format-dialog">
          <el-button
            class="work-settings-file-name-format-button"
            @click="insertFormatToken(ResFileNameFormatEnum.AUTHOR, true)"
          >
            {{ ResFileNameFormatEnum.AUTHOR.name }}
          </el-button>
          <el-button
            class="work-settings-file-name-format-button"
            @click="insertFormatToken(ResFileNameFormatEnum.LOCAL_AUTHOR_NAME, true)"
          >
            {{ ResFileNameFormatEnum.LOCAL_AUTHOR_NAME.name }}
          </el-button>
          <el-button
            class="work-settings-file-name-format-button"
            @click="insertFormatToken(ResFileNameFormatEnum.SITE_AUTHOR_NAME, true)"
          >
            {{ ResFileNameFormatEnum.SITE_AUTHOR_NAME.name }}
          </el-button>
          <el-button
            class="work-settings-file-name-format-button"
            @click="insertFormatToken(ResFileNameFormatEnum.SITE_AUTHOR_ID, true)"
          >
            {{ ResFileNameFormatEnum.SITE_AUTHOR_ID.name }}
          </el-button>
          <el-button
            class="work-settings-file-name-format-button"
            @click="insertFormatToken(ResFileNameFormatEnum.SITE_WORK_NAME, true)"
          >
            {{ ResFileNameFormatEnum.SITE_WORK_NAME.name }}
          </el-button>
          <el-button
            class="work-settings-file-name-format-button"
            @click="insertFormatToken(ResFileNameFormatEnum.SITE_WORK_ID, true)"
          >
            {{ ResFileNameFormatEnum.SITE_WORK_ID.name }}
          </el-button>
          <el-button
            class="work-settings-file-name-format-button"
            @click="insertFormatToken(ResFileNameFormatEnum.DESCRIPTION, true)"
          >
            {{ ResFileNameFormatEnum.DESCRIPTION.name }}
          </el-button>
          <el-button
            class="work-settings-file-name-format-button"
            @click="insertFormatToken(ResFileNameFormatEnum.UPLOAD_TIME_YEAR, true)"
          >
            {{ ResFileNameFormatEnum.UPLOAD_TIME_YEAR.name }}
          </el-button>
          <el-button
            class="work-settings-file-name-format-button"
            @click="insertFormatToken(ResFileNameFormatEnum.UPLOAD_TIME_MONTH, true)"
          >
            {{ ResFileNameFormatEnum.UPLOAD_TIME_MONTH.name }}
          </el-button>
          <el-button
            class="work-settings-file-name-format-button"
            @click="insertFormatToken(ResFileNameFormatEnum.UPLOAD_TIME_DAY, true)"
          >
            {{ ResFileNameFormatEnum.UPLOAD_TIME_DAY.name }}
          </el-button>
          <el-button
            class="work-settings-file-name-format-button"
            @click="insertFormatToken(ResFileNameFormatEnum.UPLOAD_TIME_HOUR, true)"
          >
            {{ ResFileNameFormatEnum.UPLOAD_TIME_HOUR.name }}
          </el-button>
          <el-button
            class="work-settings-file-name-format-button"
            @click="insertFormatToken(ResFileNameFormatEnum.UPLOAD_TIME_MINUTE, true)"
          >
            {{ ResFileNameFormatEnum.UPLOAD_TIME_MINUTE.name }}
          </el-button>
          <el-button
            class="work-settings-file-name-format-button"
            @click="insertFormatToken(ResFileNameFormatEnum.UPLOAD_TIME_SECOND, true)"
          >
            {{ ResFileNameFormatEnum.UPLOAD_TIME_SECOND.name }}
          </el-button>
          <el-button
            class="work-settings-file-name-format-button"
            @click="insertFormatToken(ResFileNameFormatEnum.DOWNLOAD_TIME_YEAR, true)"
          >
            {{ ResFileNameFormatEnum.DOWNLOAD_TIME_YEAR.name }}
          </el-button>
          <el-button
            class="work-settings-file-name-format-button"
            @click="insertFormatToken(ResFileNameFormatEnum.DOWNLOAD_TIME_MONTH, true)"
          >
            {{ ResFileNameFormatEnum.DOWNLOAD_TIME_MONTH.name }}
          </el-button>
          <el-button
            class="work-settings-file-name-format-button"
            @click="insertFormatToken(ResFileNameFormatEnum.DOWNLOAD_TIME_DAY, true)"
          >
            {{ ResFileNameFormatEnum.DOWNLOAD_TIME_DAY.name }}
          </el-button>
          <el-button
            class="work-settings-file-name-format-button"
            @click="insertFormatToken(ResFileNameFormatEnum.DOWNLOAD_TIME_HOUR, true)"
          >
            {{ ResFileNameFormatEnum.DOWNLOAD_TIME_HOUR.name }}
          </el-button>
          <el-button
            class="work-settings-file-name-format-button"
            @click="insertFormatToken(ResFileNameFormatEnum.DOWNLOAD_TIME_MINUTE, true)"
          >
            {{ ResFileNameFormatEnum.DOWNLOAD_TIME_MINUTE.name }}
          </el-button>
          <el-button
            class="work-settings-file-name-format-button"
            @click="insertFormatToken(ResFileNameFormatEnum.DOWNLOAD_TIME_SECOND, true)"
          >
            {{ ResFileNameFormatEnum.DOWNLOAD_TIME_SECOND.name }}
          </el-button>
          <div class="work-settings-file-name-format-input">
            <el-input
              ref="workSettingsFileNameFormatDialogInput"
              v-model="settings.workSettings.fileNameFormat"
              autosize
              type="textarea"
            />
          </div>
        </el-scrollbar>
      </el-dialog>
    </template>
  </base-view>
</template>

<style scoped>
.settings-container {
  background: var(--app-bg-surface);
  border-radius: var(--app-radius);
  display: flex;
  /* 页面边距统一：卡片距视口 10px（纯 margin，与其余管理页卡片一致）；padding 为本卡内衬保留 */
  width: calc(100% - 20px);
  height: calc(100% - 20px);
  padding: 5px;
  margin: 10px;
}
.settings-scrollbar {
  margin-left: 30px;
  flex-grow: 1;
}
.settings-scrollbar-container {
  margin-right: 10px;
}
.appearance-theme-list {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}
.appearance-theme-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  padding: 10px;
  border: 2px solid var(--app-border-color-light);
  border-radius: var(--app-radius);
  cursor: pointer;
  transition: border-color 0.2s;
}
.appearance-theme-card:hover {
  border-color: var(--app-color-primary-light-5);
}
.appearance-theme-card-active {
  border-color: var(--app-color-primary);
}
.appearance-theme-swatch {
  width: 96px;
  height: 56px;
  border-radius: var(--app-radius-sm);
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: center;
}
.appearance-theme-swatch-surface {
  width: 64px;
  height: 36px;
  border-radius: var(--app-radius-sm);
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: var(--app-shadow-sm);
}
.appearance-theme-swatch-primary {
  width: 24px;
  height: 24px;
  border-radius: 50%;
}
.settings-work-settings-file-name-format-dialog > :deep(.el-scrollbar__wrap) {
  max-height: 65vh;
}
.settings-consecutive-divider {
  margin-top: 40px
}
.settings-element-in-divider {
  margin-left: 20px;
}
.work-settings-file-name-format-button {
  margin-bottom: 10px;
}
.settings-protocol-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  width: 100%;
}
.work-settings-file-name-format-input {
  padding-right: 10px;
}
.settings-guard-card {
  margin-top: 12px;
}
.settings-guard-card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.settings-guard-mechanism {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 10px;
}
.settings-guard-mechanism-name {
  font-size: 13px;
  color: var(--app-text-primary);
}
.settings-guard-alert {
  margin-bottom: 10px;
}
.settings-guard-guide {
  font-size: 12px;
  line-height: 1.6;
  color: var(--app-text-secondary);
  white-space: pre-wrap;
}
.settings-auto-repair-policies {
  margin-top: 12px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.settings-auto-repair-policy-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.settings-auto-repair-policy-label {
  font-size: 13px;
  color: var(--app-text-primary);
}
.settings-auto-repair-policy-select {
  width: 160px;
}
</style>
<style>
.el-popper.is-customized {
  background: var(--app-color-warning-light-7);
}

.el-popper.is-customized .el-popper__arrow::before {
  background: var(--app-color-warning-light-7);
}
</style>
