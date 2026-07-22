# 资源类型规约体系实施计划

> **任务定位**：task-graph 谱系 `rich-article-resource` 当前焦点节点（代号 K），派生并阻塞图文文档（代号 R）。
> **架构依据**：`doc/plan/resource-display-kind-design.md`（经 plan-reviewer 审查，事实声明 F1-F9 全核验属实）。
> **本计划职责**：把 design 的架构决策（S1-S8）+ 待定细节细化为**可执行步骤**。
> **状态**：实施计划·待执行。2026-07-21 决策序列：①去主程序推断②命名 kind→ResourceType③store_type 重构（去 main）+ 基数化 Roles + PrimaryRoles；**审查后修订：声明3 降级、严格识别（空/未知值抛错）、不兼容历史数据（用户手动）、原子发布**。

## 审查摘要（给审查者，≤40 行）

> 架构层事实声明见 design.md（已审查）。本段只列**实施级**声明、设计约定、待决策。

**实施级关键声明（抽查项，挂锚点核验现有代码）**：
- **声明1. proto 加可选字段天然向后兼容** — `library-squirrel-sdk/proto/plugin.proto:117,126`，加 optional `resource_type` 不破坏旧插件
- **声明2. Resource 加列走 GORM AutoMigrate** — `backend/migration/migrate.go`（已注册 Resource），加 `ResourceType` 列自动 ALTER
- **声明3. 前端消费链当前完全缺失** — `frontend/src` 无 `resourceComplete` 引用
- **声明4. resource_store 无 (resource_id,store_type) 唯一约束** — `backend/base/model/entity/resource_store.go:29`（仅 `column:store_type`，无 uniqueIndex）
- **声明5. merged 历史"累积多行"现实瑕疵** — `backend/resource/merge_service.go:92-94`（注释"历史已累积的多行 merged 不在此清理"）

**设计约定（待实现，非代码事实）**：
- **约定1. 结构校验纯集合运算** — `ValidateResourceStructure`（待实现）设计为仅判每角色数量 ∈ [Min,Max]，无 IO。属设计目标，非已核验代码。

**待决策（plan 级，已定）**：
- 决策1 资源类型声明（**严格：插件必须声明有效值，未声明/空→抛错**；unknown 是合法显式值）
- 决策2 `resource.resource_type` 索引（暂不加）
- 决策3 历史数据处理（**不兼容、不迁移，用户手动**）
- 决策4 `ResourceComplete` 三态语义（0=未校验）
- 决策5 ResourceType 与 involved_roles 冲突（软不变量）
- 决策6 多图资源类型一致性（resource 级独立）
- 决策7 unknown 渲染降级（回退嗅探兜底）
- 决策8 store_type 体系重构（去 main，image/document 替代，6 角色）
- 决策9 Roles 基数化（`{StoreType,Min,Max}`）
- 决策10 展示主体（PrimaryRoles 优先级链 + WorkStore 后端派生 + 前端纯消费）
- 决策11 merged 收敛（**仅新数据 Max=1 守卫；历史多行不修复，用户手动**）
- 决策12 严格识别 + 原子发布（空/未知 store_type 与 resource_type 一律抛错；Phase 0+3+5 必须同步发布）

**自曝风险**：
- **风险1. 完成判断逻辑定位** — ResourceComplete 死字段（design F5），Phase 2.1 须定位/新建"全部 store 落盘"钩子。
- **风险2. 原子发布协调** — 决策12 下，**主程序（Phase 0 去 main + Phase 1 严格校验）+ SDK（Phase 3）+ 插件（Phase 5）必须同步上线**——Phase 1.6/1.7 严格校验同样拒绝老插件产出，故原子集合含 Phase 1（不止 0+3+5）。否则老插件全挂。
- **风险3. 结构校验与部分下载协调** — video 缺轨=不完整，须不干扰续传。
- **风险4. bindings 重生成触发** — proto/pb.go 改后须 `wails3 generate bindings -ts`。
- **风险5. 现役插件必须声明** — 决策1 严格，所有现役插件 Create 必须声明有效 resource_type，未声明即抛错；须排查覆盖。
- **风险6. WorkStore 派生变更跨模块影响** — 前端 `getResourceOpenPath` 等消费点须改纯消费（删前端优先级逻辑）。
- **风险7. 完成判断写入点与调度不变量冲突** — Phase 2.1 改任务状态机，须避开 actor 调度 CAS 守卫体系，否则破坏调度不变量。
- **风险8. 上线前历史数据手动处理 + NOT NULL 加列触发** — 决策3 不迁移，历史 main store/merged 多行/无 resource_type 的 resource 须用户上线前手动处理。**具体触发**：AutoMigrate 加 `ResourceType` NOT NULL 列时，SQLite 用默认 `''` 填历史行 → 历史行 `resource_type=''` 在决策12 严格抛错下**一上线即炸**（任务加载/展示即抛错）。用户须上线前手动给历史 resource 填合法 resource_type（或重建库）。

