# backupGovernance 模块说明

## 一句话职责

对 backup 保管清单做双向对账治理：正向清理无主备份（清单行不被任何业务列引用且超保留期——兜底替换/合并/崩溃中断等零清理链路造成的目录膨胀），反向清除悬空引用（业务列引用的清单行已不存在），监视哨按引用方统计引用年龄暴露「登记了但无终态清理路径」的生命周期失效。

## 边界

- 与 **backup**：backup 是纯叶子能力包（只记保管位置与时间）；本模块是第三方编排者（ORCHESTRATION_BY_CALLER），经 `BackupCatalog` 接口消费其清单查询与删除能力，backup 不感知治理与引用方。
- 与 **persistentStore / plugin**：两者以 `BackupReferencer` 身份提供各自的引用集与清列能力（接口由本模块定义、提供方实现，SERVICE_DEPENDENCY_VIA_INTERFACE）；本模块不直接读写他模块表。
- 定位对齐 **fsmonitor**：横切维护域，纯 DB 对账（清单行存在性为限）——不做备份文件存在性校验（外部文件变更感知属 fsmonitor 域）。

## 对外接口

Wails Handler（备份管理面板 `views/BackupManage.vue` 消费，「备份与回收」二级菜单）：

- `PageBackups(page, referenced *bool)`：分页查保管清单（create_time 倒序），逐行标注有主/无主并按引用态过滤（nil=全部）；fileSize 逐行 os.Stat
- `DeleteBackups(ids)`：批量删除——任一 id ∈ 当前引用集即整体拒绝（错误消息指明有主行）；单行删除不限年龄，「清理全部无主」的圈定走 GetBackupStats 的 expiredOrphanIds（超保留期）
- `RunReconciliationNow()`：手动触发一轮双向对账（复用 RunOnce，runMu 与定时巡检互斥），返回清理统计
- `GetBackupStats()`：占用统计（总占用/有主·无主拆分/按引用方分组=监视哨同源/无主超期圈定）；全量 Stat 大库可达秒级，30s TTL 缓存，删除/巡检后失效

引用态数据面与删除守卫的引用集均走 `collectReferencedStrict`——与对账同一 BackupReferencer 枚举同源，任一引用方查询失败即报错熔断（失败方引用呈现为零 = 误标误删）。

被调用方：

- app 启动序列 `Start()` / 退出 `Stop()`（后台 goroutine：启动即巡检一次 + 每 24h）
- `RunOnce(ctx)`：单轮对账（返回清理统计；与定时巡检经 runMu 串行）
- `ClearBackupRefs(ctx, ids)`：按 ID 即时清除各引用方对清单行的引用（实现 fsmonitor.BackupRefCleaner——fsmonitor backup 域确认删除清单行后联动调用，令回收站可复原状态即时准确；单方失败 Warn，残余悬空由既有反向对账兜底）

## 核心概念

- **双向对账**：反向（悬空清列，防御性先行）——引用集 ∖ 现存清单行 = 悬空，逐引用方清列；正向（无主清理）——`ListCreatedBefore(now − 保留期)` 中不被引用集覆盖的清单行逐个删除。两向无数据依赖。
- **引用方枚举**（`BackupReferencer`）：persistent_store.backup_id（含已删行——软删行是合法引用者）、plugin.BackupID（含已卸载行——持有重装能力引用）。开放集合，登记唯一落点在 app.go 装配处。
- **监视哨**：按引用方分组统计引用数量/占用/最老引用年龄；最老年龄超 90 天（回收站默认保留期 30 天的 3 倍）记 Warn——有主侧生命周期失效时年龄曲线单调上升，必然可见。合法长寿命引用（已卸载插件重装包）会周期性命中 Warn，接受为日志级噪音。
- **保留期**（settings `backupGovernance.retentionDays`，默认 7 天，常开无开关）：正确性参数而非卫生参数——替换任务在途期间其还原点备份合法地零业务引用（内存清单/无清单），保留期垫住该窗口；新备份「先建行后写引用」的写入窗口内 create_time 距今 << 保留期，不进正向候选。

## 依赖关系

- 依赖：backup（BackupCatalog）、persistentStore 与 plugin（BackupReferencer，经 app.go 注入的枚举切片）、settings（RetentionDaysProvider）
- 被依赖：app（启动/退出挂钩 + Handler 装配）、前端备份管理页（Handler 四方法）、fsmonitor（BackupRefCleaner：backup 域确认删行后的即时清列联动，经 app.go 闭包适配注入——装配次序上治理服务晚于 fsmonitor 创建）

## 关键设计

- **引用集查询失败熔断**：任一引用方 `ListReferencedBackupIDs` 失败（哨兵 nil 标记）时本轮正向清理整体跳过——该方引用呈现为零，进候选即误清活备份。
- **非法态防御清列**：`IllegalBackupRefSanitizer` 可选接口（persistentStore 实现）——活行携带备份引用构造上不可达（backup_id 与 deleted_at 单条 UPDATE 同生共死），检出即外部直改数据库痕迹。
- **无主清理不可逆**：`DeleteBackup` 直接删文件与清单行（无隔离区/软删缓冲），对齐其既有语义。
