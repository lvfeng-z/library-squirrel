# Store 落盘命名规约

> 适用于所有 store 的库内落盘目录与文件名——插件产出 store（image/document/thumbnail/videoTrack/audioTrack）与主程序派生 store（videoMain，音视频合并产物）。
> 派生规则的权威实现为 SDK `storepath` 包（`github.com/lvfeng-z/library-squirrel-sdk/storepath`），本文档为该包的行为说明；实施背景与决策见 `../library-squirrel-docs/plan/落盘身份键命名实施方案.md`。

## 落盘布局

```
store/resource/{site_key}_{siteWorkId 派生段}/{role}_{seq 三位零填充}.{ext}

示例：store/resource/pixiv_128937464/image_000.jpg
      store/resource/bilibili_BV1xx411c7mD_4538792/videoTrack_000.mp4
```

- 每作品一目录；所有 store（含缩略图与合并产物）统一进 `store/resource/` 下该作品目录，无按作者/按类型的其他布局层级
- 路径由身份键（站点复合键 + role + seq）确定性派生，站点元数据（作者/作品名/描述等展示字段）不进路径
- **空值严格拒绝**：siteKey/siteWorkId 为空时派生函数显式报错（`ErrEmptySiteKey`/`ErrEmptySiteWorkId`，写入路径严格识别不回落）；role/ext/seq 非法同样显式报错

## 目录段:{site_key}_{siteWorkId 派生段}

目录段保证**单射**——不同 siteWorkId 恒得不同目录段，碰撞在定义上消除：

- **site_key** 原文直用（站点身份注册表 slug，`^[a-z][a-z0-9-]{1,30}$`，键一经发布不可变，见 `doc/site-identity-spec.md`）
- **siteWorkId 派生段**：
  - 净化后等于原文且不超长（≤96 字符）：原文直用
  - 净化后为空：整体退为原始 ID 的 sha256 前 16 位 hex
  - 其余（净化发生变更，或超长 >96 字符）：净化结果截断至前 88 字符 + `_` + 原始 ID 的 sha256 前 8 位 hex
- 净化 = Windows 非法字符替换为全角等价（保形）+ 控制字符删除 + 尾部空格/句点剥离；消歧哈希一律按**原始 ID** 计算——净化与截断都可能令不同 ID 得同段（如全半角形近、长 ID 共享前缀），由哈希段承担区分

## 文件名:恒 {role}_{seq 三位零填充}.{ext}

- **恒带 role_seq 段**，单 store 资源不省略——role 内序号是落盘文件与续传配对的身份键；资源级单/多 store 判定口径不存在（文件名形态不随轨道总数变化，resume 时 specs 子集与全局轨道数不一致也不影响命名）
- **role** ∈ 7 预定义 store_type（`entity.StoreType*`：image/document/thumbnail/videoTrack/audioTrack/videoMain/audioMain）。**thumbnail 是普通 role，无特例**——与 image/document 等同处理
- **seq** = 同 role 内 0-based 挂载序（= `store_seq`，resume 身份键）；超三位自然扩位（如 1000 → `1000`）
- **ext** 取 spec.Format 经 normalizeExt 统一前导点（空串 = 无扩展名，带不带前导点输入均可）
- role 与 ext 是契约输入（非站点任意数据），不做净化变形——非法值（空/控制字符/Windows 非法字符/尾部句点或空格/负 seq）显式报错

## 主程序侧组装

`store/resource/` 前缀属主程序库内布局（storeRegistry 权威），不进 SDK storepath；主程序侧 `path.Join("store/resource", WorkDirName, StoreFileName)` 组合完整 relPath（正斜杠域）。组装点：

- **下载侧**：`backend/download/naming.go` `resolveStoreDir`/`resolveStorePath`——身份输入取领域行站点复合键（siteWorkId 原文 + siteId 反查站点行取 site_key），键缺失或站点行查不到时按执行失败收口
- **merge 产物**：`backend/resource/merge_service.go` `deriveMergedPaths`——同一对派生函数，身份经 resource → work → site 反查站点复合键；videoMain 为单实例派生 store，seq 恒 0（产物恒 `videoMain_000.{ext}`），与下载 store 同口径
- **插件侧推导**：插件可引用 SDK storepath 推导兄弟文件名（如 document lazy 生成时关联的 image），输入 = 任务身份（siteWorkId + identity 常量）+ specs 顺序知识 + ext 重算，本地推导不向主程序查询

## 确定性

- **同键恒同路径**：同一身份键恒派生同一路径——重试/续传/重下（站点内容未变）命中同一文件，续传锚自然成立
- **站点内容更新走替换链**：同键新实例重下命中同路径，由提交点既有顺序「软删移位（受害者文件移出）→ rename 写入」腾位；受害者文件移出先于 rename 写入同路径
- **命名输入禁止时变**：时刻类信息不参与库内命名（资源唯一性由身份键保证，无需时变量）

## resume 配对口径

seq 必须取资源全局 `store_seq`（挂载序），不能用 resume specs 内重计：

- resume 的 specs 是未完成子集（已完成 store 不在其中）
- 同 role 部分完成时，specs 内重计会与全局 `store_seq` 错位 → 配对到已完成轨 → 续传覆盖已完成 store（数据损坏）

specs 顺序确定性是 SDK 显式契约（`doc/plugin-dev-guide.md` 6.1「specs 顺序确定性」条款）：同 role 内 store_seq 由主程序按 Start/Resume 返回的 specs 顺序分配，插件必须保证同 role 的 specs 相对顺序跨 Start/Resume/重试稳定——顺序漂移 = 文件名漂移 = 引用断裂。

## 下载暂存命名(暂存模式)

下载执行面落盘走暂存模式:先写 `{workDir}/task-staging/{taskID}/`,全部轨道写满后提交点统一 rename 到最终路径。暂存文件名与最终名解耦,为 `role_seq` 派生键:

- 形态 `{role}_{seq 三位零填充}{ext}`(如 `videoTrack_007.mp4`),ext 取自 spec.Format(`StagingFileName`)
- 用途:续传定位与崩溃清扫直接按文件名还原 (role, seq) 身份,不依赖元数据重解析;最终名由执行前解析派生,随流控制器(streamController)进入提交点
- 全局 seq 推导:全新执行=specs 全集序,恢复=暂存枚举序(同 role 内按 seq 排队,返回 spec 按角色消费队列)
- `task-staging/` 不在 store/ 白名单子树内,fsmonitor 对其零感知(无需抑制登记);提交点 rename 是白名单内操作,由 download 登记 `storeRegistry.Suppress`
- 恢复时全局 seq 配对:download `pairResumeSpecs` 把插件 Resume 返回的 specs 与暂存枚举轨按 role 配对、依序消费全局 seq(同 role 多轨按序对齐);未被认领的暂存轨交 Start 整轨重产(derived 一次性产物不可续传)

## 导出命名(与库内命名的关系)

导出包内文件名独立于库内命名:按用户设置「导出文件命名格式」模板（`settings.exportSettings.fileNameFormat`，占位符 author/siteWorkId/siteWorkName/uploadTime*/exportTime* 等，详见 `backend/export/namer.go`）以作品字段渲染主名 + 源文件扩展名，同名冲突追加 `_siteWorkId` 后缀、仍冲突追加序号——导出是面向用户的可移植交付物，文件名取可读性；库内 role_seq 形态不进导出包。
