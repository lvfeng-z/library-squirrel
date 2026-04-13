/**
 * HTTP API 模块统一导出
 * 渲染进程通过此模块与 Go 后端通信
 */

// 核心组件
export { httpClient } from './client'
export { GO_BACKEND_URL } from './types'
export type { ApiResponse, HttpMethod, RequestConfig, IpcRouteMapping } from './types'
export { apiProxy } from './proxy'

// 模块化 API 包装器
export * as localTagApi from './wrappers/localTag'
export * as localAuthorApi from './wrappers/localAuthor'
export * as siteTagApi from './wrappers/siteTag'
export * as siteAuthorApi from './wrappers/siteAuthor'
export * as siteApi from './wrappers/site'
export * as workApi from './wrappers/work'
export * as workSetApi from './wrappers/workSet'
export * as reWorkWorkSetApi from './wrappers/reWorkWorkSet'
export * as reWorkTagApi from './wrappers/reWorkTag'
export * as searchApi from './wrappers/search'
export * as taskApi from './wrappers/task'
export * as pluginApi from './wrappers/plugin'
export * as settingsApi from './wrappers/settings'
export * as fileSysUtilApi from './wrappers/fileSysUtil'
