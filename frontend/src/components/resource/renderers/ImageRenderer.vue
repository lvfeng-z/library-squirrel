<script setup lang="ts">
import { computed, ref } from 'vue'
import { ResourceFullDTO, WorkFullDTO } from '@bindings/github.com//lvfeng-z/library-squirrel-sdk/dto'
import { Picture } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { buildStoreUrl } from '@renderer/utils/UrlUtil.ts'
import { appLauncherOpenImage } from '@renderer/apis/http/wrappers/appLauncher'

// 图片渲染器：展示原图（workStore，ResolvePrimaryStore 派生的展示主体），点击用应用内图片查看器打开
// 两种模式：contain=全部展示（完整缩放不滚动）；fill=较短边占满（较长边溢出可滚动）
const props = defineProps<{
  resource: ResourceFullDTO
  work: WorkFullDTO
}>()

const imagePath = computed(() => props.resource?.workStore?.filePath ?? '')

// 展示模式：默认全部展示（完整可见，不滚动）
const fitMode = ref<'contain' | 'fill'>('contain')
// 图片自然方向（load 后据 naturalWidth/Height 判定），fill 模式下决定撑宽还是撑高
const orientation = ref<'portrait' | 'landscape'>('landscape')

function toggleFitMode() {
  fitMode.value = fitMode.value === 'contain' ? 'fill' : 'contain'
}

// 图片加载完成后据自然尺寸判定方向（高>=宽=portrait，否则 landscape）
function handleImageLoad(e: Event) {
  const img = e.target as HTMLImageElement
  orientation.value = img.naturalHeight >= img.naturalWidth ? 'portrait' : 'landscape'
}

async function handlePictureClicked() {
  const filePath = imagePath.value
  if (filePath) {
    await appLauncherOpenImage(filePath)
  } else {
    ElMessage.error('无法打开，获取资源路径失败')
  }
}
</script>

<template>
  <div
    class="image-renderer-wrap"
    :class="[`mode-${fitMode}`, `orient-${orientation}`]"
  >
    <el-image
      class="image-renderer"
      :src="imagePath ? buildStoreUrl(imagePath) : ''"
      @click="handlePictureClicked"
      @load="handleImageLoad"
    >
      <template #error>
        <div class="image-renderer-error">
          <el-icon class="image-renderer-error-icon">
            <Picture />
          </el-icon>
        </div>
      </template>
    </el-image>
    <el-button
      class="image-renderer-toggle"
      size="small"
      circle
      :icon="fitMode === 'contain' ? 'FullScreen' : 'ScaleToOriginal'"
      :title="fitMode === 'contain' ? '切换为较短边占满' : '切换为全部展示'"
      @click="toggleFitMode"
    />
  </div>
</template>

<style scoped>
.image-renderer-wrap {
  width: 100%;
  height: 100%;
  position: relative;
  display: flex;
  justify-content: center;
  align-items: center;
}
/*
 * el-image 容器尺寸固定撑满 wrap、不随模式切换，且 overflow visible——
 * 容器不过渡，img 的 100% 才有稳定目标值（避免嵌套 auto+百分比导致切换瞬间图片先坍缩到极小再放大）；
 * 容器不裁剪，让 fill 模式超高的 img 溢出到 wrap，由 wrap 提供滚动
 */
.image-renderer {
  width: 100%;
  height: 100%;
  overflow: visible;
}
.image-renderer :deep(.el-image__inner) {
  /* interpolate-size 启用 width/height 在 100% 与 auto 之间双向过渡（Chromium 129+/WebView2 支持，不支持则回退单向） */
  interpolate-size: allow-keywords;
  object-fit: contain;
  display: block;
  cursor: pointer;
  transition: width 0.3s ease, height 0.3s ease, filter 0.3s ease;
}
.image-renderer :deep(.el-image__inner:hover) {
  filter: drop-shadow(var(--app-shadow-card));
}

/* 模式1：全部展示（完整缩放、不滚动） */
.mode-contain {
  overflow: hidden;
}
.mode-contain .image-renderer :deep(.el-image__inner) {
  width: 100%;
  height: 100%;
}

/* 模式2：较短边占满（较长边溢出、双向可滚动，从顶/左起始） */
.mode-fill {
  overflow: auto;
  align-items: flex-start;
  justify-content: flex-start;
}
/* 竖图（高>宽）：宽度撑满容器，高度按比例超出 → 垂直滚动 */
.mode-fill.orient-portrait .image-renderer :deep(.el-image__inner) {
  width: 100%;
  height: auto;
}
/* 横图（宽>高）：高度撑满容器，宽度按比例超出 → 水平滚动 */
.mode-fill.orient-landscape .image-renderer :deep(.el-image__inner) {
  height: 100%;
  width: auto;
}

/* 切换按钮：右上角浮层 */
.image-renderer-toggle {
  position: absolute;
  top: 8px;
  right: 8px;
  z-index: 10;
}
.image-renderer-error {
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
  background-color: var(--app-fill-color-dark);
  width: 200px;
  height: 100%;
}
.image-renderer-error-icon {
  color: var(--app-text-secondary);
  scale: 2;
}
</style>
