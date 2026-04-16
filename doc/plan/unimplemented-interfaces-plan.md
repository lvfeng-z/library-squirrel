# 未实现接口实现计划

## 概述

前端 `wrappers` 目录中存在 42 个未实现的接口，需要在 bindings 中实现以便调用后端服务。

## 实现顺序（优先级从高到低）

### Phase 1: 核心基础模块
| 模块 | 接口数量 | 文件 |
|------|----------|------|
| localTag | 1 | localTag.ts |
| site | 2 | site.ts |

### Phase 2: 关联操作模块
| 模块 | 接口数量 | 文件 |
|------|----------|------|
| reWorkWorkSet | 6 | reWorkWorkSet.ts |
| work | 5 | work.ts |
| workSet | 6 | workSet.ts |

### Phase 3: 复杂查询模块
| 模块 | 接口数量 | 文件 |
|------|----------|------|
| plugin | 3 | plugin.ts |
| siteAuthor | 7 | siteAuthor.ts |
| siteTag | 7 | siteTag.ts |

### Phase 4: 后续规划（最后实现）
| 模块 | 接口数量 | 文件 |
|------|----------|------|
| siteBrowser | 1 | siteBrowser.ts |
| task | 5 | task.ts |

## 接口清单

### Phase 1

#### localTag.ts (1个)
- `localTagQuerySelectItemPageByWorkId` - 根据作品ID分页查询选择项

#### site.ts (2个)
- `siteGetBySiteAndSiteWorkID` - 根据站点和站点作品ID获取
- `siteGetBySiteWorkSetIdAndSiteName` - 根据作品集ID和站点名获取

### Phase 2

#### reWorkWorkSet.ts (6个)
- `reWorkWorkSetLinkBatchToWorkSet` - 批量关联作品到作品集
- `reWorkWorkSetRemoveBatchFromWorkSet` - 批量从作品集移除作品
- `reWorkWorkSetUpdateSortOrders` - 批量更新排序
- `reWorkWorkSetSetCover` - 设置封面
- `reWorkWorkSetUnsetCover` - 取消封面
- `reWorkWorkSetGetCoverWorkId` - 获取封面作品ID

#### work.ts (5个)
- `workListRankedLocalAuthorWithWorkIdByWorkIds` - 根据作品ID列表获取带排名的本地作者
- `workListRankedSiteAuthorWithWorkIdByWorkIds` - 根据作品ID列表获取带排名的站点作者
- `workListReWorkAuthor` - 获取作品关联的作者
- `workUpdateLastUsed` - 更新最后使用时间

#### workSet.ts (6个)
- `workSetListWorkSetWithWorkByIds` - 根据ID列表获取作品集及作品
- `workSetQueryPageWithCover` - 分页查询作品集（带封面）
- `workSetLinkBatch` - 批量关联作品到作品集
- `workSetRemoveBatch` - 批量移除作品
- `workSetGetWorks` - 获取作品集下的作品
- `workSetSetCover` - 设置封面

### Phase 3

#### plugin.ts (3个)
- `pluginSave` - 保存插件
- `pluginUpdate` - 更新插件
- `pluginReinstallFromPath` - 从路径重新安装插件

#### siteAuthor.ts (7个)
- `siteAuthorSaveBatch` - 批量保存站点作者
- `siteAuthorQueryBoundOrUnboundInLocalAuthorPage` - 分页查询已绑定/未绑定到本地作者的站点作者
- `siteAuthorQueryLocalRelateDTOPage` - 分页查询本地关联DTO
- `siteAuthorListBySiteAuthorIds` - 根据站点作者ID列表获取
- `siteAuthorListRankedSiteAuthorWithWorkIdByWorkIds` - 根据作品ID列表获取带排名的站点作者
- `siteAuthorUpdateBindLocalAuthor` - 更新绑定本地作者
- `siteAuthorCreateAndBindSameNameLocalAuthor` - 创建并绑定同名本地作者

#### siteTag.ts (7个)
- `siteTagSaveBatch` - 批量保存站点标签
- `siteTagQueryBoundOrUnboundToLocalTagPage` - 分页查询已绑定/未绑定到本地标签的站点标签
- `siteTagQueryPageByWorkId` - 根据作品ID分页查询站点标签
- `siteTagQueryLocalRelateDTOPage` - 分页查询本地关联DTO
- `siteTagListBySiteTagIds` - 根据站点标签ID列表获取
- `siteTagUpdateBindLocalTag` - 更新绑定本地标签
- `siteTagCreateAndBindSameNameLocalTag` - 创建并绑定同名本地标签

### Phase 4

#### siteBrowser.ts (1个)
- `siteBrowserQueryPage` - 分页查询站点浏览器

#### task.ts (5个)
- `taskSave` - 保存任务
- `taskUpdate` - 更新任务
- `taskRefreshStatus` - 刷新任务状态
- `taskSetTreeStatus` - 设置任务树状态
