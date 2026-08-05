package extension

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/entity"
	pluginsdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
	"go.uber.org/zap"
)

// --- Provider Interfaces ---
// 由 extension 包定义，各 internal 服务包实现

// PluginStorageService 插件自存信息服务（由 plugin 包实现）
// 统一 KV 存储：明文项直接读写，加密项 Value 存密文
type PluginStorageService interface {
	GetValue(ctx context.Context, pluginID int64, key string) (*pluginsdkdto.StorageValue, error)
	SetValue(ctx context.Context, pluginID int64, key, value string, schemaVersion int64) error
	SetValueEncrypted(ctx context.Context, pluginID int64, key, value string, schemaVersion int64) error
	DeleteValue(ctx context.Context, pluginID int64, key string) error
	GetAllValues(ctx context.Context, pluginID int64) (map[string]*pluginsdkdto.StorageValue, error)
}

// WorkSetQueryProvider 作品集查询
type WorkSetQueryProvider interface {
	GetBySiteWorkSetIdAndSiteName(ctx context.Context, siteWorkSetId string, siteName string) (*entity.WorkSet, error)
}

// SiteSaveProvider 站点保存
type SiteSaveProvider interface {
	Create(ctx context.Context, site *entity.Site) error
}

// SiteQueryProvider 站点查询
type SiteQueryProvider interface {
	GetByName(ctx context.Context, siteName string) (*entity.Site, error)
}

// TaskCreateProvider 任务创建
type TaskCreateProvider interface {
	CreateTaskByURL(ctx context.Context, url string) (*pluginsdkdto.CreateTaskResult, error)
}

// StorePathQueryProvider 资源 store 路径查询:据 task+role+store_seq 查真实落盘路径(workDir 相对)。
// 供插件在资源路径可知后(如 document lazy 生成)按真实文件名引用兄弟文件。
type StorePathQueryProvider interface {
	GetStoreRelPath(ctx context.Context, taskId int64, role string, storeSeq int) (string, error)
}

// UrlListenerRegistry URL监听器注册
type UrlListenerRegistry interface {
	RegisterUrlListener(pluginPublicId string, extensionId string, patterns []string)
	UnregisterUrlListener(pluginPublicId string, extensionId string)
}

// PluginContextDeps PluginContext 的依赖项
type PluginContextDeps struct {
	PluginInfo          *PluginInfo
	RootPath            string
	TaskHandlerRegistry *TaskHandlerRegistry
	SiteBrowserRegistry *SiteBrowserRegistry
	Storage             PluginStorageService
	WorkSetQuery        WorkSetQueryProvider
	SiteSave            SiteSaveProvider
	SiteQuery           SiteQueryProvider
	TaskCreate          TaskCreateProvider
	UrlListener         UrlListenerRegistry
	StorePath           StorePathQueryProvider
	FrontendEvent       pluginsdkdto.FrontendEventProvider
}

// --- Implementation ---

type pluginContext struct {
	pluginInfo          *PluginInfo
	taskHandlerRegistry *TaskHandlerRegistry
	siteBrowserRegistry *SiteBrowserRegistry
	rootPath            string
	storage             PluginStorageService
	workSetQuery        WorkSetQueryProvider
	siteSave            SiteSaveProvider
	siteQuery           SiteQueryProvider
	taskCreate          TaskCreateProvider
	urlListener         UrlListenerRegistry
	storePath           StorePathQueryProvider
	frontendEvent       pluginsdkdto.FrontendEventProvider
	scopedLogger        *zap.SugaredLogger
	logger              pluginsdkdto.Logger
}

