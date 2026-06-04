# PersistentStore 模块设计方案

## 概念定义

PersistentStore 是一个拥有独立数据库表（`persistent_store`）的文件存取模块，负责所有需要长期存储的文件的**记录管理 + 磁盘文件管理**，保证数据库记录与磁盘文件的严格一致性。

### 核心职责

- 对外提供统一的文件存取接口：存入文件 → 返回记录 ID，按 ID/路径获取记录
- 保证数据库记录与磁盘文件的**严格一致**：记录在则文件在，记录删则文件删
- 不依赖任何业务模块，是纯粹的文件存取基础设施

### 与 Resource 模块的区别

| | Resource | PersistentStore |
|---|---|---|
| 归属 | Work 的资源文件 | 所有需长期存储的文件 |
| 数据库 | `resource` 表（含 `work_id`） | `persistent_store` 表（无业务外键） |
| 依赖 | 依赖 Work（`work_id`） | 不依赖任何业务模块 |
| 引用方式 | 其他模块通过 `work_id` 关联 | 其他模块存储 `persistent_store_id` |
| 未来 | 逐步将文件管理委托给 PersistentStore | 成为唯一的文件存取基础设施 |

## 子目录管理

### 设计目标

虽然文件存储路径由调用方决定，但为了避免目录混乱，所有子目录必须显式声明。PersistentStore 在 `Store` 时校验路径前缀是否匹配已注册的子目录，拒绝未声明的路径。

### 子目录枚举

```go
// backend/persistentStore/dir.go

// StoreDir 存储子目录定义
// Path 为相对于 {workDir}/store/ 的路径
// 多级子目录通过 "/" 分隔声明，如 "avatar/local"
type StoreDir struct {
    Path        string
    Description string
}

// 已注册的存储子目录
var registeredDirs = []StoreDir{
    {Path: "resource", Description: "作品资源文件（迁移过渡用）"},
    {Path: "thumbnail", Description: "视频缩略图"},
    {Path: "avatar/local", Description: "本地作者头像"},
    {Path: "avatar/site", Description: "站点作者头像"},
}

// validatePath 校验路径是否以已注册子目录开头
// relPath: 如 "resource/author/video.mp4"、"avatar/local/123.jpg"
func validatePath(relPath string) error {
    for _, dir := range registeredDirs {
        if relPath == dir.Path || strings.HasPrefix(relPath, dir.Path+"/") {
            return nil
        }
    }
    return fmt.Errorf("路径 %q 未匹配任何已注册子目录", relPath)
}
```

### 多级子目录支持

注册的子目录可以是多级的（如 `avatar/local`、`avatar/site`），调用方在此基础上可以继续创建动态子目录：

```
已注册子目录: "resource"
调用方路径:    "resource/某作者名/视频.mp4"       ← "某作者名" 是动态层级

已注册子目录: "avatar/local"
调用方路径:    "avatar/local/123.jpg"              ← 无额外动态层级

已注册子目录: "thumbnail"
调用方路径:    "thumbnail/某作者名/视频_thumb.jpg" ← "某作者名" 是动态层级
```

校验规则：路径必须以某个已注册子目录为前缀（精确匹配或后接 `/`）。

### 磁盘目录结构

```
{workDir}/
  store/                           — PersistentStore 根目录
    resource/                      — 作品资源（阶段二迁移用）
    thumbnail/                     — 缩略图
    avatar/                        — 头像
      local/                       — 本地作者头像
      site/                        — 站点作者头像
  resource/                        — 现有 Work 资源（阶段一保持不变）
```

## 数据模型

### persistent_store 表

```go
// backend/base/model/entity/persistent_store.go

type PersistentStore struct {
    *model.BaseEntity
    FilePath          sql.NullString `gorm:"column:file_path;uniqueIndex" json:"filePath"`
    FileName          sql.NullString `gorm:"column:file_name" json:"fileName"`
    FilenameExtension sql.NullString `gorm:"column:filename_extension" json:"filenameExtension"`
    FileSize          sql.NullInt64  `gorm:"column:file_size" json:"fileSize"`
}

func (PersistentStore) TableName() string {
    return "persistent_store"
}

func NewPersistentStore() *PersistentStore {
    return &PersistentStore{
        BaseEntity: &model.BaseEntity{},
    }
}
```

