<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { Close } from '@element-plus/icons-vue'
import DynamicSideMenu from '@renderer/components/slot/DynamicSideMenu.vue'
import NotificationList from '@renderer/components/oneOff/NotificationList.vue'
import ReminderStack from '@renderer/components/common/ReminderStack.vue'
import ReplaceConfirmDialog from '@renderer/components/dialogs/ReplaceConfirmDialog.vue'
import ChangeConfirmDialog from '@renderer/components/dialogs/ChangeConfirmDialog.vue'
import DialogSlotRenderer from '@renderer/components/slot/DialogSlotRenderer.vue'
import TourOverlay from '@renderer/components/tour/TourOverlay.vue'
import ShareReceiveDialog from '@renderer/components/dialogs/ShareReceiveDialog.vue'
import { usePluginUpdateStore } from '@renderer/store/UsePluginUpdateStore.ts'
import { useWorkdirStatusStore } from '@renderer/store/UseWorkdirStatusStore.ts'
import { useTourCenterStore } from '@renderer/store/UseTourCenterStore.ts'

const router = useRouter()
const route = useRoute()
const notificationListState = ref(false)
const workdirStatus = useWorkdirStatusStore()
const tourCenter = useTourCenterStore()

// 外壳挂载即拉取插件更新待办：启动期检测（pre-Run InstallBundled）的结果在此进入前端，
// 「插件」菜单红点与管理页待更新区块均消费本 store
onMounted(() => {
  usePluginUpdateStore().refresh()
  // 工作目录未配置的初始判定（拉取式）：向导未完成直接打开首启向导，否则展示常驻横幅；
  // 运行期由 workdir:unconfigured 事件（MainIpcListener）升格横幅
  void workdirStatus.refresh()
})

// 向导结束（完成/跳过）时工作目录仍未配置 → 升常驻横幅（首启向导被跳过时的补升）
watch(() => tourCenter.isActive, (active, was) => {
  if (was && !active) {
    workdirStatus.onTourEnded()
  }
})

// 横幅点击跳设置页完成工作目录配置
async function gotoSettings() {
  await router.push({ name: 'settings' })
}

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

      <el-main class="main-page-content">
        <!-- 工作目录未配置常驻横幅：文档流内占位、下方视图区让出等高空间，点击跳设置页（warn tone 令牌配色） -->
        <div
          v-if="workdirStatus.bannerVisible"
          class="workdir-banner z-layer-3"
          @click="gotoSettings"
        >
          <span>工作目录未配置，资源下载、备份与文件服务均不可用</span>
          <span class="workdir-banner-action">前往设置</span>
        </div>
        <div class="main-view-host">
          <router-view />
        </div>
      </el-main>
    </el-container>

    <!-- 弹窗组件 -->
    <notification-list
      class="main-background-task z-layer-3"
      :state="notificationListState"
    />
    <ReplaceConfirmDialog />
    <ChangeConfirmDialog />
    <DialogSlotRenderer />

    <!-- 接收分享对话框（深链到达/手动入口共用；挂载于外壳以跨页面生效） -->
    <ShareReceiveDialog />

    <!-- 消息提醒堆叠（右上角，聚合窗口合并批量提醒） -->
    <ReminderStack />

    <!-- 向导覆盖层（新向导系统） -->
    <TourOverlay />
  </div>
</template>

<style scoped>
.layout {
  display: flex;
  flex-direction: row;
  width: 100%;
  height: 100%;
  background-color: var(--app-bg-page);
  --side-menu-background-color: var(--app-bg-sidebar);
}

.close-subpage-button {
  display: flex;
  justify-content: end;
  align-items: end;
  background-color: var(--app-color-danger);
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
  background-color: var(--app-color-danger-light-3);
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

/* 内容区纵向 flex：横幅文档流占位取自然高度，视图宿主吃剩余高度，两者恰好填满不溢出 */
.main-page-content {
  display: flex;
  flex-direction: column;
  padding: 0;
}

/* 视图宿主：各视图根的 height:100%/calc(100%-*) 以本容器为基准，横幅升降时视图随之伸缩 */
.main-view-host {
  flex: 1;
  min-height: 0;
}

.workdir-banner {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  padding: 8px 16px;
  cursor: pointer;
  font-size: var(--el-font-size-base);
  background-color: var(--app-status-warn-bg);
  color: var(--app-status-warn-text);
  border-bottom: 1px solid var(--app-status-warn-border);
}

.workdir-banner-action {
  font-weight: 600;
  text-decoration: underline;
}

.aside-side-menu {
  height: 100%;
}

:deep(.main-page-sidebar) {
  overflow: visible;
}
</style>
