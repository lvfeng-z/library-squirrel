# Query DTO 设计计划

## 1. 需求概述

### 1.1 目标
设计一套通用的查询 DTO 结构和转换机制，用于替代现有各模块的 QueryDTO，实现：
- 前端构建查询条件
- 后端将 DTO 转换为 `database.PageOption` / `database.QueryOption`
- 复用现有的 `List`、`Page` 等数据库查询方法

### 1.2 使用场景
- 前端通过 JSON 构建查询条件
- 后端解析 DTO 并转换为 GORM clause 表达式
- 非数据库字段由特定业务逻辑处理

### 1.3 需求范围
| 功能 | 优先级 | 说明 |
|-----|-------|------|
| QueryAttribute 定义 | P0 | 值 + 运算符 + 排序组合 |
| query tag 字段映射 | P0 | 通过自定义 tag 映射数据库列名 |
| 字段类型区分 | P0 | query tag 为空 = 非数据库字段 |
| 结构体 → PageOption 转换 | P0 | 转换为现有查询方法所需格式 |
| 嵌套 AND/OR 逻辑 | P0 | 支持复杂查询条件组合 |
| 排序构建 | P0 | Order 不为空的字段纳入排序 |

---

## 2. 技术方案

### 2.1 文件结构

```
pkg/query/
├── dto.go              # 已有：基础 DTO（Condition, WhereDTO, OrderDTO, QueryDTO）
├── builder.go          # 已有：WhereDTO/OrderDTO → clause.Expression
├── field_mapper.go     # 新增：字段映射器（反射获取 query tag）
├── converter.go        # 新增：结构体 → PageOption/QueryOption 转换器
├── converter_test.go   # 新增：转换器单元测试
└── attribute.go        # 新增：QueryAttribute 定义
```

### 2.2 核心数据结构

#### 2.2.1 QueryAttribute（查询属性）

```go
// QueryAttribute 查询属性：值 + 运算符 + 排序
type QueryAttribute struct {
    Value    interface{} `json:"value"`              // 查询值
    Operator Operator    `json:"operator,omitempty"` // 运算符（可省略，默认 eq）
    Order    SortOrder   `json:"order,omitempty"`    // 排序（可省略）
}
```

#### 2.2.2 各模块 QueryDTO 示例

```go
// TaskQueryDTO
type TaskQueryDTO struct {
    TaskName     query.QueryAttribute `json:"taskName" query:"task_name"`
    SiteId       query.QueryAttribute `json:"siteId" query:"site_id"`
    Status       query.QueryAttribute `json:"status" query:"status"`
    CreateTime   query.QueryAttribute `json:"createTime" query:"create_time"`
    // 非数据库字段，query tag 为空
    NonDbField   query.QueryAttribute `json:"nonDbField" query:""`
}
```

#### 2.2.3 query tag 规则

| 字段类型 | query tag 示例 | 说明 |
|---------|---------------|------|
| 数据库字段 | `query:"task_name"` | 参与通用查询条件转换 |
| 非数据库字段 | `query:""` | 跳过通用转换，由业务逻辑处理 |

### 2.3 前端 JSON 示例

```json
{
  "taskName": {"value": "test", "operator": "like", "order": "asc"},
  "siteId": {"value": 123},
  "status": {"value": 1, "order": "desc"},
  "createTime": {"value": null, "order": "asc"},
  "nonDbField": {"value": "special"}
}
```

**处理逻辑**：
- `taskName`: 值非空 + 操作符 like → `task_name LIKE '%test%'`，Order=asc 纳入排序
- `siteId`: 值非空 + 操作符默认 eq → `site_id = 123`，无 Order 不排序
- `status`: 值非空 + 操作符默认 eq → `status = 1`，Order=desc 纳入排序
- `createTime`: 值为 null → 不作为查询条件，但 Order=asc 纳入排序
- `nonDbField`: query tag 为空 → 跳过通用转换

### 2.4 运算符定义（已有）

```go
type Operator string

const (
    OpEq        Operator = "eq"         // =
    OpNe        Operator = "ne"         // !=
    OpGt        Operator = "gt"         // >
    OpGte       Operator = "gte"        // >=
    OpLt        Operator = "lt"         // <
    OpLte       Operator = "lte"        // <=
    OpLike      Operator = "like"       // LIKE（自动加 %）
    OpIn        Operator = "in"         // IN
    OpIsNull    Operator = "is_null"   // IS NULL
    OpIsNotNull Operator = "is_not_null" // IS NOT NULL
)
```

### 2.5 FieldMapper（字段映射器）

