package extension

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/lvfeng-z/library-squirrel-sdk/gen"

	pluginsdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
	pluginsdkliveness "github.com/lvfeng-z/library-squirrel-sdk/liveness"
	transport "github.com/lvfeng-z/library-squirrel-sdk/transport"
)

// preFirstItemIdleWindowFactor 流式 Create 泵在首个实质块（task 块/子行批次块）到达前的
// 接收空闲窗放宽倍数：首个批次块要等插件完成首个域的核对（哈希）后才发射，该相位插件不发
// 心跳块、60s 基窗无法靠心跳维持，冷库/USB 桥冷启下首域核对实测可超 60s（2026-10-31 复测
// B1 两轮被 60s 基窗误杀收流）；首个实质块到达后回落基窗——批次间隙重新受 60s 约束，挂死
// 检测仅在该前置相位让渡（5×窗仍为有界上限，远优于无检测）
const preFirstItemIdleWindowFactor = 5

// WorkFetchProxy 通过 gRPC 代理到子进程的 WorkFetcher
type WorkFetchProxy struct {
	serviceAccessor ServiceAccessor
	pluginPublicId  string
	extensionId     string
	// readerIdleTimeout 流式链路单次接收等待数据的空闲上限（Create 首块/Start·Resume 首响应/pull
	// 响应共用），取 SDK 与插件双端一致的权威常量；测试可构造后覆写注入缩时值
	readerIdleTimeout time.Duration
}

var _ pluginsdkdto.WorkFetcher = (*WorkFetchProxy)(nil)

// NewWorkFetchProxy 构造作品拉取代理（导出面：宿主装配链与跨包集成测试共用）；
// 流式接收空闲超时取 SDK 权威常量
func NewWorkFetchProxy(serviceAccessor ServiceAccessor, pluginPublicId, extensionId string) *WorkFetchProxy {
	return &WorkFetchProxy{
		serviceAccessor:   serviceAccessor,
		pluginPublicId:    pluginPublicId,
		extensionId:       extensionId,
		readerIdleTimeout: pluginsdkliveness.ReaderIdleTimeout,
	}
}

func (p *WorkFetchProxy) getWorkFetchClient() (gen.WorkFetchServiceClient, error) {
	services, ok := p.serviceAccessor.GetServices(p.pluginPublicId)
	if !ok {
		return nil, fmt.Errorf("plugin %s not found", p.pluginPublicId)
	}
	return services.WorkFetch, nil
}

// Create 创建任务。SDK WorkFetcher 接口未携带 ctx（跨仓契约），以 Background 为基；
// 主程序内部调用方经 CreateWithContext 承接调用方取消语义
func (p *WorkFetchProxy) Create(url string) (*pluginsdkdto.TaskCreateResult, error) {
	return p.CreateWithContext(context.Background(), url)
}

