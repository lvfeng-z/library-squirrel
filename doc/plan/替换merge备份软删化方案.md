# 替换/merge 备份软删化方案（J'，吞并 E）

## 审查摘要

**代号说明**（任务派生图节点与外部引用，供无上下文读者）：E=原「替换场景备份还原点持久化」分支（已裁决并入本任务不独立立项）；F=备份管理界面（已完成的前置任务，含 GUI 批量删备份的引用守卫）；J=回收站管理单位泛化（本任务前置，已完成）；J'=本任务；「甲方向」=2026-08-21 用户拍板的软删化方向裁决；「红线 N」=用户任务书所列实施红线条款的序号（共 6 条：消费面审计先行/在途还原点保护/merge 两策略/软删写入口单点/前端规范/注释纪律）。

**关键声明（抽查项，file:line 均为本次调研实读锚点）**：

- 声明1（两处改造调用点）：替换链备份入口 `BackupStores` 内逐行 `HardDelete(rs.StoreID, true)`（backend/backup/store_backup_orchestrator.go:131），由 `m.storeBackupItems = ...BackupStores(...)` 调用（backend/taskManager/model.go:879）；merge overwrite 删原轨道 `HardDelete(videoPS.GetID(), true)` / `HardDelete(audioPS.GetID(), true)`（backend/resource/merge_service.go:246-250），随后显式删轨道关联 `DeleteByResourceIdAndTypes`（merge_service.go:254）。
- 声明2（软删写入口已就位，红线4 可零新建仓储）：`DeleteWithBackup`（backend/persistentStore/service.go:736）= MoveToBackup 移文件 + `SoftDeleteWithBackup` 单条 UPDATE 同生共死写 backup_id+deleted_at（backend/persistentStore/repository.go:89-98，含 `WHERE deleted_at = 0` 活行守卫）；复活 `RestoreByIds` 双列同清（repository.go:76-85）。契约差异：`HardDelete(id,true)` 仅备份已完成文件、备份失败降级直删、物理删行；`DeleteWithBackup` 不看完成态、失败报错中断（保全优先）、软删行。
- 声明3（内存清单及还原链现状）：`storeBackupItems` 字段（backend/taskManager/model.go:380），失败还原 defer（model.go:773-787：cleanupCreatedStores → RestoreAllStores → remountRestoredStores），重挂逻辑 `remountRestoredStores`/`mountResourceStores`（model.go:1204-1224 / 1152-1180），新建 store 清理 `cleanupCreatedStores`（model.go:1186-1202）。跨重启续传 `resumeFromPersistedState`（model.go:1440-1661）无还原链（E 问题的本体）。
- 声明4（挂载写路径现状）：`mountResourceStores` 先 `DeleteByResourceIdAndTypes`（删同 role 全部关联，不分死活）再插入（model.go:1151-1180；SQL backend/resource/resource_store_repository.go:65-77）。J' 后其调用方仅剩 saveResource 与 resume 全量重挂（merge_service.go:254 的调用随本方案移除）。
- 声明5（消费面审计结论，11 面——超出决策 6 所列 3 面，审计先行红线的实证）：见第五节审计表，需改动 6 面（DTO 组装、完整度重算双副本、GetByType、跨重启续传、作品复原链、GetStoreRelPath），零改动 5 面（任务查重、作品软删链、作品彻底删除链、/store/ 路由、fsmonitor 对账，各有理由）。表内 11 个锚点均为本次调研逐一实读（对抗式审查抽核其中 2 项证实）。
- 声明6（双代同路径合法的前提已就位）：file_path 部分唯一索引 `idx_persistent_store_file_path_active ... WHERE deleted_at = 0`（backend/migration/migrate.go:134-138）；`GetByFilePath` 走活行 scope（resource_store 无关；persistentStore/repository.go:27-43），`StoreStream`/`Store` 的「已存在」检查据此对软删行 miss → 同路径 INSERT 新行合法（persistentStore/service.go:366-378）。
- 声明7（J 资产就位）：store 条目谓词 EXISTS 集合语义天然兼容双关联（backend/search/repository.go:615-667）；挂载上下文批查仅登记活作品挂载、同 store 多挂载取首个（search/repository.go:758-794）；TTL 二轮与列表同谓词（backend/recycleBin/service.go:357-389）；`PurgeStore` 消费式删备份（recycleBin/service.go:294-322）。
- 声明8（红线 2 链条闭合验证）：victim 软删行持 backup_id > 0 → 引用集 `ListReferencedBackupIDs` Unscoped 含已删行（backend/persistentStore/service.go:961-963、repository.go:123-134）→ F 的 GUI 删备份守卫与治理正向圈定同源引用集 → 在途还原点不被误杀。J' 不新增绕过该链的引用形态。
- 声明9（「终态直清+指纹清扫」考古裁决）：全仓无两机制的存量实现——grep `终态直清|指纹清扫|terminalClean|FingerprintSweep` 于 `backend/**/*.go` 与 `frontend/src/**/*.{ts,vue}` 均零命中（2026-08-22 复核）。它们是甲方向裁决前设计迭代中的提案（TREE 决策记录），被「软删入回收站 + TTL」取代，无代码可取消；J' 亦不引入（裁决见第七节）。
- 声明10（跨重启 victim 派生的输入可用）：`runMode` 由 task 实体派生（backend/taskManager/manager.go:1298 `runModeFromTask`，model.go:72-80），跨重启后 `storeRoles` 可得；`resumeFromPersistedState` 设 `m.workId = resource.WorkID`（model.go:1489）。
- 声明11（作品级联对残留行的覆盖）：`DeleteWorkAndSurroundingData` 经关联全量收集 storeIds 事务内 `DeleteUnscopedByIds` 物理删（含软删行），关联行随 `DeleteByResourceIds` 级联删（backend/work/service.go:499-530）；`PurgeWork` 的备份收集用 `ListWorkStoresIncludeDeleted`（经关联，含残留行 → 其 backup 一并清，backend/work/service.go:618-625 + recycleBin/service.go:270-277）。关联保留形态下 purge 天然净化残留。
- 声明12（作品软删链对已删行的天然跳过）：`SoftDeleteWork` 循环 `DeleteWithBackup`，其内部 `GetById` 活行 scope 对已软删行 miss → 返回 0 跳过（work/service.go:573-583 + persistentStore/service.go:738-744）。
- 声明13（merge 补偿路径不受波及）：挂载失败补偿 `HardDelete(mergedPsId, false)`（merge_service.go:237）删的是本流程新建产物（物理删、无备份语义），J' 保留。
- 声明14（merge 策略现状）：`MergeStrategyKeep`（保留原轨道）/`MergeStrategyOverwrite`（删除原轨道 store 及文件），默认 keep（backend/settings/model.go:77-78、service.go:58）。

