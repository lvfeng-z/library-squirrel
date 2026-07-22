# 资源类型规约体系（Resource Type Spec）设计方案

> **派生自**：`rich-article-resource-plan.md` 的 D2 讨论（2026-07-21）。纠结"article 复用 main 还是新增 store_type"时发现：store_type 只表达"业务类型"，不表达"资源如何展示/应有什么结构"，是个结构性空白。经多轮讨论演化为**资源类型规约体系**：资源类型映射规定其 store 角色组合（含基数）+ 各角色的文件标准（描述性）+ 展示主体优先级。
> **状态**：设计阶段·**经 plan-reviewer 两轮审查 + 审查后修订（2026-07-21）**。决策序列：①去主程序推断②命名 kind→ResourceType③store_type 重构（去 main）+ 基数化 Roles + PrimaryRoles；**审查后修订：声明3 降级、严格识别（空/未知值抛错）、不兼容历史数据、原子发布**。
> **与图文文档计划的关系**：本方案是 `rich-article-resource-plan.md`（图文文档，article 类型）的**前置基建**；article 纳入本体系 Registry。

## 审查摘要（给审查者，≤45 行；本段为审查入口）

> 审查只需读本段 + 抽查锚点 + 确认待定实施细节。

**已核验事实（抽查项，挂锚点）**：
- **F1. store_type 完全由插件在 StoreSpec.Role 声明，主程序零处理原样落库** — `backend/taskManager/model.go:1115`；落库 `:1041-1075`/`:1100-1122`
- **F2. store_type 当前混合三维度（业务用途/媒体组成/派生关系），无"资源类型"维度** — `backend/base/model/entity/resource_store.go:9-15`
- **F3. Resource 实体无资源类型字段、无结构规约** — `backend/base/model/entity/resource.go:11-18`
- **F4. 前端展示靠"扩展名嗅探 + store_type 组合"双重凑出，已脆弱** — 图片靠 `IMAGE_EXTENSIONS`(`WorkCard.vue:29-47`/`WorkSetCard.vue:24-45`/`WorkDialog.vue:108-127`)；视频借用 `isResourceMergeable`(`ResourceUtil.ts:10-20`)；外部打开写死 `appLauncherOpenImage`；**展示主体优先级（merged>main）散落前端** `getResourceOpenPath`(`ResourceUtil.ts:25-35`)
- **F5. `ResourceComplete` 是死字段（恒 0），完整性语义需从无到有新建** — 全 `backend/` 仅 `taskManager/model.go:1047,1060` 两处 `=0`、无非零赋值；`frontend/src` 无 `resourceComplete` 消费
- **F6. 现有形态枚举清晰** — 单图`{main}`、图+缩略图`{main,thumbnail}`、视频`{videoTrack,audioTrack,thumbnail}`、合并后 keep`{...,merged}`/overwrite`{thumbnail,merged}`、多图=N 独立 Resource 各`{main}`
- **F7. proto 契约锚点实** — `library-squirrel-sdk/proto/plugin.proto:117`(`TaskCreateChildResponse`)/`:126`(`TaskCreateResponse`)
- **F8. resource_store 无 (resource_id,store_type) 唯一约束** — `backend/base/model/entity/resource_store.go:29`(仅 `column:store_type`) → 物理允许同种 store 多行
- **F9. merged 历史"累积多行"已是现实瑕疵** — `backend/resource/merge_service.go:92-94`(注释"历史已累积的多行 merged 不在此清理")

