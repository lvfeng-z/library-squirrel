<script setup lang="ts">
import { CommonInputConfig } from '@renderer/model/util/CommonInputConfig.ts'
import { ref } from 'vue'
import { ElTreeSelect } from 'element-plus'
import { isNullish } from '@renderer/utils/CommonUtil.ts'

// props
const props = defineProps<{
  config: CommonInputConfig
}>()

// model
const data = defineModel<object | number | string | boolean>('data', { default: undefined, required: false })

// 变量
// el-input组件的实例
const input = ref()
// 树形选择组件配置
const treeProps = {
  label: 'label',
  children: 'children',
  isLeaf: 'isLeaf'
}

// 方法
function focus() {
  input.value.focus()
}
function innerLoad(node, resolve) {
  if (node.isLeaf) {
    return resolve([])
  }
  if (isNullish(props.config.load)) {
    return resolve([])
  }
  props.config.load(node.data.id, node).then((children) => resolve(children))
}

// 事件
const emits = defineEmits(['change'])

// 暴露
defineExpose({ focus })
</script>
<template>
  <el-tree-select
    ref="input"
    v-model="data"
    :check-strictly="true"
    :data="config.selectList"
    :placeholder="config.placeholder"
    :remote="config.remote"
    :remote-method="(query: unknown) => config.refreshSelectData(query)"
    :filterable="config.remote"
    :props="treeProps"
    :lazy="config.lazy"
    :load="innerLoad"
    clearable
    @change="() => emits('change')"
  />
</template>

<style scoped></style>
