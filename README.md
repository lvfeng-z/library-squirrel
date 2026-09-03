# Library Squirrel

![License](https://img.shields.io/badge/license-GPL--3.0--later-blue.svg)
![Platform](https://img.shields.io/badge/platform-Windows-blue.svg)
![Go](https://img.shields.io/badge/Go-1.26-00ADD8.svg)
![Vue](https://img.shields.io/badge/Vue-3-4FC08D.svg)

**Library Squirrel** 是一个面向个人的资源库管理工具：它可以高效地把 Pixiv、Bilibili 等站点的作品下载并保存到你的本地资源库，不再担心喜欢的作品被删除，同时以「标签 / 作者 / 作品集」体系组织管理，并提供本地化的搜索与浏览体验，收集的再多也能快速找到。所有数据完全存储在本地，由你掌控。

基于 [Wails 3](https://v3.wails.io/) 构建：Go 后端 + Vue 3 前端的原生桌面应用，通过插件系统扩展站点支持。

## 界面预览

<!-- 截图占位：正式发布前实机截图补入本节，建议存放于 docs/screenshots/ 目录。
     建议截图清单：
     1. 主页（作品瀑布流 + 标签/作者/作品集过滤栏）
     2. 任务页（任务树 + 进度条 + 操作栏）
     3. 站点浏览器（插件提供的站点浏览入口）
     4. 插件管理页（来源/信任标记 + 待办升级）
     5. 设置页（主题切换）
     替换下方斜体行时同步删除本注释。 -->

*界面截图整理中*

## 功能特性

### 资源获取

- **任务式下载**：提交作品 URL 创建下载任务，支持多任务同时下载、暂停 / 恢复、断点续传
- **站点浏览器**：应用内浏览远端站点内容，直接选取作品创建下载任务
- **本地导入**：将本地已有文件导入资源库，纳入统一的标签管理体系
- **多类型资源**：图片、视频（含多轨合并）、图文文章、文档、音频；资源类型可由插件扩展

### 组织与检索

- **双层标签 / 作者体系**：本地标签 / 作者与站点标签 / 作者互相关联，实现跨站点统一搜索
- **标签分类筛选**：站点标签自带角色、同人出处等分类，搜索时可按分类缩小范围
- **作品集**：手动创建或随站点数据生成，支持嵌套子集、封面、合并纳入
- **综合搜索**：按标签、作者、作品集、站点等条件自由组合搜索

### 数据安全

- **回收站**：删除的作品 / 资源 / 作品集进入回收站，可恢复或彻底删除，过期自动清理
- **文件监控**：检测资源库文件被外部移动、删除、改名，提供确认与修复流程
- **备份**：文件变更前自动备份，支持从备份恢复

### 界面与体验

- **多主题**：默认浅色、森林绿、海洋蓝、樱花粉四套主题
- **首次启动向导**：引导完成资源库目录配置与基础设置
- **通知中心与消息提醒**：任务完成 / 失败、外部文件变更确认等消息集中管理，重要待办不遗漏

### 插件生态

- **开放插件架构**：插件为独立进程，通过 SDK 接入宿主能力
- **多种扩展点**：任务处理器、站点浏览器、自定义页面 / 对话框 / 菜单 / 资源渲染器、自定义资源类型、设置项声明
- **第三方插件**：支持从本地安装第三方插件，来源清晰标记，启用前需手动信任

## 下载与安装

### 系统要求

- Windows 10 / Windows 11
- [WebView2 Runtime](https://developer.microsoft.com/microsoft-edge/webview2/)（Windows 11 及较新版本的 Windows 10 已内置，缺失时从微软官网安装）

### 安装

前往 [Releases](https://gitee.com/lv__feng/library-squirrel/releases) 下载最新的安装包，安装后运行。

> 首次启动时，应用会自动安装捆绑插件（Pixiv、Bilibili、本地导入），并引导你配置资源库目录。

## 快速上手

1. **配置资源库**：首次启动按向导指定资源库目录（存放下载文件的根目录）
2. **浏览站点**：从主页进入站点浏览器，浏览 Pixiv / Bilibili 内容，选取作品创建下载任务；也可以直接粘贴作品 URL 创建任务
3. **管理任务**：在任务页查看进度、暂停 / 恢复 / 取消任务
4. **检索浏览**：回到主页，用标签、作者、作品集条件组合搜索，点击作品查看资源详情
5. **整理归档**：为作品挂本地标签、归入作品集；误删的内容可在回收站恢复

## 捆绑插件

| 插件 | 功能 |
| ---- | ---- |
| [pixivSuite](https://github.com/lvfeng-z/library-squirrel-plugin-pixiv) | Pixiv 作品下载与站点浏览 |
| [bilibiliSuite](https://github.com/lvfeng-z/library-squirrel-plugin-bilibili) | Bilibili 投稿视频（多轨）、图文动态、专栏下载 |
| [localImport](https://github.com/lvfeng-z/library-squirrel-plugin-local) | 从本地路径导入文件到资源库 |

## 插件生态与二次开发

本软件的核心能力（站点适配、资源类型、界面扩展）均通过插件系统开放：

- **插件 SDK**：[library-squirrel-sdk](https://github.com/lvfeng-z/library-squirrel-sdk)（MIT 协议），定义插件与宿主的完整通信契约
- **开发指南**：见 [doc/plugin-dev-guide.md](doc/plugin-dev-guide.md)，涵盖插件结构、扩展点声明、前端组件接入、配置与版本约定
- **主题令牌契约**：插件 UI 遵循主程序主题令牌（[doc/plugin-theme-tokens.md](doc/plugin-theme-tokens.md)），自动跟随用户主题

第三方插件可从本地安装包安装：安装后需在插件管理页手动授予信任才会激活；来源与信任状态在插件管理页全程可见。若遇问题，可在设置中开启受限模式（仅加载官方捆绑插件）排查。

## 从源码构建

### 前置要求

- [Go](https://go.dev/dl/) 1.26+
- [Node.js](https://nodejs.org/)（LTS）+ yarn
- [Wails 3 CLI](https://v3.wails.io/)：`go install github.com/wailsapp/wails/v3/cmd/wails3@latest`
- [Task](https://taskfile.dev/)（构建任务入口）

### 常用命令

```bash
task dev                # 开发模式（前端热重载，直接运行程序）
task build              # 构建生产版本（输出到 bin/）
task run                # 运行编译产物
task build:plugins      # 构建全部捆绑插件并更新 resources/bundled-plugins/
task build:server && task run:server   # 服务器模式（无 GUI）
```

> Windows 下因 CGO 兼容性问题，请使用 `task build`（内部调用 `wails3 build`）而非直接 `go build`。
>
> 修改 Go handler 后需执行 `wails3 generate bindings -ts` 重新生成前端 bindings。
>
> 构建插件时，插件仓库默认位于主仓库同级目录；位置不同时复制 `build/plugins.local.example.json` 为 `build/plugins.local.json` 覆盖。

### 项目结构

```
├── main.go / app.go          # 程序入口与 Wails 应用主文件
├── backend/                  # Go 后端（业务模块：handler → service → repository 三层）
│   ├── base/                 # 公共基础（logger、model、工具）
│   ├── database/             # 数据库初始化、BaseRepository、事务工具
│   ├── plugin/               # 插件加载器、扩展点注册中心
│   └── {module}/             # 各业务模块（work、task、search、fsmonitor 等）
├── frontend/                 # Vue 3 + TypeScript 前端
│   └── src/                  # views / components / store / composables 等
├── resources/bundled-plugins/ # 捆绑插件 ZIP 包（首次启动自动安装）
├── doc/                      # 项目文档与开发计划
└── build/                    # 构建配置（多平台）
```

更多架构细节见 [CLAUDE.md](CLAUDE.md) 与 [doc/](doc/) 目录（含资源类型规约、消息系统接入指南、插件开发指南等）。

## 许可协议

本项目采用 GPL-3.0-or-later（GNU 通用公共许可证第 3 版或其后续版本）授权，全文见 [LICENSE](LICENSE)。

Copyright (C) 2026 lvfeng

本程序不提供任何担保——不含适销性或特定用途适用性担保。具体条款详见 [LICENSE](LICENSE)。

> 插件 SDK 与官方插件仓库采用 MIT 协议授权，可自由用于开发你自己的插件。

## 第三方插件声明

本软件采用开放插件架构，支持安装第三方插件。第三方插件由其作者独立开发与维护，用户与插件作者之间就插件产生的一切互动，均发生在用户与插件作者之间，与本软件作者无关；本软件对第三方插件不作担保。第三方插件的问题请向其作者反馈。

插件管理页中的来源与信任标记用于区分官方捆绑插件与第三方安装的插件。
