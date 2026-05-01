<script setup lang="ts">
import { Ref, ref, UnwrapRef } from 'vue'
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
const state: Ref<UnwrapRef<boolean>> = ref(false) // 开关状态

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
      <slot name="main" />
      <el-dropdown class="search-toolbar-search-button">
        <el-button :disabled="props.searchButtonDisabled" @click="emits('searchButtonClicked')"> 搜索 </el-button>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item @click="expandCollapsePanel">更多选项</el-dropdown-item>
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
.search-toolbar-main {
  display: flex;
  height: 32px;
  width: 100%;
}
.search-toolbar-search-button {
  height: 100%;
  display: flex;
  justify-content: flex-end;
  margin-left: auto;
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
