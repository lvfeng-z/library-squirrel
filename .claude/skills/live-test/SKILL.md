---
name: live-test
description: 实机测试协议——AI 直接驱动真实运行中的应用执行验收测试（CDP 远程调试 + 真实前端链路调用 + 外部文件操作 + 只读断言），把用户的手测负担收敛为最终终审。当开发计划的测试锚定要求「dev 实机手测」、任务到达提交前的实机验证里程碑、或用户要求验证功能在真实应用中的表现时使用。产出一个清单+证据+结论一体的测试报告文件。
---

# live-test（实机测试协议）

## 技能概述

| 属性 | 值 |
| --- | --- |
| 名称 | live-test（实机测试） |
| 作用 | AI 以外部进程身份驱动**真实运行中的应用**（非单测、非 mock），执行验收级测试并产出报告 |
| 解决问题 | ① 手测状态无法记录（报告文件即进度源，跨会话/压缩可恢复）；② 用户不了解 AI 的验证操作（AI 直接执行，用户仅终审） |
| 用户残留职责 | 仅最终终审（看截图/亲点一次/签字），趋近于零 |

## 定位与触发

- **测试阶梯**：单元测试（`go test`，逻辑锚定）→ **本技能（实机测试，验收级）** → 用户终审（签收）。
- 触发时机：开发计划的「测试锚定 / dev 实机手测」环节；提交（/commit）前的实机验证；用户说「实测一下」。
- 与 task-graph 衔接：手测里程碑到达时进入本技能；报告落 `.claude/testing/runs/<YYYYMMDD>-<任务短名>/`，谱系 TREE.md 节点行外链该 run 目录。

## 测试形态基建（启动配方）

应用依赖 WebView2，CDP（Chrome DevTools Protocol）远程调试是外部驱动的唯一通路。**环境变量穿透不了 `wails3 dev` 的进程链**，因此手动分层启动：

```bash
# 0. 清残留检测（tasklist 的 IMAGENAME 通配过滤不可靠，勿用；占用者身份两步直判）：
#    wmic process where "name like 'library-squirrel%'" get processid,commandline   # 应用实例
#    netstat -ano | grep ":9245 .*LISTENING"  → 得 PID → wmic process where processid=<PID> get commandline
#    注意：监听者可能是用户活跃 dev 栈（msedgewebview2 子进程的 --webview-exe-name 可证归属）——杀前确认为残留，活跃实例问用户
# 0b. 端口避让：9245 被占且不便清理时，测试栈整体换端口（vite --port 9246 + FRONTEND_DEVSERVER_URL=http://localhost:9246），不碰用户进程
# 1. vite dev server（端口 9245）：
cd frontend && npx vite --port 9245 --strictPort          # 后台
# 2. 测试二进制（CGO 必须开——sqlite；改过 Go 代码须重建）：
go build -o bin/library-squirrel-test.exe .                # ~1min
# 3. 应用（CDP 门控环境变量，main.go cdpArgsFromEnv）：
FRONTEND_DEVSERVER_URL=http://localhost:9245 LS_CDP_PORT=9222 ./bin/library-squirrel-test.exe   # 后台
# 4. 连通性自检：
curl -s http://127.0.0.1:9222/json/version                 # 返回 Browser 版本即通
node .claude/testing/cdp-eval.mjs 'location.href'          # 页面上下文求值
```

工具脚本（`.claude/testing/`，零依赖 Node ≥22）：

