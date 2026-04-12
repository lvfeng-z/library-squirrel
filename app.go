package main

import (
	"context"
	"path/filepath"

	"gorm.io/gorm"

	"github.com/library-squirrel/wails/internal/appLauncher"
	"github.com/library-squirrel/wails/internal/backup"
	"github.com/library-squirrel/wails/internal/config"
	"github.com/library-squirrel/wails/internal/database"
	"github.com/library-squirrel/wails/internal/extension"
	"github.com/library-squirrel/wails/internal/fileSysUtil"
	"github.com/library-squirrel/wails/internal/localAuthor"
	"github.com/library-squirrel/wails/internal/localTag"
	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/internal/plugin"
	"github.com/library-squirrel/wails/internal/pluginTaskUrlListener"
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
	"github.com/library-squirrel/wails/pkg/model"
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
	ReWorkTagService     *reWorkTag.Service
	ReWorkWorkSetRepo    reWorkWorkSet.Repository
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
	TaskHandlerRegistry *extension.TaskHandlerRegistry
	SiteBrowserRegistry *extension.SiteBrowserRegistry
	SlotRegistry        *extension.SlotRegistry
	SlotPusher          *extension.WailsSlotPusher

	// Wails 事件发射器
	eventEmitter extension.WailsEventEmitter

	// 任务URL监听器
	PluginTaskUrlListenerSvc *pluginTaskUrlListener.Service
}

