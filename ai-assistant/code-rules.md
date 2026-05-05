# LibrarySquirrel 代码规则与约定

## 概述

本文档记录 LibrarySquirrel 项目的代码编写规则、命名约定、文件组织和开发规范。遵循这些规则有助于保持代码一致性、可维护性和团队协作效率。

## 文件命名规范

### 目录结构约定

- **backend/** - 后端代码 (Go)
  - `{module}/` - 业务模块（如 localTag、work）
    - `handler.go` - Handler（Wails Bind）
    - `service.go` - 业务逻辑
    - `repository.go` - 数据访问实现
    - `model.go` - 领域实体
    - `query.go` - 查询DTO
  - `database/` - 数据库基础设施
    - `db.go` - 数据库连接
    - `transaction.go` - 事务封装
    - `resources/` - SQL 迁移文件
  - `model/` - 领域模型
  - `base/` - 基础设施
    - `model/` - 共享 DTO（ApiResponse等）
- **frontend/src/** - 前端代码 (Vue 3)
  - `components/` - Vue 组件
  - `store/` - Pinia 状态管理
  - `model/` - 前端类型定义
  - `utils/` - 工具函数
  - `apis/` - API 包装器
- **plugin/** - 插件目录

### 文件命名规则

| 文件类型                 | 命名规则                    | 示例                                        |
| ------------------------ | --------------------------- | ------------------------------------------- |
| **Go 源文件**            | snake_case + `.go`          | `handler.go`, `work_service.go`             |
| **Go 结构体/接口**       | PascalCase                  | `Handler`, `WorkService`                    |
| **Vue 组件**             | PascalCase + `.vue`         | `WorkCard.vue`, `MainLayout.vue`            |
| **TypeScript 类型/接口** | PascalCase + `.ts`          | `ApiResponse.ts`, `Page.ts`                 |
| **前端工具函数**         | PascalCase + `.ts`          | `treeUtil.ts`, `apiUtil.ts`                 |
| **前端常量文件**         | camelCase + `.ts`           | `httpStatus.ts`                             |
| **配置文件**             | kebab-case + `.json`/`.yml` | `tsconfig.json`, `create-data-table.yml`    |

## 代码风格规范

### 注释规范

#### 基本原则

- **只描述目的和约束**: 注释应说明函数/方法的目的、用途和使用约束，而不是描述实现方式
- **禁止变更描述**: 注释中禁止使用"改为"、"修改为"、"重构"、"优化"等描述变更的词汇

#### 错误示例

```typescript
// 改为调用外部注入的方法  ← 禁止：描述变更
// 重构这个方法实现  ← 禁止：描述变更
// 修改为异步函数  ← 禁止：描述变更
// 优化性能问题  ← 禁止：描述变更
```

#### 正确示例

```typescript
/**
 * 批量关联作品到作品集
 * @param workIds 作品id列表
 * @param workSetId 作品集id
 * @throws {string} 当作品已存在于作品集中时抛出错误信息
 */

/**
 * 查询标签选择列表
 * @param page 分页参数
 * @param input 搜索关键字
 * @returns 包含选择项的分页数据
 */

/**
 * 作品查询函数 - 接收分页参数，返回作品分页结果
 * @param page 包含查询条件的分页对象
 * @returns 作品分页结果
 */
```

### 命名规范

#### 禁止方法名与 prop 同名

- **原则**：组件内部方法名不得与 props 中定义的属性名相同
- **原因**：避免变量遮蔽和潜在的代码混淆问题
- **解决方案**：使用动词前缀区分方法与属性

#### 方法命名模式

使用明确的前缀来区分方法与 props 属性：

| 前缀        | 用途           | 示例                               |
| ----------- | -------------- | ---------------------------------- |
| `handleXxx` | 处理事件或回调 | `handleSubmit`, `handleChange`     |
| `doXxx`     | 执行操作       | `doFetch`, `doSearch`              |
| `buildXxx`  | 构建或组装数据 | `buildConditions`, `buildQuery`    |
| `loadXxx`   | 加载数据       | `loadData`, `loadItems`            |
| `checkXxx`  | 检查或验证     | `checkPermission`, `validateInput` |

#### 正确示例

```typescript
// props 中定义了 fetchWorkPage
const props = defineProps<{
  fetchWorkPage: (page: Page) => Promise<Page>
}>()

