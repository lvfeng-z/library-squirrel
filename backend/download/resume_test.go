package download

// 跨重启续传测试：Execute 入口的恢复信号判定（ResumeRequested × pending_resource_id 分叉）、
// 续传主链（偏移推导 → 插件 Resume → 续接挂载事务 → 下载循环 → 成功收口）、全完成直达收口、
// 插件缺失错误文案翻译、资源缺失降级完整重新执行。

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/persistentStore"
	"github.com/library-squirrel/backend/plugin/extension"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// ==== 续传链 stubs ====

// stubWorkTasks 领域行读取桩（Execute 入口/中断通知查行）
type stubWorkTasks struct {
	rows map[int64]*entity.WorkTask
}

func (s *stubWorkTasks) GetById(ctx context.Context, id int64) (*entity.WorkTask, error) {
	return s.rows[id], nil
}

// stubExecFactory 执行器获取桩：恒返回预置执行器
type stubExecFactory struct {
	exec PluginExecutor
}

func (f *stubExecFactory) Executor(pluginPublicId string) (PluginExecutor, error) {
	return f.exec, nil
}

// stubStoreStreamer 落盘流创建/续传桩：记录调用并返回内存 writer
type stubStoreStreamer struct {
	storeCalls       []int64 // StoreStream 新建的 storeId 序列（按分配序）
	resumeCalls      []int64 // ResumeStream 续传的 storeId 序列
	resumeOffsets    []int64
	writers          []*fakeStoreWriter
	storePathCounter int64
}

func (s *stubStoreStreamer) StoreStream(ctx context.Context, relPath string, fileName string) (int64, persistentStore.StoreWriter, error) {
	s.storePathCounter++
	s.storeCalls = append(s.storeCalls, s.storePathCounter)
	w := &fakeStoreWriter{}
	s.writers = append(s.writers, w)
	return s.storePathCounter, w, nil
}

func (s *stubStoreStreamer) ResumeStream(ctx context.Context, storeId int64, offset int64) (persistentStore.StoreWriter, error) {
	s.resumeCalls = append(s.resumeCalls, storeId)
	s.resumeOffsets = append(s.resumeOffsets, offset)
	w := &fakeStoreWriter{}
	s.writers = append(s.writers, w)
	return w, nil
}

// stubStoreReader persistent_store 记录查询桩
type stubStoreReader struct {
	rows map[int64]*entity.PersistentStore
	path string // GetAbsPath 统一返回（指向测试预置文件）
}

func (s *stubStoreReader) GetById(ctx context.Context, id int64) (*entity.PersistentStore, error) {
	return s.rows[id], nil
}

func (s *stubStoreReader) GetAbsPath(store *entity.PersistentStore) string { return s.path }

// stubTransactor 事务桩：直接同步执行
type stubTransactor struct{}

func (stubTransactor) ExecInTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

// stubResourceSaver 资源保存桩
type stubResourceSaver struct {
	saved []int64
}

func (s *stubResourceSaver) Save(ctx context.Context, resource *entity.Resource) (int64, error) {
	id := int64(len(s.saved) + 1)
	s.saved = append(s.saved, id)
	return id, nil
}

func (s *stubResourceSaver) Updates(ctx context.Context, resource *entity.Resource) error { return nil }

// stubStoreFileCleaner 磁盘文件清理桩
type stubStoreFileCleaner struct{}

func (stubStoreFileCleaner) CleanupFile(relPath string) {}

