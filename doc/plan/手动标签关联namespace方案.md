# 手动标签关联 namespace 补全方案（I 节点）

> 本文档是任务图「主程序主要决策定稿（发布前）」I 节点（reWorkTag.Link 路径不写 namespace 洞）的待实施方案。I 由 D（tag namespace 化）引出：namespace 体系落地后，手动给作品添加标签的路径（reWorkTag.Link）未写 namespace，与入库路径（buildSiteTagLinks 已写镜像）不对齐，导致用户无法手动添加带 namespace 的标签关联。当前 `[延后]`，本方案供将来实现。

## 审查摘要

**关键声明（锚点基于会话调研，实现时需复核代码当前状态）**：

- 声明1：手动加标签 API 无 namespace 通道——前端 wrapper `reWorkTagLink(workId, tagType, tagIds)` 无 namespace 参数（`frontend/src/apis/http/wrappers/reWorkTag.ts:11`）；后端 `Link` → `LinkBatchToWork` 构造 `ReWorkTag` 时未设 `Namespace`（`backend/reWorkTag/handler.go:21`、`service.go:131-150`）。
- 声明2：入库 site 关联已写 namespace（参考实现）——`buildSiteTagLinks` 镜像 `site_tag.namespace` 写 `ReWorkTag.Namespace = sql.NullString{String: ns, Valid: ns != ""}`（`backend/work/service.go:2037-2053`）。
- 声明3：手动加标签入口在两个作品对话框——`WorkDialog.vue:255-278`（handleTagExchangeConfirm → reWorkTagLink）、`WorkDetailDialog.vue:179-199`（同构），经 ExchangeBox 选标签。
- 声明4：wrapper 有 tagIds 截断 bug——`reWorkTag.ts:20` 只传 `[tagIds[0]]`，多 id 选择时其余静默丢弃。
- 声明5：入库 local 关联（buildLocalTagLinks）也不带 namespace——`work/service.go:2069-2079`（与 I 正交，但说明 local 关联 namespace 的写入历史性缺失）。

**待决策（需用户拍板）**：

- 决策1：site 手动添加的 namespace 来源——镜像 `site_tag.namespace`（站点固定，与入库一致，推荐）vs 允许用户覆盖？
- 决策2：local 关联的 namespace 选择器 UI 形态——ExchangeBox 确认流内加下拉（内置集 + 自定义输入），还是单 tag 选中后二级交互？

**自曝风险**：

- 风险1：local namespace 用户自设无正确性约束（可能 tag:alice + namespace:language 无意义组合）——开放性代价（tag体系演化方案 风险3），靠用户自觉。
- 风险2：site 手动 link 镜像 site_tag.namespace，依赖 site_tag 已入库（手动 link site 标签时 site_tag 应已存在）。
- 风险3：本方案锚点基于会话调研（非本次亲验），实现时需复核 reWorkTag/WorkDialog 代码当前状态（可能已演进）。

---

## 一、问题

namespace 体系（D）落地后，`re_work_tag.namespace` 列就绪，但 namespace 只在**入库流程的 site 关联**被写入（`buildSiteTagLinks` 镜像 site_tag.namespace）。**手动给作品添加标签**（WorkDialog/WorkDetailDialog → reWorkTagLink → reWorkTag.Link）这条路径完全不写 namespace：

- 后端 `LinkBatchToWork` 构造 `ReWorkTag` 时未设 `Namespace` 字段。
- 因此手动添加的标签关联（无论 local 还是 site）`re_work_tag.namespace` 都是 NULL。

后果：
- 手动添加的 local 标签不能带 namespace（用户无法给作品加「alice + character」关联）。
- 手动添加的 site 标签（即使 site_tag 有 namespace，如 e-hentai character:alice）也不镜像 namespace（与入库路径不一致）。
- namespace 搜索过滤（J 节点）对手动添加的标签关联无效（它们的 namespace 全 NULL）。

## 二、方案

### 后端：reWorkTag.Link 加 namespace 透传

1. **handler**（`reWorkTag/handler.go`）：`Link(ctx, tagType, tagIds, workId)` 加 `namespaces []string` 参数（与 tagIds 等长配对，按顺序对应）。
2. **service**（`reWorkTag/service.go`）：`LinkBatchToWork(ctx, workId, tagType, tagIds, namespaces)` 构造 `ReWorkTag` 时写 `Namespace`：
   - local 关联：`Namespace = sql.NullString{String: namespaces[i], Valid: namespaces[i] != ""}`（用户自设）。
   - site 关联：从 site_tag 查 namespace 镜像（与 `buildSiteTagLinks` 一致），或接受前端传值（见决策1）。
3. 落库机制无需改（`BaseRepository.CreateBatch` 会写 entity 的 Namespace 字段）。

### 前端：wrapper + UI

1. **wrapper**（`reWorkTag.ts`）：`reWorkTagLink` 加 namespaces 参数并透传；**顺便修 tagIds[0] 截断 bug**（声明4）——按 SelectItem[] 长度配对 namespaces。
2. **WorkDialog / WorkDetailDialog**（ExchangeBox 确认流 `handleTagExchangeConfirm`）：
   - local 关联：传用户选的 namespace（ExchangeBox 内加 namespace 选择器，复用 `frontend/src/constants/namespace.ts` 内置集 + 自定义输入）。
   - site 关联：传 site_tag 自带 namespace（`extraData.namespace`，候选联想已带）或空（后端镜像，见决策1）。
3. **bindings 重生**：`wails3 generate bindings -ts`（Link 签名变更）。

## 三、影响面

| 层 | 改动 |
|---|---|
| 后端 | `reWorkTag/handler.go`（Link 签名加 namespaces）、`service.go`（LinkBatchToWork 构造时写 Namespace） |
| 前端 wrapper | `reWorkTag.ts`（加 namespace 参数 + 修 tagIds 截断 bug） |
| 前端 UI | `WorkDialog.vue`、`WorkDetailDialog.vue`（ExchangeBox 加 local namespace 选择器） |
| bindings | 重生（Link 签名变更） |

**复用**：`buildSiteTagLinks`（`work/service.go:2037`）的 namespace 镜像是 site 关联的参考实现；`frontend/src/constants/namespace.ts`（J 节点已建）可复用于 local namespace 选择器选项。

## 四、验证

- 后端：`go build ./backend/...`；测试手动 link local 标签 + namespace → 查 `re_work_tag.namespace` 落库正确（Valid: ns != ""）。
- 前端：`yarn build`；WorkDialog 选 local 标签 + namespace 关联 → 查 `re_work_tag.namespace`。
- 端到端：手动添加带 namespace 的 local 标签 → namespace 搜索过滤（J）能命中该作品。
