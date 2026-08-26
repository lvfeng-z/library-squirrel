# workdirGuard 模块说明

## 一句话职责
按平台自动装配「阻止外部修改 workDir」的能力引导与探测——Windows 用**受控文件夹访问**（系统级功能）引导用户配置 + 探针探测 workDir 当前可写性；无内置阻止机制的平台（macOS/Linux）返回 no-op 实现，防护退化为 fsmonitor 检测兜底。本模块**只做引导 + 探测，不强制**启用系统防护。

## 对外接口（Handler）
| 方法 | 作用 |
| --- | --- |
| `GetWorkDirGuardInfo(workDir)` | 返回 `Info`（平台机制/受支持否/引导文案）+ `probeOk`/`probeErr`（当前 workDir 可写性探测结果；workDir 为空跳过探测）。前端调用点：设置页工作目录区块 + workDir 保存后 |

## 核心概念
- **Guard 接口**（平台相关，`NewPlatformGuard()` 按 `runtime.GOOS` 装配，范式同 `fsmonitor.NewPlatformDeps`）：
  - `Probe(ctx, workDir) error`：探测 workDir 当前是否可写（被系统保护机制拦截时返回明确错误）
  - `Info() Info`：当前平台防护机制与用户引导文案
- **Info 结构**：`Platform`（windows/linux/darwin）/ `Mechanism`（受控文件夹访问 / 无内置机制）/ `Supported`（该平台是否有可用阻止机制）/ `Guide`（如何配置防护）
- **Windows 探针**：在 workDir 根写固定隐藏文件 `.squirrel_guard_probe` 再删除。失败语义：受控文件夹拦截 / ACL 只读 / 磁盘异常都表现为写失败（ACCESS_DENIED），多因无法区分，错误文案覆盖三类成因。探针路径固定于 workDir 根、不在 store/backup 扫描白名单内，且创建/删除登记 `storeRegistry.Suppress`/`Release`——双保险避免探测自身被 fsmonitor 误报为外部变更

## 依赖关系
- 依赖（接口注入，app.go 适配）：settings（workDir 闭包）、storeRegistry（探针操作抑制登记）
- 被依赖：前端设置页「目录保护」卡片（渲染 Info + 探测结果 + Guide + 重新检测）

## 关键设计
- **受控文件夹访问是系统级功能**：需用户手动开启（管理员 + 知情），本模块不强制——不配置即无「阻止」效果，仅探测能感知到已开启并生效
- **平台边界**：macOS 无受控文件夹对等物（TCC 管应用访问目录、不阻止外部修改）；Linux `chattr +i` 需 root/CAP_LINUX_IMMUTABLE、POSIX ACL 是身份级无法按进程区分——两平台 no-op，`Supported=false`
- **ACL 自主设置不采用**：Windows 上程序可对自有目录改 ACL（Owner 恒有 WRITE_DAC，用户自建目录免 UAC），但 ACL 按身份区分限制不了用户本人（owner 恒可改回、提权可 takeown 绕过），且网络盘 DACL 不可靠——留作未来项（需补 Owner SID 检测、知情确认+一键还原、网络盘排除）