// newResumeTestStrategy 组装续传测试策略：真实依赖为 stub、替换链为 nil（续传路径不触达）；
// backup 为 store 行含删读取桩（活性过滤行预置处，续传关联判活用）
func newResumeTestStrategy(wt *entity.WorkTask, exec PluginExecutor, resReader *stubResourceReader,
	rsReader *stubResourceStoreReader, storeReader *stubStoreReader, streamer *stubStoreStreamer,
	pending *fakePendingUpdater, recompute *stubRecomputer, assocWriter *stubResourceStoreWriter,
	backup *stubStoreBackupReader) (*PluginDownloadStrategy, *Deps) {
	deps := &Deps{
		WorkTasks:              &stubWorkTasks{rows: map[int64]*entity.WorkTask{wt.GetID(): wt}},
		PluginExecFactory:      &stubExecFactory{exec: exec},
		WorkDirProvider:        stubWorkDirProvider{dir: "E:/lib"},
		FileNameFormatProvider: pathTestFormatProvider{format: "[${author}]_[${siteWorkId}]_${siteWorkName}"},
		WorkInfoSaver:          &stubWorkInfoSaver{savedWorkId: 500},
		ResourceReader:         resReader,
		ResourceStoreReader:    rsReader,
		StoreBackupReader:      backup,
		StoreReader:            storeReader,
		StoreStreamer:          streamer,
		ResourceSaver:          &stubResourceSaver{},
		ResourceUpdater:        &stubResourceSaver{},
		ResourceStoreWriter:    assocWriter,
		ResourceRecomputer:     recompute,
		Transactor:             stubTransactor{},
		PendingResourceUpdater: pending,
		StoreFileCleaner:       stubStoreFileCleaner{},
		WorkMetaLoader:         stubWorkMetaLoader{},
	}
	return NewPluginDownloadStrategy(deps), deps
}

// aliveStoreRow 造活行 store 记录（活性过滤预置）
func aliveStoreRow(id int64) *entity.PersistentStore {
	row := entity.NewPersistentStore()
	row.SetID(id)
	return row
}

// makeResumeWorkTask 构造持 pending 的领域行
func makeResumeWorkTask(taskId int64, resourceId int64) *entity.WorkTask {
	wt := entity.NewWorkTask(taskId)
	wt.PluginPublicID = sql.NullString{String: "pub-1", Valid: true}
	wt.ResourceType = sql.NullString{String: entity.ResourceTypeImage, Valid: true}
	wt.PendingResourceID = sql.NullInt64{Int64: resourceId, Valid: true}
	wt.IncludeWorkInfo = true
	return wt
}

// ==== Execute 入口：恢复信号判定 ====

