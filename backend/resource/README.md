# resource 模块说明

## 一句话职责

Resource 实体管理与资源编排：一份 Resource 关联一个作品，通过 `resource_store` 关联表挂载多个 typed store（main/thumbnail/videoTrack/audioTrack/videoMain）；并提供音视频合并编排（MergeService）。

## 边界

- 与 **persistentStore**：persistentStore 管具体文件与 `persistent_store` 记录；resource 通过 `resource_store` 关联表引用 store（1 Resource 挂 N typed store）。合并编排依赖 persistentStore 落盘产物 + 路径变换。
- 与 **work**：work 是作品实体与编排；resource 是作品下的资源映射，work 通过 ResourceUpdater 接口操作 resource。
- 与 **merge**：merge 包提供纯合并能力（`FFmpegMuxer.MergeRemux`）；本模块 MergeService 注入它做音视频合并编排，merge 不感知 store（MODULE_BOUNDARY_PURITY）。
- 与 **backup**：StoreBackupOrchestrator 按 StoreType 选择性备份 Resource 的 store。

## 对外接口（Handler）

| 方法 | 作用 |
| --- | --- |
| `Save(resource)` | 保存资源 |
| `Update(resource)` | 更新资源 |
| `Delete(id)` | 删除资源 |
| `DeleteByWorkId(workId)` | 按作品ID删除所有资源 |
| `GetById(id)` | 按ID查询 |
| `ListByWorkId(workId)` | 查询作品关联的资源列表 |
| `MergeResource(resourceId)` | 合并指定 Resource 的 videoTrack+audioTrack 为可播放单文件（videoMain store） |

## 核心概念

- **Resource 实体字段**：`WorkID` / `TaskID`（所属作品 / 产生它的任务）、`Enabled`（启用状态）、`ResourceComplete`（完整度）、`SuggestName`（建议文件名）。Resource 不直接持有 store 外键。
- **resource_store 关联表**：1 Resource 挂 N typed store，每行含 `StoreType`（业务角色）、`Generation`（生成方式：downloaded 可续传 / derived 一次性）、`StoreID`（→ persistent_store）、`StoreSeq`（同 role 内 store 序号，路径消歧与续传身份化用）。
- **StoreType 开放枚举**（`entity/resource_store.go`）：`main`（主资源）、`thumbnail`（缩略图）、`videoTrack`（视频轨）、`audioTrack`（音频轨）、`videoMain`（视频可播放主体：本地封装原文件或分离流合并产物）。新增类型只加常量、不改表结构；backup/restore/软删按 store 集合遍历，对新类型透明（videoMain 自动被覆盖，零改）。

## 合并编排（MergeService）

音视频合并的业务编排层。设计详见 `doc/plan/merge-business.md`。

- **流程**：取 resource 的 videoTrack/audioTrack store → 调 `merge.FFmpegMuxer.MergeRemux` → 落产物 PersistentStore(videoMain)（路径由 `persistentStore.BuildVariantPath` 从源视频轨 FilePath 推导）→ 事务挂 `resource_store`(videoMain)。
- **mergeStrategy**（settings.MergeSettings）：`keep`（默认，新建 videoMain 保留原轨道）/ `overwrite`（新建 videoMain、删原轨道 store+文件）。
- **依赖注入**（接口隔离）：`Merger`（merge.FFmpegMuxer）、`StoreOps`（persistentStore.Service）、`MergeSettingsReader`（settings.Service）、`Transactor`（dbTransactorAdapter）。ffmpeg 缺失时 merger=nil，调用返回 `ErrMergeUnavailable`。
- **事务**：挂 resource_store 走 dbTransactorAdapter（tx 入 ctx，BaseRepository 经 dbFromCtx 感知）；挂载失败补偿删产物 store。

## 依赖关系

- 依赖（MergeService）：**merge**（Merger）、**persistentStore**（StoreOps）、**settings**（MergeSettingsReader）、database（Transactor）
- 被依赖：**work**（ResourceUpdater）、**backup**（StoreResourceProvider / ResourceUpdater）、**task**（任务产出资源）

## 关键设计

- **替换场景不禁用 Resource**：StoreBackupOrchestrator 备份 / 还原 store 时，Resource 记录保持 Enabled=true 不变，仅切换其 store 引用。
- **合并的模块边界**：merge 包输入输出均为文件路径，不感知 store/resource；产物路径生成（`store/resource/...`）归 persistentStore（BuildVariantPath），本模块只做编排。