// CreateWithContext 以调用方 ctx 为基创建任务流：ctx 取消即终结流与接收泵。
// 全程接收受空闲超时约束——首块与首块之后的泵循环单次等待共用同一空闲窗，插件建流后不产出
// 或发射中途数据停发（hang）表现为超时错误而非无限阻塞；心跳块豁免（到达即重置空闲窗口，
// 周期心跳可维持等待越过单个窗口，见 recvWithIdleTimeout）。
// 流式模式的流收尾责任归接收泵（EOF/错误/ctx 取消任一退出，退出时取消流 ctx 并关闭结果 channel）；
// 批量模式与错误路径在本函数内就地收尾
func (p *WorkFetchProxy) CreateWithContext(ctx context.Context, url string) (*pluginsdkdto.TaskCreateResult, error) {
	client, err := p.getWorkFetchClient()
	if err != nil {
		return nil, err
	}
	streamCtx, cancel := context.WithCancel(ctx)
	stream, err := client.Create(streamCtx, &gen.CreateRequest{
		Url:         url,
		ExtensionId: p.extensionId,
	})
	if err != nil {
		cancel()
		return nil, err
	}

	// 读取首条实质性消息：正常流首块为 mode 块；插件 Create 错误返回时首块（也是唯一块）为 error 块，
	// 无 mode 块。心跳块（插件 Create handler 执行期等待保活，如等用户输入）可先于 mode/error 到达，
	// 跳过后继续等待——每次接收的空闲窗口独立起算，心跳到达即重置窗口，周期心跳可维持等待越过
	// 单个窗口；心跳停止后连接静默照常按窗超时（hang 检测保留）
	var chunk *gen.CreateChunk
	for {
		chunk, err = recvWithIdleTimeout(stream.Recv, cancel, p.readerIdleTimeout)
		if err != nil {
			cancel()
			return nil, err
		}
		if chunk.GetHeartbeat() == nil {
			break
		}
	}

	// error 块承载插件业务失败原因（用户可读文本），必为流的最后一块
	if reason := chunk.GetError(); reason != "" {
		cancel()
		result := pluginsdkdto.BatchResult(nil)
		result.SetReason(reason)
		return result, nil
	}

	// intent 块（转入解析/已取消）承载零任务的成功收尾声明，同为流的最后一块；作为首块
	// 到达时与 error 块同形处理：零任务批量结果携带意图
	if intent := chunk.GetIntent(); intent != nil {
		kind, kindErr := createIntentKindFromProto(intent.GetKind())
		if kindErr != nil {
			cancel()
			return nil, kindErr
		}
		cancel()
		result := pluginsdkdto.BatchResult(nil)
		result.SetIntent(kind, intent.GetMessage())
		return result, nil
	}

	modeChunk := chunk.GetMode()
	if modeChunk == nil {
		cancel()
		return nil, fmt.Errorf("first CreateChunk must contain CreateMode, Error or Intent")
	}

	if modeChunk.IsStream {
		// 流式模式：条目形态通道，task 块与子行批次块（children_batch）按到达序透传；
		// 批次块的聚合裁决（按 plugin_task_id 指称挂父、last 收批、协议违规收流）归任务服务
		ch := make(chan *pluginsdkdto.CreateStreamItem, 16)
		result := pluginsdkdto.StreamItemsResult(ch)
		go func() {
			defer close(ch)
			// 泵退出即取消流 ctx：终结 gRPC stream（EOF/错误/ctx 取消三态统一收尾）
			defer cancel()
			// 首个实质块前的接收等待按 preFirstItemIdleWindowFactor 放宽（冷首域核对相位
			// 无心跳可喂窗，见常量注释）；首个实质块到达即回落基窗
			preFirstItemWindow := p.readerIdleTimeout * preFirstItemIdleWindowFactor
			firstItemSeen := false
			for {
				// 首块之后的泵循环同样受空闲超时约束：插件死亡/读挂起时连接静默、数据停发，
				// 窗口到期收流报错（cancel 流 ctx，终结阻塞中的接收与该流上后续读取）；
				// 心跳块豁免——每次接收的空闲窗口独立起算，心跳到达即重置窗口
				window := p.readerIdleTimeout
				if !firstItemSeen {
					window = preFirstItemWindow
				}
				c, err := recvWithIdleTimeout(stream.Recv, cancel, window)
				if err != nil {
					// 正常收尾（io.EOF）静默；异常收流（空闲超时/gRPC 层错误：插件死亡、通道断）
					// 把收流原因记进 reason——消费侧流结束后读到失败原因，建任务调用得以带因失败
					//（未收批父任务由任务服务统一回滚，决策4）。SetReason 与 close(ch) 在同一
					// goroutine 内顺序执行，消费侧 range 退出后读 Reason() 依赖此顺序
					if !errors.Is(err, io.EOF) {
						result.SetReason(err.Error())
					}
					return
				}
				// error 块为流最后一块：记录原因后结束接收。SetReason 与 close(ch)
				// 在同一 goroutine 内顺序执行，消费侧在 range 退出后读 Reason() 依赖此顺序
				if reason := c.GetError(); reason != "" {
					result.SetReason(reason)
					return
				}
				// intent 块（转入解析/已取消）同为流最后一块：记录零任务成功收尾意图后结束
				// 接收。SetIntent 与 close(ch) 同 goroutine 顺序执行，消费侧在 range 退出后
				// 读 Intent() 依赖此顺序；块 kind 非法（意图块只承载两态）时把违规记进
				// reason，经既有零任务失败路径透出
				if intent := c.GetIntent(); intent != nil {
					kind, kindErr := createIntentKindFromProto(intent.GetKind())
					if kindErr != nil {
						result.SetReason(kindErr.Error())
						return
					}
					result.SetIntent(kind, intent.GetMessage())
					return
				}
				taskProto := c.GetTask()
				if taskProto != nil {
					firstItemSeen = true
					// 消费方停止读取（channel 缓冲占满）时，流 ctx 取消可解除发送阻塞令泵退出
					select {
					case ch <- pluginsdkdto.TaskItem(protoToTaskCreateResponse(taskProto)):
					case <-streamCtx.Done():
						return
					}
				}
				// 子行批次块：大批次创建的分帧载荷，按块内声明（指称父 ID、子行清单、末批标志）透传
				batchProto := c.GetChildrenBatch()
				if batchProto != nil {
					firstItemSeen = true
					select {
					case ch <- pluginsdkdto.ChildrenBatchItem(
						batchProto.GetPluginTaskId(),
						protoToTaskCreateChildResponses(batchProto.GetItems()),
						batchProto.GetLast()):
					case <-streamCtx.Done():
						return
					}
				}
			}
		}()
		return result, nil
	}

	// 批量模式：收集所有 task 块，末尾收尾块（如有）承载插件声明的业务原因（error 块）或
	// 零任务成功收尾意图（intent 块）。子行批次块仅流式 mode 合法，批量流上出现即插件协议
	// 违规，按流错误收流。io.EOF 为正常收尾；其他 Recv 错误属 gRPC 层故障
	//（进程崩溃/连接中断），原样返回
	var responses []*pluginsdkdto.TaskCreateResponse
	var reason string
	var intentKind pluginsdkdto.CreateIntentKind
	var intentMessage string
	for {
		if r := chunk.GetError(); r != "" {
			reason = r
			break
		}
		if intent := chunk.GetIntent(); intent != nil {
			kind, kindErr := createIntentKindFromProto(intent.GetKind())
			if kindErr != nil {
				cancel()
				return nil, kindErr
			}
			intentKind = kind
			intentMessage = intent.GetMessage()
			break
		}
		if chunk.GetChildrenBatch() != nil {
			cancel()
			return nil, fmt.Errorf("批量 mode 的 Create 流不允许子行批次块（children_batch 仅流式 mode 合法）")
		}
		taskProto := chunk.GetTask()
		if taskProto != nil {
			responses = append(responses, protoToTaskCreateResponse(taskProto))
		}
		chunk, err = stream.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				cancel()
				return nil, err
			}
			break
		}
	}
	cancel()
	result := pluginsdkdto.BatchResult(responses)
	if intentKind != pluginsdkdto.CreateIntentNone {
		result.SetIntent(intentKind, intentMessage)
	} else if reason != "" {
		result.SetReason(reason)
	}
	return result, nil
}

