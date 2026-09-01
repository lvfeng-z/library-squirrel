import { ref } from 'vue'
import { TaskOperationCodeEnum } from '@renderer/constants/TaskOperationCodeEnum.ts'
import { TaskStatusEnum } from '@renderer/constants/TaskStatusEnum.ts'
import { taskApi } from '@renderer/apis/http'
import { ElMessage } from 'element-plus'
import { TaskProgressTreeDTO } from '@bindings/github.com/library-squirrel/backend/base/model/dto'

// ===== 操作防重入守卫（模块级单例，跨组件共享）=====
// 任务树 ID → 处于「上一次操作未返回（IPC 在途）或冷却期」的集合：期间该树操作按钮不可再点，
// 点击被静默忽略。目的：压制高频启停——启停风暴反复重拨会烧尽收件拨号配额
// （DialCoordinator 速率门控，阻塞至窗口滑出表现为进度 30-60s 不动）。
const inFlightTreeIds = ref(new Set<number>())
// 冷却时长（毫秒）：后端 config.yaml 的 task.operationCooldownMs，懒加载一次；
// 0=不启用（开发者调试放开）。缺省 1500
const DEFAULT_OP_COOLDOWN_MS = 1500
let cooldownMsPromise: Promise<number> | undefined
function getOperationCooldownMs(): Promise<number> {
  if (!cooldownMsPromise) {
    cooldownMsPromise = taskApi
      .taskGetTaskControlConfig()
      .then((res) => res.data?.operationCooldownMs ?? DEFAULT_OP_COOLDOWN_MS)
      .catch(() => DEFAULT_OP_COOLDOWN_MS)
  }
  return cooldownMsPromise
}

// 行按钮在途态（操作栏消费：按钮 loading/禁用，反馈「禁止再次操作」）
export function isOpInFlight(taskId: number): boolean {
  return inFlightTreeIds.value.has(taskId)
}

// 单树守卫执行：在途或冷却期内忽略本次操作并返回 false；否则加入集合执行，
// 冷却期后再放行（返回 true）。冷却期 = 操作返回后仍保留禁止窗口
async function runGuarded(taskId: number, op: () => Promise<unknown>): Promise<boolean> {
  if (inFlightTreeIds.value.has(taskId)) return false
  const cooldownMs = await getOperationCooldownMs()
  if (inFlightTreeIds.value.has(taskId)) return false // 取配置期间被并发操作抢占
  inFlightTreeIds.value.add(taskId)
  try {
    await op()
    return true
  } finally {
    if (cooldownMs > 0) {
      setTimeout(() => inFlightTreeIds.value.delete(taskId), cooldownMs)
    } else {
      inFlightTreeIds.value.delete(taskId)
    }
  }
}

// 批量守卫执行：任一棵在途/冷却则整批忽略；执行期整批在途，冷却期后逐个放行
async function runGuardedBatch(taskIds: number[], op: () => Promise<unknown>): Promise<boolean> {
  if (taskIds.length === 0) return false
  const blocked = taskIds.some((id) => inFlightTreeIds.value.has(id))
  if (blocked) return false
  const cooldownMs = await getOperationCooldownMs()
  if (taskIds.some((id) => inFlightTreeIds.value.has(id))) return false
  taskIds.forEach((id) => inFlightTreeIds.value.add(id))
  try {
    await op()
    return true
  } finally {
    taskIds.forEach((id) => {
      if (cooldownMs > 0) {
        setTimeout(() => inFlightTreeIds.value.delete(id), cooldownMs)
      } else {
        inFlightTreeIds.value.delete(id)
      }
    })
  }
}

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
   * onView：查看；onDeleted：删除后副作用（仅删除真正执行后触发）。
   * START 走 startOrResume（V1 对 Created 发 START→启动；Paused 由 V1 直接发 RESUME 走恢复分支）。
   * 控制类操作（开始/暂停/恢复/重试/重下/取消/删除）统一经 runGuarded：在途或冷却期内禁止再次操作。
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
      const taskId = getRowTaskId(row)
      switch (code) {
        case TaskOperationCodeEnum.VIEW:
          opts.onView(row)
          break
        case TaskOperationCodeEnum.START:
          await runGuarded(taskId, () => startOrResumeTasks([row]))
          break
        case TaskOperationCodeEnum.PAUSE:
          await runGuarded(taskId, () => taskApi.taskPauseTrees([taskId]))
          break
        case TaskOperationCodeEnum.RESUME:
          await runGuarded(taskId, () => taskApi.taskResumeTrees([taskId]))
          break
        case TaskOperationCodeEnum.RETRY:
          await runGuarded(taskId, () => retryTasks([row]))
          break
        case TaskOperationCodeEnum.REDOWNLOAD:
          await runGuarded(taskId, () => redownloadSections(row, storeRoles ?? [], includeWorkInfo ?? false))
          break
        case TaskOperationCodeEnum.CANCEL:
          await runGuarded(taskId, () => taskApi.taskStopTrees([taskId]))
          break
        case TaskOperationCodeEnum.DELETE:
          if (await runGuarded(taskId, () => deleteTasks([row]))) {
            opts.onDeleted?.(row)
          }
          break
        default:
          break
      }
    }
  }
  /**
   * 构造批量操作分发函数（标题栏 TaskControlBar 用）。
   * 「运行」融合开始/继续（START→startOrResume 按状态分流）；按钮组仅运行/暂停/删除。
   * 批量操作经 runGuardedBatch：任一棵在途/冷却则整批忽略。
   */
  function buildBatchHandler(opts: { onDone?: () => void }) {
    return async function handleBatch(
      rows: TaskProgressTreeDTO[],
      code: TaskOperationCodeEnum
    ): Promise<void> {
      const ids = rows.map(getRowTaskId)
      switch (code) {
        case TaskOperationCodeEnum.START:
          await runGuardedBatch(ids, () => startOrResumeTasks(rows))
          break
        case TaskOperationCodeEnum.PAUSE:
          await runGuardedBatch(ids, () => taskApi.taskPauseTrees(ids))
          break
        case TaskOperationCodeEnum.DELETE:
          await runGuardedBatch(ids, () => deleteTasks(rows))
          break
        default:
          break
      }
      opts.onDone?.()
    }
  }

  return { getRowTaskId, retryTasks, startOrResumeTasks, redownloadSections, deleteTasks, buildOperationHandler, buildBatchHandler }
}