---

## 一、前置依赖

- design.md 已审查通过（F1-F9 属实、S1-S8 决策已定）。
- 本计划不重复架构论述，仅细化执行步骤。

## 二、实施步骤（Phase 化）

### Phase 0 · store_type 体系重构（不兼容历史，I9 先行）

| 步骤 | 改动点 | 验收 |
|---|---|---|
| 0.1 | `backend/base/model/entity/resource_store.go`：删 `StoreTypeMain`，加 `StoreTypeImage="image"`/`StoreTypeDocument="document"` | 编译 |
| 0.2 | `library-squirrel-sdk/dto/handler_dto.go`：`StoreRole*` 常量同步（删 main、加 image/document） | SDK 编译 |
| 0.3 | 前端 `frontend/src/constants/sectionCode.ts`：`StoreRole` 删 MAIN、加 IMAGE/DOCUMENT；`ALL_STORE_ROLES`/`StoreRoleLabels` 同步 | 常量就位 |
| 0.4 | 改所有原 `main` 消费点为 `image`（Phase 0 占位；真正主体解析在 Phase 1.5 `ResolvePrimaryStore` 接管）。实测 **7 文件**（plan 原估 5 处，实施补实 2 处遗漏，详见下方注） | 全链路代码无 main 常量 |
| 0.5 | **删除 `migration/migrate.go` 的 `migrateResourceLegacyColumns` 函数**（连同 `resourceLegacyColumns` 结构 + `migrate.go:71` 调用）——它把旧 `work_store_id` 迁为 StoreTypeMain（`migrate.go:117,122`），删常量后编译失败；决策3 不兼容历史，含旧 `work_store_id` 列的库需重建 | 编译通过、无 legacy 迁移残留 |
| — | **上线前提（风险8）**：用户手动处理历史数据（main store→image/document、merged 多行收敛、历史 resource 填 resource_type、含旧 `work_store_id` 列的库重建），否则历史资源在新体系下抛错 | 用户确认历史已处理 |

> 注：决策3 不兼容历史——plan 不写自动迁移/merged 修复逻辑；历史数据由用户手动处理。

> **Phase 0 实施纪要（2026-07-21，补实 0.4）**：
> - **原估"5 处"实为 7 文件**，遗漏的 2 处均为字符串字面量（不破坏编译，故 plan 的"5 处"指编译失败点无误，但行为残留须一并处理）：
>   - `search/repository.go:236` SQL `store_type = 'main'` → **已改 `'image'`**：搜索结果 `workStore` 子查询的活跃路径，Phase 0 后当前资源已是 `image`，留 `'main'` 会让搜索 `workStore` 全空 → 展示断裂。与 `resource_dto.go` WorkStore 占位保持一致。
>   - `recycleBin/snapshot.go:78` v0 历史快照适配器 `StoreType: "main"` → **留存不改**：`SnapshotStoreBackups` 产出的 `StoreBackupRef.StoreType` 在 restore/purge（`service.go:152,232`）两处循环里**从不被读取**（只读 `BackupID`），是 v0 历史格式死标签；改 `"image"` 会误标历史数据。属决策3 历史范畴，Phase 1 restore 路径若引入 store_type 严格校验时再决（删 v0 适配器 or 映射）。
> - **占位决策（Phase 0+1 不合并）**：遵循约束"Phase 0 暂只动 store_type"，所有 `main` 消费点统一占位为 `image`（`hasStore`/`findSpec`/WorkStore/搜索 SQL），对当前插件集（pixiv/local=image、bilibili video=videoTrack 无 main）行为中性；Phase 1.5 `ResolvePrimaryStore` 正式接管 WorkStore，Phase 1 重新审视 `hasStore`/`findSpec` 的"主资源"语义（应基于 ResourceType 而非硬编码 image 角色）。
> - **pre-existing 测试失败（非 Phase 0 引入，记录在案）**：`taskManager/model_multi_stream_test.go::TestRunModeFromTask` 在 master 上即已失败（`NewTask()` 零值 → `{workInfo:false storeRoles:[]}`，断言期望全量）；本次 `Main→Image` 改名对该测试行为中性（storeRoles 空，hasStore 两种常量皆 false）。Phase 1 改 `runModeFromTask` 语义时顺带修。另 `base/query/converter_test.go` 有 pre-existing 泛型编译错误，与本次无关。

