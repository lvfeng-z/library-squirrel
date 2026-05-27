package extension

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/entity"
	pluginsdk "github.com/lvfeng-z/library-squirrel-plugin-sdk"
	"go.uber.org/zap"
)

// --- Provider Interfaces ---
// 由 extension 包定义，各 internal 服务包实现

// PluginDataProvider 插件数据持久化
type PluginDataProvider interface {
	GetByPublicId(ctx context.Context, publicId string) (*entity.Plugin, error)
	Update(ctx context.Context, plugin *entity.Plugin) error
}

// SecureStorageProvider 加密存储
type SecureStorageProvider interface {
	StoreAndGetKey(ctx context.Context, plainValue string, description string) (string, error)
	GetValueByKey(ctx context.Context, storageKey string) (string, error)
	Remove(ctx context.Context, storageKey string) (int64, error)
}

// WorkSetQueryProvider 作品集查询
type WorkSetQueryProvider interface {
	GetBySiteWorkSetIdAndSiteName(ctx context.Context, siteWorkSetId string, siteName string) (*entity.WorkSet, error)
}

// SiteSaveProvider 站点保存
type SiteSaveProvider interface {
	Save(ctx context.Context, site *entity.Site) error
}

// SiteQueryProvider 站点查询
type SiteQueryProvider interface {
	GetByName(ctx context.Context, siteName string) (*entity.Site, error)
}

// TaskCreateProvider 任务创建
type TaskCreateProvider interface {
	CreateTaskByURL(ctx context.Context, url string) (*pluginsdk.CreateTaskResult, error)
}

// UrlListenerRegistry URL监听器注册
type UrlListenerRegistry interface {
	RegisterUrlListener(pluginPublicId string, contributionId string, patterns []string)
	UnregisterUrlListener(pluginPublicId string)
}

// PluginContextDeps PluginContext 的依赖项
type PluginContextDeps struct {
	PluginInfo          *PluginInfo
	RootPath            string
	TaskHandlerRegistry *TaskHandlerRegistry
	SiteBrowserRegistry *SiteBrowserRegistry
	PluginData          PluginDataProvider
	SecureStorage       SecureStorageProvider
	WorkSetQuery        WorkSetQueryProvider
	SiteSave            SiteSaveProvider
	SiteQuery           SiteQueryProvider
	TaskCreate          TaskCreateProvider
	UrlListener         UrlListenerRegistry
	FrontendEvent       pluginsdk.FrontendEventProvider
}

// --- Implementation ---

type pluginContext struct {
	pluginInfo           *PluginInfo
	taskHandlerRegistry  *TaskHandlerRegistry
	siteBrowserRegistry  *SiteBrowserRegistry
	rootPath             string
	pluginData           PluginDataProvider
	secureStorage        SecureStorageProvider
	workSetQuery         WorkSetQueryProvider
	siteSave             SiteSaveProvider
	siteQuery            SiteQueryProvider
	taskCreate           TaskCreateProvider
	urlListener          UrlListenerRegistry
	frontendEvent        pluginsdk.FrontendEventProvider
	scopedLogger         *zap.SugaredLogger
	logger               pluginsdk.Logger
}

// NewPluginContext 创建插件上下文
func NewPluginContext(deps PluginContextDeps) pluginsdk.PluginContext {
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
		pluginData:          deps.PluginData,
		secureStorage:       deps.SecureStorage,
		workSetQuery:        deps.WorkSetQuery,
		siteSave:            deps.SiteSave,
		siteQuery:           deps.SiteQuery,
		taskCreate:          deps.TaskCreate,
		urlListener:         deps.UrlListener,
		frontendEvent:       deps.FrontendEvent,
		scopedLogger:        sugar,
		logger:              newHostLogger(sugar),
	}
}

// --- 扩展点注册 ---

func (pc *pluginContext) RegisterTaskHandler(id, name, description string, handler pluginsdk.TaskHandler) error {
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

func (pc *pluginContext) RegisterSiteBrowser(id, name, description string, browser pluginsdk.SiteBrowser) error {
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

// --- 插件数据持久化 ---

func (pc *pluginContext) GetPluginData() (string, error) {
	ctx := context.Background()
	p, err := pc.pluginData.GetByPublicId(ctx, pc.pluginInfo.PublicID)
	if err != nil {
		return "", fmt.Errorf("get plugin data: %w", err)
	}
	if p.PluginData.Valid {
		return p.PluginData.String, nil
	}
	return "", nil
}

func (pc *pluginContext) SetPluginData(data string) error {
	ctx := context.Background()
	p, err := pc.pluginData.GetByPublicId(ctx, pc.pluginInfo.PublicID)
	if err != nil {
		return fmt.Errorf("set plugin data: %w", err)
	}
	p.PluginData = sql.NullString{String: data, Valid: true}
	return pc.pluginData.Update(ctx, p)
}

// --- 加密存储 ---

func (pc *pluginContext) StoreEncryptedValue(plainValue string, description string) (string, error) {
	return pc.secureStorage.StoreAndGetKey(context.Background(), plainValue, description)
}

func (pc *pluginContext) GetDecryptedValue(storageKey string) (string, error) {
	return pc.secureStorage.GetValueByKey(context.Background(), storageKey)
}

func (pc *pluginContext) RemoveEncryptedValue(storageKey string) error {
	_, err := pc.secureStorage.Remove(context.Background(), storageKey)
	return err
}

// --- 业务查询 ---

func (pc *pluginContext) GetWorkSetBySiteWorkSetId(siteWorkSetId string, siteName string) (*pluginsdk.WorkSet, error) {
	ws, err := pc.workSetQuery.GetBySiteWorkSetIdAndSiteName(context.Background(), siteWorkSetId, siteName)
	if err != nil {
		return nil, err
	}
	return EntityWorkSetToSDK(ws), nil
}

func (pc *pluginContext) AddSite(sites []*pluginsdk.Site) error {
	ctx := context.Background()
	for _, site := range sites {
		e := SDKSiteToEntity(site)
		// 站点已存在则跳过
		if existing, _ := pc.siteQuery.GetByName(ctx, e.SiteName.String); existing != nil {
			continue
		}
		if err := pc.siteSave.Save(ctx, e); err != nil {
			return fmt.Errorf("add site: %w", err)
		}
	}
	return nil
}

// --- 任务 ---

func (pc *pluginContext) RegisterUrlListener(contributionId string, patterns []string) error {
	pc.urlListener.RegisterUrlListener(pc.pluginInfo.PublicID, contributionId, patterns)
	return nil
}

func (pc *pluginContext) UnregisterUrlListener() error {
	pc.urlListener.UnregisterUrlListener(pc.pluginInfo.PublicID)
	return nil
}

func (pc *pluginContext) CreateTask(url string) (*pluginsdk.CreateTaskResult, error) {
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

func (pc *pluginContext) GetLogger() pluginsdk.Logger {
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

func (l *hostLogger) Named(name string) pluginsdk.Logger {
	return newHostLogger(l.sugar.Named(name))
}
