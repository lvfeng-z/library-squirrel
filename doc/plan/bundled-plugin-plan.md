# 捆绑插件自动安装方案

## 背景

当前插件系统支持用户手动安装 ZIP 包，`config.yaml` 中已预留 `plugins` 配置段（`PluginConfig` 含 `packagePath` 和 `pathType`），但 Go 代码从未消费该配置。目标是让程序携带默认插件 ZIP 包，并在首次启动时自动安装。

当前发布产物是单个 `library-squirrel.exe`，前端通过 `go:embed` + Vite 内嵌，不存在统一的部署时静态资源目录。借此机会设计一个统一的静态资源目录，用于存放捆绑插件安装包、前端运行时静态图片等随程序分发的只读资源。

## 现有基础

| 已有 | 说明 |
|------|------|
| `Config.Plugins []PluginConfig` | 配置结构已就绪，含 `PackagePath` 和 `PathType`（Relative/Absolute） |
| `Service.InstallFromPath()` | 完整安装流水线：解析 ZIP → 提取到 `plugin/package/` → 保存 DB → 备份 → 激活 |
| `Service.install()` 去重逻辑 | 按 `PublicID` 查重，已安装未卸载时返回 `ErrPluginAlreadyExists` |
| `config.yaml` 已有示例 | `packagePath: initialization/pixivSuite.zip, pathType: Relative` |
| NSIS 安装器 | 当前仅安装单个 `.exe`，需要扩展以包含资源目录 |

## 设计方案

### 1. 统一静态资源目录 `resources/`

在 App Root 下新增 `resources/` 目录，作为所有部署时自带的只读静态资源的统一入口：

```
<app-root>/
  library-squirrel.exe            # 主程序
  resources/                      # 统一静态资源目录
    bundled-plugins/              # 捆绑插件安装包
      pixiv-suite.zip
      ...
    images/                       # 前端运行时静态图片（不通过 Vite 编译的）
      ...
```

**设计原则**：

- `resources/` 是部署时自带的，程序不应修改其内容（只读）
- 与运行时产生的数据（`database/`、`log/`、`plugin/package/`）严格分离
- 与用户数据（`WorkDir` 下的 `resource/`、`backup/`）严格分离
- 构建时将 `resources/` 整体打入发行包

**`resources/` 内部按资源类型划分子目录**：

| 子目录 | 用途 | 说明 |
|--------|------|------|
| `bundled-plugins/` | 捆绑插件安装包 | `.zip` 格式，首次启动时自动安装 |
| `images/` | 前端运行时静态图片 | 不方便通过 Vite 编译内嵌的图片（如占位大图、引导页图片等） |

> 未来如有新的静态资源类型（如 i18n 翻译文件、默认主题等），在 `resources/` 下新增子目录即可。

### 2. 路径管理

在 `backend/util/file.go` 中新增路径函数：

```go
const (
    // ResourcesDir 静态资源目录名
    ResourcesDir         = "resources"
    // BundledPluginsDir 捆绑插件子目录名
    BundledPluginsDir    = "bundled-plugins"
)

// ResourcesPath 返回静态资源目录的绝对路径
func ResourcesPath() string {
    return filepath.Join(RootPath(), ResourcesDir)
}

// BundledPluginsPath 返回捆绑插件目录的绝对路径
func BundledPluginsPath() string {
    return filepath.Join(RootPath(), ResourcesDir, BundledPluginsDir)
}
```

### 3. `config.yaml` 配置更新

`config.yaml` 中 `plugins` 的 `packagePath` 改为引用 `resources/` 下的路径：

```yaml
plugins:
  - packagePath: resources/bundled-plugins/pixiv-suite.zip
    pathType: Relative
```

对应地，`default_config.yaml` 保持 `plugins: []`（生产环境不捆绑默认插件，由部署配置决定）。

### 4. 捆绑插件安装流程

#### 4.1 安装时机

在 `main.go` 中，`app.LoadPlugins()` 调用前新增一步 `app.InstallBundledPlugins()`：

```
NewApp()                → 初始化 DB、PluginService
SetEventEmitter()       → 事件通道就绪
InstallBundledPlugins() → 安装捆绑插件（仅注册 DB，不激活）  ← 新增
LoadPlugins()           → loadInstalledPlugins() 正常激活所有已安装插件
```

**关键**：`InstallBundledPlugins` 仅执行安装流程（解压 + 写 DB），不调用 `activator.Activate()`。激活由后续 `loadInstalledPlugins()` 统一处理。

#### 4.2 `plugin/service.go` — 新增安静安装方法

```go
// InstallBundled 安装捆绑插件，已安装时静默跳过
// 与 InstallFromPath 的区别：不调用 activator.Activate()，已安装时不报错
func (s *Service) InstallBundled(ctx context.Context, packagePath string) (*entity2.Plugin, error)
```

实现方式：提取 `installCore(ctx, installDTO, installType) (*entity2.Plugin, error)` 为纯安装逻辑（不含激活），让 `install()` 调用 `installCore` + `activator.Activate()`，`InstallBundled` 只调用 `installCore`。

逻辑：
1. 调用 `loadPluginPackage(packagePath)` 解析 ZIP
2. 调用 `repo.GetByPublicId()` 检查是否已安装
3. 已安装且未卸载 → 跳过，返回 nil, nil
4. 未安装或已卸载 → 调用 `installCore()` 执行安装

#### 4.3 `app.go` — 新增 `InstallBundledPlugins` 方法

