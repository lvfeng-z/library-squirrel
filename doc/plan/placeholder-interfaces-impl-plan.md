# 前端占位接口实现计划

## 概述

前端页面中存在 11 个占位接口，需要按照旧项目 `D:\code\lvfeng\LibrarySquirrel\src\main` 的业务逻辑进行实现。

## 现状分析

### 后端 Handler 已实现但前端未调用

| 模块 | Handler 方法 | Wrapper 方法 | 前端视图问题 |
|------|-------------|--------------|-------------|
| siteTag | `CreateAndBindSameNameLocalTag` | `siteTagCreateAndBindSameNameLocalTag` | SiteTagManage.vue 未调用 wrapper |
| siteTag | `QueryLocalRelateDTOPage` | `siteTagQueryLocalRelateDTOPage` | SiteTagManage.vue 未调用 wrapper |
| siteTag | `QueryPageByWorkId` | `siteTagQueryPageByWorkId` | WorkDialog.vue 未调用 wrapper |
| localTag | `QuerySelectItemPageByWorkId` | `localTagQuerySelectItemPageByWorkId` | WorkDialog.vue 未调用 wrapper |
| siteAuthor | `CreateAndBindSameNameLocalAuthor` | `siteAuthorCreateAndBindSameNameLocalAuthor` | SiteAuthorManage.vue 未调用 wrapper |
| siteAuthor | `QueryLocalRelateDTOPage` | `siteAuthorQueryLocalRelateDTOPage` | SiteAuthorManage.vue 未调用 wrapper |
| siteAuthor | `UpdateBindLocalAuthor` | `siteAuthorUpdateBindLocalAuthor` | LocalAuthorManage.vue 未调用 wrapper |
| siteAuthor | `QueryBoundOrUnboundToLocalAuthorPage` | `siteAuthorQueryBoundOrUnboundInLocalAuthorPage` | LocalAuthorManage.vue 未调用 wrapper |
| siteTag | `UpdateBindLocalTag` | `siteTagUpdateBindLocalTag` | LocalTagManage.vue 未调用 wrapper |
| siteTag | `QueryBoundOrUnboundToLocalTagPage` | **缺失** | LocalTagManage.vue 未调用 wrapper |
| pluginTaskUrlListener | **缺失 Handler** | **缺失** | TaskManage.vue 无法调用 |

## 实现步骤

### Phase 1: 补充缺失的 wrapper

#### 1.1 添加 `siteTagQueryBoundOrUnboundToLocalTagPage` wrapper

**文件**: `frontend/src/apis/http/wrappers/siteTag.ts`

**参考旧项目**: `D:\code\lvfeng\LibrarySquirrel\src\main\service\SiteTagService.ts` (行130-150)

**需要添加的方法**:
```typescript
export async function siteTagQueryBoundOrUnboundToLocalTagPage(
  query: {
    page: number
    pageSize: number
    query?: { localTagId?: number; boundOnLocalTagId?: boolean }
  }
): Promise<ApiResponse<PageResult>>
```

### Phase 2: 添加 pluginTaskUrlListener Handler

#### 2.1 创建 Handler

**新建文件**: `internal/pluginTaskUrlListener/handler.go`

**参考旧项目**:
- Handler 模式参考: `internal/siteTag/handler.go`
- Service 逻辑参考: `internal/pluginTaskUrlListener/service.go` (行15-18)

**需要实现的方法**:
```go
// ListListener 根据URL获取监听此链接的插件列表
func (h *Handler) ListListener(ctx context.Context, url string) *model.ApiResponse[[]*domain.PluginWithContribution]
```

#### 2.2 在 app.go 中注册 Handler

**文件**: `app.go`

参考其他 Handler 的注册方式

#### 2.3 重新生成前端 bindings

运行 `wails generate bindings` 或类似命令

#### 2.4 创建 pluginTaskUrlListener wrapper

**新建文件**: `frontend/src/apis/http/wrappers/pluginTaskUrlListener.ts`

**方法**:
```typescript
export async function pluginTaskUrlListenerManagerListListener(
  url: string
): Promise<ApiResponse<Plugin[]>>
```

### Phase 3: 修改前端视图文件

#### 3.1 SiteTagManage.vue

**文件**: `frontend/src/views/SiteTagManage.vue`

**修改内容** (第42-54行):
- 删除 `siteTagCreateAndBindSameNameLocalTag` 占位方法
- 删除 `siteTagQueryLocalRelateDTOPage` 占位方法
- 改为直接导入并调用 `siteTagApi.siteTagCreateAndBindSameNameLocalTag` 和 `siteTagApi.siteTagQueryLocalRelateDTOPage`

