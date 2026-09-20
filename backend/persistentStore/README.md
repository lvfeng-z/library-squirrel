# persistentStore 模块说明

## 一句话职责

资源文件的**持久化存储**：管理 `persistent_store` 记录与对应的磁盘文件，提供入库事务（登记→落位→事务内建行）、按路径查询、删除（可联动备份）。本模块无 Handler，不直接暴露前端，由 download、import、resource（merge）等入库调用方与 backup、recycleBin 等内部调用。

## 边界

- 与 **resource**：resource 管 Resource 实体（作品的资源元数据抽象）；persistentStore 管具体的文件落盘与 `persistent_store` 记录。
- 与 **backup**：persistentStore 定义 `FileMover` 接口（移文件入 backup 建保管清单行，backup.Service 实现、经构造注入）；`DeleteWithBackup` 经它实现「移文件 + 软删记录 + 写行内 backup_id」同生共死。
- 与 **storeRegistry**：所有存储路径必须以已注册子目录开头（白名单单一源在 storeRegistry，见下），禁止裸路径。

## 对外能力（无 Handler，供内部调用）

| 方法 | 作用 |
| --- | --- |
| `PrepareIngest` / `PlaceIngest` / `CommitIngest` / `AbortIngest` | 入库事务四调用（唯一入库形态，见「入库事务机制」）：登记意图 → 落位 → 调用方业务事务内建行并删登记行；异常按登记声明撤回 |
| `RecoverIngest` | 启动恢复：逐登记行按「文件在最终路径与否 × 登记处置」收口（见「入库事务机制」） |
| `CommitStore` | `CommitIngest` 的建行步骤（文件须已落位最终路径）：建/复用完整行（completed_at 即时置位 + 宽高/头指纹按最终路径提取 + 哈希双列由调用方传入）；同路径已有活行时复用该行全字段覆写（Save 含零值清旧哈希） |
| `Delete(id, backup)` | 删除记录与文件（backup=true 联动备份，仅已完成文件、失败降级直接删除）；记录不存在视为已删除，返回 `(0,nil)` 而非错误 |
| `HardDelete(id, backup)` | 物理删记录 + 删文件（查行走 GetById 受软删 scope 保护——**对已软删行静默跳过**，适用于活行） |
| `DeleteUnscopedByIds(ids)` | 批量物理删行（单条 DELETE IN，dbFromCtx 模式可入事务；对已软删行的直删通路——purge 链专用） |
| `GetDeletedStore(id)` | 按 ID 获取已软删行（nil = 不存在或非已删态；回收站文件条目清理链入口校验） |
| `DeleteWithBackup(id)` | 软删链的文件侧：移文件入 backup + **记录软删**（与文件移动同生共死）；不看完成状态、失败返回错误（契约与 `HardDelete` 的区别见方法注释）。作品软删除、替换前置、merge overwrite、回收站复原置换共用 |
| `SoftDeleteAndDiscardFile(id)` | 软删记录并废弃其文件（未完成行进入软删产道的分支：partial 文件无复原价值不入备份，尽力删+抑制登记；软删仍经 SoftDeleteWithBackup 单点写入 backup_id=NULL） |
| `DeleteByFilePath` | 按路径删除（物理删） |
| `MarkInvalid(id)` / `RestoreByIds(ids)` / `ResolveFileState(relPath)` | 外部裁决失效（软删）/ 批量复活（复原/失败回滚/置换链，清软删标志与 backup_id 双列）/ 状态解析（/store/ 路由，含删口径） |
| `GetById` / `GetByIds` / `GetByFilePath` | 查询 |
| `Exists` | 存在性校验（/store/ 状态路由用 `ResolveFileState`，含删口径） |
| `ResolveStorePath` / `GetAbsPath` | 路径解析（relPath → 绝对路径） |

## 核心概念

- **已注册子目录**（`storeRegistry` 存储目录注册表）：白名单条目由各业务域在装配期注册（`app.go` `registerStoreDirs()`，先于 fsmonitor 启动——注册契约与消费面清单见 `backend/storeRegistry/README.md`），当前三条：`store/work`（属主 resource；作品资源，含缩略图与合并产物，命名见 `doc/store-naming-convention.md`）、`store/avatar/local`、`store/avatar/site`（属主 author 占位，头像域）。所有落盘路径必须命中注册子树（`ValidatePath` 闸门），代管能力面（落盘、指纹、记录行、抑制登记、URL 解析、软删语义）对全部注册子树一视同仁。
- **路径基准（PATH_SEPARATOR_DISCIPLINE 两域模型）**：所有相对路径（relPath 域）基于 workDir 且**正斜杠**——写入口（PrepareIngest/CommitStore）入口处 `ToSlash` 规范化一次，查旧/抑制登记/落库全程与 DB 基准一致；absPath（`filepath.Join(workDir, rel)`）仅存在于 os.* 调用点不回流。禁止 `../`、`./` 或绝对路径。

## 入库事务机制

persistent_store 行的创建只走四调用序列（非事务写入 API 不存在），崩溃后凭登记账本精确收口：

