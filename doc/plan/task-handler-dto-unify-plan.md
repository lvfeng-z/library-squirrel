# 重构计划：DTO 全面迁移到 SDK

## 原则

1. DTO 不依赖 entity（`WorkResponse.Work` 用 DTO 而非 `*entity.Work`）
2. 所有 DTO 定义统一到 SDK，消除主程序与 SDK 之间的类型重复
3. 依赖 entity 的方法（`New*`、`To*Entity`）转为独立函数留在主程序
4. SDK `entity.go` 中的 `Task`/`Work`/`WorkSet`/`Site` 在 DTO 迁移完成后删除，由迁移后的 DTO 类型替代

## 一、SDK entity.go 类型替换方案

`entity.go` 中的 4 个类型将被迁移后的 DTO 替代：

| entity.go 类型 | 替代为（迁移后 DTO） | 需更新的 SDK 内部引用 |
|---------------|-------------------|---------------------|
| `Task` | `TaskDTO`（从主程序迁移） | `dto.go` TaskResParam.Task、`task_handler.go` 接口、`plugin.go` protoToTask/taskToProto |
| `Work` | `WorkDTO`（从主程序迁移） | `dto.go` WorkResponse.Work、`plugin.go` workToProto |
| `WorkSet` | `WorkSetDTO`（从主程序迁移） | `context.go`、`context_client.go`、`host.go` GetWorkSetBySiteWorkSetId/workSetToProto |
| `Site` | `SiteDTO`（dto.go 中已有，字段完全相同） | `context.go`、`host.go` AddSite/protoToSite |

`Site` 与 `SiteDTO` 字段完全相同，直接统一为 `SiteDTO`，删除 `Site`。
`Task`/`Work`/`WorkSet` 在主程序 DTO 迁移后，由 `TaskDTO`/`WorkDTO`/`WorkSetDTO` 替代。

## 二、类型清单与归属

### A. SDK 已有、结构完全一致，直接替换

| 主程序 dto.* | SDK pluginsdk.* |
|-------------|-----------------|
| `SiteDTO` | `SiteDTO`（dto.go，合并删除 entity.go 的 `Site`） |
| `LocalAuthorDTO` | `LocalAuthorDTO` |
| `LocalTagDTO` | `LocalTagDTO` |
| `TaskCreateResponse` | `TaskCreateResponse` |
| `TaskCreateChildResponse` | `TaskCreateChildResponse` |
| `TaskSiteAuthorDTO` | `TaskSiteAuthorDTO` |
| `TaskSiteTagDTO` | `TaskSiteTagDTO` |
| `TaskWorkSetDTO` | `TaskWorkSetDTO` |
| `TaskResourceDTO` | `TaskResourceDTO` |
| `TaskCreateResult` | `TaskCreateResult` |
| `TaskResParam` | `TaskResParam` |
| `WorkResponse` | `WorkResponse` |
| `TaskHandler` 接口 | 统一接口签名 |

### B. 主程序独有、需新增到 SDK