```go
// FieldMapper 通过反射获取结构体字段的 query tag 映射
type FieldMapper struct {
    structType reflect.Type
}

// NewFieldMapper 创建字段映射器
func NewFieldMapper(model interface{}) *FieldMapper {
    t := reflect.TypeOf(model)
    if t.Kind() == reflect.Ptr {
        t = t.Elem()
    }
    return &FieldMapper{structType: t}
}

// GetColumnName 获取 query tag 映射的列名
// 返回 (列名, 是否为数据库字段)
func (m *FieldMapper) GetColumnName(fieldName string) (string, bool) {
    field, ok := m.structType.FieldByName(fieldName)
    if !ok {
        return "", false
    }
    colName := field.Tag.Get("query")
    if colName == "" {
        return "", false // 非数据库字段
    }
    return colName, true
}
```

### 2.6 Converter（转换器）

```go
// Converter 结构体 → PageOption/QueryOption 转换器
type Converter struct {
    mapper *FieldMapper
}

func NewConverter(model interface{}) *Converter {
    return &Converter{
        mapper: NewFieldMapper(model),
    }
}

// ToPageOption 将 DTO 转换为 PageOption
func (c *Converter) ToPageOption(dto interface{}, page, pageSize int) (*database.PageOption, error)

// ToQueryOption 将 DTO 转换为 QueryOption
func (c *Converter) ToQueryOption(dto interface{}) (*database.QueryOption, error)
```

### 2.7 转换流程

```
前端 JSON → TaskQueryDTO 结构体
    ↓
Converter.ToQueryOption(dto)
    ↓
遍历 TaskQueryDTO 的所有字段
    ↓
获取字段的 query tag：
    - 为空 → 跳过通用转换（由业务逻辑处理）
    - 非空 → 获取列名映射
    ↓
构建 WhereClause：
    - Field = query tag 值
    - Operator = QueryAttribute.Operator（默认 eq）
    - Value = QueryAttribute.Value
    ↓
收集 Order 不为空的字段 → 构建 OrderBy
    ↓
返回 database.PageOption / QueryOption
```

---

## 3. 开发任务

### 3.1 任务列表

| # | 任务 | 文件 | 优先级 |
|---|------|------|--------|
| 1 | 新增 attribute.go | `pkg/query/attribute.go` | P0 |
| 2 | 新增 field_mapper.go | `pkg/query/field_mapper.go` | P0 |
| 3 | 新增 converter.go | `pkg/query/converter.go` | P0 |
| 4 | 新增 converter_test.go | `pkg/query/converter_test.go` | P0 |

### 3.2 验收标准

1. **QueryAttribute 正确解析**
   - Value、Operator、Order 正确反序列化
   - Operator 省略时默认为 eq

2. **query tag 映射正确**
   - 有 query tag 的字段返回列名
   - query tag 为空的字段返回非数据库字段标识

3. **转换结果正确**
   - 数据库字段转换为 clause.Expression
   - 非数据库字段被正确跳过
   - Order 不为空的字段正确构建排序

4. **与现有查询方法兼容**
   - 转换结果可直接用于 `BaseRepository.Page()`
   - 转换结果可直接用于 `BaseRepository.List()`

---

## 4. 使用示例

### 4.1 定义 QueryDTO

```go
// internal/task/query.go
package task

import "github.com/library-squirrel/wails/pkg/query"

type TaskQueryDTO struct {
    TaskName   query.QueryAttribute `json:"taskName" query:"task_name"`
    SiteId     query.QueryAttribute `json:"siteId" query:"site_id"`
    Status     query.QueryAttribute `json:"status" query:"status"`
    CreateTime query.QueryAttribute `json:"createTime" query:"create_time"`
    // 非数据库字段
    ExtraField query.QueryAttribute `json:"extraField" query:""`
}
```

### 4.2 Service 层使用

```go
func (s *Service) QueryTaskPage(ctx context.Context, page, pageSize int, dto *TaskQueryDTO) (*pkgModel.Page[domain.Task], error) {
    conv := query.NewConverter(domain.Task{})
    pageOpt, err := conv.ToPageOption(dto, page, pageSize)
    if err != nil {
        return nil, err
    }

    // 处理非数据库字段（如需特殊逻辑）
    if dto.ExtraField.Value != nil {
        // 业务特定处理
    }

    return s.repo.Page(ctx, pageOpt)
}
```

---

## 5. 文件清单

| 文件 | 操作 | 说明 |
|------|------|------|
| `pkg/query/attribute.go` | 新增 | QueryAttribute 定义 |
| `pkg/query/field_mapper.go` | 新增 | 字段映射器 |
| `pkg/query/converter.go` | 新增 | 结构体转换器 |
| `pkg/query/converter_test.go` | 新增 | 转换器单元测试 |

---

## 6. 依赖说明

| 依赖 | 版本 | 用途 |
|------|------|------|
| gorm.io/gorm | ^1.31 | ORM 框架 |
| gorm.io/gorm/clause | - | SQL 字句构建 |
| internal/database | - | PageOption, QueryOption |

---
