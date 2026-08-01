# 作品集元数据 Drawer 方案

> 本方案是任务图 B 节点（`.claude/workflow/active/workset-merge/TREE.md`）的设计文档。B 由 A「作品集 merge 功能」的前端 UI 引出：merge 引入的子集区占据弹窗左侧面板顶部、压缩作品展示高度。经需求澄清，B 的真实目标是「给作品集补一套完整元数据展示与编辑页」，drawer 是载体，子集 chips 迁入是「子集属元数据 + 顺带省高度」的综合考量。

## 审查摘要

### 关键声明（抽查项）

- **声明1（WorkSet 可编辑本地字段仅 nickName）**：`backend/base/model/entity/work_set.go:19` 的 `NickName` 是唯一的本地可编辑字段；描述仅有站点抓取的 `SiteWorkSetDescription`（`work_set.go:16`），无本地描述字段。Work 实体同理（`entity/work.go` 仅 `siteWorkDescription`，无本地描述）。「名称」有 nickName(本地)/siteXxxName(站点) 分离先例，「描述」无此先例。
- **声明2（元数据聚合零后端查询改动）**：作品集作品接口 `ListWorkSetWithWorkByIds`（`backend/workSet/service.go:469-532`）对每个作品集调 `CollectDescendantWorkIDs` 做**传递包含**（指打开作品集 A 时，作品列表不只含 A 直属作品，还含其全部子作品集的子作品集……递归聚合来的后代作品，去重保序，`service.go:488-497`），再经 `GetFullWorkInfoByIds`（`backend/work/service.go:1010-1190`）**完整填充**每个 `WorkFullDTO` 的 LocalAuthors/SiteAuthors/LocalTags/SiteTags/Site/Resource。故前端可直接遍历 `workList`（`WorkSetDialog.vue:71`）聚合「所有作品」的作者/标签/站点并去重，与「含后代」语义精确吻合，无需新增后端查询。
- **声明3（workSetUpdate 现状缺陷）**：`frontend/src/apis/http/wrappers/workSet.ts:109-123` 的 `workSetUpdate` 仅构造 `{id, siteWorkSetName}`（`:114-117`），即「编辑名称」实际写的是站点抓取名 `siteWorkSetName` 而非本地 `nickName`。**正确做法应参照 Work 实体的本地优先惯例**（Work 的 `nickName` 本地名与 `siteWorkName` 站点名分离，优先用本地名）——注意此处参照的是 Work 实体的字段分离设计先例，而非 `WorkDetailDialog.vue:344`（该处回退的 `nickName ?? siteWorkName` 是 **Work 作品名**字段，与作品集的 `siteWorkSetName` 是两个不同实体）。此外该 wrapper 签名含 `coverId?: number`（`:112`）但 DTO 构造完全忽略它，属无效形参。后端 `Update`（`backend/workSet/handler.go:40-47` → `service.go:273-279`）全字段开放。
- **声明4（el-drawer 在 dialog 内可用）**：`WorkDetailDialog.vue:407-464` 已验证「el-dialog 内按钮触发 append-to-body el-drawer」组合可用（`:with-header="false"` + `size`）。WorkSetDialog 用 `StaticHeightDialog`（`WorkSetDialog.vue:538`），照搬该模式，无 z-index 冲突（drawer 在 body 层、EP 自动堆叠，高于 dialog 内 absolute 面板；且与 select 面板逻辑互斥不同时出现）。
- **声明5（迁移自动加列）**：`WorkSet` 已在 `backend/migration/migrate.go` 注册，新增 `Description` 列由 GORM AutoMigrate 自动添加，无需手写迁移。

### 待决策（需用户拍板）

原讨论中 4 项已拍板（决策1-4），对抗式审查后又补核 2 项（决策5-6）。当前无遗留阻塞项。