// NewPluginContext 创建插件上下文
func NewPluginContext(deps PluginContextDeps) pluginsdkdto.PluginContext {
	pluginName := deps.PluginInfo.Name
	if pluginName == "" {
		pluginName = deps.PluginInfo.PublicID
	}

	sugar := logger.Log.Named("Plugin[" + pluginName + "]")

	return &pluginContext{
		pluginInfo:          deps.PluginInfo,
		taskHandlerRegistry: deps.TaskHandlerRegistry,
		siteBrowserRegistry: deps.SiteBrowserRegistry,
		rootPath:            deps.RootPath,
		storage:             deps.Storage,
		workSetQuery:        deps.WorkSetQuery,
		siteSave:            deps.SiteSave,
		siteQuery:           deps.SiteQuery,
		taskCreate:          deps.TaskCreate,
		urlListener:         deps.UrlListener,
		storePath:           deps.StorePath,
		frontendEvent:       deps.FrontendEvent,
		scopedLogger:        sugar,
		logger:              newHostLogger(sugar),
	}
}

// --- 扩展点注册 ---

func (pc *pluginContext) RegisterTaskHandler(id, name, description string, handler pluginsdkdto.TaskHandler) error {
	metadata := model.ExtensionMetadata{
		Type:           model.ExtensionTypeTaskHandler,
		ID:             id,
		PluginID:       pc.pluginInfo.ID,
		PluginPublicID: pc.pluginInfo.PublicID,
		Name:           name,
		Description:    description,
	}
	return pc.taskHandlerRegistry.Register(model.NewExtension(metadata, handler))
}

func (pc *pluginContext) RegisterSiteBrowser(id, name, description string, browser pluginsdkdto.SiteBrowser) error {
	metadata := model.ExtensionMetadata{
		Type:           model.ExtensionTypeSiteBrowser,
		ID:             id,
		PluginID:       pc.pluginInfo.ID,
		PluginPublicID: pc.pluginInfo.PublicID,
		Name:           name,
		Description:    description,
	}
	return pc.siteBrowserRegistry.Register(model.NewExtension(metadata, browser))
}

// --- 扩展点注销 ---

func (pc *pluginContext) UnregisterSiteBrowser(id string) error {
	return pc.siteBrowserRegistry.Unregister(pc.pluginInfo.PublicID, id)
}

// --- 插件自存信息（统一 KV）---

func (pc *pluginContext) GetValue(key string) (*pluginsdkdto.StorageValue, error) {
	return pc.storage.GetValue(context.Background(), pc.pluginInfo.ID, key)
}

func (pc *pluginContext) SetValue(key string, value string) error {
	return pc.storage.SetValue(context.Background(), pc.pluginInfo.ID, key, value, pc.pluginInfo.ConfigSchemaVersion)
}

func (pc *pluginContext) SetValueEncrypted(key string, value string) error {
	return pc.storage.SetValueEncrypted(context.Background(), pc.pluginInfo.ID, key, value, pc.pluginInfo.ConfigSchemaVersion)
}

func (pc *pluginContext) DeleteValue(key string) error {
	return pc.storage.DeleteValue(context.Background(), pc.pluginInfo.ID, key)
}

func (pc *pluginContext) GetAllValues() (map[string]*pluginsdkdto.StorageValue, error) {
	return pc.storage.GetAllValues(context.Background(), pc.pluginInfo.ID)
}

// --- 业务查询 ---

func (pc *pluginContext) GetWorkSetBySiteWorkSetId(siteWorkSetId string, siteName string) (*pluginsdkdto.WorkSetDTO, error) {
	ws, err := pc.workSetQuery.GetBySiteWorkSetIdAndSiteName(context.Background(), siteWorkSetId, siteName)
	if err != nil {
		return nil, err
	}
	return EntityWorkSetToSDK(ws), nil
}

func (pc *pluginContext) AddSite(sites []*pluginsdkdto.SiteDTO) error {
	ctx := context.Background()
	for _, site := range sites {
		e := SDKSiteToEntity(site)
		// 站点已存在则跳过
		if existing, _ := pc.siteQuery.GetByName(ctx, e.SiteName.String); existing != nil {
			continue
		}
		if err := pc.siteSave.Create(ctx, e); err != nil {
			return fmt.Errorf("add site: %w", err)
		}
	}
	return nil
}

// --- 任务 ---

func (pc *pluginContext) RegisterUrlListener(extensionId string, patterns []string) error {
	pc.urlListener.RegisterUrlListener(pc.pluginInfo.PublicID, extensionId, patterns)
	return nil
}

