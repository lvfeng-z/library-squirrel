# Page 移除 Query 重构计划

## 需求概述

将 `Page[D any, Q any]` 中的 `Query Q` 字段和泛型参数 `Q` 移除，使 `Page` 只承载分页元数据和数据列表。Query 作为独立参数在各层方法中传递。后端响应不再回传 Query。

## 需求详情

### 核心变更

1. `Page[D any, Q any]` → `Page[D any]`，移除 `Query` 字段和 `Q` 泛型
2. `NewPage[D any, Q any]` → `NewPage[D any]`
3. 所有以 Page 为入参的方法，将 Query 拆为独立参数
4. 后端响应不再回传 Query（消除所有 `result.Query` / `page.Query` 回传）
5. 前端 `IPage<Data, Query>` / `Page<Data, Query>` 同步移除 query 属性和 Query 泛型
6. 前端 API 调用改为 page + query 分离的请求方式

### 不变更的文件

- `pkg/query/converter.go` 的 `ToPageOption` 函数 — 已接收独立参数
- `internal/database/` 的 `PageOption` 结构体 — 与 Page 无关

### 特殊 case 处理

- `search/handler.go:23` 的 `Page[SearchCondition, SearchCondition]` — 暂时保留不重构
- `task/handler.go` 中已由用户移除的 `page.Query == (TaskQueryDTO{})` 判断 — 无需处理
- `siteTag/handler.go:161` 已改为指针接收 — 无需额外处理

## 技术方案

### 涉及模块

#### Go 后端（约 27 个文件）

| 层级 | 文件 | 变更类型 |
|------|------|----------|
| Model | `pkg/model/base.go` | `Page` 和 `NewPage` 去掉 Q |
| Handler | `internal/localAuthor/handler.go` | 签名变更 + 去掉 Query 回传 |
| Handler | `internal/localTag/handler.go` | 签名变更 + 去掉 Query 回传 |
| Handler | `internal/plugin/handler.go` | 签名变更 + 去掉 Query 回传 |
| Handler | `internal/work/handler.go` | 签名变更 + 去掉 Query 回传 |
| Handler | `internal/workSet/handler.go` | 签名变更 + 去掉 Query 回传 |
| Handler | `internal/site/handler.go` | 签名变更 + 去掉 Query 回传 |
| Handler | `internal/siteAuthor/handler.go` | 签名变更 + 去掉 Query 回传 |
| Handler | `internal/siteTag/handler.go` | 签名变更 + 去掉 Query 回传 |
| Handler | `internal/task/handler.go` | 签名变更 + 去掉 Query 回传 |
| Service | `internal/localAuthor/service.go` | 接口 + 实现签名变更 |
| Service | `internal/localTag/service.go` | 接口 + 实现签名变更 |
| Service | `internal/plugin/service.go` | 接口 + 实现签名变更 |
| Service | `internal/work/service.go` | 接口 + 实现签名变更 |
| Service | `internal/workSet/service.go` | 接口 + 实现签名变更 |
| Service | `internal/site/service.go` | 接口 + 实现签名变更 |
| Service | `internal/siteAuthor/service.go` | 接口 + 实现签名变更 |
| Service | `internal/siteTag/service.go` | 接口 + 实现签名变更 |
| Service | `internal/task/service.go` | 接口 + 实现签名变更 |
| Repository | `internal/database/base_repository.go` | 返回类型简化 |
| Repository | `internal/localTag/repository.go` | 返回类型简化 |
| Repository | `internal/localAuthor/repository.go` | 返回类型简化 |
| Repository | `internal/site/repository.go` | 返回类型简化 |
| Repository | `internal/siteAuthor/repository.go` | 返回类型简化 |
| Repository | `internal/siteTag/repository.go` | 返回类型简化 |
| Repository | `internal/task/repository.go` | 返回类型简化 |

#### 前端（约 39 个文件）

| 类别 | 文件 | 变更类型 |
|------|------|----------|
| 类型定义 | `model/util/IPage.ts` | 移除 Query 泛型和 query 字段 |
| 类型定义 | `model/util/Page.ts` | 移除 Query 泛型和 query 字段 |
| 工具函数 | `utils/Pager.ts` | 适配单泛型 |
| Wails 绑定 | `bindings/.../models.ts` | 自动重新生成 |
| API 封装 | `apis/http/wrappers/*.ts` (8个) | 传参方式变更 |
| API 封装 | `apis/LocalAuthorApi.ts` | 移除 query 回传 |
| API 封装 | `apis/LocalTagApi.ts` | 移除 query 回传 |
| API 封装 | `apis/SiteApi.ts` | 移除 query 回传 |
| 视图组件 | `views/*.vue` (7个) | query 状态管理重构 |
| 通用组件 | `components/common/*.vue` (5个) | 适配新 IPage 接口 |
| 对话框组件 | `components/dialogs/*.vue` (4个) | 移除 query 回传 |