- **决策1（已定）**：新增本地描述字段 `Description`，与站点 `siteWorkSetDescription` 分离。理由：复用站点字段会在重新抓取时被覆盖，造成用户描述静默丢失。
- **决策2（已定）**：描述双区同屏——本地描述用可编辑 `el-input` 独占一项；站点描述作为只读 `el-descriptions-item`（标题「站点简介」）在下方并存。
- **决策3（已定）**：「添加子集 / 物理纳入」按钮**留标题栏**（不进 drawer）。drawer 与管理模式正交：任意模式可开 drawer 查看元数据。
- **决策4（已定）**：站点标签聚合仅按 `siteTag.id` 去重展示站点标签本身，本地标签独立按 `localTag.id` 去重。前端聚合**只读 `siteTag` 自身、不读其关联的 `siteTag.localTag` 字段**（虽然后端 `backend/work/service.go:1089-1103,1169-1173` 已把 `LocalTag` 挂到每个 `SiteTagFullDTO`，但前端消费时显式不取该字段，故「被动交叉」不会发生）。
- **决策5（审查后补定）**：重构 `workSetUpdate` 时**删除无效的 `coverId` 形参**（封面操作走独立接口 `reWorkWorkSetSetCover`，与 Update 无关，`coverId` 自始未参与 DTO 构造，属死代码）。
- **决策6（审查后补定，已核实）**：`BaseRepository.Update` 实现为 `Save`（全字段覆盖，见风险1 核实），故 `workSetUpdate` 必须先读回完整 `currentWorkSet`、合并编辑字段后整体提交，不得只传 `{id, nickName, description}`。

### 自曝风险

- **风险1（已核实，原「实施前必须核实」现已确认）**：`database.BaseRepository.Update`（`backend/database/base_repository.go:163-168`）实为 `Save`（全字段覆盖），非 `Updates`。这意味着现状 `workSetUpdate` 仅传 `{id, siteWorkSetName}` 会把 nickName 等其它字段全部清空为 NULL——这是现有代码已潜伏的 bug，本方案新增 description 编辑会暴露它。**对策见决策6**：wrapper 必须先读回完整对象再合并提交。**影响范围已核**：前端仅 `workSet.ts:118` 一处调用 `WorkSetHandler.Update`（merge、添加子集、物理纳入、移除子集均走各自独立接口，不经 Update），故本方案对策可只在 `workSetUpdate` 内落实，不波及其它调用方。
- **风险2（GORM 零值清空限制）**：`sql.NullString{Valid:false}` 在 `Save` 模式下会被写入为 NULL。因决策6 采用「读回完整对象后合并」，用户编辑只改 nickName/description，其余字段保持读回的值不变，故正常编辑不会误清空；唯一边界：用户主动把 description 置空（清空描述），会写 NULL，属预期行为。
- **风险3（聚合去重维度）**：前端聚合作者/标签/站点时的去重维度（localAuthor.id、siteAuthor.id、localTag.id、siteTag.id、site.id）需实现时统一，且 siteAuthor 可能与 localAuthor 指向同一人（`AuthorInfo` 组件 `AuthorInfo.vue:24-30` 已内建「未绑定本地作者的站点作者由本地作者代表」去重逻辑，聚合后可直接复用该组件）。
- **风险4（跨仓库改动的 bindings 同步）**：`Description` 字段加在 SDK 仓库（`library-squirrel-sdk/dto/work_set_dto.go`），主仓库 go.mod 以 replace 指向本地 SDK。SDK 改动后须在主仓库重新执行 `wails3 generate bindings -ts` 才能让前端 bindings（`frontend/bindings/.../models.ts`）出现新字段。
- **风险5（回滚，低风险）**：新增 `description` 列后若回滚到旧版本代码——GORM AutoMigrate 只加不减，旧代码读不到该列不影响运行（实体结构体无此字段，SELECT 不取该列），SQLite 保留多余列无副作用。属低风险，已明说。
- **风险6（代码库级独立隐患，非本方案范围）**：`BaseRepository.Update` 通用 `Save` 模式是影响所有走该泛型方法的模块（siteTag/localAuthor/site/plugin 等的 Update）的系统性隐患——任一调用方传不完整对象都会清空字段。本方案不修复（超出 B 范围），建议另起独立任务处理。

