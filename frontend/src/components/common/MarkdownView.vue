<script setup lang="ts">
import { computed } from 'vue'
import MarkdownIt from 'markdown-it'
import DOMPurify from 'dompurify'
import { buildStoreUrl } from '@renderer/utils/UrlUtil.ts'

// 内嵌图引用:按 .md 中 ![]() 出现顺序对应(M D3-B 位置绑定);filePath 为 store 落盘路径
interface InlineImageRef {
  filePath?: string | null
}

const props = defineProps<{
  markdown: string
  imageStores?: InlineImageRef[]
}>()

// markdown-it:html:false 转义裸 HTML(XSS 深度防御层 1);linkify 自动识别纯文本 URL
const md = new MarkdownIt({
  html: false,
  linkify: true,
  breaks: false
})

// 覆盖 image 渲染:第 k 个 image token → 第 k 个内嵌图 store(M D3-B 位置绑定)。
// .md 中 ![](NNN.ext) 的 basename 退化为占位,实际图 URL 由 store 顺序(store_seq 升序)决定,非 basename 匹配。
// 用 env 传 imageStores + 计数器(per-render 独立,多组件实例安全)。
md.renderer.rules.image = (tokens, idx, options, env, self) => {
  const token = tokens[idx]
  const envStore = env as { __imageStores?: InlineImageRef[]; __imgCounter?: { v: number } }
  const envCounter = (envStore.__imgCounter ??= { v: 0 })
  const k = envCounter.v++
  const store = envStore.__imageStores?.[k]
  if (store?.filePath) {
    token.attrSet('src', buildStoreUrl(store.filePath))
  }
  return self.renderToken(tokens, idx, options)
}

// DOMPurify:XSS 深度防御层 2。收紧白名单(只允许 md 常用标签/属性);
// DOMPurify 默认已禁 javascript: 等危险 URI,此处再收标签/属性范围
const SANITIZE_CONFIG: DOMPurify.Config = {
  ALLOWED_TAGS: ['p', 'br', 'strong', 'em', 'del', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6',
    'ul', 'ol', 'li', 'blockquote', 'code', 'pre', 'a', 'img', 'hr',
    'table', 'thead', 'tbody', 'tr', 'th', 'td'],
  // dompurify 类型仅声明 ALLOWED_ATTR 为 string[]（扁平属性白名单），tag→attrs 分组对象是其类型未覆盖的写法；cast 仅为通过类型检查，运行行为不变
  ALLOWED_ATTR: {
    img: ['src', 'alt', 'width', 'height', 'title'],
    a: ['href', 'title'],
    th: ['align'],
    td: ['align']
  } as unknown as string[],
  ALLOW_DATA_ATTR: false
}

const html = computed(() => {
  const raw = md.render(props.markdown ?? '', { __imageStores: props.imageStores })
  return DOMPurify.sanitize(raw, SANITIZE_CONFIG)
})
</script>

<template>
  <div
    class="markdown-view"
    v-html="html"
  />
</template>

<style scoped>
.markdown-view {
  width: 100%;
  max-width: 100%;
  overflow-wrap: break-word;
  word-break: break-word;
  color: var(--app-text-primary);
  font-size: 14px;
  line-height: 1.7;
}
.markdown-view :deep(h1),
.markdown-view :deep(h2),
.markdown-view :deep(h3),
.markdown-view :deep(h4),
.markdown-view :deep(h5),
.markdown-view :deep(h6) {
  margin: 0.8em 0 0.4em;
  font-weight: 600;
  color: var(--app-text-primary);
}
.markdown-view :deep(h1) { font-size: 1.5em; }
.markdown-view :deep(h2) { font-size: 1.3em; }
.markdown-view :deep(h3) { font-size: 1.15em; }
.markdown-view :deep(p) {
  margin: 0.5em 0;
}
.markdown-view :deep(img) {
  max-width: 100%;
  height: auto;
  border-radius: var(--app-radius);
}
.markdown-view :deep(a) {
  color: var(--app-color-primary);
  text-decoration: none;
}
.markdown-view :deep(a:hover) {
  text-decoration: underline;
}
.markdown-view :deep(code) {
  padding: 0.2em 0.4em;
  background-color: var(--app-fill-color);
  border-radius: var(--app-radius-sm);
  font-family: monospace;
  font-size: 0.9em;
}
.markdown-view :deep(pre) {
  padding: 0.6em 0.8em;
  background-color: var(--app-fill-color-dark);
  border-radius: var(--app-radius);
  overflow-x: auto;
}
.markdown-view :deep(pre code) {
  padding: 0;
  background-color: transparent;
}
.markdown-view :deep(blockquote) {
  margin: 0.5em 0;
  padding-left: 0.8em;
  border-left: 3px solid var(--app-border-color);
  color: var(--app-text-secondary);
}
.markdown-view :deep(ul),
.markdown-view :deep(ol) {
  padding-left: 1.5em;
  margin: 0.5em 0;
}
.markdown-view :deep(table) {
  border-collapse: collapse;
  width: 100%;
}
.markdown-view :deep(th),
.markdown-view :deep(td) {
  border: 1px solid var(--app-border-color);
  padding: 0.3em 0.6em;
}
.markdown-view :deep(hr) {
  border: none;
  border-top: 1px solid var(--app-border-color);
  margin: 1em 0;
}
</style>
