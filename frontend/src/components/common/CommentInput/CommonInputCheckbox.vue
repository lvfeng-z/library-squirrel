<script setup lang="ts">
import { CommonInputConfig } from '@renderer/model/util/CommonInputConfig.ts'
import { ref } from 'vue'

// props
const props = defineProps<{
  config: CommonInputConfig
}>()

// model
// const data = defineModel('data', { default: undefined, required: false })

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
  <el-checkbox-group v-if="props.config.type === 'checkbox'" ref="input" @change="() => emits('change')"></el-checkbox-group>
</template>

<style scoped></style>
