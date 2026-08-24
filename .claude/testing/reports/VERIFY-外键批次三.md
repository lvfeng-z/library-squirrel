# VERIFY-外键批次三：恒有值族 FK（resource/resource_store/plugin_storage）实机验证

> 任务：doc/plan/外键补充方案.md 批次三。单测已锚定：FK 声明遍历覆盖、fixture 真实化（purge mock 改真实删行 + dbFromCtx）。
> 本清单验真实环境面：真实库三表重建 + **112/113 历史孤儿清理** + 主链冒烟。
> 防护：database.db 已备份至 /tmp/db-backup-fkbatch3.db。

## 清单

### X1 应用启动与批次三迁移执行 ✅
- 证据：CDP 连通；日志基线 4413 行后仅新增「数据库迁移完成」无报错

### X2 批次三 FK 声明在册 ✅
- 证据（SQL）：resource(work_id→work)、resource_store(resource_id→resource + store_id→persistent_store)、
  plugin_storage(plugin_id→plugin) 4 对全在册

### X3 历史孤儿清理（本批独有验证点）✅
- 迁移前实测：resource_store 悬空 resource 引用 **112 行**、悬空 store 引用 **113 行**（手动删表史遗留）
- 迁移后证据（SQL）：悬空 resource 引用 **0**、`pragma_foreign_key_check` = **0**、resource_store 总行 208（清理后存量）
- 结论：清理迁移在真实库精确清掉历史孤儿，无误伤（总数 208 仍为有效行）

### X4 主链冒烟（挂载链查询）✅
- 证据（bindings）：workQueryPage 93 行正常；workGetFullWorkInfoById(1) 返回 12 资源（resource→resource_store→persistent_store 三表 JOIN 链健康）

## 结论

**批次三实机验证 4/4 通过。** 历史孤儿（resource_store 112/113 悬空行）在真实库被清理迁移精确清除；恒有值族 4 对外键落地；主链查询零违约误报。
清理：测试进程与 9245 端口已清空；库备份 /tmp/db-backup-fkbatch3.db。

## 失败与修复记录

（实施期）work 包测试一度死锁超时——purge mock 在事务内误用根连接（MaxOpenConns=1 经典死锁），改 `database.DBFromContext` 从 ctx 取事务连接后修复；全表计数断言被 fixture 种子行污染，改按 workId 圈定。
