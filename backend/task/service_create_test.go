package task

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/pluginTaskUrlListener"
	"github.com/library-squirrel/backend/site"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
	"go.uber.org/zap"
)

// 本文件为 leaf/独立任务创建路径回归测试（task-graph F 节点）。
// 守护 applyCreateResponse 单点权威：stream 与 array 两路径对 leaf/parent+children 三态行为一致。
//
// 统一契约（planCreateResponse，stream/array 共用）：
//   - 无 Children → 独立 leaf：1 任务，pid=NULL（Valid=false，根级）、HasChild=false，计数 1。
//   - 有 Children → parent + 每个 child（不折叠，Children=[1] 也建 parent+child）：
//     parent HasChild=true、pid=NULL（根级）；children pid=parent.id；计数 = len(children)（parent 容器不计）。
//   - stream 额外：同 PluginTaskId 的多响应合并为同一 parent（让插件可把一个超大 work 拆成多响应流式发）。
//
// 计数语义：叶子级单元——独立 leaf 算 1、parent+N 算 N；CreateTaskByURL 对 stream 数 Task 项
// （leaf 与 child）、对 array 用返回值，二者一致。

// TestMain 初始化 nop logger——创建路径个别分支会记 Errorf/Infof，未初始化的 logger.Log 会 nil panic。
func TestMain(m *testing.M) {
	logger.Log = zap.NewNop().Sugar()
	os.Exit(m.Run())
}

const (
	testSiteName = "pixiv"
	testSiteID   = int64(100)
	testPluginID = "plugin-1"
	testExtID    = "ext-1"
)

// fakeTaskRepo 记录 CreateTask/CreateBatch 创建的任务并模拟自增主键——
// parent→child Pid 链接（parent.GetID()）依赖 CreateTask 回填 ID。
// 其余 Repository 方法用 nil 接口嵌入满足签名：创建路径仅触达 CreateTask/CreateBatch。
type fakeTaskRepo struct {
	Repository
	mu     sync.Mutex
	tasks  []*entity.Task
	nextID int64
}

func newFakeTaskRepo() *fakeTaskRepo {
	return &fakeTaskRepo{nextID: 1}
}

func (f *fakeTaskRepo) CreateTask(_ context.Context, task *entity.Task) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	task.BaseEntity.ID = f.nextID
	f.nextID++
	f.tasks = append(f.tasks, task)
	return nil
}

func (f *fakeTaskRepo) CreateBatch(_ context.Context, tasks []*entity.Task) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, t := range tasks {
		t.BaseEntity.ID = f.nextID
		f.nextID++
		f.tasks = append(f.tasks, t)
	}
	return nil
}

// fakeSiteRepo 让 site.Service.GetByName 回填固定站点 ID（创建路径经 siteSvc.GetByName→repo.Get）。
type fakeSiteRepo struct {
	site.Repository
}

func (fakeSiteRepo) Get(_ context.Context, _ *database.QueryOption) (*entity.Site, error) {
	return &entity.Site{BaseEntity: &model.BaseEntity{ID: testSiteID}}, nil
}

// fakeTransactor 直接执行 fn（不开真实事务），让 array 路径的 ExecInTransaction 在内存里跑通。
type fakeTransactor struct{}

func (fakeTransactor) ExecInTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

// newTestService 经生产构造函数 NewService 组装 Service，注入 fake 依赖。
func newTestService(t *testing.T) (*Service, *fakeTaskRepo) {
	t.Helper()
	repo := newFakeTaskRepo()
	siteSvc := site.NewService(fakeSiteRepo{}, fakeTransactor{}, nil, nil, nil, nil, nil) // 守卫计数提供方不触达（创建路径）
	svc := NewService(repo, fakeTransactor{}, nil, nil, siteSvc)
	return svc, repo
}

func testListener() *pluginTaskUrlListener.PluginWithExtension {
	return &pluginTaskUrlListener.PluginWithExtension{
		Plugin:      &entity.Plugin{PublicID: sql.NullString{String: testPluginID, Valid: true}},
		ExtensionID: testExtID,
	}
}

// findParent 找出落盘任务中 HasChild=true 的父任务。
func findParent(ts []*entity.Task) *entity.Task {
	for _, t := range ts {
		if t.HasChild.Valid && t.HasChild.Bool {
			return t
		}
	}
	return nil
}

// countParents 统计落盘任务中父任务（HasChild=true）数量。
func countParents(ts []*entity.Task) int {
	n := 0
	for _, t := range ts {
		if t.HasChild.Valid && t.HasChild.Bool {
			n++
		}
	}
	return n
}

// drainStream 耗尽 stream 输出通道，返回总项数与叶子级单元数（Task 项；Parent 容器不计）。
func drainStream(out <-chan *CreateTaskStreamChan) (total int, leafUnits int) {
	for item := range out {
		total++
		if item.Task != nil {
			leafUnits++
		}
	}
	return
}

