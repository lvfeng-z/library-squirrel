# 多 store/多资源落盘命名规约（任务 N）

> **审查摘要**（经 plan-reviewer 对抗式审查 + 决策后修订）
> - **核心机制**：**bas 基准命名**。bas（基准名）= `FileNameFormat` 模板生成的文件名基底。单 store：`<bas>.<ext>`；多 store：`<bas>_<role>_<seq>[_<描述>].<ext>`。所有插件 store（image/document/videoTrack/audioTrack/thumbnail）统一进 `store/resource/<作者>/`，仅 `merged`（主程序派生）维持 `<bas>_merged.<ext>`。
> - **关键锚点**：命名函数 `resolveMainPath`/`resolveStorePath`（`backend/taskManager/model.go:1970/1996`）、缩略图派生 `buildThumbnailRelPath`（`model.go:2154`）、merged 派生 `BuildVariantPath`（`backend/persistentStore/service.go:640`）；`StoreSpec`（`library-squirrel-sdk/dto/handler_dto.go:107`，当前无 Description）；多 store 判定用**资源 store 总数**（start=`len(specs)` model.go:931 / resume=`len(storeRows)` model.go:1476——resume 的 specs 是未完成子集 model.go:1493-1521，不能用 `len(specs)`）；同 role 内 seq 用 per-role 计数器 `roleCounters`（算 `store_seq`），`countSpecRoles`（per-role 计数 map，`model.go:1880`）**废弃**；前端取缩略图按 `thumbnailStore.filePath`（`WorkCard.vue:33`/`WorkSetCard.vue:32`/`WorkDialog.vue:113`，**路径透明——前端不依赖目录集中性**）；`InlineImages []string`（`library-squirrel-plugin-bilibili/internal/bilibiliapi/dynamic.go:225`，纯 URL 数组，无描述字段）；`StoreStream` 删旧建新覆盖（`persistentStore/service.go:231`）；代码已回退至 `33ba595`（`git log 33ba595..HEAD -- backend/taskManager/model.go` 仅含回退 commit `97c34b0`）。
> - **决策已定**：①bilibili 描述接受空（`InlineImages` 是 URL 数组，bilibili 不填 Description，走 role+seq 兜底；SDK 仍加字段供未来/其他插件）；②resume 路径口径**不兼容旧版本**（旧任务 thumbnail 跨重启续传的路径不一致不处理）；③task.ext 兜底（D2）用**方案 B**：`GetFileNameFormat` 空时回退默认模板（根治模板空→撞名覆盖）；④**thumbnail 普通 role 化 + 资源级判定**（2026-07-27 用户校正）：thumbnail 是普通 role 遵循 bas 规约、不开特例（废弃 `buildThumbnailRelPath` 独立派生）；判定从 per-role（`countSpecRoles[role]>1`）改为资源级（`len(specs)/len(storeRows)>1`）——per-role 下 image+thumbnail 单例同 bas 同 ext 同目录会撞名。pixiv 单图 `InvolvedRoles=[image]`（pixiv task_handler.go:214）仅返回单 store → `<bas>.<ext>` 零扰动保持。
> - **自曝风险**：① thumbnail 从 `store/thumbnail/` 迁 `store/resource/`，**历史已落盘文件不迁移**；且**旧任务跨重启 resume 时路径口径切换不兼容**（DB 旧 `file_path` `store/thumbnail/...` vs 新算 `store/resource/...`）——用户已接受（假定无活跃旧任务或重下）；② `StoreSpec` 加 `Description` 改 proto 契约——proto3 新增标量字段需分配字段编号、四端（SDK→主程序→各插件）升级顺序约束，非完全双向兼容；③ `${downloadTime*}` 占位符破坏确定性，规约禁止参与命名（剥离）；④**resume seq 一致性**：resume 的 specs 是未完成子集（model.go:1493-1521），`sameRoleSeq` 现状用 specs 内 `roleCounters` 重计（model.go:1589），在“同 role 部分 store 完成部分未完成”时与全局 `store_seq` 错位 → 文件名漂移 → 续传/重建到错误路径（`StoreStream` 删旧建新=无声覆盖）。N 实施须让 resume 的 seq 取自 `storeRows` 对应行 `StoreSeq`（M 身份化），验证 M 是否已覆盖，有缺口则子任务处理。

## 一、背景与派生

**派生自上游任务 R（图文紧密结合资源），非阻塞**：R 实施 article 定 D4（`.md` 图引用 basename）时发现——作品:资源从 1:1（1 作品 1 文件）演进到 1:N（1 作品 N 资源、每资源 N store）后，store `file_path` 落盘命名规约从未系统设计。本任务（N）正式补齐。

**问题真伪甄别**：N 初始担忧的"`.md` basename ↔ 落盘名一致"经审视是**误判**——`.md` 是内容（document store），不应依赖图片（image store）物理文件名；位置绑定（前端 `MarkdownView` 第 k 个 image token → 第 k 个 image store，已实现）是正确解法。N 的**真问题**是**命名碰撞风险**（1:1→1:N 后规约缺失导致碰撞可能），不是 basename 一致性。

