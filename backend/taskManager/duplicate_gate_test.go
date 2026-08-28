package taskManager

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/duplicate"
	"github.com/library-squirrel/backend/resource"
	"github.com/library-squirrel/backend/shareLock"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// 覆盖确认行级门槛测试（改接 duplicate.DuplicateChecker 后）：任务进入 runSectionCombo 查重段，
// 按查重判定结果分流——命中冲突 → WaitingForInput 弹覆盖确认；命中无冲突/未命中 → 不弹窗、
// 保留替换定位（existingWorkId → workId、isReplace=true）。覆盖要点：
// - 命中冲突载荷透传交集角色；行级信息不可得（判定返回 nil 冲突角色）时保守弹窗
// - 命中无冲突不弹窗但保留替换定位（对齐原 model.go 空交集/零行语义）
// - 仅作品信息任务(无资源板块)不参与查重
// 判定规则本身（站点名映射/作品定位/角色求交/保守弹窗分支）由 duplicate 模块单测覆盖，
// 本文件只验证 taskManager 控制面动作与载荷透传。

// ==== fakes ====

// fakeDuplicateChecker 查重判定桩:预置判定结果并记录查询次数供"未查重"断言
type fakeDuplicateChecker struct {
	result duplicate.DuplicateCheckResult
	calls  int
}

func (f *fakeDuplicateChecker) Check(ctx context.Context, items []duplicate.DuplicateCheckItem) ([]duplicate.DuplicateCheckResult, error) {
	f.calls++
	out := make([]duplicate.DuplicateCheckResult, len(items))
	for i := range items {
		out[i] = f.result
	}
	return out, nil
}

// fakeSiteNameResolver 站点 ID → 站点名桩(查重输入键形态统一:task.SiteID 反查站点名)
type fakeSiteNameResolver struct {
	names map[int64]string
}

func (f *fakeSiteNameResolver) ListByIds(ctx context.Context, ids []int64) ([]*entity.Site, error) {
	out := make([]*entity.Site, 0, len(ids))
	for _, id := range ids {
		if name, ok := f.names[id]; ok {
			s := entity.NewSite()
			s.ID = id
			s.SiteName = sql.NullString{String: name, Valid: true}
			out = append(out, s)
		}
	}
	return out, nil
}

// fakePusher 推送桩:捕获 PushDuplicateDetected 载荷
type fakePusher struct {
	duplicates []duplicateCall
}

type duplicateCall struct {
	taskId        int64
	existingId    int64
	conflictRoles []string
}

func (p *fakePusher) PushStateChange(int64, string, TaskState)       {}
func (p *fakePusher) PushParentStateChange(int64, string, TaskState) {}
func (p *fakePusher) PushProgress(int64, int64, int64)               {}
func (p *fakePusher) PushProgressBatch([]*taskScheduleDTO)           {}
func (p *fakePusher) PushParentProgress(int64, int64, int64)         {}
func (p *fakePusher) PushError(int64, string)                        {}
func (p *fakePusher) PushTaskRemove([]int64)                         {}
func (p *fakePusher) PushParentTaskRemove([]int64)                   {}
func (p *fakePusher) PushDuplicateDetected(taskId int64, taskName string, existingWorkId int64, existingWorkName string, conflictRoles []string) {
	p.duplicates = append(p.duplicates, duplicateCall{taskId: taskId, existingId: existingWorkId, conflictRoles: conflictRoles})
}

// fakeExec 插件执行器桩:Start 固定返回错误,使查重门槛通过(不弹窗)的用例在下载前失败终止,
// 无需真实的流/存储依赖;CreateWorkInfo 正常返回空响应(仅作品信息用例)
type fakeExec struct {
	createErr  error
	savedWork  bool // CreateWorkInfo 是否被调用(作品信息板块)
	startCalls int
}

func (e *fakeExec) CreateWorkInfo(ctx context.Context, task *entity.Task) (*sdkdto.WorkResponse, error) {
	e.savedWork = true
	return nil, e.createErr
}

