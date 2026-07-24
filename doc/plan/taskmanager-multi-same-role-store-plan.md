# taskManager 支持 N-同 role 多 store（补齐多轨重构的同 role 执行路径）

> **派生自**：R「图文紧密结合资源」阶段 1（`.claude/workflow/active/rich-article-resource/TREE.md` 节点 M）。article 设计 = 1 Resource 挂 1 document(.md) + N image(downloaded) + thumbnail；实施前追踪 taskManager 数据流发现其执行层多处假设"1 role 1 store"，N 个同 role(image) store 会让 happy path 路径冲突 + resume 偏移坍缩。
> **性质**：主程序后端（taskManager）+ **数据模型改名**（`order_idx`→`store_seq`）+ **SDK 破坏性改型**（`StreamOffsets` map → structured repeated）+ 三插件 Resume 适配；原子发布。与多轨重构（`doc/plan/主程序多轨资源与多流任务重构方案.md`）同源——那次建了 N-store 数据模型但执行层只铺了"N 个不同 role"，本任务补"N 个同 role"并把 `order_idx` 正名为身份字段。

## 审查摘要（给审查者，≤25 行）

> 审查本文档只需读本段 + 抽查下列证据锚点 + 回答待决策项，无需通读全文。

**关键声明（抽查项）**——"已核验 / 不受影响"类论断全部挂证据：
- **C1. `m.streams` 是切片、全量遍历，N 同 role 对控制面无影响** — 证据：`backend/taskManager/model.go:728/1184/1189/1236/1740/1749/1760/1774` 均为 `for _, s := range m.streams`（Pause/Stop/cleanup/进度全遍历，无 role 查找）
- **C2. 缺陷集中在 4 处 role 键**（`m.streams`=任务活跃流切片、`streamOffsets`=续传偏移表、`findStoreRow/findSpec`=按 role 查行/spec、`resolveStorePath`=落盘路径解析） — 证据：①路径 `resolveStorePath→resolveMainPath` 不含 per-store 区分（`model.go:1932-1938`/`1907-1928`）②`streamOffsets[row.StoreType]` role 键坍缩（`model.go:1496/1511/1513`）③`findStoreRow(role)`/`findSpec(role)` 首匹配歧义（`model.go:1580/1844`/`922/1566`）④resume 用 `streamOffsets[spec.Role]`（`model.go:1583-1584`）
- **C3. backup/recyclebin/search/DTO 无 role 键单 store 查找** — 证据：`grep "findSpec|findStoreRow|StoreType==" backend/{backup,recycleBin,resource,search}/` 零命中（遍历集合）→ 不为"同 role"缺陷改；**也不引用 `order_idx`（全 backend 仅 4 处：entity:33、repo:25/33、model.go:1169），D5 不波及这些模块**
- **C4. storeRows 按序排列，顺序身份可用** — 证据：`backend/resource/resource_store_repository.go:33`（`ListByResourceId` ORDER BY `order_idx`，将改名 `store_seq`）
- **C5. 默认文件名模板非空** — 证据：`backend/settings/model.go:65` `"[${author}]_[${siteWorkId}]_${siteWorkName}"`（模板模式下 N 同 role 同扩展名 → 同路径 → 覆盖，happy path 即坏）
- **C6. SDK `StreamOffsets` 现为 `map[string]int64`（role→offset），D1-B 将改为 structured repeated** — 证据：`library-squirrel-sdk/dto/handler_dto.go:42`；pixiv(`task_handler.go:524`)/local(`:324`) Resume 现读 `param.StreamOffsets[StoreRoleImage]`，需随改型适配
- **C7. mountResourceStores 按 role 批删 → resume 重建连带删同 role 已完成 store** — 证据：`backend/taskManager/model.go:1151-1152`（`uniqueRoles(mounts)`→`DeleteByResourceIdAndTypes`）+ resume 重建分支 `:1601-1608`（仅未完成 spec 进 mounts）
- **C8. `order_idx` 现语义=显示排序提示，名不副实** — 证据：`backend/base/model/entity/resource_store.go:33`（`OrderIdx`）；M 后它成路径消歧/resume 身份/.md 绑定的稳定标识 → 改名 `store_seq`（D5）

