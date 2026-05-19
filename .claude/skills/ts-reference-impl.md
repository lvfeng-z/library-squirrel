# TS 版参考实现迁移技能

## 适用场景

当用户要求参照 TS 版项目（TypeScript/SWIFT 版）实现某个功能时使用此技能。典型触发词：
- "参照 TS 版实现"
- "对比 TS 版"
- "复现 TS 版的 xxx 逻辑"
- "TS 版有这个功能"

## 工作流程

### 1. 定位 TS 版代码

TS 版项目通常位于一个独立代码库。先确认 TS 版项目的路径，然后定位相关模块：
- 后端逻辑通常在 `src/service/` 或 `src/api/` 目录
- 前端组件在 `src/components/` 或 `src/views/`
- 类型定义在 `src/types/` 或 `src/models/`

### 2. 分析 TS 版逻辑（只读）

**必须完整阅读 TS 版相关代码**，理解：
- 数据流：输入 → 中间处理 → 输出
- 查询模式：单条/批量、JOIN/子查询、分页策略
- DTO 结构：字段、嵌套关系、可选字段
- 边界条件：空值处理、默认值、兜底逻辑

**禁止在未完整阅读的情况下开始编码。**

### 3. 设计 Go 版实现方案

对照 TS 版逻辑，设计 Go 版实现时必须考虑以下差异：

#### 3.1 架构映射

| TS 版模式 | Go 版对应 |
|-----------|-----------|
| Service 类方法 | `Repository`/`Service`/`Handler` 三层 |
| Prisma/TypeORM 查询 | GORM + `BaseRepository` |
| `async/await` | `context.Context` 透传 |
| TypeScript interface | Go interface（由调用方定义） |
| `Record<K, V>` | `map[K]V` |
| `Array.map/filter` | Go for 循环 + append |
| Promise.all 并行 | 顺序调用（SQLite 单写锁下并行无收益） |

#### 3.2 强制遵守的 Go 版项目规则

即使 TS 版使用了不同模式，Go 版也必须遵守：

1. **DTO 规则**：
   - DTO 禁止嵌入实体（`entity.Work`），必须使用其他 DTO（`WorkDTO`）
   - DTO 必须声明在 `backend/base/model/dto/` 目录
   - 使用 `NewXxxDTO(entity)` 构造函数，使用 `util.NullStringToPointer`/`util.NullInt64ToPointer` 转换可空字段

2. **接口规则**：
   - Service 依赖通过接口注入，接口由调用方（workSet/search 等）定义
   - 禁止持有具体 `*OtherService`
   - 接口尽量窄（只声明需要的方法）

3. **批量查询规则**：
   - 禁止 N+1 查询（循环中调用数据库）
   - 模式：收集所有 ID → 批量查询 → 构建 map → 组装 DTO
   - Repository 批量方法返回 `map[id][]item` 分组结构

4. **工具函数**：
   - `sql.NullString`/`sql.NullInt64` → `*string`/`*int64`：用 `util.NullStringToPointer`/`util.NullInt64ToPointer`
   - `string`/`int64` → `*string`/`*int64`（非空/非零时返回指针）：用 `util.StringPtrIfValid`/`util.Int64PtrIfValid`
   - 不要凭空假设 util 函数存在，先用 Grep 确认

### 4. 实现顺序

推荐按依赖关系从底向上实现：

1. **DTO**：`backend/base/model/dto/` 中新增或修改 DTO 结构体
2. **Repository**：新增批量查询方法（先在 Repository 接口声明，再在具体 repo 实现）
3. **Service 接口**：在调用方模块定义所需的窄接口
4. **Service 实现**：核心业务逻辑，参照 TS 版的数据流
5. **Handler**：薄包装，调用 Service 返回 `ApiResponse`
6. **app.go**：构造函数注入新依赖
7. **前端 wrapper**：更新 TypeScript wrapper 适配新接口签名
8. **bindings**：运行 `wails3 generate bindings` 重新生成

### 5. 验证清单

每完成一个模块后：
- [ ] `CGO_ENABLED=0 go build .` 编译通过
- [ ] 无 N+1 查询（Grep 检查循环中的数据库调用）
- [ ] DTO 未嵌入实体
- [ ] Service 未直接导入 `backend/database`
- [ ] 接口由调用方定义
- [ ] 前端 TypeScript 编译无新增错误

## 常见陷阱

### 陷阱 1：直接翻译 TS 语法

```typescript
// TS 版 - 箭头函数 + 链式调用
const result = items.map(i => transform(i)).filter(i => i !== null)
```

```go
// 错误 - Go 没有泛型 map/filter
result := Map(items, func(i) { ... }).Filter(...)

// 正确 - 普通 for 循环
result := make([]*DTO, 0, len(items))
for _, item := range items {
    if dto := transform(item); dto != nil {
        result = append(result, dto)
    }
}
```

### 陷阱 2：忽略 DTO 构造规则

```go
// 错误 - 直接嵌入实体
type WorkFullDTO struct {
    entity.Work        // 禁止
    Resources []*entity.Resource  // 禁止
}

// 正确 - 使用 DTO 组合
type WorkFullDTO struct {
    Work       *WorkDTO       `json:"work"`
    Resources  []*ResourceDTO `json:"resources,omitempty"`
}
```

### 陷阱 3：假设 util 函数存在

在写 `util.SomeFunction()` 之前，先 Grep 确认它存在于 `backend/util/` 目录。
如果不存在，检查是否有功能等价的已有函数，或使用标准库实现。

### 陷阱 4：忘记更新 Service 的 Repository 接口

在 Repository 具体实现中添加新方法后，Service 中的 Repository 接口定义也需要同步声明，
否则 Go 编译器会报 "does not implement" 错误。

### 陷阱 5：前端 wrapper 未适配接口变更

Go Handler 签名变更后（如单条 → 批量），需要：
1. 运行 `wails3 generate bindings` 更新 bindings
2. 更新 `frontend/src/apis/http/wrappers/` 中对应 wrapper 的调用方式和返回值处理
3. 检查使用该 wrapper 的 Vue 组件是否需要适配
