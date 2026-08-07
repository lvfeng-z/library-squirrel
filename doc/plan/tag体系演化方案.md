# tag 体系演化方案（namespace 关联级属性）

> 本文档是任务图中「tag 体系演化设计」设计节点的交付物。核心决策（已与用户确认）：**namespace 是 work↔tag 关联级属性，不是 tag 身份的一部分**（方向 b）。tag 实体的统一身份保持扁平；namespace 作为关联维度，local/site 关联通用，无 namespace 时为 null。本文给出 schema 改造、插件声明面扩展、搜索改造与 tag_type 清障步骤，供后续实施节点执行。

## 审查摘要

**关键声明（抽查项）**：

- 声明1：`local_tag` 字段为 LocalTagName / BaseLocalTagID（层级，0=根）/ Description / LastUse，**扁平、无 namespace** — 本方案保持不变 — `backend/base/model/entity/local_tag.go:10-16`
- 声明2：`site_tag` 字段含 SiteID+SiteTagID（复合唯一 `idx_site_tag_site_site_tag`）、SiteTagName、BaseSiteTagID（孤儿字段，未被插件填充，本方案不涉及）、LocalTagID（site→local 桥接外键）、Description、LastUse，无 namespace — 本方案加 nullable namespace 列 — `entity/site_tag.go:10-19`
- 声明3：`re_work_tag` 结构为 WorkID + TagType（local/site 来源）+ LocalTagID + SiteTagID（两 ID 二选一）— 本方案加 nullable namespace 列 — `entity/re_work_tag.go:10-16`
- 声明4：跨站点统一搜索在 `search/repository.go:390-403` 用 `rwt.local_tag_id = ? OR st.local_tag_id = ?` 实现（扁平 by name）— 本方案在此基础上加 re_work_tag.namespace 过滤 — `backend/search/repository.go:390-403`
- 声明5：`re_work_tag.tag_type` 值不一致——插件导入路径写 1/2（SearchType，`work/service.go:1935,1962`），用户 UI 路径写 0/1（OriginType，`reWorkTag`），查询不读该列故未爆 — `work/service.go:1935,1962`、`reWorkTag/service.go`、`backend/base/constant/OriginType.go:3-6`
- 声明6：插件声明面 `TaskSiteTagDTO` 仅 SiteTagID/TagName/Description — 本方案加 Namespace 字段 — `library-squirrel-sdk/dto/handler_dto.go:128-133`
- 声明7：作者与 tag 是两套并行体系，author 带富字段（SiteAuthorID/AuthorName/FixedAuthorName/Introduce/Homepage，见 TaskSiteAuthorDTO）— author 保持独立，不入 tag namespace — `library-squirrel-sdk/dto/handler_dto.go:119-126`
- 声明8：e-hentai 用 `namespace:tagname` 复合身份（female/male/parody/character/artist/language 等，性别即 namespace），同名不同 namespace 为不同 tag — 但 e-hentai 是单站点、namespace 体系一致；本项目是多站点聚合、多数站点无 namespace，故不照搬其复合身份模型 — 来源见文末

**待决策（需用户拍板）**：

- 决策1（已决）：namespace 取值 = **开放字符串 + 内置已知集**；未知 namespace 前端兜底渲染（见风险1）。
- 决策2（已决）：内置 namespace 集合 = `language`/`character`/`parody`/`female`/`male`/`misc`/`general`（artist 排除，归 author 体系）。
- 决策3（已决）：双存一致性 = link 时写镜像后**不再自动同步**；site_tag.namespace 事后修正不回填历史 re_work_tag（历史关联保留其链接时的 namespace，避免批量回填代价与语义歧义）。

**自曝风险**：

- 风险1：namespace 双存（site_tag + re_work_tag 镜像）有一致性维护成本——决策3 须明确，否则 site_tag.namespace 修正后与历史 re_work_tag.namespace 漂移。
- 风险2：per-namespace 搜索依赖 `re_work_tag.namespace` 索引；大量历史数据 namespace=null（namespace-less 站点 + 历史数据）时索引选择性差，需评估查询计划。
- 风险3：用户本地补充 namespace 无正确性约束（可能误标 character/parody），仅靠用户自觉。
- 风险4：e-hentai `artist:` namespace → author 体系的映射规则须明确（哪些 namespace 进 author、哪些进 tag 池），规则错配丢信息。
- 风险5：character/parody 入 tag namespace（B1，薄元数据）；若将来要富字段（角色头像/原作简介）须迁移为独立体系，届时 namespace 方案需调整。
- 风险6：本方案给 site_tag/re_work_tag 加列 + tag_type 清障，属结构性迁移，须配迁移脚本 + 回滚预案。

---

## 一、核心决策（方向 b）

**namespace 是 work↔tag 关联级属性，不是 tag 身份的一部分。**