**设计规定（用户拍板）**：
- **S1. 资源类型封闭枚举**：主程序中央注册表 `ResourceTypeRegistry`（代码常量）+ 文档化。插件不能自定义（需注入解析逻辑，列待办 T3）。
- **S2. store_type 随之封闭**：自定义 store_type 与自定义资源类型同问题（T3）。
- **S3. 规约消费者全选**：①插件开发看文档 ②主程序结构校验 ③前端渲染信任结构 ④前端板块勾选按 Roles 过滤。
- **S4. 校验边界**：文件内容不校验（成本过高）；结构校验做（数量区间，纯集合运算）。
- **S5. 资源类型声明（严格，审查后修订）**：插件**必须声明**有效 resource_type（5 预定义之一）；主程序不推断、**不兜底**——未声明（空）/未知值**一律抛错**。`unknown` 是合法显式值（插件确实无法分类时声明）。无 InferType。
- **S6. store_type 体系重构（去 main）**：`main` 语义模糊（图/.md/pdf 漂移），去除；用 `image`/`document` 替代。新 6 角色：`thumbnail`/`videoTrack`/`audioTrack`/`merged`/`image`/`document`。
- **S7. 基数化 Roles**：`Roles []StoreRoleSpec{StoreType,Min,Max}` 取代集合语义；Min≥1 即必含；结构校验判数量 ∈ [Min,Max]。
- **S8. 展示主体优先级链 + WorkStore 后端派生**：`PrimaryRoles []string`（video=`[merged,videoTrack]`）；`WorkStore` 后端 `ResolvePrimaryStore` 派生；前端纯消费，零主体决策。
- **S9. 严格识别 + 不兼容历史 + 原子发布（审查后修订）**：store_type 与 resource_type 一律严格识别（空/未知抛错，不兜底）；**不为历史数据做迁移/回填**（用户上线前手动处理）；主程序 + SDK + 所有插件**原子发布**（无兼容窗口，否则老插件被抛错拒绝）；T3 自定义类型通过注册进 Registry 纳入可识别范围。

**待定实施细节（plan 定）**：I1 代码位置 `resource_type.go`；I2 完整性语义新建；I3 历史数据（不迁移，用户手动）；I4 proto/SDK；I5 迁移（仅 AutoMigrate 加列，无数据迁移）；I6 完整性激活时序；I7 WorkSet 层消歧；I8 Start 声明契约；I9 store_type 重构先行（Phase 0）。

**自曝风险**：
- **R1. 未声明/未知资源类型→抛错（严格）**：主程序不兜底，现役插件必须声明有效 resource_type，否则任务创建失败（Phase 5 强制 + Phase 7 原子发布）。
- **R2. 三维度立约需固化**：展示看资源类型、业务行为看 store_type、结构合法性看资源类型→Roles 映射。
- **R3. 封闭枚举与历史 store_type 开放的过渡**：排查现有插件无非预定义值。
- **R4. 结构校验与部分下载协调**：video 缺轨=不完整，须不干扰续传。
- **R5. 完整性语义跨模块副作用**：激活后历史 Resource 被判"不完整"，须三态语义（0=未校验）化解。
- **R6. SDK/proto 跨仓库变更回归**：SDK + pixiv/local + bindings，且须原子发布。
- **R7. 上线前历史数据手动处理**：S9 不迁移，历史 main store/merged 多行/无 resource_type 的 resource 须用户上线前手动处理，否则新体系下抛错。
- **R8. WorkStore 派生变更跨模块影响**：前端 `getResourceOpenPath` 须改纯消费。
- **R9. 完成判断写入点与调度不变量冲突**：Phase 2 改任务状态机须避开 actor 调度 CAS 守卫。

---

## 一、问题（F1-F9）

当前 store_type 体系两个结构性缺陷：
1. **缺"资源类型"维度**（F2）：store_type 只表达业务角色，前端靠嗅探凑（F4）。
2. **`main` 语义模糊**（S6）：main 是"万能主资源"（图/.md/pdf 漂移），靠 resource_type 反推——正是要引入 resource_type 才能消除的模糊。留 main 等于保留要解决的缺陷。

且 resource_store 无唯一约束（F8）、merged 已多行累积（F9）。

## 二、设计：资源类型规约体系

### 2.1 三层结构
```
Resource.ResourceType (string，NOT NULL，引用一个资源类型)
   ↓ 指向
ResourceTypeRegistry (主程序中央注册表，封闭枚举，代码常量)
   ↓ 含
ResourceTypeSpec { Roles[](基数), PrimaryRoles[](展示优先级), StoreStandards(文件标准) }
```

