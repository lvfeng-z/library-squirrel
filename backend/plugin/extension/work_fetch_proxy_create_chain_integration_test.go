package extension_test

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"testing"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/database"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/plugin/extension"
	"github.com/library-squirrel/backend/pluginTaskUrlListener"
	"github.com/library-squirrel/backend/site"
	"github.com/library-squirrel/backend/task"
	"github.com/library-squirrel/backend/util"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
	"github.com/lvfeng-z/library-squirrel-sdk/gen"
	transport "github.com/lvfeng-z/library-squirrel-sdk/transport"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 本文件为「大选择集创建链取消回滚」的真接线集成锚定（live-test 缺陷 1 的复现与回归）。
// 外部测试包（extension_test）：本包经 task→stickymemory→extension 存在传递依赖，内部测试包
// 直接 import task 会成环；导出构造器 extension.NewWorkFetchProxy 供此处组装真接收泵。
// in-process gRPC（bufconn）承载假插件 WorkFetch 流（真 proto 分帧、真 Recv 泵路径），
// 宿主侧经生产链全程——WorkFetchProxy.CreateWithContext 接收泵 → task.Service.CreateTaskByURL
//（路由 + taskRouteAdapter.Invoke + handleCreateTaskStreamWithOutcome）→ 真库断言零残留。
// 既有桩流单测（task 包 handleCreateTaskStream 直调）未覆盖「泵+真流」接线，本文件补齐；
// 四形态：
//   - 内联首批 + 取消意图收尾（B3 实测形态：首批 2 万子行随父块内联、取消落在首个批次块前）
//     → 未收批统一回滚零残留
//   - 批次块开批未收批 + 提前 EOF → 回滚零残留（既有面经真泵回归）
//   - 内联单批 + 干净 EOF → 隐式原子提交点，行保留（过度回滚对照）
//   - 批次块收批（last=true）→ 原子提交点，行保留（过度回滚对照）

// —— 假插件流服务端（按预置块序列发送，return nil 即 gRPC EOF=插件提前收流形态）——

// scriptedWorkFetchServer 按序吐出预置 CreateChunk 的 WorkFetch 服务端替身
type scriptedWorkFetchServer struct {
	gen.UnimplementedWorkFetchServiceServer
	chunks []*gen.CreateChunk
}

func (s *scriptedWorkFetchServer) Create(_ *gen.CreateRequest, stream gen.WorkFetchService_CreateServer) error {
	for _, c := range s.chunks {
		if err := stream.Send(c); err != nil {
			return err
		}
	}
	return nil
}

// chainModeChunk 流式 mode 块（真 transport 首块同形）
func chainModeChunk() *gen.CreateChunk {
	return &gen.CreateChunk{Payload: &gen.CreateChunk_Mode{Mode: &gen.CreateMode{IsStream: true}}}
}

// chainChildN 第 i 个子行声明（字段取最短合法集；SiteKey 空继承父块站点键）
func chainChildN(i int) *gen.TaskCreateChildResponse {
	return &gen.TaskCreateChildResponse{
		TaskName:   fmt.Sprintf("child-%05d", i),
		SiteWorkId: fmt.Sprintf("sw-%05d", i),
		Url:        fmt.Sprintf("local://G:/it/file-%05d.zip", i),
	}
}

// chainParentChunk 父块（PluginTaskId 指称 + inlineCount 个内联子行——分帧插件首批随父块内联，契约 21）
func chainParentChunk(pluginTaskId string, inlineCount int) *gen.CreateChunk {
	children := make([]*gen.TaskCreateChildResponse, inlineCount)
	for i := range children {
		children[i] = chainChildN(i)
	}
	return &gen.CreateChunk{Payload: &gen.CreateChunk_Task{Task: &gen.TaskCreateResponse{
		PluginTaskId: pluginTaskId,
		TaskName:     "本地导入 · it · 集成批次",
		Url:          "local://G:/it",
		SiteKey:      chainSiteKey,
		Children:     children,
	}}}
}

// chainBatchChunk 子行批次块（plugin_task_id 指称挂父；last=末批声明）
func chainBatchChunk(pluginTaskId string, start, count int, last bool) *gen.CreateChunk {
	items := make([]*gen.TaskCreateChildResponse, count)
	for i := range items {
		items[i] = chainChildN(start + i)
	}
	return &gen.CreateChunk{Payload: &gen.CreateChunk_ChildrenBatch{ChildrenBatch: &gen.TaskChildrenBatch{
		PluginTaskId: pluginTaskId,
		Items:        items,
		Last:         last,
	}}}
}

