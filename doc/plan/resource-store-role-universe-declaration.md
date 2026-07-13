# 资源板块指定协议细化：创建期声明 universe（节点 M）

> 谱系：multitrack-resource-lineage · 节点 M（派生自 B）
> 创建：2026-07-13 · 状态：已实施（2026-07-13，跨 SDK+主程序+pixiv+前端落地，各仓库编译通过；运行期验证待 pixiv 图片任务实测：Create 声明 [main] → 首跑不触发缩略图 → 403 消失）
> 字段分离策略：**A**（task 新增 `InvolvedRoles` 承载 universe，保留 `StoreRoles` 承载 selection）
> 范围：主程序 + SDK + pixiv 插件 + 前端（跨仓库）
> 关联：B 多板块模型（`doc/plan/主程序多轨资源与多流任务重构方案.md`）；F 连接复用（`doc/plan/pixiv-connection-reuse.md`）；代理修复（`doc/bug/pixiv规则模式VPN不可访问-代理仅读env不读系统代理.md` §6 缩略图 403 同源）

## 1. 背景与缺口

B 多板块模型引入了 role（main/thumbnail/videoTrack/audioTrack/merged）+ generation（downloaded/derived），但**未界定"谁是 role 适用性权威"**：

- **主程序硬编码全集**：`allStoreRoles`（`backend/taskManager/model.go:57`）= main+thumbnail+…，首次执行 `runModeFull` 无条件请求全集。
- **前端固定勾选集**：`frontend/src/constants/sectionCode.ts:11` `ALL_STORE_ROLES = [main, thumbnail]`——对所有任务展示同样的板块勾选项（且**漏了 videoTrack/audioTrack/merged**）。
- **板块只在 Start（下载）产出**：`TaskCreateResponse`（`library-squirrel-sdk/dto/handler_dto.go:34-43`）无板块字段，创建期不声明。

**后果**：pixiv（纯图片）被默认驱动产缩略图 → `fetchThumbnail` 被 CDN 拒（403，`log/server.log` 大量 `缩略图下载失败: HTTP 403`）；前端对图片任务展示无关的 videoTrack 等。

## 2. 目标

1. **插件在创建期声明本任务涉及的板块（universe）**，供主程序与前端按任务感知。
2. **前端按任务的 universe 动态渲染自选板块**（重执行 UI 只展示该任务涉及的板块）。
3. **兼容无法创建期确定板块的任务**：`default` 兜底，插件执行期下全量。
4. 顺带清理主程序 `allStoreRoles` 硬编码常量 + 前端固定 `ALL_STORE_ROLES`。

## 3. 核心决策（字段分离策略 A）

引入两个**正交概念**，分别落字段：

| 概念 | 字段 | 生命周期 | 含义 | NULL 语义 |
|---|---|---|---|---|
| **universe**（任务涉及板块） | `task.InvolvedRoles`（**新增**） | 创建期、稳定 | 此任务**能有哪些**板块 | undetermined/default → 执行期插件下全量 |
| **selection**（按次所选子集） | `task.StoreRoles`（**保留**，语义不变） | 按次、瞬态 | 此运行**要下哪些**（universe 子集） | 全量（= universe 全集或全下） |

**不复用 `task.StoreRoles`**——它是"按次选择"，与"创建期声明"生命周期不同；复用会导致重下残留值被误读为 universe（M 设计讨论中的主要风险，策略 A 规避）。

**sentinel**：`InvolvedRoles` NULL = undetermined/default（直接复用现有"NULL=全量"心智，零迁移负担）；非 NULL = 声明的 universe。不引入显式 "auto" token。

**声明 = hint，Start 产出 = truth**：universe 用于前端 UI + 主程序默认请求；**主程序挂载以插件 Start 实际产出为准**，不据 universe 拒绝超出 spec。保证声明不全（创建期信息不足）时仍健壮。

## 4. 数据模型与 DTO 变更

### 4.1 Task 实体（`backend/base/model/entity/task.go:25`）

`StoreRoles` 行后新增：

```go
InvolvedRoles sql.NullString `gorm:"column:involved_roles" json:"involvedRoles"` // 任务涉及的 store_type 集合(创建期声明,逗号分隔);NULL=未确定/默认,执行期插件下全量
```

GORM AutoMigrate 自动加列；老任务该列 NULL = default，无破坏。

### 4.2 Create 响应 DTO（`library-squirrel-sdk/dto/handler_dto.go:34-52`）

`TaskCreateResponse` 与 `TaskCreateChildResponse` 各加：

```go
InvolvedRoles []string `json:"involvedRoles"` // 任务涉及的 store_type 集合(创建期声明);空/nil=未确定,执行期插件下全量
```