---

## 一、背景与目标

作品集（WorkSet）此前**没有任何完整的元数据展示与编辑页**：`WorkSetDialog.vue` 的 `currentWorkSet`（`:69`）在模板中仅被 header 标题引用一次（`WorkSetDialog.vue:548`，展示 `siteWorkSetName`），其余元数据（描述、作者、标签、站点、子作品集）全未展示，也无编辑入口。

本方案为 WorkSetDialog 增加一个元数据 drawer（标题栏按钮触发，照搬作品详情弹窗 `WorkDetailDialog` 的 drawer 模式），实现：
1. **展示**作品集的名称、本地描述、站点描述、所属站点集合、所有作品的作者/标签聚合集合、子作品集列表。
2. **编辑**名称（nickName）与本地描述（description）。
3. 子作品集 chips（`WorkSetDialog.vue:677-685`）迁入 drawer（属元数据 + 释放左侧面板高度）。
4. 标签仅展示**不编辑**（与作品元数据 drawer 的区别）。

## 二、字段读写矩阵

drawer 内容照搬作品元数据 drawer（`WorkDetailDialog.vue:407-464` 的 `el-descriptions` + `TagBox`/`AuthorInfo` 模式），去掉标签编辑：

| 区块 | 字段/内容 | 读写 | 展示组件 | 数据来源 |
|---|---|---|---|---|
| 名称 | `nickName` | **可编辑** | el-input | `currentWorkSet.nickName` |
| 本地描述 | 新增 `description` | **可编辑** | el-input(type=textarea) | `currentWorkSet.description` |
| 站点简介 | `siteWorkSetDescription` | 只读 | el-descriptions-item | `currentWorkSet.siteWorkSetDescription` |
| 所属站点 | 所有作品站点集合 | 只读 | TagBox（聚合去重） | `workList[*].site`，按 `site.id` 去重 |
| 作者 | 所有作品作者集合 | 只读 | AuthorInfo（聚合去重） | `workList[*].localAuthors + siteAuthors`，各自按 id 去重 |
| 本地标签 | 所有作品本地标签集合 | 只读（无编辑按钮） | TagBox（聚合去重） | `workList[*].localTags`，按 `localTag.id` 去重 |
| 站点标签 | 所有作品站点标签集合 | 只读（无编辑按钮） | TagBox（聚合去重） | `workList[*].siteTags`，按 `siteTag.id` 去重 |
| 子作品集 | chips | 展示 + 移除（closable） | el-tag closable | `childWorkSets`（`WorkSetDialog.vue:82`） |

编辑态：drawer 底部放「保存」按钮 → 调 `workSetUpdate(id, {nickName, description})`，成功后刷新 `loadWorkList()`。

## 三、后端改动清单

1. **实体** `backend/base/model/entity/work_set.go`：在 `NickName`（`:19`）下方加
   `Description sql.NullString \`gorm:"column:description" json:"description"\``
2. **SDK DTO** `library-squirrel-sdk/dto/work_set_dto.go`：在 `NickName *string` 旁加 `Description *string`
3. **双向映射** `backend/base/model/dto/work_set_dto.go`：
   - `NewWorkSetDTO`（`:10-28`）加 `Description: util.NullStringToPointer(workSet.Description)`
   - `ToWorkSetEntity`（`:31-116`）仿 `NickName` 块（`:93-98`）加 `Description` 的 Valid/String 赋值
