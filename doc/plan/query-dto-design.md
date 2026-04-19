# Query DTO 设计计划

## 1. 需求概述

### 1.1 目标
设计一套能够被 JSON 反序列化的 DTO 数据结构，用于前端构建 SQL 查询条件（WHERE 和 ORDER BY），并提供对应的反序列化与转换逻辑，将 DTO 转换为 GORM 的 `clause.Expression`。

### 1.2 使用场景
- 前端通过 JSON 构建查询条件
- 后端接收 JSON 后反序列化为 DTO，再转换为 GORM clause 使用
- **限制**：后端构建 SQL 时不允许使用此 DTO

### 1.3 需求范围
| 功能 | 优先级 | 说明 |
|-----|-------|------|
| WHERE 条件 | P0 | 支持 AND/OR 嵌套逻辑组合 |
| ORDER BY 排序 | P0 | 支持多字段排序 |
| 操作符支持 | P0 | eq, ne, gt, gte, lt, lte, like, in, is_null, is_not_null |
| 泛型约束 | P0 | 使用 Go 泛型约束 LogicalGroup 的条件类型 |
| JSON 多态反序列化 | P0 | 支持 conditions 中混合 Condition 和嵌套 LogicGroup |

---

## 2. 技术方案

### 2.1 文件结构

```
pkg/query/
├── dto.go           # DTO 数据结构定义（含泛型）
├── dto_test.go      # DTO 单元测试
└── builder.go       # 反序列化与转换逻辑
```

### 2.2 目录定位

选择 `pkg/query/` 而非 `internal/`：
- `pkg/` 表示公共包，可被外部引用
- `internal/` 仅限内部使用
- 此 DTO 需被前端调用（通过 Wails），属于公共接口

### 2.3 核心数据结构

#### 2.3.1 操作符定义

```go
type Operator string

const (
    OpEq        Operator = "eq"
    OpNe        Operator = "ne"
    OpGt        Operator = "gt"
    OpGte       Operator = "gte"
    OpLt        Operator = "lt"
    OpLte       Operator = "lte"
    OpLike      Operator = "like"
    OpIn        Operator = "in"
    OpIsNull    Operator = "is_null"
    OpIsNotNull Operator = "is_not_null"
)
```

#### 2.3.2 Condition（单个条件）

```go
type Condition struct {
    Field    string      `json:"field"`
    Operator Operator    `json:"operator"`
    Value    interface{} `json:"value"`
}
```

#### 2.3.3 LogicGroup（逻辑组合树）

使用 Go 泛型约束条件类型，支持递归嵌套：

```go
type LogicalItem any

type LogicGroup[T LogicalItem] struct {
    Type       string `json:"type"`       // "and" 或 "or"
    Conditions []T    `json:"conditions"` // T 只能是 Condition 或 *LogicGroup[Condition]
}
```

#### 2.3.4 WhereDTO

直接使用 `LogicGroup[Condition]` 作为 WHERE 子句：

```go
type WhereDTO = LogicGroup[Condition]
```

#### 2.3.5 OrderDTO（排序）

```go
type SortOrder string

const (
    OrderAsc  SortOrder = "asc"
    OrderDesc SortOrder = "desc"
)

type OrderDTO struct {
    Field string    `json:"field"`
    Order SortOrder `json:"order"`
}
```

#### 2.3.6 QueryDTO（完整查询）

```go
type QueryDTO struct {
    Where *WhereDTO  `json:"where,omitempty"`
    Order []OrderDTO `json:"order,omitempty"`
}
```

### 2.4 JSON 格式示例

```json
{
  "where": {
    "type": "and",
    "conditions": [
      {"field": "status", "operator": "eq", "value": 1},
      {
        "type": "or",
        "conditions": [
          {"field": "name", "operator": "like", "value": "Alice"},
          {"field": "email", "operator": "like", "value": "alice@example.com"}
        ]
      }
    ]
  },
  "order": [
    {"field": "create_time", "order": "desc"},
    {"field": "update_time", "order": "asc"}
  ]
}
```

