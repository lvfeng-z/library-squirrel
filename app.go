package main

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"

	"github.com/library-squirrel/backend/base"
	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/dto"
	entity2 "github.com/library-squirrel/backend/base/model/entity"
	extension2 "github.com/library-squirrel/backend/plugin/extension"

	pluginsdk "github.com/lvfeng-z/library-squirrel-plugin-sdk"
	"gorm.io/gorm"

	"github.com/library-squirrel/backend/assetserver"
	"github.com/library-squirrel/backend/appLauncher"
	"github.com/library-squirrel/backend/backup"
	"github.com/library-squirrel/backend/config"
	"github.com/library-squirrel/backend/database"
	"github.com/library-squirrel/backend/fileSysUtil"
	"github.com/library-squirrel/backend/localAuthor"
	"github.com/library-squirrel/backend/localTag"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/plugin"
	"github.com/library-squirrel/backend/pluginTaskUrlListener"
	"github.com/library-squirrel/backend/reWorkAuthor"
	"github.com/library-squirrel/backend/reWorkTag"
	"github.com/library-squirrel/backend/reWorkWorkSet"
	"github.com/library-squirrel/backend/resource"
	"github.com/library-squirrel/backend/search"
	"github.com/library-squirrel/backend/secureStorage"
	"github.com/library-squirrel/backend/settings"
	"github.com/library-squirrel/backend/site"
	"github.com/library-squirrel/backend/siteAuthor"
	"github.com/library-squirrel/backend/siteBrowser"
	"github.com/library-squirrel/backend/siteTag"
	"github.com/library-squirrel/backend/slot"
	"github.com/library-squirrel/backend/task"
	"github.com/library-squirrel/backend/taskManager"
	"github.com/library-squirrel/backend/util"
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
	LocalTagService      *localTag.Service
	LocalAuthorService   *localAuthor.Service
	SiteTagService       *siteTag.Service
	SiteAuthorService    *siteAuthor.Service
	SiteService          *site.Service
	ResourceService      *resource.Service
	ReWorkAuthorService  *reWorkAuthor.Service
	ReWorkTagService     *reWorkTag.Service
	WorkService          *work.Service
	WorkSetService       *workSet.Service
	SearchService        *search.Service
	SettingsService      *settings.Service
	SecureStorageService *secureStorage.Service
	BackupService        *backup.Service
	AppLauncherService   *appLauncher.Service
	FileSysUtilService   *fileSysUtil.Service
	PluginService        *plugin.Service
	TaskService          *task.Service
	TaskManagerService   *taskManager.Manager
	SlotService          *slot.SlotSyncService
	SiteBrowserService   *siteBrowser.Service

	// 任务仓储（用于TaskManager）
	taskRepo task.Repository

	// 扩展注册中心
	TaskHandlerRegistry *extension2.TaskHandlerRegistry
	SiteBrowserRegistry *extension2.SiteBrowserRegistry
	SlotRegistry        *extension2.SlotRegistry

	// 插件加载器
	pluginLoader *extension2.Loader

	// 静态资源服务
	StaticResourceService *extension2.StaticResourceService

	// HTTP 路由
	AssetRouter      *assetserver.Router
	HttpFileHandler *assetserver.ResourceHandler

	// 任务URL监听器
	PluginTaskUrlListenerSvc *pluginTaskUrlListener.Service

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
	SecureStorageHandler         *secureStorage.Handler
	BackupHandler                *backup.Handler
	AppLauncherHandler           *appLauncher.Handler
	FileSysUtilHandler           *fileSysUtil.Handler
	PluginHandler                *plugin.Handler
	TaskHandler                  *task.Handler
	TaskManagerHandler           *taskManager.Handler
	SlotHandler                  *slot.Handler
	SiteBrowserHandler           *siteBrowser.Handler
	ReWorkAuthorHandler          *reWorkAuthor.Handler
	ReWorkTagHandler             *reWorkTag.Handler
	PluginTaskUrlListenerHandler *pluginTaskUrlListener.Handler
}

// NewApp 创建Wails应用实例
func NewApp() (*App, error) {
	app := &App{}

	// 1. 加载配置
	cfg, err := config.Load("config.yaml")
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
	app.SlotRegistry = extension2.NewSlotRegistry()
	// SlotPusher 会在 SetEventEmitter 中创建并接入 SlotRegistry

	// 3.5 初始化静态资源服务
	app.StaticResourceService = extension2.NewStaticResourceService()
	app.HttpFileHandler = assetserver.NewResourceHandler()

	// 4. 初始化基础服务（按依赖顺序）
	app.initBaseServices()

	// 5. 初始化高级服务
	if err := app.initAdvancedServices(); err != nil {
		return nil, err
	}

	// 6. 初始化 Handlers
	app.initHandlers()

	// 7. 加载已安装的插件
	app.loadInstalledPlugins()

	return app, nil
}

