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
	testSiteKey  = "pixiv" // 与 identity.Pixiv 键一致的种子值（不走注册表校验，任务路径仅按键查库）
	testSiteID   = int64(100)
	testPluginID = "plugin-1"
	testExtID    = "ext-1"
)

// fakeTaskRepo 记录 CreateTask/CreateBatch 创建的核心行与成对创建的作品领域行并模拟自增主键——
// parent→child Pid 链接（parent.GetID()）依赖 CreateTask 回填 ID。
// 其余 Repository 方法用 nil 接口嵌入满足签名：创建路径仅触达 CreateTask/CreateBatch 与
// CreateWorkTaskForTask/CreateWorkTaskBatch。
type fakeTaskRepo struct {
	Repository
	mu        sync.Mutex
	tasks     []*entity.Task
	workTasks map[int64]*entity.WorkTask
	nextID    int64
}

func newFakeTaskRepo() *fakeTaskRepo {
	return &fakeTaskRepo{workTasks: make(map[int64]*entity.WorkTask), nextID: 1}
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

func (f *fakeTaskRepo) CreateWorkTaskForTask(_ context.Context, taskID int64, wt *entity.WorkTask) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	wt.SetID(taskID)
	f.workTasks[taskID] = wt
	return nil
}

func (f *fakeTaskRepo) CreateWorkTaskBatch(_ context.Context, wts []*entity.WorkTask) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, wt := range wts {
		f.workTasks[wt.GetID()] = wt
	}
	return nil
}

// workTaskOf 取核心行 id 对应的作品领域行（无则 nil）
func (f *fakeTaskRepo) workTaskOf(id int64) *entity.WorkTask {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.workTasks[id]
}

// fakeSiteRepo 让 site.Service.GetByKey 回填固定站点 ID（创建路径经 siteSvc.GetByKey→repo.GetByKey）。
type fakeSiteRepo struct {
	site.Repository
}