func (p *WorkFetchProxy) CreateWorkInfo(task *pluginsdkdto.TaskDTO) (*pluginsdkdto.WorkResponse, error) {
	return p.CreateWorkInfoWithContext(context.Background(), task)
}

// CreateWorkInfoWithContext 以调用方 ctx 为基生成作品信息：调用方取消立即打断 gRPC 等待，
// UnaryRPCTimeout 仍作为插件 handler 卡死的超时上限
func (p *WorkFetchProxy) CreateWorkInfoWithContext(ctx context.Context, task *pluginsdkdto.TaskDTO) (*pluginsdkdto.WorkResponse, error) {
	client, err := p.getWorkFetchClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, pluginsdkliveness.UnaryRPCTimeout)
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
func (p *WorkFetchProxy) Start(ctx context.Context, task *pluginsdkdto.TaskDTO, storeRoles []string) ([]*pluginsdkdto.StoreSpec, *pluginsdkdto.WorkResponse, error) {
	client, err := p.getWorkFetchClient()
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
		p.readerIdleTimeout,
	)
}

func (p *WorkFetchProxy) Retry(task *pluginsdkdto.TaskDTO) (*pluginsdkdto.WorkResponse, error) {
	client, err := p.getWorkFetchClient()
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

// QueryWorkSetOrder 查询作品集内作品的原站顺序（主程序作品入库后拉取，仅写 site_sort_order）
func (p *WorkFetchProxy) QueryWorkSetOrder(ctx context.Context, siteId int64, siteWorkSetId string) ([]*pluginsdkdto.WorkOrderEntry, error) {
	client, err := p.getWorkFetchClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, pluginsdkliveness.UnaryRPCTimeout)
	defer cancel()
	resp, err := client.QueryWorkSetOrder(ctx, &gen.QueryWorkSetOrderRequest{
		SiteId:        siteId,
		SiteWorkSetId: siteWorkSetId,
	})
	if err != nil {
		return nil, err
	}
	return workOrderEntriesFromProto(resp.GetEntries()), nil
}

