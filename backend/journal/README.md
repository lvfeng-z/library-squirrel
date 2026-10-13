# journal 模块说明（记账目录治理）

## 一句话职责

领域侧的**迁移记账**：一张 append-only 账表 `journal_entry` 统一承载各领域的「需记账状态迁移」，供任何需要「变化史」的机制按游标消费。本模块提供写入器（提交后尽力写）、游标查询（`ListSince`）与轮转，**无 Handler、不直接暴露前端**；记账目录（哪些域、哪些事件落账）由领域需求治理，新域按本文件「新域入账指引」入册。

设计全貌见 `../../library-squirrel-docs/plan/事件记账基建方案.md`（谱系 external-ai-interface · B 节点）。

## 边界（与两个邻居，谱系决策勿回退）

- 与**外部操作记录**（接口侧账：只记外部 API 调用、问责用）：**两领域分离**。journal 记的是领域内部的状态迁移，定位是**感知保真而非问责账**——不追求账本完备，缺口由可推导查询兜底（见「写入时序」）。
- 与**通知系统**（给人看的 UX 策展事件）：**同源不同轨**——共享发射时刻（挂同一发射点位），**不共享记录形态与生命周期**。记账行的取舍口径（状态类 + 终态/建行）由本模块治理，不随通知策展规则变化。

## 账表形态

统一一张表、跨域一条游标流、不按域分表；行只追加不更新，**无软删语义、无外键**（老化行物理 `DELETE`，见「轮转参数」）。

- 实体：`JournalEntry`（`backend/base/model/entity/journal.go:17`，表名 `journal_entry`:29，工厂 `NewJournalEntry`:34）。
- 列：`id`（自增消费游标，全表单调递增）、`create_time`、`domain`、`event_type`、`subject_id`（域内主体引用）、`payload`（JSON 文本摘要，空串 = 无摘要）。
- 迁移注册：`backend/migration/migrate.go:151-152`（随 AutoMigrate 建表，无业务表改动）。
- 仓储：`JournalRepository`（`repository.go:19`）——`Create`、`ListSinceSnapshot`、`RotateOldest`；`dbFromCtx` 支持事务感知。

## 记账目录清单（当前两条）

| 域（domain 常量） | 事件类型 | 主体引用 | 载荷（payload） | 过滤边界 |
| --- | --- | --- | --- | --- |
| `task`（`entity.JournalDomainTask`，`entity/journal.go:9`） | `task_terminal`（`task_terminal_adapter.go:13`） | 任务 ID | `{taskId,taskName,state,stateName,parent}`（:16-27） | 仅状态推送方法 `PushStateChange`/`PushParentStateChange`，且状态 ∈ 三终态（:32-39） |
| `ingest`（`entity.JournalDomainIngest`，`entity/journal.go:11`） | `work_ingested`（`ingest_adapter.go:17`） | 作品 ID | `{workId,title,source}`，`source` ∈ {`plugin_download`,`manifest_import`}（:20-25） | 仅建行路径业务提交成功后，**每建行作品一行**（:138-142） |

## 新域入账指引

新增一个记账域 = **加目录**（常量 + 事件类型 + 载荷 + 适配器 + 装配点），**不新增写入口**。步骤：