**已决策（2026-07-24 全部拍板）**：
- **D1 = B**。`StreamOffsets map<string,int64>` → `repeated StoreResumeOffset{ role, store_seq, offset }`（structured）。SDK proto 改型 + 重新生成 bindings + 主程序 + pixiv/local/bilibili 三插件 Resume 同步改 + 原子发布（S9）。理由：不用 role#idx 字符串约定 hack，显式字段类型安全。
- **D2 = A**。`mainSpec` 统一改用 `task.ResourceType` 的 PrimaryRoles 取首个命中 spec（article→document、image→image、video→首个 PrimaryRole）。
- **D3 = B**。同 role 多 store 文件名 = 模板推导 + `store_seq` 后缀（如 `[作者]_[作品id]_001.png`），**不复用 `SuggestName`**（注：StoreSpec.SuggestName 现役、pixiv 在用 `task_handler.go:373`；Resource.SuggestName 是 1:1 遗留列——article 两者都不沾）。`.md` 图引用走**位置绑定**（第 N 个 `![]()` → 第 N 个 image store，按 store_seq），与落盘文件名解耦——绑定在 R 阶段2 前端渲染器实现，M 仅提供 store_seq 顺序稳定性。
- **D4 作废**。D1-B 的 structured 类型有显式 `store_seq` 字段，单例/多例统一（单例 store_seq=0），无 key 形态决策。
- **D5**。`order_idx` → `store_seq`（entity `StoreSeq` / 列 `store_seq`）。M 内做：DB 迁移 + entity/repository/DTO/backup/recycleBin/taskManager 消费点同步。理由：M 把它从"排序提示"提升为"store 稳定身份"，名须反映职责（FIELD_RENAME_GUARD_AUDIT）。
- **D6 = in-place proto 改型**。`StreamOffsets` field 号复用、类型从 `map` 换 `repeated`（不新增 v2 字段）。破坏 wire 兼容，但 dev 期 + S9 原子发布、无灰度需求；不可逆（升即四端同步）。
- **D7 = 显式 RENAME 迁移**。migrate.go 写 `ALTER TABLE resource_store RENAME COLUMN order_idx TO store_seq;`（GORM AutoMigrate 不 rename，否则留孤儿列，见 §2.6）。

**自曝风险（作者没把握 / 可能错的地方）**：
- **R1. mainSpec 改 PrimaryRoles 可能扰动现有 video/pixiv/local 落盘路径** — 现状 `findSpec(specs, image)` 对 video 返回 nil 走 `specs[0]`（`model.go:922-927`）、对 pixiv/local 命中 image；改 PrimaryRoles 后语义更正但路径可能变 → Phase 1 须对比前后一致，不一致按"不兼容历史、用户上线前手动"处理（同 K 的 S9 原则）。
- **R3. 隐性 role 假设** — 仅审计 taskManager model.go 主路径；`manager.go`（resumeTaskTrees/buildOrReuseChild）、`task_executor.go`、进度推送可能有隐性 role 假设，实施时若发现新缺陷点须现记。
- **R4. 改名 + SDK 改型的原子发布** — `order_idx`→`store_seq` 与 SDK `StreamOffsets` 改型都是破坏性，主程序 + SDK + pixiv/local/bilibili 四端须同步升级；开发期无用户数据，DB 迁移可 drop+重建，但四端编译/联调一次过的风险存在。
- **R5. store_seq 顺序稳定性** — `.md` 位置绑定（R 阶段2）依赖 image store 的 store_seq 顺序 = parseOpusState 的 InlineImages 文档顺序。Start 与 Resume 产 StoreSpec 顺序须确定一致（article 的 InlineImages 稳定，满足）；taskManager 须按 spec 顺序赋 store_seq（已如此，`model.go:1169`）。

---

## 一、问题与范围

### 1.1 现象
article（1 Resource 挂 N 个同 role=image 的图 store）在当前 taskManager 下：
- **happy path 坏**：N 张图同模板文件名 → 磁盘互相覆盖（C5 + C2①）。
- **resume 坏**：偏移按 role 坍缩、按 role 匹配行歧义、批删连带删已完成同 role store（C2②③④ + C7）。

### 1.2 为什么多轨重构没覆盖（根因）
多轨重构把**数据模型**泛化到 N store（`resource_store` + `order_idx`），但**执行层**只实现"N 个不同 role"——SDK `StreamOffsets map<string,int64> // role→偏移`（plan:126/137）结构上装不下同 role 多偏移；streamController "按 role 索引"（plan:176）；`findSpec/findStoreRow/streamOffsets` 全 role 键。video 能跑因 role 不同 + 扩展名不同（.mp4/.m4a）。`order_idx` 仅"写入 + 列表排序"用上，未进路径/续传/匹配。同 role 多 store 用例（article）当时不存在 → 执行路径从未铺。

### 1.3 不在范围（已核验无需为"同 role"改）
- `m.streams` 控制面（C1：切片全遍历）。
- backup/recyclebin/search/DTO 的**遍历逻辑**（C3：无 role 键单 store 查找，也不引用 order_idx——D5 不波及）。
- 数据模型**结构**（`resource_store` 表 + `order_idx`/`store_seq` 列已支持 N 同 role；M 只改名，不加列）。

