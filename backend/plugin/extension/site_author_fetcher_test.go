package extension

// siteAuthorFetcher 能力广播路由锚定：未归属（PermissionDenied，开流或首个 Recv 到达均覆盖）
// 静默跳过继续广播、真失败命中即止、无能力/无归属收口报错、消费侧免字节时取消流正常收尾。

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

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

// authorInfoLister 广播遍历清单替身
type authorInfoLister struct {
	ids []string
}

func (l *authorInfoLister) ListActivePluginIds() []string {
	return l.ids
}

func newBroadcastFetcher(ids []string, caps map[string][]string, byPlugin map[string]*transport.GRPCPluginClient) *siteAuthorFetcher {
	return NewSiteAuthorFetcher(&authorInfoLister{ids: ids}, &authorInfoCapsQuery{caps: caps}, &authorInfoAccessor{byPlugin: byPlugin})
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
	err := fetcher.FetchSiteAuthorInfo(context.Background(), "pixiv", "42",
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
	err := fetcher.FetchSiteAuthorInfo(context.Background(), "pixiv", "42",
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
	err := fetcher.FetchSiteAuthorInfo(context.Background(), "pixiv", "42",
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
	err := fetcher.FetchSiteAuthorInfo(context.Background(), "pixiv", "42",
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
		err := fetcher.FetchSiteAuthorInfo(context.Background(), "pixiv", "42",
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
	err := fetcher.FetchSiteAuthorInfo(context.Background(), "pixiv", "42",
		func(meta *gen.AuthorInfoMeta) (bool, error) { return false, wantErr },
		func(data []byte) error { return nil })
	if !errors.Is(err, wantErr) {
		t.Fatalf("消费回调错误应原样上抛, 实际 %v", err)
	}
}
