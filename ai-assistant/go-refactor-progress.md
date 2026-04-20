# Go 主进程重构进度记录

## 概述

本项目将 Electron 主进程从 Node.js 重构为 Go 语言，以提高性能并保持代码一致性。

**当前分支**: `go-refactor`
**工作目录**: `E:\code\lvfeng\LibrarySquirrel\src\main-go`

---

## 一、已完成项

### 1.1 基础设施层

| 功能 | 文件 | 状态 |
|------|------|------|
| 统一配置管理 | `config.yaml` | ✅ 完成 |
| 配置加载 | `internal/config/config.go` | ✅ 完成 |
| 数据库连接 (GORM) | `internal/database/db.go` | ✅ 完成 |
| GORM AutoMigrate | `internal/migration/migrate.go` | ✅ 完成 |
| BaseRepository[T] 接口 | `internal/database/base_repository.go` | ✅ 完成 |
| BaseRepository[T] 实现 | `internal/database/base_repository_impl.go` | ✅ 完成 |
| Transaction 封装 | `internal/database/transaction.go` | ✅ 完成 |
| 模块依赖注入 | `cmd/server/wire.go` | ✅ 完成 |

### 1.2 共享模型层 (pkg/model)

| 功能 | 文件 | 状态 |
|------|------|------|
| API 响应封装 | `pkg/model/api_response.go` | ✅ 完成 |
| 分页请求/响应 | `pkg/model/base.go` | ✅ 完成 |
| Example 查询构建器 | `pkg/model/example.go` | ✅ 完成 |
| 本地作者扩展类型 | `pkg/model/local_author.go` | ✅ 完成 |
| Entity 接口 & BaseEntity 结构体 | `pkg/model/base.go` | ✅ 完成 |

### 1.3 实体模型架构

**Entity 接口**（所有领域实体必须实现）：
```go
type Entity interface {
    GetID() int64
    SetID(id int64)
    GetCreateTime() int64
    SetCreateTime(time int64)
    GetUpdateTime() int64
    SetUpdateTime(time int64)
}
```

**BaseEntity 结构体**（通过嵌入获得公共字段和方法）：
```go
type BaseEntity struct {
    ID         int64 `gorm:"primaryKey;column:id" json:"id"`
    CreateTime int64 `gorm:"column:create_time" json:"createTime"`
    UpdateTime int64 `gorm:"column:update_time" json:"updateTime"`
}
```

**领域实体示例**（嵌入 BaseEntity + 工厂方法）：
```go
type LocalTag struct {
    *model.BaseEntity
    LocalTagName   string `gorm:"column:local_tag_name" json:"localTagName"`
    BaseLocalTagID int64  `gorm:"column:base_local_tag_id" json:"baseLocalTagId"`
    LastUse        int64  `gorm:"column:last_use" json:"lastUse"`
}

func NewLocalTag() *LocalTag {
    return &LocalTag{
        BaseEntity: &model.BaseEntity{},
    }
}
```

**BaseRepositoryImpl**（通过泛型约束 `T model.Entity` 直接调用 Entity 方法）：
```go
type BaseRepositoryImpl[T model.Entity] struct {
    db *gorm.DB
}

func (r *BaseRepositoryImpl[T]) Save(ctx context.Context, entity *T) error {
    now := util.GetCurrentTimestamp()
    e := *entity
    if e.GetID() == 0 {
        e.SetCreateTime(now)
    }
    e.SetUpdateTime(now)
    return r.db.WithContext(ctx).Create(entity).Error
}
```

| 实体 | 嵌入 BaseEntity | 工厂方法 | 状态 |
|------|-----------------|----------|------|
| LocalTag | ✅ | ✅ NewLocalTag() | ✅ |
| LocalAuthor | ✅ | ✅ NewLocalAuthor() | ✅ |
| SiteTag | ✅ | ✅ NewSiteTag() | ✅ |
| SiteAuthor | ✅ | ✅ NewSiteAuthor() | ✅ |
| Work | ✅ | ✅ NewWork() | ✅ |
| WorkSet | ✅ | ✅ NewWorkSet() | ✅ |
| ReWorkWorkSet | ✅ | ✅ NewReWorkWorkSet() | ✅ |

### 1.4 业务模块

