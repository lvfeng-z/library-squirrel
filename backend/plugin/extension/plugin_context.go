package extension

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/pluginpreference"
	"github.com/library-squirrel/backend/sysproxy"
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

// PluginPreferenceService 插件偏好服务（由 pluginpreference 包实现）：读（无记录返回
// nil 不报错）/ 整值覆写 / 列键三能力；无删除——「忘掉」是用户权利，删除仅经该模块
// ManagementService（宿主记忆管理面）
type PluginPreferenceService interface {
	Get(ctx context.Context, pluginID int64, prefKey string) (*pluginpreference.PreferenceValue, error)
	Set(ctx context.Context, pluginID int64, prefKey string, value *pluginpreference.PreferenceValue) error
	ListKeys(ctx context.Context, pluginID int64) ([]string, error)
}

// TaskCreateProvider 任务创建
type TaskCreateProvider interface {
	CreateTaskByURL(ctx context.Context, url string) (*pluginsdkdto.CreateTaskResult, error)
}

// PluginContextDeps PluginContext 的依赖项
type PluginContextDeps struct {
	PluginInfo    *PluginInfo
	RootPath      string
	Storage       PluginStorageService
	TaskCreate    TaskCreateProvider
	FrontendEvent pluginsdkdto.FrontendEventProvider
	// LibraryQuery 库查询核心（Tier 1 只读查询；装配处构造一次全体插件共享）
	LibraryQuery *libraryQueryProvider
	// Preference 插件偏好服务（偏好域读写列；未注入时插件偏好调用得 Unimplemented）
	Preference PluginPreferenceService
}

// --- Implementation ---

type pluginContext struct {
	pluginInfo    *PluginInfo
	rootPath      string
	storage       PluginStorageService
	taskCreate    TaskCreateProvider
	frontendEvent pluginsdkdto.FrontendEventProvider
	query         *libraryQueryProvider
	preference    PluginPreferenceService
	scopedLogger  *zap.SugaredLogger
	logger        pluginsdkdto.Logger
}

// 能力族满足性编译期断言（与 SDK 侧 transport.PluginContextClient 的断言块互为表里）：
// 宿主侧 pluginContext 实现全部能力族与复合接口——任一族增改方法而实现未跟进，编译期即失败
var (
	_ pluginsdkdto.ContextKVStore        = (*pluginContext)(nil)
	_ pluginsdkdto.ContextPreference     = (*pluginContext)(nil)
	_ pluginsdkdto.ContextProxy          = (*pluginContext)(nil)
	_ pluginsdkdto.ContextTaskTrigger    = (*pluginContext)(nil)
	_ pluginsdkdto.ContextFrontendEvents = (*pluginContext)(nil)
	_ pluginsdkdto.ContextEnvironment    = (*pluginContext)(nil)
	_ pluginsdkdto.ContextLibraryQuery   = (*pluginContext)(nil)
	_ pluginsdkdto.ContextLogging        = (*pluginContext)(nil)
	_ pluginsdkdto.PluginContext         = (*pluginContext)(nil)
)

// NewPluginContext 创建插件上下文
func NewPluginContext(deps PluginContextDeps) pluginsdkdto.PluginContext {
	pluginName := deps.PluginInfo.Name
	if pluginName == "" {
		pluginName = deps.PluginInfo.PublicID
	}

	sugar := logger.Log.Named("Plugin[" + pluginName + "]")

	return &pluginContext{
		pluginInfo:    deps.PluginInfo,
		rootPath:      deps.RootPath,
		storage:       deps.Storage,
		taskCreate:    deps.TaskCreate,
		frontendEvent: deps.FrontendEvent,
		query:         deps.LibraryQuery,
		preference:    deps.Preference,
		scopedLogger:  sugar,
		logger:        newHostLogger(sugar),
	}
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

// --- 用户决策偏好（偏好域）---
// 与统一 KV 配置面正交：问答沉淀的用户决策记忆（删掉后会被重新问），调用方插件身份
// 由 pluginInfo.ID 锚定（与 plugin_storage 同一先例），跨插件键天然隔离

// preferenceCore 偏好域依赖取用，未注入时返回 Unimplemented
func (pc *pluginContext) preferenceCore() (PluginPreferenceService, error) {
	if pc.preference == nil {
		return nil, status.Error(codes.Unimplemented, "偏好域能力未配置")
	}
	return pc.preference, nil
}

// GetPreference 读偏好：无记录返回 (nil,false,nil) 不报错——无记录是合法状态（用户
// 已删除或从未写入），调用方据此重新发起问答。出线方向：主仓信封负载（any）序列化
// 为线级 JSON 文本
func (pc *pluginContext) GetPreference(key string) (*pluginsdkdto.PreferenceValue, bool, error) {
	svc, err := pc.preferenceCore()
	if err != nil {
		return nil, false, err
	}
	v, err := svc.Get(context.Background(), pc.pluginInfo.ID, key)
	if err != nil {
		return nil, false, err
	}
	if v == nil {
		return nil, false, nil
	}
	data := ""
	if v.Data != nil {
		raw, err := json.Marshal(v.Data)
		if err != nil {
			return nil, false, fmt.Errorf("偏好值负载序列化失败: %w", err)
		}
		data = string(raw)
	}
	return &pluginsdkdto.PreferenceValue{
		SchemaVersion: int32(v.SchemaVersion),
		Title:         v.Title,
		Description:   v.Description,
		Data:          data,
	}, true, nil
}

// SetPreference 整值覆写。入线方向：线级 JSON 文本经 json.RawMessage 原样字节嵌入
// 主仓信封（不做解码再编码的往返），非法 JSON 在落库序列化时报错回传插件
func (pc *pluginContext) SetPreference(key string, value *pluginsdkdto.PreferenceValue) error {
	svc, err := pc.preferenceCore()
	if err != nil {
		return err
	}
	if value == nil {
		return pluginpreference.ErrNilPreferenceValue
	}
	var data any
	if value.Data != "" {
		data = json.RawMessage(value.Data)
	}
	return svc.Set(context.Background(), pc.pluginInfo.ID, key, &pluginpreference.PreferenceValue{
		SchemaVersion: int(value.SchemaVersion),
		Title:         value.Title,
		Description:   value.Description,
		Data:          data,
	})
}

// ListMyPreferences 列本插件全部偏好键（插件自身域）
func (pc *pluginContext) ListMyPreferences() ([]string, error) {
	svc, err := pc.preferenceCore()
	if err != nil {
		return nil, err
	}
	return svc.ListKeys(context.Background(), pc.pluginInfo.ID)
}

// --- 代理解析 ---

// ResolveProxy 宿主三级代理解析（显式 > 系统代理 > 环境变量），逐请求现查无缓存，
// 代理开关对下一请求即时生效；返回代理地址（空 = 直连）与来源标签。诊断日志只记
// 来源标签、Debug 级（每请求一条）：代理地址可含 userinfo（口令）、目标地址属插件
// 出网行为，均不落日志
func (pc *pluginContext) ResolveProxy(explicitURL, requestURL string) (string, string, error) {
	proxyURL, source := sysproxy.Resolve(explicitURL, requestURL)
	pc.scopedLogger.Debugf("代理解析 ResolveProxy(source=%s)", source)
	return proxyURL, source, nil
}

// --- 任务 ---

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