## 二、现状审计（Explore 2026-07-26）

命名权集中在主程序 `taskManager`，插件只贡献素材（`Format`/`SuggestName`/`Markdown` 占位）。

| # | 位置 | 逻辑 | 风险 |
|---|------|------|------|
| 1 | `model.go:1970` `resolveMainPath` | 读 `FileNameFormat` → 占位符替换 → sanitize + ext；`store/resource/<authorDir>/<fileName>` | 中（`${downloadTime*}` 时变）|
| 2 | `model.go:1996` `resolveStorePath` | 同 role 多 store 加 `_%03d`（从 0 起）；thumbnail 走 `buildThumbnailRelPath` 派生 | 低 |
| 3 | `model.go` `buildSuggestedFileName` | 模板空兜底：`SuggestName` → 否则 `task`/`TaskName` + ext | **高（撞名无声覆盖）—— 由 D2 方案 B 修复** |
| 4 | `model.go:2154` `buildThumbnailRelPath` | 缩略图派生：`store/thumbnail/<作者>/<bas>_thumbnail.<format>` | 待改造（进 store/resource）|
| 5 | `persistentStore/service.go:640` `BuildVariantPath` | merged 派生：源 relPath + `_merged` | 低（维持现状）|
| 6 | `persistentStore/service.go:231` `StoreStream` | 落盘执行；已有记录**删旧建新** | 关键防线（碰撞=无声覆盖）|

- **SDK**：`StoreSpec.SuggestName`（`dto/handler_dto.go:107`），当前**无 Description 字段**。
- **pixiv**：唯一给 `SuggestName`；**bilibili/local**：不给。
- **bilibili article**：`InlineImages []string`（`dynamic.go:225`，纯 URL 数组，**无描述字段**）；`.md` 占位 `![001.jpg](001.jpg)`，靠前端位置绑定兜底（占位是逻辑符号）。

## 三、问题与风险

### 高风险碰撞场景
1. **模板空 + 无 SuggestName**（bilibili/local）→ 落盘名退化为 `task.ext` → `StoreStream` 删旧建新**无声覆盖丢数据**（`service.go:231`）。**D2 方案 B 修复**（settings 模板非空）。
2. `${siteWorkId}` 站点未返回 → 字面保留 → 同作者同名碰撞。
3. `${downloadTime*}` 时变 → 破坏确定性。

### 规约盲区
- 1:1→1:N 演进后命名规约从未系统设计（无多 store 文件名规则）。
- thumbnail 走独立目录 `store/thumbnail/`（前端按 filePath 取，**路径透明，目录集中无必要性**）。

## 四、命名规约（bas 基准）—— 核心

### 4.1 bas 定义
**bas**（基准名）= `FileNameFormat` 模板经 `ExtractTokenData` + `FormatFileName` + `SanitizeFileName` 生成的文件名基底。如模板 `[${author}]_[${siteWorkId}]_${siteWorkName}` → bas = `[作者]_[workId]_作品名`。模板空时由 D2 方案 B 保证回退默认（不会空）。

### 4.2 文件名规则
| 场景 | 文件名 | 示例 |
|---|---|---|
| **单 store** | `<bas>.<ext>` | `[作者]_[workId]_作品名.jpg` |
| **多 store · 有描述** | `<bas>_<role>_<seq>_<描述>.<ext>` | `[作者]_[workId]_作品名_image_000_封面图.jpg` |
| **多 store · 无描述** | `<bas>_<role>_<seq>.<ext>` | `[作者]_[workId]_作品名_document_000.md` |

- **路径**：统一 `store/resource/<作者>/<文件名>`（扁平，无子目录）。
- **seq**：同 role 内 0-based（= `store_seq`，resume 身份键不变）；多 store 场景总加；单 store 不加。
- **描述**：`StoreSpec.Description`（新增字段），插件可选填；空则省略描述段。**bilibili 当前不填**（`InlineImages` 是 URL 数组，无描述数据）→ 走 role+seq 兜底（如 `[bas]_image_000.jpg`）。SDK 仍加字段，供未来扩展或其他插件使用。
- **role**（多 store 总带）：`image`/`document`/`videoTrack`/`audioTrack`/`thumbnail`。
- **“单/多 store”判定（资源级，2026-07-27 校正）**：`资源 store 总数 > 1` → 多 store（全部带 role+seq）；`== 1` → 单 store（`<bas>.<ext>`）。判定**不基于 per-role 计数**——thumbnail 普通 role 后，per-role 判定下 image+thumbnail 各自单例会跨 role 撞名（同 bas 同 ext 同目录）。`startDownload` 总数=`len(specs)`（全量）；`resume` 总数=`len(storeRows)`（specs 是未完成子集，不能用 `len(specs)`，否则 resume 判定翻转→文件名漂移）。

