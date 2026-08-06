---
description: "Go 后端架构与编码规则，适用于修改 backend/ 目录下的代码时加载"
globs:
  - "backend/**"
---

# 后端架构与规则

## 模块模式（Repository-Service-Handler）

每个业务模块位于 `backend/{module}/`，遵循以下结构：
```
handler.go          — Wails Bind 方法（通过 IPC 暴露给前端）
service.go          — 业务逻辑
repository.go       — 数据访问接口 + 实现
query.go            — 查询 DTO
```

共享模型位于 `backend/base/model/entity/`（18 个实体）和 `backend/base/model/dto/`。

## 文件与命名规范

| 元素        | 规则                       | 示例                              |
| ----------- | -------------------------- | --------------------------------- |
| Go 源文件   | snake_case + `.go`         | `handler.go`、`work_service.go`   |
| 目录        | 单元命名，全小写           | `backend/author/`                 |
| 包名        | 与目录同名，简洁           | `package author`                  |
| 结构体/接口 | PascalCase                 | `WorkService`、`Repository`       |
| 变量/函数   | camelCase                  | `getWorkById`                     |
| 常量        | UPPER_SNAKE_CASE           | `MAX_RETRY_COUNT`                 |
| 错误变量    | `Err` 前缀                 | `ErrNameEmpty`                    |
| 接口命名    | `er` 后缀或名词            | `Repository`、`Provider`          |

## 代码组织

文件内声明顺序：
1. 包声明
2. 导入（标准库在前，按长度排序）
3. 错误定义（`var ErrXxx = errors.New(...)`）
4. 领域实体
5. 接口定义
6. Service/Repository 实现

函数/方法顺序：构造函数（`NewXxx`）→ 接口实现方法 → 业务方法 → 私有辅助方法。

## 核心业务概念

- **Site（站点）**: 远程来源（pixiv 等）
- **Work（作品）**: 核心实体 — 资源集合 + 元数据
- **Resource（资源）**: Work 下的可展示单元，带 `ResourceType`（内置 image/video/article/document/audio/unknown 或插件自定义类型）
- **ResourceType（资源类型）**: 内置 6 类（image/video/article/document/audio/unknown）封闭 + 插件可经 manifest `resourceTypes` 段声明自定义类型（注册进 `ResourceTypeRegistry`，注册时强校验）；决定 store 角色组合（基数）+ 展示主体优先级（PrimaryRoles）+ 文件标准；规约见 `doc/resource-type-spec.md`，实现 `backend/base/model/entity/resource_type.go`（`ResourceTypeRegistry`）
- **store（resource_store）**: Resource 挂 N 个 typed store，`store_type` ∈ image/document/thumbnail/videoTrack/audioTrack/videoMain/audioMain（内置封闭枚举；videoMain 为视频可播放主体、audioMain 为音频可播放主体；插件自定义 store 角色延后）
- **本地标签/作者 ↔ 站点标签/作者**: 通过关联实现跨站点统一搜索
- **Task（任务）**: 作品创建流程（URL → 插件 → 保存），插件 Create 声明 ResourceType

## Go 后端编码规则

