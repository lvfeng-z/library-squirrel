# pixiv 规则模式 VPN 不可访问：Go 仅读 env 代理、不读 Windows 系统代理

> 类型：已修复（根因确证 + 运行期验证通过）
> 定位日期：2026-07-12 · 修复完成：2026-07-14 · 运行期验证：2026-07-14（用户实测规则模式 VPN 恢复访问）
> 严重程度：高（规则/系统代理模式 VPN 用户完全无法访问 pixiv，仅全局/TUN 模式可用）
> 范围：`library-squirrel-plugin-pixiv`（跨仓库）；主程序/SDK 不动
> 关联：`doc/plan/pixiv-connection-reuse.md`（F 连接复用，与本文档共享 `newPixivTransport` 工厂但属独立任务）

## 1. 现象

使用 VPN 时，pixiv 插件**仅在 VPN 采用全局代理（TUN）模式时**能访问 pixiv 站点；切到规则/系统代理模式则访问失败（任务创建/下载不可用）。

## 2. 根因

pixiv 插件所有 HTTP 出口的代理决策依赖 `http.ProxyFromEnvironment`，**该函数只读 `HTTP_PROXY`/`HTTPS_PROXY`/`NO_PROXY` 环境变量，不读 Windows 系统代理（注册表）**。由此与 VPN 两种模式产生差异：

### 2.1 证据链（代码侧已确证）

1. **插件 5 个 HTTP 出口全用 `http.ProxyFromEnvironment`**（仅 env，无系统代理读取、无自定义 Proxy 函数）：
   - `task_handler.go:452`（图片下载 `pixivRequestFn`）
   - `internal/pixivapi/app_api.go:29`、`browser_api.go:23`、`pixpedia_api.go:22`、`login.go:129`
2. **插件子进程继承主程序 env**：`backend/plugin/extension/loader.go:146` `cmd.Env = os.Environ()`。即插件能否拿到代理 env，取决于主程序进程的 env。
3. **Windows GUI 启动的进程默认没有 `HTTP_PROXY`/`HTTPS_PROXY`**（Windows 把代理配置存注册表 `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`，不进进程 env）。
4. `internal/pixivapi/browser_api.go:31` 的调试打印 `fmt.Printf("[DEBUG BrowserAPI] HTTP_PROXY=%s HTTPS_PROXY=%s ...")` 印证开发者已知插件代理模型是 env-based——但该 `fmt.Printf` 走插件 stdout、不进主日志，排查时不可见。

### 2.2 为何"仅全局模式可用"

| VPN 模式 | 代理生效路径 | Go app（仅读 env）结果 |
|---|---|---|
| **全局/TUN** | 网络层 TUN 接口劫持**所有 TCP**，与 app 代理配置无关 | 直连也被 TUN 接管 → **能访问** |
| **规则/系统代理** | 设置 **Windows 系统代理**（注册表），按域名规则仅代理"走代理的流量" | Go 不读系统代理、env 又为空 → **直连被墙** |

一句话：**规则模式 VPN 设的是"Windows 系统代理"，而 Go 的 `ProxyFromEnvironment` 只认 env、不认系统代理；全局模式在网络层兜底故无感。**

### 2.3 次要可能（叠加或并存）

规则模式若 **DNS 未被代理**，pixiv 域名解析被污染，即便代理修好仍可能失败。需 §3 步骤 3 区分。

## 3. 运行时确认（可选，§2 已高置信）

规则模式 VPN 下：

1. PowerShell：`echo $env:HTTPS_PROXY; echo $env:HTTP_PROXY` → 预期为空（印证 env 无代理）。
2. 注册表 `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`：`ProxyEnable=1` 且 `ProxyServer` 为 VPN 代理地址 → 确认 VPN 设了系统代理（Go 没读）。
3. `nslookup i.pximg.net` → 是否污染 IP（DNS 因素）。

## 4. 修复方案（独立任务，默认开——修访问能力，非 opt-in）

### 4.1 自定义 Proxy 函数（核心）

插件 Transport 的 `Proxy` 从 `http.ProxyFromEnvironment` 换为自定义函数，优先级：

1. **显式代理设置**（插件设置项 `proxyUrl`，最高优先级，兜底自动检测失败的场景）
2. **Windows 系统代理**（注册表 `ProxyEnable`/`ProxyServer`/`ProxyOverride`）
3. **env 变量回退**（`http.ProxyFromEnvironment`，保留对已设 env 用户的兼容）

