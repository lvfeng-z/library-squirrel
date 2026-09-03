# storeRegistry 模块说明

## 一句话职责

store 域协作中介：① store 子目录白名单单一源（`RegisteredDirs`/`ValidatePath`/`InScanDirs`）+ 备份根单一源（`BackupDirPath`/`InBackupDir`）；② 运行时操作抑制集（`Suppress`/`Release`/`IsSuppressed`），让 fsmonitor 区分软件自身写入与外部文件操作。

## 对外能力

| 能力 | 用途 |
| --- | --- |
| `RegisteredDirs` / `RegisteredPaths` | store 子目录清单（store 域白名单单一源；`RegisteredPaths` 为其 `[]string` 派生视图） |
| `ValidatePath(relPath)` | 落盘前路径校验（persistentStore 落盘点调用；store-only，backup 路径被拒绝） |
| `InScanDirs(rel)` | 路径是否命中 store 白名单子树（fsmonitor 对账扫描 + USN 过滤的 store 域口径） |
| `BackupDirPath` / `InBackupDir(rel)` | 备份根常量与子树谓词（fsmonitor backup 域监控范围单一源；`backup.BackupRootDirName` 引用该常量） |
| `Suppress(relPath)` / `Release(relPath)` | 操作抑制登记 / 宽限释放（写方调用；store/ 与 backup/ 两域写方共用） |
| `IsSuppressed(relPath)` | 查询路径是否被抑制（fsmonitor `handleFileChange` 调用，含祖先前缀匹配） |
| `SetSuppressEnabled(bool)` | 紧急回滚开关（`settings.fsmonitor.suppressEnabled` 注入） |

## 依赖关系

- 仅依赖标准库（`path/filepath`/`strings`/`sync`/`sync/atomic`/`time`），无业务模块反向依赖。
- 被依赖：persistentStore（白名单校验 + 写方登记）、backup（备份根常量引用 + backup/ 文件操作抑制登记）、fsmonitor（两域扫描过滤与事件路由 + 读方抑制查询）、app.go（开关注入）。

## 关键设计

- **白名单单一源**：`RegisteredDirs` 为权威（store 域），`RegisteredPaths` 从其派生，`ValidatePath`/`InScanDirs` 都基于 `RegisteredDirs`——消除历史上 `persistentStore/dir.go` 与 `fsmonitor/scanner.go` 的双份镜像。backup 根（`BackupDirPath`）与 store 白名单分立——backup 不参与 persistentStore 落盘校验，仅作 fsmonitor backup 域监控范围谓词。
- **操作抑制（suppression）**：写方在产生 fsnotify Create/Remove 事件的磁盘操作（`os.Create`/`os.Remove`/`os.Rename`）前 `Suppress`，`Release` 走 3s 宽限期覆盖 fsnotify 异步延迟；读方 `IsSuppressed` 查精确 + 祖先前缀。键为 workDir 相对正斜杠路径（与 `FileChange.Path` / DB `file_path` 同基准）。仅作用于 fsnotify 实时事件，离线对账不经抑制。详见 `../library-squirrel-docs/plan/store操作抑制suppression方案.md`。
- **事件模型前提**：fsnotify 只对 Create/Remove 发事件（Write 不发），故抑制只需覆盖"文件出现/消失那一瞬 + 延迟"，不需覆盖整个写入过程——各落盘点局部登记即可，storeWriter 不持有抑制状态。
- **泄漏兜底**：`Suppress` 设 30s 最长存活，防写方崩溃/忘 Release 致永久抑制；写操作时顺带惰性清理过期项。
