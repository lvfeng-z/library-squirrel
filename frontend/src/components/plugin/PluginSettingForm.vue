<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { SettingItem } from '@bindings/github.com/library-squirrel/backend/plugin/models'

const props = defineProps<{ items: SettingItem[] }>()
const emit = defineEmits<{
  (e: 'change', values: Record<string, string>): void
}>()

// 本地值副本（props 不可直接修改，设置值统一以 string 存储）
const values = ref<Record<string, string>>({})

watch(
  () => props.items,
  (items) => {
    const v: Record<string, string> = {}
    items.forEach((i) => {
      v[i.key] = i.value
    })
    values.value = v
    emit('change', { ...v })
  },
  { immediate: true, deep: true }
)

// 按 group 分组，组内按 order 排序；无 group 归入空组
const groupedItems = computed<Array<{ group: string; items: SettingItem[] }>>(() => {
  const sorted = [...props.items].sort((a, b) => (a.order ?? 0) - (b.order ?? 0))
  const groups = new Map<string, SettingItem[]>()
  for (const item of sorted) {
    const g = item.group ?? ''
    if (!groups.has(g)) groups.set(g, [])
    groups.get(g)!.push(item)
  }
  return Array.from(groups.entries()).map(([group, items]) => ({ group, items }))
})

function onChanged() {
  emit('change', { ...values.value })
}

// integer 控件以 number 输入、string 存储
function onNumberChange(key: string, val: number | undefined) {
  values.value[key] = val == null ? '' : String(val)
  onChanged()
}
</script>

<template>
  <div class="plugin-setting-form">
    <template v-for="(grp, gi) in groupedItems" :key="gi">
      <el-divider v-if="grp.group" content-position="left">
        {{ grp.group }}
      </el-divider>
      <el-form label-position="top">
        <el-form-item v-for="item in grp.items" :key="item.key">
          <template #label>
            <span class="setting-label">{{ item.title }}</span>
            <span v-if="item.description" class="setting-desc">{{ item.description }}</span>
          </template>

          <!-- boolean -->
          <el-switch
            v-if="item.type === 'boolean'"
            v-model="values[item.key]"
            active-value="true"
            inactive-value="false"
            @change="onChanged"
          />

          <!-- integer -->
          <el-input-number
            v-else-if="item.type === 'integer'"
            :model-value="values[item.key] === '' ? undefined : Number(values[item.key])"
            :min="item.min ?? undefined"
            :max="item.max ?? undefined"
            @update:model-value="(v: number | undefined) => onNumberChange(item.key, v)"
          />

          <!-- select -->
          <el-select
            v-else-if="item.type === 'select'"
            v-model="values[item.key]"
            @change="onChanged"
          >
            <el-option
              v-for="opt in item.options"
              :key="opt.value"
              :label="opt.label"
              :value="opt.value"
            />
          </el-select>

          <!-- string（encrypted 用密码框） -->
          <el-input
            v-else
            v-model="values[item.key]"
            :type="item.encrypted ? 'password' : 'text'"
            :show-password="item.encrypted"
            @change="onChanged"
          />
        </el-form-item>
      </el-form>
    </template>
  </div>
</template>

<style scoped>
.plugin-setting-form {
  width: 100%;
}
.setting-label {
  color: var(--app-text-primary);
}
.setting-desc {
  display: block;
  font-size: 12px;
  font-weight: normal;
  color: var(--app-text-secondary);
}
.plugin-setting-form :deep(.el-divider__text) {
  color: var(--app-text-primary);
}
</style>
