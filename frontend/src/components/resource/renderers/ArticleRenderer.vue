<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ResourceFullDTO, WorkFullDTO } from '@bindings/github.com//lvfeng-z/library-squirrel-sdk/dto'
import { StoreRole } from '@renderer/constants/sectionCode.ts'
import { buildStoreUrl } from '@renderer/utils/UrlUtil.ts'
import MarkdownView from '@renderer/components/common/MarkdownView.vue'

// 图文文档渲染器：渲染 article 正文 .md（document store）+ 内嵌图（image stores，按 store_seq 顺序位置绑定）
const props = defineProps<{ resource: ResourceFullDTO; work: WorkFullDTO }>()

// article 正文 .md 文本（异步 fetch document store）
const articleMarkdown = ref('')

// 内嵌图引用：stores filter(image)，数组顺序=store_seq 升序（后端 ORDER BY store_seq），位置绑定依据
const articleImageStores = computed(() => {
  const stores = props.resource?.stores ?? []
  return stores
    .filter((s) => s.storeType === StoreRole.IMAGE)
    .map((s) => ({ filePath: s.store?.filePath ?? '' }))
})

// fetch document store 的 .md 文本（失败降级空正文，图绑定仍可用）
async function loadArticleMarkdown() {
  articleMarkdown.value = ''
  const stores = props.resource?.stores ?? []
  const docStore = stores.find((s) => s.storeType === StoreRole.DOCUMENT)
  const filePath = docStore?.store?.filePath
  if (!filePath) return
  try {
    const resp = await fetch(buildStoreUrl(filePath))
    if (resp.ok) {
      articleMarkdown.value = await resp.text()
    }
  } catch {
    // .md 加载失败，降级空正文
  }
}

// resource 变化时重新加载正文
watch(() => props.resource, loadArticleMarkdown, { immediate: true })
</script>

<template>
  <MarkdownView
    class="article-renderer"
    :markdown="articleMarkdown"
    :image-stores="articleImageStores"
  />
</template>

<style scoped>
.article-renderer {
  max-width: 100%;
  max-height: 100%;
  overflow-y: auto;
  padding: 0 10px;
}
</style>
