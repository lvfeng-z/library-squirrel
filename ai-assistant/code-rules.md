# LibrarySquirrel 代码规则与约定

## 概述

本文档记录 LibrarySquirrel 项目的代码编写规则、命名约定、文件组织和开发规范。遵循这些规则有助于保持代码一致性、可维护性和团队协作效率。

## 文件命名规范

### 目录结构约定

- **src/main/** - 主进程代码 (Node.js/Electron)
  - `base/` - 基类 (BaseDao, BaseService, BasePlugin)
  - `constant/` - 常量与枚举（主进程专用）
  - `core/` - 核心服务 (MainProcessApi, Initialize)
  - `dao/` - 数据访问对象
  - `database/` - 数据库初始化
  - `model/` - 领域模型和实体（主进程专用）
  - `plugin/` - 插件系统
  - `service/` - 业务逻辑服务
  - `resources/` - YAML 配置文件
  - `util/` - 工具类（主进程专用）
- **src/shared/** - 主进程和渲染进程共用代码
  - `model/` - 共用实体类、DTO、枚举、常量
    - `constant/` - 枚举和常量
    - `dto/` - 数据传输对象
    - `domain/` - 领域模型
    - `entity/` - 实体类
    - `interface/` - 接口定义
    - `queryDTO/` - 查询DTO
    - `util/` - 工具类型
  - `util/` - 共用工具函数（StringUtil, TreeUtil, AssertUtil等）
- **src/preload/** - 预加载脚本 (IPC 桥接)
- **src/renderer/** - 渲染进程代码 (Vue 3)
  - `src/apis/` - IPC API 封装
  - `src/components/` - Vue 组件
  - `src/store/` - Pinia 状态管理
  - `src/utils/` - 渲染进程工具类

### 文件命名规则

| 文件类型                 | 命名规则                    | 示例                                        |
| ------------------------ | --------------------------- | ------------------------------------------- |
| **类文件**               | PascalCase + `.ts`          | `WorkService.ts`, `BaseDao.ts`              |
| **Vue 组件**             | PascalCase + `.vue`         | `WorkCard.vue`, `WorkSetDialog.vue`         |
| **TypeScript 类型/接口** | PascalCase + `.ts`          | `WorkDTO.ts`, `TaskStatus.ts`               |
| **工具函数**             | camelCase + `.ts`           | `logUtil.ts`, `apiUtil.ts`                  |
| **常量文件**             | camelCase + `.ts`           | `errorCode.ts`, `httpStatus.ts`             |
| **配置文件**             | kebab-case + `.yml`/`.json` | `createDataTables.yml`, `tsconfig.web.json` |

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

- **模块解析**: 主进程使用 `node16`，渲染进程使用 `bundler`
- **路径别名**:
  - `@renderer/*` → `src/renderer/src/*`（渲染进程专用）
  - `@shared/*` → `src/shared/*`（主进程和渲染进程共用，推荐使用）
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

- **方法命名**: `serviceName-methodName` (kebab-case)
- **主进程注册** (`MainProcessApi.ts`):
  ```typescript
  Electron.ipcMain.handle('workService-getById', async (_event, args) => {
    const service = new WorkService()
    try {
      return ApiUtil.response(await service.getById(args))
    } catch (error) {
      return returnError(error)
    }
  })
  ```
- **预加载桥接** (`preload/index.ts`):
  ```typescript
  workServiceGetById: (args) => Electron.ipcRenderer.invoke('workService-getById', args)
  ```
- **渲染进程调用**:
  ```typescript
  const response = await window.api.workServiceGetById(workId)
  ```

### 响应处理

- **统一响应格式**: 使用 `ApiUtil.response(data)` / `ApiUtil.error(message)`
- **错误包装**: 所有错误通过 `returnError(error)` 包装
- **前端处理**:
  ```typescript
  const response = await window.api.someMethod(args)
  if (ApiUtil.check(response)) {
    const data = ApiUtil.data(response)
    // 处理成功数据
  } else {
    const error = ApiUtil.error(response)
    // 处理错误
  }
  ```

### DTO 设计与数据暴露原则

规则名称：DTO_FLATTENING_AND_ISOLATION
优先级：P0 (最高)
适用范围：所有涉及数据传输对象（DTO）、API 响应/请求类、VO（View Object）生成的场景。

#### 1. 核心指令 (Core Directive)

在生成或修改 DTO 类时，严禁直接将领域实体（Entity/Domain Model）或数据库模型作为属性嵌套在 DTO 中。必须采用扁平化结构，将需要的属性显式复制到 DTO 类中。

#### 2. 具体执行标准 (Execution Standards)

**❌ 禁止模式 (Anti-Pattern)**：

- 禁止在 DTO 中定义类型为 Entity 的属性（例如：`user: User`）
- 禁止直接使用 `@JsonIgnore` 等注解来"修补"直接嵌套实体带来的安全问题
- 禁止让 DTO 的字段结构完全被动地跟随数据库表结构变更

**✅ 推荐模式 (Best Practice)**：

- 显式复制：DTO 应包含独立的、基础类型的字段（如 `id: number`, `username: string`, `createTime: number`）
- 按需裁剪：只复制业务场景真正需要的字段，自动过滤掉敏感字段（如 `password`, `salt`, `internalFlag`）
- 语义重命名：如果前端需要的字段名与数据库不一致，直接在 DTO 中定义符合前端语义的字段名（例如：数据库叫 `usr_nm`，DTO 叫 `userName`）
- 嵌套即嵌套 DTO：如果确实需要层级结构（如订单包含地址），嵌套的对象必须是另一个专门的 DTO（如 `AddressDTO`），绝不能是 `Address` 实体

**🛠️ 映射要求 (Mapping Requirement)**：

当实体与 DTO 字段较多时，必须生成或使用现有的映射代码（推荐使用手动 `convert()` 方法），严禁省略映射步骤。

#### 3. 理由与约束 (Rationale & Constraints)

- **安全性 (Security)**：防止因实体类新增敏感字段而意外泄露给前端（Over-exposure）
- **解耦 (Decoupling)**：确保 API 契约（Contract）独立于内部数据库模型，允许数据库重构而不破坏前端接口
- **序列化安全 (Serialization Safety)**：避免懒加载（Lazy Loading）导致的异常或循环引用导致的栈溢出
- **前端友好 (Frontend Friendly)**：提供扁平、直观的 JSON 结构，减少前端访问路径深度

#### 4. 示例对比 (Example)

**❌ 错误示例 (AI 不应生成)**：

```typescript
// 违反规则：直接嵌套实体，可能泄露 password 和 salt
class UserResponseDTO {
  user: User
  token: string
}
// 输出 JSON: { "user": { "id": 1, "username": "admin", "password": "...", "salt": "..." }, "token": "..." }
```

**✅ 正确示例 (AI 应生成)**：

```typescript
// 遵守规则：扁平化，只暴露必要字段，独立于 User 实体
class UserResponseDTO {
  id: number
  username: string
  email: string
  token: string
  // 即使 User 实体中有 password 字段，这里也不会出现
}
// 输出 JSON: { "id": 1, "username": "admin", "email": "admin@example.com", "token": "..." }

// 映射方法
function fromEntity(user: User, token: string): UserResponseDTO {
  const dto = new UserResponseDTO()
  dto.id = user.id
  dto.username = user.username
  dto.email = user.email
  dto.token = token
  return dto
}
```

#### 5. 异常处理 (Exception Handling)

仅在以下极端情况可考虑嵌套对象，但嵌套的子对象必须也是 DTO：

- 业务逻辑明确要求复杂的层级分组，且该子结构在多个 API 中复用（此时应创建 `XXXSubDTO`）

### 数据库操作

- **事务处理**: 使用 `db.transaction()` 支持嵌套 SAVEPOINT
  ```typescript
  await db.transaction(async (tx) => {
    await tx.run('INSERT INTO ...')
    await tx.run('UPDATE ...')
  }, '操作描述')
  ```
- **DAO 模式**: 所有数据库操作通过 DAO 层进行
- **SQL 文件**: 表结构定义在 YAML 配置文件中 (`src/main/resources/database/`)

### 数据库连接使用规范

#### 1. Service 之间的数据库连接传递原则

**核心原则**: 数据库客户端实例的传递仅用于**事务功能**，普通查询不应传递。

- **需要传递数据库连接的场景**:
  - 多个 Service 需要在同一个事务中执行操作
  - 需要保证数据一致性（如同时操作作品和作品集）

- **不应传递数据库连接的场景**:
  - 独立的查询操作
  - 各 Service 独立管理自己的数据库连接和生命周期

#### 2. Service 实例创建规范

**❌ 错误示例**:

```typescript
// 错误：传入数据库连接但不用于事务，导致连接无法自动释放
class SearchService {
  queryWorkSetPage() {
    // 传入 db 但不是为了事务，会导致 WorkSetService 无法释放连接
    const workSetService = new WorkSetService(this.db)
    return workSetService.queryPageByWorkConditionsWithCover(page, searchConditions)
  }
}
```

**✅ 正确示例**:

```typescript
// 正确：不传入数据库连接，让被调用的 Service 创建并管理自己的连接
class SearchService {
  queryWorkSetPage() {
    // WorkSetService 会创建自己的数据库连接，查询完成后自动释放
    const workSetService = new WorkSetService()
    return workSetService.queryPageByWorkConditionsWithCover(page, searchConditions)
  }
}

// 正确：需要事务时传入数据库连接
class SomeTransactionService {
  async doTransaction() {
    const db = new DatabaseClient(this.constructor.name)
    try {
      const workService = new WorkService(db)
      const workSetService = new WorkSetService(db)
      // 在同一个事务中执行
      await db.transaction(async (tx) => {
        await workService.save(work)
        await workSetService.save(workSet)
      }, '操作描述')
    } finally {
      db.release()
    }
  }
}
```

#### 3. BaseService 构造函数参数说明

```typescript
// BaseService 构造函数
class BaseService {
  constructor(
    dao: new (db: DatabaseClient, injectedDB: boolean) => Dao,
    db?: DatabaseClient, // 可选：传入数据库客户端用于事务
    injectedDB?: boolean // 可选：显式指定是否为注入的连接
  ) {
    //...
  }
}
```

- **不传 db 参数**: Service 创建自己的数据库连接，查询完成后自动释放
- **传入 db 参数**: 使用传入的数据库连接，通常仅用于事务场景

#### 4. DAO 层连接管理

DAO 层通过 `injectedDB` 标志管理连接释放：

- `injectedDB = false`: Service 创建的连接，查询完成后在 `finally` 块中释放
- `injectedDB = true`: 外部注入的连接（用于事务），不自动释放，由外部事务管理器控制

**DAO 查询方法标准模式**:

```typescript
class demoDao {
  async queryByConditions(params): Promise<Result> {
    const db = this.acquire()
    try {
      const rows = await db.all(/*...*/)
      return this.toResultTypeDataList(rows)
    } finally {
      if (!this.injectedDB) {
        db.release()
      }
    }
  }
}
```

#### 5. 常见错误避免

1. **连接泄漏**: 不要在非事务场景下传递数据库连接
2. **重复释放**: 确保只在 `injectedDB = false` 时释放连接
3. **事务回滚**: 事务中使用 `throw` 触发回滚，避免手动处理

### 数据库布尔类型转换

- **原则**: 数据库中使用整数（0/1）表示布尔值，转换为 JS 布尔值时必须使用 `BOOL` 常量进行比较
- **常量位置**: `src/shared/model/constant/BOOL.ts`
- **正确示例**:

  ```typescript
  import { BOOL } from '@shared/model/constant/BOOL.ts'

  // 从数据库读取后转换
  item.isCover = isCoverValue === BOOL.TRUE

  // 写入数据库前转换
  link.isCover = true ? BOOL.TRUE : BOOL.FALSE
  ```

- **错误示例**:

  ```typescript
  // 禁止使用硬编码数字比较
  item.isCover = isCoverValue === 1 // ✗

  // 禁止使用布尔字面量比较
  item.isCover = isCoverValue === true // ✗
  ```

### 数据库查询结果转换为实体类

- **原则**: 从数据库查询出的元数据应先通过框架的转换方法（`toResultTypeData`/`toResultTypeDataList`）转换为项目中定义的实体类或其他对应类，再进行后续操作
- **避免使用 `as` 类型断言**: 不要直接使用 `as` 从 `Record<string, unknown>` 中提取字段
- **需要额外字段时创建 DTO 类**: 如果查询结果需要返回比实体类更多的字段（如关联表的外键），应在 `src/shared/model/domain/` 目录下创建对应的 DTO 类继承原实体类
- **正确示例**:

  ```typescript
  // 1. 创建继承实体类的 DTO（包含额外字段）
  // 文件: src/shared/model/domain/ResourceWithWorkSetId.ts
  import Resource from '../entity/Resource.ts'
  export default class ResourceWithWorkSetId extends Resource {
    workSetId: number | undefined | null
  }

  // 2. 在 DAO 中使用框架方法转换
  const resultList = super.toResultTypeDataList<ResourceWithWorkSetId>(rows)

  // 3. 从实体类中获取属性（无需类型断言）
  for (const item of resultList) {
    const workSetId = item.workSetId // 类型安全
  }
  ```

- **错误示例**:

  ```typescript
  // 禁止直接使用 as 类型断言提取字段
  const workSetId = row['work_set_id'] as number // ✗

  // 禁止直接访问 Record 的索引
  const id = row['id'] // ✗ 类型为 unknown
  ```

### 数据库路径存储规范

- **原则**: 数据库中存储的所有相对路径必须相对于项目根目录（资源库根目录）
- **根目录定义**: 项目配置中定义的资源库根目录路径
- **正确示例**:

  ```typescript
  // 正确：相对于根目录的路径
  const path = 'images/2024/01/photo.jpg'
  const coverPath = 'covers/work_001.jpg'

  // 存储到数据库
  await resourceDao.save({ path: 'images/2024/01/photo.jpg' })
  ```

- **错误示例**:

  ```typescript
  // 错误：相对于当前工作目录或其他位置的路径
  const path = '../shared/images/photo.jpg' // ✗ 包含 ../
  const path = './cache/thumbnail.jpg' // ✗ 包含 ./
  const path = 'C:/Users/Admin/pictures/1.jpg' // ✗ 绝对路径
  ```

- **路径拼接规范**: 使用统一的路径工具类进行路径拼接，确保生成相对路径

#### 例外情况

以下场景可以使用独立于程序目录的绝对路径：

- **用户自定义资源目录**: 用户在设置中配置的资源库目录（即workdir），该路径存储在配置表中，程序启动时读取
- **下载的临时文件**: 插件下载资源时使用的临时缓存目录，通常由系统临时目录或用户配置决定
- **外部关联文件**: 用户手动关联到作品的外部文件路径（如关联本地已存在的图片）

当存储此类路径时，应在字段命名或注释中明确标识其性质：

```typescript
// 正确示例：使用明确的字段命名
interface ResourceRecord {
  // 相对于根目录的资源路径
  path: string
  // 外部关联文件的绝对路径
  externalPath: string | null
  // 用户配置的资源库根目录
  libraryRoot: string
}
```

### 插件开发规范

- **目录位置**: `src/main/plugin/package/`
- **插件结构**: 每个插件是独立包，包含 `package.json`
- **基类**: 实现 `BasePlugin` 接口，至少包含 `pluginId: number`
- **插件工厂**: 通过 `PluginFactory.create()` 创建插件实例

## 代码质量工具

### ESLint 配置

- **基础配置**: `@electron-toolkit/eslint-config`
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

### 1. 添加新 Service

1. 在 `src/main/service/` 创建 Service 类
2. 在 `MainProcessApi.ts` 注册 IPC 处理器
3. 在 `preload/index.ts` 添加 API 包装
4. 前端通过 `window.api` 调用

### 2. 添加新 Vue 组件

1. 在 `src/renderer/src/components/` 创建 `.vue` 文件
2. 使用 `<script setup lang="ts">` 语法
3. 定义 Props 接口（使用 `Props` 后缀）
4. 通过路径别名导入其他组件

### 3. 添加数据库表

1. 在 `src/main/resources/database/createDataTables.yml` 定义表结构
2. 创建对应的 Model 类（`src/main/model/`）
3. 创建 DAO 类（`src/main/dao/`）
4. 在 Service 层调用 DAO 方法

## 常见注意事项

1. **避免直接使用 SQLite API**: 始终通过 DAO 层访问数据库
2. **错误处理**: 所有异步操作都需要 try-catch 并返回统一错误格式
3. **类型安全**: 充分利用 TypeScript 类型系统，避免使用 `any`
4. **组件通信**: 优先使用 Props/Emits，复杂状态使用 Pinia Store
5. **性能优化**: 大型列表使用虚拟滚动，图片使用懒加载

## 文档维护

当代码规则发生变化时，需要同步更新以下文档：

1. 本文档（`code-rules.md`）
2. `CLAUDE.md` 中的 Key Conventions 部分
3. 相关开发示例（`development-scenarios.md`）

---

## Go 主进程代码规范

### 文件命名规范

| 元素 | 命名规则 | 示例 |
|------|----------|------|
| Go 源文件 | snake_case 或与类型同名 | `model.go`, `repository_impl.go` |
| 目录 | 单元命名，全部小写 | `internal/author/` |
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

    "my-ipc-service/internal/database"
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
| 在 Service 层 import `internal/database` | 在 `repository_impl.go` 中 import |
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

### DTO 设计规范（Go 主进程）

**核心原则**：Go 的结构体嵌入（embedding）不能复现 TypeScript 的继承语义，DTO 必须显式定义所有字段。

**规则名称**：DTO_EXPLICIT_FIELDS
**优先级**：P0（最高）
**适用范围**：所有 DTO、DTO 构造器函数的实现

#### 1. 禁止使用嵌入（Embedding）

**❌ 禁止模式**：

```go
// 错误：使用嵌入来"继承"父结构体的字段
type SiteTagFullDTO struct {
    *SiteTag              // 禁止：使用嵌入
    LocalTag *LocalTag    `json:"localTag,omitempty"`
    Site     *Site        `json:"site,omitempty"`
}