### Phase 1 · 后端规约（Registry + Spec + 校验 + 主体解析）

| 步骤 | 改动点 | 验收 |
|---|---|---|
| 1.1 | `backend/base/model/entity/resource.go`：加 `ResourceType string \`gorm:"column:resource_type;not null" json:"resourceType"\``（**NOT NULL，决策1 不允许空**） | 编译通过 |
| 1.2 | 新建 `backend/base/model/resource_type.go`：`ResourceTypeSpec`（含 `Roles []StoreRoleSpec`/`PrimaryRoles []string`/`StoreStandards`）、`StoreRoleSpec{StoreType,Min,Max}`、`StoreStandard`、`ResourceTypeRegistry`（预定义 image/video/article/document/unknown，按 design §2.4）+ `ResourceType*` 常量 | Registry 单测覆盖 4+1 类型 |
| 1.3 | `resource_type.go`：`ValidateResourceStructure(resourceType, storeTypeCounts) (missing, excess []string)`（判每角色 count∈[Min,Max]，约定1 纯集合运算） | 校验单测 |
| 1.4 | `resource_type.go`：`ResolvePrimaryStore(stores, spec) *ResourceStore`（按 PrimaryRoles 优先级取第一个存在） | Resolve 单测：video 有 merged→merged；无→videoTrack |
| 1.5 | `dto/resource_dto.go`：填充 ResourceType；**`WorkStore` 派生改为 `ResolvePrimaryStore(stores, spec)`** | DTO 单测 |
| 1.6 | `taskManager/model.go`（saveResource）：**严格校验+写入**——`resource_type` 空→抛错（决策1）；非预定义值→抛错（决策12）；合法值写入 | 集成：合法声明→写入；空/未知→抛错 |
| 1.7 | store_type 严格识别：mountResourceStores 校验 `StoreType` ∈ 预定义 6 角色，未知→抛错（决策12） | 未知 store_type 被拒 |

