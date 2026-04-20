# Go 主进程重构规范

## 核心目标

构建**高内聚、低耦合**的模块化系统。

| 核心手段 | 实现方式 |
|---------|---------|
| 消除重复代码 | 泛型抽象：`BaseRepository[T]` 一次编写，全项目复用 |
| 解决循环依赖 | 依赖倒置：接口定义在调用方，实现方隐式满足 |
| 业务模块独立性 | 每个模块独立演进，不影响其他模块 |
| 可测试性 | Service 层依赖接口，可轻松 Mock |

**依赖方向**（严格遵循）：

```
Service → Repository Interface ← Repository Implementation
                              ↑
                       internal/database
```

---

## 三层架构

```
┌─────────────────────────────────────────────────────────────┐
│  共享层 (pkg/model)                                         │
│  ├── 纯数据结构：API Response、Page、Example                 │
│  └── 约束接口：BaseEntity（GetID）                          │
│  ⚠️ 严禁包含任何业务逻辑或数据库依赖                         │
└─────────────────────────────────────────────────────────────┘
                              ↑
┌─────────────────────────────────────────────────────────────┐
│  基础设施层 (internal/database)                              │
│  ├── BaseRepository[T] 泛型基础仓储                         │
│  ├── Transaction 事务封装                                   │
│  └── Example 查询条件构建器                                  │
└─────────────────────────────────────────────────────────────┘
                              ↑
┌─────────────────────────────────────────────────────────────┐
│  业务层 (internal/{module})                                 │
│  ├── model.go        领域实体                               │
│  ├── repository.go   Repository 接口（嵌入 BaseRepository）  │
│  ├── repository_impl.go  数据库实现                         │
│  └── service.go     业务逻辑（依赖注入）                     │
└─────────────────────────────────────────────────────────────┘
                              ↑
┌─────────────────────────────────────────────────────────────┐
│  组装层 (cmd/server/main.go)                                │
│  └── 依赖注入：new Service(repo, externalDeps)              │
└─────────────────────────────────────────────────────────────┘
```

---

## 项目目录结构

```
my-ipc-service/
├── cmd/
│   └── server/
│       └── main.go              # 程序入口：组装依赖并启动服务
├── internal/
│   ├── config/                  # [基础设施] 程序配置 (viper/env)
│   ├── database/                # [基础设施] 数据库连接、迁移、事务管理
│   │
│   # --- 业务领域模块 ---
│   ├── site/                    # 站点
│   │   ├── model.go            # 领域实体
│   │   ├── repository.go       # 数据存取接口
│   │   ├── repository_impl.go  # 数据库实现
│   │   └── service.go          # 业务逻辑
│   ├── author/                  # 作者
│   ├── localTag/                # 本地标签
│   ├── localAuthor/             # 本地作者
│   ├── work/                    # 作品（含 link/unlink 逻辑）
│   ├── workSet/                 # 作品集
│   │
│   # --- 基础设施/功能模块 ---
│   ├── plugin/                  # 插件系统
│   ├── task/                    # 后台任务/调度
│   ├── search/                  # 搜索逻辑
│   ├── settings/                # 业务设置
│   ├── secureStorage/           # 密钥/安全存储
│   ├── appLauncher/             # 外部应用启动
│   ├── autoExplainPath/         # 自动解释路径
│   ├── slot/                    # Slot 同步
│   ├── siteBrowser/             # 站点浏览器
│   ├── pluginTaskUrlListener/   # 插件任务 URL 监听
│   │
│   # --- 公共基础设施 ---
│   ├── error/                   # 自定义错误类型
│   └── util/                   # 纯工具函数 (无状态)
├── pkg/
│   └── model/                   # 全局共享 DTO (PageRequest, APIResponse 等)
└── go.mod
```

---

## 3. 模块独立性原则

### 核心原则

**模块之间不产生依赖**。每个模块只依赖通用 CRUD Repository（通过依赖注入）。

```
模块A ──┐
        ├──→ Repository Interface ←─── Repository Implementation
模块B ──┘                      ↑
                           database
```

### 跨模块操作的下沉处理

| 原 Node.js | Go 迁移 |
|-----------|---------|
| WorkService → ReWorkTagService.link() | **下沉到 Work 模块内部** |
| WorkService → ReWorkAuthorService.link() | **下沉到 Work 模块内部** |
| SearchService → WorkService | **不依赖**，各自使用 Example 查询 |

**好处**：
- 模块之间无依赖，可以独立演进
- 消除循环依赖风险
- 每个模块的内聚性更高