type SiteTagLocalRelateDTO struct {
    *SiteTagFullDTO       // 禁止：使用嵌入
    HasSameNameLocalTag bool `json:"hasSameNameLocalTag"`
}
```

**问题**：
- Go 的嵌入是"委托"而非"继承"，字段访问语义不同
- JSON 序列化时字段提升（field promotion）行为与 TypeScript class inheritance 不同
- 无法显式控制字段的 JSON tag
- DTO 与实体类耦合过紧，实体变更会直接影响 DTO

#### 2. 正确做法：显式定义所有字段

**✅ 推荐模式**：

```go
type SiteTagFullDTO struct {
    // 基础实体字段（显式列出）
    ID         int64  `json:"id"`
    CreateTime int64  `json:"createTime"`
    UpdateTime int64  `json:"updateTime"`
    // 站点标签字段（显式列出）
    SiteID        int64  `json:"siteId"`
    SiteTagID     string `json:"siteTagId"`
    SiteTagName   string `json:"siteTagName"`
    BaseSiteTagID string `json:"baseSiteTagId"`
    Description   string `json:"description"`
    LocalTagID    int64  `json:"localTagId"`
    LastUse       int64  `json:"lastUse"`
    // 关联对象（显式定义）
    LocalTag *LocalTag `json:"localTag,omitempty"`
    Site     *Site     `json:"site,omitempty"`
}

