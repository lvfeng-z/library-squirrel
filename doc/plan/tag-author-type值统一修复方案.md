# tag_type / author_type 值体系统一修复方案（节点 C，W2 bug）

> **状态：已完成（2026-08-06）**。代码改 + 数据迁移 + 校验全通过：`re_work_tag.tag_type` / `re_work_author.author_type` 值域 {0,1}（tag 7+355、author 7+49），recycle_bin 45 条 snapshot 迁移、无旧值 2 残留（mcp sqlite 双校验）。
>
> 修复 W2 确凿 bug：`re_work_tag.tag_type` / `re_work_author.author_type` 两套值体系冲突。统一到 `constant.OriginType`(LOCAL=0/SITE=1)。
> 涉及数据迁移（不可逆），本方案先落 plan + 审查，再实施。

## 审查摘要

**关键声明（抽查项）**：
- 声明1：两套值体系冲突——`constant.OriginType`(LOCAL=0/SITE=1, base 正统) vs `work/service.go` `AuthorTypeLocal=1/AuthorTypeSite=2`(:1330-1331) + tag 字面量 1/2(:2042,:2069) — `backend/base/constant/OriginType.go:4-5`、`backend/work/service.go:1330-1331,2042,2069`
- 声明2：修复**仅改 `work/service.go`**（4 处写值 :2027/:2042/:2054/:2069 + 2 处读分支 :896/:901 + 删 AuthorType 常量块 :1328-1332 + 新增 import constant）；`reWorkTag`/`reWorkAuthor`/`recycleBin` 模块、前端、SDK **全不改** — 审计：reWorkTag 已用 constant、reWorkAuthor 按.ID 列判别不读 author_type、recycleBin 透传不解释、前端 `OriginType.ts` 已 0/1、SDK WorkResponse 按数组隐式分类无类型字段
- 声明3：`re_work_author.author_type` **无歧义**（唯一写入=入库 1/2，无前端写入入口），迁移规则 `1→0(Local) / 2→1(Site)` — 审计第 3 节（reWorkAuthor Handler 无 Link/Unlink）
- 声明4：`re_work_tag.tag_type` 代码路径值不一致（前端 Link/Unlink 走 constant 0/1、入库走字面量 1/2），tag_type=1 在两路径语义相反（前端=SITE/入库=Local）=潜在风险；**当前 DB 实测全 1/2 入库体系且 ID 列自洽（tag_type=1×7 全 local_tag_id>0、tag_type=2×355 全 site_tag_id>0，无 0），无实际歧义数据**；migration 用 ID 列消歧（`local_tag_id>0⇒LOCAL(0)`/`site_tag_id>0⇒SITE(1)`，鲁棒应对含前端数据的库），当前库执行结果=1→0/2→1
- 声明5：`recycle_bin.snapshot`（text 列存完整快照 JSON，`entity/recycle_bin.go:20`）携带 `authorType`(旧全 1/2)/`tagType`(1/2)；**当前 59 条存量**；作者还原代码 :896/:901 改比较 constant(0/1) 后，旧 snapshot(1/2) 两分支都不进 → **还原丢作者**，必须处理
- 声明6：SDK `WorkResponse` 把作者/标签拆四个独立数组（localAuthors/siteAuthors/localTags/siteTags），类型由数组归属隐式表达，DTO 无类型字段 — `library-squirrel-sdk/proto/plugin.proto:145-148`；修复不涉及 SDK

**决策已定**：
- 决策1 → **迁移存量 snapshot**：脚本遍历 recycle_bin 59 条，解析 snapshot JSON 改 authorType(1→0/2→1)+tagType(按 ID 消歧)，写回。彻底，不留兼容技术债
- 决策2 → **独立一次性脚本**（Go，连 database.db，事务迁移 + snapshot JSON 改写 + 校验），手动跑迁当前开发库；发布版 fresh 库不需迁移（代码已改对，新数据全 0/1）

**自曝风险**：
- 风险1：数据迁移**不可逆**；消歧规则（ID 列判别）若与实际写入不符会损坏标签关联。需迁移前备份 DB + 迁移后 `SELECT DISTINCT` 校验值域 {0,1}。
- 风险2：snapshot JSON 迁移（决策1 选 a）需解析 JSON 改值，结构异常会损坏 snapshot；需先确认回收站是否有存量（开发期常为空，空则无成本）。
- 风险3：migration 幂等判断须可靠——用"值域是否仍含 2（author）或歧义 1（tag）"作未迁移标志；迁移后值域 {0,1} 自然幂等。误判重复跑会二次损坏。
- 风险4：代码改动与数据迁移**同次部署**；migration 须在业务代码前执行（启动顺序已是 migration→业务），无并发实例则无窗口。

---

## 1. 根因

两套值体系并存，写入同一组列：
- `constant.OriginType`（base 层正统）：LOCAL=0/SITE=1。`reWorkTag` 模块（前端 Link/Unlink 唯一入口）+ 前端 `OriginType.ts` 用它。
- `work/service.go` 入库路径：自造 `AuthorTypeLocal=1/AuthorTypeSite=2`（注释谎称"与 reWorkTag.TagType 对齐"）+ tag 裸字面量 1/2。完全没引用 constant 包。