**字段说明**：

| 字段 | 说明 |
|------|------|
| `file_path` | 文件相对于 `{workDir}/store/` 的相对路径，**唯一索引**，由调用方指定 |
| `file_name` | 原始文件名（展示用） |
| `filename_extension` | 文件扩展名（如 `.jpg`、`.mp4`） |
| `file_size` | 文件大小（字节） |

**设计决策**：

- `file_path` 为唯一索引：同一相对路径只能有一条记录，保证记录与文件一一对应
- 无 `category` 字段：调用方通过路径前缀组织，由子目录枚举约束
- 无任何业务外键：不依赖 Work、Author 等模块

### 其他实体的引用方式

各业务实体通过存储 `persistent_store_id` 引用文件，具体字段由各业务模块的计划定义（见独立计划）。

## 模块结构

```
backend/persistentStore/
  dir.go              — 子目录枚举与校验
  handler.go          — Wails Bind 方法
  service.go          — 业务逻辑（接口定义 + 实现）
  repository.go       — 数据访问
```

## 接口设计

### Repository 接口

```go
// Repository 数据访问接口
type Repository interface {
    // Save 保存记录（BaseRepository.Save 模式：Create 后通过指针回填 ID）
    Save(ctx context.Context, store *entity.PersistentStore) error
    Update(ctx context.Context, store *entity.PersistentStore) error
    GetById(ctx context.Context, id int64) (*entity.PersistentStore, error)
    GetByFilePath(ctx context.Context, filePath string) (*entity.PersistentStore, error)
    Delete(ctx context.Context, id int64) error
    ExistsByFilePath(ctx context.Context, filePath string) bool
}
```

Repository 实现嵌入 `*database.BaseRepository[entity.PersistentStore]`，`Save`/`Update`/`Delete`/`GetById` 由 BaseRepository 提供，`GetByFilePath` 和 `ExistsByFilePath` 为自定义方法。参考 `backend/resource/repository.go` 的模式。

### Service 接口

```go
// Service 文件存取服务接口
type Service interface {
    // Store 存入文件
    // relPath: 相对于 {workDir}/store/ 的路径（由调用方指定，必须匹配已注册子目录）
    // fileName: 原始文件名
    // reader: 文件内容
    // 返回 persistent_store 记录 ID
    Store(ctx context.Context, relPath string, fileName string, reader io.Reader) (int64, error)

    // StoreFromFile 从本地文件存入
    // relPath: 相对于 {workDir}/store/ 的路径
    // fileName: 原始文件名
    // srcAbsPath: 源文件的绝对路径
    StoreFromFile(ctx context.Context, relPath string, fileName string, srcAbsPath string) (int64, error)

    // GetById 根据 ID 获取记录
    GetById(ctx context.Context, id int64) (*entity.PersistentStore, error)

    // GetByFilePath 根据路径获取记录
    GetByFilePath(ctx context.Context, filePath string) (*entity.PersistentStore, error)

    // Delete 删除记录及对应文件（严格一致）
    Delete(ctx context.Context, id int64) error

    // DeleteByFilePath 根据路径删除记录及文件
    DeleteByFilePath(ctx context.Context, filePath string) error

    // Exists 检查文件是否存在（记录存在且磁盘文件存在）
    Exists(ctx context.Context, id int64) bool
}
```

### Store 方法流程

```
Store(ctx, "thumbnail/author/video_thumb.jpg", "video_thumb.jpg", reader)
  │
  ├─ 1. 校验 relPath 是否匹配已注册子目录
  │     └─ 不匹配 → 返回错误
  │
  ├─ 2. 检查 relPath 是否已存在记录（唯一约束）
  │     ├─ 已存在 → 删除旧文件 + 更新记录
  │     └─ 不存在 → 继续
  │
  ├─ 3. 确保 {workDir}/store/thumbnail/author/ 目录存在
  │
  ├─ 4. 将 reader 内容写入 {workDir}/store/thumbnail/author/video_thumb.jpg
  │
  ├─ 5. 读取写入字节数作为 fileSize
  │
  ├─ 6. 创建 persistent_store 记录
  │     {file_path: "thumbnail/author/video_thumb.jpg", file_name: "video_thumb.jpg",
  │      filename_extension: ".jpg", file_size: fileSize}
  │
  └─ 7. 返回记录 ID
```

