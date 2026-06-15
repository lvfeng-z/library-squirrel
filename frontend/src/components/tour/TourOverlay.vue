<script setup lang="ts">
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useTourCenterStore, resolveTourTarget } from '@renderer/store/UseTourCenterStore'

const store = useTourCenterStore()
const { status, stepResolved, activeStep, activeStepIndex, isLastStep } = storeToRefs(store)

// 显式依赖 stepResolved，确保元素挂载后再解析 DOM
const targetEl = computed<Element | undefined>(() => {
  if (!stepResolved.value) return undefined
  const key = activeStep.value?.target.targetKey
  return key ? (resolveTourTarget(key) ?? undefined) : undefined
})

const visible = computed<boolean>({
  get: () => status.value === 'running' && stepResolved.value && !!activeStep.value,
  // 用户关闭气泡（点遮罩等）视为跳过当前向导
  set: (v: boolean) => {
    if (!v) {
      store.skip()
    }
  },
})

// el-tour-step 重建 key，确保 target 变化时重新定位
const stepKey = computed(() => `${store.activeTourId}-${activeStepIndex.value}`)

const hasPrev = computed(() => activeStepIndex.value > 0)

function handlePrev() {
  store.prev()
}

async function handleNext() {
  await store.next()
}

function handleSkip() {
  store.skip()
}
</script>

<template>
  <el-tour
    v-model="visible"
    :mask="true"
    :scroll-into-view-options="true"
  >
    <el-tour-step
      :key="stepKey"
      :target="targetEl"
      :title="activeStep?.title"
      :placement="activeStep?.placement"
    >
      <span class="tour-overlay-desc">{{ activeStep?.description }}</span>
      <div class="tour-overlay-actions">
        <el-button
          size="small"
          @click="handleSkip"
        >
          跳过
        </el-button>
        <div class="tour-overlay-actions-right">
          <el-button
            v-if="hasPrev"
            size="small"
            @click="handlePrev"
          >
            上一步
          </el-button>
          <el-button
            size="small"
            type="primary"
            @click="handleNext"
          >
            {{ isLastStep ? '完成' : '下一步' }}
          </el-button>
        </div>
      </div>
    </el-tour-step>
  </el-tour>
</template>

<style scoped>
.tour-overlay-desc {
  display: block;
  line-height: 1.5;
}

.tour-overlay-actions {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 12px;
}

.tour-overlay-actions-right {
  display: flex;
  gap: 8px;
}
</style>

<!--
  el-tour 内容通过 Teleport 渲染到 body，脱离本组件 DOM 树，
  scoped 的 :deep() 无法穿透到 Teleport 目标，故用全局样式隐藏其内置 footer
  （进度点 + 上一步/Finish 按钮），改由 el-tour-step 默认插槽内的自绘按钮接管。
  本项目仅 TourOverlay 一处使用 el-tour，全局隐藏无副作用。
-->
<style>
.el-tour__footer {
  display: none;
}
</style>
