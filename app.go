package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/library-squirrel/backend/base"
	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/dto"
	entity2 "github.com/library-squirrel/backend/base/model/entity"
	extension2 "github.com/library-squirrel/backend/plugin/extension"
	pluginsdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
	"github.com/wailsapp/wails/v3/pkg/application"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/library-squirrel/backend/appLauncher"
	"github.com/library-squirrel/backend/assetserver"
	"github.com/library-squirrel/backend/backup"
	"github.com/library-squirrel/backend/config"
	"github.com/library-squirrel/backend/database"
	"github.com/library-squirrel/backend/fileSysUtil"
	"github.com/library-squirrel/backend/frontendLog"
	"github.com/library-squirrel/backend/fsmonitor"
	"github.com/library-squirrel/backend/localAuthor"
	"github.com/library-squirrel/backend/localTag"
	"github.com/library-squirrel/backend/merge"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/persistentStore"
	"github.com/library-squirrel/backend/plugin"
	"github.com/library-squirrel/backend/pluginTaskUrlListener"
	"github.com/library-squirrel/backend/reWorkAuthor"
	"github.com/library-squirrel/backend/reWorkSetWorkSet"
	"github.com/library-squirrel/backend/reWorkTag"
	"github.com/library-squirrel/backend/reWorkWorkSet"
	"github.com/library-squirrel/backend/recycleBin"
	"github.com/library-squirrel/backend/resource"
	"github.com/library-squirrel/backend/search"
	"github.com/library-squirrel/backend/settings"
	"github.com/library-squirrel/backend/site"
	"github.com/library-squirrel/backend/siteAuthor"
	"github.com/library-squirrel/backend/siteBrowser"
	"github.com/library-squirrel/backend/siteTag"
	"github.com/library-squirrel/backend/storeRegistry"
	"github.com/library-squirrel/backend/task"
	"github.com/library-squirrel/backend/taskManager"
	"github.com/library-squirrel/backend/util"
	"github.com/library-squirrel/backend/util/fingerprint"
	"github.com/library-squirrel/backend/window"
	"github.com/library-squirrel/backend/work"
	"github.com/library-squirrel/backend/workSet"
)

// App Wails应用主结构
type App struct {
	// 配置
	cfg *config.Config
	// 数据库
	db *gorm.DB

	// 业务服务
	LocalTagService        *localTag.Service
	LocalAuthorService     *localAuthor.Service
	SiteTagService         *siteTag.Service
	SiteAuthorService      *siteAuthor.Service
	SiteService            *site.Service
	ResourceService        *resource.Service
	MergeService           *resource.MergeService
	ReWorkAuthorService    *reWorkAuthor.Service
	ReWorkTagService       *reWorkTag.Service
	WorkService            *work.Service
	WorkSetService         *workSet.Service
	SearchService          *search.Service
	SettingsService        *settings.Service
	BackupService          *backup.Service
	AppLauncherService     *appLauncher.Service
	FileSysUtilService     *fileSysUtil.Service
	FrontendLogService     *frontendLog.Service
	PluginService          *plugin.Service
	PluginStorageService   *plugin.PluginStorageService
	PluginSettingService   *plugin.PluginSettingService
	TaskService            *task.Service
	TaskManagerService     *taskManager.Manager
	SiteBrowserService     *siteBrowser.Service
	PersistentStoreService *persistentStore.Service
	RecycleBinService      *recycleBin.Service
	FsmonitorService       *fsmonitor.Service

	// 任务仓储（用于TaskManager）
	taskRepo *task.TaskRepository

	// 扩展注册中心
	TaskHandlerRegistry       *extension2.TaskHandlerRegistry
	SiteBrowserRegistry       *extension2.SiteBrowserRegistry
	FrontendExtensionRegistry *extension2.FrontendExtensionRegistry

	// 插件加载器
	pluginLoader *extension2.Loader

	// 主窗口原生句柄
	mainHWND uintptr
	// 主窗口实例（用于实时获取原生句柄，标题栏等能力使用）
	mainWindow *application.WebviewWindow

	// 静态资源服务
	StaticResourceService *extension2.StaticResourceService

	// HTTP 路由
	AssetRouter      *assetserver.Router
	StoreFileHandler *assetserver.StoreFileHandler

	// 任务URL监听器
	PluginTaskUrlListenerSvc *pluginTaskUrlListener.Service

	// Wails 事件发射器（用于任务进度推送）
	taskProgressEmitter taskManager.WailsEventEmitter
	// 前端事件监听函数（用于插件 SubscribeFrontend）
	frontendEventOn func(topic string, callback func(data any)) func()

	// Handlers（用于 Bind[] 参数）
	LocalTagHandler              *localTag.Handler
	LocalAuthorHandler           *localAuthor.Handler
	SiteTagHandler               *siteTag.Handler
	SiteAuthorHandler            *siteAuthor.Handler
	SiteHandler                  *site.Handler
	ResourceHandler              *resource.Handler
	WorkHandler                  *work.Handler
	WorkSetHandler               *workSet.Handler
	SearchHandler                *search.Handler
	SettingsHandler              *settings.Handler
	AppLauncherHandler           *appLauncher.Handler
	FileSysUtilHandler           *fileSysUtil.Handler
	FrontendLogHandler           *frontendLog.Handler
	PluginHandler                *plugin.Handler
	PluginSettingHandler         *plugin.SettingHandler
	TaskHandler                  *task.Handler
	TaskManagerHandler           *taskManager.Handler
	FrontendExtensionHandler     *extension2.FrontendExtensionHandler
	SiteBrowserHandler           *siteBrowser.Handler
	ReWorkAuthorHandler          *reWorkAuthor.Handler
	ReWorkTagHandler             *reWorkTag.Handler
	PluginTaskUrlListenerHandler *pluginTaskUrlListener.Handler
	RecycleBinHandler            *recycleBin.Handler
	FsmonitorHandler             *fsmonitor.Handler
	WindowHandler                *window.Handler
}

// NewApp 创建Wails应用实例
func NewApp() (*App, error) {
	app := &App{}

	// 1. 加载配置
	cfg, err := config.LoadFromDir(util.RootPath())
	if err != nil {
		logger.Log.Errorf("加载配置失败: %v", err)
		return nil, err
	}
	app.cfg = cfg
	logger.Log.Infof("配置已加载")

	// 2. 初始化数据库
	dbPath := filepath.Join(util.RootPath(), "database/database.db")
	if err := database.Init(dbPath); err != nil {
		logger.Log.Errorf("初始化数据库失败: %v", err)
		return nil, err
	}
	app.db = database.GetDB()
	logger.Log.Infof("数据库已初始化: %s", dbPath)

	// 自动迁移数据库表结构
	if err := migration.AutoMigrate(app.db); err != nil {
		logger.Log.Errorf("数据库迁移失败: %v", err)
		return nil, err
	}
	logger.Log.Infof("数据库迁移完成")

	// 3. 初始化扩展注册中心
	app.TaskHandlerRegistry = extension2.NewTaskHandlerRegistry()
	app.SiteBrowserRegistry = extension2.NewSiteBrowserRegistry()
	app.FrontendExtensionRegistry = extension2.NewFrontendExtensionRegistry()
	// FrontendExtensionPusher 会在 SetEventEmitter 中创建并接入 FrontendExtensionRegistry

	// 3.5 初始化静态资源服务
	app.StaticResourceService = extension2.NewStaticResourceService()
	app.StoreFileHandler = assetserver.NewStoreFileHandler(nil) // statusChecker 在 initBaseServices 后设置

	// 4. 初始化基础服务（按依赖顺序）
	app.initBaseServices()

	// 5. 初始化高级服务
	if err := app.initAdvancedServices(); err != nil {
		return nil, err
	}

	// 6. 初始化 Handlers
	app.initHandlers()

	// 7. 加载已安装的插件
	// 插件加载延迟到 SetEventEmitter 之后，由 LoadPlugins 显式调用

	return app, nil
}