// 方法使用 doFetchWorkPage 而不是 fetchWorkPage
async function doFetchWorkPage(page: Page) {
  // 实现
}
```

#### 错误示例

```typescript
// 方法名与 prop 同名 - 禁止
async function fetchWorkPage(page: Page) {
  // 这会导致变量遮蔽
}
```

#### Vue 组件特有规则

- Props 中的函数属性通常使用名词命名（如 `fetchWorkPage`）
- 组件内部方法应使用动词前缀（如 `doFetchWorkPage`）
- 事件处理统一使用 `handle` 前缀（如 `handleClick`）

### TypeScript 规范

- **模块解析**: 使用 `bundler`
- **路径别名**:
  - `@renderer/*` → `frontend/src/*`（前端专用）
  - `@shared/*` → `frontend/src/model/*`（共用类型）
- **类型定义**: 优先使用 `interface` 定义对象结构，`type` 用于联合类型或工具类型
- **空值处理**: 使用可选链 `?.` 和空值合并 `??` 运算符
- **严格模式**: 启用 TypeScript 严格模式 (`strict: true`)

### 空值判断规范

#### 1. 必须使用语义化空值判断函数

**原则**: 判断变量是否为 `undefined` 或 `null` 时，必须使用 `@shared/util/CommonUtil.ts` 中定义的 `NotNullish` 和 `IsNullish` 函数，避免使用"逻辑非"语法（`!value`），以保证语义清晰。

**函数定义**:

```typescript
// 判断值是否不为 null 或 undefined
export function NotNullish<T>(value: T | undefined | null): value is T

// 判断值是否为 null 或 undefined
export function IsNullish(value: unknown): value is undefined | null
```

**导入方式**:

```typescript
import { NotNullish, IsNullish } from '@shared/util/CommonUtil.ts'
```

**✅ 正确示例**:

```typescript
import { NotNullish, IsNullish } from '@shared/util/CommonUtil.ts'

// 使用 NotNullish 判断非空
if (NotNullish(user)) {
  // TypeScript 会自动推断 user 不为 null 或 undefined
  console.log(user.name)
}

// 使用 IsNullish 判断为空
if (IsNullish(data)) {
  return
}

// 结合可选链和空值合并
const name = NotNullish(user) ? user.name : '默认值'

// 在数组过滤中使用
const validItems = items.filter(NotNullish)

// 在条件断言中使用
assertIsDefined(value: T): asserts value is T {
  if (IsNullish(value)) {
    throw new Error('Value is required')
  }
}
```

**❌ 错误示例**:

```typescript
// 禁止使用逻辑非判断空值（语义不清晰）
if (!value) {
  // ✗ 这会过滤掉 falsy 值（如 0, '', false）
  return
}

// 禁止使用 != null 判断（会同时过滤 undefined 和 null，但语义不明确）
if (value != null) {
  // ✗ 语义不清晰，不知道是判断非空还是其他意图
  // 处理
}

// 禁止使用双重否定（语义不清晰）
if (!!value) {
  // ✗
  // 处理
}
```

**🛠️ TypeScript 类型守卫优势**:

使用 `NotNullish` 和 `IsNullish` 作为类型守卫函数，TypeScript 可以在条件分支中自动收窄类型：

```typescript
function processValue(value: string | undefined | null) {
  if (IsNullish(value)) {
    // TypeScript 自动推断 value 为 undefined | null
    return
  }
  // TypeScript 自动推断 value 为 string（类型收窄）
  console.log(value.toUpperCase())
}
```

#### 2. 数组非空判断

**原则**: 判断数组是否为空时，必须使用 `@shared/util/CommonUtil.ts` 中定义的 `ArrayNotEmpty` 和 `ArrayIsEmpty` 函数。

**函数定义**:

```typescript
// 判断数组是否不为空
export function ArrayNotEmpty(value: unknown): value is unknown[]

// 判断数组是否为空
export function ArrayIsEmpty(value: unknown): value is null | undefined | []
```

**✅ 正确示例**:

```typescript
import { ArrayNotEmpty, ArrayIsEmpty } from '@shared/util/CommonUtil.ts'

// 判断数组非空
if (ArrayNotEmpty(items)) {
  // TypeScript 自动推断 items 为非空数组
  console.log(items.length)
}

// 判断数组为空
if (ArrayIsEmpty(items)) {
  return
}

// 在条件中使用
const hasItems = ArrayNotEmpty(items)
```

**❌ 错误示例**:

```typescript
// 禁止使用 length 属性直接判断
if (items.length) {
  // ✗ 语义不清晰
  // 处理
}

if (items && items.length > 0) {
  // ✗ 冗长且语义不明确
  // 处理
}
```

#### 3. 字符串空白判断

**原则**: 判断字符串是否为空白时，必须使用 `@shared/util/StringUtil.ts` 中定义的 `isBlank` 和 `isNotBlank` 函数。

**函数定义**:

```typescript
// 判断字符串是否为空白（null、undefined 或只包含空白字符）
export function isBlank(input: string | null | undefined): input is undefined | null | ''

// 判断字符串是否不为空白
export function isNotBlank(input: string | null | undefined): input is string
```

**✅ 正确示例**:

```typescript
import { isBlank, isNotBlank } from '@shared/util/StringUtil.ts'

// 判断字符串为空
if (isBlank(input)) {
  return
}

// 判断字符串非空
if (isNotBlank(input)) {
  // TypeScript 自动推断 input 为 string
  console.log(input.trim())
}

// 在条件分支中类型收窄
function process(input: string | undefined | null) {
  if (isBlank(input)) {
    return
  }
  // TypeScript 自动推断 input 为 string
  console.log(input.length)
}
```

**❌ 错误示例**:

```typescript
// 禁止使用逻辑非判断空字符串
if (!input) {
  // ✗ 会过滤掉 falsy 值
  return
}

// 禁止使用 length 判断
if (input && input.length === 0) {
  // ✗ 没有判断空白字符
  return
}
```

#### 4. 适用场景汇总

| 场景                          | 推荐方式                         | 导入来源                     |
| ----------------------------- | -------------------------------- | ---------------------------- |
| 判断值是否为 null/undefined   | `IsNullish(value)`               | `@shared/util/CommonUtil.ts` |
| 判断值是否不为 null/undefined | `NotNullish(value)`              | `@shared/util/CommonUtil.ts` |
| 判断数组是否为空              | `ArrayIsEmpty(value)`            | `@shared/util/CommonUtil.ts` |
| 判断数组是否不为空            | `ArrayNotEmpty(value)`           | `@shared/util/CommonUtil.ts` |
| 过滤数组中的 null/undefined   | `array.filter(NotNullish)`       | `@shared/util/CommonUtil.ts` |
| 判断字符串是否为空白          | `isBlank(value)`                 | `@shared/util/StringUtil.ts` |
| 判断字符串是否不为空白        | `isNotBlank(value)`              | `@shared/util/StringUtil.ts` |
| 条件分支中的类型收窄          | `if (NotNullish(value)) { ... }` | 对应工具类                   |

**⚠️ 尽量避免**:

```typescript
// 尽量避免简单的逻辑非
if (!value) { ... }
if (!items.length) { ... }
if (!name) { ... }
```

#### 6. 例外情况

以下情况可使用逻辑非：

- 判断值为 falsy（如 `0`, `''`, `false`）而非仅 null/undefined
- 布尔值取反（如 `!isLoading`）
- 复杂布尔表达式（如 `!value || !value.enabled`）

### Vue 组件规范

- **语法**: 使用 `<script setup lang="ts">` 组合式 API
- **Props 定义**: Props 接口使用 `Props` 后缀
  ```typescript
  interface WorkCardProps {
    work: WorkDTO
    showActions?: boolean
  }
  ```
- **Emits 定义**: 使用 `defineEmits` 和 TypeScript 字面量类型
- **组件导入**: 使用 `@renderer/components/...` 路径别名导入组件
- **样式选择器**: 尽可能使用类选择器 (`.class-name`) 设置样式，避免使用元素选择器 (`div`, `span`) 和 ID 选择器 (`#id`)，仅在必要时使用style属性，以提高样式复用性和可维护性

### 命名约定

| 元素类型      | 命名规则                     | 示例                                   |
| ------------- | ---------------------------- | -------------------------------------- |
| **类名**      | PascalCase                   | `WorkService`, `BaseDao`               |
| **接口/类型** | PascalCase                   | `WorkDTO`, `TaskStatus`                |
| **变量/函数** | camelCase                    | `workList`, `getWorkById`              |
| **常量**      | UPPER_SNAKE_CASE             | `MAX_RETRY_COUNT`, `DEFAULT_PAGE_SIZE` |
| **私有成员**  | 前缀 `_` (可选)              | `_internalCache`, `_privateMethod()`   |
| **布尔变量**  | 使用 `is`, `has`, `can` 前缀 | `isLoading`, `hasError`, `canEdit`     |

## 开发约定

### IPC 通信模式

- **方法命名**: Wails 自动绑定，无需手动注册
- **Go 主进程注册** (使用 Wails `Bind`):
  ```go
  // wails.go 中定义
  type App struct {
    workService *service.WorkService
  }
  // Wails 自动将方法绑定到 window.api
  ```
- **前端调用**:
  ```typescript
  const response = await window.api.workServiceGetById(workId)
  ```

### 响应处理

- **Go 主进程**: 直接返回数据或 error，Wails 自动序列化
- **前端处理**:
  ```typescript
  try {
    const result = await window.api.someMethod(args)
    // result 是 Go 端直接返回的数据
  } catch (error) {
    // error 是 Go 端返回的 error
  }
  ```

### DTO 设计规范

**核心原则**：禁止在 DTO 中直接嵌入 Entity（领域实体），允许组合其他 DTO 类型。

**规则名称**：DTO_COMPOSITION_OVER_EMBEDDING
**优先级**：P0（最高）
**适用范围**：所有 DTO、DTO 构造器函数的实现

#### 1. 禁止直接嵌入 Entity

**❌ 禁止模式**：

```go
// 错误：直接嵌入 Entity（领域实体）
type SiteTagFullDTO struct {
    *SiteTag              // ✗ 禁止：嵌入 Entity
}
```

```typescript
// 错误：直接嵌入 Entity
class UserResponseDTO {
  user: User  // ✗ 禁止
  token: string
}
```

**问题**：
- DTO 与 Entity 耦合过紧，实体变更会直接影响 DTO
- 可能意外暴露实体中的敏感字段（如 password、salt）
- 无法显式控制字段的 JSON tag

#### 2. 允许组合其他 DTO

**✅ 允许模式**：

```go
// 正确：组合其他 DTO 类型
type SiteTagFullDTO struct {
    ID         int64        `json:"id"`
    CreateTime int64        `json:"createTime"`
    UpdateTime int64        `json:"updateTime"`
    SiteID     int64        `json:"siteId"`
    SiteTagID  string       `json:"siteTagId"`
    SiteTagName string      `json:"siteTagName"`
    // 组合其他 DTO（允许）
    LocalTag *LocalTagDTO   `json:"localTag,omitempty"`
    Site     *SiteDTO       `json:"site,omitempty"`
}
```

```typescript
// 正确：组合其他 DTO
class UserResponseDTO {
  id: number
  username: string
  email: string
  token: string
  // 组合其他 DTO（允许）
  address: AddressDTO
}
```

**优点**：
- DTO 之间解耦，各自独立演进
- 显式控制字段的 JSON tag
- 便于组合复杂业务场景

#### 3. 具体执行标准

**✅ 推荐模式**：

- 显式定义：DTO 应包含独立的、基础类型的字段（如 `id: number`, `username: string`）
- 按需裁剪：只复制业务场景真正需要的字段，过滤敏感字段
- 语义重命名：如果前端需要的字段名与数据库不一致，在 DTO 中定义符合前端语义的字段名
- 组合优于嵌入：嵌套对象必须是另一个 DTO，不能是 Entity

**🛠️ 映射要求**：

当实体与 DTO 字段较多时，必须生成或使用映射代码（推荐手动 `convert()` 方法），严禁省略映射步骤。

#### 4. 理由与约束

| 原因 | 说明 |
|------|------|
| **安全性** | 防止实体类新增敏感字段而意外泄露 |
| **解耦** | API 契约独立于内部数据库模型 |
| **显式控制** | 显式定义字段，便于控制 JSON 序列化 |

#### 5. 适用场景汇总

| 场景 | 推荐方式 |
|------|----------|
| 基础字段 | 显式定义所有字段 |
| 关联其他 DTO | 组合（composition）其他 DTO |
| 关联 Entity | ✗ 禁止，必须先转换为 DTO |

#### 6. DTO 组合禁止匿名嵌入

**规则名称**: `DTO_COMPOSITION_NO_EMBEDDING`（补充 `DTO_COMPOSITION_OVER_EMBEDDING`）
**优先级**: P0（最高）

复用其他 DTO 时禁止使用 Go 匿名嵌入（anonymous embedding），必须使用命名字段（组合方式）。

**原因**：Wails 绑定生成会**扁平化** Go 匿名嵌入的 JSON tag，导致前端类型结构与后端不一致。组合方式的 JSON 为嵌套结构，前后端一致。

**❌ 禁止模式**：

```go
// 禁止：Go 匿名嵌入，JSON 被扁平化为顶层字段
type FullDTO struct {
    TaskDTO  // JSON: taskName, siteId... 全部提升到顶层
}
```

**✅ 正确模式**：

```go
// 正确：命名字段组合，JSON 保持嵌套结构
type TaskProgressDTO struct {
    Task     *TaskDTO `json:"task,omitempty"`
    Total    *int64   `json:"total,omitempty"`
    Finished *int64   `json:"finished,omitempty"`
}

type TaskProgressTreeDTO struct {
    TaskProgress *TaskProgressDTO       `json:"taskProgress,omitempty"`
    Children     []*TaskProgressTreeDTO `json:"children,omitempty"`
    HasChildren  *bool                  `json:"hasChildren,omitempty"`
    IsLeaf       *bool                  `json:"isLeaf,omitempty"`
}
```

**参考示例**：`SiteTagLocalRelateDTO` 使用 `SiteTag *SiteTagDTO`、`LocalTag *LocalTagDTO` 命名字段组合。

### DTO 转换规范

**规则名称**: `DTO_USE_TO_ENTITY`
**优先级**: P1

所有 DTO 全量属性转换到 Entity 的逻辑都应使用 `ToXxxEntity()` 转换函数，禁止手动逐字段赋值。

**❌ 禁止模式**：

```go
// 禁止：手动逐字段转换，容易遗漏且冗长
func (h *Handler) Save(ctx context.Context, req *dto.SiteTagDTO) error {
    entity := &domain.SiteTag{}
    if req.Name != nil {
        entity.Name = *req.Name
    }
    if req.SiteID != nil {
        entity.SiteID = *req.SiteID
    }
    // ... 更多字段
}
```

**✅ 正确模式**：

```go
// 正确：使用 ToXxxEntity() 转换函数
func (h *Handler) Save(ctx context.Context, req *dto.SiteTagDTO) error {
    entity := req.ToEntity()
    return h.svc.Save(ctx, entity)
}
```

### 数据库操作

- **事务处理**: 使用 `gorm.Transaction` 支持嵌套事务
  ```go
  err := database.WithTransaction(db, func(tx *gorm.DB) error {
      // 多个操作
      return nil
  })
  ```
- **Repository 模式**: 所有数据库操作通过 Repository 层进行
- **SQL 文件**: 表结构定义在 YAML 配置文件中 (`backend/database/resources/`)

### 数据库连接使用规范

#### 1. Service 之间的数据库连接传递原则

**核心原则**: 数据库客户端实例的传递仅用于**事务功能**，普通查询不应传递。

- **需要传递数据库连接的场景**:
  - 多个 Service 需要在同一个事务中执行操作
  - 需要保证数据一致性（如同时操作作品和作品集）

- **不应传递数据库连接的场景**:
  - 独立的查询操作
  - 各 Service 独立管理自己的数据库连接和生命周期

#### 2. Repository 数据库连接管理

**核心原则**: GORM 自动管理连接池，Repository 持有 `*gorm.DB` 实例。

- **Repository 创建**: 通过 `NewRepository(db *gorm.DB)` 创建
- **GORM 连接池**: GORM 自动处理连接复用，无需手动管理
- **事务处理**: 使用 `database.WithTransaction()` 包装事务
- **Context 传递**: 所有 Repository 方法接收 `context.Context`

#### 3. Repository 查询方法标准模式

```go
// Repository 实现示例
type localTagRepository struct {
    db *gorm.DB
}

func NewRepository(db *gorm.DB) Repository {
    return &localTagRepository{db: db}
}

func (r *localTagRepository) Save(ctx context.Context, tag *domain.LocalTag) error {
    return r.db.WithContext(ctx).Create(tag).Error
}

func (r *localTagRepository) GetById(ctx context.Context, id int64) (*domain.LocalTag, error) {
    var tag domain.LocalTag
    err := r.db.WithContext(ctx).First(&tag, id).Error
    if err != nil {
        return nil, err
    }
    return &tag, nil
}
```

**关键点**:
- 使用 `r.db.WithContext(ctx)` 传递 Context
- GORM 自动处理连接释放
- 返回领域实体而非 raw rows

#### 4. 常见错误避免

1. **Context 泄漏**: 所有数据库操作必须传递 Context
2. **错误处理**: 使用 `errors.Is()` 判断特定错误类型
3. **事务回滚**: 事务中返回 error 即可触发回滚

#### 5. BaseRepository 复用原则

**核心原则**: 各模块的 Service 和 Repository 应**尽可能复用 `BaseRepository` 提供的功能**，只在通过 `BaseRepository` 实现时较为复杂或无法实现时，才自行实现逻辑。

**规则名称**: BASE_REPOSITORY_REUSE
**优先级**: P0（最高）

**BaseRepository 提供的核心能力**:

| 方法 | 功能 |
|------|------|
| `Save` | 保存单个实体 |
| `SaveBatch` | 批量保存 |
| `Update` | 更新实体 |
| `GetById` | 根据 ID 查询 |
| `List` | 列表查询（支持 QueryOption 条件） |
| `Count` | 统计数量 |
| `Delete` | 删除 |
| `Page` | 分页查询（支持 WHERE、ORDER、分页） |

**✅ 正确做法**:

```go
// siteTag/service.go
// 通过 BaseRepository.Page() 实现分页，不自行构建分页逻辑
func (s *Service) QueryBoundOrUnboundToLocalTagPage(...) (*Page[SiteTagFullDTO], error) {
    // 构建查询条件
    pageOption := database.PageOption{
        PageSize: pageQuery.PageSize,
        Page: pageQuery.PageNumber,
        QueryOption: database.QueryOption{
            Conditions: where,
            OrderBy: order,
        },
    }

    // ✅ 复用 BaseRepository.Page()，不自行实现分页
    rawPage, err := s.repo.Page(ctx, &pageOption)
    if err != nil {
        return nil, err
    }

    // 关联数据填充在 Service 层处理
    return s.enrichSiteTagsWithRelations(ctx, rawPage)
}
```

**❌ 错误做法**:

```go
// ❌ 错误：在 Repository 中自行实现完整分页逻辑
func (r *SiteTagRepository) QueryBoundOrUnboundToLocalTagPage(...) (*Page[SiteTagFullDTO], error) {
    db := r.GORM().Model(&domain.SiteTag{})

    // ❌ 自行构建 WHERE 条件
    if boundOnLocalTagId != nil {
        db = db.Where("local_tag_id = ?", *localTagId)
    }

    // ❌ 自行处理分页和排序
    queryCondition := database.QueryOption{...}
    pageCondition := database.PageOption{...}
    resPage, _ := r.Page(ctx, &pageCondition)

    // ❌ 循环中查询关联数据（N+1 问题）
    for _, tag := range siteTags {
        r.GORM().First(dto.LocalTag, tag.LocalTagID)
    }

    return model.NewPage(results, total, page, pageSize), nil
}
```

**收益**:

| 方面 | 说明 |
|------|------|
| **代码复用** | 分页逻辑统一由 BaseRepository 处理，避免重复实现 |
| **职责分离** | Repository 只负责数据存取，Service 负责业务组装 |
| **可维护性** | 修

改分页逻辑只需在一处 |
| **可测试性** | BaseRepository 逻辑可独立测试 |

### N+1 查询消除模式

**规则名称**: `ELIMINATE_N_PLUS_1_QUERY`
**优先级**: P0（最高）
**适用范围**: 所有包含关联数据的查询方法

**核心原则**: 禁止在遍历结果时逐条查询关联实体，必须在 Service 层批量查询后通过 Map 组装。

**执行步骤**:

1. 通过 `repo.Page()` 获取原始实体分页
2. 收集所有关联 ID，使用 `util.UniqueInt64()` / `util.UniqueString()` 去重
3. 通过依赖倒置接口批量查询关联数据（如 `localTagOp.ListByIds()`）
4. 构建 `id → entity` 的 Map
5. 遍历原始结果，组合 DTO 并填充关联数据
6. 使用 `model.NewPage()` 包装结果

**❌ 禁止模式**：

```go
// 禁止：N+1 查询，遍历中逐条查询关联数据
for _, tag := range tags {
    localTag, _ := localTagRepo.GetById(ctx, tag.LocalTagID)
    // ...
}
```

**✅ 正确模式**：

```go
func (s *Service) enrichWithRelations(ctx context.Context, rawPage *model.Page[SiteTag]) (*model.Page[dto.SiteTagFullDTO], error) {
    // 1. 收集并去重 ID
    localTagIds := util.UniqueInt64(collectLocalTagIds(rawPage.Data))

    // 2. 批量查询
    localTags, _ := s.localTagOp.ListByIds(ctx, localTagIds)

    // 3. 构建 Map
    localTagMap := lo.SliceToMap(localTags, func(t *domain.LocalTag) (int64, *domain.LocalTag) {
        return t.ID, t
    })

    // 4. 组装 DTO
    results := lo.Map(rawPage.Data, func(tag *domain.SiteTag, _ int) *dto.SiteTagFullDTO {
        d := dto.NewSiteTagFullDTO(tag)
        if lt, ok := localTagMap[tag.LocalTagID]; ok {
            d.LocalTag = dto.NewLocalTagDTO(lt)
        }
        return d
    })

    return model.NewPage(results, *rawPage.Total, rawPage.Page, rawPage.PageSize), nil
}
```

### 批量 UPDATE 优化

**规则名称**: `BATCH_UPDATE_OPTIMIZATION`
**优先级**: P1

更新多条记录的同一字段时，禁止循环逐条执行 UPDATE SQL，应在 Repository 新增批量 UPDATE 方法，单条 SQL 完成。

**❌ 禁止模式**：

```go
// 禁止：循环逐条更新
for _, id := range ids {
    db.Model(&Task{}).Where("id = ?", id).Update("status", newStatus)
}
```

**✅ 正确模式**：

```go
// 正确：单条 SQL 批量更新
func (r *repository) BatchUpdateStatus(ctx context.Context, ids []int64, status int) error {
    return r.DB.WithContext(ctx).Model(&Task{}).
        Where("id IN ?", ids).
        Update("status", status).Error
}
```

### Nullable 参数使用指针类型

**规则名称**: `NULLABLE_PARAM_USE_POINTER`
**优先级**: P1

当参数需要支持"设为 NULL"语义（如绑定/解绑、可选关联）时，Handler/Service/Repository 参数使用 `*int64` / `*string` 等指针类型。前端传 `null` 表示清除关联，传具体值表示绑定。

```go
// Handler: *int64 允许 null 值
func (h *Handler) BindToLocalTag(ctx context.Context, id int64, localTagID *int64) error {
    return h.svc.BindToLocalTag(ctx, id, localTagID)
}
```

### 数据库布尔值处理

Go 实体类与数据库之间的布尔值转换由 GORM 自动处理：

- **Go 实体使用 native `bool` 类型**
- **数据库存储使用 `int` 类型**（0 = false, 1 = true）
- **GORM 自动处理类型转换**，无需手动转换

**Go DTO 示例**（使用 native bool）:
```go
type SiteTagLocalRelateDTO struct {
    HasSameNameLocalTag bool `json:"hasSameNameLocalTag"`
}
```

**Go Model 示例**（使用 int 存储）:
```go
type Resource struct {
    State            int `gorm:"column:state" json:"state"`
    ResourceComplete int `gorm:"column:resource_complete" json:"resourceComplete"`
}
```

### 数据库路径存储规范

- **原则**: 数据库中存储的所有相对路径必须相对于项目根目录（资源库根目录）
- **根目录定义**: 项目配置中定义的资源库根目录路径
- **正确示例**:

  ```
  // 正确：相对于根目录的路径
  path = 'images/2024/01/photo.jpg'
  coverPath = 'covers/work_001.jpg'

  // 存储到数据库
  resource.FilePath = 'images/2024/01/photo.jpg'
  ```

- **错误示例**:

  ```
  // 错误：相对于当前工作目录或其他位置的路径
  path = '../shared/images/photo.jpg' // ✗ 包含 ../
  path = './cache/thumbnail.jpg' // ✗ 包含 ./
  path = 'C:/Users/Admin/pictures/1.jpg' // ✗ 绝对路径
  ```

- **路径拼接规范**: 使用统一的路径工具类进行路径拼接，确保生成相对路径

#### 例外情况

以下场景可以使用独立于程序目录的绝对路径：

- **用户自定义资源目录**: 用户在设置中配置的资源库目录（即workdir），该路径存储在配置表中，程序启动时读取
- **下载的临时文件**: 插件下载资源时使用的临时缓存目录，通常由系统临时目录或用户配置决定
- **外部关联文件**: 用户手动关联到作品的外部文件路径（如关联本地已存在的图片）

当存储此类路径时，应在字段命名或注释中明确标识其性质：

```go
// 正确示例：使用明确的字段命名
type Resource struct {
    // 相对于根目录的资源路径
    FilePath sql.NullString `gorm:"column:file_path" json:"filePath"`
    // 外部关联文件的绝对路径
    ExternalPath sql.NullString `gorm:"column:external_path" json:"externalPath"`
    // 用户配置的资源库根目录
    Workdir sql.NullString `gorm:"column:workdir" json:"workdir"`
}
```

### 插件开发规范

- **目录位置**: `plugin/package/`
- **插件结构**: 每个插件是独立包，包含 `package.json`
- **基类**: 实现插件接口，至少包含插件标识
- **插件工厂**: 通过工厂方法创建插件实例

## 代码质量工具

### ESLint 配置

- **基础配置**: Wails 默认 ESLint 配置
- **Vue 规则**: `vue/require-default-prop` 已禁用
- **自动修复**: `yarn lint` 运行 ESLint 并自动修复问题

### Prettier 格式化

- **格式化命令**: `yarn format`
- **集成**: 与 ESLint 配合，确保代码风格统一

### 类型检查

- **全项目检查**: `yarn typecheck`
- **主进程**: `yarn typecheck:node`
- **渲染进程**: `yarn typecheck:web`

## 日期与时间处理

- **统一格式**: 所有日期时间字段使用 Unix 时间戳（毫秒）
- **数据库存储**: 使用 `INTEGER` 类型存储时间戳
- **显示转换**: 在前端进行本地化时间格式转换

## 资源文件管理

- **本地文件协议**: 使用自定义 `resource://` 协议访问本地文件
- **图像处理**: 支持通过 sharp 进行图像尺寸调整
- **文件路径**: 使用绝对路径，避免相对路径歧义

## Git 提交规范

- **提交信息**: 使用中文描述，格式为 `类型(范围): 描述`
  - `feat(渲染进程): 添加作品卡片组件`
  - `fix(主进程): 修复数据库连接泄漏`
  - `docs(README): 更新项目说明`
- **类型前缀**: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`

## 新增功能开发流程

### 1. 添加新 Handler（Wails Bind）

1. 在 `backend/{module}/handler.go` 创建 Handler 结构体
2. 在 Handler 中定义业务方法（Wails 自动绑定到 `window.api`）
3. 运行 `wails3 generate bindings -ts` 生成前端 TypeScript 绑定
4. 前端通过 `window.api.{methodName}()` 调用

### 2. 添加新 Service

1. 在 `backend/{module}/` 目录创建 `service.go`
2. 定义 Service 结构体和业务方法
3. 在 Handler 中引用 Service 实例

### 3. 添加新 Repository

1. 在 `backend/{module}/` 目录创建 `repository.go`
2. 定义 Repository 接口
3. 创建 `repository_impl.go` 实现接口
4. 在 Service 中通过接口依赖

### 4. 添加数据库表

1. 在 `backend/database/resources/` 创建 YAML 迁移文件
2. 创建对应的 Model（`backend/model/`）
3. 创建 Repository 实现（`backend/{module}/repository_impl.go`）
4. 在 Service 层调用 Repository 方法

## 常见注意事项

1. **避免直接使用 GORM DB**: 始终通过 Repository 层访问数据库
2. **错误处理**: Go 方法返回 error，前端通过 try-catch 处理
3. **类型安全**: 充分利用 Go 类型系统，避免使用 `any`
4. **组件通信**: 优先使用 Props/Emits，复杂状态使用 Pinia Store
5. **性能优化**: 大型列表使用虚拟滚动，图片使用懒加载

## 文档维护

当代码规则发生变化时，需要同步更新以下文档：

1. 本文档（`code-rules.md`）
2. `CLAUDE.md` 中的 Key Conventions 部分

---

## 前端 API 与组件规范

### requireResponse + ApiResult 统一响应校验

**规则名称**: `WRAPPER_REQUIRE_RESPONSE`
**优先级**: P0（最高）
**适用范围**: 所有 `frontend/src/apis/http/wrappers/` 下的 Wrapper 函数

**核心原则**: Wrapper 函数使用 `requireResponse<T>()` 封装 Wails Handler 返回值，统一返回 `ApiResult<T>`。

- 查询类接口默认 `requireData=true`（校验 data 非空）
- 变更类接口（Save/Update/Delete）传 `requireData=false`
- `requireResponse` 内部已统一处理 null 检查和错误校验，禁止重复手写同类检查
- Wrapper 中允许存在非重复的、针对特定业务逻辑的非空校验

```typescript
import { requireResponse } from '../types'
import type { ApiResult } from '../types'

// 查询类：默认校验 data 非空
export async function queryPage(page: Page<TaskProgressTreeDTO>): ApiResult<Page<TaskProgressTreeDTO>> {
  const resp = await handler.QueryPage(page)
  return requireResponse(resp)
}

// 变更类：不校验 data
export async function save(task: TaskDTO): ApiResult<void> {
  const resp = await handler.Save(task)
  return requireResponse(resp, false)
}
```

### 统一错误处理：throw 不返回 undefined

**规则名称**: `UNIFIED_ERROR_THROW`
**优先级**: P0（最高）

所有异步查询函数（SearchTable queryPageFn、Dialog 适配器等）校验失败时必须 `throw new Error(msg)`，禁止返回 `undefined` 或静默失败（返回空 Page）。

**❌ 禁止模式**：

```typescript
// 禁止：返回 undefined 或空 Page
async function loadPage(page: Page) {
  const result = await taskQueryPage(page)
  if (!result) return undefined  // ✗
  return result
}
```

**✅ 正确模式**：

```typescript
async function loadPage(page: Page) {
  try {
    return await taskQueryPage(page)
  } catch (e) {
    ElMessage.error(String(e))
    throw e  // 向上抛出，让 SearchTable 知道失败
  }
}
```

### QueryAttribute .value 绑定

**规则名称**: `QUERY_ATTRIBUTE_VALUE_BINDING`
**优先级**: P1

查询参数使用 `QueryAttribute` 包装时：

- 通过 `.value` 属性读写实际值
- `v-model` 绑定 `xxx.value`
- `@clear` 事件重置为 `null`（而非空字符串）
- 模糊搜索时设置 `operator: Operator.OpLike`

```typescript
const queryAttr = ref<QueryAttribute>({
  value: null,
  operator: Operator.OpLike  // 模糊匹配
})

// v-model 绑定 .value
// <el-input v-model="queryAttr.value" @clear="queryAttr.value = null" />
```

### Page 类型统一

**规则名称**: `PAGE_TYPE_UNIFICATION`
**优先级**: P1

前端统一使用 Wails 绑定层的 `Page<T>` 类型（`@bindings/.../backend/base/model`），禁止自定义 `Page` 模型。

- 使用 `copyPage<T>()` 转换类型（保留分页信息）
- 使用 `newPage<T>()` 创建新分页实例
- 禁止导入旧的本地 `Page` 类型定义

```typescript
import { Page } from '@bindings/.../backend/base/model'
import { copyPage, newPage } from '@renderer/utils/pageUtil'

// 正确：使用绑定层 Page 类型
async function loadPage(page: Page<TaskProgressTreeDTO>): Promise<Page<TaskProgressTreeDTO>> {
  const result = await taskQueryPage(page)
  return copyPage(result)
}
```

### 组合 DTO 嵌套路径适配

**规则名称**: `COMPOSITION_DTO_NESTED_PATH`
**优先级**: P1

后端使用组合 DTO 后，前端 DataTable thead 的 key 使用点号嵌套路径访问子 DTO 字段。

```typescript
// 组合 DTO 结构：TaskProgressTreeDTO { taskProgress: { task: { taskName }, siteName } }
const thead = [
  { label: '任务名称', key: 'taskProgress.task.taskName' },
  { label: '站点名称', key: 'taskProgress.siteName' },
  { label: '创建时间', key: 'taskProgress.task.createTime' },
]

// data-key 同理使用嵌套路径
// <search-table :data-key="'taskProgress.task.id'" />
```

### Dialog 使用绑定层 DTO

**规则名称**: `DIALOG_USE_BINDING_DTO`
**优先级**: P1

- Dialog 的 `formData` 使用 Wails 绑定层 DTO 类型，禁止使用旧实体类型
- 组合 DTO 初始化时预创建嵌套对象，模板直接绑定，禁止使用 `computed` 中间层
- 简单 DTO 直接透传给 API；组合 DTO 提取子字段构造新 DTO 传入

```typescript
import { TaskDTO } from '@bindings/.../backend/base/model/dto'
import { TaskProgressTreeDTO } from '@renderer/model/model/dto/TaskProgressTreeDTO'

// formData 使用绑定层 DTO
const formData = ref<TaskDTO>(new TaskDTO())

// 组合 DTO 提取子字段传入 API
async function handleSave() {
  await taskSave(formData.value)  // 直接透传简单 DTO
}
```

### ID 类型统一为 number

**规则名称**: `ID_TYPE_NUMBER`
**优先级**: P2

前端 ID 统一使用 `number` 类型。从 SelectItem 的 `value`（string）取出时显式 `Number()` 转换，禁止函数签名中使用 `string` 类型的 ID。

```typescript
// ✅ 正确
function handleSelect(item: SelectItem) {
  formData.siteId = Number(item.value)
}

// ❌ 禁止
function handleSelect(item: SelectItem) {
  formData.siteId = item.value  // string 类型
}
```

---

## Go 主进程代码规范

### 文件命名规范

| 元素 | 命名规则 | 示例 |
|------|----------|------|
| Go 源文件 | snake_case 或与类型同名 | `model.go`, `repository_impl.go` |
| 目录 | 单元命名，全部小写 | `backend/author/` |
| 包名 | 与目录同名，简洁 | `package author` |

### 命名规范

| 元素 | 命名规则 | 示例 |
|------|----------|------|
| 结构体/接口 | PascalCase | `type Service struct` |
| 变量/函数 | camelCase | `func NewService()` |
| 常量 | UPPER_SNAKE_CASE | `ErrNameEmpty` |
| 错误类型 | 以 `Err` 开头 | `ErrNameEmpty` |
| 接口 | 以 `er` 结尾或名词 | `Repository`, `Provider` |

### 代码组织

```go
// 1. 包声明
package author

// 2. 导入 (按长度排序，标准库在前)
import (
    "context"
    "errors"

    "my-ipc-service/backend/database"
)

// 3. 错误定义
var ErrNameEmpty = errors.New("author: name is empty")

// 4. 领域实体
type Author struct {
    ID   int64
    Name string
}

// 5. 接口定义
type Repository interface {
    FindByID(ctx context.Context, id int64) (*Author, error)
}

// 6. Service 实现
type Service struct {
    repo Repository
}

func NewService(repo Repository) *Service {
    return &Service{repo: repo}
}
```

### 函数/方法顺序

1. 构造函数 (`NewXxx`)
2. 接口实现方法 (按接口定义顺序)
3. 业务方法 (按调用频率或重要性)
4. 私有辅助方法

### Entity 使用 NewXxx() 工厂方法

**规则名称**: `ENTITY_USE_NEW_FACTORY`
**优先级**: P1

创建实体实例时使用 `entity.NewXxx()` 工厂方法，禁止使用结构体字面量 `&entity.Xxx{}`，防止遗漏 `BaseEntity` 等必要字段的初始化。

```go
// ✅ 正确：使用工厂方法
task := entity.NewTask()
task.TaskName = req.TaskName

// ❌ 禁止：结构体字面量，容易遗漏字段初始化
task := &entity.Task{
    TaskName: req.TaskName,
}
```

### 移除冗余查询字段

**规则名称**: `REMOVE_REDUNDANT_QUERY_FIELDS`
**优先级**: P2

QueryDTO 中禁止为同一数据库列定义多个语义重复的字段（如同时定义精确匹配和模糊匹配字段），应保留一个字段，通过 `QueryAttribute.operator` 控制匹配方式。

### 重构后死代码清理

**规则名称**: `DEAD_CODE_CLEANUP`
**优先级**: P2

重构后确认无调用方的旧方法直接删除，禁止保留"以防万一"的代码。可通过 IDE 的 "Find Usages" 确认无引用后安全删除。

### 注释规范

```go
// Author 领域实体
type Author struct {
    ID   int64
    Name string
}

// Repository 定义了 Author 模块的数据存取契约
type Repository interface {
    // FindByID 根据 ID 查找作者
    FindByID(ctx context.Context, id int64) (*Author, error)
}

// Register 创建新作者
// ctx: 上下文
// name: 作者名称
// 返回创建的作者或错误
func (s *Service) Register(ctx context.Context, name string) (*Author, error) {
    // ...
}
```

### 禁止的做法

| 禁止 | 正确做法 |
|------|----------|
| 在 Service 层 import `backend/database` | 在 `repository_impl.go` 中 import |
| 返回 `*gorm.DB` 或 `sql.Rows` | 返回领域实体或 DTO |
| 使用裸 `error` 作为全局变量 | 使用 `var ErrXxx = errors.New(...)` |
| 跨模块直接引用其他业务的 Service | 使用接口隔离 |
| 在 `util` 包中包含有状态逻辑 | 使用纯函数 |

### 错误处理模式

```go
// 定义错误
var (
    ErrNameEmpty   = errors.New("author: name is empty")
    ErrNotFound    = errors.New("author: not found")
)

// 在 Service 中使用
func (s *Service) Register(ctx context.Context, name string) (*Author, error) {
    if name == "" {
        return nil, ErrNameEmpty
    }
    // ...
}

// 调用方判断错误类型
if errors.Is(err, author.ErrNotFound) {
    // 处理未找到
}
```

### context 传递规范

- 所有 public 方法必须接收 `context.Context` 作为第一个参数
- 在调用下游服务时传递 `ctx`
- 不要在 `context.WithValue` 中存储业务数据

### Service 依赖规范（Go 主进程）

**核心原则**：Service 之间的依赖必须通过接口解耦，禁止直接调用其他模块的 Service。

**规则名称**：SERVICE_DEPENDENCY_VIA_INTERFACE
**优先级**：P0（最高）
**适用范围**：所有跨 Service 的业务调用

#### 1. 接口定义原则

**由使用方（调用方）定义接口，被使用方（提供方）实现接口**。

```go
// ✅ 正确：调用方定义接口
// work/service.go

// LocalTagReader 定义了 Work 模块需要的 LocalTag 查询能力
type LocalTagReader interface {
    ListByWorkId(ctx context.Context, workId int64) ([]*domain.LocalTag, error)
    GetById(ctx context.Context, id int64) (*domain.LocalTag, error)
}

// Service 结构体依赖接口
type Service struct {
    repo Repository
    localTagReader LocalTagReader  // 通过接口依赖
}
```

#### 2. 提供方实现接口

```go
// ✅ 正确：提供方实现接口（不需要在自己模块定义接口）
// localTag/service.go

func (s *Service) ListByWorkId(ctx context.Context, workId int64) ([]*domain.LocalTag, error) {
    return s.repo.ListByWorkId(ctx, workId)
}

func (s *Service) GetById(ctx context.Context, id int64) (*domain.LocalTag, error) {
    return s.repo.GetById(ctx, id)
}
```

#### 3. 禁止直接调用

**❌ 禁止模式**：

```go
// work/service.go

// 错误：直接持有其他 Service 的具体类型
type Service struct {
    repo Repository
    localTagSvc *localTag.Service  // ❌ 禁止：直接依赖具体 Service 类型
}

// 错误：在方法内直接实例化
func (s *Service) DoSomething(ctx context.Context, workId int64) error {
    localTagSvc := localTag.NewService(...)  // ❌ 禁止：方法内直接创建
    return nil
}
```

#### 4. 依赖注入

通过构造函数注入接口实现：

```go
// work/service.go

type Service struct {
    repo Repository
    localTagReader    LocalTagReader
    localAuthorReader LocalAuthorReader
    // ... 其他依赖
}

func NewService(
    repo Repository,
    localTagReader LocalTagReader,
    localAuthorReader LocalAuthorReader,
) *Service {
    return &Service{
        repo:               repo,
        localTagReader:    localTagReader,
        localAuthorReader: localAuthorReader,
    }
}
```

#### 5. 依赖倒置原则（DIP）

**规则名称**: DEPENDENCY_INVERSION
**优先级**: P0（最高）

**核心原则**:
- 高层模块不应依赖低层模块，两者都应依赖抽象
- **接口由调用方定义，实现方负责实现**
- 抽象不应依赖细节，细节应依赖抽象

**接口定义位置**: 接口定义在**调用方模块**，而非被调用方模块

**✅ 正确示例 - siteTag 模块**:

```go
// siteTag/service.go - 调用方定义接口

// LocalTagQueryOperator 本地标签查询接口（由 siteTag 模块定义）
type LocalTagQueryOperator interface {
    ListByIds(ctx context.Context, ids []int64) ([]*domain.LocalTag, error)
}

// SiteQueryOperator 站点查询接口（由 siteTag 模块定义）
type SiteQueryOperator interface {
    ListByIds(ctx context.Context, ids []int64) ([]*domain.Site, error)
}

// Service 结构体
type Service struct {
    repo             Repository
    localTagOperator LocalTagOperator        // 已有接口
    localTagQueryOp  LocalTagQueryOperator  // 注入接口
    siteQueryOp      SiteQueryOperator       // 注入接口
}
```

```go
// localTag/service.go - 实现方实现接口
func (s *Service) ListByIds(ctx context.Context, ids []int64) ([]*domain.LocalTag, error) {
    if len(ids) == 0 {
        return make([]*domain.LocalTag, 0), nil
    }
    return s.repo.List(ctx, &database.QueryOption{
        Conditions: []clause.Expression{clause.IN{Column: "id", Values: ids}},
    })
}
```

**❌ 错误示例**:

```go
// ❌ 错误：接口定义在被调用方（localTag）
type LocalTagReader interface {
    ListByIds(ctx context.Context, ids []int64) ([]*domain.LocalTag, error)
}

// ❌ 错误：调用方直接依赖具体实现
type Service struct {
    localTagSvc *localTag.Service  // 直接依赖具体实现
}
```

**依赖倒置与复用 BaseRepository 的结合**:

```go
// siteTag/service.go - 复用 BaseRepository.Page() + 依赖倒置填充关联数据
func (s *Service) QueryBoundOrUnboundToLocalTagPage(...) (*Page[SiteTagFullDTO], error) {
    // ✅ 复用 BaseRepository.Page() 实现分页
    pageOption := database.PageOption{
        PageSize: pageQuery.PageSize,
        Page: pageQuery.PageNumber,
        QueryOption: database.QueryOption{
            Conditions: where,
            OrderBy: order,
        },
    }
    rawPage, err := s.repo.Page(ctx, &pageOption)
    if err != nil {
        return nil, err
    }

    // ✅ 通过依赖倒置接口填充关联数据
    return s.enrichSiteTagsWithRelations(ctx, rawPage)
}

func (s *Service) enrichSiteTagsWithRelations(ctx, rawPage) (*Page[SiteTagFullDTO], error) {
    localTagIds := collectIds(siteTags)
    siteIds := collectIds(siteTags)

    // ✅ 通过接口查询，不直接依赖实现
    localTags, _ := s.localTagQueryOp.ListByIds(ctx, localTagIds)
    sites, _ := s.siteQueryOp.ListByIds(ctx, siteIds)

    // 组装结果
    ...
}
```

#### 6. wire.go 注入依赖

```go
// cmd/server/wire.go

func InitModules(db *gorm.DB, r *gin.Engine) *Modules {
    // 创建各模块的 Repository 和 Service
    localTagRepo := localTag.NewRepository(db)
    localTagSvc := localTag.NewService(localTagRepo)

    // Work Service 依赖 LocalTagReader 接口，传入 localTagSvc（实现了该接口）
    workSvc := work.NewService(workRepo, localTagSvc)  // localTagSvc 实现了 LocalTagReader
    // ...
}
```

#### 7. 理由与约束

| 原因 | 说明 |
|------|------|
| **解耦** | 接口定义隔离了调用方和实现方，实现方可以替换 |
| **可测试性** | 便于在测试时注入 mock 实现 |
| **循环依赖避免** | 通过接口打破模块间的循环依赖 |
| **单一职责** | 接口定义由调用方负责，清晰表达"我需要什么" |

#### 8. 常见错误

| 错误 | 正确做法 |
|------|----------|
| Service A 直接持有 `*BService` | A 定义 `XxxReader` 接口，B Service 实现它 |
| 在方法内 `new(BService)` | 通过构造函数注入 |
| 两个模块互相定义对方的接口 | 重构为单向依赖，或通过事件解耦 |

---

**最后更新**: 2026-05-05
**维护者**: AI Assistant

---

## 文档更新记录

### 2026-05-05
- [修改] 目录结构调整：`internal/` → `backend/`，`pkg/` → `backend/base/`
