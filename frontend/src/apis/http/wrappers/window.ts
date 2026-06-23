/**
 * Window HTTP API 包装器
 * 直接调用 bindings 接口
 */
import type { ApiResponse } from '../types'
import { Handler as WindowHandler } from '@bindings/github.com/library-squirrel/backend/window'

/**
 * 设置主窗口标题栏背景色与文字色
 * bg/text 为 #RRGGBB 格式，仅 Windows 11 (22000+) 完整生效，其它平台静默失败
 */
export async function windowSetTitleBarColor(bg: string, text: string): Promise<ApiResponse<boolean>> {
  const result = await WindowHandler.SetTitleBarColor(bg, text)
  if (!result) {
    return { success: false, msg: '设置标题栏颜色失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '', data: result.success }
}