**待决策（需用户裁决，均附建议）**：

- 决策1：merge overwrite 语义变化确认——原轨道从「物理删除（不可恢复）」变为「软删入回收站（默认 30 天内可复原、TTL 收尾）」。严格减少数据损失，settings 文案「删除原轨道 store 及文件」补「（转入回收站）」。**已裁决：接受**。
- 决策2：RestoreStore 置换的对称性——复原某历史版本时，同 file_path 的当前活行先软删入回收站（自身亦可再复原/由 TTL 收尾），不新增「删活行」专机制。**已裁决：接受**。
- 决策3：未完成行（completed_at=0 的活行）进入软删产道时一律走废弃分支——不走备份（partial 文件入 backup/ 会膨胀且无复原价值），改为「废弃文件 + 无备份软删」，失败还原时该类行复活后无文件、交 fsmonitor 对账裁决。对齐现 `HardDelete(id,true)` 只备份已完成文件的既有语义。该原则适用于两个产道：**替换备份步**（被替换对象是未完成行）与 **RestoreStore 置换步**（同路径占位行是未完成行，如替换中断期的 partial 新行）——原独立的「决策7」系审查补遗，与本文决策同源，已并入。**已裁决：接受**。
- 决策4：停止（Stop）在途替换任务时自动回滚到替换前状态——含跨重启后 Stop（旧模型跨重启无还原能力，新模型补齐，E 吞并的兑现）。与现行失败还原语义一致。**已裁决：接受**。
- 决策5：消费面零改动 5 项结论确认（第五节审计表，各附理由）。**已裁决：确认**。
- 决策6：作品删除 × 在途替换并发时的让位语义——作品删除链停止任务后，还原链守卫检测作品已软删则跳过回滚（两代皆留已删态，归作品条目管理；作品复原取最新死代=删除时活代，如需更早版本经 RestoreStore 人工置换）。**已裁决：接受**。

