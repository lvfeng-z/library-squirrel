<script setup lang="ts">
import IPage from '@renderer/model/util/IPage.ts'
import { nextTick, onMounted, onUnmounted, Ref, ref, watch } from 'vue'
import { SelectItem } from "@bindings/github.com/library-squirrel/wails/backend/base/model/dto"
import TagBox from '@renderer/components/common/TagBox.vue'
import lodash, { throttle } from 'lodash'
import { Close } from '@element-plus/icons-vue'
import { isNullish, notNullish } from '@renderer/utils/CommonUtil.ts'
import SegmentedTag from '@renderer/components/common/SegmentedTag.vue'
import SegmentedTagItem from '@renderer/model/util/SegmentedTagItem.ts'
import Page from '@renderer/model/util/Page.ts'
import { isNotBlank } from '@renderer/utils/StringUtil.ts'

// props
const props = withDefaults(
  defineProps<{
    load: (page: IPage<SelectItem>, input?: string) => Promise<IPage<SelectItem>>
    pageSize?: number
    tagsGap?: string
    maxHeight?: string
    minHeight?: string
    colorResolver?: (segmentedTagItem: SegmentedTagItem) => void
  }>(),
  {
    pageSize: 10
  }
)

// model
// 已选数据
const selectedData: Ref<SegmentedTagItem[]> = defineModel<SegmentedTagItem[]>('selectedData', { required: true })
// 用户输入的自定义数据
const customData: Ref<SegmentedTagItem[]> = defineModel<SegmentedTagItem[]>('customData', { required: true })
// 输入文本
const input: Ref<string | undefined> = defineModel<string | undefined>('input', { required: true })

// 暴露
defineExpose({ newSearch })

// onMounted
onMounted(() => {
  // 监听compositionstart事件
  inputElement.value.addEventListener('compositionstart', (e) => {
    console.log('compositionstart', e)
  })
  // 监听compositionupdate事件
  inputElement.value.addEventListener('compositionupdate', (e: { data: string }) => {
    console.log('compositionupdate', e)
    input.value = e.data
  })
  // 监听compositionend事件
  inputElement.value.addEventListener('compositionend', (e) => {
    console.log('compositionend', e)
    newSearch()
  })
  // 监听宽度变化
  resizeObserver.observe(wrapper.value)
})

// onUnmounted
onUnmounted(() => {
  if (notNullish(wrapper.value)) {
    resizeObserver.unobserve(wrapper.value)
  }
})

// 变量
// 最外部div的实例
const wrapper = ref()
// 是否持有焦点
const focused = ref(false)
// 可选择数据TagBox组件的实例
const optionalTagBox = ref()
// input的实例
const inputElement = ref()
// hiddenSpan的实例
const hiddenSpan = ref()
// 可选数据
const optionalData: Ref<SegmentedTagItem[]> = ref([])
// 可选数据被勾选的id数组
const optionalCheckedIdBuffer: Set<string> = new Set()
// 总体宽度
const width: Ref<number> = ref(0)
// 总体宽度
const popoverOffset: Ref<number> = ref(0)
// 输入框宽度
const inputWidth: Ref<string> = ref('11px')
// 监听wrapper div的宽度变化
const resizeObserver = new ResizeObserver((entries) => {
  throttle(() => (width.value = entries[0].contentRect.width), 300, { leading: true, trailing: true })()
  throttle(() => (popoverOffset.value = (entries[0].contentRect.height + 1000) / 100), 300, { leading: true, trailing: true })()
})
// 控制已选中栏的展开和折叠
const expand: Ref<boolean> = ref(false)