### 技术要点

#### 1. Go 后端重构模式

**Handler 签名变更模式**：

```go
// 变更前
func (h *Handler) QueryPage(ctx context.Context, page *model.Page[dto.XX, QueryDTO]) *model.ApiResponse[*model.Page[dto.XX, QueryDTO]]

// 变更后
func (h *Handler) QueryPage(ctx context.Context, page *model.Page[dto.XX], query QueryDTO) *model.ApiResponse[*model.Page[dto.XX]]
```

**Handler 内部构造 entityPage 模式变更**：

```go
// 变更前
entityPage := &model.Page[entity.XX, QueryDTO]{
    PageNumber: page.PageNumber,
    PageSize:   page.PageSize,
    Query:      page.Query,
}

// 变更后
entityPage := &model.Page[entity.XX]{
    PageNumber: page.PageNumber,
    PageSize:   page.PageSize,
}
```

**Service 方法变更模式**：

```go
// 变更前
func (s *Service) Page(ctx context.Context, page *model.Page[entity.XX, QueryDTO]) (*model.Page[entity.XX, any], error) {
    opt, err := conv.ToPageOption(page.Query, page.PageNumber, page.PageSize, nil)
    ...
}

// 变更后
func (s *Service) Page(ctx context.Context, page *model.Page[entity.XX], query QueryDTO) (*model.Page[entity.XX], error) {
    opt, err := conv.ToPageOption(query, page.PageNumber, page.PageSize, nil)
    ...
}
```

**Handler 响应构造变更模式**：

```go
// 变更前
return model.Success(&model.Page[dto.XX, QueryDTO]{
    PageNumber:   result.PageNumber,
    PageSize:     result.PageSize,
    PageCount:    result.PageCount,
    DataCount:    result.DataCount,
    CurrentCount: result.CurrentCount,
    Query:        page.Query,  // 移除此行
    Data:         data,
})

// 变更后
return model.Success(&model.Page[dto.XX]{
    PageNumber:   result.PageNumber,
    PageSize:     result.PageSize,
    PageCount:    result.PageCount,
    DataCount:    result.DataCount,
    CurrentCount: result.CurrentCount,
    Data:         data,
})
```

#### 2. 前端重构模式

**IPage / Page 类型变更**：

```typescript
// 变更前
export default interface IPage<Data, Query> { ... query?: Query ... }
export default class Page<Data, Query> implements IPage<Data, Query> { ... }

// 变更后
export default interface IPage<Data> { ... }
export default class Page<Data> implements IPage<Data> { ... }
```

**API 调用变更模式**（以 wrapper 为例）：

```typescript
// 变更前
const pageObj = new Page<SelectItem, QueryDTO>({
    pageNumber: query.pageNumber,
    pageSize: query.pageSize,
    query: queryDTO
})
const result = await Handler.QueryPage(pageObj)

// 变更后
const pageObj = new Page<SelectItem>({
    pageNumber: query.pageNumber,
    pageSize: query.pageSize,
})
const queryDTO = new QueryDTO({ ... })
const result = await Handler.QueryPage(pageObj, queryDTO)
```

**前端 query 状态管理模式变更**：

前端组件中原本通过 `page.query` 维护的查询状态，需要改为独立的 `query` ref 变量。

```typescript
// 变更前 — query 封装在 page 内
const page = ref(new Page<DTO, QueryDTO>())
if (isNullish(page.value.query)) {
    page.value.query = new QueryDTO()
}
page.value.query.updateTime = { ... }
const result = await handler.QueryPage(page.value)

// 变更后 — query 独立管理
const page = ref(new Page<DTO>())
const query = ref(new QueryDTO())
query.value.updateTime = { ... }
const result = await handler.QueryPage(page.value, query.value)
```

## 开发步骤

### Phase 1: Go Model 定义变更

1. 修改 `pkg/model/base.go`
   - `Page[D any, Q any]` → `Page[D any]`
   - 移除 `Query Q` 字段
   - `NewPage[D any, Q any]` → `NewPage[D any]`

### Phase 2: Go Repository 层返回类型简化

按模块逐一修改，每个模块确保编译通过：

1. `internal/database/base_repository.go` — `*model.Page[T, any]` → `*model.Page[T]`
2. `internal/localTag/repository.go` — 返回类型去掉 Q，`NewPage` 调用去掉 Q
3. `internal/localAuthor/repository.go` — 同上
4. `internal/site/repository.go` — 同上
5. `internal/siteAuthor/repository.go` — 同上
6. `internal/siteTag/repository.go` — 同上
7. `internal/task/repository.go` — 同上

### Phase 3: Go Service 层接口和实现变更

按模块逐一修改，将 `page.Query` 拆为独立 `query` 参数：

