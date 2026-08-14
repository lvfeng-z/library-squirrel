import { VNode } from 'vue'

/** 通知严重度，驱动颜色/图标（映射状态 tone：info→active、success→done、warning→warn、error→fail） */
export type NotificationLevel = 'info' | 'success' | 'warning' | 'error'

/** 动态进度；percent 可显式给出（无 total 的 indeterminate 进度），否则由 current/total 派生 */
export interface NotificationProgress {
  current?: number
  total?: number
  percent?: number
}

/** 跳转目标（vue-router location，name 如 'taskManage'） */
export interface NotificationRoute {
  name: string
  params?: Record<string, unknown>
  query?: Record<string, unknown>
}

/** 通知条目（完整形态，含 store 分配的 id/createTime） */
export interface NotificationItem {
  /** store 分配的唯一标识，外部持有以做后续 update/remove */
  id: string
  /** 严重度 */
  level: NotificationLevel
  /** 标题 */
  title: string
  /** 业务分类（'task'/'merge'/'fsmonitor'…），供未来过滤 */
  category?: string
  /** 状态描述（"下载中"/"完成"/"失败"） */
  statusText?: string
  /** 动态进度 */
  progress?: NotificationProgress
  /** 异常/错误描述（失败时展示） */
  exception?: string
  /** 跳转目标，存在则点击通知跳转 */
  route?: NotificationRoute
  /** 兜底自绘，存在则覆盖默认渲染 */
  render?: (() => VNode)
  /** 是否终态（默认 false）；置 true 后脱离任务 store 生命周期，角标按此过滤活跃条目 */
  terminal?: boolean
  /** 创建时间戳（ms），store 分配，供排序/展示 */
  createTime: number
}

/** 新建通知的输入形态（不含 store 分配的 id/createTime） */
export type NewNotificationItem = Omit<NotificationItem, 'id' | 'createTime'>