// 方法
// 加载分页的函数
async function innerLoad(page: IPage<SegmentedTagItem>): Promise<IPage<SegmentedTagItem>> {
  page.pageSize = props.pageSize
  const tempPage = lodash.cloneDeep(page)
  return props.load(tempPage, input.value).then((apiRespPage) => {
    const result = new Page(apiRespPage).transform<SegmentedTagItem>()
    result.data = apiRespPage.data?.map((selectItem) => {
      const checked = optionalCheckedIdBuffer.has(String(selectItem.value))
      if (checked) {
        const result = new SegmentedTagItem({ disabled: true, ...selectItem })
        if (notNullish(props.colorResolver)) {
          props.colorResolver(result)
        }
        return result
      } else {
        const result = new SegmentedTagItem(selectItem)
        if (notNullish(props.colorResolver)) {
          props.colorResolver(result)
        }
        return result
      }
    })
    return result
  })
}
// 重新分页查询
function newSearch() {
  throttle(() => optionalTagBox.value.newSearch(), 500, { leading: true, trailing: true })()
}
// 处理input焦点事件
function handleInputFocus(focus: boolean) {
  if (focus && !focused.value) {
    newSearch()
  }
  focused.value = focus
}
// 处理标签点击事件
function handelTagClicked(tag: SegmentedTagItem, optional: boolean) {
  if (optional) {
    // 待选栏
    if (isNullish(tag.disabled) || !tag.disabled) {
      // 如果标签未禁用，则把这个标签放进已选栏，待选栏中的这个标签设为禁用，同时清除输入文本
      const tempTag = lodash.cloneDeep(tag)
      selectedData.value.push(tempTag)
      optionalCheck(tag, true)
      input.value = undefined
    } else {
      // 如果标签已经禁用，则从已选栏中移除这个标签，并把这个标签设为启用
      const index = selectedData.value.findIndex((selected) => selected.value === tag.value)
      if (index !== -1) {
        selectedData.value.splice(index, 1)
      }
      optionalCheck(tag, false)
    }
  } else {
    // 已选栏
    tag.disabled = !tag.disabled
  }
}
// 处理标签关闭按钮点击事件
function handelTagClosed(tag: SegmentedTagItem) {
  let index = selectedData.value.indexOf(tag)
  if (index > -1) {
    selectedData.value.splice(index, 1)
  }
  index = optionalData.value.findIndex((selected) => selected.value === tag.value)
  if (index > -1) {
    optionalCheck(optionalData.value[index], false)
  }
}
// 处理自定义标签关闭按钮点击事件
function handelCustomTagClosed(tag: SegmentedTagItem) {
  let index = customData.value.indexOf(tag)
  if (index > -1) {
    customData.value.splice(index, 1)
  }
}
// 改变待选栏中标签的选中状态
function optionalCheck(tag: SegmentedTagItem, check: boolean) {
  tag.disabled = check
  // 更新选中状态缓存
  if (check) {
    optionalCheckedIdBuffer.add(String(tag.value))
  } else {
    optionalCheckedIdBuffer.delete(String(tag.value))
  }
}
// 清除所有选择
function clear() {
  customData.value = []
  selectedData.value = []
  input.value = undefined
  optionalData.value.forEach((selectItem) => (selectItem.disabled = false))
  optionalCheckedIdBuffer.clear()
}
function handleKeyPress(event: KeyboardEvent) {
  // 按下Enter键时根据输入框内容创建一个自定义标签
  if (event.key === 'Enter' && isNotBlank(input.value)) {
    customData.value.push(
      new SegmentedTagItem({
        value: input.value,
        label: input.value,
        mainBackGround: 'rgb(230, 162, 60, 30%)',
        mainBackGroundHover: 'rgb(230, 162, 60, 15%)',
        mainTextColor: 'rgb(230, 162, 60, 85%)'
      })
    )
    input.value = ''
  }
}

// watch
watch(input, () => {
  nextTick(() => (inputWidth.value = hiddenSpan.value.offsetWidth + 11 + 'px'))
})
</script>

