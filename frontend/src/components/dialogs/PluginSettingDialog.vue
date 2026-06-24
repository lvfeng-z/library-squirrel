<script setup lang="ts">
import { ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import PluginSettingForm from '@renderer/components/plugin/PluginSettingForm.vue'
import { SettingItem } from '@bindings/github.com/library-squirrel/backend/plugin/models'
import { pluginSettingApi } from '@renderer/apis/http'

const props = defineProps<{ publicId: string }>()
const state = defineModel<boolean>('state', { required: true })

const items = ref<SettingItem[]>([])
const loading = ref(false)
// 当前编辑值（key → value）
const currentValues = ref<Record<string, string>>({})

// 打开时加载设置
watch(
  () => state.value,
  async (open) => {
    if (open && props.publicId) {
      await loadSettings()
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
  <el-dialog
    v-model="state"
    title="插件设置"
    width="560px"
    append-to-body
    :close-on-click-modal="false"
  >
    <div v-loading="loading" class="plugin-setting-dialog-body">
      <el-empty v-if="!loading && items.length === 0" description="该插件无可配置项" />
      <plugin-setting-form
        v-else
        :items="items"
        @change="handleChange"
      />
    </div>
    <template #footer>
      <el-button v-if="items.length > 0" @click="handleReset">重置默认</el-button>
      <el-button @click="state = false">取消</el-button>
      <el-button v-if="items.length > 0" type="primary" @click="handleSave">保存</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.plugin-setting-dialog-body {
  min-height: 120px;
  max-height: 60vh;
  overflow-y: auto;
}
</style>
