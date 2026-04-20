---
name: librarysquirrel-patterns
description: Coding patterns extracted from LibrarySquirrel repository
version: 1.0.0
source: local-git-analysis
analyzed_commits: 200
---

# LibrarySquirrel Patterns

## Commit Conventions

This project uses **conventional commits** with Chinese scope:

| Type | Description |
|------|-------------|
| `fix` | Bug fixes (most common - 65 commits) |
| `feat` | New features (45 commits) |
| `refactor` | Code refactoring (28 commits) |
| `perf` | Performance improvements (28 commits) |
| `docs` | Documentation updates (18 commits) |
| `build` | Build/dependency updates (12 commits) |

**Format**: `type(范围): 描述` or `type: 描述`

Examples:
- `fix(主进程): 修复主窗口的session设置影响了其他窗口的问题`
- `feat(插件): 添加webpack编译vue的方法`
- `perf(渲染进程、插件): 优化属性命名`
- `refactor: 编译vue的过程迁移到主进程`

## Code Architecture

### Directory Structure

```
src/
├── main/                    # Electron main process (Node.js)
│   ├── base/               # Base classes (BaseDao, BasePlugin)
│   ├── constant/           # Constants and enums (PascalCase)
│   ├── core/               # Core services (MainProcessApi, Initialize)
│   ├── dao/                # Data Access Objects
│   ├── database/           # Database initialization
│   ├── error/              # Custom error classes
│   ├── model/              # Domain models
│   ├── plugin/             # Plugin system
│   ├── service/            # Business logic services
│   ├── resources/          # YAML config files
│   └── util/               # Utilities
├── preload/                # Preload script (IPC bridge)
├── renderer/                # Vue 3 renderer process
│   └── src/
│       ├── apis/           # API wrappers for IPC
│       ├── components/     # Vue components
│       │   ├── common/     # Common components
│       │   ├── dialogs/    # Dialog components
│       │   ├── subpage/    # Page-level components
│       │   └── oneOff/     # One-off components
│       ├── composables/    # Vue composables
│       ├── constants/      # Frontend constants
│       ├── directives/     # Vue directives
│       ├── model/          # TypeScript models
│       ├── plugins/        # Plugin configurations
│       ├── store/          # Pinia stores
│       └── utils/          # Utilities
└── shared/                  # Shared code between main/renderer
    └── model/              # Shared types
```

### Naming Conventions

| Category | Convention | Example |
|----------|------------|---------|
| Files | PascalCase for components/classes | `MainProcessApi.ts`, `TaskService.ts` |
| Functions | camelCase | `getTaskList`, `saveWorkSet` |
| Vue Components | PascalCase with suffix | `TaskManage.vue`, `WorksGrid.vue` |
| Constants/Enums | PascalCase | `TaskStatusEnum.ts`, `PageEnum.ts` |
| Directives | camelCase with v- prefix | `clickOutSide.ts` → `v-click-out-side` |
| Composables | camelCase with use prefix | `useSlotSyncListener.ts` |

### Key Files (Most Modified)

1. `src/main/core/MainProcessApi.ts` - IPC handler dispatcher (28 changes)
2. `src/renderer/src/App.vue` - Main Vue app (28 changes)
3. `src/main/service/TaskService.ts` - Task business logic (25 changes)
4. `src/main/service/PluginService.ts` - Plugin management (25 changes)
5. `src/preload/index.ts` - IPC bridge (20 changes)

## Workflows

### Adding a New Service

1. Create service in `src/main/service/ServiceName.ts`
2. Add IPC handlers in `MainProcessApi.ts`:
   ```typescript
   Electron.ipcMain.handle('serviceName-method', async (_event, args) => {
     const service = new ServiceName()
     try {
       return ApiUtil.response(await service.method(args))
     } catch (error) {
       return returnError(error)
     }
   })
   ```
3. Add wrapper in `src/preload/index.ts`:
   ```typescript
   serviceNameMethod: (args) => Electron.ipcRenderer.invoke('serviceName-method', args)
   ```

### Adding a New DAO

1. Create DAO in `src/main/dao/EntityNameDao.ts`
2. Extend `BaseDao` for CRUD operations
3. Register in `Database.ts` or appropriate service

### Adding a New Vue Component

1. Create in appropriate directory under `src/renderer/src/components/`
2. Use Composition API with `<script setup lang="ts">`
3. Props interface should use `Props` suffix
4. Use Element Plus components

### Database Operations

- **Schema**: Defined in YAML at `src/main/resources/database/createDataTables.yml`
- **Transactions**: Uses **SAVEPOINT** (not SQLite BEGIN/COMMIT) via `Database.transaction()`
- **Pattern**: DAO layer with `BaseDao` for CRUD operations

## Testing Patterns

- No dedicated test framework detected in recent commits
- Manual testing via development server

## IPC Communication Pattern

**Renderer → Main**: `ipcRenderer.invoke('service-method', args)` via preload bridge
**Main → Renderer**: `ipcMain.send/ipcMain.on` for push notifications

**Response Format**: All responses use `ApiUtil.response(data)` or `ApiUtil.error(message)`

## Plugin System

- Plugins located in `plugin/package/`
- Loader: `PluginLoader.ts`
- Execution: `TaskHandler.ts` in plugin module
- Base interface: `BasePlugin` with minimal `pluginId: number`

## Shared Code

- Constants duplicated in main and renderer where needed
- Shared models in `src/shared/model/` for cross-process types
- Uses custom `resource://` protocol for local file serving
