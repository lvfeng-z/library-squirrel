import { reactive } from 'vue'

// 合并状态（按 resourceId 索引），由后端 merge-events 事件驱动。
// 阶段1 合并不进 taskManager 控制面，故独立维护状态；当前仅 WorkDialog 消费，
// 用模块级单例 reactive Map 保持最小（消费方增多再升格 Pinia store）。
export interface MergeState {
  // 合并进度百分比：-1=不定态（尚未收到进度事件），0~100
  percent: number
  // running=进行中 / done=成功 / failed=失败（含取消，errMsg 区分）
  status: 'running' | 'done' | 'failed'
  // 失败时的用户可读信息（success=true 时为 undefined）；取消时为"已取消"
  errMsg?: string
}

const mergeStates = reactive(new Map<number, MergeState>())
// 终态条目清理延时：complete 后保留一段时间供 UI 收尾（按钮复位/结果展示），再删除条目。
const TERMINAL_CLEANUP_MS = 3000

// onMergeEvent 处理一条 merge-events 事件（{type,data} 由 MainIpcListener 解出后传入）。
// complete 视为权威终态：忽略其后到达的迟到 progress（防 Wails emit 乱序导致的"已完成→倒退"闪烁）。
export function onMergeEvent(type: string, data: any) {
  if (type === 'progress') {
    const resourceId: number = data.resourceId
    const existing = mergeStates.get(resourceId)
    if (existing && existing.status !== 'running') {
      return // 已终态，忽略迟到 progress（乱序防护）
    }
    if (existing) {
      existing.percent = data.percent as number
    } else {
      mergeStates.set(resourceId, { percent: data.percent as number, status: 'running' })
    }
  } else if (type === 'complete') {
    const resourceId: number = data.resourceId
    const success: boolean = data.success
    mergeStates.set(resourceId, {
      percent: 100,
      status: success ? 'done' : 'failed',
      errMsg: success ? undefined : (data.errMsg as string)
    })
    setTimeout(() => mergeStates.delete(resourceId), TERMINAL_CLEANUP_MS)
  }
}

// markMergeStarted 在前端发起合并时立即标记 running（不等首个 progress 事件，按钮即时反馈）。
// 若启动 IPC 失败（缺轨/已合并/已在合并中），调用方须 clearMergeState 回退。
export function markMergeStarted(resourceId: number) {
  mergeStates.set(resourceId, { percent: -1, status: 'running' })
}

// clearMergeState 清除条目（合并启动失败需回退 running 标记时调用）。
export function clearMergeState(resourceId: number) {
  mergeStates.delete(resourceId)
}

// getMergeState 取某 resource 的合并状态（无则 undefined=未在合并）。
export function getMergeState(resourceId: number | undefined): MergeState | undefined {
  if (resourceId === undefined) return undefined
  return mergeStates.get(resourceId)
}
