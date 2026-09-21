package extension

// siteAuthorFetcher 能力广播路由锚定：插件侧拒绝（PermissionDenied，开流或首个 Recv 到达均覆盖）
// 与其余错误同为真失败（命中即止并点名插件）、无候选时收口报错、消费侧免字节时取消流正常收尾。

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/route"
	"github.com/lvfeng-z/library-squirrel-sdk/gen"
	transport "github.com/lvfeng-z/library-squirrel-sdk/transport"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeAuthorInfoStream 顺序吐出预置块的作者信息流替身；块耗尽后按 err 收尾（nil 为 io.EOF）
type fakeAuthorInfoStream struct {
	grpc.ClientStream
	chunks []*gen.AuthorInfoChunk
	next   int
	err    error
}

func (s *fakeAuthorInfoStream) Recv() (*gen.AuthorInfoChunk, error) {
	if s.next >= len(s.chunks) {
		if s.err != nil {
			return nil, s.err
		}
		return nil, io.EOF
	}
	c := s.chunks[s.next]
	s.next++
	return c, nil
}

// fakeSiteAuthorFetchClient 作者信息拉取服务客户端替身：openErr 非 nil 时开流即失败（错误
// 可出现在开流），否则返回预置流（错误亦可出现在首个 Recv——经 stream.err 表达）。
// opened 记录开流是否发生——「零插件调用」的机检锚点
type fakeSiteAuthorFetchClient struct {
	gen.SiteAuthorFetchServiceClient
	openErr error
	stream  *fakeAuthorInfoStream
	opened  bool
}

func (c *fakeSiteAuthorFetchClient) FetchSiteAuthorInfo(context.Context, *gen.FetchSiteAuthorInfoRequest, ...grpc.CallOption) (grpc.ServerStreamingClient[gen.AuthorInfoChunk], error) {
	c.opened = true
	if c.openErr != nil {
		return nil, c.openErr
	}
	return c.stream, nil
}

// authorInfoAccessor 多插件服务访问器替身
type authorInfoAccessor struct {
	byPlugin map[string]*transport.GRPCPluginClient
}

func (a *authorInfoAccessor) GetServices(pluginId string) (*transport.GRPCPluginClient, bool) {
	c, ok := a.byPlugin[pluginId]
	return c, ok
}

// authorInfoCapsQuery 声明驱动查询器替身：能力集由归属站点声明派生——sites 有该插件条目即视作
// 声明了 siteAuthorFetch 能力包（与生产 Loader 的能力集/站点清单两查询同源，同一结构段派生）
type authorInfoCapsQuery struct {
	sites map[string][]string
}

func (q *authorInfoCapsQuery) GetCapabilities(pluginId string) []string {
	if _, ok := q.sites[pluginId]; !ok {
		return nil
	}
	return []string{CapabilitySiteAuthorFetch}
}

// authorInfoScopeQuery 站点归属声明查询器替身：sites 按公开 ID 给出能力包声明的站点键清单
type authorInfoScopeQuery struct {
	sites map[string][]string
}

func (q *authorInfoScopeQuery) SiteAuthorFetchSites(pluginId string) []string {
	return q.sites[pluginId]
}

// authorInfoLister 广播遍历清单替身：ids 给出激活序，names 按公开 ID 给出插件展示名
// （缺项/空串=该插件未设置展示名）
type authorInfoLister struct {
	ids   []string
	names map[string]string
}

func (l *authorInfoLister) ListActivePlugins() []ActivePlugin {
	plugins := make([]ActivePlugin, 0, len(l.ids))
	for _, id := range l.ids {
		plugins = append(plugins, ActivePlugin{PublicID: id, Name: l.names[id]})
	}
	return plugins
}

func newBroadcastFetcher(ids []string, sites map[string][]string, byPlugin map[string]*transport.GRPCPluginClient) *siteAuthorFetcher {
	return newBroadcastFetcherNamed(ids, nil, sites, byPlugin)
}

