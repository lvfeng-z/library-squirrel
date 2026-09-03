# Store 落盘命名规约

> 适用于插件产出的所有 store(image/document/videoTrack/audioTrack/thumbnail)的落盘文件名与路径。
> 实施背景与决策见 `../library-squirrel-docs/plan/store-naming-convention-plan.md`。

## bas 基准名

**bas** = `FileNameFormat` 模板经占位符替换 + 净化生成的文件名基底(**不含扩展名**)。

- 示例:模板 `[${author}]_[${siteWorkId}]_${siteWorkName}` → bas = `[作者]_[workId]_作品名`
- 模板空时由 `settings.GetFileNameFormat` 回退默认模板 `DefaultFileNameFormat`(D2 方案 B,根治模板空→落盘名退化为 `task.ext`→`StoreStream` 删旧建新无声覆盖)

## 文件名规则

判定口径为**资源级**:`资源 store 总数 > 1` 为多 store,`== 1` 为单 store。

| 场景 | 文件名 | 示例 |
|---|---|---|
| 单 store | `<bas>.<ext>` | `[作者]_[workId]_作品名.jpg` |
| 多 store · 有描述 | `<bas>_<role>_<seq>_<描述>.<ext>` | `[作者]_[workId]_作品名_image_000_封面.png` |
| 多 store · 无描述 | `<bas>_<role>_<seq>.<ext>` | `[作者]_[workId]_作品名_document_000.md` |

- **role**:`image`/`document`/`videoTrack`/`audioTrack`/`thumbnail`(多 store 总带)。**thumbnail 是普通 role,无特例**——与 image/document 等同处理
- **seq**:同 role 内 0-based 序号(= `store_seq`,resume 身份键);多 store 场景总带,单 store 不加
- **描述**:`StoreSpec.Description`(插件可选填,SDK DTO + proto `StoreSpecMeta.description`),净化后为空则省略描述段

## 路径规则

所有插件 store 统一进 `store/resource/<作者>/<文件名>`(扁平,无子目录)。

- thumbnail 普通 role,不再独立 `store/thumbnail/` 目录(前端按 `filePath` 取,路径透明)
- **videoMain**(合并产物,主程序派生)维持 `<bas>_merged.<ext>`(`BuildVariantPath`),不纳入 bas 规约

## 判定口径:资源级(不基于 per-role)

判定基于**资源 store 总数**,不基于 per-role 计数。原因:thumbnail 普通 role 后,per-role 判定下 `image+thumbnail` 各自单例会跨 role 撞名(同 bas 同 ext 同目录)。

- `startDownload`:总数 = `len(specs)`(全量首发)
- `resume`:总数 = `len(storeRows)`(specs 是未完成子集,已完成 store 不在其中;不能用 `len(specs)`,否则部分完成时判定翻转→文件名漂移→续传/重建到错误路径)

## 确定性约束

命名输入禁止时变:`${downloadTime*}` 占位符破坏确定性,规约剥离(不参与命名)。资源唯一性由 `bas + role + seq` 保证,无需时变量。

## resume 一致性

seq 必须取自资源全局 `store_seq`(M 身份化),不能用 resume specs 内重计:

- resume 的 specs 是未完成子集(已完成 store 不在其中)
- 同 role 部分完成时,specs 内 roleCounters 重计会与全局 `store_seq` 错位 → `findStoreRowByIdentity` 匹配到已完成行 → 续传覆盖已完成 store(数据损坏)
- `taskManager.resumeSpecSeq` 按 `streamOffsets`(downloaded,携带全局 StoreSeq)+ `storeRows`(derived,按 role 查未完成行)配对全局 seq