// TestExecuteEntry_ResumeRequestedFlagAndPending 恢复信号 × pending 分叉：
// 信号置位且领域行持 pending → 走跨重启续传（插件 Resume 被调、Start 不被调）；
// 信号未置位（首启/重试/跳过后重跑）或领域行无 pending → 全新执行（板块组合，Start 被调）
func TestExecuteEntry_ResumeRequestedFlagAndPending(t *testing.T) {
	newCase := func(resumeFlag bool, withPending bool) (*PluginDownloadStrategy, *confirmHandle, context.CancelFunc, *fakePluginExec, *stubResourceReader, *stubStoreReader) {
		var pendingId int64 = 700
		wt := entity.NewWorkTask(1)
		wt.PluginPublicID = sql.NullString{String: "pub-1", Valid: true}
		wt.ResourceType = sql.NullString{String: entity.ResourceTypeImage, Valid: true}
		wt.IncludeWorkInfo = true
		if withPending {
			wt.PendingResourceID = sql.NullInt64{Int64: pendingId, Valid: true}
		}
		exec := &fakePluginExec{}
		resReader := &stubResourceReader{}
		storeReader := &stubStoreReader{rows: map[int64]*entity.PersistentStore{}}
		streamer := &stubStoreStreamer{}
		strategy, _ := newResumeTestStrategy(wt, exec, resReader, &stubResourceStoreReader{}, storeReader, streamer,
			&fakePendingUpdater{}, &stubRecomputer{}, &stubResourceStoreWriter{}, &stubStoreBackupReader{})
		h, cancel := newConfirmHandle()
		h.resumeFlag = resumeFlag
		return strategy, h, cancel, exec, resReader, storeReader
	}

	t.Run("信号置位且持pending_走续传", func(t *testing.T) {
		// 完整续传前置：pending 指向的资源可查、有活行关联与已完成 store（全完成直达收口）
		dir := t.TempDir()
		file := dir + "/done.png"
		if err := os.WriteFile(file, []byte("done"), 0o644); err != nil {
			t.Fatalf("预置完成文件失败: %v", err)
		}
		wt := makeResumeWorkTask(1, 700)
		exec := &fakePluginExec{resumeResp: &sdkdto.WorkResponse{}}
		res := entity.NewResource()
		res.ID = 700
		res.WorkID = 500
		storeRow := entity.NewPersistentStore()
		storeRow.SetID(800)
		storeRow.CompletedAt = 1
		strategy, _ := newResumeTestStrategy(wt, exec, &stubResourceReader{byId: res},
			&stubResourceStoreReader{assocs: []*entity.ResourceStore{makeReplaceAssoc(700, entity.StoreTypeImage, 0, 800)}},
			&stubStoreReader{rows: map[int64]*entity.PersistentStore{800: storeRow}, path: file},
			&stubStoreStreamer{}, &fakePendingUpdater{}, &stubRecomputer{}, &stubResourceStoreWriter{},
			&stubStoreBackupReader{rows: []*entity.PersistentStore{aliveStoreRow(800)}})
		h, cancel := newConfirmHandle()
		defer cancel()
		h.resumeFlag = true
		strategy.Execute(h)
		if exec.resumeCalls != 1 {
			t.Fatalf("恢复信号置位且持 pending 应走插件 Resume, 实际 %d 次 (failed=%v msg=%q)", exec.resumeCalls, h.failed, h.failMsg)
		}
		if exec.startCalls != 0 {
			t.Fatalf("续传路径不应走板块组合 Start, 实际 %d 次", exec.startCalls)
		}
	})
	t.Run("信号未置位_全新执行", func(t *testing.T) {
		strategy, h, cancel, exec, _, _ := newCase(false, true)
		defer cancel()
		strategy.Execute(h)
		if exec.startCalls != 1 || exec.resumeCalls != 0 {
			t.Fatalf("首启/重试应走板块组合(Start), 实际 Start=%d Resume=%d", exec.startCalls, exec.resumeCalls)
		}
	})
	t.Run("信号置位但无pending_全新执行", func(t *testing.T) {
		strategy, h, cancel, exec, _, _ := newCase(true, false)
		defer cancel()
		strategy.Execute(h)
		if exec.startCalls != 1 || exec.resumeCalls != 0 {
			t.Fatalf("无 pending 时恢复信号不构成续传, 应走板块组合, 实际 Start=%d Resume=%d", exec.startCalls, exec.resumeCalls)
		}
	})
}

// TestExecuteEntry_MissingWorkTaskFails 领域行缺失：显式失败收口「任务缺少作品领域数据」
func TestExecuteEntry_MissingWorkTaskFails(t *testing.T) {
	exec := &fakePluginExec{}
	wt := entity.NewWorkTask(1) // 占位（仓储内不登记该 id 的行——改用空仓储）
	strategy, _ := newResumeTestStrategy(wt, exec, &stubResourceReader{}, &stubResourceStoreReader{},
		&stubStoreReader{rows: map[int64]*entity.PersistentStore{}}, &stubStoreStreamer{},
		&fakePendingUpdater{}, &stubRecomputer{}, &stubResourceStoreWriter{}, &stubStoreBackupReader{})
	strategy.deps.WorkTasks = &stubWorkTasks{rows: map[int64]*entity.WorkTask{}}
	h, cancel := newConfirmHandle()
	defer cancel()

	strategy.Execute(h)

	if !h.failed || h.failMsg != "任务缺少作品领域数据" {
		t.Fatalf("领域行缺失应显式失败, 实际 failed=%v msg=%q", h.failed, h.failMsg)
	}
}

// ==== 续传主链 ====

