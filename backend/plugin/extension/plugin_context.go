package extension

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model"
	pluginsdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

// TaskCreateProvider 任务创建
type TaskCreateProvider interface {
	CreateTaskByURL(ctx context.Context, url string) (*pluginsdkdto.CreateTaskResult, error)
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
	TaskCreate          TaskCreateProvider
	UrlListener         UrlListenerRegistry
	FrontendEvent       pluginsdkdto.FrontendEventProvider
	// LibraryQuery 库查询核心（Tier 1 只读查询；装配处构造一次全体插件共享）
	LibraryQuery *libraryQueryProvider
}

// --- Implementation ---

type pluginContext struct {
	pluginInfo          *PluginInfo
	taskHandlerRegistry *TaskHandlerRegistry
	siteBrowserRegistry *SiteBrowserRegistry
	rootPath            string
	storage             PluginStorageService
	taskCreate          TaskCreateProvider
	urlListener         UrlListenerRegistry
	frontendEvent       pluginsdkdto.FrontendEventProvider
	query               *libraryQueryProvider
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
		taskCreate:          deps.TaskCreate,
		urlListener:         deps.UrlListener,
		frontendEvent:       deps.FrontendEvent,
		query:               deps.LibraryQuery,
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

// --- 库查询（Tier 1 只读）---
// 逐方法记录调用方插件与端点（诊断级使用线索，随日志轮转留存），委托共享查询核心。
// 未注入查询核心时以 Unimplemented 语义码拒绝（对应宿主不注册 LibraryQuery 服务的形态）

// queryCore 取库查询核心，未注入时返回 Unimplemented
func (pc *pluginContext) queryCore() (*libraryQueryProvider, error) {
	if pc.query == nil {
		return nil, status.Error(codes.Unimplemented, "库查询能力未配置")
	}
	return pc.query, nil
}

func (pc *pluginContext) GetWorkById(workId int64) (*pluginsdkdto.WorkWithSite, error) {
	pc.scopedLogger.Infof("库查询 GetWorkById(workId=%d)", workId)
	core, err := pc.queryCore()
	if err != nil {
		return nil, err
	}
	return core.getWorkById(context.Background(), workId)
}

func (pc *pluginContext) GetWorkBySiteKey(siteKey string, siteWorkId string) (*pluginsdkdto.WorkWithSite, error) {
	pc.scopedLogger.Infof("库查询 GetWorkBySiteKey(siteKey=%s, siteWorkId=%s)", siteKey, siteWorkId)
	core, err := pc.queryCore()
	if err != nil {
		return nil, err
	}
	return core.getWorkBySiteKey(context.Background(), siteKey, siteWorkId)
}

func (pc *pluginContext) QueryWorks(req *pluginsdkdto.QueryWorksRequest) (*pluginsdkdto.QueryWorksResponse, error) {
	pc.scopedLogger.Infof("库查询 QueryWorks(siteKey=%s, page=%d, pageSize=%d)", req.SiteKey, req.GetPage().GetPage(), req.GetPage().GetPageSize())
	core, err := pc.queryCore()
	if err != nil {
		return nil, err
	}
	return core.queryWorks(context.Background(), req)
}

func (pc *pluginContext) ListResourcesByWorkId(workId int64) (*pluginsdkdto.ListResourcesByWorkIdResponse, error) {
	pc.scopedLogger.Infof("库查询 ListResourcesByWorkId(workId=%d)", workId)
	core, err := pc.queryCore()
	if err != nil {
		return nil, err
	}
	return core.listResourcesByWorkId(context.Background(), workId)
}

func (pc *pluginContext) GetLocalAuthorById(localAuthorId int64) (*pluginsdkdto.LocalAuthorDTO, error) {
	pc.scopedLogger.Infof("库查询 GetLocalAuthorById(localAuthorId=%d)", localAuthorId)
	core, err := pc.queryCore()
	if err != nil {
		return nil, err
	}
	return core.getLocalAuthorById(context.Background(), localAuthorId)
}

func (pc *pluginContext) QueryLocalAuthors(req *pluginsdkdto.QueryLocalAuthorsRequest) (*pluginsdkdto.QueryLocalAuthorsResponse, error) {
	pc.scopedLogger.Infof("库查询 QueryLocalAuthors(page=%d, pageSize=%d)", req.GetPage().GetPage(), req.GetPage().GetPageSize())
	core, err := pc.queryCore()
	if err != nil {
		return nil, err
	}
	return core.queryLocalAuthors(context.Background(), req)
}

func (pc *pluginContext) GetSiteAuthorBySiteKey(siteKey string, siteAuthorId string) (*pluginsdkdto.SiteAuthorInfo, error) {
	pc.scopedLogger.Infof("库查询 GetSiteAuthorBySiteKey(siteKey=%s, siteAuthorId=%s)", siteKey, siteAuthorId)
	core, err := pc.queryCore()
	if err != nil {
		return nil, err
	}
	return core.getSiteAuthorBySiteKey(context.Background(), siteKey, siteAuthorId)
}

func (pc *pluginContext) QuerySiteAuthors(req *pluginsdkdto.QuerySiteAuthorsRequest) (*pluginsdkdto.QuerySiteAuthorsResponse, error) {
	pc.scopedLogger.Infof("库查询 QuerySiteAuthors(siteKey=%s, page=%d, pageSize=%d)", req.SiteKey, req.GetPage().GetPage(), req.GetPage().GetPageSize())
	core, err := pc.queryCore()
	if err != nil {
		return nil, err
	}
	return core.querySiteAuthors(context.Background(), req)
}

func (pc *pluginContext) ListAuthorsByWorkId(workId int64) (*pluginsdkdto.ListAuthorsByWorkIdResponse, error) {
	pc.scopedLogger.Infof("库查询 ListAuthorsByWorkId(workId=%d)", workId)
	core, err := pc.queryCore()
	if err != nil {
		return nil, err
	}
	return core.listAuthorsByWorkId(context.Background(), workId)
}

func (pc *pluginContext) GetLocalTagById(localTagId int64) (*pluginsdkdto.LocalTagDTO, error) {
	pc.scopedLogger.Infof("库查询 GetLocalTagById(localTagId=%d)", localTagId)
	core, err := pc.queryCore()
	if err != nil {
		return nil, err
	}
	return core.getLocalTagById(context.Background(), localTagId)
}

func (pc *pluginContext) QueryLocalTags(req *pluginsdkdto.QueryLocalTagsRequest) (*pluginsdkdto.QueryLocalTagsResponse, error) {
	pc.scopedLogger.Infof("库查询 QueryLocalTags(page=%d, pageSize=%d)", req.GetPage().GetPage(), req.GetPage().GetPageSize())
	core, err := pc.queryCore()
	if err != nil {
		return nil, err
	}
	return core.queryLocalTags(context.Background(), req)
}

func (pc *pluginContext) GetSiteTagBySiteKey(siteKey string, siteTagId string) (*pluginsdkdto.SiteTagInfo, error) {
	pc.scopedLogger.Infof("库查询 GetSiteTagBySiteKey(siteKey=%s, siteTagId=%s)", siteKey, siteTagId)
	core, err := pc.queryCore()
	if err != nil {
		return nil, err
	}
	return core.getSiteTagBySiteKey(context.Background(), siteKey, siteTagId)
}

func (pc *pluginContext) QuerySiteTags(req *pluginsdkdto.QuerySiteTagsRequest) (*pluginsdkdto.QuerySiteTagsResponse, error) {
	pc.scopedLogger.Infof("库查询 QuerySiteTags(siteKey=%s, page=%d, pageSize=%d)", req.SiteKey, req.GetPage().GetPage(), req.GetPage().GetPageSize())
	core, err := pc.queryCore()
	if err != nil {
		return nil, err
	}
	return core.querySiteTags(context.Background(), req)
}

func (pc *pluginContext) ListTagsByWorkId(workId int64) (*pluginsdkdto.ListTagsByWorkIdResponse, error) {
	pc.scopedLogger.Infof("库查询 ListTagsByWorkId(workId=%d)", workId)
	core, err := pc.queryCore()
	if err != nil {
		return nil, err
	}
	return core.listTagsByWorkId(context.Background(), workId)
}

func (pc *pluginContext) GetWorkSetById(workSetId int64) (*pluginsdkdto.WorkSetDTO, error) {
	pc.scopedLogger.Infof("库查询 GetWorkSetById(workSetId=%d)", workSetId)
	core, err := pc.queryCore()
	if err != nil {
		return nil, err
	}
	return core.getWorkSetById(context.Background(), workSetId)
}

func (pc *pluginContext) GetWorkSetBySiteKey(siteKey string, siteWorkSetId string) (*pluginsdkdto.WorkSetDTO, error) {
	pc.scopedLogger.Infof("库查询 GetWorkSetBySiteKey(siteKey=%s, siteWorkSetId=%s)", siteKey, siteWorkSetId)
	core, err := pc.queryCore()
	if err != nil {
		return nil, err
	}
	return core.getWorkSetBySiteKey(context.Background(), siteKey, siteWorkSetId)
}

func (pc *pluginContext) ListWorkSetsByWorkId(workId int64) (*pluginsdkdto.ListWorkSetsByWorkIdResponse, error) {
	pc.scopedLogger.Infof("库查询 ListWorkSetsByWorkId(workId=%d)", workId)
	core, err := pc.queryCore()
	if err != nil {
		return nil, err
	}
	return core.listWorkSetsByWorkId(context.Background(), workId)
}

func (pc *pluginContext) ListParentWorkSets(workSetId int64) (*pluginsdkdto.ListParentWorkSetsResponse, error) {
	pc.scopedLogger.Infof("库查询 ListParentWorkSets(workSetId=%d)", workSetId)
	core, err := pc.queryCore()
	if err != nil {
		return nil, err
	}
	return core.listParentWorkSets(context.Background(), workSetId)
}

func (pc *pluginContext) ListChildWorkSets(workSetId int64) (*pluginsdkdto.ListChildWorkSetsResponse, error) {
	pc.scopedLogger.Infof("库查询 ListChildWorkSets(workSetId=%d)", workSetId)
	core, err := pc.queryCore()
	if err != nil {
		return nil, err
	}
	return core.listChildWorkSets(context.Background(), workSetId)
}

func (pc *pluginContext) ListSites() (*pluginsdkdto.ListSitesResponse, error) {
	pc.scopedLogger.Infof("库查询 ListSites()")
	core, err := pc.queryCore()
	if err != nil {
		return nil, err
	}
	return core.listSites(context.Background())
}

func (pc *pluginContext) GetWorkDir() (*pluginsdkdto.GetWorkDirResponse, error) {
	pc.scopedLogger.Infof("库查询 GetWorkDir()")
	core, err := pc.queryCore()
	if err != nil {
		return nil, err
	}
	return core.getWorkDir(pc.pluginInfo.PublicID)
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