// SetWailsApp 设置 Wails 应用实例（延迟注入，供需要 Dialog 等运行时能力的服务使用）
func (app *App) SetWailsApp(wailsApp *application.App) {
	if app.FileSysUtilService != nil {
		app.FileSysUtilService.SetApp(wailsApp)
	}
}

// SetMainWindow 设置主窗口实例（供模态对话框等服务使用）
func (app *App) SetMainWindow(window application.Window) {
	if app.FileSysUtilService != nil {
		app.FileSysUtilService.SetWindow(window)
	}
}

// LoadPlugins 加载已安装的插件（必须在 SetEventEmitter 之后调用）
func (app *App) LoadPlugins() {
	app.loadInstalledPlugins()
}

// InstallBundledPlugins 安装 config.yaml 中声明的捆绑插件
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

		if !util.FileExists(path) {
			logger.Log.Warnf("捆绑插件包不存在: %s", path)
			continue
		}

		bundledPlugin, err := app.PluginService.InstallBundled(ctx, path)
		if err != nil {
			logger.Log.Errorf("安装捆绑插件失败: %s, %v", path, err)
			continue
		}
		if bundledPlugin != nil {
			logger.Log.Infof("捆绑插件已安装: %s", bundledPlugin.PublicID.String)
		}
	}
}

// SetEventEmitter 设置 Wails 事件发射器并创建 FrontendExtensionPusher 和 TaskProgressPusher
func (app *App) SetEventEmitter(emitter extension2.WailsEventEmitter, onEvent func(topic string, callback func(data any)) func()) {
	pusher := extension2.NewWailsFrontendExtensionPusher(emitter)
	app.FrontendExtensionRegistry.SetPusher(pusher)
	app.taskProgressEmitter = emitter
	app.frontendEventOn = onEvent
	// Manager 在 initAdvancedServices 中已创建（此时 emitter 尚未就绪），需要补设 pusher
	if app.TaskManagerService != nil {
		var pusher taskManager.TaskProgressPusher
		if app.cfg.Task.UseSnapshotMode {
			pusher = taskManager.NewSnapshotPusher(emitter, app.TaskManagerService, 50)
		} else {
			pusher = taskManager.NewWailsTaskProgressPusher(emitter)
		}
		app.TaskManagerService.SetPusher(pusher)
	}
}

// CreateAssetHandler 创建路由多路复用器并注册所有路由
func (app *App) CreateAssetHandler(frontendAssets fs.FS) http.Handler {
	router := assetserver.NewRouter(frontendAssets)
	router.Handle("/plugin/", app.StaticResourceService, 0)
	router.Handle("/store/", app.StoreFileHandler, 0)
	app.AssetRouter = router
	return router
}

// loadInstalledPlugins 加载所有已安装且需要启动时激活的插件
func (app *App) loadInstalledPlugins() {
	ctx := context.Background()

	// 查询所有插件
	plugins, err := app.PluginService.List(ctx, &database.QueryOption{})
	if err != nil {
		logger.Log.Errorf("查询已安装插件失败: %v", err)
		return
	}

	runtimeLoaded := 0
	pureUICount := 0
	pendingTrust := 0      // trusted=false 未激活（需用户手动信任）
	restrictedSkipped := 0 // 受限模式下跳过的非 bundled 插件

	restrictedMode := false
	if app.SettingsService != nil {
		restrictedMode = app.SettingsService.GetSettings().PluginSettings.RestrictedMode
	}

	for _, p := range plugins {
		// 跳过已卸载的插件
		if p.Uninstalled.Valid && p.Uninstalled.Bool {
			continue
		}

		// 信任门控（决策6）：trusted 非真（未设置或显式 false）不激活
		if !p.Trusted.Valid || !p.Trusted.Bool {
			pendingTrust++
			logger.Log.Infof("插件未信任，跳过激活（需手动信任）: %s", p.PublicID.String)
			continue
		}

		// 受限模式（决策4）：开启时跳过所有非 bundled 插件（来源未设置视作非 bundled）
		if restrictedMode && (!p.Source.Valid || p.Source.String != plugin.SourceBundled) {
			restrictedSkipped++
			logger.Log.Infof("受限模式启用，跳过非 bundled 插件: %s", p.PublicID.String)
			continue
		}

		if err := app.activatePlugin(p); err != nil {
			continue
		}

		// 统计类型
		pluginRootDir := filepath.Join(util.RootPath(), p.RootPath.String)
		manifestPath := filepath.Join(pluginRootDir, "plugin.json")
		manifestBytes, _ := os.ReadFile(manifestPath)
		var manifest dto.PluginManifest
		_ = json.Unmarshal(manifestBytes, &manifest)
		if manifest.Extensions != nil {
			hasRuntime := len(manifest.Extensions.TaskHandlers) > 0 || len(manifest.Extensions.SiteBrowsers) > 0
			if hasRuntime {
				runtimeLoaded++
			} else {
				pureUICount++
			}
		}
	}

	logger.Log.Infof("插件加载完成: %d 个运行时, %d 个纯 UI, 共 %d 个; 未信任待激活 %d 个, 受限模式跳过 %d 个",
		runtimeLoaded, pureUICount, len(plugins), pendingTrust, restrictedSkipped)
}

// Activate 实现 PluginActivator 接口，激活单个插件
func (app *App) Activate(p *entity2.Plugin) error {
	return app.activatePlugin(p)
}