**自曝风险**：

- 风险1：**「同键最新死代」派生规则依赖 deleted_at 毫秒时间戳可区分代次**——同毫秒多代删除理论混淆（概率极低）；命中时复活错代，但仍是该键的合法一代（argmax 单选不产生双活同路径的非法态）。
- 风险2：**作品删除 × 在途替换并发**（决策6）中，作品删除链会把在途 partial 新行软删（其 deleted_at 晚于替换 victim）→ 作品复原将复活 partial 新代而非替换前原代——原代仍在文件条目中可人工 RestoreStore 置换换回。不产生非法态，但「复原得到部分文件」对用户有惊讶面。测试锚定。
- 风险3：**长暂停超保留期**——victim 行被 TTL 清理后任务才失败/停止 → 还原派生 no-op → 被替换角色缺失（资源完整度降级），用户可重新下载恢复。属用户超期放弃的合理代价。
- 风险4：**`RecomputeResourceComplete` 抽取的等价性**——merge/taskManager 两处私有重算副本收敛为共享能力，行为须等价（活行过滤为新增语义）；以双副本现有行为为基线写锚定测试。
- 风险5：**GetByType 语义变更为接口级**——加活行过滤后仅 merge 三处消费（已核无其他调用方），若未来有「查关联无论死活」的需求需另立方法（当前无此需求，不留占位）。
- 风险6：**并发双替换任务打同作品**为既有危害（旧模型同样存在，StoreStream 同路径互踩），非 J' 引入、不在本方案扩 scope；J' 的 victim 派生（argmax）在该场景下取最新死代，不加剧非法态。
- 风险7：**失败还原的文件序**——cleanupCreatedStores 先物理删新行+文件、再还原 victim 文件到原路径；若新下载写的是同路径，删除与还原的先后顺序保证路径净空（实现顺序固定，测试覆盖）。跨进程重启后 m.streams 为空（新建行经 resume 换血），残余 partial 行的清理依赖 Stop 后还原链对「无关联活行」的处理（按 DeleteByStoreIds 圈定 m.streams 内 ID；重启场景 partial 行即 resume 续传行，Stop 由 handleStopCmd abort 物理删，同路径冲突不存在）。

---

## 一、背景与定位

J'（甲方向裁决，2026-08-21）：替换/merge 场景 `HardDelete(true)` 改软删（同生共死 backup_id），在途备份从「内存引用+保留期时间垫」升格为「持久行内引用」；失败还原链简化为复活原行（吞并 E——内存清单退役，软删行本身即持久还原点）；成功/中断残留由 J 的 store 条目 TTL 收尾。前置 J 已完成（33e33d5）。

**定位**：本方案是 J' 的实施设计，收口 J 延后的决策 5（RestoreStore 操作接通）与决策 6（消费面审计）。核心新命题是**关联保留**：替换软删行不再摘除其 resource_store 关联，换取「残留行始终可联作品、work Purge 顺带级联、离链产道结构性关闭」三收益，代价是同 (resource, role) 双关联形态——全部消费面须按行活性过滤与按 role 去重。

## 二、核心设计：同键最新死代不变量（关联保留模型）

**键** = `(resource_id, store_type, store_seq)`（挂载身份，resume 续传已用同一身份 model.go:1593）。

**挂载写路径维持**：每键活行至多一条——`mountResourceStores` 的「先删同 role 旧关联」改为**只摘指向活行的关联**（`DeleteByResourceIdAndTypes` SQL 加 `AND store_id IN (SELECT id FROM persistent_store WHERE deleted_at = 0)`，语义在原方法上收紧，调用方零改动；J' 后该方法仅剩 mountResourceStores 一个调用方）。

**软删路径新增形态**：软删不摘关联 → 每键呈「活行 0..1 条 + 死行 0..N 条（按 deleted_at 降序即代次）」。死行挂活作品 → J 的 store 条目（TTL 按行自身 deleted_at）；死行挂已删作品 → work 条目聚合。