// newBroadcastFetcherNamed 同 newBroadcastFetcher，另指定各插件的展示名
func newBroadcastFetcherNamed(ids []string, names map[string]string, sites map[string][]string, byPlugin map[string]*transport.GRPCPluginClient) *siteAuthorFetcher {
	return NewSiteAuthorFetcher(&authorInfoLister{ids: ids, names: names},
		&authorInfoCapsQuery{sites: sites}, &authorInfoScopeQuery{sites: sites}, &authorInfoAccessor{byPlugin: byPlugin})
}

// permissionDeniedErr 插件侧拒绝（gRPC PermissionDenied）：与任何插件错误一样按真失败处理
func permissionDeniedErr() error {
	return status.Error(codes.PermissionDenied, "site not owned")
}

func metaChunk(name string) *gen.AuthorInfoChunk {
	return &gen.AuthorInfoChunk{Payload: &gen.AuthorInfoChunk_Meta{Meta: &gen.AuthorInfoMeta{AuthorName: name}}}
}

func resourceChunk(data string) *gen.AuthorInfoChunk {
	return &gen.AuthorInfoChunk{Payload: &gen.AuthorInfoChunk_Resource{Resource: &gen.AuthorResourceData{Data: []byte(data)}}}
}

// TestSiteAuthorFetcherPluginErrorStopsBroadcast 插件侧拒绝（开流与首个 Recv 两种到达形态）按真
// 失败终止并点名插件，不再尝试后续候选
func TestSiteAuthorFetcherPluginErrorStopsBroadcast(t *testing.T) {
	cases := []struct {
		name   string
		client *fakeSiteAuthorFetchClient
	}{
		{"拒绝出现在开流", &fakeSiteAuthorFetchClient{openErr: permissionDeniedErr()}},
		{"拒绝出现在首个 Recv", &fakeSiteAuthorFetchClient{stream: &fakeAuthorInfoStream{err: permissionDeniedErr()}}},
	}
	for _, tc := range cases {
		fetcher := newBroadcastFetcher(
			[]string{"p-a", "p-b"},
			map[string][]string{"p-a": {"pixiv"}, "p-b": {"pixiv"}},
			map[string]*transport.GRPCPluginClient{
				"p-a": {SiteAuthorFetch: tc.client},
				"p-b": {SiteAuthorFetch: &fakeSiteAuthorFetchClient{stream: &fakeAuthorInfoStream{chunks: []*gen.AuthorInfoChunk{
					metaChunk("后续候选"),
				}}}},
			},
		)
		var gotMeta string
		err := fetcher.FetchSiteAuthorInfo(context.Background(), "pixiv", "42", "",
			func(meta *gen.AuthorInfoMeta) (bool, error) { gotMeta = meta.GetAuthorName(); return true, nil },
			func(data []byte) error { return nil })
		if err == nil || !strings.Contains(err.Error(), "p-a") {
			t.Fatalf("%s: 应真失败并点名 p-a, 实际 %v", tc.name, err)
		}
		if gotMeta != "" {
			t.Fatalf("%s: 失败后不应继续尝试后续候选, 实际交付 meta=%q", tc.name, gotMeta)
		}
	}
}

// TestSiteAuthorFetcherStreamDeliveredOnSuccess 首个候选的成功流：meta 与资源字节按序交付消费侧
func TestSiteAuthorFetcherStreamDeliveredOnSuccess(t *testing.T) {
	fetcher := newBroadcastFetcher(
		[]string{"p-a"},
		map[string][]string{"p-a": {"pixiv"}},
		map[string]*transport.GRPCPluginClient{
			"p-a": {SiteAuthorFetch: &fakeSiteAuthorFetchClient{stream: &fakeAuthorInfoStream{chunks: []*gen.AuthorInfoChunk{
				metaChunk("归属作者"),
				resourceChunk("bytes"),
			}}}},
		},
	)
	var gotMeta string
	var gotData []string
	err := fetcher.FetchSiteAuthorInfo(context.Background(), "pixiv", "42", "",
		func(meta *gen.AuthorInfoMeta) (bool, error) { gotMeta = meta.GetAuthorName(); return true, nil },
		func(data []byte) error { gotData = append(gotData, string(data)); return nil })
	if err != nil {
		t.Fatalf("成功流应无错误: %v", err)
	}
	if gotMeta != "归属作者" || len(gotData) != 1 || gotData[0] != "bytes" {
		t.Fatalf("交付不符: meta=%q data=%v", gotMeta, gotData)
	}
}