func (fakeSiteRepo) GetByKey(_ context.Context, _ string) (*entity.Site, error) {
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
	siteSvc := site.NewService(fakeSiteRepo{}) // 站点服务仅消费查询（创建路径）
	svc := NewService(repo, fakeTransactor{}, nil, nil, siteSvc, nil)
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

// ---- 共享断言：leaf 任务字段（核心行 + 作品领域行成对） ----

func assertLeafTask(t *testing.T, leaf *entity.Task, wt *entity.WorkTask) {
	t.Helper()
	if leaf.HasChild.Valid && leaf.HasChild.Bool {
		t.Errorf("leaf HasChild 应为 false，得到 true")
	}
	// 统一契约：leaf 无父写 pid=NULL（Valid=false），与 parent 的根级形态一致（外键下无 id=0 行，0 必违约）
	if leaf.Pid.Valid {
		t.Errorf("leaf Pid 应为 NULL（Valid=false，根级），得到 %+v", leaf.Pid)
	}
	if !leaf.TaskType.Valid || leaf.TaskType.String != entity.TaskTypePluginDownload {
		t.Errorf("leaf TaskType 应为插件下载类型 %q，得到 %+v", entity.TaskTypePluginDownload, leaf.TaskType)
	}
	if wt == nil {
		t.Fatal("插件任务应有成对的作品领域行")
	}
	if !wt.SiteID.Valid || wt.SiteID.Int64 != testSiteID {
		t.Errorf("SiteID 回填应为 %d，得到 %+v", testSiteID, wt.SiteID)
	}
	if !wt.PluginPublicID.Valid || wt.PluginPublicID.String != testPluginID {
		t.Errorf("PluginPublicID 回填错误，得到 %+v", wt.PluginPublicID)
	}
	if !wt.PluginExtensionID.Valid || wt.PluginExtensionID.String != testExtID {
		t.Errorf("PluginExtensionID 回填错误，得到 %+v", wt.PluginExtensionID)
	}
	// 1:1 共享主键：领域行 id 与核心行 id 严格同值
	if wt.GetID() != leaf.GetID() {
		t.Errorf("作品领域行 id %d 应与核心行 id %d 同值", wt.GetID(), leaf.GetID())
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
		SiteKey:      testSiteKey,
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
	assertLeafTask(t, leaf, repo.workTaskOf(leaf.GetID()))
	if wt := repo.workTaskOf(leaf.GetID()); !wt.ResourceType.Valid || wt.ResourceType.String != entity.ResourceTypeImage {
		t.Errorf("ResourceType 回填错误，得到 %+v", wt.ResourceType)
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
		SiteKey:      testSiteKey,
		ResourceType: entity.ResourceTypeImage,
		Children: []*sdkdto.TaskCreateChildResponse{
			{TaskName: "p1", SiteWorkId: "c-1", Url: "http://x/1", ResourceType: entity.ResourceTypeImage},
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
		SiteKey:      testSiteKey,
		ResourceType: entity.ResourceTypeImage,
		Children: []*sdkdto.TaskCreateChildResponse{
			{TaskName: "p1", SiteWorkId: "c-1", Url: "http://x/1", ResourceType: entity.ResourceTypeImage},
			{TaskName: "p2", SiteWorkId: "c-2", Url: "http://x/2", ResourceType: entity.ResourceTypeImage},
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
		PluginTaskId: "work-X", TaskName: "work-X", Url: "http://x", SiteKey: testSiteKey,
		Children: []*sdkdto.TaskCreateChildResponse{
			{TaskName: "p1", SiteWorkId: "c-1", Url: "http://x/1", ResourceType: entity.ResourceTypeImage},
			{TaskName: "p2", SiteWorkId: "c-2", Url: "http://x/2", ResourceType: entity.ResourceTypeImage},
		},
	}
	in <- &sdkdto.TaskCreateResponse{
		PluginTaskId: "work-X", TaskName: "work-X", Url: "http://x", SiteKey: testSiteKey,
		Children: []*sdkdto.TaskCreateChildResponse{
			{TaskName: "p3", SiteWorkId: "c-3", Url: "http://x/3", ResourceType: entity.ResourceTypeImage},
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
		PluginTaskId: "work-A", TaskName: "work-A", Url: "http://x/a", SiteKey: testSiteKey,
		Children: []*sdkdto.TaskCreateChildResponse{
			{TaskName: "p1", SiteWorkId: "c-1", Url: "http://x/1", ResourceType: entity.ResourceTypeImage},
		},
	}
	in <- &sdkdto.TaskCreateResponse{
		PluginTaskId: "work-B", TaskName: "work-B", Url: "http://x/b", SiteKey: testSiteKey,
		Children: []*sdkdto.TaskCreateChildResponse{
			{TaskName: "p2", SiteWorkId: "c-2", Url: "http://x/2", ResourceType: entity.ResourceTypeImage},
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
			SiteKey:      testSiteKey,
			ResourceType: entity.ResourceTypeImage,
			// 无 Children
		},
	}

	count, failed, err := svc.handleCreateTaskArray(context.Background(), responses, testListener())
	if err != nil {
		t.Fatalf("handleCreateTaskArray 返回错误: %v", err)
	}
	if count != 1 || failed != 0 {
		t.Fatalf("期望返回叶子单元计数 1、失败 0，得到 count=%d failed=%d", count, failed)
	}
	if len(repo.tasks) != 1 {
		t.Fatalf("期望落盘 1 个 leaf 任务，得到 %d 个", len(repo.tasks))
	}

	assertLeafTask(t, repo.tasks[0], repo.workTaskOf(repo.tasks[0].GetID()))
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
			SiteKey:  testSiteKey,
			Children: []*sdkdto.TaskCreateChildResponse{
				{TaskName: "p1", SiteWorkId: "c-1", Url: "http://x/1", ResourceType: entity.ResourceTypeImage},
			},
		},
	}

	count, failed, err := svc.handleCreateTaskArray(context.Background(), responses, testListener())
	if err != nil {
		t.Fatalf("handleCreateTaskArray 返回错误: %v", err)
	}
	if count != 1 || failed != 0 {
		t.Fatalf("期望返回叶子单元计数 1（1 child）、失败 0，得到 count=%d failed=%d", count, failed)
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
	// 子任务身份字段取自 child 响应（领域行承载）
	for _, tk := range repo.tasks {
		if tk.HasChild.Valid && tk.HasChild.Bool {
			continue
		}
		if !tk.TaskName.Valid || tk.TaskName.String != "p1" {
			t.Errorf("子任务 TaskName 应为 p1，得到 %+v", tk.TaskName)
		}
		wt := repo.workTaskOf(tk.GetID())
		if wt == nil || !wt.SiteWorkID.Valid || wt.SiteWorkID.String != "c-1" {
			t.Errorf("子任务领域行 SiteWorkID 应为 c-1，得到 %+v", wt)
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
			SiteKey:      testSiteKey,
			ResourceType: entity.ResourceTypeImage,
			Children: []*sdkdto.TaskCreateChildResponse{
				{TaskName: "p1", SiteWorkId: "c-1", Url: "http://x/1", ResourceType: entity.ResourceTypeImage},
				{TaskName: "p2", SiteWorkId: "c-2", Url: "http://x/2", ResourceType: entity.ResourceTypeImage},
			},
		},
	}

	count, failed, err := svc.handleCreateTaskArray(context.Background(), responses, testListener())
	if err != nil {
		t.Fatalf("handleCreateTaskArray 返回错误: %v", err)
	}
	if count != 2 || failed != 0 {
		t.Fatalf("期望返回叶子单元计数 2（2 children）、失败 0，得到 count=%d failed=%d", count, failed)
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

	// 站点种子（task.site_id 外键防线；插件响应路径经 GetByKey 查到本行）
	seedSite := entity.NewSite()
	seedSite.SiteKey = testSiteKey
	seedSite.SiteName = sql.NullString{String: testSiteName, Valid: true}
	if err := db.Create(seedSite).Error; err != nil {
		t.Fatalf("建站点种子失败: %v", err)
	}

	// 经生产构造函数组装（真实 task 仓储 + 真事务执行器 + 真实 site 服务）
	siteSvc := site.NewService(site.NewRepository(db))
	wtStore := newTestWorkTaskStore(db)
	svc := NewService(NewRepository(db, wtStore, wtStore), &testTransactor{db: db}, nil, nil, siteSvc, nil)
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
		{TaskName: "独立leaf", SiteWorkId: "w-1", Url: "http://x/1", SiteKey: testSiteKey, ResourceType: entity.ResourceTypeImage},
		{
			TaskName: "work-A", Url: "http://x/a", SiteKey: testSiteKey, ResourceType: entity.ResourceTypeImage,
			Children: []*sdkdto.TaskCreateChildResponse{
				{TaskName: "p1", SiteWorkId: "c-1", Url: "http://x/c1", ResourceType: entity.ResourceTypeImage},
			},
		},
	}
	if _, _, err := svc.handleCreateTaskArray(ctx, responses, testListener()); err != nil {
		t.Fatalf("插件响应路径创建失败: %v", err)
	}

	// pid 落库形态断言（直接查列，不经实体扫描；site_work_id 在作品领域行，经共享主键关联查）
	pidOf := func(where string, args ...any) sql.NullInt64 {
		t.Helper()
		var pid sql.NullInt64
		if err := db.Raw("SELECT pid FROM task WHERE "+where, args...).Scan(&pid).Error; err != nil {
			t.Fatalf("查 pid 失败(%s): %v", where, err)
		}
		return pid
	}
	taskIDBySiteWorkID := func(siteWorkID string) int64 {
		t.Helper()
		var id int64
		if err := db.Raw("SELECT id FROM work_task WHERE site_work_id = ?", siteWorkID).Scan(&id).Error; err != nil {
			t.Fatalf("按 site_work_id 查任务 id 失败(%s): %v", siteWorkID, err)
		}
		return id
	}
	if pid := pidOf("id = ?", root.GetID()); pid.Valid {
		t.Errorf("CreateTask 根级任务 pid 应落 NULL，得到 %+v", pid)
	}
	if pid := pidOf("id = ?", child.GetID()); !pid.Valid || pid.Int64 != root.GetID() {
		t.Errorf("CreateTask 子任务 pid 应落父 ID %d，得到 %+v", root.GetID(), pid)
	}
	if pid := pidOf("id = ?", taskIDBySiteWorkID("w-1")); pid.Valid {
		t.Errorf("插件路径独立 leaf pid 应落 NULL，得到 %+v", pid)
	}
	if pid := pidOf("task_name = ?", "work-A"); pid.Valid {
		t.Errorf("插件路径 parent 容器 pid 应落 NULL，得到 %+v", pid)
	}
	var parentId int64
	if err := db.Raw("SELECT id FROM task WHERE task_name = ?", "work-A").Scan(&parentId).Error; err != nil {
		t.Fatalf("查 parent id 失败: %v", err)
	}
	if pid := pidOf("id = ?", taskIDBySiteWorkID("c-1")); !pid.Valid || pid.Int64 != parentId {
		t.Errorf("插件路径子任务 pid 应落父 ID %d，得到 %+v", parentId, pid)
	}

	// site_id 落库形态断言（直接查列，不经实体扫描；站点归属列在作品领域行）
	siteIdOf := func(where string, args ...any) sql.NullInt64 {
		t.Helper()
		var siteId sql.NullInt64
		if err := db.Raw("SELECT site_id FROM work_task WHERE "+where, args...).Scan(&siteId).Error; err != nil {
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

	// 1:1 共享主键锚定：全部插件下载任务（task_type=plugin-download）均有同值 id 的作品领域行
	var missing, mismatched int64
	if err := db.Raw(`SELECT COUNT(*) FROM task t
		WHERE t.task_type = ? AND NOT EXISTS (SELECT 1 FROM work_task w WHERE w.id = t.id)`,
		entity.TaskTypePluginDownload).Scan(&missing).Error; err != nil {
		t.Fatalf("统计缺失领域行的任务失败: %v", err)
	}
	if missing != 0 {
		t.Errorf("插件下载任务应有 %d 个缺失作品领域行", missing)
	}
	if err := db.Raw(`SELECT COUNT(*) FROM work_task w JOIN task t ON t.id = w.id`).Scan(&mismatched).Error; err != nil {
		t.Fatalf("统计领域行失败: %v", err)
	}
	if mismatched != 6 {
		t.Errorf("期望 6 个作品领域行（CreateTask×3 + 插件响应×3），得到 %d", mismatched)
	}
}

// TestCreateBuiltinTaskColumns 内置任务创建落库形态：task_type 落值、无作品领域行、
// 状态 Created 根级；空类型拒绝（真实 FK 库锚定，与 CreateTask 同一锚定口径）
func TestCreateBuiltinTaskColumns(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	wtStore := newTestWorkTaskStore(db)
	svc := NewService(NewRepository(db, wtStore, wtStore), nil, nil, nil, nil, nil)
	ctx := context.Background()

	if _, err := svc.CreateBuiltinTask(ctx, "  ", "空类型"); err == nil {
		t.Fatal("空任务类型应拒绝")
	}

	created, err := svc.CreateBuiltinTask(ctx, "share-receive", "拉取分享")
	if err != nil {
		t.Fatalf("创建内置任务失败: %v", err)
	}
	if created.Status != int(TaskStatusCreated) {
		t.Fatalf("创建后状态应为 Created: %d", created.Status)
	}
	var taskType sql.NullString
	var pid sql.NullInt64
	if err := db.Raw("SELECT task_type, pid FROM task WHERE id = ?", created.GetID()).
		Row().Scan(&taskType, &pid); err != nil {
		t.Fatalf("查内置任务列失败: %v", err)
	}
	if !taskType.Valid || taskType.String != "share-receive" {
		t.Errorf("task_type 落库不符: %+v", taskType)
	}
	if pid.Valid {
		t.Errorf("内置任务应为根级（pid NULL）: %+v", pid)
	}
	// 内置类型任务不建作品领域行
	var workRows int64
	if err := db.Raw("SELECT COUNT(*) FROM work_task WHERE id = ?", created.GetID()).Scan(&workRows).Error; err != nil {
		t.Fatalf("统计作品领域行失败: %v", err)
	}
	if workRows != 0 {
		t.Errorf("内置任务不应有作品领域行，得到 %d 行", workRows)
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
		{TaskName: "作品A"},
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
	}{
		{name: "拉取分享", pidValid: false, hasChild: true},
		{name: "作品A", pidValid: true, pid: parentID},
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
		if task.Status != int(TaskStatusCreated) {
			t.Errorf("落盘顺序 %d 状态应为 Created: %d", i, task.Status)
		}
		if repo.workTaskOf(task.GetID()) != nil {
			t.Errorf("落盘顺序 %d 内置任务不应有作品领域行", i)
		}
	}
}

// TestCreateBuiltinTaskTreeColumns 内置任务树落库形态（真实 FK 库锚定）：
// 父行 has_child=1/pid NULL，子行 pid=父ID/has_child=0/task_type 落值，子行按入参顺序排列。
func TestCreateBuiltinTaskTreeColumns(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	wtStore := newTestWorkTaskStore(db)
	svc := NewService(NewRepository(db, wtStore, wtStore), &testTransactor{db: db}, nil, nil, nil, nil)
	ctx := context.Background()

	parent, err := svc.CreateBuiltinTaskTree(ctx, "share-receive", "拉取分享", []BuiltinTaskChild{
		{TaskName: "作品A"},
		{TaskName: "作品B"},
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

	rows, err := db.Raw("SELECT task_name, pid, has_child, task_type, status FROM task WHERE pid = ? ORDER BY id", parentID).Rows()
	if err != nil {
		t.Fatalf("查子任务列失败: %v", err)
	}
	defer rows.Close()

	expectedChildren := []string{"作品A", "作品B"}
	i := 0
	for rows.Next() {
		var taskName, taskType sql.NullString
		var childPid sql.NullInt64
		var childHasChild, status int
		if err := rows.Scan(&taskName, &childPid, &childHasChild, &taskType, &status); err != nil {
			t.Fatalf("扫子任务行失败: %v", err)
		}
		if i >= len(expectedChildren) {
			t.Fatalf("子任务行数超出预期")
		}
		if taskName.String != expectedChildren[i] {
			t.Errorf("子任务 %d 任务名不符: 期望 %q 得到 %q", i, expectedChildren[i], taskName.String)
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
	wtStore := newTestWorkTaskStore(db)
	failRepo := &failOnNthCreateRepo{TaskRepository: NewRepository(db, wtStore, wtStore), failAt: 2}
	svc := NewService(failRepo, &testTransactor{db: db}, nil, nil, nil, nil)
	ctx := context.Background()

	if _, err := svc.CreateBuiltinTaskTree(ctx, "share-receive", "拉取分享", []BuiltinTaskChild{
		{TaskName: "作品A"},
		{TaskName: "作品B"},
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

// ---- 站点键解析（任务创建主路径按 site_key 直查站点行）----

// TestTaskCreateResolvesSiteByKey 插件应答的 siteKey 经 GetByKey 解析为 task.SiteID：
// 独立 leaf 与 parent+children 均按键归属站点；child 应答无键字段，站点归属继承父应答的键。
// 真实 FK 库锚定（site_id 外键防线，站点行须真实存在）。
func TestTaskCreateResolvesSiteByKey(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}

	seed := entity.NewSite()
	seed.SiteKey = testSiteKey
	seed.SiteName = sql.NullString{String: testSiteName, Valid: true}
	if err := db.Create(seed).Error; err != nil {
		t.Fatalf("建站点种子失败: %v", err)
	}

	siteSvc := site.NewService(site.NewRepository(db))
	wtStore := newTestWorkTaskStore(db)
	svc := NewService(NewRepository(db, wtStore, wtStore), &testTransactor{db: db}, nil, nil, siteSvc, nil)

	responses := []*sdkdto.TaskCreateResponse{
		{TaskName: "leaf-1", SiteWorkId: "k-1", Url: "http://x/1", SiteKey: testSiteKey, ResourceType: entity.ResourceTypeImage},
		{
			TaskName: "parent-1", Url: "http://x/p", SiteKey: testSiteKey, ResourceType: entity.ResourceTypeImage,
			Children: []*sdkdto.TaskCreateChildResponse{
				{TaskName: "c-1", SiteWorkId: "k-2", Url: "http://x/2", ResourceType: entity.ResourceTypeImage},
			},
		},
	}
	if _, _, err := svc.handleCreateTaskArray(context.Background(), responses, testListener()); err != nil {
		t.Fatalf("插件响应路径创建失败: %v", err)
	}

	var got int64
	if err := db.Raw(`SELECT COUNT(*) FROM task t LEFT JOIN work_task w ON w.id = t.id
		WHERE w.site_id IS NULL OR w.site_id != ?`, seed.GetID()).Scan(&got).Error; err != nil {
		t.Fatalf("统计任务行失败: %v", err)
	}
	if got != 0 {
		t.Errorf("期望全部 3 个任务（leaf + parent + child）site_id 均解析为种子站点 %d，得到 %d 个未归属行", seed.GetID(), got)
	}
}

// nilSiteRepo 站点查询恒空（按键查不到行），供键解析失败测试使用。
type nilSiteRepo struct {
	site.Repository
}

func (nilSiteRepo) GetByKey(_ context.Context, _ string) (*entity.Site, error) {
	return nil, nil
}

// TestTaskCreateUnknownKeyFails 站点键解析失败的两种形态：键在库中查不到（未注册/未建行）报
// ErrSiteNotFound 同型错误；键缺失报 ErrSiteKeyRequired。失败在字段填充阶段前置暴露，任务不落盘。
func TestTaskCreateUnknownKeyFails(t *testing.T) {
	siteSvc := site.NewService(nilSiteRepo{})
	svc := NewService(newFakeTaskRepo(), fakeTransactor{}, nil, nil, siteSvc, nil)
	siteCache := make(map[string]int)

	// 键查不到行：ErrSiteNotFound 同型错误（leaf 路径）
	_, err := svc.planCreateResponse(context.Background(),
		&sdkdto.TaskCreateResponse{TaskName: "t", Url: "http://x", SiteKey: "s-not-exist", ResourceType: entity.ResourceTypeImage},
		testListener(), siteCache)
	if !errors.Is(err, ErrSiteNotFound) {
		t.Fatalf("未知站点键应报 ErrSiteNotFound 同型错误，得到 %v", err)
	}

	// 键缺失：ErrSiteKeyRequired（child 继承父键，父键缺失对 children 同样生效）
	_, err = svc.planCreateResponse(context.Background(),
		&sdkdto.TaskCreateResponse{TaskName: "t", Url: "http://x", ResourceType: entity.ResourceTypeImage,
			Children: []*sdkdto.TaskCreateChildResponse{
				{TaskName: "c", SiteWorkId: "k", Url: "http://x/1", ResourceType: entity.ResourceTypeImage},
			}},
		testListener(), siteCache)
	if !errors.Is(err, ErrSiteKeyRequired) {
		t.Fatalf("缺失站点键应报 ErrSiteKeyRequired，得到 %v", err)
	}
}

// ---- CreateTaskByURL：失败即终止 + 原因/兜底文案 + 失败计数 ----

// fakeTaskHandlerGetter TaskHandlerProvider 替身：按 (publicId, extensionId) 返回预置处理器，
// 未预置的键返回错误（模拟插件未激活/处理器不可用）。
type fakeTaskHandlerGetter struct {
	handlers map[string]sdkdto.TaskHandler
}

func (f *fakeTaskHandlerGetter) GetTaskHandler(pluginPublicId, extensionId string) (sdkdto.TaskHandler, error) {
	h, ok := f.handlers[pluginPublicId+"/"+extensionId]
	if !ok {
		return nil, errors.New("task handler not found")
	}
	return h, nil
}

// fakePluginTaskHandler 仅实现 Create 的任务处理器替身；其余方法经接口嵌入满足签名，创建路径不触达。
// createCalls 记录 Create 被调用次数，供「首个失败即终止不轮询」断言。
type fakePluginTaskHandler struct {
	sdkdto.TaskHandler
	createCalls int
	create      func(url string) (*sdkdto.TaskCreateResult, error)
}

func (f *fakePluginTaskHandler) Create(url string) (*sdkdto.TaskCreateResult, error) {
	f.createCalls++
	return f.create(url)
}

// namedListener 构造带插件名与 PublicID 的监听器条目（提示文案按插件名点名插件）。
func namedListener(publicId, name, extId string) *pluginTaskUrlListener.PluginWithExtension {
	plugin := entity.NewPlugin()
	plugin.PublicID = sql.NullString{String: publicId, Valid: true}
	plugin.Name = sql.NullString{String: name, Valid: true}
	return &pluginTaskUrlListener.PluginWithExtension{Plugin: plugin, ExtensionID: extId}
}

// newURLListenerService 把监听器条目注册到同一匹配模式（同模式内按注册序返回，顺序确定）。
func newURLListenerService(entries ...*pluginTaskUrlListener.PluginWithExtension) *pluginTaskUrlListener.Service {
	svc := pluginTaskUrlListener.NewService(pluginTaskUrlListener.NewManager())
	for _, e := range entries {
		svc.Register(e, []string{"^http"})
	}
	return svc
}

// newCreateByURLService 经生产构造函数组装带处理器提供者与监听器服务的 Service（fake 落库）。
func newCreateByURLService(t *testing.T, getter TaskHandlerProvider, listenerSvc *pluginTaskUrlListener.Service) (*Service, *fakeTaskRepo) {
	t.Helper()
	repo := newFakeTaskRepo()
	siteSvc := site.NewService(fakeSiteRepo{})
	svc := NewService(repo, fakeTransactor{}, getter, listenerSvc, siteSvc, nil)
	return svc, repo
}

// batchResultWithReason 构造带原因的批量结果（reason 空则不声明）。
func batchResultWithReason(responses []*sdkdto.TaskCreateResponse, reason string) *sdkdto.TaskCreateResult {
	result := sdkdto.BatchResult(responses)
	if reason != "" {
		result.SetReason(reason)
	}
	return result
}

// viableHandler 可正常创建 1 个独立任务的处理器替身，用于「失败后不轮询后续监听器」的对照侧。
func viableHandler() *fakePluginTaskHandler {
	return &fakePluginTaskHandler{create: func(string) (*sdkdto.TaskCreateResult, error) {
		return batchResultWithReason([]*sdkdto.TaskCreateResponse{
			{TaskName: "t", SiteWorkId: "w", Url: "http://x", SiteKey: testSiteKey, ResourceType: entity.ResourceTypeImage},
		}, ""), nil
	}}
}

// TestCreateTaskByURL_PluginReasonPassedThrough 零任务 + reason → 提示点名插件并透传插件业务原因。
func TestCreateTaskByURL_PluginReasonPassedThrough(t *testing.T) {
	handler := &fakePluginTaskHandler{create: func(string) (*sdkdto.TaskCreateResult, error) {
		return batchResultWithReason(nil, "获取令牌失败（限制级作品需要登录）"), nil
	}}
	svc, _ := newCreateByURLService(t,
		&fakeTaskHandlerGetter{handlers: map[string]sdkdto.TaskHandler{"pub-a/ext-a": handler}},
		newURLListenerService(namedListener("pub-a", "插件A", "ext-a")))

	resp, err := svc.CreateTaskByURL(context.Background(), "http://x/1")
	if err != nil {
		t.Fatalf("CreateTaskByURL 返回错误: %v", err)
	}
	if resp.Succeed {
		t.Fatal("零任务不应标记成功")
	}
	if want := "插件 插件A 未创建任务：获取令牌失败（限制级作品需要登录）"; resp.Msg != want {
		t.Fatalf("期望 Msg %q，得到 %q", want, resp.Msg)
	}
}

// TestCreateTaskByURL_FallbackWhenNoTasksNoReason 零任务且无原因 → 兜底文案点名插件。
func TestCreateTaskByURL_FallbackWhenNoTasksNoReason(t *testing.T) {
	handler := &fakePluginTaskHandler{create: func(string) (*sdkdto.TaskCreateResult, error) {
		return sdkdto.BatchResult(nil), nil
	}}
	svc, _ := newCreateByURLService(t,
		&fakeTaskHandlerGetter{handlers: map[string]sdkdto.TaskHandler{"pub-a/ext-a": handler}},
		newURLListenerService(namedListener("pub-a", "插件A", "ext-a")))

	resp, err := svc.CreateTaskByURL(context.Background(), "http://x/1")
	if err != nil {
		t.Fatalf("CreateTaskByURL 返回错误: %v", err)
	}
	if want := "插件 插件A 未返回任务，也未说明原因"; resp.Msg != want {
		t.Fatalf("期望兜底 Msg %q，得到 %q", want, resp.Msg)
	}
}

// TestCreateTaskByURL_HandlerUnavailableStopsPolling 处理器不可用 → 终止并提示，不轮询后续监听器。
func TestCreateTaskByURL_HandlerUnavailableStopsPolling(t *testing.T) {
	viable := viableHandler()
	svc, _ := newCreateByURLService(t,
		&fakeTaskHandlerGetter{handlers: map[string]sdkdto.TaskHandler{"pub-b/ext-b": viable}},
		newURLListenerService(
			namedListener("pub-a", "插件A", "ext-a"),
			namedListener("pub-b", "插件B", "ext-b"),
		))

	resp, err := svc.CreateTaskByURL(context.Background(), "http://x/1")
	if err != nil {
		t.Fatalf("CreateTaskByURL 返回错误: %v", err)
	}
	if want := "插件 插件A 未激活或任务处理器不可用"; resp.Msg != want {
		t.Fatalf("期望 Msg %q，得到 %q", want, resp.Msg)
	}
	if viable.createCalls != 0 {
		t.Fatalf("处理器不可用应终止，后续监听器不应被调用，得到 createCalls=%d", viable.createCalls)
	}
}

// TestCreateTaskByURL_CreateErrStopsPolling Create 返回 gRPC 层错误（基础设施故障）→
// 中性措辞提示点名插件，不轮询后续监听器。
func TestCreateTaskByURL_CreateErrStopsPolling(t *testing.T) {
	handler := &fakePluginTaskHandler{create: func(string) (*sdkdto.TaskCreateResult, error) {
		return nil, errors.New("connection refused")
	}}
	viable := viableHandler()
	svc, _ := newCreateByURLService(t,
		&fakeTaskHandlerGetter{handlers: map[string]sdkdto.TaskHandler{
			"pub-a/ext-a": handler,
			"pub-b/ext-b": viable,
		}},
		newURLListenerService(
			namedListener("pub-a", "插件A", "ext-a"),
			namedListener("pub-b", "插件B", "ext-b"),
		))

	resp, err := svc.CreateTaskByURL(context.Background(), "http://x/1")
	if err != nil {
		t.Fatalf("CreateTaskByURL 返回错误: %v", err)
	}
	if want := "插件 插件A 创建任务失败（异常退出或连接中断）：connection refused"; resp.Msg != want {
		t.Fatalf("期望 Msg %q，得到 %q", want, resp.Msg)
	}
	if viable.createCalls != 0 {
		t.Fatalf("插件错误返回应终止，后续监听器不应被调用，得到 createCalls=%d", viable.createCalls)
	}
}

// TestCreateTaskByURL_FirstFailureStopsPolling 首个监听器零任务且给出原因 → 以其原因返回，
// 不再尝试后续监听器（首个明确结果即终止）。
func TestCreateTaskByURL_FirstFailureStopsPolling(t *testing.T) {
	reasoned := &fakePluginTaskHandler{create: func(string) (*sdkdto.TaskCreateResult, error) {
		return batchResultWithReason(nil, "未发现可导入的文件（目录为空）"), nil
	}}
	viable := viableHandler()
	svc, _ := newCreateByURLService(t,
		&fakeTaskHandlerGetter{handlers: map[string]sdkdto.TaskHandler{
			"pub-a/ext-a": reasoned,
			"pub-b/ext-b": viable,
		}},
		newURLListenerService(
			namedListener("pub-a", "插件A", "ext-a"),
			namedListener("pub-b", "插件B", "ext-b"),
		))

	resp, err := svc.CreateTaskByURL(context.Background(), "http://x/1")
	if err != nil {
		t.Fatalf("CreateTaskByURL 返回错误: %v", err)
	}
	if want := "插件 插件A 未创建任务：未发现可导入的文件（目录为空）"; resp.Msg != want {
		t.Fatalf("期望 Msg %q，得到 %q", want, resp.Msg)
	}
	if viable.createCalls != 0 {
		t.Fatalf("首个失败即终止，后续监听器不应被调用，得到 createCalls=%d", viable.createCalls)
	}
}

// TestCreateTaskByURL_SuccessMsgFromBackend 全部落库成功 → 成功文案由后端 Msg 产出。
func TestCreateTaskByURL_SuccessMsgFromBackend(t *testing.T) {
	handler := &fakePluginTaskHandler{create: func(string) (*sdkdto.TaskCreateResult, error) {
		return batchResultWithReason([]*sdkdto.TaskCreateResponse{
			{TaskName: "t-1", SiteWorkId: "w-1", Url: "http://x/1", SiteKey: testSiteKey, ResourceType: entity.ResourceTypeImage},
			{TaskName: "t-2", SiteWorkId: "w-2", Url: "http://x/2", SiteKey: testSiteKey, ResourceType: entity.ResourceTypeImage},
		}, ""), nil
	}}
	svc, repo := newCreateByURLService(t,
		&fakeTaskHandlerGetter{handlers: map[string]sdkdto.TaskHandler{"pub-a/ext-a": handler}},
		newURLListenerService(namedListener("pub-a", "插件A", "ext-a")))

	resp, err := svc.CreateTaskByURL(context.Background(), "http://x/1")
	if err != nil {
		t.Fatalf("CreateTaskByURL 返回错误: %v", err)
	}
	if !resp.Succeed || resp.AddedQuantity != 2 {
		t.Fatalf("期望 Succeed=true AddedQuantity=2，得到 Succeed=%v AddedQuantity=%d", resp.Succeed, resp.AddedQuantity)
	}
	if want := "成功创建 2 个任务"; resp.Msg != want {
		t.Fatalf("期望 Msg %q，得到 %q", want, resp.Msg)
	}
	if len(repo.tasks) != 2 {
		t.Fatalf("期望落盘 2 个任务，得到 %d 个", len(repo.tasks))
	}
}

// TestCreateTaskByURL_ArrayPartialFailureCounted 批量路径部分失败：成功项与失败项（填充失败的响应）
// 分开计数，插件报告的原因（业务维度）追加在失败数（落库维度）之后。
func TestCreateTaskByURL_ArrayPartialFailureCounted(t *testing.T) {
	handler := &fakePluginTaskHandler{create: func(string) (*sdkdto.TaskCreateResult, error) {
		return batchResultWithReason([]*sdkdto.TaskCreateResponse{
			{TaskName: "ok-1", SiteWorkId: "w-1", Url: "http://x/1", SiteKey: testSiteKey, ResourceType: entity.ResourceTypeImage},
			{TaskName: "bad", Url: "http://x/2", SiteKey: "", ResourceType: entity.ResourceTypeImage}, // SiteKey 缺失 → 字段填充失败
			{TaskName: "ok-2", SiteWorkId: "w-2", Url: "http://x/3", SiteKey: testSiteKey, ResourceType: entity.ResourceTypeImage},
		}, "部分任务创建失败"), nil
	}}
	svc, repo := newCreateByURLService(t,
		&fakeTaskHandlerGetter{handlers: map[string]sdkdto.TaskHandler{"pub-a/ext-a": handler}},
		newURLListenerService(namedListener("pub-a", "插件A", "ext-a")))

	resp, err := svc.CreateTaskByURL(context.Background(), "http://x/1")
	if err != nil {
		t.Fatalf("CreateTaskByURL 返回错误: %v", err)
	}
	if !resp.Succeed || resp.AddedQuantity != 2 {
		t.Fatalf("期望 Succeed=true AddedQuantity=2，得到 Succeed=%v AddedQuantity=%d", resp.Succeed, resp.AddedQuantity)
	}
	if want := "成功创建 2 个任务，1 个失败（详见日志）；插件报告：部分任务创建失败"; resp.Msg != want {
		t.Fatalf("期望 Msg %q，得到 %q", want, resp.Msg)
	}
	if len(repo.tasks) != 2 {
		t.Fatalf("期望落盘 2 个任务，得到 %d 个", len(repo.tasks))
	}
}

// TestCreateTaskByURL_StreamPartialFailureCounted 流式路径部分失败：输出 channel 的 Error 项计入失败数。
func TestCreateTaskByURL_StreamPartialFailureCounted(t *testing.T) {
	in := make(chan *sdkdto.TaskCreateResponse, 3)
	in <- &sdkdto.TaskCreateResponse{TaskName: "ok-1", SiteWorkId: "w-1", Url: "http://x/1", SiteKey: testSiteKey, ResourceType: entity.ResourceTypeImage}
	in <- &sdkdto.TaskCreateResponse{TaskName: "bad", Url: "http://x/2", SiteKey: "", ResourceType: entity.ResourceTypeImage} // SiteKey 缺失 → 填充失败（Error 项）
	in <- &sdkdto.TaskCreateResponse{TaskName: "ok-2", SiteWorkId: "w-2", Url: "http://x/3", SiteKey: testSiteKey, ResourceType: entity.ResourceTypeImage}
	close(in)

	handler := &fakePluginTaskHandler{create: func(string) (*sdkdto.TaskCreateResult, error) {
		result := sdkdto.StreamResult(in)
		result.SetReason("部分任务创建失败")
		return result, nil
	}}
	svc, repo := newCreateByURLService(t,
		&fakeTaskHandlerGetter{handlers: map[string]sdkdto.TaskHandler{"pub-a/ext-a": handler}},
		newURLListenerService(namedListener("pub-a", "插件A", "ext-a")))

	resp, err := svc.CreateTaskByURL(context.Background(), "http://x/1")
	if err != nil {
		t.Fatalf("CreateTaskByURL 返回错误: %v", err)
	}
	if !resp.Succeed || resp.AddedQuantity != 2 {
		t.Fatalf("期望 Succeed=true AddedQuantity=2，得到 Succeed=%v AddedQuantity=%d", resp.Succeed, resp.AddedQuantity)
	}
	if want := "成功创建 2 个任务，1 个失败（详见日志）；插件报告：部分任务创建失败"; resp.Msg != want {
		t.Fatalf("期望 Msg %q，得到 %q", want, resp.Msg)
	}
	if len(repo.tasks) != 2 {
		t.Fatalf("期望落盘 2 个任务，得到 %d 个", len(repo.tasks))
	}
}

// ---- CreateTaskByURL：候选路由（确定性序 / 显选 / 冲突载荷） ----
//
// 候选粒度 = 扩展点（插件 × extensionId），尝试序 = 候选全键（插件 ID + 扩展点 ID）字典序。
// 交互入口 CreateTaskByURLWithChoice 在候选多于一个且未显选时返回冲突载荷且不调用任何插件；
// 程序化入口 CreateTaskByURL 无显选通道，恒按全键字典序尝试。

// countingTaskHandlerGetter 记录 GetTaskHandler 调用次数，锚定「冲突路径不触达插件」。
type countingTaskHandlerGetter struct {
	inner *fakeTaskHandlerGetter
	calls int
}

func (c *countingTaskHandlerGetter) GetTaskHandler(pluginPublicId, extensionId string) (sdkdto.TaskHandler, error) {
	c.calls++
	return c.inner.GetTaskHandler(pluginPublicId, extensionId)
}

// TestCreateTaskByURL_SingleCandidateNoConflict 单候选：交互入口与程序化入口同走该候选，
// 无冲突载荷，行为与现状一致。
func TestCreateTaskByURL_SingleCandidateNoConflict(t *testing.T) {
	handler := viableHandler()
	svc, repo := newCreateByURLService(t,
		&fakeTaskHandlerGetter{handlers: map[string]sdkdto.TaskHandler{"pub-a/ext-a": handler}},
		newURLListenerService(namedListener("pub-a", "插件A", "ext-a")))

	resp, err := svc.CreateTaskByURLWithChoice(context.Background(), "http://x/1", "", "")
	if err != nil {
		t.Fatalf("CreateTaskByURLWithChoice 返回错误: %v", err)
	}
	if resp.Conflict {
		t.Fatal("单候选不应产生冲突载荷")
	}
	if len(resp.ConflictCandidates) != 0 {
		t.Fatalf("单候选不应返回候选清单，得到 %d 项", len(resp.ConflictCandidates))
	}
	if !resp.Succeed || resp.AddedQuantity != 1 {
		t.Fatalf("期望 Succeed=true AddedQuantity=1，得到 Succeed=%v AddedQuantity=%d", resp.Succeed, resp.AddedQuantity)
	}
	if want := "成功创建 1 个任务"; resp.Msg != want {
		t.Fatalf("期望 Msg %q，得到 %q", want, resp.Msg)
	}
	if handler.createCalls != 1 || len(repo.tasks) != 1 {
		t.Fatalf("期望候选被调用 1 次并落盘 1 个任务，得到 createCalls=%d 落盘=%d", handler.createCalls, len(repo.tasks))
	}
}

// TestCreateTaskByURL_ConflictReturnsWithoutCallingPlugins 多候选且未显选：返回冲突载荷与候选清单
// （按候选全键字典序，首位即默认选中项），不调用任何插件。
func TestCreateTaskByURL_ConflictReturnsWithoutCallingPlugins(t *testing.T) {
	handlerA, handlerB := viableHandler(), viableHandler()
	getter := &countingTaskHandlerGetter{inner: &fakeTaskHandlerGetter{handlers: map[string]sdkdto.TaskHandler{
		"pub-a/ext-a": handlerA,
		"pub-b/ext-b": handlerB,
	}}}
	// 注册序为 B→A：冲突载荷仍应按全键字典序给出 A 在前
	svc, repo := newCreateByURLService(t, getter, newURLListenerService(
		namedListener("pub-b", "插件B", "ext-b"),
		namedListener("pub-a", "插件A", "ext-a"),
	))

	resp, err := svc.CreateTaskByURLWithChoice(context.Background(), "http://x/1", "", "")
	if err != nil {
		t.Fatalf("CreateTaskByURLWithChoice 返回错误: %v", err)
	}
	if !resp.Conflict {
		t.Fatal("多候选且未显选应返回冲突载荷")
	}
	if resp.Succeed {
		t.Fatal("冲突路径不应标记成功")
	}
	if want := "该链接可由多个插件处理，请选择插件"; resp.Msg != want {
		t.Fatalf("期望引导性 Msg %q，得到 %q", want, resp.Msg)
	}
	if len(resp.ConflictCandidates) != 2 {
		t.Fatalf("期望候选清单 2 项，得到 %d 项", len(resp.ConflictCandidates))
	}
	if first := resp.ConflictCandidates[0]; first.PluginPublicId != "pub-a" || first.ExtensionId != "ext-a" || first.PluginName != "插件A" {
		t.Fatalf("候选清单首位应为全键字典序最小的 pub-a/ext-a，得到 %+v", first)
	}
	if second := resp.ConflictCandidates[1]; second.PluginPublicId != "pub-b" || second.ExtensionId != "ext-b" || second.PluginName != "插件B" {
		t.Fatalf("候选清单次位应为 pub-b/ext-b，得到 %+v", second)
	}
	if getter.calls != 0 || handlerA.createCalls != 0 || handlerB.createCalls != 0 || len(repo.tasks) != 0 {
		t.Fatalf("冲突路径不得调用任何插件（getter=%d createA=%d createB=%d 落盘=%d）",
			getter.calls, handlerA.createCalls, handlerB.createCalls, len(repo.tasks))
	}
}

// TestCreateTaskByURL_ChosenCandidateRoutesToItOnly 显选非默认候选：该候选置于路由首位，
// 非被选候选不被调用（非被选者若被调用必失败，可反证尝试序）。
func TestCreateTaskByURL_ChosenCandidateRoutesToItOnly(t *testing.T) {
	handlerA := &fakePluginTaskHandler{create: func(string) (*sdkdto.TaskCreateResult, error) {
		return nil, errors.New("非被选候选被调用")
	}}
	handlerB := viableHandler()
	svc, _ := newCreateByURLService(t,
		&fakeTaskHandlerGetter{handlers: map[string]sdkdto.TaskHandler{
			"pub-a/ext-a": handlerA,
			"pub-b/ext-b": handlerB,
		}},
		newURLListenerService(
			namedListener("pub-a", "插件A", "ext-a"),
			namedListener("pub-b", "插件B", "ext-b"),
		))

	resp, err := svc.CreateTaskByURLWithChoice(context.Background(), "http://x/1", "pub-b", "ext-b")
	if err != nil {
		t.Fatalf("CreateTaskByURLWithChoice 返回错误: %v", err)
	}
	if resp.Conflict {
		t.Fatal("已显选不应返回冲突载荷")
	}
	if !resp.Succeed {
		t.Fatalf("被选候选应创建成功，得到 Msg=%q", resp.Msg)
	}
	if handlerB.createCalls != 1 || handlerA.createCalls != 0 {
		t.Fatalf("仅被选扩展点应被调用，得到 createB=%d createA=%d", handlerB.createCalls, handlerA.createCalls)
	}
}

// TestCreateTaskByURL_ChosenFailureTerminates 被选候选真失败即终止：文案点名被选插件且不带路由
// 基座的候选前缀，也不回退其余候选（任务创建面无协议级不适配信号）。
func TestCreateTaskByURL_ChosenFailureTerminates(t *testing.T) {
	handlerA := viableHandler()
	handlerA.create = func(string) (*sdkdto.TaskCreateResult, error) {
		return nil, errors.New("connection refused")
	}
	handlerB := viableHandler()
	svc, _ := newCreateByURLService(t,
		&fakeTaskHandlerGetter{handlers: map[string]sdkdto.TaskHandler{
			"pub-a/ext-a": handlerA,
			"pub-b/ext-b": handlerB,
		}},
		newURLListenerService(
			namedListener("pub-a", "插件A", "ext-a"),
			namedListener("pub-b", "插件B", "ext-b"),
		))

	resp, err := svc.CreateTaskByURLWithChoice(context.Background(), "http://x/1", "pub-a", "ext-a")
	if err != nil {
		t.Fatalf("CreateTaskByURLWithChoice 返回错误: %v", err)
	}
	if resp.Succeed {
		t.Fatal("被选候选失败不应标记成功")
	}
	if want := "插件 插件A 创建任务失败（异常退出或连接中断）：connection refused"; resp.Msg != want {
		t.Fatalf("期望 Msg %q，得到 %q", want, resp.Msg)
	}
	if handlerB.createCalls != 0 {
		t.Fatalf("被选候选失败即终止，不得回退其余候选，得到 createB=%d", handlerB.createCalls)
	}
}

// TestCreateTaskByURL_ChosenCandidateNotInCandidatesFails 显选键不匹配候选集：报
// ErrChosenCandidateInvalid 且不调用任何插件；插件键命中而扩展点键不符同样非法（两键联合匹配）。
func TestCreateTaskByURL_ChosenCandidateNotInCandidatesFails(t *testing.T) {
	handlerA, handlerB := viableHandler(), viableHandler()
	getter := &countingTaskHandlerGetter{inner: &fakeTaskHandlerGetter{handlers: map[string]sdkdto.TaskHandler{
		"pub-a/ext-a": handlerA,
		"pub-b/ext-b": handlerB,
	}}}
	svc, _ := newCreateByURLService(t, getter, newURLListenerService(
		namedListener("pub-a", "插件A", "ext-a"),
		namedListener("pub-b", "插件B", "ext-b"),
	))
	ctx := context.Background()

	if _, err := svc.CreateTaskByURLWithChoice(ctx, "http://x/1", "pub-c", "ext-c"); !errors.Is(err, ErrChosenCandidateInvalid) {
		t.Fatalf("插件键不匹配候选集应报 ErrChosenCandidateInvalid，得到 %v", err)
	}
	if _, err := svc.CreateTaskByURLWithChoice(ctx, "http://x/1", "pub-a", "ext-z"); !errors.Is(err, ErrChosenCandidateInvalid) {
		t.Fatalf("扩展点键不匹配候选集应报 ErrChosenCandidateInvalid，得到 %v", err)
	}
	if getter.calls != 0 || handlerA.createCalls != 0 || handlerB.createCalls != 0 {
		t.Fatalf("非法显选不得调用任何插件（getter=%d createA=%d createB=%d）",
			getter.calls, handlerA.createCalls, handlerB.createCalls)
	}
}

// TestCreateTaskByURL_ProgrammaticEntryUsesFullKeyOrder 程序化入口无显选通道：候选经两个匹配模式
// 产出（ListListener 遍历 Go map，产出序随迭代随机）时，仍恒按全键字典序命中首个候选。
func TestCreateTaskByURL_ProgrammaticEntryUsesFullKeyOrder(t *testing.T) {
	handlerA, handlerB := viableHandler(), viableHandler()
	listenerSvc := pluginTaskUrlListener.NewService(pluginTaskUrlListener.NewManager())
	listenerSvc.Register(namedListener("pub-b", "插件B", "ext-b"), []string{"^http"})
	listenerSvc.Register(namedListener("pub-a", "插件A", "ext-a"), []string{"^http://x"})
	svc, _ := newCreateByURLService(t,
		&fakeTaskHandlerGetter{handlers: map[string]sdkdto.TaskHandler{
			"pub-a/ext-a": handlerA,
			"pub-b/ext-b": handlerB,
		}}, listenerSvc)

	const rounds = 8
	for i := 0; i < rounds; i++ {
		resp, err := svc.CreateTaskByURL(context.Background(), "http://x/1")
		if err != nil {
			t.Fatalf("第 %d 次 CreateTaskByURL 返回错误: %v", i, err)
		}
		if !resp.Succeed {
			t.Fatalf("第 %d 次 CreateTaskByURL 应由 pub-a/ext-a 创建成功，得到 Msg=%q", i, resp.Msg)
		}
	}
	if handlerA.createCalls != rounds || handlerB.createCalls != 0 {
		t.Fatalf("程序化入口应恒以全键字典序命中 pub-a/ext-a，得到 createA=%d createB=%d",
			handlerA.createCalls, handlerB.createCalls)
	}
}
