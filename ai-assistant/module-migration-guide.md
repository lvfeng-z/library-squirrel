# 模块迁移修复指南（Electron → Wails）

## 背景

项目已完成后端从 Node.js (Electron) 到 Go (Wails) 的重构，但前端模块仍保留旧的 Electron 架构模式，导致大量模块无法正常使用。每个模块需要按照统一的模式进行修复。

本文档以**本地作者模块（LocalAuthor）**和**站点作者模块（SiteAuthor）**的实际修复过程为参考，提炼出可复用的修复模式，供修复其他模块时参考。

---

## 修复清单总览

每个模块的修复通常涉及以下层次：

| 层次 | 常见问题 | 修复方向 |
|------|----------|----------|
| Repository | N+1 查询、跨域直接查询 | 删除特殊查询方法，复用通用分页 |
| Service | 关联数据逐条加载 | 批量查询 + Map 组装；依赖注入 |
| Handler | 参数类型不支持 nullable、手动 DTO→Entity 转换 | 按需改为指针类型；使用 ToXxxEntity |
| Frontend Wrapper | 冗余类型、旧 API 签名 | 简化，直接使用绑定层类型 |
| Vue 页面 | 旧查询参数结构、类型错误 | 适配 QueryAttribute、修正类型 |

---

## 后端修复模式

### 1. 消除 N+1 查询 — 从 Repository 提升到 Service

**问题**：Repository 层存在特殊查询方法（如 `QueryBoundOrUnboundToLocalAuthorPage`），在遍历结果时逐条查询关联实体。

**修复方式**：
- **删除** Repository 中的特殊查询方法
- Service 层复用通用 `repo.Page()` 获取原始实体分页
- 新增 `enrichXxx()` 方法，收集所有关联 ID → 批量查询 → Map 组装

```go
// Service 层批量填充模式（以 enrichLocalRelateDTO 为例）
func (s *Service) enrichLocalRelateDTO(ctx context.Context, rawPage *model.Page[entity.SiteAuthor]) (*model.Page[dto.SiteAuthorLocalRelateDTO], error) {
    siteAuthors := rawPage.Data
    if len(siteAuthors) == 0 {
        return model.NewPage[dto.SiteAuthorLocalRelateDTO](nil, rawPage.DataCount, rawPage.PageNumber, rawPage.PageSize), nil
    }

    // 1. 收集需要查询的关联 ID 和名称
    localAuthorIds := make([]int64, 0)
    siteIds := make([]int64, 0)
    authorNames := make([]string, 0)
    for _, author := range siteAuthors {
        if author.LocalAuthorID.Valid && author.LocalAuthorID.Int64 > 0 {
            localAuthorIds = append(localAuthorIds, author.LocalAuthorID.Int64)
        }
        if author.SiteID.Valid && author.SiteID.Int64 > 0 {
            siteIds = append(siteIds, author.SiteID.Int64)
        }
        if author.AuthorName.Valid && author.AuthorName.String != "" {
            authorNames = append(authorNames, author.AuthorName.String)
        }
    }

    // 2. 批量查询，构建 Map
    localAuthorMap := make(map[int64]*dto.LocalAuthorDTO)
    if len(localAuthorIds) > 0 {
        localAuthors, err := s.localAuthorOp.ListByIds(ctx, localAuthorIds)
        // ... 转为 Map
    }

    siteMap := make(map[int64]*dto.SiteDTO)
    if len(siteIds) > 0 {
        sites, err := s.siteOp.ListByIds(ctx, util.UniqueInt64(siteIds))
        // ... 转为 Map
    }

    // 3. 批量查询计算字段（如同名本地作者）
    sameNameMap := make(map[string]bool)
    if len(authorNames) > 0 {
        sameNameAuthors, _ := s.localAuthorOp.GetByNames(ctx, util.UniqueString(authorNames))
        for _, a := range sameNameAuthors {
            sameNameMap[a.AuthorName.String] = true
        }
    }

    // 4. 组装结果（使用组合 DTO）
    results := make([]*dto.SiteAuthorLocalRelateDTO, 0, len(siteAuthors))
    for _, author := range siteAuthors {
        relateDTO := dto.NewSiteAuthorLocalRelateDTO(author)
        if author.LocalAuthorID.Valid && author.LocalAuthorID.Int64 > 0 {
            relateDTO.LocalAuthor = localAuthorMap[author.LocalAuthorID.Int64]
        }
        if author.SiteID.Valid && author.SiteID.Int64 > 0 {
            relateDTO.Site = siteMap[author.SiteID.Int64]
        }
        relateDTO.HasSameNameLocalAuthor = sameNameMap[author.AuthorName.String]
        results = append(results, relateDTO)
    }

    return model.NewPage[dto.SiteAuthorLocalRelateDTO](results, rawPage.DataCount, rawPage.PageNumber, rawPage.PageSize), nil
}
```