> **Phase 1 实施纪要（2026-07-22，补实 1.2/1.5/1.6/1.7）**：
> - **缝隙1·资源 DTO 字段归属**：`ResourceDTO`/`ResourceFullDTO` 定义在 **SDK**（`dto/resource_dto.go`/`resource_full_dto.go`）。Phase 1.5 填充 ResourceType 的前提是先给这两个 SDK 资源 DTO 加字段——plan Phase 3 只覆盖 proto/handler 契约（`TaskCreateResponse`/`StoreSpec`），**漏了资源 DTO**。资源 DTO 属资源模型，归 Phase 1（本地 replace 立即可用；插件不直接消费，不违反"插件 Phase 5"）。已加 `ResourceType` 字段。
> - **resource_type.go 位置调整**：plan 写 `backend/base/model/resource_type.go`，实际放 `backend/base/model/entity/resource_type.go`——`model` 包已被 `entity` import（BaseEntity），若放 model 反向引用 `entity.StoreType*` 会 **model↔entity 循环**。design I1 只指定文件名未指定包。
> - **缝隙2·严格校验运行时接入推迟**：1.6（saveResource 严格校验 resource_type）/1.7（mountResourceStores 严格校验 store_type）的**运行时接入有硬依赖**——1.6 需 proto `resource_type` 字段（Phase 3）才有值来源；1.7 会在插件仍产 `"main"`（Phase 5 前）时拒绝。按 plan 字面接入 → 开发环境 `task dev` 跑下载任务立即全炸（resource_type 空 + store_type=main 双拒），直到 Phase 7 原子发布——此为 plan 风险8（上线即炸）的 dev 环境对应面，plan 未提。**已实现校验函数 `ValidateResourceType`/`ValidateStoreType` + 单测（entity 包全过），运行时接入 saveResource/mountResourceStores 推迟到 Phase 3+5 就位后作原子发布前最后一步**。
> - **ResolvePrimaryStore 安全降级**：对未知/历史 resource_type（`LookupResourceTypeSpec` 返回 nil）降级取 image store → 首个非 thumbnail store，保证历史资源（Phase 1 后现有 resource 的 `resource_type=''`）展示不回归。读路径专用，不抛错。
> - **search SQL WorkStore 推迟到 Phase 4**：`search/repository.go:236` workStore 子查询暂保持 Phase 0 的 `'image'`（占位），PrimaryRoles 优先级统一（COALESCE merged>image>document>videoTrack）推迟到 Phase 4——Phase 4 本就 owning 所有 WorkStore 消费者（`getResourceOpenPath`/WorkCard），search SQL 归入此批；raw SQL 多行 COALESCE 重构风险较高，Phase 1 不发布不值得冒险。Go DTO（`NewResourceFullDTO`）已正确用 ResolvePrimaryStore。
> - **验收**：`go build ./backend/...` EXIT 0；SDK `go build ./...` EXIT 0；entity 包单测全过。

### Phase 2 · 完整性语义新建（激活死字段 ResourceComplete）

| 步骤 | 改动点 | 验收 |
|---|---|---|
| 2.1 | **定位/新建完成判断钩子**（风险1/7）：候选落点——单子任务 `downloadLoop` 全部 streamController 完成时 / 父任务所有子任务完成聚合时；须避开 actor 调度 CAS 守卫；**须决策**写入时机（单子任务完成即写 vs 父任务聚合写） | 定位 file:line 或新建点；调度不变量未破坏 |
| 2.2 | 完成判断处调 `ValidateResourceStructure` → 写 `ResourceComplete`（三态：0=未校验、1=完整、2=不完整，决策4） | video 完成→1；缺轨→2 |
| 2.3 | 前端新建 ResourceComplete 三态消费链（声明3） | 前端按三态展示 |

> **Phase 2 实施纪要（2026-07-22）**：写入时机（plan 2.1 待决）采纳**单子任务完成即写**——落点 `downloadLoop` 全部完成分支（`model.go:1233`）+ resume 无轨道完成（`:1509`），覆盖新建/恢复两路径；**不在父聚合层写**（父 `ParentTask` 不持有 resource）。**风险7排除**：`setState` 是无守卫 atomic Swap，完成写入在 downloadLoop 尾部，与 `dispatch` 的 `actorStarted` CAS（`manager.go:467`）完全分离，不破坏调度不变量。`markResourceComplete`：查 `ResourceType` + `ResourceStoreReader.ListByResourceId` 计数 → `ValidateResourceStructure` → 三态（0=未校验/未知类型/reader 缺失、1=完整、2=不完整）→ `ResourceUpdater.Update`（值未变跳过，查询异常降级 0 不阻断完成）。**不在 saveResource 内写**（其在下载前调用 + resume 不调）。前端：WorkCard 读 `resourceComplete`，===2 显示"不完整"徽标（0/1 不显示，本环境 image 资源不触发）。backend/frontend build EXIT 0。

### Phase 3 · SDK/proto resource_type 字段（跨仓库，与 Phase 0/5 原子发布）

