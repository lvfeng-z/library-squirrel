<script setup lang="ts">
import { computed } from 'vue'
import { ResourceFullDTO, WorkFullDTO } from '@bindings/github.com//lvfeng-z/library-squirrel-sdk/dto'
import { Document as DocumentIcon } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { appLauncherOpen } from '@renderer/apis/http/wrappers/appLauncher'

// 现成文档渲染器：无内联查看器，占位展示文件名 + 外部打开按钮（系统默认应用）
const props = defineProps<{ resource: ResourceFullDTO; work: WorkFullDTO }>()

const fileName = computed(() => props.resource?.workStore?.fileName || '文档')
const filePath = computed(() => props.resource?.workStore?.filePath ?? '')

async function handleOpen() {
  if (filePath.value) {
    await appLauncherOpen(filePath.value)
  } else {
    ElMessage.error('无法打开，获取资源路径失败')
  }
}
</script>

<template>
  <div class="document-renderer">
    <el-icon class="document-renderer-icon">
      <DocumentIcon />
    </el-icon>
    <span class="document-renderer-name">{{ fileName }}</span>
    <el-button
      type="primary"
      @click="handleOpen"
    >
      外部打开
    </el-button>
  </div>
</template>

<style scoped>
.document-renderer {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  color: var(--app-text-secondary);
}
.document-renderer-icon {
  font-size: 48px;
  color: var(--app-text-regular);
}
.document-renderer-name {
  font-size: 14px;
  word-break: break-all;
}
</style>
