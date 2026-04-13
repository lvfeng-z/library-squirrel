<script setup lang="ts">
import { onBeforeMount, Ref, ref, UnwrapRef } from 'vue'
import { InputBox } from '../../model/util/InputBox'
import CollapseForm from './CollapseForm.vue'
import ScrollTextBox from './ScrollTextBox.vue'
import CommonInput from './CommentInput/CommonInput.vue'
import lodash from 'lodash'
import { arrayNotEmpty, notNullish } from '@renderer/utils/CommonUtil.ts'

// props
const props = withDefaults(
  defineProps<{
    mainInputBoxes?: InputBox[]
    dropDownInputBoxes?: InputBox[]
    createButton?: boolean
    reverse?: boolean
    searchButtonDisabled?: boolean
  }>(),
  {
    createButton: false,
    reverse: false,
    searchButtonDisabled: false
  }
)

// model
const params = defineModel<object>('params', { default: {}, required: true }) // 查询参数

// onBeforeMount
onBeforeMount(() => {
  calculateSpan()
})

// 事件
const emits = defineEmits(['searchButtonClicked', 'createButtonClicked'])

// 变量
const barButtonSpan = ref(3) // 查询和新增按钮的span
const innerMainInputBoxes: Ref<UnwrapRef<InputBox[]>> = ref([]) // 主搜索栏中元素的列表
const innerDropdownInputBoxes: Ref<UnwrapRef<InputBox[]>> = ref([]) // 下拉搜索框中元素的列表
const state: Ref<UnwrapRef<boolean>> = ref(false) // 开关状态

// 方法
// 处理主搜索栏和下拉搜索框
function calculateSpan() {
  let spanRest = 24 - (props.createButton ? barButtonSpan.value * 2 : barButtonSpan.value)
  if (notNullish(props.mainInputBoxes)) {
    for (const inputBox of props.mainInputBoxes) {
      // 储存当前box的长度
      let boxSpan = 0
      // 不更改props属性
      const tempInputBox: InputBox = lodash.cloneDeep(inputBox)

      // 未设置是否展示标题，默认为false，labelSpan设为0
      if (tempInputBox.showLabel == undefined) {
        tempInputBox.showLabel = false
        tempInputBox.labelSpan = 0
      }

      if (spanRest > 0) {
        // 未设置tag长度则设置为2
        if (tempInputBox.labelSpan == undefined) {
          tempInputBox.labelSpan = 2
        }
        // 未设置input长度则设置为5
        if (tempInputBox.inputSpan == undefined) {
          tempInputBox.inputSpan = 5
        }

        // 判断是否能够容纳此box，空间不足就放进dropDownInputBoxes
        boxSpan += tempInputBox.labelSpan + tempInputBox.inputSpan
        spanRest -= boxSpan
        if (spanRest < 0) {
          innerDropdownInputBoxes.value.push(tempInputBox)
        } else {
          innerMainInputBoxes.value.push(tempInputBox)
        }
      } else {
        innerDropdownInputBoxes.value.push(tempInputBox)
      }
    }

    // 长度不相等，说明有元素被放进dropDownInputBoxes
    if (innerMainInputBoxes.value.length != props.mainInputBoxes.length) {
      console.debug('主搜索栏长度不足以容纳所有mainInputBoxes元素，不能容纳的元素以被移动至dropDownInputBoxes')
    }
  }

  if (notNullish(props.dropDownInputBoxes)) {
    const tempDropdownInputBoxes = lodash.cloneDeep(props.dropDownInputBoxes)
    innerDropdownInputBoxes.value.push(...tempDropdownInputBoxes)
  }

  // // 下拉菜单中有内容则显示下拉菜单，否则不显示
  // collapseState.value = innerDropdownInputBoxes.value.length > 0
}
// 处理搜索按钮点击事件
function handleSearchButtonClicked() {
  emits('searchButtonClicked')
}
// 处理新增按钮点击事件
function handleCreateButtonClicked() {
  emits('createButtonClicked')
}
// 展开折叠面板
function expandCollapsePanel(event) {
  event.stopPropagation()
  state.value = true
}
</script>

<template>
  <div
    :class="{
      'search-toolbar': true,
      'search-toolbar-normal': !reverse,
      'search-toolbar-reverse': reverse
    }"
  >
    <el-row class="search-toolbar-main-row">
      <el-col v-if="createButton" class="search-toolbar-create-button" :span="barButtonSpan">
        <el-button type="primary" @click="handleCreateButtonClicked">新增</el-button>
      </el-col>
      <template v-for="item in innerMainInputBoxes" :key="item.name">
        <el-col v-if="item.showLabel" class="search-toolbar-label" :span="item.labelSpan">
          <div class="search-toolbar-label-scroll-text-wrapper el-tag-mimic" style="width: 100%">
            <scroll-text-box>{{ item.label }}</scroll-text-box>
          </div>
        </el-col>
        <el-col class="search-toolbar-input" :span="item.inputSpan">
          <el-form-item class="search-toolbar-input-form-item">
            <common-input v-model:data="params[item.name]" class="search-toolbar-input-form-item-input" :config="item" />
          </el-form-item>
        </el-col>
      </template>
      <el-col class="search-toolbar-search-button" :span="barButtonSpan">
        <el-dropdown>
          <el-button :disabled="props.searchButtonDisabled" @click="handleSearchButtonClicked"> 搜索 </el-button>
          <template v-if="innerDropdownInputBoxes.length > 0" #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item @click="expandCollapsePanel">更多选项</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </el-col>
    </el-row>
    <collapse-form
      v-if="arrayNotEmpty(innerDropdownInputBoxes)"
      v-model:form-data="params"
      v-model:state="state"
      class="dropdown-menu rounded-borders"
      :position="reverse ? 'bottom' : 'top'"
      :input-boxes="innerDropdownInputBoxes"
    />
  </div>
</template>

<style scoped>
.search-toolbar {
  width: 100%;
  height: calc(32px);
  display: flex;
  flex-direction: column;
  align-content: center;
}
.search-toolbar-normal {
  flex-direction: column;
}
.search-toolbar-reverse {
  flex-direction: column-reverse;
}
.search-toolbar-main-row {
  height: 32px;
  width: 100%;
}
.search-toolbar-create-button {
  height: 100%;
}
.search-toolbar-search-button {
  height: 100%;
  display: flex;
  justify-content: flex-end;
  margin-left: auto;
}
.search-toolbar-label {
  height: 100%;
  display: flex;
  justify-content: flex-end;
}
.search-toolbar-input {
  height: 100%;
  display: flex;
  justify-content: flex-start;
}
.search-toolbar-input-form-item {
  height: 100%;
  width: 100%;
}
.search-toolbar-input-form-item-input {
  width: 100%;
  height: 100%;
}
.dropdown-menu {
  width: 100%;
  height: auto;
  margin-top: 1px;
  background-clip: padding-box;
}
</style>
