# VERIFY 主数据删除关联清理缺口修复（阶段7 实机验证）

> 方案：`doc/plan/主数据删除关联清理缺口修复方案.md`（八入口编排化/守卫/退役）
> 形态：dev 实机（vite 9245 + 测试二进制 CGO `bin/library-squirrel-test.exe` + CDP 9222），真实前端链路（CDP 调 wrapper / 真实 UI 按钮点击 = 用户操作同链路），只读 SQL/日志/DOM 断言。
> 数据红线：删除类验证全部在本会话自建数据（`delv` 前缀 / `delv-*` 键）上闭环；site 守卫拒绝路径对既有站点（pixiv/bilibili）天然只读安全（删除被拒不动数据）。
> **补验轮（2026-08-25 下午）**：pid=0 缺口修复（`Valid: pid != 0`）后任务链路恢复，V9/V10/V11 三项实机补验全过（12/12）；新发现 bug 2（替换流挂起）与基建根因修正见各专节。

## 基建与基线

| 项 | 值 |
|---|---|
| 测试二进制 | `bin/library-squirrel-test.exe`（2026-08-25 构建，含阶段1-6 全部改动） |
| vite / CDP | `http://localhost:9245` / `127.0.0.1:9222`（`json/version` + `cdp-eval` 双探活通过） |
| 日志基线 | `log/server.log` 行 4933（首次启动段）/ 5093（重启段） |
| 库基线 | site 3（pixiv=1/local=2/bilibili=3）、活作品 93（软删 0）、task 260、回收站软删 store 3（用户既有） |
| 站点管理页路由 | `#/siteManage`；操作按钮收在行内 el-dropdown |

## 执行记录与断言证据

### V1 site.Delete 全空删除（缺口5 正向）—— PASS

- 链路：`siteSave(SiteDTO{siteName:"delv-empty-site"})` → id=4 → `siteDeleteById(4)`，均前端 wrapper。
- 断言：创建后站点列表 4 项 → 删除成功 → 列表回 3 项（pixiv/local/bilibili）；SQL `SELECT COUNT(*) FROM site WHERE id=4` = **0**。
- 全程无 FK 报错。

### V2 site.Delete 守卫拒绝·活行引用（缺口5）—— PASS（wrapper 链 + 真实 UI 按钮链双验）

- wrapper 链：`siteDeleteById(1=pixiv)` → 拒绝，消息原文：
  > 无法删除站点：作品 14、任务 43、作品集 3、站点标签 64、站点作者 10。请先在作品页删除相关作品、在任务列表删除相关任务、在作品集页删除相关作品集、在站点标签页删除相关标签、在站点作者页删除相关作者，再重试删除
- SQL 核对五类计数：14/43/3/64/10 **全部一致**；site 行保留（COUNT=1）；无「FOREIGN KEY constraint failed」外露。
- **UI 真实按钮链**（站点管理页 → 行 el-dropdown →「删除」菜单项）：ElMessage.error 展示业务提示（bilibili 行）：
  > 无法删除站点：作品 33、任务 156、站点标签 128、站点作者 27。请先在作品页删除相关作品、在任务列表删除相关任务、在站点标签页删除相关标签、在站点作者页删除相关作者，再重试删除
- SQL 核对：33/156/128/27 全对；bilibili 作品集=0 → 消息不列该项，「明细只列非零项」语义正确。
- 截图存档：`.claude/testing/shots/v2-site-page.png`（页面初始态）、`v2-ui-guard-message.png`（ElMessage 展示中）。

### V3 site.Delete 守卫拒绝·软删 work 占用（风险4 实机复核）—— PASS

- 素材：bindings 链 `WorkHandler.Save` 造 work 200（siteId=2/local，前端无作品直建按钮，记录为 bindings 链）→ 前端 wrapper `workSoftDelete(200)` 成功。
- 操作：`siteDeleteById(2=local)` → 拒绝，消息原文：
  > 无法删除站点：作品 47（含回收站 1）、任务 61。请先在作品页删除相关作品、在回收站彻底删除相关作品、在任务列表删除相关任务，再重试删除
- 断言：消息含「（含回收站 1）」——软删行计数实测确认；含「在回收站彻底删除相关作品」清理指引；SQL：work site_id=2 总 47/软删 1、task=61、其余三类 0 全对；work 200 deleted_at=1787597165900。
- **风险4 的「FK 不看 deleted_at」守卫拦截面语义推导实机成立**。