| 步骤 | 改动点 | 验收 |
|---|---|---|
| 3.1 | `proto/plugin.proto:117,126`：加 `optional string resource_type = N;`（声明1） | proto 编译通过 |
| 3.2 | 重生成 pb.go | pb.go 含 ResourceType |
| 3.3 | `dto/handler_dto.go`：加 `ResourceType string` + `ResourceType*` 常量 | SDK 编译 |
| 3.4 | 主程序 `wails3 generate bindings -ts`（风险4） | bindings 含 resourceType |

> **Phase 3 实施纪要（2026-07-22，补实 3.1-3.4 + 链路缝隙）**：
> - **缝隙3·Task 实体作 resource_type 载体**：plan Phase 3 只列 proto/pb/handler_dto/bindings，但 resource_type 从 TaskCreateResponse 到 saveResource 需要载体——`entity.Task` 加 `ResourceType sql.NullString`（创建期声明、执行期读），plan 未提。已加（AutoMigrate 自动 ALTER）。
> - **完整传输链（plan 字面 4 步实际需 ~10 处）**：以 `InvolvedRoles` 为模板打通 resource_type 全链：①proto（Child `resource_type=7` / Parent `resource_type=9`）②pb.go 重生成（protoc v5.29.5 / protoc-gen-go v1.36.1 与 pb.go 头部版本匹配）③SDK `handler_dto.go` TaskCreateResponse/Child 加字段 + `ResourceType*` 常量 ④SDK `task_dto.go` TaskDTO 加字段 ⑤SDK `transport/plugin_server.go` dto→pb ⑥主程序 `task_handler_proxy.go` pb→dto ⑦`entity/task.go` 加 ResourceType ⑧`task_dto.go` 双向转换 ⑨`task/service.go` assignTask 存储 + 3 处构造传参 + 重跑父/子存储 ⑩`saveResource` 新建 resource 时 `resource.ResourceType = m.task.ResourceType.String` ⑪bindings 重生成。
> - **saveResource 仅写入值，严格校验仍待 Phase 5 后（缝隙2 续）**：Phase 3 打通"值能传到 resource"，但 `ValidateResourceType`/`ValidateStoreType` 运行时接入仍推迟——插件 Phase 5 前不声明 resource_type，`task.ResourceType=NULL`→`resource.ResourceType=""`（空串写入，不抛错）；Phase 5 插件声明后值自动流转，届时接入 1.6/1.7 严格校验。
> - **替换场景未改**：saveResource 替换分支（existing resource）不动 ResourceType（保留原值）；新建分支覆盖主路径。
> - **验收**：backend `go build` EXIT 0；SDK `go build` EXIT 0；pb.go 含 ResourceType（field+Get+descriptor）；bindings 含 resourceType（ResourceDTO/ResourceFullDTO/TaskDTO 三处）；frontend `yarn build` EXIT 0。
> - **LSP 噪音**：编辑期 cwd 切换致 gopls workspace 噪音（BrokenImport/UndefinedName），go build EXIT 0 证伪，非真实错误。

### Phase 4 · 前端渲染分发 + 板块过滤

| 步骤 | 改动点 | 验收 |
|---|---|---|
| 4.1 | `sectionCode.ts`：加 `ResourceType` 常量集 | 常量就位 |
| 4.2 | `ResourceUtil.ts`：抽 `getResourcePreviewType`；**`getResourceOpenPath` 改纯消费 WorkStore（删前端 merged>main 优先级逻辑，风险6）** | 单测；外部打开用 WorkStore |
| 4.3 | `WorkCard/WorkSetCard/WorkDialog`：按 getResourcePreviewType 分发；外部打开按类型选 OpenImage/视频/Open（**顺带修 appLauncherOpenImage 写死 + WorkSetCard 不走 getResourceOpenPath**） | image/video/article 分发正确 |
| 4.4 | `TaskOperationBarActiveV1`：板块勾选按 `ResourceTypeRegistry[type].Roles` 过滤 | 板块仅展示该类型 Roles |
| 4.5 | markdown 渲染器引入（图文文档消费者，留 type=article 分发位） | type=article 走 md 分支 |

