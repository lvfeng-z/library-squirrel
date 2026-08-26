# export 模块说明

## 一句话职责
把用户选中的作品/作品集（含成员作品递归闭包、标签/作者/作品集/文件等全部关联）收集为可移植的导出数据模型（manifest 契约 + 源文件清单），并按确定性命名的作品目录结构打包为 ZIP（异步执行 + 进度事件 + 可取消），是导出功能的**数据面 + 执行面**（方案见 `doc/plan/导出功能总体方案.md`）。

## 边界
- 与 backup：backup 是纯文件仓库（无业务语义）；export 是业务模块，消费 work/resource/tag/author/workSet 等业务实体做语义保真导出。
- 与 localImport：localImport 是「目录→作品」语义弱导入；export 走「manifest → 作品」语义保真格式，二者档位不同。
- **当前范围**：数据收集（阶段2）+ ZIP 打包执行/进度/取消（阶段3）已实现。回灌导入（阶段4）排期另做，manifest 契约已定稿。

## 对外接口（Handler）
| 方法 | 作用 |
| --- | --- |
| `Collect(workIDs, workSetIDs)` | 收集导出数据模型（决策5：前端透传选中 id 列表；超上限报错提示分批） |
| `StartExport(workIDs, workSetIDs, outputDir)` | 启动异步导出，立即返回 `exportID`；`outputDir` 为空落盘工作目录根、非空为自选输出目录；进度/完成经 `export-events` 事件推送（不阻塞 IPC） |
| `CancelExport(exportID)` | 取消进行中导出（无则 no-op）；中断打包并清理临时文件，推送「已取消」 |

## 核心概念
- **选择即单位**（决策2）：选择只作用于作品/作品集；标签/作者恒完整连带导出。选中作品集 + 其成员作品（含子作品集递归闭包）构成导出闭包；成员关系（`re_work_work_set`）、作品集父子边（`re_work_set_work_set`）仅保留两端均在闭包内的边，指向未选作品集的边丢弃。
- **manifest 契约**：`schemaVersion` 版本锚（对齐插件 plugin_data 版本纪律）；`meta` 导出时间/来源 app 版本/计数；`sites[]`/`localAuthors[]`/`siteAuthors[]`/`localTags[]`（含层级引用）/`siteTags[]`（含 namespace）；`worksets[]`（全字段 + 父边 + 封面）；`works[]`（全字段 + `resources[]` 的 resource_store 活行挂载 + 标签/作者关联含 namespace + 作品集成员关系）；`files[]`（被 store 挂载按 StoreID 引用；打包时填充 `path`/`size`/`sha256`，缺失置 `missing`）。
- **store 活行过滤**：`resource_store` 关联只取指向活行 `persistent_store` 的行（`deleted_at = 0`），软删行关联（替换/merge 残留代）不进导出，遵循 STORE_ASSOCIATION_LIVENESS_FILTER。
- **完整连带**：站点（work/work_set 的 site_id 去重）、本地标签祖先链（`base_local_tag_id` 逐层补齐）、site→local 桥接（`site_author.local_tag_id`/`site_tag.local_tag_id`、work 的 `local_author_id` 镜像列）均随行导出，保证回灌不悬空。
- **确定性打包**（决策3，风险3 对治）：包内按 `works/<作品目录名>/<文件>` 组织。作品目录名 `sanitize(siteWorkName)` → 空回退 `sanitize(siteWorkId)` → 再空回退 `work_<id>`，冲突追加序号 `_2`/`_3`；文件保留原名，同目录同名冲突按 store 命名规约 `<bas>_<role>_<seq>` 消解（声明8）。同输入同输出、结构可复现。
- **源文件缺失**（决策4）：打包逐文件判源存在性，缺失 → 跳过 + `files[]` 该条目置 `missing=true`（store 缺席、其余照常），不中断导出。
- **执行形态**（决策1，异步轻量壳）：`StartExport` 起后台 goroutine（Collect→Plan→磁盘预检→Pack→原子 rename），进度按「已处理文件数/总文件数 + 累计字节」经 `export-events` topic 推送，可 `CancelExport` 中断。临时文件写目标盘同级临时名，成功原子重命名为最终 zip，失败/取消不留半成品；启动清理工作目录残留（对齐 merge `ls-merge-` 先例）。
- **自选输出目录**：`outputDir` 空 = 工作目录根（缺省），非空 = 输出目录。默认值由**设置页显式配置**（`settings.exportSettings.outputDir`，设置页「导出默认目录」字段）；导出弹窗内可临时改选（仅本次有效、不写回设置）。目标路径/磁盘预检/临时清扫均以输出目录为准；输出目录不存在时创建；输出目录的崩溃残留由每次导出前 `sweepStaleTemp(outDir)` 兜底（启动清理只扫工作目录）。
- **磁盘预检**（风险6）：导出前查输出目录所在卷可用空间（store 模式 zip≈源文件总量，预检新增 zip 容量 + 1/10 余量），不足报错中止。

## 依赖关系
- 依赖：`Repository` 接口（`backend/export/repository.go`，直查共享表；由 app.go 以 `*gorm.DB` 装配）；`workDir` 来源（settings 服务，`func() string` 延迟读取）；Wails 事件发射器（`export-events`，延迟闭包读 emitter）。
- 被依赖：前端导出入口（阶段1 操作栏接线后调用 `StartExport`/`CancelExport` 并订阅 `export-events`）。

## 关键设计
- **批量查询**：按 ELIMINATE_N_PLUS_1 收集 ID → 批量 `IN` 查询 → 构建 map → 组装；所有查询走 `database.DBFromContext`（事务感知）。
- **上限保护**（决策5）：`Collect` 对 work/workSet id 数量设上限（10000/5000），万级全选时超限报错提示分批。
- **确定性**：manifest 各域数组按 ID 升序输出；命名分配按固定顺序（作品 ID → 资源 ID → store 挂载序），zip 条目按 `files[]`（StoreID 升序）写入，zip 条目时间固定为导出时刻——同输入同输出、字节级可复现。
- **压缩策略**（风险5）：`manifest.json` deflate 压缩（体积小）；媒体文件 store 模式不压缩（大文件免重复压缩）。
- **路径纪律**：包内路径（`works/...`）与源 `store_path` 均为 relPath 域正斜杠（`path.Join`/`path.Base`）；absPath 仅 os.* 调用点现场 `filepath.Join(workDir, rel)` 构造。
- **IPC 形态**：`Collect(ctx, workIDs []int64, workSetIDs []int64)`——id 列表透传，不引入后端范围查询条件上下文。