- tag 实体身份保持**扁平**：local_tag（统一概念）按 name，site_tag 按站点原生 id。namespace 不进入 tag 的唯一键。
- namespace 挂在 **re_work_tag（关联）** 上，作为「这条 work↔tag 关联属于哪个 namespace」的维度；site_tag 也记录站点侧的 namespace（站点已知，如 e-hentai character:alice）。
- local/site 关联**通用**：站点来源的 namespace 取自站点；本地（用户手加）来源的 namespace 由用户自设（可 null）。
- 无 namespace 的站点（pixiv 等）：site_tag.namespace = null，re_work_tag.namespace = null，零别扭。

## 二、为何选 b（而非 a）

| | a·namespace 进 tag 身份 | **b·namespace 关联级属性（选定）** |
|---|---|---|
| local_tag（统一概念） | 重键为 (namespace,name)，所有 tag 重新身份化 | **保持扁平，不变** |
| 无 namespace 站点 | 强制默认 namespace（别扭） | null，自然 |
| 一个 tag word 多 namespace | 不同 tag 行共享字符串 | 一个 tag word 多 namespace 用法（关联级） |
| 对现有模型改动 | 大（重键 + 迁移） | **小（加列，local_tag 不变）** |
| 跨站点 per-namespace 搜索 | 按 (namespace,name) local_tag 直查 | 扁平 tag word + re_work_tag.namespace 过滤 |
| 本地补充 namespace | 须建对应 (namespace,name) local_tag | 直接在 re_work_tag 设 namespace |

e-hentai 用复合身份（声明8），但它是单站点、namespace 一致；本项目多站点、多数无 namespace，照搬复合身份会逼所有站点填 namespace。b 让 namespace 成为可选维度，更贴合多站点现实，且对现有扁平模型改动最小。

**b 的唯一取舍**：无法把「character alice」绑成独立于「parody alice」的统一 local 概念（二者共享扁平 local "alice"）。但 per-namespace 搜索通过 re_work_tag.namespace 过滤仍可达（搜 alice + 过滤 namespace=character 命中所有站点 character:alice 作品）。判断：用户更可能「搜 + 过滤」而非「绑两个独立统一标签」，损失可接受。

## 三、schema 改造

### local_tag — 不变
扁平（name + base_local_tag_id 层级 + description + last_use），无 namespace。统一概念保持扁平是 b 的核心。

### site_tag — 加 nullable `namespace` 列
- 新增 `namespace`（sql.NullString）。
- 记录站点侧的 namespace（e-hentai character:alice → namespace=character；pixiv → null）。
- 唯一键 `(site_id, site_tag_id)` 不变（site_tag_id 为站点原生 id；e-hentai 可用 `ns:name` 复合或纯 name，由插件决定，唯一性靠 site_tag_id）。
- 历史数据：namespace = null（新增列，前向填充）。

### re_work_tag — 加 nullable `namespace` 列
- 新增 `namespace`（sql.NullString）。
- **local/site 关联通用**：site 关联 = 所指 site_tag 的 namespace 镜像；local 关联 = 用户自设或 null。
- 始终存储（适度冗余），per-namespace 搜索直接过滤 `re_work_tag.namespace`，不必 join site_tag。
- **一致性策略（决策3）**：site 关联 link 时写入 site_tag.namespace 镜像，此后**不再自动同步**——site_tag.namespace 事后修正不回填历史 re_work_tag（历史关联保留链接时的 namespace）。
- 历史数据：namespace = null。
- tag_type 清障见第七节（与本列正交）。

### 双存职责分离（避免误解）

`site_tag.namespace` 与 `re_work_tag.namespace` 双存，但职责不同——**namespace 维度的关联/搜索只走 `re_work_tag.namespace`**：

| 列 | 职责 | 参与关联/搜索维度？ |
|---|---|---|
| `re_work_tag.namespace` | 关联级 namespace——每条作品↔tag 关联的 namespace | ✅ 唯一参与者（搜索过滤 `rwt.namespace`、关联维度都看它） |
| `site_tag.namespace` | 站点侧元数据——记录 site_tag 在站点的固定 namespace | ❌ 不直接参与。用途：候选联想展示（让用户看到 site 标签的 namespace）+ 入库镜像源 |

- **搜索不读 site_tag.namespace**：`buildWhereClause` 的 LocalTag/SiteTag 两路过滤的都是 `rwt.namespace`（re_work_tag），即使 SiteTag 搜索也用 re_work_tag.namespace（镜像值），不 join site_tag。
- **site 关联的 namespace 维度经镜像实现**：site 关联要进入 namespace 维度，必须在 link 时把 `site_tag.namespace` 拷进 `re_work_tag.namespace`（入库由 `buildSiteTagLinks` 做；手动 link 由 I 节点补），之后搜索只认 re_work_tag.namespace。
- **为什么分离**：搜索只认 re_work_tag.namespace 一处，语义清晰且不必 join site_tag；site_tag.namespace 作为站点 truth 的存放处，其变更不自动回填历史关联（决策3），避免批量同步代价与语义歧义。

## 四、namespace 模型