// assertChildrenPid 断言 ts 中所有非父任务的 Pid 都指向 parentID，且子任务数等于 want。
func assertChildrenPid(t *testing.T, ts []*entity.Task, parentID int64, want int) {
	t.Helper()
	got := 0
	for _, tk := range ts {
		if tk.HasChild.Valid && tk.HasChild.Bool {
			continue
		}
		got++
		if !tk.Pid.Valid || tk.Pid.Int64 != parentID {
			t.Errorf("子任务 Pid 应为父任务 ID %d，得到 %+v", parentID, tk.Pid)
		}
	}
	if got != want {
		t.Errorf("期望 %d 个子任务，得到 %d", want, got)
	}
}

// ---- 共享断言：leaf 任务字段 ----

func assertLeafTask(t *testing.T, leaf *entity.Task) {
	t.Helper()
	if leaf.HasChild.Valid && leaf.HasChild.Bool {
		t.Errorf("leaf HasChild 应为 false，得到 true")
	}
	// 统一契约：leaf 无父写 pid=NULL（Valid=false），与 parent 的根级形态一致（外键下无 id=0 行，0 必违约）
	if leaf.Pid.Valid {
		t.Errorf("leaf Pid 应为 NULL（Valid=false，根级），得到 %+v", leaf.Pid)
	}
	if !leaf.SiteID.Valid || leaf.SiteID.Int64 != testSiteID {
		t.Errorf("SiteID 回填应为 %d，得到 %+v", testSiteID, leaf.SiteID)
	}
	if !leaf.PluginPublicID.Valid || leaf.PluginPublicID.String != testPluginID {
		t.Errorf("PluginPublicID 回填错误，得到 %+v", leaf.PluginPublicID)
	}
	if !leaf.PluginExtensionID.Valid || leaf.PluginExtensionID.String != testExtID {
		t.Errorf("PluginExtensionID 回填错误，得到 %+v", leaf.PluginExtensionID)
	}
}

// ---- handleCreateTaskStream：stream 路径三态 ----

// TestHandleCreateTaskStream_Leaf 无 Children 的响应（如 local 单文件导入）创建为独立 leaf。
func TestHandleCreateTaskStream_Leaf(t *testing.T) {
	svc, repo := newTestService(t)
	in := make(chan *sdkdto.TaskCreateResponse, 1)
	in <- &sdkdto.TaskCreateResponse{
		TaskName:     "单图作品",
		SiteWorkId:   "w-1",
		Url:          "http://x/1",
		SiteName:     testSiteName,
		ResourceType: entity.ResourceTypeImage,
	}
	close(in)

	out, err := svc.handleCreateTaskStream(context.Background(), in, testListener(), 100)
	if err != nil {
		t.Fatalf("handleCreateTaskStream 返回错误: %v", err)
	}
	total, leafUnits := drainStream(out)
	if total != 1 || leafUnits != 1 {
		t.Fatalf("期望 outChan 1 项（leaf）/ 叶子单元 1，得到 total=%d leafUnits=%d", total, leafUnits)
	}
	if len(repo.tasks) != 1 {
		t.Fatalf("期望落盘 1 个 leaf 任务，得到 %d 个", len(repo.tasks))
	}

	leaf := repo.tasks[0]
	assertLeafTask(t, leaf)
	if !leaf.ResourceType.Valid || leaf.ResourceType.String != entity.ResourceTypeImage {
		t.Errorf("ResourceType 回填错误，得到 %+v", leaf.ResourceType)
	}
	if !leaf.TaskName.Valid || leaf.TaskName.String != "单图作品" {
		t.Errorf("TaskName 回填错误，得到 %+v", leaf.TaskName)
	}
}

// TestHandleCreateTaskStream_SingleChild Children=[1] → parent+1child（不折叠）。
func TestHandleCreateTaskStream_SingleChild(t *testing.T) {
	svc, repo := newTestService(t)
	in := make(chan *sdkdto.TaskCreateResponse, 1)
	in <- &sdkdto.TaskCreateResponse{
		TaskName:     "work-A",
		Url:          "http://x/a",
		SiteName:     testSiteName,
		ResourceType: entity.ResourceTypeImage,
		Children: []*sdkdto.TaskCreateChildResponse{
			{TaskName: "p1", SiteWorkId: "c-1", Url: "http://x/1", SiteName: testSiteName, ResourceType: entity.ResourceTypeImage},
		},
	}
	close(in)

	out, err := svc.handleCreateTaskStream(context.Background(), in, testListener(), 100)
	if err != nil {
		t.Fatalf("handleCreateTaskStream 返回错误: %v", err)
	}
	if total, leafUnits := drainStream(out); total != 2 || leafUnits != 1 {
		t.Fatalf("期望 outChan 2 项（1 Parent + 1 child）/ 叶子单元 1，得到 total=%d leafUnits=%d", total, leafUnits)
	}
	if len(repo.tasks) != 2 {
		t.Fatalf("期望落盘 2 个任务（parent+1child），得到 %d 个", len(repo.tasks))
	}

	parent := findParent(repo.tasks)
	if parent == nil {
		t.Fatal("未找到 HasChild=true 的父任务")
	}
	if parent.Pid.Valid {
		t.Errorf("父任务 Pid 应为 NULL（Valid=false，根级），得到 %+v", parent.Pid)
	}
	assertChildrenPid(t, repo.tasks, parent.GetID(), 1)
}

