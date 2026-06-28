package extension

import (
	"context"
	"fmt"
	"io"

	"github.com/lvfeng-z/library-squirrel-sdk/gen"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pluginsdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
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
	resp, err := client.CreateWorkInfo(context.Background(), &gen.CreateWorkInfoRequest{
		Task:        taskToProto(task),
		ExtensionId: p.extensionId,
	})
	if err != nil {
		return nil, err
	}
	return protoToWorkResponse(resp), nil
}

// Start 开始任务:按 storeRoles 选择性产出,读取 WorkResponse(可选)+ Specs(声明所选轨道元数据),
// 为每个 role 建 io.Pipe,后台 goroutine 按 role 分发 data/eof/error,立即返回 StoreSpec 集合
func (p *TaskHandlerProxy) Start(task *pluginsdkdto.TaskDTO, storeRoles []string) ([]*pluginsdkdto.StoreSpec, *pluginsdkdto.WorkResponse, error) {
	client, err := p.getTaskClient()
	if err != nil {
		return nil, nil, err
	}
	stream, err := client.Start(context.Background(), &gen.StartRequest{
		Task:        taskToProto(task),
		ExtensionId: p.extensionId,
		StoreRoles:  storeRoles,
	})
	if err != nil {
		return nil, nil, err
	}
	return recvSpecsAndDemux(stream)
}

func (p *TaskHandlerProxy) Retry(task *pluginsdkdto.TaskDTO) (*pluginsdkdto.WorkResponse, error) {
	client, err := p.getTaskClient()
	if err != nil {
		return nil, err
	}
	resp, err := client.Retry(context.Background(), &gen.RetryRequest{
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
	_, err = client.Pause(context.Background(), &gen.TaskResParamMessage{
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
	_, err = client.Stop(context.Background(), &gen.TaskResParamMessage{
		Param:       taskResParamToProto(param),
		ExtensionId: p.extensionId,
	})
	return err
}

// Resume 恢复任务:按 TaskResumeParam.StreamOffsets 续传,返回新的 StoreSpec 集合
func (p *TaskHandlerProxy) Resume(param *pluginsdkdto.TaskResumeParam) ([]*pluginsdkdto.StoreSpec, *pluginsdkdto.WorkResponse, error) {
	client, err := p.getTaskClient()
	if err != nil {
		return nil, nil, err
	}
	stream, err := client.Resume(context.Background(), &gen.TaskResumeParamMessage{
		Param:       taskResumeParamToProto(param),
		ExtensionId: p.extensionId,
	})
	if err != nil {
		return nil, nil, err
	}
	return recvSpecsAndDemux(stream)
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

// ========== 多流解复用 ==========

// recvSpecsAndDemux 读取 Start/Resume 流:先收 WorkResponse(可选)+ Specs(声明全部 role 元数据),
// 随后为每个 role 建 io.Pipe,后台 goroutine 持续 Recv 并按 role 分发 data/eof/error。
// 立即返回 []*StoreSpec(每个含 pipe reader + 元数据);调用方读 reader 时后台持续喂数据。
func recvSpecsAndDemux(stream grpc.ServerStreamingClient[gen.StreamChunk]) ([]*pluginsdkdto.StoreSpec, *pluginsdkdto.WorkResponse, error) {
	var workResp *pluginsdkdto.WorkResponse
	var metas []*gen.StoreSpecMeta
	for {
		chunk, err := stream.Recv()
		if err != nil {
			return nil, nil, err
		}
		switch payload := chunk.Payload.(type) {
		case *gen.StreamChunk_WorkResponse:
			workResp = protoToWorkResponse(payload.WorkResponse)
			continue
		case *gen.StreamChunk_Specs:
			metas = payload.Specs.GetItems()
		default:
			return nil, nil, fmt.Errorf("期望 WorkResponse/Specs 块,收到 %T", chunk.Payload)
		}
		if metas != nil {
			break
		}
	}

	pipes := make(map[string]*io.PipeWriter, len(metas))
	specs := make([]*pluginsdkdto.StoreSpec, 0, len(metas))
	for _, meta := range metas {
		pr, pw := io.Pipe()
		pipes[meta.GetRole()] = pw
		specs = append(specs, &pluginsdkdto.StoreSpec{
			Role:        meta.GetRole(),
			Generation:  meta.GetGeneration(),
			ReadCloser:  pr,
			Format:      meta.GetFormat(),
			Size:        meta.GetSize(),
			SuggestName: meta.GetSuggestName(),
			Continuable: meta.Continuable,               // *bool(oneof optional)
			ResumeWriteOffset: meta.ResumeWriteOffset,     // *int64(oneof optional)
		})
	}

	go demuxStream(stream, pipes)
	return specs, workResp, nil
}

// demuxStream 后台解复用 gRPC 流:按 role 把 data 写入对应 pipe,eof/error 关闭对应 pipe
// data 写入在消费方停止读取(reader 关闭)时返回 ErrClosedPipe,标记该 role 已废后续丢弃
func demuxStream(stream grpc.ServerStreamingClient[gen.StreamChunk], pipes map[string]*io.PipeWriter) {
	closed := make(map[string]struct{})
	for {
		chunk, err := stream.Recv()
		if err != nil {
			closeAllPipes(pipes, closed, normalizeStreamErr(err))
			return
		}
		role := chunk.GetRole()
		if _, ok := closed[role]; ok {
			continue
		}
		switch payload := chunk.Payload.(type) {
		case *gen.StreamChunk_Data:
			pw, ok := pipes[role]
			if !ok {
				continue
			}
			if _, werr := pw.Write(payload.Data); werr != nil {
				// 消费方已关闭 reader,标记已废,后续数据丢弃
				closed[role] = struct{}{}
			}
		case *gen.StreamChunk_Eof:
			if pw, ok := pipes[role]; ok {
				pw.Close()
				closed[role] = struct{}{}
			}
		case *gen.StreamChunk_Error:
			if pw, ok := pipes[role]; ok {
				pw.CloseWithError(fmt.Errorf("插件流错误: %s", payload.Error))
				closed[role] = struct{}{}
			}
		case *gen.StreamChunk_Specs, *gen.StreamChunk_WorkResponse:
			// Specs/WorkResponse 已在 recvSpecsAndDemux 处理,忽略重复
		}
	}
}

// normalizeStreamErr 流结束或被取消时视作正常关闭(nil);其余错误原样返回
func normalizeStreamErr(err error) error {
	if err == nil || err == io.EOF {
		return nil
	}
	if st, ok := status.FromError(err); ok && st.Code() == codes.Canceled {
		return nil
	}
	return err
}

// closeAllPipes 关闭所有未关闭的 pipe;err 非空时以错误关闭(传播给消费方 reader)
func closeAllPipes(pipes map[string]*io.PipeWriter, closed map[string]struct{}, err error) {
	for role, pw := range pipes {
		if _, ok := closed[role]; ok {
			continue
		}
		if err != nil {
			pw.CloseWithError(err)
		} else {
			pw.Close()
		}
		closed[role] = struct{}{}
	}
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
			TaskName:   c.TaskName,
			SiteWorkID: c.SiteWorkId,
			URL:        c.Url,
			PluginData: c.PluginData,
			SiteName:   c.SiteName,
		}
	}
	return &pluginsdkdto.TaskCreateResponse{
		PluginTaskID: r.PluginTaskId,
		TaskName:     r.TaskName,
		SiteWorkID:   r.SiteWorkId,
		URL:          r.Url,
		PluginData:   r.PluginData,
		SiteName:     r.SiteName,
		Children:     children,
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
