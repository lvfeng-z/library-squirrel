/**
 * HTTP API 模块统一导出
 * 渲染进程通过此模块与 Go 后端通信
 */

// 核心组件
export { httpClient } from './client'
export { GO_BACKEND_URL } from './types'
export type { ApiResponse, HttpMethod, RequestConfig, IpcRouteMapping } from './types'
export { apiProxy } from './proxy'

// 模块化 API 包装器（从 Wails 封装导出）
export * as localTagApi from '../wails/localTag'
export * as localAuthorApi from '../wails/localAuthor'
export * as siteTagApi from '../wails/siteTag'
export * as siteAuthorApi from '../wails/siteAuthor'
export * as siteApi from '../wails/site'
export * as workApi from '../wails/work'
export * as workSetApi from '../wails/workSet'
export * as reWorkWorkSetApi from '../wails/workSet'
export * as reWorkTagApi from '../wails/reWorkTag'
export * as searchApi from '../wails/search'
export * as taskApi from '../wails/task'
export * as pluginApi from '../wails/plugin'
export * as settingsApi from '../wails/settings'
export * as fileSysUtilApi from '../wails/common'
export * as appLauncherApi from '../wails/appLauncher'
export * as siteBrowserApi from '../wails/siteBrowser'
