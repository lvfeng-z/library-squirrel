package extension

import (
	"fmt"
	"io"
	"sync/atomic"

	pluginsdk "github.com/lvfeng-z/library-squirrel-plugin-sdk"
)

// TaskHandlerProxy 通过 JSON-RPC 代理到子进程的 TaskHandler
// 实现 pluginsdk.TaskHandler 接口，由 taskHandlerAdapter 适配为 dto.TaskHandler
type TaskHandlerProxy struct {
	process       *PluginProcess
	contributionId string
}

// Ensure TaskHandlerProxy implements pluginsdk.TaskHandler
var _ pluginsdk.TaskHandler = (*TaskHandlerProxy)(nil)

func (p *TaskHandlerProxy) Create(url string) ([]*pluginsdk.TaskCreateResponse, error) {
	type params struct {
		ContributionID string `json:"contributionId"`
		URL            string `json:"url"`
	}
	var result []*pluginsdk.TaskCreateResponse
	if err := p.process.rpcClient.Call("taskHandler/create",
		params{ContributionID: p.contributionId, URL: url}, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (p *TaskHandlerProxy) CreateWorkInfo(task *pluginsdk.Task) (*pluginsdk.WorkResponse, error) {
	type params struct {
		ContributionID string         `json:"contributionId"`
		Task           *pluginsdk.Task `json:"task"`
	}
	var result *pluginsdk.WorkResponse
	if err := p.process.rpcClient.Call("taskHandler/createWorkInfo",
		params{ContributionID: p.contributionId, Task: task}, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (p *TaskHandlerProxy) Start(task *pluginsdk.Task) (io.ReadCloser, *pluginsdk.WorkResponse, error) {
	// 生成 streamID 并预先注册流通道，避免竞态
	streamID := generateStreamID()
	ch := p.process.streamMgr.RegisterStream(streamID)

	type params struct {
		ContributionID string         `json:"contributionId"`
		Task           *pluginsdk.Task `json:"task"`
		StreamID       string         `json:"streamId"`
	}
	type result struct {
		WorkResponse *pluginsdk.WorkResponse `json:"workResponse"`
	}
	var r result
	if err := p.process.rpcClient.Call("taskHandler/start",
		params{ContributionID: p.contributionId, Task: task, StreamID: streamID}, &r); err != nil {
		p.process.streamMgr.UnregisterStream(streamID)
		return nil, nil, err
	}

	reader := NewStreamReader(ch, func() {
		p.process.streamMgr.UnregisterStream(streamID)
	}, p.process.streamMgr.GetCloseError)
	return reader, r.WorkResponse, nil
}

func (p *TaskHandlerProxy) Retry(task *pluginsdk.Task) (*pluginsdk.WorkResponse, error) {
	type params struct {
		ContributionID string         `json:"contributionId"`
		Task           *pluginsdk.Task `json:"task"`
	}
	var result *pluginsdk.WorkResponse
	if err := p.process.rpcClient.Call("taskHandler/retry",
		params{ContributionID: p.contributionId, Task: task}, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (p *TaskHandlerProxy) Pause(param *pluginsdk.TaskResParam) error {
	type params struct {
		ContributionID string                `json:"contributionId"`
		Param          *pluginsdk.TaskResParam `json:"param"`
	}
	return p.process.rpcClient.Call("taskHandler/pause",
		params{ContributionID: p.contributionId, Param: param}, nil)
}

func (p *TaskHandlerProxy) Stop(param *pluginsdk.TaskResParam) error {
	type params struct {
		ContributionID string                `json:"contributionId"`
		Param          *pluginsdk.TaskResParam `json:"param"`
	}
	return p.process.rpcClient.Call("taskHandler/stop",
		params{ContributionID: p.contributionId, Param: param}, nil)
}

func (p *TaskHandlerProxy) Resume(param *pluginsdk.TaskResParam) (*pluginsdk.WorkResponse, error) {
	type params struct {
		ContributionID string                `json:"contributionId"`
		Param          *pluginsdk.TaskResParam `json:"param"`
	}
	var result *pluginsdk.WorkResponse
	if err := p.process.rpcClient.Call("taskHandler/resume",
		params{ContributionID: p.contributionId, Param: param}, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// SiteBrowserProxy 通过 JSON-RPC 代理到子进程的 SiteBrowser
type SiteBrowserProxy struct {
	process       *PluginProcess
	contributionId string
}

// Ensure SiteBrowserProxy implements pluginsdk.SiteBrowser
var _ pluginsdk.SiteBrowser = (*SiteBrowserProxy)(nil)

func (p *SiteBrowserProxy) Open() error {
	type params struct {
		ContributionID string `json:"contributionId"`
	}
	return p.process.rpcClient.Call("siteBrowser/open",
		params{ContributionID: p.contributionId}, nil)
}

func (p *SiteBrowserProxy) Close() error {
	type params struct {
		ContributionID string `json:"contributionId"`
	}
	return p.process.rpcClient.Call("siteBrowser/close",
		params{ContributionID: p.contributionId}, nil)
}

// streamIDCounter 用于生成唯一的流 ID
var streamIDCounter atomic.Int64

func generateStreamID() string {
	id := streamIDCounter.Add(1)
	return fmt.Sprintf("stream-%d", id)
}
