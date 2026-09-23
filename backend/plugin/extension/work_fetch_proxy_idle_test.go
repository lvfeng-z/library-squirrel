package extension

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	pluginsdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
	"github.com/lvfeng-z/library-squirrel-sdk/gen"
	transport "github.com/lvfeng-z/library-squirrel-sdk/transport"
	"google.golang.org/grpc"
)

// 本文件锚定流式链路的空闲超时与取消语义：
// - 单次接收等待数据的窗口为「两次数据之间的最大空闲」，到期 cancel 所属流 ctx（插件 hang → 超时错误）
// - Create 接收泵在流 ctx 取消后退出（结果 channel 关闭），不再因消费方停止读取而永久阻塞
// - unary 调用以调用方 ctx 为基：调用方取消立即打断 gRPC 等待
// 超时窗口经 readerIdleTimeout 覆写注入缩时值（生产默认 60s 语义以「缩时内报错」等价验证）

// hangingTaskClient 建流即成功的客户端替身：流的 Recv 永久阻塞直至流 ctx 取消
// （模拟插件 hang：连接活但不产出数据）。
type hangingTaskClient struct {
	gen.WorkFetchServiceClient
}

func (c *hangingTaskClient) Create(ctx context.Context, _ *gen.CreateRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[gen.CreateChunk], error) {
	return &ctxBlockCreateStream{streamCtx: ctx}, nil
}

// ctxBlockCreateStream Recv 阻塞至流 ctx 取消的流替身
type ctxBlockCreateStream struct {
	grpc.ClientStream
	streamCtx context.Context
}

func (s *ctxBlockCreateStream) Recv() (*gen.CreateChunk, error) {
	<-s.streamCtx.Done()
	return nil, s.streamCtx.Err()
}

// endlessTaskClient 建流即成功的客户端替身：首块声明流式模式，之后无限产出任务块直至流 ctx 取消
// （模拟插件持续产出、消费方停止读取的泄漏场景）。
type endlessTaskClient struct {
	gen.WorkFetchServiceClient
}

func (c *endlessTaskClient) Create(ctx context.Context, _ *gen.CreateRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[gen.CreateChunk], error) {
	return &endlessCreateStream{streamCtx: ctx, first: true}, nil
}

type endlessCreateStream struct {
	grpc.ClientStream
	streamCtx context.Context
	first     bool
}

func (s *endlessCreateStream) Recv() (*gen.CreateChunk, error) {
	if s.first {
		s.first = false
		return &gen.CreateChunk{Payload: &gen.CreateChunk_Mode{Mode: &gen.CreateMode{IsStream: true}}}, nil
	}
	select {
	case <-s.streamCtx.Done():
		return nil, s.streamCtx.Err()
	default:
		return taskChunk("w-endless"), nil
	}
}

// blockingPauseClient Pause 调用阻塞直至 ctx 取消的客户端替身（模拟插件 handler 卡死）
type blockingPauseClient struct {
	gen.WorkFetchServiceClient
}

