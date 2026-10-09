package plugin

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/util"
	pluginsdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
	"github.com/lvfeng-z/library-squirrel-sdk/gen"
	pluginsdktransport "github.com/lvfeng-z/library-squirrel-sdk/transport"
	"google.golang.org/grpc"
)

// newSettingTestServiceWithDeps 设置服务测试装配：内存库真实插件仓储 + 可注入存储仓储、
// 求值触发器与通知器（与生产装配同构，见 app.go 的 NewRepository → NewPluginSettingService
// 链），根目录与 plantPluginWithManifestOnDisk 的落盘位置一致（应用根目录）
func newSettingTestServiceWithDeps(t *testing.T, storageRepo StorageRepository, evalTrigger ParticipationEvalTrigger, notifier SettingChangeNotifier) (*PluginSettingService, *Service) {
	t.Helper()
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	if err := db.AutoMigrate(&entity.Plugin{}); err != nil {
		t.Fatalf("迁移测试实体失败: %v", err)
	}
	repo := NewRepository(db)
	svc := NewService(repo, nil)
	return NewPluginSettingService(repo, NewPluginStorageService(storageRepo, newTestStorageCipher(t)), util.RootPath(), evalTrigger, notifier), svc
}

// newSettingTestService 设置服务测试装配（求值触发器与通知器均未装配）
func newSettingTestService(t *testing.T) (*PluginSettingService, *Service) {
	return newSettingTestServiceWithDeps(t, newMockStorageRepo(), nil, nil)
}