// TestHandleCreateTaskStream_ParentChildren Children=[2] → parent+2child。
func TestHandleCreateTaskStream_ParentChildren(t *testing.T) {
	svc, repo := newTestService(t)
	in := make(chan *sdkdto.TaskCreateResponse, 1)
	in <- &sdkdto.TaskCreateResponse{
		TaskName:     "work-B",
		Url:          "http://x/b",
		SiteName:     testSiteName,
		ResourceType: entity.ResourceTypeImage,
		Children: []*sdkdto.TaskCreateChildResponse{
			{TaskName: "p1", SiteWorkId: "c-1", Url: "http://x/1", SiteName: testSiteName, ResourceType: entity.ResourceTypeImage},
			{TaskName: "p2", SiteWorkId: "c-2", Url: "http://x/2", SiteName: testSiteName, ResourceType: entity.ResourceTypeImage},
		},
	}
	close(in)

	out, err := svc.handleCreateTaskStream(context.Background(), in, testListener(), 100)
	if err != nil {
		t.Fatalf("handleCreateTaskStream 返回错误: %v", err)
	}
	if total, leafUnits := drainStream(out); total != 3 || leafUnits != 2 {
		t.Fatalf("期望 outChan 3 项（1 Parent + 2 child）/ 叶子单元 2，得到 total=%d leafUnits=%d", total, leafUnits)
	}
	if len(repo.tasks) != 3 {
		t.Fatalf("期望落盘 3 个任务（parent+2child），得到 %d 个", len(repo.tasks))
	}

	parent := findParent(repo.tasks)
	if parent == nil {
		t.Fatal("未找到 HasChild=true 的父任务")
	}
	if parent.Pid.Valid {
		t.Errorf("父任务 Pid 应为 NULL（Valid=false，根级），得到 %+v", parent.Pid)
	}
	assertChildrenPid(t, repo.tasks, parent.GetID(), 2)
}

// ---- handleCreateTaskStream：stream 合并（同 PluginTaskId 归同一 parent）----

// TestHandleCreateTaskStream_SameWorkMergedAcrossResponses 同 PluginTaskId 的多响应合并为同一 parent。
// 守护 stream 合并能力：插件可把一个超大 work 拆成多响应流式发（复用同一 PluginTaskId），host 拼回同一父。
func TestHandleCreateTaskStream_SameWorkMergedAcrossResponses(t *testing.T) {
	svc, repo := newTestService(t)
	in := make(chan *sdkdto.TaskCreateResponse, 2)
	in <- &sdkdto.TaskCreateResponse{
		PluginTaskId: "work-X", TaskName: "work-X", Url: "http://x", SiteName: testSiteName,
		Children: []*sdkdto.TaskCreateChildResponse{
			{TaskName: "p1", SiteWorkId: "c-1", Url: "http://x/1", SiteName: testSiteName, ResourceType: entity.ResourceTypeImage},
			{TaskName: "p2", SiteWorkId: "c-2", Url: "http://x/2", SiteName: testSiteName, ResourceType: entity.ResourceTypeImage},
		},
	}
	in <- &sdkdto.TaskCreateResponse{
		PluginTaskId: "work-X", TaskName: "work-X", Url: "http://x", SiteName: testSiteName,
		Children: []*sdkdto.TaskCreateChildResponse{
			{TaskName: "p3", SiteWorkId: "c-3", Url: "http://x/3", SiteName: testSiteName, ResourceType: entity.ResourceTypeImage},
		},
	}
	close(in)

	out, err := svc.handleCreateTaskStream(context.Background(), in, testListener(), 100)
	if err != nil {
		t.Fatalf("handleCreateTaskStream 返回错误: %v", err)
	}
	if total, leafUnits := drainStream(out); total != 4 || leafUnits != 3 {
		t.Fatalf("期望 outChan 4 项（1 Parent + 3 child）/ 叶子单元 3，得到 total=%d leafUnits=%d", total, leafUnits)
	}
	if got := countParents(repo.tasks); got != 1 {
		t.Fatalf("同 PluginTaskId 应合并为 1 个 parent，得到 %d", got)
	}
	if len(repo.tasks) != 4 {
		t.Fatalf("期望落盘 4 个任务（1 parent + 3 child），得到 %d 个", len(repo.tasks))
	}
	assertChildrenPid(t, repo.tasks, findParent(repo.tasks).GetID(), 3)
}