// SetEventEmitter 设置 Wails 事件发射器并创建 SlotPusher
func (app *App) SetEventEmitter(emitter extension2.WailsEventEmitter) {
	pusher := extension2.NewWailsSlotPusher(emitter)
	app.SlotRegistry.SetPusher(pusher)
}

// CreateAssetHandler 创建路由多路复用器并注册所有路由
func (app *App) CreateAssetHandler(frontendAssets fs.FS) http.Handler {
	router := assetserver.NewRouter(frontendAssets)
	router.Handle("/plugin/", app.StaticResourceService, 0)
	router.Handle("/resource/", app.HttpFileHandler, 0)
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

	rootPath := util.RootPath()
	runtimeLoaded := 0
	pureUICount := 0

	for _, p := range plugins {
		// 跳过已卸载的插件
		if p.Uninstalled.Valid && p.Uninstalled.Int64 == plugin.UninstalledTrue {
			continue
		}

		// 跳过没有 PublicID 的插件
		if !p.PublicID.Valid || p.PublicID.String == "" {
			continue
		}

		// 跳过没有 RootPath 的插件
		if !p.RootPath.Valid || p.RootPath.String == "" {
			continue
		}

		publicId := p.PublicID.String
		pluginRootDir := filepath.Join(rootPath, p.RootPath.String)
		logger.Log.Infof("正在加载插件: %s (root=%s)", publicId, pluginRootDir)

		// 读取 plugin.json
		manifestPath := filepath.Join(pluginRootDir, "plugin.json")
		manifestBytes, err := os.ReadFile(manifestPath)
		if err != nil {
			logger.Log.Errorf("读取 plugin.json 失败 %s: %v", publicId, err)
			continue
		}

		// 解析 manifest
		var manifest dto.PluginManifest
		if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
			logger.Log.Errorf("解析 plugin.json 失败 %s: %v", publicId, err)
			continue
		}

		ext := manifest.Extensions
		if ext == nil {
			logger.Log.Warnf("插件 %s 无扩展点，跳过", publicId)
			continue
		}

		// 注册静态资源
		var allowedDirs []string
		if ext.StaticResources != nil {
			allowedDirs = ext.StaticResources.Directories
		}
		version := ""
		if p.Version.Valid {
			version = p.Version.String
		}
		app.StaticResourceService.RegisterPlugin(publicId, pluginRootDir, allowedDirs, version)
		logger.Log.Infof("插件 %s: 静态资源已注册 (dirs=%v)", publicId, allowedDirs)

		// 声明式注册 Slot
		for _, slot := range ext.Slots {
			slotConfig := base.NewSlotConfig()
			slotConfig.ExtensionMetadata.ID = slot.ID
			slotConfig.ExtensionMetadata.PluginID = p.GetID()
			slotConfig.ExtensionMetadata.PluginPublicID = publicId
			slotConfig.ExtensionMetadata.Name = slot.Name
			slotConfig.ExtensionMetadata.Description = slot.Description
			slotConfig.SlotType = base.SlotType(slot.SlotType)
			slotConfig.ContentType = base.ContentType(slot.ContentType)
			slotConfig.Content = resolveContentURLs(slot.Content, slot.ContentType, publicId, version)
			slotConfig.Title = slot.Title
			slotConfig.Order = slot.Order
			slotConfig.Position = slot.Position
			slotConfig.Width = slot.Width
			slotConfig.Height = slot.Height
			slotConfig.ViewId = slot.ViewId
			slotConfig.ContributionId = slot.ContributionId

			// 将相对路径的 Icon 转换为 URL
			if slot.Icon != "" {
				slotConfig.Icon = app.StaticResourceService.ResolveURL(publicId, version, slot.Icon)
			}

			extension := model.NewExtension(*slotConfig.ExtensionMetadata, slotConfig)
			if err := app.SlotRegistry.Register(extension); err != nil {
				logger.Log.Errorf("注册 Slot 失败 %s/%s: %v", publicId, slot.ID, err)
			}
		}

		if len(ext.Slots) > 0 {
			logger.Log.Infof("插件 %s: 已注册 %d 个 Slot", publicId, len(ext.Slots))
		}

		// 判断是否为纯 UI 插件（无运行时扩展点）
		hasRuntime := len(ext.TaskHandlers) > 0 || len(ext.SiteBrowsers) > 0
		if !hasRuntime {
			pureUICount++
			logger.Log.Infof("插件 %s: 纯 UI 插件，跳过子进程", publicId)
			continue
		}

		// 非纯 UI 插件：以子进程模式加载运行时
		if !p.EntryPath.Valid || p.EntryPath.String == "" {
			logger.Log.Warnf("插件 %s 有运行时扩展点但无入口路径", publicId)
			continue
		}

		pluginPath := filepath.Join(rootPath, p.EntryPath.String)
		pluginInfo := &extension2.PluginInfo{
			ID:        p.GetID(),
			PublicID:  publicId,
			Name:      p.Name.String,
			Version:   p.Version.String,
			Author:    p.Author.String,
			EntryPath: p.EntryPath.String,
			RootPath:  p.RootPath.String,
		}

		pluginCtx := extension2.NewPluginContext(extension2.PluginContextDeps{
			PluginInfo:          pluginInfo,
			RootPath:            rootPath,
			TaskHandlerRegistry: app.TaskHandlerRegistry,
			SiteBrowserRegistry: app.SiteBrowserRegistry,
			PluginData:          app.PluginService,
			SecureStorage:       app.SecureStorageService,
			WorkSetQuery:        app.WorkSetService,
			SiteSave:            app.SiteService,
			TaskCreate:          &taskCreateAdapter{svc: app.TaskService},
			UrlListener:         &urlListenerAdapter{svc: app.PluginTaskUrlListenerSvc, pluginEntity: p},
		})

		logger.Log.Infof("插件 %s: 正在启动子进程 %s", publicId, pluginPath)
		if err := app.pluginLoader.LoadPluginProcess(pluginPath, publicId, extension2.PluginProcessDeps{
			PluginInfo:           pluginInfo,
			PluginCtx:            pluginCtx,
			TaskHandlerRegistry:  app.TaskHandlerRegistry,
			SiteBrowserRegistry:  app.SiteBrowserRegistry,
		}); err != nil {
			logger.Log.Errorf("加载插件失败 %s: %v", publicId, err)
			continue
		}
		runtimeLoaded++
	}

	logger.Log.Infof("插件加载完成: %d 个运行时, %d 个纯 UI, 共 %d 个", runtimeLoaded, pureUICount, len(plugins))
}

