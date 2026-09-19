# import 模块说明

> 目录名 `backend/import`，包名 `importer`（`import` 是 Go 关键字不能作包名）。

## 一句话职责
把导出产物（manifest 契约 + 包内文件）回灌导入本库：完整重建作品与全部关联（往返保真），入库逻辑抽为 **ManifestIngestor 能力接口**，供导出回灌（本模块 Handler）与分享收件人侧（share-receive 任务执行器）两个消费方复用（方案见 `../library-squirrel-docs/plan/分享功能总体方案.md` 阶段2）。

## 边界
- 与 export：export 是导出数据面（收集 + 打包），本模块是其逆向（回灌）；manifest 契约直接复用 `backend/export/manifest.go`（SchemaVersion 版本锚），**禁止重定义**。
- 与 localImport：localImport 是「目录→作品」语义弱导入；本模块是「manifest→作品」语义保真导入，档位对齐 export。

## 对外接口（Handler）
| 方法 | 作用 |
| --- | --- |
| `ImportFromZip(zipPath)` | 从导出 ZIP 产物回灌导入：解包读 manifest → 版本锚校验 → 入库 → 返回 `ImportResult` 摘要。同步执行（进度/取消形态归二期任务模块接入） |

## 核心概念
- **ManifestIngestor**（能力接口）：`Ingest(ctx, manifest, fileSource)`——fileSource 为包内路径→内容流的文件源（zip 打开器与分享拉取流各自实现，导入核心不感知来源）。返回摘要 `ImportResult` 携带本次新建的 persistent_store 行 ID 清单（`CreatedStoreIDs`，导入事务提交后填充）——供调用方在导入成功后登记终态回滚（复活被替换旧代前先丢弃新建行）。
- **查重语义（方案决策15）**：作品/作品集按 `site_id+site_work_id`（`site_work_set_id`）联合键查重，命中整体跳过（关联/资源一概不动）；站点按 `site_key` find-only（manifest `SiteRecord` 携带必填 `siteKey`，未注册键导入报错；注册键的本库站点行由启动期注册表投影保证存在，缺失属数据异常显式报错；站点身份规范见 `doc/site-identity-spec.md`）；本地标签/本地作者按名称 find-or-create（同名复用，歧义归 todo#10 合并功能）；站点标签/站点作者按（`site_key` 解析的站点 + `site_tag_id`/`site_author_id`）复合身份匹配（与 work 模块 upsert 复合键口径一致）。无站点身份的作品/作品集无跨库稳定身份，恒新建（重复导入会重复建，属查重兜底范围外）。
- **三相推进**：①只读预检（既有站点键映射 + 作品查重圈定待建集）→ ②解包到暂存（仅待建与替换作品；经暂存层流式落暂存，TeeReader 边读边算 sha 并与 manifest 声明比对；落位路径冲突派生 `_import<n>` 变体，不改写既有文件）→ ③入库事务序列：登记入库意图（撤回处置按暂存层声明）→ 落位（暂存同卷 rename 进最终路径）→ 业务事务内逐文件建 persistent_store 行删登记行 + 主数据入库（站点 find-only、标签/作者 find-or-create + 新建作品/资源/挂载/关联，全部导出库 ID→本库 ID 重映射）。②先于事务（文件 IO 不占唯一 DB 连接）；任一相位失败：已落位文件按登记时声明的处置撤回（退回暂存/丢弃），暂存层统一退出收尾。
- **保真面**：作品/作品集/标签/作者字段与源库时间戳原样落库；关联级维度（`re_work_tag.namespace` / `re_work_author.role_name`，随 `TagLink`/`AuthorLink` 往返）、role/sort、作品集父边双轨排序、封面、标签层级全保真；同名坍缩令多条 manifest 关联落同一本库行时按唯一索引键（含维度列）折叠、保留首条属性。旧版导出包 `TagRecord.Namespace`（site_tag 行级字段）回灌静默忽略——关联侧 ns 由 `TagLink` 承载，语义不丢。
- **降级面**：源库 task_id 不落地（NULL——导入资源非本库任务产出）；挂载缺席（决策4 源文件缺失/无包内路径）跳过该挂载不报错；引用悬空（父标签/桥接/封面指向 manifest 外）按 NULL 落库；环状标签引用按根标签降级。

## 依赖关系
- 依赖：`Repository`（本模块自有，直查共享表——对齐 export 先例）；`Transactor`（app.go 以 dbTransactorAdapter 装配）；`StoreIngestOperator`（persistentStore.Service 实现：按路径查活行（落位冲突检测）+ 入库事务四调用）；staging 能力包（zip 解包轨的暂存作用域，接入形态见 `IngestStaging` 接口注释——收件暂存轨由 share 模块自实现）。
- 被依赖：前端导入入口（待接线）；share-receive 任务执行器（已注入 ManifestIngestor，app.go 与本模块 Handler 共用同一实例）。

## 关键设计
- **文件落位冲突消解**：落位目标路径与库内既有活行（`GetByFilePath`）及本次导入已占用路径均不重叠才采用原路径，命中冲突保留目录与扩展名派生 `_import<n>` 变体——不改写既有作品文件（RECORD_STATE_TRUTHFUL）。
- **结构校验前置**：ResourceType/StoreType/Generation 严格识别（RESOURCE_TYPE_STRICT）在解包相位开头完成，非法产物零副作用失败；包内文件 sha256 随解包读流边读边算校验（manifest 声明才比对）。
- **幂等**：重复导入同产物 → 作品/作品集全量查重跳过 + 主数据全量复用，任何表不增行。