// chainCancelIntentChunk 取消意图块（分帧插件取消收流的流终态声明，work_fetch.go 甲3/决策3 同形）
func chainCancelIntentChunk() *gen.CreateChunk {
	return &gen.CreateChunk{Payload: &gen.CreateChunk_Intent{Intent: &gen.CreateIntent{
		Kind:    gen.CreateIntentKind_CREATE_INTENT_KIND_CANCELLED,
		Message: "已取消本次创建：未发射的子任务不再产出，已发射的在途任务由主程序统一回滚清理",
	}}}
}

const (
	chainSiteKey = "it-site"
	chainURL     = "local://G:/it"
	// chainInlineBatchCount 内联首批子行数：取分帧插件的计数界（2 万）同规模复现 B3
	chainInlineBatchCount = 20000
)

// —— 宿主侧生产链装配（真泵 + 真服务 + 真库）——

// integrationWorkTaskStore 作品任务领域行存取（task 模块测试替身同款：BaseRepository 直写 +
// 共享主键覆写 + 零值时间戳补齐——接口与生产仓储一致，extension 包内自持实现）
type integrationWorkTaskStore struct {
	base *database.BaseRepository[entity.WorkTask]
}

func (s *integrationWorkTaskStore) CreateForTask(ctx context.Context, taskID int64, wt *entity.WorkTask) error {
	if taskID <= 0 {
		return fmt.Errorf("创建 work_task 领域行失败: 所属任务 ID 须为正数，实际 %d", taskID)
	}
	wt.SetID(taskID)
	if wt.GetCreateTime() == 0 {
		wt.SetCreateTime(util.GetCurrentTimestamp())
	}
	return s.base.Create(ctx, wt)
}

func (s *integrationWorkTaskStore) CreateBatchForTask(ctx context.Context, wts []*entity.WorkTask) error {
	if len(wts) == 0 {
		return nil
	}
	for _, wt := range wts {
		if wt.GetID() <= 0 {
			return fmt.Errorf("批量创建 work_task 领域行失败: 领域行须已持核心行 id，实际 %d", wt.GetID())
		}
		if wt.GetCreateTime() == 0 {
			wt.SetCreateTime(util.GetCurrentTimestamp())
		}
	}
	return s.base.CreateBatch(ctx, wts)
}

func (s *integrationWorkTaskStore) SaveForTask(ctx context.Context, taskID int64, wt *entity.WorkTask) error {
	if taskID <= 0 {
		return fmt.Errorf("保存 work_task 领域行失败: 所属任务 ID 须为正数，实际 %d", taskID)
	}
	wt.SetID(taskID)
	if wt.GetCreateTime() == 0 {
		wt.SetCreateTime(util.GetCurrentTimestamp())
	}
	return s.base.Save(ctx, wt)
}

func (s *integrationWorkTaskStore) GetById(ctx context.Context, id int64) (*entity.WorkTask, error) {
	return s.base.GetById(ctx, id)
}

func (s *integrationWorkTaskStore) ListByIds(ctx context.Context, ids []int64) (map[int64]*entity.WorkTask, error) {
	result := make(map[int64]*entity.WorkTask, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	vals := make([]any, len(ids))
	for i, id := range ids {
		vals[i] = id
	}
	rows, err := s.base.List(ctx, &database.QueryOption{
		Conditions: []clause.Expression{clause.IN{Column: clause.Column{Name: "id"}, Values: vals}},
	})
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.GetID()] = row
	}
	return result, nil
}

// integrationTransactor 真事务执行器（事务 DB 经 ctx 传递，仓储 dbFromCtx 感知——与生产一致）
type integrationTransactor struct{ db *gorm.DB }

func (t *integrationTransactor) ExecInTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	return database.WithTransactionContext(ctx, t.db, func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, database.TxKey, tx)
		return fn(txCtx)
	})
}

// fixedWorkFetchGetter 恒返回真泵代理的 WorkFetchProvider 替身（路由 Invoke 经它取 Create 通道）
type fixedWorkFetchGetter struct{ fetcher sdkdto.WorkFetcher }