### V4 localTag.Delete 子标签重挂（缺口8）—— PASS

- 上提面：三级树 grand(15)→parent(16)→child(17)（`localTagSave` 带 `baseLocalTagId`）→ `localTagDeleteById(16)` 成功 → `localTagGetById(17)` 返回 `baseLocalTagId=15`（子上提祖父）。
- 置根面：补测 p2(18)→c2(19)，`localTagDeleteById(18)` 成功 → c2 行存在且 `baseLocalTagId` 缺失（JSON null = SQL NULL）——置根。
- 全程无 FK 报错（删除本身即顺序正确性证明——FK 强制库下漏步先违约）。

### V5 localTag.Delete 绑定清理（缺口1 绑定列）—— PASS

- 素材：`localTagSave("delv-bind")`=20 + `siteTagSave`=1533 + `siteTagUpdateBindLocalTag(20,[1533])` 成功。
- `localTagDeleteById(20)` 成功（无 FK 报错——绑定列无 FK，静默悬空面靠编排清理）。
- SQL：`site_tag` 1533 行保留且 `local_tag_id IS NULL`（unbound=1/stillbound=0）；`local_tag` id=20 行=0。

### V6 localTag.Delete 作品关联清理（缺口1 re_work_tag）—— PASS

- 素材：bindings `WorkHandler.Save` 造 work 201（siteId=2）+ `localTagSave("delv-rel")`=21 + 前端 `reWorkTagLink(201, LOCAL=0, [21], [""])` 成功。
- `localTagDeleteById(21)` 成功。
- 断言：`reWorkTagListByWorkId(201)` 返回 `[]`；SQL `re_work_tag WHERE local_tag_id=21`=0、`WHERE work_id=201`=0、local_tag 行=0。

### V7 localAuthor.Delete（缺口2）—— PASS（三面中两面实机，re_work_author 面单测锚定）

- 素材：`localAuthorSave("delv-author")`=5 + `siteAuthorSave`=208 + `siteAuthorUpdateBindLocalAuthor(5,[208])` 成功 + bindings `WorkHandler.Save` 造 work 202 带 `localAuthorId=5`。
- 前置 SQL 在位：site_author 208.`local_author_id`=5、work 202.`local_author_id`=5。
- `localAuthorDeleteById(5)` 成功（无 FK 报错）。
- 删除后 SQL：site_author.`local_author_id`→**NULL**、work.`local_author_id`→**NULL**（work 202 活行保留）、`local_author` 行=0。
- re_work_author 面：re_work_author 关联行唯一产道为任务入库流（`SaveBatchOnConflict` 仅 task 模块编排调用，handler/wrapper 无创建入口）——被 V10 同款阻塞，单测 `TestDeleteLocalAuthorCleansReferences`（OpenTestDB FK 库）PASS 锚定。

### V8 siteTag.Delete（缺口3）—— PASS

- 素材：`siteTagSave("delv-st-rel")`=1534 + `reWorkTagLink(201, SITE=1, [1534], [""])` 成功。
- `siteTagDeleteById(1534)` 成功。
- SQL：`re_work_tag WHERE site_tag_id=1534`=0；`site_tag` 行=0。

### V9 siteAuthor.Delete（缺口4）—— PASS（2026-08-25 补验轮实机）

- 原阻断：re_work_author 唯一产道=任务入库流，被 pid=0 缺口阻断；pid 修复后补验。
- 素材闭环：localImport 目录导入（`local://D:/lstest/delvB`，分类应答经 Events 链自动答 siteAuthor）→ 任务树 355(父)+356/357(子) 全 Finished → 产 work 204/205，各挂 site_author 209（delvAuthor9）+ re_work_author×1。
- 操作：前端 wrapper `siteAuthorDeleteById(209)`（= SiteAuthorManage 页删除按钮同链路），成功、零 FK 报错。
- 删后 SQL：site_author 209 行=0；`re_work_author WHERE site_author_id=209`=0（**关联清理**）；work 204/205 活行保留、resource_store 关联与 store 683/684 完好。
- 截图：`.claude/testing/shots/v9-siteauthor-before.png`（站点作者页含 delvAuthor9 行）。

### V10 task.DeleteTask（缺口6）—— PASS（2026-08-25 补验轮实机，根任务与子任务两形态）