- **DTO_COMPOSITION_OVER_EMBEDDING** (P0): DTO 禁止嵌入实体，使用其他 DTO 的命名字段。DTO 之间禁止匿名嵌入（Wails 会展平 JSON tag）。
- **ELIMINATE_N_PLUS_1_QUERY** (P0): 收集 ID → 批量查询 → 构建 map → 组装 DTO。禁止在循环中查询。
- **SERVICE_DEPENDENCY_VIA_INTERFACE** (P0): Service 依赖由**调用方定义**的接口，由**提供方实现**。禁止持有具体 `*OtherService`。通过构造函数注入。
- **BASE_REPOSITORY_REUSE** (P0): 复用 `BaseRepository` 方法。仅当 `BaseRepository` 无法表达时才编写自定义 repository 逻辑。
- **MODULE_BOUNDARY_PURITY** (P0): 能力包（如 `merge`）只提供领域无关的纯能力，输入输出用基础类型（文件路径等），禁止感知/耦合业务实体（store/resource 等）与领域规范（落盘路径、`store_type` 枚举）。接受领域实体、决定落盘位置、挂载 store 关联等业务编排归对应业务模块（resource/persistentStore）。例：`merge` 包只做文件合并（`MergeRemux(ctx, videoPath, audioPath, outPath)`）；取 resource 的 store、落盘到 `store/resource/...`、挂 `resource_store`(videoMain) 等归 resource/persistentStore。
- **ENTITY_USE_NEW_FACTORY** (P1): 使用 `entity.NewXxx()` 工厂方法，禁止 `&entity.Xxx{}`。
- **DTO_USE_TO_ENTITY** (P1): 使用 `ToXxxEntity()` 转换函数，禁止手动逐字段映射。
- **BATCH_UPDATE_OPTIMIZATION** (P1): 批量更新使用单条 SQL，禁止循环逐条 UPDATE。
- **NULLABLE_PARAM_USE_POINTER** (P1): 可空参数使用 `*int64`/`*string`（null = 清除关联）。
- **REMOVE_REDUNDANT_QUERY_FIELDS** (P2): QueryDTO 中禁止为同一列定义多个语义重复的字段（如精确+模糊），保留一个字段通过 `QueryAttribute.operator` 控制匹配方式。
- **DEAD_CODE_CLEANUP** (P2): 重构后确认无调用方的旧方法直接删除，禁止保留"以防万一"的代码。
- **FIELD_RENAME_GUARD_AUDIT** (P1): 重命名或复用字段承担新职责时，必须审计该字段的**所有读写点**——基于初值的守卫（CAS 首派、状态机初态、零值可用契约）不得被新增赋值破坏。字段命名须反映其唯一真实职责；一个字段禁止同时承担“对象生命周期标志”与“派发/状态守卫”两个初值要求互斥的职责。例：`dispatchState`(三态,零值即初态) 迁移为 `actorStarted`(单 bool) 时，`dispatch` 的 `CAS(false→true)` 守卫依赖初值 false，创建期对其 `Store(true)` 会令守卫恒失败、新任务永不启动。
- **CONSTRUCTOR_TEST_VIA_FACTORY** (P1): 验证受生产构造函数（`NewXxx`）初始化影响的行为时，测试必须经生产构造函数构造对象，禁止用绕过其初始化的字面量（`&Xxx{}`）来测试该行为；字面量构造仅用于无依赖的纯逻辑测试。否则生产构造中的错误初值对测试不可见（测试恒绿反而掩盖 bug）。
- **错误处理**：`var ErrXxx = errors.New(...)`，使用 `errors.Is()` 判断。
- **所有公开方法以** `context.Context` 作为第一个参数，禁止在 `context.WithValue` 中存储业务数据。
- **Service 层禁止直接导入** `backend/database`，仅 Repository 层可导入。
- **RESOURCE_TYPE_STRICT** (P0): 资源类型与 store_type 严格识别——插件 Create 必须声明有效 `ResourceType`（已注册即可：内置 `entity.ResourceType*` 之一含 audio，或插件经 resourceTypes 段注册的自定义类型；unknown 合法），`StoreSpec.Role` 必须 ∈ 7 预定义 `entity.StoreType*`（含 audioMain）；空/未注册值在写入路径抛错（`entity.ValidateResourceType`/`ValidateStoreType`），不推断、不兜底。插件自定义类型注册时强校验（反向域名前缀 + Roles 合法性，`entity.ResourceTypeRegistry.Register`）。展示主体由后端 `ResolvePrimaryStore`（按 PrimaryRoles）派生，前端纯消费。规约见 `doc/resource-type-spec.md`。
- **ORCHESTRATION_BY_CALLER** (P0): 业务编排（串联多个原子能力完成一个流程）归**发起方**模块——发起方通过依赖注入获取各提供方的能力接口，自行串联；**禁止**把多个模块的能力揉进一个「集成器/编排器」接口在某个提供方模块集中实现。例：「拉取插件原站序 + 映射 + 写 site_sort_order」的编排归发起该流程的模块（如入库流程的 `SaveWorkInfo`）；「从插件获取」归 plugin（提供获取接口）、「写入 sort_order」归 workSet（提供写入接口），发起方注入两者并编排。不建 `WorkSetOrderSyncer` 这种把两者揉在单一模块的接口——它会导致模块职责越界。

## 禁止的做法

| 禁止                                   | 正确做法               |
| -------------------------------------- | ---------------------- |
| 在 Service 层 import `backend/database` | 在 Repository 层 import |
| 返回 `*gorm.DB` 或 `sql.Rows`          | 返回领域实体或 DTO     |
| 跨模块直接引用其他 Service             | 使用接口隔离           |
| 在 `util` 包中包含有状态逻辑           | 使用纯函数             |
