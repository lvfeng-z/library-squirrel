package extension

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/lvfeng-z/library-squirrel-sdk/gen"

	pluginsdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
	pluginsdkliveness "github.com/lvfeng-z/library-squirrel-sdk/liveness"
)

// TaskHandlerProxy 通过 gRPC 代理到子进程的 TaskHandler
type TaskHandlerProxy struct {
	serviceAccessor ServiceAccessor
	pluginPublicId  string
	extensionId     string
}

var _ pluginsdkdto.TaskHandler = (*TaskHandlerProxy)(nil)

func (p *TaskHandlerProxy) getTaskClient() (gen.TaskHandlerServiceClient, error) {
	services, ok := p.serviceAccessor.GetServices(p.pluginPublicId)
	if !ok {
		return nil, fmt.Errorf("plugin %s not found", p.pluginPublicId)
	}
	return services.Task, nil
}

func (p *TaskHandlerProxy) Create(url string) (*pluginsdkdto.TaskCreateResult, error) {
	client, err := p.getTaskClient()
	if err != nil {
		return nil, err
	}
	stream, err := client.Create(context.Background(), &gen.CreateRequest{
		Url:         url,
		ExtensionId: p.extensionId,
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
		ch := make(chan *pluginsdkdto.TaskCreateResponse, 16)
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
		return pluginsdkdto.StreamResult(ch), nil
	}

	// 批量模式：收集所有 task
	var responses []*pluginsdkdto.TaskCreateResponse
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
	return pluginsdkdto.BatchResult(responses), nil
}

func (p *TaskHandlerProxy) CreateWorkInfo(task *pluginsdkdto.TaskDTO) (*pluginsdkdto.WorkResponse, error) {
	client, err := p.getTaskClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), pluginsdkliveness.UnaryRPCTimeout)
	defer cancel()
	resp, err := client.CreateWorkInfo(ctx, &gen.CreateWorkInfoRequest{
		Task:        taskToProto(task),
		ExtensionId: p.extensionId,
	})
	if err != nil {
		return nil, err
	}
	return protoToWorkResponse(resp), nil
}

// Start 开始任务:bidi 流,首帧 StartRequest,之后主程序按需 PullRequest 拉取。
// reader.Read 由主程序 copyLoop 驱动,reader 不领先主程序落盘
func (p *TaskHandlerProxy) Start(ctx context.Context, task *pluginsdkdto.TaskDTO, storeRoles []string) ([]*pluginsdkdto.StoreSpec, *pluginsdkdto.WorkResponse, error) {
	client, err := p.getTaskClient()
	if err != nil {
		return nil, nil, err
	}
	// stream ctx 继承任务 ctx:任务取消时经 gRPC stream 传播到插件,
	// 既中断主程序 pullReadCloser.Read 的 Recv 阻塞,又触发插件 serveSpecsPull 的 ctx Done(从而 Close reader)
	streamCtx, cancel := context.WithCancel(ctx)
	stream, err := client.Start(streamCtx)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	if err := stream.Send(&gen.StartFrame{Frame: &gen.StartFrame_Start{Start: &gen.StartRequest{
		Task:        taskToProto(task),
		ExtensionId: p.extensionId,
		StoreRoles:  storeRoles,
	}}}); err != nil {
		cancel()
		return nil, nil, err
	}
	return recvSpecsAndPull(
		func(role string, maxBytes int) error {
			return stream.Send(&gen.StartFrame{Frame: &gen.StartFrame_Pull{Pull: &gen.PullRequest{Role: role, MaxBytes: int32(maxBytes)}}})
		},
		stream.Recv,
		cancel,
	)
}

func (p *TaskHandlerProxy) Retry(task *pluginsdkdto.TaskDTO) (*pluginsdkdto.WorkResponse, error) {
	client, err := p.getTaskClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), pluginsdkliveness.UnaryRPCTimeout)
	defer cancel()
	resp, err := client.Retry(ctx, &gen.RetryRequest{
		Task:        taskToProto(task),
		ExtensionId: p.extensionId,
	})
	if err != nil {
		return nil, err
	}
	return protoToWorkResponse(resp), nil
}

