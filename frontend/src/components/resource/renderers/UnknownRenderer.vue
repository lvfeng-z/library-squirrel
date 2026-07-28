<script setup lang="ts">
import { computed } from 'vue'
import { ResourceFullDTO, WorkFullDTO } from '@bindings/github.com//lvfeng-z/library-squirrel-sdk/dto'
import { QuestionFilled } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { appLauncherOpen } from '@renderer/apis/http/wrappers/appLauncher'

// 未知类型渲染器：ResourceType 嗅探后仍无法分类的资源，占位 + 外部打开兜底
const props = defineProps<{ resource: ResourceFullDTO; work: WorkFullDTO }>()

const fileName = computed(() => props.resource?.workStore?.fileName || '未知资源')
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
  <div class="unknown-renderer">
    <el-icon class="unknown-renderer-icon">
      <QuestionFilled />
    </el-icon>
    <span class="unknown-renderer-name">{{ fileName }}</span>
    <el-button
      type="primary"
      @click="handleOpen"
    >
      外部打开
    </el-button>
  </div>
</template>

<style scoped>
.unknown-renderer {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  color: var(--app-text-secondary);
}
.unknown-renderer-icon {
  font-size: 48px;
  color: var(--app-text-regular);
}
.unknown-renderer-name {
  font-size: 14px;
  word-break: break-all;
}
</style>
