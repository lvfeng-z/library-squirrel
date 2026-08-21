/**
 * BackupGovernance HTTP API 包装器（备份管理面板数据面）
 * 封装 Wails 绑定层响应校验，校验失败时抛出异常，调用方通过 try/catch 捕获
 */

import {
	Handler as BackupGovernanceHandler
} from '@bindings/github.com/library-squirrel/backend/backupGovernance'
import { BackupDTO, BackupStatsDTO, ReconciliationResult } from '@bindings/github.com/library-squirrel/backend/backupGovernance/models'
import { Page } from '@bindings/github.com/library-squirrel/backend/base/model/models'
import type { ApiResult } from '@renderer/apis/http/types'
import { requireResponse } from '@renderer/apis/http/types'

// ========== API 方法 ==========

/**
 * 分页查询备份保管清单（保管时间倒序）
 * referenced: 引用态过滤（null=全部 / true=有主 / false=无主）
 */
export async function backupGovernancePageBackups(page: Page<BackupDTO>, referenced: boolean | null): Promise<ApiResult<Page<BackupDTO>>> {
  return requireResponse(await BackupGovernanceHandler.PageBackups(page, referenced), '查询备份列表')
}

/**
 * 批量删除备份（磁盘文件与清单行）。任一 id 被业务行引用即整体拒绝；
 * 「清理全部无主」的批量圈定取 backupGovernanceGetBackupStats 的 expiredOrphanIds（超保留期）
 */
export async function backupGovernanceDeleteBackups(ids: number[]): Promise<ApiResult<any>> {
  return requireResponse(await BackupGovernanceHandler.DeleteBackups(ids), '删除备份', false)
}

/**
 * 手动触发一轮双向对账（与定时巡检互斥），返回清理统计
 */
export async function backupGovernanceRunReconciliationNow(): Promise<ApiResult<ReconciliationResult>> {
  return requireResponse(await BackupGovernanceHandler.RunReconciliationNow(), '立即巡检', false)
}

/**
 * 备份占用统计：总占用 / 有主·无主拆分 / 按引用方分组 / 无主超期圈定（服务端短 TTL 缓存）
 */
export async function backupGovernanceGetBackupStats(): Promise<ApiResult<BackupStatsDTO>> {
  return requireResponse(await BackupGovernanceHandler.GetBackupStats(), '查询备份统计')
}
