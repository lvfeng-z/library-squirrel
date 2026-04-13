<script setup lang="ts">
import { InputBox } from '../../model/util/InputBox'
import { onBeforeMount, ref, Ref, UnwrapRef } from 'vue'
import ScrollTextBox from './ScrollTextBox.vue'
import CommonInput from './CommentInput/CommonInput.vue'
import lodash from 'lodash'
import CollapsePanel from '@renderer/components/common/CollapsePanel.vue'

// props
const props = withDefaults(
  defineProps<{
    inputBoxes: InputBox[] // InputBox数组
    position?: 'top' | 'bottom' | 'left' | 'right'
    maxLength?: string
  }>(),
  {
    position: 'top'
  }
)

// model
const formData = defineModel<object>('formData', { required: true }) //
const state: Ref<boolean> = defineModel<boolean>('state', { required: true }) // 开关状态

// onBeforeMount
onBeforeMount(() => {
  calculateSpan()
})

// 变量
const inputBoxInRow: Ref<UnwrapRef<InputBox[][]>> = ref([]) // InputBox分行数组

// 方法
// 处理InputBox布局
function calculateSpan() {
  // 用于计算当前行在插入box之后还剩多少span
  let spanRest = 24
  // 临时用于储存行信息的数组
  let tempRow: InputBox[] = []
  inputBoxInRow.value.push(tempRow)

  // 遍历box数组，处理如何分布这些box
  for (const inputBox of props.inputBoxes) {
    // 储存当前box的长度
    let boxSpan = 0
    // 不更改props属性
    const tempInputBox: InputBox = lodash.cloneDeep(inputBox)
    // 未设置tag长度则设置为2
    if (tempInputBox.labelSpan == undefined) {
      tempInputBox.labelSpan = 2
    }
    // 未设置input长度则设置为6
    if (tempInputBox.inputSpan == undefined) {
      tempInputBox.inputSpan = 4
    }

    // 判断是否能够容纳此box
    boxSpan += tempInputBox.labelSpan + tempInputBox.inputSpan
    spanRest -= boxSpan

    if (spanRest >= 0) {
      tempRow.push(tempInputBox)
    } else {
      tempRow = []
      inputBoxInRow.value.push(tempRow)
      tempRow.push(tempInputBox)
      spanRest = 24 - boxSpan
    }
  }
}
</script>

<template>
  <collapse-panel v-model:state="state" :position="props.position" :max-length="props.maxLength">
    <el-scrollbar class="dropdown-table-rows">
      <el-form v-model="formData">
        <template v-for="(boxRow, boxRowindex) in inputBoxInRow" :key="boxRowindex">
          <el-row class="dropdown-table-row">
            <template v-for="(item, index) in boxRow" :key="index">
              <el-col class="dropdown-table-label" :span="item.labelSpan">
                <div
                  :key="boxRowindex + '-' + index"
                  class="dropdown-table-label-scroll-text-wrapper el-tag-mimic"
                  style="width: 100%"
                >
                  <scroll-text-box class="dropdown-table-label-scroll-text">{{ item.label }}</scroll-text-box>
                </div>
              </el-col>
              <el-col class="dropdown-table-input" :span="item.inputSpan">
                <el-form-item class="dropdown-table-input-form-item">
                  <common-input
                    v-model:data="formData[item.name]"
                    class="search-toolbar-input-form-item-input"
                    :config="item"
                  ></common-input>
                </el-form-item>
              </el-col>
            </template>
          </el-row>
        </template>
      </el-form>
    </el-scrollbar>
  </collapse-panel>
</template>

<style scoped>
.dropdown-table-row {
  height: 32px;
  margin-top: 3px;
}
.dropdown-table-label {
  height: 100%;
  display: flex;
  justify-content: flex-end;
}
.dropdown-table-input {
  height: 100%;
  display: flex;
  justify-content: flex-start;
}
.dropdown-table-input-form-item {
  width: 100%;
  height: 100%;
}
.search-toolbar-input-form-item-input {
  width: 100%;
  height: 100%;
}
.dropdown-table-rows {
  flex-direction: column;
  background-color: white;
  padding: 7px;
  height: calc(100% - 14px);
}
.dropdown-table-label-scroll-text {
  width: 100%;
  height: 100%;
}
</style>