> **Phase 4 实施纪要（2026-07-22）**：**范围调整**——本环境仅 image 资源（pixiv/local），做**分发结构**（ResourceType 判断 + 各类型分支），image 实际渲染，video/article/document 预留分支（外部打开走系统默认 `appLauncherOpen`）；**markdown 渲染依赖（4.5）推迟**到 bilibili 谱系（本环境无 article 资源，引入未用库无意义）。**search SQL workStore COALESCE 推迟**：`search/repository.go` 仅补 `select resource_type`（前端 `getResourcePreviewType` 读 `resource.resourceType` 必需），workStore 仍 `'image'`（本环境 image 资源列表页正确），COALESCE 优先级链待 bilibili video 上线统一。4.1 ResourceType 常量；4.2 ResourceUtil 新建 `getResourcePreviewType`（未知/空降级扩展名嗅探）+ `getResourceOpenPath` 改纯消费 workStore（删前端 merged 优先级，后端 ResolvePrimaryStore 已做）+ 常量去重（用 StoreRole）；4.3 三组件删各自 IMAGE_EXTENSIONS 嗅探，imagePath/coverFilePath 改 `getResourcePreviewType===IMAGE`（非 image 返回空走 error/占位）；4.4 外部打开 image→`appLauncherOpenImage` / 其他→`appLauncherOpen` + WorkSetCard 统一 getResourceOpenPath（原绕过）。**板块勾选（TaskOperationBarActiveV1）不改**：已是 StoreRole 模型，与 ResourceType 分发正交。backend/frontend build EXIT 0。
> - **实施后修复（2026-07-22，实测打开 bilibili video 触发 `invalid external app`）**：① `appLauncherOpen` pre-existing bug——调 `Open(ExternalAppEnum.$zero=0)` 但后端 enum 仅 1/2/3，0→`ErrInvalidApp`；改调 `OpenPath`（系统默认打开，后端已有+binding 已暴露）。② `ResolvePrimaryStore` 降级加 merged 优先——历史 video（`resource_type=''`）降级原取 videoTrack（无音频），改 merged 优先打开完整视频。根因链：历史 video resource_type='' → 降级嗅探 workStore(.mp4)→VIDEO → 非 IMAGE 分支 appLauncherOpen → Open(0) 报错。
> - **实施后修复二（2026-07-22，实测打开 video "找不到文件"+"获取路径失败"）**：① **search SQL workStore COALESCE 不再推迟**（原 Phase 4 推迟到 bilibili 上线）——库中已有历史 bilibili video 数据立即触发：workStore 子查询 `store_type='image'` 对 video（无 image store）返回 null → 主页面打不开。改 `COALESCE(merged>image>document>videoTrack)`，与 Go `ResolvePrimaryStore` 降级一致。**教训：历史数据可能即时存在，"无 video 资源所以推迟"判断错误**。② **OpenPath pre-existing bug**：不 join workDir，相对路径直接 `cmd /c start` → Windows 在 cwd 找不到。改 OpenPath 统一 join workDir，OpenImage 委托（传相对路径，避免 double join）。两个修复后：主页面/WorkDialog 的 video 经 COALESCE→merged + OpenPath(join)→ 系统播放器打开完整视频。

### Phase 5 · 插件声明资源类型 + 新 store 角色（决策1/12 下强制，与 Phase 0/3 原子发布）

| 步骤 | 改动点 | 验收 |
|---|---|---|
| 5.1 | `library-squirrel-plugin-bilibili`：video→`video`、dynamic→`image`、article→`article`（正文 .md 用 document 角色、内嵌图用 image 角色）；**必须声明 resource_type** | 各 Create 声明正确 |
| 5.2 | `library-squirrel-plugin-pixiv`：`image`（必须声明） | 声明正确 |
| 5.3 | `library-squirrel-plugin-local`：`image`（必须声明） | 声明正确 |
| 5.4 | 排查现役插件均声明有效 resource_type（风险5） | 全覆盖 |

