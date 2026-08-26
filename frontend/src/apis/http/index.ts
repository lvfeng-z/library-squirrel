/**
 * HTTP API 模块统一导出
 * 渲染进程通过此模块与 Go 后端通信
 * 现在直接调用 bindings 接口，不再使用 HTTP 代理
 */

// 核心类型与工具函数
export { GO_BACKEND_URL, requireResponse } from './types'
export type { ApiResponse, ApiResult } from './types'

// 模块化 API 包装器（直接从 bindings 调用）
export * as localTagApi from './wrappers/localTag'
export * as localAuthorApi from './wrappers/localAuthor'
export * as siteTagApi from './wrappers/siteTag'

// 适配器函数（直接导出，供 AutoLoadSelect 等组件使用）
export { localTagQuerySelectItemPageByName } from './wrappers/localTag'
export { localAuthorQuerySelectItemPageByName } from './wrappers/localAuthor'
export * as siteAuthorApi from './wrappers/siteAuthor'
export * as siteApi from './wrappers/site'
export { siteQuerySelectItemPageBySiteName } from './wrappers/site'
export * as workApi from './wrappers/work'
export * as resourceApi from './wrappers/resource'
export * as recycleBinApi from './wrappers/recycleBin'
export * as backupGovernanceApi from './wrappers/backupGovernance'
export * as workSetApi from './wrappers/workSet'
export * as reWorkWorkSetApi from './wrappers/reWorkWorkSet'
export * as reWorkTagApi from './wrappers/reWorkTag'
export * as searchApi from './wrappers/search'
export * as taskApi from './wrappers/task'
export * as pluginApi from './wrappers/plugin'
export * as pluginSettingApi from './wrappers/pluginSetting'
export * as settingsApi from './wrappers/settings'
export * as fileSysUtilApi from './wrappers/fileSysUtil'
export * as appLauncherApi from './wrappers/appLauncher'
export * as siteBrowserApi from './wrappers/siteBrowser'
export * as pluginTaskUrlListenerApi from './wrappers/pluginTaskUrlListener'
export * as windowApi from './wrappers/window'
export * as fsmonitorApi from './wrappers/fsmonitor'
export * as workdirGuardApi from './wrappers/workdirGuard'

// 注意：client.ts, proxy.ts, routes.ts 已不再需要，不再导出