// activatePlugin 激活单个插件：读取 manifest、注册静态资源、注册前端扩展、启动运行时子进程
func (app *App) activatePlugin(p *entity2.Plugin) error {
	// 跳过没有 PublicID 的插件
	if !p.PublicID.Valid || p.PublicID.String == "" {
		return fmt.Errorf("插件缺少 PublicID")
	}

	// 信任门控（决策6）：trusted 非真（未设置或显式 false）不激活，需用户在管理页显式信任后由 Activate 重新激活
	if !p.Trusted.Valid || !p.Trusted.Bool {
		return fmt.Errorf("插件未信任，拒绝激活: %s", p.PublicID.String)
	}

	// 跳过没有 RootPath 的插件
	if !p.RootPath.Valid || p.RootPath.String == "" {
		return fmt.Errorf("插件缺少 RootPath")
	}

	rootPath := util.RootPath()
	publicId := p.PublicID.String
	pluginRootDir := filepath.Join(rootPath, p.RootPath.String)
	logger.Log.Infof("正在激活插件: %s (root=%s)", publicId, pluginRootDir)

	// 读取 plugin.json
	manifestPath := filepath.Join(pluginRootDir, "plugin.json")
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("读取 plugin.json 失败 %s: %w", publicId, err)
	}

	// 解析 manifest
	var manifest dto.PluginManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return fmt.Errorf("解析 plugin.json 失败 %s: %w", publicId, err)
	}

	ext := manifest.Extensions
	if ext == nil {
		logger.Log.Warnf("插件 %s 无扩展点，跳过", publicId)
		return nil
	}

	// 注册静态资源
	var allowedDirs []string
	if ext.StaticResources != nil {
		allowedDirs = ext.StaticResources.Directories
	}
	// 缓存键：构建身份 buildId 优先（同源码状态永远同值、源码变化必变，令资产 URL/ETag 随构建失效，
	// immutable 长缓存才安全）；未打标包回落 version
	cacheKey := manifest.BuildID
	if cacheKey == "" && p.Version.Valid {
		cacheKey = p.Version.String
	}
	app.StaticResourceService.RegisterPlugin(publicId, pluginRootDir, allowedDirs, cacheKey)
	logger.Log.Infof("插件 %s: 静态资源已注册 (dirs=%v)", publicId, allowedDirs)

	// 声明式注册前端扩展
	for _, fe := range ext.FrontendExtensions {
		feConfig := base.NewFrontendExtensionConfig()
		feConfig.Metadata.ID = fe.ID
		feConfig.Metadata.PluginID = p.GetID()
		feConfig.Metadata.PluginPublicID = publicId
		feConfig.Metadata.Name = fe.Name
		feConfig.Metadata.Description = fe.Description
		feConfig.Kind = base.FrontendExtensionKind(fe.Kind)
		feConfig.Order = fe.Order

		// 按 kind 解析 content
		if err := parseFrontendExtensionContent(fe, feConfig, publicId, cacheKey); err != nil {
			logger.Log.Errorf("解析前端扩展 content 失败 %s/%s: %v", publicId, fe.ID, err)
			continue
		}

		extension := model.NewExtension(*feConfig.Metadata, feConfig)
		if err := app.FrontendExtensionRegistry.Register(extension); err != nil {
			logger.Log.Errorf("注册前端扩展失败 %s/%s: %v", publicId, fe.ID, err)
		}
	}

	if len(ext.FrontendExtensions) > 0 {
		logger.Log.Infof("插件 %s: 已注册 %d 个前端扩展", publicId, len(ext.FrontendExtensions))
	}

	// 判断是否为纯 UI 插件（无运行时扩展点）
	hasRuntime := len(ext.TaskHandlers) > 0 || len(ext.SiteBrowsers) > 0
	if !hasRuntime {
		logger.Log.Infof("插件 %s: 纯 UI 插件，跳过子进程", publicId)
		return nil
	}

	// 非纯 UI 插件：以子进程模式加载运行时
	if !p.EntryPath.Valid || p.EntryPath.String == "" {
		logger.Log.Warnf("插件 %s 有运行时扩展点但无入口路径", publicId)
		return nil
	}

	pluginPath := filepath.Join(rootPath, p.EntryPath.String)
	pluginInfo := &extension2.PluginInfo{
		ID:                  p.GetID(),
		PublicID:            publicId,
		Name:                p.Name.String,
		Version:             p.Version.String,
		ContractVersion:     int(p.ContractVersion.Int64),
		ConfigSchemaVersion: p.ConfigSchemaVersion.Int64,
		Capabilities:        extension2.UnmarshalCapabilities(p.Capabilities),
		ResourceTypes:       extension2.UnmarshalResourceTypes(p.ResourceTypes),
		Author:              p.Author.String,
		EntryPath:           p.EntryPath.String,
		RootPath:            p.RootPath.String,
	}

	pluginCtx := extension2.NewPluginContext(extension2.PluginContextDeps{
		PluginInfo:          pluginInfo,
		RootPath:            rootPath,
		TaskHandlerRegistry: app.TaskHandlerRegistry,
		SiteBrowserRegistry: app.SiteBrowserRegistry,
		Storage:             app.PluginStorageService,
		WorkSetQuery:        app.WorkSetService,
		SiteSave:            app.SiteService,
		SiteQuery:           app.SiteService,
		TaskCreate:          &taskCreateAdapter{svc: app.TaskService},
		StorePath: &storePathQueryAdapter{
			taskSvc:       app.TaskService,
			storeRepo:     resource.NewResourceStoreRepository(app.db),
			persistentSvc: app.PersistentStoreService,
		},
		UrlListener: &urlListenerAdapter{svc: app.PluginTaskUrlListenerSvc, pluginEntity: p},
		FrontendEvent: &wailsFrontendEventProvider{
			emitterFunc: func() extension2.WailsEventEmitter { return app.taskProgressEmitter },
			onEventFunc: func() func(topic string, callback func(data any)) func() { return app.frontendEventOn },
		},
	})

	logger.Log.Infof("插件 %s: 正在启动子进程 %s", publicId, pluginPath)
	if err := app.pluginLoader.LoadPluginProcess(pluginPath, publicId, extension2.PluginProcessDeps{
		PluginInfo:          pluginInfo,
		PluginCtx:           pluginCtx,
		TaskHandlerRegistry: app.TaskHandlerRegistry,
		SiteBrowserRegistry: app.SiteBrowserRegistry,
		MainHWND:            app.mainHWND,
	}); err != nil {
		return fmt.Errorf("加载插件失败 %s: %w", publicId, err)
	}

	return nil
}

// parseFrontendExtensionContent 按 kind 解析 content 字段并填充 FrontendExtensionConfig（决策6：各类定位键非空校验统一在后端）
func parseFrontendExtensionContent(fe dto.FrontendExtensionDeclaration, cfg *base.FrontendExtensionConfig, publicId, cacheKey string) error {
	if len(fe.Content) == 0 {
		return nil
	}

	switch cfg.Kind {
	case base.FrontendExtensionKindEmbed:
		var c dto.EmbedContent
		if err := json.Unmarshal(fe.Content, &c); err != nil {
			return fmt.Errorf("解析 embed content 失败: %w", err)
		}
		if c.Position == "" {
			return fmt.Errorf("embed 前端扩展 position 不能为空")
		}
		cfg.ContentType = base.ContentType(c.ContentType)
		cfg.Content = resolveSourceURLs(c.Source, c.ContentType, publicId, cacheKey)
		cfg.Position = c.Position
		cfg.Props = c.Props

	case base.FrontendExtensionKindView:
		var c dto.ViewContent
		if err := json.Unmarshal(fe.Content, &c); err != nil {
			return fmt.Errorf("解析 view content 失败: %w", err)
		}
		cfg.ContentType = base.ContentType(c.ContentType)
		cfg.Content = resolveSourceURLs(c.Source, c.ContentType, publicId, cacheKey)
		cfg.Title = c.Title
		cfg.Props = c.Props

	case base.FrontendExtensionKindMenu:
		var c dto.MenuContent
		if err := json.Unmarshal(fe.Content, &c); err != nil {
			return fmt.Errorf("解析 menu content 失败: %w", err)
		}
		// menu 须为叶子项（viewId 跳转）或父项（children 展开）之一，两者皆空为无效声明
		if c.ViewId == "" && len(c.Children) == 0 {
			return fmt.Errorf("menu 前端扩展须提供 viewId（叶子项）或 children（父项）")
		}
		cfg.ViewId = c.ViewId
		cfg.Children = convertFrontendExtensionChildren(c.Children, cfg.Metadata.PluginID, publicId, cacheKey)
		if c.Icon != "" {
			cfg.Icon = resolveIconURL(c.Icon, publicId, cacheKey)
		}

	case base.FrontendExtensionKindSiteBrowserList:
		var c dto.SiteBrowserListContent
		if err := json.Unmarshal(fe.Content, &c); err != nil {
			return fmt.Errorf("解析 siteBrowserList content 失败: %w", err)
		}
		if c.ExtensionId == "" {
			return fmt.Errorf("siteBrowserList 前端扩展 extensionId 不能为空")
		}
		cfg.ExtensionId = c.ExtensionId
		if c.Icon != "" {
			cfg.Icon = resolveIconURL(c.Icon, publicId, cacheKey)
		}

	case base.FrontendExtensionKindReplaceView:
		var c dto.ReplaceViewContent
		if err := json.Unmarshal(fe.Content, &c); err != nil {
			return fmt.Errorf("解析 replaceView content 失败: %w", err)
		}
		if c.Target == "" {
			return fmt.Errorf("replaceView 前端扩展 target 不能为空")
		}
		cfg.ContentType = base.ContentType(c.ContentType)
		cfg.Content = resolveSourceURLs(c.Source, c.ContentType, publicId, cacheKey)
		cfg.Target = c.Target
		cfg.Props = c.Props

	case base.FrontendExtensionKindDialog:
		var c dto.DialogContent
		if err := json.Unmarshal(fe.Content, &c); err != nil {
			return fmt.Errorf("解析 dialog content 失败: %w", err)
		}
		cfg.ContentType = base.ContentType(c.ContentType)
		cfg.Content = resolveSourceURLs(c.Source, c.ContentType, publicId, cacheKey)
		cfg.Props = c.Props

	case base.FrontendExtensionKindResourceViewer:
		var c dto.ResourceViewerContent
		if err := json.Unmarshal(fe.Content, &c); err != nil {
			return fmt.Errorf("解析 resourceViewer content 失败: %w", err)
		}
		// resourceType 必填：前端按 resourceType 路由渲染器，空值无法命中
		if c.ResourceType == "" {
			return fmt.Errorf("resourceViewer 前端扩展 resourceType 不能为空")
		}
		cfg.ContentType = base.ContentType(c.ContentType)
		cfg.Content = resolveSourceURLs(c.Source, c.ContentType, publicId, cacheKey)
		cfg.ResourceType = c.ResourceType
		cfg.Props = c.Props
	}

	return nil
}