// workOrderEntriesFromProto 原站排序条目 proto → DTO
func workOrderEntriesFromProto(entries []*gen.WorkOrderEntry) []*pluginsdkdto.WorkOrderEntry {
	result := make([]*pluginsdkdto.WorkOrderEntry, 0, len(entries))
	for _, e := range entries {
		result = append(result, &pluginsdkdto.WorkOrderEntry{
			SiteWorkId: e.GetSiteWorkId(),
			SortOrder:  e.GetSortOrder(),
		})
	}
	return result
}

// QueryWorkSetRelations 查询本作品集的父集关系 + 在各父集下的原站序（主程序作品入库后拉取，仅写 site_sort_order）
func (p *WorkFetchProxy) QueryWorkSetRelations(ctx context.Context, siteId int64, siteWorkSetId string) ([]*pluginsdkdto.WorkSetRelationEntry, error) {
	client, err := p.getWorkFetchClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, pluginsdkliveness.UnaryRPCTimeout)
	defer cancel()
	resp, err := client.QueryWorkSetRelations(ctx, &gen.QueryWorkSetRelationsRequest{
		SiteId:        siteId,
		SiteWorkSetId: siteWorkSetId,
	})
	if err != nil {
		return nil, err
	}
	return workSetRelationEntriesFromProto(resp.GetParents()), nil
}

// workSetRelationEntriesFromProto 作品集父集关系条目 proto → DTO
func workSetRelationEntriesFromProto(entries []*gen.WorkSetRelationEntry) []*pluginsdkdto.WorkSetRelationEntry {
	result := make([]*pluginsdkdto.WorkSetRelationEntry, 0, len(entries))
	for _, e := range entries {
		result = append(result, &pluginsdkdto.WorkSetRelationEntry{
			ParentSiteWorkSetId: e.GetParentSiteWorkSetId(),
			ParentWorkSetName:   e.GetParentWorkSetName(),
			SortOrder:           e.GetSortOrder(),
		})
	}
	return result
}

func (p *WorkFetchProxy) Pause(param *pluginsdkdto.TaskResParam) error {
	return p.PauseWithContext(context.Background(), param)
}

// PauseWithContext 以调用方 ctx 为基暂停任务：调用方取消立即打断 gRPC 等待，
// UnaryRPCTimeout 仍作为插件 handler 卡死的超时上限
func (p *WorkFetchProxy) PauseWithContext(ctx context.Context, param *pluginsdkdto.TaskResParam) error {
	client, err := p.getWorkFetchClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, pluginsdkliveness.UnaryRPCTimeout)
	defer cancel()
	_, err = client.Pause(ctx, &gen.TaskResParamMessage{
		Param:       taskResParamToProto(param),
		ExtensionId: p.extensionId,
	})
	return err
}

func (p *WorkFetchProxy) Stop(param *pluginsdkdto.TaskResParam) error {
	return p.StopWithContext(context.Background(), param)
}

// StopWithContext 以调用方 ctx 为基停止任务：调用方取消立即打断 gRPC 等待，
// UnaryRPCTimeout 仍作为插件 handler 卡死的超时上限
func (p *WorkFetchProxy) StopWithContext(ctx context.Context, param *pluginsdkdto.TaskResParam) error {
	client, err := p.getWorkFetchClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, pluginsdkliveness.UnaryRPCTimeout)
	defer cancel()
	_, err = client.Stop(ctx, &gen.TaskResParamMessage{
		Param:       taskResParamToProto(param),
		ExtensionId: p.extensionId,
	})
	return err
}

