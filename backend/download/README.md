# download 模块说明

## 一句话职责

插件下载任务的**执行面**：实现 `plugin-download` 任务类型的 ExecutionStrategy（板块组合执行、查重内移确认、多轨下载与断点续传、提交窗口替换软删与失败回滚登记），另持有作品任务领域行（work_task）仓储与板块重执行选择写行器。落盘走**暂存模式**：下载内容先写 `{workDir}/staging/download/{taskID}/` 下的 role_seq 键暂存文件（作用域创建经 task 暂存基建确保入口，目录内含 scope.json 自证描述），全部轨道写满后在提交点统一 rename 进 store/ 并建行挂载——长下载全程零 DB 副作用。

## 边界

- 与 **taskManager**：taskManager 是任务运行时**控制面**（调度/状态机/信号量），本模块是 plugin-download 的执行面——经 `taskManager.StrategyHandle` 上报终态/进度/覆盖确认/跳过，经其恢复信号分叉续传与全新执行；暂停/停止的插件 RPC 转发经 `taskManager.InterruptNotifier`。
- 与 **task**：本模块持有作品任务领域行（work_task）仓储；task 模块建树写行与树双查读行经窄接口（`WorkTaskWriter`/`WorkTaskReader`）注入本仓储消费，双向零 import（app.go 装配缝合）。暂存目录派生、暂存文件命名与作用域确保（task 模块 `DownloadStagingPath`/`StagingFileName`/`EnsureDownloadScope` 基建）同样经窄接口 `StagingPaths` 注入，维持零 import。

## 文件结构

| 文件 | 职责 |
| --- | --- |
| `strategy.go` | PluginDownloadStrategy 执行入口（按 taskId 查领域行、按恢复信号×暂存存在性分叉）+ 中断通知（转发插件 Pause/Stop RPC） |
| `session.go` | 执行会话（一次执行的状态载体：领域行快照/流集合/软暂停消费/失败终态清 pending） |
| `combo.go` | 板块组合执行 + runMode 三态派生（板块模式唯一源=领域行）+ 查重编排（WaitReplaceConfirm 挂起、决策记忆匹配） |
| `persist.go` | 执行前规划（最终路径解析+暂存写入器打开）+ 提交点序列（替换软删+rename+建行挂载事务+抑制登记+失败补偿逆操作） |
| `staging.go` | 暂存基建：暂存写入器（文件句柄+全量 sha256 流式哈希+finalize 比对）、暂存目录枚举（role_seq 键解析+偏移推导） |
| `loop.go` | 多轨流管理与下载循环（copyLoop/handleEOF 完整性与哈希校验/软暂停信号消费/进度聚合） |
| `resume.go` | 跨重启续传（暂存枚举偏移、插件 Resume、认领配对与未认领重产；作品定位失败/暂存为空降级完整重跑） |
| `naming.go` | 落盘路径派生（SDK storepath：桶段 = 复合键 SHA256 前 2 位 hex、作品目录段 = 站点复合键单射派生、文件名恒 `{role}_{seq三位}.{ext}`；本文件组装身份输入与 `store/resource/` 库内布局前缀） |
| `interfaces.go` | 对外窄接口与依赖集合（Deps；各能力接口由提供方模块实现） |
| `work_task_repository.go` | 作品任务领域行仓储（共享主键覆写守卫、板块选择写行、pending 直写 SQL） |
| `section_recorder.go` | 板块重执行选择写行器（重下载入口两步编排第一步；父请求展开到全部子成员） |

## 核心概念

