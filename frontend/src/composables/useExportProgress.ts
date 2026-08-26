import { reactive } from 'vue'

// 导出进度状态（按 exportId 索引），由后端 export-events 事件驱动。
// 单个导出对应一个弹窗实例；弹窗关闭/完成时清除条目。
export interface ExportProgressState {
  exportId: string
  totalFiles: number
  processedFiles: number
  totalBytes: number
  processedBytes: number
  // 进度百分比：0~100；totalFiles>0 时按文件数算，否则按字节算（决策4 缺失文件不占总文件数）
  percent: number
  // running=进行中 / done=成功 / failed=失败（含取消，errMsg 区分）
  status: 'running' | 'done' | 'failed'
  // 成功时最终 zip 绝对路径
  targetPath?: string
  // 失败时的用户可读信息（success=true 时为 undefined）；取消时为「已取消」
  errMsg?: string
}

const exportStates = reactive(new Map<string, ExportProgressState>())

// onExportEvent 处理一条 export-events 事件（{type,data} 由 MainIpcListener 解出后传入）。
// complete 视为权威终态：忽略其后到达的迟到 progress（防 Wails emit 乱序导致的"已完成→倒退"闪烁）。
export function onExportEvent(type: string, data: any) {
  if (type === 'progress') {
    const id = data.exportId as string
    const existing = exportStates.get(id)
    if (existing && existing.status !== 'running') {
      return // 已终态，忽略迟到 progress（乱序防护）
    }
    const state: ExportProgressState = {
      exportId: id,
      totalFiles: data.totalFiles as number,
      processedFiles: data.processedFiles as number,
      totalBytes: data.totalBytes as number,
      processedBytes: data.processedBytes as number,
      percent: computePercent(data.totalFiles as number, data.processedFiles as number, data.totalBytes as number, data.processedBytes as number),
      status: 'running'
    }
    exportStates.set(id, existing ? { ...existing, ...state } : state)
  } else if (type === 'complete') {
    const id = data.exportId as string
    const existing = exportStates.get(id)
    const success = data.success as boolean
    exportStates.set(id, {
      exportId: id,
      totalFiles: existing?.totalFiles ?? 0,
      processedFiles: existing?.processedFiles ?? 0,
      totalBytes: existing?.totalBytes ?? 0,
      processedBytes: existing?.processedBytes ?? 0,
      percent: 100,
      status: success ? 'done' : 'failed',
      targetPath: data.targetPath as string | undefined,
      errMsg: success ? undefined : (data.errMsg as string)
    })
  }
}

// markExportStarted 在前端发起导出时立即标记 running（等首个 progress 事件，按钮即时反馈）。
// 若启动 IPC 失败（空选择/磁盘预检等前置错误），调用方须 clearExportState 回退。
// 终态竞态防护：导出极快时 complete 事件可能先于 StartExport IPC 返回到达，此时保留终态不覆盖。
export function markExportStarted(exportId: string) {
  const existing = exportStates.get(exportId)
  if (existing && existing.status !== 'running') {
    return
  }
  exportStates.set(exportId, {
    exportId,
    totalFiles: 0,
    processedFiles: 0,
    totalBytes: 0,
    processedBytes: 0,
    percent: 0,
    status: 'running'
  })
}

// clearExportState 清除条目（导出启动失败需回退 running 标记或弹窗关闭时调用）。
export function clearExportState(exportId: string) {
  exportStates.delete(exportId)
}

// getExportState 取某导出的进度状态（无则 undefined=未在导出）。
export function getExportState(exportId: string | undefined): ExportProgressState | undefined {
  if (!exportId) return undefined
  return exportStates.get(exportId)
}

// computePercent 进度百分比：优先按文件数（缺失文件不占总文件数，进度真实反映写入工作量），
// 文件数为 0 时回退按字节。
function computePercent(totalFiles: number, processedFiles: number, totalBytes: number, processedBytes: number): number {
  if (totalFiles > 0) {
    return Math.floor((processedFiles / totalFiles) * 100)
  }
  if (totalBytes > 0) {
    return Math.floor((processedBytes / totalBytes) * 100)
  }
  return 0
}
