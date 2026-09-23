package task

import (
	"context"
	"testing"
	"time"

	"github.com/library-squirrel/backend/base/model/entity"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// TestCreateTaskByURL_StreamConsumerReturnsOnCtxCancel 流式创建中调用方 ctx 取消：
// 消费循环退出、CreateTaskByURL 返回，不因插件流持续产出而永久等待
// （消费侧逐项检查 ctx.Done 的保持性锚定——插件侧泵的对应退出行为由 extension 包用例锚定）。
func TestCreateTaskByURL_StreamConsumerReturnsOnCtxCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 生产者随 ctx 取消退出并关闭流 channel（与代理接收泵的收尾契约一致：泵退出即 close）；
	// 取消前持续产出任务声明
	taskChan := make(chan *sdkdto.TaskCreateResponse)
	producerDone := make(chan struct{})
	go func() {
		defer close(producerDone)
		defer close(taskChan)
		for {
			select {
			case taskChan <- &sdkdto.TaskCreateResponse{
				TaskName:     "t",
				SiteWorkId:   "w",
				Url:          "http://x",
				SiteKey:      testSiteKey,
				ResourceType: entity.ResourceTypeImage,
			}:
			case <-ctx.Done():
				return
			}
		}
	}()

	handler := &fakePluginWorkFetcher{create: func(string) (*sdkdto.TaskCreateResult, error) {
		return sdkdto.StreamResult(taskChan), nil
	}}
	svc, _ := newCreateByURLService(t,
		&fakeWorkFetchGetter{handlers: map[string]sdkdto.WorkFetcher{"pub-a/ext-a": handler}},
		newURLListenerService(namedListener("pub-a", "插件A", "ext-a")))

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	returned := make(chan struct{})
	go func() {
		defer close(returned)
		svc.CreateTaskByURL(ctx, "http://x/1")
	}()
	select {
	case <-returned:
	case <-time.After(3 * time.Second):
		t.Fatal("调用方取消后 CreateTaskByURL 未返回（消费侧取消检查失效）")
	}

	select {
	case <-producerDone:
	case <-time.After(3 * time.Second):
		t.Fatal("生产者应随 ctx 取消退出")
	}
}

// ctxAwareStreamHandler 同时实现 Create 与 CreateWithContext 的处理器替身：CreateWithContext
// 返回随 ctx 取消关闭的流，Create 记录降级路径被使用（生产接线应经 ctx 感知通道）。
type ctxAwareStreamHandler struct {
	sdkdto.WorkFetcher
	usedFallback bool
}

func (f *ctxAwareStreamHandler) Create(string) (*sdkdto.TaskCreateResult, error) {
	f.usedFallback = true
	return sdkdto.BatchResult(nil), nil
}

func (f *ctxAwareStreamHandler) CreateWithContext(ctx context.Context, url string) (*sdkdto.TaskCreateResult, error) {
	ch := make(chan *sdkdto.TaskCreateResponse)
	go func() {
		defer close(ch)
		for {
			select {
			case ch <- &sdkdto.TaskCreateResponse{
				TaskName:     "t",
				SiteWorkId:   "w",
				Url:          "http://x",
				SiteKey:      testSiteKey,
				ResourceType: entity.ResourceTypeImage,
			}:
			case <-ctx.Done():
				return
			}
		}
	}()
	return sdkdto.StreamResult(ch), nil
}

// TestCreateTaskByURL_RoutesThroughCtxAwareChannel 生产通道接线锚定：CreateTaskByURL 经
// ctx 感知通道（CreateWithContext）继承调用方 ctx——取消后消费终止、调用返回，
// 且不走无 ctx 的 Create 降级路径。
func TestCreateTaskByURL_RoutesThroughCtxAwareChannel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler := &ctxAwareStreamHandler{}
	svc, _ := newCreateByURLService(t,
		&fakeWorkFetchGetter{handlers: map[string]sdkdto.WorkFetcher{"pub-a/ext-a": handler}},
		newURLListenerService(namedListener("pub-a", "插件A", "ext-a")))

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	returned := make(chan struct{})
	go func() {
		defer close(returned)
		svc.CreateTaskByURL(ctx, "http://x/1")
	}()
	select {
	case <-returned:
	case <-time.After(3 * time.Second):
		t.Fatal("调用方取消后 CreateTaskByURL 未返回（ctx 感知通道未接线或取消未传播）")
	}
	if handler.usedFallback {
		t.Fatal("应经 ctx 感知通道创建，实际走了无 ctx 的 Create 降级路径")
	}
}