冲突后果：`tag_type=1` 在前端路径=SITE、入库路径=Local（语义相反）；`author_type` 全表 1/2（仅入库写入）但前端/reWorkAuthor 体系是 0/1。

## 2. 影响面（审计结论，全在 work/service.go + DB + snapshot）

**代码改动**（仅 `backend/work/service.go`）：
| 行 | 当前 | 改为 |
|---|---|---|
| :2027 写 author | `AuthorType: {Int64: AuthorTypeSite}`(2) | `constant.SITE`(1) |
| :2042 写 tag | `TagType: {Int64: 2}` | `constant.SITE`(1) |
| :2054 写 author | `AuthorType: {Int64: AuthorTypeLocal}`(1) | `constant.LOCAL`(0) |
| :2069 写 tag | `TagType: {Int64: 1}` | `constant.LOCAL`(0) |
| :896 读 author | `== AuthorTypeLocal` | `== constant.LOCAL` |
| :901 读 author | `== AuthorTypeSite` | `== constant.SITE` |
| :1328-1332 | `AuthorTypeLocal/Site` 常量块 | 删除 |
| import | 无 | 加 `base/constant` |

**不需改**（审计确认）：reWorkTag 全模块（已 constant）、reWorkAuthor 全模块（按 ID 列，不读 type）、recycleBin 全模块（透传）、前端（已 0/1）、SDK（无类型字段）、snapshot 构建点(:704/:713)/还原透传点(:891/:920)（拷贝语义）、标签还原分支(:916-928)（不读 tag_type）。

**另（设计第七节 step 2/3 尾巴，补全 tag_type 清障）**：
- `reWorkTag/service.go` `LinkTagToWork` 未用 ID 列改随零值 NULL（原写 `{0,Valid:true}` 占位，与 `LinkBatchToWork` 的 `Valid:false` 不一致；统一消除占位歧义）。
- `doc/ai-assistant/glossary.md` 区分「work↔tag 关联 = re_work_tag」与「local↔site 桥接 = site_tag.local_tag_id」（原文把 local↔site 关联误指 re_work_tag）。

## 3. 数据迁移

**re_work_author.author_type（无歧义）**：
```sql
UPDATE re_work_author SET author_type = 0 WHERE author_type = 1;  -- Local
UPDATE re_work_author SET author_type = 1 WHERE author_type = 2;  -- Site
```
注意顺序：先 1→0 再 2→1（不可反过来，否则 1→2 与 2→1 撞）。或一条带 CASE：`SET author_type = author_type - 1 WHERE author_type IN (1,2)`。

**re_work_tag.tag_type（歧义，按 ID 列消歧）**：
```sql
-- 以哪个 ID 列有效为准（DB 唯一索引本就按 ID 列建）
UPDATE re_work_tag SET tag_type = 0 WHERE local_tag_id > 0;   -- LOCAL
UPDATE re_work_tag SET tag_type = 1 WHERE site_tag_id > 0;   -- SITE
-- 异常行（两列都>0 或都空）单独核对（按写入逻辑不应存在）
```

**recycle_bin.snapshot（决策1）**：见待决策。若选 (a)，migration 遍历 recycle_bin 表，解析 snapshot JSON，改 `authors[].authorType`(1→0/2→1) 与 `tags[].tagType`(按对应 ID 列消歧)，写回。

## 4. 实施步骤

1. **代码改动**：`work/service.go` 按第 2 节表改（4 写 + 2 读 + 删常量 + import）。
2. **数据迁移**（决策2 选 migration）：`migration/migrate.go` 在 AutoMigrate 前加幂等数据修复段——
   - 幂等判断：`SELECT count(*) FROM re_work_author WHERE author_type IN (1,2)` > 0 则未迁；
   - re_work_author：`author_type = author_type - 1 WHERE author_type IN (1,2)`；
   - re_work_tag：按 ID 列消歧 UPDATE；
   - recycle_bin snapshot（决策1 选 a）：遍历 + JSON 改写。
3. **编译 + 单测**：`go build ./...` + taskManager/work 相关测试。
4. **迁移校验**：`SELECT DISTINCT tag_type FROM re_work_tag`（应 {0,1}）、`SELECT DISTINCT author_type FROM re_work_author`（应 {0,1}）。

## 5. 验证
- 编译通过（主程序；SDK/插件零改动）。
- 数据迁移幂等：连跑两次，第二次无变化。
- 端到端：创建任务入库（站点/本地标签+作者）→ 查 DB tag_type/author_type 为 0/1；前端 Link/Unlink 标签 → 查 DB 为 0/1；回收站删除+还原 → 作者关联不丢。
- 历史数据：迁移后值域 {0,1}，无 1/2 残留。

## 6. 决策1 备选（若不选 a）
若选 (b) 还原 fallback：`work/service.go:896/:901` 改为兼容旧值——`if authorType == constant.LOCAL || authorType == 1 /*旧 Local*/` 视为 LOCAL，`== constant.SITE || authorType == 2 /*旧 Site*/` 视为 SITE。简单但留兼容代码（技术债，未来清理需追踪 snapshot 是否全迁）。
