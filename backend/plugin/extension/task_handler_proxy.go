package extension

import (
	"context"
	"fmt"
	"io"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pluginsdk "github.com/lvfeng-z/library-squirrel-plugin-sdk"
	"github.com/lvfeng-z/library-squirrel-plugin-sdk/gen"
)

// TaskHandlerProxy 通过 gRPC 代理到子进程的 TaskHandler
type TaskHandlerProxy struct {
	loader         *Loader
	pluginPublicId string
	contributionId string
}

var _ pluginsdk.TaskHandler = (*TaskHandlerProxy)(nil)

func (p *TaskHandlerProxy) getTaskClient() (gen.TaskHandlerServiceClient, error) {
	services, ok := p.loader.GetServices(p.pluginPublicId)
	if !ok {
		return nil, fmt.Errorf("plugin %s not found", p.pluginPublicId)
	}
	return services.Task, nil
}

func (p *TaskHandlerProxy) Create(url string) (*pluginsdk.TaskCreateResult, error) {
	client, err := p.getTaskClient()
	if err != nil {
		return nil, err
	}
	stream, err := client.Create(context.Background(), &gen.CreateRequest{
		Url:            url,
		ContributionId: p.contributionId,
	})
	if err != nil {
		return nil, err
	}

	// 读取首条消息获取模式
	chunk, err := stream.Recv()
	if err != nil {
		return nil, err
	}

	modeChunk := chunk.GetMode()
	if modeChunk == nil {
		return nil, fmt.Errorf("first CreateChunk must contain CreateMode")
	}

	if modeChunk.IsStream {
		// 流式模式
		ch := make(chan *pluginsdk.TaskCreateResponse, 16)
		go func() {
			defer close(ch)
			for {
				c, err := stream.Recv()
				if err != nil {
					return
				}
				taskProto := c.GetTask()
				if taskProto != nil {
					ch <- protoToTaskCreateResponse(taskProto)
				}
			}
		}()
		return pluginsdk.StreamResult(ch), nil
	}

	// 批量模式：收集所有 task
	var responses []*pluginsdk.TaskCreateResponse
	for {
		taskProto := chunk.GetTask()
		if taskProto != nil {
			responses = append(responses, protoToTaskCreateResponse(taskProto))
		}
		chunk, err = stream.Recv()
		if err != nil {
			break
		}
	}
	return pluginsdk.BatchResult(responses), nil
}

func (p *TaskHandlerProxy) CreateWorkInfo(task *pluginsdk.Task) (*pluginsdk.WorkResponse, error) {
	client, err := p.getTaskClient()
	if err != nil {
		return nil, err
	}
	resp, err := client.CreateWorkInfo(context.Background(), &gen.CreateWorkInfoRequest{
		Task:          taskToProto(task),
		ContributionId: p.contributionId,
	})
	if err != nil {
		return nil, err
	}
	return protoToWorkResponse(resp), nil
}

func (p *TaskHandlerProxy) Start(task *pluginsdk.Task) (io.ReadCloser, *pluginsdk.WorkResponse, error) {
	client, err := p.getTaskClient()
	if err != nil {
		return nil, nil, err
	}
	stream, err := client.Start(context.Background(), &gen.StartRequest{
		Task:          taskToProto(task),
		ContributionId: p.contributionId,
	})
	if err != nil {
		return nil, nil, err
	}

	// 读取第一个 chunk 获取 WorkResponse
	chunk, err := stream.Recv()
	if err != nil {
		return nil, nil, err
	}

	wrProto := chunk.GetWorkResponse()
	if wrProto == nil {
		return nil, nil, fmt.Errorf("first chunk must contain WorkResponse")
	}
	workResp := protoToWorkResponse(wrProto)

	reader := &grpcStreamReader{stream: stream}
	return reader, workResp, nil
}

func (p *TaskHandlerProxy) Retry(task *pluginsdk.Task) (*pluginsdk.WorkResponse, error) {
	client, err := p.getTaskClient()
	if err != nil {
		return nil, err
	}
	resp, err := client.Retry(context.Background(), &gen.RetryRequest{
		Task:          taskToProto(task),
		ContributionId: p.contributionId,
	})
	if err != nil {
		return nil, err
	}
	return protoToWorkResponse(resp), nil
}

func (p *TaskHandlerProxy) Pause(param *pluginsdk.TaskResParam) error {
	client, err := p.getTaskClient()
	if err != nil {
		return err
	}
	_, err = client.Pause(context.Background(), &gen.TaskResParamMessage{
		Param:         taskResParamToProto(param),
		ContributionId: p.contributionId,
	})
	return err
}

func (p *TaskHandlerProxy) Stop(param *pluginsdk.TaskResParam) error {
	client, err := p.getTaskClient()
	if err != nil {
		return err
	}
	_, err = client.Stop(context.Background(), &gen.TaskResParamMessage{
		Param:         taskResParamToProto(param),
		ContributionId: p.contributionId,
	})
	return err
}

