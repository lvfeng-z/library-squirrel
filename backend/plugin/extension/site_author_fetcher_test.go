package extension

// siteAuthorFetcher 能力广播路由锚定：候选粒度 = 插件 × siteAuthorFetch 条目对（全键 NUL 拼接
// 字典序、拉取请求携带候选条目 id）、插件侧拒绝（PermissionDenied，开流或首个 Recv 到达均覆盖）
// 与其余错误同为真失败（命中即止并点名候选）、无候选时收口报错、消费侧免字节时取消流正常收尾。

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
// opened 记录开流是否发生——「零插件调用」的机检锚点；requests 逐次记录开流请求——
// 拉取请求携带条目 id 的机检锚点
type fakeSiteAuthorFetchClient struct {
	gen.SiteAuthorFetchServiceClient
	openErr  error
	stream   *fakeAuthorInfoStream
	opened   bool
	requests []*gen.FetchSiteAuthorInfoRequest
}

func (c *fakeSiteAuthorFetchClient) FetchSiteAuthorInfo(_ context.Context, req *gen.FetchSiteAuthorInfoRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[gen.AuthorInfoChunk], error) {
	c.opened = true
	c.requests = append(c.requests, req)
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

// authorInfoEntriesQuery 条目声明查询器替身：entries 按公开 ID 给出该插件的 siteAuthorFetch
// 条目声明（与生产 Loader 的 SiteAuthorFetchEntries 同源同一结构段；无条目 = 未声明该能力包）
type authorInfoEntriesQuery struct {
	entries map[string][]dto.SiteAuthorFetchDeclaration
}

func (q *authorInfoEntriesQuery) SiteAuthorFetchEntries(pluginId string) []dto.SiteAuthorFetchDeclaration {
	return q.entries[pluginId]
}

// fetchEntry 构造单个 siteAuthorFetch 条目声明
func fetchEntry(id, name string, sites ...string) dto.SiteAuthorFetchDeclaration {
	return dto.SiteAuthorFetchDeclaration{ID: id, Name: name, Sites: sites}
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

func newBroadcastFetcher(ids []string, entries map[string][]dto.SiteAuthorFetchDeclaration, byPlugin map[string]*transport.GRPCPluginClient) *siteAuthorFetcher {
	return newBroadcastFetcherNamed(ids, nil, entries, byPlugin)
}

// newBroadcastFetcherNamed 同 newBroadcastFetcher，另指定各插件的展示名
func newBroadcastFetcherNamed(ids []string, names map[string]string, entries map[string][]dto.SiteAuthorFetchDeclaration, byPlugin map[string]*transport.GRPCPluginClient) *siteAuthorFetcher {
	return NewSiteAuthorFetcher(&authorInfoLister{ids: ids, names: names},
		&authorInfoEntriesQuery{entries: entries}, &authorInfoAccessor{byPlugin: byPlugin})
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
// 失败终止并点名候选（插件标识 + 条目 id），不再尝试后续候选
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
			map[string][]dto.SiteAuthorFetchDeclaration{
				"p-a": {fetchEntry("main", "A 源", "pixiv")},
				"p-b": {fetchEntry("main", "B 源", "pixiv")},
			},
			map[string]*transport.GRPCPluginClient{
				"p-a": {SiteAuthorFetch: tc.client},
				"p-b": {SiteAuthorFetch: &fakeSiteAuthorFetchClient{stream: &fakeAuthorInfoStream{chunks: []*gen.AuthorInfoChunk{
					metaChunk("后续候选"),
				}}}},
			},
		)
		var gotMeta string
		err := fetcher.FetchSiteAuthorInfo(context.Background(), "pixiv", "42", "", "",
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
		map[string][]dto.SiteAuthorFetchDeclaration{"p-a": {fetchEntry("main", "A 源", "pixiv")}},
		map[string]*transport.GRPCPluginClient{
			"p-a": {SiteAuthorFetch: &fakeSiteAuthorFetchClient{stream: &fakeAuthorInfoStream{chunks: []*gen.AuthorInfoChunk{
				metaChunk("归属作者"),
				resourceChunk("bytes"),
			}}}},
		},
	)
	var gotMeta string
	var gotData []string
	err := fetcher.FetchSiteAuthorInfo(context.Background(), "pixiv", "42", "", "",
		func(meta *gen.AuthorInfoMeta) (bool, error) { gotMeta = meta.GetAuthorName(); return true, nil },
		func(data []byte) error { gotData = append(gotData, string(data)); return nil })
	if err != nil {
		t.Fatalf("成功流应无错误: %v", err)
	}
	if gotMeta != "归属作者" || len(gotData) != 1 || gotData[0] != "bytes" {
		t.Fatalf("交付不符: meta=%q data=%v", gotMeta, gotData)
	}
}

// TestSiteAuthorFetcherStopsAtTrueFailure 非归属错误（Internal）按真失败上抛并点名候选（插件
// 标识 + 条目 id），不继续广播后续候选
func TestSiteAuthorFetcherStopsAtTrueFailure(t *testing.T) {
	fetcher := newBroadcastFetcher(
		[]string{"p-a", "p-b"},
		map[string][]dto.SiteAuthorFetchDeclaration{
			"p-a": {fetchEntry("main", "A 源", "pixiv")},
			"p-b": {fetchEntry("main", "B 源", "pixiv")},
		},
		map[string]*transport.GRPCPluginClient{
			"p-a": {SiteAuthorFetch: &fakeSiteAuthorFetchClient{openErr: status.Error(codes.Internal, "boom")}},
			"p-b": {SiteAuthorFetch: &fakeSiteAuthorFetchClient{stream: &fakeAuthorInfoStream{chunks: []*gen.AuthorInfoChunk{metaChunk("不应到达")}}}},
		},
	)
	err := fetcher.FetchSiteAuthorInfo(context.Background(), "pixiv", "42", "", "",
		func(meta *gen.AuthorInfoMeta) (bool, error) { return false, nil },
		func(data []byte) error { return nil })
	if err == nil {
		t.Fatal("真失败应上抛")
	}
	if !strings.Contains(err.Error(), "p-a") {
		t.Fatalf("错误应携带失败候选的插件标识: %v", err)
	}
	if !strings.Contains(err.Error(), "main") {
		t.Fatalf("错误应携带失败候选的条目 id: %v", err)
	}
}

// TestSiteAuthorFetcherNoOwner 无已激活插件条目声明可处理该站点时零候选收口报错，不盲调
func TestSiteAuthorFetcherNoOwner(t *testing.T) {
	fetcher := newBroadcastFetcher(
		[]string{"p-a", "p-b"},
		// p-a 声明了能力包但条目未覆盖 pixiv；p-b 未声明能力包
		map[string][]dto.SiteAuthorFetchDeclaration{"p-a": {fetchEntry("main", "A 源", "bilibili")}},
		map[string]*transport.GRPCPluginClient{
			"p-a": {SiteAuthorFetch: &fakeSiteAuthorFetchClient{stream: &fakeAuthorInfoStream{}}},
		},
	)
	err := fetcher.FetchSiteAuthorInfo(context.Background(), "pixiv", "42", "", "",
		func(meta *gen.AuthorInfoMeta) (bool, error) { return false, nil },
		func(data []byte) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "无归属") {
		t.Fatalf("零候选应收口报错, 实际 %v", err)
	}
	if !strings.Contains(err.Error(), "无已激活插件声明") {
		t.Fatalf("零候选文案应点名无能力声明, 实际 %v", err)
	}
}

// TestSiteAuthorFetchUnclaimedSiteZeroCandidateNoCall 非归属站点（无条目把该站点键列入声明）：
// 零候选且零插件调用——「零插件调用」以客户端 open 标志机检（候选为空时基座不接入调用）
func TestSiteAuthorFetchUnclaimedSiteZeroCandidateNoCall(t *testing.T) {
	client := &fakeSiteAuthorFetchClient{stream: &fakeAuthorInfoStream{chunks: []*gen.AuthorInfoChunk{
		metaChunk("不应到达"),
	}}}
	fetcher := newBroadcastFetcher(
		[]string{"p-a", "p-b"},
		map[string][]dto.SiteAuthorFetchDeclaration{
			"p-a": {fetchEntry("main", "A 源", "pixiv")},
			"p-b": {fetchEntry("main", "B 源", "bilibili")},
		},
		map[string]*transport.GRPCPluginClient{
			"p-a": {SiteAuthorFetch: client},
			"p-b": {SiteAuthorFetch: client},
		},
	)
	err := fetcher.FetchSiteAuthorInfo(context.Background(), "local", "42", "", "",
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

// TestSiteAuthorFetcherMetaOnlyEndsCleanly 消费侧不需要头像字节（onMeta 返回 false）：取消流
// 正常收尾（无错误、字节块不交付）
func TestSiteAuthorFetcherMetaOnlyEndsCleanly(t *testing.T) {
	fetcher := newBroadcastFetcher(
		[]string{"p-a"},
		map[string][]dto.SiteAuthorFetchDeclaration{"p-a": {fetchEntry("main", "A 源", "pixiv")}},
		map[string]*transport.GRPCPluginClient{
			"p-a": {SiteAuthorFetch: &fakeSiteAuthorFetchClient{stream: &fakeAuthorInfoStream{chunks: []*gen.AuthorInfoChunk{
				metaChunk("作者"),
				resourceChunk("不应交付"),
			}}}},
		},
	)
	dataSeen := false
	err := fetcher.FetchSiteAuthorInfo(context.Background(), "pixiv", "42", "", "",
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
			map[string][]dto.SiteAuthorFetchDeclaration{"p-a": {fetchEntry("main", "A 源", "pixiv")}},
			map[string]*transport.GRPCPluginClient{
				"p-a": {SiteAuthorFetch: &fakeSiteAuthorFetchClient{stream: &fakeAuthorInfoStream{chunks: tc.chunks}}},
			},
		)
		err := fetcher.FetchSiteAuthorInfo(context.Background(), "pixiv", "42", "", "",
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
		map[string][]dto.SiteAuthorFetchDeclaration{"p-a": {fetchEntry("main", "A 源", "pixiv")}},
		map[string]*transport.GRPCPluginClient{
			"p-a": {SiteAuthorFetch: &fakeSiteAuthorFetchClient{stream: &fakeAuthorInfoStream{chunks: []*gen.AuthorInfoChunk{metaChunk("作者")}}}},
		},
	)
	wantErr := errors.New("消费侧中止")
	err := fetcher.FetchSiteAuthorInfo(context.Background(), "pixiv", "42", "", "",
		func(meta *gen.AuthorInfoMeta) (bool, error) { return false, wantErr },
		func(data []byte) error { return nil })
	if !errors.Is(err, wantErr) {
		t.Fatalf("消费回调错误应原样上抛, 实际 %v", err)
	}
}

// TestSiteAuthorFetchAdapterPreferredCandidateGoesFirst 显选两键命中候选集时该候选先于全键
// 字典序在前的候选被尝试：两候选均可成功时交付显选者，显选键未命中或两键均空时回落全键字典序
// 首位（交互面显选注入的接缝）
func TestSiteAuthorFetchAdapterPreferredCandidateGoesFirst(t *testing.T) {
	pluginClient := func(authorName string) *transport.GRPCPluginClient {
		return &transport.GRPCPluginClient{SiteAuthorFetch: &fakeSiteAuthorFetchClient{
			stream: &fakeAuthorInfoStream{chunks: []*gen.AuthorInfoChunk{metaChunk(authorName)}},
		}}
	}
	cases := []struct {
		name              string
		preferredPluginId string
		preferredEntryId  string
		wantMeta          string
	}{
		{"显选 p-b/main", "p-b", "main", "p-b 作者"},
		{"显选条目未命中回落全键首位", "p-x", "main", "p-a 作者"},
		{"无显选回落全键首位", "", "", "p-a 作者"},
	}
	for _, tc := range cases {
		fetcher := newBroadcastFetcher(
			[]string{"p-a", "p-b"},
			map[string][]dto.SiteAuthorFetchDeclaration{
				"p-a": {fetchEntry("main", "A 源", "pixiv")},
				"p-b": {fetchEntry("main", "B 源", "pixiv")},
			},
			map[string]*transport.GRPCPluginClient{"p-a": pluginClient("p-a 作者"), "p-b": pluginClient("p-b 作者")},
		)
		var gotMeta string
		adapter := &siteAuthorFetchAdapter{
			fetcher:           fetcher,
			siteKey:           "pixiv",
			siteAuthorId:      "42",
			chosenPluginId:    tc.preferredPluginId,
			chosenExtensionId: tc.preferredEntryId,
			onMeta:            func(meta *gen.AuthorInfoMeta) (bool, error) { gotMeta = meta.GetAuthorName(); return false, nil },
			onData:            func(data []byte) error { return nil },
		}
		if _, err := route.Route[authorFetchCandidate, struct{}](context.Background(), adapter); err != nil {
			t.Fatalf("%s: 应路由成功: %v", tc.name, err)
		}
		if gotMeta != tc.wantMeta {
			t.Fatalf("%s: 交付作者应为 %q, 实际 %q", tc.name, tc.wantMeta, gotMeta)
		}
	}
}

// TestSiteAuthorFetcherChosenCandidateGoesFirst 显选两键经公开拉取入口贯通到候选排序：两候选均
// 可成功时交付显选者，显选键未命中或未显选时回落全键字典序首位（同插件双条目的显选路由由
// TestSiteAuthorFetcherRequestCarriesExtensionId 按请求条目 id 机检）
func TestSiteAuthorFetcherChosenCandidateGoesFirst(t *testing.T) {
	pluginClient := func(authorName string) *transport.GRPCPluginClient {
		return &transport.GRPCPluginClient{SiteAuthorFetch: &fakeSiteAuthorFetchClient{
			stream: &fakeAuthorInfoStream{chunks: []*gen.AuthorInfoChunk{metaChunk(authorName)}},
		}}
	}
	cases := []struct {
		name           string
		chosenPluginId string
		chosenEntryId  string
		wantMeta       string
	}{
		{"显选 p-b", "p-b", "main", "p-b 作者"},
		{"显选条目未命中回落全键首位", "p-b", "ghost", "p-a 作者"},
		{"未显选回落全键首位", "", "", "p-a 作者"},
	}
	for _, tc := range cases {
		fetcher := newBroadcastFetcher(
			[]string{"p-a", "p-b"},
			map[string][]dto.SiteAuthorFetchDeclaration{
				"p-a": {fetchEntry("main", "甲源", "pixiv")},
				"p-b": {fetchEntry("main", "乙源", "pixiv")},
			},
			map[string]*transport.GRPCPluginClient{"p-a": pluginClient("p-a 作者"), "p-b": pluginClient("p-b 作者")},
		)
		var gotMeta string
		err := fetcher.FetchSiteAuthorInfo(context.Background(), "pixiv", "42", tc.chosenPluginId, tc.chosenEntryId,
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

// TestSiteAuthorFetcherRequestCarriesExtensionId 拉取请求携带候选条目 id（插件侧按条目分派的
// 实例身份）：默认路由按候选全键字典序取首位条目，显选两键命中时取显选条目
func TestSiteAuthorFetcherRequestCarriesExtensionId(t *testing.T) {
	cases := []struct {
		name           string
		chosenPluginId string
		chosenEntryId  string
		wantEntryId    string
	}{
		{"默认路由取全键首位条目", "", "", "alpha"},
		{"显选条目随请求下达", "p-a", "beta", "beta"},
	}
	for _, tc := range cases {
		client := &fakeSiteAuthorFetchClient{stream: &fakeAuthorInfoStream{chunks: []*gen.AuthorInfoChunk{
			metaChunk("作者"),
		}}}
		fetcher := newBroadcastFetcher(
			[]string{"p-a"},
			map[string][]dto.SiteAuthorFetchDeclaration{
				"p-a": {fetchEntry("alpha", "甲源", "pixiv"), fetchEntry("beta", "乙源", "pixiv")},
			},
			map[string]*transport.GRPCPluginClient{"p-a": {SiteAuthorFetch: client}},
		)
		err := fetcher.FetchSiteAuthorInfo(context.Background(), "pixiv", "42", tc.chosenPluginId, tc.chosenEntryId,
			func(meta *gen.AuthorInfoMeta) (bool, error) { return false, nil },
			func(data []byte) error { return nil })
		if err != nil {
			t.Fatalf("%s: 应路由成功: %v", tc.name, err)
		}
		if len(client.requests) != 1 {
			t.Fatalf("%s: 应恰好调用一次, 实际 %d 次", tc.name, len(client.requests))
		}
		if got := client.requests[0].GetExtensionId(); got != tc.wantEntryId {
			t.Fatalf("%s: 请求应携带条目 id %q, 实际 %q", tc.name, tc.wantEntryId, got)
		}
		if got := client.requests[0].GetSiteKey(); got != "pixiv" {
			t.Fatalf("%s: 请求应携带站点键, 实际 %q", tc.name, got)
		}
	}
}

// TestListSiteAuthorFetchCandidatesFilteredOrdered 候选清单=已激活 ∩ 有条目声明 ∩ 条目声明站点
// 含请求站点键（按条目收窄：插件的其他条目不覆盖该站点不产出候选） ∩ 客户端可用，按候选全键
// （插件公开 ID 与条目 id 的 NUL 拼接）字典序——同插件双条目按条目 id 排序、跨插件按插件 ID
// 优先；元素为条目级候选（ExtensionId = 条目 id）
func TestListSiteAuthorFetchCandidatesFilteredOrdered(t *testing.T) {
	fetcher := newBroadcastFetcher(
		[]string{"p-c", "p-b", "p-a"},
		map[string][]dto.SiteAuthorFetchDeclaration{
			"p-a": {fetchEntry("alt", "甲源乙", "pixiv"), fetchEntry("main", "甲源甲", "pixiv")},
			"p-b": {fetchEntry("main", "乙源", "pixiv"), fetchEntry("other", "乙源他站", "bilibili")},
			"p-c": {fetchEntry("main", "丙源", "pixiv")}, // 无可用服务客户端
		},
		map[string]*transport.GRPCPluginClient{
			"p-a": {SiteAuthorFetch: &fakeSiteAuthorFetchClient{stream: &fakeAuthorInfoStream{}}},
			"p-b": {SiteAuthorFetch: &fakeSiteAuthorFetchClient{stream: &fakeAuthorInfoStream{}}},
		},
	)
	candidates, err := fetcher.ListSiteAuthorFetchCandidates(context.Background(), "pixiv")
	if err != nil {
		t.Fatalf("枚举候选失败: %v", err)
	}
	want := []struct{ pluginId, extensionId string }{
		{"p-a", "alt"},
		{"p-a", "main"},
		{"p-b", "main"},
	}
	if len(candidates) != len(want) {
		t.Fatalf("候选应为 p-a/alt、p-a/main、p-b/main（p-c 无客户端、p-b/other 不覆盖 pixiv）, 实际 %+v", candidates)
	}
	for i, w := range want {
		if candidates[i].PluginPublicId != w.pluginId || candidates[i].ExtensionId != w.extensionId {
			t.Fatalf("候选 %d 应为 %s/%s, 实际 %+v", i, w.pluginId, w.extensionId, candidates[i])
		}
	}
}

// TestListSiteAuthorFetchCandidatesSameSiteDeclarers 同站点两个声明插件（各一个条目）→ 两候选，
// 按候选全键字典序；未声明该站点的插件不进候选
func TestListSiteAuthorFetchCandidatesSameSiteDeclarers(t *testing.T) {
	fetcher := newBroadcastFetcher(
		[]string{"p-b", "p-a", "p-other"},
		map[string][]dto.SiteAuthorFetchDeclaration{
			"p-a":     {fetchEntry("main", "甲源", "pixiv")},
			"p-b":     {fetchEntry("main", "乙源", "pixiv")},
			"p-other": {fetchEntry("main", "他站源", "bilibili")},
		},
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
		t.Fatalf("同站点两声明者应成两候选且按全键字典序, 实际 %+v", candidates)
	}
}

// TestListSiteAuthorFetchCandidatesPluginName 候选展示名为「插件名 · 条目名」两段拼接，
// 插件未设置展示名时第一段回落公开标识（两态均由展示面消费）
func TestListSiteAuthorFetchCandidatesPluginName(t *testing.T) {
	fetcher := newBroadcastFetcherNamed(
		[]string{"p-a", "p-b"},
		map[string]string{"p-a": "比卡套件"}, // p-b 未设置展示名
		map[string][]dto.SiteAuthorFetchDeclaration{
			"p-a": {fetchEntry("main", "Pixiv 源", "pixiv")},
			"p-b": {fetchEntry("main", "B 站源", "pixiv")},
		},
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
	if got := byId["p-a"]; got == nil || got.PluginName != "比卡套件 · Pixiv 源" {
		t.Fatalf("候选展示名应为「插件名 · 条目名」两段, 实际 %+v", got)
	}
	if got := byId["p-b"]; got == nil || got.PluginName != "p-b · B 站源" {
		t.Fatalf("插件未设置展示名时第一段应回落公开标识, 实际 %+v", got)
	}
}