### 2. 依赖注入解耦与接口扩展

**问题**：Repository 直接跨模块查询其他实体，违反职责边界。

**修复方式**：
- Service 定义所需的外部接口（如 `LocalAuthorOperator`、`SiteOperator`）
- 在 `NewService()` 中注入依赖
- `app.go` 中组装时传入对应 Service 实例
- 接口按需扩展，保持最小化原则：初始只需 `ListByIds`，后续根据业务需要逐步添加 `GetByName`、`GetByNames`、`Save` 等方法

```go
// Service 层定义接口（按需扩展）
type LocalAuthorOperator interface {
    ListByIds(ctx context.Context, ids []int64) ([]*entity.LocalAuthor, error)
    GetByName(ctx context.Context, name string) (*entity.LocalAuthor, error)
    GetByNames(ctx context.Context, names []string) ([]*entity.LocalAuthor, error)
    Save(ctx context.Context, entity *entity.LocalAuthor) (int64, error)
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
func UniqueString(items []string) []string { ... }
```

### 5. DTO 组合模式（Composition）

**问题**：复合 DTO（如 `SiteAuthorFullDTO`）显式重复定义基础 DTO 的所有字段，维护成本高且与基础 DTO 不同步。

**修复方式**：使用**组合**（命名字段引用其他 DTO），**不使用 Go 结构体嵌入**（匿名字段）。

```go
// ❌ 错误：显式重复定义字段（过时）
type SiteAuthorFullDTO struct {
    ID         int64   `json:"id"`
    AuthorName *string `json:"authorName"`
    // ... 重复基础 DTO 的所有字段
    LocalAuthor *LocalAuthorDTO `json:"localAuthor,omitempty"`
    Site        *SiteDTO        `json:"site,omitempty"`
}

// ❌ 错误：Go 结构体嵌入（匿名字段）—— Wails 绑定会扁平化 JSON
type SiteAuthorFullDTO struct {
    SiteAuthorDTO  // 匿名嵌入，JSON 会扁平化为顶层字段
    LocalAuthor *LocalAuthorDTO `json:"localAuthor,omitempty"`
}

// ✅ 正确：组合（命名字段）—— JSON 为嵌套结构
type SiteAuthorFullDTO struct {
    SiteAuthor  *SiteAuthorDTO  `json:"siteAuthor,omitempty"`
    LocalAuthor *LocalAuthorDTO `json:"localAuthor,omitempty"`
    Site        *SiteDTO        `json:"site,omitempty"`
}
```

**注意**：
- Wails 绑定生成会**扁平化** Go 匿名嵌入的 JSON，导致前端类型与后端不一致
- 组合方式的 JSON 为嵌套结构：`{"siteAuthor": {"id": 1, ...}, "localAuthor": {...}}`
- 构造函数模式：`NewSiteAuthorFullDTO(entity)` 创建并初始化 `SiteAuthor` 字段，关联字段由 Service 层填充

### 6. 批量 UPDATE 优化

**问题**：更新多条记录的同一字段时逐条执行 UPDATE SQL，产生 N 次数据库操作。

**修复方式**：在 Repository 新增批量 UPDATE 方法，单条 SQL 完成。