| 模块 | Model | Repository | Service | Handler | HTTP Routes | 接口定义位置 |
|------|-------|------------|---------|---------|-------------|-------------|
| localTag | ✅ | ✅ | ✅ | ✅ | ✅ | service.go |
| localAuthor | ✅ | ✅ | ✅ | ✅ | ✅ | service.go |
| work | ✅ | ✅ | ✅ | ✅ | ✅ | service.go |
| siteTag | ✅ | ✅ | ✅ | ✅ | ✅ | service.go |
| siteAuthor | ✅ | ✅ | ✅ | ✅ | ✅ | service.go |
| site | ✅ | ✅ | ✅ | ✅ | ✅ | service.go |
| workSet | ✅ | ✅ | ✅ | ✅ | ✅ | service.go |
| search | ✅ | ✅ | ✅ | ✅ | ✅ | service.go |
| settings | ✅ | N/A (JSON文件) | ✅ | ✅ | ✅ | service.go |
| **taskManager** | ✅ | N/A | ✅ | ✅ | ✅ | task_executor.go |

> **注**: 所有模块的 `Repository` 接口已在 `service.go` 中定义，遵循"接口由使用者定义"原则

### 1.5 GORM 模型 (18个)

| 模型                    | 外键 | 索引 | 状态 |
|-----------------------|------|------|----|
| Work                  | ✅ | ✅ | ✅  |
| Resource              | ✅ | ✅ | ✅  |
| LocalTag              | ✅ | ✅ | ✅  |
| SiteTag               | ✅ | ✅ | ✅  |
| LocalAuthor           | ✅ | ✅ | ✅  |
| SiteAuthor            | ✅ | ✅ | ✅  |
| Task                  | ✅ | ✅ | ✅  |
| Plugin                | ✅ | ✅ | ✅  |
| ~~AutoExplainPath~~   | ✅ | ✅ | 弃用 |
| Site                  | ✅ | ✅ | ✅  |
| WorkSet               | ✅ | ✅ | ✅  |
| WorkSetResourceRelate | ✅ | ✅ | ✅  |
| WorkAuthorRelate      | ✅ | ✅ | ✅  |
| WorkTagRelate         | ✅ | ✅ | ✅  |
| TaskNodeRelate        | ✅ | ✅ | ✅  |
| SlotBinding           | ✅ | ✅ | ✅  |
| SiteBrowser           | ✅ | ✅ | ✅  |
| PluginTaskUrlListener | ✅ | ✅ | ✅  |

### 1.6 分页参数对齐

- **已完成**: `Paging` 参数已从 `PageRequest` 和 `Page` 中移除
- **Query 转换**: 添加 `PageRequest.ToExample()` 方法，支持将前端查询条件转换为 `Example`

---

## 二、分页参数设计

### 当前结构

```go
// PageRequest - 分页请求（与渲染进程 IPage 保持一致）
type PageRequest struct {
    PageNumber int                   `json:"pageNumber"`
    PageSize   int                   `json:"pageSize"`
    Query      map[string]interface{} `json:"query,omitempty"`
}

// Page - 分页响应
type Page[T any] struct {
    PageNumber   int     `json:"pageNumber"`
    PageSize     int     `json:"pageSize"`
    PageCount    int     `json:"pageCount"`
    DataCount    int64   `json:"dataCount"`
    CurrentCount int     `json:"currentCount"`
    Query        interface{} `json:"query,omitempty"`
    Data         []*T    `json:"data"`
}
```

### Query 操作符支持

| 前缀 | 操作符 | 示例 |
|------|--------|------|
| 无 | `=` | `{"name": "test"}` |
| `!` | `!=` | `{"status!": 0}` |
| `>` | `>` | `{"age>": 18}` |
| `>=` | `>=` | `{"age>=": 18}` |
| `<` | `<` | `{"age<": 65}` |
| `<=` | `<=` | `{"age<=": 65}` |
| `~` | `LIKE` | `{"name~": "%test%"}` |

### 使用示例

```
GET /api/localTag/page?pageNumber=1&pageSize=10&query={"name":"test","status!":0}
```

---

## 三、配置统一

### config.yaml 结构

```yaml
server:
  port: 8080

database:
  driver: sqlite3
  dsn: E:/code/lvfeng/LibrarySquirrel/database/database.db

log:
  level: info
  file: logs/app.log

app:
  workDir: E:/code/lvfeng/LibrarySquirrel

sites:
  - name: nhentai
    url: https://nhentai.net

plugins:
  path: plugins
```

---

## 四、待完成模块

### 4.1 待开发模块

