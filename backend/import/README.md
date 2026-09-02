# import 模块说明

> 目录名 `backend/import`，包名 `importer`（`import` 是 Go 关键字不能作包名）。

## 一句话职责
把导出产物（manifest 契约 + 包内文件）回灌导入本库：完整重建作品与全部关联（往返保真），入库逻辑抽为 **ManifestIngestor 能力接口**，供导出回灌（本模块 Handler）与分享收件人侧（share-receive 任务执行器）两个消费方复用（方案见 `doc/plan/分享功能总体方案.md` 阶段2）。

## 边界
- 与 export：export 是导出数据面（收集 + 打包），本模块是其逆向（回灌）；manifest 契约直接复用 `backend/export/manifest.go`（SchemaVersion 版本锚），**禁止重定义**。
- 与 localImport：localImport 是「目录→作品」语义弱导入；本模块是「manifest→作品」语义保真导入，档位对齐 export。

## 对外接口（Handler）
| 方法 | 作用 |
| --- | --- |
| `ImportFromZip(zipPath)` | 从导出 ZIP 产物回灌导入：解包读 manifest → 版本锚校验 → 入库 → 返回 `ImportResult` 摘要。同步执行（进度/取消形态归二期任务模块接入） |

## 核心概念
- **ManifestIngestor**（能力接口）：`Ingest(ctx, manifest, fileSource)`——fileSource 为包内路径→内容流的文件源（zip 打开器与分享拉取流各自实现，导入核心不感知来源）。
- **查重语义（方案决策15）**：作品/作品集按 `site_id+site_work_id`（`site_work_set_id`）联合键查重，命中整体跳过（关联/资源一概不动）；站点按 `site_key` find-or-create（manifest `SiteRecord` 携带必填 `siteKey`，未注册键导入报错；站点身份规范见 `doc/site-identity-spec.md`）；本地标签/本地作者按名称 find-or-create（同名复用，歧义归 todo#10 合并功能）；站点标签/站点作者按（`site_key` 解析的站点 + `site_tag_id`/`site_author_id`）复合身份匹配（与 work 模块 upsert 复合键口径一致）。无站点身份的作品/作品集无跨库稳定身份，恒新建（重复导入会重复建，属查重兜底范围外）。
- **三相推进**：①只读预检（既有站点键映射 + 作品查重圈定待建集）→ ②文件落盘（仅待建作品；路径冲突派生 `_import<n>` 变体，不改写既有文件）→ ③单事务入库（find-or-create 主数据 + 新建作品/资源/挂载/关联，全部导出库 ID→本库 ID 重映射）。②先于事务（文件 IO 不占唯一 DB 连接），任一相位失败对已落盘文件补偿清理（物理删，不留半成品）。
- **保真面**：作品/作品集/标签/作者字段与源库时间戳原样落库；namespace（site_tag 行 + re_work_tag 关联级）、role/sort、作品集父边双轨排序、封面、标签层级全保真；同名坍缩令多条 manifest 关联落同一本库行时按唯一索引键折叠、保留首条属性。
- **降级面**：源库 task_id 不落地（NULL——导入资源非本库任务产出）；挂载缺席（决策4 源文件缺失/无包内路径）跳过该挂载不报错；引用悬空（父标签/桥接/封面指向 manifest 外）按 NULL 落库；环状标签引用按根标签降级。

## 依赖关系
- 依赖：`Repository`（本模块自有，直查共享表——对齐 export 先例）；`Transactor`（app.go 以 dbTransactorAdapter 装配）；`FileStoreOperator`（persistentStore.Service 实现：落盘+建记录含抑制登记/指纹/宽高、按路径查活行、补偿物理删）。
- 被依赖：前端导入入口（待接线）；share-receive 任务执行器（已注入 ManifestIngestor，app.go 与本模块 Handler 共用同一实例）。

## 关键设计
- **文件落位冲突消解**：persistentStore.Store 的同路径语义是「删旧文件+改旧记录」（面向重下载），导入命中既有占用时改为派生变体路径，保护既有作品文件（RECORD_STATE_TRUTHFUL）。
- **结构校验前置**：ResourceType/StoreType/Generation 严格识别（RESOURCE_TYPE_STRICT）在写盘前完成，非法产物零副作用失败；包内文件 sha256 随流校验（manifest 有值时）。
- **幂等**：重复导入同产物 → 作品/作品集全量查重跳过 + 主数据全量复用，任何表不增行。
