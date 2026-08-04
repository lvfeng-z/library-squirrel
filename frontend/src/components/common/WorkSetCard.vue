<script setup lang="ts">
import { computed, Ref, ref } from 'vue'
import { notNullish } from '@renderer/utils/CommonUtil.ts'
import { ElMessage } from 'element-plus'
import { Picture } from '@element-plus/icons-vue'
import { WorkSetWithCoverDTO } from '@bindings/github.com/library-squirrel/backend/base/model/dto'
import { appLauncherOpen, appLauncherOpenImage } from '@renderer/apis/http/wrappers/appLauncher'
import { buildStoreUrl } from '@renderer/utils/UrlUtil.ts'
import { getResourceOpenPath, getResourcePreviewType } from '@renderer/utils/ResourceUtil.ts'
import { ResourceType } from '@renderer/constants/sectionCode.ts'

// props
const props = defineProps<{
  workSet: WorkSetWithCoverDTO
  checkable: boolean
  maxHeight?: number
  maxWidth?: number
}>()

const checked = defineModel<boolean>('checked', { required: false, default: false })

// 事件
const emit = defineEmits(['imageClicked'])

// 变量
const imageFit: Ref<'contain' | 'cover' | 'fill' | 'none' | 'scale-down'> = ref('contain')
const caseHeight: Ref<string> = computed(() => (props.maxHeight === undefined ? 'auto' : String(props.maxHeight) + 'px'))
// 封面资源路径：优先缩略图，仅图片类型才用 workStore，否则返回空触发 error 插槽
const coverFilePath: Ref<string> = computed(() => {
  const resource = props.workSet.coverResource
  // 1. 优先使用缩略图
  if (resource?.thumbnailStore?.filePath) {
    return resource.thumbnailStore.filePath
  }
  // 2. 无缩略图时，仅图片类型返回路径
  if (resource?.workStore?.filePath && getResourcePreviewType(resource) === ResourceType.IMAGE) {
    return resource.workStore.filePath
  }
  return ''
})
let clickTimeout

// 方法
// 判断el-image使用什么模式
function handleElImageFit() {
  imageFit.value = 'contain'
}
// 处理图片被点击
function handleImageClicked() {
  clearTimeout(clickTimeout)
  clickTimeout = setTimeout(() => emit('imageClicked', props.workSet), 300)
}
// 处理图片双击事件
async function handlePictureClicked() {
  clearTimeout(clickTimeout)
  const filePath = getResourceOpenPath(props.workSet.coverResource)
  if (notNullish(filePath) && filePath !== '') {
    // 图片用应用内图片查看；其他类型用系统默认应用打开
    if (getResourcePreviewType(props.workSet.coverResource) === ResourceType.IMAGE) {
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
// 获取作品集名称
function getWorkSetName(): string {
  return props.workSet.workSet?.nickName || props.workSet.workSet?.siteWorkSetName || '未命名作品集'
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
    <el-image
      :fit="imageFit"
      class="work-card-image"
      :src="coverFilePath ? buildStoreUrl(coverFilePath) : ''"
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
    <div class="work-card-info">
      {{ getWorkSetName() }}
    </div>
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