1. **域常量入册**：在 `backend/base/model/entity/journal.go` 的封闭取值域内加 `JournalDomain*` 常量。域值入列前必须取常量，**禁止裸串散落**——域歧义会令不同目录的账行互串（`entity/journal.go:5-12`）。
2. **事件类型与载荷**：在本包内定义 `EventType*` 常量与载荷结构体（JSON 摘要）。主体引用放账行 `subject_id` 列，载荷内**冗余一份**便于消费方直接读摘要（先例：`task_terminal_adapter.go:16-27`、`ingest_adapter.go:28-35`）。
3. **写入口**：统一走 `Service.Append(ctx, domain, eventType, subjectID, payload)`（`service.go:89`）——内含写入校验（域/类型非空、主体引用为正，`service.go:26-33`）与写前顺带轮转（:99-101）。新域只定义目录与载荷，不改写入口。
4. **域内适配器（过滤边界在适配器内做）**：适配器文件放 `backend/journal/`，按事件源形态二选一：
   - **事件源型**（推送/回调）：组合器包住原接口——**先纯透传原实现（语义零变化），再落账**。实例 `TaskTerminalPusher`（`task_terminal_adapter.go:102`）：内嵌 `taskManager.TaskEventPusher` 接口而非逐方法实现，令形参未导出的进度方法经接口提升天然转发、**零落账**（:99-103、:117-126）。
   - **提交点型**（业务事务已提交后挂写）：装饰器包住原能力——成功返回后落账，**失败原样透传零落账**。实例 `SaveWorkInfoIngestDecorator`（`ingest_adapter.go:80`，包装 `SaveWorkInfo`，:94-101）与 `ManifestIngestJournalDecorator`（:123，包装 `importer.ManifestIngestor`，:140-150）。
   - **依赖方向纪律**：适配器可 import 领域包取类型；若该领域包是记账的**消费方**（如 download 依赖 `work.Service`），journal 侧按**同签名结构化接口**声明依赖面（`WorkInfoSaver` `ingest_adapter.go:73`、`WorkTitleReader` :113），装配期由装饰器满足——**禁止 journal 反向 import 消费方**（避免装配环）。
   - **健壮性约定**：`recorder`/`svc` 为 nil（未装配）时**静默跳过**，不影响业务；写账失败**仅日志、不上抛**（`ingest_adapter.go:54-68`、`task_terminal_adapter.go:88-93`）；落账用独立 `Background` 上下文，不随调用方任务取消而取消（`ingest_adapter.go:51-53`）。
5. **装配点**（`app.go`，被记账域**自身零改动**）：
   - 服务构造：`initBaseServices` `app.go:734`（`app.JournalService`，字段声明 :135）；
   - task 域（组合推送器）：`app.go:374` `SetPusher` 前经 `wrapTaskPusherWithJournal` 外包一路（`app.go:383-388`，nil/未装配安全回退）；
   - ingest 域（装饰器）：manifest 装饰器 `app.go:1252-1262`（分享收件与 zip 导入两执行器共用同一注入实例），`SaveWorkInfo` 装饰器 `app.go:1264-1271`。
   - 新域照此**在装配点外包注入**，原域代码不改。
6. **测试**：同目录 `*_adapter_test.go`，至少覆盖四类——业务返回值/错误原样透传、过滤集遍历（全枚举）、写账失败不影响业务返回、并发写（`-race`）。参照 `ingest_adapter_test.go`、`task_terminal_adapter_test.go`。

## 过滤边界（防写账风暴）

**只有「状态类且仅终态 / 仅建行」才落账；进度类与高频事件零落账**（方案风险3）。这不是可选优化：`Append` 顺带承担轮转触发（`service.go:85-88`），写入口天然假设低频；若组合器/装饰器误复制高频事件，会造成写账风暴。

- **task 域**：仅 `PushStateChange` / `PushParentStateChange` 两个方法落账，其余（进度批等）经内嵌接口提升转发、天然零落账。终态集 = `{Finished, Failed, PartlyFinished}`（`task_terminal_adapter.go:32-39`，与 `taskManager` 域内 `isImmediateTerminal` 同集）；`Paused` 是稳定态但可续传、**非终态不落账**；父任务终态一并落账，载荷 `parent=true` 区分（:25-26、:122-126）。判定不通过时**零落账零日志**（:70-73）。
- **ingest 域**：口径 = **建行**。两获取路径（插件下载 / manifest 回灌导入）建行后各落一行，主体 = 作品 ID；**替换、自动增补、查重跳过不新建作品行，故不落账**（`ingest_adapter.go:138-142`）。标题读取失败仅降级空标题、不阻断落账（:152-171）。

## 轮转参数

- **常量**（`service.go:15-24`，常量化；可配置需求出现时再议）：
  - `RotationTTL = 365 * 24h`——创建时刻早于该年龄的账行被裁剪（消费方可触及约一年历史，游标休眠近一年不过期）；
  - `RotationMaxRows = 200000`——存活账行超容量时裁最旧（≈548 事件/天持续一年的兜底上限，正常速率下 TTL 先于容量生效）。
