# 模块迁移修复指南（Electron → Wails）

## 背景

项目已完成后端从 Node.js (Electron) 到 Go (Wails) 的重构，但前端模块仍保留旧的 Electron 架构模式，导致大量模块无法正常使用。每个模块需要按照统一的模式进行修复。

本文档以**本地作者模块（LocalAuthor）**的实际修复过程为参考，提炼出可复用的修复模式，供修复其他模块时参考。

---

## 修复清单总览

每个模块的修复通常涉及以下层次：

| 层次 | 常见问题 | 修复方向 |
|------|----------|----------|
| Repository | N+1 查询、跨域直接查询 | 删除特殊查询方法，复用通用分页 |
| Service | 关联数据逐条加载 | 批量查询 + Map 组装；依赖注入 |
| Handler | 参数类型不支持 nullable | 按需改为指针类型 |
| Frontend Wrapper | 冗余类型、旧 API 签名 | 简化，直接使用绑定层类型 |
| Vue 页面 | 旧查询参数结构、类型错误 | 适配 QueryAttribute、修正类型 |

---

## 后端修复模式

### 1. 消除 N+1 查询 — 从 Repository 提升到 Service

**问题**：Repository 层存在特殊查询方法（如 `QueryBoundOrUnboundToLocalAuthorPage`），在遍历结果时逐条查询关联实体。

**修复方式**：
- **删除** Repository 中的特殊查询方法
- Service 层复用通用 `repo.Page()` 获取原始实体分页
- 新增 `enrichXxxWithRelations()` 方法，收集所有关联 ID → 批量查询 → Map 组装

```go
// Service 层批量填充模式
func (s *Service) enrichSiteAuthorsWithRelations(ctx context.Context, rawPage *model.Page[entity.SiteAuthor]) (*model.Page[dto.SiteAuthorFullDTO], error) {
    siteAuthors := rawPage.Data
    if len(siteAuthors) == 0 {
        return model.NewPage[dto.SiteAuthorFullDTO](nil, rawPage.DataCount, rawPage.PageNumber, rawPage.PageSize), nil
    }

    // 1. 收集需要查询的关联 ID
    localAuthorIds := make([]int64, 0)
    siteIds := make([]int64, 0)
    for _, author := range siteAuthors {
        if author.LocalAuthorID.Valid && author.LocalAuthorID.Int64 > 0 {
            localAuthorIds = append(localAuthorIds, author.LocalAuthorID.Int64)
        }
        // ... 收集其他关联 ID
    }

    // 2. 批量查询，构建 Map
    localAuthorMap := make(map[int64]*dto.LocalAuthorDTO)
    if len(localAuthorIds) > 0 {
        localAuthors, err := s.localAuthorOp.ListByIds(ctx, localAuthorIds)
        // ... 转为 Map
    }

    // 3. 组装结果
    results := make([]*dto.SiteAuthorFullDTO, 0, len(siteAuthors))
    for _, author := range siteAuthors {
        fullDTO := dto.NewSiteAuthorFullDTO(author)
        if author.LocalAuthorID.Valid && author.LocalAuthorID.Int64 > 0 {
            fullDTO.LocalAuthor = localAuthorMap[author.LocalAuthorID.Int64]
        }
        results = append(results, fullDTO)
    }

    return model.NewPage[dto.SiteAuthorFullDTO](results, rawPage.DataCount, rawPage.PageNumber, rawPage.PageSize), nil
}
```

### 2. 依赖注入解耦

**问题**：Repository 直接跨模块查询其他实体，违反职责边界。

**修复方式**：
- Service 定义所需的外部接口（如 `LocalAuthorOperator`、`SiteOperator`）
- 在 `NewService()` 中注入依赖
- `app.go` 中组装时传入对应 Service 实例

```go
// Service 层定义接口
type LocalAuthorOperator interface {
    ListByIds(ctx context.Context, ids []int64) ([]*entity.LocalAuthor, error)
}

type Service struct {
    repo          Repository
    localAuthorOp LocalAuthorOperator
    siteOp        SiteOperator
}

func NewService(repo Repository, localAuthorOp LocalAuthorOperator, siteOp SiteOperator) *Service {
    return &Service{repo: repo, localAuthorOp: localAuthorOp, siteOp: siteOp}
}
```

```go
// app.go 中注入
siteAuthorRepo := siteAuthor.NewRepository(app.db)
app.SiteAuthorService = siteAuthor.NewService(siteAuthorRepo, app.LocalAuthorService, app.SiteService)
```

### 3. Nullable 参数支持绑定/解绑

**问题**：绑定方法中 `localAuthorId` 为 `int64`，无法表示"解绑"（设为 NULL）。

**修复方式**：
- Handler/Service/Repository 的 `localAuthorId` 参数改为 `*int64`
- 前端绑定层参数改为 `number | null`
- 解绑时传 `null`

```go
// Handler
func (h *Handler) UpdateBindLocalAuthor(ctx context.Context, localAuthorId *int64, siteAuthorIds []int64) *model.ApiResponse[bool]

// Service
func (s *Service) UpdateBindLocalAuthor(ctx context.Context, localAuthorId *int64, siteAuthorIds []int64) (bool, error)

// Repository
func (r *Repository) UpdateBindLocalAuthor(ctx context.Context, localAuthorId *int64, siteAuthorIds []int64) (int64, error)
```

### 4. 工具函数公共化

**问题**：多个模块中存在重复的工具函数（如 `unique`）。

**修复方式**：提取到 `internal/util/common.go`，各模块统一调用。

```go
// internal/util/common.go
func UniqueInt64(ids []int64) []int64 { ... }
```

