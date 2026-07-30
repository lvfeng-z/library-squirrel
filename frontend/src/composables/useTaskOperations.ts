import { TaskOperationCodeEnum } from '@renderer/constants/TaskOperationCodeEnum.ts'
import { TaskStatusEnum } from '@renderer/constants/TaskStatusEnum.ts'
import { taskApi } from '@renderer/apis/http'
import { ElMessage } from 'element-plus'
import { TaskProgressTreeDTO } from '@bindings/github.com//lvfeng-z/library-squirrel-sdk/dto'

/**
 * 任务操作组合式函数：封装任务控制栏通用的操作分发逻辑。
 * 后端控制接口已批量化（taskIds []），单行操作传 [id]、批量传多 id。「查看」与「删除后副作用」由调用方注入。
 * 「运行」按钮融合开始/继续：按任务状态分流（Paused/Pausing→恢复保留进度，其余→启动/终态重跑）。
 */
export function useTaskOperations() {
  // 行任务 id
  function getRowTaskId(row: TaskProgressTreeDTO): number {
    return Number(row?.taskProgress?.task?.id ?? 0)
  }
  // 重试任务（保留各任务已记录的执行模式）
  async function retryTasks(rows: TaskProgressTreeDTO[]): Promise<void> {
    const ids = rows.map(getRowTaskId)
    if (ids.length === 0) return
    try {
      await taskApi.taskRetryTrees(ids)
    } catch (e: any) {
      ElMessage.error(`重试任务失败：${e.message}`)
    }
  }
  // 启动或恢复任务（按状态分流）：Paused/Pausing→恢复（保留进度），其余→启动（Created 启动/终态重跑）
  async function startOrResumeTasks(rows: TaskProgressTreeDTO[]): Promise<void> {
    const paused: number[] = []
    const others: number[] = []
    for (const row of rows) {
      const status = row.taskProgress?.task?.status
      if (status === TaskStatusEnum.PAUSED || status === TaskStatusEnum.PAUSING) {
        paused.push(getRowTaskId(row))
      } else {
        others.push(getRowTaskId(row))
      }
    }
    try {
      if (paused.length > 0) await taskApi.taskResumeTrees(paused)
      if (others.length > 0) await taskApi.taskStartTrees(others)
    } catch (e: any) {
      ElMessage.error(`启动/恢复任务失败：${e.message}`)
    }
  }
  // 终态板块单独重执行（单行 V1 专属，保留 row 入参）
  async function redownloadSections(row: TaskProgressTreeDTO, storeRoles: string[], includeWorkInfo: boolean): Promise<void> {
    try {
      await taskApi.taskRedownload([getRowTaskId(row)], storeRoles, includeWorkInfo)
    } catch (e: any) {
      ElMessage.error(`重新下载板块失败：${e.message}`)
    }
  }
  // 批量删除任务
  async function deleteTasks(rows: TaskProgressTreeDTO[]): Promise<void> {
    const ids = rows.map(getRowTaskId)
    if (ids.length === 0) return
    try {
      await taskApi.taskDelete(ids)
    } catch (e: any) {
      ElMessage.error(e.message)
    }
  }
  /**
   * 构造操作栏点击分发函数（单行，V1 用）。
   * onView：查看；onDeleted：删除后副作用。
   * START 走 startOrResume（V1 对 Created 发 START→启动；Paused 由 V1 直接发 RESUME 走恢复分支）。
   */
  function buildOperationHandler(opts: {
    onView: (row: TaskProgressTreeDTO) => void
    onDeleted?: (row: TaskProgressTreeDTO) => void
  }) {
    return async function handleOperation(
      row: TaskProgressTreeDTO,
      code: TaskOperationCodeEnum,
      storeRoles?: string[],
      includeWorkInfo?: boolean
    ): Promise<void> {
      switch (code) {
        case TaskOperationCodeEnum.VIEW:
          opts.onView(row)
          break
        case TaskOperationCodeEnum.START:
          await startOrResumeTasks([row])
          break
        case TaskOperationCodeEnum.PAUSE:
          await taskApi.taskPauseTrees([getRowTaskId(row)])
          break
        case TaskOperationCodeEnum.RESUME:
          await taskApi.taskResumeTrees([getRowTaskId(row)])
          break
        case TaskOperationCodeEnum.RETRY:
          await retryTasks([row])
          break
        case TaskOperationCodeEnum.REDOWNLOAD:
          await redownloadSections(row, storeRoles ?? [], includeWorkInfo ?? false)
          break
        case TaskOperationCodeEnum.CANCEL:
          await taskApi.taskStopTrees([getRowTaskId(row)])
          break
        case TaskOperationCodeEnum.DELETE:
          await deleteTasks([row])
          opts.onDeleted?.(row)
          break
        default:
          break
      }
    }
  }
  /**
   * 构造批量操作分发函数（标题栏 TaskControlBar 用）。
   * 「运行」融合开始/继续（START→startOrResume 按状态分流）；按钮组仅运行/暂停/删除。
   */
  function buildBatchHandler(opts: { onDone?: () => void }) {
    return async function handleBatch(
      rows: TaskProgressTreeDTO[],
      code: TaskOperationCodeEnum
    ): Promise<void> {
      switch (code) {
        case TaskOperationCodeEnum.START:
          await startOrResumeTasks(rows)
          break
        case TaskOperationCodeEnum.PAUSE:
          await taskApi.taskPauseTrees(rows.map(getRowTaskId))
          break
        case TaskOperationCodeEnum.DELETE:
          await deleteTasks(rows)
          break
        default:
          break
      }
      opts.onDone?.()
    }
  }

  return { getRowTaskId, retryTasks, startOrResumeTasks, redownloadSections, deleteTasks, buildOperationHandler, buildBatchHandler }
}
