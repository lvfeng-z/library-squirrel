# VERIFY-外键批次四：0 哨兵三列 NULL 化 + FK 实机验证

> 任务：doc/plan/外键补充方案.md 批次四（裁决3：persistent_store.backup_id / resource.task_id /
> local_tag.base_local_tag_id 三列 NULL 化纳入）。单测已锚定：类型/SQL 语义改造全量回归绿。
> 防护：database.db 已备份至 /tmp/db-backup-fkbatch4.db（保留）。

## 清单

### Y1 应用启动与批次四迁移执行 ✅（含一次实机抓真 bug）
- 首跑失败：`resource` 表二次重建报「table already exists」——**实机独有 bug**：批次三 RENAME 后
  SQLite 将 sqlite_master 存储的 DDL 规范化为双引号标识符，renameCreateTable 只匹配反引号导致
  新表名未替换。单测未捕（内存库首建即反引号、无 RENAME 归一化路径）。
- 修复：双引号风格兼容 + 已在册 FK 子句去重注入（附两回归单测）；事务回滚保证失败零残留、
  修复后二次启动幂等通过（11:49:18「数据库迁移完成」）。

### Y2 三 FK 在册 + 哨兵归一 + 违约清零 ✅
- 证据（SQL）：resource.task_id→task、persistent_store.backup_id→backup、
  local_tag.base_local_tag_id→local_tag 各 1 对在册；`pragma_foreign_key_check` = **0**；
  三列 `= 0` 残留全部为 **0 行**（0→NULL 迁移完成，含行内 backup 引用现状 0 行合法）

### Y3 主链冒烟（NULL 根标签树 + 回收站文件页）✅
- 证据（bindings）：localTagGetTree 返回 13 根 0 子（与库内真实形态一致——转换前即无子标签）；
  recycleBinPageStores 3 条（HasBackup 走 NULL 扫描路径正常）

### Y4 FK 防线锚定：删除被资源引用的任务被拒 ✅
- 素材：task id=1（被 resource id=1 引用）
- 证据（bindings 真实链路）：taskDelete([1]) → `REJECTED: FOREIGN KEY constraint failed`
- 结论：缺口6（task.DeleteTask 悬空 resource.task_id）已由 FK 强制——缺口方案落地前该入口持续报错（裁决4 接受）

## 结论

**批次四实机验证 4/4 通过（含一次实机独有 bug 的发现-修复-回归闭环）。**

- 至此外键补充方案四批全部落地：DSN 强制执行 + 全关系面（批次一~四共 25 对 FK）+ 存量清理/哨兵归一 + cover 裁决 + 兜底退役。
- 实机独有价值：RENAME 后 DDL 引号归一化是单测覆盖不到的真实库路径，本批实机验证捕获并修复。
- 清理：测试进程与 9245 端口清空；批次 1-3 库备份已删（远端无变更），批次四备份保留 /tmp/db-backup-fkbatch4.db。
- 残余：八个缺口删除入口（localTag/localAuthor/siteTag/siteAuthor/site/task/PurgeStore/localTag 子标签）在缺口方案落地前操作即报违约错——doc/plan/主数据删除关联清理缺口修复方案.md 待实施（其决策3/4 已由本方案裁决3 锁定 NULL 方向）。

## 失败与修复记录

- Y1 首跑迁移失败（resource 二次重建）：根因 = RENAME 后 DDL 双引号归一化 + FK 子句重复注入；修复 = renameCreateTable 双风格匹配 + injectForeignKeys 去重；回归测试 TestRenameCreateTableBothQuoteStyles / TestInjectForeignKeysSkipsExisting 锚定。