```go
// Repository
func (r *Repository) UpdateLastUseByIds(ctx context.Context, ids []int64, lastUse int64) error {
    if len(ids) == 0 {
        return nil
    }
    return r.GORM().WithContext(ctx).Model(new(entity.SiteAuthor)).
        Where("id IN ?", ids).
        Update("last_use", lastUse).Error
}

// Service
func (s *Service) UpdateLastUse(ctx context.Context, ids []int64) error {
    now := util.GetCurrentTimestamp()
    return s.repo.UpdateLastUseByIds(ctx, ids, now)
}
```

### 7. 返回类型对齐前端需求

**问题**：查询方法的返回 DTO 缺少前端需要的计算字段（如 `hasSameNameLocalAuthor`），导致前端功能不完整。

**修复方式**：
- 新增或使用包含计算字段的 DTO 作为返回类型
- 在 Service 层的 enrich 方法中计算并填充这些字段
- Handler 方法签名同步更新

```go
// Handler 返回类型改为含计算字段的 DTO
func (h *Handler) QueryBoundOrUnboundToLocalAuthorPage(
    ctx context.Context, page *model.Page[dto.SiteAuthorLocalRelateDTO], query SiteAuthorQueryDTO,
) *model.ApiResponse[model.Page[dto.SiteAuthorLocalRelateDTO]]

// Service 层 enrich 方法中计算
func (s *Service) enrichLocalRelateDTO(...) {
    // 批量查询同名本地作者
    sameNameMap := make(map[string]bool)
    if len(authorNames) > 0 {
        sameNameAuthors, _ := s.localAuthorOp.GetByNames(ctx, util.UniqueString(authorNames))
        for _, a := range sameNameAuthors {
            sameNameMap[a.AuthorName.String] = true
        }
    }
    // 填充计算字段
    relateDTO.HasSameNameLocalAuthor = sameNameMap[author.AuthorName.String]
}
```

### 8. 重构后死代码清理

**问题**：重构后旧的 enrich 方法不再被调用，但仍留在代码中。

**修复方式**：确认无调用方后直接删除，不要保留 "以防万一" 的代码。

### 9. Handler 使用 ToXxxEntity 转换函数

**问题**：Handler 的 Save/Update 方法中手动逐字段将 DTO 转换为 Entity，产生大量样板代码（每个方法 15+ 行重复的 `if field != nil { entity.Field.Valid = true; entity.Field.String = *field }`）。

**修复方式**：使用 DTO 包中已有的 `ToXxxEntity()` 转换函数，一行代码完成。

```go
// ❌ 之前：手动逐字段转换（15+ 行）
func (h *Handler) Save(ctx context.Context, tag *dto.SiteTagDTO) *model.ApiResponse[int64] {
    domainTag := &entity.SiteTag{BaseEntity: &model.BaseEntity{}}
    if tag.SiteID != nil {
        domainTag.SiteID.Valid = true
        domainTag.SiteID.Int64 = *tag.SiteID
    }
    if tag.SiteTagName != nil {
        domainTag.SiteTagName.Valid = true
        domainTag.SiteTagName.String = *tag.SiteTagName
    }
    // ... 每个字段都重复这个模式
}

// ✅ 之后：一行搞定
func (h *Handler) Save(ctx context.Context, tag *dto.SiteTagDTO) *model.ApiResponse[int64] {
    domainTag := dto.ToSiteTagEntity(tag)
    // ...
}
```

### 10. Entity 使用 NewXxx() 工厂方法

**问题**：代码中使用 `&entity.LocalAuthor{AuthorName: xxx}` 结构体字面量创建实体，可能遗漏 `BaseEntity` 等必要字段的初始化。

**修复方式**：使用实体包提供的 `NewXxx()` 工厂方法创建实例，再赋值业务字段。