- **执行入口分叉**：按 taskId 查 work_task 行（缺失即失败收口）→ 恢复信号（`handle.ResumeRequested()`，控制面按执行前实时状态==Paused 置位）且暂存目录有轨道文件时走跨重启续传（暂存枚举推导偏移），其余走板块组合（重走查重/板块选择/替换链）。旧模式暂停任务（有未完成行无暂存锚）自动降级全新重下。
- **暂存模式**：执行前为全部 specs 解析最终路径（最终名前置解析，暂存文件名按 role_seq 三位零填充键——续传定位与清扫不依赖元数据重解析，与最终名解耦）。下载循环写暂存（流写入器带全量 sha256 边写边算），失败轨暂存保留（诊断可见、重试重下覆盖）。
- **板块模式三态**（runMode）：`None`=仅作品信息（不拉资源、经 Skip 收口不产生终态）/`All`=全量（roles 取插件 universe，空=插件自决）/`Selected`=用户子集；从领域行 `StoreRoles`（NULL→All、空串→None、非空→Selected）+ `IncludeWorkInfo` 派生，每执行查行（板块模式唯一源）。
- **查重内移**：含资源板块时执行内判定重复——命中冲突先匹配确认决策记忆（冲突作品集一致才复用，跨暂停/恢复不重复弹窗），未命中经 `WaitReplaceConfirm` 挂起等待用户整体答复；跳过答复经 `Skip` 上报回执行前状态。
- **替换链（坍缩到提交窗口）**：含资源板块在已有作品上重执行即替换——软删所选板块旧 store 并登记受害者清单发生在提交点序列首步（全部暂存写满、提交开始才触碰旧 store；腾空 rename 目标：已完成行移文件入 backup、未完成行废弃文件）。下载窗口零软删零登记——失败/暂停/停止于下载时旧 store 一动不动、暂存保留，无回滚需求。提交序列失败时经 Fail 上报由控制面 setFailed 单点按清单复活受害者（同一调用栈同步触发）；成功则 Finish 清登记（软删行进入被替换终态）。恢复会话在作品定位后置替换位（暂停期间软删未发生，提交点补位软删）。会话失败收口只负责关闭流写入句柄释放文件锁，复活动作全部收在控制面单点。
- **提交点**（全部暂存写满后单点，不随执行 ctx 中断——暂停/停止落进窗口时序列走完不留半提交态）：替换软删（首步）→ 逐轨 rename 暂存→最终路径（同卷原子；store/ 白名单内操作，rename 前登记 `storeRegistry.Suppress`、序列结束统一 Release——防 rename→建行窗口内 fsmonitor 误裁决）→ 单事务建 persistent_store 完整行（completed_at 即时置位+宽高/头指纹+哈希双列，经 `StoreCommitter.CommitStore`）+ resource_store 挂载 + Resource Save（find-or-create）+ pending 直写 → 暂存目录回收。失败补偿=序列内逆操作（已 rename 轨逆 rename 回退暂存；软删受害者复活经 Fail 上报由 setFailed 单点同步触发；建行段事务原子性兜底），同步完成不跨会话。
- **完整性哈希**：spec 声明 `ExpectedSha256` 时暂存写满后比对，不符任务 Fail「资源完整性校验失败（role）：来源声明的哈希与下载内容不符」+暂存保留；未声明跳过。实测哈希恒边写边算，提交点建行落 `actual_sha256` 列（双列语义：expected=来源声明、actual=实测）。
- **续传（暂存锚）**：暂停/崩溃后恢复——枚举暂存 role_seq 键文件 stat 得各轨偏移 → Resume RPC StreamOffsets 全量下发 → 返回 spec 按角色配对暂存轨队列消费全局 seq；认领的 downloaded 轨按偏移续接（`spec.ResumeWriteOffset` 插件指定优先，前缀读入哈希器保证跨会话实测哈希一致；暂存残留超声明大小截断重下），未认领轨（derived 一次性产物未完成须整轨重产等）经 Start(role) 整轨重产。任务所属作品按领域复合键 (site, site_work_id) 定位（暂存模式暂停任务无 pending/资源行）。
- **软暂停**：控制面进入软暂停时 close 广播通道，copyLoop 完成当前在途读取并落盘后收尾退出（磁盘落点对齐真实中断点，供 Range 续传）；排空超时（控制面 2s 兜底）强制取消退化为有损暂停。

## 依赖关系

- 依赖（`Deps`，接口由本模块定义、提供方实现、app.go 装配）：插件执行器工厂（plugin 扩展桥）、作品信息保存/作品复合键定位（work）、资源保存/关联写入/完整度重算（resource）、查重判定（duplicate）、站点键解析（site，查重输入键与目录段派生共用）、提交点建行（persistentStore）、替换链能力（resource）、工作目录（settings）、事务执行器、pending 直写、任务核心行查询（task）、暂存目录派生（task 基建适配）
- 被依赖：taskManager（plugin-download 执行面策略注册 + 重下载板块写行 `SectionRecorder` + 活跃插件计数投影 + work 删除链 pending 清理）、task（建树写行/树双查读行窄接口消费本模块仓储）

## 关键设计

- **无跨执行可变状态**：策略对象无状态，每次 Execute 自建执行会话（领域行快照/流集合/决策位随会话生灭）。
- **多 store 身份**：同 role 内 `store_seq` 唯一定位 store（落盘文件名键/暂存文件名键/续传配对同维度）；resume 的 specs 是未完成子集，全局 seq 从暂存枚举（或全新执行的 specs 全集序）推导，不按 specs 内重计（防部分完成时错位覆盖）。文件名恒带 role_seq 段（单 store 资源不省略），派生规则见 `doc/store-naming-convention.md`。
- **失败终态清 pending**：失败任务不续传（残留 pending 在作品/资源被外部删除或还原后指向失效对象），Fail 上报前清理；暂停保留 pending 供恢复续传定位。