func (p *TaskHandlerProxy) Resume(param *pluginsdk.TaskResParam) (*pluginsdk.WorkResponse, error) {
	client, err := p.getTaskClient()
	if err != nil {
		return nil, err
	}
	stream, err := client.Resume(context.Background(), &gen.TaskResParamMessage{
		Param:         taskResParamToProto(param),
		ContributionId: p.contributionId,
	})
	if err != nil {
		return nil, err
	}

	// Resume 流式返回，读取第一个 chunk 获取 WorkResponse
	chunk, err := stream.Recv()
	if err != nil {
		return nil, err
	}

	wrProto := chunk.GetWorkResponse()
	if wrProto != nil {
		return protoToWorkResponse(wrProto), nil
	}
	// 无 WorkResponse 时返回 nil
	return nil, nil
}

// SiteBrowserProxy 通过 gRPC 代理到子进程的 SiteBrowser
type SiteBrowserProxy struct {
	loader         *Loader
	pluginPublicId string
	contributionId string
}

var _ pluginsdk.SiteBrowser = (*SiteBrowserProxy)(nil)

func (p *SiteBrowserProxy) Open() error {
	services, ok := p.loader.GetServices(p.pluginPublicId)
	if !ok {
		return fmt.Errorf("plugin %s not found", p.pluginPublicId)
	}
	_, err := services.Browser.Open(context.Background(), &gen.BrowserRequest{
		ContributionId: p.contributionId,
	})
	return err
}

func (p *SiteBrowserProxy) Close() error {
	services, ok := p.loader.GetServices(p.pluginPublicId)
	if !ok {
		return fmt.Errorf("plugin %s not found", p.pluginPublicId)
	}
	_, err := services.Browser.Close(context.Background(), &gen.BrowserRequest{
		ContributionId: p.contributionId,
	})
	return err
}

// ========== Proto 转换函数（proxy 专用）==========

func taskToProto(t *pluginsdk.Task) *gen.Task {
	if t == nil {
		return nil
	}
	return &gen.Task{
		Id:                   t.ID,
		CreateTime:           t.CreateTime,
		UpdateTime:           t.UpdateTime,
		IsCollection:         t.IsCollection,
		Pid:                  t.Pid,
		TaskName:             t.TaskName,
		SiteId:               t.SiteID,
		SiteWorkId:           t.SiteWorkID,
		Url:                  t.URL,
		Status:               int32(t.Status),
		PendingResourceId:    t.PendingResourceID,
		Continuable:          t.Continuable,
		PluginPublicId:       t.PluginPublicID,
		PluginContributionId: t.PluginContributionID,
		PluginData:           t.PluginData,
		ErrorMessage:         t.ErrorMessage,
	}
}

func taskResParamToProto(p *pluginsdk.TaskResParam) *gen.TaskResParam {
	if p == nil {
		return nil
	}
	return &gen.TaskResParam{
		Task:         taskToProto(p.Task),
		ResourceId:   p.ResourceID,
		ResourcePath: p.ResourcePath,
	}
}

func protoToTaskCreateResponse(r *gen.TaskCreateResponse) *pluginsdk.TaskCreateResponse {
	children := make([]*pluginsdk.TaskCreateChildResponse, len(r.Children))
	for j, c := range r.Children {
		children[j] = &pluginsdk.TaskCreateChildResponse{
			TaskName:   c.TaskName,
			SiteWorkID: c.SiteWorkId,
			URL:        c.Url,
			PluginData: c.PluginData,
			SiteName:   c.SiteName,
		}
	}
	return &pluginsdk.TaskCreateResponse{
		PluginTaskID: r.PluginTaskId,
		TaskName:     r.TaskName,
		SiteWorkID:   r.SiteWorkId,
		URL:          r.Url,
		PluginData:   r.PluginData,
		SiteName:     r.SiteName,
		Children:     children,
	}
}

