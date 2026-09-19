# staging 模块说明

## 一句话职责
暂存目录作用域能力包：统一暂存总根 `{workDir}/staging/` 的属主根注册表、作用域自证描述（scope.json）读写、原子化创建入口与启动清扫派发——为下载/收件/导入/合并/导出提供一致的暂存位置与生命周期治理。

## 布局与自证
```
{workDir}/
├─ store/            ← 库内资源（store 白名单，fsmonitor 监控）
├─ backup/           ← 备份根（backup 域，fsmonitor 监控）
└─ staging/          ← 暂存总根（不在白名单与 backup 域，fsmonitor 零感知：事件不消费 + watch 层整体免挂——动态补 watch 的打开窗口会与作用域「临时目录→rename 定名」竞态，Windows 上 rename 撞 sharing violation）
   ├─ download/{taskID}/       ← 插件下载任务：role_seq 键暂存文件平铺
   ├─ share-receive/{taskID}/  ← 分享收件任务：父作用域含共享 manifest.json，子作用域为按清单路径镜像命名的暂存文件
   ├─ import/{scopeKey}/       ← UI 回灌导入（根已登记，暂无生产消费方）
   ├─ merge/{scopeKey}/        ← 合并产物
   └─ export/{scopeKey}/       ← 导出：目录内只放描述，物理临时文件留最终产物的目标目录同级
```

- **作用域描述**（每作用域目录内一份 `scope.json`）：`scopeKey`（稳定键，与目录名一致）+ `contentShape`（内容形态标签，创建方定义并自行消费，本包只存取不解释）+ `createdAt`（Unix 毫秒）；export 形态另带账本 `ExportLedger`（`targetDir` 目标目录 + `tempFiles` 临时文件名清单）。
- **作用域键**：任务属主根=任务 ID（目录名与任务行一一对应）；非任务属主根=`MintScopeKey` 铸造的 16 字节随机 hex（非计数器，无需 DB 归属即唯一）。

## 回收策略分型
根注册表（`registry.go` 的 `rootRegistry`）集中登记「属主 → 回收策略」，是新增属主根的唯一登记点；包外不得另建暂存根。

| 属主根 | 策略 | 规则 |
| --- | --- | --- |
| download / share-receive | 归属存活性 | 谓词判活（消费方在装配处注入任务行存在性查询）：活=保留（含暂停待恢复），死=回收；谓词未注入宁留勿毁 |
| import / merge | 启动清空 | 非任务入库是单次操作，进程重启即无在途 |
| export | 描述账本 | 按账本删目标目录临时文件，落定后回收作用域；真失败（目标盘不可达/占用）保留作用域留待下次启动重试。账本缺失（外部改动）时目标侧文件不可定位，作用域照常回收 |

无描述/描述损坏/描述键与目录名不一致/暂存总根下未登记根名/非目录条目一律回收并记 Warn（含非空目录）——暂存域是草稿区，不保无主数据。

## 对外接口（无 Handler；被 app.go 装配与各执行面调用）
| 函数 | 作用 |
| --- | --- |
| `CreateScope` / `CreateExportScope` | 原子化创建作用域（唯一合法创建路径），返回目录绝对路径 |
| `SweepAtStartup` | 启动清扫：按根注册表策略统一派发回收（workDir 空串=未配置直接返回） |
| `RetireLegacyRoots` | 旧版暂存根（workDir 顶层 `task-staging/`、`share-receive/`）一次性整体作废回收 |
| `MintScopeKey` | 铸造非任务属主稳定键 |
| `ScopePath` / `RemoveScope` | 作用域路径派生单点 / 单作用域回收 |

## 边界
- 纯能力包：不感知任何业务实体（任务/作品/资源）——落什么文件、怎么消费暂存归消费方，本包只管位置（根+作用域）、自证（描述）与回收（清扫派发）。
- 旧根不迁移：`task-staging/` 与 `share-receive/` 内容整体作废（存量暂停态任务的半成品随回收丢弃）；`RetireLegacyRoots` 为一次性代码，旧根回收完成后随之可删。
- 作用域路径属 absPath 域（含 workDir 的绝对路径），仅供 os.* 文件系统调用点现场消费、不入库；入 DB 的暂存相对路径（如收件 `manifest_path`）为 relPath 域正斜杠。

## 依赖关系
- 依赖：`base/logger`（清扫日志）
- 被依赖：app.go（启动清扫装配）、task（download/share-receive 两任务属主根的薄封装）、resource（merge）、export——消费方一律经本包入口创建作用域

## 关键设计
- **原子创建**：先建临时名目录、在其内写好描述、再 rename 为正式名（同卷目录 rename 原子），「目录存在 ⟺ 描述在」恒成立；无描述/描述损坏目录只能来自外部改动。绕过本入口自建目录即破坏该不变量。
- **清扫时序契约**：`SweepAtStartup` 须在启动序列中先于任何能创建作用域的服务同步调用（app.go 装配保证）——此后创建的作用域不进本次清扫，不存在在途作用域被误回收的并发窗口。
- **健壮性**：单个回收失败不中断其余目录（记 Warn 留待下次启动重试，幂等）。