| 优先级 | 模块 | 说明 |
|--------|------|------|
| ~~中~~ | ~~site~~ | ~~站点管理~~ ✅ 已完成 |
| ~~中~~ | ~~task~~ | ~~任务管理~~ ✅ 已完成 |
| ~~中~~ | ~~plugin~~ | ~~插件管理~~ ✅ 已完成 |
| ~~低~~ | ~~workSet~~ | ~~作品集~~ ✅ 已完成 |
| ~~低~~ | ~~search~~ | ~~搜索服务~~ ✅ 已完成 |
| ~~低~~ | ~~settings~~ | ~~设置管理~~ ✅ 已完成 |
| ~~中~~ | ~~taskManager~~ | ~~任务管理器（多协程）~~ ✅ 已完成（2026-04-08） |

### 4.2 待完成功能

| 功能 | 状态    | 说明 |
|------|-------|------|
| HTTP Server 路由注册 | ✅ 完成 | localTag/localAuthor/work/site/task/plugin/workSet/search/settings 模块已完成 |
| Electron IPC 集成 | 已验证可行 | 与渲染进程通信 |
| 插件系统迁移 | ✅ 完成 | Plugin 模块已完成 |
| 任务调度系统 | ✅ 完成 | Task 模块已完成 |
| TaskManager 多协程 | ✅ 完成 | 独立协程模式 + 信号量并发控制 + SSE 推送（2026-04-08） |

---

## 五、技术决策记录

### 5.1 数据库选型

- **驱动**: `gorm.io/driver/sqlite`
- **CGO**: 使用 TDM-GCC-64 解决编译问题
- **迁移**: GORM AutoMigrate 自动维护表结构

### 5.2 模块解耦策略

#### 核心原则：**接口由使用者定义**

**模块间依赖模式**（依赖倒置原则 DIP）：
```
模块 A 定义自己需要的接口（ARepository），只包含 a1, a2 方法
模块 C 定义自己需要的接口（CRepository），只包含 c1, c2 方法
模块 B 的结构体同时实现这两个接口
```

**关键点**：
- 接口由**使用者**定义，不是提供者
- 每个接口只包含使用者需要的最小方法集
- 不定义"大而全"的接口强迫实现者实现不需要的功能

#### 标准模块结构

```
internal/{module}/
├── model.go           # 领域实体（嵌入 BaseEntity + 工厂方法）
├── repository.go      # 数据库操作实现（嵌入 BaseRepositoryImpl）
└── service.go         # 业务逻辑 + 定义依赖接口
```

**service.go 标准结构**：
```go
package moduleA

// 1. 定义当前模块需要的数据库操作接口
// 注意：需要什么方法就写什么方法，不需要的方法就一律不包含
type ARepository interface {
    Save(data string) error
    GetById(id int64) (*Model, error)
    // 只包含模块 A 自己需要的数据库方法
}

// 2. 定义需要的外部模块接口
// 这是模块 A 依赖的外部能力
type BRepository interface {
    FindByID(id int64) (*ExternalModel, error)
    // 只包含模块 A 需要的外部模块能力
}
// ...其他外部模块接口

// 3. 服务结构体
type AService struct {
    aRepo ARepository  // 当前模块的数据库依赖
    bRepo BRepository  // 外部模块的依赖
  // ...其他外部模块依赖
}

// 4. 构造函数（依赖注入）
func NewAService(aRepo ARepository, bRepo BRepository) *AService {
    return &AService{
        aRepo: aRepo,
        bRepo: bRepo,
    }
}

// 5. 业务逻辑
func (s *AService) DoSomething(id int64) error {
    // 调用外部依赖
    extModel, err := s.bRepo.FindByID(id)
    if err != nil {
        return err
    }
    // 调用内部依赖
    return s.aRepo.Save(extModel.Data)
}
```

**repository.go 结构**：
```go
package moduleA

// 嵌入 BaseRepositoryImpl 获得 CRUD 能力
type aRepository struct {
    *database.BaseRepositoryImpl[domain.ModelA]
}

// 实现 ARepository 接口（只实现 A 模块需要的方法）
func (r *aRepository) Save(data string) error {
    // 实现 Save
}

func (r *aRepository) GetById(id int64) (*domain.ModelA, error) {
    // 实现 GetById
}
```

**重要说明**：
- 模块的 `Repository` 接口只声明自己需要的方法
- `repository_impl.go` 嵌入 `BaseRepositoryImpl` 来获得 CRUD 能力

**已完成重构**（2026-04-05）：
- localTag、localAuthor、siteTag、siteAuthor、work、site、workSet、search、settings 模块
- `Repository` 接口已从 `repository.go` 移至 `service.go`
- 遵循"接口由使用者定义"原则，每个接口只包含 service 实际需要的方法

### 5.3 依赖注入

通过构造函数注入接口（ARepository、BRepository），实现松耦合。
- 模块不直接依赖其他模块的实现，而是依赖接口
- 接口由使用者（调用方）定义
- 实现者（被调用方）实现接口

