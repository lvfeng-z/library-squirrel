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
	pluginsdkdto "github.com/lvfeng-z/library-squirrel-plugin-sdk/dto"
	"github.com/wailsapp/wails/v3/pkg/application"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/library-squirrel/backend/appLauncher"
	"github.com/library-squirrel/backend/assetserver"
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
	SiteBrowserService   *siteBrowser.Service

	// 任务仓储（用于TaskManager）
	taskRepo task.Repository

	// 扩展注册中心
	TaskHandlerRegistry *extension2.TaskHandlerRegistry
	SiteBrowserRegistry *extension2.SiteBrowserRegistry
	SlotRegistry        *extension2.SlotRegistry

	// 插件加载器
	pluginLoader *extension2.Loader

	// 主窗口原生句柄
	mainHWND uintptr

	// 静态资源服务
	StaticResourceService *extension2.StaticResourceService

	// HTTP 路由
	AssetRouter     *assetserver.Router
	HttpFileHandler *assetserver.ResourceHandler

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
	SecureStorageHandler         *secureStorage.Handler
	BackupHandler                *backup.Handler
	AppLauncherHandler           *appLauncher.Handler
	FileSysUtilHandler           *fileSysUtil.Handler
	PluginHandler                *plugin.Handler
	TaskHandler                  *task.Handler
	TaskManagerHandler           *taskManager.Handler
	SlotHandler                  *extension2.SlotHandler
	SiteBrowserHandler           *siteBrowser.Handler
	ReWorkAuthorHandler          *reWorkAuthor.Handler
	ReWorkTagHandler             *reWorkTag.Handler
	PluginTaskUrlListenerHandler *pluginTaskUrlListener.Handler
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