**复活规则（本方案的不变量，三个复活消费方共用）**：

> 每键只有**最新死代**（argmax deleted_at）是复活候选；更早死代是历史残留（回收站条目 / TTL 终态）。

- **J' 失败还原**：victim = 替换角色键的最新死代（替换杀的正是最近的活代）。
- **作品复原**：作品删除链最后杀的就是删除时的活代 → 每键最新死代 = 复原目标；更早代（替换残留、MarkInvalid 前代）保持死态留在文件条目。作品已删窗口内无新软删写入（替换需活作品、fsmonitor 无 store/ 文件可裁决、RestoreStore 守卫拒绝非活作品挂载），规则自洽。
- **RestoreStore 人工复原**：直接指定行，但置换操作把当前活行软删后，该键最新死代自然更新为「被置换行」——若随后作品删除再复原，复活的是置换前活代（即用户人工选定的那代），规则仍自洽。

**为什么必须此规则（不加的后果）**：残留行与替换它的活行**同 file_path**（新下载 StoreStream 对软删行 miss 后同路径 INSERT，声明 6）——若作品复原无差别复活全部关联行，两活行同路径撞部分唯一索引，`RestoreByIds` 整批 UPDATE 失败，**作品复原链断裂**；即使不撞路径，restoreWorkFiles 也会两代文件互相覆盖。按键取最新死代后，同键至多复活一行，路径唯一性保持。

## 三、替换链改造（taskManager）

### 3.1 备份步骤 → 软删 victim（原 BackupStores 位点，model.go:878-880）

taskManager 私有方法 `softDeleteReplaceTargets(ctx, workId, roles)`（发起方编排，ORCHESTRATION_BY_CALLER；backup 包编排器整体退役，见第九节）：

1. `ResourceReader.ListByWorkId(workId)` → 逐资源 `ResourceStoreReader.ListStoresByResourceIds`（批量变体，接口扩展）→ 按 roles 过滤（空=全量）。
2. `ListByIdsIncludeDeleted` 批量取行判活性：已删行跳过（历史残留不动）；活行分两支：
   - **已完成**（CompletedAt>0）→ `DeleteWithBackup(storeId)`（移文件入 backup/ + 同生共死软删；失败报错中断——保全优先，替换流程 fail-fast）。
   - **未完成** → `SoftDeleteAndDiscardFile(storeId)`（persistentStore.Service 新薄封装：`CleanupFile` + `repo.SoftDeleteWithBackup(id, 0)`；软删写入口仍是 SoftDeleteWithBackup 单点，红线4。决策3）。
3. 关联**不摘**（关联保留）。

### 3.2 失败还原链 → 复活原行（替换 run() defer，model.go:773-787）

新私有方法 `restoreReplaceTargets(ctx)`（数据驱动、无内存态——崩溃重启后同样可派生，E 的兑现）：

1. **作品活性守卫**（决策6）：work 已软删 → 记日志跳过（两代归作品条目）。
2. `cleanupCreatedStores`（不变）+ **新增**：`ResourceStoreWriter.DeleteByStoreIds(m.streams 的 storeId 集)`——新行的关联必须显式摘（活行过滤版 DeleteByResourceIdAndTypes 摘不到指向已物理删行的关联）。
3. **victim 派生**：work 的资源 × roles（runMode.storeRoles，跨重启可用——声明10）→ 关联 → 行集（ListByIdsIncludeDeleted）→ 按键分组取最新死代。
4. 逐 victim：`RestoreFile(backupPath → 原路径)`（`storeRegistry.Suppress/Release` 登记，参照 restoreWorkFiles 先例 recycleBin/service.go:241-246）→ `DeleteBackup(backupId)`；随后 `RestoreByIds(ids)` 一次性复活（双列同清）。
5. victim 关联从未摘除，复活即挂载回位——**无重挂步骤**（remountRestoredStores 退役）。

### 3.3 跨重启续传补还原链（resumeFromPersistedState）

- **消费面修复（审计 #5）**：step 2 的 `storeRows` 按 store 活性过滤（批量 GetByIds 判活）——死行关联不再进续传判定（否则被当「store 记录丢失」误整轨重下、findStoreRowByIdentity 首匹配错位）。
- **失败路径**：加与 run() 相同的失败还原调用（`m.setFailed` 的各分支后统一经 defer 或显式收口到 `restoreReplaceTargets`）。守卫与派生同 3.2。

