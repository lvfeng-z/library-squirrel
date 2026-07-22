<script setup lang="ts">
import WorkInfo from './WorkInfo.vue'
import AuthorInfo from './AuthorInfo.vue'
import { computed, Ref, ref } from 'vue'
import { notNullish } from '@renderer/utils/CommonUtil.ts'
import { ElMessage } from 'element-plus'
import { Picture } from '@element-plus/icons-vue'
import WorkCardItem from '@renderer/model/dto/WorkCardItem.ts'
import { appLauncherOpen, appLauncherOpenImage } from '@renderer/apis/http/wrappers/appLauncher'
import { buildStoreUrl } from '@renderer/utils/UrlUtil.ts'
import { getResourceOpenPath, getResourcePreviewType } from '@renderer/utils/ResourceUtil.ts'
import { ResourceType } from '@renderer/constants/sectionCode.ts'

// props
const props = defineProps<{
  work: WorkCardItem
  checkable: boolean
  maxHeight?: number
  maxWidth?: number
  workInfoPopperWidth?: string
  authorInfoPopperWidth?: string
}>()

const checked = defineModel<boolean>('checked', { required: false, default: false })

// 事件
const emit = defineEmits(['imageClicked'])

// 变量
const imagePath: Ref<string> = computed(() => {
  const resource = props.work.resource
  // 1. 优先使用缩略图
  if (resource?.thumbnailStore?.filePath) {
    return resource.thumbnailStore.filePath
  }
  // 2. 无缩略图时，仅图片类型资源在 el-image 渲染；非图片类型返回空走 error/占位
  if (resource?.workStore?.filePath && getResourcePreviewType(resource) === ResourceType.IMAGE) {
    return resource.workStore.filePath
  }
  return ''
})
// 资源不完整(ResourceComplete=2)卡片角标提示;0(未校验)/1(完整)不显示,不阻断打开
const isResourceIncomplete: Ref<boolean> = computed(() => props.work.resource?.resourceComplete === 2)
const imageFit: Ref<'contain' | 'cover' | 'fill' | 'none' | 'scale-down'> = ref('contain')
const caseHeight: Ref<string> = computed(() => (props.maxHeight === undefined ? 'auto' : String(props.maxHeight) + 'px'))
let clickTimeout

// 方法
function handleElImageFit() {
  imageFit.value = 'contain'
}
function handleImageClicked() {
  clearTimeout(clickTimeout)
  clickTimeout = setTimeout(() => emit('imageClicked', props.work), 300)
}
async function handlePictureClicked() {
  clearTimeout(clickTimeout)
  const filePath = getResourceOpenPath(props.work.resource)
  if (notNullish(filePath) && filePath !== '') {
    // 图片用应用内图片查看；其他类型(视频/文档/article)用系统默认应用打开
    if (getResourcePreviewType(props.work.resource) === ResourceType.IMAGE) {
      await appLauncherOpenImage(filePath)
    } else {
      await appLauncherOpen(filePath)
    }
  } else {
    ElMessage({
      type: 'error',
      message: '无法打开，获取资源路径失败'
    })
  }
}
</script>

<template>
  <div class="work-card">
    <div
      v-show="checkable"
      class="work-card-checkmark-container z-layer-1"
    >
      <div
        class="work-card-checkmark"
        @click.stop="checked = !checked"
      >
        <el-icon
          v-if="checked && checkable"
          class="work-card-icon-checked"
        >
          <Check />
        </el-icon>
      </div>
    </div>
    <el-tag
      v-if="isResourceIncomplete"
      type="warning"
      size="small"
      effect="dark"
      :style="{ position: 'absolute', top: '4px', left: '4px', zIndex: 2 }"
    >不完整</el-tag>
    <el-image
      :fit="imageFit"
      class="work-card-image"
      :src="imagePath ? buildStoreUrl(imagePath) : ''"
      @load="handleElImageFit"
      @click="handleImageClicked"
      @dblclick="handlePictureClicked"
    >
      <template #error>
        <div class="work-card-error">
          <el-icon class="work-card-error-icon">
            <Picture />
          </el-icon>
        </div>
      </template>
    </el-image>
    <work-info
      class="work-card-info"
      :work="work"
      :width="workInfoPopperWidth"
    />
    <author-info
      class="work-card-info"
      :local-authors="props.work.localAuthors"
      :site-authors="props.work.siteAuthors"
      :width="authorInfoPopperWidth"
    />
  </div>
</template>

<style scoped>
.work-card {
  display: flex;
  flex-direction: column;
  position: relative;
}
.work-card-image {
  width: auto;
  margin-top: auto;
  margin-bottom: auto;
  cursor: pointer;
  max-height: calc(v-bind(caseHeight) - 100px);
  border-radius: var(--app-radius-lg);
}
.work-card-error {
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
  height: 200px;
  width: 100%;
}
.work-card-error-icon {
  color: var(--app-text-secondary);
  scale: 2;
}
.work-card-info {
  width: calc(100% - 10px);
  display: flex;
  overflow: hidden;
  text-overflow: ellipsis;
  word-wrap: break-word;
  white-space: nowrap;
  border-radius: var(--app-radius);
  margin-left: 3px;
  margin-right: 3px;
  margin-top: 3px;
  padding-left: 4px;
  transition: background-color 0.3s;
}
.work-card-info:hover {
  background-color: var(--app-fill-color);
}
.work-card-checkmark-container {
  position: absolute;
  top: 8px;
  right: 8px;
}
.work-card-checkmark {
  width: 20px;
  height: 20px;
  border: 2px solid var(--app-color-primary-light-3);
  border-radius: var(--app-radius);
  background-color: var(--app-bg-surface);
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  pointer-events: visibleFill;
  transition: all 0.2s;
  position: static;
}
.work-card-checkmark:hover {
  border-color: var(--app-color-primary);
}
.work-card-icon-checked {
  color: var(--app-color-primary);
  font-size: 15px;
  transition: 0.3s;
}
.work-card-icon-checked:hover {
  scale: 1.2;
}
</style>