func (pc *pluginContext) UnregisterUrlListener(extensionId string) error {
	pc.urlListener.UnregisterUrlListener(pc.pluginInfo.PublicID, extensionId)
	return nil
}

func (pc *pluginContext) CreateTask(url string) (*pluginsdkdto.CreateTaskResult, error) {
	return pc.taskCreate.CreateTaskByURL(context.Background(), url)
}

func (pc *pluginContext) PublishToFrontend(topic string, data []byte) error {
	if pc.frontendEvent == nil {
		return fmt.Errorf("frontend event provider not configured")
	}
	return pc.frontendEvent.PublishToFrontend(topic, data)
}

func (pc *pluginContext) SubscribeFrontend(topic string) (<-chan []byte, error) {
	if pc.frontendEvent == nil {
		return nil, fmt.Errorf("frontend event provider not configured")
	}
	ch := make(chan []byte, 16)
	cancel, err := pc.frontendEvent.SubscribeFrontend(topic, func(data []byte) {
		ch <- data
	})
	if err != nil {
		close(ch)
		return nil, err
	}
	_ = cancel
	return ch, nil
}

func (pc *pluginContext) UnsubscribeFrontend(topic string) error {
	if pc.frontendEvent == nil {
		return fmt.Errorf("frontend event provider not configured")
	}
	return pc.frontendEvent.UnsubscribeFrontend(topic)
}

// --- 路径 ---

func (pc *pluginContext) GetPluginRoot(isRelative bool) string {
	if isRelative {
		return pc.pluginInfo.RootPath
	}
	return filepath.Join(pc.rootPath, pc.pluginInfo.RootPath)
}

// GetStoreRelPath 查询当前任务资源中指定 store 的真实落盘路径(workDir 相对)。
// 插件 Start 时资源尚未创建(PendingResourceID 不可用),故用 taskId;主程序在 downloadLoop
// 查询时事务已提交,PendingResourceID 已就位(provider 据 taskId 反查任务记录定位资源)。
func (pc *pluginContext) GetStoreRelPath(taskId int64, role string, storeSeq int) (string, error) {
	return pc.storePath.GetStoreRelPath(context.Background(), taskId, role, storeSeq)
}

func (pc *pluginContext) GetMainWindowHandle() uintptr {
	return 0
}

// --- 日志 ---

func (pc *pluginContext) Infof(template string, args ...any) {
	pc.scopedLogger.Infof(template, args...)
}

func (pc *pluginContext) Debugf(template string, args ...any) {
	pc.scopedLogger.Debugf(template, args...)
}

func (pc *pluginContext) Warnf(template string, args ...any) {
	pc.scopedLogger.Warnf(template, args...)
}

func (pc *pluginContext) Errorf(template string, args ...any) {
	pc.scopedLogger.Errorf(template, args...)
}

func (pc *pluginContext) GetLogger() pluginsdkdto.Logger {
	return pc.logger
}

// ResolveLogger 根据 loggerName 返回对应的 zap logger
// loggerName 为空时返回默认 scopedLogger，非空时返回 Named 子 logger
func (pc *pluginContext) ResolveLogger(loggerName string) *zap.SugaredLogger {
	if loggerName == "" {
		return pc.scopedLogger
	}
	return pc.scopedLogger.Named(loggerName)
}

// hostLogger 主进程侧 Logger 实现，委托给 zap
type hostLogger struct {
	sugar *zap.SugaredLogger
}

func newHostLogger(sugar *zap.SugaredLogger) *hostLogger {
	return &hostLogger{sugar: sugar}
}

func (l *hostLogger) Debugf(template string, args ...any) { l.sugar.Debugf(template, args...) }
func (l *hostLogger) Infof(template string, args ...any)  { l.sugar.Infof(template, args...) }
func (l *hostLogger) Warnf(template string, args ...any)  { l.sugar.Warnf(template, args...) }
func (l *hostLogger) Errorf(template string, args ...any) { l.sugar.Errorf(template, args...) }

func (l *hostLogger) Named(name string) pluginsdkdto.Logger {
	return newHostLogger(l.sugar.Named(name))
}