> **Phase 5 实施纪要（2026-07-22，pixiv+local；bilibili 在 multitrack 谱系）**：
> - **环境范围**：pixiv + local 在本环境；bilibili 在 multitrack-resource-lineage 谱系。**bilibili store 角色已适配**（2026-07-22 因打包需求修复：`StoreRoleMain`→`StoreRoleImage`，8 处全为图片场景——`startImage`/`resumeImage` + dynamic/article 图片 Create；video 多轨 `videoTrack`/`audioTrack` 不受影响，`go build` EXIT 0）。**bilibili ResourceType 声明仍待 Phase 5.1**（图片 Create→image、video Create→video、article→article），Phase 7 严格校验前置。
> - **pixiv（5.2）**：parent + child TaskCreateResponse 加 `ResourceType: sdkdto.ResourceTypeImage`；`sdkdto.StoreRoleMain`→`StoreRoleImage` 全替换（6 处：InvolvedRoles×2、wantsRole、StreamOffsets、StoreSpec.Role×2）。pixiv `go build` EXIT 0。
> - **local（5.3）**：child + parent×2 TaskCreateResponse 加 `ResourceType: sdkdto.ResourceTypeImage`（local 不声明 InvolvedRoles）；`sdkdto.StoreRoleMain`→`StoreRoleImage` 全替换（4 处：wantsRole、StreamOffsets、StoreSpec.Role×2）。local `go build` EXIT 0。
> - **资源类型语义**：pixiv/local 均图片资源→`image`（store 角色 image 主体 + thumbnail 缩略图），符合 ResourceTypeRegistry.image 规约。
> - **Phase 0 遗留修复**：Phase 0 删 SDK `StoreRoleMain` 后插件引用即编译失败，Phase 5 修复（main→image）+ 声明 resource_type 一并完成。
> - **现役插件全覆盖（5.4/风险5）**：本环境 pixiv+local 已声明；bilibili 待其谱系处理。原子发布（Phase 7）前须确认 bilibili 也声明。
> - **至此 resource_type 全链贯通**：插件声明 image → proto → Task → saveResource 写入 resource.ResourceType → DTO/前端。1.6/1.7 运行时严格校验接入的前置（值来源 + store 角色合规）已就绪——本环境 pixiv/local 合规，可接入不炸；正式接入在 Phase 7 原子发布（待 bilibili 声明后同步上线）。

### Phase 6 · 文档

| 步骤 | 改动点 | 验收 |
|---|---|---|
| 6.1 | 新建 `doc/resource-type-spec.md`：每类型 Roles/PrimaryRoles/文件标准 + Create 须声明 ResourceType + 新 store 角色 + **严格识别（空/未知抛错）**说明 | 文档完整 |
| 6.2 | `doc/plugin-dev-guide.md` 补 StoreSpec/TaskCreateResponse 声明 ResourceType + 新 store 角色 + 严格识别契约 | 指引就位 |

> **Phase 6 实施纪要（2026-07-22）**：
> - **6.1 resource-type-spec.md**（新建）：三层结构、store_type 6 角色、5 预定义类型（Roles 基数/PrimaryRoles/文件标准，源自 `resource_type.go` 实际 Registry）、声明契约（必须声明 + 严格识别）、完整性三态、消费者、扩展待办 T3。权威实现指向 `resource_type.go`，文档为其人类可读说明。
> - **6.2 plugin-dev-guide.md**（补充）：新增「声明资源类型(ResourceType,必填)」小节（TaskCreateResponse/Child 必填 + 严格识别 + 引用 resource-type-spec.md）；StoreSpec.Role 注释 main→六角色；InvolvedRoles 例子 main→image；Format 前导点约定 downloaded 轨 main→image。

### Phase 7 · 原子发布（风险2）

| 步骤 | 改动点 | 验收 |
|---|---|---|
| 7.1 | SDK 发版（tag） | 版本号更新 |
| 7.2 | pixiv/local 插件 go.mod 升级 SDK + 重测 | 插件编译通过 |
| 7.3 | **主程序 + SDK + 所有插件同步上线**（决策12 原子发布，否则老插件被抛错拒绝） | 全链路通、无兼容窗口 |

### Phase 8 · 验证