// TestHandleCreateTaskStream_DifferentWorksNotMerged 不同 PluginTaskId 的响应各自独立 parent（不合并）。
func TestHandleCreateTaskStream_DifferentWorksNotMerged(t *testing.T) {
	svc, repo := newTestService(t)
	in := make(chan *sdkdto.TaskCreateResponse, 2)
	in <- &sdkdto.TaskCreateResponse{
		PluginTaskId: "work-A", TaskName: "work-A", Url: "http://x/a", SiteName: testSiteName,
		Children: []*sdkdto.TaskCreateChildResponse{
			{TaskName: "p1", SiteWorkId: "c-1", Url: "http://x/1", SiteName: testSiteName, ResourceType: entity.ResourceTypeImage},
		},
	}
	in <- &sdkdto.TaskCreateResponse{
		PluginTaskId: "work-B", TaskName: "work-B", Url: "http://x/b", SiteName: testSiteName,
		Children: []*sdkdto.TaskCreateChildResponse{
			{TaskName: "p2", SiteWorkId: "c-2", Url: "http://x/2", SiteName: testSiteName, ResourceType: entity.ResourceTypeImage},
		},
	}
	close(in)

	out, err := svc.handleCreateTaskStream(context.Background(), in, testListener(), 100)
	if err != nil {
		t.Fatalf("handleCreateTaskStream 返回错误: %v", err)
	}
	if total, leafUnits := drainStream(out); total != 4 || leafUnits != 2 {
		t.Fatalf("期望 outChan 4 项（2 Parent + 2 child）/ 叶子单元 2，得到 total=%d leafUnits=%d", total, leafUnits)
	}
	if got := countParents(repo.tasks); got != 2 {
		t.Fatalf("不同 PluginTaskId 应各为独立 parent（2 个），得到 %d", got)
	}
	if len(repo.tasks) != 4 {
		t.Fatalf("期望落盘 4 个任务（2 parent + 2 child），得到 %d 个", len(repo.tasks))
	}
	// 两个 child 各指向不同 parent（Pid 分布应有 2 个不同值）
	childrenByPid := make(map[int64]int)
	for _, tk := range repo.tasks {
		if tk.HasChild.Valid && tk.HasChild.Bool {
			continue
		}
		childrenByPid[tk.Pid.Int64]++
	}
	if len(childrenByPid) != 2 {
		t.Errorf("期望 2 个 child 各指向不同 parent，得到 Pid 分布 %+v", childrenByPid)
	}
}

// ---- handleCreateTaskArray：array 路径三态（与 stream 行为一致）----

// TestHandleTaskArray_Leaf 无 Children 的响应创建为独立 leaf（与 stream 一致；不再静默丢弃）。
func TestHandleTaskArray_Leaf(t *testing.T) {
	svc, repo := newTestService(t)
	responses := []*sdkdto.TaskCreateResponse{
		{
			TaskName:     "独立任务",
			SiteWorkId:   "w-1",
			Url:          "http://x/1",
			SiteName:     testSiteName,
			ResourceType: entity.ResourceTypeImage,
			// 无 Children
		},
	}

	count, err := svc.handleCreateTaskArray(context.Background(), responses, testListener())
	if err != nil {
		t.Fatalf("handleCreateTaskArray 返回错误: %v", err)
	}
	if count != 1 {
		t.Fatalf("期望返回叶子单元计数 1，得到 %d", count)
	}
	if len(repo.tasks) != 1 {
		t.Fatalf("期望落盘 1 个 leaf 任务，得到 %d 个", len(repo.tasks))
	}

	assertLeafTask(t, repo.tasks[0])
	if !repo.tasks[0].TaskName.Valid || repo.tasks[0].TaskName.String != "独立任务" {
		t.Errorf("TaskName 回填错误，得到 %+v", repo.tasks[0].TaskName)
	}
}

