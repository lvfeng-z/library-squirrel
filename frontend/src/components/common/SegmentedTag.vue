<script setup lang="ts">
import SegmentedTagItem from '@renderer/model/util/SegmentedTagItem.ts'
import { Close } from '@element-plus/icons-vue'
import { computed, type Ref } from 'vue'
import { arrayIsEmpty, isNullish } from '@renderer/utils/CommonUtil.ts'

// props
const props = withDefaults(
  defineProps<{
    item: SegmentedTagItem
    closeable?: boolean
  }>(),
  {
    closeable: true
  }
)

// 变量
const subLabelsLength: Ref<number> = computed(() => {
  return isNullish(props.item.subLabels) ? 0 : props.item.subLabels.length
})
const tagLabelWrapperMaxWidth: Ref<string> = computed(() => {
  return props.closeable ? 'calc(100% - 18px)' : '100%'
})
// main 段：item.status 优先（由 --app-status-{status}-* 令牌驱动），其次显式传入色，最后 neutral 默认；
// sub 段统一走 neutral 令牌。用 computed 保证 colorResolver 后置 status 也能响应。
const colorConfig = computed(() => {
  const status = props.item.status
  return {
    mainBackground: status ? `var(--app-status-${status}-bg)` : (props.item.mainBackGround ?? 'var(--app-tag-neutral-bg)'),
    mainBackgroundHover: status ? `var(--app-status-${status}-border)` : (props.item.mainBackGroundHover ?? 'var(--app-tag-neutral-bg-hover)'),
    mainTextColor: status ? `var(--app-status-${status}-text)` : (props.item.mainTextColor ?? 'var(--app-tag-neutral-text)'),
    sub1BackGround: props.item.sub1BackGround ?? 'var(--app-tag-neutral-bg)',
    sub1BackGroundHover: props.item.sub1BackGroundHover ?? 'var(--app-tag-neutral-bg-hover)',
    sub1TextColor: props.item.sub1TextColor ?? 'var(--app-tag-neutral-text)',
    sub2BackGround: props.item.sub2BackGround ?? 'var(--app-tag-neutral-bg-strong)',
    sub2BackGroundHover: props.item.sub2BackGroundHover ?? 'var(--app-tag-neutral-bg-hover)',
    sub2TextColor: props.item.sub2TextColor ?? 'var(--app-tag-neutral-text)'
  }
})

// 事件
const emits = defineEmits(['clicked', 'mainLabelClicked', 'subLabelClicked', 'close'])

// 方法
// 处理点击事件
function handleClicked() {
  emits('clicked')
}
function handleMainLabelClicked() {
  emits('mainLabelClicked')
}
function handleSubLabelClicked(index: number) {
  emits('subLabelClicked', index)
}
function handleCloseButtonClicked() {
  emits('close')
}
</script>

<template>
  <div
    class="segmented-tag"
    @click="handleClicked"
  >
    <div class="segmented-tag-label-wrapper">
      <div
        :class="{
          'segmented-tag-main-label': true,
          'segmented-tag-main-label-checked': !item.disabled,
          'segmented-tag-main-label-unchecked': item.disabled
        }"
        @click="handleMainLabelClicked"
      >
        <span
          :class="{
            'segmented-tag-main-text': true,
            'segmented-tag-ellipsis': true,
            'segmented-tag-sub-text-last': arrayIsEmpty(props.item.subLabels),
            'segmented-tag-main-text-checked': !item.disabled,
            'segmented-tag-main-text-unchecked': item.disabled
          }"
        >{{ props.item.label }}</span>
      </div>
      <template
        v-for="(subLabel, index) of props.item.subLabels"
        :key="index"
      >
        <div
          :class="{
            'segmented-tag-sub-label': true,
            'segmented-tag-sub-1-label': index % 2 === 0,
            'segmented-tag-sub-2-label': !(index % 2 === 0)
          }"
          @click="handleSubLabelClicked(index)"
        >
          <span
            :class="{
              'segmented-tag-sub-1-text': index % 2 === 0,
              'segmented-tag-sub-2-text': !(index % 2 === 0),
              'segmented-tag-ellipsis': true,
              'segmented-tag-sub-text-last': isNullish(props.item.subLabels) ? false : index === props.item.subLabels.length - 1
            }"
          >
            {{ subLabel }}
          </span>
        </div>
      </template>
    </div>
    <div
      v-if="closeable"
      :class="{
        'segmented-tag-sub-close-wrapper': true,
        'segmented-tag-sub-2-label': subLabelsLength % 2 === 0,
        'segmented-tag-sub-1-label': !(subLabelsLength % 2 === 0)
      }"
      @click="handleCloseButtonClicked"
    >
      <el-icon
        class="segmented-tag-sub-close"
        color="var(--app-tag-neutral-text)"
      >
        <Close />
      </el-icon>
    </div>
  </div>