- 素材：任务 348（单文件导入=根 leaf，pid=NULL）+ 任务树 355(父,pid=NULL)+356/357(子,pid=355)，全 Finished，resource 194(task_id=348)/195(task_id=356)/196(task_id=357) 在位。
- 操作（wrapper `taskDelete` = useTaskOperations.deleteTasks→taskApi.taskDelete 同链路）：
  - 形态一 根 leaf：`taskDelete([348])` → 成功、零 FK 报错 → task 348 行=0、**resource 194 行保留且 task_id=NULL**。
  - 形态二 父树：`taskDelete([355])`（单传父 ID，子行经 `pid IN` 覆盖）→ 成功、零 FK 报错 → 355/356/357 全删、**resource 195/196 保留且 task_id=NULL**、work 204/205 活行保留。
- **pid 修复实机铁证**：修复后 4 次任务创建（348/349-351/352-354/355-357/358）全部成功——根任务 `pid IS NULL` 落库（348/352/355 均 pid_is_null=1）、子任务 pid=真实父 ID（353→352、356/357→355）、全程零 `FOREIGN KEY constraint failed`；修复前同链路必败（「尝试了所有插件均未成功」+日志 pid 列写 0 违约）。

### V11 recycleBin.PurgeStore（缺口7）—— PASS（2026-08-25 补验轮实机，含场景偏离说明）

- 素材：work 204 软删（`workSoftDelete`）→ 其 store 683 软删（deleted_at=1787600742912）+ 移文件入 backup（backup 483）+ **resource_store 关联 1198（resource 195↔store 683）保留**——正是缺口7 的 FK 拦截形态（关联未摘即物理删行必被拒）。
- 操作：前端 wrapper `recycleBinPurgeStore(683)`（= 回收站「彻底删除」按钮同链路），成功、零 FK 报错。
- 删后 SQL：persistent_store 683 行=0（物理删）；`resource_store WHERE store_id=683`=0（**关联清理**）；resource 195 行保留；work 204 软删态不受扰；**backup 483 消费式删除**（终态清理义务生效）。
- **场景偏离说明（诚实记录）**：回收站文件条目页只收录「挂载链不指向软删作品」的软删行（`buildRecycleStoreWhere` 排除子句，`backend/search/repository.go:616-624`）；带关联文件条目的规范产道=替换流（ConfirmReplace→softDeleteReplaceTargets），但替换流被**新发现 bug 2**（确定性挂起，见下）阻断。本项改在「随作品软删的 store 行」上经同一 handler→service→事务（摘关联→物理删）链验证——`PurgeStore` 仅校验「已删条目」无可见性守卫，缺口7 修复代码路径被完整覆盖；「文件条目页 UI 上点按钮」的端到端留待 bug 2 修复后补。

### V12 resource.Delete 退役（阶段6）—— PASS

- bindings：`frontend/bindings/github.com/library-squirrel/backend/resource/handler.ts` 方法清单 = DeleteByWorkId/GetById/ListByWorkId/MergeCancel/MergeResource/Save/Update——**无 Delete**。
- wrapper：`frontend/src/apis/http/wrappers/resource.ts` 仅 MergeResource/MergeCancel——无删除函数。
- `frontend/src` 全树 grep `resourceDelete|ResourceHandler.Delete|.DeleteResource` 零命中。
- 页面全程（站点管理页导航/表格渲染/删除操作）加载运行无报错；vite dev 编译通过（dev server 200、页面模块图完整加载）。

## 新发现高危缺陷（V9/V10/V11 阻塞根因）：任务创建链路 FK 违约全坏

- **现象**：前端链路 `taskCreateByUrl("local://<目录>")` → 返回「尝试了所有插件均未成功」→ 日志（server.log 重启段）：
  > `INSERT INTO task (...) VALUES (...,true,0,...) | FOREIGN KEY constraint failed`（`pid` 列写 0）