// convertFrontendExtensionChildren 递归转换子前端扩展声明为 FrontendExtensionConfig
func convertFrontendExtensionChildren(children []dto.FrontendExtensionDeclaration, pluginID int64, publicId, cacheKey string) []base.FrontendExtensionConfig {
	if len(children) == 0 {
		return nil
	}
	result := make([]base.FrontendExtensionConfig, len(children))
	for i, child := range children {
		result[i].Metadata = &model.ExtensionMetadata{
			Type:           model.ExtensionTypeFrontendExtension,
			ID:             child.ID,
			PluginID:       pluginID,
			PluginPublicID: publicId,
			Name:           child.Name,
			Description:    child.Description,
		}
		result[i].Kind = base.FrontendExtensionKind(child.Kind)
		result[i].Order = child.Order

		if err := parseFrontendExtensionContent(child, &result[i], publicId, cacheKey); err != nil {
			logger.Log.Warnf("解析子前端扩展 content 失败 %s/%s: %v", publicId, child.ID, err)
		}
	}
	return result
}

// resolveSourceURLs 将组件源中的相对路径转换为完整 URL
func resolveSourceURLs(source json.RawMessage, contentType, publicId, cacheKey string) json.RawMessage {
	if len(source) == 0 || contentType == "code" {
		return source
	}

	var c map[string]string
	if err := json.Unmarshal(source, &c); err != nil {
		return source
	}

	prefix := "/plugin/" + publicId + "/" + cacheKey + "/"
	for key, path := range c {
		if path != "" {
			c[key] = prefix + path
		}
	}

	result, err := json.Marshal(c)
	if err != nil {
		return source
	}
	return result
}

// resolveIconURL 将图标相对路径转换为完整 URL
func resolveIconURL(iconPath, publicId, cacheKey string) string {
	if iconPath == "" {
		return ""
	}
	return "/plugin/" + publicId + "/" + cacheKey + "/" + iconPath
}

type taskCreateAdapter struct {
	svc *task.Service
}

func (a *taskCreateAdapter) CreateTaskByURL(ctx context.Context, url string) (*pluginsdkdto.CreateTaskResult, error) {
	resp, err := a.svc.CreateTaskByURL(ctx, url)
	if err != nil {
		return nil, err
	}
	return &pluginsdkdto.CreateTaskResult{
		Succeed:       resp.Succeed,
		AddedQuantity: resp.AddedQuantity,
		Msg:           resp.Msg,
	}, nil
}

// storePathQueryAdapter 实现 extension2.StorePathQueryProvider:据 task+role+seq 查资源 store 真实落盘路径。
// 链路:taskId → 任务 PendingResourceID → resource_store(role+store_seq) → store_id → persistent_store.file_path(workDir 相对)。
// 时序前提:downloadLoop(此处被插件 lazy 生成调用)在 startDownload/resume 事务提交之后,故 PendingResourceID 与 resource_store 已落盘可见。
type storePathQueryAdapter struct {
	taskSvc       *task.Service
	storeRepo     *resource.ResourceStoreRepository
	persistentSvc *persistentStore.Service
}

func (a *storePathQueryAdapter) GetStoreRelPath(ctx context.Context, taskId int64, role string, storeSeq int) (string, error) {
	t, err := a.taskSvc.GetById(ctx, taskId)
	if err != nil {
		return "", fmt.Errorf("查询任务 %d 失败: %w", taskId, err)
	}
	if !t.PendingResourceID.Valid {
		return "", fmt.Errorf("任务 %d 无 PendingResourceID(资源未创建)", taskId)
	}
	resourceId := t.PendingResourceID.Int64
	stores, err := a.storeRepo.ListByResourceId(ctx, resourceId)
	if err != nil {
		return "", fmt.Errorf("查询资源 %d 的 store 列表失败: %w", resourceId, err)
	}
	for _, s := range stores {
		if s.StoreType != role || s.StoreSeq != storeSeq {
			continue
		}
		ps, err := a.persistentSvc.GetById(ctx, s.StoreID)
		if err != nil {
			return "", fmt.Errorf("查询 store %d 失败: %w", s.StoreID, err)
		}
		if !ps.FilePath.Valid {
			return "", fmt.Errorf("store %d 无 file_path", s.StoreID)
		}
		return ps.FilePath.String, nil
	}
	return "", fmt.Errorf("资源 %d 无 (role=%s, store_seq=%d) 的 store", resourceId, role, storeSeq)
}

// urlListenerAdapter 适配 PluginTaskUrlListener.Service 到 UrlListenerRegistry 接口
type urlListenerAdapter struct {
	svc          *pluginTaskUrlListener.Service
	pluginEntity *entity2.Plugin
}

func (a *urlListenerAdapter) RegisterUrlListener(pluginPublicId string, extensionId string, patterns []string) {
	pwc := &pluginTaskUrlListener.PluginWithExtension{
		Plugin:       a.pluginEntity,
		ExtensionKey: "taskHandler",
		ExtensionID:  extensionId,
	}
	a.svc.Register(pwc, patterns)
}

func (a *urlListenerAdapter) UnregisterUrlListener(pluginPublicId string, extensionId string) {
	a.svc.Unregister(pluginPublicId, extensionId)
}

// wailsFrontendEventProvider 桥接 Wails Events 实现前后端通信
// 使用延迟获取避免初始化时序问题（插件加载早于 SetEventEmitter）
type wailsFrontendEventProvider struct {
	emitterFunc func() extension2.WailsEventEmitter
	onEventFunc func() func(topic string, callback func(data any)) func()
}

func (p *wailsFrontendEventProvider) PublishToFrontend(topic string, data []byte) error {
	emitter := p.emitterFunc()
	if emitter != nil {
		logger.Log.Info("插件事件转发到前端", zap.String("topic", topic), zap.Int("dataLen", len(data)))
		emitter.Emit(topic, data)
	} else {
		logger.Log.Warn("插件事件转发失败: emitter 为 nil", zap.String("topic", topic))
	}
	return nil
}

func (p *wailsFrontendEventProvider) SubscribeFrontend(topic string, pushCh func([]byte)) (func(), error) {
	onEvent := p.onEventFunc()
	if onEvent == nil {
		logger.Log.Warn("插件订阅前端事件失败: onEvent 为 nil", zap.String("topic", topic))
		return func() {}, nil
	}
	cancel := onEvent(topic, func(data any) {
		var bytes []byte
		switch v := data.(type) {
		case []byte:
			bytes = v
		case string:
			bytes = []byte(v)
		default:
			bytes, _ = json.Marshal(v)
		}
		if bytes != nil {
			pushCh(bytes)
		}
	})
	logger.Log.Info("插件已订阅前端事件", zap.String("topic", topic))
	return cancel, nil
}

func (p *wailsFrontendEventProvider) UnsubscribeFrontend(topic string) error {
	return nil
}