### Delete 方法流程

```
Delete(ctx, id)
  │
  ├─ 1. 根据 ID 查询记录
  │     └─ 记录不存在 → 返回 nil（幂等）
  │
  ├─ 2. 删除磁盘文件 {workDir}/store/{file_path}
  │     └─ 文件不存在 → 仅记录日志，不报错（容忍外部删除）
  │
  └─ 3. 删除数据库记录
```

### Handler（Wails Bind）

```go
type Handler struct {
    service Service
}

// StoreFile 前端上传文件
func (h *Handler) StoreFile(ctx context.Context, relPath string, fileName string, fileData []byte) (int64, error)

// GetById 获取文件记录
func (h *Handler) GetById(ctx context.Context, id int64) (*sdkdto.PersistentStoreDTO, error)

// Delete 删除文件记录及文件
func (h *Handler) Delete(ctx context.Context, id int64) error
```

## HTTP 文件服务

### 路由

新增 `/store/` 路由，与现有 `/resource/` 模式一致：

```go
// app.go CreateAssetHandler
router.Handle("/resource/", app.HttpFileHandler, 0)     // 现有
router.Handle("/store/", app.PersistentStoreHandler, 0)  // 新增
```

### StoreFileHandler

```go
// backend/assetserver/store_handler.go

type StoreFileHandler struct {
    mu      sync.RWMutex
    workDir string
}

// SetWorkDir 设置工作目录
func (h *StoreFileHandler) SetWorkDir(dir string)

// ServeHTTP 处理 /store/{relativePath} 请求
// 从 {workDir}/store/{relativePath} 返回文件
func (h *StoreFileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    // 与 ResourceHandler 相同模式：
    // 1. 从 URL 提取 relativePath
    // 2. 安全校验（路径穿越防护）
    // 3. 设置 Content-Type
    // 4. http.ServeFile
}
```

前端 URL 构建：

```typescript
// frontend/src/utils/UrlUtil.ts
export function buildStoreUrl(filePath: string, queryString: string = ''): string {
  if (!filePath) return ''
  const segments = filePath.split(/[/\\]/)
  const encoded = segments.map(segment => encodeURIComponent(segment)).join('/')
  return `/store/${encoded}${queryString}`
}
```

## SDK DTO 变更

### 新增 PersistentStoreDTO

```go
// plugin-sdk/dto/persistent_store_dto.go

type PersistentStoreDTO struct {
    ID                int64   `json:"id"`
    FilePath          *string `json:"filePath"`
    FileName          *string `json:"fileName"`
    FilenameExtension *string `json:"filenameExtension"`
    FileSize          *int64  `json:"fileSize"`
    CreateTime        int64   `json:"createTime"`
    UpdateTime        int64   `json:"updateTime"`
}
```

## 一致性保障

### 记录删则文件删

`Service.Delete()` 方法中：先删磁盘文件，再删数据库记录。数据库删除失败时磁盘文件已删但记录仍在 → 可通过 `ConsistencyCheck()` 修复。

### 文件删则记录删

- **预防**：所有文件操作必须通过 PersistentStore，禁止直接操作 `{workDir}/store/` 下的文件
- **检测**：提供 `ConsistencyCheck()` 方法，扫描所有记录检查文件是否存在，清理孤儿记录
- **访问时**：HTTP Handler 发现文件不存在时返回 404，可选触发记录清理

## 迁移路径（Resource → PersistentStore）

### 阶段一（本计划）

- 建立 PersistentStore 核心模块（实体、Service、Repository、Handler、HTTP 服务、子目录管理）
- Resource 模块不变
- 两条路由并存：`/resource/` + `/store/`
- 缩略图、头像等业务集成在独立计划中

### 阶段二（后续）

