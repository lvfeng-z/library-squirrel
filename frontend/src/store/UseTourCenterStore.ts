import { defineStore } from 'pinia'
import { nextTick } from 'vue'
import { getRouterInstance } from '@renderer/store/SlotRegistryStore'
import { settingsGetSettings, settingsSaveSettings } from '@renderer/apis/http/wrappers/settings'
import ApiUtil from '@renderer/utils/ApiUtil.ts'
import type { SettingChange } from '@bindings/github.com/library-squirrel/backend/settings/models'
import type { TourContext, TourDefinition, TourStep, TourStatus } from '@renderer/model/tour/TourDefinition'

// 目标元素等待轮询参数
const TARGET_WAIT_INTERVAL = 50
const TARGET_WAIT_TIMEOUT = 3000

/** 向导结束原因：done = 走完全部步骤；skip = 跳过（气泡「跳过」按钮 / 遮罩关闭 / 向导中心「结束」） */
export type TourEndReason = 'done' | 'skip'

/** 最近一次向导结束记录（start 时清空、finish 时写入，供外部编排消费——如工作目录向导走完后的自动接续） */
export interface TourEndedInfo {
  tourId: string
  reason: TourEndReason
}

// ============ 模块级：目标元素注册表 ============
const targetResolvers = new Map<string, () => Element | null | undefined>()

/**
 * 注册可被向导高亮的元素解析函数
 */
export function registerTourTarget(key: string, resolver: () => Element | null | undefined) {
  targetResolvers.set(key, resolver)
}

/**
 * 注销目标元素
 */
export function unregisterTourTarget(key: string) {
  targetResolvers.delete(key)
}

/**
 * 解析目标元素（供 TourOverlay 渲染时取 DOM）
 */
export function resolveTourTarget(key: string): Element | null | undefined {
  return targetResolvers.get(key)?.()
}

// ============ 模块级：目标页面就绪信号 ============
let readyResolveFn: ((value: void) => void) | null = null
let readyPromise: Promise<void> | null = null

/**
 * 开始等待目标页面就绪（必须在路由跳转前调用，页面 mount 后即可 reportReady）
 */
function beginReadyWait() {
  readyPromise = new Promise<void>((resolve) => {
    readyResolveFn = resolve
  })
}

/**
 * 等待目标页面就绪，带超时兜底
 */
async function awaitReady(): Promise<void> {
  if (!readyPromise) return
  await Promise.race([
    readyPromise,
    new Promise<void>((resolve) => setTimeout(resolve, TARGET_WAIT_TIMEOUT)),
  ])
}

/**
 * 目标页面报告就绪（数据已加载、元素已定位）
 */
export function reportTourReady() {
  if (readyResolveFn) {
    readyResolveFn()
    readyResolveFn = null
  }
}

