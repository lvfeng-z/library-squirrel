<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { Close } from '@element-plus/icons-vue'
import DynamicSideMenu from '@renderer/components/slot/DynamicSideMenu.vue'
import NotificationList from '@renderer/components/oneOff/NotificationList.vue'
import ReplaceConfirmDialog from '@renderer/components/dialogs/ReplaceConfirmDialog.vue'
import DialogSlotRenderer from '@renderer/components/slot/DialogSlotRenderer.vue'
import { useTourStatesStore } from '@renderer/store/UseTourStatesStore.ts'

const router = useRouter()
const route = useRoute()
const notificationListState = ref(false)

// 根据当前路由路径判断是否显示关闭按钮（非主页时显示）
const showCloseButton = computed(() => {
  return route.path !== '/'
})

async function handleCloseCurrentView() {
  if (route.path !== '/') {
    await router.push('/')
  }
}
</script>

<template>
  <div class="layout">
    <!-- 关闭按钮 -->
    <div
      :class="{
        'close-subpage-button': true,
        'z-layer-5': true,
        'close-subpage-button-hide': !showCloseButton
      }"
      @click="handleCloseCurrentView"
    >
      <Close class="close-subpage-button-icon" />
    </div>

    <el-container>
      <el-aside
        class="main-page-sidebar z-layer-4"
        width="auto"
      >
        <dynamic-side-menu
          class="aside-side-menu"
          width="160px"
          fold-width="64px"
        />
      </el-aside>

      <el-main style="padding: 0">
        <router-view />
      </el-main>
    </el-container>

    <!-- 弹窗组件 -->
    <notification-list
      class="main-background-task z-layer-3"
      :state="notificationListState"
    />
    <ReplaceConfirmDialog />
    <DialogSlotRenderer />

    <!-- Tour 向导 -->
    <el-tour
      v-model="useTourStatesStore().tourStates.guideMenuTour"
      :scroll-into-view-options="true"
      :mask="false"
      @finish="useTourStatesStore().tourStates.getCallback('guideMenuTour')"
    >
      <el-tour-step
        title="向导"
        description="后续可以点击这里进入向导页面"
        placement="right"
      />
    </el-tour>
    <el-tour
      v-model="useTourStatesStore().tourStates.taskMenuTour"
      :scroll-into-view-options="true"
      @finish="useTourStatesStore().tourStates.getCallback('taskMenuTour')"
    >
      <el-tour-step
        title="任务向导"
        description="点击这里进入任务页面"
      />
    </el-tour>
  </div>
</template>

<style scoped>
.layout {
  display: flex;
  flex-direction: row;
  width: 100%;
  height: 100%;
  background-color: #fafafa;
  --side-menu-background-color: #ebf0f5;
}

.close-subpage-button {
  display: flex;
  justify-content: end;
  align-items: end;
  background-color: var(--el-color-danger);
  cursor: pointer;
  position: absolute;
  left: -65px;
  top: -65px;
  width: 100px;
  height: 100px;
  pointer-events: visibleFill;
  clip-path: circle(50px);
  transition: 0.3s;
}

.close-subpage-button:hover {
  left: -55px;
  top: -55px;
  background-color: var(--el-color-danger-light-3);
}

.close-subpage-button-hide {
  left: -100px;
  top: -100px;
}

.close-subpage-button-icon {
  width: 25%;
  height: 25%;
  color: #fafafa;
  margin-right: 12%;
  margin-bottom: 12%;
}

.main-background-task {
  align-self: center;
  height: 85%;
}

.aside-side-menu {
  height: 100%;
}

:deep(.main-page-sidebar) {
  overflow: visible;
}
</style>
