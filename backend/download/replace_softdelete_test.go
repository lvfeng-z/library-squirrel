package download

// 提交窗口替换软删与终端回滚登记测试：软删分派（已完成→备份软删；未完成→废弃软删）、
// 软删后经 SetTerminalRollback 登记受害者清单（由软删返回值构建，提交序列失败时复活归
// 控制面 setFailed 单点）、会话失败收口只关闭写入句柄（复活不在会话侧执行）、中断于下载
// 窗口零软删零登记（替换链坍缩：长下载全程零 DB 副作用）。
// 替换链能力注入真实 resource.ReplacementService（本文件 stub 作其依赖接口）——软删/分派
// 逻辑在 resource 域执行，断言仍落到同一批 stub 上。

import (
	"bytes"
	"context"
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/resource"
	"github.com/library-squirrel/backend/shareLock"
	"github.com/library-squirrel/backend/taskManager"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"

	"gorm.io/plugin/soft_delete"
)

// ==== 回滚链 stubs（combo/resume 测试复用）====

// stubResourceReader 资源查询桩（替换软删链取作品资源）
type stubResourceReader struct {
	resources []*entity.Resource
	byId      *entity.Resource
}

func (f *stubResourceReader) ListByWorkId(ctx context.Context, workId int64) ([]*entity.Resource, error) {
	return f.resources, nil
}

func (f *stubResourceReader) GetById(ctx context.Context, id int64) (*entity.Resource, error) {
	return f.byId, nil
}

// stubResourceStoreReader 关联查询桩（ListByResourceIds 供替换软删/回滚派生）
type stubResourceStoreReader struct {
	assocs        []*entity.ResourceStore
	byResourceIds []*entity.ResourceStore
}

func (f *stubResourceStoreReader) ListByResourceId(ctx context.Context, resourceId int64) ([]*entity.ResourceStore, error) {
	return f.assocs, nil
}

func (f *stubResourceStoreReader) ListByResourceIds(ctx context.Context, resourceIds []int64) ([]*entity.ResourceStore, error) {
	return f.byResourceIds, nil
}

// stubStoreBackupReader store 行含删读取与复活桩（软删判活与回滚复活共用）
type stubStoreBackupReader struct {
	rows        []*entity.PersistentStore
	restoredIds []int64
}