type SiteTagLocalRelateDTO struct {
    // 基础实体字段（显式列出）
    ID         int64  `json:"id"`
    CreateTime int64  `json:"createTime"`
    UpdateTime int64  `json:"updateTime"`
    // 站点标签字段（显式列出）
    SiteID        int64  `json:"siteId"`
    SiteTagID     string `json:"siteTagId"`
    SiteTagName   string `json:"siteTagName"`
    BaseSiteTagID string `json:"baseSiteTagId"`
    Description   string `json:"description"`
    LocalTagID    int64  `json:"localTagId"`
    LastUse       int64  `json:"lastUse"`
    // 关联对象（显式定义）
    LocalTag *LocalTag `json:"localTag,omitempty"`
    Site     *Site     `json:"site,omitempty"`
    // 额外字段（显式定义）
    HasSameNameLocalTag bool `json:"hasSameNameLocalTag"`
}
```

#### 3. 构造函数实现

```go
// NewSiteTagFullDTO 创建站点标签完整DTO
func NewSiteTagFullDTO(siteTag *SiteTag) *SiteTagFullDTO {
    if siteTag == nil {
        return nil
    }
    dto := &SiteTagFullDTO{
        ID:            siteTag.ID,
        CreateTime:    siteTag.CreateTime,
        UpdateTime:    siteTag.UpdateTime,
        SiteID:        siteTag.SiteID,
        SiteTagID:     siteTag.SiteTagID,
        SiteTagName:   siteTag.SiteTagName,
        BaseSiteTagID: siteTag.BaseSiteTagID,
        Description:   siteTag.Description,
        LocalTagID:    siteTag.LocalTagID,
        LastUse:       siteTag.LastUse,
    }
    return dto
}
```

#### 4. 理由与约束

| 问题 | 说明 |
|------|------|
| **语义差异** | Go embedding 是"委托"，不是 TypeScript 的"继承" |
| **JSON 序列化** | 嵌入字段的 JSON tag 行为与预期不符 |
| **解耦** | DTO 应独立于实体类，允许各自独立演进 |
| **显式优于隐式** | 显式列出所有字段，代码可读性更好 |

#### 5. 适用场景汇总

| 场景 | 推荐方式 |
|------|----------|
| 创建包含实体字段的 DTO | 显式定义所有字段 |
| 创建继承自其他 DTO 的 DTO | 显式定义所有字段（包括父 DTO 的字段） |
| 添加额外关联对象 | 在 DTO 中显式添加关联字段 |

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

#### 5. wire.go 注入依赖

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

#### 6. 理由与约束

| 原因 | 说明 |
|------|------|
| **解耦** | 接口定义隔离了调用方和实现方，实现方可以替换 |
| **可测试性** | 便于在测试时注入 mock 实现 |
| **循环依赖避免** | 通过接口打破模块间的循环依赖 |
| **单一职责** | 接口定义由调用方负责，清晰表达"我需要什么" |

#### 7. 常见错误

| 错误 | 正确做法 |
|------|----------|
| Service A 直接持有 `*BService` | A 定义 `XxxReader` 接口，B Service 实现它 |
| 在方法内 `new(BService)` | 通过构造函数注入 |
| 两个模块互相定义对方的接口 | 重构为单向依赖，或通过事件解耦 |

---

**最后更新**: 2026-04-06
**维护者**: AI Assistant
