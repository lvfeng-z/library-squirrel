<script setup lang="ts">
import { computed } from 'vue'
import { ResourceFullDTO, WorkFullDTO } from '@bindings/github.com//lvfeng-z/library-squirrel-sdk/dto'
import { StoreRole } from '@renderer/constants/sectionCode.ts'
import { buildStoreUrl } from '@renderer/utils/UrlUtil.ts'

// 视频渲染器：内联播放，优先 merged（音视频已合并），否则 videoTrack（未合并的视频轨）
const props = defineProps<{
  resource: ResourceFullDTO
  work: WorkFullDTO
}>()

const videoSource = computed(() => {
  const stores = props.resource?.stores ?? []
  const merged = stores.find((s) => s.storeType === StoreRole.MERGED)
  const track = stores.find((s) => s.storeType === StoreRole.VIDEO_TRACK)
  const filePath = merged?.store?.filePath || track?.store?.filePath
  return filePath ? buildStoreUrl(filePath) : ''
})
</script>

<template>
  <video
    v-if="videoSource"
    class="video-renderer"
    controls
    :src="videoSource"
  />
  <div
    v-else
    class="video-renderer-empty"
  >
    视频文件缺失
  </div>
</template>

<style scoped>
.video-renderer {
  max-width: 100%;
  max-height: 100%;
}
.video-renderer-empty {
  color: var(--app-text-secondary);
  font-size: 14px;
}
</style>