- Resource 实体新增 `store_id` 字段，指向 `persistent_store.id`
- 新下载的 Work 资源通过 PersistentStore 存储
- 迁移工具：将现有 Resource 的文件注册到 PersistentStore，回填 `store_id`
- Resource 的 `file_path`、`file_name`、`filename_extension`、`resource_size`、`workdir` 字段逐步废弃

### 阶段三（最终状态）

```
Resource 实体:
  - work_id, task_id, enabled, suggest_name, resource_complete  ← 业务字段
  - store_id                                                    ← 文件引用
  - thumbnail_store_id                                          ← 缩略图引用

persistent_store:
  - file_path, file_name, filename_extension, file_size         ← 文件元数据

HTTP: 统一使用 /store/ 路由，/resource/ 可废弃或重定向
```

## 修改文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| **SDK** | | |
| `plugin-sdk/dto/persistent_store_dto.go` | 新建 | PersistentStoreDTO |
| **后端 — 新建** | | |
| `backend/base/model/entity/persistent_store.go` | 新建 | PersistentStore 实体 |
| `backend/base/model/dto/persistent_store_dto.go` | 新建 | DTO 转换 |
| `backend/persistentStore/dir.go` | 新建 | 子目录枚举与校验 |
| `backend/persistentStore/service.go` | 新建 | Service 接口 + 实现 |
| `backend/persistentStore/repository.go` | 新建 | Repository 实现 |
| `backend/persistentStore/handler.go` | 新建 | Wails Bind Handler |
| `backend/assetserver/store_handler.go` | 新建 | HTTP 文件服务 Handler |
| **后端 — 修改** | | |
| `backend/migration/migrate.go` | 修改 | 新增 PersistentStore 实体迁移 |
| `backend/app.go` | 修改 | 初始化 PersistentStore + 路由注册 |
| **前端** | | |
| `frontend/src/utils/UrlUtil.ts` | 修改 | 新增 `buildStoreUrl` |

## 执行顺序

1. SDK：新增 `PersistentStoreDTO`
2. 主项目 go.mod：更新 SDK 依赖
3. 后端实体：`PersistentStore` 实体 + DTO 转换
4. `backend/persistentStore/` 模块：dir.go + Service + Repository + Handler
5. `backend/assetserver/store_handler.go`：HTTP 文件服务
6. `backend/migration/migrate.go`：注册迁移
7. `backend/app.go`：初始化 + 路由注册
8. `wails3 generate bindings -ts` 更新前端绑定
9. 前端：`UrlUtil.ts` 新增 `buildStoreUrl`

## 关联计划

- [视频缩略图方案](video-thumbnail-plan.md) — 缩略图生成与前端展示集成
- 作者头像方案 — 待编写

---

## 新会话实现上下文

以下信息供新会话快速理解现有代码模式，无需重新探索。

### 项目路径

- 主项目：`E:\code\lvfeng\library-squirrel`
- SDK（go.mod replace 指向）：`E:\code\lvfeng\library-squirrel-plugin-sdk`（同级目录）
- SDK DTO 文件：`library-squirrel-plugin-sdk/dto/resource_dto.go`（PersistentStoreDTO 同目录新建）

### 现有模块的代码模式

所有后端模块遵循统一的 Repository-Service-Handler 分层，以 `backend/resource/` 为参考模板：

**Repository**（`repository.go`）：
- 嵌入 `*database.BaseRepository[T]`，复用通用 CRUD
- 自定义方法使用 `database.QueryOption` + `gorm/clause` 构建条件
- 构造函数：`func NewRepository(db *gorm.DB) *XxxRepository`

**Service**（`service.go`）：
- 在文件顶部定义 `Repository` 接口（调用方视角）
- Service 结构体持有 `repo Repository`
- 构造函数：`func NewService(repo Repository) *Service`
- 方法直接委托 repo，仅在有业务附加值时添加逻辑

**Handler**（`handler.go`）：
- 持有 `svc *Service`（具体类型，非接口）
- 所有方法签名：`func (h *Handler) Xxx(ctx context.Context, ...) *model.ApiResponse[T]`
- 返回值使用 `model.Success(data)` / `model.HandleError[T](err)` / `model.HandleVoid(err)`
- 构造函数：`func NewHandler(svc *Service) *Handler`

