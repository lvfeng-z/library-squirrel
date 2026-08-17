/**
 * Plugin HTTP API 包装器
 * 封装 Wails 绑定层响应校验，校验失败时抛出异常，调用方通过 try/catch 捕获
 */

import {
  Handler as PluginHandler,
  PluginQueryDTO
} from '@bindings/github.com/library-squirrel/backend/plugin'
import { PluginDTO, PendingUpgradeDTO } from '@bindings/github.com/library-squirrel/backend/base/model/dto'
import { PluginStatusDTO } from '@bindings/github.com/library-squirrel/backend/plugin/models'
import { Page } from '@bindings/github.com/library-squirrel/backend/base/model'
import type { ApiResult } from '@renderer/apis/http/types'
import { requireResponse } from '@renderer/apis/http/types'

// ========== API 方法 ==========

/** 保存插件 */
export async function pluginSave(plugin: PluginDTO): Promise<ApiResult<number>> {
  return requireResponse(await PluginHandler.Save(plugin), '保存插件', false)
}

/** 更新插件 */
export async function pluginUpdate(plugin: PluginDTO): Promise<ApiResult<any>> {
  return requireResponse(await PluginHandler.Update(plugin), '更新插件', false)
}

/** 删除插件（设置为已卸载状态） */
export async function pluginDelete(id: number): Promise<ApiResult<any>> {
  return requireResponse(await PluginHandler.SetUninstalled(id), '删除插件', false)
}

/** 根据ID获取插件 */
export async function pluginGetById(id: number): Promise<ApiResult<PluginDTO>> {
  return requireResponse(await PluginHandler.GetById(id), '获取插件')
}

/** 根据公开ID获取插件 */
export async function pluginGetByPublicId(publicId: string): Promise<ApiResult<PluginDTO>> {
  return requireResponse(await PluginHandler.GetByPublicId(publicId), '获取插件')
}

/** 分页查询插件 */
export async function pluginQueryPage(page: Page<PluginDTO>, query: PluginQueryDTO): Promise<ApiResult<Page<PluginDTO>>> {
  return requireResponse(await PluginHandler.Page(page, query), '查询插件')
}

/** 检查插件是否已安装 */
export async function pluginCheckInstalled(publicId: string): Promise<ApiResult<boolean>> {
  return requireResponse(await PluginHandler.CheckInstalled(publicId), '检查插件安装状态')
}

/** 从路径安装插件。trusted 透传用户知情同意结果：仅在安装确认弹窗中用户点击确认后传 true；缺省/绕过弹窗传 false（插件标为未信任，需手动信任后才运行） */
export async function pluginInstallFromPath(packagePath: string, trusted: boolean = false): Promise<ApiResult<PluginDTO>> {
  return requireResponse(await PluginHandler.InstallFromPath(packagePath, trusted), '安装插件')
}

/** 重新安装插件。trusted 透传用户知情同意结果（bundled 插件后端强制信任，此参数仅影响第三方） */
export async function pluginReinstall(publicId: string, trusted: boolean = false): Promise<ApiResult<PluginDTO>> {
  return requireResponse(await PluginHandler.Reinstall(publicId, trusted), '重新安装插件')
}

/** 从路径重新安装插件。trusted 透传用户知情同意结果（bundled 插件后端强制信任，此参数仅影响第三方） */
export async function pluginReinstallFromPath(pluginPublicId: string, packagePath: string, trusted: boolean = false): Promise<ApiResult<PluginDTO>> {
  return requireResponse(await PluginHandler.ReinstallFromPath(pluginPublicId, packagePath, trusted), '重新安装插件')
}

/** 卸载插件 */
export async function pluginUnInstall(publicId: string): Promise<ApiResult<any>> {
  return requireResponse(await PluginHandler.Uninstall(publicId), '卸载插件', false)
}

/**
 * 设置插件信任状态（手动信任/取消信任）。trusted=true 时后端激活插件；
 * trusted=false 即时停用运行时，force=确认对话框明示代价后强制停（跳过参与者否决检查）
 */
export async function pluginSetTrusted(publicId: string, trusted: boolean, force: boolean = false): Promise<ApiResult<PluginDTO>> {
  return requireResponse(await PluginHandler.SetTrusted(publicId, trusted, force), '设置插件信任状态')
}

/** 获取插件状态 */
export async function pluginGetStatus(publicId: string): Promise<ApiResult<PluginStatusDTO>> {
  return requireResponse(await PluginHandler.GetPluginStatus(publicId), '获取插件状态')
}

// ========== 检查更新流 ==========

/** 获取插件更新待办（available 可答复计入红点；forced/error 只读告知） */
export async function pluginGetPendingUpgrades(): Promise<ApiResult<PendingUpgradeDTO[]>> {
  return requireResponse(await PluginHandler.GetPendingUpgrades(), '获取插件更新待办')
}

/** 答复「升级」：对 available 待办执行运行期换版（当次会话生效；运行中任务被后端参与者否决并报错引导） */
export async function pluginApplyPendingUpgrade(publicId: string): Promise<ApiResult<PluginDTO>> {
  return requireResponse(await PluginHandler.ApplyPendingUpgrade(publicId), '升级插件')
}

/** 答复「跳过此构建」：持久化拒绝标记，下次启动对等值 buildId 静默跳过 */
export async function pluginDeclinePendingUpgrade(publicId: string): Promise<ApiResult<PluginDTO>> {
  return requireResponse(await PluginHandler.DeclinePendingUpgrade(publicId), '跳过插件更新')
}

/** 「重新提示」：清除拒绝标记并立即重跑检测重建待办（反悔入口） */
export async function pluginRestorePendingUpgrade(publicId: string): Promise<ApiResult<PluginDTO>> {
  return requireResponse(await PluginHandler.RestorePendingUpgrade(publicId), '恢复插件更新提示')
}
