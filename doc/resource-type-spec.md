# 资源类型规约（Resource Type Spec）

> 资源类型规约体系：**资源类型 → store 角色组合(含基数) + 展示主体优先级 + 文件标准**。
> 插件开发据此声明 ResourceType + 产出对应 Roles；主程序据此结构校验与展示主体派生；前端据此渲染分发。
> 权威实现：`backend/base/model/entity/resource_type.go`（`ResourceTypeRegistry`）。本文档为其人类可读说明，代码改动须同步本文。

## 一、三层结构

```
Resource.ResourceType  (string，NOT NULL，引用一个资源类型)
   ↓ 指向
ResourceTypeRegistry   (主程序中央注册表，内置 6 种 + 插件运行时注册自定义类型)
   ↓ 含
ResourceTypeSpec { Roles[](结构角色+基数), PrimaryRoles[](展示优先级), StoreStandards(文件标准) }
```

- `Resource.ResourceType` 由插件在创建任务时声明，主程序写入 `resource.resource_type` 列（NOT NULL）。
- `ResourceTypeRegistry` 内置 6 种资源类型（image/video/article/document/audio/unknown）；插件可经 manifest `resourceTypes` 段 + `resourceTypeProvider` 通行证声明自定义类型（注册时强校验，见第七节）。
- 前端**纯消费**：渲染/外部打开按 ResourceType 分发，展示主体用后端派生的 `workStore`，零主体决策。

## 二、store_type 七角色（内置封闭枚举）

`store_type` 标识 resource_store 的业务角色，内置 7 角色封闭枚举（插件自定义资源类型本次仅复用内置角色；插件自定义 store 角色延后）。

| store_type | 语义 | generation 典型 |
|---|---|---|
| `image` | 图片（image 资源主体；article 内嵌图多例） | downloaded |
| `document` | 文档文件（article 正文 .md；document 原文件 .pdf/.docx） | article=derived / document=downloaded |
| `thumbnail` | 封面/缩略图（各类型通用） | derived |
| `videoTrack` | 视频轨（video 组成） | downloaded |
| `audioTrack` | 音频轨（video 组成） | downloaded |
| `videoMain` | 视频可播放主体（封装原文件 / 合并产物，单例） | downloaded 或 derived |
| `audioMain` | 音频可播放主体（独立音频资源主体，单例） | downloaded |

> **generation 是 store 实例属性，非 role 属性**：上表"generation 典型"列仅描述性参考。每个 store 行的实际 generation 由**产出方**决定（插件 `StoreSpec.Generation` / `MergeService` / 还原），以 `resource_store.generation` 列为准，**不从 store_type 推断**。同一 store_type 可跨 generation——`videoMain` 可由本地导入产出（downloaded，封装原文件）或由分离流合并产出（derived，MergeService 合成）。generation 决定执行流程：downloaded 走流式 copy + 断点续传 + size 完整性校验；derived 走一次性 + 完成即完整 + 失败整轨重产。

## 三、内置资源类型（6 种）

Roles 记法 `角色(Min~Max)`：Min=最少数量（0=可选，1=必含）；Max=最多数量（0=不限，1=单例）。

### image — 图片资源

- **Roles**：`image(1~1)`、`thumbnail(0~1)`
- **PrimaryRoles**：`[image]`
- **文件标准**：image=图片(`.jpg`/`.jpeg`/`.png`/`.webp`/`.gif`，downloaded)；thumbnail=封面(`.jpg`/`.png`/`.webp`，derived)
- **典型**：pixiv 插画、local 图片、多图（每子资源一个 image resource）

### video — 视频资源

- **Roles**：`videoMain(1~1)`、`videoTrack(0~1)`、`audioTrack(0~1)`、`thumbnail(0~1)`
- **PrimaryRoles**：`[videoMain]`（唯一可播放主体）
- **文件标准**：videoMain=可播放主体(`.mp4`，封装原文件 downloaded / 合并产物 derived)；videoTrack=分离流视频原料(`.mp4`，downloaded)；audioTrack=分离流音频原料(`.m4a`/`.mp3`/`.aac`，downloaded)；thumbnail=封面(derived)
- **典型**：本地封装视频（仅 videoMain）；bilibili 分离流（videoTrack+audioTrack，合并后增 videoMain）

### article — 图文紧密结合文档

- **Roles**：`document(1~1)`、`image(0~不限)`、`thumbnail(0~1)`
- **PrimaryRoles**：`[document]`
- **文件标准**：document=专栏正文(`.md`，derived)；image=内嵌图(`.jpg`/`.png`/`.webp`，downloaded)；thumbnail=封面(derived)
- **典型**：bilibili 专栏（cv，正文 markdown + 位置相关内嵌图）

### document — 现成文档原文件

- **Roles**：`document(1~1)`、`thumbnail(0~1)`
- **PrimaryRoles**：`[document]`
- **文件标准**：document=现成文档(`.pdf`/`.docx`/`.doc`/`.txt`/`.rtf`，downloaded)；thumbnail=封面(derived)
- **典型**：站点提供的现成文档文件

### audio — 音频资源

