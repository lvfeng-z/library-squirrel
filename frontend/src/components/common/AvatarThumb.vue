<script setup lang="ts">
import { computed } from 'vue'
import { UserFilled } from '@element-plus/icons-vue'
import { buildStoreUrl } from '@renderer/utils/UrlUtil.ts'
import { isBlank } from '@renderer/utils/StringUtil.ts'

// 作者头像缩略图：头像文件 workDir 相对路径 → /store/ URL 展示；
// 无头像（null/缺省/空串）与加载失败统一降级为占位图标，调用方无需自行兜底

// props
const props = withDefaults(
  defineProps<{
    // 头像文件 workDir 相对路径（正斜杠；null/缺省/空串=无头像，走占位图标）
    filePath?: string | null
    // 展示尺寸（px 数值，宽高同值）
    size?: number
  }>(),
  {
    filePath: null,
    size: 32
  }
)

// 变量
// 头像展示 URL；无头像时为空串（空串走占位分支，不进 el-image）
const avatarUrl = computed<string>(() => {
  const path = props.filePath
  return isBlank(path) ? '' : buildStoreUrl(path)
})
// 占位图标尺寸：约为容器尺寸的一半（小尺寸 32px → 16px，与正文小图标观感一致）
const placeholderIconSize = computed<number>(() => Math.max(Math.round(props.size / 2), 12))
</script>

<template>
  <el-image
    v-if="avatarUrl !== ''"
    :src="avatarUrl"
    fit="cover"
    class="avatar-thumb-image"
    :style="{ width: `${props.size}px`, height: `${props.size}px` }"
  >
    <template #error>
      <div
        class="avatar-thumb-placeholder"
        :style="{ width: `${props.size}px`, height: `${props.size}px` }"
      >
        <el-icon :size="placeholderIconSize">
          <UserFilled />
        </el-icon>
      </div>
    </template>
  </el-image>
  <div
    v-else
    class="avatar-thumb-placeholder"
    :style="{ width: `${props.size}px`, height: `${props.size}px` }"
  >
    <el-icon :size="placeholderIconSize">
      <UserFilled />
    </el-icon>
  </div>
</template>

<style scoped>
.avatar-thumb-image {
  flex-shrink: 0;
  border-radius: 50%;
}
.avatar-thumb-image :deep(.el-image__inner) {
  border-radius: 50%;
}
.avatar-thumb-placeholder {
  display: flex;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  box-sizing: border-box;
  border: 1px solid var(--app-border-color);
  border-radius: 50%;
  background-color: var(--app-fill-color-light);
  color: var(--app-text-secondary);
}
</style>