func (e *fakeExec) Start(ctx context.Context, task *entity.Task, storeRoles []string) ([]*sdkdto.StoreSpec, *sdkdto.WorkResponse, error) {
	e.startCalls++
	return nil, nil, fmt.Errorf("测试桩:不进入下载")
}

func (e *fakeExec) Pause(ctx context.Context, param *sdkdto.TaskResParam) error { return nil }

func (e *fakeExec) Stop(ctx context.Context, param *sdkdto.TaskResParam) error { return nil }

func (e *fakeExec) Resume(ctx context.Context, param *sdkdto.TaskResumeParam) ([]*sdkdto.StoreSpec, *sdkdto.WorkResponse, error) {
	return nil, nil, fmt.Errorf("测试桩:不进入续传")
}

// fakeResourceReader 资源查询桩（替换软删链取作品资源）
type fakeResourceReader struct {
	resources []*entity.Resource
}

func (f *fakeResourceReader) ListByWorkId(ctx context.Context, workId int64) ([]*entity.Resource, error) {
	return f.resources, nil
}

func (f *fakeResourceReader) GetById(ctx context.Context, id int64) (*entity.Resource, error) {
	return nil, nil
}

// fakeResourceStoreReader 关联查询桩（ListByResourceIds 供替换软删/回滚派生）
type fakeResourceStoreReader struct {
	assocs []*entity.ResourceStore
}

func (f *fakeResourceStoreReader) ListByResourceId(ctx context.Context, resourceId int64) ([]*entity.ResourceStore, error) {
	return f.assocs, nil
}

func (f *fakeResourceStoreReader) ListByResourceIds(ctx context.Context, resourceIds []int64) ([]*entity.ResourceStore, error) {
	return f.assocs, nil
}

// fakeStoreBackupReader store 行含删读取桩（软删判活与回滚派生共用）
type fakeStoreBackupReader struct {
	rows        []*entity.PersistentStore
	restoredIds []int64
}