## 二、设计（五缺陷修法 + SDK 改型 + 改名）

### 2.1 缺陷①：路径消歧（resolveStorePath/resolveMainPath，D3-B）
调用方（startDownload:938、resume:1578 的 specs 循环）按顺序遍历，传入**同 role 计数**（= 该 spec 在同 role 内的 `store_seq`）。当某 role 出现 >1 次时消歧：
- fileName = `{模板推导 base}_{zeroPad(store_seq)}{ext}`（如 `[作者]_[作品id]_001.png`）。
- 单例 role（video 的 videoTrack/audioTrack、pixiv main）不加后缀，保持现有命名（回归零扰动）。
- **不复用 `SuggestName`**（D3-B）；article 的 `.md` 图绑定走位置（见下）。

**mainSpec 选取（D2-A）**：`findSpec(specs, StoreTypeImage)` 改为按 `task.ResourceType` 的 PrimaryRoles 取首个命中 spec。`mainRelPath` 由主 spec 推导，其余 store 按 2.1 消歧。PrimaryRoles 为空（ResourceType=unknown）时兜底 `specs[0]`（同现状 video 的 nil 分支）。

**.md 位置绑定（R 阶段2 前端，M 仅赋能）**：渲染器维护计数器，第 k 个 `![]()` → 第 k 个 image store（按 `store_seq` 升序）的 filePath → `buildStoreUrl`。`.md` 的 `001.png` 退化为 alt/符号，与落盘文件名解耦。M 须保证 image store 的 `store_seq` = Start 产 StoreSpec 顺序 = InlineImages 文档顺序（确定）。

### 2.2 缺陷②：resume 偏移按 store 身份（D1-B structured）
SDK `StreamOffsets` 改为 `repeated StoreResumeOffset{ role, store_seq, offset }`。主程序 `resumeFromPersistedState`（`model.go:1486-1517`）构建时按 **(role, store_seq)** 而非 role 收集：每个未完成 downloaded store 产一条 `{role, store_seq, offset}`；完成态按 (role, store_seq) 记 `completedSet`。N 同 role 的偏移/完成态各自独立。

### 2.3 缺陷③：findStoreRow/findSpec 按 store 身份匹配
`findStoreRow(rows, role)`（`model.go:1844`）→ `findStoreRowByIdentity(rows, role, storeSeq)`：同 role 内按 store_seq 精确匹配。resume 循环遍历 specs 时，spec 携带其 store_seq（= specs 切片内同 role 下标），据此找对应行。`findSpec`（mainSpec）改 PrimaryRoles（2.1）。

### 2.4 缺陷④：SDK 改型 + 插件 Resume（D1-B）
- **SDK proto**：`TaskResumeParam.StreamOffsets` 从 `map<string,int64>` 改为 `repeated StoreResumeOffset{ string role; int32 store_seq; int64 offset; }`；bump 版本；`wails3 generate bindings -ts`。
- **主程序**：按 (role, store_seq) 填/读 structured 列表（替代 map）。
- **pixiv/local Resume**（`task_handler.go:524`/`:324`）：`param.StreamOffsets[StoreRoleImage]` → 遍历列表找 `{role=image, store_seq=0}`（单例，行为等价）。
- **bilibili 现存 resumeVideo/resumeImage**（`download.go:219` `for role,offset := range param.StreamOffsets`、`:268` `param.StreamOffsets[StoreRoleImage]`）：**D1-B 编译断点**，须改——遍历 structured 列表按 role（单例 store_seq=0）匹配取 offset。属 M Phase 3 范围，不等 R 阶段1。
- **bilibili `resumeArticle`**（R 阶段1 1c）：遍历列表，`store_seq` → `ArticleImageURLs[seq]`，产带 `ResumeWriteOffset` 的 StoreSpec。

### 2.5 缺陷⑤：mountResourceStores 批删改全量重挂（⑤-B）
resume 时不再"仅挂未完成 spec"，改为**重挂全量 store**：已完成 store 用原 storeId 重新 mount（不重下），未完成按 2.2/2.3 续传/重建。这样 `mountResourceStores` 的 delete-then-insert（按 role 批删批插，`model.go:1151-1173`）保留全量、不丢已完成同 role 关联。倾向此法（不动 `DeleteByResourceIdAndTypes` 的 role 语义、resume 侧补全量更局部）。