func (c *blockingPauseClient) Pause(ctx context.Context, _ *gen.TaskResParamMessage, _ ...grpc.CallOption) (*gen.Empty, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// TestProxyCreate_FirstChunkIdleTimeout 插件建流后不产出（hang）：首块接收在空闲窗口内到期，
// 返回超时错误而非无限阻塞。
func TestProxyCreate_FirstChunkIdleTimeout(t *testing.T) {
	proxy := newWorkFetchProxy(&fakeServiceAccessor{client: &transport.GRPCPluginClient{WorkFetch: &hangingTaskClient{}}}, "", "")
	proxy.readerIdleTimeout = 40 * time.Millisecond

	start := time.Now()
	_, err := proxy.Create("http://x")
	if err == nil {
		t.Fatal("插件 hang 时 Create 应返回超时错误")
	}
	if !strings.Contains(err.Error(), "空闲超时") {
		t.Fatalf("期望空闲超时错误，得到 %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("超时应在缩时窗口内触发，实际耗时 %v", elapsed)
	}
}

// TestProxyCreateStream_PumpExitsOnCallerCancel 流式模式消费方停止读取（channel 缓冲占满）后：
// 调用方取消流 ctx，接收泵应退出并关闭结果 channel，不再永久阻塞在发送上。
func TestProxyCreateStream_PumpExitsOnCallerCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	proxy := newWorkFetchProxy(&fakeServiceAccessor{client: &transport.GRPCPluginClient{WorkFetch: &endlessTaskClient{}}}, "", "")

	result, err := proxy.CreateWithContext(ctx, "http://x")
	if err != nil {
		t.Fatalf("createStream 返回错误: %v", err)
	}
	if !result.IsStream() {
		t.Fatal("期望流式模式")
	}

	// 不消费结果 channel：泵填满缓冲后阻塞在发送上
	time.Sleep(50 * time.Millisecond)
	cancel()

	drained := make(chan int)
	go func() {
		n := 0
		for range result.Stream() {
			n++
		}
		drained <- n
	}()
	select {
	case n := <-drained:
		if n < 16 {
			t.Fatalf("泵应已填满 16 项缓冲后进入发送阻塞，实际仅产出 %d 项", n)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("调用方取消后泵未退出（结果 channel 未关闭）")
	}
}

// TestProxyPauseWithContext_CallerCancelBreaksWait unary 调用以调用方 ctx 为基：
// 插件 handler 阻塞时调用方取消应立即打断等待（Background 基则须等满 UnaryRPCTimeout）。
func TestProxyPauseWithContext_CallerCancelBreaksWait(t *testing.T) {
	proxy := newWorkFetchProxy(&fakeServiceAccessor{client: &transport.GRPCPluginClient{WorkFetch: &blockingPauseClient{}}}, "", "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	err := proxy.PauseWithContext(ctx, &pluginsdkdto.TaskResParam{})
	if err == nil {
		t.Fatal("插件 handler 阻塞时 Pause 应返回错误")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("期望调用方取消传播（context.Canceled），得到 %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("调用方取消应立即打断等待，实际耗时 %v", elapsed)
	}
}

// TestPullReadCloserRead_HangingRecvTimesOut 插件对 pull 请求 hang（连接活但不响应）：
// Read 在空闲窗口内到期报错并 cancel 会话所属流。
func TestPullReadCloserRead_HangingRecvTimesOut(t *testing.T) {
	streamCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	session := &pullSession{
		sendPull:    func(string, int) error { return nil },
		recvChunk:   func() (*gen.StreamChunk, error) { <-streamCtx.Done(); return nil, streamCtx.Err() },
		cancel:      cancel,
		idleTimeout: 40 * time.Millisecond,
		refCount:    1,
	}
	reader := &pullReadCloser{session: session, role: "image"}

	start := time.Now()
	_, err := reader.Read(make([]byte, 8))
	if err == nil || !strings.Contains(err.Error(), "空闲超时") {
		t.Fatalf("期望空闲超时错误，得到 %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("超时应在缩时窗口内触发，实际耗时 %v", elapsed)
	}
	if streamCtx.Err() == nil {
		t.Fatal("超时应 cancel 会话所属流 ctx")
	}
}

// TestPullReadCloserRead_WindowResetsPerRead 窗口语义为「两次数据之间的最大空闲」而非总时长：
// 三次 Read 各耗时 40ms（单次低于 60ms 窗口）连续执行，累计 120ms 不触发超时。
func TestPullReadCloserRead_WindowResetsPerRead(t *testing.T) {
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	session := &pullSession{
		sendPull: func(string, int) error { return nil },
		recvChunk: func() (*gen.StreamChunk, error) {
			time.Sleep(40 * time.Millisecond)
			return &gen.StreamChunk{Role: "image", Payload: &gen.StreamChunk_Data{Data: []byte("x")}}, nil
		},
		cancel:      cancel,
		idleTimeout: 60 * time.Millisecond,
		refCount:    1,
	}
	reader := &pullReadCloser{session: session, role: "image"}

	for i := 0; i < 3; i++ {
		n, err := reader.Read(make([]byte, 8))
		if err != nil {
			t.Fatalf("第 %d 次 Read 不应超时（单次等待在窗口内）: %v", i+1, err)
		}
		if n != 1 {
			t.Fatalf("期望读到 1 字节，得到 %d", n)
		}
	}
}

// TestRecvSpecsAndPull_FirstResponseIdleTimeout Start/Resume 首响应阶段（WorkResponse 可选 + Specs
// 声明）插件 hang：空闲窗口内到期报错并取消流。
func TestRecvSpecsAndPull_FirstResponseIdleTimeout(t *testing.T) {
	streamCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, _, err := recvSpecsAndPull(
		func(string, int) error { return nil },
		func() (*gen.StreamChunk, error) { <-streamCtx.Done(); return nil, streamCtx.Err() },
		cancel,
		40*time.Millisecond,
	)
	if err == nil || !strings.Contains(err.Error(), "空闲超时") {
		t.Fatalf("期望空闲超时错误，得到 %v", err)
	}
	if streamCtx.Err() == nil {
		t.Fatal("超时应 cancel 所属流 ctx")
	}
}