**修改后**:
```typescript
const apis = {
  localTagQuerySelectItemPage: localTagApi.localTagQuerySelectItemPage,
  siteTagCreateAndBindSameNameLocalTag: siteTagApi.siteTagCreateAndBindSameNameLocalTag,
  siteTagDeleteById: siteTagApi.siteTagDeleteById,
  siteTagUpdateById: siteTagApi.siteTagUpdateById,
  siteTagQueryLocalRelateDTOPage: siteTagApi.siteTagQueryLocalRelateDTOPage
}
```

#### 3.2 WorkDialog.vue

**文件**: `frontend/src/components/dialogs/WorkDialog.vue`

**修改内容** (约第72-87行):
- 删除 `localTagQuerySelectItemPageByWorkId` 占位方法
- 删除 `siteTagQueryPageByWorkId` 占位方法
- 改为调用 `localTagApi.localTagQuerySelectItemPageByWorkId` 和 `siteTagApi.siteTagQueryPageByWorkId`

#### 3.3 SiteAuthorManage.vue

**文件**: `frontend/src/views/SiteAuthorManage.vue`

**修改内容** (第40-52行):
- 删除 `siteAuthorCreateAndBindSameNameLocalAuthor` 占位方法
- 删除 `siteAuthorQueryLocalRelateDTOPage` 占位方法
- 改为调用 `siteAuthorApi.siteAuthorCreateAndBindSameNameLocalAuthor` 和 `siteAuthorApi.siteAuthorQueryLocalRelateDTOPage`

#### 3.4 LocalAuthorManage.vue

**文件**: `frontend/src/views/LocalAuthorManage.vue`

**修改内容** (约第42-55行):
- 删除 `siteAuthorUpdateBindLocalAuthor` 占位方法
- 删除 `siteAuthorQueryBoundOrUnboundInLocalAuthorPage` 占位方法
- 改为调用 `siteAuthorApi.siteAuthorUpdateBindLocalAuthor` 和 `siteAuthorApi.siteAuthorQueryBoundOrUnboundInLocalAuthorPage`

#### 3.5 LocalTagManage.vue

**文件**: `frontend/src/views/LocalTagManage.vue`

**修改内容** (约第44-60行):
- 删除 `siteTagUpdateBindLocalTag` 占位方法
- 删除 `siteTagQueryBoundOrUnboundToLocalTagPage` 占位方法
- 改为调用 `siteTagApi.siteTagUpdateBindLocalTag` 和 `siteTagApi.siteTagQueryBoundOrUnboundToLocalTagPage`

#### 3.6 TaskManage.vue

**文件**: `frontend/src/views/TaskManage.vue`

**修改内容** (约第45-49行):
- 删除 `pluginTaskUrlListenerManagerListListener` 占位方法
- 改为调用新创建的 `pluginTaskUrlListenerApi.pluginTaskUrlListenerManagerListListener`

### Phase 4: 验证与测试

1. 确保 `wails generate bindings` 成功生成 pluginTaskUrlListener 相关 bindings
2. 编译项目，确认无编译错误
3. 测试各页面的占位接口功能

## 涉及文件清单

### 后端 (Go)
| 文件 | 操作 | 说明 |
|------|------|------|
| `internal/pluginTaskUrlListener/handler.go` | 新建 | 插件任务URL监听器 Handler |
| `app.go` | 修改 | 注册新 Handler |

### 前端 Wrapper (TypeScript)
| 文件 | 操作 | 说明 |
|------|------|------|
| `frontend/src/apis/http/wrappers/siteTag.ts` | 修改 | 添加 `siteTagQueryBoundOrUnboundToLocalTagPage` |
| `frontend/src/apis/http/wrappers/pluginTaskUrlListener.ts` | 新建 | 插件任务URL监听器 wrapper |

### 前端视图 (Vue)
| 文件 | 操作 | 说明 |
|------|------|------|
| `frontend/src/views/SiteTagManage.vue` | 修改 | 使用 wrapper 方法替代占位 |
| `frontend/src/views/SiteAuthorManage.vue` | 修改 | 使用 wrapper 方法替代占位 |
| `frontend/src/views/LocalAuthorManage.vue` | 修改 | 使用 wrapper 方法替代占位 |
| `frontend/src/views/LocalTagManage.vue` | 修改 | 使用 wrapper 方法替代占位 |
| `frontend/src/views/TaskManage.vue` | 修改 | 使用 wrapper 方法替代占位 |
| `frontend/src/components/dialogs/WorkDialog.vue` | 修改 | 使用 wrapper 方法替代占位 |

## 验收标准

1. `siteTagQueryBoundOrUnboundToLocalTagPage` wrapper 正确实现
2. `pluginTaskUrlListener` Handler 和 wrapper 正确实现
3. 所有前端视图文件不再包含"此功能暂未实现"占位代码
4. 项目编译成功，无错误
5. 各页面功能正常工作
