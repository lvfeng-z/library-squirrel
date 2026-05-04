package main

import (
	"context"
	"path/filepath"

	extension2 "github.com/library-squirrel/wails/internal/plugin/extension"
	entity2 "github.com/library-squirrel/wails/pkg/model/entity"
	"gorm.io/gorm"

	"github.com/library-squirrel/wails/internal/appLauncher"
	"github.com/library-squirrel/wails/internal/backup"
	"github.com/library-squirrel/wails/internal/config"
	"github.com/library-squirrel/wails/internal/database"
	"github.com/library-squirrel/wails/internal/fileSysUtil"
	"github.com/library-squirrel/wails/internal/localAuthor"
	"github.com/library-squirrel/wails/internal/localTag"
	"github.com/library-squirrel/wails/internal/migration"
	"github.com/library-squirrel/wails/internal/plugin"
	"github.com/library-squirrel/wails/internal/pluginTaskUrlListener"
	"github.com/library-squirrel/wails/internal/reWorkAuthor"
	"github.com/library-squirrel/wails/internal/reWorkTag"
	"github.com/library-squirrel/wails/internal/reWorkWorkSet"
	"github.com/library-squirrel/wails/internal/resource"
	"github.com/library-squirrel/wails/internal/search"
	"github.com/library-squirrel/wails/internal/secureStorage"
	"github.com/library-squirrel/wails/internal/settings"
	"github.com/library-squirrel/wails/internal/site"
	"github.com/library-squirrel/wails/internal/siteAuthor"
	"github.com/library-squirrel/wails/internal/siteBrowser"
	"github.com/library-squirrel/wails/internal/siteTag"
	"github.com/library-squirrel/wails/internal/slot"
	"github.com/library-squirrel/wails/internal/task"
	"github.com/library-squirrel/wails/internal/taskManager"
	"github.com/library-squirrel/wails/internal/util"
	"github.com/library-squirrel/wails/internal/work"
	"github.com/library-squirrel/wails/internal/workSet"
	"github.com/library-squirrel/wails/pkg/logger"
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
		logger.Log.Errorf("Failed to load config: %v", err)
		return nil, err
	}
	app.cfg = cfg
	logger.Log.Infof("Config loaded")

	// 2. 初始化数据库
	dbPath := filepath.Join(util.RootPath(), "database/database.db")
	if err := database.Init(dbPath); err != nil {
		logger.Log.Errorf("Failed to init database: %v", err)
		return nil, err
	}
	app.db = database.GetDB()
	logger.Log.Infof("Database initialized: %s", dbPath)

	// 自动迁移数据库表结构
	if err := migration.AutoMigrate(app.db); err != nil {
		logger.Log.Errorf("Failed to auto migrate database: %v", err)
		return nil, err
	}
	logger.Log.Infof("Database migration completed")

	// 3. 初始化扩展注册中心
	app.TaskHandlerRegistry = extension2.NewTaskHandlerRegistry()
	app.SiteBrowserRegistry = extension2.NewSiteBrowserRegistry()
	app.SlotRegistry = extension2.NewSlotRegistry()
	// SlotPusher 会在 SetEventEmitter 中创建并接入 SlotRegistry

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