func (f *fakeStoreBackupReader) ListByIdsIncludeDeleted(ctx context.Context, ids []int64) []*entity.PersistentStore {
	want := make(map[int64]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	result := make([]*entity.PersistentStore, 0, len(f.rows))
	for _, row := range f.rows {
		if want[row.GetID()] {
			result = append(result, row)
		}
	}
	return result
}

func (f *fakeStoreBackupReader) RestoreByIds(ctx context.Context, ids []int64) error {
	f.restoredIds = append(f.restoredIds, ids...)
	return nil
}

// fakeStoreReplacer 替换软删桩：记录软删调用（备份软删/废弃软删分流）
type fakeStoreReplacer struct {
	backupIds    []int64
	discardedIds []int64
}

func (f *fakeStoreReplacer) DeleteWithBackup(ctx context.Context, id int64) (int64, error) {
	f.backupIds = append(f.backupIds, id)
	return 0, nil
}

func (f *fakeStoreReplacer) SoftDeleteAndDiscardFile(ctx context.Context, id int64) error {
	f.discardedIds = append(f.discardedIds, id)
	return nil
}

// stubWorkDirProvider 工作目录桩
type stubWorkDirProvider struct {
	dir string
}

func (p stubWorkDirProvider) GetWorkDir() string { return p.dir }

// fakeWorkInfoSaver 作品信息保存桩
type fakeWorkInfoSaver struct {
	savedWorkId int64
}

func (s *fakeWorkInfoSaver) SaveWorkInfo(ctx context.Context, task *entity.Task, workResp *sdkdto.WorkResponse) (int64, error) {
	return s.savedWorkId, nil
}

// newRoleGateTask 构造进入 runSectionCombo 查重段的 ManagedTask(不启动 actor)。
// 门槛后各段依赖(替换软删/插件执行器)注入桩:Start 桩固定报错,不弹窗用例在下载前终止。
// 返回替换链桩供用例预置资源图与断言
func newRoleGateTask(taskId int64, mode runMode, checker *fakeDuplicateChecker, resolver *fakeSiteNameResolver, pusher *fakePusher) (*ManagedTask, *fakeResourceReader, *fakeResourceStoreReader, *fakeStoreBackupReader, *fakeStoreReplacer) {
	m := newTestManagedTask()
	m.taskId = taskId
	m.task.TaskName = sql.NullString{String: "t", Valid: true}
	m.task.SiteID = sql.NullInt64{Int64: 1, Valid: true}
	m.task.SiteWorkID = sql.NullString{String: "sw1", Valid: true}
	m.runMode = mode
	m.pluginExec = &fakeExec{}
	resReader := &fakeResourceReader{}
	rsReader := &fakeResourceStoreReader{}
	backupReader := &fakeStoreBackupReader{}
	replacer := &fakeStoreReplacer{}
	m.deps = &TaskDeps{
		DuplicateChecker:    checker,
		SiteNameResolver:    resolver,
		Pusher:              pusher,
		WorkDirProvider:     stubWorkDirProvider{dir: "E:/lib"},
		ResourceReader:      resReader,
		ResourceStoreReader: rsReader,
		StoreBackupReader:   backupReader,
		// 替换链能力注入真实 resource.ReplacementService（复用本文件 fakes 作其依赖接口）——
		// 门槛通过后的前置软删经 resource 域执行，断言仍落到同一批桩上
		ReplaceStoreOps: resource.NewReplacementService(
			resReader, rsReader, backupReader, replacer,
			&fakeBackupFileRestorer{}, &fakeWorkLivenessReader{work: entity.NewWork()},
			&fakeResourceRecomputer{}, stubWorkDirProvider{dir: "E:/lib"},
			shareLock.NewShareLockRegistry(),
		),
	}
	return m, resReader, rsReader, backupReader, replacer
}

// fakeCheckResult 构造查重判定结果
func fakeCheckResult(class duplicate.DuplicateClass, conflictRoles []string) duplicate.DuplicateCheckResult {
	return duplicate.DuplicateCheckResult{Class: class, WorkID: 500, WorkName: "已存在作品", ConflictRoles: conflictRoles}
}

// runDuplicateGate 执行 runSectionCombo 并断言弹窗语义:
// 弹窗 → WaitingForInput;不弹窗 → 门槛通过后由 Start 桩报错转 Failed
func runDuplicateGate(t *testing.T, m *ManagedTask, pusher *fakePusher) (pushed bool) {
	t.Helper()
	_ = m.runSectionCombo()
	pushed = len(pusher.duplicates) > 0
	if pushed && m.state.Load() != int32(TaskStateWaitingForInput) {
		t.Fatalf("弹窗后任务应进入 WaitingForInput, 实际 %d", m.state.Load())
	}
	return pushed
}

// ==== 表驱动用例 ====

// TestDuplicateGate_RowLevel 验证 fallback 路径(runSectionCombo 查重段)对三分类判定的控制面分流
func TestDuplicateGate_RowLevel(t *testing.T) {
	cases := []struct {
		name string
		// 查重判定预置结果
		result   duplicate.DuplicateCheckResult
		wantPush bool
		// wantMiss 未命中:不定位已有作品(资源重执行无法挂载转 Failed);false 表示命中无冲突(定位为替换目标)
		wantMiss bool
		// wantRoles 期望载荷板块明细;wantPush 且 wantNil 时期望载荷为 nil
		wantRoles []string
		wantNil   bool
	}{
		{
			name:     "命中无冲突_交集空_不弹窗",
			result:   fakeCheckResult(duplicate.DuplicateHitNoConflict, nil),
			wantPush: false,
		},
		{
			name:      "命中冲突_载荷为交集",
			result:    fakeCheckResult(duplicate.DuplicateHitConflict, []string{entity.StoreTypeThumbnail}),
			wantPush:  true,
			wantRoles: []string{entity.StoreTypeThumbnail},
		},
		{
			name:      "命中冲突_载荷为已有行全集",
			result:    fakeCheckResult(duplicate.DuplicateHitConflict, []string{entity.StoreTypeImage, entity.StoreTypeThumbnail}),
			wantPush:  true,
			wantRoles: []string{entity.StoreTypeImage, entity.StoreTypeThumbnail},
		},
		{
			name:     "命中冲突_行级信息不可得_保守弹窗_载荷nil", // 宁多弹不漏弹(原行级查询失败/未配置角色查询器)
			result:   fakeCheckResult(duplicate.DuplicateHitConflict, nil),
			wantPush: true,
			wantNil:  true,
		},
		{
			name:     "命中无冲突_零行_不弹窗",
			result:   fakeCheckResult(duplicate.DuplicateHitNoConflict, nil),
			wantPush: false,
		},
		{
			name:     "未命中_不弹窗",
			result:   duplicate.DuplicateCheckResult{Class: duplicate.DuplicateMiss},
			wantPush: false,
			wantMiss: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checker := &fakeDuplicateChecker{result: tc.result}
			pusher := &fakePusher{}

			m, _, _, _, _ := newRoleGateTask(300, runMode{storeScope: storeScope{kind: scopeSelected, roles: []string{entity.StoreTypeThumbnail}}},
				checker, &fakeSiteNameResolver{names: map[int64]string{1: "test-site"}}, pusher)

			pushed := runDuplicateGate(t, m, pusher)
			if pushed != tc.wantPush {
				t.Fatalf("弹窗期望 %v, 实际 %v (pushes=%d)", tc.wantPush, pushed, len(pusher.duplicates))
			}
			if !tc.wantPush {
				if m.state.Load() != int32(TaskStateFailed) {
					t.Fatalf("未弹窗场景门槛通过后应转 Failed, 实际 %d", m.state.Load())
				}
				if tc.wantMiss {
					// 未命中:不定位已有作品,资源重执行无法挂载
					if m.workId != 0 || m.isReplace {
						t.Fatalf("未命中不应定位已有作品(workId=%d isReplace=%v)", m.workId, m.isReplace)
					}
				} else if m.workId != 500 || !m.isReplace {
					// 命中无冲突:已有作品定位为替换目标
					t.Fatalf("未弹窗时应定位已有作品为替换目标(workId=%d isReplace=%v)", m.workId, m.isReplace)
				}
				return
			}
			call := pusher.duplicates[0]
			if call.existingId != 500 {
				t.Fatalf("载荷已有作品 ID 期望 500, 实际 %d", call.existingId)
			}
			if tc.wantNil {
				if call.conflictRoles != nil {
					t.Fatalf("载荷 conflictRoles 期望 nil, 实际 %v", call.conflictRoles)
				}
				return
			}
			if len(call.conflictRoles) != len(tc.wantRoles) {
				t.Fatalf("载荷 conflictRoles 期望 %v, 实际 %v", tc.wantRoles, call.conflictRoles)
			}
			for i, r := range tc.wantRoles {
				if call.conflictRoles[i] != r {
					t.Fatalf("载荷 conflictRoles 期望 %v, 实际 %v", tc.wantRoles, call.conflictRoles)
				}
			}
		})
	}
}