```go
// InstallBundledPlugins 安装配置中声明的捆绑插件
func (app *App) InstallBundledPlugins() {
    ctx := context.Background()
    cfg := config.Get()
    if len(cfg.Plugins) == 0 {
        return
    }

    rootPath := util.RootPath()
    for _, pc := range cfg.Plugins {
        path := pc.PackagePath
        if pc.PathType == "Relative" || pc.PathType == "" {
            path = filepath.Join(rootPath, path)
        }

        if _, err := os.Stat(path); os.IsNotExist(err) {
            logger.Log.Warnf("捆绑插件包不存在: %s", path)
            continue
        }

        plugin, err := app.PluginService.InstallBundled(ctx, path)
        if err != nil {
            logger.Log.Errorf("安装捆绑插件失败: %s, %v", path, err)
            continue
        }
        if plugin != nil {
            logger.Log.Infof("捆绑插件已安装: %s", plugin.PublicID.String)
        }
    }
}
```

#### 4.4 `main.go` — 调用时机

```go
// 改为：
app.SetEventEmitter(wailsApp.Event, ...)
app.InstallBundledPlugins()   // 新增
app.LoadPlugins()
```

### 5. 前端静态图片访问

对于 `resources/images/` 下的图片，通过 Asset Router 提供访问。在 `app.go` 的 `CreateAssetHandler` 中新增路由：

```go
func (app *App) CreateAssetHandler(frontendAssets fs.FS) http.Handler {
    router := assetserver.NewRouter(frontendAssets)
    router.Handle("/plugin/", app.StaticResourceService, 0)
    router.Handle("/resource/", app.HttpFileHandler, 0)
    // 新增：静态资源路由
    router.Handle("/static/", http.FileServer(http.Dir(util.ResourcesPath())), 0)
    return router
}
```

前端通过 `/static/images/xxx.png` 访问这些图片。

> 注：此路由为可选功能。如果当前没有 `resources/images/` 的实际需求，可暂时不添加路由，仅建立目录结构。

### 6. 构建集成

#### 6.1 NSIS 安装器（`build/windows/nsis/project.nsi`）

在安装器脚本中增加 `resources/` 目录的安装：

```nsi
; 安装 resources 目录
SetOutPath $INSTDIR\resources
File /r "${ROOT}\resources\*.*"
```

在卸载器中增加清理：

```nsi
RMDir /r "$INSTDIR\resources"
```

#### 6.2 Taskfile（`build/windows/Taskfile.yml`）

确保 `build` 任务前将 `resources/` 目录就绪（如果插件 ZIP 需要在构建时从外部复制，可在此步骤完成）。

#### 6.3 `.gitignore`

`resources/bundled-plugins/*.zip` 通常不应提交到 Git（体积大且可能是私有插件）。在 `.gitignore` 中添加：

```gitignore
resources/bundled-plugins/*.zip
```

如果需要携带某个默认插件的安装包，可通过构建脚本从指定位置复制。

### 7. 捆绑插件更新策略

- 捆绑插件仅在**未安装**时自动安装
- 已安装的插件不会被覆盖
- 如需升级捆绑插件版本，用户通过正常的重新安装流程操作
- 捆绑机制只保证"开箱即有"，不负责版本升级

### 8. 边界情况

| 场景 | 处理 |
|------|------|
| 插件已安装且未卸载 | 静默跳过 |
| 插件已安装但已卸载 | 重新安装（复用 `installCore` 中的 uninstalled 分支） |
| ZIP 文件不存在 | 记录警告日志，跳过 |
| ZIP 格式无效/manifest 缺失 | 记录错误日志，跳过 |
| 插件激活失败 | 不阻塞其他插件 |
| `resources/` 目录不存在 | `InstallBundledPlugins` 正常跳过（无插件可安装） |

## 最终目录结构

```
<app-root>/
  library-squirrel.exe                # 主程序
  config.yaml                         # 应用配置
  resources/                          # ← 新增：统一静态资源目录
    bundled-plugins/                  # 捆绑插件安装包
      pixiv-suite.zip
    images/                           # 前端运行时静态图片（按需使用）
  database/                           # 运行时：数据库
    database.db
  config/                             # 运行时：设置
    settings.json
  log/                                # 运行时：日志
    server.log
  plugin/                             # 运行时：已安装插件
    package/
      <publicId>/<version>/...
```

## 涉及文件

| 文件 | 改动 |
|------|------|
| `backend/util/file.go` | 新增 `ResourcesPath()`、`BundledPluginsPath()` 路径函数 |
| `backend/plugin/service.go` | 新增 `InstallBundled()` 方法；提取 `installCore()` |
| `app.go` | 新增 `InstallBundledPlugins()` 方法；`CreateAssetHandler` 可选增加 `/static/` 路由 |
| `main.go` | 在 `LoadPlugins()` 前调用 `InstallBundledPlugins()` |
| `config.yaml` | 更新 `plugins.packagePath` 为 `resources/bundled-plugins/xxx.zip` |
| `build/windows/nsis/project.nsi` | 安装/卸载时包含 `resources/` 目录 |
| `build/windows/Taskfile.yml` | 构建流程中确保 `resources/` 目录就绪 |
| `.gitignore` | 忽略 `resources/bundled-plugins/*.zip` |

`backend/config/config.go`、`backend/base/model/dto/plugin_types.go` 无需改动，现有结构已满足需求。