### 5.4 实体模型设计

**决策**: BaseEntity 作为结构体（而非接口），领域实体通过嵌入方式获得公共字段。

| 方案 | 优点 | 缺点 |
|------|------|------|
| BaseEntity 作为结构体 | 字段直接可见，无需反射；嵌入即可继承 gorm 标签 | 方法需使用指针接收者 |
| BaseEntity 作为接口 | 可约束类型行为 | Go 泛型约束限制，无法直接约束 `*T` |

**关键决策**: 所有 Get/Set 方法使用指针接收者 `*BaseEntity`，确保修改有效。

**工厂方法模式**: 所有实体必须通过工厂方法创建，确保 `BaseEntity` 正确初始化：
```go
func NewLocalTag() *LocalTag {
    return &LocalTag{
        BaseEntity: &model.BaseEntity{},
    }
}
```

### 5.6 时间戳自动管理

**决策**: 时间戳（CreateTime/UpdateTime）在 BaseRepositoryImpl 的 Save/Update 方法中自动设置。

通过泛型约束 `T model.Entity` 直接调用 Entity 方法：
```go
func (r *BaseRepositoryImpl[T]) Save(ctx context.Context, entity *T) error {
    now := util.GetCurrentTimestamp()
    e := *entity
    if e.GetID() == 0 {
        e.SetCreateTime(now)
    }
    e.SetUpdateTime(now)
    return r.db.WithContext(ctx).Create(entity).Error
}
```

---

## 六、问题与解决方案

| 问题 | 解决方案 |
|------|----------|
| CGO gcc 找不到 | 安装 TDM-GCC-64 |
| 导入循环依赖 | 将 migrate.go 移至独立 migration 包 |
| 类型推断失败 | 使用显式类型参数 `database.NewBaseRepository[LocalTag](db)` |
| 数据库路径问题 | 使用绝对路径 `E:/code/lvfeng/LibrarySquirrel/database/database.db` |
| Go 泛型 `*T` 不满足接口 | 使用 `e := *entity` 解引用后直接调用接口方法 |
| 实体创建统一管理 | 所有实体通过工厂方法创建，migration 也使用工厂方法 |

---

## 七、最近提交

| 提交 | 描述 |
|------|------|
| xxxxxxxx | refactor(主进程): 新增 localAuthor 模块 (Repository/Service/Handler) |
| xxxxxxxx | refactor(主进程): 新增 work 模块基础结构 (Repository/Service/Handler) |
| xxxxxxxx | refactor(主进程): 抽取模块初始化逻辑到 wire.go |
| 61992ec2 | refactor(主进程): electron启动时将go主进程作为子进程启动 |
| 10dc89ea | refactor(主进程): 补充数据表结构体的索引外键等 |
| ad073582 | refactor(主进程): 开发环境启动时使用node启动子进程的方式启动go主进程，使用gorm的Auto Migration功能自动迁移数据表 |

---

## 八、下一步计划

1. ~~**迁移 site 模块**~~：站点配置管理 ✅ 已完成
2. ~~**迁移 task 模块**~~：任务调度系统 ✅ 已完成
3. ~~**迁移 plugin 模块**~~：插件系统 ✅ 已完成
4. ~~**迁移 workSet 模块**~~：作品集 ✅ 已完成
5. ~~**迁移 search 模块**~~：搜索服务 ✅ 已完成
6. ~~**迁移 settings 模块**~~：设置管理 ✅ 已完成
7. **补充缺失接口**：见第九节"缺失接口清单"
8. **集成 Electron IPC**：创建 preload 桥接
9. **测试验证**：确保与渲染进程正常通信

---

## 九、缺失接口清单

> 更新时间：2026-04-06
> 对比基准：Node.js `src/main/core/MainProcessApi.ts` 中的所有 IPC 通道

### 9.1 尚未实现的模块（完全未开发）