### 2.2 ResourceTypeSpec 结构
```go
// backend/base/model/resource_type.go
type ResourceTypeSpec struct {
    ResourceType   string
    Roles          []StoreRoleSpec      // 结构角色+基数(完整性校验用)
    PrimaryRoles   []string             // 展示主体优先级链(高→低)
    StoreStandards map[string]StoreStandard
}
type StoreRoleSpec struct {
    StoreType string
    Min       int  // 最少(0=可选,1=必含)
    Max       int  // 最多(0=不限,1=单例)
}
type StoreStandard struct {
    Description string
    Formats     []string
    Generation  string
}
```

### 2.3 store_type 体系（S6，去 main，6 角色）
| store_type | 语义 |
|---|---|
| `thumbnail` | 封面/缩略图（各类型通用） |
| `videoTrack`/`audioTrack` | 视频/音频轨（video 组成） |
| `merged` | 合并产物（video 派生，Max=1） |
| `image` | 图片（image 主体；article 内嵌图多例） |
| `document` | 文档文件（article 正文 .md；document 原文件 pdf） |

### 2.4 预定义资源类型（初始 Registry）
| 资源类型 | Roles（Min~Max） | PrimaryRoles | 文件标准（描述性） |
|---|---|---|---|
| `image` | image(1~1), thumbnail(0~1) | `[image]` | image=图片(.jpg/.png/.webp, downloaded) |
| `video` | videoTrack(1~1), audioTrack(1~1), thumbnail(0~1), merged(0~1) | `[merged, videoTrack]` | videoTrack=视频流(.mp4, downloaded)；merged=合并产物(.mp4, derived) |
| `article` | document(1~1), image(0~N), thumbnail(0~1) | `[document]` | document=markdown 正文(.md, derived)；image=内嵌图(downloaded) |
| `document` | document(1~1), thumbnail(0~1) | `[document]` | document=现成文档原文件(.pdf/.docx, downloaded) |

> `unknown`：合法显式值（插件确实无法分类时声明），无 Roles 约束、不做结构校验，前端走嗅探兜底。

### 2.5 文档化（S1）
新增 `doc/resource-type-spec.md`：列每类型 Roles/PrimaryRoles/文件标准 + 严格识别契约（必须声明有效值，空/未知抛错）。

## 三、校验与主体解析（S4/S7/S8）

| 操作 | 函数（待实现） | 语义 |
|---|---|---|
| 结构校验 | `ValidateResourceStructure(resourceType, storeTypeCounts) (missing, excess []string)` | 每角色 count ∈ [Min,Max] |
| 展示主体 | `ResolvePrimaryStore(stores, spec) *ResourceStore` | 按 PrimaryRoles 取第一个存在 |

**接入点**：结构校验在完成判断处（全部 store 落盘后）；展示主体由 `NewResourceFullDTO` 派生 WorkStore（后端集中，前端纯消费）。

**两概念分离**（都在 spec）：结构完整性（Roles）vs 展示主体（PrimaryRoles）。

## 四、规约消费者（S3）

| 消费者 | 用法 |
|---|---|
| ① 插件开发 | 看 `doc/resource-type-spec`，声明 ResourceType + 产出 Roles |
| ② 主程序完整性 | 结构校验写 ResourceComplete |
| ③ 前端渲染 | 按资源类型选渲染器；展示主体用 WorkStore |
| ④ 前端板块勾选 | 按 Roles 过滤 |

## 五、影响面（分清单）

### 5.1 后端（主程序）
- `entity/resource.go`：加 `ResourceType`（NOT NULL）
- `entity/resource_store.go`：删 `StoreTypeMain`、加 `StoreTypeImage`/`StoreTypeDocument`
- `resource_type.go`（新）：`ResourceTypeSpec`/`StoreRoleSpec`/`ResourceTypeRegistry`/`ValidateResourceStructure`/`ResolvePrimaryStore`
- `dto/resource_dto.go`：填充 ResourceType；WorkStore 改 `ResolvePrimaryStore` 派生
- `taskManager/model.go`（saveResource）：**严格校验+写入**——resource_type 空/未知→抛错（S5/S9）；合法值写入
- mountResourceStores：store_type 未知→抛错（S9）
- 新建完成判断逻辑（F5）：调 ValidateResourceStructure 写 ResourceComplete
- `merge_service.go`：merged Max=1 守卫（仅新数据）
- `migration`：AutoMigrate 加 ResourceType 列（**无数据迁移**，S9）