func (f *stubStoreBackupReader) ListByIdsIncludeDeleted(ctx context.Context, ids []int64) []*entity.PersistentStore {
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

func (f *stubStoreBackupReader) RestoreByIds(ctx context.Context, ids []int64) error {
	f.restoredIds = append(f.restoredIds, ids...)
	return nil
}

// stubStoreReplacer 替换软删原语桩：记录软删调用（备份软删/废弃软删分流），备份行 ID 递增
type stubStoreReplacer struct {
	backupIds    []int64
	discardedIds []int64
	nextBackupId int64
}

func (f *stubStoreReplacer) DeleteWithBackup(ctx context.Context, id int64) (int64, error) {
	f.backupIds = append(f.backupIds, id)
	f.nextBackupId++
	return f.nextBackupId, nil
}

func (f *stubStoreReplacer) SoftDeleteAndDiscardFile(ctx context.Context, id int64) error {
	f.discardedIds = append(f.discardedIds, id)
	return nil
}

// stubWorkLivenessReader 作品活性桩：nil work = 已软删
type stubWorkLivenessReader struct {
	work *entity.Work
}

func (f *stubWorkLivenessReader) GetById(ctx context.Context, id int64) (*entity.Work, error) {
	return f.work, nil
}

// stubResourceStoreWriter 关联写入桩：记录 DeleteByStoreIds 调用
type stubResourceStoreWriter struct {
	deletedByStoreIds []int64
	created           []*entity.ResourceStore
}

func (f *stubResourceStoreWriter) CreateBatch(ctx context.Context, stores []*entity.ResourceStore) error {
	f.created = append(f.created, stores...)
	return nil
}

func (f *stubResourceStoreWriter) DeleteByResourceIdAndTypes(ctx context.Context, resourceId int64, storeTypes []string) error {
	return nil
}

func (f *stubResourceStoreWriter) DeleteByStoreIds(ctx context.Context, storeIds []int64) error {
	f.deletedByStoreIds = append(f.deletedByStoreIds, storeIds...)
	return nil
}

// stubStoreDeleter HardDelete 桩（清理本次新建 store 用）
type stubStoreDeleter struct {
	hardDeleted []int64
}

func (f *stubStoreDeleter) HardDelete(ctx context.Context, id int64, backup bool) (int64, error) {
	f.hardDeleted = append(f.hardDeleted, id)
	return 0, nil
}

// stubBackupFileRestorer 备份文件还原桩：记录还原与删除的清单行 ID
type stubBackupFileRestorer struct {
	deletedBackupIds []int64
}

func (f *stubBackupFileRestorer) GetById(ctx context.Context, id int64) (*entity.Backup, error) {
	b := entity.NewBackup()
	b.SetID(id)
	return b, nil
}

func (f *stubBackupFileRestorer) GetBackupPath(backup *entity.Backup) string { return "" }

func (f *stubBackupFileRestorer) RestoreFile(ctx context.Context, backupPath string, targetPath string) error {
	return nil
}

func (f *stubBackupFileRestorer) DeleteBackup(ctx context.Context, id int64) error {
	f.deletedBackupIds = append(f.deletedBackupIds, id)
	return nil
}

// stubRecomputer 完整度重算记账桩
type stubRecomputer struct {
	calledResourceIds []int64
}

func (f *stubRecomputer) RecomputeResourceComplete(ctx context.Context, resourceId int64) {
	f.calledResourceIds = append(f.calledResourceIds, resourceId)
}

// stubWorkDirProvider 工作目录桩
type stubWorkDirProvider struct {
	dir string
}

func (p stubWorkDirProvider) GetWorkDir() string { return p.dir }

// replaceStubs 替换链测试的一批 stub（真实 ReplacementService 以其为依赖组装）
type replaceStubs struct {
	res       *stubResourceReader
	rs        *stubResourceStoreReader
	rows      *stubStoreBackupReader
	replacer  *stubStoreReplacer
	liveness  *stubWorkLivenessReader
	writer    *stubResourceStoreWriter
	deleter   *stubStoreDeleter
	restorer  *stubBackupFileRestorer
	recompute *stubRecomputer
}

func newReplaceStubs() *replaceStubs {
	return &replaceStubs{
		res:       &stubResourceReader{},
		rs:        &stubResourceStoreReader{},
		rows:      &stubStoreBackupReader{},
		replacer:  &stubStoreReplacer{},
		liveness:  &stubWorkLivenessReader{work: entity.NewWork()},
		writer:    &stubResourceStoreWriter{},
		deleter:   &stubStoreDeleter{},
		restorer:  &stubBackupFileRestorer{},
		recompute: &stubRecomputer{},
	}
}

// newRealReplacementService 以本文件 stub 组装真实 resource 替换链能力（软删/分派/回滚逻辑
// 在 resource 域执行，断言落到 stub 上）；lock 可传持锁注册中心构造拒绝分支
func newRealReplacementService(stubs *replaceStubs, lock shareLock.ShareLockRegistry) resource.ReplaceStoreOps {
	if lock == nil {
		lock = shareLock.NewShareLockRegistry()
	}
	return resource.NewReplacementService(
		stubs.res, stubs.rs, stubs.rows, stubs.replacer,
		stubs.restorer, stubs.liveness, stubs.recompute,
		stubWorkDirProvider{dir: "E:/lib"},
		lock,
		stubs.writer,
		stubs.deleter,
	)
}

// makeReplaceStoreRow 造一行 persistent_store（供 ListByIdsIncludeDeleted 桩返回）
func makeReplaceStoreRow(id int64, completed int64, deletedAt int64, backupId int64, path string) *entity.PersistentStore {
	row := entity.NewPersistentStore()
	row.SetID(id)
	row.CompletedAt = completed
	if deletedAt > 0 {
		row.DeletedAt = soft_delete.DeletedAt(deletedAt)
	}
	row.BackupID = sql.NullInt64{Int64: backupId, Valid: true}
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

// TestSoftDeleteReplacedStoresDispatchesByCompletion 提交窗口软删分派（板块选择=仅 image）：
// 已完成行走备份软删、未完成行走废弃软删、历史残留死行跳过、角色过滤外的不动
func TestSoftDeleteReplacedStoresDispatchesByCompletion(t *testing.T) {
	sess, h, _, stubs := newReplaceTestSession(500)
	sess.mode = runMode{storeScope: storeScope{kind: scopeSelected, roles: []string{entity.StoreTypeImage}}}
	res := entity.NewResource()
	res.ID = 700
	res.WorkID = 500
	stubs.res.resources = []*entity.Resource{res}
	stubs.rs.byResourceIds = []*entity.ResourceStore{
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

	if err := sess.softDeleteReplacedStores(); err != nil {
		t.Fatalf("软删成功不应返回错误, 实际 %v", err)
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
	// 被软删行清单经 SetTerminalRollback 登记（由软删返回值构建，供控制面失败单点复活）
	if len(h.rollbacks) != 1 {
		t.Fatalf("软删成功应登记终端回滚，实际 %d 次", len(h.rollbacks))
	}
	got := h.rollbacks[0].Victims
	if len(got) != 2 {
		t.Fatalf("登记受害者应含备份软删(800)与废弃软删(801)两行，实际 %v", got)
	}
	if got[0].StoreID != 800 || got[0].BackupID == 0 || got[0].ResourceID != 700 {
		t.Fatalf("受害者(800)应携带资源 ID 与备份行 ID，实际 %+v", got[0])
	}
	if got[1].StoreID != 801 || got[1].BackupID != 0 {
		t.Fatalf("受害者(801)为无备份废弃分支，实际 %+v", got[1])
	}
}

// TestFailPathDelegatesRevivalToControlPlane 替换场景失败收口：会话只关闭流写入句柄，
// 软删受害者的复活统一由控制面 setFailed 单点按登记清单执行——会话侧不直接复活，
// 中断路径（停止/恢复后失败）与会话收口路径共用控制面单点
func TestFailPathDelegatesRevivalToControlPlane(t *testing.T) {
	sess, h, cancel, stubs := newReplaceTestSession(500)
	defer cancel()
	stream := newStream(t, entity.StoreTypeImage, entity.GenerationDownloaded, 10, nil)
	sess.streams = []*streamController{stream}
	// 提交窗口软删后的登记形态（软删与登记在 commitStaged 内一体发生，此处直填登记
	// 模拟软删之后序列失败的收口路径）
	sess.handle.SetTerminalRollback(taskManager.TerminalRollback{Victims: []resource.StoreRef{
		{StoreID: 800, ResourceID: 700, BackupID: 1, FilePath: "store/resource/a/old.png"},
	}})
	sess.mode = runMode{storeScope: storeScope{kind: scopeAll}}

	sess.comboFail("下载失败")

	if !stream.writer.closed {
		t.Fatal("失败收口前应关闭流写入句柄（释放文件锁，控制面回滚还原文件需要）")
	}
	if len(stubs.rows.restoredIds) != 0 {
		t.Fatalf("会话侧不应直接复活受害者, 实际 %v", stubs.rows.restoredIds)
	}
	var victims []int64
	for _, rb := range h.rollbacks {
		for _, v := range rb.Victims {
			victims = append(victims, v.StoreID)
		}
	}
	if len(victims) != 1 || victims[0] != 800 {
		t.Fatalf("受害者(800)应保留登记（控制面回滚据此复活）, 实际 %v", victims)
	}
	if !h.failed {
		t.Fatal("含资源板块失败应上报失败终态")
	}
}

// newReplaceTestSession 构造替换链测试会话（真实 ReplacementService + stub 依赖），
// 板块模式=All（软删/回滚生效角色经 storeScope 三态派生）
func newReplaceTestSession(workId int64) (*execSession, *confirmHandle, context.CancelFunc, *replaceStubs) {
	stubs := newReplaceStubs()
	deps := &Deps{
		WorkDirProvider:     stubWorkDirProvider{dir: "E:/lib"},
		ResourceReader:      stubs.res,
		ResourceStoreWriter: stubs.writer,
		ReplaceStoreOps:     newRealReplacementService(stubs, nil),
	}
	h, cancel := newConfirmHandle()
	wt := entity.NewWorkTask(1)
	sess := newExecSession(deps, h, wt)
	sess.workId = workId
	sess.mode = runMode{storeScope: storeScope{kind: scopeAll}}
	return sess, h, cancel, stubs
}

// TestStartDownloadInterruptRegistersNoRollback 替换执行中被停止（runCtx 取消、执行面
// 不收口直接返回）时零回滚登记——替换链坍缩后软删在提交窗口，下载窗口中断时旧 store 未动、
// 暂存保留，无任何回滚需求。readers 阻塞至 runCtx 取消，覆盖下载循环内中断形态
func TestStartDownloadInterruptRegistersNoRollback(t *testing.T) {
	env := newStagingTestEnv(t)
	h, cancel := newConfirmHandle()
	defer cancel()
	ctx := h.runCtx
	deps := &Deps{
		SiteKeyResolver:     &fakeSiteKeyResolver{keys: map[int64]string{1: "test-site"}},
		WorkDirProvider:     stubWorkDirProvider{dir: env.workDir},
		Transactor:          stubTransactor{},
		ResourceReader:      &stubResourceReader{},
		ResourceSaver:       &stubResourceSaver{},
		ResourceUpdater:     &stubResourceSaver{},
		StoreCommitter:      env.streamer,
		ResourceStoreWriter: env.assocWrite,
		StagingPaths:        stubStagingPaths{},
	}
	wt := makeResumeWorkTask(1)
	sess := newExecSession(deps, h, wt)
	sess.workId = 500
	blocked := []chan struct{}{make(chan struct{}), make(chan struct{})}
	specs := []*sdkdto.StoreSpec{
		{Role: entity.StoreTypeImage, Generation: entity.GenerationDownloaded, Format: "png",
			ReadCloser: &ctxAwareReader{data: []byte("aa"), ctx: ctx, blockedCh: blocked[0]}},
		{Role: entity.StoreTypeThumbnail, Generation: entity.GenerationDownloaded, Format: "jpg",
			ReadCloser: &ctxAwareReader{data: []byte("bb"), ctx: ctx, blockedCh: blocked[1]}},
	}
	// 两流数据读尽阻塞后模拟停止的 runCancel（执行面据此中断返回、不上报终态）
	go func() {
		<-blocked[0]
		<-blocked[1]
		cancel()
	}()

	res := sess.startDownload(specs)
	if res != comboInterrupted {
		t.Fatalf("runCtx 取消应中断返回,实际 %v", res)
	}
	if len(h.rollbacks) != 0 {
		t.Fatalf("中断于下载窗口应零回滚登记（软删未发生，旧 store 不动）,实际 %v", h.rollbacks)
	}
	// 暂存保留（两轨部分写入落暂存），无任何建行
	if len(env.streamer.commits) != 0 {
		t.Fatalf("中断不应建行,实际 %d", len(env.streamer.commits))
	}
	for _, name := range []string{"image_000.png", "thumbnail_000.jpg"} {
		if _, err := os.Stat(filepath.Join(env.stagingDir, name)); err != nil {
			t.Fatalf("中断暂存应保留(%s): %v", name, err)
		}
	}
}

// TestResumeCommitRegistersNoRollbackLedger 暂停→恢复的会话：续接轨（Resume 认领）与重产轨
// （未认领经 Start）在提交点建行后零回滚登记——替换链坍缩后 plugin-download 不登记新建行
// （提交事务原子性兜底建行段，长下载窗口零 DB 副作用）
func TestResumeCommitRegistersNoRollbackLedger(t *testing.T) {
	env := newStagingTestEnv(t)
	seedStaging(t, env, entity.StoreTypeImage, 0, bytes.Repeat([]byte("a"), 4))
	seedStaging(t, env, entity.StoreTypeThumbnail, 0, []byte("t"))
	wt := makeResumeWorkTask(1)
	exec := &fakePluginExec{
		resumeSpecs: []*sdkdto.StoreSpec{{
			Role: entity.StoreTypeImage, Generation: entity.GenerationDownloaded, Format: "png",
			Size: 10, Continuable: boolPtr(true),
			ReadCloser: io.NopCloser(bytes.NewReader(bytes.Repeat([]byte("b"), 6))),
		}},
		resumeResp: &sdkdto.WorkResponse{},
		// 未认领的 thumbnail 轨经 Start 重产（恢复会话的重建轨）
		startSpecs: []*sdkdto.StoreSpec{{
			Role: entity.StoreTypeThumbnail, Generation: entity.GenerationDerived, Format: "jpg",
			ReadCloser: io.NopCloser(bytes.NewReader([]byte("t"))),
		}},
	}
	strategy, _ := newResumeTestStrategy(wt, exec, &stubResourceReader{resources: []*entity.Resource{}}, env)
	h, cancel := newConfirmHandle()
	defer cancel()
	h.resumeFlag = true

	strategy.Execute(h)

	if !h.finished || h.failed {
		t.Fatalf("续传+重产完成应成功收口,实际 finished=%v failed=%v(%s)", h.finished, h.failed, h.failMsg)
	}
	if len(h.rollbacks) != 0 {
		t.Fatalf("恢复会话提交应零回滚登记（软删无对象或成功即终态清空）,实际 %v", h.rollbacks)
	}
	if len(env.streamer.commits) != 2 {
		t.Fatalf("续接轨与重产轨应各建一行,实际 %d", len(env.streamer.commits))
	}
}