// Resume 恢复任务:bidi 流,首帧 TaskResumeParamMessage,之后按需 PullRequest
func (p *WorkFetchProxy) Resume(ctx context.Context, param *pluginsdkdto.TaskResumeParam) ([]*pluginsdkdto.StoreSpec, *pluginsdkdto.WorkResponse, error) {
	client, err := p.getWorkFetchClient()
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
		p.readerIdleTimeout,
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

// recvWithIdleTimeout 在空闲窗口内等待一次流接收：数据到达即返回（窗口按次起算，不在跨次
// 等待间累计），到期 cancel 所属流 ctx——阻塞中的接收与该流上的后续读取立即失败，插件 hang
// （连接活但不产出）表现为调用方收到超时错误而非无限等待。接收在独立 goroutine 执行，
// cancel 后其结果经缓冲通道送达即弃置
func recvWithIdleTimeout[T any](recv func() (T, error), cancel context.CancelFunc, idleTimeout time.Duration) (T, error) {
	var zero T
	type recvOutcome struct {
		value T
		err   error
	}
	outcomeCh := make(chan recvOutcome, 1)
	go func() {
		value, err := recv()
		outcomeCh <- recvOutcome{value, err}
	}()
	timer := time.NewTimer(idleTimeout)
	defer timer.Stop()
	select {
	case outcome := <-outcomeCh:
		return outcome.value, outcome.err
	case <-timer.C:
		cancel()
		return zero, fmt.Errorf("插件流空闲超时: %s 内未收到数据", idleTimeout)
	}
}

// recvSpecsAndPull 接收 WorkResponse(可选)+ Specs 声明,为每个 role 建 pullReadCloser(共享 bidi stream)。
// 主程序按需 Read 驱动插件 reader.Read,reader 不领先主程序落盘
func recvSpecsAndPull(
	sendPull func(role string, maxBytes int) error,
	recvChunk func() (*gen.StreamChunk, error),
	cancel context.CancelFunc,
	idleTimeout time.Duration,
) ([]*pluginsdkdto.StoreSpec, *pluginsdkdto.WorkResponse, error) {
	var workResp *pluginsdkdto.WorkResponse
	var metas []*gen.StoreSpecMeta
	for {
		// 首响应（WorkResponse 可选 + Specs 声明）的每次接收受空闲超时约束
		chunk, err := recvWithIdleTimeout(recvChunk, cancel, idleTimeout)
		if err != nil {
			cancel()
			return nil, nil, err
		}
		switch payload := chunk.Payload.(type) {
		case *gen.StreamChunk_WorkResponse:
			workResp = protoToWorkResponse(payload.WorkResponse)
			continue
		case *gen.StreamChunk_Heartbeat:
			// 心跳块（插件 Start/Resume handler 执行期等待保活）：忽略继续等首响应，
			// 每次接收的空闲窗口独立起算，心跳到达即重置窗口
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
		sendPull:    sendPull,
		recvChunk:   recvChunk,
		cancel:      cancel,
		refCount:    len(metas),
		idleTimeout: idleTimeout,
	}
	specs := make([]*pluginsdkdto.StoreSpec, 0, len(metas))
	for i, meta := range metas {
		specs = append(specs, &pluginsdkdto.StoreSpec{
			Role:              meta.GetRole(),
			Generation:        meta.GetGeneration(),
			ReadCloser:        &pullReadCloser{session: session, role: transport.EncodePullRole(meta.GetRole(), i)},
			Format:            meta.GetFormat(),
			Size:              meta.GetSize(),
			SuggestName:       meta.GetSuggestName(),
			Description:       meta.GetDescription(),
			Continuable:       meta.Continuable,
			ResumeWriteOffset: meta.ResumeWriteOffset,
			ExpectedSha256:    meta.ExpectedSha256,
		})
	}
	return specs, workResp, nil
}

// pullSession 共享一条 bidi stream 的多 role pull 会话
type pullSession struct {
	sendPull    func(role string, maxBytes int) error
	recvChunk   func() (*gen.StreamChunk, error)
	cancel      context.CancelFunc
	idleTimeout time.Duration // 单次 pull 响应等待的空闲上限
	mu          sync.Mutex    // 串行化 Send(Pull)+Recv 配对,保证请求/响应配对
	refCount    int           // 剩余未 Close 的 role
	closed      bool
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
	// 单次 pull 响应受空闲超时约束：窗口从本次等待起算，数据到达即返回；到期 cancel 会话流，
	// 阻塞中的接收与该流上全部后续读取立即失败（多 role 共享一条流，超时即整体失败）
	chunk, err := recvWithIdleTimeout(r.session.recvChunk, r.session.cancel, r.session.idleTimeout)
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

// createIntentKindFromProto 意图信号枚举 proto → DTO：意图块只承载两态（转入解析/已取消），
// UNSPECIFIED 与未知值为非法意图块，按协议错误拒绝
func createIntentKindFromProto(kind gen.CreateIntentKind) (pluginsdkdto.CreateIntentKind, error) {
	switch kind {
	case gen.CreateIntentKind_CREATE_INTENT_KIND_TO_PARSE:
		return pluginsdkdto.CreateIntentToParse, nil
	case gen.CreateIntentKind_CREATE_INTENT_KIND_CANCELLED:
		return pluginsdkdto.CreateIntentCancelled, nil
	default:
		return pluginsdkdto.CreateIntentNone, fmt.Errorf("意图块 kind 非法: %d", kind)
	}
}

func taskToProto(t *pluginsdkdto.TaskDTO) *gen.Task {
	if t == nil {
		return nil
	}
	return &gen.Task{
		Id:                t.Id,
		CreateTime:        t.CreateTime,
		UpdateTime:        t.UpdateTime,
		HasChild:          t.HasChild,
		Pid:               t.Pid,
		TaskName:          t.TaskName,
		SiteId:            t.SiteId,
		SiteWorkId:        t.SiteWorkId,
		Url:               t.Url,
		Status:            int32(t.Status),
		Continuable:       t.Continuable,
		PluginPublicId:    t.PluginPublicId,
		PluginExtensionId: t.PluginExtensionId,
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
		ResourceId:      p.ResourceId,
		ResourcePath:    p.ResourcePath,
		DownloadedBytes: p.DownloadedBytes,
	}
}

func taskResumeParamToProto(p *pluginsdkdto.TaskResumeParam) *gen.TaskResumeParam {
	if p == nil {
		return nil
	}
	offsets := make([]*gen.StoreResumeOffset, 0, len(p.StreamOffsets))
	for _, o := range p.StreamOffsets {
		if o == nil {
			continue
		}
		offsets = append(offsets, &gen.StoreResumeOffset{
			Role:     o.Role,
			StoreSeq: o.StoreSeq,
			Offset:   o.Offset,
		})
	}
	return &gen.TaskResumeParam{
		Task:          taskToProto(p.Task),
		StreamOffsets: offsets,
	}
}

// protoToTaskCreateChildResponses 子行声明 proto → DTO（批量转换；task 块内联子行与
// children_batch 批次子行共用）
func protoToTaskCreateChildResponses(children []*gen.TaskCreateChildResponse) []*pluginsdkdto.TaskCreateChildResponse {
	result := make([]*pluginsdkdto.TaskCreateChildResponse, len(children))
	for j, c := range children {
		result[j] = &pluginsdkdto.TaskCreateChildResponse{
			TaskName:      c.TaskName,
			SiteWorkId:    c.SiteWorkId,
			Url:           c.Url,
			PluginData:    c.PluginData,
			SiteName:      c.SiteName,
			SiteKey:       c.SiteKey,
			InvolvedRoles: c.InvolvedRoles,
			ResourceType:  c.ResourceType,
		}
	}
	return result
}

func protoToTaskCreateResponse(r *gen.TaskCreateResponse) *pluginsdkdto.TaskCreateResponse {
	return &pluginsdkdto.TaskCreateResponse{
		PluginTaskId:  r.PluginTaskId,
		TaskName:      r.TaskName,
		SiteWorkId:    r.SiteWorkId,
		Url:           r.Url,
		PluginData:    r.PluginData,
		SiteName:      r.SiteName,
		SiteKey:       r.SiteKey,
		InvolvedRoles: r.InvolvedRoles,
		ResourceType:  r.ResourceType,
		Children:      protoToTaskCreateChildResponses(r.Children),
	}
}

func protoToWorkResponse(pb *gen.WorkResponse) *pluginsdkdto.WorkResponse {
	if pb == nil {
		return nil
	}
	resp := &pluginsdkdto.WorkResponse{}
	if pb.Work != nil {
		resp.Work = &pluginsdkdto.WorkDTO{
			Id:                  pb.Work.Id,
			CreateTime:          pb.Work.CreateTime,
			UpdateTime:          pb.Work.UpdateTime,
			SiteId:              pb.Work.SiteId,
			SiteWorkId:          pb.Work.SiteWorkId,
			SiteWorkName:        pb.Work.SiteWorkName,
			SiteAuthorId:        pb.Work.SiteAuthorId,
			SiteWorkDescription: pb.Work.SiteWorkDescription,
			SiteUploadTime:      pb.Work.SiteUploadTime,
			SiteUpdateTime:      pb.Work.SiteUpdateTime,
			NickName:            pb.Work.NickName,
			LocalAuthorId:       pb.Work.LocalAuthorId,
			LastView:            pb.Work.LastView,
		}
	}
	if pb.Site != nil {
		resp.Site = &pluginsdkdto.SiteDTO{
			Id:         pb.Site.Id,
			SiteName:   pb.Site.SiteName,
			Homepage:   pb.Site.Homepage,
			CreateTime: pb.Site.CreateTime,
			UpdateTime: pb.Site.UpdateTime,
		}
	}
	for _, a := range pb.LocalAuthors {
		resp.LocalAuthors = append(resp.LocalAuthors, &pluginsdkdto.LocalAuthorDTO{
			Id:         a.Id,
			AuthorName: a.AuthorName,
			Introduce:  a.Introduce,
			LastUse:    a.LastUse,
			CreateTime: a.CreateTime,
			UpdateTime: a.UpdateTime,
			// RoleName 为 proto optional（*string），直接拷指针以保留 nil=未声明角色的语义，
			// 与同循环 AuthorName/Introduce 的指针拷贝惯例一致
			RoleName: a.RoleName,
		})
	}
	for _, t := range pb.LocalTags {
		resp.LocalTags = append(resp.LocalTags, &pluginsdkdto.LocalTagDTO{
			Id:             t.Id,
			LocalTagName:   t.LocalTagName,
			BaseLocalTagId: t.BaseLocalTagId,
			Description:    t.Description,
			LastUse:        t.LastUse,
			CreateTime:     t.CreateTime,
			UpdateTime:     t.UpdateTime,
		})
	}
	for _, a := range pb.SiteAuthors {
		resp.SiteAuthors = append(resp.SiteAuthors, &pluginsdkdto.TaskSiteAuthorDTO{
			SiteAuthorId:    a.SiteAuthorId,
			AuthorName:      a.AuthorName,
			Homepage:        a.Homepage,
			FixedAuthorName: a.FixedAuthorName,
			Introduce:       a.Introduce,
			RoleName:        a.RoleName,
			SiteKey:         a.SiteKey,
		})
	}
	for _, t := range pb.SiteTags {
		resp.SiteTags = append(resp.SiteTags, &pluginsdkdto.TaskSiteTagDTO{
			SiteTagId:   t.SiteTagId,
			TagName:     t.TagName,
			Description: t.Description,
			Namespace:   t.Namespace,
			SiteKey:     t.SiteKey,
		})
	}
	for _, ws := range pb.WorkSets {
		resp.WorkSets = append(resp.WorkSets, &pluginsdkdto.TaskWorkSetDTO{
			SiteWorkSetId: ws.SiteWorkSetId,
			WorkSetName:   ws.WorkSetName,
			SiteKey:       ws.SiteKey,
		})
	}
	return resp
}