// resolveContentURLs 将 Slot Content 中的相对路径转换为完整的 resource:// URL
// 支持 vueSource、precompiled、html 类型，code 类型为行内代码无需转换
func resolveContentURLs(content json.RawMessage, contentType, publicId, version string) json.RawMessage {
	if len(content) == 0 || contentType == "code" {
		return content
	}

	var c map[string]string
	if err := json.Unmarshal(content, &c); err != nil {
		return content
	}

	prefix := "/plugin/" + publicId + "/" + version + "/"
	for key, path := range c {
		if path != "" {
			c[key] = prefix + path
		}
	}

	result, err := json.Marshal(c)
	if err != nil {
		return content
	}
	return result
}
type taskCreateAdapter struct {
	svc *task.Service
}

func (a *taskCreateAdapter) CreateTaskByURL(ctx context.Context, url string) (*pluginsdk.TaskCreateResult, error) {
	resp, err := a.svc.CreateTaskByURL(ctx, url)
	if err != nil {
		return nil, err
	}
	return &pluginsdk.TaskCreateResult{
		Succeed:       resp.Succeed,
		AddedQuantity: resp.AddedQuantity,
		Msg:           resp.Msg,
	}, nil
}

// urlListenerAdapter 适配 PluginTaskUrlListener.Service 到 UrlListenerRegistry 接口
type urlListenerAdapter struct {
	svc          *pluginTaskUrlListener.Service
	pluginEntity *entity2.Plugin
}

func (a *urlListenerAdapter) RegisterUrlListener(pluginPublicId string, contributionId string, patterns []string) {
	pwc := &pluginTaskUrlListener.PluginWithContribution{
		Plugin:         a.pluginEntity,
		ContributeKey:  "taskHandler",
		ContributionID: contributionId,
	}
	a.svc.Register(pwc, patterns)
}

