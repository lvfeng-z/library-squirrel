<script setup lang="ts">
import { Ref, ref } from 'vue'
import CollapsePanel from '@renderer/components/common/CollapsePanel.vue'

// props
const props = withDefaults(
  defineProps<{
    reverse?: boolean
    searchButtonDisabled?: boolean
  }>(),
  {
    reverse: false,
    searchButtonDisabled: false
  }
)

// 事件
const emits = defineEmits(['searchButtonClicked'])

// 变量
const state: Ref<boolean> = ref(false) // 开关状态

// 方法
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
    <div class="search-toolbar-main">
      <div class="search-toolbar-content">
        <slot name="main" />
      </div>
      <!-- 内嵌式竖向分隔线（inset divider）：不与上下边缘相接，分隔自定义区与搜索按钮区 -->
      <div class="search-toolbar-divider" />
      <el-dropdown class="search-toolbar-search-button">
        <el-button
          :disabled="props.searchButtonDisabled"
          @click="emits('searchButtonClicked')"
        >
          搜索
        </el-button>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item @click="expandCollapsePanel">
              更多选项
            </el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
    </div>
    <collapse-panel
      v-model:state="state"
      class="search-toolbar-dropdown-menu rounded-borders"
      :position="reverse ? 'bottom' : 'top'"
      border-radios="10px"
    >
      <el-scrollbar class="search-toolbar-dropdown-menu-scrollbar">
        <template #default>
          <slot name="dropdown" />
        </template>
      </el-scrollbar>
    </collapse-panel>
  </div>
</template>

<style scoped>
.search-toolbar {
  width: 100%;
  /* 高度随内容自适应（多行时撑开），单行时与原 32px 视觉一致 */
  height: auto;
  min-height: 32px;
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
.search-toolbar-main {
  /* 左右两列：自定义内容（左，内部自由折行）与搜索按钮（右，相对内容总高垂直居中） */
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  box-sizing: border-box;
  /* 皮肤内边距：内容列、分隔线、搜索按钮对卡片边缘的统一留白（皮肤与边距同宿主，消费者只管内容排布） */
  padding: 4px 8px;
}
.search-toolbar-content {
  flex: 1;
  min-width: 0;
  /* 自动折行：内容拥挤时换行，显式分行由消费者用全宽元素自行实现 */
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
}
.search-toolbar-divider {
  flex-shrink: 0;
  /* 高度跟随内容区（行内最高子项）：stretch 拉满行高，上下 margin 负责与边缘脱开（inset） */
  align-self: stretch;
  height: auto;
  margin-top: 8px;
  margin-bottom: 8px;
  width: 1px;
  background-color: var(--app-border-color);
}
.search-toolbar-search-button {
  flex-shrink: 0;
  display: flex;
}
.search-toolbar-dropdown-menu {
  width: 100%;
  height: auto;
  margin-top: 1px;
  background-clip: padding-box;
}
.search-toolbar-dropdown-menu-scrollbar {
  flex-direction: column;
  background-color: white;
  padding: 7px;
  height: calc(100% - 14px);
}
</style>
