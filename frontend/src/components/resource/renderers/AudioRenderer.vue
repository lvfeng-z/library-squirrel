<script setup lang="ts">
import { computed } from 'vue'
import { ResourceFullDTO, WorkFullDTO } from '@bindings/github.com/library-squirrel/backend/base/model/dto'
import { StoreRole } from '@renderer/constants/sectionCode.ts'
import { buildStoreUrl } from '@renderer/utils/UrlUtil.ts'

// 音频渲染器:内联播放 audioMain(可播放主体,独立音频资源主体)
const props = defineProps<{
  resource: ResourceFullDTO
  work: WorkFullDTO
}>()

const audioSource = computed(() => {
  const stores = props.resource?.stores ?? []
  const audioMain = stores.find((s) => s.storeType === StoreRole.AUDIO_MAIN)
  const filePath = audioMain?.store?.filePath
  return filePath ? buildStoreUrl(filePath) : ''
})
</script>

<template>
  <audio
    v-if="audioSource"
    class="audio-renderer"
    controls
    :src="audioSource"
  />
  <div
    v-else
    class="audio-renderer-empty"
  >
    音频文件缺失
  </div>
</template>

<style scoped>
.audio-renderer {
  width: 100%;
  max-width: 600px;
}
.audio-renderer-empty {
  color: var(--app-text-secondary);
  font-size: 14px;
}
</style>