// TestGetSettingsReadsRootLevelDeclarations 根级 settings 段解析与设置服务读取：
// 声明逐字段从 plugin.json 根级可达，未存储项回落声明默认值，保存后存储值覆盖默认值
func TestGetSettingsReadsRootLevelDeclarations(t *testing.T) {
	settingSvc, svc := newSettingTestService(t)
	const publicId = "com.settings.root"
	manifest := `{"id":"` + publicId + `","name":"设置插件","version":"1.0.0",` +
		fmt.Sprintf(`"contractVersion":%d,`, pluginsdktransport.ContractVersion) +
		`"settings":[{"key":"apiToken","type":"string","title":"访问令牌","default":"anon","encrypted":true},` +
		`{"key":"pageSize","type":"integer","title":"分页大小","default":"20"}],` +
		`"extensions":{` + frontendExtensionsField + `}}`
	plantPluginWithManifestOnDisk(t, svc, publicId, manifest)

	ctx := context.Background()
	items, err := settingSvc.GetSettings(ctx, publicId)
	if err != nil {
		t.Fatalf("读取设置项失败: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("设置项条数 = %d, 期望 2（根级 settings 全量解析）", len(items))
	}
	if items[0].Key != "apiToken" || !items[0].Encrypted || items[0].Value != "anon" {
		t.Errorf("apiToken 项 = %+v, 期望 key=apiToken encrypted=true 默认值 anon", items[0])
	}
	if items[1].Key != "pageSize" || items[1].Value != "20" {
		t.Errorf("pageSize 项 = %+v, 期望 key=pageSize 默认值 20", items[1])
	}

	if err := settingSvc.SaveSetting(ctx, publicId, "pageSize", "50"); err != nil {
		t.Fatalf("保存设置项失败: %v", err)
	}
	items, err = settingSvc.GetSettings(ctx, publicId)
	if err != nil {
		t.Fatalf("回读设置项失败: %v", err)
	}
	if items[1].Value != "50" {
		t.Errorf("已保存设置项值 = %q, 期望 50（存储值覆盖声明默认值）", items[1].Value)
	}
}

// TestGetSettingsWithoutExtensionsSection 清单无 extensions 段时根级 settings 照常解析
// （用户设置项声明不依赖 extensions 能力包段在场）
func TestGetSettingsWithoutExtensionsSection(t *testing.T) {
	settingSvc, svc := newSettingTestService(t)
	const publicId = "com.settings.noext"
	manifest := `{"id":"` + publicId + `","name":"纯设置插件","version":"1.0.0",` +
		fmt.Sprintf(`"contractVersion":%d,`, pluginsdktransport.ContractVersion) +
		`"settings":[{"key":"locale","type":"string","title":"语言","default":"zh"}]}`
	plantPluginWithManifestOnDisk(t, svc, publicId, manifest)

	items, err := settingSvc.GetSettings(context.Background(), publicId)
	if err != nil {
		t.Fatalf("读取设置项失败: %v", err)
	}
	if len(items) != 1 || items[0].Key != "locale" || items[0].Value != "zh" {
		t.Fatalf("无 extensions 段清单的设置项 = %+v, 期望单条 locale=zh", items)
	}
}

// ===== 设置变更通知（SettingChangeNotifier 两落库点挂接）=====

// recordedNotifyCall 一次通知调用的实参快照
type recordedNotifyCall struct {
	publicId string
	source   string
	keys     []string
}

// recordingSettingNotifier 通知记录替身：记录每次 NotifyAsync 实参（服务在调用方
// goroutine 同步调用 NotifyAsync，异步边界在真实通知器内部——记录无并发）
type recordingSettingNotifier struct {
	calls []recordedNotifyCall
}

func (r *recordingSettingNotifier) NotifyAsync(pluginPublicId, source string, keys []string) {
	r.calls = append(r.calls, recordedNotifyCall{publicId: pluginPublicId, source: source, keys: keys})
}

// notifierAccessorStub 通知器测试的访问器替身：按 active 返回预置客户端（客户端与
// active 独立可控，用于锚定 ok=false 时跳过而非客户端判空），每次调用经 channel 回报
// publicId
type notifierAccessorStub struct {
	client    *pluginsdktransport.GRPCPluginClient
	active    bool
	accesseds chan string
}

func (a *notifierAccessorStub) GetServices(pluginPublicId string) (*pluginsdktransport.GRPCPluginClient, bool) {
	a.accesseds <- pluginPublicId
	return a.client, a.active
}

// recordingLifecycleClient 记录 SettingChanged 请求的生命周期客户端替身（err 非 nil
// 时原样返回，模拟发送失败）
type recordingLifecycleClient struct {
	gen.PluginLifecycleClient
	requests chan *pluginsdkdto.SettingChangedRequest
	err      error
}

func (c *recordingLifecycleClient) SettingChanged(_ context.Context, req *gen.SettingChangedRequest, _ ...grpc.CallOption) (*gen.Empty, error) {
	c.requests <- req
	if c.err != nil {
		return nil, c.err
	}
	return &gen.Empty{}, nil
}

// hangingLifecycleClient 无响应插件替身：SettingChanged 阻塞至调用 ctx 取消（invoked/
// returned 标记请求到达与放弃两个时点）
type hangingLifecycleClient struct {
	gen.PluginLifecycleClient
	invoked chan struct{}
	returned chan struct{}
}

func (c *hangingLifecycleClient) SettingChanged(ctx context.Context, _ *gen.SettingChangedRequest, _ ...grpc.CallOption) (*gen.Empty, error) {
	close(c.invoked)
	<-ctx.Done()
	close(c.returned)
	return nil, ctx.Err()
}

// failingStorageRepo 写路径恒失败的存储仓储替身（锚定落库失败路径；读路径复用内存桩）
type failingStorageRepo struct {
	*mockStorageRepo
	failErr error
}

func (f *failingStorageRepo) DeleteByKey(_ context.Context, _ int64, _ string) error {
	return f.failErr
}

func (f *failingStorageRepo) Create(_ context.Context, _ *entity.PluginStorage) error {
	return f.failErr
}

func (f *failingStorageRepo) Updates(_ context.Context, _ *entity.PluginStorage) error {
	return f.failErr
}

// notifyTestManifest 带单个明文设置项的测试清单（通知路径不感知加密路由，单键即可）
func notifyTestManifest(publicId string) string {
	return `{"id":"` + publicId + `","name":"通知插件","version":"1.0.0",` +
		fmt.Sprintf(`"contractVersion":%d,`, pluginsdktransport.ContractVersion) +
		`"settings":[{"key":"pageSize","type":"integer","title":"分页大小","default":"20"}]}`
}

// TestSaveResetSettingNotifyChange 落库成功后触发设置变更通知：保存（明文与加密两
// 分支）推送 source=save/单键，重置推送 source=reset/单键
func TestSaveResetSettingNotifyChange(t *testing.T) {
	notifier := &recordingSettingNotifier{}
	settingSvc, svc := newSettingTestServiceWithDeps(t, newMockStorageRepo(), nil, notifier)
	const publicId = "com.settings.notify"
	manifest := `{"id":"` + publicId + `","name":"通知插件","version":"1.0.0",` +
		fmt.Sprintf(`"contractVersion":%d,`, pluginsdktransport.ContractVersion) +
		`"settings":[{"key":"apiToken","type":"string","title":"访问令牌","default":"anon","encrypted":true},` +
		`{"key":"pageSize","type":"integer","title":"分页大小","default":"20"}]}`
	plantPluginWithManifestOnDisk(t, svc, publicId, manifest)

	ctx := context.Background()
	if err := settingSvc.SaveSetting(ctx, publicId, "pageSize", "50"); err != nil {
		t.Fatalf("保存明文设置失败: %v", err)
	}
	if err := settingSvc.SaveSetting(ctx, publicId, "apiToken", "secret"); err != nil {
		t.Fatalf("保存加密设置失败: %v", err)
	}
	if err := settingSvc.ResetSetting(ctx, publicId, "pageSize"); err != nil {
		t.Fatalf("重置设置失败: %v", err)
	}

	want := []recordedNotifyCall{
		{publicId: publicId, source: pluginsdkdto.SettingChangeSourceSave, keys: []string{"pageSize"}},
		{publicId: publicId, source: pluginsdkdto.SettingChangeSourceSave, keys: []string{"apiToken"}},
		{publicId: publicId, source: pluginsdkdto.SettingChangeSourceReset, keys: []string{"pageSize"}},
	}
	if len(notifier.calls) != len(want) {
		t.Fatalf("通知次数 = %d, 期望 %d, 实际记录 %+v", len(notifier.calls), len(want), notifier.calls)
	}
	for i, w := range want {
		got := notifier.calls[i]
		if got.publicId != w.publicId || got.source != w.source || !slices.Equal(got.keys, w.keys) {
			t.Errorf("第 %d 次通知 = %+v, 期望 %+v", i, got, w)
		}
	}
}

// TestSettingChangeNotifySkippedOnStorageFailure 落库失败不通知：存储写失败时保存/重置
// 返回错误且不触发任何通知
func TestSettingChangeNotifySkippedOnStorageFailure(t *testing.T) {
	notifier := &recordingSettingNotifier{}
	settingSvc, svc := newSettingTestServiceWithDeps(t,
		&failingStorageRepo{mockStorageRepo: newMockStorageRepo(), failErr: errors.New("落库失败")},
		nil, notifier)
	const publicId = "com.settings.notifyfail"
	plantPluginWithManifestOnDisk(t, svc, publicId, notifyTestManifest(publicId))

	ctx := context.Background()
	if err := settingSvc.SaveSetting(ctx, publicId, "pageSize", "50"); err == nil {
		t.Fatal("落库失败时保存应返回错误")
	}
	if err := settingSvc.ResetSetting(ctx, publicId, "pageSize"); err == nil {
		t.Fatal("落库失败时重置应返回错误")
	}
	if len(notifier.calls) != 0 {
		t.Fatalf("落库失败不应触发通知, 实际记录 %+v", notifier.calls)
	}
}

// TestSaveSettingWithoutNotifier 通知器未装配（nil）时保存/重置照常（空操作，与参与度
// 触发器未装配同纪律）
func TestSaveSettingWithoutNotifier(t *testing.T) {
	settingSvc, svc := newSettingTestService(t)
	const publicId = "com.settings.nonotify"
	plantPluginWithManifestOnDisk(t, svc, publicId, notifyTestManifest(publicId))

	ctx := context.Background()
	if err := settingSvc.SaveSetting(ctx, publicId, "pageSize", "50"); err != nil {
		t.Fatalf("未装配通知器时保存应照常: %v", err)
	}
	if err := settingSvc.ResetSetting(ctx, publicId, "pageSize"); err != nil {
		t.Fatalf("未装配通知器时重置应照常: %v", err)
	}
}

// TestSettingChangeNotifierDeliversToActivatedPlugin 已激活插件收到通知：publicId 经
// 访问器解析，请求透传 source 与 keys
func TestSettingChangeNotifierDeliversToActivatedPlugin(t *testing.T) {
	lifecycle := &recordingLifecycleClient{requests: make(chan *pluginsdkdto.SettingChangedRequest, 1)}
	accessor := &notifierAccessorStub{
		client:    &pluginsdktransport.GRPCPluginClient{Lifecycle: lifecycle},
		active:    true,
		accesseds: make(chan string, 1),
	}
	notifier := NewSettingChangeNotifier(accessor)

	notifier.NotifyAsync("com.x.active", pluginsdkdto.SettingChangeSourceReset, []string{"locale"})

	select {
	case id := <-accessor.accesseds:
		if id != "com.x.active" {
			t.Errorf("访问器收到的 publicId = %q, 期望 com.x.active", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("通知未触达访问器")
	}
	select {
	case req := <-lifecycle.requests:
		if req.Source != pluginsdkdto.SettingChangeSourceReset || !slices.Equal(req.Keys, []string{"locale"}) {
			t.Errorf("通知请求 = {source:%q keys:%v}, 期望 {source:reset keys:[locale]}", req.Source, req.Keys)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("通知未到达插件生命周期客户端")
	}
}

// TestSettingChangeNotifierSkipsInactivePlugin 未激活插件（GetServices 返回 false）跳过
// 通知：不触达生命周期客户端（客户端在场，锚定跳过判据是活性布尔而非客户端判空）
func TestSettingChangeNotifierSkipsInactivePlugin(t *testing.T) {
	lifecycle := &recordingLifecycleClient{requests: make(chan *pluginsdkdto.SettingChangedRequest, 1)}
	accessor := &notifierAccessorStub{
		client:    &pluginsdktransport.GRPCPluginClient{Lifecycle: lifecycle},
		active:    false,
		accesseds: make(chan string, 1),
	}
	notifier := NewSettingChangeNotifier(accessor)

	notifier.NotifyAsync("com.x.inactive", pluginsdkdto.SettingChangeSourceSave, []string{"pageSize"})

	select {
	case <-accessor.accesseds:
	case <-time.After(2 * time.Second):
		t.Fatal("通知未触达访问器")
	}
	select {
	case req := <-lifecycle.requests:
		t.Fatalf("未激活插件不应收到通知, 实际请求 %+v", req)
	default:
	}
}

// TestSettingChangeNotifyFailureDoesNotAffectSave 通知发送失败不影响保存：插件端
// SettingChanged 返回错误，保存仍成功
func TestSettingChangeNotifyFailureDoesNotAffectSave(t *testing.T) {
	lifecycle := &recordingLifecycleClient{
		requests: make(chan *pluginsdkdto.SettingChangedRequest, 1),
		err:      errors.New("插件端处理失败"),
	}
	accessor := &notifierAccessorStub{
		client:    &pluginsdktransport.GRPCPluginClient{Lifecycle: lifecycle},
		active:    true,
		accesseds: make(chan string, 1),
	}
	notifier := newSettingChangeNotifier(accessor, time.Second)
	settingSvc, svc := newSettingTestServiceWithDeps(t, newMockStorageRepo(), nil, notifier)
	const publicId = "com.settings.notifyerr"
	plantPluginWithManifestOnDisk(t, svc, publicId, notifyTestManifest(publicId))

	if err := settingSvc.SaveSetting(context.Background(), publicId, "pageSize", "50"); err != nil {
		t.Fatalf("通知发送失败不应影响保存: %v", err)
	}
	select {
	case <-lifecycle.requests:
	case <-time.After(2 * time.Second):
		t.Fatal("通知未到达插件生命周期客户端")
	}
}

// TestSettingChangeNotifyTimeoutDoesNotAffectSave 通知超时不影响保存：插件端无响应
// （阻塞至超时预算耗尽后放弃），保存先行返回成功
func TestSettingChangeNotifyTimeoutDoesNotAffectSave(t *testing.T) {
	lifecycle := &hangingLifecycleClient{invoked: make(chan struct{}), returned: make(chan struct{})}
	accessor := &notifierAccessorStub{
		client:    &pluginsdktransport.GRPCPluginClient{Lifecycle: lifecycle},
		active:    true,
		accesseds: make(chan string, 1),
	}
	notifier := newSettingChangeNotifier(accessor, 50*time.Millisecond)
	settingSvc, svc := newSettingTestServiceWithDeps(t, newMockStorageRepo(), nil, notifier)
	const publicId = "com.settings.notifytimeout"
	plantPluginWithManifestOnDisk(t, svc, publicId, notifyTestManifest(publicId))

	saved := make(chan error, 1)
	go func() {
		saved <- settingSvc.SaveSetting(context.Background(), publicId, "pageSize", "50")
	}()

	select {
	case <-lifecycle.invoked:
	case <-time.After(2 * time.Second):
		t.Fatal("通知未到达插件生命周期客户端")
	}
	select {
	case err := <-saved:
		if err != nil {
			t.Fatalf("通知超时不应影响保存: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("保存被通知发送阻塞：通知须异步尽力而为")
	}
	select {
	case <-lifecycle.returned:
	case <-time.After(2 * time.Second):
		t.Fatal("超时预算耗尽后通知未放弃")
	}
}