- **实现**：单语句两条件 OR 裁最旧——删除 `create_time < now-TTL`，或 `id` 落在最新 20 万行之外（`repository.go:58-63`）；两条件都不命中时零命中（幂等）。`create_time` 索引（`idx_journal_entry_create_time`，迁移后置段命名建出）保证 TTL 轨为索引区间探测——轮转随每次写账顺带触发，无索引时 OR 谓词将退化为全表扫描。
- **触发**：`Append` **写前顺带触发**轮转，失败仅 Warn、**不阻断写账**（`service.go:99-101`）；`Rotate` 为**独立入口**（`service.go:142`），供启动/定期调度接入——**当前生产路径只有 Append 顺带这一处调用**。
- **过期游标语义**：游标非 0 且（表已被裁空 **或** 游标早于最旧存活账行 id）→ `CursorExpired=true`，消费方应**回退可推导查询补缺口**，不得把本次行集当完整历史；`cursor=0`（从头消费）**不判过期**——新消费从最旧存活行起读属正常引导（`service.go:66-70`、:126）。过期判定与行集在**同一读事务快照**内读取（`repository.go:32-53`），不会出现「过期判定落在裁剪前、行集落在裁剪后」的错位。

## 游标契约（`ListSince`，外部接口的依赖面）

`Service.ListSince(ctx, cursor, limit) (*ListResult, error)`（`service.go:115`；返回形态 `service.go:57-71`）——**签名与返回形态稳定优先**。

| 字段 | 消费方语义 |
| --- | --- |
| 入参 `cursor` | 返回 `id` **严格大于** cursor 的账行；`0` = 从头消费 |
| 入参 `limit` | `limit <= 0` 取缺省 `defaultListLimit = 100`（:23、:116-118） |
| `Entries` | 本次返回账行，`id` 升序；追平后为**空切片**（len=0，`service.go:61`） |
| `NextCursor` | 有返回行 = 末行 id；无返回行 = **回传传入游标**（消费位置不动） |
| `HasMore` | 库层多拉 1 行探测（:119-131），true 表示可立即续读；false = 已读到当前末尾 |
| `CursorExpired` | 见「轮转参数」——消费方收到后回退 `create_time`/状态过滤的推导查询补缺口 |

- 消费循环：`cursor=0` 起读 → 处理 `Entries` → `cursor = NextCursor` → `HasMore` 为 true 立即续读、false 则等待下次；任一轮 `CursorExpired` 即回退推导查询对齐。
- 错误面：读库失败包装返回（:121-123）；写入口校验错误 `ErrDomainEmpty` / `ErrEventTypeEmpty` / `ErrSubjectIDInvalid`（:26-33）。
- 消费方示例：外部 AI 操作接口（谱系 A 节点）规划的 `/api/journal` 端点以此契约为依赖面；本模块只提供契约，端点归 A 节点实施。

## 写入时序（提交后写、尽力而为）

- **提交后写**：调用方在**业务事务成功后**调用 `Append`；账行不参与业务事务（方案定论，自决项）。
- **尽力而为**：写账失败**仅日志告警，不回滚业务、不重试风暴**（`service.go:73-75`）。装饰器与组合器**不改变业务返回值与错误语义**——返回同一结果实例、失败原样透传（`ingest_adapter_test.go`、`task_terminal_adapter_test.go` 逐项断言）。
- **缺口兜底**：存在崩溃窗口（业务已提交、账未落，方案风险1），**已接受**——缺口由 `create_time`/状态过滤的**可推导查询**补，账本完备性不是目标。
- **并发**：多任务终态、多作品入库并发写账，行级 append 由自增 id 保序、无重复（`-race` 并发测试覆盖）。

## 验证

- 单测：`go test -race -count=1 ./backend/journal/...`（内存 SQLite 依赖 CGO；无 CGO 环境自动 Skip，`journal_test.go:25-36`）。
- 实机冒烟（真实下载/导入后经只读 DB 断言账行存在）属验收会话执行项，**本文档不作断言**。