- **内置已知集**（决策2，UI 特殊渲染：过滤 chip / 配色 / 图标）：`language`、`character`、`parody`、`female`、`male`、`misc`、`general`。
- **开放性**（决策1，建议开放字符串 + 内置集）：未知 namespace 允许存在，前端兜底渲染。
- **性别即 namespace**：female/male 本身是 namespace（如 `female:big_breasts`），无独立性别字段。
- **author/artist 不入 namespace**（声明7）：e-hentai `artist:` 在导入时映射到现有 author 体系（插件声明 site author），不进 tag 池。映射规则见风险4。

## 五、插件声明面扩展

`TaskSiteTagDTO`（声明6）新增 `Namespace`：
```go
type TaskSiteTagDTO struct {
    Namespace   string `json:"namespace"`  // tag 命名空间（language/character/parody/female/male/misc/general 等）；空=无 namespace
    SiteTagID   string `json:"siteTagId"`
    TagName     string `json:"tagName"`
    Description string `json:"description"`
}
```
- 插件解析站点原始 tag（e-hentai `female:big_breasts`）→ Namespace=female、TagName=big_breasts。
- `artist:` namespace：插件不声明为 tag，声明为 site author（TaskSiteAuthorDTO）。
- 落库：site_tag upsert 写 namespace；re_work_tag link 时写 namespace（镜像 site_tag.namespace）。

## 六、搜索改造

- **作品过滤**（声明4）：`buildWhereClause` 的 LocalTag/SiteTag 分支在现有 `rwt.local_tag_id = ? OR st.local_tag_id = ?` 基础上，按可选 namespace 追加 `AND rwt.namespace = ?`（per-namespace 过滤）。
- **候选页**：`QuerySearchConditionPage` 的 tag 联想结果携带 namespace（前端按 namespace 分组/着色，扩展 `SearchTagColorUtil.ts`）。
- namespace-less（null）的 tag：不指定 namespace 过滤时命中全部（含 null，保持现有扁平搜索行为）；指定某 namespace（如 character）过滤时**排除 null**（仅命中该 namespace 的关联）。

## 七、tag_type 清障（前置，与 namespace 列正交，先行）

现状（声明5）：两套写入路径用不同值（work 路径 1/2 vs reWorkTag 路径 0/1），查询不读该列故未爆。**审查补充**：两路径对「未用列」的占位写法也不一致——work 路径写 NULL、reWorkTag 的 LinkTagToWork 写 `0`（`sql.NullInt64{0,Valid:true}`，见 `reWorkTag/service.go:113-118`），故两列可能同时「非空」（一为合法 ID、一为 0）。

修复（用 `> 0` 判定，对 NULL 与 0 占位都正确）：
1. **数据迁移**：`UPDATE re_work_tag SET tag_type = CASE WHEN local_tag_id > 0 THEN 0 ELSE 1 END`（local_tag_id>0 → LOCAL=0，否则 SITE=1）。**关键：禁用 `IS NOT NULL`**——SITE 关联的未用列 local_tag_id=0（Valid:true）会被 `IS NOT NULL` 误判为非空 → 错判 LOCAL；`> 0` 对 NULL 与 0 占位均正确（ID 自 1 起，0/NULL 非合法值）。迁移前校验「无两列同时 >0」的脏数据。迁移为确定性重算（幂等），无需备份原 tag_type。
2. **写路径统一**：`work/service.go:1935,1962` 的 buildSiteTagLinks/buildLocalTagLinks 改用 `OriginType` 常量（LOCAL=0/SITE=1），删字面量 1/2；reWorkTag LinkTagToWork 对未用列统一写 NULL（而非 0），消除占位歧义。
3. **文档修正**：glossary.md 把 local↔site 关联表误写为 re_work_tag，改为 `site_tag.local_tag_id` 列。
4. **回滚**：tag_type UPDATE 幂等可重算（step 1 可重复执行）；namespace 加列为 nullable 新列，回滚 = DROP COLUMN（无历史数据损失）。

## 八、既有特性保留

- **跨站点桥接**：site_tag.local_tag_id 机制不变，桥接键仍扁平 by name（namespace 不参与桥接，统一概念保持扁平）。
- **base_local_tag_id 层级**：保留，与 namespace 正交。
- **lastUse**：不变。

## 九、实施顺序（I' 节点执行）

1. tag_type 清障（第七节：迁移 + 写路径统一 + 文档修正）——先行清地面。
2. schema 加列：site_tag + namespace、re_work_tag + namespace（nullable，历史 null）。
3. 插件声明面：TaskSiteTagDTO + Namespace（SDK）+ 主程序落库（site_tag upsert 写 namespace、re_work_tag link 写 namespace 镜像）。
4. 搜索改造（re_work_tag.namespace 过滤 + 候选页携带 namespace）。
5. 前端：tag 管理/搜索 UI 适配 namespace（过滤 chip、配色、未知 namespace 兜底、local 关联可选 namespace 选择器）。
6. pixiv/e-hentai 插件按需声明 namespace（e-hentai 解析 `ns:name`、artist 转声明 site author）。

---

**来源（e-hentai 核实）**：
- [ehwiki - Gallery Tagging](https://ehwiki.org/wiki/Gallery_Tagging)
- [ehDonate - eh搜索规则](https://github.com/kk9448/ehDonate/blob/main/eh搜索规则.md)