### 4.3 Proto（`library-squirrel-sdk` 的 `.proto` → `gen/`）

Create 流式响应的 proto message（映射 TaskCreateResponse，含 child）加 `repeated string involved_roles`；`make` 重新生成 `gen/`。主程序 `task_handler_proxy.go` 的 `protoToTaskCreateResponse`（:405-425）/ child 转换同步加字段。

## 5. 主程序变更

### 5.1 Create 持久化注入（`backend/task/service.go:596` `handleCreateTaskArray`）

把 plugin Create 响应的 `InvolvedRoles` 写入新建 task 实体的 `InvolvedRoles` 字段（逗号分隔 `sql.NullString`，nil→NULL）。父任务含子任务时，子任务各自的 `InvolvedRoles` 独立写入（child 响应已带该字段，§4.2）。

### 5.2 runMode 派生（`backend/taskManager/model.go`）

引入 `runMode.fetchStores` 标志区分"本次是否拉取资源"，根治**空 storeRoles 语义重载**（既表"仅作品信息"，又表"默认插件首跑下全量"）：

```go
// runModeFromTask：按 StoreRoles 是否 Valid 区分 Start 与 Redownload
func runModeFromTask(t *entity.Task) runMode {
    if !t.StoreRoles.Valid {
        // NULL(Start/首次,含默认插件):拉取资源;storeRoles=universe(空 universe→Start 传空,插件下全量)
        return runMode{workInfo: t.IncludeWorkInfo, storeRoles: parseStoreRoles(t.InvolvedRoles), fetchStores: true}
    }
    // Valid(Redownload):空 selection=仅作品信息(不拉资源),非空=所选子集
    sel := parseStoreRoles(t.StoreRoles)
    return runMode{workInfo: t.IncludeWorkInfo, storeRoles: sel, fetchStores: len(sel) > 0}
}
```

**录制须配合区分 NULL 与 Valid**（`loadAndStartTaskTrees`）：Start(`runModeFull`,storeRoles=nil)→重置 StoreRoles 为 NULL；Redownload(storeRoles 非 nil,可空)→记录 Valid（空=仅作品信息）。靠 recordMode.storeRoles 的 nil(Start) vs 空 slice(Redownload) 区分。

`runSectionCombo` 的资源门控（workDir 检查 / Start 调用 / comboFail 终态 / isNonTerminalMode）改用 `fetchStores`（而非 `hasAnyStore`），使"仅作品信息"跳过 Start、默认插件首跑仍调 Start（空→插件下全量）；角色计数语义的备份/非 main 定位仍用 `hasAnyStore`。

> 早期伪码用 `len(sel)>0` 区分 selection/universe，致"仅作品信息"(Redownload 空 selection)被误派生为 universe→下载资源；且默认插件(空 universe)首跑 storeRoles 空→`hasAnyStore=false`→Start 被跳过→不导入。`fetchStores` + Valid 区分修复二者。

### 5.3 清理 `allStoreRoles`（`model.go`）

`runModeFull = runMode{workInfo: true}`（storeRoles 留空），全集依赖删除；首跑资源集由 `runModeFromTask` 据 universe(InvolvedRoles) 派生。

## 6. 插件契约变更（SDK + 插件）

1. **Create 返回 universe**：插件 `Create` 据任务情况填 `TaskCreateResponse.InvolvedRoles`；能定则定，不能定留空（default）。
2. **Start 语义不变**：`storeRoles` 参数仍是"本次所选子集（空=全量）"，插件用 `wantsRole` 过滤产出；`Start` 实际产出 = truth。
3. **plugin-dev-guide 契约更新**：`doc/plugin-dev-guide.md` 加"创建期声明 involvedRoles"节，说明 hint 性质 + default 兜底 + Start 产出为 truth。

## 7. 前端变更

### 7.1 动态板块集（`frontend/src/constants/sectionCode.ts`）

`ALL_STORE_ROLES`（固定 [main,thumbnail]）降级为"universe 未声明时的兜底展示集"；自选 UI 改为**读行任务 `involvedRoles`**：

- `involvedRoles` 非空 → 展示该集合（pixiv 图片=[main]；视频=[main,videoTrack,audioTrack,thumbnail]）。
- `involvedRoles` 空（default 任务）→ 展示兜底集。

顺带修复当前固定集漏 videoTrack/audioTrack/merged 的限制。

### 7.2 自选 UI（`frontend/src/components/common/TaskOperationBarActiveV1.vue`）