func (a *urlListenerAdapter) UnregisterUrlListener(pluginPublicId string) {
	a.svc.Unregister(pluginPublicId)
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
	app.ResourceService = resource.NewService(resourceRepo)

	// reWorkAuthor 服务
	reWorkAuthorRepo := reWorkAuthor.NewRepository(app.db)
	app.ReWorkAuthorService = reWorkAuthor.NewService(reWorkAuthorRepo)

	// reWorkTag 服务
	reWorkTagRepo := reWorkTag.NewRepository(app.db)
	app.ReWorkTagService = reWorkTag.NewService(reWorkTagRepo)

	// settings 服务
	settingsFilePath := filepath.Join(rootPath, "config/settings.json")
	app.SettingsService = settings.NewService(settingsFilePath)

	// 设置工作目录
	app.HttpFileHandler.SetWorkDir(app.SettingsService.GetWorkDir())

	// secureStorage 服务
	secureStorageRepo := secureStorage.NewRepository(app.db)
	app.SecureStorageService = secureStorage.NewService(secureStorageRepo)

	// backup 服务
	backupRepo := backup.NewRepository(app.db)
	app.BackupService = backup.NewService(backupRepo)

	// appLauncher 服务
	app.AppLauncherService = appLauncher.NewService(rootPath)

	// fileSysUtil 服务
	app.FileSysUtilService = fileSysUtil.NewService(rootPath)
}

// initAdvancedServices 初始化高级服务（依赖其他服务）
func (app *App) initAdvancedServices() error {
	// workSet 仓储（提前创建，用于 workSetWriterAdapter）
	workSetRepo := workSet.NewRepository(app.db)

	// work 服务
	workRepo := work.NewRepository(app.db)
	app.WorkService = work.NewService(
		workRepo,
		app.LocalTagService,
		app.LocalAuthorService,
		app.SiteTagService,
		app.SiteAuthorService,
		app.SiteService,
		app.ResourceService,
		app.ReWorkTagService,
		reWorkWorkSet.NewRepository(app.db),
		app.ResourceService,
		app.SiteAuthorService,
		app.SiteTagService,
		&workSetWriterAdapter{repo: workSetRepo},
		app.ReWorkAuthorService,
	)

	// workSet 服务
	app.WorkSetService = workSet.NewService(workSetRepo, reWorkWorkSet.NewRepository(app.db), app.WorkService)

	// search 服务
	searchRepo := search.NewRepository(app.db)
	app.SearchService = search.NewService(
		searchRepo,
		app.LocalTagService,
		app.SiteTagService,
		app.LocalAuthorService,
		app.SiteAuthorService,
	)

	// slot 服务
	app.SlotService = slot.NewSlotSyncService(app.SlotRegistry)

	// siteBrowser 服务
	app.SiteBrowserService = siteBrowser.NewService(app.SiteBrowserRegistry)

	// plugin 服务
	app.pluginLoader = extension2.NewLoader(app.TaskHandlerRegistry, app.SiteBrowserRegistry)
	pluginRepo := plugin.NewRepository(app.db)
	app.PluginService = plugin.NewService(pluginRepo, app.BackupService)
	app.PluginService.SetOnUnload(func(pluginPublicId string) {
		app.pluginLoader.UnloadPlugin(pluginPublicId)
		app.StaticResourceService.UnregisterPlugin(pluginPublicId)
		app.SlotRegistry.UnregisterAll(pluginPublicId)
	})

	// pluginTaskUrlListener 服务
	pluginTaskUrlListenerManager := pluginTaskUrlListener.NewManager()
	app.PluginTaskUrlListenerSvc = pluginTaskUrlListener.NewService(pluginTaskUrlListenerManager)

	// task 仓储和服务
	app.taskRepo = task.NewRepository(app.db)

	// task 服务（依赖 pluginLoader 作为 TaskHandlerProvider）
	app.TaskService = task.NewService(
		app.taskRepo,
		app.WorkService,
		app.ResourceService,
		app.pluginLoader, // 实现 TaskHandlerProvider 接口
		app.PluginTaskUrlListenerSvc,
		app.SiteService,
	)

	// taskManager 服务
	taskManagerPusher := taskManager.NewSSEProgressPusher()
	pluginExecFactory := func(pluginPublicId string) (taskManager.TaskExecutor, error) {
		return extension2.NewTaskExecutor(app.pluginLoader), nil
	}

	// 创建 WorkInfoSaver 和 ResourceSaver 适配器
	workInfoSaverAdapter := &workInfoSaverAdapter{svc: app.WorkService}
	resourceSaverAdapter := &resourceSaverAdapter{svc: app.ResourceService}

	app.TaskManagerService = taskManager.NewManager(
		app.SettingsService.GetSettings().ImportSettings.MaxParallelImport,
		app.SettingsService,
		app.taskRepo,
		taskManagerPusher,
		pluginExecFactory,
		workInfoSaverAdapter,
		resourceSaverAdapter,
	)

	return nil
}

