package extension

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/lvfeng-z/library-squirrel-sdk/gen"
)

// 本文件锚定宿主对流等待期心跳块的接收面行为（Start/Resume 首响应段与 pull 问答）：
// - 首响应段（WorkResponse 可选 + Specs 声明）：心跳块先于或穿插 WorkResponse/Specs 到达时忽略
//   继续等，正常响应照常生效
// - pull 问答（Read = 发一次 PullRequest + 收一次响应）：数据传输期心跳块结构性不存在
//   （插件 handler 返回时心跳上报器已关闭），未知块形态 default 报错保持即协议自校验

// streamChunkSeq 构造按序吐出预置 StreamChunk 的接收函数（供 recvSpecsAndPull 直调；
// 序列耗尽后返回 io.EOF，对应真实流上 Specs 之后进入 pull 阶段不再经首响应段收块）。
func streamChunkSeq(chunks ...*gen.StreamChunk) func() (*gen.StreamChunk, error) {
	next := 0
	return func() (*gen.StreamChunk, error) {
		if next >= len(chunks) {
			return nil, io.EOF
		}
		c := chunks[next]
		next++
		return c, nil
	}
}

// streamHeartbeatChunk 构造流通道上的心跳块（Start/Resume handler 执行期等待保活）。
func streamHeartbeatChunk() *gen.StreamChunk {
	return &gen.StreamChunk{Payload: &gen.StreamChunk_Heartbeat{Heartbeat: &gen.Heartbeat{}}}
}

// workResponseChunk 构造承载作品信息的块（首响应可选段）。
func workResponseChunk() *gen.StreamChunk {
	return &gen.StreamChunk{Payload: &gen.StreamChunk_WorkResponse{WorkResponse: &gen.WorkResponse{}}}
}

// specsChunk 构造承载单 role 声明的块（首响应必达段）。
func specsChunk(role string) *gen.StreamChunk {
	return &gen.StreamChunk{Payload: &gen.StreamChunk_Specs{Specs: &gen.StoreSpecs{
		Items: []*gen.StoreSpecMeta{{Role: role}},
	}}}
}

// TestRecvSpecsAndPull_HeartbeatBeforeFirstResponse 心跳块先于首响应到达（插件在 Start/Resume
// handler 内等待慢初始化等长逻辑时保活）：首响应等待忽略心跳块，WorkResponse+Specs 照常解析。
func TestRecvSpecsAndPull_HeartbeatBeforeFirstResponse(t *testing.T) {
	specs, workResp, err := recvSpecsAndPull(
		func(string, int) error { return nil },
		streamChunkSeq(
			streamHeartbeatChunk(),
			workResponseChunk(),
			specsChunk("image"),
		),
		func() {},
		time.Minute,
	)
	if err != nil {
		t.Fatalf("recvSpecsAndPull 返回错误: %v", err)
	}
	if workResp == nil {
		t.Fatal("期望 WorkResponse 解析为非 nil")
	}
	if len(specs) != 1 || specs[0].Role != "image" {
		t.Fatalf("期望单 role 声明，得到 %+v", specs)
	}
}

// TestRecvSpecsAndPull_HeartbeatInterleavedWithFirstResponse 心跳块穿插 WorkResponse 与 Specs
// 之间：穿插心跳不影响首响应段收集。
func TestRecvSpecsAndPull_HeartbeatInterleavedWithFirstResponse(t *testing.T) {
	specs, workResp, err := recvSpecsAndPull(
		func(string, int) error { return nil },
		streamChunkSeq(
			streamHeartbeatChunk(),
			workResponseChunk(),
			streamHeartbeatChunk(),
			specsChunk("image"),
		),
		func() {},
		time.Minute,
	)
	if err != nil {
		t.Fatalf("recvSpecsAndPull 返回错误: %v", err)
	}
	if workResp == nil {
		t.Fatal("期望 WorkResponse 解析为非 nil")
	}
	if len(specs) != 1 || specs[0].Role != "image" {
		t.Fatalf("期望单 role 声明，得到 %+v", specs)
	}
}

// TestPullReadCloserRead_HeartbeatRejected pull 问答对心跳块 default 报错保持：数据传输期
// 心跳块结构性不存在（插件 handler 返回时上报器已关闭），意外块形态报错即协议自校验。
func TestPullReadCloserRead_HeartbeatRejected(t *testing.T) {
	session := &pullSession{
		sendPull:    func(string, int) error { return nil },
		recvChunk:   func() (*gen.StreamChunk, error) { return streamHeartbeatChunk(), nil },
		cancel:      func() {},
		idleTimeout: time.Minute,
		refCount:    1,
	}
	reader := &pullReadCloser{session: session, role: "image"}

	_, err := reader.Read(make([]byte, 8))
	if err == nil || !strings.Contains(err.Error(), "意外的 pull 响应") {
		t.Fatalf("期望 pull 问答对心跳块报错（协议自校验），得到 %v", err)
	}
}
