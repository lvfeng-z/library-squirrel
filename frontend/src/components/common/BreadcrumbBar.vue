<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { isBlank } from '@renderer/utils/StringUtil.ts'

/**
 * 页面层级面包屑条：沿当前路由 meta.parent 链上溯展示「一级祖先 / … / 当前页」。
 * 祖先项可点（普通路由跳转），当前页尾项不可点；链上已注销（路由不在场）的祖先跳过显示。
 * 层级单一事实源是路由 meta.parent（宿主子页静态声明、插件子页注册期解析写入），
 * 组件不问路由出身
 */

interface BreadcrumbItem {
  name: string
  title: string
}

const route = useRoute()
const router = useRouter()

// 链上溯步数上限：环/超长链截断保护（宿主侧静态声明笔误），异常链告警
const MAX_CHAIN_STEPS = 32

// 沿 meta.parent 链自当前页面上溯收集在场祖先，一级祖先在前。
// 已注销祖先跳过显示：其路由记录已不在场，无法继续沿链上溯，链在该处截断
const ancestors = computed<BreadcrumbItem[]>(() => {
  const items: BreadcrumbItem[] = []
  const seen = new Set<string>()
  let cursor: string | undefined = route.meta.parent
  while (!isBlank(cursor) && items.length < MAX_CHAIN_STEPS) {
    const name = cursor
    if (seen.has(name)) {
      console.warn(`[BreadcrumbBar] 页面 ${String(route.name)} 的 meta.parent 链在 "${name}" 处闭环，上溯已截断`)
      break
    }
    seen.add(name)
    if (!router.hasRoute(name)) break
    const record = router.getRoutes().find((r) => r.name === name)
    items.push({ name, title: (record?.meta?.title as string | undefined) ?? name })
    cursor = record?.meta?.parent
  }
  // 收集顺序为紧邻父级 → 一级祖先，展示反转
  return items.reverse()
})

// 可见条件：当前路由 meta.parent 链非空（无可展示祖先即整体隐藏）
const isVisible = computed(() => ancestors.value.length > 0)

// 当前页尾项标题：路由 meta.title（宿主与插件路由注册均承载），缺省回退路由名
const currentTitle = computed(() => {
  const title = route.meta.title as string | undefined
  return !isBlank(title) ? title : String(route.name ?? '')
})

// 点击祖先项普通路由跳转；目标在点击瞬间已注销则不跳（链快照与点击之间存在注销窗口）
function gotoAncestor(item: BreadcrumbItem) {
  if (router.hasRoute(item.name)) {
    void router.push({ name: item.name })
  }
}
</script>

<template>
  <nav
    v-if="isVisible"
    class="breadcrumb-bar"
  >
    <template
      v-for="item in ancestors"
      :key="item.name"
    >
      <a
        class="breadcrumb-link"
        @click="gotoAncestor(item)"
      >{{ item.title }}</a>
      <span class="breadcrumb-separator">/</span>
    </template>
    <span class="breadcrumb-current">{{ currentTitle }}</span>
  </nav>
</template>

<style scoped>
/* 文档流内占位：进场/离场下方视图让位（与工作目录横幅同机制），整体高度固定一行 */
.breadcrumb-bar {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 16px;
  font-size: var(--el-font-size-base);
  color: var(--app-text-regular);
  border-bottom: 1px solid var(--app-border-color-light);
  white-space: nowrap;
  overflow: hidden;
}

.breadcrumb-link {
  color: var(--app-color-primary);
  cursor: pointer;
}

.breadcrumb-link:hover {
  text-decoration: underline;
}

.breadcrumb-separator {
  color: var(--app-text-placeholder);
}

.breadcrumb-current {
  color: var(--app-text-primary);
  font-weight: 600;
}
</style>
