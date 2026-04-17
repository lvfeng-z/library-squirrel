# 接口不一致修复计划

## 概述

本计划用于记录 library-squirrel 项目 handler 层与重构前 LibrarySquirrel 项目 Service 层之间的接口不一致问题，并制定修复方案。

**重构前项目**: `E:/code/lvfeng/LibrarySquirrel/src/main/service/`
**本项目**: `E:/code/lvfeng/library-squirrel/internal/`

**排除项**:
- 上下文参数（`ctx context.Context`）差异 - wails 生成的 bindings 不会包含 ctx
- Handler 层新增的方法 - 不需要修复

---

## 需要修复的不一致

### 1. siteTag

| 方法名 | 本项目 handler | 重构前 Service | 状态 |
|--------|----------------|----------------|------|
| QueryPage | `QueryPage(ctx, page, pageSize int, queryDTO *SiteTagQueryDTO)` | `queryPage(page Page<SiteTagQueryDTO, SiteTag>)` | **待修复** |
| QuerySelectItemPageByWorkId | `QuerySelectItemPageByWorkId(ctx, page, pageSize int, queryDTO *SiteTagQueryDTO, workId int64)` | `querySelectItemPageByWorkId(page Page<...>)` | **待修复** |
| QueryLocalRelateDTOPage | `QueryLocalRelateDTOPage(ctx, page, pageSize int, queryDTO *SiteTagQueryDTO)` | `queryLocalRelateDTOPage(page Page<...>)` | **待修复** |

### 2. site

| 方法名 | 本项目 handler | 重构前 Service | 状态 |
|--------|----------------|----------------|------|
| QuerySelectItemPage | `QuerySelectItemPage(ctx, page, pageSize int, queryDTO *SiteQueryDTO)` | `querySelectItemPage(page Page<SiteQueryDTO, Site>)` | **待修复** |

### 3. localTag

| 方法名 | 本项目 handler | 重构前 Service | 状态 |
|--------|----------------|----------------|------|
| QuerySelectItemPage | `QuerySelectItemPage(ctx, page, pageSize int, queryDTO *LocalTagQueryDTO, secondaryLabel string)` | `querySelectItemPage(page Page<...>, secondaryLabelName string)` | **待修复** |
| QuerySelectItemPageByWorkId | `QuerySelectItemPageByWorkId(ctx, page, pageSize int, queryDTO *LocalTagQueryDTO, workId int64)` | `querySelectItemPageByWorkId(page Page<...>)` | **待修复** |

### 4. localAuthor

| 方法名 | 本项目 handler | 重构前 Service | 状态 |
|--------|----------------|----------------|------|
| QuerySelectItemPage | `QuerySelectItemPage(ctx, page, pageSize int, queryDTO *LocalAuthorQueryDTO)` | `querySelectItemPage(page Page<LocalAuthorQueryDTO, LocalAuthor>)` | **待修复** |

### 5. siteAuthor

| 方法名 | 本项目 handler | 重构前 Service | 状态 |
|--------|----------------|----------------|------|
| QueryBoundOrUnboundToLocalAuthorPage | `QueryBoundOrUnboundToLocalAuthorPage(ctx, page, pageSize int, queryDTO *SiteAuthorQueryDTO)` | `queryBoundOrUnboundInLocalAuthorPage(page Page<...>)` | **待修复**（命名差异） |
| QuerySelectItemPage | `QuerySelectItemPage(ctx, page, pageSize int, queryDTO *SiteAuthorQueryDTO)` | `querySelectItemPage(page Page<...>)` | **待修复** |
| QueryLocalRelateDTOPage | `QueryLocalRelateDTOPage(ctx, page, pageSize int, queryDTO *SiteAuthorQueryDTO)` | `queryLocalRelateDTOPage(page Page<...>)` | **待修复** |

### 6. reWorkTag

| 方法名 | 本项目 handler | 重构前 Service | 状态 |
|--------|----------------|----------------|------|
| Save | `Save(ctx, workId int64, tagType int, tagId int64)` | `link(type OriginType, tagIds number[], workId number)` | **待修复** |
| SaveBatch | `SaveBatch(ctx, workId int64, tagType int, tagIds []int64)` | `link(type OriginType, tagIds number[], workId number)` | **待修复** |
| Delete | `Delete(ctx, workId int64, tagType int, tagId int64)` | `unlink(type OriginType, tagIds number[], workId number)` | **待修复** |
| DeleteBatch | `DeleteBatch(ctx, workId int64, tagType int, tagIds []int64)` | `unlink(type OriginType, tagIds number[], workId number)` | **待修复** |

### 7. search

| 方法名 | 本项目 handler | 重构前 Service | 状态 |
|--------|----------------|----------------|------|
| QueryWorkPage | `QueryWorkPage(ctx, page, pageSize int, conditions []*domain.SearchCondition)` | `queryWorkPage(page Page<SearchCondition[], WorkFullDTO>)` | **待修复** |
| QueryWorkSetPage | `QueryWorkPage(ctx, page, pageSize int, keyword string, siteId int64)` | `queryWorkSetPage(page Page<...>)` | **待修复** |

