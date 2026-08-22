package taskManager

import (
	"context"
	"database/sql"
	"testing"

	"github.com/library-squirrel/backend/base/model/entity"

	"gorm.io/plugin/soft_delete"
)

// 替换链软删化测试：softDeleteReplaceTargets（前置软删分派）与 restoreReplaceTargets
// （失败回滚复活原行）。覆盖要点：
// - 前置软删按完成态分派（已完成→备份软删；未完成→废弃软删），历史残留死行不动，角色过滤生效
// - 失败回滚：作品已删守卫跳过；新建 store 关联清理；同键最新死代复活（含文件还原与备份清理）；
//   更早死代不复活；关联零重挂（复活即挂载回位）

// ==== 回滚链 fakes（其余复用 duplicate_gate_test 的 fake 体系）====

// fakeWorkLivenessReader 作品活性桩：nil work = 已软删
type fakeWorkLivenessReader struct {
	work *entity.Work
}

func (f *fakeWorkLivenessReader) GetById(ctx context.Context, id int64) (*entity.Work, error) {
	return f.work, nil
}

// fakeResourceStoreWriter 关联写入桩：记录 DeleteByStoreIds 调用
type fakeResourceStoreWriter struct {
	deletedByStoreIds []int64
}

func (f *fakeResourceStoreWriter) CreateBatch(ctx context.Context, stores []*entity.ResourceStore) error {
	return nil
}

func (f *fakeResourceStoreWriter) DeleteByResourceIdAndTypes(ctx context.Context, resourceId int64, storeTypes []string) error {
	return nil
}

func (f *fakeResourceStoreWriter) DeleteByStoreIds(ctx context.Context, storeIds []int64) error {
	f.deletedByStoreIds = append(f.deletedByStoreIds, storeIds...)
	return nil
}

// fakeStoreDeleter HardDelete 桩（cleanupCreatedStores 用）
type fakeStoreDeleter struct {
	hardDeleted []int64
}

func (f *fakeStoreDeleter) HardDelete(ctx context.Context, id int64, backup bool) (int64, error) {
	f.hardDeleted = append(f.hardDeleted, id)
	return 0, nil
}

// fakeBackupFileRestorer 备份文件还原桩：记录还原与删除的清单行 ID
type fakeBackupFileRestorer struct {
	restoredBackupIds []int64
	deletedBackupIds  []int64
}

func (f *fakeBackupFileRestorer) GetById(ctx context.Context, id int64) (*entity.Backup, error) {
	b := entity.NewBackup()
	b.SetID(id)
	return b, nil
}

func (f *fakeBackupFileRestorer) GetBackupPath(backup *entity.Backup) string { return "" }

func (f *fakeBackupFileRestorer) RestoreFile(ctx context.Context, backupPath string, targetPath string) error {
	return nil
}

func (f *fakeBackupFileRestorer) DeleteBackup(ctx context.Context, id int64) error {
	f.deletedBackupIds = append(f.deletedBackupIds, id)
	return nil
}

// fakeResourceRecomputer 完整度重算记账桩（回滚后刷新回滚前状态）
type fakeResourceRecomputer struct {
	calledResourceIds []int64
}

func (f *fakeResourceRecomputer) RecomputeResourceComplete(ctx context.Context, resourceId int64) {
	f.calledResourceIds = append(f.calledResourceIds, resourceId)
}

// newReplaceTestTask 构造替换链测试任务（不启动 actor），返回各桩供断言
type replaceStubs struct {
	res       *fakeResourceReader
	rs        *fakeResourceStoreReader
	rows      *fakeStoreBackupReader
	replacer  *fakeStoreReplacer
	liveness  *fakeWorkLivenessReader
	writer    *fakeResourceStoreWriter
	deleter   *fakeStoreDeleter
	restorer  *fakeBackupFileRestorer
	recompute *fakeResourceRecomputer
}

func newReplaceTestTask(workId int64) (*ManagedTask, *replaceStubs) {
	m := newTestManagedTask()
	m.workId = workId
	stubs := &replaceStubs{
		res:       &fakeResourceReader{},
		rs:        &fakeResourceStoreReader{},
		rows:      &fakeStoreBackupReader{},
		replacer:  &fakeStoreReplacer{},
		liveness:  &fakeWorkLivenessReader{work: entity.NewWork()},
		writer:    &fakeResourceStoreWriter{},
		deleter:   &fakeStoreDeleter{},
		restorer:  &fakeBackupFileRestorer{},
		recompute: &fakeResourceRecomputer{},
	}
	m.deps = &TaskDeps{
		WorkDirProvider:     stubWorkDirProvider{dir: "E:/lib"},
		ResourceReader:      stubs.res,
		ResourceStoreReader: stubs.rs,
		StoreBackupReader:   stubs.rows,
		StoreReplacer:       stubs.replacer,
		WorkLivenessReader:  stubs.liveness,
		ResourceStoreWriter: stubs.writer,
		StoreDeleter:        stubs.deleter,
		BackupFileRestorer:  stubs.restorer,
		ResourceRecomputer:  stubs.recompute,
	}
	return m, stubs
}

