import { defineStore } from 'pinia'
import { v4 } from 'uuid'
import { isNullish, notNullish } from '@renderer/utils/CommonUtil.ts'
import { type NotificationLevel } from '@renderer/model/util/NotificationItem.ts'

/** 提醒输入（生产者调用 announce 时给出） */
export interface ReminderInput {
  /** 严重度，映射状态 tone 着色 */
  level: NotificationLevel
  /** 卡片标题，兼做分组头（如「任务完成」） */
  title: string
  /** 单条详情一行（如「任务【A】完成」） */
  message: string
  /** 业务分类（'task'/'merge'…），参与分组键 */
  category?: string
  /** 自动关闭毫秒数，缺省按 level（error 更长） */
  duration?: number
}

/** 聚合后的提醒卡片（渲染模型） */
export interface ReminderCard {
  id: string
  level: NotificationLevel
  title: string
  /** 合并进本卡的详情条目（1..N） */
  items: string[]
  /** 自动关闭毫秒数 */
  duration: number
}

/** 聚合窗口（毫秒）：窗口内多条按（category+level+title）合并为一张卡片。
 *  一次快照触发的多条 announce 在同一同步循环内入队，短窗口即可完整聚合；
 *  窗口越长合并率越高但提醒越迟，300ms 为即时性与跨快照合并的平衡点 */
const WINDOW_MS = 300
/** 同屏最多展示卡片数，超出排队随关闭补位 */
const MAX_VISIBLE = 3
/** 默认自动关闭时长 */
const DEFAULT_DURATION_MS = 4500
/** error 级自动关闭时长（更长以供阅读） */
const ERROR_DURATION_MS = 8000

/** 聚合窗口内的待合并缓冲（窗口到期才产出卡片，无需响应式） */
const pendingBuffer: ReminderInput[] = []
/** 聚合窗口定时器 */
let windowTimer: ReturnType<typeof setTimeout> | null = null
/** 卡片自动关闭定时器（id → timer） */
const closeTimers = new Map<string, ReturnType<typeof setTimeout>>()

function defaultDuration(level: NotificationLevel): number {
  return level === 'error' ? ERROR_DURATION_MS : DEFAULT_DURATION_MS
}

export const useReminderStore = defineStore('reminder', {
  state: (): { reminderList: ReminderCard[] } => {
    return { reminderList: [] }
  },
  actions: {
    /**
     * 推送一条提醒。进入聚合窗口，窗口到期按（category+level+title）合并为卡片展示。
     */
    announce(input: ReminderInput): void {
      pendingBuffer.push(input)
      if (isNullish(windowTimer)) {
        windowTimer = setTimeout(() => this.flush(), WINDOW_MS)
      }
    },
    /** 聚合窗口到期：分组产出卡片并展示 */
    flush(): void {
      windowTimer = null
      if (pendingBuffer.length === 0) {
        return
      }
      const batch = pendingBuffer.splice(0, pendingBuffer.length)
      const groups = new Map<string, ReminderCard>()
      for (const input of batch) {
        const key = `${input.category ?? ''}|${input.level}|${input.title}`
        const existing = groups.get(key)
        if (isNullish(existing)) {
          groups.set(key, {
            id: v4(),
            level: input.level,
            title: input.title,
            items: [input.message],
            duration: input.duration ?? defaultDuration(input.level)
          })
        } else {
          existing.items.push(input.message)
          existing.duration = Math.max(existing.duration, input.duration ?? defaultDuration(input.level))
        }
      }
      for (const card of groups.values()) {
        const timer = setTimeout(() => this.dismiss(card.id), card.duration)
        closeTimers.set(card.id, timer)
        this.reminderList.push(card)
      }
    },
    /** 立即关闭一张卡片并清理其定时器 */
    dismiss(id: string): void {
      const timer = closeTimers.get(id)
      if (notNullish(timer)) {
        clearTimeout(timer)
        closeTimers.delete(id)
      }
      const index = this.reminderList.findIndex((card) => card.id === id)
      if (index !== -1) {
        this.reminderList.splice(index, 1)
      }
    }
  },
  getters: {
    /** 同屏可见卡片（最多 MAX_VISIBLE 张，其余排队） */
    visibleCards: (state): ReminderCard[] => state.reminderList.slice(0, MAX_VISIBLE),
    /** 排队等待展示的卡片数 */
    overflowCount: (state): number => Math.max(0, state.reminderList.length - MAX_VISIBLE)
  }
})