func (p *TaskHandlerProxy) Pause(param *pluginsdkdto.TaskResParam) error {
	client, err := p.getTaskClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), pluginsdkliveness.UnaryRPCTimeout)
	defer cancel()
	_, err = client.Pause(ctx, &gen.TaskResParamMessage{
		Param:       taskResParamToProto(param),
		ExtensionId: p.extensionId,
	})
	return err
}

func (p *TaskHandlerProxy) Stop(param *pluginsdkdto.TaskResParam) error {
	client, err := p.getTaskClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), pluginsdkliveness.UnaryRPCTimeout)
	defer cancel()
	_, err = client.Stop(ctx, &gen.TaskResParamMessage{
		Param:       taskResParamToProto(param),
		ExtensionId: p.extensionId,
	})
	return err
}

// Resume 恢复任务:bidi 流,首帧 TaskResumeParamMessage,之后按需 PullRequest
func (p *TaskHandlerProxy) Resume(ctx context.Context, param *pluginsdkdto.TaskResumeParam) ([]*pluginsdkdto.StoreSpec, *pluginsdkdto.WorkResponse, error) {
	client, err := p.getTaskClient()
	if err != nil {
		return nil, nil, err
	}
	// stream ctx 继承任务 ctx,语义同 Start
	streamCtx, cancel := context.WithCancel(ctx)
	stream, err := client.Resume(streamCtx)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	if err := stream.Send(&gen.ResumeFrame{Frame: &gen.ResumeFrame_Resume{Resume: &gen.TaskResumeParamMessage{
		Param:       taskResumeParamToProto(param),
		ExtensionId: p.extensionId,
	}}}); err != nil {
		cancel()
		return nil, nil, err
	}
	return recvSpecsAndPull(
		func(role string, maxBytes int) error {
			return stream.Send(&gen.ResumeFrame{Frame: &gen.ResumeFrame_Pull{Pull: &gen.PullRequest{Role: role, MaxBytes: int32(maxBytes)}}})
		},
		stream.Recv,
		cancel,
	)
}

// SiteBrowserProxy 通过 gRPC 代理到子进程的 SiteBrowser
type SiteBrowserProxy struct {
	serviceAccessor ServiceAccessor
	pluginPublicId  string
	extensionId     string
}

var _ pluginsdkdto.SiteBrowser = (*SiteBrowserProxy)(nil)

func (p *SiteBrowserProxy) Open() error {
	services, ok := p.serviceAccessor.GetServices(p.pluginPublicId)
	if !ok {
		return fmt.Errorf("plugin %s not found", p.pluginPublicId)
	}
	_, err := services.Browser.Open(context.Background(), &gen.BrowserRequest{
		ExtensionId: p.extensionId,
	})
	return err
}

func (p *SiteBrowserProxy) Close() error {
	services, ok := p.serviceAccessor.GetServices(p.pluginPublicId)
	if !ok {
		return fmt.Errorf("plugin %s not found", p.pluginPublicId)
	}
	_, err := services.Browser.Close(context.Background(), &gen.BrowserRequest{
		ExtensionId: p.extensionId,
	})
	return err
}

// ========== 多流按需拉取(pull)==========

// recvSpecsAndPull 接收 WorkResponse(可选)+ Specs 声明,为每个 role 建 pullReadCloser(共享 bidi stream)。
// 主程序按需 Read 驱动插件 reader.Read,reader 不领先主程序落盘
func recvSpecsAndPull(
	sendPull func(role string, maxBytes int) error,
	recvChunk func() (*gen.StreamChunk, error),
	cancel context.CancelFunc,
) ([]*pluginsdkdto.StoreSpec, *pluginsdkdto.WorkResponse, error) {
	var workResp *pluginsdkdto.WorkResponse
	var metas []*gen.StoreSpecMeta
	for {
		chunk, err := recvChunk()
		if err != nil {
			cancel()
			return nil, nil, err
		}
		switch payload := chunk.Payload.(type) {
		case *gen.StreamChunk_WorkResponse:
			workResp = protoToWorkResponse(payload.WorkResponse)
			continue
		case *gen.StreamChunk_Specs:
			metas = payload.Specs.GetItems()
		default:
			cancel()
			return nil, nil, fmt.Errorf("期望 WorkResponse/Specs 块,收到 %T", chunk.Payload)
		}
		if metas != nil {
			break
		}
	}

	session := &pullSession{
		sendPull:  sendPull,
		recvChunk: recvChunk,
		cancel:    cancel,
		refCount:  len(metas),
	}
	specs := make([]*pluginsdkdto.StoreSpec, 0, len(metas))
	for _, meta := range metas {
		specs = append(specs, &pluginsdkdto.StoreSpec{
			Role:              meta.GetRole(),
			Generation:        meta.GetGeneration(),
			ReadCloser:        &pullReadCloser{session: session, role: meta.GetRole()},
			Format:            meta.GetFormat(),
			Size:              meta.GetSize(),
			SuggestName:       meta.GetSuggestName(),
			Continuable:       meta.Continuable,
			ResumeWriteOffset: meta.ResumeWriteOffset,
		})
	}
	return specs, workResp, nil
}