### 实体模式

实体定义在 `backend/base/model/entity/`，遵循以下规则：

```go
type Xxx struct {
    *model.BaseEntity                                          // 提供 ID、CreateTime、UpdateTime
    FieldName sql.NullString `gorm:"column:field_name" json:"fieldName"`
    FieldInt  sql.NullInt64  `gorm:"column:field_int" json:"fieldInt"`
}

func (Xxx) TableName() string { return "xxx" }

func NewXxx() *Xxx {
    return &Xxx{BaseEntity: &model.BaseEntity{}}
}
```

- 可空字段使用 `sql.NullString` / `sql.NullInt64`
- 禁止 `&entity.Xxx{}`，必须使用 `entity.NewXxx()`
- DTO 转换在 `backend/base/model/dto/` 中（`NewXxxDTO(entity)` 和 `ToXxxEntity(dto)`）
- Nullable ↔ Pointer 转换使用 `util.NullStringToPointer()` / `util.NullInt64ToPointer()`

### HTTP 文件服务模式

参考 `backend/assetserver/resource_handler.go`：

```go
type XxxHandler struct {
    mu      sync.RWMutex
    workDir string
}

func NewXxxHandler() *XxxHandler { return &XxxHandler{} }

func (h *XxxHandler) SetWorkDir(dir string) {
    h.mu.Lock()
    defer h.mu.Unlock()
    h.workDir = dir
}

func (h *XxxHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    // 1. 方法检查（GET/HEAD only）
    // 2. RLock 读取 workDir
    // 3. TrimPrefix 提取 relativePath
    // 4. filepath.Clean + ".." 检查 + 前缀边界检查（防路径穿越）
    // 5. os.Stat 确认文件存在
    // 6. mime.TypeByExtension 设置 Content-Type
    // 7. http.ServeFile(w, r, absPath)
}
```

StoreFileHandler 直接复制此模式，将 `resource` 替换为 `store`。

### app.go 集成模式

**App 结构体**（`app.go:54`）需要添加：

```go
type App struct {
    // 业务服务（新增）
    PersistentStoreService *persistentStore.Service

    // HTTP 路由（新增）
    StoreFileHandler *assetserver.StoreFileHandler

    // Handlers（新增）
    PersistentStoreHandler *persistentStore.Handler
}
```

**初始化顺序**：

1. `NewApp()` 中（`app.go:136`），`NewResourceHandler()` 之后：
   ```go
   app.StoreFileHandler = assetserver.NewStoreFileHandler()
   ```

2. `initBaseServices()` 中（`app.go:647`），resource 服务之后：
   ```go
   psRepo := persistentStore.NewRepository(app.db)
   app.PersistentStoreService = persistentStore.NewService(psRepo)
   ```

3. `initBaseServices()` 中设置 workDir 的位置（`app.go:687`）：
   ```go
   app.StoreFileHandler.SetWorkDir(app.SettingsService.GetWorkDir())
   ```

4. `CreateAssetHandler()` 中（`app.go:254`），添加路由：
   ```go
   router.Handle("/store/", app.StoreFileHandler, 0)
   ```

5. `initHandlers()` 中（`app.go:892`）：
   ```go
   app.PersistentStoreHandler = persistentStore.NewHandler(app.PersistentStoreService)
   ```

6. `main.go` 的 Wails `Services` 切片中（`main.go:53`）添加 `application.NewService(app.PersistentStoreHandler)`

**迁移注册**（`backend/migration/migrate.go`）：
```go
models := []interface{}{
    // ... 在 Resource 之后添加 ...
    entity2.NewPersistentStore(),
}
```

### 前端 URL 工具

现有 `frontend/src/utils/UrlUtil.ts`：
```typescript
export function buildResourceUrl(filePath: string, queryString: string = ''): string {
  if (!filePath) return ''
  const segments = filePath.split(/[/\\]/)
  const encoded = segments.map(segment => encodeURIComponent(segment)).join('/')
  return `/resource/${encoded}${queryString}`
}
```

新增 `buildStoreUrl` 遵循完全相同的模式，将 `/resource/` 替换为 `/store/`。