// SetEventEmitter 设置 Wails 事件发射器并创建 SlotPusher 和 TaskProgressPusher
func (app *App) SetEventEmitter(emitter extension2.WailsEventEmitter, onEvent func(topic string, callback func(data any)) func()) {
	pusher := extension2.NewWailsSlotPusher(emitter)
	app.SlotRegistry.SetPusher(pusher)
	app.taskProgressEmitter = emitter
	app.frontendEventOn = onEvent
	// Manager 在 initAdvancedServices 中已创建（此时 emitter 尚未就绪），需要补设 pusher
	if app.TaskManagerService != nil {
		app.TaskManagerService.SetPusher(taskManager.NewWailsTaskProgressPusher(emitter))
	}
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

	runtimeLoaded := 0
	pureUICount := 0

	for _, p := range plugins {
		// 跳过已卸载的插件
		if p.Uninstalled.Valid && p.Uninstalled.Bool {
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

	logger.Log.Infof("插件加载完成: %d 个运行时, %d 个纯 UI, 共 %d 个", runtimeLoaded, pureUICount, len(plugins))
}

// Activate 实现 PluginActivator 接口，激活单个插件
func (app *App) Activate(p *entity2.Plugin) error {
	return app.activatePlugin(p)
}

// activatePlugin 激活单个插件：读取 manifest、注册静态资源、注册 Slot、启动运行时子进程
func (app *App) activatePlugin(p *entity2.Plugin) error {
	// 跳过没有 PublicID 的插件
	if !p.PublicID.Valid || p.PublicID.String == "" {
		return fmt.Errorf("插件缺少 PublicID")
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
	version := ""
	if p.Version.Valid {
		version = p.Version.String
	}
	app.StaticResourceService.RegisterPlugin(publicId, pluginRootDir, allowedDirs, version)
	logger.Log.Infof("插件 %s: 静态资源已注册 (dirs=%v)", publicId, allowedDirs)

	// 声明式注册 Slot
	for _, slot := range ext.Slots {
		slotConfig := base.NewSlotConfig()
		slotConfig.Metadata.ID = slot.ID
		slotConfig.Metadata.PluginID = p.GetID()
		slotConfig.Metadata.PluginPublicID = publicId
		slotConfig.Metadata.Name = slot.Name
		slotConfig.Metadata.Description = slot.Description
		slotConfig.SlotType = base.SlotType(slot.SlotType)
		slotConfig.Order = slot.Order

		// 按 slotType 解析 content
		if err := parseSlotContent(slot, slotConfig, publicId, version); err != nil {
			logger.Log.Errorf("解析 Slot content 失败 %s/%s: %v", publicId, slot.ID, err)
			continue
		}

		extension := model.NewExtension(*slotConfig.Metadata, slotConfig)
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
		SiteQuery:           app.SiteService,
		TaskCreate:          &taskCreateAdapter{svc: app.TaskService},
		UrlListener:         &urlListenerAdapter{svc: app.PluginTaskUrlListenerSvc, pluginEntity: p},
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

// parseSlotContent 按 slotType 解析 content 字段并填充 SlotConfig
func parseSlotContent(slot dto.SlotDeclaration, cfg *base.SlotConfig, publicId, version string) error {
	if len(slot.Content) == 0 {
		return nil
	}

	switch cfg.SlotType {
	case base.SlotTypeEmbed:
		var c dto.EmbedSlotContent
		if err := json.Unmarshal(slot.Content, &c); err != nil {
			return fmt.Errorf("解析 embed content 失败: %w", err)
		}
		cfg.ContentType = base.ContentType(c.ContentType)
		cfg.Content = resolveSourceURLs(c.Source, c.ContentType, publicId, version)
		cfg.Position = c.Position
		cfg.ContributionId = c.ContributionId
		cfg.Props = c.Props

	case base.SlotTypePanel:
		var c dto.PanelSlotContent
		if err := json.Unmarshal(slot.Content, &c); err != nil {
			return fmt.Errorf("解析 panel content 失败: %w", err)
		}
		cfg.ContentType = base.ContentType(c.ContentType)
		cfg.Content = resolveSourceURLs(c.Source, c.ContentType, publicId, version)
		cfg.Position = c.Position
		cfg.Width = c.Width
		cfg.Height = c.Height
		cfg.Props = c.Props

	case base.SlotTypeView:
		var c dto.ViewSlotContent
		if err := json.Unmarshal(slot.Content, &c); err != nil {
			return fmt.Errorf("解析 view content 失败: %w", err)
		}
		cfg.ContentType = base.ContentType(c.ContentType)
		cfg.Content = resolveSourceURLs(c.Source, c.ContentType, publicId, version)
		cfg.Title = c.Title
		cfg.Props = c.Props

	case base.SlotTypeMenu:
		var c dto.MenuSlotContent
		if err := json.Unmarshal(slot.Content, &c); err != nil {
			return fmt.Errorf("解析 menu content 失败: %w", err)
		}
		cfg.ViewId = c.ViewId
		cfg.Children = convertSlotChildren(c.Children, cfg.Metadata.PluginID, publicId, version)
		if c.Icon != "" {
			cfg.Icon = resolveIconURL(c.Icon, publicId, version)
		}

	case base.SlotTypeSiteBrowserList:
		var c dto.SiteBrowserListSlotContent
		if err := json.Unmarshal(slot.Content, &c); err != nil {
			return fmt.Errorf("解析 siteBrowserList content 失败: %w", err)
		}
		cfg.ContributionId = c.ContributionId
		if c.Icon != "" {
			cfg.Icon = resolveIconURL(c.Icon, publicId, version)
		}
	}

	return nil
}

// convertSlotChildren 递归转换子插槽声明为 SlotConfig
func convertSlotChildren(children []dto.SlotDeclaration, pluginID int64, publicId, version string) []base.SlotConfig {
	if len(children) == 0 {
		return nil
	}
	result := make([]base.SlotConfig, len(children))
	for i, child := range children {
		result[i].Metadata = &model.ExtensionMetadata{
			Type:           model.ExtensionTypeSlot,
			ID:             child.ID,
			PluginID:       pluginID,
			PluginPublicID: publicId,
			Name:           child.Name,
			Description:    child.Description,
		}
		result[i].SlotType = base.SlotType(child.SlotType)
		result[i].Order = child.Order

		if err := parseSlotContent(child, &result[i], publicId, version); err != nil {
			logger.Log.Warnf("解析子 Slot content 失败 %s/%s: %v", publicId, child.ID, err)
		}
	}
	return result
}

// resolveSourceURLs 将组件源中的相对路径转换为完整 URL
func resolveSourceURLs(source json.RawMessage, contentType, publicId, version string) json.RawMessage {
	if len(source) == 0 || contentType == "code" {
		return source
	}

	var c map[string]string
	if err := json.Unmarshal(source, &c); err != nil {
		return source
	}

	prefix := "/plugin/" + publicId + "/" + version + "/"
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
func resolveIconURL(iconPath, publicId, version string) string {
	if iconPath == "" {
		return ""
	}
	return "/plugin/" + publicId + "/" + version + "/" + iconPath
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
	app.AppLauncherService = appLauncher.NewService(app.SettingsService)

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
		app.LocalTagService,
		app.SiteTagService,
		app.SiteService,
		app.LocalAuthorService,
		app.ReWorkAuthorService,
		app.ResourceService,
		app.ReWorkTagService,
	)

	// workSet 服务
	reWorkWorkSetRepo := reWorkWorkSet.NewRepository(app.db)
	app.WorkSetService = workSet.NewService(workSetRepo, reWorkWorkSetRepo, app.WorkService, app.WorkService)

	// search 服务
	searchRepo := search.NewRepository(app.db)
	app.SearchService = search.NewService(
		searchRepo,
		reWorkWorkSetRepo,
		app.WorkService,
		app.ResourceService,
		app.LocalTagService,
		app.SiteTagService,
		app.LocalAuthorService,
		app.SiteAuthorService,
	)


	// siteBrowser 服务
	app.SiteBrowserService = siteBrowser.NewService(app.SiteBrowserRegistry)

	// pluginTaskUrlListener 服务
	pluginTaskUrlListenerManager := pluginTaskUrlListener.NewManager()
	app.PluginTaskUrlListenerSvc = pluginTaskUrlListener.NewService(pluginTaskUrlListenerManager)

	// plugin 服务
	app.pluginLoader = extension2.NewLoader(app.TaskHandlerRegistry, app.SiteBrowserRegistry)
	pluginRepo := plugin.NewRepository(app.db)
	app.PluginService = plugin.NewService(pluginRepo, app.BackupService, app.SettingsService)
	app.PluginService.SetActivator(app)
	app.PluginService.SetOnUnload(func(pluginPublicId string) {
		app.pluginLoader.UnloadPlugin(pluginPublicId)
		app.StaticResourceService.UnregisterPlugin(pluginPublicId)
		app.SlotRegistry.UnregisterAll(pluginPublicId)
	})
	app.PluginService.SetRuntimeStatusProvider(&runtimeStatusAdapter{loader: app.pluginLoader})
	app.PluginService.SetExtensionListProvider(&extensionListProviderAdapter{
		taskHandlerRegistry: app.TaskHandlerRegistry,
		siteBrowserRegistry: app.SiteBrowserRegistry,
		slotRegistry:        app.SlotRegistry,
	})
	app.PluginService.SetUrlListenerProvider(&pluginUrlListenerAdapter{manager: pluginTaskUrlListenerManager})

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
	var taskManagerPusher taskManager.TaskProgressPusher
	if app.taskProgressEmitter != nil {
		taskManagerPusher = taskManager.NewWailsTaskProgressPusher(app.taskProgressEmitter)
	} else {
		taskManagerPusher = taskManager.NewNoopProgressPusher()
	}
	pluginExecFactory := func(pluginPublicId string) (taskManager.TaskExecutor, error) {
		return extension2.NewTaskExecutor(app.pluginLoader), nil
	}

	// 创建 ResourceSaver 适配器
	resourceSaverAdapter := &resourceSaverAdapter{svc: app.ResourceService}
	resourceFileBackuperAdapter := &resourceFileBackuperAdapter{svc: app.BackupService}

	app.TaskManagerService = taskManager.NewManager(
		app.SettingsService.GetSettings().ImportSettings.MaxParallelImport,
		app.SettingsService,
		app.SettingsService,
		app.taskRepo,
		taskManagerPusher,
		pluginExecFactory,
		app.WorkService, // 实现 WorkInfoSaver 接口
		resourceSaverAdapter,
		app.WorkService,             // 实现 WorkChecker 接口
		app.ResourceService,         // 实现 ResourceReader 接口
		resourceFileBackuperAdapter, // 实现 ResourceFileBackuper 接口
	)

	return nil
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

// resourceFileBackuperAdapter ResourceFileBackuper 接口适配器
type resourceFileBackuperAdapter struct {
	svc *backup.Service
}

func (a *resourceFileBackuperAdapter) BackupFile(ctx context.Context, sourceType int, sourceId int64, fileName string, sourcePath string, workDir string) error {
	_, err := a.svc.CreateBackup(ctx, sourceType, sourceId, fileName, sourcePath, workDir)
	return err
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
	app.SlotHandler = extension2.NewSlotHandler(app.SlotRegistry)
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
	taskHandlerRegistry *extension2.TaskHandlerRegistry
	siteBrowserRegistry *extension2.SiteBrowserRegistry
	slotRegistry        *extension2.SlotRegistry
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

func (a *extensionListProviderAdapter) GetSlotsByPlugin(pluginPublicId string) []plugin.SlotMeta {
	exts, _ := a.slotRegistry.GetByPlugin(pluginPublicId)
	result := make([]plugin.SlotMeta, 0, len(exts))
	for _, ext := range exts {
		cfg := ext.Instance
		result = append(result, plugin.SlotMeta{ID: cfg.Metadata.ID, Name: cfg.Metadata.Name, SlotType: string(cfg.SlotType)})
	}
	return result
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
	if app.pluginLoader == nil {
		return
	}

	// UnloadAll 返回已卸载的插件 ID 列表，并已注销 TaskHandler/SiteBrowser
	ids := app.pluginLoader.UnloadAll()

	// 逐个清理静态资源和 Slot（复用卸载时的 onUnload 逻辑）
	for _, id := range ids {
		app.StaticResourceService.UnregisterPlugin(id)
		app.SlotRegistry.UnregisterAll(id)
	}

	// 清理纯 UI 插件注册的 Slot（没有运行时进程的插件不在 Loader.processes 中）
	if app.SlotRegistry != nil {
		app.SlotRegistry.UnregisterAll("")
	}

	logger.Log.Infof("所有插件已关闭，共 %d 个运行时插件", len(ids))
}