### 5.2 SDK（含 proto，跨仓库，原子发布）
- `proto/plugin.proto:117,126`：加 optional `resource_type`
- `dto/handler_dto.go`：加 `ResourceType` + 新 store_type 常量
- 跨仓库回归 + 原子发布（R6/S9）

### 5.3 插件（强制声明 + 新 store 角色）
- 各插件 Create 必须声明有效 resource_type；store 用新角色（image/document）
- bilibili video→video、dynamic→image、article→article（正文 document、图 image）；pixiv/local→image

### 5.4 前端
- `sectionCode.ts`：store 角色去 main、加 image/document；加 ResourceType 常量
- `ResourceUtil.ts`：`getResourceOpenPath` 改纯消费 WorkStore（删前端优先级逻辑）；抽 `getResourcePreviewType`
- `WorkCard/WorkSetCard/WorkDialog`：按资源类型分发；外部打开按类型
- `TaskOperationBarActiveV1`：板块按 Roles 过滤
- 新建 ResourceComplete 三态消费链
- 保留 store_type 业务判定：`isResourceMergeable`

### 5.5 文档
- 新增 `doc/resource-type-spec.md`

## 六、历史数据（S9，不兼容）

**不迁移、不回填、不兜底**（严格模式）。历史数据由用户上线前手动处理：
- 历史 main store → 用户手动转 image/document。
- 历史 merged 多行 → 用户手动收敛。
- 历史 resource 无 resource_type → 用户手动填值或接受异常。

plan 不写自动迁移逻辑。上线前用户须确认历史已处理，否则历史资源在新体系下抛错。

## 七、待办：插件自定义资源类型扩展（T3，reveal 延后）

让插件注册自定义资源类型（含自定义类型值 + store_type + 解析/渲染逻辑注入），通过注册进 Registry 纳入可识别范围（S9 严格识别下，未注册的自定义类型会被抛错）。现插件模块不支持，等封闭枚举版落地 + 真实需求时 focus。TREE.md 作 [延后] 节点。

## 八、工作量与阶段（详见实施 plan）

Phase 0 store_type 重构（不迁移历史）→ Phase 1 后端规约（严格校验）→ Phase 2 完整性语义 → Phase 3 SDK/proto → Phase 4 前端 → Phase 5 插件（强制声明）→ Phase 6 文档 → Phase 7 原子发布 → Phase 8 验证。

## 九、决策对照（已收敛 2026-07-21，含审查后修订）

| 决策 | 结论 |
|---|---|
| 走乙（资源类型规约） | 是 |
| 资源类型封闭 vs 插件自定义 | 封闭（T3） |
| 规约消费者 | 全选 |
| store_type 开放性 | 随之封闭（T3；DB 不加 CHECK） |
| 校验范围 | 文件内容不校验、结构校验做 |
| **资源类型声明** | **严格：必须声明有效值，未声明/空/未知→抛错（不兜底）；unknown 是合法显式值**（S5，审查后修订） |
| **store_type 体系** | **去 main，image/document 替代，6 角色**（S6） |
| **Roles 形态** | **基数化 `{Min,Max}`**（S7） |
| **展示主体** | **PrimaryRoles + WorkStore 后端派生 + 前端纯消费**（S8） |
| **历史数据** | **不兼容、不迁移，用户上线前手动处理**（S9，审查后修订） |
| **识别与发布** | **严格识别（空/未知抛错）+ 原子发布（主程序+SDK+插件同步）**（S9，审查后修订） |
| 多图 gallery | 不做（WorkSet 层不依赖资源类型） |
| 完整性语义 | 新建（F5 死字段，三态 ResourceComplete） |
| 命名 | ResourceType（kind→type） |

---

## 十、若采纳的后续动作

1. 实施计划 `doc/plan/resource-display-kind-plan.md`（Phase 0-8）。
2. TREE.md：K 含 store_type 重构；reveal T3。
3. task-graph 维护派生图。
4. `rich-article-resource-plan.md` 更新：article=ResourceType（document .md + image 0~N）。