// TestResumeFromPersistedState_AllCompleteDirectFinish 全部轨道已完成（状态 Complete 且文件在）：
// 插件 Resume 返回空 specs → 不进下载循环，直接重算完整度、清 pending、成功收口
func TestResumeFromPersistedState_AllCompleteDirectFinish(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "done.png")
	if err := os.WriteFile(file, []byte("done"), 0o644); err != nil {
		t.Fatalf("预置完成文件失败: %v", err)
	}
	wt := makeResumeWorkTask(1, 700)
	exec := &fakePluginExec{resumeSpecs: nil, resumeResp: &sdkdto.WorkResponse{}}
	res := entity.NewResource()
	res.ID = 700
	res.WorkID = 500
	resReader := &stubResourceReader{byId: res}
	rsReader := &stubResourceStoreReader{assocs: []*entity.ResourceStore{
		makeReplaceAssoc(700, entity.StoreTypeImage, 0, 800),
	}}
	storeRow := entity.NewPersistentStore()
	storeRow.SetID(800)
	storeRow.CompletedAt = 1
	storeRow.FilePath = sql.NullString{String: "store/resource/a/done.png", Valid: true}
	storeReader := &stubStoreReader{rows: map[int64]*entity.PersistentStore{800: storeRow}, path: file}
	streamer := &stubStoreStreamer{}
	pending := &fakePendingUpdater{}
	recompute := &stubRecomputer{}
	strategy, _ := newResumeTestStrategy(wt, exec, resReader, rsReader, storeReader, streamer, pending, recompute, &stubResourceStoreWriter{}, &stubStoreBackupReader{rows: []*entity.PersistentStore{aliveStoreRow(800)}})
	h, cancel := newConfirmHandle()
	defer cancel()
	h.resumeFlag = true

	strategy.Execute(h)

	if !h.finished || h.failed {
		t.Fatalf("全完成场景应直接成功收口, 实际 finished=%v failed=%v(%s)", h.finished, h.failed, h.failMsg)
	}
	if exec.resumeCalls != 1 {
		t.Fatalf("应调插件 Resume 一次, 实际 %d", exec.resumeCalls)
	}
	if len(recompute.calledResourceIds) != 1 || recompute.calledResourceIds[0] != 700 {
		t.Fatalf("收口前应重算资源(700)完整度, 实际 %v", recompute.calledResourceIds)
	}
	if len(pending.updates) != 1 || pending.updates[0].id.Valid {
		t.Fatalf("收口应清除 pending_resource_id, 实际 %+v", pending.updates)
	}
}

