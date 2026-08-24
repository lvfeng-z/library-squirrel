# VERIFY-外键批次一：re_* 四表外键声明与强制执行实机验证

> 任务：doc/plan/外键补充方案.md 批次一（DSN 翻转 + OpenTestDB + re_* 四表 FK + 存量清理）。
> 单测已锚定：声明/违约写入/删除防线/幂等/索引复原/foreign_key_check（backend/migration/foreign_keys_test.go 全绿）。
> 本清单只验单测覆盖不了的真实环境面：真实 dev 库迁移舞步 + 真实前端链路行为。
> 防护：database.db 已备份至 /tmp/db-backup-fkbatch1.db。

## 基建配方

分层启动：vite(9245) → 测试二进制（CGO，改过 Go 代码已重建）→ LS_CDP_PORT=9222 直启。
断言手段：只读 SQL（sqlite3 MCP）／CDP bindings（真实前端链路）／日志 grep／截图。

## 清单

### V1 应用启动与真实库迁移执行 ✅
- 断言证据：CDP 连通（Edg/151.0.4129.101）；log/server.log 基线 4241 行后仅新增 `{"msg":"数据库迁移完成"}`，无 fail/error/panic
- 结论：真实 dev 库上悬空清理 + 四表重建舞步执行成功，应用存活

### V2 四表 FK 声明在册（真实库终态） ✅
- 断言证据：`pragma_foreign_key_list` 四表并查返回 **10 对**全部命中——re_work_tag(work_id→work/local_tag_id→local_tag/site_tag_id→site_tag)、re_work_author(work_id→work/local_author_id→local_author/site_author_id→site_author)、re_work_work_set(work_id→work/work_set_id→work_set)、re_work_set_work_set(parent/child_work_set_id→work_set)
- 结论：声明面与方案登记一致

### V3 foreign_key_check 干净（存量清理效果） ✅
- 断言证据：`SELECT COUNT(*) FROM pragma_foreign_key_check()` = **0**
- 结论：真实库无外键违约存量（清理迁移生效且无遗留）

### V4 FK 强制执行锚定：删除被引用的本地标签被拒 ✅
- 素材：local_tag id=1「test」（33 处 re_work_tag 引用）
- 断言证据：CDP 真实链路 `localTagDeleteById(1)` → requireResponse 抛 `Error: FOREIGN KEY constraint failed`（前端 wrapper 层完整传播）；SQL 复核 id=1 仍在
- 结论：缺口入口（localTag.Delete 裸删）在 FK 强制下响亮报错——裁决4 接受的行为实锚

### V5 写入合法路径冒烟：作品挂本地标签成功 ✅
- 素材：新建 `[fktest]批次一验证` 标签（经 localTagSave 得 id=14）
- 断言证据：`reWorkTagLink(199, 0, [14])` 返回 success；SQL 复核 re_work_tag id=1933 (work_id=199, local_tag_id=14) 在库
- 结论：FK 开启下合法关联写入畅通

### V6 作品软删 → 回收站 → 复原（主链路无违约误报） ✅
- 素材：作品 199「大肥鱼也会有烦恼吧」
- 断言证据：workSoftDelete(199) success → recycleBinPageWorks 含 199 → recycleBinRestoreWork(199,false) success(data=199)；SQL 终态：work.deleted_at=0（复活）、re 行 1933 全程保留、work 199 关联 6 条（原 5 + 测试 1）
- 结论：软删/复原链在新 FK 环境零违约误报

### 附加对照断言（素材回收时顺带） ✅
- `reWorkTagUnlink(199,0,[14])` + `localTagDeleteById(14)` 均成功——**无引用标签删除放行**，FK 防线不误伤合法操作

## 结论

**批次一实机验证全过（V1-V6 + 对照，7/7）。**

- 通过面：真实库迁移舞步无损落地（10 对 FK 在册 + foreign_key_check 零违约）；三类防线中「删除侧漏步骤」已实际生效（V4 实锚：缺口入口报错而非静默悬空）；写入合法路径与软删/复原主链路零误伤。
- 证据摘要：见各清单项（日志行、SQL 返回值、CDP 链路返回）。
- 残余风险：其余七个缺口入口（localAuthor/siteTag/siteAuthor/site/task 删除与 PurgeStore）同 V4 形态会违约报错，待缺口方案落地解除——裁决4 既定接受项。
- 清理：测试素材全回收（[fktest] 标签 0 行、关联 0 行）；测试应用与 vite 进程已杀净（9245 端口空）；库备份保留于 /tmp/db-backup-fkbatch1.db 供回滚。
- 报告勘误：清单初稿「11 对」为笔误，实际登记面 10 对（3+3+2+2）。

## 失败与修复记录

无失败项。
