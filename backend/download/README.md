# download 模块说明

## 一句话职责

插件下载任务的**执行面**：实现 `plugin-download` 任务类型的 ExecutionStrategy（板块组合执行、查重内移确认、多轨下载与断点续传、替换链前置软删与失败回滚登记），另持有作品任务领域行（work_task）仓储与板块重执行选择写行器。

## 边界

- 与 **taskManager**：taskManager 是任务运行时**控制面**（调度/状态机/信号量），本模块是 plugin-download 的执行面——经 `taskManager.StrategyHandle` 上报终态/进度/覆盖确认/跳过，经其恢复信号分叉续传与全新执行；暂停/停止的插件 RPC 转发经 `taskManager.InterruptNotifier`。
- 与 **task**：本模块持有作品任务领域行（work_task）仓储；task 模块建树写行与树双查读行经窄接口（`WorkTaskWriter`/`WorkTaskReader`）注入本仓储消费，双向零 import（app.go 装配缝合）。

## 文件结构

| 文件 | 职责 |
| --- | --- |
| `strategy.go` | PluginDownloadStrategy 执行入口（按 taskId 查领域行、按恢复信号分叉）+ 中断通知（转发插件 Pause/Stop RPC） |
| `session.go` | 执行会话（一次执行的状态载体：领域行快照/流集合/软暂停消费/失败终态清 pending） |
| `combo.go` | 板块组合执行 + runMode 三态派生（板块模式唯一源=领域行）+ 查重编排（WaitReplaceConfirm 挂起、决策记忆匹配） |
| `persist.go` | 入库编排（资源落盘事务/pending 直写）+ 替换前置软删与终端回滚登记 + 新建行清单登记/失败收口关闭流句柄 |
| `loop.go` | 多轨流管理与下载循环（copyLoop/handleEOF/软暂停信号消费/进度聚合） |
| `resume.go` | 跨重启续传（偏移推导、插件 Resume、derived 重产；资源缺失降级完整重跑） |
| `naming.go` | 文件名模板与落盘路径解析（bas 基准名、多 store 消歧） |
| `interfaces.go` | 对外窄接口与依赖集合（Deps；各能力接口由提供方模块实现） |
| `work_task_repository.go` | 作品任务领域行仓储（共享主键覆写守卫、板块选择写行、pending 直写 SQL） |
| `section_recorder.go` | 板块重执行选择写行器（重下载入口两步编排第一步；父请求展开到全部子成员） |

## 核心概念

- **执行入口分叉**：按 taskId 查 work_task 行（缺失即失败收口）→ 恢复信号（`handle.ResumeRequested()`，控制面按执行前实时状态==Paused 置位）且领域行持 pending 时走跨重启续传，其余走板块组合（重走查重/板块选择/替换链）。
- **板块模式三态**（runMode）：`None`=仅作品信息（不拉资源、经 Skip 收口不产生终态）/`All`=全量（roles 取插件 universe，空=插件自决）/`Selected`=用户子集；从领域行 `StoreRoles`（NULL→All、空串→None、非空→Selected）+ `IncludeWorkInfo` 派生，每执行查行（板块模式唯一源）。
- **查重内移**：含资源板块时执行内判定重复——命中冲突先匹配确认决策记忆（冲突作品集一致才复用，跨暂停/恢复不重复弹窗），未命中经 `WaitReplaceConfirm` 挂起等待用户整体答复；跳过答复经 `Skip` 上报回执行前状态。
- **替换链**：含资源板块在已有作品上重执行即替换——前置软删所选板块旧 store 并登记受害者清单（多次软删合并去重）；新建/续接的 store 行在创建事务提交后登记进同一终端回滚载荷（跨执行轮次并集，覆盖停止/暂停恢复后失败等不经会话收口的中断路径）。失败/停止时由控制面 setFailed 单点统一处置：先按登记丢弃新建行（行+文件+关联）、再复活受害者（非替换失败无受害者，保留已下载成果）。会话失败收口只负责关闭流写入句柄释放文件锁，删除动作全部收在控制面单点。
- **软暂停**：控制面进入软暂停时 close 广播通道，copyLoop 完成当前在途读取并落盘后收尾退出（磁盘落点对齐真实中断点，供 Range 续传）；排空超时（控制面 2s 兜底）强制取消退化为有损暂停。

## 依赖关系

- 依赖（`Deps`，接口由本模块定义、提供方实现、app.go 装配）：插件执行器工厂（plugin 扩展桥）、作品信息保存/命名元数据加载（work）、资源保存/查询/关联读写/完整度重算（resource）、查重判定（duplicate）、站点键解析（site）、store 落盘流/记录查询/删除（persistentStore）、替换链能力（resource）、工作目录与文件名模板（settings）、事务执行器、pending 直写、任务核心行查询（task）
- 被依赖：taskManager（plugin-download 执行面策略注册 + 重下载板块写行 `SectionRecorder` + 活跃插件计数投影 + work 删除链 pending 清理）、task（建树写行/树双查读行窄接口消费本模块仓储）、plugin 扩展桥（`GetStoreRelPath` 查领域行 pending）

## 关键设计

- **无跨执行可变状态**：策略对象无状态，每次 Execute 自建执行会话（领域行快照/流集合/决策位随会话生灭）。
- **时序不变量**：downloadLoop 须在资源落盘事务提交后执行——插件 pull chunk 时可能经 GetStoreRelPath 查 resource_store 路径，仅事务提交后对其可见。
- **多 store 身份**：同 role 内 `store_seq` 唯一定位 store（文件名消歧/续传身份匹配同维度）；resume 的 specs 是未完成子集，全局 seq 按 streamOffsets/storeRows 推导，不按 specs 内重计（防部分完成时错位覆盖）。
- **失败终态清 pending**：失败任务不续传（残留 pending 在作品/资源被外部删除或还原后指向失效对象），Fail 上报前清理；暂停保留 pending 供恢复续传定位。