1. `internal/localAuthor/service.go` — 接口签名 + 实现方法
2. `internal/localTag/service.go` — 接口签名 + 实现方法
3. `internal/plugin/service.go` — 同上
4. `internal/work/service.go` — 同上
5. `internal/workSet/service.go` — 同上
6. `internal/site/service.go` — 同上
7. `internal/siteAuthor/service.go` — 同上
8. `internal/siteTag/service.go` — 同上（含 enrichSiteTagsWithRelations 返回类型）
9. `internal/task/service.go` — 同上

### Phase 4: Go Handler 层签名和逻辑变更

按模块逐一修改，Wails 绑定入口点变更：

1. `internal/localAuthor/handler.go` — 签名 + 去掉 Query 回传
2. `internal/localTag/handler.go` — 同上
3. `internal/plugin/handler.go` — 同上
4. `internal/work/handler.go` — 同上
5. `internal/workSet/handler.go` — 同上
6. `internal/site/handler.go` — 同上
7. `internal/siteAuthor/handler.go` — 同上
8. `internal/siteTag/handler.go` — 同上
9. `internal/task/handler.go` — 同上（QueryTreeDataPage 已接收独立 query 参数）

**注意**: `search/handler.go` 暂不重构（`Page[SearchCondition, SearchCondition]`）

### Phase 5: Go 编译验证

1. 运行 `go build ./...` 确保所有模块编译通过
2. 修复所有编译错误

### Phase 6: 重新生成 Wails 绑定

1. 运行 `wails3 generate bindings -ts`
2. 验证生成的 TypeScript 绑定文件中 `Page` 只有一个泛型参数

### Phase 7: 前端类型定义变更

1. `frontend/src/model/util/IPage.ts` — 移除 Query 泛型和 query 字段
2. `frontend/src/model/util/Page.ts` — 移除 Query 泛型、query 字段、transform/copy 方法简化
3. `frontend/src/utils/Pager.ts` — 适配单泛型

### Phase 8: 前端 API 封装层适配

1. `apis/http/wrappers/localTag.ts` — 传参方式变更
2. `apis/http/wrappers/localAuthor.ts` — 同上
3. `apis/http/wrappers/site.ts` — 同上
4. `apis/http/wrappers/siteAuthor.ts` — 同上
5. `apis/http/wrappers/siteTag.ts` — 同上
6. `apis/http/wrappers/work.ts` — 同上
7. `apis/http/wrappers/workSet.ts` — 同上
8. `apis/http/wrappers/task.ts` — 同上
9. `apis/http/wrappers/plugin.ts` — 同上
10. `apis/LocalAuthorApi.ts` — 移除 `query: response.data.query`
11. `apis/LocalTagApi.ts` — 同上
12. `apis/SiteApi.ts` — 同上

### Phase 9: 前端视图和组件适配

按组件逐一修改，将 `page.query` 的状态管理迁移到独立 `query` ref：

1. `views/LocalAuthorManage.vue`
2. `views/LocalTagManage.vue`
3. `views/SiteManage.vue`
4. `views/SiteAuthorManage.vue`
5. `views/SiteTagManage.vue`
6. `views/TaskManage.vue`
7. `views/PluginManage.vue`
8. `components/common/SearchTable.vue`
9. `components/common/AutoLoadSelect.vue`
10. `components/common/TagBox.vue`
11. `components/common/ExchangeBox.vue`
12. `components/common/WorkQueryView.vue`
13. `components/common/AutoLoadTagSelect.vue`
14. `components/common/CommentInput/CommonInputAutoLoadSelect.vue`
15. `components/oneOff/MainPageWrapper.vue`
16. `components/dialogs/LocalTagDialog.vue`
17. `components/dialogs/SiteAuthorDialog.vue`
18. `components/dialogs/SiteTagDialog.vue`
19. `components/dialogs/WorkDialog.vue`
20. `components/dialogs/WorkSetDialog.vue`
21. `components/dialogs/TaskDialog.vue`
22. `model/util/CommonInputConfig.ts`
23. `views/SiteBrowserManage.vue`

### Phase 10: 前端编译验证

1. 运行 `yarn typecheck` 确保 TypeScript 类型检查通过
2. 运行 `yarn lint` 确保 ESLint 检查通过
3. 运行 `yarn build` 确保构建成功

## 验收标准

1. `go build ./...` 编译通过，无错误
2. `yarn typecheck` 类型检查通过
3. `yarn lint` ESLint 检查通过
4. `yarn build` 构建成功
5. 所有 `Page` 使用点只有单泛型 `Page[D]`
6. 后端响应中无 `Query` 字段回传
7. 前端 query 状态独立于 Page 管理

## 预计工作量

- Go 后端：约 27 个文件，机械性重构，风险可控
- 前端：约 39 个文件，需要理解每个组件的 query 使用模式
- 最复杂的部分：前端视图组件中 query 状态管理的拆分（Phase 9）