---

## 3. 实现细节

### 3.1 多态反序列化实现

由于 Go 的 `encoding/json` 无法自动区分泛型类型，需自定义 `UnmarshalJSON` 方法：

```go
func (lg *LogicGroup[Condition]) UnmarshalJSON(data []byte) error {
    // 1. 解析基础结构（type 和 raw conditions）
    // 2. 遍历 conditions，逐个判断类型：
    //    - 如果是对象且包含 "type" 和 "conditions" 字段 -> 递归解析为 *LogicGroup[Condition]
    //    - 否则解析为 Condition
    // 3. 返回解析结果
}
```

### 3.2 GORM Clause 转换逻辑

```go
type QueryBuilder struct{}

func (b *QueryBuilder) BuildWhere(dto *WhereDTO) ([]clause.Expression, error)
func (b *QueryBuilder) BuildOrder(dtos []OrderDTO) []clause.Expression
```

转换映射关系：

| Operator | GORM Clause |
|----------|-------------|
| eq | `clause.Eq{Column, Value}` |
| ne | `clause.Neq{Column, Value}` |
| gt | `clause.Gt{Column, Value}` |
| gte | `clause.Gte{Column, Value}` |
| lt | `clause.Lt{Column, Value}` |
| lte | `clause.Lte{Column, Value}` |
| like | `clause.Like{Column, Value}` (自动添加 %) |
| in | `clause.IN{Column, Values}` |
| is_null | `clause.Expr{SQL: "? IS NULL", Vars: []}` |
| is_not_null | `clause.Expr{SQL: "? IS NOT NULL", Vars: []}` |

逻辑组合：
- `type: "and"` -> `clause.And(subExprs...)`
- `type: "or"` -> `clause.Or(subExprs...)`

---

## 4. 开发任务

### 4.1 任务列表

| # | 任务 | 描述 | 优先级 |
|---|------|------|--------|
| 1 | 创建 pkg/query/ 目录 | 确定包路径 | P0 |
| 2 | 实现 dto.go | 定义数据结构 | P0 |
| 3 | 实现 builder.go | 实现反序列化与转换逻辑 | P0 |
| 4 | 编写单元测试 | 覆盖核心功能 | P0 |

### 4.2 验收标准

1. **JSON 反序列化正确**
   - `{"field": "status", "operator": "eq", "value": 1}` -> `Condition{Field: "status", Operator: OpEq, Value: 1}`
   - 嵌套的 LogicGroup 能正确解析

2. **GORM Clause 转换正确**
   - 简单条件转换正确
   - AND/OR 嵌套逻辑转换正确

3. **单元测试覆盖**
   - `BuildWhere` 方法覆盖
   - `BuildOrder` 方法覆盖
   - 边界条件测试

---

## 5. 文件清单

| 文件 | 操作 | 说明 |
|------|------|------|
| `pkg/query/dto.go` | 新增 | DTO 数据结构定义 |
| `pkg/query/dto_test.go` | 新增 | DTO 单元测试 |
| `pkg/query/builder.go` | 新增 | 反序列化与转换逻辑 |

---

## 6. 依赖说明

| 依赖 | 版本 | 用途 |
|------|------|------|
| gorm.io/gorm | ^1.31 | ORM 框架 |
| gorm.io/gorm/clause | - | SQL 字句构建 |

---

## 7. 后续扩展（预留）

以下功能暂不实现，记录于此以便后续扩展：

| 功能 | 说明 |
|------|------|
| SELECT 字句 | 指定返回字段 |
| JOIN 字句 | 关联查询 |
| GROUP BY | 分组聚合 |
| HAVING | 分组后过滤 |
| LIMIT/OFFSET | 分页控制 |
| 字段白名单验证 | SQL 注入防护 |
