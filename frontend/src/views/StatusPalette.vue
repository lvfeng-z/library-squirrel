<script setup lang="ts">
import BaseView from '@renderer/views/BaseView.vue'
import StatusTag from '@renderer/components/common/StatusTag.vue'
import { STATUS_REGISTRY, type StatusCategory } from '@renderer/constants/StatusRegistry'

/** 8 个 tone：6 个纯 tone（无 registry 条目）；pending 与 source 两项跟随主题主色（source 既是 tone 又在 registry），其余标注文案与色相 */
interface ToneItem {
  key: string
  label: string
  hue?: number
  note?: string
}
const TONES: ToneItem[] = [
  { key: 'active', label: '进行中', hue: 25 },
  { key: 'done', label: '完成', hue: 101 },
  { key: 'fail', label: '失败', hue: 0 },
  { key: 'warn', label: '警示', hue: 39 },
  { key: 'pending', label: '待激活', note: '跟随主题主色' },
  { key: 'idle', label: '空闲', hue: 220 },
  { key: 'source-local', label: '本地来源', note: '跟随主题主色' },
  { key: 'source-site', label: '站点来源', note: '跟随主题主色' }
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
        <div class="section-label">Tone 原色（8）—— 全主题统一 default 基准；pending 与 source 两项跟随主题主色，其余 hue 为 default 近似值，仅供参考</div>
        <div class="tag-grid">
          <div v-for="t in TONES" :key="t.key" class="tag-cell">
            <StatusTag :status="t.key">{{ t.label }}</StatusTag>
            <span class="tag-meta">{{ t.key }} · {{ t.note ?? 'hue ' + t.hue }}</span>
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