// TestDuplicateGate_EmptyIntersectionKeepsExistingWorkId 命中无冲突不弹窗但视为替换:
// 定位到已有作品(workId 置位、isReplace=true)并按所选板块前置软删旧 store
func TestDuplicateGate_EmptyIntersectionKeepsExistingWorkId(t *testing.T) {
	checker := &fakeDuplicateChecker{result: fakeCheckResult(duplicate.DuplicateHitNoConflict, nil)}
	m, resReader, rsReader, backupReader, replacer := newRoleGateTask(300, runMode{storeScope: storeScope{kind: scopeSelected, roles: []string{entity.StoreTypeThumbnail}}},
		checker, &fakeSiteNameResolver{names: map[int64]string{1: "test-site"}}, &fakePusher{})
	pusher := m.deps.Pusher.(*fakePusher)
	// 预置资源图：作品 500 → 资源 700 → thumbnail 关联(store 800，已完成活行)
	res := entity.NewResource()
	res.ID = 700
	res.WorkID = 500
	resReader.resources = []*entity.Resource{res}
	assoc := entity.NewResourceStore()
	assoc.ID = 1
	assoc.ResourceID = 700
	assoc.StoreType = entity.StoreTypeThumbnail
	assoc.StoreID = 800
	assoc.StoreSeq = 0
	rsReader.assocs = []*entity.ResourceStore{assoc}
	storeRow := entity.NewPersistentStore()
	storeRow.SetID(800)
	storeRow.CompletedAt = 1
	backupReader.rows = []*entity.PersistentStore{storeRow}

	pushed := runDuplicateGate(t, m, pusher)
	if pushed {
		t.Fatal("空交集不应弹窗")
	}
	if m.existingWorkId != 0 || m.workId != 500 || !m.isReplace {
		t.Fatalf("空交集应定位到已有作品并视为替换(existingWorkId=%d workId=%d isReplace=%v)",
			m.existingWorkId, m.workId, m.isReplace)
	}
	if len(replacer.backupIds) != 1 || replacer.backupIds[0] != 800 {
		t.Fatalf("替换场景应按所选板块前置软删旧 store(800), 实际 %v", replacer.backupIds)
	}
	if len(replacer.discardedIds) != 0 {
		t.Fatalf("已完成行不应走废弃分支, 实际 %v", replacer.discardedIds)
	}
}