// makeReplaceStoreRow 造一行 persistent_store（供 ListByIdsIncludeDeleted 桩返回）
func makeReplaceStoreRow(id int64, completed int64, deletedAt int64, backupId int64, path string) *entity.PersistentStore {
	row := entity.NewPersistentStore()
	row.SetID(id)
	row.CompletedAt = completed
	if deletedAt > 0 {
		row.DeletedAt = soft_delete.DeletedAt(deletedAt)
	}
	row.BackupID = backupId
	row.FilePath = sql.NullString{String: path, Valid: true}
	return row
}

// makeReplaceAssoc 造一行挂载关联（resource → store，带挂载键）
func makeReplaceAssoc(resourceId int64, role string, seq int, storeId int64) *entity.ResourceStore {
	rs := entity.NewResourceStore()
	rs.ResourceID = resourceId
	rs.StoreType = role
	rs.StoreSeq = seq
	rs.StoreID = storeId
	return rs
}

// TestSoftDeleteReplaceTargetsDispatchesByCompletion 前置软删分派：
// 已完成行走备份软删、未完成行走废弃软删、历史残留死行跳过、角色过滤外的不动
func TestSoftDeleteReplaceTargetsDispatchesByCompletion(t *testing.T) {
	m, stubs := newReplaceTestTask(500)
	res := entity.NewResource()
	res.ID = 700
	res.WorkID = 500
	stubs.res.resources = []*entity.Resource{res}
	stubs.rs.assocs = []*entity.ResourceStore{
		makeReplaceAssoc(700, entity.StoreTypeImage, 0, 800),
		makeReplaceAssoc(700, entity.StoreTypeImage, 0, 801),
		makeReplaceAssoc(700, entity.StoreTypeImage, 0, 802),
		makeReplaceAssoc(700, entity.StoreTypeThumbnail, 0, 803),
	}
	stubs.rows.rows = []*entity.PersistentStore{
		makeReplaceStoreRow(800, 1, 0, 0, "store/resource/a/已完成.png"),    // → 备份软删
		makeReplaceStoreRow(801, 0, 0, 0, "store/resource/a/未完成.png"),    // → 废弃软删
		makeReplaceStoreRow(802, 1, 1000, 99, "store/resource/a/残留.png"), // 死行跳过
		makeReplaceStoreRow(803, 1, 0, 0, "store/thumb/t.png"),           // 角色外不动
	}

	if err := m.softDeleteReplaceTargets(context.Background(), 500, []string{entity.StoreTypeImage}); err != nil {
		t.Fatalf("前置软删失败: %v", err)
	}
	if len(stubs.replacer.backupIds) != 1 || stubs.replacer.backupIds[0] != 800 {
		t.Fatalf("已完成行(800)应走备份软删，实际 %v", stubs.replacer.backupIds)
	}
	if len(stubs.replacer.discardedIds) != 1 || stubs.replacer.discardedIds[0] != 801 {
		t.Fatalf("未完成行(801)应走废弃软删，实际 %v", stubs.replacer.discardedIds)
	}
	for _, id := range append(stubs.replacer.backupIds, stubs.replacer.discardedIds...) {
		if id == 802 || id == 803 {
			t.Fatalf("残留死行(802)与角色外行(803)不应被软删")
		}
	}
}

// TestRestoreReplaceTargetsRevivesNewestDeadGeneration 失败回滚核心链：
// 新建 store 的关联被摘、行被物理删；同键最新死代复活（文件还原+备份清理+双列清），
// 更早死代保持死态；关联零重挂（复活即挂载回位，无 remount 调用面）
func TestRestoreReplaceTargetsRevivesNewestDeadGeneration(t *testing.T) {
	m, stubs := newReplaceTestTask(500)
	res := entity.NewResource()
	res.ID = 700
	res.WorkID = 500
	stubs.res.resources = []*entity.Resource{res}
	stubs.rs.assocs = []*entity.ResourceStore{
		// 同键 (700,image,0)：旧代死行 810(早)、victim 811(晚，本代被替换)、新建行 812 的关联
		makeReplaceAssoc(700, entity.StoreTypeImage, 0, 810),
		makeReplaceAssoc(700, entity.StoreTypeImage, 0, 811),
		makeReplaceAssoc(700, entity.StoreTypeImage, 0, 812),
	}
	const victimPath = "store/resource/a/被替换.png"
	stubs.rows.rows = []*entity.PersistentStore{
		makeReplaceStoreRow(810, 1, 1000, 91, "store/resource/a/旧代.png"),
		makeReplaceStoreRow(811, 1, 2000, 92, victimPath),
		// 新建行 812 已被 cleanupCreatedStores 物理删——派生时 ListByIdsIncludeDeleted 不返回它
	}
	m.streams = []*streamController{{storeId: 812, relPath: victimPath}}

	m.restoreReplaceTargets(context.Background())

	if len(stubs.writer.deletedByStoreIds) != 1 || stubs.writer.deletedByStoreIds[0] != 812 {
		t.Fatalf("新建行(812)的关联应被摘除，实际 %v", stubs.writer.deletedByStoreIds)
	}
	if len(stubs.deleter.hardDeleted) != 1 || stubs.deleter.hardDeleted[0] != 812 {
		t.Fatalf("新建行(812)应被物理删，实际 %v", stubs.deleter.hardDeleted)
	}
	if len(stubs.rows.restoredIds) != 1 || stubs.rows.restoredIds[0] != 811 {
		t.Fatalf("应只复活 victim(811)，实际 %v", stubs.rows.restoredIds)
	}
	if len(stubs.restorer.deletedBackupIds) != 1 || stubs.restorer.deletedBackupIds[0] != 92 {
		t.Fatalf("victim 备份(92)应被清理，实际 %v", stubs.restorer.deletedBackupIds)
	}
	// 回滚后按 victim 所属资源重算完整度（saveResource 曾重置为未校验，须刷回回滚前状态）
	if len(stubs.recompute.calledResourceIds) != 1 || stubs.recompute.calledResourceIds[0] != 700 {
		t.Fatalf("回滚后应重算 victim 所属资源(700) 完整度，实际 %v", stubs.recompute.calledResourceIds)
	}
}

