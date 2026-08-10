package task

import (
	"context"
	"database/sql"
	"os"
	"sync"
	"testing"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	"github.com/library-squirrel/backend/pluginTaskUrlListener"
	"github.com/library-squirrel/backend/site"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
	"go.uber.org/zap"
)

// 本文件为 leaf/独立任务创建路径回归测试（task-graph F 节点）。
// 守护 applyCreateResponse 单点权威：stream 与 array 两路径对 leaf/parent+children 三态行为一致。
//
// 统一契约（planCreateResponse，stream/array 共用）：
//   - 无 Children → 独立 leaf：1 任务，pid=0（Valid=true）、HasChild=false，计数 1。
//   - 有 Children → parent + 每个 child（不折叠，Children=[1] 也建 parent+child）：
//     parent HasChild=true、pid=0；children pid=parent.id；计数 = len(children)（parent 容器不计）。
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
	siteSvc := site.NewService(fakeSiteRepo{})
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
	// 统一契约：leaf 显式置 pid=0（Valid=true），与 parent 的 pid=0 一致，标志无父
	if !leaf.Pid.Valid || leaf.Pid.Int64 != 0 {
		t.Errorf("leaf Pid 应为 0 且 Valid=true，得到 %+v", leaf.Pid)
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
	if !parent.Pid.Valid || parent.Pid.Int64 != 0 {
		t.Errorf("父任务 Pid 应为 0（Valid=true），得到 %+v", parent.Pid)
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
	if !parent.Pid.Valid || parent.Pid.Int64 != 0 {
		t.Errorf("父任务 Pid 应为 0（Valid=true），得到 %+v", parent.Pid)
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
	if !parent.Pid.Valid || parent.Pid.Int64 != 0 {
		t.Errorf("父任务 Pid 应为 0，得到 %+v", parent.Pid)
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
	if !parent.Pid.Valid || parent.Pid.Int64 != 0 {
		t.Errorf("父任务 Pid 应为 0，得到 %+v", parent.Pid)
	}
	assertChildrenPid(t, repo.tasks, parent.GetID(), 2)
}
