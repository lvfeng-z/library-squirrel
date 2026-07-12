# persistentStore 模块说明

## 一句话职责

资源文件的**持久化存储**：管理 `persistent_store` 记录与对应的磁盘文件，提供流式 / 文件写入、按路径查询、删除（可联动备份）。本模块无 Handler，不直接暴露前端，由 task 下载流程、backup、recycleBin 等内部调用。

## 边界

- 与 **resource**：resource 管 Resource 实体（作品的资源元数据抽象）；persistentStore 管具体的文件落盘与 `persistent_store` 记录。
- 与 **backup**：persistentStore 的 `Delete(id, backup)` 可联动 backup 创建备份；StoreBackupOrchestrator 调用本模块的导入 / 删除接口。
- 与 **dir.go**：所有存储路径必须以已注册子目录开头（见下），禁止裸路径。

## 对外能力（无 Handler，供内部调用）

| 方法 | 作用 |
| --- | --- |
| `StoreStream` / `ResumeStream` | 流式写入（支持断点续传，StoreWriter） |
| `Store` / `StoreFromFile` / `StoreFromExternal` | 从 Reader / 本地文件 / 外部文件写入；`StoreFromExternal` 导入前先清同 `file_path` 旧记录（避免 UNIQUE 冲突） |
| `Delete(id, backup)` | 删除记录与文件（backup=true 联动备份）；记录不存在视为已删除，返回 `(0,nil)` 而非错误 |
| `DeleteByFilePath` / `DeleteRecord` | 按路径删除 / 仅删记录 |
| `GetById` / `GetByIds` / `GetByFilePath` | 查询 |
| `Exists` / `IsCompleteByPath` | 存在性 / 完整性校验 |
| `ResolveStorePath` / `GetAbsPath` | 路径解析（relPath → 绝对路径） |
| `BuildVariantPath(sourceRelPath, suffix)` | 路径变换：从源 store relPath 派生变体路径（同目录 + 文件名追加 suffix + 保扩展 + 净化，正斜杠入库），供合并在已有 store 旁派生产物路径 |

## 核心概念

- **StoreWriter**：流式写入句柄，支持 `Complete`（将记录状态置为已完成）/ `Abort`（中止清理：删文件 + 删记录）/ `Sync`。流式写入直接落到最终路径，DB 记录在写入开始时即创建并标记为未完成（`Incomplete`），`Complete` 后才置为已完成（`Complete`）。
- **已注册子目录**（`dir.go`）：路径必须以下列前缀开头——
  `store/resource`（作品资源）、`store/thumbnail`（视频缩略图）、`store/avatar/local`（本地作者头像）、`store/avatar/site`（站点作者头像）。
- **路径基准**：所有相对路径基于 workDir，禁止 `../`、`./` 或绝对路径。

## 依赖关系

- 依赖：workDir 提供者（根目录）
- 被依赖：**task**（下载资源落盘）、**backup**（StoreBackupOrchestrator 的导入 / 删除）、**recycleBin**、**resource**

## 关键设计

- **直接落最终路径 + status 占位**：流式写入直接落到最终路径（不经过临时文件），DB 记录以 `status=Incomplete` 起步，`Complete` 时才置为 `Complete`。未完成文件不靠临时后缀隔离，而是由读取层 `StoreFileHandler`（`backend/assetserver/store_handler.go`）依据 `status` 校验——未完成记录的 `/store/` 请求直接返回 404，从而避免半成品被读取。
- **路径强校验**：`validatePath` 拒绝未注册子目录，统一正斜杠比较以兼容 Windows。
- **图像宽高提取**：`Complete`/`Store`/`StoreFromExternal` 落盘后，若是图片（`util.IsImageExt`）则用 `image.DecodeConfig` 读头部解码填入 `Width`/`Height`（供前端瀑布流精准布局）。解码失败仅记日志、留 0，不阻断入库。