<template>
  <el-popover
    :width="width"
    :show-after="300"
    :hide-after="300"
    transition="el-zoom-in-top"
    trigger="hover"
    placement="top"
    :offset="popoverOffset"
    @before-enter="
      () => {
        handleInputFocus(true)
        expand = true
      }
    "
    @hide="handleInputFocus(false)"
    @before-leave="expand = false"
  >
    <template #reference>
      <div
        ref="wrapper"
        :class="{
          'auto-load-tag-select-main': true,
          'auto-load-tag-select-main-fold': expand,
          'auto-load-tag-select-main-unfold': !expand
        }"
      >
        <div class="auto-load-tag-select-selected-wrapper">
          <tag-box
            v-model:data="selectedData"
            class="auto-load-tag-select-selected"
            tag-closeable
            @tag-close="handelTagClosed"
            @tag-clicked="(tag) => handelTagClicked(tag, false)"
            @click="inputElement.focus()"
          >
            <template #head>
              <template v-for="customItem in customData" :key="customItem.value">
                <segmented-tag :item="customItem" @close="handelCustomTagClosed(customItem)" />
              </template>
            </template>
            <template #tail>
              <input
                ref="inputElement"
                v-model="input"
                class="auto-load-tag-select-input"
                :onkeydown="handleKeyPress"
                @input="newSearch"
              />
              <span ref="hiddenSpan" style="visibility: hidden">{{ input }}</span>
            </template>
          </tag-box>
          <div class="auto-load-tag-close-wrapper">
            <button class="auto-load-tag-close" @click="clear">
              <el-icon color="rgb(166.2, 168.6, 173.4, 75%)"><Close /></el-icon>
            </button>
          </div>
        </div>
      </div>
    </template>
    <template #default>
      <div class="auto-load-tag-select-popper-container">
        <slot name="left" />
        <tag-box
          ref="optionalTagBox"
          v-model:data="optionalData"
          class="auto-load-tag-select-optional"
          :load="innerLoad"
          :tags-gap="tagsGap"
          :max-height="maxHeight"
          @tag-clicked="(tag) => handelTagClicked(tag, true)"
        />
        <slot name="right" />
      </div>
    </template>
  </el-popover>
</template>

<style scoped>
.auto-load-tag-select-main {
  display: flex;
  transition: height 0.5s ease;
}
.auto-load-tag-select-main-fold {
  height: auto;
}
.auto-load-tag-select-main-unfold {
  height: v-bind(minHeight);
}
.auto-load-tag-select-selected-wrapper {
  display: flex;
  width: 100%;
  min-height: v-bind(minHeight);
  background-color: var(--el-fill-color-blank);
  border: 1px solid var(--el-border-color);
  border-radius: 4px;
  box-sizing: border-box;
}
.auto-load-tag-select-selected {
  height: 100%;
  width: calc(100% - 30px);
  cursor: text;
}
.auto-load-tag-close-wrapper {
  width: 30px;
  display: flex;
  align-items: center;
  justify-content: center;
  background-color: rgb(166.2, 168.6, 173.4, 5%);
}
.auto-load-tag-close {
  display: flex;
  align-items: center;
  justify-content: center;
  /* 设置宽度和高度相等，以确保按钮是正圆 */
  width: 20px;
  height: 20px;
  margin-left: 3px;
  margin-right: 3px;

  /* 圆角半径设置为50%以形成圆形 */
  border-radius: 50%;

  background-color: rgb(166.2, 168.6, 173.4, 30%);
  border: none; /* 无边框 */
  cursor: pointer; /* 鼠标悬停时显示为手型指针 */

  transition-duration: 0.4s;
}
.auto-load-tag-close:hover {
  width: 24px;
  height: 24px;
  background-color: rgb(166.2, 168.6, 173.4, 45%);
  transition-duration: 0.2s;
}
.auto-load-tag-select-input {
  width: v-bind(inputWidth);
  appearance: none;
  background-color: transparent;
  border: none;
  color: rgb(166.2, 168.6, 173.4);
  font-family: inherit;
  font-size: inherit;
  height: 24px;
  max-width: 100%;
  outline: none;
  padding: 0;
}
.auto-load-tag-select-popper-container {
  display: flex;
  flex-direction: row;
}
</style>
