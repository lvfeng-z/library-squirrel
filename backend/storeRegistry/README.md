# storeRegistry 模块说明

## 一句话职责

`store/` 持久存储域的**存储目录注册表**（白名单单一源）+ 软件自身文件操作的操作抑制登记：各业务域在装配期向本包注册自己的库内持久数据子树，persistentStore 据注册表代管落盘校验与记录行，fsmonitor 据注册表圈定监控范围。纯内存注册表与谓词集合，无 DB、无 Handler、无外部依赖。

## 边界

- 与 **persistentStore**：本包只登记「哪些子树受代管」；落盘、指纹、记录行、删除等代管能力全在 persistentStore。
- 与 **fsmonitor**：注册表只表达 store 域的**正向范围**；监控的排除面（暂存总根等域外目录）由 fsmonitor 装配期 excludeDirs 承载，不进注册表——两者共同决定监控范围，分工不重叠（「仅监控」「域外」目录均不注册）。
- 与 **backup**：`backup/` 是与 store 域并列的另一监控域根（`BackupDirPath` 常量），不参与落盘校验（ValidatePath 拒绝 backup 路径），仅作 fsmonitor backup 域范围谓词。

## 对外能力（无 Handler，供内部调用）

| 方法 | 作用 |
| --- | --- |
| `Register(StoreDir) error` | 注册存储目录。路径须 workDir 相对、正斜杠规范形；同路径重复注册（`ErrDuplicateDir`）、与既有条目父子前缀子树重叠（`ErrPrefixConflict`）、路径非法（`ErrInvalidDirPath`）/Owner 为空（`ErrEmptyOwner`）均拒绝——属装配错误，调用方 fail-fast 阻断启动 |
| `RegisteredDirs() []StoreDir` | 注册表快照（独立拷贝，调用方修改不回流），启动期消费方取本快照遍历 |
| `RegisteredPaths() []string` | 已注册路径列表（`RegisteredDirs` 派生，每次调用现取） |
| `ValidatePath(relPath)` | 落盘前路径校验：命中任一注册子树才放行（内部反斜杠归一） |
| `InScanDirs(rel)` | store 域范围谓词（离线对账扫描、实时事件过滤、USN 过滤共用） |
| `InBackupDir(rel)` | backup 域范围谓词（与 store 域口径分立） |
| `Suppress` / `Release` / `IsSuppressed` | 操作抑制登记/宽限释放/查询：写方在 Create/Remove/Rename 前登记路径，fsmonitor 据此丢弃软件自身写入产生的事件 |
| `SetSuppressEnabled` | 抑制总开关（`settings.fsmonitor.suppressEnabled` 经 app.go 注入，可紧急关闭） |

## 注册契约

- **恒行支撑**：注册目录子树内的文件一律有 persistent_store 行（参与指纹配对与缺失对账）；不存在「仅注册不代管」的形态。
- **注册时机**：装配期（`app.go` `registerStoreDirs()`）、fsmonitor 启动之前——fsmonitor 的对账扫描根与 USN 缓存种子在监控启动时取注册快照一次，晚于监控启动的注册对既取快照不可见（时序守卫测试 `TestStartupSnapshotTakesPostRegistrationState` 锚定）。
- **前缀唯一**：注册目录两两不重叠（父子前缀均拒绝），按 `/` 段边界判定（`store/workX` 不与 `store/work` 冲突）。
- **注册权不开放插件与前端**：插件如需持久文件产出，走宿主入库通道写已注册目录，不得自建存储目录。
- **当前条目**：`store/work`（属主 `resource`，路径与属主声明在 `backend/resource/store_dir.go`——单一源，装配只按调用时序接入）、`store/avatar/local` 与 `store/avatar/site`（属主 `authorInfo`，路径与属主声明在 `backend/authorInfo/store_dir.go`——同款单一源）。守卫探针路径（workdirGuard）不纳入注册表——白名单隐式排除 + 探针自登记抑制双保险；暂存总根不纳入——走 fsmonitor excludeDirs。

## 五处消费面（全部读同一注册表）

| 消费点 | 位置 | 取值时机 |
| --- | --- | --- |
| 写入闸门 `ValidatePath` | `backend/persistentStore/ingest_tx.go:60`（PrepareIngest）、`backend/persistentStore/service.go:271`（CommitStore）、`backend/persistentStore/service.go:144`（UpdateFilePath）、`backend/import/ingest.go:558`（导入回灌） | 每次写路径校验 |
| 离线对账扫描根 | `backend/fsmonitor/scanner.go:95` | 监控启动时快照一次 |
| USN 文件名缓存种子 | `backend/fsmonitor/frn_cache_windows.go:87` | 监控启动时快照一次 |
| 实时事件域过滤 | `backend/fsmonitor/service.go:364` | 按事件动态判定 |
| USN 路径过滤 | `backend/fsmonitor/usn_provider_windows.go:246,251` | 按记录动态判定 |

域边界的另一半：fsmonitor watch 层排除清单 excludeDirs（`backend/fsmonitor/deps.go:55` 装配传入暂存总根）不读注册表——域外目录的排除唯一走 excludeDirs。

## 依赖关系

- 依赖：无（纯内存，标准库 only）
- 被依赖：**resource**（注册 `store/work`）、**app.go**（装配注册入口 + avatar 两条目 inline 注册）、**persistentStore**（ValidatePath 闸门 + 各落盘点抑制登记）、**fsmonitor**（扫描根/USN 种子/事件过滤 + `IsSuppressed`）、**import**（回灌路径校验）、**backup / recycleBin / resource（替换链）/ fsmonitor repair / workdirGuard**（各自文件操作点登记抑制）、**settings**（suppressEnabled 开关经 app.go 注入）

## 测试进程注册模式

注册只发生在装配期（单一窗口），包内无重置入口。消费方测试进程（fsmonitor、persistentStore、import）在各自 `TestMain` 按生产同值注册三条目（`backend/fsmonitor/backup_domain_test.go`、`backend/persistentStore/main_test.go`、`backend/import/main_test.go`），令白名单相关用例以生产口径运行；本包测试用私有 `resetRegistry` 隔离各用例的注册状态。