### 4.3 各 store 处理
- **插件 store**（image/document/videoTrack/audioTrack/**thumbnail**）：统一走 4.2 bas 规约，进 `store/resource/`。thumbnail 不再走 `buildThumbnailRelPath` 独立派生命名、不再进 `store/thumbnail/`。
- **merged**（主程序派生）：维持 `<bas>_merged.<ext>`（`BuildVariantPath` `service.go:640` 现状，不纳入 bas 规约）。

### 4.4 确定性约束
命名输入禁止时变：`${downloadTime*}` 占位符破坏确定性，规约明确剥离（不参与命名），唯一性由 bas + role + seq 保证。

## 五、实施

### 5.1 SDK（`library-squirrel-sdk`）
- `proto/plugin.proto` + `dto/handler_dto.go`：`StoreSpec` 加 `Description` 字段（string，proto3 标量，分配新字段编号，不复用）。
- 重生成 `gen/`（protoc，环境齐全）。
- **四端升级顺序**：SDK 发新版本 → 主程序升级引用 → 各插件按需升级（bilibili 当前不填，无需改）。向前兼容（旧插件不填则空）。

### 5.2 主程序（`backend/`）
- **命名改造**（`taskManager/model.go`）：
  - `resolveMainPath`：算 bas（模板结果），返回 bas（不含 ext，ext 由规约统一拼）。
  - `resolveStorePath`：多 store 拼 `<bas>_<role>_<seq>[_<描述>].<ext>`；单 store 用 `<bas>.<ext>`。**多 store 判定用资源 store 总数**（`len(specs)`/`len(storeRows)` > 1，传入参数），不用 `countSpecRoles`（per-role，废弃）。seq 用同 role per-role 计数器（= `store_seq`，resume 取自 `storeRows` 对应行）。
  - `buildThumbnailRelPath/FileName`：**废弃**——thumbnail 走 4.2 bas 规约进 `store/resource/`。
- **D2 方案 B**（`backend/settings/service.go` `GetFileNameFormat`）：空时回退默认模板 `"[${author}]_[${siteWorkId}]_${siteWorkName}"`。根治模板空→撞名覆盖（`buildSuggestedFileName` 的 `task.ext` 分支不再触发）。
- **不兼容旧版本**：thumbnail 路径从 `store/thumbnail/` 切到 `store/resource/`，旧任务跨重启 resume 时路径口径不一致**不处理**（用户接受，假定无活跃旧任务或重下）。

### 5.3 插件
- **bilibili**：当前不填 `Description`（`InlineImages` 是 URL 数组）。未来若扩展解析层提取图片注释，可填。
- 其他插件（pixiv/local）：按需填 `Description`（当前都不填，走 role+seq 兜底）。

### 5.4 文档
- 新建 `doc/store-naming-convention.md`（本规约）。
- `database.md` 路径规约：thumbnail 不再独立 `store/thumbnail/`（白名单 `store/thumbnail` 可保留供历史路径，或最终移除）。

## 六、验收
1. 单 store（pixiv 单图）：`store/resource/<作者>/<bas>.<ext>`（与现状一致，零扰动）。
2. 多 store（bilibili article）：所有 store 在 `store/resource/<作者>/<bas>_<role>_<seq>.<ext>`（bilibili 无描述，走 role+seq）。
3. thumbnail 在 `store/resource/`（非 `store/thumbnail/`），文件名含 role+seq。
4. **模板空时不撞名**：`GetFileNameFormat` 回退默认模板，落盘名正常（非 `task.ext`）。
5. 位置绑定仍工作（`.md` 占位是逻辑符号，与文件名解耦）。
6. 历史已落盘文件不迁移、仍可访问。
7. 四端编译通过 + taskManager/entity 单测无回归。

## 七、风险与边界
- **thumbnail 目录迁移**：`store/thumbnail/` → `store/resource/`。历史不迁移；**旧任务 resume 路径口径切换不兼容**（已接受）。前端按 filePath 取（路径透明），不受影响。
- **proto 兼容性**：`StoreSpec` 加 `Description` 需四端升级顺序（SDK→主程序→插件），proto3 新字段对旧客户端反序列化行为取决于字段编号（不复用旧编号）。
- **范围边界**：本规约只管"插件 store 的文件名/路径"。`.md` basename 一致性（位置绑定解决）不在范围。

## 八、历史：前置命名弯路（已废弃）

N 曾深入"前置命名"方向（`host.ResolveStorePath` RPC + 确定性命名函数 + sameRoleCount + 改命名规则 + Phase 1-4 跨仓库实施）。经审视废弃：
- **动机不成立**：前置命名为"让 bilibili `.md` 占位 == 物理落盘名"而设计，但 `.md` 是内容、不应依赖资源文件名，位置绑定（已实现）是正确解法。
- **sameRoleCount 矛盾**：前置命名要求插件传 sameRoleCount（同 role 总数），违背"store 列表渐进可知、插件不知总数"的前提。
- **代码状态**：Phase 1（命名函数去方法化，commit `3155626`）已回退（commit `97c34b0`），代码回到 `33ba595`。本规约在 `33ba595` 基础上推进。
