/**
 * Task HTTP API 包装器
 * 使用 requireResponse + ApiResult 统一响应校验
 */

import { requireResponse, type ApiResult } from '../types'
import { Handler as TaskHandler } from '@bindings/github.com/library-squirrel/backend/task'
import { Handler as TaskManagerHandler } from '@bindings/github.com/library-squirrel/backend/taskManager'
import { TaskQueryDTO, CreateTaskByURLResponse } from '@bindings/github.com/library-squirrel/backend/task/models'
import { TaskDTO } from '@bindings/github.com/lvfeng-z/library-squirrel-sdk/dto/models'
import { CreateTaskRequest, TaskProgressDTO, TaskProgressTreeDTO } from '@bindings/github.com/library-squirrel/backend/base/model/dto'
import { Page } from '@bindings/github.com/library-squirrel/backend/base/model/models'

// ========== 查询操作 ==========

export async function taskGetById(id: number): Promise<ApiResult<TaskDTO>> {
  return requireResponse(await TaskHandler.GetById(id), '获取任务')
}

export async function taskQueryPage(page: Page<TaskDTO>, query: TaskQueryDTO): Promise<ApiResult<Page<TaskDTO>>> {
  return requireResponse(await TaskHandler.QueryPage(page, query), '查询任务')
}

export async function taskQueryParentPage(page: Page<TaskProgressTreeDTO>, query: TaskQueryDTO): Promise<ApiResult<Page<TaskProgressTreeDTO>>> {
  return requireResponse(await TaskHandler.QueryParentPage(page, query), '查询父任务')
}

export async function taskQueryChildrenTaskPage(
  page: Page<TaskProgressTreeDTO>,
  query: TaskQueryDTO
): Promise<ApiResult<Page<TaskProgressTreeDTO>>> {
  return requireResponse(await TaskHandler.QueryChildrenTaskPage(page, query), '查询子任务')
}

export async function taskListTree(taskIds: number[], ...includeStatus: number[]): Promise<ApiResult<TaskDTO[]>> {
  const result = requireResponse(await TaskHandler.ListTaskTree(taskIds, ...includeStatus), '获取任务树')
  const data = result.data?.filter((item): item is TaskDTO => item !== null) ?? []
  return { success: true as const, msg: result.msg, data }
}

export async function taskListStatus(ids: number[]): Promise<ApiResult<TaskProgressDTO[]>> {
  const result = requireResponse(await TaskHandler.ListStatus(ids), '查询任务状态')
  const data = result.data?.filter((item): item is TaskProgressDTO => item !== null) ?? []
  return { success: true as const, msg: result.msg, data }
}

// ========== 增删改操作 ==========

export async function taskSave(task: TaskDTO): Promise<ApiResult<number>> {
  return requireResponse(await TaskHandler.Save(task), '保存任务', false)
}

export async function taskUpdate(task: TaskDTO): Promise<ApiResult<void>> {
  return requireResponse(await TaskHandler.Update(task), '更新任务', false)
}

export async function taskDelete(taskIds: number[]): Promise<ApiResult<void>> {
  return requireResponse(await TaskHandler.DeleteTask(taskIds), '删除任务', false)
}

export async function taskRefreshStatus(id: number): Promise<ApiResult<number>> {
  return requireResponse(await TaskHandler.RefreshStatus(id), '刷新任务状态')
}

export async function taskSetTreeStatus(
  taskIds: number[],
  status: number,
  includeStatus?: number[]
): Promise<ApiResult<number>> {
  return requireResponse(await TaskHandler.SetTreeStatus(taskIds, status, includeStatus ?? []), '设置任务树状态')
}

export async function taskCreate(request: CreateTaskRequest): Promise<ApiResult<number>> {
  return requireResponse(await TaskHandler.CreateTask(request), '创建任务')
}

export async function taskCreateByUrl(url: string): Promise<ApiResult<CreateTaskByURLResponse>> {
  return requireResponse(await TaskHandler.CreateTaskByURL(url), '创建任务')
}

// ========== TaskManager 操作 ==========

export async function taskStartTrees(taskIds: number[]): Promise<ApiResult<void>> {
  return requireResponse(await TaskManagerHandler.StartTaskTrees(taskIds), '启动任务', false)
}

export async function taskPauseTrees(taskIds: number[]): Promise<ApiResult<void>> {
  return requireResponse(await TaskManagerHandler.PauseTaskTrees(taskIds), '暂停任务', false)
}

export async function taskResumeTrees(taskIds: number[]): Promise<ApiResult<void>> {
  return requireResponse(await TaskManagerHandler.ResumeTaskTrees(taskIds), '恢复任务', false)
}

export async function taskStopTrees(taskIds: number[]): Promise<ApiResult<void>> {
  return requireResponse(await TaskManagerHandler.StopTaskTrees(taskIds), '停止任务', false)
}

export async function taskRetryTrees(taskIds: number[]): Promise<ApiResult<void>> {
  return requireResponse(await TaskManagerHandler.RetryTaskTrees(taskIds), '重试任务', false)
}

// ========== 作品板块重执行（多选） ==========

// storeRoles 为所选资源 store_type 集合,includeWorkInfo 决定是否执行作品元数据板块
export async function taskRedownload(taskIds: number[], storeRoles: string[], includeWorkInfo: boolean): Promise<ApiResult<void>> {
  return requireResponse(await TaskManagerHandler.Redownload(taskIds, storeRoles, includeWorkInfo), '重新下载板块', false)
}

// ========== 作品替换确认 ==========

export async function taskManagerConfirmReplace(taskId: number, action: string): Promise<ApiResult<void>> {
  return requireResponse(await TaskManagerHandler.ConfirmReplace(taskId, action), '确认替换', false)
}

export async function taskManagerConfirmReplaceBatch(taskIds: number[], action: string): Promise<ApiResult<void>> {
  return requireResponse(await TaskManagerHandler.ConfirmReplaceBatch(taskIds, action), '批量确认替换', false)
}