### 2.6 D5 改名：`order_idx` → `store_seq`
- **entity** `ResourceStore.OrderIdx`→`StoreSeq`（`resource_store.go:33`）；GORM tag `column:store_seq`。
- **repository** `ORDER BY order_idx`→`store_seq`（`resource_store_repository.go:33`）。
- **taskManager** mount 赋值（`model.go:1169` `s.OrderIdx = i`→`s.StoreSeq = i`）。
- backup/recycleBin/search/DTO **不引用 OrderIdx**（C3 已核验），无需改。
- **DB 迁移（D7，显式 RENAME）**：GORM AutoMigrate **不会 rename column**——仅改 entity 会新增 `store_seq` 但留 `order_idx` 孤儿列、旧值不迁移。故在 `backend/migration/migrate.go` 显式写 `ALTER TABLE resource_store RENAME COLUMN order_idx TO store_seq;`（SQLite 3.25+），列名正改、值保留、无孤儿。
- **前端 bindings**：重新生成；前端若消费 `orderIdx` 同步改名（抽查 `frontend/`）。

## 三、实施阶段

**Phase 0 · 数据模型改名 + SDK 契约（D5 + D1-B 基建）**
- `order_idx`→`store_seq`：entity/repository/DTO/backup/recycleBin/taskManager + DB 迁移 + 前端 bindings 重生成。
- SDK proto：`StreamOffsets`→`repeated StoreResumeOffset{role, store_seq, offset}`；bump；bindings。
- 确认 `entity` 暴露按 ResourceType 取 PrimaryRoles（供 mainSpec）；定位 task.ResourceType 在 startDownload/resume 上下文可达。
- 审计 `manager.go`/`task_executor.go`/进度推送有无隐性 role 假设（R3）。

**Phase 1 · 路径消歧（缺陷①，解锁 happy path）**
- `resolveStorePath`/`resolveMainPath`：同 role 多 store 加 `store_seq` 后缀（D3-B）。
- mainSpec 改 PrimaryRoles（D2-A）；单测对比 video/pixiv 路径前后（R1）。
- 单测：N 同 role spec → N 个不同路径。

**Phase 2 · resume 身份化（缺陷②③⑤，解锁 resume）**
- streamOffsets/completedRoles 按 (role, store_seq)（缺陷②，consumes structured SDK type）。
- findStoreRow→按 (role, store_seq)（缺陷③）。
- mountResourceStores resume 改全量重挂（缺陷⑤-B）。
- 单测：N 同 role 部分完成 → 各自偏移正确、已完成关联不丢。

**Phase 3 · 插件 Resume 适配（缺陷④）**
- pixiv/local Resume：读 structured 列表找单例（store_seq=0）。
- bilibili 现存 resumeVideo/resumeImage（`download.go:219/268`）：改遍历 structured 列表按 role 匹配（D1-B 编译断点修复）。
- bilibili `resumeArticle`（R 阶段1 1c 一并）：store_seq→ArticleImageURLs、产 ResumeWriteOffset。

**Phase 4 · 集成验证 + 回归**（见 §四）

## 四、验证

- **article N 图（cv48146678 = 29 图）**：fresh 下载产 29 个不同文件（路径消歧）；暂停→恢复每图续各自偏移；跨重启续传 29 图偏移不坍缩、已完成图关联不丢。
- **video（不同 role）回归**：videoTrack+audioTrack+thumbnail 下载/暂停/恢复/重启续传行为不变。
- **pixiv/local 回归**：main+thumbnail 路径与续传不变（mainSpec 改 PrimaryRoles 后路径一致——R1 核验点；Resume 读 structured 列表行为等价）。
- **结构校验**：article 资源 `ValidateResourceStructure` 通过（document 1 + image N + thumbnail 0~1）。
- **改名回归**：`order_idx`→`store_seq` 后 backup/还原、recycleBin 往返、search 结果一致。

## 五、风险（守住勿回退）

- **R1（mainSpec 扰动）**：Phase 1 须对比 video/pixiv 路径前后一致；若不一致，按"不兼容历史"处理并记入决策。
- **R3（隐性 role 假设）**：Phase 0 审计 manager.go/task_executor.go/进度推送，发现即记。
- **R4（原子发布）**：改名 + SDK 改型破坏性，四端同步升级；DB 迁移 dev期 drop+重建可接受。
- **R5（store_seq 顺序稳定）**：taskManager 须按 spec 顺序赋 store_seq（已如此），插件 Resume 产 spec 顺序须与 Start 一致（article InlineImages 稳定，满足）。
- **不回退数据模型结构**：M 只改名 + SDK 改型，不加表/列（除改名）、不迁历史数据。
- **与 R 阶段1 的接合**：M Phase 3 的 bilibili `resumeArticle` 与 R 任务 1c（`TaskPluginData + startImage/resumeImage article 多 store`）是同一份代码；M 完成前 R 阶段1（1b/1c）阻塞。
