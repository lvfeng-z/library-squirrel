<script setup lang="ts">
import { ref } from 'vue'
import { CommonInputConfig } from '@renderer/model/util/CommonInputConfig.ts'
import AutoLoadSelect from '@renderer/components/common/AutoLoadSelect.vue'
import { notNullish } from '@renderer/utils/CommonUtil.ts'
import { SelectItem } from "@bindings/github.com/library-squirrel/wails/pkg/model/dto"
import IPage from '@renderer/model/util/IPage.ts'
import { assertNotNullish } from '@renderer/utils/AssertUtil'

// props
const props = defineProps<{
  config: CommonInputConfig
}>()

// model
const data = defineModel<string | number>('data', { default: undefined, required: false })
const selectList = defineModel<SelectItem[]>('selectList', { default: undefined, required: false })
// 在选择项加载之前用于展示的数据
const cacheData = defineModel<SelectItem>('cacheData', { default: undefined, required: false })

// 变量
// el-input组件的实例
const input = ref()

// 方法
function focus() {
  input.value.focus()
}
function load(page: IPage<SelectItem>, input: string) {
  assertNotNullish(props.config.remotePageMethod)
  return props.config.remotePageMethod(page, input)
}
function handleChange(newData: string | number) {
  const newCache = selectList.value.find((selectData) => selectData.value === newData)
  if (notNullish(newCache)) {
    cacheData.value = newCache
  }
  emits('change', newData)
}

// 事件
const emits = defineEmits(['change'])

// 暴露
defineExpose({ focus })
</script>
<template>
  <auto-load-select
    ref="input"
    v-model:data="data"
    v-model:select-list="selectList"
    :placeholder="props.config.placeholder"
    :remote="props.config.remote"
    :filterable="props.config.remote"
    :load="load"
    clearable
    @change="handleChange"
  >
    <template #default>
      <el-option v-if="notNullish(cacheData)" :hidden="true" :value="cacheData.value" :label="cacheData.label" />
      <el-option v-for="item in selectList" :key="item.value" :value="item.value" :label="item.label" />
    </template>
  </auto-load-select>
</template>

<style scoped></style>