func (g fixedWorkFetchGetter) GetWorkFetcher(_, _ string) (sdkdto.WorkFetcher, error) {
	return g.fetcher, nil
}

// chainServiceAccessor 恒返回预置 gRPC 客户端的 ServiceAccessor 替身（extension.ServiceAccessor
// 的测试实现——生产由 Loader 实现）
type chainServiceAccessor struct{ client *transport.GRPCPluginClient }

func (a *chainServiceAccessor) GetServices(string) (*transport.GRPCPluginClient, bool) {
	return a.client, true
}

// newCreateChainFixture 组装真接线创建链：bufconn gRPC 假插件流 → 真客户端 → WorkFetchProxy
// 接收泵 → 真 task.Service（真仓储/真事务/真站点服务/URL 监听派生索引）→ 真测试库。
// 返回服务与库句柄供行数断言。
func newCreateChainFixture(t *testing.T, chunks []*gen.CreateChunk) (*task.Service, *gorm.DB) {
	t.Helper()
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}

	// 假插件流：in-process gRPC 服务端 + 真客户端连接
	lis := bufconn.Listen(16 << 20)
	grpcSrv := grpc.NewServer()
	gen.RegisterWorkFetchServiceServer(grpcSrv, &scriptedWorkFetchServer{chunks: chunks})
	serveErr := make(chan error, 1)
	go func() { serveErr <- grpcSrv.Serve(lis) }()
	t.Cleanup(grpcSrv.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.DialContext(context.Background())
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("建 gRPC 客户端失败: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	// 真接收泵：WorkFetchProxy 经 ServiceAccessor 替身取客户端（生产 GetServices 通道同款）
	proxy := extension.NewWorkFetchProxy(&chainServiceAccessor{client: &transport.GRPCPluginClient{
		WorkFetch: gen.NewWorkFetchServiceClient(conn),
	}}, "pub-it", "ext-it")

	// 真服务：站点种子 + 仓储 + 事务执行器 + URL 监听派生索引（local:// 模式）
	seedSite := entity.NewSite()
	seedSite.SiteKey = chainSiteKey
	seedSite.SiteName = sql.NullString{String: "集成站点", Valid: true}
	if err := db.Create(seedSite).Error; err != nil {
		t.Fatalf("建站点种子失败: %v", err)
	}
	wtStore := &integrationWorkTaskStore{base: database.NewBaseRepository[entity.WorkTask](db)}
	listenerSvc := pluginTaskUrlListener.NewService(pluginTaskUrlListener.NewManager())
	pluginEntity := entity.NewPlugin()
	pluginEntity.PublicID = sql.NullString{String: "pub-it", Valid: true}
	pluginEntity.Name = sql.NullString{String: "localImport 集成替身", Valid: true}
	listenerSvc.RegisterDeclared(pluginEntity, []dto.WorkFetchDeclaration{
		{ID: "ext-it", Name: "本地导入", UrlPatterns: []string{"^local://"}},
	})
	svc := task.NewService(
		task.NewRepository(db, wtStore, wtStore),
		&integrationTransactor{db: db},
		fixedWorkFetchGetter{fetcher: proxy},
		listenerSvc,
		site.NewService(site.NewRepository(db)),
		nil, nil)
	return svc, db
}

// assertChainTableCount 真库表行数断言
func assertChainTableCount(t *testing.T, db *gorm.DB, table string, want int) {
	t.Helper()
	var got int
	if err := db.Raw(fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&got).Error; err != nil {
		t.Fatalf("统计 %s 行数失败: %v", table, err)
	}
	if got != want {
		t.Fatalf("期望 %s 表 %d 行，得到 %d", table, want, got)
	}
}

// TestCreateChainPump_InlineBatchCancelIntentRollsBack B3 实测形态复现（缺陷 1 锚）：
// mode → 父块（首批 2 万子行内联，分帧插件的计数界同规模）→ 取消意图块 → EOF——取消落在
// 首个 children_batch 批次块之前（开批标志恒 false），须经内联首批面判定走未收批统一回滚：
// 父行与全部内联子行删除，task/work_task 零残留；应答为取消意图的成功零任务（AddedQuantity=0）。
func TestCreateChainPump_InlineBatchCancelIntentRollsBack(t *testing.T) {
	svc, db := newCreateChainFixture(t, []*gen.CreateChunk{
		chainModeChunk(),
		chainParentChunk("local-batch-1790000000000", chainInlineBatchCount),
		chainCancelIntentChunk(),
	})

	resp, err := svc.CreateTaskByURL(context.Background(), chainURL)
	if err != nil {
		t.Fatalf("CreateTaskByURL 返回错误: %v", err)
	}
	if !resp.Succeed || resp.AddedQuantity != 0 {
		t.Fatalf("取消意图收尾应为成功零任务（Succeed=true, AddedQuantity=0），得到 Succeed=%v Added=%d Msg=%q",
			resp.Succeed, resp.AddedQuantity, resp.Msg)
	}
	assertChainTableCount(t, db, "task", 0)
	assertChainTableCount(t, db, "work_task", 0)
}

// TestCreateChainPump_OpenBatchEOFRollsBack 批次块面（既有语义）经真泵回归：父块（1 内联子行）
// → children_batch（2 万行，last=false）→ 插件提前 EOF（无意图块形态）——开批未收批，
// 未收批统一回滚零残留；应答失败（零任务且未说明原因形态）。
func TestCreateChainPump_OpenBatchEOFRollsBack(t *testing.T) {
	svc, db := newCreateChainFixture(t, []*gen.CreateChunk{
		chainModeChunk(),
		chainParentChunk("local-batch-1790000000001", 1),
		chainBatchChunk("local-batch-1790000000001", 1, chainInlineBatchCount, false),
	})

	resp, err := svc.CreateTaskByURL(context.Background(), chainURL)
	if err != nil {
		t.Fatalf("CreateTaskByURL 返回错误: %v", err)
	}
	if resp.Succeed {
		t.Fatalf("开批未收批回滚后应为失败应答，得到 Succeed=true Msg=%q", resp.Msg)
	}
	assertChainTableCount(t, db, "task", 0)
	assertChainTableCount(t, db, "work_task", 0)
}

// TestCreateChainPump_SingleInlineBatchCleanEOFCommits 过度回滚对照①：单批全内联（2 万子行随父块）
// + 干净 EOF（无意图/无失败声明）= 隐式原子提交点（无批次块=无回滚面），父行与子行全部保留。
func TestCreateChainPump_SingleInlineBatchCleanEOFCommits(t *testing.T) {
	svc, db := newCreateChainFixture(t, []*gen.CreateChunk{
		chainModeChunk(),
		chainParentChunk("local-batch-1790000000002", chainInlineBatchCount),
	})

	resp, err := svc.CreateTaskByURL(context.Background(), chainURL)
	if err != nil {
		t.Fatalf("CreateTaskByURL 返回错误: %v", err)
	}
	if !resp.Succeed || resp.AddedQuantity != chainInlineBatchCount {
		t.Fatalf("干净收尾应为成功 %d 个任务，得到 Succeed=%v Added=%d", chainInlineBatchCount, resp.Succeed, resp.AddedQuantity)
	}
	assertChainTableCount(t, db, "task", chainInlineBatchCount+1)
	assertChainTableCount(t, db, "work_task", chainInlineBatchCount+1)
}

// TestCreateChainPump_ClosedBatchCommits 过度回滚对照②：末批 last=true 已到（收批=原子提交点），
// 其后流结束不回滚——父行 + 内联子行 + 批次子行全部保留。
func TestCreateChainPump_ClosedBatchCommits(t *testing.T) {
	svc, db := newCreateChainFixture(t, []*gen.CreateChunk{
		chainModeChunk(),
		chainParentChunk("local-batch-1790000000003", 1),
		chainBatchChunk("local-batch-1790000000003", 1, 2, true),
	})

	resp, err := svc.CreateTaskByURL(context.Background(), chainURL)
	if err != nil {
		t.Fatalf("CreateTaskByURL 返回错误: %v", err)
	}
	if !resp.Succeed || resp.AddedQuantity != 3 {
		t.Fatalf("收批提交应为成功 3 个任务（1 内联+2 批次），得到 Succeed=%v Added=%d", resp.Succeed, resp.AddedQuantity)
	}
	assertChainTableCount(t, db, "task", 4) // 父 1 + 子 3
	assertChainTableCount(t, db, "work_task", 4)
}