### 🔴 禁止的模块依赖

以下情况是**禁止**的：

| 禁止 | 示例 |
|------|------|
| 业务模块之间直接调用 | `author` 包 import `work` 包 |
| Service 层调用其他 Service | `WorkService` 调用 `TagService` |

---

## 4. database 包的范围

**职责范围**（必须）：
- 数据库连接初始化（基于 Go 原生 `database/sql`）
- 连接池参数配置（`SetMaxOpenConns`, `SetMaxIdleConns`, `SetConnMaxLifetime`）
- 迁移脚本执行
- 事务上下文管理（封装 `db.BeginTx()`）

**禁止行为**：
- ❌ 业务层直接 import `internal/database` 写 SQL
- ❌ 在 `database` 包内实现业务逻辑
- ❌ 手动实现连接池（Go `database/sql` 已自带）

**正确做法**：
```
业务模块 (如 author)
  ↓ 依赖接口
author/repository.go (接口定义)
  ↓ 实现
author/repository_impl.go (import database, 实现 SQL)
```

**示例**：

```go
// internal/database/db.go
package database

import (
    "database/sql"
    "time"
    _ "github.com/mattn/go-sqlite3"
)

type DB struct {
    *sql.DB
}

func Open(dsn string) (*DB, error) {
    db, err := sql.Open("sqlite3", dsn)
    if err != nil {
        return nil, err
    }
    db.SetMaxOpenConns(25)
    db.SetMaxIdleConns(5)
    db.SetConnMaxLifetime(5 * time.Minute)
    return &DB{db}, nil
}

// Transaction 封装事务
func (db *DB) Transaction(fn func(*sql.Tx) error) error {
    tx, err := db.Begin()
    if err != nil {
        return err
    }
    if err := fn(tx); err != nil {
        tx.Rollback()
        return err
    }
    return tx.Commit()
}
```

---

## 5. model 的边界划分

### pkg/model - 共享层（中立区）

**定位**：跨模块共享的纯数据结构，切断业务模块间的直接类型引用。

**作用**：作为"中立区"，从物理上杜绝循环依赖。如果 A 模块需要引用 B 模块的某个类型，不能直接引用（会循环），而是把这个类型提升到 `pkg/model`。

**存放内容**：
- API 响应格式 (`APIResponse<T>`)
- 全局分页结构 (`PageRequest`, `PageResponse`)
- Example 查询条件构建器
- BaseEntity 约束接口
- 全局枚举常量

**原则**：
- ❌ 严禁包含任何业务逻辑
- ❌ 严禁依赖 `internal/database`
- ❌ 严禁依赖任何业务模块

### internal/{module}/model - 领域实体

存放本模块独有的实体定义及其业务方法：

```go
// internal/author/model.go
package author

type Author struct {
    ID   int64
    Name string
}

// GetID 实现 BaseEntity 接口
func (a *Author) GetID() int64 {
    return a.ID
}

// IsValid 业务方法 - 与领域相关但不需要外部依赖
func (a *Author) IsValid() bool {
    return a.Name != ""
}
```

**原则**：
- 实体及其业务方法必须在同一模块内
- 不得跨模块引用其他实体的业务方法
- 如需被其他模块引用类型，考虑提升到 `pkg/model`

---

## 6. config vs settings 的区分

| 目录 | 职责 | 内容示例 |
|------|------|----------|
| `internal/config` | 程序级配置 | 数据库连接、服务器端口、插件根目录路径、日志级别 |
| `internal/settings` | 业务设置 | 用户偏好、主题、默认下载路径、资源库根目录 |

**加载优先级**：
1. 环境变量 (最高)
2. 命令行参数
3. 配置文件 (config.yaml)
4. 默认值 (最低)

---

## 7. Repository 模式实施

### 7.1 包内聚合结构

采用 **"包内聚合"** 方式，将接口定义、实现和领域模型放在同一个业务包内：

```
internal/author/
├── model.go             # [领域实体] 纯数据结构
├── repository.go        # [接口定义] Repository 接口 (Port)
├── repository_impl.go   # [接口实现] 具体的数据库操作 (Adapter)
└── service.go          # [业务逻辑] 核心业务规则
```

### 7.2 领域实体 (model.go)

**职责**：定义数据结构和验证方法。

**规范**：
- 不包含业务逻辑（如"计算折扣"）
- 仅包含基础的数据验证方法（如 `Validate()`）
- 严禁引用其他业务模块的实体
- 必须实现 `GetID()` 方法供泛型基础仓储使用