// ============ Store 定义 ============
export const useTourCenterStore = defineStore('tourCenter', {
  state: () => ({
    registry: new Map<string, TourDefinition>(),
    activeTourId: null as string | null,
    activeStepIndex: 0,
    status: 'idle' as TourStatus,
    stepResolved: false,
    context: null as TourContext | null,
    completed: new Map<string, boolean>(),
    lastEnded: null as TourEndedInfo | null,
  }),
  getters: {
    activeTour(state): TourDefinition | null {
      if (!state.activeTourId) return null
      return state.registry.get(state.activeTourId) ?? null
    },
    activeStep(state): TourStep | null {
      const tour = state.activeTourId ? state.registry.get(state.activeTourId) : null
      if (!tour) return null
      return tour.steps[state.activeStepIndex] ?? null
    },
    isLastStep(state): boolean {
      const tour = state.activeTourId ? state.registry.get(state.activeTourId) : null
      return !!tour && state.activeStepIndex >= tour.steps.length - 1
    },
    tourList(state): TourDefinition[] {
      return Array.from(state.registry.values())
    },
    isActive(state): boolean {
      return state.status === 'running'
    },
  },
  actions: {
    registerTour(def: TourDefinition) {
      this.registry.set(def.id, def)
    },

    isCompleted(id: string): boolean {
      return !!this.completed.get(id)
    },

    markCompleted(id: string) {
      this.completed.set(id, true)
      void this.persist()
    },

    resetCompleted(id: string) {
      this.completed.delete(id)
      void this.persist()
    },

    /** 重置所有向导的完成状态 */
    async resetAllCompleted() {
      this.completed.clear()
      await this.persist()
    },

    /** 启动向导 */
    async start(tourId: string, payload?: Record<string, unknown>) {
      const def = this.registry.get(tourId)
      if (!def) {
        throw new Error(`向导不存在: ${tourId}`)
      }
      this.activeTourId = tourId
      this.activeStepIndex = 0
      this.status = 'running'
      this.lastEnded = null
      await this.resolveStep(0, payload)
    },

    /** 下一步（最后一步则走完结束） */
    async next() {
      const tour = this.activeTour
      if (!tour) return
      if (this.activeStepIndex >= tour.steps.length - 1) {
        await this.finish('done')
        return
      }
      this.activeStepIndex++
      await this.resolveStep(this.activeStepIndex)
    },

    /** 上一步（仅回退气泡，不重新跳路由） */
    prev() {
      if (this.activeStepIndex > 0) {
        this.activeStepIndex--
      }
    },

    /** 跳过当前向导（与走完同记完成标记，结束原因不同） */
    skip() {
      void this.finish('skip')
    },

    /**
     * 结束向导：先写结束记录（外部 watch isActive 回落时消费，此刻数据已就位），
     * 再记完成标记（走完/跳过同语义）并重置运行态
     */
    async finish(reason: TourEndReason) {
      if (this.activeTourId) {
        this.lastEnded = { tourId: this.activeTourId, reason }
        this.markCompleted(this.activeTourId)
      }
      this.reset()
    },

    /** 重置运行状态 */
    reset() {
      this.activeTourId = null
      this.activeStepIndex = 0
      this.status = 'idle'
      this.stepResolved = false
      this.context = null
      readyResolveFn = null
      readyPromise = null
    },

    /**
     * 解析单步：跳路由 + 等元素 + 等就绪，并写入 context
     */
    async resolveStep(index: number, payload?: Record<string, unknown>) {
      const tour = this.activeTour
      if (!tour) return
      const step = tour.steps[index]
      if (!step) return
      this.stepResolved = false

      // 构建上下文
      this.context = {
        tourId: tour.id,
        stepIndex: index,
        data: step.data,
        payload,
      }

      // 进入页面前的钩子
      if (step.onEnterPage) {
        await step.onEnterPage(this.context)
      }

      // 该步需要目标数据时，先注册就绪等待（必须在跳转前，页面 mount 后即可 reportReady）
      const needReady = !!step.data && step.data.kind !== 'none'
      if (needReady) {
        beginReadyWait()
      }

      // 跳转目标路由
      const router = getRouterInstance()
      if (router && router.currentRoute.value.name !== step.target.route) {
        await router.push({ name: step.target.route })
        await nextTick()
      }

      // 等待目标元素挂载
      if (step.target.targetKey) {
        await waitTargetEl(step.target.targetKey)
      }

      // 等待目标页面就绪
      if (needReady) {
        await awaitReady()
      }
      this.stepResolved = true
    },

    /** 应用启动时从 settings 加载已完成向导，并做旧向导键的存量迁移 */
    async loadCompleted() {
      const response = await settingsGetSettings()
      if (!ApiUtil.check(response)) return
      const settings = ApiUtil.data<{ tour?: { completed?: Record<string, boolean> } }>(response)
      const completed = settings?.tour?.completed
      if (completed && typeof completed === 'object') {
        this.completed = new Map(Object.entries(completed).map(([k, v]) => [k, !!v]))
      }
      await this.migrateLegacyFirstTime()
    },

    /**
     * 存量迁移：settings 中旧首启向导键 first-time 为 true 视为已看过拆分后的
     * workdir-setup 与 task-creation 两段内容——置新键、删旧键并持久化；
     * 旧键删除后不再命中，天然幂等。持久化失败时下次启动重迁，自愈
     */
    async migrateLegacyFirstTime() {
      if (this.completed.get('first-time') !== true) return
      if (!this.completed.get('workdir-setup')) {
        this.completed.set('workdir-setup', true)
      }
      if (!this.completed.get('task-creation')) {
        this.completed.set('task-creation', true)
      }
      this.completed.delete('first-time')
      try {
        await this.persist()
      } catch (e) {
        console.warn('[tourCenter] 首启向导存量迁移持久化失败', e)
      }
    },

    /** 持久化已完成状态到 settings */
    async persist() {
      const changes: SettingChange[] = [
        { path: 'tour.completed', value: Object.fromEntries(this.completed) },
      ]
      await settingsSaveSettings(changes)
    },
  },
})

/**
 * 轮询等待目标元素挂载，超时降级（继续推进，气泡将居中显示）
 */
function waitTargetEl(key: string): Promise<void> {
  return new Promise((resolve) => {
    const start = Date.now()
    const tick = () => {
      const el = targetResolvers.get(key)?.()
      if (el) {
        resolve()
        return
      }
      if (Date.now() - start > TARGET_WAIT_TIMEOUT) {
        resolve()
        return
      }
      setTimeout(tick, TARGET_WAIT_INTERVAL)
    }
    tick()
  })
}