```go
// pixivProxy 决定 pixiv HTTP 请求使用的代理：显式设置 > Windows 系统代理 > env 回退。
// 替代 http.ProxyFromEnvironment：后者只读 env、不读 Windows 系统代理，
// 致规则模式 VPN（设系统代理）下插件直连被墙。
func pixivProxy(req *http.Request) (*url.URL, error) {
    if explicit := getExplicitProxySetting(); explicit != "" {
        return url.Parse(explicit)
    }
    if sys, ok := readWindowsSystemProxy(); ok && sys != "" {
        return url.Parse(sys)
    }
    return http.ProxyFromEnvironment(req) // env 回退
}
```

### 4.2 Windows 系统代理读取

用 `golang.org/x/sys/windows/registry`（go.mod 已间接依赖 `golang.org/x/sys`）读 `Internet Settings`：
- `ProxyEnable`（DWORD）=1 启用；
- `ProxyServer`（string）解析为 `host:port`（可能含 `http=...;https=...` 多协议格式，取 https/http）；
- `ProxyOverride`（string）→ 映射为 `NO_PROXY` 语义。
- 非 Windows 平台（`runtime.GOOS != "windows"`）跳过系统代理读取，回落 env。

### 4.3 显式代理设置项

`plugin.json` `extensions.settings` 加 `proxyUrl`（string，可选，encrypted=false，group 网络）：用户在自动检测失败时手动指定（如 `http://127.0.0.1:7890`）。

### 4.4 诊断可观测（附带改进）

`browser_api.go:31` 的 `fmt.Printf("[DEBUG BrowserAPI] HTTP_PROXY=...")` 改走 SDK logger（`logger.Warnf`/`Debugf`），使代理决策（最终用了哪个代理源）进主日志可排查。去掉裸 `fmt.Printf`。

### 4.5 与 F（连接复用）的代码局部性

本修复与 `doc/plan/pixiv-connection-reuse.md`（F）**共享 `newPixivTransport` 工厂**：F 引入工厂集中 Transport 调参，本修复把 `Proxy: pixivProxy` 也注入同一工厂。两者**独立任务、独立交付模型**（本修复默认开、F opt-in），但实现上建议协调，避免重复改 Transport 构造点。建议顺序：**先做本修复（高优先级、功能性），F 随后扩展同一工厂**。

## 5. 改动点（全在 `library-squirrel-plugin-pixiv`）

1. 新增 `pixivProxy` 函数 + `readWindowsSystemProxy`（新文件或 `internal/pixivapi/transport.go`）
2. 5 个 Transport 站点的 `Proxy: http.ProxyFromEnvironment` → `Proxy: pixivProxy`（含 F 引入工厂后的统一注入）
3. `plugin.json` 加 `proxyUrl` 设置项；`Activate` 读取
4. `browser_api.go:31` DEBUG 改 SDK logger

## 6. 可能同源：缩略图 HTTP 403

日志曾大量出现 `[pixivSuite] 缩略图下载失败: HTTP 403`。原计划代理修复后复测是否同源，但**事实上已无法测试**：M 节点（资源板块声明 `InvolvedRoles=[main]`）使主程序不再驱动 pixiv 缩略图下载，缩略图请求不再发出，403 无从触发。该问题事实上已被 M 从「不请求」路径消除；是否同源于代理已不可考、亦无实际意义，故关闭本节、不再另立排查。

## 7. 测试计划

> **落地结果（2026-07-14 用户实测）**：规则模式 VPN 下代理修复生效，pixiv 任务创建/下载恢复正常（项 1 通过）。项 2-4 为回归/边界用例，按需复测；项 5 缩略图 403 见 §6。

1. **规则模式 VPN**：修复前不可访问 → 修复后 `pixivProxy` 读到系统代理、任务创建+下载成功。
2. **全局/TUN 模式**：修复前后均可用（回归，确保未破坏网络层路由）。
3. **显式 `proxyUrl` 设置**：填入 VPN 代理地址，规则模式可用（绕过系统代理检测）。
4. **无 VPN 直连**：保持失败（预期，pixiv 被墙），不应误读系统代理导致异常。
5. **缩略图 403 复测**（§6）。

## 8. 不在范围

- F 连接复用（`doc/plan/pixiv-connection-reuse.md`，独立任务，opt-in）
- DNS 污染治理（若 §3 步骤 3 确认存在，另立；app 层难修，多依赖 VPN 的 DNS 代理或 DoH）
- 主程序/SDK 改动（无）