### 3.4 storeBackupItems 体系退役

`storeBackupItems` 字段、`remountRestoredStores`、`findReplaceResource` 的清单首项定位（回退 ListByWorkId 首资源即全量）、TaskDeps 的 `StoreBackupOrchestrator` 依赖全部删除（DEAD_CODE_CLEANUP）。`isReplace` 保留（备份决策照旧）。`markResourceComplete` 改调共享重算能力（第五节 #2）。

## 四、merge 链改造（resource/merge_service.go）

overwrite 分支（merge_service.go:245-258）：

- `HardDelete(videoPS/audioPS, true)` ×2 → `DeleteWithBackup` ×2（StoreOps 接口扩该方法）。轨道行软删带 backup_id，入回收站文件条目（CanRestore=true——**merge 从此可人工回滚**）。
- `DeleteByResourceIdAndTypes(...videoTrack/audioTrack)` **调用移除**（关联保留）。
- keep 策略零改动（决策1 的另一半）。
- `recomputeComplete` 私有副本删除，改调共享重算（第五节 #2）——overwrite 后轨道关联仍在（指向死行），活性过滤后计数正确（videoMain 在、轨道不计超量）。
- 补偿路径 `HardDelete(mergedPsId, false)` 保留（声明13）。

## 五、消费面审计与修复（决策 6 收口 + 审计扩展）

| # | 消费面 | 位置 | 双关联下现状行为 | 裁决 |
|---|---|---|---|---|
| 1 | 展示主体解析与 Stores 组装 | dto/resource_dto.go:107-149（NewResourceFullDTO + ResolvePrimaryStore 调用）；调用点 search/service.go:311、work/service.go:851 | storeMap 由活行 scope 的 GetByIds 构建（work/service.go:721-727）→ 死行关联的 Store 为 nil、ResolvePrimaryStore 可选中死行关联致 WorkStore 落空/封面丢失 | **修**：NewResourceFullDTO 内按 storeMap 命中过滤——死行关联不进 Stores、不参与 ThumbnailStore/WorkStore 派生（单点修复覆盖两调用方） |
| 2 | 完整度重算 | merge_service.go:292-320 与 model.go:1094-1127 双私有副本 | ListByResourceId 全量计数，同 role 双关联计 2 → Max 受限类型误判超量（complete=2） | **修**：抽取共享能力 `resource.Service.RecomputeResourceComplete(ctx, resourceId)`（ListByResourceId → 活性过滤 → ComputeResourceComplete → Updates），merge/taskManager/recycleBin（RestoreStore 后）三方消费；两私有副本删除 |
| 3 | 任务查重（行级覆盖判定） | resource/service.go:108-145（ListStoreTypeSetsByWorkIds）；taskManager model.go:820-858、manager.go:499 | set 语义天然按 role 去重；在途替换期死行关联令角色仍在集合 | ~~零改动~~ **实测修正（2026-08-22 dev 手测问题1）**：终态残留形态（merge overwrite 轨道仅剩死行）下死行角色仍进集合，同作品重下永远弹覆盖确认——改为活行角色集合（`ListAliveByResourceIds` JOIN 判活）。设计期「零改动」结论只考虑了在途替换期场景，漏终态残留形态 |
| 4 | merge 幂等/缺轨检查 | GetByType（resource_store_repository.go:54-62），merge_service.go:131/140/147 | 首个命中可为死行关联：overwrite 后重合并误判 AlreadyMerged、版本回滚后误判缺轨 | **修**：GetByType 加活性过滤（EXISTS persistent_store deleted_at=0）；仅 merge 三处消费（已核） |
| 5 | 跨重启续传 | model.go:1479-1525 | 死行关联被当「store 记录丢失」→ 误整轨重下；findStoreRowByIdentity 首匹配错位 | **修**：storeRows 活性过滤（3.3） |
| 6 | 作品复原链 | work/service.go:650-656（RestoreWorkStores）+ recycleBin/service.go:229-256（restoreWorkFiles）+ recycleBin/service.go:216-223 | 按关联全量复活 + 全量还原文件：双代同路径撞部分唯一索引 → 复原失败；文件互相覆盖 | **修**：work.Service 私有 `deriveRevivableStoreIds`（同键最新死代）+ 公开 `ListRevivableWorkStores(workId)`；`RestoreWorkStores` 内部改用派生集；recycleBin `restoreWorkFiles` 改圈定复活集（`ListWorkStoresIncludeDeleted` 保留给 PurgeWork 备份收集——全量语义正确） |
| 7 | 作品软删链 | work/service.go:573-583 | DeleteWithBackup 内 GetById 活行 scope 对已删行 miss 跳过 | **零改动**（声明12，天然正确） |
| 8 | 作品彻底删除链 | work/service.go:488-541 + PurgeWork | 关联全量收集 → 死行随级联物理删、其备份随 PurgeWork 清 | **零改动**（声明11，残留随作品净化=设计意图） |
| 9 | /store/ 文件路由 | persistentStore/service.go:908-929（ResolveFileState） | 活行优先、全删取最新删代 | **零改动** |
| 10 | fsmonitor 对账 | persistentStore/service.go:632-639 等活行 scope 查询 | 软删行不入对账基线 | **零改动** |
| 11 | 插件 GetStoreRelPath | app.go:688-715（storePathQueryAdapter） | 死行关联先命中 (role,seq) 时 GetById 报错**中断查询**，即使后面有活行 | **修**：GetById NotFound 视为「非本代」continue（活性过滤） |

