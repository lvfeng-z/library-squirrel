<script setup lang="ts">
import { CommonInputConfig } from '../../../model/util/CommonInputConfig.ts'
import { computed, nextTick, onBeforeMount, ref, Ref, UnwrapRef } from 'vue'
import TreeSelectNode from '../../../model/util/TreeSelectNode.ts'
import CommonInputText from '@renderer/components/common/CommentInput/CommonInputText.vue'
import lodash from 'lodash'
import CommonInputDate from '@renderer/components/common/CommentInput/CommonInputDate.vue'
import CommonInputSelect from '@renderer/components/common/CommentInput/CommonInputSelect.vue'
import CommonInputTreeSelect from '@renderer/components/common/CommentInput/CommonInputTreeSelect.vue'
import CommonInputAutoLoadSelect from '@renderer/components/common/CommentInput/CommonInputAutoLoadSelect.vue'
import { arrayNotEmpty, isNullish, notNullish } from '@renderer/utils/CommonUtil.ts'
import { getNode } from '@renderer/utils/TreeUtil.ts'
import { SelectItem } from "@bindings/github.com//lvfeng-z/library-squirrel-sdk/dto"

// props
const props = defineProps<{
  config: CommonInputConfig
  extraData?: unknown
}>()

// onBeforeMount
onBeforeMount(() => {
  // 处理默认开关状态
  if (props.config.defaultDisabled === undefined) {
    enable()
  } else {
    if (props.config.defaultDisabled) {
      disable()
    } else {
      enable()
    }
  }
})

// model
const data = defineModel<unknown>('data', { default: undefined, required: true })
// 在选择项加载之前用于展示的数据
const cacheData = defineModel<SelectItem>('cacheData', { default: undefined, required: false })

// 事件
const emits = defineEmits(['dataChanged'])

// 变量
const inputRef = ref<HTMLElement>()
const config: Ref<CommonInputConfig> = ref(lodash.cloneDeep(props.config))
const disabled: Ref<boolean> = ref(false)
const selectList: Ref<SelectItem[]> = ref([])
const dynamicComponent = computed(() => {
  if (!disabled.value || props.config.type === 'custom') {
    switch (props.config.type) {
      case 'text':
      case 'textarea':
        return CommonInputText
      case 'date':
      case 'datetime':
        return CommonInputDate
      case 'select':
        props.config.refreshSelectData()
        return CommonInputSelect
      case 'treeSelect':
        props.config.refreshSelectData()
        return CommonInputTreeSelect
      case 'autoLoadSelect':
        return CommonInputAutoLoadSelect
      case 'custom':
        if (notNullish(props.config.render)) {
          return props.config.render(data.value, props.extraData)
        }
        break
    }
  }
  return undefined
})
const spanText = computed(() => {
  const type = props.config.type
  const tempData = data.value
  if (type === 'custom') {
    return
  }
  if (type === 'date' || type === 'datetime') {
    const datetime = new Date(tempData as number)
    const year = datetime.getFullYear() + '-'
    const month = (datetime.getMonth() + 1 < 10 ? '0' + (datetime.getMonth() + 1) : datetime.getMonth() + 1) + '-'
    const day = (datetime.getDate() + 1 < 10 ? '0' + datetime.getDate() : datetime.getDate()) + ' '
    const date = year + month + day
    if (type === 'date') {
      return date
    } else {
      const hour = (datetime.getHours() < 10 ? '0' + datetime.getHours() : datetime.getHours()) + ':'
      const minute = (datetime.getMinutes() < 10 ? '0' + datetime.getMinutes() : datetime.getMinutes()) + ':'
      const second = datetime.getSeconds() < 10 ? '0' + datetime.getSeconds() : datetime.getSeconds()
      return date + hour + minute + second
    }
  } else if (type === 'select') {
    let target
    if (notNullish(props.config.selectList)) {
      target = props.config.selectList.find((selectData) => selectData.value === tempData)
    }
    return isNullish(target) ? '-' : target.value
  } else if (type === 'treeSelect') {
    let tempRoot = new TreeSelectNode()
    tempRoot.children = props.config.selectList as TreeSelectNode[]
    tempRoot = new TreeSelectNode(tempRoot)
    const node = getNode(tempRoot, tempData as number)
    return isNullish(node) ? '-' : node.label
  } else if (type === 'autoLoadSelect') {
    if (cacheData.value?.value === tempData) {
      return isNullish(cacheData.value) ? '-' : cacheData.value.label
    }
    if (arrayNotEmpty(selectList.value)) {
      const target = selectList.value.find((selectData) => selectData.value === tempData)
      return isNullish(target) ? '-' : target.label
    }
    return '-'
  } else {
    return tempData
  }
})
const cursor = ref(props.config.dblclickToEdit ? 'pointer' : 'default')

// 方法
// 启用
function enable() {
  disabled.value = false
  nextTick(() => {
    if (notNullish(inputRef.value)) {
      inputRef.value.focus()
    }
  })
}
// 禁用
function disable() {
  disabled.value = true
}
// 处理组件被双击事件
function handleDblclick() {
  if (props.config.dblclickToEdit) {
    enable()
  }
}
// 处理失去焦点事件
function handleBlur() {
  if (props.config.defaultDisabled) {
    disable()
  }
}
// 处理数据改变事件
function handleDataChange(newData) {
  emits('dataChanged', newData)
}
</script>

<template>
  <div class="common-input" @dblclick="handleDblclick">
    <span v-if="disabled && props.config.type !== 'custom'">{{ spanText }}</span>
    <component
      :is="dynamicComponent"
      v-if="!disabled || props.config.type === 'custom'"
      ref="inputRef"
      v-bind="{ config: config, cacheData: cacheData }"
      v-model="data"
      v-model:select-list="selectList"
      v-model:cache-data="cacheData"
      mark-row
      @blur="handleBlur"
      @change="handleDataChange"
    />
  </div>
</template>

<style scoped>
.common-input {
  min-width: 10px;
  min-height: 10px;
  cursor: v-bind(cursor);
}
</style>
