# resource 模块说明

## 一句话职责

Resource 实体管理：作品资源的**元数据抽象**——一份 Resource 关联一个作品，持有资源文件与缩略图的 PersistentStore 引用，并记录启用状态、完整度与建议文件名。

## 边界

- 与 **persistentStore**：persistentStore 管具体文件与 `persistent_store` 记录；resource 管 Resource 实体，通过 `WorkStoreID` / `ThumbnailStoreID` 外键引用 store。一份 Resource 引用两份 store（资源 + 缩略图）。
- 与 **work**：work 是作品实体与编排；resource 是作品下的资源映射，work 通过 ResourceUpdater 接口操作 resource。
- 与 **backup**：StoreBackupOrchestrator 按 StoreType 选择性备份 Resource 的某个 store 字段。

## 对外接口（Handler）

| 方法 | 作用 |
| --- | --- |
| `Save(resource)` | 保存资源 |
| `Update(resource)` | 更新资源 |
| `Delete(id)` | 删除资源 |
| `DeleteByWorkId(workId)` | 按作品ID删除所有资源 |
| `GetById(id)` | 按ID查询 |
| `ListByWorkId(workId)` | 查询作品关联的资源列表 |

## 核心概念

- **Resource 实体字段**：
  - `WorkID` / `TaskID`：所属作品 / 产生它的任务。
  - `WorkStoreID` / `ThumbnailStoreID`：资源文件 / 缩略图的 PersistentStore 引用。
  - `Enabled`：启用状态（替换场景下保持 true，由 store 层做新旧切换）。
  - `ResourceComplete`：资源完整度。
  - `SuggestName`：建议文件名。
- **StoreType**（由 backup 定义）：标识 Resource 上的不同 store 字段（资源 / 缩略图），backup 按此做板块隔离。

## 依赖关系

- 依赖：无（纯 Resource 实体 CRUD，store 引用为外键ID）
- 被依赖：**work**（ResourceUpdater）、**backup**（StoreResourceProvider / ResourceUpdater，StoreBackupOrchestrator）、**task**（任务产出资源）

## 关键设计

- **替换场景不禁用 Resource**：StoreBackupOrchestrator 备份 / 还原 store 时，Resource 记录保持 Enabled=true 不变，仅切换其 store 引用。