// initBaseServices 初始化基础服务（无服务依赖）
func (app *App) initBaseServices() {
	rootPath := util.RootPath()

	// localTag 服务
	localTagRepo := localTag.NewRepository(app.db)
	app.LocalTagService = localTag.NewService(localTagRepo)

	// localAuthor 服务
	localAuthorRepo := localAuthor.NewRepository(app.db)
	app.LocalAuthorService = localAuthor.NewService(localAuthorRepo)

	// site 服务（需在 siteTag 之前初始化，因为 siteTag 依赖 site 的查询接口）
	siteRepo := site.NewRepository(app.db)
	app.SiteService = site.NewService(siteRepo)

	// siteTag 服务
	siteTagRepo := siteTag.NewRepository(app.db)
	app.SiteTagService = siteTag.NewService(siteTagRepo, app.LocalTagService, app.LocalTagService, app.SiteService)

	// siteAuthor 服务
	siteAuthorRepo := siteAuthor.NewRepository(app.db)
	app.SiteAuthorService = siteAuthor.NewService(siteAuthorRepo, app.LocalAuthorService, app.SiteService)

	// resource 服务
	resourceRepo := resource.NewRepository(app.db)
	resourceStoreRepo := resource.NewResourceStoreRepository(app.db)
	app.ResourceService = resource.NewService(resourceRepo, resourceStoreRepo)

	// backup 服务
	backupRepo := backup.NewRepository(app.db)
	app.BackupService = backup.NewService(backupRepo, func() string {
		return app.SettingsService.GetWorkDir()
	})
	// persistentStore 服务
	psRepo := persistentStore.NewRepository(app.db)
	app.PersistentStoreService = persistentStore.NewService(psRepo, app.BackupService, func() string {
		return app.SettingsService.GetWorkDir()
	})
	app.StoreFileHandler.SetStateResolver(app.PersistentStoreService)
	// /store/ 状态路由：软删记录的文件在 backup/，按 original_file_path 反查服务
	app.StoreFileHandler.SetBackupResolver(app.BackupService)

	// reWorkAuthor 服务
	reWorkAuthorRepo := reWorkAuthor.NewRepository(app.db)
	app.ReWorkAuthorService = reWorkAuthor.NewService(reWorkAuthorRepo)

	// reWorkTag 服务
	reWorkTagRepo := reWorkTag.NewRepository(app.db)
	app.ReWorkTagService = reWorkTag.NewService(reWorkTagRepo, app.SiteTagService)

	// settings 服务
	settingsFilePath := filepath.Join(rootPath, "config/settings.json")
	app.SettingsService = settings.NewService(settingsFilePath)

	// 设置工作目录
	app.StoreFileHandler.SetWorkDir(app.SettingsService.GetWorkDir())

	// appLauncher 服务
	app.AppLauncherService = appLauncher.NewService(app.SettingsService)

	// fileSysUtil 服务
	app.FileSysUtilService = fileSysUtil.NewService(rootPath)

	// frontendLog 服务
	app.FrontendLogService = frontendLog.NewService()
}