| 模块                               | IPC 通道                                      | 说明             |
|----------------------------------|---------------------------------------------|----------------|
| **Slot**                         | `slot-getAllSlots`                          | 获取所有插槽         |
| **AppLauncher**                  | `appLauncher-openImage`                     | 打开图片           |
| **~~AutoExplainPath~~**          | ~~`autoExplainPath-getListenerPage`~~       | ~~获取监听页面~~（弃用） |
| **~~AutoExplainPath~~**          | ~~`autoExplainPath-getListenerList`~~       | ~~获取监听列表~~（弃用）     |
| **FileSysUtil**                  | `fileSysUtil-dirSelect`                     | 目录选择对话框        |
| **SiteBrowser**                  | `siteBrowser-queryPage`                     | 站点浏览器分页查询      |
| **SiteBrowser**                  | `siteBrowser-open`                          | 打开站点浏览器        |
| **PluginTaskUrlListenerManager** | `pluginTaskUrlListenerManager-listListener` | 插件任务URL监听列表    |
| **SecureStorage**                | `secureStorage-set`                         | 安全存储设置         |
| **SecureStorage**                | `secureStorage-get`                         | 安全存储获取         |
| **SecureStorage**                | `secureStorage-delete`                      | 安全存储删除         |
| **SecureStorage**                | `secureStorage-hasKey`                      | 安全存储是否包含键      |
| **SecureStorage**                | `secureStorage-listKeys`                    | 安全存储键列表        |
| **Test**                         | `test-*` 系列                                 | 测试相关接口（可忽略）    |

### 9.2 模块内缺失的接口

#### reWorkTag 模块
| IPC 通道 | Go 路由 | 状态 |
|----------|---------|------|
| `reWorkTag-link` | `POST /api/reWorkTag/:workId/link` | ✅ 已补充 |
| `reWorkTag-unlink` | `POST /api/reWorkTag/:workId/unlink` | ✅ 已补充 |

#### Task 模块（数据层 - 2026-04-08 职责划分）
| IPC 通道 | Go 路由 | 状态 |
|----------|---------|------|
| `task-createTask` | `POST /api/task/create` | ✅ 已完成 |
| `task-deleteTask` | `POST /api/task/delete` | ✅ 已完成 |
| `task-queryTreeDataPage` | `GET /api/task/treeDataPage` | ✅ 已完成 |
| `task-listChildrenTask` | `GET /api/task/children` | ✅ 已完成 |
| `task-queryChildrenTaskPage` | `GET /api/task/childrenPage` | ✅ 已完成 |
| `task-listSchedule` | `GET /api/task/schedule` | ✅ 已完成 |

#### TaskManager 模块（执行层 - 2026-04-08）
| IPC 通道 | Go 路由 | 状态 |
|----------|---------|------|
| `task-startTaskTree` | `POST /api/taskManager/startTree` | ✅ 已完成 |
| `task-stopTaskTree` | `POST /api/taskManager/stopTree` | ✅ 已完成 |
| `task-pauseTaskTree` | `POST /api/taskManager/pauseTree` | ✅ 已完成 |
| `task-resumeTaskTree` | `POST /api/taskManager/resumeTree` | ✅ 已完成 |
| `task-retryTaskTree` | `POST /api/taskManager/retryTree` | ✅ 已完成 |

#### Plugin 模块（缺少安装/卸载操作）
| IPC 通道 | 说明 | 状态 |
|----------|------|------|
| `plugin-installFromPath` | 从路径安装插件 | ❌ 缺失 |
| `plugin-reinstall` | 重新安装插件 | ❌ 缺失 |
| `plugin-reinstallFromPath` | 从路径重新安装插件 | ❌ 缺失 |
| `plugin-unInstall` | 卸载插件 | ❌ 缺失 |

### 9.3 已完成接口对齐

以下接口已在 Go 中实现，与 Node.js IPC 通道对应关系正确：

| 模块 | IPC 通道 | Go 路由 |
|------|----------|---------|
| localTag | `localTag-*` | `/api/localTag/*` |
| localAuthor | `localAuthor-*` | `/api/localAuthor/*` |
| siteTag | `siteTag-*` | `/api/siteTag/*` |
| siteAuthor | `siteAuthor-*` | `/api/siteAuthor/*` |
| work | `work-*` | `/api/work/*` |
| site | `site-*` | `/api/site/*` |
| workSet | `workSet-*` | `/api/workSet/*` |
| search | `search-*` | `/api/search/*` |
| settings | `settings-*` | `/api/settings/*` |
| reWorkTag | `reWorkTag-*` (batch) | `/api/reWorkTag/*` |
| reWorkWorkSet | `reWorkWorkSet-*` | `/api/reWorkWorkSet/*` |
| task | `task-queryPage`, `task-listStatus` 等基础接口 | `/api/task/*` |
| plugin | `plugin-queryPage`, `plugin-getById` 等基础接口 | `/api/plugin/*` |

### 9.4 缺失接口统计

| 类别 | 数量 |
|------|------|
| 完全未实现的模块 | 13 个接口 |
| Task 模块缺失 | ~~11 个接口~~ → ✅ 已完成 |
| Plugin 模块缺失 | 4 个接口 |
| **合计** | ~~28 个接口~~ → **17 个接口** |
