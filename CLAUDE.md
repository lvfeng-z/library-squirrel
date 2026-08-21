# CLAUDE.md

本文件为 Claude Code (claude.ai/code) 在此仓库中工作提供指导。

## 项目概述

**LibrarySquirrel** 是一个基于 Wails 3 的桌面应用，用于创建和维护基于标签的个人资源库。它从远程站点（如 pixiv）下载资源到本地库，并提供基于标签的搜索。后端：Go，前端：Vue 3 + TypeScript，通信：Wails IPC bindings。

## 项目背景
本项目由[LibrarySquirrel](https://gitee.com/lv__feng/library-squirrel.git)重构而来，本项目依赖[SDK](https://github.com/lvfeng-z/library-squirrel-sdk)，此外存在两个插件[localImport](https://github.com/lvfeng-z/library-squirrel-plugin-local.git)和[pixivSuite](https://github.com/lvfeng-z/library-squirrel-plugin-pixiv-go.git)，以上仓库的开发环境都位于本项目的同级目录下。

## 构建与开发命令

```bash
# 开发（前端热重载，直接运行程序）
task dev                           # 或: wails3 dev -config ./build/config.yml

# 构建
task build                         # 或: wails3 build

# 运行编译产物（直接运行，无调试器）
task run

# 调试运行（Delve 调试器，监听 :2345）
task run:debug

# 一键构建全部插件并更新捆绑安装包 resources/bundled-plugins/
# 插件仓库默认取主仓库同级目录，位置不同时复制 build/plugins.local.example.json 为 build/plugins.local.json 覆盖
task build:plugins

# 仅前端
cd frontend && yarn build          # 生产构建
cd frontend && yarn build:dev      # 开发构建（不压缩）

# 修改 Go handler 后生成 TypeScript bindings
wails3 generate bindings -ts

# Go 测试
go test ./...
go test ./backend/util/...         # 单个包

# 服务端模式（无 GUI）
task build:server && task run:server
```

## 架构

### 技术栈
- **框架**: Wails v3（Go 后端 + Web 前端原生窗口）
- **后端**: Go 1.26、GORM（SQLite WAL 模式）、Viper（配置）、Zap（日志）
- **前端**: Vue 3 + Composition API (`<script setup lang="ts">`)、Element Plus、Pinia、Vue Router（hash 模式）、Vite 8
- **IPC**: Wails Bind — Go handler 方法自动暴露给前端，TypeScript bindings 自动生成

## 目录结构

```
项目根目录（= RootPath，开发环境为工作目录，生产环境为可执行文件所在目录）
├── main.go / app.go          — 程序入口与 Wails 应用主文件
├── config.yaml               — 应用配置文件（覆盖嵌入的 default_config.yaml）
├── backend/                  — Go 后端（33 个业务模块 + 基础设施模块）
│   ├── base/                 — 公共基础：logger、model（entity/dto）、工具
│   ├── config/               — 配置加载（嵌入 default_config.yaml 作为默认层）
│   ├── database/             — 数据库初始化、BaseRepository、事务工具
│   ├── migration/            — GORM 自动迁移入口
│   ├── plugin/               — 插件加载器、扩展点注册中心
│   └── {module}/             — 各业务模块（一般分为三层：handler → service → repository）
├── frontend/                 — Vue 3 前端
│   ├── bindings/             — 自动生成的 Wails TypeScript bindings（禁止手动编辑）
│   └── src/
│       ├── apis/http/wrappers/ — 封装 Wails bindings 的 API wrapper
│       ├── components/       — 通用组件（DataTable、WorkCard、TagBox）与对话框
│       ├── model/            — TypeScript 类型定义
│       ├── store/            — Pinia Store（主要用于前端全局动态信息存储）
│       ├── views/            — 页面组件（16 个视图，含 BaseView 外壳）
│       └── composables/      — 组合式函数
├── database/                 — SQLite 数据库文件（database.db）
├── log/                      — 日志文件（server.log，lumberjack 自动轮转）
├── resources/                — 资源文件
│   └── bundled-plugins/      — 捆绑插件 ZIP 包（首次启动自动安装）
├── build/                    — 构建配置（多平台：windows/darwin/linux/ios/android）
├── plugin/                   — 插件运行时目录
└── doc/                      — 项目文档与开发计划
```

## 编译与构建

- **构建工具**: Task（Taskfile.yml）+ Wails 3 CLI
- **Windows 编译**: 因 CGO 兼容性问题，直接 `go build` 可能失败，使用 `task build`（内部调用 wails3 build）或 `CGO_ENABLED=0 go build`
- **编译产物**: 输出到 `bin/` 目录
- **多平台**: build/ 下各平台子目录有独立的 Taskfile，通过 `task windows:build` 等调用

## 数据库

- **引擎**: SQLite（WAL 模式），通过 GORM 操作
- **文件位置**: `{RootPath}/database/database.db`
- **迁移**: `backend/migration/migrate.go` 集中注册所有实体，应用启动时自动迁移
- **访问层**: `BaseRepository[T]` 提供泛型 CRUD + 分页，各模块仅定义接口和特殊查询

## 日志

- **库**: Zap（SugaredLogger）+ Lumberjack（轮转）
- **初始化**: 两阶段 — `logger.Init()` 用默认配置启动 → 配置加载后 `logger.Reinit()` 应用用户配置
- **输出**: 文件（`{RootPath}/log/server.log`，JSON 格式）+ 控制台（Console 格式）
- **前端日志**: `{RootPath}/log/frontend.log`（仅 dev）。前端 `console.*` 全级别 + 未捕获异常（`window.onerror`/`unhandledrejection`）经 `frontendLog` handler 批量转发，由独立 `logger.FrontendLog` 实例（DebugLevel 全量落盘、仅写文件、不回控制台）写入；生产构建前端不启用（`import.meta.env.DEV` 为 false），且文件惰性创建（无写入则不产生），故生产环境无此文件。
- **级别**: 文件级别由 `config.yaml` 的 `log.level` 控制（默认 info），控制台始终 debug
- **轮转**: 单文件 2.5MB 触发轮转，保留 3 个备份，30 天过期，默认不压缩
- **使用**: 全局 `logger.Log.xxx()`，无需导入具体实现

## 配置

- **加载策略**: 两层合并 — 嵌入的 `backend/config/default_config.yaml`（基础层）+ 磁盘 `config.yaml`（覆盖层）
- **根目录 config.yaml**: 开发环境配置（服务器、数据库、站点、插件、日志）
- **config/settings.json**: 运行时用户设置（通过 SettingsHandler 读写），含工作目录、下载/作品/插件/回收站设置、向导完成状态、外观主题（`appearance.theme`，由前端 `useThemeStore` 管理）等
- **build/config.yml**: Wails 开发模式配置（端口、前端开发服务器等）
- **默认值**: 通过 viper SetDefault + default_config.yaml 双重保障

## 编码规则（全局）

### 通用

- **注释**: 使用中文，专有名词用对应语言。注释须**自包含**——读者不查阅外部文档即可理解当前代码：
  - 只写代码当前“做什么”与非显然时的“为什么”（用领域语言）；**禁止变更叙事**（“从 A 改为 B”“已移除 X”“原逻辑为”）与**假设性后果**（“防止 X”“否则会 Y”）。
  - **禁止引用计划/工单标签**：“纪律 N”“见 doc/plan/x.md”“fix #123”等只在开发计划里有意义的标识不进注释；需要的背景用领域语言直接写进注释本身。
  - 名称已表达的含义不复述；注释补充名称承载不了的信息。
  - 反例：`// 纪律 4:已完成 role 过滤(防 416)。磁盘已写满(offset>=size)但 store.Status 未对齐 Complete 的 role,按 Range@满 续传会 416`（计划标签 + 假设性后果 + 术语缩写）
  - 正例：`// 剔除已落盘完整的 role(spec.Size>0 且 streamOffsets[role]>=spec.Size)：上游仅按 store.Status==Complete 排除,文件已写满时 Status 可能仍为未完成,故据实际偏移补充过滤`
- **日志**: 输出中文日志，出现专有名词时使用对应语言。

### 决策原则

- **架构一致性优先于局部性能**：跨模块的引用键、接口、数据形态全链统一一种；「省一次查询/少一次 JOIN」级别的局部性能收益不构成引入形态特例的理由——特例并存与方案并存同源，皆为混乱之源。性能优化需实测证据支撑，且解法不得分裂键型。**自行决策时同样遵守**，不得以「实现细节」为由豁免；与「契约直通」（不为局部便利加兼容映射层）同族。

### 领域架构与规则
各板块的架构说明和编码规则已拆分到独立文件，按 globs 自动加载：
- **后端**: `.claude/rules/backend.md` — 模块模式、业务概念、Go 编码规则
- **前端**: `.claude/rules/frontend.md` — 目录结构、TypeScript 编码规则
- **数据库**: `.claude/rules/database.md` — SQLite/GORM 架构、Repository 规则
- **插件**: `.claude/rules/plugin.md` — 插件系统架构

**规约文档**（跨模块领域规约，非单模块 README）：
- `doc/resource-type-spec.md` — 资源类型规约体系（ResourceType 内置 6 类 image/video/article/document/audio/unknown + 插件可声明自定义类型 + store_type 7 角色 + 严格识别 + 完整性三态）；插件声明契约见 `doc/plugin-dev-guide.md`。
- `doc/message-integration-guide.md` — 前端消息系统接入指南（ElMessage/通知中心/对话框/消息提醒四轨机制地图、通知中心与 `announce` 的 API 及生命周期骨架、终态脱离不变量、后端事件转译形态）。新的消息生产者接入前必读。

### Git 提交
- 中文，格式：`类型(范围): 描述`
- 类型：`feat`、`fix`、`docs`、`style`、`refactor`、`test`、`chore`、`build`

## 模块文档

核心复杂模块与命名不直观模块在其目录下维护 `README.md`，描述当前职责、对外接口、依赖关系与关键设计，供快速定位（不读代码即知"是否来对地方"）。简单 CRUD 模块（<300 行且职责单一）无需写。

- **模板**：`doc/module-spec-template.md`（结构骨架、填写要点、维护机制）
- **范围**：只写"是什么 / 提供什么 / 依赖谁"，不重复编码规则（`.claude/rules/`）与变更历史（`doc/plan/`）
- **维护**：修改模块行为时同步更新其 `README.md`
- 已覆盖：`taskManager`、`task`、`work`、`resource`、`plugin`、`backup`、`persistentStore`、`reWorkAuthor`、`reWorkTag`、`recycleBin`、`fsmonitor`、`backupGovernance`

## 添加新功能

1. 创建 `backend/{module}/handler.go`、`service.go`、`repository.go`
2. 在 `app.go` 的 App 结构体中注册 handler 并初始化 service
3. 运行 `wails3 generate bindings -ts` 生成前端 bindings
4. 在 `frontend/src/apis/http/wrappers/` 中创建 wrapper
5. 按需创建页面/对话框组件
6. 新增核心复杂模块时，按 `doc/module-spec-template.md` 创建该模块 `README.md`

## 交互规则
- 当用户意图制定计划或方案时，直接用 Write 工具将计划文件写到 `doc/plan/` 目录下，然后停下来询问用户要执行计划还是要检查计划，不要直接开始执行。
- 写入 `doc/plan/` 的计划/设计文档，首段必须为「审查摘要」（关键声明带 `file:line` 锚点 / 待决策 / 自曝风险三清单），详见 `doc/plan/_CONVENTIONS.md`；凡"已核验/已就位/零改动"类论断无锚点视为未核验。
- 当本次对项目的修改达到某个里程碑或完成时，请判断当前的修改是否导致项目的实际逻辑与你所理解的项目上下文（比如本文档或rule中的规则）出现了差异，如果有差异，则向用户询问是否需要对文档进行同步。
- 当用户描述现象而又没提供日志时，尝试从开发环境的日志文件[log](log)中获取相关日志
- 执行 Bash 命令时，工作目录默认为项目根，禁止添加 `cd <项目根>` 前缀；需要切换目录时使用绝对路径，避免 `cd` 与输出重定向（`2>&1`、`2>/dev/null`、`>` 等）出现在同一复合命令中触发安全检查。
- 当开始一个会层层派生出多层基建任务的功能/修复/重构时，启用 `task-graph` 技能（`/task-graph`）维护宏观任务派生图，防止跨会话/上下文压缩后遗忘根任务、派生链与延后分支。**派生即记**：干活时一旦发现「这需要先做 X」（阻塞 → `derive` 下钻）或「这引出了 Y」（可延后 → `reveal` 记分支），必须先把该任务 `derive`/`reveal` 写进 `.claude/workflow/active/<谱系>/TREE.md` 再继续当前工作——未入树的派生终会被遗忘。**链接同步**：当某节点新建/确定详情文档（`doc/plan/` 或 `<ID>.md`）或状态/焦点变化时，立即在 TREE.md 该节点行同步外链与状态——TREE.md 是导航中枢，节点信息不得滞后（如为节点写完 plan 后必须回树补 `| doc/plan/<x>.md`）。

## 架构分析规则
- 绝不把理论上的缺陷描述为已确认/已观察到的缺陷
- 不要编造日志文本，也不要引用实际并未读取的日志
- 在得出根因结论之前，先完成对数据流的观察验证
- 诊断 SDK/插件问题时，追踪完整路径：proto 定义 → SDK 协议 → 主应用 → 插件实现