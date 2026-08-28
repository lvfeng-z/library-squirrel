# resource 模块说明

## 一句话职责

Resource 实体管理与资源编排：一份 Resource 关联一个作品，通过 `resource_store` 关联表挂载多个 typed store（image/thumbnail/videoTrack/videoMain/...）；并提供音视频合并编排（MergeService，overwrite 原轨道置换前置带分享拉取作品锁守卫）与替换链能力（ReplacementService——替换前置软删、失败回滚复活，前置带分享拉取作品锁守卫）。

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
| `DeleteByWorkId(workId)` | 按作品ID删除所有资源 |
| `GetById(id)` | 按ID查询 |
| `ListByWorkId(workId)` | 查询作品关联的资源列表 |
| `MergeResource(resourceId)` | 异步启动合并（立即返回，进度与结果经 merge-events 事件推送） |
| `MergeCancel(resourceId)` | 取消指定 Resource 的进行中合并（无则 no-op） |
| `RecomputeResourceComplete(resourceId)` | 完整度共享重算（活行 store 角色计数→三态持久化；下载完成/合并/回收站复原置换三方共用，读路径不抛错） |

## 核心概念

- **Resource 实体字段**：`WorkID` / `TaskID`（所属作品 / 产生它的任务）、`ResourceComplete`（完整度三态：0=未校验/1=完整/2=不完整，`sql.NullInt64`）、`SuggestName`（建议文件名）。Resource 不直接持有 store 外键。
- **resource_store 关联表**：1 Resource 挂 N typed store，每行含 `StoreType`（业务角色）、`Generation`（生成方式：downloaded 可续传 / derived 一次性）、`StoreID`（→ persistent_store）、`StoreSeq`（同 role 内 store 序号，路径消歧与续传身份化用）；作品软删除期间关联行原地保留（复原零重建），彻底删除时随级联物理清理（`DeleteByResourceIds`）。**关联保留形态**：store 行软删（替换/merge 残留、外部裁决失效）时其关联行保留——每挂载键 (resource_id, store_type, store_seq) 呈「活行 0..1 + 死行 0..N（按 deleted_at 代次）」；本模块消费面（GetByType/CountAliveTypesByResourceId/DeleteByResourceIdAndTypes）与 DTO 组装（NewResourceFullDTO 经 storeMap 命中过滤）均按行活性过滤。
- **StoreType 开放枚举**（`entity/resource_store.go`，别名 SDK contract 包）：`image`（主资源）、`document`（图文文档）、`thumbnail`（缩略图）、`videoTrack`（视频轨）、`audioTrack`（音频轨）、`videoMain`（视频可播放主体：本地封装原文件或分离流合并产物）、`audioMain`（音频可播放主体）。新增类型只加常量、不改表结构；backup/restore/软删按 store 集合遍历，对新类型透明（videoMain 自动被覆盖，零改）。

## 合并编排（MergeService）

音视频合并的业务编排层。合并**异步执行**（不阻塞 IPC），进度与结果经独立 `merge-events` 事件推送（不进 taskManager 控制面，阶段1 止血设计）。设计详见 `doc/plan/merge-business.md`（同步期）与 `doc/plan/merge-async-stage1.md`（异步化）。

- **异步执行**：`MergeResource` 同步做前置校验（ffmpeg 可用 / 已存在 videoMain 幂等 / 缺轨 fail-fast / in-flight 守卫），通过则注册 in-flight job（detached ctx，脱离 IPC handler ctx，handler 返回后合并仍跑）并在独立 goroutine 跑合并，立即返回。in-flight 注册表（resourceId→job）防并发叠加 + 作 cancel 锚点。
- **流程**（goroutine 内）：取 videoTrack/audioTrack store → 调 `merge.FFmpegMuxer.MergeRemux`（带进度回调）→ 落产物 PersistentStore(videoMain)（路径由 `persistentStore.BuildVariantPath` 从源视频轨 FilePath 推导）→ 事务挂 `resource_store`(videoMain)。
- **进度与完成**：ffmpeg stderr 的 `-progress` 输出解析为百分比，经 `MergeEventEmitter.PushProgress` 推前端；终态（成功 mergedStoreId / 失败 errMsg）经 `PushComplete` 推送。前端 useMergeProgress 组合式消费（complete 为权威终态，忽略迟到 progress 防乱序闪烁）。
- **取消**：`MergeCancel` 调 job 的 ctx.cancel，杀 ffmpeg 子进程（`exec.CommandContext`）；取消在 MergeRemux 阶段生效，落盘/overwrite 仅成功路径执行，故不误删原轨。
- **mergeStrategy**（settings.MergeSettings）：`keep`（默认，新建 videoMain 保留原轨道）/ `overwrite`（新建 videoMain、原轨道 store+文件转入回收站——经 `DeleteWithBackup` 软删带备份，可经回收站文件条目复原置换回滚，TTL 到期自动清理）。overwrite 置换前置带作品锁守卫：原轨道所属作品正被分享拉取持有时拒绝置换（返回 `shareLock.ErrWorkLocked`，产物已挂载保留、原轨道不动；资源反查异常时告警放行的软防护）。
- **依赖注入**（接口隔离）：`Merger`（merge.FFmpegMuxer）、`StoreOps`（persistentStore.Service，含 DeleteWithBackup）、`MergeSettingsReader`（settings.Service）、`Transactor`（dbTransactorAdapter）、`ResourceRecomputer`（resource.Service 完整度共享重算）、`MergeEventEmitter`（wailsMergeEmitter，闭包延迟读 Wails emitter，app.go 注入）、`MergeWorkLockChecker`（shareLock.ShareLockRegistry——overwrite 原轨道置换前置作品锁守卫）。ffmpeg 缺失时 merger=nil，调用返回 `ErrMergeUnavailable`。
- **事务**：挂 resource_store 走 dbTransactorAdapter（tx 入 ctx，BaseRepository 经 dbFromCtx 感知）；挂载失败补偿删产物 store。

