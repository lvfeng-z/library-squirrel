# 日志轮转失败：server.log 被另一进程占用

> 类型：Bug（已定位并修复）
> 发现日期：2026-06-25
> 修复日期：2026-06-27
> 严重程度：低（非致命，日志轮转失败但不影响程序运行）
> 发现场景：插件自存信息重构测试（pixiv 登录调试时观察到，非本次重构引入）

## 现象

主程序运行期间，日志频繁出现 lumberjack 轮转失败错误：

```
write error: can't rename log file: rename
  E:\code\lvfeng\library-squirrel\log\server.log
  E:\code\lvfeng\library-squirrel\log\server-2026-06-27T06-49-37.252.log:
  The process cannot access the file because it is being used by another process.
```

伴随特征：

- `log/server.log` 持续增长、**从未成功轮转**（目录中无 `server-*.log` 备份）
- 插件子进程激活后报错更频繁

## 根因

**`logger.Reinit()` 泄漏了 `logger.Init()` 创建的旧 lumberjack 实例，旧实例从启动起持续持有 `server.log` 的文件句柄，导致活跃实例每次轮转 rename 都失败。**

时序：

1. `main.go` 启动时 `logger.Init()` 创建 lumberjack 实例 A，打开并持有 `server.log` 句柄。
2. 配置加载后 `logger.Reinit()` 仅调用 `Sync()`（只 flush zap 缓冲，不关闭 A），随即又创建实例 B。
3. A 的句柄从此常驻（lumberjack 无 finalizer，要到进程退出才释放）。
4. B 写满阈值触发 `rotate`：先关闭自身句柄，再 `os.Rename(server.log → server-ts.log)`。
5. 此时 A 的句柄仍打开着 `server.log`。Windows 上 `os.Rename`（底层 `MoveFileEx`）对被其他句柄打开的文件，即使该句柄带 `FILE_SHARE_DELETE`，rename 仍会返回 sharing violation——`SHARE_DELETE` 主要保证 `DeleteFile`，对"持续写入中"文件的 rename 不可靠。
6. rename 失败 → B 的 Write 报错被 zap 静默吞掉 → B 下次 Write 经 `openExistingOrNew` 重新打开旧文件继续追加 → 再次触发轮转 → 再次失败，循环往复，从未成功。

### 排除的假设

- **插件子进程直写 `server.log`**：已排除。插件 SDK（`library-squirrel-sdk/transport/logger.go`）日志走 gRPC 转发到主程序，由主程序端 `backend/plugin/extension/loader.go` 的 `LogFunc` 统一写入全局 logger；SDK 内无任何 `os.OpenFile/server.log` 调用，`exec.Command` 也未重定向子进程 stdout。所有日志（主程序 + 全部插件子进程）汇入主程序单一 lumberjack 实例。
- **外部进程占用**：已排除（无多实例、无编辑器/tail、非 OneDrive 同步目录）。"从未成功轮转"也指向稳定的内部句柄占用，而非瞬时的外部扫描。

### 各观察的对应

| 观察 | 解释 |
| --- | --- |
| 启动时即被占用 | A 在 `Init()` 时打开句柄 |
| 程序内多个句柄占用 | A（泄漏）+ B（活跃）= 同一文件的两个独立 OS 句柄 |
| 从未成功轮转 | A 永不释放，B 每次 rename 都撞上它 |
| 插件激活后频繁 | 插件日志经 gRPC 汇入主程序，写入量暴增，更快触达阈值 |
| 文件卡在略超阈值（如 2147KB） | 轮转被触发但 rename 失败，文件继续追加 |

## 附带发现

`backend/config/default_config.yaml` 的 `log.maxSize: 2.5` 是无效值——lumberjack 的 `MaxSize` 字段为 `int`（单位 MB），`2.5` 经 Viper 解析被截断为 `2`。各文档中描述的"2.5MB"实际并不存在。已修正为整数。

## 影响

- 日志轮转失效，`server.log` 持续增长不被切分（长期运行产生超大日志文件）
- 非致命：程序功能正常，仅日志维护受影响

## 修复（已执行）

1. **`backend/base/logger/logger.go`**：新增包级 `logWriter io.Closer` 持有当前 lumberjack 实例；`Reinit` 重建前 `Close` 旧实例以释放 `server.log` 句柄；`initWithConfig` 中将新建 writer 记入 `logWriter`。修复后活跃实例成为唯一句柄持有者，轮转时关闭自身句柄即可无障碍 rename。
2. **`backend/config/default_config.yaml`**：`maxSize: 2.5` → `3`。

## 相关

- 日志配置：`CLAUDE.md`「日志」节、`backend/base/logger`
- 插件日志路径：插件 SDK `PluginContext.Infof/Debugf/...`（gRPC 转发），见 `.claude/rules/plugin.md`「插件 SDK 能力边界」