// initAdvancedServices 初始化高级服务（依赖其他服务）
func (app *App) initAdvancedServices() error {
	// workSet 仓储（提前创建，用于 workSetWriterAdapter + 复原引用校验）
	workSetRepo := workSet.NewRepository(app.db)
	// reWorkWorkSet 仓储（提前创建，复用给 work 的 Writer/Reader 与 workSet/search）
	reWorkWorkSetRepo := reWorkWorkSet.NewRepository(app.db)
	// reWorkSetWorkSet 仓储（作品集间父子关联，workSet 传递包含原语 CollectDescendantWorkIDs 用）
	reWorkSetWorkSetRepo := reWorkSetWorkSet.NewRepository(app.db)

	// work 服务
	workRepo := work.NewRepository(app.db)
	workResourceStoreRepo := resource.NewResourceStoreRepository(app.db)
	app.WorkService = work.NewService(
		workRepo,
		&dbTransactorAdapter{db: app.db},
		app.LocalTagService,
		app.LocalAuthorService,
		app.SiteTagService,
		app.SiteAuthorService,
		app.SiteService,
		app.ResourceService,
		app.ReWorkTagService,
		reWorkWorkSetRepo,
		app.ResourceService,
		app.SiteAuthorService,
		app.SiteTagService,
		&workSetWriterAdapter{repo: workSetRepo},
		app.ReWorkAuthorService,
		app.LocalTagService,
		app.SiteTagService,
		app.SiteService,
		app.LocalAuthorService,
		app.ReWorkAuthorService,
		app.ResourceService,
		app.ResourceService, // ResourceStoreBatchReader(ListStoresByResourceIds)
		app.PersistentStoreService,
		app.ReWorkTagService,
		app.LocalTagService,
		app.LocalAuthorService,
		app.PersistentStoreService,
		nil,
		workResourceStoreRepo, // ResourceStoreHardDeleter(DeleteByResourceIds)
		reWorkSetWorkSetRepo,
	)

	// fsmonitor 工作目录监控服务（事件驱动监控外部文件操作 + workDir 切换暂停）
	// 操作抑制开关（D7）：按设置注入 storeRegistry，关闭则不抑制内部写入（退回误报原状态）
	storeRegistry.SetSuppressEnabled(app.SettingsService.GetSettings().FsmonitorSettings.SuppressEnabled)
	// D7 开关即时生效：设置保存/重置后联动同步到 storeRegistry（避免改开关需重启）
	app.SettingsService.SetAfterSave(func(s *settings.Settings) {
		storeRegistry.SetSuppressEnabled(s.FsmonitorSettings.SuppressEnabled)
	})
	// 启动监控前规范化 file_path 分隔符（历史数据统一正斜杠，避免对账路径比对误报）
	if n, err := app.PersistentStoreService.NormalizeFilePaths(context.Background()); err != nil {
		logger.Log.Warnf("规范化 file_path 分隔符失败: %v", err)
	} else if n > 0 {
		logger.Log.Infof("[persistentStore] file_path 分隔符规范化完成，共 %d 条", n)
	}
	cursorRepo := fsmonitor.NewCursorRepository(app.db)
	fsmonitorDeps := fsmonitor.NewPlatformDeps(app.SettingsService.GetWorkDir(), app.SettingsService.GetSettings().FsmonitorSettings.UsnEnabled, cursorRepo)
	fsmonitorDeps.StoreReader = &storeReaderAdapter{svc: app.PersistentStoreService}
	fsmonitorDeps.StoreRepairer = &storeRepairerAdapter{svc: app.PersistentStoreService}
	fsmonitorDeps.Scanner = fsmonitor.NewScanner(fsmonitorDeps.StoreReader, func() string { return app.SettingsService.GetWorkDir() })
	app.FsmonitorService = fsmonitor.NewService(
		fsmonitorDeps,
		func() string { return app.SettingsService.GetWorkDir() },
		func() fsmonitor.EventEmitter { return app.taskProgressEmitter },
	)
	// 把内容指纹计算器注入 persistentStore（落盘完成时同步算指纹落库）
	app.PersistentStoreService.SetFingerprinter(fingerprint.NewHeadComputer())
	// 存量指纹回填（异步，不阻塞启动）
	app.PersistentStoreService.BackfillFingerprints(context.Background())
	app.FsmonitorService.Start()

	// workSet 服务
	app.WorkSetService = workSet.NewService(workSetRepo, reWorkWorkSetRepo, reWorkSetWorkSetRepo, &dbTransactorAdapter{db: app.db}, app.WorkService, app.WorkService)

	// search 服务
	searchRepo := search.NewRepository(app.db)
	app.SearchService = search.NewService(
		searchRepo,
		reWorkWorkSetRepo,
		app.WorkService,
		app.ResourceService,
		app.PersistentStoreService, // StoreBatchReader
		app.ResourceService,        // ResourceStoreBatchReader
		app.LocalTagService,
		app.SiteTagService,
		app.LocalAuthorService,
		app.SiteAuthorService,
	)

	// recycleBin 服务（work 与 search 之后创建：WorkRestorer=app.WorkService、查询链转发 search.Service；
	// 文件还原的 workDir 经设置读取器闭包注入）
	app.RecycleBinService = recycleBin.NewService(
		app.WorkService,
		app.BackupService,
		app.SearchService,
		app.SettingsService,
		func() string { return app.SettingsService.GetWorkDir() },
	)
	// 启动 TTL 自动清理后台 goroutine（启动即清理一次 + 每 24h）
	app.RecycleBinService.StartCleanup()

	// siteBrowser 服务
	app.SiteBrowserService = siteBrowser.NewService(app.SiteBrowserRegistry)

	// pluginTaskUrlListener 服务
	pluginTaskUrlListenerManager := pluginTaskUrlListener.NewManager()
	app.PluginTaskUrlListenerSvc = pluginTaskUrlListener.NewService(pluginTaskUrlListenerManager)

	// plugin 服务
	app.pluginLoader = extension2.NewLoader(app.TaskHandlerRegistry, app.SiteBrowserRegistry)
	app.pluginLoader.SetUrlListenerCleaner(func(pluginPublicId string) {
		app.PluginTaskUrlListenerSvc.Unregister(pluginPublicId, "")
	})
	pluginRepo := plugin.NewRepository(app.db)
	app.PluginService = plugin.NewService(pluginRepo, app.BackupService)
	app.PluginStorageService = plugin.NewPluginStorageService(plugin.NewStorageRepository(app.db))
	app.PluginSettingService = plugin.NewPluginSettingService(pluginRepo, app.PluginStorageService, util.RootPath())
	app.PluginService.SetActivator(app)
	// 插件停用生命周期：loader 为运行时停止器（停进程+清其所属注册表），
	// 持运行时痕迹的模块注册为参与者（停用清理完备性的审计点），清单集中于此
	app.PluginService.SetRuntimeStopper(app.pluginLoader.UnloadPlugin)
	app.PluginService.RegisterLifecycleParticipant(&staticResourceParticipant{svc: app.StaticResourceService})
	app.PluginService.RegisterLifecycleParticipant(&frontendExtensionParticipant{registry: app.FrontendExtensionRegistry})
	// 崩溃路径与显式停用共用参与者清理集合：Loader 自身清理后回调触发参与者 OnStopped
	app.pluginLoader.SetCrashNotifier(func(pluginPublicId string) {
		app.PluginService.NotifyPluginCrashed(context.Background(), pluginPublicId)
	})
	app.PluginService.SetRuntimeStatusProvider(&runtimeStatusAdapter{loader: app.pluginLoader})
	app.PluginService.SetExtensionListProvider(&extensionListProviderAdapter{
		taskHandlerRegistry:       app.TaskHandlerRegistry,
		siteBrowserRegistry:       app.SiteBrowserRegistry,
		frontendExtensionRegistry: app.FrontendExtensionRegistry,
	})
	app.PluginService.SetUrlListenerProvider(&pluginUrlListenerAdapter{manager: pluginTaskUrlListenerManager})

	// task 仓储和服务
	app.taskRepo = task.NewRepository(app.db)

	// task 服务（依赖 TaskHandlerRegistry 作为 TaskHandlerProvider）
	app.TaskService = task.NewService(
		app.taskRepo,
		&dbTransactorAdapter{db: app.db}, // Transactor
		app.TaskHandlerRegistry,          // 直接满足 TaskHandlerProvider 接口
		app.PluginTaskUrlListenerSvc,
		app.SiteService,
	)

	// taskManager 服务
	var taskManagerPusher taskManager.TaskProgressPusher
	if app.taskProgressEmitter != nil {
		taskManagerPusher = taskManager.NewWailsTaskProgressPusher(app.taskProgressEmitter)
	} else {
		taskManagerPusher = taskManager.NewNoopProgressPusher()
	}
	pluginExecFactory := func(pluginPublicId string) (taskManager.TaskExecutor, error) {
		return extension2.NewTaskExecutor(app.TaskHandlerRegistry), nil
	}

	// 创建 ResourceSaver 适配器
	resourceSaverAdapter := &resourceSaverAdapter{svc: app.ResourceService}
	// 创建资源存储备份编排器
	backupResourceStoreRepo := resource.NewResourceStoreRepository(app.db)
	storeBackupOrchestrator := backup.NewStoreBackupOrchestrator(
		app.ResourceService,        // StoreResourceProvider
		backupResourceStoreRepo,    // StoreResourceStoreReader(resource_store 批量查询)
		app.PersistentStoreService, // StoreDeleter
		app.PersistentStoreService, // StoreImporter
		app.BackupService,          // BackupReader
	)
	// resource_store 仓储(taskManager 多轨续传使用;initBaseServices 中也有一个用于 ResourceService)
	taskMgrResourceStoreRepo := resource.NewResourceStoreRepository(app.db)

	app.TaskManagerService = taskManager.NewManager(
		app.SettingsService.GetSettings().ImportSettings.MaxParallelImport,
		app.taskRepo,
		taskManagerPusher,
		pluginExecFactory,
		&taskManager.TaskDeps{
			WorkInfoSaver:           app.WorkService, // 实现 WorkInfoSaver 接口
			WorkMetaLoader:          app.WorkService, // 实现 WorkMetaLoader 接口(资源板块单独重下时取命名元数据)
			ResourceSaver:           resourceSaverAdapter,
			WorkDirProvider:         app.SettingsService,
			FileNameFormatProvider:  app.SettingsService,
			WorkChecker:             app.WorkService,     // 实现 WorkChecker 接口
			ResourceReader:          app.ResourceService, // 实现 ResourceReader 接口
			StoreBackupOrchestrator: storeBackupOrchestrator,
			ResourceUpdater:         resourceSaverAdapter, // 实现 ResourceUpdater 接口
			Pusher:                  taskManagerPusher,
			StoreStreamer:           app.PersistentStoreService, // 实现 StoreStreamer 接口
			StoreReader:             app.PersistentStoreService, // 实现 StoreReader 接口
			ResourceStoreReader:     taskMgrResourceStoreRepo,   // 实现 ResourceStoreReader 接口
			ResourceStoreWriter:     taskMgrResourceStoreRepo,   // 实现 ResourceStoreWriter 接口
			WorkStoreRoleChecker:    app.ResourceService,        // 实现 WorkStoreRoleChecker 接口(覆盖确认行级判定)
			Transactor:              &dbTransactorAdapter{db: app.db},
			PendingResourceUpdater:  app.taskRepo,               // 实现 PendingResourceUpdater 接口
			StoreFileCleaner:        app.PersistentStoreService, // 实现 StoreFileCleaner 接口
			StoreDeleter:            app.PersistentStoreService, // 实现 StoreDeleter 接口
		},
	)
	// taskManager 在 Manager 创建后注册为参与者（拦截该插件运行中任务的停用/换版操作）
	app.PluginService.RegisterLifecycleParticipant(&taskManagerParticipant{mgr: app.TaskManagerService})

	// merge 合并服务（音视频合并编排；ffmpeg 缺失时 merger=nil，合并调用返回 ErrMergeUnavailable）
	mergeResourceStoreRepo := resource.NewResourceStoreRepository(app.db)
	mergeResourceRepo := resource.NewRepository(app.db)
	var mergeMerger resource.Merger
	if m, ferr := merge.NewFFmpegMuxer(); ferr == nil {
		mergeMerger = m
	} else {
		logger.Log.Warnf("ffmpeg 未安装，音视频合并功能不可用: %v", ferr)
	}
	app.MergeService = resource.NewMergeService(
		mergeResourceStoreRepo,
		mergeResourceRepo,
		mergeMerger,
		app.PersistentStoreService,
		app.SettingsService,
		&dbTransactorAdapter{db: app.db},
		resource.NewWailsMergeEmitter(func() resource.EventEmitter { return app.taskProgressEmitter }),
	)

	// 启动清理：合并产物临时文件残留（ls-merge-*，进程崩溃未 os.Remove 时残留）
	if err := app.MergeService.CleanupResidualTempFiles(context.Background()); err != nil {
		logger.Log.Warnf("清理合并产物临时残留失败: %v", err)
	}

	// 将 TaskManager 注入到 TaskService 作为内存状态提供者
	app.TaskService.SetMemoryProvider(app.TaskManagerService)

	// 将 TaskManager 注入到 work 作为运行中任务停止器（打破 work ↔ TaskManager 循环依赖）
	app.WorkService.SetRunningTaskStopper(app.TaskManagerService)

	// 注入原站序获取能力（plugin 提供，work 作品入库后异步拉取写 site_sort_order；registry 已就绪）
	app.WorkService.SetWorkSetOrderFetcher(extension2.NewWorkSetOrderFetcher(app.TaskHandlerRegistry, app.pluginLoader))
	// 注入作品集父集关系获取能力（plugin 提供，work 作品入库后异步拉取建立层级 + 写 site_sort_order）
	app.WorkService.SetWorkSetRelationFetcher(extension2.NewWorkSetRelationFetcher(app.TaskHandlerRegistry, app.pluginLoader))

	return nil
}