| 类型 | 所在文件 | 说明 |
|------|---------|------|
| `TaskDTO` | task_dto.go | 替代 entity.go 的 `Task` |
| `WorkDTO` | work_dto.go | 替代 entity.go 的 `Work` |
| `WorkSetDTO` | work_set_dto.go | 替代 entity.go 的 `WorkSet` |
| `ResourceDTO` | resource_dto.go | 资源 DTO |
| `SiteAuthorDTO` | site_author_dto.go | 站点作者 DTO |
| `SiteTagDTO` | site_tag_dto.go | 站点标签 DTO |
| `BackupDTO` | backup_dto.go | 备份 DTO |
| `SelectItem` | select_item.go | UI 下拉选项 |
| `SearchCondition` 等 | search.go | 搜索相关类型/枚举 |
| `RankedLocalAuthor` / `RankedLocalAuthorWithWorkId` | local_author_dto.go | 排名结果 |
| `RankedSiteAuthor` / `RankedSiteAuthorWithWorkId` / `RankedSiteAuthorWithWorkIdDTO` | site_author_dto.go | 排名结果 |
| `LocalTagWithBaseTagDTO` | local_tag_dto.go | 标签+基础标签 |
| `SiteTagFullDTO` / `SiteTagLocalRelateDTO` | site_tag_dto.go | 站点标签组合 DTO |
| `SiteAuthorFullDTO` / `SiteAuthorLocalRelateDTO` | site_author_dto.go | 站点作者组合 DTO |
| `WorkFullDTO` | work_dto.go | 作品完整 DTO |
| `WorkSetWithWorksResultDTO` / `WorkSetWithCoverDTO` | work_set_dto.go | 作品集组合 DTO |
| `TaskProgressDTO` / `TaskProgressTreeDTO` / `CreateTaskRequest` / `TreeDataPageDTO` | task_dto.go | 任务进度/UI DTO |
| plugin_types.go 中所有类型 | plugin_types.go | 插件清单/Slot 相关 |
| `PluginDTO` | plugin_dto.go | 需确认是否仍在使用 |
| `PoiDTO` | poi_dto.go | 需确认是否仍在使用 |

### C. 需要留在主程序的独立函数

每个有 `New*DTO`/`To*Entity` 的 DTO 对应一对转换函数：

| DTO | 独立函数（留在主程序） |
|-----|---------------------|
| `TaskDTO` | `NewTaskDTOFromEntity(*entity.Task) *pluginsdk.TaskDTO`<br>`TaskDTOToEntity(*pluginsdk.TaskDTO) *entity.Task` |
| `WorkDTO` | `NewWorkDTOFromEntity(*entity.Work) *pluginsdk.WorkDTO`<br>`WorkDTOToEntity(*pluginsdk.WorkDTO) *entity.Work` |
| `WorkSetDTO` | `NewWorkSetDTOFromEntity(...)`<br>`WorkSetDTOToEntity(...)` |
| `SiteDTO` | `NewSiteDTOFromEntity(...)`<br>`SiteDTOToEntity(...)` |
| `LocalAuthorDTO` | 同上模式 |
| `LocalTagDTO` | 同上模式 |
| `ResourceDTO` | 同上模式 |
| `SiteAuthorDTO` | 同上模式 |
| `SiteTagDTO` | 同上模式 |
| `BackupDTO` | 同上模式 |

组合 DTO 的构造函数（如 `NewWorkFullDTO(*entity.Work)`）拆为：
- 先调用各子 DTO 的独立函数
- 再组装组合 DTO

## 三、执行步骤

### 步骤 1：SDK 补齐缺失类型 + 删除 entity.go

1. 在 SDK 中新增 B 类中列出的所有类型定义（含 `TaskDTO`、`WorkDTO`、`WorkSetDTO`）
2. 将 `TaskResParam.Task`、`WorkResponse.Work`、`TaskHandler` 接口等引用从 `Task`/`Work` 改为 `TaskDTO`/`WorkDTO`
3. 更新 SDK 内部 proto 转换函数（`plugin.go`/`context_client.go`/`host.go`）中的类型引用
4. 合并 `Site` → `SiteDTO`，更新所有引用
5. 删除 `entity.go`

### 步骤 2：在主程序创建转换函数文件

在 `backend/base/model/dto/` 下新建 `convert.go`，将所有 `New*DTO`/`To*Entity` 方法转为独立函数。
原各 `*_dto.go` 文件中的方法定义删除。

### 步骤 3：删除主程序中的 DTO 定义

将 A 类和 B 类中的所有 DTO struct 定义从主程序删除。
`task_handler.go`、`search.go`、`select_item.go`、`plugin_types.go` 等文件清空 DTO 定义。
目录下仅保留 `convert.go`（转换函数）。

### 步骤 4：全量更新 import 和引用

所有 `dto.XXX` → `pluginsdk.XXX`。涉及文件极多，需逐一更新。

### 步骤 5：删除主程序 convert.go 中不再需要的映射函数

`backend/plugin/extension/convert.go` 中，对所有 A 类类型的逐字段映射函数全部删除，直接透传。

