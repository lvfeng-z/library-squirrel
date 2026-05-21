<script setup lang="ts">
import BaseSubpage from './BaseSubpage.vue'
import { nextTick, onBeforeMount, onBeforeUnmount, Ref, ref } from 'vue'
import { useRouter } from 'vue-router'
import lodash from 'lodash'
import { emptySettings, Settings } from '@renderer/model/model/base/Settings.ts'
import ApiUtil from '@renderer/utils/ApiUtil.ts'
import { arrayNotEmpty, isNullish, notNullish } from '@renderer/utils/CommonUtil.ts'
import { ElMessage, ElMessageBox } from 'element-plus'
import ApiResponse from '@renderer/model/util/ApiResponse.ts'
import ResFileNameFormatEnum from '@renderer/constants/ResFileNameFormatEnum.ts'
import { useTourStatesStore } from '@renderer/store/UseTourStatesStore.ts'
import { settingsApi, fileSysUtilApi } from '@renderer/apis/http'
import {fileSysUtilDirSelect} from "@apis/http/wrappers/fileSysUtil.ts";

// onBeforeMount
onBeforeMount(() => {
  loadSettings()
})

// 变量
const apis = {
  fileSysUtilDirSelect: fileSysUtilApi.fileSysUtilDirSelect,
  settingsGetSettings: settingsApi.settingsGetSettings,
  settingsSaveSettings: settingsApi.settingsSaveSettings,
  settingsResetSettings: settingsApi.settingsResetSettings
} // 接口
// 工作目录输入组件实例
const workdirInput = ref()
// 主要容器的实例
const containerRef = ref()
// 作品文件名称命名格式输入组件实例
const workSettingsFileNameFormatInput = ref()
// 作品文件名称命名格式输入弹窗里的组件实例
const workSettingsFileNameFormatDialogInput = ref()
// 设置
const settings: Ref<Settings> = ref(emptySettings)
let oldSettings: Settings = emptySettings // 原设置
// 作品文件名称命名格式对话框开关
const workSettingsFileNameFormatDialogState: Ref<boolean> = ref(false)
// 路由实例
const router = useRouter()

