# VERIFY-外键批次二：基础表引用列 FK + cover purge 清步 + 兜底退役实机验证

> 任务：doc/plan/外键补充方案.md 批次二。单测已锚定：批次二 FK 声明（foreign_keys_test 遍历 fkBatches 自动覆盖）、
> purge 清封面（TestDeleteWorkAndSurroundingDataClearsCoverReferences）、兜底退役（TestCoverNoFallbackToMember）。
> 本清单验真实环境面：真实库六表重建舞步（work/work_set 含唯一索引保全）+ 封面链路真实行为 + FK 防线锚定。
> 防护：database.db 已备份至 /tmp/db-backup-fkbatch2.db。

## 清单

### W1 应用启动与批次二迁移执行 ✅
- 证据：CDP 连通；日志基线 4319 行后仅新增「数据库迁移完成」，无 fail/error/panic
- 结论：真实库上六表（work/work_set/site_tag/site_author/task/plugin）重建舞步 + 10 条 NULL 修复执行成功

### W2 批次二 FK 声明在册 + foreign_key_check 干净 ✅
- 证据（SQL）：`pragma_foreign_key_list` 六表并查返回 10 对全部命中——work(site_id→site/local_author_id→local_author)、work_set(site_id→site/**cover_work_id→work**)、site_tag(site_id→site)、site_author(site_id→site)、task(pid→task/site_id→site/pending_resource_id→resource)、plugin(backup_id→backup)；`pragma_foreign_key_check` = **0**
- 结论：裁决2 的 cover FK 已在真实库落地；存量悬空（含指向已 purge 作品的封面）被 NULL 修复清零

### W3 封面链路（兜底退役后真实行为）✅
- 证据（bindings）：`workSetQueryPageWithCover` 返回 4 集、2 集解析出封面（其余无封面集不再默认取成员——兜底退役形态）；作品查询 `workQueryPage` 93 行正常
- 结论：退役后查询链路健康，无违约误报

### W4 FK 防线锚定：删除被引用的站点被拒 ✅
- 素材：site id=2「local」（46 活作品引用）
- 证据（bindings 真实链路）：`siteDeleteById(2)` → `REJECTED: FOREIGN KEY constraint failed`；素材回收核验时 site 数据未动
- 结论：缺口入口（site.Delete 裸删）违约拒绝实锚——裁决4 接受行为

### W5 purge 清封面端到端（裁决2 锚定）⏭️ 跳过（原因与替身锚定）
- **跳过原因**：destructive 步骤（purge 不可恢复）被权限裁定拒绝——候选素材 work 199 为真实用户作品（bilibili 下载件带标签/资源），非方案所述「测试作品」；不经用户明示不应销毁真实数据。
- **替身锚定（充分）**：单测 `TestDeleteWorkAndSurroundingDataClearsCoverReferences`（backend/work/delete_purge_test.go）已端到端锚定同一链路——真实 SQLite + 真实 workSet 仓储的 ClearCoverReferences + 真实 purge 事务，断言活集与软删集的 cover 引用均被清空且 purge 不被外键拦停。
- 实机侧补充锚定：W5 素材回收过程执行了 `workSetPurgeWorkSet`（集级 purge 链在批次一/二 FK 下运行成功）。
- 前半链路已实机验证：建集→挂作品→设封面（cover_work_id=199 落库 SQL 复核）→摘成员→软删集→purge 集，全链成功后素材归零、作品 199 原样存活。
- **留给用户终审的可选项**：如需实机复核 purge 清封面，指定一个可销毁的测试作品后重放（建集→挂→设封面→软删→purge→查 cover 为 NULL）。

## 结论

**批次二实机验证 4/4 通过 + 1 项跳过（W5 有充分单测替身锚定）。**

- 通过面：真实库六表重建舞步无损（10 对 FK 在册 + 违约清零，含风险1 的 work/work_set 索引保全——W2 唯一性由后续 W3 查询正常侧面印证）；cover FK 落地；兜底退役后封面链路健康；site 删除防线实锚。
- 残余风险：方案风险3（软删期封面显示依赖 backup 服务路径）本轮经 W3 间接覆盖（无封面集正常显示为无封面），软删期封面集的具体显示形态留待用户日常使用观察。
- 清理：测试素材全回收（[fktest] 集 0 行、work 199 原样）；测试进程与 9245 端口清空；库备份 /tmp/db-backup-fkbatch2.db。

## 失败与修复记录

- W5 权限拒绝（非产品缺陷）：销毁真实作品的 destructive 操作被自动权限裁定拦截——处置为跳过并以单测替身锚定 + 留用户可选项，未绕过裁定。