// TestDuplicateGate_FullModeReplaceSoftDeletesAllRoles 空 universe 全量(All)模式的替换软删锚：
// All 语义=全量板块，替换 gate 按「是否拉取资源」判定而非「用户是否指定子集」——
// 前置软删经封闭枚举全集展开覆盖作品全部角色活行。修复前 gate 按空选择旁路软删：
// 锁不查+无备份+旧 store 行/文件孤儿泄漏(purge 不级联)
func TestDuplicateGate_FullModeReplaceSoftDeletesAllRoles(t *testing.T) {
	checker := &fakeDuplicateChecker{result: fakeCheckResult(duplicate.DuplicateHitNoConflict, nil)}
	m, resReader, rsReader, backupReader, replacer := newRoleGateTask(300, runMode{storeScope: storeScope{kind: scopeAll}},
		checker, &fakeSiteNameResolver{names: map[int64]string{1: "test-site"}}, &fakePusher{})
	pusher := m.deps.Pusher.(*fakePusher)
	// 预置资源图：作品 500 → 资源 700 → image(800) 与 thumbnail(801) 两条已完成活行
	res := entity.NewResource()
	res.ID = 700
	res.WorkID = 500
	resReader.resources = []*entity.Resource{res}
	rsReader.assocs = nil
	for i, role := range []string{entity.StoreTypeImage, entity.StoreTypeThumbnail} {
		assoc := entity.NewResourceStore()
		assoc.ID = int64(i + 1)
		assoc.ResourceID = 700
		assoc.StoreType = role
		assoc.StoreID = int64(800 + i)
		assoc.StoreSeq = 0
		rsReader.assocs = append(rsReader.assocs, assoc)
		row := entity.NewPersistentStore()
		row.SetID(int64(800 + i))
		row.CompletedAt = 1
		backupReader.rows = append(backupReader.rows, row)
	}

	pushed := runDuplicateGate(t, m, pusher)
	if pushed {
		t.Fatal("空交集不应弹窗")
	}
	if m.workId != 500 || !m.isReplace {
		t.Fatalf("全量模式命中已有作品应视为替换(workId=%d isReplace=%v)", m.workId, m.isReplace)
	}
	if len(replacer.backupIds) != 2 {
		t.Fatalf("全量模式前置软删应覆盖作品全部角色活行(800,801)，实际 %v", replacer.backupIds)
	}
}