// workSetWriterAdapter WorkSetWriter 接口适配器（打破 work ↔ workSet 循环依赖）
type workSetWriterAdapter struct {
	repo *workSet.WorkSetRepository
}

// dbTransactorAdapter 数据库事务执行器适配器
type dbTransactorAdapter struct {
	db *gorm.DB
}

func (a *dbTransactorAdapter) ExecInTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	return database.WithTransactionContext(ctx, a.db, func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, database.TxKey, tx)
		return fn(txCtx)
	})
}

// storeReaderAdapter 将 persistentStore.Service 适配为 fsmonitor.StoreReader，
// domain.PersistentStore → fsmonitor.StoreRecord 转换，避免 fsmonitor 依赖 domain 实体
type storeReaderAdapter struct {
	svc *persistentStore.Service
}

func (a *storeReaderAdapter) GetByFingerprint(ctx context.Context, fingerprint string, excludePath string) (*fsmonitor.StoreRecord, error) {
	r, err := a.svc.GetByFingerprint(ctx, fingerprint, excludePath)
	if err != nil || r == nil {
		return nil, err
	}
	return toStoreRecord(r), nil
}

func (a *storeReaderAdapter) GetByFilePathComplete(ctx context.Context, filePath string) (*fsmonitor.StoreRecord, error) {
	r, err := a.svc.GetByFilePathComplete(ctx, filePath)
	if err != nil || r == nil {
		return nil, err
	}
	return toStoreRecord(r), nil
}

func (a *storeReaderAdapter) ListValidComplete(ctx context.Context) ([]fsmonitor.StoreRecord, error) {
	records, err := a.svc.ListValidComplete(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]fsmonitor.StoreRecord, 0, len(records))
	for _, r := range records {
		result = append(result, *toStoreRecord(r))
	}
	return result, nil
}

// storeRepairerAdapter 将 persistentStore.Service 适配为 fsmonitor.StoreRepairer
type storeRepairerAdapter struct {
	svc *persistentStore.Service
}

func (a *storeRepairerAdapter) UpdateFilePath(ctx context.Context, id int64, newFilePath string) error {
	return a.svc.UpdateFilePath(ctx, id, newFilePath)
}

func (a *storeRepairerAdapter) MarkInvalid(ctx context.Context, id int64) error {
	return a.svc.MarkInvalid(ctx, id)
}

func (a *storeRepairerAdapter) RenameDirectoryPrefix(ctx context.Context, oldPrefix string, newPrefix string) (int64, error) {
	return a.svc.RenameDirectoryPrefix(ctx, oldPrefix, newPrefix)
}

// toStoreRecord domain.PersistentStore → fsmonitor.StoreRecord
func toStoreRecord(r *entity2.PersistentStore) *fsmonitor.StoreRecord {
	rec := &fsmonitor.StoreRecord{ID: r.GetID()}
	if r.FilePath.Valid {
		rec.FilePath = r.FilePath.String
	}
	if r.ContentFingerprint.Valid {
		rec.ContentFingerprint = r.ContentFingerprint.String
	}
	return rec
}

func (a *workSetWriterAdapter) SaveOrUpdateByCompositeKey(ctx context.Context, ws *entity2.WorkSet) (int64, error) {
	existing, err := a.repo.GetBySiteAndSiteWorkSetID(ctx, ws.SiteID.Int64, ws.SiteWorkSetID.String)
	if err == nil && existing != nil {
		ws.ID = existing.ID
		if err := a.repo.Updates(ctx, ws); err != nil {
			return 0, err
		}
		return existing.ID, nil
	}
	if err := a.repo.Create(ctx, ws); err != nil {
		return 0, err
	}
	return ws.ID, nil
}

func (a *workSetWriterAdapter) GetBySiteAndSiteWorkSetID(ctx context.Context, siteId int64, siteWorkSetId string) (*entity2.WorkSet, error) {
	return a.repo.GetBySiteAndSiteWorkSetID(ctx, siteId, siteWorkSetId)
}

func (a *workSetWriterAdapter) BatchUpsert(ctx context.Context, workSets []*entity2.WorkSet) error {
	return a.repo.BatchUpsert(ctx, workSets)
}

func (a *workSetWriterAdapter) ListBySiteAndSiteWorkSetIDs(ctx context.Context, siteId int64, siteWorkSetIds []string) ([]*entity2.WorkSet, error) {
	return a.repo.ListBySiteAndSiteWorkSetIDs(ctx, siteId, siteWorkSetIds)
}

// ResourceSaverAdapter ResourceSaver 接口适配器
type resourceSaverAdapter struct {
	svc *resource.Service
}

func (a *resourceSaverAdapter) Save(ctx context.Context, resource *entity2.Resource) (int64, error) {
	if err := a.svc.Save(ctx, resource); err != nil {
		return 0, err
	}
	return resource.GetID(), nil
}

func (a *resourceSaverAdapter) Updates(ctx context.Context, resource *entity2.Resource) error {
	return a.svc.Updates(ctx, resource)
}

// initHandlers 初始化 Handlers（用于 Bind[] 参数暴露给前端）
func (app *App) initHandlers() {
	app.LocalTagHandler = localTag.NewHandler(app.LocalTagService)
	app.LocalAuthorHandler = localAuthor.NewHandler(app.LocalAuthorService)
	app.SiteTagHandler = siteTag.NewHandler(app.SiteTagService)
	app.SiteAuthorHandler = siteAuthor.NewHandler(app.SiteAuthorService)
	app.SiteHandler = site.NewHandler(app.SiteService)
	app.ResourceHandler = resource.NewHandler(app.ResourceService, app.MergeService)
	app.WorkHandler = work.NewHandler(app.WorkService)
	app.WorkSetHandler = workSet.NewHandler(app.WorkSetService)
	app.SearchHandler = search.NewHandler(app.SearchService)
	app.SettingsHandler = settings.NewHandler(app.SettingsService)
	app.AppLauncherHandler = appLauncher.NewHandler(app.AppLauncherService)
	app.FileSysUtilHandler = fileSysUtil.NewHandler(app.FileSysUtilService)
	app.FrontendLogHandler = frontendLog.NewHandler(app.FrontendLogService)
	app.PluginHandler = plugin.NewHandler(app.PluginService)
	app.PluginSettingHandler = plugin.NewSettingHandler(app.PluginSettingService)
	app.TaskHandler = task.NewHandler(app.TaskService)
	app.TaskManagerHandler = taskManager.NewHandler(app.TaskManagerService)
	app.FrontendExtensionHandler = extension2.NewFrontendExtensionHandler(app.FrontendExtensionRegistry)
	app.SiteBrowserHandler = siteBrowser.NewHandler(app.SiteBrowserService)
	app.ReWorkAuthorHandler = reWorkAuthor.NewHandler(app.ReWorkAuthorService)
	app.ReWorkTagHandler = reWorkTag.NewHandler(app.ReWorkTagService)
	app.PluginTaskUrlListenerHandler = pluginTaskUrlListener.NewHandler(app.PluginTaskUrlListenerSvc)
	app.RecycleBinHandler = recycleBin.NewHandler(app.RecycleBinService)
	app.FsmonitorHandler = fsmonitor.NewHandler(app.FsmonitorService)
	// 主窗口句柄实时获取（构造时窗口尚未创建，运行时通过 mainWindow 实时读取原生句柄）
	app.WindowHandler = window.NewHandler(window.NewService(func() uintptr {
		if app.mainWindow == nil {
			return 0
		}
		if handle := app.mainWindow.NativeWindow(); handle != nil {
			return uintptr(handle)
		}
		return 0
	}))
}