// TestSiteAuthorFetcherStopsAtTrueFailure 非归属错误（Internal）按真失败上抛并携带插件 ID，
// 不继续广播后续插件
func TestSiteAuthorFetcherStopsAtTrueFailure(t *testing.T) {
	fetcher := newBroadcastFetcher(
		[]string{"p-a", "p-b"},
		map[string][]string{"p-a": {"pixiv"}, "p-b": {"pixiv"}},
		map[string]*transport.GRPCPluginClient{
			"p-a": {SiteAuthorFetch: &fakeSiteAuthorFetchClient{openErr: status.Error(codes.Internal, "boom")}},
			"p-b": {SiteAuthorFetch: &fakeSiteAuthorFetchClient{stream: &fakeAuthorInfoStream{chunks: []*gen.AuthorInfoChunk{metaChunk("不应到达")}}}},
		},
	)
	err := fetcher.FetchSiteAuthorInfo(context.Background(), "pixiv", "42", "",
		func(meta *gen.AuthorInfoMeta) (bool, error) { return false, nil },
		func(data []byte) error { return nil })
	if err == nil {
		t.Fatal("真失败应上抛")
	}
	if !strings.Contains(err.Error(), "p-a") {
		t.Fatalf("错误应携带失败插件 ID: %v", err)
	}
}

// TestSiteAuthorFetcherNoOwner 无已激活插件声明可处理该站点的能力包时零候选收口报错，不盲调
func TestSiteAuthorFetcherNoOwner(t *testing.T) {
	fetcher := newBroadcastFetcher(
		[]string{"p-a", "p-b"},
		map[string][]string{"p-a": {"bilibili"}}, // 声明了能力包但未声明 pixiv；p-b 未声明能力包
		map[string]*transport.GRPCPluginClient{
			"p-a": {SiteAuthorFetch: &fakeSiteAuthorFetchClient{stream: &fakeAuthorInfoStream{}}},
		},
	)
	err := fetcher.FetchSiteAuthorInfo(context.Background(), "pixiv", "42", "",
		func(meta *gen.AuthorInfoMeta) (bool, error) { return false, nil },
		func(data []byte) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "无归属") {
		t.Fatalf("零候选应收口报错, 实际 %v", err)
	}
	if !strings.Contains(err.Error(), "无已激活插件声明") {
		t.Fatalf("零候选文案应点名无能力声明, 实际 %v", err)
	}
}

