# LibrarySquirrel 架构速查

## 核心业务概念（一句话总结）

| 概念         | 定义                             | 关键模块                |
| ------------ | -------------------------------- | ----------------------- |
| **站点**     | 远程作品源（如pixiv、bilibili）  | `backend/site/`        |
| **作品**     | 资源与信息的集合（核心数据实体） | `backend/work/`        |
| **任务**     | 作品创建执行流程                 | `backend/task/`        |
| **站点作者** | 来自站点的原始作者               | `backend/siteAuthor/`  |
| **本地作者** | 本地创建，用于统一跨站点作者身份 | `backend/localAuthor/` |
| **站点标签** | 来自站点的原始标签               | `backend/siteTag/`     |
| **本地标签** | 本地创建，用于统一跨站点标签     | `backend/localTag/`    |
| **插件**     | 扩展对不同站点的支持             | `backend/plugin/`      |

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

### 后端架构 (Go)

- **IPC模式**：Wails Bind（`window.api.method(args)`）
- **API注册**：`backend/{module}/handler.go` 中的 Handler 方法
- **数据库**：GORM + Repository模式
- **响应格式**：`model.Success(data)` / `model.Error(msg)`
- **路径别名**：`@shared/*` → `src/shared/*`（前后端共用代码）

### 核心目录

```
library-squirrel/
├── backend/                  # 后端 (Go)
│   ├── {module}/             # 业务模块（以 localTag 为例）
│   │   ├── handler.go        # Handler（Wails Bind）
│   │   ├── service.go        # 业务逻辑
│   │   ├── repository.go     # 数据访问接口
│   │   ├── repository_impl.go # 数据访问实现
│   │   └── model.go          # 领域实体
│   │   └── query.go          # 查询DTO
│   ├── database/             # 数据库基础设施
│   │   ├── db.go             # 数据库连接
│   │   ├── transaction.go    # 事务封装
│   │   └── resources/        # SQL 迁移文件
│   ├── model/                # 后端领域模型
│   └── base/model            # 共享模型
│       └── entity/           # 实体 子目录
│       └── dto/              # DTO 子目录
├── frontend/src/             # 前端 (Vue 3)
│   ├── router/               # Vue Router 配置
│   ├── views/                # 视图组件
│   ├── components/           # Vue 组件
│   ├── store/                # Pinia 状态管理
│   ├── model/                # 前端 DTO/类型定义
│   │   └── util/             # 工具类型
│   ├── utils/                # 前端工具函数
│   └── apis/                 # API 包装器
├── plugin/                    # 插件目录
└── wails.json               # Wails 配置

## 开发模式

### 添加新 Handler

1. `backend/{module}/handler.go` 创建 Handler（Wails Bind 方法）
2. `backend/{module}/service.go` 实现业务逻辑
3. `backend/{module}/repository.go` 实现数据访问
4. 在 `wails.go` 中将 Handler 注入到 App 结构体
5. 执行 `wails3 generate bindings -ts` 生成前端 TypeScript 绑定
6. 前端通过 `window.api.handlerMethod(args)` 调用

### 数据库事务

```go
err := database.WithTransaction(db, func(tx *gorm.DB) error {
    // 多个操作
    return nil
})
```

### 响应格式

- 成功：`model.Success(data)` → `{success: true, msg: "success", data: ...}`
- 错误：`model.Error(msg)` → `{success: false, msg: "错误信息", data: nil}`

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

- 插件为 Go 共享库（`.dll`/`.so`）或纯 UI 配置，存储在 `plugin/package/{publicId}/{version}/`
- **两种模式**：运行时插件（需 DLL + `entryFile`）、纯 UI 插件（仅 `plugin.json` 声明 slots）
- 入口函数：`func Activate(ctx pluginsdk.PluginContext)`，运行时插件通过 PluginContext 注册 TaskHandler/SiteBrowser
- PluginContext 接口定义在独立的 SDK 库 `github.com/lvfeng-z/library-squirrel-sdk` 中，主程序和插件共同依赖
- 三个扩展点：TaskHandler（运行时注册）、SiteBrowser（运行时注册）、Slot（**声明式注册**，通过 `plugin.json`）
- 三个注册中心：`TaskHandlerRegistry`、`SiteBrowserRegistry`、`SlotRegistry`（线程安全）
- **静态资源服务**：`StaticResourceService` 提供 `resource://plugin/{id}/{ver}/...` URL 访问插件文件
- **组合 Asset Handler**：`PluginAwareAssetHandler` 路由 `/plugin/` 到静态资源服务，其余到前端 embed.FS
- 启动引导：`app.go` 的 `loadInstalledPlugins()` 读取 `plugin.json` → 注册静态资源 → 声明式注册 Slot → 按需加载 DLL
- Slot 同步：通过 Wails Events 推送到前端
- 内容类型（ContentType）：`vueSource`、`precompiled`、`code`、`html`

## 更新记录

### 2026-05-06
- [重构] 插件静态资源模块：声明式 Slot 注册、StaticResourceService、PluginAwareAssetHandler
- [修改] 模块路径从 `github.com/library-squirrel/wails` 调整为 `github.com/library-squirrel`
- [修改] 引入 SDK 第三方库 `github.com/lvfeng-z/library-squirrel-sdk`，PluginContext 等接口迁移至 SDK

### 2026-05-05
- [修改] 目录结构调整：`internal/` → `backend/`，`pkg/` → `backend/base/`
