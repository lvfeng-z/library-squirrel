<script setup lang="ts">
import { ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import AutoHeightDialog from '@renderer/components/dialogs/AutoHeightDialog.vue'
import PluginSettingForm from '@renderer/components/plugin/PluginSettingForm.vue'
import { SettingItem } from '@bindings/github.com/library-squirrel/backend/plugin/models'
import type { PluginPreferenceEntryDTO } from '@bindings/github.com/library-squirrel/backend/base/model/dto'
import { pluginSettingApi, pluginPreferenceApi } from '@renderer/apis/http'

const props = defineProps<{ publicId: string }>()
const state = defineModel<boolean>('state', { required: true })

const items = ref<SettingItem[]>([])
const loading = ref(false)
// 当前编辑值（key → value）
const currentValues = ref<Record<string, string>>({})
// 该插件偏好条目（经问答沉淀的用户决策偏好：只读展示 + 删除入口；删除即忘掉，插件下次重新询问）
const preferences = ref<PluginPreferenceEntryDTO[]>([])

// 打开时加载设置与该插件偏好
watch(
  () => state.value,
  async (open) => {
    if (open && props.publicId) {
      await Promise.all([loadSettings(), loadPreferences()])
    }
  },
  { immediate: true }
)

async function loadSettings() {
  loading.value = true
  try {
    const res = await pluginSettingApi.pluginSettingGetSettings(props.publicId)
    items.value = res.data ?? []
    currentValues.value = {}
  } catch (e) {
    ElMessage.error(`加载设置失败：${(e as Error).message}`)
    items.value = []
  } finally {
    loading.value = false
  }
}

function handleChange(values: Record<string, string>) {
  currentValues.value = values
}

// 加载该插件偏好条目（无偏好时列表为空、区块不渲染；加载失败提示错误不阻塞设置表单）
async function loadPreferences() {
  try {
    const res = await pluginPreferenceApi.pluginPreferenceListByPlugin(props.publicId)
    preferences.value = res.data
  } catch (e) {
    ElMessage.error(`加载插件偏好失败：${(e as Error).message}`)
    preferences.value = []
  }
}

// 偏好更新时间（本地化展示）
function formatPreferenceTime(timestamp: number): string {
  return timestamp > 0 ? new Date(timestamp).toLocaleString() : '—'
}

// 删除一条该插件偏好（确认后删除；删除即忘掉，该插件下次遇到相同情况会重新询问；删除后即时刷新列表）
async function handleDeletePreference(entry: PluginPreferenceEntryDTO) {
  const confirm = await ElMessageBox.confirm(
    `删除后「${entry.title}」将被忘掉，该插件下次遇到相同情况时会重新询问。`,
    '删除插件偏好',
    { confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning' }
  ).then(() => true).catch(() => false)
  if (!confirm) return
  try {
    await pluginPreferenceApi.pluginPreferenceDelete(entry.id)
    await loadPreferences()
  } catch (e) {
    ElMessage.error(`删除插件偏好失败：${(e as Error).message}`)
  }
}

// 仅保存与初始值不同的项
async function handleSave() {
  try {
    const initial: Record<string, string> = {}
    items.value.forEach((i) => {
      initial[i.key] = i.value
    })
    const tasks: Promise<unknown>[] = []
    for (const [key, val] of Object.entries(currentValues.value)) {
      if (initial[key] !== val) {
        tasks.push(pluginSettingApi.pluginSettingSave(props.publicId, key, val))
      }
    }
    if (tasks.length === 0) {
      ElMessage.info('无变更')
      state.value = false
      return
    }
    await Promise.all(tasks)
    ElMessage.success('设置已保存')
    state.value = false
  } catch (e) {
    ElMessage.error(`保存失败：${(e as Error).message}`)
  }
}

// 重置全部为默认值
async function handleReset() {
  try {
    const tasks = items.value.map((i) =>
      pluginSettingApi.pluginSettingReset(props.publicId, i.key)
    )
    await Promise.all(tasks)
    ElMessage.success('已重置为默认值')
    await loadSettings()
  } catch (e) {
    ElMessage.error(`重置失败：${(e as Error).message}`)
  }
}
</script>

<template>
  <auto-height-dialog
    v-model:state="state"
    width="min(560px, 90vw)"
    :close-on-click-modal="false"
  >
    <template #header>
      <span class="plugin-setting-dialog-title">插件设置</span>
    </template>
    <div v-loading="loading" class="plugin-setting-dialog-body">
      <el-empty v-if="!loading && items.length === 0" description="该插件无可配置项" />
      <plugin-setting-form
        v-else
        :items="items"
        @change="handleChange"
      />
      <!-- 该插件偏好条目（只读列表 + 删除入口；无偏好不渲染） -->
      <div
        v-if="preferences.length > 0"
        class="plugin-preference-section"
      >
        <el-divider content-position="left">
          记住的偏好
        </el-divider>
        <div
          v-for="entry in preferences"
          :key="entry.id"
          class="plugin-preference-row"
        >
          <span class="plugin-preference-title">{{ entry.title }}</span>
          <span class="plugin-preference-key">{{ entry.prefKey }}</span>
          <span class="plugin-preference-time">{{ formatPreferenceTime(entry.updateTime) }}</span>
          <el-button
            type="danger"
            class="tone-fail"
            size="small"
            @click="handleDeletePreference(entry)"
          >
            删除
          </el-button>
        </div>
      </div>
    </div>
    <template #footer>
      <el-button v-if="items.length > 0" @click="handleReset">重置默认</el-button>
      <el-button @click="state = false">取消</el-button>
      <el-button v-if="items.length > 0" type="primary" @click="handleSave">保存</el-button>
    </template>
  </auto-height-dialog>
</template>

<style scoped>
/* 内容区滚动由 AutoHeightDialog 内置 el-scrollbar 承担（随窗口高度封顶），此处只保底加载/空态占位高度 */
.plugin-setting-dialog-body {
  min-height: 120px;
}
.plugin-setting-dialog-title {
  font-size: 20px;
  color: var(--app-text-primary);
}
/* 插件偏好条目行：标题 + 键 + 更新时间 + 删除按钮，窄容器下允许折行 */
.plugin-preference-section {
  margin-top: 4px;
}
.plugin-preference-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 4px 12px;
  padding: 6px 0;
}
.plugin-preference-title {
  font-weight: 500;
  color: var(--app-text-primary);
}
.plugin-preference-key {
  font-size: 13px;
  color: var(--app-text-regular);
}
.plugin-preference-time {
  margin-left: auto;
  font-size: 12px;
  color: var(--app-text-secondary);
}
</style>