```go
// ❌ 之前
newLocalAuthor := &entity.LocalAuthor{
    AuthorName: siteAuthor.AuthorName,
    Introduce:  siteAuthor.Introduce,
}

// ✅ 之后
newLocalAuthor := entity.NewLocalAuthor()
newLocalAuthor.AuthorName = siteAuthor.AuthorName
newLocalAuthor.Introduce = siteAuthor.Introduce
```

### 11. 移除冗余查询字段

**问题**：QueryDTO 中存在语义重复的字段（如 `SiteName` 精确匹配和 `SiteNameStr` 模糊匹配），增加维护成本且容易误用。

**修复方式**：保留一个字段，通过 QueryAttribute 的 `operator` 属性控制匹配方式。删除冗余字段后需重新生成 Wails 绑定。

```go
// ❌ 之前：两个字段映射同一个数据库列
type SiteQueryDTO struct {
    SiteName    query.QueryAttribute[string] `json:"siteName" query:"site_name"`    // 精确匹配
    SiteNameStr query.QueryAttribute[string] `json:"siteNameStr" query:"site_name"` // 模糊匹配
}

// ✅ 之后：一个字段，通过 operator 区分
type SiteQueryDTO struct {
    SiteName query.QueryAttribute[string] `json:"siteName" query:"site_name"` // 通过 operator 控制匹配方式
}
```

```typescript
// 前端使用时设置 operator
query.siteName.operator = Operator.OpLike  // 模糊匹配
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

### 2. 去除冗余类型和中间层 + Wrapper 响应校验封装 + API 整合

**问题**：
- Wrapper 层存在冗余接口定义（如 `PageResult`），与绑定层类型重复
- 部分 Wrapper 函数接受匿名对象参数再手动构造 DTO
- Wails 绑定层返回 `ApiResponse<T | null> | null`（双层可空），调用方需重复做 null 检查
- 部分 API 适配器函数散落在单独的 `XxxApi.ts` 文件中，与 Wrapper 职责重叠

**修复方式**：
- 删除 Wrapper 中自定义的 `PageResult` 等冗余类型
- 直接使用绑定层生成的类型（`Page<T>`、`LocalAuthorDTO` 等）
- Wrapper 函数签名简化，直接接受绑定层 DTO 并透传给 Handler
- 使用公共 `requireResponse<T>` 函数封装 Wails 绑定层的响应校验，将 `ApiResponse<T | null> | null` 统一转换为 `ApiResult<T>`
- `requireResponse` 定义在 `frontend/src/apis/http/types.ts`，各 Wrapper 通过 `import { requireResponse } from '@renderer/apis/http/types'` 导入使用
- 将散落的 `XxxApi.ts` 适配器函数整合进 Wrapper 文件，通过 `http/index.ts` 统一导出

**ApiResult 类型定义**（`frontend/src/apis/http/types.ts`）：

Wails 生成的 `ApiResponse<T>` 的 `data` 为 `data?: T`（可 undefined），即使校验了外层 null 和 success，调用方仍需检查 `response.data`。因此定义前端独有的 `ApiResult<T>` 类型，`data` 保证非空：

```typescript
// Wails 生成的类型（data 可为 undefined）
// interface ApiResponse<T> { success: boolean; msg: string; data?: T }

// 前端定义的校验后类型（data 保证非空）
export interface ApiResult<T = unknown> {
  readonly success: true       // 字面量类型，校验通过后不会是 false
  readonly msg: string
  readonly data: T             // 非可选，保证非空
}
```

**requireResponse 公共校验函数**（`frontend/src/apis/http/types.ts`）：

```typescript
/**
 * @param requireData 是否校验 data 非空，默认 true
 *   - 查询类接口（QueryPage, GetById, List）：默认 true，校验 data 非空
 *   - 变更类接口（Save, Update, Delete）：传 false，不校验 data（成功时 data 可能为 null）
 */