- **根因**：`backend/task/service.go:646` `fillTaskFromResponse` 统一写 `task.Pid = sql.NullInt64{Int64: pid, Valid: true}`——根级任务（leaf/parent）pid=0 且 Valid=true 落库 0 哨兵；task.pid→task.id 外键对 0 无豁免 → INSERT 违约。外键方案迁移已将存量 pid 0 全量 NULL 化（存储契约 NULL=根，存量 260 行全 NULL），**写入路径的旧「0=根」契约未同步**。
- **为何测试没拦**：`backend/task/service_create_test.go:169` 明确断言「leaf Pid 应为 0 且 Valid=true」（旧写入契约）；`newTestService`（service_create_test.go:96-102）用 **fakeTaskRepo 内存假仓储**，不经 OpenTestDB 落盘——FK 免费锚定对任务创建路径完全失效。
- **影响面**：外键落地（commit 09950f3）后**任何插件任何 URL 的任务创建全部失败**。历史日志全量仅 1 次 task INSERT（即本次失败尝试）——用户此后未创建过任务，缺陷未暴露。
- **修复**：`backend/task/service.go:648` 改 `task.Pid = sql.NullInt64{Int64: pid, Valid: pid != 0}`（根任务 NULL、子任务 Valid=true 指向真实父 ID）+ 修正 service_create_test 旧契约断言 + OpenTestDB 负向探针。**补验轮实机确认修复生效**（V10 节铁证：4 次创建全成、根 pid=NULL、子 pid=父、零 FK 报约）。

## 新发现 bug 2（补验轮）：ConfirmReplace 替换流确定性挂起 + 全局 DB/IPC 楔死

- **复现步骤**（两轮独立复现，跨应用重启确定性）：①对已有作品重导入同 URL（`local://D:/lstest/delvA/one.png`，siteWorkId 同 hash）→ 创建任务 358；②`taskStartTrees` → 任务进 WaitingForInput（查重命中、覆盖确认弹窗路径）；③`taskManagerConfirmReplace(358, "replace")` → 日志停在：
  > `确认替换任务: taskId=358` / `WaitingForInput → Processing` / `run() 入口: taskId=358, runMode={workInfo:true, stores:[]}, ...`
  此后**再无任何日志**（查重回退/softDeleteReplaceTargets/CreateWorkInfo/SaveWorkInfo 均未到达）。
- **楔死面实测**：纯 DOM eval 正常响应（页面活着）；一切经 DB 的 IPC 全部挂起（`taskGetById` 8s 超时无响应）；外部 sqlite 只读连接正常（WAL）——**单一 DB 连接被持、Go 连接池永久等待**，与 MaxOpenConns=1 死锁家族特征吻合（rules/database.md「事务」节）。DB 中 task 358 status 停留 0（Processing 未提交）。
- **静态排查到此为止**：actor 链（handleRunCmd→runOnce→run→runSectionCombo）无事务包裹；batchCheckDuplicates（manager.go:428-506）纯查询无事务且带 30s 超时；confirm 重入路径 skipDuplicateCheck=true 跳过查重；DeleteWithBackup（persistentStore/service.go:736-767）无重试环。精确挂点需 goroutine dump（delve attach 或 pprof），建议按 diagnose 协议续查。
- **影响**：替换确认（重下覆盖）后任务永久卡 Processing、应用所有 DB 功能瘫痪（需重启恢复）；同时阻断 V11 的规范素材产道（带关联回收站文件条目=替换流产物）。
- **测试盲区同源**：与 pid 缺口同族——替换流集成路径无 OpenTestDB 落盘测试覆盖（ConfirmReplace→run 全链）。

## 新发现观察（低危）：work 服务站点作者 SaveBatch 对重复作者无幂等

- 现象：分类元数据含两条同名 siteAuthor（目录两级都被分类为 siteAuthor）时，CreateWorkInfo 返回重复作者两份 → work 服务 `SaveBatch`（非 OnConflict 变体）单批插入 2 行同 (work_id, site_author_id) → `UNIQUE constraint failed: re_work_author.work_id, re_work_author_id` → 任务 Failed（事务整体回滚、无残留）。
- 判定：真实插件数据（pixiv 等）作者去重后不太可能重复，属**鲁棒性缺口**而非主链缺陷；触发条件由本轮合成元数据构造。同族先例：reWorkAuthor 已有 `SaveBatchOnConflict`（task 入库流在用）——work 服务侧可对齐。

## 基建发现（备用记录）

