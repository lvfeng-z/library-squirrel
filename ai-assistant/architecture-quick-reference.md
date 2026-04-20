# LibrarySquirrel 架构速查

## 核心业务概念（一句话总结）

| 概念         | 定义                             | 关键服务                |
| ------------ | -------------------------------- | ----------------------- |
| **站点**     | 远程作品源（如pixiv、bilibili）  | `SiteService.ts`        |
| **作品**     | 资源与信息的集合（核心数据实体） | `WorkService.ts`        |
| **任务**     | 作品创建执行流程                 | `TaskService.ts`        |
| **站点作者** | 来自站点的原始作者               | `SiteAuthorService.ts`  |
| **本地作者** | 本地创建，用于统一跨站点作者身份 | `LocalAuthorService.ts` |
| **站点标签** | 来自站点的原始标签               | `SiteTagService.ts`     |
| **本地标签** | 本地创建，用于统一跨站点标签     | `LocalTagService.ts`    |
| **插件**     | 扩展对不同站点的支持             | `PluginService.ts`      |

## 关键业务逻辑

### 双架构统一检索模式

- **本地作者** ↔ **站点作者**：关联后，搜索本地作者可检索所有关联站点的作品
- **本地标签** ↔ **站点标签**：关联后，搜索本地标签可检索所有关联站点的作品

### 作品下载流程

```
URL输入 → 任务创建 → 插件执行 → 获取作品信息 → 保存作品
                                    ↓
                              创建站点作者/标签
```

### 统一检索优势

- **一次检索，全站结果**：通过本地实体关联实现跨站点统一检索
- **语义统一**：不同站点的相似标签/作者可统一管理

## 技术架构速查

### 前端架构 (Renderer)

- **框架**：Vue 3 + Composition API + Element Plus
- **路由系统**：Vue Router (hash 模式，使用 `createWebHashHistory()`)
- **路由配置**：`src/renderer/src/router/` 目录
  - `index.ts` - Router 实例配置
  - `routes.ts` - 路由定义
- **状态管理**：Pinia stores (`Use*Store.ts`)
- **组件模式**：`<script setup lang="ts">` + Props后缀
- **路径别名**：
  - `@renderer/*` → `src/renderer/src/*`
  - `@shared/*` → `src/shared/*`

### 后端架构 (Main Process)

- **IPC模式**：`ipcRenderer.invoke('service-method', args)`
- **API注册**：`MainProcessApi.ts` 集中注册所有服务方法
- **数据库**：better-sqlite3 + DAO模式 + SAVEPOINT事务
- **自定义协议**：`resource://` 处理本地文件访问
- **路径别名**：`@shared/*` → `src/shared/*`（主进程和渲染进程共用代码）

### 核心目录

```
src/
├── main/                    # 主进程 (Node.js/Electron)
│   ├── service/             # 业务服务层
│   ├── model/               # 数据模型（主进程专用）
│   ├── dao/                 # 数据访问层
│   ├── plugin/              # 插件系统
│   ├── core/                # 核心组件（MainProcessApi, taskQueue）
│   ├── resources/           # 配置文件
│   └── util/                # 工具类
├── shared/                  # 主进程和渲染进程共用代码
│   ├── model/               # 共用实体类、DTO、枚举、常量
│   │   ├── constant/        # 枚举和常量
│   │   ├── dto/             # 数据传输对象
│   │   ├── domain/           # 领域模型
│   │   ├── entity/          # 实体类
│   │   ├── interface/        # 接口定义
│   │   ├── queryDTO/        # 查询DTO
│   │   └── util/            # 工具类型
│   └── util/                # 共用工具函数
├── preload/                 # 预加载脚本 (IPC桥接)
└── renderer/src/           # 渲染进程 (Vue 3)
    ├── router/              # Vue Router 路由配置
    ├── views/               # 视图组件（如 MainLayout.vue）
    ├── components/          # Vue组件
    ├── store/               # Pinia状态管理
    ├── apis/                # IPC API包装器
    └── utils/               # 渲染进程工具类
```

## 开发模式

### 添加新服务

1. 在 `src/main/service/` 创建Service类
2. 在 `MainProcessApi.ts` 注册IPC处理器
3. 在 `preload/index.ts` 添加API包装器
4. 在前端通过 `window.api.serviceNameMethodName()` 调用

### 数据库事务

```typescript
await db.transaction(async (tx) => {
  await tx.run('INSERT ...')
  await tx.run('UPDATE ...')
}, 'operation-name')
```

### 响应格式

- 成功：`ApiUtil.response(data)`
- 错误：`ApiUtil.error(message)`

### 页面导航 (Vue Router)

```typescript
import router from '@renderer/router'

// 编程式导航
await router.push('/settings')

// 或使用路由名称
await router.push({ name: 'Settings' })
```

路由配置示例（`routes.ts`）:

```typescript
export const routes = [
  {
    path: '/',
    component: () => import('@renderer/src/views/MainLayout.vue'),
    children: [
      {
        path: '',
        name: 'Home',
        component: () => import('@renderer/components/main/MainPageWrapper.vue'),
        meta: { title: '主页', icon: 'HomeFilled', order: 0 }
      }
    ]
  }
]
```

## 插件系统要点

- 插件存储在 `plugin/package/` 目录
- 每个插件是独立包（作者、名称、版本）
- 插件通过 `PluginManager.ts` 管理
- `BasePlugin` 接口简化为只包含 `pluginId: number`
- 任务执行由 `PluginTaskResParam.ts` 处理

## 典型开发场景

### 场景1：添加新功能

1. 确定需要哪些Service
2. 创建或修改数据库表（`createDataTables.yml`）
3. 实现业务逻辑（Service层）
4. 注册IPC方法
5. 创建前端组件

### 场景2：修复业务逻辑

1. 定位相关Service类
2. 检查数据库操作（DAO层）
3. 验证IPC调用链
4. 测试前后端集成

### 场景3：扩展插件功能

1. 修改插件接口定义
2. 更新插件实现
3. 调整任务处理逻辑
4. 确保向后兼容

## 快速调试

- **前端调试**：检查 `window.api` 调用和Pinia状态
- **后端调试**：查看 `LogUtil` 输出
- **数据库调试**：检查SQL执行和事务
- **IPC调试**：验证方法名匹配（`service-method` ↔ `serviceMethod`）

---

## Go 主进程架构 (my-ipc-service)

> **项目状态**：正在从 Node.js (`src/main-old/`) 重构到 Go

### 三层架构

```
┌────────────────────────────────────────────┐
│  共享层: pkg/model                         │
│  ├── BaseEntity、Example、API Response      │
│  └── ⚠️ 严禁业务逻辑或数据库依赖            │
└────────────────────────────────────────────┘
                      ↑
┌────────────────────────────────────────────┐
│  基础设施层: internal/database              │
│  ├── BaseRepository[T] 泛型基础仓储         │
│  └── Transaction 事务封装                   │
└────────────────────────────────────────────┘
                      ↑
┌────────────────────────────────────────────┐
│  业务层: internal/{module}/                 │
│  ├── model.go        领域实体               │
│  ├── repository.go   Repository 接口        │
│  ├── repository_impl.go 数据库实现          │
│  └── service.go     业务逻辑                │
└────────────────────────────────────────────┘
```

**模块间解耦**：模块之间通过接口通信，不直接依赖。

```
调用方 ──→ 接口（定义在调用方）
              ↑
实现方 ──────┘（实现接口）
                ↑
          组装层注入
```

### 渲染进程通信架构

**方案**：Electron + 内嵌 Web 服务器

```
┌─────────────────────────────────────┐
│  Electron（仅负责窗口和生命周期）    │
└─────────────────────────────────────┘
              ↓ HTTP (:34567)
┌─────────────────────────────────────┐
│  Go HTTP 服务器（处理业务逻辑）      │
└─────────────────────────────────────┘
```

| 原方案（Node.js） | 新方案（Go HTTP） |
|------------------|------------------|
| `ipcRenderer.invoke('xxx')` | `fetch('http://localhost:34567/api/xxx')` |
| preload 桥接 | 无需 preload |
| Electron IPC | HTTP REST API |

### Go 项目目录结构

```
my-ipc-service/
├── cmd/server/main.go           # 程序入口
├── internal/
│   ├── config/                  # 程序配置
│   ├── database/                # 数据库连接、迁移、事务
│   ├── site/                    # 站点模块
│   ├── author/                  # 作者模块
│   ├── localTag/                # 本地标签
│   ├── localAuthor/             # 本地作者
│   ├── work/                    # 作品模块（含 link/unlink 逻辑）
│   ├── workSet/                 # 作品集模块
│   ├── plugin/                  # 插件系统
│   ├── task/                    # 任务管理
│   ├── search/                 # 搜索（不依赖其他业务模块）
│   ├── settings/                # 业务设置
│   ├── secureStorage/           # 安全存储
│   ├── appLauncher/            # App启动器
│   ├── autoExplainPath/        # 自动解释路径
│   ├── slot/                   # Slot同步
│   ├── siteBrowser/            # 站点浏览器
│   ├── pluginTaskUrlListener/  # 插件任务URL监听
│   ├── error/                  # 自定义错误
│   └── util/                   # 工具函数
├── pkg/model/                   # 全局共享 DTO
└── go.mod
```

### Go 开发检查清单

- [ ] `pkg/model` 不含业务逻辑或数据库依赖（中立区）
- [ ] **模块间通过接口通信**：不直接 import 对方模块
- [ ] 接口由调用方定义，实现方实现
- [ ] Service 层不直接 import `internal/database`
- [ ] 所有 Repository 方法接收 `context.Context`
