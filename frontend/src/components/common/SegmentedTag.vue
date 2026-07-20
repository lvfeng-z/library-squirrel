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
    /** 视觉方案：outlined=外围描边+浅主色/透明全局交替分节（默认）；block=各段独立色块分节 */
    variant?: 'block' | 'outlined'
  }>(),
  {
    closeable: true,
    variant: 'outlined'
  }
)

// 变量
const subLabelsLength: Ref<number> = computed(() => {
  return isNullish(props.item.subLabels) ? 0 : props.item.subLabels.length
})
const tagLabelWrapperMaxWidth: Ref<string> = computed(() => {
  return props.closeable ? 'calc(100% - 18px)' : '100%'
})
// main 段代表色优先级：status 令牌 > 调用方显式色字段 > variant 默认色。
// block：main 走 neutral、sub 走 neutral/neutral-strong 色块分节，各段独立配色。
// outlined：整体色族统一跟随 main 段——非空白段(sub2)复用 main 底色、透明段(sub1)透明底，
//           所有文字、close 图标与外围描边均取 main 文字色，形成单色族描边交替。
//           用 computed 保证 colorResolver 后置 status 也能响应。
const colorConfig = computed(() => {
  const status = props.item.status
  const outlined = props.variant === 'outlined'

  // main 段代表色：status 令牌 > 调用方显式色 > variant 默认（block=neutral，outlined=主色族）
  const mainBgDefault = outlined ? 'var(--app-color-primary-light-9)' : 'var(--app-tag-neutral-bg)'
  const mainBgHoverDefault = outlined ? 'var(--app-color-primary-light-8)' : 'var(--app-tag-neutral-bg-hover)'
  const mainTextDefault = outlined ? 'var(--app-color-primary)' : 'var(--app-tag-neutral-text)'
  const mainBackground = status ? `var(--app-status-${status}-bg)` : (props.item.mainBackGround ?? mainBgDefault)
  const mainBackgroundHover = status ? `var(--app-status-${status}-border)` : (props.item.mainBackGroundHover ?? mainBgHoverDefault)
  const mainTextColor = status ? `var(--app-status-${status}-text)` : (props.item.mainTextColor ?? mainTextDefault)

  if (outlined) {
    if (props.item.disabled) {
      // outlined 禁用态：整体褪为 neutral 灰，保留透明/浅灰交替结构（sub2 浅灰、sub1 透明）。
      // main 段实际由 .segmented-tag-main-label-unchecked 类控制（同为 neutral-bg-strong + neutral-text），
      // 此处覆盖 sub 段、外围边框（取自 mainTextColor）与 close 图标，使整个标签统一褪色。
      return {
        mainBackground: 'var(--app-tag-neutral-bg-strong)',
        mainBackgroundHover: 'var(--app-tag-neutral-bg-hover)',
        mainTextColor: 'var(--app-tag-neutral-text)',
        sub1BackGround: 'transparent',
        sub1BackGroundHover: 'var(--app-tag-neutral-bg)',
        sub1TextColor: 'var(--app-tag-neutral-text)',
        sub2BackGround: 'var(--app-tag-neutral-bg)',
        sub2BackGroundHover: 'var(--app-tag-neutral-bg-hover)',
        sub2TextColor: 'var(--app-tag-neutral-text)',
        closeIconColor: 'var(--app-tag-neutral-text)'
      }
    }
    // sub 段派生自 main 色族：sub2(非空白)复用 main 底色，sub1(透明)透明底但 hover 显现 main 底、文字同族。
    // sub1/sub2 显式色字段在此模式下被忽略，以保证整体单色族；close 图标亦取 main 文字色。
    return {
      mainBackground,
      mainBackgroundHover,
      mainTextColor,
      sub1BackGround: 'transparent',
      sub1BackGroundHover: mainBackground,
      sub1TextColor: mainTextColor,
      sub2BackGround: mainBackground,
      sub2BackGroundHover: mainBackgroundHover,
      sub2TextColor: mainTextColor,
      closeIconColor: mainTextColor
    }
  }

  // block：各段独立配色（main 走 status/显式/neutral，sub 走 neutral 体系，close 图标 neutral）
  return {
    mainBackground,
    mainBackgroundHover,
    mainTextColor,
    sub1BackGround: props.item.sub1BackGround ?? 'var(--app-tag-neutral-bg)',
    sub1BackGroundHover: props.item.sub1BackGroundHover ?? 'var(--app-tag-neutral-bg-hover)',
    sub1TextColor: props.item.sub1TextColor ?? 'var(--app-tag-neutral-text)',
    sub2BackGround: props.item.sub2BackGround ?? 'var(--app-tag-neutral-bg-strong)',
    sub2BackGroundHover: props.item.sub2BackGroundHover ?? 'var(--app-tag-neutral-bg-hover)',
    sub2TextColor: props.item.sub2TextColor ?? 'var(--app-tag-neutral-text)',
    closeIconColor: 'var(--app-tag-neutral-text)'
  }
})

// close 段全局序号 = subLabels.length + 1。outlined 的交替基准与 block 相反，
// 故 close 挂载的 sub1/sub2 class 需随 variant 反转，确保与左邻 sub 颜色相异、维持全局交替。
const closeSegmentClass = computed(() => {
  const lengthEven = subLabelsLength.value % 2 === 0
  if (props.variant === 'outlined') {
    return { sub1: lengthEven, sub2: !lengthEven }
  }
  return { sub1: !lengthEven, sub2: lengthEven }
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
    :class="{
      'segmented-tag': true,
      'segmented-tag--outlined': props.variant === 'outlined'
    }"
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
        'segmented-tag-sub-1-label': closeSegmentClass.sub1,
        'segmented-tag-sub-2-label': closeSegmentClass.sub2
      }"
      @click="handleCloseButtonClicked"
    >
      <el-icon
        class="segmented-tag-sub-close"
        :color="colorConfig.closeIconColor"
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
.segmented-tag--outlined {
  /* outlined 方案：外围描边作为容器标识，内部各段按浅主色/透明全局交替分节。
     边框色跟随 main 段文字色（status 令牌 > 调用方显式色 > 主色），使标签整体色调统一 */
  border: 1px solid v-bind('colorConfig.mainTextColor');
  box-sizing: border-box; /* 边框计入尺寸，避免与 block 模式标签高度错位 */
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