// loadInstalledPlugins 加载所有已安装且需要启动时激活的插件
func (app *App) loadInstalledPlugins() {
	ctx := context.Background()

	// 查询所有未卸载的插件
	plugins, err := app.PluginService.List(ctx, &database.QueryOption{})
	if err != nil {
		logger.Log.Errorf("Failed to query installed plugins: %v", err)
		return
	}

	rootPath := util.RootPath()
	loaded := 0

	for _, p := range plugins {
		// 跳过已卸载的插件
		if p.Uninstalled.Valid && p.Uninstalled.Int64 == plugin.UninstalledTrue {
			continue
		}

		// 跳过没有入口文件的插件
		if !p.EntryPath.Valid || p.EntryPath.String == "" {
			continue
		}

		// 跳过没有 PublicID 的插件
		if !p.PublicID.Valid || p.PublicID.String == "" {
			continue
		}

		pluginPath := filepath.Join(rootPath, p.EntryPath.String)
		pluginInfo := &extension2.PluginInfo{
			ID:        p.GetID(),
			PublicID:  p.PublicID.String,
			Name:      p.Name.String,
			Version:   p.Version.String,
			Author:    p.Author.String,
			EntryPath: p.EntryPath.String,
			RootPath:  p.RootPath.String,
		}

		// 构建插件上下文
		pluginCtx := extension2.NewPluginContext(extension2.PluginContextDeps{
			PluginInfo:          pluginInfo,
			RootPath:            rootPath,
			TaskHandlerRegistry: app.TaskHandlerRegistry,
			SiteBrowserRegistry: app.SiteBrowserRegistry,
			SlotRegistry:        app.SlotRegistry,
			PluginData:          app.PluginService,
			SecureStorage:       app.SecureStorageService,
			WorkSetQuery:        app.WorkSetService,
			SiteSave:            app.SiteService,
			TaskCreate:          &taskCreateAdapter{svc: app.TaskService},
			UrlListener:         &urlListenerAdapter{svc: app.PluginTaskUrlListenerSvc, pluginEntity: p},
		})

		if err := app.pluginLoader.LoadPlugin(pluginPath, p.PublicID.String, pluginCtx); err != nil {
			logger.Log.Errorf("Failed to load plugin %s: %v", p.PublicID.String, err)
			continue
		}
		loaded++
	}

	logger.Log.Infof("Plugins loaded: %d/%d", loaded, len(plugins))
}

// taskCreateAdapter 适配 task.Service.CreateTaskByURL 到 TaskCreateProvider 接口
type taskCreateAdapter struct {
	svc *task.Service
}

func (a *taskCreateAdapter) CreateTaskByURL(ctx context.Context, url string) (*extension2.TaskCreateResult, error) {
	resp, err := a.svc.CreateTaskByURL(ctx, url)
	if err != nil {
		return nil, err
	}
	return &extension2.TaskCreateResult{
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
	)

	// workSet 服务
	workSetRepo := workSet.NewRepository(app.db)
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
	app.pluginLoader = extension2.NewLoader(app.TaskHandlerRegistry, app.SiteBrowserRegistry, app.SlotRegistry)
	pluginRepo := plugin.NewRepository(app.db)
	app.PluginService = plugin.NewService(pluginRepo, app.BackupService)

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

	// 创建 WorkSaver 和 ResourceSaver 适配器
	workSaverAdapter := &workSaverAdapter{svc: app.WorkService}
	resourceSaverAdapter := &resourceSaverAdapter{svc: app.ResourceService}

	app.TaskManagerService = taskManager.NewManager(
		app.SettingsService.GetSettings().ImportSettings.MaxParallelImport,
		app.taskRepo,
		taskManagerPusher,
		pluginExecFactory,
		workSaverAdapter,
		resourceSaverAdapter,
	)

	return nil
}

// WorkSaverAdapter WorkSaver 接口适配器
type workSaverAdapter struct {
	svc *work.Service
}

func (a *workSaverAdapter) Save(ctx context.Context, work *entity2.Work) (int64, error) {
	if err := a.svc.Save(ctx, work); err != nil {
		return 0, err
	}
	return work.GetID(), nil
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
	logger.Log.Info("Window DOM is ready")
}

// onBeforeClose 窗口关闭前的回调（内部使用，不暴露给前端）
// 返回 true 表示阻止关闭，false 表示允许关闭
func (app *App) onBeforeClose() bool {
	// 检查任务队列是否空闲
	if !app.TaskManagerService.IsIdle() {
		logger.Log.Info("Tasks are running, window close cancelled")
		// TODO: 显示确认对话框让用户选择是否强制关闭
		// 在 Wails v3 中，可以通过 dialog.MessageBox 或前端对话框实现
		return true // 阻止关闭，等待任务完成
	}
	logger.Log.Info("Window closing, all tasks idle")
	return false // 允许关闭
}