// pullSession 共享一条 bidi stream 的多 role pull 会话
type pullSession struct {
	sendPull  func(role string, maxBytes int) error
	recvChunk func() (*gen.StreamChunk, error)
	cancel    context.CancelFunc
	mu        sync.Mutex // 串行化 Send(Pull)+Recv 配对,保证请求/响应配对
	refCount  int        // 剩余未 Close 的 role
	closed    bool
}

// pullReadCloser 单 role 的按需读取:Read 一次 = 发一次 PullRequest + 收一次响应
type pullReadCloser struct {
	session *pullSession
	role    string
	eof     bool
}

func (r *pullReadCloser) Read(p []byte) (int, error) {
	if r.eof {
		return 0, io.EOF
	}
	r.session.mu.Lock()
	defer r.session.mu.Unlock()

	if err := r.session.sendPull(r.role, len(p)); err != nil {
		return 0, err
	}
	chunk, err := r.session.recvChunk()
	if err != nil {
		return 0, err
	}
	switch payload := chunk.Payload.(type) {
	case *gen.StreamChunk_Data:
		if chunk.GetRole() != r.role {
			return 0, fmt.Errorf("pull 响应 role 不匹配: 期望 %s, 收到 %s", r.role, chunk.GetRole())
		}
		return copy(p, payload.Data), nil
	case *gen.StreamChunk_Eof:
		r.eof = true
		return 0, io.EOF
	case *gen.StreamChunk_Error:
		return 0, fmt.Errorf("插件流错误: %s", payload.Error)
	default:
		return 0, fmt.Errorf("意外的 pull 响应: %T", chunk.Payload)
	}
}

func (r *pullReadCloser) Close() error {
	r.eof = true
	r.session.mu.Lock()
	defer r.session.mu.Unlock()
	r.session.refCount--
	if r.session.refCount <= 0 && !r.session.closed {
		r.session.closed = true
		r.session.cancel() // 全部 role 关闭,cancel context 关闭 bidi stream
	}
	return nil
}

// ========== Proto 转换函数（proxy 专用）==========

func taskToProto(t *pluginsdkdto.TaskDTO) *gen.Task {
	if t == nil {
		return nil
	}
	return &gen.Task{
		Id:                t.ID,
		CreateTime:        t.CreateTime,
		UpdateTime:        t.UpdateTime,
		HasChild:          t.HasChild,
		Pid:               t.Pid,
		TaskName:          t.TaskName,
		SiteId:            t.SiteID,
		SiteWorkId:        t.SiteWorkID,
		Url:               t.URL,
		Status:            int32(t.Status),
		PendingResourceId: t.PendingResourceID,
		Continuable:       t.Continuable,
		PluginPublicId:    t.PluginPublicID,
		PluginExtensionId: t.PluginExtensionID,
		PluginData:        t.PluginData,
		ErrorMessage:      t.ErrorMessage,
	}
}

func taskResParamToProto(p *pluginsdkdto.TaskResParam) *gen.TaskResParam {
	if p == nil {
		return nil
	}
	return &gen.TaskResParam{
		Task:            taskToProto(p.Task),
		ResourceId:      p.ResourceID,
		ResourcePath:    p.ResourcePath,
		DownloadedBytes: p.DownloadedBytes,
	}
}

