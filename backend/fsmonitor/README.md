# fsmonitor 模块说明

## 一句话职责
感知软件外部对工作目录（workDir）的文件操作（移动/重命名/删除/目录改名），通知用户并提供一键修复（同步 DB 路径 / 复原 / 确认失效）。覆盖**运行时**（事件驱动）与**启动时**（离线对账）两个时机。

## 边界
- 与 persistentStore：persistentStore 管文件落盘 + DB 记录的全生命周期；fsmonitor 监控外部变更并**编排修复**（经接口注入 persistentStore 的查询/修复能力，不直接写其表）
- 不溯源：只关心"文件变了 + 怎么修"，不追踪"哪个进程改的"

## 对外接口（Handler）
| 方法 | 作用 |
| --- | --- |
| `ListPendingChanges` | 列出待修复变更（供前端确认列表） |
| `ConfirmChange(id, action)` | 用户确认修复：`sync`(同步路径) / `restore`(复原) / `ack`(确认失效) |

前端另监听 Wails 事件 `fsmonitor:change`（变更发生时实时推送，data 含待修复 id/kind/fromPath/toPath）。

## 核心概念
- **FileChange**：原始文件变更（Create/Remove，相对 workDir 的正斜杠路径，平台无关语义；Move 仅 USN 离线产出——已配对的旧→新路径，运行时 fsnotify 不产）
- **SemanticChange**：关联后的语义变更（`Move` 文件移动 / `Delete` 删除 / `Untracked` 外部新增 / `DirMove` 目录改名）
- **能力接口（Deps 注入，nil = 降级）**：`LiveEventSource`(实时事件) / `OfflineChangeProvider`(USN 离线追溯 `usn_provider_windows.go`，Windows 管理员可选、D8 开关门控，C-4) / `ReconciliationScanner`(离线对账) / 指纹(`util/fingerprint.Computer`，`NewPlatformDeps` 自建) / `StoreReader`+`StoreRepairer`(DB 读写)
- **RepairManager**：待修复变更队列 + 用户确认执行

## 依赖关系
- 依赖（接口注入，app.go 适配）：persistentStore（`StoreReader` 查记录 / `StoreRepairer` 改路径+失效）、settings（workDir 闭包）、WailsEventEmitter（闭包延迟）；指纹由 `util/fingerprint` 提供（`NewPlatformDeps` 自建注入 correlator）；白名单扫描范围由 `storeRegistry` 提供
- 被依赖：前端 `ChangeConfirmDialog`（确认 UI）、`MainIpcListener`（事件监听）

## 关键设计
- **指纹落库是移动匹配的必要条件**：Missing 文件已不在磁盘无法现场算指纹，故 `persistent_store` 落盘完成时同步算 `content_fingerprint`（size + 头部 64KB SHA256，几毫秒，不异步）
- **fsnotify Windows rename 只发 Create(新名)**：`renamedFrom` 字段未导出、旧名不发事件；移动配对全靠指纹匹配 DB（Create 新名 → 算指纹 → 查 DB 同指纹旧记录 → Move）
- **目录改名检测**：目录 Create 触发下级扫描（采样 50 文件算指纹配对 DB）→ 聚合最常见旧目录前缀 → `DirMove`；修复用 `GLOB` 批量 REPLACE 下级路径前缀（走 `file_path` 索引，`LIKE` 默认不走索引）
- **路径分隔符统一正斜杠**：DB `file_path` 规范正斜杠（`NormalizeFilePaths` 启动迁移）；`filepath.Dir` 在 Windows 会规范成反斜杠，必须 `ToSlash` 还原
- **操作抑制（suppression）**：`handleFileChange` 关联前查 `storeRegistry.IsSuppressed`，命中即丢弃——避免软件自身的 store/ 写入（下载落盘、合并、还原等）被误报为外部变更。写方（persistentStore 各落盘点、repair 复原）在 Create/Remove/Rename 前向 storeRegistry 登记路径。仅作用于 fsnotify 实时事件，离线对账不经抑制。`settings.fsmonitor.suppressEnabled`（D7）可紧急关闭。
- **降级**：能力 nil（如实时事件源不可用、网络盘）时上层走降级路径（仅离线对账 / 关联降级为原始事件日志）
- **USN readFRN 缓冲须堆分配**：`FSCTL_READ_FILE_USN_DATA`（经文件句柄读单文件/目录 FRN，非管理员可用）属 METHOD_NEITHER，`DeviceIoControl` 输出缓冲必须堆分配（`make([]byte, 64KB)`），栈缓冲（`var buf [N]byte`）触发 `ERROR_INVALID_USER_BUFFER`（C-0b PoC 踩坑）。FRN→路径缓存只存目录（路径解析只查 ParentFRN，恒为目录）