- **Roles**：`audioMain(1~1)`、`thumbnail(0~1)`
- **PrimaryRoles**：`[audioMain]`（唯一可播放主体）
- **文件标准**：audioMain=可播放音频主体(`.mp3`/`.m4a`/`.aac`/`.flac`/`.wav`/`.ogg`，downloaded)；thumbnail=封面(derived)
- **典型**：纯音频资源（独立音频主体；audioTrack 仍属 video 分离流原料，不复用为 audio 主体）

### unknown — 无法分类（合法显式值）

- **Roles**：无约束
- **PrimaryRoles**：无（前端走扩展名嗅探兜底）
- **语义**：插件确实无法分类时声明。是合法显式值（与"空/未声明"不同：空是错误，unknown 是显式选择）。

## 四、声明契约（插件 Create 必须遵守）

1. **必须声明 ResourceType**：`TaskCreateResponse` / `TaskCreateChildResponse` 的 `ResourceType` 字段必须填内置值之一（`image`/`video`/`article`/`document`/`audio`/`unknown`）或插件经 `resourceTypes` 段注册的自定义类型。

   ```go
   // 插件 SDK 常量（真相源 SDK contract 子包，主程序 entity/dto 别名 re-export，改一处即处处同步）
   sdkdto.ResourceTypeImage / Video / Article / Document / Audio / Unknown
   ```

2. **严格识别**（主程序不推断、不兜底）：
   - `ResourceType` 空 / 未注册值 → **写入路径抛错**（任务创建/执行失败）
   - `store_type`（StoreSpec.Role）非七预定义角色 → **写入路径抛错**
   - `unknown` 是合法显式值，不报错
3. **store 角色产出**：插件 `Start` 返回的 `StoreSpec.Role` 必须用七预定义角色，且与声明的 ResourceType 的 Roles 一致（软不变量：Roles 角色 ⊆ 实际产出，以实际产出为准）。
4. **多图一致性**：每个子 resource 独立声明 ResourceType（resource 级，无 work 级约束）。

## 五、完整性（ResourceComplete 三态）

主程序在资源下载完成时（`downloadLoop` 全部完成）按 ResourceType 的 Roles 基数做结构校验，写入 `resource_complete`：

| 值 | 语义 | 前端表现 |
|---|---|---|
| `0` | 未校验（未知 ResourceType / 校验未激活） | 不展示（不阻断） |
| `1` | 完整（每角色数量 ∈ [Min,Max]） | 正常展示 |
| `2` | 不完整（缺角色或超量） | "不完整"徽标提示，**不阻断打开** |

校验为纯集合运算（`ValidateResourceStructure`），无 IO，不校验文件内容（成本过高）。自定义类型注册时声明的 Roles 即其完整性规则，零计算逻辑改动自动跟随。

## 六、消费者

| 消费者 | 用法 |
|---|---|
| **插件开发** | 看本文档，声明 ResourceType + 产出对应 Roles 的 store（详见 `doc/plugin-dev-guide.md`） |
| **主程序·完整性** | `ValidateResourceStructure` 结构校验写 ResourceComplete |
| **主程序·展示主体** | `ResolvePrimaryStore` 按 PrimaryRoles 派生 `workStore`（DTO 便捷访问器） |
| **前端·渲染** | ResourceViewer 按 ResourceType 分发：先查 HandlerRegistry（插件渲染器命中则覆盖内置），否则内置渲染器（image→el-image / video→`<video>` 内联播放 / audio→`<audio>` 内联播放 / article→markdown / document·unknown→占位+外部打开） |
| **前端·外部打开** | 按 ResourceType 选 `OpenImage`（图片应用内查看）/ 系统默认 `Open`（视频/音频/文档） |
| **前端·板块勾选** | 按 store_type（StoreRole）勾选，与 ResourceType 分发正交 |

## 七、扩展（T3 已落地）

插件可声明**自定义资源类型**，经注册进 `ResourceTypeRegistry` 纳入可识别范围。机制：

1. **声明载体**：plugin.json `resourceTypes` 段（结构化声明：type + roles + primaryRoles）+ `capabilities` 含 `resourceTypeProvider` 通行证。要求主程序 `contractVersion`≥3。
2. **注册时强校验**（守卫严格识别不变量）：type 强制反向域名前缀（如 `com.example.xxx`，防抢占内置名）；`roles.storeType` 必须 ∈ 内置 7 角色（插件自定义 store 角色延后）；Min≤Max；primaryRoles ⊆ roles。坏 spec 拒绝并记日志跳过，不株连插件其他能力。
3. **同名冲突**：两插件声明同 type 值 → 后注册者拒绝 + 日志告警。
4. **渲染/完整性自动跟随**：自定义类型注册后，前端 resourceViewer 按该类型查找插件渲染器（命中则覆盖内置）；完整性校验按声明的 Roles 基数自动判定。无插件渲染器则降级 UnknownRenderer。
5. **卸载**：插件卸载时反注册其类型；内置类型受保护不可卸载。历史 resource 引用已卸载类型时 Lookup 返回 nil，走 unknown 降级（读路径安全，完整性徽标失效）。

严格识别不变量不变：`ValidateResourceType`「在 Registry 中存在」语义，来源从编译期扩为编译期+运行时注册；空/未注册值仍写入路径抛错，不推断不兜底。声明格式与示例见 `doc/plugin-dev-guide.md`。