// onDomReady 窗口 DOM 准备就绪时的回调（内部使用，不暴露给前端）
func (app *App) onDomReady() {
	logger.Log.Info("[———————————————— Library Squirrel  已启动🛰️ ————————————————]")
}

// onBeforeClose 窗口关闭前的回调（内部使用，不暴露给前端）
// 返回 true 表示阻止关闭，false 表示允许关闭
func (app *App) onBeforeClose() bool {
	if app.TaskManagerService.IsIdle() {
		return false
	}

	if app.TaskManagerService.IsShuttingDown() {
		return true // 正在关闭中，阻止重复触发
	}

	logger.Log.Info("任务正在运行，开始暂停任务...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := app.TaskManagerService.GracefulShutdown(ctx); err != nil {
		logger.Log.Warnf("优雅关闭超时，强制退出: %v", err)
	}
	return false
}

// extensionListProviderAdapter 聚合三个 Registry 的扩展点查询能力
type extensionListProviderAdapter struct {
	taskHandlerRegistry       *extension2.TaskHandlerRegistry
	siteBrowserRegistry       *extension2.SiteBrowserRegistry
	frontendExtensionRegistry *extension2.FrontendExtensionRegistry
}

func (a *extensionListProviderAdapter) GetTaskHandlersByPlugin(pluginPublicId string) []plugin.ExtensionMeta {
	exts, _ := a.taskHandlerRegistry.GetByPlugin(pluginPublicId)
	result := make([]plugin.ExtensionMeta, 0, len(exts))
	for _, ext := range exts {
		result = append(result, plugin.ExtensionMeta{ID: ext.Metadata.ID, Name: ext.Metadata.Name, Description: ext.Metadata.Description})
	}
	return result
}

func (a *extensionListProviderAdapter) GetSiteBrowsersByPlugin(pluginPublicId string) []plugin.ExtensionMeta {
	exts, _ := a.siteBrowserRegistry.GetByPlugin(pluginPublicId)
	result := make([]plugin.ExtensionMeta, 0, len(exts))
	for _, ext := range exts {
		result = append(result, plugin.ExtensionMeta{ID: ext.Metadata.ID, Name: ext.Metadata.Name, Description: ext.Metadata.Description})
	}
	return result
}

func (a *extensionListProviderAdapter) GetFrontendExtensionsByPlugin(pluginPublicId string) []plugin.FrontendExtensionMeta {
	exts, _ := a.frontendExtensionRegistry.GetByPlugin(pluginPublicId)
	result := make([]plugin.FrontendExtensionMeta, 0, len(exts))
	for _, ext := range exts {
		cfg := ext.Instance
		result = append(result, plugin.FrontendExtensionMeta{ID: cfg.Metadata.ID, Name: cfg.Metadata.Name, Kind: string(cfg.Kind)})
	}
	return result
}

// staticResourceParticipant 静态资源生命周期参与者：停用后注销插件静态资源目录
type staticResourceParticipant struct {
	svc *extension2.StaticResourceService
}

func (p *staticResourceParticipant) PrepareStop(ctx context.Context, pluginPublicId string, op plugin.PluginStopOp, force bool) error {
	return nil // 静态资源无否决条件
}

func (p *staticResourceParticipant) OnStopped(ctx context.Context, pluginPublicId string) {
	p.svc.UnregisterPlugin(pluginPublicId)
}

// frontendExtensionParticipant 前端扩展生命周期参与者：停用后注销全部前端扩展（触发前端注销事件）
type frontendExtensionParticipant struct {
	registry *extension2.FrontendExtensionRegistry
}

func (p *frontendExtensionParticipant) PrepareStop(ctx context.Context, pluginPublicId string, op plugin.PluginStopOp, force bool) error {
	return nil // 前端扩展注册无否决条件
}

func (p *frontendExtensionParticipant) OnStopped(ctx context.Context, pluginPublicId string) {
	if err := p.registry.UnregisterAll(pluginPublicId); err != nil {
		logger.Log.Warnf("停用注销前端扩展失败: %s, %v", pluginPublicId, err)
	}
}

// taskManagerParticipant 任务生命周期参与者：停用/换版前拦截该插件运行中任务的操作。
// 卸载/更新/重装统一拦「运行中」（Paused 不拦，存续由用户处置）；取消信任不否决——
// 安全意图执行，任务代价由前端确认对话框明示
type taskManagerParticipant struct {
	mgr *taskManager.Manager
}

func (p *taskManagerParticipant) PrepareStop(ctx context.Context, pluginPublicId string, op plugin.PluginStopOp, force bool) error {
	if op == plugin.PluginStopOpUntrust {
		return nil
	}
	if n := p.mgr.CountActiveByPlugin(pluginPublicId); n > 0 {
		return fmt.Errorf("该插件有 %d 个运行中任务，请先暂停或等待其完成后再操作", n)
	}
	return nil
}

func (p *taskManagerParticipant) OnStopped(ctx context.Context, pluginPublicId string) {
	// 任务侧无停用后清理：非运行中任务保留原状，插件重装后由用户手动恢复
}

// runtimeStatusAdapter 将 extension.RuntimeStatus 适配为 plugin.RuntimeStatus
type runtimeStatusAdapter struct {
	loader *extension2.Loader
}

func (a *runtimeStatusAdapter) GetPluginRuntimeStatus(pluginPublicId string) *plugin.RuntimeStatus {
	rt := a.loader.GetPluginRuntimeStatus(pluginPublicId)
	return &plugin.RuntimeStatus{
		IsRunning:   rt.IsRunning,
		PID:         rt.PID,
		ActivatedAt: rt.ActivatedAt.UnixMilli(),
	}
}

// urlListenerAdapter 将 Manager 适配为 plugin.UrlListenerProvider
type pluginUrlListenerAdapter struct {
	manager *pluginTaskUrlListener.Manager
}

func (a *pluginUrlListenerAdapter) ListPatternsByPlugin(pluginPublicId string) []string {
	return a.manager.ListPatternsByPlugin(pluginPublicId)
}

// shutdownPlugins 关闭所有已加载的插件（停止子进程、注销扩展点和静态资源）
func (app *App) shutdownPlugins() {
	// 停止回收站 TTL 自动清理 goroutine（在 database.Close 前停止，避免操作已关闭的 DB）
	if app.RecycleBinService != nil {
		app.RecycleBinService.Stop()
	}

	// 停止工作目录监控（在数据库关闭前停止，避免操作已关闭的资源）
	if app.FsmonitorService != nil {
		app.FsmonitorService.Stop()
	}

	if app.pluginLoader == nil {
		return
	}

	// UnloadAll 返回已卸载的插件 ID 列表，并已注销 TaskHandler/SiteBrowser
	ids := app.pluginLoader.UnloadAll()

	// 逐个清理静态资源和前端扩展（复用卸载时的 onUnload 逻辑）
	for _, id := range ids {
		app.StaticResourceService.UnregisterPlugin(id)
		app.FrontendExtensionRegistry.UnregisterAll(id)
	}

	// 清理纯 UI 插件注册的前端扩展（没有运行时进程的插件不在 Loader.processes 中）
	if app.FrontendExtensionRegistry != nil {
		app.FrontendExtensionRegistry.UnregisterAll("")
	}

	logger.Log.Infof("所有插件已关闭，共 %d 个运行时插件", len(ids))
}
