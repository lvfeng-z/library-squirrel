package taskManager

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/library-squirrel/backend/base/model/entity"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// 覆盖确认行级门槛测试:任务所选板块角色与已有作品 resource_store 行的 store_type 集合求交,
// 交集非空才弹覆盖确认。覆盖要点:
// - thumbnail 行命中同样弹窗(缩略图覆盖需用户知情,无板块级豁免)
// - 行级角色查询失败/未配置角色查询器时保守退回弹窗(宁多弹不漏弹),载荷 conflictRoles=nil
// - 空角色集(插件自决全量)已有任意行即弹,载荷取已有行角色全集
// - 空交集/零行不弹窗,但保留替换定位(workId 置为已有作品、isReplace=true)
// - 仅作品信息任务(无资源板块)不参与查重

// ==== fakes ====

// fakeWorkChecker 查重接口桩:按 (siteId, siteWorkId) 返回预置作品,并记录查询次数供"未查重"断言
type fakeWorkChecker struct {
	existing *entity.Work
	getCalls int
}

func (f *fakeWorkChecker) GetBySiteAndSiteWorkID(ctx context.Context, siteId int64, siteWorkId string) (*entity.Work, error) {
	f.getCalls++
	if f.existing != nil && f.existing.SiteID.Int64 == siteId && f.existing.SiteWorkID.String == siteWorkId {
		return f.existing, nil
	}
	return nil, nil
}

func (f *fakeWorkChecker) ListBySiteAndSiteWorkIDs(ctx context.Context, siteIds []int64, siteWorkIds []string) ([]*entity.Work, error) {
	return nil, fmt.Errorf("批量查重未在本测试使用")
}

// fakeRoleChecker 行级角色查询桩:返回预置 {workId: store_type 集合};err 非 nil 时模拟查询失败
type fakeRoleChecker struct {
	sets map[int64]map[string]struct{}
	err  error
}