**挂载写路径**（第二节）：`DeleteByResourceIdAndTypes` 收紧为只摘活行关联——这是双关联形态的**生产侧**约定，与消费侧过滤配套。

## 六、RestoreStore 操作接通（J 决策 5）

recycleBin 新增（Handler 面从五方法扩为六，命名对齐实体归属）：

```go
// RestoreStore 复原文件条目（版本回滚置换：行内备份还原为当前版本，被替换的当前版本转入回收站）
func (h *Handler) RestoreStore(ctx context.Context, storeId int64) *model.ApiResponse[any]
```

Service 流程（决策2 置换对称性）：

1. **校验**：`GetDeletedStore(storeId)`（StoreCleaner 已有）非空且 `BackupID > 0`；挂载可达且作品活（`RecycleStoreQuerier` 扩 `GetRecycleStoreMount(ctx, storeId) (resourceId int64, workAlive bool, err error)`——search 侧复用 queryStoreMountContext 查询族加 rs.resource_id 投影）。任一不满足返回明确错误（MarkInvalid 行/离链行不可复原，与前端 CanRestore 三态一致）。
2. **置换**：`GetByFilePath(本行 file_path)` 活行占位且属于**同 resource**（跨 resource 占位为路径冲突异常态，报错拒绝人工处理）→ 按占位行完成态分派（决策 3 的两个产道之一）：已完成 `DeleteWithBackup(占位行)`（其自身入回收站，可再复原）；未完成 `SoftDeleteAndDiscardFile(占位行)`（partial 文件废弃，不入备份）。
3. **文件还原**：`RestoreFile(backupPath → file_path 原路径)`，Suppress/Release 登记（restoreWorkFiles 同款）。
4. **复活**：`RestoreByIds([]{storeId})`（StoreCleaner 扩该方法）。
5. **清备份**：`DeleteBackup(backupId)`（失败容忍——行已复活 backup_id 已清，残余清单行由治理反向对账兜底）。
6. **重算完整度**：`RecomputeResourceComplete(resourceId)`（角色构成可能变化，如 merge 回滚补回轨道）。

**关联处置**：零操作——本行关联保留在（复活即挂载回位）、占位行关联保留成死（双关联标准形态）。与 J 方案「对偶替换不动关联」预设一致。

**幂等性**：步骤 2-4 任一步失败后重试安全（占位行已死则置换步跳过；文件还原可重跑；复活幂等）。

## 七、残留收尾与「终态直清+指纹清扫」裁决