// workInfoSaverAdapter WorkInfoSaver 接口适配器
type workInfoSaverAdapter struct {
	svc *work.Service
}

func (a *workInfoSaverAdapter) SaveWorkInfo(ctx context.Context, task *entity2.Task, workResp *dto.WorkResponse) (int64, error) {
	return a.svc.SaveWorkInfo(ctx, task, workResp)
}

// workSetWriterAdapter WorkSetWriter 接口适配器（打破 work ↔ workSet 循环依赖）
type workSetWriterAdapter struct {
	repo *workSet.WorkSetRepository
}

func (a *workSetWriterAdapter) SaveOrUpdateByCompositeKey(ctx context.Context, ws *entity2.WorkSet) (int64, error) {
	existing, err := a.repo.GetBySiteAndSiteWorkSetID(ctx, ws.SiteID.Int64, ws.SiteWorkSetID.String)
	if err == nil && existing != nil {
		ws.ID = existing.ID
		if err := a.repo.Update(ctx, ws); err != nil {
			return 0, err
		}
		return existing.ID, nil
	}
	if err := a.repo.Save(ctx, ws); err != nil {
		return 0, err
	}
	return ws.ID, nil
}

func (a *workSetWriterAdapter) GetBySiteAndSiteWorkSetID(ctx context.Context, siteId int64, siteWorkSetId string) (*entity2.WorkSet, error) {
	return a.repo.GetBySiteAndSiteWorkSetID(ctx, siteId, siteWorkSetId)
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

// initHandlers 初始化 Handlers（用于 Bind[] 参数暴露给前端）
func (app *App) initHandlers() {
	app.LocalTagHandler = localTag.NewHandler(app.LocalTagService)
	app.LocalAuthorHandler = localAuthor.NewHandler(app.LocalAuthorService)
	app.SiteTagHandler = siteTag.NewHandler(app.SiteTagService)
	app.SiteAuthorHandler = siteAuthor.NewHandler(app.SiteAuthorService)
	app.SiteHandler = site.NewHandler(app.SiteService)
	app.ResourceHandler = resource.NewHandler(app.ResourceService)
	app.WorkHandler = work.NewHandler(app.WorkService)
	app.WorkSetHandler = workSet.NewHandler(app.WorkSetService)
	app.SearchHandler = search.NewHandler(app.SearchService)
	app.SettingsHandler = settings.NewHandler(app.SettingsService)
	app.SecureStorageHandler = secureStorage.NewHandler(app.SecureStorageService)
	app.BackupHandler = backup.NewHandler(app.BackupService)
	app.AppLauncherHandler = appLauncher.NewHandler(app.AppLauncherService)
	app.FileSysUtilHandler = fileSysUtil.NewHandler(app.FileSysUtilService)
	app.PluginHandler = plugin.NewHandler(app.PluginService)
	app.TaskHandler = task.NewHandler(app.TaskService)
	app.TaskManagerHandler = taskManager.NewHandler(app.TaskManagerService)
	app.SlotHandler = slot.NewHandler(app.SlotService)
	app.SiteBrowserHandler = siteBrowser.NewHandler(app.SiteBrowserService)
	app.ReWorkAuthorHandler = reWorkAuthor.NewHandler(app.ReWorkAuthorService)
	app.ReWorkTagHandler = reWorkTag.NewHandler(app.ReWorkTagService)
	app.PluginTaskUrlListenerHandler = pluginTaskUrlListener.NewHandler(app.PluginTaskUrlListenerSvc)
}

// onDomReady 窗口 DOM 准备就绪时的回调（内部使用，不暴露给前端）
func (app *App) onDomReady() {
	logger.Log.Info("窗口 DOM 已就绪")
}

// onBeforeClose 窗口关闭前的回调（内部使用，不暴露给前端）
// 返回 true 表示阻止关闭，false 表示允许关闭
func (app *App) onBeforeClose() bool {
	// 检查任务队列是否空闲
	if !app.TaskManagerService.IsIdle() {
		logger.Log.Info("任务正在运行，取消窗口关闭")
		// TODO: 显示确认对话框让用户选择是否强制关闭
		// 在 Wails v3 中，可以通过 dialog.MessageBox 或前端对话框实现
		return true // 阻止关闭，等待任务完成
	}
	logger.Log.Info("窗口关闭，所有任务已完成")
	return false // 允许关闭
}
