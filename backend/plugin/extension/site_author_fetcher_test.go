package extension

// siteAuthorFetcher 能力广播路由锚定：未归属（PermissionDenied，开流或首个 Recv 到达均覆盖）
// 静默跳过继续广播、真失败命中即止、无能力/无归属收口报错、消费侧免字节时取消流正常收尾。

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

// fakeSiteAuthorFetchClient 作者信息拉取服务客户端替身：openErr 非 nil 时开流即失败（未归属
// 信号可出现在开流），否则返回预置流（未归属信号亦可出现在首个 Recv——经 stream.err 表达）
type fakeSiteAuthorFetchClient struct {
	gen.SiteAuthorFetchServiceClient
	openErr error
	stream  *fakeAuthorInfoStream
}

func (c *fakeSiteAuthorFetchClient) FetchSiteAuthorInfo(context.Context, *gen.FetchSiteAuthorInfoRequest, ...grpc.CallOption) (grpc.ServerStreamingClient[gen.AuthorInfoChunk], error) {
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

// authorInfoCapsQuery 声明驱动查询器替身
type authorInfoCapsQuery struct {
	caps map[string][]string
}

func (q *authorInfoCapsQuery) GetCapabilities(pluginId string) []string {
	return q.caps[pluginId]
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

func newBroadcastFetcher(ids []string, caps map[string][]string, byPlugin map[string]*transport.GRPCPluginClient) *siteAuthorFetcher {
	return newBroadcastFetcherNamed(ids, nil, caps, byPlugin)
}

// newBroadcastFetcherNamed 同 newBroadcastFetcher，另指定各插件的展示名
func newBroadcastFetcherNamed(ids []string, names map[string]string, caps map[string][]string, byPlugin map[string]*transport.GRPCPluginClient) *siteAuthorFetcher {
	return NewSiteAuthorFetcher(&authorInfoLister{ids: ids, names: names}, &authorInfoCapsQuery{caps: caps}, &authorInfoAccessor{byPlugin: byPlugin})
}

func notOwnedErr() error {
	return status.Error(codes.PermissionDenied, "site not owned")
}

func metaChunk(name string) *gen.AuthorInfoChunk {
	return &gen.AuthorInfoChunk{Payload: &gen.AuthorInfoChunk_Meta{Meta: &gen.AuthorInfoMeta{AuthorName: name}}}
}

func resourceChunk(data string) *gen.AuthorInfoChunk {
	return &gen.AuthorInfoChunk{Payload: &gen.AuthorInfoChunk_Resource{Resource: &gen.AuthorResourceData{Data: []byte(data)}}}
}

// TestSiteAuthorFetcherBroadcastSkipsNotOwned 未归属信号（开流与首个 Recv 两种到达形态）均
// 静默跳过继续广播，命中归属插件后正常交付
func TestSiteAuthorFetcherBroadcastSkipsNotOwned(t *testing.T) {
	// p-a：未归属出现在开流；p-b：未归属出现在首个 Recv（流首即错误）；p-c：归属，正常流
	fetcher := newBroadcastFetcher(
		[]string{"p-a", "p-b", "p-c"},
		map[string][]string{"p-a": {CapabilitySiteAuthorFetch}, "p-b": {CapabilitySiteAuthorFetch}, "p-c": {CapabilitySiteAuthorFetch}},
		map[string]*transport.GRPCPluginClient{
			"p-a": {SiteAuthorFetch: &fakeSiteAuthorFetchClient{openErr: notOwnedErr()}},
			"p-b": {SiteAuthorFetch: &fakeSiteAuthorFetchClient{stream: &fakeAuthorInfoStream{err: notOwnedErr()}}},
			"p-c": {SiteAuthorFetch: &fakeSiteAuthorFetchClient{stream: &fakeAuthorInfoStream{chunks: []*gen.AuthorInfoChunk{
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
		t.Fatalf("广播应命中 p-c 成功: %v", err)
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
		map[string][]string{"p-a": {CapabilitySiteAuthorFetch}, "p-b": {CapabilitySiteAuthorFetch}},
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

// TestSiteAuthorFetcherNoOwner 无已激活插件声明能力（或全部未归属）时收口报错，不盲调
func TestSiteAuthorFetcherNoOwner(t *testing.T) {
	fetcher := newBroadcastFetcher(
		[]string{"p-a", "p-b"},
		map[string][]string{"p-a": {"otherCapability"}},
		map[string]*transport.GRPCPluginClient{
			"p-a": {SiteAuthorFetch: &fakeSiteAuthorFetchClient{stream: &fakeAuthorInfoStream{}}},
		},
	)
	err := fetcher.FetchSiteAuthorInfo(context.Background(), "pixiv", "42", "",
		func(meta *gen.AuthorInfoMeta) (bool, error) { return false, nil },
		func(data []byte) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "无归属") {
		t.Fatalf("无归属应收口报错, 实际 %v", err)
	}
}

// TestSiteAuthorFetcherMetaOnlyEndsCleanly 消费侧不需要头像字节（onMeta 返回 false）：取消流
// 正常收尾（无错误、字节块不交付）
func TestSiteAuthorFetcherMetaOnlyEndsCleanly(t *testing.T) {
	fetcher := newBroadcastFetcher(
		[]string{"p-a"},
		map[string][]string{"p-a": {CapabilitySiteAuthorFetch}},
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
			map[string][]string{"p-a": {CapabilitySiteAuthorFetch}},
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
		map[string][]string{"p-a": {CapabilitySiteAuthorFetch}},
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

// TestSiteAuthorFetcherClosureStatesReportedSeparately 收口两态分报：零声明者（无插件声明能力）
// 与候选全不适配（候选均声明不归属）各自独立文案，后者列名已试候选
func TestSiteAuthorFetcherClosureStatesReportedSeparately(t *testing.T) {
	noDeclarer := newBroadcastFetcher(
		[]string{"p-a", "p-b"},
		map[string][]string{"p-a": {"otherCapability"}},
		map[string]*transport.GRPCPluginClient{
			"p-a": {SiteAuthorFetch: &fakeSiteAuthorFetchClient{stream: &fakeAuthorInfoStream{chunks: []*gen.AuthorInfoChunk{metaChunk("不应到达")}}}},
		},
	)
	allNotOwned := newBroadcastFetcher(
		[]string{"p-a", "p-b"},
		map[string][]string{"p-a": {CapabilitySiteAuthorFetch}, "p-b": {CapabilitySiteAuthorFetch}},
		map[string]*transport.GRPCPluginClient{
			"p-a": {SiteAuthorFetch: &fakeSiteAuthorFetchClient{openErr: notOwnedErr()}},
			"p-b": {SiteAuthorFetch: &fakeSiteAuthorFetchClient{stream: &fakeAuthorInfoStream{err: notOwnedErr()}}},
		},
	)
	noDeclarerErr := noDeclarer.FetchSiteAuthorInfo(context.Background(), "pixiv", "42", "",
		func(meta *gen.AuthorInfoMeta) (bool, error) { return false, nil },
		func(data []byte) error { return nil })
	allNotOwnedErr := allNotOwned.FetchSiteAuthorInfo(context.Background(), "pixiv", "42", "",
		func(meta *gen.AuthorInfoMeta) (bool, error) { return false, nil },
		func(data []byte) error { return nil })
	if noDeclarerErr == nil || allNotOwnedErr == nil {
		t.Fatalf("两态均应报错: 零声明者=%v 全不适配=%v", noDeclarerErr, allNotOwnedErr)
	}
	if noDeclarerErr.Error() == allNotOwnedErr.Error() {
		t.Fatalf("两态应收口为不同文案, 实际同为 %v", noDeclarerErr)
	}
	if !strings.Contains(noDeclarerErr.Error(), "无已激活插件声明") {
		t.Fatalf("零声明者文案应点名无能力声明: %v", noDeclarerErr)
	}
	if strings.Contains(allNotOwnedErr.Error(), "无已激活插件声明") {
		t.Fatalf("全不适配文案不应复用零声明者文案: %v", allNotOwnedErr)
	}
	if !strings.Contains(allNotOwnedErr.Error(), "p-a") || !strings.Contains(allNotOwnedErr.Error(), "p-b") {
		t.Fatalf("全不适配文案应列出已试候选: %v", allNotOwnedErr)
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
			map[string][]string{"p-a": {CapabilitySiteAuthorFetch}, "p-b": {CapabilitySiteAuthorFetch}},
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
			map[string][]string{"p-a": {CapabilitySiteAuthorFetch}, "p-b": {CapabilitySiteAuthorFetch}},
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

// TestListSiteAuthorFetchCandidatesFilteredOrdered 候选清单=已激活 ∩ 声明能力 ∩ 客户端可用，
// 按插件标识字典序（与激活清单给出的序无关），元素为插件级候选（扩展点 ID 恒空串）
func TestListSiteAuthorFetchCandidatesFilteredOrdered(t *testing.T) {
	fetcher := newBroadcastFetcher(
		[]string{"p-c", "p-b", "p-a"},
		map[string][]string{"p-a": {CapabilitySiteAuthorFetch}, "p-b": {CapabilitySiteAuthorFetch}, "p-c": {CapabilitySiteAuthorFetch}},
		map[string]*transport.GRPCPluginClient{
			"p-a": {SiteAuthorFetch: &fakeSiteAuthorFetchClient{stream: &fakeAuthorInfoStream{}}},
			"p-b": {SiteAuthorFetch: &fakeSiteAuthorFetchClient{stream: &fakeAuthorInfoStream{}}},
			// p-c 声明能力但无可用服务客户端，不成候选
		},
	)
	candidates, err := fetcher.ListSiteAuthorFetchCandidates(context.Background())
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
		map[string][]string{"p-a": {CapabilitySiteAuthorFetch}, "p-b": {CapabilitySiteAuthorFetch}},
		map[string]*transport.GRPCPluginClient{
			"p-a": {SiteAuthorFetch: &fakeSiteAuthorFetchClient{stream: &fakeAuthorInfoStream{}}},
			"p-b": {SiteAuthorFetch: &fakeSiteAuthorFetchClient{stream: &fakeAuthorInfoStream{}}},
		},
	)
	candidates, err := fetcher.ListSiteAuthorFetchCandidates(context.Background())
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
