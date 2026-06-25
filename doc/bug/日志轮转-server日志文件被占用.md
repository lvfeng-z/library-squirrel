# 日志轮转失败：server.log 被另一进程占用

> 类型：Bug 备忘（待排查）
> 发现日期：2026-06-25
> 严重程度：低（非致命，日志轮转失败但不影响程序运行）
> 发现场景：插件自存信息重构测试（pixiv 登录调试时观察到，**非本次重构引入**）

## 现象

主程序运行期间，日志频繁出现 lumberjack 轮转失败错误：

```
write error: can't rename log file: rename
  E:\code\lvfeng\library-squirrel\log\server.log
  E:\code\lvfeng\library-squirrel\log\server-2026-06-24T19-01-11.839.log:
  The process cannot access the file because it is being used by another process.
```

## 出现条件

- 主程序运行期间，**插件子进程激活后**尤为频繁
- lumberjack 触发日志轮转时（单文件达 2.5MB 阈值）

## 原因分析（初步推断）

lumberjack 轮转需要 `rename server.log → server-<timestamp>.log`，但 `server.log` 被另一进程持有文件句柄，Windows 下 rename 失败。

最可能：**插件子进程（如 `pixiv_plugin.exe`）的日志也写入了主程序的 `server.log`**——主程序 + 多个插件子进程同时持有该文件，轮转时无法 rename。

其他可能：
- 主程序内多个 logger 实例指向同一文件
- 日志输出的文件句柄未做独占/共享控制

## 影响

- 日志轮转失效，`server.log` 持续增长（不被切分）
- 非致命：程序功能正常，仅日志维护受影响
- 长期运行可能产生超大日志文件

## 排查方向

1. **确认插件子进程日志去向**：插件 SDK 的 `Infof/Debugf/...` 等日志方法是写主程序日志系统（gRPC 转发到主进程写文件），还是子进程自行写文件？若是后者且指向同一 `server.log`，即为占用源。
2. **检查 logger 初始化**：`backend/base/logger` 的 lumberbeck 配置，确认主程序是否单点持有 `server.log`。
3. **Windows 文件共享模式**：lumberjack `os.OpenFile` 的 flag/share 模式是否允许其他进程读取/写入同名文件。

## 修复建议（待定）

- **方案 A**：插件子进程日志独立（各子进程独立日志文件，或仅 stdout 由主程序 stdio 收集后统一写），不直接写主 `server.log`。
- **方案 B**：lumberjack 轮转容错——rename 失败时降级（如继续写当前文件、跳过本次轮转），避免反复报错刷屏。
- **方案 C**：调整文件打开的共享模式，允许多进程 rename。

## 相关

- 日志配置：`CLAUDE.md`「日志」节、`backend/base/logger`
- 插件日志：插件 SDK `PluginContext.Infof/Debugf/...`（见 `.claude/rules/plugin.md`「插件 SDK 能力边界」）
