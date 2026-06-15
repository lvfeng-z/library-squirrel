/**
 * 向导步的目标定位（路由 + 可选元素 key）
 */
export interface TourStepTarget {
  /**
   * 目标路由 name（与 SlotRegistry 的 viewId 对齐，如 'settings'、'taskManage'）
   */
  route: string

  /**
   * 该页面内可被高亮的元素 key（页面通过 useTourTargets 注册）；
   * 不填则气泡居中显示，不依附具体元素
   */
  targetKey?: string
}

/**
 * 向导步需要的目标数据（强类型，按业务扩展）
 */
export type TourStepData =
  | { kind: 'work'; workId: number }
  | { kind: 'tag'; tagId: number; scope?: 'local' | 'site' }
  | { kind: 'author'; authorId: number; scope?: 'local' | 'site' }
  | { kind: 'task'; taskId: number }
  | { kind: 'none' }

/**
 * 气泡相对目标元素的位置
 */
export type TourStepPlacement =
  | 'top'
  | 'top-start'
  | 'top-end'
  | 'bottom'
  | 'bottom-start'
  | 'bottom-end'
  | 'left'
  | 'right'
  | 'center'

/**
 * 单步向导
 */
export interface TourStep {
  /** 目标定位 */
  target: TourStepTarget

  /** 标题 */
  title?: string

  /** 描述 */
  description: string

  /** 气泡位置 */
  placement?: TourStepPlacement

  /**
   * 该步需要的目标数据（引擎写入 context.data，目标页面读取并据此定位）
   */
  data?: TourStepData

  /**
   * 进入该步前、在目标页面侧执行的钩子（如打开某个对话框）
   */
  onEnterPage?: (ctx: TourContext) => void | Promise<void>
}

/**
 * 向导定义
 */
export interface TourDefinition {
  /** 向导唯一标识 */
  id: string

  /** 名称 */
  name: string

  /** 描述 */
  description: string

  /** 步骤列表（按顺序执行） */
  steps: TourStep[]
}

/**
 * 运行期上下文（控制中心 → 目标页面的数据载体）
 */
export interface TourContext {
  /** 当前向导 ID */
  tourId: string

  /** 当前步骤索引 */
  stepIndex: number

  /** 该步的目标数据 */
  data?: TourStepData

  /** 调用方自定义载荷 */
  payload?: Record<string, unknown>
}

/**
 * 向导运行状态
 */
export type TourStatus = 'idle' | 'running' | 'finished'
