/**
 * Common Wails 绑定包装器
 * 通用方法：DirSelect, OpenPath, OpenExternal
 */

import {Handler as FileSysUtilHandler} from '@bindings/github.com/library-squirrel/wails/internal/fileSysUtil'
import {Handler as AppLauncherHandler} from '@bindings/github.com/library-squirrel/wails/internal/appLauncher'
import type {ApiResponse} from '@apis/http'
import type {OpenDialogResult} from '@bindings/github.com/library-squirrel/wails/internal/fileSysUtil/models'

/**
 * 打开目录/文件选择对话框
 * @param openFile true=选择文件, false=选择目录
 */
export async function dirSelect(openFile: boolean): Promise<ApiResponse<OpenDialogResult | null> | null> {
  return FileSysUtilHandler.DirSelect(openFile, false);
}

/**
 * 使用系统默认应用打开文件
 */
export async function openPath(path: string): Promise<ApiResponse<void> | null> {
  return AppLauncherHandler.OpenPath(path)
}

/**
 * 在默认浏览器中打开 URL
 */
export async function openExternal(url: string): Promise<ApiResponse<void> | null> {
  return AppLauncherHandler.OpenExternal(url)
}