- **「偶发插件双激活」根因实锤（修正前轮记录）**：前轮记「偶发、重启未复现」——补验轮两次命中并定位：**CDP eval 里 `import("/node_modules/.vite/deps/@wailsio_runtime.js")`（不带 `?v=<hash>` 查询串）会实例化第二份 Wails runtime**，其顶层初始化副作用令 `WindowRuntimeReady` 重触发 → `LoadPlugins()` 全量重跑（main.go:159）→ 各插件扩展/TaskHandler 二次注册全部 `extension already exists` → localImport 任务处理器失效，此后 CreateTaskByURL 必报「尝试了所有插件均未成功」。页面自身模块引用的 runtime 带 `?v=1985f57e`（vite 依赖预打包版本查询串），**带相同查询串 import 即复用同实例、零副作用**。测试脚本经 Events 链自动应答 localImport 分类时务必用带查询串的 URL。
- localImport 目录分类询问可经 Events 链自动应答（`plugin:local-import:classify:response`，与 ClassifyPanel confirm() 同构）；插件只消费响应的 `Meanings[].Type/Name`（level/dirName 由插件自身语境提供），固定载荷 `{meanings:[{type,name,id:""}]}` 即可定向分类。
- CDP 动态 import bindings：`/bindings/<pkg>/index.ts`（**.ts 后缀**；.js 会回退 index.html）；wails vite 插件接管 bindings 服务路径，须用页面 origin `http://wails.localhost:9245/bindings/.../index.ts`。
- 回收站文件条目页收录口径（`backend/search/repository.go:616-624`）：软删行 + 挂载链**不**指向软删作品（随作品软删的行聚合进作品条目）；「带关联的文件条目」规范产道=替换流（bug 2 阻断中）。

## 清理与终态

- 首轮清理：三个 delv work 经前端链 SoftDelete→PurgeWork 全成功；delv 标签/站点标签/站点作者/本地作者全删；失败的任务创建未落任何行。
- 补验轮清理：任务 358（bug 2 残留 Created）删除；works 203/204/205 经软删+PurgeWork 全回收（store/关联/备份/物理文件全清，`store/resource/delvAuthor9` 空目录一并移除）；源素材目录 `D:\lstest` 与临时脚本删除。
- **终态 SQL（两轮后）**：task=260（基线）、work=93（基线）、store 软删=3（用户既有基线）、新增 task/work/resource/resource_store/site_author/re_work_author/backup 计数全 0、`delv%` 全表计数=0。
- 物理面：`store/resource/unknownAuthor` 17 文件全为用户既有（测试 one.png 已随 purge 消失）；backup 域按测试指纹扫描 0 残留。
- 测试进程全杀（测试应用 + 独立 vite），端口 9245/9222 无监听残留。

## 结论

- **通过面（12/12 项，八入口全覆盖）**：localTag.Delete（缺口1 re 关联面 + 缺口8 树重挂）、localAuthor.Delete（缺口2 双面实机）、siteTag.Delete（缺口3）、**siteAuthor.Delete（缺口4，补验轮实机——re_work_author 关联清理实证）**、site.Delete 纯守卫（缺口5：全空删除 + 活行拒绝 + 软删占用拒绝）、**task.DeleteTask（缺口6，补验轮实机——根 leaf 与父树两形态、resource 行保留 task_id=NULL 实证）**、**recycleBin.PurgeStore（缺口7，补验轮实机——摘关联+物理删+消费式清备份实证，场景偏离见 V11 节）**、resource.Delete 退役（阶段6）——全部删除成功、无 FK 报错、无悬空引用。
- **pid=0 缺口修复实机生效**：`Valid: pid != 0` 落地后 4 次任务创建全成（根 pid=NULL/子 pid=父 ID/零 FK 违约），任务链路恢复正常，V9/V10/V11 三项验证解锁并全过。
- **site 守卫业务提示实测正确**（首轮）：计数与库逐项一致、明细只列非零项、含分类清理指引、软删行「（含回收站 N）」聚合；真实 UI 按钮链展示正常。方案风险4 实机复核成立。
- **新发现 bug 2（未修复，高优先）**：ConfirmReplace 替换流确定性挂起并楔死全局 DB/IPC（复现步骤与证据见专节）——建议按 diagnose 协议以 goroutine dump 定位挂点后修复，修复后补「文件条目页 UI 端到端」的 PurgeStore 验证。
- **低危观察**：work 服务站点作者 SaveBatch 对重复作者无幂等（UNIQUE 违约致任务 Failed，事务回滚无残留）。
- **测试基建修正**：「插件双激活 flake」根因=CDP eval 不带 `?v=` 查询串 import runtime 预打包件（第二实例副作用）；已固化规避方法。

**终审建议**：①对照 `.claude/testing/shots/v2-ui-guard-message.png` 亲点一次站点管理页「删除」核对提示文案；②（可选）亲历一次重下同作品的替换确认，体验 bug 2 的卡死现象以评估修复优先级。
