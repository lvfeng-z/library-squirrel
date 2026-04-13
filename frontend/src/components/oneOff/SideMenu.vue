<script setup lang="ts">
import { computed, ref, Ref } from 'vue'
import { ArrowLeftBold, ArrowRightBold, Expand, Fold, Lock, Unlock } from '@element-plus/icons-vue'

// props
const props = defineProps<{
  width: string
  foldWidth: string
  defaultActive: string[]
  backgroundColor?: string
}>()

// 变量
const folded: Ref<boolean> = ref(true)
const hidden: Ref<boolean> = ref(false)
const locked: Ref<boolean> = ref(false)
const containerWidth: Ref<string> = computed(() => {
  if (hidden.value) {
    return '0'
  }
  if (locked.value) {
    return 'auto'
  }
  return props.foldWidth
})
const mainWidth: Ref<string> = computed(() => {
  if (hidden.value) {
    return '0'
  }
  if (folded.value) {
    return props.foldWidth
  } else {
    return props.width
  }
})
const collapseButtonVisibility: Ref<string> = computed(() => {
  return hidden.value ? 'hidden' : 'visible'
})

// 方法
function collapse() {
  folded.value = !folded.value
}
function lock() {
  locked.value = !locked.value
  // if (!locked.value) {
  //   collapsed.value = true
  // }
}
// 处理点击外部区域
function handleClickOutSide() {
  if (!locked.value) {
    folded.value = true
  }
}
</script>

<template>
  <div v-click-out-side="handleClickOutSide" class="side-menu-container">
    <div class="side-menu-main">
      <div class="side-menu-collapse-button">
        <el-icon class="side-menu-collapse-button-collapse" @click="collapse">
          <Expand v-if="folded" />
          <Fold v-if="!folded" />
        </el-icon>
        <el-tooltip placement="right" :show-after="650" :auto-close="2150">
          <template #default>
            <el-icon v-show="!folded" class="side-menu-collapse-button-lock" @click="lock">
              <Lock v-show="locked" />
              <Unlock v-show="!locked" />
            </el-icon>
          </template>
          <template #content> {{ locked ? '解锁侧边栏' : '锁定侧边栏' }} </template>
        </el-tooltip>
      </div>
      <el-scrollbar class="side-menu-scrollbar">
        <el-menu :default-openeds="props.defaultActive" class="side-menu-main-menu" :collapse="folded">
          <slot name="default"></slot>
        </el-menu>
      </el-scrollbar>
      <div class="side-menu-hide-button" @click="hidden = !hidden">
        <el-icon v-show="!hidden"><ArrowLeftBold /></el-icon>
        <el-icon v-show="hidden"><ArrowRightBold /></el-icon>
      </div>
    </div>
  </div>
</template>

<style scoped>
.side-menu-container {
  width: v-bind(containerWidth);
  transition: width 0.3s ease;
  --el-menu-bg-color: var(--side-menu-background-color, white);
}
.side-menu-main {
  height: 100%;
  width: v-bind(mainWidth);
  overflow: visible;
  background-color: var(--side-menu-background-color, white);
  border-right: solid 1px var(--el-border-color);
  transition: width 0.3s ease;
}
.side-menu-collapse-button {
  display: grid;
  height: 56px;
  visibility: v-bind(collapseButtonVisibility);
}
.side-menu-collapse-button-collapse {
  grid-area: 1 / 1;
  justify-self: center;
  align-self: center;
  height: 48px;
  width: 48px;
  cursor: pointer;
  border-radius: 5px;
  transition: background-color 0.2s;
}
.side-menu-collapse-button-collapse:hover {
  background-color: var(--el-color-primary-light-9);
}
.side-menu-collapse-button-lock {
  grid-area: 1 / 1;
  justify-self: end;
  align-self: center;
  margin-right: 10px;
  height: 36px;
  width: 36px;
  cursor: pointer;
  border-radius: 5px;
  transition: background-color 0.2s;
}
.side-menu-collapse-button-lock:hover {
  background-color: var(--el-color-primary-light-9);
}
.side-menu-scrollbar {
  border-top: solid 1px var(--el-border-color);
  border-bottom: solid 1px var(--el-border-color);
  height: calc(100% - 56px - 56px - 1px - 1px);
}
.side-menu-main-menu {
  height: 100%;
  width: 100%;
  display: flex;
  flex-direction: column;
  border-right: none;
}
.side-menu-main-menu:not(.el-menu--collapse) {
  width: v-bind(width);
  height: 100%;
}
.side-menu-hide-button {
  display: flex;
  justify-content: center;
  align-items: center;
  height: 56px;
  min-width: 30px;
  cursor: pointer;
}
.side-menu-hide-button:hover {
  background-color: rgb(197.7, 225.9, 255, 80%);
  transition: background-color 0.3s;
  border-radius: 5px;
}
</style>