---

## 前端修复模式

### 1. 查询参数适配 QueryAttribute 体系

**问题**：旧代码中查询参数直接赋值（如 `query.authorName = "xxx"`），新架构使用 `QueryAttribute` 包装。

**修复方式**：
- 查询参数通过 `.value` 读写
- 模板中 `v-model` 绑定改为 `xxx.value`
- `el-input` 添加 `@clear` 事件重置为 `null`
- 需要模糊搜索时设置 `operator: Operator.OpLike`

```vue
<!-- 之前 -->
<el-input v-model="searchParams.authorName" clearable />

<!-- 之后 -->
<el-input v-model="searchParams.authorName.value" clearable
          @clear="() => searchParams.authorName.value = null" />
```

```typescript
// 查询函数中设置操作符
query.authorName.operator = Operator.OpLike
```

### 2. 去除冗余类型和中间层

**问题**：Wrapper 层存在冗余接口定义（如 `PageResult`），与绑定层类型重复。

**修复方式**：
- 删除 Wrapper 中自定义的 `PageResult` 等冗余类型
- 直接使用绑定层生成的类型（`Page<T>`、`LocalAuthorDTO` 等）
- Wrapper 函数签名简化，直接接受绑定层 DTO

```typescript
// 之前
export async function localAuthorUpdateById(author: {
  id: number; authorName?: string; introduce?: string
}): Promise<ApiResponse<LocalAuthorVO>>

// 之后 — 直接使用绑定层类型
export async function localAuthorUpdateById(author: LocalAuthorDTO): Promise<ApiResponse<LocalAuthorDTO>>
```

### 3. Page 类型统一

**问题**：前端自定义了 `Page` 模型（`frontend/src/model/util/Page.ts`），与绑定层生成的 `Page` 不兼容。

**修复方式**：
- 统一使用绑定层的 `Page`：`import { Page } from "@bindings/.../pkg/model"`
- 使用 `copyPage<T>()` 工具函数进行 Page 的类型转换（保留分页信息，替换 data 类型）
- 使用 `newPage<T>()` 创建新的 Page 实例

### 4. 分页查询函数错误处理

**问题**：旧代码在查询失败时返回 `undefined`，SearchTable 无法正确处理。

**修复方式**：统一改为 `throw Error`，让 SearchTable 的错误处理机制统一捕获。

```typescript
// 之前
async function queryPageFn(page: Page<DTO>): Promise<Page<DTO> | undefined> {
  if (ApiUtil.check(response)) {
    return ApiUtil.data(response)  // 可能为 undefined
  } else {
    return undefined
  }
}

// 之后
async function queryPageFn(page: Page<DTO>): Promise<Page<DTO>> {
  if (ApiUtil.check(response)) {
    const result = ApiUtil.data<Page<DTO>>(response)
    if (isNullish(result)) {
      throw new Error('查询未返回数据')
    }
    return result
  } else {
    throw new Error(response.msg)
  }
}
```

### 5. 关联数据手动转换为 SelectItem

**问题**：旧架构后端直接返回 `SelectItem`，新架构返回完整 DTO，需前端自行转换。

**修复方式**：

```typescript
async function requestSelectItemPage(page: IPage<SelectItem>, bounded: boolean): Promise<Page<SelectItem>> {
  const response = await api.queryPage(tempPage, query)
  if (ApiUtil.check(response)) {
    const responsePage = ApiUtil.data(response)
    if (isNullish(responsePage)) throw new Error('未返回数据')

    const resultPage = copyPage<SelectItem>(responsePage)
    resultPage.data = responsePage.data.filter(notNullish).map(data => {
      return new SelectItem({
        value: String(data.id),
        label: isBlank(data.name) ? '?' : data.name,
        subLabels: [isBlank(data.relatedName) ? '?' : data.relatedName],
        extraData: undefined
      })
    })
    return resultPage
  } else {
    throw new Error(response.msg)
  }
}
```

### 6. ID 类型修正

**问题**：旧代码中 ID 混用 `string` 和 `number`。

**修复方式**：
- 统一使用 `number` 类型
- 从 SelectItem 的 `value`（string）取出时显式 `Number()` 转换
- 删除函数签名中不必要的 `string` 类型

---

## 已完成模块记录

| 模块 | 状态 | 关键修复点 |
|------|------|-----------|
| 本地作者 (LocalAuthor) | ✅ 已完成 | N+1 查询消除、依赖注入、QueryAttribute 适配、nullable 绑定/解绑 |
| 站点标签 (SiteTag) | ✅ 已完成 | N+1 查询消除、`unique` 函数公共化、Page 类型统一 |
| 本地标签 (LocalTag) | ⏳ 待确认 | — |
| 站点作者 (SiteAuthor) | ✅ 已完成 | N+1 查询消除（QueryLocalRelateDTOPage）、UpdateLastUse 批量化、跨域查询迁移到依赖注入、Wrapper 冗余类型清理、QueryAttribute 适配、Page 类型统一 |
| 作品 (Work) | ⏳ 待修复 | — |
| 任务 (Task) | ⏳ 待修复 | — |
| 资源 (Resource) | ⏳ 待修复 | — |
| 站点 (Site) | ⏳ 待修复 | — |

---

## 修复验证要点

每个模块修复完成后，需验证以下功能：

1. **分页查询**：搜索条件、分页切换、排序是否生效
2. **新增/编辑**：表单提交、数据回显
3. **删除**：单条删除、确认提示
4. **关联操作**（如适用）：绑定/解绑、ExchangeBox 双向列表查询与搜索
5. **类型安全**：无 TypeScript 编译错误，ID 类型统一为 number