| 脚本 | 用途 |
| --- | --- |
| `cdp-eval.mjs '<js>'` | 在应用真实页面上下文求值——可 `await import("/src/apis/http/wrappers/x.js")` 调用**前端 wrapper = 用户点按钮的同一条链路**；可读 DOM（弹框渲染/按钮文案断言）；可读 Pinia store |
| `cdp-shot.mjs <out.png>` | 页面截图（存当前 run 目录 `shots/`，可视觉核验与给用户终审看） |
| `backup-file-op.mjs <dir> <asciiPattern>` | **长路径文件操作**（按 ASCII 通配定位→暂存临时区→删除）：应用产出的文件名常超 Windows MAX_PATH，bash 的 ls/cp/rm/find 全部失效（stat 报错/枚举截断），必须走本脚本（Node `\\?\` 前缀） |

## 协议流程

### 1. 生成测试清单（报告文件即清单）

落点（唯一）：`.claude/testing/runs/<YYYYMMDD>-<任务短名>/VERIFY-<任务>.md`。头部记基建配方与进度计数，每项四要素：

- **目的**（一句话，验收语义）
- **前置**（AI 备好的素材——查库选定/经真实链路造数）
- **操作**（谁做：AI 或用户）
- **断言**（可机检的预期：bindings 返回值 / DOM 状态 / DB 行 / 日志行 / 截图）

**run 目录规约**（run 目录 = 一次测试的全生命周期，报告与证据同目录绑定）：

- 目录名 `YYYYMMDD-<任务短名>`（日期前缀使 `ls runs/` 天然按时间序）；同日同短名冲突追加 `-2`。
- 同目录 `shots/` 存本 run 全部截图、`evidence/` 存本 run 的日志/dump 等文本证据文件。
- 截图命名 `<报告项编号或短代号>-<语义描述>.png`（如 `01-replace-confirm.png`）；复验不覆盖已有文件，追加 `-rN` 后缀（历史证据保留）。
- 报告内截图引用用 Markdown 嵌入语法 `![描述](shots/x.png)`（相对路径，报告与证据同目录，IDE 预览直接见图）；文本类证据仍以行内代码路径提及。
- **遗留冻结区**：旧落点模型的 `shots/`、`reports/`、`evidence/` 三目录已迁私有文档库 `../library-squirrel-docs/testing/` 下（存量报告以原相对结构引用截图，引用链保持可解析），冻结勿往其中新增任何文件；主仓库不再有这三目录，勿重建。

**项数收敛原则**：能自动验证的不设用户项；单元测试已锚定的逻辑不重复实测（记跳过+原因）；用户项只留终审。

### 2. 逐项执行

断言手段（按可信度排序）：

1. **CDP bindings 调用**：`node cdp-eval.mjs '(async () => { const m = await import("/src/apis/http/wrappers/<模块>.js"); return await m.<函数>(参数); })()'`——真实前端链路（Wails bindings → Go handler），返回值即断言数据。查 wrapper 函数名：`frontend/src/apis/http/wrappers/`。
2. **DOM 断言**：`document.querySelector('.el-dialog__title')?.textContent`、按钮 `textContent` 列表——UI 渲染正确性的机检面。
3. **只读 SQL**（sqlite3 MCP 工具，`database/database.db`）：行存在性/字段值终验；不熟表结构先 `pragma_table_info`。
4. **日志 grep**（`log/server.log`）：先 `wc -l` 打标记行号，操作后 `tail -n +<标记> | grep` 断言新增内容。
5. **截图**：渲染类断言存档 + 终审材料；落当前 run 目录 `shots/`，报告内以 `![描述](shots/x.png)` 嵌入引用（文本类证据落 `evidence/`，行内路径提及）。
6. **外部文件操作**：`backup-file-op.mjs`（模拟用户经文件管理器删改——对应用而言与真实外部操作无异）。

**截图有效性铁律**（2026-09-04 空截图事故增设）：

- **截图前必须过「页面就绪」机检**——断言被测内容已在 DOM（目标元素数/文本/数据 store 非空），未就绪则等待重试（如 `await new Promise(r=>setTimeout(r,500))` 循环 + 上限），仍不就绪禁止以该截图作通过证据并在报告标注「未能驱动就绪」。
- **截图结论必须与 DOM/数据断言交叉一致才可采信**；矛盾时以机检断言为准。禁止仅凭「看图」下通过结论——图像解读存在把空页面读成有内容的失败实例。
- **交互驱动加载的页面须先驱动交互**：本项目主页作品列表无自动首查（触发点=搜索按钮/滚动触底/加载更多，`elScrollbarBottomed` 指令仅监听 scroll 事件、mounted 不触发）——CDP 形态下须先程序化驱动（`el.dispatchEvent(new Event('scroll'))` 模拟触底、或点击「搜索」按钮、或直接调组件内查询函数），再等 DOM 就绪再截图/断言。「bindings 有数据但 DOM 空」≠后端缺陷，先排查前端加载触发形态。

**造数纪律**：测试素材一律经**真实链路**造（bindings 调用软删作品/卸载插件等），禁止直写运行中的库；素材选测试号数据（如 site=2 测试下载、`[test01]_` 前缀）或可再生资源。

### 3. 失败循环

断言不过 → 诊断（diagnose 技能纪律：追数据流、不编日志）→ 修复 → 该项标回「已修复待复验」→ 复验。全部记入报告「失败与修复记录」节。

报告的「失败与修复记录」「关键发现」类内容必须两级显式标注：**已断言**（有证据采集落盘：SQL/DOM/日志/截图）与**推断**（解释性结论、未单独验证）。推断级内容不得直接转化为修复方向/修复建议供下游采纳——修复方向须另经代码锚点核码定准（先例：谓词命名与语义失配令未验证推断经「发现→建议」通道被下游当事实采纳）。

### 4. 清理

- **回收全部测试进程**（红线级）：独立 vite 实例（占 9245 的 node.exe）与测试应用二进制都要杀干净——漏杀 vite 会以 `bind: Only one usage of each socket address` 阻塞用户的标准 `task dev`；
- 测试素材经真实链路回收：purge 测试作品（级联清 store 与备份）、恢复被改状态；
- `backup-file-op.mjs` 暂存临时区的文件副本删除；
- 终态核对 SQL（测试创建的行全零）；
- 测试进程保留（供用户终审）或按用户习惯恢复标准 dev 形态。

### 5. 报告与终审

清单文件补「结论」节（通过面/证据摘要/残余风险）→ 请用户终审（看截图或亲手复现一个关键项）→ 签收后进入提交。报告常驻 run 目录，不随谱系归档移动。

## 红线

- **绝不直写运行中的数据库**（fixture 走真实链路，断言走只读查询）。
- **外部文件操作走 backup-file-op.mjs**（长路径陷阱；ASCII 通配定位，不传输 Unicode 文件名——bash 传输层会损坏中文/特殊引号）。
- **改 Go 代码后必须重建测试二进制并重启应用**（本形态无热重载；前端改动 vite 即时生效）。
- **CDP 仅测试实例开启**（`LS_CDP_PORT` 未设置零影响；勿在标准 dev/生产形态常开）。
- 一次通过≠跳过断言——每项证据必须实际采集并落盘。

## 已知边界（实测探明，勿重趟）

- **桌面模式 bindings 无 TCP 出口**（WebView2 内部拦截，应用监听端口全是非 HTTP 管道）——外部驱动只有 CDP 一条路。
- **wails `-tags server` 在 alpha.98 于 Windows 编译不过**（符号重声明）——server 模式路线不可用。
- **`CGO_ENABLED=0` 构建因 sqlite 不可用**——测试二进制必须默认 CGO 构建。
- **环境变量（含 WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS）不穿透 wails3 dev 进程链**——须直启二进制；故 CDP 门控做进 main.go（`LS_CDP_PORT`）而非依赖官方 env。
- **el-dialog 弹框断言**：用 `.el-dialog__title` / `.change-item` 等实际选择器；Element Plus 弹框常驻 DOM，须结合 store list 长度或可见性判断。

## 与其他体系的衔接

- **task-graph**：手测里程碑触发本技能；报告落主仓 runs/ run 目录，谱系 TREE.md（在私有文档库）节点行以 `../library-squirrel/.claude/testing/runs/<dir>/` 路径外链（私有库根相对）。
- **plan 文档**：清单项从方案的「测试锚定」节提取；结论回填提交信息。
- **commit**：本技能报告通过 + 用户终审 = 提交前置条件之一。
- 首跑范例：`../library-squirrel-docs/workflow/archive/work-lineage-soft-delete/VERIFY-H.md`（属旧落点模型遗留——当时报告落谱系目录随归档收存；新报告一律落 runs/）。