- **成功残留**：替换/merge overwrite 成功后 victim 软删行留驻——回收站文件条目可见（CanRestore=true）、TTL 二轮按行自身 deleted_at 收尾（J 已就位，声明7）。无终态直清动作。
- **中断残留**：暂停/崩溃后 victim 持久在；续传成功 → 同成功残留；停止/失败 → 3.2/3.3 还原链回滚；超保留期长暂停 → TTL 已清、还原 no-op（风险3）。
- **裁决（声明9）**：「终态直清」（任务终态时主动清残留）与「指纹清扫」（按内容指纹清孤儿文件）系甲方向前的设计提案，无存量代码；在软删+TTL 模型下前者被「可见+可反悔+自动收尾」全面取代、后者失去对象（文件随行入 backup/ 被清单行引用，行亡备份随删，无孤儿文件产道）。**两机制确认不引入**。

## 八、在途还原点保护（红线 2 验证）

victim 软删行 backup_id 行内引用 → `ListReferencedBackupIDs`（Unscoped 含已删行）→ F 备份管理「删除守卫/清理全部无主/统计引用态」与 backupGovernance 正向圈定同源引用集 → 在途还原点结构性受保护（声明8）。J' 全部新增引用（替换 victim、merge 轨道、RestoreStore 置换行）均经 `SoftDeleteWithBackup` 写入同一条链，无绕行引用形态。监视哨引用年龄 Warn（90 天）> TTL 默认 30 天 → 正常收口不告警。

## 九、backup 包收缩与装配

- **退役删除**：`StoreBackupOrchestratorImpl`、`StoreBackupItem`、五个注入接口（StoreResourceProvider/StoreResourceStoreReader/StoreDeleter/StoreImporter/BackupReader 的 orchestrator 副本）、`NewStoreBackupOrchestrator`（backend/backup/store_backup_orchestrator.go 整文件）；backup README 同步。backup 包回归纯保管清单定位（消除编排器驻留能力包的边界瑕疵）。
- **TaskDeps 改造**：删 `StoreBackupOrchestrator`；增 `StoreReplacer{DeleteWithBackup, SoftDeleteAndDiscardFile}`、`StoreBackupReader{ListByIdsIncludeDeleted, RestoreByIds}`、`BackupFileRestorer{GetById, GetBackupPath, RestoreFile, DeleteBackup}`（均由 persistentStore.Service / backup.Service 既有方法满足）；`ResourceStoreReader` 扩批量变体、`ResourceStoreWriter` 扩 `DeleteByStoreIds`、新增 `ResourceRecomputer{RecomputeResourceComplete}`（resource.Service）。
- **app.go**：orchestrator 装配段（app.go:1031-1039、1056）删除，新依赖注入；recycleBin 装配扩三个依赖（StoreCleaner 扩两方法、RecycleStoreQuerier 扩一方法、ResourceRecomputer）。
- bindings 再生成。

## 十、前端设计（红线 5，照 J/F 先例）

- RecycleBin.vue 文件 tab 操作列：**复原**按钮（`el-button size=small`，非破坏性不用 danger）——仅 `row.canRestore` 可用；不可用时禁用并以 tooltip 说明原因（无备份=已失效不可复原 / 离链=所属作品已不可达）。确认框文案表达置换语义（「将还原该版本；当前生效版本会转入回收站」）。成功后刷新文件条目列表。
- wrapper `recycleBin.ts` 增 `restoreStore(storeId)`（requireResponse，requireData=false）；DTO 从 `@bindings` 引用；StatusRegistry/tokens.css 零新增（三态 key 已登记）。

## 十一、实施步骤

1. **resource**：`GetByType` 活性过滤；`DeleteByResourceIdAndTypes` 收紧只摘活行；新增 `DeleteByStoreIds`、`Service.RecomputeResourceComplete`（含活性过滤）。
2. **persistentStore**：新增 `SoftDeleteAndDiscardFile`（CleanupFile + SoftDeleteWithBackup(id,0) 薄封装）。
3. **taskManager**：TaskDeps 改造；`softDeleteReplaceTargets`（3.1）；`restoreReplaceTargets`（3.2）替换 run() defer；resumeFromPersistedState 活性过滤 + 失败还原（3.3）；storeBackupItems/remount/findReplaceResource 清单项退役（3.4）；markResourceComplete 改共享重算。
4. **merge**：overwrite 分支切换 DeleteWithBackup、移除关联删除；recomputeComplete 副本删除改共享；StoreOps 扩接口。
5. **work**：`deriveRevivableStoreIds` + `ListRevivableWorkStores`；`RestoreWorkStores` 改派生集。
6. **recycleBin**：`restoreWorkFiles` 改圈定复活集；新增 `RestoreStore`（第六节）+ handler + 依赖注入面。
7. **search**：`GetRecycleStoreMount`（queryStoreMountContext 查询族扩展）。
8. **dto**：`NewResourceFullDTO` 按 storeMap 命中过滤（审计 #1）。
9. **backup**：orchestrator 整文件删除；README 同步。
10. **app.go**：装配改造；`wails3 generate bindings -ts`。
11. **前端**：复原按钮 + wrapper（第十节）。
12. **测试**（第十二节）+ 全量 `go test`（排除 build/）+ `yarn build` + dev 实机手测。
13. **文档**：taskManager/backup/recycleBin/resource/work 五模块 README、TREE.md J' 节点、（如触及）CLAUDE.md。