</template>

<style scoped>
.segmented-tag {
  width: auto;
  display: flex;
  flex-direction: row;
  justify-content: space-between; /* 在一行时左右对齐，换行时每个标签独占一行 */
  border-radius: var(--app-radius-lg);
  max-width: 100%;
  cursor: pointer; /* 鼠标悬停时显示为手型指针 */
  overflow: hidden;
}
.segmented-tag-label-wrapper {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  max-width: v-bind(tagLabelWrapperMaxWidth);
}
.segmented-tag-main-label {
  max-width: 100%;
  flex-grow: 1;
  align-content: center;
  transition-duration: 0.8s;
}
.segmented-tag-main-label-checked {
  background-color: v-bind('colorConfig.mainBackground');
}
.segmented-tag-main-label-checked:hover {
  background-color: v-bind('colorConfig.mainBackgroundHover');
}
.segmented-tag-main-label-unchecked {
  background-color: var(--app-tag-neutral-bg-strong);
}
.segmented-tag-main-label-unchecked:hover {
  background-color: var(--app-tag-neutral-bg-hover);
}
.segmented-tag-main-text {
  max-width: 100%;
  margin-left: 6px;
  margin-right: 3px;
  font-weight: bolder;
  text-align: center;
}
.segmented-tag-main-text-checked {
  color: v-bind('colorConfig.mainTextColor');
}
.segmented-tag-main-text-unchecked {
  color: var(--app-tag-neutral-text);
}
.segmented-tag-sub-label {
  display: flex;
  flex-grow: 1;
  transition-duration: 0.4s;
}
.segmented-tag-sub-1-label {
  background-color: v-bind('colorConfig.sub1BackGround');
}
.segmented-tag-sub-1-label:hover {
  background-color: v-bind('colorConfig.sub1BackGroundHover');
}
.segmented-tag-sub-2-label {
  background-color: v-bind('colorConfig.sub2BackGround');
}
.segmented-tag-sub-2-label:hover {
  background-color: v-bind('colorConfig.sub2BackGroundHover');
}
.segmented-tag-sub-1-text {
  width: 100%;
  margin-left: 3px;
  margin-right: 3px;
  font-weight: inherit;
  text-align: center;
  color: v-bind('colorConfig.sub1TextColor');
}
.segmented-tag-sub-2-text {
  width: 100%;
  margin-left: 3px;
  margin-right: 3px;
  font-weight: inherit;
  text-align: center;
  color: v-bind('colorConfig.sub2TextColor');
}
.segmented-tag-sub-text-last {
  margin-right: 6px;
}
.segmented-tag-sub-close-wrapper {
  display: flex;
  align-items: center;
  justify-items: center;
  width: 18px;
  transition-duration: 0.4s;
}
.segmented-tag-sub-close {
  margin-right: 3px;
}
.segmented-tag-ellipsis {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  display: block;
}
</style>
