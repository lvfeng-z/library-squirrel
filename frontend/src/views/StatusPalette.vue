<script setup lang="ts">
import BaseView from '@renderer/views/BaseView.vue'
import StatusTag from '@renderer/components/common/StatusTag.vue'
import { STATUS_REGISTRY, type StatusCategory } from '@renderer/constants/StatusRegistry'

/** 8 个 tone：6 个纯 tone（无 registry 条目）手动标注文案与色相；source 两个既是 tone 又在 registry */
interface ToneItem {
  key: string
  label: string
  hue: number
}
const TONES: ToneItem[] = [
  { key: 'active', label: '进行中', hue: 30 },
  { key: 'done', label: '完成', hue: 120 },
  { key: 'fail', label: '失败', hue: 0 },
  { key: 'warn', label: '警示', hue: 55 },
  { key: 'pending', label: '待激活', hue: 325 },
  { key: 'idle', label: '空闲', hue: 220 },
  { key: 'source-local', label: '本地来源', hue: 170 },
  { key: 'source-site', label: '站点来源', hue: 275 }
]

/** 状态别名按类目分组展示，文案由 StatusRegistry 提供 */
const CATEGORY_TITLES: { category: StatusCategory; title: string }[] = [
  { category: 'task', title: '任务状态（task）' },
  { category: 'source', title: '来源类型（source）' },
  { category: 'toggle', title: '开关/运行态（toggle）' },
  { category: 'resource', title: '资源/作品状态（resource）' },
  { category: 'plugin', title: '插件来源/信任（plugin）' },
  { category: 'backup', title: '备份引用态（backup）' },
  { category: 'recycle', title: '回收站文件条目（recycle）' }
]
const statusByCategory = (cat: StatusCategory) =>
  Object.values(STATUS_REGISTRY).filter(item => item.category === cat)
</script>

<template>
  <base-view>
    <div class="palette-page">
      <h2 class="palette-title">状态语义 tone 色板</h2>

      <section class="palette-section">
        <div class="section-label">Tone 原色（8）—— 随当前主题变化（default 为基色，各主题在 theme-*.css 覆盖；下方 hue 为 default 近似值，仅供参考）</div>
        <div class="tag-grid">
          <div v-for="t in TONES" :key="t.key" class="tag-cell">
            <StatusTag :status="t.key">{{ t.label }}</StatusTag>
            <span class="tag-meta">{{ t.key }} · hue {{ t.hue }}</span>
          </div>
        </div>
      </section>

      <section v-for="c in CATEGORY_TITLES" :key="c.category" class="palette-section">
        <div class="section-label">{{ c.title }}</div>
        <div class="tag-grid">
          <div v-for="item in statusByCategory(c.category)" :key="item.key" class="tag-cell">
            <StatusTag :status="item.key" />
            <span class="tag-meta">{{ item.key }}</span>
          </div>
        </div>
      </section>
    </div>
  </base-view>
</template>

<style scoped>
.palette-page {
  width: 100%;
  height: 100%;
  padding: 20px 24px;
  overflow: auto;
  background: var(--app-bg-page);
}
.palette-title {
  margin: 0 0 16px;
  font-size: 18px;
  font-weight: 600;
  color: var(--app-text-primary);
}
.palette-section {
  margin-bottom: 24px;
}
.section-label {
  margin-bottom: 12px;
  font-size: 13px;
  color: var(--app-text-secondary);
}
.tag-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 16px 20px;
}
.tag-cell {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 6px;
}
.tag-meta {
  font-size: 11px;
  color: var(--app-text-placeholder);
  font-family: monospace;
}
</style>