- **登记表 `store_ingest_journal`**（本模块自有账本，每文件一行）：`file_path`（最终落位路径，relPath 域正斜杠）、`staging_path`（内容当前所在暂存位置，处置为退回暂存时的退回目标）、`workdir`（登记时的库根——恢复只处理匹配行，不匹配行保留并记 Warn）、`abort_action`（调用方登记时声明的撤回处置：退回暂存/丢弃，**必填无默认**——该轨能否续传、是否可重产只有调用方知道，本模块只执行不推断）。无状态列：登记行存在 ⟺ 该次入库未提交。
- **四调用序列**：`PrepareIngest`（登记意图，独立事务立即提交，返回登记行 ID 清单）→ `PlaceIngest`（暂存同卷 rename 落位最终路径，操作抑制内置）→ `CommitIngest`（调用方业务事务内建 persistent_store 行 + **删登记行，两动作同事务**，ctx 未携带事务显式拒绝）→ 异常路径 `AbortIngest`（按各行登记声明的处置撤回，幂等）。调用方自持业务事务，编排归发起方（ORCHESTRATION_BY_CALLER）。
- **判据精确性**：删登记行与建行同事务——两行不可能一个存在另一个不存在。「行已建、引用未写」由业务事务原子性覆盖；「文件已落位、行未建」由先于落位持久化的登记行覆盖。
- **启动恢复 `RecoverIngest`**：app 启动序列调用（早于 fsmonitor 启动），逐登记行按「文件在最终路径与否 × 登记处置」收口——退回暂存（退回不可达时删文件兜底 + Warn）、丢弃删文件、未落位仅删登记行；单行失败不阻断其余行（登记行保留下次启动重试），工作目录未配置时短路返回。
- **撤回处置为何由调用方声明**：退回暂存的价值限于可续传轨（退回后恢复会话可直达提交点，保住已下载字节）；derived 一次性产物等退回无收益，声明丢弃。声明随登记行持久化，撤回与恢复同源执行。

设计全貌见 `../library-squirrel-docs/plan/persistent-store异常治理机制方案.md`。

## 依赖关系

- 依赖：workDir 提供者（根目录）
- 被依赖：**download**（提交点入库事务 StoreIngestor）、**import**（回灌入库 StoreIngestOperator，share-receive 执行器经其复用）、**resource**（merge 产物提交点 StoreIngestor）、**work**（软删除经 `DeleteWithBackup` 移文件并软删记录，复原经 `RestoreByIds` 复活，purge 经 `DeleteUnscopedByIds` 物理删行）、**taskManager**（替换前置软删 StoreReplacer / 失败回滚派生与复活 StoreBackupReader）、**recycleBin**（复原编排 StoreRestorer + 文件条目清理 StoreCleaner）、**fsmonitor**（StoreReader 对账 + 裁决失效经 MarkInvalid）、**assetserver**（/store/ 状态路由 ResolveFileState）、**backupGovernance**（BackupReferencer：引用集投影 `ListReferencedBackupIDs`（Unscoped 含已删行）、悬空清列 `ClearBackupRefsByBackupIDs`、非法活行防御清列）、**extension**（插件库查询：仓储 `ListByIds` 批量读 store 行组装活行摘要，软删行经 scope 自动排除）

## 关键设计

- **记录-文件不变量（2026-08-19 修复，2026-08-20 补 backup_id）**：状态字段如实反映物理世界——`completed_at`（落盘完成时刻，0=未完成）+ `deleted_at`（软删：文件移 backup 或 fsmonitor 外部裁决不复从，复原/裁决链清除）+ `backup_id`（备份清单行引用，软删链与 deleted_at 单条 UPDATE 同生共死写入、复原双列同清，外部删除失效行保持 0）。/store/ 文件服务按状态路由：软删记录按行内 backup_id 查 backup 保管清单定位文件（同路径多代各行指各的，代次隔离）。
- **行只在提交点建（必然完整）**：下载链走暂存模式——内容先写 `{workDir}/staging/download/`（fsmonitor 零感知），全部写满后由 download 提交点经入库事务序列落位到最终路径并在业务事务内建行（见「入库事务机制」），`completed_at` 即时置位，不产生未完成中间态行。`completed_at=0` 的未完成行为历史中间态遗留（存量库可能有）：/store/ 读取层按 completed 状态路由（未完成 404 防半成品）、对账基线 `ListValidComplete` 仅收 completed_at>0、其磁盘文件列 Untracked 提示，由 recycleBin/replacement 的未完成分流分支与删除链处置。
- **路径强校验**：`storeRegistry.ValidatePath` 拒绝未注册子目录，统一正斜杠比较以兼容 Windows。
- **操作抑制登记（suppression）**：各 Create/Remove/Rename 落盘点（`PlaceIngest`/`AbortIngest`/`Delete`/`DeleteWithBackup`/`CleanupFile`/`CleanupFileResult`）在磁盘操作前 `storeRegistry.Suppress(relPath)` + `defer Release`，让 fsmonitor 把自身写入与外部操作区分开。`CleanupFileResult` 为 `CleanupFile` 的返回错误变体（供删除流「先文件后记录」Phase A 判定文件删除是否真实失败）。backup 模块的 `MoveBackup` 亦在汇点自登记（覆盖所有移入 backup 的调用方）；落位（`PlaceIngest`）的 rename 窗口抑制内置其中。
- **fsmonitor 查询的软删排除**：store 行软删后经 GORM 自动 scope 从 StoreReader 三方法（`GetByFingerprint`/`GetByFilePathComplete`/`ListValidComplete`）排除——文件软删期间位于 backup/（监控白名单外）。曾用消费侧 JOIN work 的 `notDeletedWorkCond`（persistentStore 越界感知业务实体），已随软删落地删除。fsmonitor 外部裁决不复从经 `MarkInvalid`（软删）实现，原 invalid_at 列退役。
- **图像宽高提取**：`CommitStore` 建行时，若是图片（`util.IsImageExt`）则用 `image.DecodeConfig` 读头部解码填入 `Width`/`Height`（供前端瀑布流精准布局）。解码失败仅记日志、留 0，不阻断入库。