### 8. task

| 方法名 | 本项目 handler | 重构前 Service | 状态 |
|--------|----------------|----------------|------|
| DeleteTask | `DeleteTask(ctx, id int64)` | `deleteTask(taskIds number[])` | **待修复**（单个 vs 批量） |
| ListTaskTree | `ListTaskTree(ctx, taskIds []int64)` | `listTaskTree(taskIds, includeStatus[])` | **待修复**（缺少参数） |
| QueryTreeDataPage | `QueryTreeDataPage(ctx, treeId int64)` | `queryTreeDataPage(page Page<...>)` | **待修复**（参数完全不同） |
| QueryParentPage | `QueryParentPage(ctx, page, pageSize int, queryDTO *TaskQueryDTO)` | `queryParentPage(page Page<...>)` | **待修复** |
| QueryChildrenTaskPage | `QueryChildrenTaskPage(ctx, pid int64, page, pageSize int, queryDTO *TaskQueryDTO)` | `queryChildrenTaskPage(page Page<...>)` | **待修复** |

### 9. workSet

| 方法名 | 本项目 handler | 重构前 Service | 状态 |
|--------|----------------|----------------|------|
| QueryPageWithCover | `QueryPageWithCover(ctx, page, pageSize int, queryDTO *WorkSetQueryDTO)` | `queryPageWithCover(page Page<...>)` | **待修复** |
| UpdateSortOrders | `UpdateSortOrders(ctx, workSetId int64, sortOrders map[int64]int)` | `updateWorkSortOrderInWorkSet(workSetId, workIds number[])` | **待修复**（参数结构） |

### 10. plugin

| 方法名 | 本项目 handler | 重构前 Service | 状态 |
|--------|----------------|----------------|------|
| Page | `Page(ctx, page, pageSize int, queryDTO *PluginQueryDTO)` | `queryPage(page Page<...>)` | **待修复** |

---

## 不需要修复的不一致（已排除）

| 模块 | 原因 |
|------|------|
| 所有模块的 ctx 参数差异 | wails bindings 不包含 ctx |
| backup 模块新增方法 | CreatePluginBackup, GetPluginBackup 是新增方法 |
| appLauncher 模块新增方法 | Open, OpenPath, OpenExternal 是新增方法 |
| workSet 模块新增方法 | Update, LinkBatchToWorkSet, RemoveBatchFromWorkSet 是新增方法 |
| task 模块新增方法 | CreateTask, RefreshStatus, SetTreeStatus 是新增方法 |
| reWorkAuthor 模块新增方法 | ListByWorkIds 是新增方法 |
| secureStorage 模块 | Keys vs listKeys 命名差异（可接受） |

---

## 修复策略

### 1. 分页方法统一使用 Page<T> 封装参数

**涉及模块**: siteTag, site, localTag, localAuthor, siteAuthor, workSet, plugin, task

**修复方式**: 将独立的 `page, pageSize int` 参数改为使用 `*model.Page[DTO]` 封装

### 2. reWorkTag 方法参数结构调整

**修复方式**:
- `Save`/`SaveBatch` → 改为 `link(type OriginType, tagIds []int64, workId int64)`
- `Delete`/`DeleteBatch` → 改为 `unlink(type OriginType, tagIds []int64, workId int64)`

### 3. search 模块参数调整

**修复方式**: 将 `QueryWorkPage` 和 `QueryWorkSetPage` 的独立参数封装到 Page 对象中

### 4. task 模块参数调整

**修复方式**: 调整 `ListTaskTree` 添加 `includeStatus` 参数，调整 `QueryTreeDataPage` 使用 Page 封装

### 5. workSet 参数调整

**修复方式**: `UpdateSortOrders` 的 `map[int64]int` 参数改为 `[]int64`（作品ID数组）

---

## 修复优先级

### P0 - 必须修复（影响前端调用）

| 模块 | 方法 | 问题 |
|------|------|------|
| siteTag | QueryPage | 分页参数不一致 |
| siteTag | QueryBoundOrUnboundToLocalTagPage | 分页参数不一致 |
| localTag | QuerySelectItemPage | 分页参数不一致 |
| reWorkTag | Save/SaveBatch | 参数结构不同 |
| reWorkTag | Delete/DeleteBatch | 参数结构不同 |
| search | QueryWorkPage | 分页参数不一致 |

### P1 - 建议修复

| 模块 | 方法 | 问题 |
|------|------|------|
| task | QueryParentPage | 分页参数不一致 |
| task | QueryChildrenTaskPage | 分页参数不一致 |
| workSet | QueryPageWithCover | 分页参数不一致 |
| plugin | Page | 分页参数不一致 |

### P2 - 可延后处理

| 模块 | 方法 | 问题 |
|------|------|------|
| siteAuthor | QueryBoundOrUnboundToLocalAuthorPage | 命名差异 |
| task | DeleteTask | 单个 vs 批量 |
| task | ListTaskTree | 缺少 includeStatus 参数 |
| task | QueryTreeDataPage | 参数完全不同 |
| workSet | UpdateSortOrders | 参数结构不同 |

---

生成时间: 2026-04-17