// TestSiteAuthorFetchUnclaimedSiteZeroCandidateNoCall 非归属站点（声明者未把该站点键列入能力包）：
// 零候选且零插件调用——「零插件调用」以客户端 open 标志机检（候选为空时基座不接入调用）
func TestSiteAuthorFetchUnclaimedSiteZeroCandidateNoCall(t *testing.T) {
	client := &fakeSiteAuthorFetchClient{stream: &fakeAuthorInfoStream{chunks: []*gen.AuthorInfoChunk{
		metaChunk("不应到达"),
	}}}
	fetcher := newBroadcastFetcher(
		[]string{"p-a", "p-b"},
		map[string][]string{"p-a": {"pixiv"}, "p-b": {"bilibili"}},
		map[string]*transport.GRPCPluginClient{
			"p-a": {SiteAuthorFetch: client},
			"p-b": {SiteAuthorFetch: client},
		},
	)
	err := fetcher.FetchSiteAuthorInfo(context.Background(), "local", "42", "",
		func(meta *gen.AuthorInfoMeta) (bool, error) { return false, nil },
		func(data []byte) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "无归属") {
		t.Fatalf("非归属站点应零候选收口报错, 实际 %v", err)
	}
	if client.opened {
		t.Fatal("非归属站点不应调用任何插件")
	}
	candidates, err := fetcher.ListSiteAuthorFetchCandidates(context.Background(), "local")
	if err != nil {
		t.Fatalf("枚举候选失败: %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("非归属站点候选应为空, 实际 %+v", candidates)
	}
}

// TestListSiteAuthorFetchCandidatesSameSiteDeclarers 同站点两个声明者 → 两候选（按插件标识
// 字典序），非声明该站点的插件不进候选
func TestListSiteAuthorFetchCandidatesSameSiteDeclarers(t *testing.T) {
	fetcher := newBroadcastFetcher(
		[]string{"p-b", "p-a", "p-other"},
		map[string][]string{"p-a": {"pixiv"}, "p-b": {"pixiv"}, "p-other": {"bilibili"}},
		map[string]*transport.GRPCPluginClient{
			"p-a":     {SiteAuthorFetch: &fakeSiteAuthorFetchClient{stream: &fakeAuthorInfoStream{}}},
			"p-b":     {SiteAuthorFetch: &fakeSiteAuthorFetchClient{stream: &fakeAuthorInfoStream{}}},
			"p-other": {SiteAuthorFetch: &fakeSiteAuthorFetchClient{stream: &fakeAuthorInfoStream{}}},
		},
	)
	candidates, err := fetcher.ListSiteAuthorFetchCandidates(context.Background(), "pixiv")
	if err != nil {
		t.Fatalf("枚举候选失败: %v", err)
	}
	if len(candidates) != 2 || candidates[0].PluginPublicId != "p-a" || candidates[1].PluginPublicId != "p-b" {
		t.Fatalf("同站点两声明者应成两候选且按标识字典序, 实际 %+v", candidates)
	}
}

// TestSiteAuthorFetcherMetaOnlyEndsCleanly 消费侧不需要头像字节（onMeta 返回 false）：取消流
// 正常收尾（无错误、字节块不交付）
func TestSiteAuthorFetcherMetaOnlyEndsCleanly(t *testing.T) {
	fetcher := newBroadcastFetcher(
		[]string{"p-a"},
		map[string][]string{"p-a": {"pixiv"}},
		map[string]*transport.GRPCPluginClient{
			"p-a": {SiteAuthorFetch: &fakeSiteAuthorFetchClient{stream: &fakeAuthorInfoStream{chunks: []*gen.AuthorInfoChunk{
				metaChunk("作者"),
				resourceChunk("不应交付"),
			}}}},
		},
	)
	dataSeen := false
	err := fetcher.FetchSiteAuthorInfo(context.Background(), "pixiv", "42", "",
		func(meta *gen.AuthorInfoMeta) (bool, error) { return false, nil },
		func(data []byte) error { dataSeen = true; return nil })
	if err != nil {
		t.Fatalf("免字节收尾应无错误: %v", err)
	}
	if dataSeen {
		t.Fatal("免字节收尾不应交付字节块")
	}
}

// TestSiteAuthorFetcherProtocolViolations 协议违规收口：资源块先于 meta、流终结缺 meta
func TestSiteAuthorFetcherProtocolViolations(t *testing.T) {
	cases := []struct {
		name   string
		chunks []*gen.AuthorInfoChunk
	}{
		{"资源块先于 meta", []*gen.AuthorInfoChunk{resourceChunk("x"), metaChunk("m")}},
		{"流终结缺 meta", nil},
	}
	for _, tc := range cases {
		fetcher := newBroadcastFetcher(
			[]string{"p-a"},
			map[string][]string{"p-a": {"pixiv"}},
			map[string]*transport.GRPCPluginClient{
				"p-a": {SiteAuthorFetch: &fakeSiteAuthorFetchClient{stream: &fakeAuthorInfoStream{chunks: tc.chunks}}},
			},
		)
		err := fetcher.FetchSiteAuthorInfo(context.Background(), "pixiv", "42", "",
			func(meta *gen.AuthorInfoMeta) (bool, error) { return true, nil },
			func(data []byte) error { return nil })
		if err == nil || !strings.Contains(err.Error(), "协议违规") {
			t.Fatalf("%s: 应报协议违规, 实际 %v", tc.name, err)
		}
	}
}

// TestSiteAuthorFetcherConsumerErrorPropagates 消费回调错误中止流并作为整体失败上抛
func TestSiteAuthorFetcherConsumerErrorPropagates(t *testing.T) {
	fetcher := newBroadcastFetcher(
		[]string{"p-a"},
		map[string][]string{"p-a": {"pixiv"}},
		map[string]*transport.GRPCPluginClient{
			"p-a": {SiteAuthorFetch: &fakeSiteAuthorFetchClient{stream: &fakeAuthorInfoStream{chunks: []*gen.AuthorInfoChunk{metaChunk("作者")}}}},
		},
	)
	wantErr := errors.New("消费侧中止")
	err := fetcher.FetchSiteAuthorInfo(context.Background(), "pixiv", "42", "",
		func(meta *gen.AuthorInfoMeta) (bool, error) { return false, wantErr },
		func(data []byte) error { return nil })
	if !errors.Is(err, wantErr) {
		t.Fatalf("消费回调错误应原样上抛, 实际 %v", err)
	}
}

// TestSiteAuthorFetchAdapterPreferredCandidateGoesFirst 置首候选键命中候选集时该候选先于标识
// 字典序在前的候选被尝试：两候选均可成功时交付置首者，置首键为空时回落标识字典序首位
// （交互面显选注入的接缝）
func TestSiteAuthorFetchAdapterPreferredCandidateGoesFirst(t *testing.T) {
	pluginClient := func(authorName string) *transport.GRPCPluginClient {
		return &transport.GRPCPluginClient{SiteAuthorFetch: &fakeSiteAuthorFetchClient{
			stream: &fakeAuthorInfoStream{chunks: []*gen.AuthorInfoChunk{metaChunk(authorName)}},
		}}
	}
	cases := []struct {
		name              string
		preferredPluginId string
		wantMeta          string
	}{
		{"置首 p-b", "p-b", "p-b 作者"},
		{"无置首回落字典序首位", "", "p-a 作者"},
	}
	for _, tc := range cases {
		fetcher := newBroadcastFetcher(
			[]string{"p-a", "p-b"},
			map[string][]string{"p-a": {"pixiv"}, "p-b": {"pixiv"}},
			map[string]*transport.GRPCPluginClient{"p-a": pluginClient("p-a 作者"), "p-b": pluginClient("p-b 作者")},
		)
		var gotMeta string
		adapter := &siteAuthorFetchAdapter{
			fetcher:           fetcher,
			siteKey:           "pixiv",
			siteAuthorId:      "42",
			preferredPluginId: tc.preferredPluginId,
			onMeta:            func(meta *gen.AuthorInfoMeta) (bool, error) { gotMeta = meta.GetAuthorName(); return false, nil },
			onData:            func(data []byte) error { return nil },
		}
		if _, err := route.Route[string, struct{}](context.Background(), adapter); err != nil {
			t.Fatalf("%s: 应路由成功: %v", tc.name, err)
		}
		if gotMeta != tc.wantMeta {
			t.Fatalf("%s: 交付作者应为 %q, 实际 %q", tc.name, tc.wantMeta, gotMeta)
		}
	}
}

// TestSiteAuthorFetcherChosenCandidateGoesFirst 显选键经公开拉取入口贯通到候选排序：两候选均
// 可成功时交付显选者，未显选回落标识字典序首位
func TestSiteAuthorFetcherChosenCandidateGoesFirst(t *testing.T) {
	pluginClient := func(authorName string) *transport.GRPCPluginClient {
		return &transport.GRPCPluginClient{SiteAuthorFetch: &fakeSiteAuthorFetchClient{
			stream: &fakeAuthorInfoStream{chunks: []*gen.AuthorInfoChunk{metaChunk(authorName)}},
		}}
	}
	cases := []struct {
		name           string
		chosenPluginId string
		wantMeta       string
	}{
		{"显选 p-b", "p-b", "p-b 作者"},
		{"未显选回落字典序首位", "", "p-a 作者"},
	}
	for _, tc := range cases {
		fetcher := newBroadcastFetcher(
			[]string{"p-a", "p-b"},
			map[string][]string{"p-a": {"pixiv"}, "p-b": {"pixiv"}},
			map[string]*transport.GRPCPluginClient{"p-a": pluginClient("p-a 作者"), "p-b": pluginClient("p-b 作者")},
		)
		var gotMeta string
		err := fetcher.FetchSiteAuthorInfo(context.Background(), "pixiv", "42", tc.chosenPluginId,
			func(meta *gen.AuthorInfoMeta) (bool, error) { gotMeta = meta.GetAuthorName(); return false, nil },
			func(data []byte) error { return nil })
		if err != nil {
			t.Fatalf("%s: 应路由成功: %v", tc.name, err)
		}
		if gotMeta != tc.wantMeta {
			t.Fatalf("%s: 交付作者应为 %q, 实际 %q", tc.name, tc.wantMeta, gotMeta)
		}
	}
}

// TestListSiteAuthorFetchCandidatesFilteredOrdered 候选清单=已激活 ∩ 声明能力包 ∩ 声明站点
// 含请求站点键 ∩ 客户端可用，按插件标识字典序（与激活清单给出的序无关），元素为插件级候选
// （扩展点 ID 恒空串）
func TestListSiteAuthorFetchCandidatesFilteredOrdered(t *testing.T) {
	fetcher := newBroadcastFetcher(
		[]string{"p-c", "p-b", "p-a"},
		map[string][]string{"p-a": {"pixiv"}, "p-b": {"pixiv"}, "p-c": {"pixiv"}},
		map[string]*transport.GRPCPluginClient{
			"p-a": {SiteAuthorFetch: &fakeSiteAuthorFetchClient{stream: &fakeAuthorInfoStream{}}},
			"p-b": {SiteAuthorFetch: &fakeSiteAuthorFetchClient{stream: &fakeAuthorInfoStream{}}},
			// p-c 声明能力但无可用服务客户端，不成候选
		},
	)
	candidates, err := fetcher.ListSiteAuthorFetchCandidates(context.Background(), "pixiv")
	if err != nil {
		t.Fatalf("枚举候选失败: %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("候选应为声明能力且有客户端的 p-a/p-b, 实际 %+v", candidates)
	}
	if candidates[0].PluginPublicId != "p-a" || candidates[1].PluginPublicId != "p-b" {
		t.Fatalf("候选应按插件标识字典序, 实际 %+v", candidates)
	}
	if candidates[0].ExtensionId != "" || candidates[1].ExtensionId != "" {
		t.Fatalf("插件级消费面候选的扩展点 ID 应恒为空串, 实际 %+v", candidates)
	}
}

// TestListSiteAuthorFetchCandidatesPluginName 候选展示名取插件展示名，插件未设置展示名时
// 回落公开标识（两态均由展示面消费）
func TestListSiteAuthorFetchCandidatesPluginName(t *testing.T) {
	fetcher := newBroadcastFetcherNamed(
		[]string{"p-a", "p-b"},
		map[string]string{"p-a": "比卡套件"}, // p-b 未设置展示名
		map[string][]string{"p-a": {"pixiv"}, "p-b": {"pixiv"}},
		map[string]*transport.GRPCPluginClient{
			"p-a": {SiteAuthorFetch: &fakeSiteAuthorFetchClient{stream: &fakeAuthorInfoStream{}}},
			"p-b": {SiteAuthorFetch: &fakeSiteAuthorFetchClient{stream: &fakeAuthorInfoStream{}}},
		},
	)
	candidates, err := fetcher.ListSiteAuthorFetchCandidates(context.Background(), "pixiv")
	if err != nil {
		t.Fatalf("枚举候选失败: %v", err)
	}
	byId := make(map[string]*dto.PluginCandidate, len(candidates))
	for _, c := range candidates {
		byId[c.PluginPublicId] = c
	}
	if got := byId["p-a"]; got == nil || got.PluginName != "比卡套件" {
		t.Fatalf("候选展示名应取插件展示名, 实际 %+v", got)
	}
	if got := byId["p-b"]; got == nil || got.PluginName != "p-b" {
		t.Fatalf("插件未设置展示名时应回落公开标识, 实际 %+v", got)
	}
}