func taskResumeParamToProto(p *pluginsdkdto.TaskResumeParam) *gen.TaskResumeParam {
	if p == nil {
		return nil
	}
	return &gen.TaskResumeParam{
		Task:          taskToProto(p.Task),
		StreamOffsets: p.StreamOffsets,
	}
}

func protoToTaskCreateResponse(r *gen.TaskCreateResponse) *pluginsdkdto.TaskCreateResponse {
	children := make([]*pluginsdkdto.TaskCreateChildResponse, len(r.Children))
	for j, c := range r.Children {
		children[j] = &pluginsdkdto.TaskCreateChildResponse{
			TaskName:      c.TaskName,
			SiteWorkID:    c.SiteWorkId,
			URL:           c.Url,
			PluginData:    c.PluginData,
			SiteName:      c.SiteName,
			InvolvedRoles: c.InvolvedRoles,
			ResourceType:  c.ResourceType,
		}
	}
	return &pluginsdkdto.TaskCreateResponse{
		PluginTaskID:  r.PluginTaskId,
		TaskName:      r.TaskName,
		SiteWorkID:    r.SiteWorkId,
		URL:           r.Url,
		PluginData:    r.PluginData,
		SiteName:      r.SiteName,
		InvolvedRoles: r.InvolvedRoles,
		ResourceType:  r.ResourceType,
		Children:      children,
	}
}

func protoToWorkResponse(pb *gen.WorkResponse) *pluginsdkdto.WorkResponse {
	if pb == nil {
		return nil
	}
	resp := &pluginsdkdto.WorkResponse{}
	if pb.Work != nil {
		resp.Work = &pluginsdkdto.WorkDTO{
			ID:                  pb.Work.Id,
			CreateTime:          pb.Work.CreateTime,
			UpdateTime:          pb.Work.UpdateTime,
			SiteID:              pb.Work.SiteId,
			SiteWorkID:          pb.Work.SiteWorkId,
			SiteWorkName:        pb.Work.SiteWorkName,
			SiteAuthorID:        pb.Work.SiteAuthorId,
			SiteWorkDescription: pb.Work.SiteWorkDescription,
			SiteUploadTime:      pb.Work.SiteUploadTime,
			SiteUpdateTime:      pb.Work.SiteUpdateTime,
			NickName:            pb.Work.NickName,
			LocalAuthorID:       pb.Work.LocalAuthorId,
			LastView:            pb.Work.LastView,
		}
	}
	if pb.Site != nil {
		resp.Site = &pluginsdkdto.SiteDTO{
			ID:              pb.Site.Id,
			SiteName:        pb.Site.SiteName,
			SiteDescription: pb.Site.SiteDescription,
			Homepage:        pb.Site.Homepage,
			CreateTime:      pb.Site.CreateTime,
			UpdateTime:      pb.Site.UpdateTime,
		}
	}
	for _, a := range pb.LocalAuthors {
		resp.LocalAuthors = append(resp.LocalAuthors, &pluginsdkdto.LocalAuthorDTO{
			ID:         a.Id,
			AuthorName: a.AuthorName,
			Introduce:  a.Introduce,
			LastUse:    a.LastUse,
			CreateTime: a.CreateTime,
			UpdateTime: a.UpdateTime,
		})
	}
	for _, t := range pb.LocalTags {
		resp.LocalTags = append(resp.LocalTags, &pluginsdkdto.LocalTagDTO{
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
		resp.SiteAuthors = append(resp.SiteAuthors, &pluginsdkdto.TaskSiteAuthorDTO{
			SiteAuthorID:    a.SiteAuthorId,
			AuthorName:      a.AuthorName,
			Homepage:        a.Homepage,
			FixedAuthorName: a.FixedAuthorName,
			Introduce:       a.Introduce,
		})
	}
	for _, t := range pb.SiteTags {
		resp.SiteTags = append(resp.SiteTags, &pluginsdkdto.TaskSiteTagDTO{
			SiteTagID:   t.SiteTagId,
			TagName:     t.TagName,
			Description: t.Description,
		})
	}
	for _, ws := range pb.WorkSets {
		resp.WorkSets = append(resp.WorkSets, &pluginsdkdto.TaskWorkSetDTO{
			SiteWorkSetID: ws.SiteWorkSetId,
			WorkSetName:   ws.WorkSetName,
		})
	}
	return resp
}