// NewApp 创建Wails应用实例
func NewApp() (*App, error) {
	app := &App{}

	// 1. 初始化日志
	if err := logger.Init(); err != nil {
		return nil, err
	}
	defer logger.Sync()

	// 2. 加载配置
	cfg, err := config.Load("config.yaml")
	if err != nil {
		logger.Log.Errorf("Failed to load config: %v", err)
		return nil, err
	}
	app.cfg = cfg
	logger.Log.Infof("Config loaded")

	// 3. 初始化数据库
	dbPath := filepath.Join(util.RootPath(), "database/database.db")
	if err := database.Init(dbPath); err != nil {
		logger.Log.Errorf("Failed to init database: %v", err)
		return nil, err
	}
	app.db = database.GetDB()
	logger.Log.Infof("Database initialized: %s", dbPath)
	defer func() {
		if err := database.Close(); err != nil {
			logger.Log.Errorf("Failed to close database: %v", err)
		}
	}()

	// 4. 初始化扩展注册中心
	app.TaskHandlerRegistry = extension.NewTaskHandlerRegistry()
	app.SiteBrowserRegistry = extension.NewSiteBrowserRegistry()
	app.SlotRegistry = extension.NewSlotRegistry()
	// SlotPusher 会在 SetEventEmitter 中创建

	// 5. 初始化基础服务（按依赖顺序）
	app.initBaseServices()

	// 6. 初始化高级服务
	if err := app.initAdvancedServices(); err != nil {
		return nil, err
	}

	return app, nil
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

	// siteTag 服务
	siteTagRepo := siteTag.NewRepository(app.db)
	app.SiteTagService = siteTag.NewService(siteTagRepo, app.LocalTagService)

	// siteAuthor 服务
	siteAuthorRepo := siteAuthor.NewRepository(app.db)
	app.SiteAuthorService = siteAuthor.NewService(siteAuthorRepo)

	// site 服务
	siteRepo := site.NewRepository(app.db)
	app.SiteService = site.NewService(siteRepo)

	// resource 服务
	resourceRepo := resource.NewRepository(app.db)
	app.ResourceService = resource.NewService(resourceRepo)

	// reWorkWorkSet 仓储
	app.ReWorkWorkSetRepo = reWorkWorkSet.NewRepository(app.db)

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
		app.ReWorkWorkSetRepo,
		app.ResourceService,
	)

	// workSet 服务
	workSetRepo := workSet.NewRepository(app.db)
	app.WorkSetService = workSet.NewService(workSetRepo, app.ReWorkWorkSetRepo, app.WorkService)

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
	pluginLoader := plugin.NewLoader(app.TaskHandlerRegistry, app.SiteBrowserRegistry, app.SlotRegistry)
	pluginRepo := plugin.NewRepository(app.db)
	app.PluginService = plugin.NewService(pluginRepo, app.BackupService, pluginLoader)

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
		pluginLoader, // 实现 TaskHandlerProvider 接口
		app.PluginTaskUrlListenerSvc,
		app.SiteService,
	)

	// taskManager 服务
	taskManagerPusher := taskManager.NewSSEProgressPusher()
	pluginExecFactory := func(pluginPublicId string) (taskManager.TaskExecutor, error) {
		return plugin.NewTaskExecutor(pluginLoader), nil
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

func (a *workSaverAdapter) Save(ctx context.Context, work *domain.Work) (int64, error) {
	if err := a.svc.Save(ctx, work); err != nil {
		return 0, err
	}
	return work.GetID(), nil
}

// ResourceSaverAdapter ResourceSaver 接口适配器
type resourceSaverAdapter struct {
	svc *resource.Service
}

func (a *resourceSaverAdapter) Save(ctx context.Context, resource *domain.Resource) (int64, error) {
	if err := a.svc.Save(ctx, resource); err != nil {
		return 0, err
	}
	return resource.GetID(), nil
}

// Wails bindings - these methods will be exposed to the frontend

// Greet is a simple test method for Wails bindings
func (a *App) Greet(name string) string {
	return "Hello, " + name + "! Welcome to Library Squirrel."
}

// GetVersion returns the application version
func (a *App) GetVersion() string {
	return "1.0.0-wails"
}

// DirSelect opens a directory/file selection dialog
// openFile: true=select file, false=select directory
func (a *App) DirSelect(openFile bool) (*fileSysUtil.OpenDialogResult, error) {
	return a.FileSysUtilService.DirSelect(openFile, false)
}

// OpenPath opens a file with the system's default application
func (a *App) OpenPath(path string) error {
	return a.AppLauncherService.OpenPath(path)
}

// OpenExternal opens a URL in the default browser
func (a *App) OpenExternal(url string) error {
	return a.AppLauncherService.OpenExternal(url)
}

// onDomReady 窗口 DOM 准备就绪时的回调（内部使用，不暴露给前端）
func (a *App) onDomReady() {
	logger.Log.Info("Window DOM is ready")
}

// onBeforeClose 窗口关闭前的回调（内部使用，不暴露给前端）
// 返回 true 表示阻止关闭，false 表示允许关闭
func (a *App) onBeforeClose() bool {
	// 检查任务队列是否空闲
	if !a.TaskManagerService.IsIdle() {
		logger.Log.Info("Tasks are running, window close cancelled")
		// TODO: 显示确认对话框让用户选择是否强制关闭
		// 在 Wails v3 中，可以通过 dialog.MessageBox 或前端对话框实现
		return true // 阻止关闭，等待任务完成
	}
	logger.Log.Info("Window closing, all tasks idle")
	return false // 允许关闭
}

// ==================== LocalTag Wails Bindings ====================

// LocalTagSave 保存本地标签
func (a *App) LocalTagSave(tag *domain.LocalTag) (int64, error) {
	if err := a.LocalTagService.Save(context.Background(), tag); err != nil {
		return 0, err
	}
	return tag.ID, nil
}

// LocalTagDeleteById 删除本地标签
func (a *App) LocalTagDeleteById(id int64) error {
	return a.LocalTagService.Delete(context.Background(), id)
}

// LocalTagUpdateById 更新本地标签
func (a *App) LocalTagUpdateById(tag *domain.LocalTag) error {
	return a.LocalTagService.UpdateById(context.Background(), tag)
}

// LocalTagGetById 获取本地标签
func (a *App) LocalTagGetById(id int64) (*domain.LocalTag, error) {
	return a.LocalTagService.GetById(context.Background(), id)
}

// LocalTagQueryPage 分页查询本地标签
func (a *App) LocalTagQueryPage(query *localTag.LocalTagQueryDTO) (*model.Page[domain.LocalTag], error) {
	return a.LocalTagService.PageByDTO(context.Background(), 1, 10, *query)
}

// LocalTagQueryDTOPage DTO分页查询本地标签
func (a *App) LocalTagQueryDTOPage(query *localTag.LocalTagQueryDTO) (*model.Page[domain.LocalTag], error) {
	return a.LocalTagService.PageByDTO(context.Background(), 1, 10, *query)
}

// LocalTagGetTree 获取标签树形结构
func (a *App) LocalTagGetTree(rootId int64, depth int) ([]*domain.LocalTag, error) {
	return a.LocalTagService.GetTree(context.Background(), rootId, depth)
}

// LocalTagListSelectItems 查询选择项列表
func (a *App) LocalTagListSelectItems(query *localTag.LocalTagQueryDTO) ([]*domain.SelectItem, error) {
	return a.LocalTagService.ListSelectItemsByDTO(context.Background(), *query)
}

// LocalTagQuerySelectItemPage 分页查询选择项
func (a *App) LocalTagQuerySelectItemPage(query *localTag.LocalTagQueryDTO) (*model.Page[domain.SelectItem], error) {
	return a.LocalTagService.QuerySelectItemPageByDTO(context.Background(), 1, 10, *query, "")
}

// LocalTagListByWorkId 查询作品关联的标签
func (a *App) LocalTagListByWorkId(workId int64) ([]*domain.LocalTag, error) {
	return a.LocalTagService.ListByWorkId(context.Background(), workId)
}

// LocalTagQuerySelectItemPageByWorkId 根据作品ID分页查询选择项
func (a *App) LocalTagQuerySelectItemPageByWorkId(query *localTag.LocalTagQueryDTO, workId int64) (*model.Page[domain.SelectItem], error) {
	return a.LocalTagService.QuerySelectItemPageByWorkIdByDTO(context.Background(), 1, 10, *query, workId)
}

// ==================== Slot Wails Bindings ====================

// SetEventEmitter 设置 Wails 事件发射器（在应用启动后调用）
func (a *App) SetEventEmitter(emitter extension.WailsEventEmitter) {
	a.eventEmitter = emitter
	a.SlotPusher = extension.NewWailsSlotPusher(emitter)
	logger.Log.Info("Wails event emitter set for slot pusher")
}

// GetAllSlots 获取所有插槽配置
func (a *App) GetAllSlots() []*domain.SlotConfig {
	return a.SlotRegistry.GetSlotConfigs()
}

// ==================== Plugin Wails Bindings ====================

// GetPluginVueFile 读取插件的 Vue 文件内容
func (a *App) GetPluginVueFile(pluginPublicId string, filePath string) (string, error) {
	return a.PluginService.ReadVueFile(pluginPublicId, filePath)
}