```go
package author

// Author 领域实体
type Author struct {
    ID   int64  `json:"id"`
    Name string `json:"name"`
}

func (a *Author) GetID() int64 {
    return a.ID
}

// IsValid 验证作者是否有效
func (a *Author) IsValid() bool {
    return a.Name != ""
}
```

### 7.3 泛型基础仓储接口 (internal/database/base_repository.go)

**职责**：提供通用的 CRUD 方法，所有业务 Repository 自动获得这些能力。

```go
package database

import "context"

// BaseEntity 定义所有实体必须包含的基础字段
type BaseEntity interface {
    GetID() int64
}

// Example 用于动态构建查询条件
type Example struct {
    Where  []Condition  // AND 条件
    Or     []Condition  // OR 条件（与 Where 是 OR 关系）
    OrderBy []OrderField // 排序
    Limit  int          // 限制返回条数
    Offset int          // 偏移量
}

type Condition struct {
    Field    string      // 字段名
    Op       string      // 操作符: =, !=, >, <, >=, <=, LIKE, IN, IS_NULL, IS_NOT_NULL
    Value    interface{} // 值
}

type OrderField struct {
    Field string // 字段名
    Asc  bool   // true=ASC, false=DESC
}

// BaseRepository 通用 CRUD 接口
type BaseRepository[T BaseEntity] interface {
    // Save 插入或更新（根据 ID 是否为 0 决定）
    Save(ctx context.Context, entity *T) error
    // SaveBatch 批量插入
    SaveBatch(ctx context.Context, entities []*T) error
    // Delete 根据 ID 删除
    Delete(ctx context.Context, id int64) error
    // DeleteBatch 根据 ID 批量删除
    DeleteBatch(ctx context.Context, ids []int64) error
    // Update 更新非零字段
    Update(ctx context.Context, entity *T) error
    // UpdateBatch 批量更新
    UpdateBatch(ctx context.Context, entities []*T) error
    // GetById 根据 ID 查询
    GetById(ctx context.Context, id int64) (*T, error)
    // Get 根据 Example 查询，返回单条
    Get(ctx context.Context, example *Example) (*T, error)
    // List 根据 Example 查询，返回列表
    List(ctx context.Context, example *Example) ([]*T, error)
}
```

### 7.4 业务模块 Repository 接口 (repository.go)

**职责**：定义业务模块特有的数据存取方法。

**规范**：
- 接口定义在调用方（Service 所在的包）
- 方法签名必须使用 `context.Context`
- 返回领域实体或 DTO，严禁返回 `*gorm.DB` 或 `sql.Rows`

```go
package author

import "context"

// Repository 定义了 Author 模块的数据存取契约
// 嵌入 BaseRepository[Author] 自动获得基础 CRUD 能力
type Repository interface {
    BaseRepository[Author]  // 基础 CRUD

    // 特有方法
    FindByName(ctx context.Context, name string) (*Author, error)
    ListByConditions(ctx context.Context, name string, status int) ([]*Author, error)
}
```

### 7.5 业务逻辑 (service.go)

**职责**：处理业务流程、事务控制。

**规范**：
- 依赖注入：通过构造函数注入 Repository 接口
- 面向接口编程：不知道底层是 SQLite 还是其他数据库

```go
package author

type Service struct {
    repo Repository
}

func NewService(repo Repository) *Service {
    return &Service{repo: repo}
}

func (s *Service) Register(ctx context.Context, name string) (*Author, error) {
    if name == "" {
        return nil, ErrNameEmpty
    }
    author := &Author{Name: name}
    if err := s.repo.Save(ctx, author); err != nil {
        return nil, err
    }
    return author, nil
}
```

### 7.6 数据库实现 (repository_impl.go)

**职责**：将接口调用转换为具体的 SQL 操作。

**规范**：
- 这是唯一允许 import `internal/database` 的地方
- 实现结构体通常命名为 `repositoryImpl`
- 必须实现 Repository 接口

```go
package author

import (
    "context"
    "database/sql"
    "strings"

    "my-ipc-service/internal/database"
)

type repositoryImpl struct {
    db *database.DB
}

func NewRepository(db *database.DB) Repository {
    return &repositoryImpl{db: db}
}

// FindByName 特有查询实现
func (r *repositoryImpl) FindByName(ctx context.Context, name string) (*Author, error) {
    example := &database.Example{
        Where: []database.Condition{
            {Field: "name", Op: "=", Value: name},
        },
    }
    return r.repo.Get(ctx, example)
}

// ListByConditions 特有查询实现
func (r *repositoryImpl) ListByConditions(ctx context.Context, name string, status int) ([]*Author, error) {
    example := &database.Example{
        Where: []database.Condition{
            {Field: "name", Op: "LIKE", Value: "%" + name + "%"},
            {Field: "status", Op: "=", Value: status},
        },
        OrderBy: []database.OrderField{
            {Field: "id", Asc: false},
        },
    }
    return r.repo.List(ctx, example)
}
```