// TestRestoreReplaceTargetsSkipsWhenWorkDeleted 作品活性守卫：作品已软删时回滚让位
// （两代归回收站作品条目管理），不做任何摘关联/复活动作
func TestRestoreReplaceTargetsSkipsWhenWorkDeleted(t *testing.T) {
	m, stubs := newReplaceTestTask(500)
	stubs.liveness.work = nil // 作品已软删（GetById 活行 scope miss）
	m.streams = []*streamController{{storeId: 812, relPath: "store/resource/a/x.png"}}

	m.restoreReplaceTargets(context.Background())

	if len(stubs.writer.deletedByStoreIds) != 0 || len(stubs.rows.restoredIds) != 0 || len(stubs.deleter.hardDeleted) != 0 {
		t.Fatalf("作品已软删时回滚应整体跳过（摘关联/复活/物理删均不应发生）")
	}
}

// TestSetFailedTriggersRollback 失败回滚挂接单点：setFailed 触发复活（含 Stop 场景——其 setFailed
// 在 run() 返回后的 pendingCmds 循环里才执行，出口 defer 检查不到 Failed 态，故挂 setFailed）
func TestSetFailedTriggersRollback(t *testing.T) {
	m, stubs := newReplaceTestTask(500)
	res := entity.NewResource()
	res.ID = 700
	res.WorkID = 500
	stubs.res.resources = []*entity.Resource{res}
	stubs.rs.assocs = []*entity.ResourceStore{
		makeReplaceAssoc(700, entity.StoreTypeImage, 0, 810),
		makeReplaceAssoc(700, entity.StoreTypeImage, 0, 811),
	}
	const victimPath = "store/resource/a/被替换.png"
	stubs.rows.rows = []*entity.PersistentStore{
		makeReplaceStoreRow(810, 1, 1000, 91, "store/resource/a/旧代.png"),
		makeReplaceStoreRow(811, 1, 2000, 92, victimPath),
	}

	m.setFailed("任务被用户停止")

	if len(stubs.rows.restoredIds) != 1 || stubs.rows.restoredIds[0] != 811 {
		t.Fatalf("setFailed 应触发回滚复活 victim(811)，实际 %v", stubs.rows.restoredIds)
	}
	if m.GetState() != TaskStateFailed {
		t.Fatalf("setFailed 后任务应为 Failed，实际 %d", m.GetState())
	}
}

// TestRestoreReplaceTargetsNoVictimNoOp 非替换任务（无死行）回滚 no-op：
// 新建行照常清理，复活集为空
func TestRestoreReplaceTargetsNoVictimNoOp(t *testing.T) {
	m, stubs := newReplaceTestTask(500)
	res := entity.NewResource()
	res.ID = 700
	res.WorkID = 500
	stubs.res.resources = []*entity.Resource{res}
	stubs.rs.assocs = []*entity.ResourceStore{
		makeReplaceAssoc(700, entity.StoreTypeImage, 0, 812),
	}
	// 无任何死行（新建作品的常规失败）
	m.streams = []*streamController{{storeId: 812, relPath: "store/resource/a/new.png"}}

	m.restoreReplaceTargets(context.Background())

	if len(stubs.rows.restoredIds) != 0 {
		t.Fatalf("无 victim 时不应复活任何行，实际 %v", stubs.rows.restoredIds)
	}
	if len(stubs.deleter.hardDeleted) != 1 || stubs.deleter.hardDeleted[0] != 812 {
		t.Fatalf("新建行仍应被清理，实际 %v", stubs.deleter.hardDeleted)
	}
}
