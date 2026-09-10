# export 模块说明

## 一句话职责
把用户选中的作品/作品集（含成员作品递归闭包、标签/作者/作品集/文件等全部关联）收集为可移植的导出数据模型（manifest 契约 + 源文件清单），并打包为确定性命名的 ZIP——既是导出任务（`task_type='export'`）的执行主体，也是分享发布的数据面（方案见 `../library-squirrel-docs/plan/导出功能总体方案.md`）。

## 边界
- 与 backup：backup 是纯文件仓库（无业务语义）；export 是业务模块，消费 work/resource/tag/author/workSet 等业务实体做语义保真导出。
- 与 localImport：localImport 是「目录→作品」语义弱导入；export 走「manifest → 作品」语义保真格式，二者档位不同。
- 与 import：export 产出 manifest 契约与导出包，回灌导入归 import 模块（`ManifestIngestor` 能力接口，分享收件侧复用同一入库核心）。
- 与 share：share 发布注入本模块的 Collect/Plan 能力（`ExportCollector`/`ExportPlanner` 接口）做数据收集，不重复收集；share 不落盘打包（无 ZIP）。

## 对外接口（Handler）
| 方法 | 作用 |
| --- | --- |
| `Collect(workIDs, workSetIDs)` | 收集导出数据模型（前端透传选中 id 列表；超上限报错提示分批） |
| `StartExport(workIDs, workSetIDs, outputDir)` | 创建导出任务并启动（两步建任务），返回 `ExportTaskResult{TaskID}`——进度与终态经任务面板统一承载 |

## 核心概念
- **选择即单位**：选择只作用于作品/作品集；标签/作者恒完整连带导出。选中作品集 + 其成员作品（含子作品集递归闭包）构成导出闭包；成员关系（`re_work_work_set`）、作品集父子边（`re_work_set_work_set`）仅保留两端均在闭包内的边，指向未选作品集的边丢弃。
- **manifest 契约**：`schemaVersion` 版本锚（对齐插件 plugin_data 版本纪律）；`meta` 导出时间/来源 app 版本/计数；`sites[]`/`localAuthors[]`/`siteAuthors[]`/`localTags[]`（含层级引用）/`siteTags[]`（含 namespace）；`worksets[]`（全字段 + 父边 + 封面）；`works[]`（全字段 + `resources[]` 的 resource_store 活行挂载 + 标签/作者关联含 namespace + 作品集成员关系）；`files[]`（被 store 挂载按 StoreID 引用；打包时填充 `path`/`size`/`sha256`，缺失置 `missing`）。
- **store 活行过滤**：`resource_store` 关联只取指向活行 `persistent_store` 的行（`deleted_at = 0`），软删行关联（替换/merge 残留代）不进导出，遵循 STORE_ASSOCIATION_LIVENESS_FILTER。
- **完整连带**：站点（work/work_set 的 site_id 去重）、本地标签祖先链（`base_local_tag_id` 逐层补齐）、site→local 桥接（`site_author.local_tag_id`/`site_tag.local_tag_id`、work 的 `local_author_id` 镜像列）均随行导出，保证回灌不悬空。
- **确定性打包**：包内按 `works/<作品目录名>/<文件>` 组织。作品目录名 `sanitize(siteWorkName)` → 空回退 `sanitize(siteWorkId)` → 再空回退 `work_<id>`，冲突追加序号 `_2`/`_3`；文件保留原名，同目录同名冲突按 store 命名规约 `<bas>_<role>_<seq>` 消解（`doc/store-naming-convention.md`）。同输入同输出、结构可复现。
- **源文件缺失**：打包逐文件判源存在性，缺失 → 跳过 + `files[]` 该条目置 `missing=true`（store 缺席、其余照常），不中断导出。
- **export_task 领域行**：导出任务的选择参数载体，与所属 task 核心行 1:1 共享主键（`entity.ExportTask`，工厂 `NewExportTask(taskID)` 对非正 id panic）。列：`work_ids`/`work_set_ids`（JSON 数组文本，恒序列化、空集存 `[]`）、`output_dir`（输出目录原值，空串=执行时取 workDir 根）。仓储归本模块（`ExportTaskRepository`：`CreateForTask` 单口写入 + `GetById`）。
- **两步建任务**（`Service.StartExport`）：前置校验选择非空（失败不建任务行）→ 建任务核心行（经 `TaskControl.CreateBuiltinTask`，任务名「导出（N 项）」）→ 补写 export_task 领域行 → `StartTasks` 启动；任一步失败显式 `DeleteTask` 回滚（不留孤儿任务）。`TaskControl` 窄接口（CreateBuiltinTask/StartTasks/DeleteTask）由 task.Service 与 taskManager 经 app.go 适配器组合实现、延迟 setter 注入（ExportService 创建早于二者）。
- **执行面策略 ExportExecution**（`task_execution.go`，按 task_type 注册进 taskManager 执行面策略表）：读 export_task 领域行取选择参数（行缺失/JSON 损坏按过时载荷显式 Fail「请删除本任务后重新导出」）→ Collect → workDir 守卫（空则 `settings.NotifyWorkDirUnconfigured("export")` + Fail 可读文案）→ Plan → 输出目录解析（`output_dir` 空取 workDir 根、非空且≠workDir 则 MkdirAll）→ 目标目录残留清扫 → 磁盘预检 → Pack 写 `.zip.tmp`（进度回调按字节映射 `ReportProgress(totalBytes, processedBytes)`，前端按比值展示）→ 原子 rename 为最终 zip → `Finish`。`StrategyHandle` 使用集恰五方法（Task/RunCtx/Fail/Finish/ReportProgress）：无覆盖确认、无 DB 回滚登记、无板块/排空概念（暂停走立即取消，Pack 逐文件 ctx 检查点即退出点）；RunCtx 取消（暂停/停止不可区分）静默返回不上报终态，由控制面接管。
- **暂停/恢复语义**：暂停/停止检查点退出并保留 `.zip.tmp`；恢复/重试走重新 Execute 全量重跑（zip 不支持续写，重跑前清扫目标目录残留）；停止终态后的残留由下次导出前清扫兜底。
- **导出暂存私例**：`.zip.tmp` 写目标目录同级（同卷保证 rename 原子），不进统一暂存根 `task-staging/`——输出目录可为任意自选目录（含与工作目录异卷），统一暂存根固定在 workDir 卷，跨卷 rename 不可绕。
- **产物命名与并发**：最终 zip 为 `library-squirrel-export-<毫秒时间戳>.zip`（毫秒时间戳抗同目录并发碰撞）；并发导出同 outDir 不互斥——写入期 Windows 句柄占用使清扫删除失败仅告警不中断。
- **自选输出目录**：`outputDir` 空 = 工作目录根（缺省），非空 = 自选输出目录。默认值由设置页显式配置（`settings.exportSettings.outputDir`）；导出弹窗内可临时改选（仅本次有效、不写回设置）。目标路径/磁盘预检/残留清扫均以输出目录为准；输出目录不存在时创建。
- **磁盘预检**：导出前查输出目录所在卷可用空间（store 模式 zip≈源文件总量，预检新增 zip 容量 + 1/10 余量），不足 Fail 带容量文案。