`storeRolesSelected` 勾选项来源从固定 `sectionCode` 改为 `row.involvedRoles`（经 `TaskProgressTreeDTO` 下发）。"全不选=全集"语义保留（= 该任务 universe 全集）。

### 7.3 任务 DTO 透传（`frontend/bindings` + `TaskProgressTreeDTO`）

task 相关 DTO/视图模型加 `involvedRoles`，bindings 重新生成（`wails3 generate bindings -ts`）。

### 7.4 消费侧契约（缩略图回退）

核实瀑布流/卡片组件无"必须 thumbnail store"硬依赖；图片任务（universe=[main]，无 thumbnail）回退用 main 展示。需核查 `WorkCard`/`CardGrid` 的图片源选择逻辑。

## 8. pixiv 即时应用（消 403）

pixiv `Create`（`library-squirrel-plugin-pixiv/task_handler.go`）声明 `InvolvedRoles=[main]`：
- 首跑 Start 请求集 = [main] → 不触发 `fetchThumbnail`（task_handler.go:374 块） → **缩略图 403 消失**。
- 与代理修复（`doc/bug/pixiv规则模式VPN不可访问...` §6）正交——代理修复后若 403 仍在，则是 CDN referer/auth 问题，另立；代理修复前 403 部分由本变更消除（不再请求缩略图）。

> 注：pixiv 当前是否在 Create 阶段就能判定"图片无缩略图"？pixiv 资源恒为图片，Create 拿到 URL 即可定 universe=[main]。无需 CreateWorkInfo。其他站点若需深度元数据才能定，留空走 default。

## 9. Legacy 迁移

- **新字段 `InvolvedRoles`**：老任务 NULL = default，无破坏，无需迁移。
- **`StoreRoles`（selection）**：语义不变，老值（重下残留）仍按"按次选择"读，无影响。
- **前端**：老任务无 `involvedRoles`（bindings 旧版）→ 展示兜底集，无破坏。

## 10. 跨仓库改动清单

| 仓库 | 文件 | 改动 |
|---|---|---|
| 主程序 | `backend/base/model/entity/task.go` | 加 `InvolvedRoles` 字段 |
| 主程序 | `backend/task/service.go` (`handleCreateTaskArray`) | Create 响应 `InvolvedRoles` → task 实体 |
| 主程序 | `backend/taskManager/model.go` | `runModeFromTask` selection∘universe；删 `allStoreRoles` 全集依赖 |
| 主程序 | `backend/plugin/extension/task_handler_proxy.go` | `protoToTaskCreateResponse` 加 `InvolvedRoles` |
| 主程序 | `doc/plugin-dev-guide.md` | 创建期声明 involvedRoles 契约节 |
| SDK | `.proto` + `gen/` | Create 响应 message 加 `involved_roles` |
| SDK | `dto/handler_dto.go` | `TaskCreateResponse`/`TaskCreateChildResponse` 加 `InvolvedRoles` |
| pixiv 插件 | `task_handler.go` `Create` | 声明 `InvolvedRoles=[main]` |
| 前端 | `constants/sectionCode.ts` | 兜底集；动态读 `involvedRoles` |
| 前端 | `components/common/TaskOperationBarActiveV1.vue` | 勾选项来源改 `row.involvedRoles` |
| 前端 | task DTO/bindings | 透传 `involvedRoles` |
| 前端 | `WorkCard`/`CardGrid`（核查） | 无 thumbnail 时回退 main |

## 11. 测试计划

1. **pixiv 图片任务**：Create 声明 [main] → 首跑只下 main、无 thumbnail 请求、无 403；重执行 UI 只显示 [main] 勾选项。
2. **default 任务**（模拟插件 Create 留空 `InvolvedRoles`）：首跑 Start 收到空 storeRoles → 插件下全量；UI 显示兜底集。
3. **视频任务**（未来/A bilibili）：universe=[main,videoTrack,audioTrack,thumbnail] → 重执行 UI 显示 4 项；勾选子集重下仅下所选。
4. **声明=hint**：插件声明 [main] 但 Start 产出 [main,thumbnail] → 主程序挂载两者（不拒绝超出）。
5. **legacy**：老任务（`InvolvedRoles` NULL）行为不变（首跑全量）。
6. **前端回退**：无 thumbnail store 的图片任务，卡片用 main 展示，不报错。

## 12. 不在范围

- F 连接复用（独立任务，opt-in）
- 代理修复（独立任务，`doc/bug/pixiv规则模式VPN不可访问...`）
- generation（downloaded/derived）协议（B 已定，不变）
- bilibili 插件的 universe 声明（随 A bilibili 开发落地，本方案只定协议 + pixiv 首应用）