// TestHandleTaskArray_SingleChild Children=[1] → parent+1child（不折叠，与 stream 一致）。
func TestHandleTaskArray_SingleChild(t *testing.T) {
	svc, repo := newTestService(t)
	responses := []*sdkdto.TaskCreateResponse{
		{
			TaskName: "work-A",
			Url:      "http://x/a",
			SiteName: testSiteName,
			Children: []*sdkdto.TaskCreateChildResponse{
				{TaskName: "p1", SiteWorkId: "c-1", Url: "http://x/1", SiteName: testSiteName, ResourceType: entity.ResourceTypeImage},
			},
		},
	}

	count, err := svc.handleCreateTaskArray(context.Background(), responses, testListener())
	if err != nil {
		t.Fatalf("handleCreateTaskArray 返回错误: %v", err)
	}
	if count != 1 {
		t.Fatalf("期望返回叶子单元计数 1（1 child），得到 %d", count)
	}
	if len(repo.tasks) != 2 {
		t.Fatalf("期望落盘 2 个任务（parent+1child，不折叠），得到 %d 个", len(repo.tasks))
	}

	parent := findParent(repo.tasks)
	if parent == nil {
		t.Fatal("未找到 HasChild=true 的父任务")
	}
	if parent.Pid.Valid {
		t.Errorf("父任务 Pid 应为 NULL（Valid=false，根级），得到 %+v", parent.Pid)
	}
	assertChildrenPid(t, repo.tasks, parent.GetID(), 1)
	// 子任务身份字段取自 child 响应
	for _, tk := range repo.tasks {
		if tk.HasChild.Valid && tk.HasChild.Bool {
			continue
		}
		if !tk.TaskName.Valid || tk.TaskName.String != "p1" {
			t.Errorf("子任务 TaskName 应为 p1，得到 %+v", tk.TaskName)
		}
		if !tk.SiteWorkID.Valid || tk.SiteWorkID.String != "c-1" {
			t.Errorf("子任务 SiteWorkID 应为 c-1，得到 %+v", tk.SiteWorkID)
		}
	}
}

// TestHandleTaskArray_ParentChildren Children=[2] → parent+2child。
func TestHandleTaskArray_ParentChildren(t *testing.T) {
	svc, repo := newTestService(t)
	responses := []*sdkdto.TaskCreateResponse{
		{
			TaskName:     "work-B",
			Url:          "http://x/b",
			SiteName:     testSiteName,
			ResourceType: entity.ResourceTypeImage,
			Children: []*sdkdto.TaskCreateChildResponse{
				{TaskName: "p1", SiteWorkId: "c-1", Url: "http://x/1", SiteName: testSiteName, ResourceType: entity.ResourceTypeImage},
				{TaskName: "p2", SiteWorkId: "c-2", Url: "http://x/2", SiteName: testSiteName, ResourceType: entity.ResourceTypeImage},
			},
		},
	}

	count, err := svc.handleCreateTaskArray(context.Background(), responses, testListener())
	if err != nil {
		t.Fatalf("handleCreateTaskArray 返回错误: %v", err)
	}
	if count != 2 {
		t.Fatalf("期望返回叶子单元计数 2（2 children），得到 %d", count)
	}
	if len(repo.tasks) != 3 {
		t.Fatalf("期望落盘 3 个任务（parent+2child），得到 %d 个", len(repo.tasks))
	}

	parent := findParent(repo.tasks)
	if parent == nil {
		t.Fatal("未找到 HasChild=true 的父任务")
	}
	if parent.Pid.Valid {
		t.Errorf("父任务 Pid 应为 NULL（Valid=false，根级），得到 %+v", parent.Pid)
	}
	assertChildrenPid(t, repo.tasks, parent.GetID(), 2)
}

// ---- OpenTestDB 外键强制库落盘锚定：pid 形态回归 ----