4. **迁移**：无需手写（声明5）。开发期若数据库已存在，AutoMigrate 会自动 `ALTER TABLE work_set ADD COLUMN description`。
5. **实体工厂**：`entity.NewWorkSet()`（`:23-27`）无需改（字段零值即可）。
6. **前端 wrapper** `frontend/src/apis/http/wrappers/workSet.ts:109-123`：重构 `workSetUpdate`：
   - 签名改为 `{id, nickName?: string, description?: string}`（删除无效 `coverId` 形参，见决策5）
   - **因 BaseRepository.Update 实为 Save 全字段覆盖（风险1 已核实）**：wrapper 必须先读回完整 `currentWorkSet`（已有，由 `loadWorkList` 填充），合并用户编辑的 `nickName`/`description` 后，**整体提交完整 DTO**（保留其余字段原值），否则会清空其它字段（决策6）
   - DTO 构造写 `nickName`（修正声明3 的字段错写）+ 新增 `description`

## 四、前端结构（WorkSetDialog.vue）

### 4.1 触发入口（标题栏）
在管理模式 header 的「管理」按钮旁（`WorkSetDialog.vue:549-558`）加「元数据」按钮（`Document` 图标，`@click="openWorkSetDrawer"`）。与管理按钮并列，不受 `isCheckable` 影响（任意模式可开）。

### 4.2 drawer 主体
在 `</static-height-dialog>` 前（`WorkSetDialog.vue:729`）加：
```vue
<el-drawer v-model="workSetDrawerState" size="40%" :with-header="false">
  <el-scrollbar>
    <!-- 名称（可编辑 input） + 本地描述（可编辑 textarea） -->
    <!-- 站点简介（只读 descriptions-item） -->
    <!-- 所属站点 / 作者 / 本地标签 / 站点标签（TagBox / AuthorInfo，聚合去重） -->
    <!-- 子作品集 chips（从 677-685 迁入） -->
    <!-- 保存按钮 -->
  </el-scrollbar>
</el-drawer>
```

### 4.3 聚合计算属性（新增）
```ts
// 所属站点（按 site.id 去重 → SegmentedTagItem[]）
// 作者（localAuthors + siteAuthors 各按 id 去重 → 传 AuthorInfo）
// 本地标签（按 localTag.id 去重 → SegmentedTagItem[]）
// 站点标签（按 siteTag.id 去重 → SegmentedTagItem[]）
```
均基于 `workList`（`WorkSetDialog.vue:71`）computed 派生。`SegmentedTagItem` 构造参照 `WorkDetailDialog.vue` 现有标签转换逻辑。

### 4.4 删除旧子集区
删除 `WorkSetDialog.vue:646-687` 的 `child-work-set-section`（含 chips `677-685`），其 chips 与移除逻辑迁入 drawer 子作品集区块。「添加子集/物理纳入」按钮（`654-671`）按决策3 **保留在标题栏**（移至与「管理/元数据」同行的操作区，或独立行——实现时定，不阻塞）。

## 五、实施步骤（建议顺序）

1. 后端：加实体字段 → SDK DTO → 映射函数（改动清单 1-3）。
2. 主仓库：`wails3 generate bindings -ts`（风险4）。
3. 前端 wrapper：重构 `workSetUpdate`（改动清单 6，含删 coverId + Save 合并策略）。
4. 前端：标题栏触发按钮 + drawer 主体 + 聚合 computed + 保存逻辑（第四节）。
5. 删除旧子集区、chips 迁入 drawer（第四节）。
6. 实机验证：开 drawer 看聚合展示、改名称/描述保存、移除子集、确认左侧面板高度释放。

## 六、约束（守住勿回退）

- 标签在作品集 drawer **只展示不编辑**（与作品 drawer 区别，不可回退为可编辑）。
- 名称编辑必须写 `nickName`，不得写 `siteWorkSetName`（声明3）。
- 描述字段必须本地/站点分离（决策1），不得复用 `siteWorkSetDescription` 承载用户编辑。
- 站点标签聚合不做本地标签映射交叉（决策4）。
- 新 repository/查询方法用 `dbFromCtx(ctx)` 禁 `GORM()`（MaxOpenConns=1 死锁，任务图约束）——本方案无新增后端查询，不涉及。
