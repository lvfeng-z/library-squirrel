package extension

import (
	"context"
	"io"
	"testing"

	"github.com/lvfeng-z/library-squirrel-sdk/gen"
	transport "github.com/lvfeng-z/library-squirrel-sdk/transport"
	"google.golang.org/grpc"
)

// Create 流的 error 块语义：承载插件业务失败原因（用户可读文本），必为流的最后一块；
// gRPC 层错误（进程崩溃/连接中断/传输异常）不经 error 块、由 err 原样返回。
// 本文件以预置块序列的服务端流替身锚定代理侧的解析行为。

// fakeCreateStream 顺序吐出预置 CreateChunk 的服务端流替身；块耗尽后按 err 字段收尾
//（nil 为 io.EOF 正常结束，非 nil 模拟连接中断）。
type fakeCreateStream struct {
	grpc.ClientStream
	chunks []*gen.CreateChunk
	next   int
	err    error
}

func (s *fakeCreateStream) Recv() (*gen.CreateChunk, error) {
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

// fakeTaskClient 仅实现 Create 的任务服务客户端替身（其余方法经接口嵌入满足签名）。
type fakeTaskClient struct {
	gen.TaskHandlerServiceClient
	stream *fakeCreateStream
}

func (c *fakeTaskClient) Create(context.Context, *gen.CreateRequest, ...grpc.CallOption) (grpc.ServerStreamingClient[gen.CreateChunk], error) {
	return c.stream, nil
}

// fakeServiceAccessor 返回预置 GRPCPluginClient 的访问器替身。
type fakeServiceAccessor struct {
	client *transport.GRPCPluginClient
}

func (a *fakeServiceAccessor) GetServices(string) (*transport.GRPCPluginClient, bool) {
	if a.client == nil {
		return nil, false
	}
	return a.client, true
}

// newCreateProxy 构造以预置块序列为 Create 流的代理。
func newCreateProxy(chunks ...*gen.CreateChunk) *TaskHandlerProxy {
	return &TaskHandlerProxy{
		serviceAccessor: &fakeServiceAccessor{client: &transport.GRPCPluginClient{
			Task: &fakeTaskClient{stream: &fakeCreateStream{chunks: chunks}},
		}},
	}
}

// taskChunk 构造承载单个任务声明的块。
func taskChunk(siteWorkId string) *gen.CreateChunk {
	return &gen.CreateChunk{Payload: &gen.CreateChunk_Task{
		Task: &gen.TaskCreateResponse{TaskName: "t-" + siteWorkId, SiteWorkId: siteWorkId, Url: "http://x/" + siteWorkId},
	}}
}

// TestProxyCreate_FirstChunkError 插件 Create 错误返回时流仅含单个 error 块（无 mode 块）：
// 代理解析为零任务批量结果并承载 reason。
func TestProxyCreate_FirstChunkError(t *testing.T) {
	proxy := newCreateProxy(&gen.CreateChunk{Payload: &gen.CreateChunk_Error{Error: "获取令牌失败"}})

	result, err := proxy.Create("http://x")
	if err != nil {
		t.Fatalf("Create 返回错误: %v", err)
	}
	if result.IsStream() {
		t.Fatal("期望批量模式，得到流式")
	}
	if got := len(result.Array()); got != 0 {
		t.Fatalf("期望零任务，得到 %d 个", got)
	}
	if got := result.Reason(); got != "获取令牌失败" {
		t.Fatalf("期望 reason 透传，得到 %q", got)
	}
}

// TestProxyCreate_BatchTrailingError 批量模式末尾 error 块承载插件声明的业务原因，任务照常收集。
func TestProxyCreate_BatchTrailingError(t *testing.T) {
	proxy := newCreateProxy(
		&gen.CreateChunk{Payload: &gen.CreateChunk_Mode{Mode: &gen.CreateMode{IsStream: false}}},
		taskChunk("w-1"),
		&gen.CreateChunk{Payload: &gen.CreateChunk_Error{Error: "部分任务创建失败"}},
	)

	result, err := proxy.Create("http://x")
	if err != nil {
		t.Fatalf("Create 返回错误: %v", err)
	}
	if got := len(result.Array()); got != 1 {
		t.Fatalf("期望收集 1 个任务，得到 %d 个", got)
	}
	if got := result.Reason(); got != "部分任务创建失败" {
		t.Fatalf("期望 reason 透传，得到 %q", got)
	}
}

// TestProxyCreate_StreamTrailingError 流式模式末尾 error 块：任务照常流经 channel，
// reason 在 channel close 后可读（happens-before 由 close 建立）。
func TestProxyCreate_StreamTrailingError(t *testing.T) {
	proxy := newCreateProxy(
		&gen.CreateChunk{Payload: &gen.CreateChunk_Mode{Mode: &gen.CreateMode{IsStream: true}}},
		taskChunk("w-1"),
		&gen.CreateChunk{Payload: &gen.CreateChunk_Error{Error: "部分任务创建失败"}},
	)

	result, err := proxy.Create("http://x")
	if err != nil {
		t.Fatalf("Create 返回错误: %v", err)
	}
	if !result.IsStream() {
		t.Fatal("期望流式模式，得到批量")
	}
	var n int
	for resp := range result.Stream() {
		if resp.SiteWorkId != "w-1" {
			t.Fatalf("期望任务声明流经 channel，得到 SiteWorkId=%q", resp.SiteWorkId)
		}
		n++
	}
	if n != 1 {
		t.Fatalf("期望流经 1 个任务，得到 %d 个", n)
	}
	if got := result.Reason(); got != "部分任务创建失败" {
		t.Fatalf("期望流消费完毕后 reason 可读，得到 %q", got)
	}
}

// TestProxyCreate_BatchCleanEnd 批量模式无 error 块时以 io.EOF 正常收尾，reason 为空。
func TestProxyCreate_BatchCleanEnd(t *testing.T) {
	proxy := newCreateProxy(
		&gen.CreateChunk{Payload: &gen.CreateChunk_Mode{Mode: &gen.CreateMode{IsStream: false}}},
		taskChunk("w-1"),
	)

	result, err := proxy.Create("http://x")
	if err != nil {
		t.Fatalf("Create 返回错误: %v", err)
	}
	if got := len(result.Array()); got != 1 {
		t.Fatalf("期望收集 1 个任务，得到 %d 个", got)
	}
	if got := result.Reason(); got != "" {
		t.Fatalf("无 error 块时 reason 应为空，得到 %q", got)
	}
}

// TestProxyCreate_RecvErrorReturned 批量模式收流途中 gRPC 层错误（非 io.EOF）：
// 原样返回错误，不吞成截断的批量结果。
func TestProxyCreate_RecvErrorReturned(t *testing.T) {
	client := &fakeTaskClient{stream: &fakeCreateStream{
		chunks: []*gen.CreateChunk{
			{Payload: &gen.CreateChunk_Mode{Mode: &gen.CreateMode{IsStream: false}}},
			taskChunk("w-1"),
		},
		err: io.ErrUnexpectedEOF,
	}}
	proxy := &TaskHandlerProxy{
		serviceAccessor: &fakeServiceAccessor{client: &transport.GRPCPluginClient{Task: client}},
	}

	if _, err := proxy.Create("http://x"); err == nil {
		t.Fatal("gRPC 层收流错误应原样返回，得到 nil")
	}
}