export function requireResponse<T>(
  response: ApiResponse<T | null> | null,
  operation: string,
  requireData = true
): ApiResult<T> {
  if (!response) throw new Error(`${operation}：接口返回为空`)
  if (!response.success) throw new Error(response.msg || `${operation}：操作失败`)
  if (requireData && isNullish(response.data)) throw new Error(`${operation}：未返回数据`)
  return response as unknown as ApiResult<T>
}
```

校验失败抛出 Error，调用方通过 try/catch 捕获。

**Wrapper 方法简化为单行**：

```typescript
// ❌ 之前：每个方法都重复 null 检查
export async function localTagSave(tag: LocalTagDTO): Promise<ApiResponse<number>> {
  const result = await LocalTagHandler.Save(tag)
  if (!result) return { success: false, msg: '保存失败：接口返回为空' }
  if (!result.success) return { success: false, msg: result.msg ?? '保存失败' }
  return { success: true, msg: result.msg ?? '', data: ... }
}

// ✅ 之后：查询类接口（默认校验 data）
export async function localTagQueryPage(page, query): Promise<ApiResult<Page<LocalTagDTO>>> {
  return requireResponse(await LocalTagHandler.QueryPage(page, query), '查询本地标签')
}

// ✅ 之后：变更类接口（requireData=false，不校验 data）
export async function localTagSave(tag: LocalTagDTO): Promise<ApiResult<number>> {
  return requireResponse(await LocalTagHandler.Save(tag), '保存本地标签', false)
}
```

**调用方适配**：

```typescript
// 变更操作（Save/Update/Delete）：try/catch + ApiUtil.msg
try {
  const response = await localTagApi.localTagSave(tagDTO)
  ApiUtil.msg(response)    // ApiResult 结构兼容 ApiResponse，可直接传入
  emits('requestSuccess')
  state.value = false
} catch (e) {
  ElMessage.error((e as Error).message)
}

// 查询操作：直接取 data，类型为 T（不再是 T | undefined）
const response = await localTagApi.localTagQueryWithBaseTagPage(page, query)
return response.data  // 无需 null/undefined 检查，类型推断为 Page<LocalTagWithBaseTagDTO>
```

**API 整合模式**（将散落的适配器合并到 Wrapper）：

```typescript
// 之前：LocalTagApi.ts 中单独定义适配器
// frontend/src/apis/LocalTagApi.ts
export async function localTagQuerySelectItemPageByName(...) { ... }

// 之后：整合到 Wrapper 文件，统一导出
// frontend/src/apis/http/wrappers/localTag.ts
export async function localTagQuerySelectItemPageByName(
  page: IPage<SelectItem>, input: string
): Promise<IPage<SelectItem>> {
  // 适配器直接使用 wrapper 方法，response.data 已保证非空，无需额外校验
  const response = await localTagQuerySelectItemPage(pageObj, queryDTO, '')
  return response.data
}

// frontend/src/apis/http/index.ts 统一导出
export * as localTagApi from './wrappers/localTag'
export { localTagQuerySelectItemPageByName } from './wrappers/localTag'  // 适配器单独导出
```

### 3. Page 类型统一

**问题**：前端自定义了 `Page` 模型（`frontend/src/model/util/Page.ts`），与绑定层生成的 `Page` 不兼容。

**修复方式**：
- 统一使用绑定层的 `Page`：`import { Page } from "@bindings/.../pkg/model"`
- 使用 `copyPage<T>()` 工具函数进行 Page 的类型转换（保留分页信息，替换 data 类型）
- 使用 `newPage<T>()` 创建新的 Page 实例

### 4. 统一错误处理

**问题**：旧代码在查询失败时返回 `undefined`（静默失败），SearchTable 和 SelectItemPage 适配器无法正确处理。

**修复方式**：所有异步查询函数（SearchTable queryPageFn、Dialog SelectItemPage 适配器等）统一使用 `ApiUtil.check` + `throw Error` 模式，不返回 `undefined`。

```typescript
// ❌ 之前：返回 undefined（SearchTable queryPageFn）
async function queryPageFn(page: Page<DTO>): Promise<Page<DTO> | undefined> {
  if (ApiUtil.check(response)) {
    return ApiUtil.data(response)  // 可能为 undefined
  } else {
    return undefined
  }
}

