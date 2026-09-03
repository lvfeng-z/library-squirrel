<script setup lang="ts">
import { useWorkdirStatusStore } from '@renderer/store/UseWorkdirStatusStore.ts'
import { useTourCenterStore } from '@renderer/store/UseTourCenterStore.ts'

const workdirStatus = useWorkdirStatusStore()
const tourCenter = useTourCenterStore()

// 「开始使用」：关弹窗记已看过欢迎，仍未配置则升常驻横幅
function handleStartUsing(): void {
  workdirStatus.closeWelcome('start-using')
}

// 「查看新手向导」：先关弹窗，再启动工作目录向导——跳设置页由向导引擎按首步 target.route 完成
async function handleViewTour(): Promise<void> {
  workdirStatus.closeWelcome('view-tour')
  await tourCenter.start('workdir-setup').catch((e) => console.warn('[WelcomeDialog] 打开工作目录配置向导失败', e))
}
</script>

<template>
  <el-dialog
    v-model="workdirStatus.welcomeVisible"
    modal-class="welcome-overlay"
    width="420px"
    align-center
    append-to-body
    :show-close="false"
    :close-on-click-modal="false"
    :close-on-press-escape="false"
  >
    <div class="welcome-body">
      <div class="welcome-title">欢迎使用 LibrarySquirrel</div>
      <div class="welcome-desc">个人资源库——从远程站点下载资源到本地，用标签组织你的收藏</div>
      <div class="welcome-actions">
        <el-button
          size="large"
          @click="handleStartUsing"
        >
          开始使用
        </el-button>
        <el-button
          type="primary"
          size="large"
          @click="handleViewTour"
        >
          查看新手向导
        </el-button>
      </div>
    </div>
  </el-dialog>
</template>

<style scoped>
.welcome-body {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  padding: 12px 8px 8px;
}

.welcome-title {
  font-size: var(--el-font-size-large);
  font-weight: 600;
  color: var(--app-text-primary);
}

.welcome-desc {
  font-size: var(--el-font-size-base);
  line-height: 1.6;
  color: var(--app-text-regular);
}

.welcome-actions {
  display: flex;
  justify-content: center;
  gap: 16px;
  margin-top: 12px;
}

/* flex gap 接管按钮间距，清掉 Element Plus 相邻按钮默认 margin-left */
.welcome-actions .el-button + .el-button {
  margin-left: 0;
}
</style>

<!--
  欢迎弹窗遮罩透明化：modal=true 保留遮罩的点击拦截（模态），modal-class 落在
  Teleport 到 body 的遮罩元素上，scoped 样式无法命中，故用全局样式把该实例的
  遮罩背景改透明——模态而无暗色视觉遮罩
-->
<style>
.welcome-overlay {
  background-color: transparent;
}
</style>
