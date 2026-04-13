<script setup lang="ts">
import { CommonInputConfig } from '@renderer/model/util/CommonInputConfig.ts'
import { ref } from 'vue'

// props
const props = defineProps<{
  config: CommonInputConfig
}>()

// 方法

// model
const data = defineModel<unknown>('data', { default: undefined, required: false })

// 变量
// el-input组件的实例
const input = ref()

// 方法
function focus() {
  input.value.focus()
}

// 事件
const emits = defineEmits(['change'])

// 暴露
defineExpose({ focus })
</script>
<template>
  <el-select
    ref="input"
    v-model="data"
    :placeholder="props.config.placeholder"
    :remote="props.config.remote"
    :remote-method="(query: unknown) => props.config.refreshSelectData(query)"
    :filterable="props.config.remote"
    clearable
    @change="() => emits('change')"
  >
    <el-option v-for="item in props.config.selectList" :key="item.value" :value="item.value" :label="item.label" />
  </el-select>
</template>

<style scoped></style>