// TestResumeFromPersistedState_ResumeSpecContinues 未完成轨续传主链：
// 偏移按已落盘文件大小推导 → 插件 Resume 返回未完成 spec → 续传流续接（同 storeId+偏移）→
// 事务内按 storeRows 全量重挂 → 下载循环完成 → 成功收口
func TestResumeFromPersistedState_ResumeSpecContinues(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "partial.png")
	if err := os.WriteFile(file, bytes.Repeat([]byte("a"), 4), 0o644); err != nil {
		t.Fatalf("预置半成品文件失败: %v", err)
	}
	rest := bytes.Repeat([]byte("b"), 6)
	wt := makeResumeWorkTask(1, 700)
	exec := &fakePluginExec{
		resumeSpecs: []*sdkdto.StoreSpec{{
			Role:        entity.StoreTypeImage,
			Generation:  entity.GenerationDownloaded,
			Format:      "png",
			Size:        10,
			ReadCloser:  io.NopCloser(bytes.NewReader(rest)),
			Continuable: boolPtr(true),
		}},
		resumeResp: &sdkdto.WorkResponse{},
	}
	res := entity.NewResource()
	res.ID = 700
	res.WorkID = 500
	resReader := &stubResourceReader{byId: res}
	rsReader := &stubResourceStoreReader{assocs: []*entity.ResourceStore{
		makeReplaceAssoc(700, entity.StoreTypeImage, 0, 800),
	}}
	storeRow := entity.NewPersistentStore()
	storeRow.SetID(800)
	storeRow.CompletedAt = 0
	storeRow.FilePath = sql.NullString{String: "store/resource/a/partial.png", Valid: true}
	storeReader := &stubStoreReader{rows: map[int64]*entity.PersistentStore{800: storeRow}, path: file}
	streamer := &stubStoreStreamer{}
	pending := &fakePendingUpdater{}
	recompute := &stubRecomputer{}
	assocWriter := &stubResourceStoreWriter{}
	strategy, _ := newResumeTestStrategy(wt, exec, resReader, rsReader, storeReader, streamer, pending, recompute, assocWriter, &stubStoreBackupReader{rows: []*entity.PersistentStore{aliveStoreRow(800)}})
	h, cancel := newConfirmHandle()
	defer cancel()
	h.resumeFlag = true

	strategy.Execute(h)

	if !h.finished || h.failed {
		t.Fatalf("续传完成场景应成功收口, 实际 finished=%v failed=%v(%s)", h.finished, h.failed, h.failMsg)
	}
	// 续接判定：同 storeId(800) + 写入偏移=已落盘大小(4)
	if len(streamer.resumeCalls) != 1 || streamer.resumeCalls[0] != 800 {
		t.Fatalf("未完成 downloaded 轨应按已有 store(800) 续接, 实际 %v", streamer.resumeCalls)
	}
	if len(streamer.resumeOffsets) != 1 || streamer.resumeOffsets[0] != 4 {
		t.Fatalf("续传写入偏移应为已落盘大小(4), 实际 %v", streamer.resumeOffsets)
	}
	// 全量重挂：storeRows 全量关联（已完成+续传轨）插入
	if len(assocWriter.created) != 1 || assocWriter.created[0].StoreID != 800 || assocWriter.created[0].StoreType != entity.StoreTypeImage {
		t.Fatalf("事务内应按 storeRows 全量重挂(store 800/image), 实际 %+v", assocWriter.created)
	}
	// 收口清 pending
	if len(pending.updates) == 0 || pending.updates[len(pending.updates)-1].id.Valid {
		t.Fatalf("收口应清除 pending_resource_id, 实际 %+v", pending.updates)
	}
	// 写入完成（4 已落盘 + 6 续传）
	if len(streamer.writers) != 1 || !streamer.writers[0].completed {
		t.Fatalf("续传轨应完成落盘, 实际 %+v", streamer.writers)
	}
	if got := streamer.writers[0].buf.Len(); got != 6 {
		t.Fatalf("续传写入字节期望 6（已落盘 4 不经 writer）, 实际 %d", got)
	}
	// 恢复会话经下载循环收尾：完整度重算以 pending 加载的资源 ID 定位
	// （恢复会话的资源非本次新建，须由续传主体回填会话的产出资源 ID）
	if len(recompute.calledResourceIds) != 1 || recompute.calledResourceIds[0] != 700 {
		t.Fatalf("下载循环完成应以加载的资源(700)重算完整度, 实际 %v", recompute.calledResourceIds)
	}
}

// TestResumeFromPersistedState_PluginNotFoundMessage 插件已停用的 Resume 错误翻译：
// 错误为扩展不存在时用可操作文案，不落泛化续传失败文案
func TestResumeFromPersistedState_PluginNotFoundMessage(t *testing.T) {
	wt := makeResumeWorkTask(1, 700)
	exec := &fakePluginExec{resumeErr: extension.ErrExtensionNotFound}
	res := entity.NewResource()
	res.ID = 700
	res.WorkID = 500
	resReader := &stubResourceReader{byId: res}
	rsReader := &stubResourceStoreReader{assocs: []*entity.ResourceStore{
		makeReplaceAssoc(700, entity.StoreTypeImage, 0, 800),
	}}
	storeRow := entity.NewPersistentStore()
	storeRow.SetID(800)
	storeRow.CompletedAt = 0
	storeReader := &stubStoreReader{rows: map[int64]*entity.PersistentStore{800: storeRow}, path: filepath.Join(t.TempDir(), "absent.png")}
	strategy, _ := newResumeTestStrategy(wt, exec, resReader, rsReader, storeReader, &stubStoreStreamer{},
		&fakePendingUpdater{}, &stubRecomputer{}, &stubResourceStoreWriter{}, &stubStoreBackupReader{rows: []*entity.PersistentStore{aliveStoreRow(800)}})
	h, cancel := newConfirmHandle()
	defer cancel()
	h.resumeFlag = true

	strategy.Execute(h)

	if !h.failed {
		t.Fatal("插件缺失应失败收口")
	}
	if h.failMsg != "插件已停止运行，请确认插件已启用后重试" {
		t.Fatalf("插件缺失文案期望可操作引导, 实际 %q", h.failMsg)
	}
}