// TestDuplicateGate_FullModeReplaceLockGuard 守卫链含锁锚：全量模式前置软删走作品锁守卫——
// 作品被分享拉取持有时软删被拒、任务转 Failed，不触碰任何 store 行(修复前 gate 旁路连锁都不查)
func TestDuplicateGate_FullModeReplaceLockGuard(t *testing.T) {
	checker := &fakeDuplicateChecker{result: fakeCheckResult(duplicate.DuplicateHitNoConflict, nil)}
	m, resReader, rsReader, backupReader, replacer := newRoleGateTask(300, runMode{storeScope: storeScope{kind: scopeAll}},
		checker, &fakeSiteNameResolver{names: map[int64]string{1: "test-site"}}, &fakePusher{})
	pusher := m.deps.Pusher.(*fakePusher)
	res := entity.NewResource()
	res.ID = 700
	res.WorkID = 500
	resReader.resources = []*entity.Resource{res}
	assoc := entity.NewResourceStore()
	assoc.ID = 1
	assoc.ResourceID = 700
	assoc.StoreType = entity.StoreTypeImage
	assoc.StoreID = 800
	assoc.StoreSeq = 0
	rsReader.assocs = []*entity.ResourceStore{assoc}
	storeRow := entity.NewPersistentStore()
	storeRow.SetID(800)
	storeRow.CompletedAt = 1
	backupReader.rows = []*entity.PersistentStore{storeRow}
	// 重挂带锁注册中心的替换链能力（其余依赖复用同批桩）
	lock := shareLock.NewShareLockRegistry()
	lock.Register(context.Background(), []int64{500}, "session-x")
	m.deps.ReplaceStoreOps = resource.NewReplacementService(
		resReader, rsReader, backupReader, replacer,
		&fakeBackupFileRestorer{}, &fakeWorkLivenessReader{work: entity.NewWork()},
		&fakeResourceRecomputer{}, stubWorkDirProvider{dir: "E:/lib"},
		lock,
	)

	pushed := runDuplicateGate(t, m, pusher)
	if pushed {
		t.Fatal("空交集不应弹窗")
	}
	if m.GetState() != TaskStateFailed {
		t.Fatalf("锁命中应令替换前置软删失败转 Failed，实际 %d", m.GetState())
	}
	if len(replacer.backupIds) != 0 || len(replacer.discardedIds) != 0 {
		t.Fatalf("锁命中不应触碰 store 行，实际备份软删 %v 废弃 %v", replacer.backupIds, replacer.discardedIds)
	}
}

// TestDuplicateGate_InfoOnlySkipsCheck 仅作品信息任务(无资源板块)不查重:
// DuplicateChecker 零调用、无弹窗,作品信息板块正常执行
func TestDuplicateGate_InfoOnlySkipsCheck(t *testing.T) {
	checker := &fakeDuplicateChecker{result: fakeCheckResult(duplicate.DuplicateHitConflict, nil)}
	pusher := &fakePusher{}
	m, _, _, _, _ := newRoleGateTask(300, runMode{workInfo: true, storeScope: storeScope{kind: scopeNone}},
		checker, &fakeSiteNameResolver{names: map[int64]string{1: "test-site"}}, pusher)
	m.deps.WorkInfoSaver = &fakeWorkInfoSaver{savedWorkId: 100}

	_ = m.runSectionCombo()

	if checker.calls != 0 {
		t.Fatalf("仅作品信息任务不应触发查重, DuplicateChecker 被调用 %d 次", checker.calls)
	}
	if len(pusher.duplicates) != 0 {
		t.Fatalf("仅作品信息任务不应弹覆盖确认, 实际 %d 次", len(pusher.duplicates))
	}
	if exec := m.pluginExec.(*fakeExec); !exec.savedWork {
		t.Fatal("作品信息板块应正常执行 CreateWorkInfo")
	}
	if m.workId != 100 {
		t.Fatalf("作品信息保存后应定位新作品 workId=100, 实际 %d", m.workId)
	}
}