// TestCreateTaskFKColumnsOnFKDB 在外键强制库（migration.OpenTestDB）真实落盘锚定 CreateTask 的
// 外键列形态（pid/site_id）：两列各引用 task.id/site.id 且库内无 id=0 行，写 0（Valid=true）必违约——
// 落盘成功本身即不违约的证明，另断言落库值：pid 根级/子任务= NULL/父 ID、site_id 无站点/带站点=
// NULL/站点 ID。插件响应 array 路径（leaf 与 parent+children）pid 一并锚定（其 site_id 恒经站点
// 查找解析，无 0 值形态）。fakeRepo 测试（上文）无 FK 约束，锚不住本形态，故须真实库锚定。
func TestCreateTaskFKColumnsOnFKDB(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}

	// 站点种子（task.site_id 外键防线；插件响应路径经 GetByName 查到本行）
	seedSite := entity.NewSite()
	seedSite.SiteName = sql.NullString{String: testSiteName, Valid: true}
	if err := db.Create(seedSite).Error; err != nil {
		t.Fatalf("建站点种子失败: %v", err)
	}

	// 经生产构造函数组装（真实 task 仓储 + 真事务执行器 + 真实 site 服务）
	siteSvc := site.NewService(site.NewRepository(db), nil, nil, nil, nil, nil, nil)
	svc := NewService(NewRepository(db), &testTransactor{db: db}, nil, nil, siteSvc)
	ctx := context.Background()

	// 入口一/二：CreateTask——req.Pid=0 落 NULL=根级；req.Pid=父 落父 ID
	root, err := svc.CreateTask(ctx, &dto.CreateTaskRequest{TaskName: "根任务", SiteID: int(seedSite.GetID())})
	if err != nil {
		t.Fatalf("创建根任务失败: %v", err)
	}
	child, err := svc.CreateTask(ctx, &dto.CreateTaskRequest{Pid: root.GetID(), TaskName: "子任务", SiteID: int(seedSite.GetID())})
	if err != nil {
		t.Fatalf("创建子任务失败: %v", err)
	}
	// req.SiteID=0（未关联站点）落 NULL——site 无 id=0 行，写 0 必 FK 违约
	noSite, err := svc.CreateTask(ctx, &dto.CreateTaskRequest{TaskName: "无站点任务"})
	if err != nil {
		t.Fatalf("创建无站点任务失败: %v", err)
	}

	// 入口三：插件响应 array 路径——独立 leaf 与 parent 容器均根级（pid=NULL）、child 指向父
	responses := []*sdkdto.TaskCreateResponse{
		{TaskName: "独立leaf", SiteWorkId: "w-1", Url: "http://x/1", SiteName: testSiteName, ResourceType: entity.ResourceTypeImage},
		{
			TaskName: "work-A", Url: "http://x/a", SiteName: testSiteName, ResourceType: entity.ResourceTypeImage,
			Children: []*sdkdto.TaskCreateChildResponse{
				{TaskName: "p1", SiteWorkId: "c-1", Url: "http://x/c1", SiteName: testSiteName, ResourceType: entity.ResourceTypeImage},
			},
		},
	}
	if _, err := svc.handleCreateTaskArray(ctx, responses, testListener()); err != nil {
		t.Fatalf("插件响应路径创建失败: %v", err)
	}

	// pid 落库形态断言（直接查列，不经实体扫描）
	pidOf := func(where string, args ...any) sql.NullInt64 {
		t.Helper()
		var pid sql.NullInt64
		if err := db.Raw("SELECT pid FROM task WHERE "+where, args...).Scan(&pid).Error; err != nil {
			t.Fatalf("查 pid 失败(%s): %v", where, err)
		}
		return pid
	}
	if pid := pidOf("id = ?", root.GetID()); pid.Valid {
		t.Errorf("CreateTask 根级任务 pid 应落 NULL，得到 %+v", pid)
	}
	if pid := pidOf("id = ?", child.GetID()); !pid.Valid || pid.Int64 != root.GetID() {
		t.Errorf("CreateTask 子任务 pid 应落父 ID %d，得到 %+v", root.GetID(), pid)
	}
	if pid := pidOf("site_work_id = ?", "w-1"); pid.Valid {
		t.Errorf("插件路径独立 leaf pid 应落 NULL，得到 %+v", pid)
	}
	if pid := pidOf("task_name = ?", "work-A"); pid.Valid {
		t.Errorf("插件路径 parent 容器 pid 应落 NULL，得到 %+v", pid)
	}
	var parentId int64
	if err := db.Raw("SELECT id FROM task WHERE task_name = ?", "work-A").Scan(&parentId).Error; err != nil {
		t.Fatalf("查 parent id 失败: %v", err)
	}
	if pid := pidOf("site_work_id = ?", "c-1"); !pid.Valid || pid.Int64 != parentId {
		t.Errorf("插件路径子任务 pid 应落父 ID %d，得到 %+v", parentId, pid)
	}

	// site_id 落库形态断言（直接查列，不经实体扫描）
	siteIdOf := func(where string, args ...any) sql.NullInt64 {
		t.Helper()
		var siteId sql.NullInt64
		if err := db.Raw("SELECT site_id FROM task WHERE "+where, args...).Scan(&siteId).Error; err != nil {
			t.Fatalf("查 site_id 失败(%s): %v", where, err)
		}
		return siteId
	}
	if siteId := siteIdOf("id = ?", root.GetID()); !siteId.Valid || siteId.Int64 != seedSite.GetID() {
		t.Errorf("CreateTask 带站点任务 site_id 应落站点 ID %d，得到 %+v", seedSite.GetID(), siteId)
	}
	if siteId := siteIdOf("id = ?", noSite.GetID()); siteId.Valid {
		t.Errorf("CreateTask 无站点任务 site_id 应落 NULL，得到 %+v", siteId)
	}
}