// ❌ 之前：静默失败（SelectItemPage 适配器）
async function siteQuerySelectItemPageAdapter(page: IPage<SelectItem>, _input: string): Promise<IPage<SelectItem>> {
  const response = await siteApi.siteQuerySelectItemPage({ page: page.pageNumber, pageSize: page.pageSize })
  if (!response.success || !response.data) {
    return new Page<SelectItem>()  // 静默失败
  }
  return { /* 手动映射字段 */ }
}

// ✅ 之后：统一 throw Error（适用于所有异步查询函数）
async function queryPageFn(page: Page<DTO>): Promise<Page<DTO>> {
  const response = await api.query(page, query)
  if (ApiUtil.check(response)) {
    const result = ApiUtil.data<Page<DTO>>(response)
    if (isNullish(result)) throw new Error('查询未返回数据')
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

### 7. 组合 DTO 的嵌套路径适配

**问题**：后端使用组合 DTO 后，JSON 变为嵌套结构（`{siteAuthor: {id, authorName}, localAuthor: {...}}`），前端 DataTable 的 key 需要对应调整。

**修复方式**：

**DataTable thead** 的 key 使用点号嵌套路径，DataTable 的 `getPropByPath`/`setPropByPath` 已支持：

```typescript
// thead key 使用嵌套路径
const thead: Thead<SiteAuthorLocalRelateDTO>[] = [
  new Thead({ key: 'siteAuthor.authorName', title: '名称' }),
  new Thead({ key: 'siteAuthor.introduce', title: '介绍' }),
  new Thead({ key: 'siteAuthor.updateTime', title: '修改时间' }),
]

// data-key 也使用嵌套路径
// <search-table data-key="siteAuthor.id" />
```

**关联数据的 getCacheData/setCacheData** 操作嵌套对象：

```typescript
getCacheData: (rowData: SiteAuthorLocalRelateDTO) => {
    if (isNullish(rowData.localAuthor?.id)) return undefined
    return new SelectItem({ value: rowData.localAuthor.id, label: rowData.localAuthor.authorName })
},
setCacheData: (rowData: SiteAuthorLocalRelateDTO, data: SelectItem) => {
    if (isNullish(rowData.localAuthor)) rowData.localAuthor = new LocalAuthorDTO()
    rowData.localAuthor.id = Number(data.value)
    rowData.localAuthor.authorName = data.label
}
```

### 8. Dialog 适配

**问题**：弹窗组件需要适配嵌套的组合 DTO 结构；部分 Dialog 使用旧的实体类型或 `computed` 中间层。

**修复方式**：

**类型统一**：Dialog 的 `formData` 使用绑定层 DTO（如 `LocalAuthorDTO`），不使用旧实体类型（如 `LocalAuthor`）。

```typescript
// ❌ 之前：使用旧实体类型，保存时需手动构造
const formData = defineModel<LocalAuthor>('formData', { required: true })
const response = await apis.localAuthorUpdateById({
  id: tempFormData.id ?? 0,
  authorName: tempFormData.authorName ?? undefined
})

// ✅ 之后：使用绑定层 DTO，简单 DTO 直接透传
const formData = defineModel<LocalAuthorDTO>('formData', { required: true })
const response = await apis.localAuthorUpdateById(tempFormData)
```

**初始化**：在构造函数中一步完成嵌套对象的预创建，避免模板中的空值问题。

```typescript
// ✅ 推荐：构造函数中一步初始化
dialogData.value = new SiteTagLocalRelateDTO({ siteTag: new SiteTagDTO(), site: new SiteDTO() })

// 也可以分步初始化（效果相同）
dialogData.value = new SiteAuthorLocalRelateDTO()
dialogData.value.siteAuthor = new SiteAuthorDTO()
```

**模板绑定**：初始化后直接使用 `formData.siteTag.xxx` 绑定，无需 `computed` 中间层。是否需要 `!` 非空断言取决于 Wails 生成的属性类型——若属性类型为 `T | null` 则需要 `!`，若为 `T` 则不需要。

```vue
<!-- 属性类型为 SiteTagDTO | null 时需要 ! -->
<el-input v-model="formData.siteTag!.siteTagName"></el-input>

<!-- 属性类型为 SiteTagDTO 时不需要 ! -->
<el-input v-model="formData.siteTag.siteTagName"></el-input>
```

```typescript
// ❌ 之前：computed 包装（不必要）
const siteTagRef = computed<SiteTagDTO>({
  get() { return formData.value.siteTag ?? new SiteTagDTO() },
  set(val) { formData.value.siteTag = val }
})
// 模板中使用 siteTagRef.siteTagName

// ✅ 之后：直接绑定（初始化时预创建嵌套对象）
// <el-input v-model="formData.siteTag.siteTagName">
```

**保存时区分 DTO 类型**：
- **简单 DTO**（Dialog formData 本身就是目标 DTO）：直接透传
- **组合 DTO**（需要从嵌套结构中提取子 DTO）：构造新的基础 DTO 传入

```typescript
// 简单 DTO：直接透传
const response = await apis.localAuthorUpdateById(tempFormData)

// 组合 DTO：提取子字段构造新 DTO
const authorDTO = new SiteAuthorDTO({
    id: tempFormData.siteAuthor?.id,
    authorName: tempFormData.siteAuthor?.authorName || null,
    introduce: tempFormData.siteAuthor?.introduce || null,
    localAuthorId: tempFormData.localAuthor?.id || null,
})
const response = await apis.siteAuthorUpdateById(authorDTO)
```

---

## 已完成模块记录

| 模块 | 状态 | 关键修复点 |
|------|------|-----------|
| 本地作者 (LocalAuthor) | ✅ 已完成 | N+1 查询消除、依赖注入、QueryAttribute 适配、nullable 绑定/解绑、Dialog 类型迁移到 LocalAuthorDTO、Wrapper requireResponse+ApiResult 重构、LocalAuthorApi.ts 整合到 Wrapper、适配器导出 |
| 站点标签 (SiteTag) | ✅ 已完成 | N+1 查询消除、`unique` 函数公共化、Page 类型统一、Handler 使用 ToSiteTagEntity、Dialog 去掉 computed 中间层、Wrapper requireResponse+ApiResult 重构（移除 SiteTagVO/PageResult）、统一错误处理、嵌套对象预初始化 |
| 本地标签 (LocalTag) | ✅ 已完成 | DTO 组合重构（LocalTagWithBaseTagDTO）、QueryWithBaseTagPage LEFT JOIN 简化结果映射、Wrapper requireResponse 响应校验封装、API 适配器整合到 Wrapper、Dialog 嵌套绑定适配、组合 DTO 嵌套路径适配 |
| 站点作者 (SiteAuthor) | ✅ 已完成 | N+1 查询消除（QueryLocalRelateDTOPage）、UpdateLastUse 批量化、跨域查询迁移到依赖注入、Wrapper 冗余类型清理、QueryAttribute 适配、Page 类型统一、DTO 组合重构、返回类型对齐前端需求（hasSameNameLocalAuthor）、嵌套路径适配、DI 接口扩展、Handler 使用 ToSiteAuthorEntity、Entity 使用 NewLocalAuthor 工厂方法 |
| 作品 (Work) | ⏳ 待修复 | — |
| 任务 (Task) | ⏳ 待修复 | — |
| 资源 (Resource) | ⏳ 待修复 | — |
| 站点 (Site) | ⏳ 部分完成 | 移除冗余查询字段（SiteNameStr）、移除未使用的 API 函数 |

---

## 修复验证要点

每个模块修复完成后，需验证以下功能：

1. **分页查询**：搜索条件、分页切换、排序是否生效
2. **新增/编辑**：表单提交、数据回显
3. **删除**：单条删除、确认提示
4. **关联操作**（如适用）：绑定/解绑、ExchangeBox 双向列表查询与搜索
5. **类型安全**：无 TypeScript 编译错误，ID 类型统一为 number