func (f *fakeRoleChecker) ListStoreTypeSetsByWorkIds(ctx context.Context, workIds []int64) (map[int64]map[string]struct{}, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.sets, nil
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
func newRoleGateTask(taskId int64, mode runMode, checker *fakeWorkChecker, roles *fakeRoleChecker, pusher *fakePusher) (*ManagedTask, *fakeResourceReader, *fakeResourceStoreReader, *fakeStoreBackupReader, *fakeStoreReplacer) {
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
	// 角色查询器为 nil 时接口字段保持未设置(类型化 nil 指针赋给接口会令接口非 nil,走进空接收者调用)
	m.deps = &TaskDeps{
		WorkChecker:         checker,
		Pusher:              pusher,
		WorkDirProvider:     stubWorkDirProvider{dir: "E:/lib"},
		ResourceReader:      resReader,
		ResourceStoreReader: rsReader,
		StoreBackupReader:   backupReader,
		StoreReplacer:       replacer,
	}
	if roles != nil {
		m.deps.WorkStoreRoleChecker = roles
	}
	return m, resReader, rsReader, backupReader, replacer
}

// newExistingWork 预置已有作品
func newExistingWork(workId int64) *entity.Work {
	w := entity.NewWork()
	w.ID = workId
	w.SiteID = sql.NullInt64{Int64: 1, Valid: true}
	w.SiteWorkID = sql.NullString{String: "sw1", Valid: true}
	w.SiteWorkName = sql.NullString{String: "已存在作品", Valid: true}
	return w
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

// TestDuplicateGate_RowLevel 验证 fallback 路径(runSectionCombo 查重段)的行级门槛判定
func TestDuplicateGate_RowLevel(t *testing.T) {
	cases := []struct {
		name string
		// 任务所选板块;nil+fetchStores=true 表示插件自决全量(空角色集)
		roles []string
		// 已有作品 store 行角色集合
		storeTypes []string
		zeroRows   bool  // 已有作品零 store 行
		roleErr    error // 行级查询失败模拟
		roleNil    bool  // 未配置 WorkStoreRoleChecker
		wantPush   bool
		// wantRoles 期望载荷板块明细;wantPush 且 wantNil 时期望载荷为 nil
		wantRoles []string
		wantNil   bool
	}{
		{
			name:       "行全缺_交集空_不弹窗",
			roles:      []string{entity.StoreTypeVideoTrack, entity.StoreTypeAudioTrack},
			storeTypes: []string{entity.StoreTypeImage, entity.StoreTypeThumbnail},
			wantPush:   false,
		},
		{
			name:       "部分板块存在_交集非空_弹窗且载荷为交集",
			roles:      []string{entity.StoreTypeVideoTrack, entity.StoreTypeAudioTrack, entity.StoreTypeThumbnail},
			storeTypes: []string{entity.StoreTypeImage, entity.StoreTypeThumbnail},
			wantPush:   true,
			wantRoles:  []string{entity.StoreTypeThumbnail},
		},
		{
			name:       "仅thumbnail行命中_弹窗", // 缩略图覆盖不豁免
			roles:      []string{entity.StoreTypeThumbnail},
			storeTypes: []string{entity.StoreTypeThumbnail},
			wantPush:   true,
			wantRoles:  []string{entity.StoreTypeThumbnail},
		},
		{
			name:       "行级查询失败_退回弹窗_载荷nil", // 宁多弹不漏弹
			roles:      []string{entity.StoreTypeVideoTrack},
			storeTypes: []string{entity.StoreTypeImage},
			roleErr:    fmt.Errorf("db down"),
			wantPush:   true,
			wantNil:    true,
		},
		{
			name:       "未配置角色查询器_退回弹窗_载荷nil",
			roles:      []string{entity.StoreTypeVideoTrack},
			storeTypes: []string{entity.StoreTypeImage},
			roleNil:    true,
			wantPush:   true,
			wantNil:    true,
		},
		{
			name:       "空角色集_已有任意行_弹窗_载荷为已有行全集", // 插件自决全量视为覆盖所有已有行
			roles:      nil,
			storeTypes: []string{entity.StoreTypeImage, entity.StoreTypeThumbnail},
			wantPush:   true,
			wantRoles:  []string{entity.StoreTypeImage, entity.StoreTypeThumbnail},
		},
		{
			name:     "空角色集_已有零行_不弹窗",
			roles:    nil,
			zeroRows: true,
			wantPush: false,
		},
		{
			name:     "显式所选_已有零行_不弹窗",
			roles:    []string{entity.StoreTypeImage},
			zeroRows: true,
			wantPush: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checker := &fakeWorkChecker{existing: newExistingWork(500)}
			pusher := &fakePusher{}

			var roles *fakeRoleChecker
			if !tc.roleNil {
				set := map[string]struct{}{}
				for _, s := range tc.storeTypes {
					set[s] = struct{}{}
				}
				sets := map[int64]map[string]struct{}{500: set}
				if tc.zeroRows {
					sets[500] = map[string]struct{}{}
				}
				roles = &fakeRoleChecker{sets: sets, err: tc.roleErr}
			}

			m, _, _, _, _ := newRoleGateTask(300, runMode{storeRoles: tc.roles, fetchStores: true}, checker, roles, pusher)

			pushed := runDuplicateGate(t, m, pusher)
			if pushed != tc.wantPush {
				t.Fatalf("弹窗期望 %v, 实际 %v (pushes=%d)", tc.wantPush, pushed, len(pusher.duplicates))
			}
			if !tc.wantPush {
				// 不弹窗:已有作品定位为替换目标,门槛后由下载桩报错转 Failed
				if m.workId != 500 || !m.isReplace {
					t.Fatalf("未弹窗时应定位已有作品为替换目标(workId=%d isReplace=%v)", m.workId, m.isReplace)
				}
				if m.state.Load() != int32(TaskStateFailed) {
					t.Fatalf("未弹窗场景门槛通过后应由下载桩报错转 Failed, 实际 %d", m.state.Load())
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

// TestDuplicateGate_EmptyIntersectionKeepsExistingWorkId 空交集不弹窗但视为替换:
// 定位到已有作品(workId 置位、isReplace=true)并按所选板块前置软删旧 store
func TestDuplicateGate_EmptyIntersectionKeepsExistingWorkId(t *testing.T) {
	existing := newExistingWork(500)
	roles := &fakeRoleChecker{sets: map[int64]map[string]struct{}{
		500: {entity.StoreTypeImage: {}},
	}}
	m, resReader, rsReader, backupReader, replacer := newRoleGateTask(300, runMode{storeRoles: []string{entity.StoreTypeThumbnail}, fetchStores: true},
		&fakeWorkChecker{existing: existing}, roles, &fakePusher{})
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

// TestDuplicateGate_InfoOnlySkipsCheck 仅作品信息任务(无资源板块)不查重:
// WorkChecker 零调用、无弹窗,作品信息板块正常执行
func TestDuplicateGate_InfoOnlySkipsCheck(t *testing.T) {
	checker := &fakeWorkChecker{existing: newExistingWork(500)}
	pusher := &fakePusher{}
	m, _, _, _, _ := newRoleGateTask(300, runMode{workInfo: true, fetchStores: false}, checker, nil, pusher)
	m.deps.WorkInfoSaver = &fakeWorkInfoSaver{savedWorkId: 100}

	_ = m.runSectionCombo()

	if checker.getCalls != 0 {
		t.Fatalf("仅作品信息任务不应触发查重, WorkChecker 被调用 %d 次", checker.getCalls)
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

// TestIntersectRoles 纯逻辑:交集保持 roles 原始顺序
func TestIntersectRoles(t *testing.T) {
	roles := []string{entity.StoreTypeVideoTrack, entity.StoreTypeImage, entity.StoreTypeThumbnail}
	existing := map[string]struct{}{entity.StoreTypeImage: {}, entity.StoreTypeThumbnail: {}, "other": {}}
	got := intersectRoles(roles, existing)
	if len(got) != 2 || got[0] != entity.StoreTypeImage || got[1] != entity.StoreTypeThumbnail {
		t.Fatalf("交集期望按 roles 原序 [image thumbnail], 实际 %v", got)
	}
	if got := intersectRoles(roles, map[string]struct{}{}); len(got) != 0 {
		t.Fatalf("空已有集合期望空交集, 实际 %v", got)
	}
}

// TestSortedStoreRoles 纯逻辑:集合转切片按字母序(载荷确定性)
func TestSortedStoreRoles(t *testing.T) {
	got := sortedStoreRoles(map[string]struct{}{entity.StoreTypeThumbnail: {}, entity.StoreTypeImage: {}, entity.StoreTypeVideoTrack: {}})
	if len(got) != 3 || got[0] != entity.StoreTypeImage || got[1] != entity.StoreTypeThumbnail || got[2] != entity.StoreTypeVideoTrack {
		t.Fatalf("期望字母序 [image thumbnail videoTrack], 实际 %v", got)
	}
}