// TestResumeFromPersistedState_GenericErrorKeepsMessage 其余 Resume 错误保持泛化续传失败文案
func TestResumeFromPersistedState_GenericErrorKeepsMessage(t *testing.T) {
	wt := makeResumeWorkTask(1, 700)
	exec := &fakePluginExec{resumeErr: errors.New("connection reset")}
	res := entity.NewResource()
	res.ID = 700
	res.WorkID = 500
	resReader := &stubResourceReader{byId: res}
	rsReader := &stubResourceStoreReader{assocs: []*entity.ResourceStore{
		makeReplaceAssoc(700, entity.StoreTypeImage, 0, 800),
	}}
	storeRow := entity.NewPersistentStore()
	storeRow.SetID(800)
	storeRow.CompletedAt = 0
	storeReader := &stubStoreReader{rows: map[int64]*entity.PersistentStore{800: storeRow}, path: filepath.Join(t.TempDir(), "absent.png")}
	strategy, _ := newResumeTestStrategy(wt, exec, resReader, rsReader, storeReader, &stubStoreStreamer{},
		&fakePendingUpdater{}, &stubRecomputer{}, &stubResourceStoreWriter{}, &stubStoreBackupReader{rows: []*entity.PersistentStore{aliveStoreRow(800)}})
	h, cancel := newConfirmHandle()
	defer cancel()
	h.resumeFlag = true

	strategy.Execute(h)

	if !h.failed {
		t.Fatal("续传错误应失败收口")
	}
	if h.failMsg != "跨重启续传失败: connection reset" {
		t.Fatalf("非插件缺失错误应保持泛化文案, 实际 %q", h.failMsg)
	}
}

// TestResumeDegradestoFullRerun 资源缺失降级：pending 指向的 resource 查不到 →
// 降级完整重新执行（板块组合，Start 被调）
func TestResumeDegradestoFullRerun(t *testing.T) {
	wt := makeResumeWorkTask(1, 700)
	exec := &fakePluginExec{}
	strategy, _ := newResumeTestStrategy(wt, exec, &stubResourceReader{}, &stubResourceStoreReader{},
		&stubStoreReader{rows: map[int64]*entity.PersistentStore{}}, &stubStoreStreamer{},
		&fakePendingUpdater{}, &stubRecomputer{}, &stubResourceStoreWriter{}, &stubStoreBackupReader{})
	h, cancel := newConfirmHandle()
	defer cancel()
	h.resumeFlag = true

	strategy.Execute(h)

	if exec.startCalls != 1 {
		t.Fatalf("资源缺失应降级完整重新执行(Start), 实际 %d 次", exec.startCalls)
	}
	if exec.resumeCalls != 0 {
		t.Fatalf("降级路径不应调用插件 Resume, 实际 %d 次", exec.resumeCalls)
	}
	// Start 桩报错 → 失败收口（降级链贯通板块组合）
	if !h.failed {
		t.Fatal("降级板块组合后应由 Start 桩报错转失败收口")
	}
}

// boolPtr 测试辅助
func boolPtr(b bool) *bool { return &b }
