# 资源类型规约（Resource Type Spec）

> 资源类型规约体系：**资源类型 → store 角色组合(含基数) + 展示主体优先级 + 文件标准**。
> 插件开发据此声明 ResourceType + 产出对应 Roles；主程序据此结构校验与展示主体派生；前端据此渲染分发。
> 权威实现：`backend/base/model/entity/resource_type.go`（`ResourceTypeRegistry`）。本文档为其人类可读说明，代码改动须同步本文。

## 一、三层结构

```
Resource.ResourceType  (string，NOT NULL，引用一个资源类型)
   ↓ 指向
ResourceTypeRegistry   (主程序中央注册表，封闭枚举，代码常量)
   ↓ 含
ResourceTypeSpec { Roles[](结构角色+基数), PrimaryRoles[](展示优先级), StoreStandards(文件标准) }
```

- `Resource.ResourceType` 由插件在创建任务时声明，主程序写入 `resource.resource_type` 列（NOT NULL）。
- `ResourceTypeRegistry` 是封闭枚举，**插件不能自定义资源类型**（需注入解析逻辑，见第七节待办）。
- 前端**纯消费**：渲染/外部打开按 ResourceType 分发，展示主体用后端派生的 `workStore`，零主体决策。

## 二、store_type 六角色（封闭枚举）

`store_type` 标识 resource_store 的业务角色，随资源类型封闭（自定义 store_type 与自定义资源类型同问题）。

| store_type | 语义 | generation 典型 |
|---|---|---|
| `image` | 图片（image 资源主体；article 内嵌图多例） | downloaded |
| `document` | 文档文件（article 正文 .md；document 原文件 .pdf/.docx） | article=derived / document=downloaded |
| `thumbnail` | 封面/缩略图（各类型通用） | derived |
| `videoTrack` | 视频轨（video 组成） | downloaded |
| `audioTrack` | 音频轨（video 组成） | downloaded |
| `merged` | 合并产物（video 派生，单例） | derived |

## 三、预定义资源类型（5 种）

Roles 记法 `角色(Min~Max)`：Min=最少数量（0=可选，1=必含）；Max=最多数量（0=不限，1=单例）。

### image — 图片资源

- **Roles**：`image(1~1)`、`thumbnail(0~1)`
- **PrimaryRoles**：`[image]`
- **文件标准**：image=图片(`.jpg`/`.jpeg`/`.png`/`.webp`/`.gif`，downloaded)；thumbnail=封面(`.jpg`/`.png`/`.webp`，derived)
- **典型**：pixiv 插画、local 图片、多图（每子资源一个 image resource）

### video — 视频资源

- **Roles**：`videoTrack(1~1)`、`audioTrack(1~1)`、`thumbnail(0~1)`、`merged(0~1)`
- **PrimaryRoles**：`[merged, videoTrack]`（有合并产物优先 merged，否则 videoTrack）
- **文件标准**：videoTrack=视频流(`.mp4`，downloaded)；audioTrack=音频流(`.m4a`/`.mp3`/`.aac`，downloaded)；merged=合并产物(`.mp4`，derived)；thumbnail=封面(derived)
- **典型**：bilibili 视频

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

### unknown — 无法分类（合法显式值）

- **Roles**：无约束
- **PrimaryRoles**：无（前端走扩展名嗅探兜底）
- **语义**：插件确实无法分类时声明。是合法显式值（与"空/未声明"不同：空是错误，unknown 是显式选择）。

## 四、声明契约（插件 Create 必须遵守）

1. **必须声明 ResourceType**：`TaskCreateResponse` / `TaskCreateChildResponse` 的 `ResourceType` 字段必须填预定义值之一（`image`/`video`/`article`/`document`/`unknown`）。

   ```go
   // 插件 SDK 常量（与主程序 entity.ResourceType* 一致）
   sdkdto.ResourceTypeImage / Video / Article / Document / Unknown
   ```

2. **严格识别**（主程序不推断、不兜底）：
   - `ResourceType` 空 / 非预定义值 → **写入路径抛错**（任务创建/执行失败）
   - `store_type`（StoreSpec.Role）非六预定义角色 → **写入路径抛错**
   - `unknown` 是合法显式值，不报错
3. **store 角色产出**：插件 `Start` 返回的 `StoreSpec.Role` 必须用六预定义角色，且与声明的 ResourceType 的 Roles 一致（软不变量：Roles 角色 ⊆ 实际产出，以实际产出为准）。
4. **多图一致性**：每个子 resource 独立声明 ResourceType（resource 级，无 work 级约束）。

## 五、完整性（ResourceComplete 三态）

主程序在资源下载完成时（`downloadLoop` 全部完成）按 ResourceType 的 Roles 基数做结构校验，写入 `resource_complete`：

| 值 | 语义 | 前端表现 |
|---|---|---|
| `0` | 未校验（未知 ResourceType / 校验未激活） | 不展示（不阻断） |
| `1` | 完整（每角色数量 ∈ [Min,Max]） | 正常展示 |
| `2` | 不完整（缺角色或超量） | "不完整"徽标提示，**不阻断打开** |

校验为纯集合运算（`ValidateResourceStructure`），无 IO，不校验文件内容（成本过高）。

## 六、消费者

| 消费者 | 用法 |
|---|---|
| **插件开发** | 看本文档，声明 ResourceType + 产出对应 Roles 的 store（详见 `doc/plugin-dev-guide.md`） |
| **主程序·完整性** | `ValidateResourceStructure` 结构校验写 ResourceComplete |
| **主程序·展示主体** | `ResolvePrimaryStore` 按 PrimaryRoles 派生 `workStore`（DTO 便捷访问器） |
| **前端·渲染** | ResourceViewer 按 ResourceType 分发：先查 HandlerRegistry（插件渲染器命中则覆盖内置），否则内置渲染器（image→el-image / video→`<video>` 内联播放 / article→markdown / document·unknown→占位+外部打开） |
| **前端·外部打开** | 按 ResourceType 选 `OpenImage`（图片应用内查看）/ 系统默认 `Open`（视频/文档） |
| **前端·板块勾选** | 按 store_type（StoreRole）勾选，与 ResourceType 分发正交 |

## 七、扩展（待办 T3）

插件自定义资源类型当前**不支持**（需注入解析/渲染逻辑，现插件扩展点不具备）。封闭枚举版落地后，等真实需求出现时扩展——通过注册进 `ResourceTypeRegistry` 纳入可识别范围（严格识别下，未注册的自定义类型会被抛错）。