// TestCreateBuiltinTaskColumns 内置任务创建落库形态：task_type/payload 落值、插件字段恒 NULL、
// 状态 Created 根级；空类型拒绝（真实 FK 库锚定，与 CreateTask 同一锚定口径）
func TestCreateBuiltinTaskColumns(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	svc := NewService(NewRepository(db), nil, nil, nil, nil)
	ctx := context.Background()

	if _, err := svc.CreateBuiltinTask(ctx, "  ", "空类型", "{}"); err == nil {
		t.Fatal("空任务类型应拒绝")
	}

	created, err := svc.CreateBuiltinTask(ctx, "share-receive", "拉取分享", `{"schemaVersion":1}`)
	if err != nil {
		t.Fatalf("创建内置任务失败: %v", err)
	}
	if created.Status != int(TaskStatusCreated) {
		t.Fatalf("创建后状态应为 Created: %d", created.Status)
	}
	var taskType, payload, pluginID sql.NullString
	var pid sql.NullInt64
	if err := db.Raw("SELECT task_type, payload, plugin_public_id, pid FROM task WHERE id = ?", created.GetID()).
		Row().Scan(&taskType, &payload, &pluginID, &pid); err != nil {
		t.Fatalf("查内置任务列失败: %v", err)
	}
	if !taskType.Valid || taskType.String != "share-receive" {
		t.Errorf("task_type 落库不符: %+v", taskType)
	}
	if !payload.Valid || payload.String != `{"schemaVersion":1}` {
		t.Errorf("payload 落库不符: %+v", payload)
	}
	if pluginID.Valid {
		t.Errorf("内置任务 plugin_public_id 应落 NULL: %+v", pluginID)
	}
	if pid.Valid {
		t.Errorf("内置任务应为根级（pid NULL）: %+v", pid)
	}
}

// failOnNthCreateRepo 包装真实仓储：第 N 次 CreateTask 调用注入失败，用于事务原子性测试
// （父=call1，指定 failAt>1 即令第 failAt-1 个子任务创建失败）。
type failOnNthCreateRepo struct {
	*TaskRepository
	call   int
	failAt int
}

func (r *failOnNthCreateRepo) CreateTask(ctx context.Context, task *entity.Task) error {
	r.call++
	if r.call == r.failAt {
		return errors.New("注入子任务创建失败")
	}
	return r.TaskRepository.CreateTask(ctx, task)
}

// TestCreateBuiltinTaskTree 内置任务树创建：父容器（has_child=true、pid=NULL、task_type 落值）
// + N 子任务（pid=父ID、has_child=false、task_type/payload 落值、顺序即入参顺序）。内存仓储锚定结构。
func TestCreateBuiltinTaskTree(t *testing.T) {
	svc, repo := newTestService(t)
	ctx := context.Background()

	if _, err := svc.CreateBuiltinTaskTree(ctx, "  ", "父", []BuiltinTaskChild{{TaskName: "子"}}); err == nil {
		t.Fatal("空任务类型应拒绝")
	}
	if _, err := svc.CreateBuiltinTaskTree(ctx, "share-receive", "父", nil); err == nil {
		t.Fatal("空子任务应拒绝")
	}

	parent, err := svc.CreateBuiltinTaskTree(ctx, "share-receive", "拉取分享", []BuiltinTaskChild{
		{TaskName: "作品A", Payload: `{"manifestID":1}`},
		{TaskName: "作品B"},
	})
	if err != nil {
		t.Fatalf("创建内置任务树失败: %v", err)
	}
	if parent.Status != int(TaskStatusCreated) {
		t.Errorf("父任务状态应为 Created: %d", parent.Status)
	}
	if !parent.HasChild.Valid || !parent.HasChild.Bool {
		t.Errorf("父任务 has_child 应为 true 且 Valid: %+v", parent.HasChild)
	}
	if parent.Pid.Valid {
		t.Errorf("父任务应为根级（pid NULL）: %+v", parent.Pid)
	}
	if !parent.TaskType.Valid || parent.TaskType.String != "share-receive" {
		t.Errorf("父任务 task_type 落值不符: %+v", parent.TaskType)
	}
	if len(repo.tasks) != 3 {
		t.Fatalf("应落盘 3 行（父+2 子），得到 %d", len(repo.tasks))
	}

	parentID := parent.GetID()
	expected := []struct {
		name     string
		pidValid bool
		pid      int64
		hasChild bool
		payload  string
	}{
		{name: "拉取分享", pidValid: false, hasChild: true},
		{name: "作品A", pidValid: true, pid: parentID, payload: `{"manifestID":1}`},
		{name: "作品B", pidValid: true, pid: parentID},
	}
	for i, exp := range expected {
		task := repo.tasks[i]
		if task.TaskName.String != exp.name {
			t.Errorf("落盘顺序 %d 任务名不符: 期望 %q 得到 %q", i, exp.name, task.TaskName.String)
		}
		if task.Pid.Valid != exp.pidValid || (exp.pidValid && task.Pid.Int64 != exp.pid) {
			t.Errorf("落盘顺序 %d pid 不符: %+v（期望 valid=%v pid=%d）", i, task.Pid, exp.pidValid, exp.pid)
		}
		if !task.HasChild.Valid || task.HasChild.Bool != exp.hasChild {
			t.Errorf("落盘顺序 %d has_child 不符: %+v（期望 %v）", i, task.HasChild, exp.hasChild)
		}
		if !task.TaskType.Valid || task.TaskType.String != "share-receive" {
			t.Errorf("落盘顺序 %d task_type 不符: %+v", i, task.TaskType)
		}
		if task.Payload.String != exp.payload || task.Payload.Valid != (exp.payload != "") {
			t.Errorf("落盘顺序 %d payload 不符: %+v（期望 %q）", i, task.Payload, exp.payload)
		}
		if task.Status != int(TaskStatusCreated) {
			t.Errorf("落盘顺序 %d 状态应为 Created: %d", i, task.Status)
		}
	}
}