### 步骤 6：编译验证

## 四、SDK 命名对照表（供步骤 4 引用）

| 原 dto.* | 新 pluginsdk.* | 来源 |
|---------|----------------|------|
| `dto.TaskDTO` | `pluginsdk.TaskDTO` | 新增到 SDK（替代原 entity.go Task） |
| `dto.WorkDTO` | `pluginsdk.WorkDTO` | 新增到 SDK（替代原 entity.go Work） |
| `dto.WorkSetDTO` | `pluginsdk.WorkSetDTO` | 新增到 SDK（替代原 entity.go WorkSet） |
| `dto.SiteDTO` | `pluginsdk.SiteDTO` | SDK dto.go 已有（合并删除 entity.go Site） |
| `dto.LocalAuthorDTO` | `pluginsdk.LocalAuthorDTO` | SDK dto.go 已有 |
| `dto.LocalTagDTO` | `pluginsdk.LocalTagDTO` | SDK dto.go 已有 |
| `dto.TaskCreateResponse` | `pluginsdk.TaskCreateResponse` | SDK dto.go 已有 |
| `dto.TaskCreateChildResponse` | `pluginsdk.TaskCreateChildResponse` | SDK dto.go 已有 |
| `dto.TaskSiteAuthorDTO` | `pluginsdk.TaskSiteAuthorDTO` | SDK dto.go 已有 |
| `dto.TaskSiteTagDTO` | `pluginsdk.TaskSiteTagDTO` | SDK dto.go 已有 |
| `dto.TaskWorkSetDTO` | `pluginsdk.TaskWorkSetDTO` | SDK dto.go 已有 |
| `dto.TaskResourceDTO` | `pluginsdk.TaskResourceDTO` | SDK dto.go 已有 |
| `dto.TaskCreateResult` | `pluginsdk.TaskCreateResult` | SDK 已有 |
| `dto.TaskResParam` | `pluginsdk.TaskResParam` | SDK dto.go 已有 |
| `dto.WorkResponse` | `pluginsdk.WorkResponse` | SDK dto.go 已有 |
| `dto.TaskHandler` | `pluginsdk.TaskHandler` | 统一接口签名 |
| `dto.ResourceDTO` | `pluginsdk.ResourceDTO` | 新增到 SDK |
| `dto.SiteAuthorDTO` | `pluginsdk.SiteAuthorDTO` | 新增到 SDK |
| `dto.SiteTagDTO` | `pluginsdk.SiteTagDTO` | 新增到 SDK |
| `dto.BackupDTO` | `pluginsdk.BackupDTO` | 新增到 SDK |
| `dto.SelectItem` | `pluginsdk.SelectItem` | 新增到 SDK |
| `dto.SearchCondition` 等 | `pluginsdk.*` | 新增到 SDK |
| 其余组合/排名 DTO | `pluginsdk.*` | 新增到 SDK |

## 五、风险与注意事项

1. **SDK 内部 proto 转换**：`plugin.go` 中 `protoToTask`/`taskToProto`/`workToProto` 等函数引用 entity.go 类型，需改为引用迁移后的 DTO 类型。由于字段结构一致，仅类型名变化。
2. **PluginDTO / PoiDTO**：探索报告显示这两个 DTO 可能未在外部使用，迁移前需确认是否直接删除。
3. **组合 DTO 构造函数**：如 `NewWorkFullDTO(*entity.Work)` 内部调用了 `NewLocalAuthorDTO`、`NewSiteAuthorFullDTO` 等，迁移后需要拆解为多步调用。
4. **`work/service.go` 大量字段访问**：`SaveWorkInfo` 中直接读写 `work.SiteID.Valid/.Int64` 等 `sql.Null*` 字段，迁移后 WorkDTO 字段为 `*int64`，访问方式需全面适配。
5. **改动面极大**：涉及 `backend/base/model/dto/` 全部 16 个文件、SDK 仓库、以及 `backend/` 下几乎所有引用 dto 的模块。建议分批提交。
