/**
 * IPC Channel → HTTP Route 映射表
 * 基于 Go 后端 handler 路由和 LocalTagHttpApi.ts 调用模式
 */

import type { IpcRouteMapping } from './types'

/**
 * 路由映射表
 * 格式: IPC Channel 名称 -> { method, path }
 */
export const routeMapping: IpcRouteMapping = {
  // ========== LocalTag ==========
  'localTag-save': { method: 'POST', path: '/api/localTag/save' },
  'localTag-deleteById': { method: 'POST', path: '/api/localTag/delete' },
  'localTag-updateById': { method: 'POST', path: '/api/localTag/update' },
  'localTag-getById': { method: 'GET', path: '/api/localTag' },
  'localTag-queryPage': { method: 'GET', path: '/api/localTag/page' },
  'localTag-queryDTOPage': { method: 'GET', path: '/api/localTag/dtoPage' },
  'localTag-getTree': { method: 'GET', path: '/api/localTag/tree' },
  'localTag-listSelectItems': { method: 'GET', path: '/api/localTag/selectItems' },
  'localTag-querySelectItemPage': { method: 'GET', path: '/api/localTag/selectItemPage' },
  'localTag-listByWorkId': { method: 'GET', path: '/api/localTag/listByWorkId' },
  'localTag-querySelectItemPageByWorkId': { method: 'GET', path: '/api/localTag/work' },

  // ========== LocalAuthor ==========
  'localAuthor-save': { method: 'POST', path: '/api/localAuthor/save' },
  'localAuthor-deleteById': { method: 'POST', path: '/api/localAuthor/delete' },
  'localAuthor-updateById': { method: 'POST', path: '/api/localAuthor/update' },
  'localAuthor-getById': { method: 'GET', path: '/api/localAuthor' },
  'localAuthor-queryPage': { method: 'GET', path: '/api/localAuthor/page' },
  'localAuthor-listSelectItems': { method: 'GET', path: '/api/localAuthor/selectItems' },
  'localAuthor-querySelectItemPage': { method: 'GET', path: '/api/localAuthor/selectItemPage' },

  // ========== Site ==========
  'site-save': { method: 'POST', path: '/api/site/save' },
  'site-deleteById': { method: 'POST', path: '/api/site/delete' },
  'site-updateById': { method: 'POST', path: '/api/site/update' },
  'site-getById': { method: 'GET', path: '/api/site' },
  'site-queryPage': { method: 'GET', path: '/api/site/page' },
  'site-querySelectItemPage': { method: 'GET', path: '/api/site/selectItemPage' },
  'site-getBySiteAndSiteWorkID': { method: 'GET', path: '/api/site/getBySiteAndSiteWorkID' },
  'site-getBySiteWorkSetIdAndSiteName': { method: 'GET', path: '/api/site/getBySiteWorkSetIdAndSiteName' },

  // ========== SiteTag ==========
  'siteTag-save': { method: 'POST', path: '/api/siteTag/save' },
  'siteTag-saveBatch': { method: 'POST', path: '/api/siteTag/saveBatch' },
  'siteTag-deleteById': { method: 'POST', path: '/api/siteTag/delete' },
  'siteTag-updateById': { method: 'POST', path: '/api/siteTag/update' },
  'siteTag-getById': { method: 'GET', path: '/api/siteTag' },
  'siteTag-queryPage': { method: 'GET', path: '/api/siteTag/page' },
  'siteTag-queryDTOPage': { method: 'GET', path: '/api/siteTag/dtoPage' },
  'siteTag-queryBoundOrUnboundToLocalTagPage': { method: 'GET', path: '/api/siteTag/boundOrUnboundPage' },
  'siteTag-queryPageByWorkId': { method: 'GET', path: '/api/siteTag/work' },
  'siteTag-queryLocalRelateDTOPage': { method: 'GET', path: '/api/siteTag/localRelatePage' },
  'siteTag-querySelectItemPageByWorkId': { method: 'GET', path: '/api/siteTag/work' },
  'siteTag-listByWorkId': { method: 'GET', path: '/api/siteTag/listByWorkId' },
  'siteTag-listBySiteTagIds': { method: 'POST', path: '/api/siteTag/listBySiteTagIds' },
  'siteTag-updateBindLocalTag': { method: 'POST', path: '/api/siteTag/updateBindLocalTag' },
  'siteTag-createAndBindSameNameLocalTag': { method: 'POST', path: '/api/siteTag/createAndBindSameNameLocalTag' },

  // ========== SiteAuthor ==========
  'siteAuthor-save': { method: 'POST', path: '/api/siteAuthor/save' },
  'siteAuthor-saveBatch': { method: 'POST', path: '/api/siteAuthor/saveBatch' },
  'siteAuthor-deleteById': { method: 'POST', path: '/api/siteAuthor/delete' },
  'siteAuthor-updateById': { method: 'POST', path: '/api/siteAuthor/update' },
  'siteAuthor-getById': { method: 'GET', path: '/api/siteAuthor' },
  'siteAuthor-queryPage': { method: 'GET', path: '/api/siteAuthor/page' },
  'siteAuthor-queryBoundOrUnboundInLocalAuthorPage': { method: 'GET', path: '/api/siteAuthor/boundOrUnboundPage' },
  'siteAuthor-queryLocalRelateDTOPage': { method: 'GET', path: '/api/siteAuthor/localRelatePage' },
  'siteAuthor-listByWorkId': { method: 'GET', path: '/api/siteAuthor/listByWorkId' },
  'siteAuthor-listBySiteAuthorIds': { method: 'POST', path: '/api/siteAuthor/listBySiteAuthorIds' },
  'siteAuthor-listRankedSiteAuthorWithWorkIdByWorkIds': {
    method: 'POST',
    path: '/api/siteAuthor/listRankedSiteAuthorWithWorkIdByWorkIds'
  },
  'siteAuthor-updateBindLocalAuthor': { method: 'POST', path: '/api/siteAuthor/updateBindLocalAuthor' },
  'siteAuthor-createAndBindSameNameLocalAuthor': { method: 'POST', path: '/api/siteAuthor/createAndBindSameNameLocalAuthor' },

  // ========== Work ==========
  'work-getFullWorkInfoById': { method: 'GET', path: '/api/work' },
  'work-queryPage': { method: 'GET', path: '/api/work/page' },
  'work-deleteWorkAndSurroundingData': { method: 'POST', path: '/api/work/delete' },
  'work-listRankedLocalAuthorWithWorkIdByWorkIds': { method: 'POST', path: '/api/work/listRankedLocalAuthorWithWorkIdByWorkIds' },
  'work-listRankedSiteAuthorWithWorkIdByWorkIds': { method: 'POST', path: '/api/work/listRankedSiteAuthorWithWorkIdByWorkIds' },
  'work-listReWorkAuthor': { method: 'POST', path: '/api/work/listReWorkAuthor' },
  'work-updateLastUsed': { method: 'POST', path: '/api/work/updateLastUsed' },

  // ========== WorkSet ==========
  'workSet-listWorkSetWithWorkByIds': { method: 'POST', path: '/api/workSet/listWithWorkByIds' },
  'workSet-queryPageWithCover': { method: 'GET', path: '/api/workSet/pageWithCover' },
  'workSet-getById': { method: 'GET', path: '/api/workSet' },
  'workSet-queryPage': { method: 'GET', path: '/api/workSet/page' },
  'workSet-save': { method: 'POST', path: '/api/workSet/save' },
  'workSet-update': { method: 'POST', path: '/api/workSet/update' },
  'workSet-delete': { method: 'POST', path: '/api/workSet/delete' },
  'workSet-linkBatch': { method: 'POST', path: '/api/workSet/:id/linkBatch' },
  'workSet-removeBatch': { method: 'POST', path: '/api/workSet/:id/removeBatch' },
  'workSet-getWorks': { method: 'GET', path: '/api/workSet/:id/works' },
  'workSet-setCover': { method: 'POST', path: '/api/workSet/:id/setCover' },

  // ========== ReWorkWorkSet ==========
  'reWorkWorkSet-linkBatchToWorkSet': { method: 'POST', path: '/api/workSet/:workSetId/linkBatch' },
  'reWorkWorkSet-removeBatchFromWorkSet': { method: 'POST', path: '/api/workSet/:workSetId/removeBatch' },
  'reWorkWorkSet-updateSortOrders': { method: 'POST', path: '/api/workSet/:workSetId/updateSortOrders' },
  'reWorkWorkSet-setCover': { method: 'POST', path: '/api/workSet/:workSetId/setCover/:workId' },
  'reWorkWorkSet-unsetCover': { method: 'POST', path: '/api/workSet/:workSetId/unsetCover/:workId' },
  'reWorkWorkSet-getCoverWorkId': { method: 'GET', path: '/api/workSet/:workSetId/coverWorkId' },

  // ========== ReWorkTag ==========
  'reWorkTag-link': { method: 'POST', path: '/api/reWorkTag/link' },
  'reWorkTag-linkBatch': { method: 'POST', path: '/api/reWorkTag/linkBatch' },
  'reWorkTag-unlink': { method: 'POST', path: '/api/reWorkTag/unlink' },
  'reWorkTag-removeBatch': { method: 'POST', path: '/api/reWorkTag/removeBatch' },
  'reWorkTag-list': { method: 'GET', path: '/api/reWorkTag/list' },

  // ========== Search ==========
  'search-querySearchConditionPage': { method: 'GET', path: '/api/search/conditionPage' },
  'search-queryWorkPage': { method: 'POST', path: '/api/search/workPage' },
  'search-queryWorkSetPage': { method: 'GET', path: '/api/search/workSetPage' },

  // ========== Task ==========
  'task-getById': { method: 'GET', path: '/api/task' },
  'task-queryPage': { method: 'GET', path: '/api/task/page' },
  'task-queryParentPage': { method: 'GET', path: '/api/task/parentPage' },
  'task-save': { method: 'POST', path: '/api/task/save' },
  'task-update': { method: 'POST', path: '/api/task/update' },
  'task-delete': { method: 'POST', path: '/api/task/delete' },
  'task-refreshStatus': { method: 'POST', path: '/api/task/refreshStatus' },
  'task-setTreeStatus': { method: 'POST', path: '/api/task/setTreeStatus' },
  'task-listTree': { method: 'POST', path: '/api/task/listTree' },
  'task-listStatus': { method: 'GET', path: '/api/task/status' },
  'task-listSchedule': { method: 'GET', path: '/api/task/schedule' },
  'task-create': { method: 'POST', path: '/api/task/create' },
  'task-createByUrl': { method: 'POST', path: '/api/task/createByUrl' },
  'task-queryChildrenTaskPage': { method: 'GET', path: '/api/task/childrenPage' },

  // ========== TaskManager ==========
  'taskManager-startTree': { method: 'POST', path: '/api/taskManager/startTree' },
  'taskManager-pauseTree': { method: 'POST', path: '/api/taskManager/pauseTree' },
  'taskManager-resumeTree': { method: 'POST', path: '/api/taskManager/resumeTree' },
  'taskManager-stopTree': { method: 'POST', path: '/api/taskManager/stopTree' },
  'taskManager-retryTree': { method: 'POST', path: '/api/taskManager/retryTree' },

  // ========== Plugin ==========
  'plugin-getById': { method: 'GET', path: '/api/plugin/:id' },
  'plugin-getByPublicId': { method: 'GET', path: '/api/plugin/publicId/:publicId' },
  'plugin-queryPage': { method: 'GET', path: '/api/plugin/page' },
  'plugin-checkInstalled': { method: 'GET', path: '/api/plugin/installed/:publicId' },
  'plugin-save': { method: 'POST', path: '/api/plugin/save' },
  'plugin-update': { method: 'POST', path: '/api/plugin/update' },
  'plugin-delete': { method: 'POST', path: '/api/plugin/delete/:id' },
  'plugin-readVueFile': { method: 'GET', path: '/api/plugin/readVueFile' },
  'plugin-installFromPath': { method: 'POST', path: '/api/plugin/installFromPath' },
  'plugin-reinstall': { method: 'POST', path: '/api/plugin/reinstall/:publicId' },
  'plugin-reinstallFromPath': { method: 'POST', path: '/api/plugin/reinstallFromPath/:publicId' },
  'plugin-uninstall': { method: 'POST', path: '/api/plugin/uninstall/:publicId' },

  // ========== Settings ==========
  'settings-getSettings': { method: 'GET', path: '/api/settings' },
  'settings-saveSettings': { method: 'POST', path: '/api/settings/save' },
  'settings-resetSettings': { method: 'POST', path: '/api/settings/reset' },

  // ========== SecureStorage ==========
  'secureStorage-set': { method: 'POST', path: '/api/secureStorage/set' },
  'secureStorage-get': { method: 'GET', path: '/api/secureStorage/getValue' },
  'secureStorage-delete': { method: 'POST', path: '/api/secureStorage/remove' },
  'secureStorage-hasKey': { method: 'GET', path: '/api/secureStorage/hasKey' },
  'secureStorage-listKeys': { method: 'GET', path: '/api/secureStorage/listKeys' },

  // ========== AppLauncher ==========
  'appLauncher-openImage': { method: 'POST', path: '/api/appLauncher/openImage' },
  'appLauncher-open': { method: 'POST', path: '/api/appLauncher/open' },

  // ========== FileSysUtil ==========
  'fileSysUtil-dirSelect': { method: 'POST', path: '/api/fileSysUtil/dirSelect' },

  // ========== Slot ==========
  'slot-getAllSlots': { method: 'GET', path: '/api/slot/all' },

  // ========== SiteBrowser ==========
  'siteBrowser-queryPage': { method: 'GET', path: '/api/siteBrowser/page' },
  'siteBrowser-list': { method: 'GET', path: '/api/siteBrowser/list' },
  'siteBrowser-getById': { method: 'GET', path: '/api/siteBrowser' },
  'siteBrowser-getByPluginId': { method: 'GET', path: '/api/siteBrowser/byPluginId' },
  'siteBrowser-open': { method: 'POST', path: '/api/siteBrowser/open/:pluginPublicId/:contributionId' },

  // ========== PluginTaskUrlListener ==========
  'pluginTaskUrlListener-listListener': { method: 'GET', path: '/api/pluginTaskUrlListener/listListener' }
}

/**
 * 获取所有已映射的 IPC Channel
 */
export function getMappedChannels(): string[] {
  return Object.keys(routeMapping)
}

/**
 * 检查 channel 是否已映射
 */
export function hasChannel(channel: string): boolean {
  return channel in routeMapping
}
