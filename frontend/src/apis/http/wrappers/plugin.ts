/**
 * Plugin HTTP API 包装器
 * 封装 Wails 绑定层响应校验，校验失败时抛出异常，调用方通过 try/catch 捕获
 */

import {
  Handler as PluginHandler,
  PluginQueryDTO
} from '@bindings/github.com/library-squirrel/backend/plugin'
import { PluginDTO } from '@bindings/github.com/lvfeng-z/library-squirrel-sdk/dto'
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

/** 从路径安装插件 */
export async function pluginInstallFromPath(packagePath: string, installType?: number): Promise<ApiResult<PluginDTO>> {
  return requireResponse(await PluginHandler.InstallFromPath(packagePath, installType ?? 0), '安装插件')
}

/** 重新安装插件 */
export async function pluginReinstall(publicId: string, installType?: number): Promise<ApiResult<PluginDTO>> {
  return requireResponse(await PluginHandler.Reinstall(publicId, installType ?? 0), '重新安装插件')
}

/** 从路径重新安装插件 */
export async function pluginReinstallFromPath(pluginPublicId: string, packagePath: string, installType?: number): Promise<ApiResult<PluginDTO>> {
  return requireResponse(await PluginHandler.ReinstallFromPath(pluginPublicId, packagePath, installType ?? 0), '重新安装插件')
}

/** 卸载插件 */
export async function pluginUnInstall(publicId: string): Promise<ApiResult<any>> {
  return requireResponse(await PluginHandler.Uninstall(publicId), '卸载插件', false)
}

/** 获取插件状态 */
export async function pluginGetStatus(publicId: string): Promise<ApiResult<PluginStatusDTO>> {
  return requireResponse(await PluginHandler.GetPluginStatus(publicId), '获取插件状态')
}