// 方法
// 检查设置是否有未保存的更改
function hasUnsavedChanges(): boolean {
  const changed = getChangedProperties(settings.value, oldSettings)
  return arrayNotEmpty(changed)
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
}
// 保存或重置设置
async function saveOrReset(fun: (arg?: unknown) => Promise<ApiResponse>) {
  if (notNullish(settings.value)) {
    const changed = getChangedProperties(settings.value, oldSettings)
    const response = await fun(changed)
    if (ApiUtil.check(response)) {
      const succeed = ApiUtil.data<boolean>(response)
      if (succeed) {
        ElMessage({
          message: '修改成功',
          type: 'success'
        })
      } else {
        ElMessage({
          message: '修改失败',
          type: 'error'
        })
      }
    }
  }
}
// 保存设置
async function saveSettings() {
  return saveOrReset(apis.settingsSaveSettings).then(() => loadSettings())
}
// 所有设置重置为默认
async function resetSettings() {
  const confirm = await askBeforeReset()
  if (confirm) {
    return saveOrReset(apis.settingsResetSettings).then(() => loadSettings())
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
  const response = await apis.fileSysUtilDirSelect(false)
  if (ApiUtil.check(response)) {
    const dirSelectResult = ApiUtil.data(response) as { canceled: boolean; filePaths: string[] }
    if (!dirSelectResult.canceled && arrayNotEmpty(dirSelectResult.filePaths) && notNullish(settings.value)) {
      settings.value.workdir = dirSelectResult.filePaths[0]
    }
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
  <base-subpage>
    <template #default>
      <el-container class="settings-container">
        <el-main style="display: flex; flex-direction: row; padding: 0">
          <el-anchor :container="containerRef?.parentElement?.parentElement" direction="vertical" type="default" :offset="30" @click="(e: MouseEvent) => e.preventDefault()">
            <el-anchor-link href="#basicSettings" title="基本设置" />
            <el-anchor-link href="#downloadSettings" title="下载" />
            <el-anchor-link href="#workSettings" title="作品" />
            <el-anchor-link href="#pluginSettings" title="插件" />
            <el-anchor-link href="#otherSettings" title="其他" />
          </el-anchor>
          <el-scrollbar class="settings-scrollbar">
            <div ref="containerRef" class="settings-scrollbar-container">
              <div id="basicSettings">
                <el-text class="mx-1" size="large">基本设置</el-text>
                <el-divider content-position="left" border-style="dotted"><el-text>工作目录</el-text></el-divider>
                <el-tooltip
                  placement="top"
                  effect="customized"
                  content="LibrarySquirrel所管理的所有资源都会被保存到这个目录下，请确保这个目录有足够的空间，并且非必要的情况下不要更改此项"
                >
                  <el-row>
                    <el-col :span="22">
                      <el-input ref="workdirInput" v-model="settings.workdir"></el-input>
                    </el-col>
                    <el-col :span="1">
                      <el-button icon="FolderOpened" @click="selectDir"></el-button>
                    </el-col>
                    <el-col :span="1">
                      <el-button type="danger" icon="RefreshLeft" @click="settings.workdir = oldSettings.workdir"></el-button>
                    </el-col>
                  </el-row>
                </el-tooltip>
                <el-divider />
              </div>
              <div id="downloadSettings">
                <el-text class="mx-1" size="large">下载</el-text>
                <el-divider content-position="left" border-style="dotted"><el-text>并行下载数</el-text></el-divider>
                <el-input-number
                  v-model="settings.importSettings.maxParallelImport"
                  :max="20"
                  :min="1"
                  controls-position="right"
                ></el-input-number>
                <el-divider content-position="left" border-style="dotted">
                  <el-text>重新下载时是否更新作品信息</el-text>
                  <el-switch
                    v-model="settings.importSettings.updateWorkInfoWhenImport"
                    class="work-settings-update-work-info-when-import-switch"
                    inline-prompt
                    size="large"
                    active-text="是"
                    inactive-text="否"
                  ></el-switch
                ></el-divider>
              </div>
              <div id="workSettings">
                <el-text size="large">作品</el-text>
                <el-divider content-position="left" border-style="dotted"><el-text>作品的文件命名格式</el-text></el-divider>
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
                    <template #content>查看更多选项</template>
                    <el-button @click="workSettingsFileNameFormatDialogState = true">...</el-button>
                  </el-tooltip>
                </el-row>
                <el-input ref="workSettingsFileNameFormatInput" v-model="settings.workSettings.fileNameFormat"></el-input>
                <el-divider />
              </div>
              <div id="pluginSettings">
                <el-text size="large">插件</el-text>
                <el-divider content-position="left" border-style="dotted">
                  <el-text>运行时编译</el-text>
                  <el-switch
                    v-model="settings.pluginSettings.allowUnsafeEval"
                    class="plugin-settings-allow-unsafe-eval-switch"
                    inline-prompt
                    size="large"
                    active-text="开"
                    inactive-text="关"
                  ></el-switch
                ></el-divider>
                <el-tooltip placement="top" effect="customized">
                  <template #content>
                    开启此选项可以解决某些插件页面无法正常显示的问题。<br />这会降低应用的安全性，建议仅在遇到页面加载异常时临时开启。
                  </template>
                  <el-text type="info" size="small">什么是运行时编译？</el-text>
                </el-tooltip>
                <el-divider />
              </div>
              <div id="otherSettings">
                <el-text class="mx-1" size="large">其他</el-text>
                <el-divider content-position="left" border-style="dotted"><el-text>向导</el-text></el-divider>
                <el-button @click="() => (settings.tour.firstTimeTourPassed = false)">重置向导</el-button>
                <el-divider />
              </div>
            </div>
          </el-scrollbar>
        </el-main>
        <el-footer height="32px">
          <el-row>
            <el-col :span="6">
              <el-button type="primary" @click="saveSettings">保存</el-button>
              <el-button type="danger" @click="resetSettings">默认设置</el-button>
            </el-col>
          </el-row>
        </el-footer>
      </el-container>
      <el-tour
        v-model="useTourStatesStore().tourStates.workdirTour"
        :scroll-into-view-options="true"
        @finish="useTourStatesStore().tourStates.getCallback('workdirTour')"
      >
        <el-tour-step :target="workdirInput?.$el" title="工作目录" description="在这里设置工作目录"></el-tour-step>
      </el-tour>
    </template>
    <template #dialog>
      <el-dialog v-model="workSettingsFileNameFormatDialogState" center align-center>
        <el-scrollbar class="settings-work-settings-file-name-format-dialog">
          <el-button class="work-settings-file-name-format-button" @click="insertFormatToken(ResFileNameFormatEnum.AUTHOR, true)">
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
          <el-button class="work-settings-file-name-format-button" @click="insertFormatToken(ResFileNameFormatEnum.DESCRIPTION, true)">
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
            ></el-input>
          </div>
        </el-scrollbar>
      </el-dialog>
    </template>
  </base-subpage>
</template>

<style scoped>
.settings-container {
  border-radius: 6px;
  display: flex;
  width: calc(100% - 20px);
  height: calc(100% - 20px);
  padding: 5px;
  margin: 5px;
}
.settings-scrollbar {
  margin-left: 30px;
  flex-grow: 1;
}
.settings-scrollbar-container {
  margin-right: 10px;
}
.settings-work-settings-file-name-format-dialog > :deep(.el-scrollbar__wrap) {
  max-height: 65vh;
}
.work-settings-update-work-info-when-import-switch {
  margin-left: 20px;
}
.plugin-settings-allow-unsafe-eval-switch {
  margin-left: 20px;
}
.work-settings-file-name-format-button {
  margin-bottom: 10px;
}
.work-settings-file-name-format-input {
  padding-right: 10px;
}
</style>
<style>
.el-popper.is-customized {
  background: var(--el-color-warning-light-7);
}

.el-popper.is-customized .el-popper__arrow::before {
  background: var(--el-color-warning-light-7);
}
</style>
