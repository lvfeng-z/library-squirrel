<script setup lang="ts">
import { computed, defineAsyncComponent, h, ref, watch, type Component } from 'vue'
import { ResourceFullDTO, WorkFullDTO } from '@bindings/github.com/library-squirrel/backend/base/model/dto'
import { Context as RenderContext } from '@bindings/github.com/lvfeng-z/library-squirrel-sdk/dto/render'
import { Loading } from '@element-plus/icons-vue'
import { getResourcePreviewType } from '@renderer/utils/ResourceUtil.ts'
import { ResourceType } from '@renderer/constants/sectionCode.ts'
import { useHandlerRegistryStore } from '@renderer/store/HandlerRegistryStore'
import { workToRenderContext } from '@renderer/apis/http/wrappers/work'
import PluginBoundary from '@renderer/components/common/PluginBoundary.vue'
import ImageRenderer from './renderers/ImageRenderer.vue'
import VideoRenderer from './renderers/VideoRenderer.vue'
import ArticleRenderer from './renderers/ArticleRenderer.vue'
import DocumentRenderer from './renderers/DocumentRenderer.vue'
import UnknownRenderer from './renderers/UnknownRenderer.vue'

// 作品资源展示主体：按 ResourceType 分发内置渲染器；插件渲染器命中时覆盖内置（被动响应型扩展）
const props = defineProps<{
  resource: ResourceFullDTO
  work: WorkFullDTO
}>()

const handlerStore = useHandlerRegistryStore()

// 展示类型（含 unknown 降级嗅探）
const previewType = computed(() => getResourcePreviewType(props.resource))

// 插件渲染器：HandlerRegistry 按 resourceType 查找命中者
const pluginHandler = computed(() => handlerStore.resourceViewerByType(previewType.value))

// 加载中/失败占位（与 EmbedSlotRenderer 一致）
const LoadingComponent: Component = {
  render() {
    return h(Loading, { class: 'is-loading', size: 24 })
  }
}
const ErrorComponent: Component = {
  render() {
    return h('span', { style: 'color: var(--app-color-danger); font-size: 12px;' }, '渲染器加载失败')
  }
}

// handler.component 是 loader（(): Promise<DefineComponent>），包成异步组件供 PluginBoundary 渲染
const pluginComponent = computed(() => {
  const handler = pluginHandler.value
  if (!handler) return null
  return defineAsyncComponent({
    loader: () => handler.component(),
    loadingComponent: LoadingComponent,
    errorComponent: ErrorComponent,
    delay: 100,
    timeout: 5000
  })
})

// 插件渲染器注入的稳定契约 props（render.Context）：主程序展示 WorkFullDTO 经后端单向投射而来，
// 此后独立演进——主程序展示 DTO 变更不传导，破坏性变更由主程序 contractVersion 约束。随 work 变化异步刷新。
const renderContext = ref<RenderContext | null>(null)
watch(
  () => props.work,
  async (work) => {
    if (!work) {
      renderContext.value = null
      return
    }
    try {
      const resp = await workToRenderContext(work)
      renderContext.value = resp.data ?? null
    } catch {
      renderContext.value = null
    }
  },
  { immediate: true }
)
</script>

<template>
  <div class="resource-viewer">
    <PluginBoundary
      v-if="pluginComponent && renderContext"
      :component="pluginComponent"
      :component-props="{ context: renderContext }"
      :name="pluginHandler?.slotId"
    />
    <ImageRenderer
      v-else-if="previewType === ResourceType.IMAGE"
      :resource="resource"
      :work="work"
    />
    <VideoRenderer
      v-else-if="previewType === ResourceType.VIDEO"
      :resource="resource"
      :work="work"
    />
    <ArticleRenderer
      v-else-if="previewType === ResourceType.ARTICLE"
      :resource="resource"
      :work="work"
    />
    <DocumentRenderer
      v-else-if="previewType === ResourceType.DOCUMENT"
      :resource="resource"
      :work="work"
    />
    <UnknownRenderer
      v-else
      :resource="resource"
      :work="work"
    />
  </div>
</template>

<style scoped>
.resource-viewer {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
}
</style>