func protoToWorkResponse(pb *gen.WorkResponse) *pluginsdk.WorkResponse {
	if pb == nil {
		return nil
	}
	resp := &pluginsdk.WorkResponse{}
	if pb.Work != nil {
		resp.Work = &pluginsdk.Work{
			ID:                   pb.Work.Id,
			CreateTime:           pb.Work.CreateTime,
			UpdateTime:           pb.Work.UpdateTime,
			SiteID:               pb.Work.SiteId,
			SiteWorkID:           pb.Work.SiteWorkId,
			SiteWorkName:         pb.Work.SiteWorkName,
			SiteAuthorID:         pb.Work.SiteAuthorId,
			SiteWorkDescription:  pb.Work.SiteWorkDescription,
			SiteUploadTime:       pb.Work.SiteUploadTime,
			SiteUpdateTime:       pb.Work.SiteUpdateTime,
			NickName:             pb.Work.NickName,
			LocalAuthorID:        pb.Work.LocalAuthorId,
			LastView:             pb.Work.LastView,
		}
	}
	if pb.Site != nil {
		resp.Site = &pluginsdk.SiteDTO{
			ID:              pb.Site.Id,
			SiteName:        pb.Site.SiteName,
			SiteDescription: pb.Site.SiteDescription,
			Homepage:        pb.Site.Homepage,
			CreateTime:      pb.Site.CreateTime,
			UpdateTime:      pb.Site.UpdateTime,
		}
	}
	for _, a := range pb.LocalAuthors {
		resp.LocalAuthors = append(resp.LocalAuthors, &pluginsdk.LocalAuthorDTO{
			ID:         a.Id,
			AuthorName: a.AuthorName,
			Introduce:  a.Introduce,
			LastUse:    a.LastUse,
			CreateTime: a.CreateTime,
			UpdateTime: a.UpdateTime,
		})
	}
	for _, t := range pb.LocalTags {
		resp.LocalTags = append(resp.LocalTags, &pluginsdk.LocalTagDTO{
			ID:             t.Id,
			LocalTagName:   t.LocalTagName,
			BaseLocalTagID: t.BaseLocalTagId,
			Description:    t.Description,
			LastUse:        t.LastUse,
			CreateTime:     t.CreateTime,
			UpdateTime:     t.UpdateTime,
		})
	}
	for _, a := range pb.SiteAuthors {
		resp.SiteAuthors = append(resp.SiteAuthors, &pluginsdk.TaskSiteAuthorDTO{
			SiteAuthorID:    a.SiteAuthorId,
			AuthorName:      a.AuthorName,
			Homepage:        a.Homepage,
			FixedAuthorName: a.FixedAuthorName,
			Introduce:       a.Introduce,
		})
	}
	for _, t := range pb.SiteTags {
		resp.SiteTags = append(resp.SiteTags, &pluginsdk.TaskSiteTagDTO{
			SiteTagID:   t.SiteTagId,
			TagName:     t.TagName,
			Description: t.Description,
		})
	}
	for _, ws := range pb.WorkSets {
		resp.WorkSets = append(resp.WorkSets, &pluginsdk.TaskWorkSetDTO{
			SiteWorkSetID: ws.SiteWorkSetId,
			WorkSetName:   ws.WorkSetName,
		})
	}
	if pb.Resource != nil {
		resp.Resource = &pluginsdk.TaskResourceDTO{
			ResourceID:   pb.Resource.ResourceId,
			URL:          pb.Resource.Url,
			Type:         pb.Resource.Type,
			Format:       pb.Resource.Format,
			LocalPath:    pb.Resource.LocalPath,
			RemotePath:   pb.Resource.RemotePath,
			Size:         pb.Resource.Size,
			Completeness: int(pb.Resource.Completeness),
		}
	}
	return resp
}

// ========== gRPC Stream Reader ==========

// grpcStreamReader 从 gRPC server streaming 读取 StreamChunk，实现 io.ReadCloser
type grpcStreamReader struct {
	stream grpc.ServerStreamingClient[gen.StreamChunk]
	buf    []byte
	bufOff int
	closed bool
	readMu sync.Mutex
}

func (r *grpcStreamReader) Read(p []byte) (int, error) {
	r.readMu.Lock()
	defer r.readMu.Unlock()

	if r.closed {
		return 0, io.EOF
	}

	// 缓冲区有剩余数据
	if r.bufOff < len(r.buf) {
		n := copy(p, r.buf[r.bufOff:])
		r.bufOff += n
		return n, nil
	}

	// 从 gRPC stream 读取下一个 chunk
	for {
		chunk, err := r.stream.Recv()
		if err == io.EOF {
			r.closed = true
			return 0, io.EOF
		}
		if err != nil {
			st, ok := status.FromError(err)
			if ok && st.Code() == codes.Canceled {
				r.closed = true
				return 0, io.EOF
			}
			return 0, err
		}

		switch payload := chunk.Payload.(type) {
		case *gen.StreamChunk_Data:
			n := copy(p, payload.Data)
			if n < len(payload.Data) {
				r.buf = payload.Data
				r.bufOff = n
			}
			return n, nil
		case *gen.StreamChunk_Eof:
			r.closed = true
			return 0, io.EOF
		case *gen.StreamChunk_Error:
			r.closed = true
			return 0, fmt.Errorf("plugin stream error: %s", payload.Error)
		case *gen.StreamChunk_WorkResponse:
			// Start 的首个 chunk（WorkResponse）在 Start() 中已处理，后续不应再出现
			continue
		}
	}
}

func (r *grpcStreamReader) Close() error {
	r.readMu.Lock()
	defer r.readMu.Unlock()
	r.closed = true
	return nil
}