// TestCreateBuiltinTaskTreeColumns 内置任务树落库形态（真实 FK 库锚定）：
// 父行 has_child=1/pid NULL，子行 pid=父ID/has_child=0/task_type/payload 落值，子行按入参顺序排列。
func TestCreateBuiltinTaskTreeColumns(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	svc := NewService(NewRepository(db), &testTransactor{db: db}, nil, nil, nil)
	ctx := context.Background()

	parent, err := svc.CreateBuiltinTaskTree(ctx, "share-receive", "拉取分享", []BuiltinTaskChild{
		{TaskName: "作品A", Payload: `{"schemaVersion":1,"manifestID":1}`},
		{TaskName: "作品B", Payload: `{"schemaVersion":1,"manifestID":2}`},
	})
	if err != nil {
		t.Fatalf("创建内置任务树失败: %v", err)
	}
	parentID := parent.GetID()

	var hasChild int
	var pid sql.NullInt64
	if err := db.Raw("SELECT has_child, pid FROM task WHERE id = ?", parentID).Row().Scan(&hasChild, &pid); err != nil {
		t.Fatalf("查父任务列失败: %v", err)
	}
	if hasChild != 1 {
		t.Errorf("父任务 has_child 应落 1: %d", hasChild)
	}
	if pid.Valid {
		t.Errorf("父任务 pid 应落 NULL: %+v", pid)
	}

	rows, err := db.Raw("SELECT task_name, pid, has_child, task_type, payload, status FROM task WHERE pid = ? ORDER BY id", parentID).Rows()
	if err != nil {
		t.Fatalf("查子任务列失败: %v", err)
	}
	defer rows.Close()

	expectedChildren := []struct{ name, payload string }{
		{name: "作品A", payload: `{"schemaVersion":1,"manifestID":1}`},
		{name: "作品B", payload: `{"schemaVersion":1,"manifestID":2}`},
	}
	i := 0
	for rows.Next() {
		var taskName, taskType, payload sql.NullString
		var childPid sql.NullInt64
		var childHasChild, status int
		if err := rows.Scan(&taskName, &childPid, &childHasChild, &taskType, &payload, &status); err != nil {
			t.Fatalf("扫子任务行失败: %v", err)
		}
		if i >= len(expectedChildren) {
			t.Fatalf("子任务行数超出预期")
		}
		exp := expectedChildren[i]
		if taskName.String != exp.name {
			t.Errorf("子任务 %d 任务名不符: 期望 %q 得到 %q", i, exp.name, taskName.String)
		}
		if !childPid.Valid || childPid.Int64 != parentID {
			t.Errorf("子任务 %d pid 应指向父 %d: %+v", i, parentID, childPid)
		}
		if childHasChild != 0 {
			t.Errorf("子任务 %d has_child 应落 0: %d", i, childHasChild)
		}
		if !taskType.Valid || taskType.String != "share-receive" {
			t.Errorf("子任务 %d task_type 不符: %+v", i, taskType)
		}
		if payload.String != exp.payload {
			t.Errorf("子任务 %d payload 不符: 期望 %q 得到 %q", i, exp.payload, payload.String)
		}
		if status != int(TaskStatusCreated) {
			t.Errorf("子任务 %d 状态应为 Created: %d", i, status)
		}
		i++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("遍历子任务行出错: %v", err)
	}
	if i != len(expectedChildren) {
		t.Errorf("子任务行数不符: 期望 %d 得到 %d", len(expectedChildren), i)
	}
}

// TestCreateBuiltinTaskTreeRollback 事务原子性：子任务创建失败时父行整体回滚，不留孤儿任务。
func TestCreateBuiltinTaskTreeRollback(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	// 父=call1，第一个子任务（call2）注入失败
	failRepo := &failOnNthCreateRepo{TaskRepository: NewRepository(db), failAt: 2}
	svc := NewService(failRepo, &testTransactor{db: db}, nil, nil, nil)
	ctx := context.Background()

	if _, err := svc.CreateBuiltinTaskTree(ctx, "share-receive", "拉取分享", []BuiltinTaskChild{
		{TaskName: "作品A", Payload: "{}"},
		{TaskName: "作品B", Payload: "{}"},
	}); err == nil {
		t.Fatal("子任务创建失败应返回错误")
	}

	var count int64
	if err := db.Model(&entity.Task{}).Count(&count).Error; err != nil {
		t.Fatalf("统计任务行失败: %v", err)
	}
	if count != 0 {
		t.Errorf("子任务创建失败后事务应整体回滚（无残留任务行），得到 %d", count)
	}
}