## 替换链能力（ReplacementService）

替换链的通用能力（输入 `(workId, roles)` 纯领域参数，不感知任务语义），自 taskManager 抽入本模块，插件任务与 share-receive 两发起方复用。实现于 `replacement.go`，对外接口 `ReplaceStoreOps`。

- **`SoftDeleteWorkStoreRoles(ctx, workId, roles)`**：替换前置软删——软删作品下**指定角色集合**的活行 store（roles 为显式集合，**空集=不软删任何行**；「空选择=全量板块」的展开归发起方——taskManager 展开为 store_type 封闭枚举全集后传入，能力不承接该语义），已完成行走 `DeleteWithBackup`（移文件入 backup 并写行内 backup_id）、未完成行废弃文件软删（partial 无复原价值）、历史残留死行跳过；返回被软删行清单 `[]StoreRef`（供回滚登记）。`resource_store` 关联不摘——软删行经挂载链可联作品、随作品级联净化，失败回滚复活即挂载回位。前置作品锁守卫：软删会移走作品的活行 store 文件，作品正被分享拉取持有时在途拉取会读到源文件消失，拒绝执行并返回 `shareLock.ErrWorkLocked`（上层透传，用户知情强制解锁后重试本操作）。
- **`RestoreReplacedStores(ctx, scope RestoreScope)`**：失败回滚复活——备份还原文件（还原后清理备份）、批量复活行、重算 victim 所属资源完整度。清单来源两途（`RestoreScope`）：`WorkID` 数据驱动派生（插件任务，按挂载键同键最新死代圈定，软删行即持久还原点；Roles 同为显式集合，空集=无 victim）/ `Victims` 显式清单（策略任务，执行器软删成功后登记的多作品清单）；`WorkID` 途带作品活性守卫（作品已软删则回滚让位）。
- **派生函数**：`deriveReplaceVictims`/`replaceVictimKey`（同键最新死代圈定，活行残留的键跳过——复活会撞部分唯一索引）随迁本模块。
- **依赖注入**（接口隔离）：`ReplaceResourceLister`/`ResourceRecomputer`（resource.Service）、`ReplaceResourceStoreLister`（ResourceStoreRepository）、`ReplaceStoreRowReader`/`ReplaceStoreDeleter`（persistentStore.Service）、`ReplaceBackupRestorer`（backup.Service）、`ReplaceWorkLivenessReader`（work.Service）、`ReplaceWorkDirProvider`（settings.Service）、`ReplaceWorkLockChecker`（shareLock.ShareLockRegistry——前置作品锁守卫）。插件任务侧的「清本次新建 store」（依赖执行期 streams 状态）仍由发起方（taskManager 失败回滚单点）在调用前完成，不在能力内。

## 依赖关系

- 依赖（MergeService）：**merge**（Merger）、**persistentStore**（StoreOps）、**settings**（MergeSettingsReader）、database（Transactor）、Wails 事件管道（MergeEventEmitter，经闭包延迟读取，merge-events topic）、**shareLock**（MergeWorkLockChecker——overwrite 原轨道置换前置作品锁守卫）
- 依赖（ReplacementService）：**shareLock**（ReplaceWorkLockChecker——替换前置作品锁守卫；其余依赖见「替换链能力」节）
- 被依赖：**work**（ResourceUpdater）、**task**（任务产出资源）、**taskManager**（`ListStoreTypeSetsByWorkIds`——覆盖确认行级判定查已有作品**活行** store 角色集合，软删残留代不算；`RecomputeResourceComplete`——完整度共享重算；`ReplaceStoreOps`——替换链能力，前置软删/失败回滚复活）、**recycleBin**（`RecomputeResourceComplete`——复原置换后重算）、**merge**（由本模块 MergeService 编排）

## 关键设计

- **合并的模块边界**：merge 包输入输出均为文件路径，不感知 store/resource；产物路径生成（`store/resource/...`）归 persistentStore（BuildVariantPath），本模块只做编排。

> 历史 `Enabled` 字段已移除（无激活/禁用 UI，恒为 true，过滤冗余）；`GetEnabledByWorkId` 已删，改用 `ListByWorkId`。