- 现有 video/image 任务不回归（展示、外部打开、合并、板块勾选）。
- 严格识别：空/未知 resource_type 与 store_type 被抛错拒绝（决策12）。
- 结构校验与部分下载协调（风险3）。
- 历史数据完整性展示（决策4）：上线前用户已手动处理历史（风险8）。
- 现役插件均声明有效 resource_type（风险5）。
- 原子发布：主程序+SDK+插件同步上线，无老插件残留（风险2）。

## 三、待决策详述

- **决策1 资源类型声明（严格）**：插件**必须声明**有效 resource_type（5 个预定义之一）；未声明/空→抛错；声明未知值→抛错（决策12）。unknown 是合法显式值（插件确实无法分类时声明）。**不兜底、不推断**。
- **决策2 索引**（暂不加）：数据量级千-万，resource_type 在内存 DTO 分发、不查 DB；待数据量增长再评估。
- **决策3 历史数据处理（不兼容）**：不为历史数据做迁移/回填。历史 main store / merged 多行 / 无 resource_type 的 resource 由用户上线前手动处理（转 image/document、收敛 merged、填 resource_type 或接受异常）。
- **决策4 `ResourceComplete` 三态**：0=未校验（历史/未激活，前端"未知"不阻断，化解 design 风险5）、1=完整（正常展示）、2=不完整（前端**徽标提示但不阻断打开**——用户仍可访问缺损资源，仅提示结构不齐）。
- **决策5 ResourceType 与 involved_roles 冲突**（软不变量）：文档立约"Roles 角色 ⊆ involved_roles"，不强制校验，以实际产出为准。
- **决策6 多图类型一致性**（resource 级独立）：每子 resource 独立声明，无 work 级约束。
- **决策7 unknown 渲染降级**（回退嗅探）：前端 resourceType=unknown 走扩展名嗅探兜底。
- **决策8 store_type 重构（去 main）**：main 语义模糊，用 image/document 替代。6 角色。
- **决策9 Roles 基数化**：`{StoreType,Min,Max}` 取代集合语义。
- **决策10 展示主体**：PrimaryRoles 优先级链 + WorkStore 后端派生 + 前端纯消费。
- **决策11 merged 收敛**：仅新数据 Max=1 守卫（merge_service 已有幂等守卫，声明5）；**历史多行不修复**（决策3，用户手动）。
- **决策12 严格识别 + 原子发布**：store_type 与 resource_type 一律严格识别——空/未知值抛错（不兜底、不容忍）；主程序 + SDK + 所有插件必须原子发布（无兼容窗口，否则老插件挂）；T3（插件自定义类型）通过注册进 Registry 纳入可识别范围。

## 四、整体验收标准

1. `Resource.ResourceType` NOT NULL、新任务合法声明（未声明/未知→抛错）。
2. store_type 体系：无 main 残留，image/document 就位，未知 store_type 抛错。
3. `ResourceTypeRegistry` 4+1 类型规约完整（Roles 基数 + PrimaryRoles）、Validate/Resolve 可用、WorkStore 按 PrimaryRoles 派生。
4. ResourceComplete 三态激活；完成判断不破坏调度不变量。
5. 前端按资源类型分发 + 外部打开按类型 + 板块按 Roles + getResourceOpenPath 纯消费 WorkStore。
6. 现有 video/image 不回归。
7. **原子发布**：主程序 + SDK + 插件同步上线，无兼容窗口。
8. 上线前用户已手动处理历史数据。
9. `doc/resource-type-spec.md` 就位。

## 五、回滚策略

- **Phase 0（store_type 重构）**：不迁移历史（决策3），回滚=恢复 main 常量 + 插件回退；无数据迁移可逆问题（因没迁移）。上线前历史手动处理，回滚后历史仍是旧态。
- Phase 1（ResourceType NOT NULL 列）：加列回滚=删列（无历史数据依赖，因不迁移）。
- Phase 2（ResourceComplete）：回滚=停止写非零值。
- Phase 3/7（SDK/proto）：optional 字段向后兼容。
- **整体**：因不兼容历史（决策3），各 Phase 无数据迁移耦合，可独立回滚；唯一硬约束是 Phase 0+3+5 原子发布（决策12），回滚也须原子（主程序+SDK+插件同步回退）。