## 依赖关系
- 依赖：`Repository`（`repository.go`，数据面直查共享表，app.go 以 `*gorm.DB` 装配）、`ExportTaskRepository`（自有领域行仓储）、`TaskControl` 窄接口（app.go 延迟 setter 适配）、workDir 来源（settings，`func() string` 延迟读取）、`settings.NotifyWorkDirUnconfigured`（workDir 未配置统一通知）。
- 被依赖：taskManager（`ExportExecution` 按 task_type 注册进执行面策略表，app.go 装配）；share（发布复用 Collect/Plan 数据面）；前端导出入口（`ExportProgressDialog` 确认弹窗发起 StartExport；导出任务行在导出视图 `ExportTaskManage` 呈现——任务面板与导出视图经 `task.task_type` 的 ne/eq 查询过滤分工，任务成功/失败终态由任务模块统一通知承载并按类型路由到导出视图）。

## 关键设计
- **批量查询**：按 ELIMINATE_N_PLUS_1 收集 ID → 批量 `IN` 查询 → 构建 map → 组装；所有查询走 `database.DBFromContext`（事务感知）。
- **上限保护**：`Collect` 对 work/workSet id 数量设上限（10000/5000），万级全选时超限报错提示分批。
- **确定性**：manifest 各域数组按 ID 升序输出；命名分配按固定顺序（作品 ID → 资源 ID → store 挂载序），zip 条目按 `files[]`（StoreID 升序）写入，zip 条目时间固定为导出时刻——同输入同输出、字节级可复现。
- **压缩策略**：`manifest.json` deflate 压缩（体积小）；媒体文件 store 模式不压缩（大文件免重复压缩）。
- **路径纪律**：包内路径（`works/...`）与源 `store_path` 均为 relPath 域正斜杠（`path.Join`/`path.Base`）；absPath 仅 os.* 调用点现场 `filepath.Join(workDir, rel)` 构造。
- **残留清扫双轨**：启动清扫 `CleanupResidualTempFiles`（app.go 接线，只扫工作目录根）；每次导出前 `sweepStaleTemp(outDir)` 兜底自选目录（自选目录不在启动清扫范围，并发占用删除失败留待下次再清）。