### 7.7 Example 查询构建示例

```go
// 单表简单查询
example := &Example{
    Where: []Condition{
        {Field: "name", Op: "=", Value: "张三"},
        {Field: "age", Op: ">=", Value: 18},
    },
    OrderBy: []OrderField{
        {Field: "created_at", Asc: false},
    },
    Limit: 10,
    Offset: 0,
}

// 模糊查询
example := &Example{
    Where: []Condition{
        {Field: "title", Op: "LIKE", Value: "%关键词%"},
    },
}

// IN 查询
example := &Example{
    Where: []Condition{
        {Field: "status", Op: "IN", Value: []int{1, 2, 3}},
    },
}

// NULL 判断
example := &Example{
    Where: []Condition{
        {Field: "deleted_at", Op: "IS_NULL", Value: nil},
    },
}

// OR 条件
example := &Example{
    Where: []Condition{
        {Field: "name", Op: "=", Value: "张三"},
    },
    Or: []Condition{
        {Field: "name", Op: "=", Value: "李四"},
    },
}
```

---

## 8. 模块间解耦：接口通信模式

**核心原则**：模块之间通过接口通信，不直接依赖。

```
调用方模块A ──→ 接口（定义在A或公共层）
                  ↑
实现方模块B ──────┘（实现A所需的接口）
                    ↑
              组装层注入
```

**接口设计模式（适用于所有模块）**：

| 步骤 | 说明 |
|------|------|
| 1. 定义接口 | 调用方在 `interfaces.go` 中定义所需接口 |
| 2. 实现接口 | 被调用方实现接口 |
| 3. 注入 | 组装层（cmd/server/main.go）把实现注入给调用方 |

**示例：Search 调用 Work**

```go
// 1. 调用方 Search 定义接口
// internal/search/interfaces.go
package search

type WorkFinder interface {
    FindByConditions(ctx context.Context, cond *Example) ([]*Work, error)
}

// 2. Search 使用接口
// internal/search/service.go
package search

type Service struct {
    workFinder WorkFinder
}

func NewService(finder WorkFinder) *Service {
    return &Service{workFinder: finder}
}

// 3. Work 实现接口
// internal/work/service.go
func (s *Service) FindByConditions(ctx context.Context, cond *Example) ([]*Work, error) {
    return s.repo.List(ctx, cond)
}

// 4. 组装层注入
// cmd/server/main.go
workService := work.NewService(workRepo)
searchService := search.NewService(workService) // 注入
```

**好处**：
- 模块之间无直接依赖，可以独立演进
- 避免循环依赖
- 可测试性强：测试时可以传入 Mock 实现

---

## 9. 组装与依赖注入 (cmd/server/main.go)

所有具体的依赖关系必须在 main.go 中组装。这是唯一发生"具体类型耦合"的地方。

```go
func main() {
    // 1. 初始化配置
    cfg := config.Load()

    // 2. 初始化数据库（使用 Go 原生连接池）
    db, err := database.Open(cfg.DSN)
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()

    // 3. 初始化 Repository (实现层)
    authorRepo := author.NewRepository(db)
    workRepo := work.NewRepository(db)

    // 4. 初始化 Service (业务层)
    workService := work.NewService(workRepo)
    // 注意：将 workService 注入给 authorService 以满足 WorkProvider 接口
    authorService := author.NewService(authorRepo, workService)

    // 5. 注册 IPC handlers...

    // 6. 启动服务...
}
```

---

## 10. 检查清单

在提交代码前，请检查：

- [ ] 目录结构：是否遵循 model, repository, service 分离？
- [ ] 依赖方向：Service 是否只引用了 Repository 接口？
- [ ] 数据库隔离：Service 层是否完全没有 import `internal/database`？
- [ ] 循环依赖：是否存在 A→B→A 的依赖？（如有，请改为接口）
- [ ] 上下文：所有 Repository 方法是否都接收了 `context.Context`？
- [ ] config/settings：是否正确区分了程序配置和业务设置？
- [ ] util 包：是否是无状态的纯函数？