## 十二、测试计划

后端（基建平移 J 的 search/recycle_store_query_test.go 四形态 fixture、work/delete_purge_test.go、recycleBin/purge_store_test.go）：

1. **同键最新死代**：多代死行按键分组取最新（含同 resource 多 role、跨 resource 不串键）。
2. **替换备份**：软删后行 deleted_at>0 ∧ backup_id>0、文件入 backup/、关联保留；未完成行走废弃分支（无备份+文件清除）；已删行跳过。
3. **失败还原**（核心回归）：替换失败 → 新行物理消亡 + 新关联清理（DeleteByStoreIds）+ victim 复活（deleted_at=0 ∧ backup_id=0）+ 文件回原路径 + backup 清单行消亡 + **关联零重挂**（关联行 ID 不变）；多代只复活最新；work 已删守卫跳过；崩溃模拟（重建 ManagedTask from DB）后 Stop 仍可还原（E 兑现锚定）。
4. **merge**：overwrite 轨道软删入条目（CanRestore=true）、关联保留、完整度=1（轨道不计超量）；keep 零变化（行为基线锚定）；overwrite 后重合并可行（GetByType 活性）。
5. **消费面**：RecomputeResourceComplete 双关联计数=1（对比旧双副本行为基线）；GetByType 死行不命中；NewResourceFullDTO 死行过滤（Stores/WorkStore/ThumbnailStore）；resume 活性过滤（死行关联不触发重下）。
6. **作品复原**（部分唯一索引回归锚定）：双代同路径形态下 work 删→复原 → 只复活最新代、索引不炸、restoreWorkFiles 只还原复活集（旧代备份文件不被误耗）。
7. **RestoreStore**：CanRestore 行全链（置换活行入回收站/文件还原/复活/清备份/完整度重算）；MarkInvalid 行与离链行守卫拒绝；跨 resource 占位拒绝。
8. **在途还原点保护**：victim 引用使 F 删备份守卫拒绝（引用集 Unscoped 断言）；TTL 二轮清理含 J' 例行态残留。
9. **purge 回归**：work 级联含残留行与备份（既有测试扩残留形态 fixture）。

前端：`yarn build` 绿 + dev 实机手测（替换失败回滚、替换成功残留入条目、merge overwrite 入条目、文件条目复原置换、canRestore 三态禁用逻辑、作品删→复原残留形态）。

## 十三、红线对照

| 红线 | 落实 |
|---|---|
| 1 消费面审计先行 | 第五节 11 面审计表（3 项任务书所列 + 8 项审计扩展）逐面裁决，改动 6 面全部有锚点与改法 |
| 2 在途还原点保护 | 第八节：引用链经 SoftDeleteWithBackup 单点写入，Unscoped 引用集闭合，无新绕行形态 |
| 3 merge 两策略 | keep 零改动；overwrite 语义变化（决策1，已裁决接受：软删入回收站+settings 文案补注） |
| 4 软删写入口单点 | 全部软删经 `SoftDeleteWithBackup`（DeleteWithBackup / SoftDeleteAndDiscardFile 均为其上层编排），无新写入口 |
| 5 前端规范 | 第十节：StatusRegistry 复用、@bindings、requireResponse、danger 语义区分（复原非破坏性） |
| 6 注释纪律 | 实施时遵守：只写做什么/为什么，无变更叙事与计划标签 |
