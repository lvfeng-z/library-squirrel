package download

// 跨重启续传测试（暂存模式）：Execute 入口的恢复信号判定（ResumeRequested × 暂存目录
// 非空分叉）、续传主链（暂存枚举偏移 → 插件 Resume → 认领配对/未认领重产 → 暂存续接 →
// 下载循环 → 提交点收口）、ResumeWriteOffset 插件指定优先、写满轨瞬时完成、陈旧暂存重建、
// 无认领显式失败保留暂存、作品定位失败降级完整重新执行、插件缺失错误文案翻译。

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/library-squirrel/backend/base/model/entity"
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

// stubStoreCommitter 提交点建行桩：记录建行参数并分配递增行 ID；hook 供抑制登记时点断言
type stubStoreCommitter struct {
	commits []stubCommitRecord
	nextId  int64
	hook    func(s *stubStoreCommitter)
}

type stubCommitRecord struct {
	relPath  string
	fileName string
	expected sql.NullString
	actual   sql.NullString
}

func (s *stubStoreCommitter) CommitStore(ctx context.Context, relPath string, fileName string, expectedSha, actualSha sql.NullString) (int64, error) {
	if s.hook != nil {
		s.hook(s)
	}
	s.nextId++
	s.commits = append(s.commits, stubCommitRecord{relPath: relPath, fileName: fileName, expected: expectedSha, actual: actualSha})
	return s.nextId, nil
}

// stubWorkLocator 任务所属作品定位桩
type stubWorkLocator struct {
	work *entity.Work
	err  error
}

func (s *stubWorkLocator) GetBySiteAndSiteWorkID(ctx context.Context, siteId int64, siteWorkId string) (*entity.Work, error) {
	return s.work, s.err
}

// stubStagingPaths 暂存目录派生桩（与 task.StagingPath/StagingFileName 同构；download 包
// 不 import task，测试自造同构实现）
type stubStagingPaths struct{}

func (p stubStagingPaths) StagingPath(workDir string, taskID int64) string {
	return filepath.Join(workDir, "task-staging", strconv.FormatInt(taskID, 10))
}

func (p stubStagingPaths) StagingFileName(role string, storeSeq int, ext string) string {
	return fmt.Sprintf("%s_%03d%s", role, storeSeq, ext)
}

// stagingTestEnv 续传/提交点测试环境：临时 workDir + 已就位依赖（真实暂存文件落盘）
type stagingTestEnv struct {
	workDir    string
	stagingDir string
	streamer   *stubStoreCommitter
	recompute  *stubRecomputer
	assocWrite *stubResourceStoreWriter
	locator    *stubWorkLocator
}

// newStagingTestEnv 构造测试环境（workDir=t.TempDir()，暂存目录含预置种子文件的父目录）
func newStagingTestEnv(t *testing.T) *stagingTestEnv {
	t.Helper()
	workDir := t.TempDir()
	env := &stagingTestEnv{
		workDir:    workDir,
		stagingDir: stubStagingPaths{}.StagingPath(workDir, 1),
		streamer:   &stubStoreCommitter{},
		recompute:  &stubRecomputer{},
		assocWrite: &stubResourceStoreWriter{},
		locator:    &stubWorkLocator{work: makeStagingWork()},
	}
	return env
}

// makeStagingWork 造任务所属作品（ID=500）
func makeStagingWork() *entity.Work {
	w := entity.NewWork()
	w.ID = 500
	return w
}

// seedStaging 预置暂存轨道文件（文件名 role_seq.ext，内容重复字节）
func seedStaging(t *testing.T, env *stagingTestEnv, role string, seq int, content []byte) {
	t.Helper()
	if err := os.MkdirAll(env.stagingDir, 0o755); err != nil {
		t.Fatalf("创建暂存目录失败: %v", err)
	}
	name := stubStagingPaths{}.StagingFileName(role, seq, ".png")
	if err := os.WriteFile(filepath.Join(env.stagingDir, name), content, 0o644); err != nil {
		t.Fatalf("预置暂存文件失败: %v", err)
	}
}

// newResumeTestStrategy 组装续传测试策略（真实暂存文件 + 桩依赖；workLocator 定位作品 500）
func newResumeTestStrategy(wt *entity.WorkTask, exec PluginExecutor, resReader *stubResourceReader,
	env *stagingTestEnv) (*PluginDownloadStrategy, *Deps) {
	deps := &Deps{
		WorkTasks:           &stubWorkTasks{rows: map[int64]*entity.WorkTask{wt.GetID(): wt}},
		PluginExecFactory:   &stubExecFactory{exec: exec},
		WorkDirProvider:     stubWorkDirProvider{dir: env.workDir},
		SiteKeyResolver:     &fakeSiteKeyResolver{keys: map[int64]string{1: "test-site"}},
		WorkInfoSaver:       &stubWorkInfoSaver{savedWorkId: 500},
		WorkLocator:         env.locator,
		ResourceReader:      resReader,
		ResourceSaver:       &stubResourceSaver{},
		ResourceUpdater:     &stubResourceSaver{},
		ReplaceStoreOps:     newRealReplacementService(newReplaceStubs(), nil),
		StoreCommitter:      env.streamer,
		ResourceStoreWriter: env.assocWrite,
		ResourceRecomputer:  env.recompute,
		Transactor:          stubTransactor{},
		StagingPaths:        stubStagingPaths{},
	}
	return NewPluginDownloadStrategy(deps), deps
}

// makeResumeWorkTask 构造持站点复合键的领域行（暂存模式无 pending，恢复定位走复合键）
func makeResumeWorkTask(taskId int64) *entity.WorkTask {
	wt := entity.NewWorkTask(taskId)
	wt.PluginPublicID = sql.NullString{String: "pub-1", Valid: true}
	wt.ResourceType = sql.NullString{String: entity.ResourceTypeImage, Valid: true}
	wt.SiteID = sql.NullInt64{Int64: 1, Valid: true}
	wt.SiteWorkID = sql.NullString{String: "sw1", Valid: true}
	wt.IncludeWorkInfo = true
	return wt
}

// stagingWorkResp 作品信息响应桩载荷（Start/Resume 返回的作品元数据，落盘派生不消费，
// 仅保持插件执行器桩返回形态完整）
func stagingWorkResp() *sdkdto.WorkResponse {
	return &sdkdto.WorkResponse{
		Work:         &sdkdto.WorkDTO{SiteWorkId: pathTestStrPtr("sw1"), SiteWorkName: pathTestStrPtr("name")},
		LocalAuthors: []*sdkdto.LocalAuthorDTO{{AuthorName: pathTestStrPtr("author")}},
	}
}

// finalAbsPath 命名派生基准下的最终文件绝对路径（makeResumeWorkTask 复合键 siteId=1→
// test-site、siteWorkId=sw1 → store/resource/{bucketOf("test-site","sw1")}/test-site_sw1 目录）
func finalAbsPath(env *stagingTestEnv, fileName string) string {
	return filepath.Join(env.workDir, "store", "resource", bucketOf("test-site", "sw1"), "test-site_sw1", fileName)
}

// ==== Execute 入口：恢复信号判定 ====

// TestExecuteEntry_ResumeRequestedFlagAndStaging 恢复信号 × 暂存分叉：
// 信号置位且暂存有轨道文件 → 走跨重启续传（插件 Resume 被调、Start 仅在未认领重产时被调）；
// 信号未置位（首启/重试）或暂存为空（旧模式暂停任务无暂存锚）→ 全新执行（板块组合 Start）
func TestExecuteEntry_ResumeRequestedFlagAndStaging(t *testing.T) {
	t.Run("信号置位且有暂存_走续传", func(t *testing.T) {
		env := newStagingTestEnv(t)
		seedStaging(t, env, entity.StoreTypeImage, 0, bytes.Repeat([]byte("a"), 4))
		wt := makeResumeWorkTask(1)
		exec := &fakePluginExec{
			resumeSpecs: []*sdkdto.StoreSpec{{
				Role: entity.StoreTypeImage, Generation: entity.GenerationDownloaded, Format: "png",
				Size: 10, Continuable: boolPtr(true),
				ReadCloser: io.NopCloser(bytes.NewReader(bytes.Repeat([]byte("b"), 6))),
			}},
			resumeResp: stagingWorkResp(),
		}
		strategy, _ := newResumeTestStrategy(wt, exec, &stubResourceReader{resources: []*entity.Resource{}}, env)
		h, cancel := newConfirmHandle()
		defer cancel()
		h.resumeFlag = true

		strategy.Execute(h)

		if exec.resumeCalls != 1 {
			t.Fatalf("恢复信号置位且有暂存应走插件 Resume, 实际 %d 次 (failed=%v msg=%q)", exec.resumeCalls, h.failed, h.failMsg)
		}
		if !h.finished {
			t.Fatalf("续传完成应成功收口 (failed=%v msg=%q)", h.failed, h.failMsg)
		}
	})
	t.Run("信号未置位_全新执行", func(t *testing.T) {
		env := newStagingTestEnv(t)
		seedStaging(t, env, entity.StoreTypeImage, 0, []byte("a"))
		wt := makeResumeWorkTask(1)
		exec := &fakePluginExec{}
		strategy, _ := newResumeTestStrategy(wt, exec, &stubResourceReader{}, env)
		h, cancel := newConfirmHandle()
		defer cancel()

		strategy.Execute(h)

		if exec.startCalls != 1 || exec.resumeCalls != 0 {
			t.Fatalf("首启/重试应走板块组合(Start), 实际 Start=%d Resume=%d", exec.startCalls, exec.resumeCalls)
		}
	})
	t.Run("信号置位但暂存为空_全新执行(旧模式暂停任务降级)", func(t *testing.T) {
		env := newStagingTestEnv(t)
		wt := makeResumeWorkTask(1)
		exec := &fakePluginExec{}
		strategy, _ := newResumeTestStrategy(wt, exec, &stubResourceReader{}, env)
		h, cancel := newConfirmHandle()
		defer cancel()
		h.resumeFlag = true

		strategy.Execute(h)

		if exec.startCalls != 1 || exec.resumeCalls != 0 {
			t.Fatalf("无暂存时恢复信号不构成续传(降级全新重下), 实际 Start=%d Resume=%d", exec.startCalls, exec.resumeCalls)
		}
	})
}

// TestExecuteEntry_MissingWorkTaskFails 领域行缺失：显式失败收口「任务缺少作品领域数据」
func TestExecuteEntry_MissingWorkTaskFails(t *testing.T) {
	env := newStagingTestEnv(t)
	exec := &fakePluginExec{}
	wt := entity.NewWorkTask(1) // 占位（仓储内不登记该 id 的行——改用空仓储）
	strategy, _ := newResumeTestStrategy(wt, exec, &stubResourceReader{}, env)
	strategy.deps.WorkTasks = &stubWorkTasks{rows: map[int64]*entity.WorkTask{}}
	h, cancel := newConfirmHandle()
	defer cancel()

	strategy.Execute(h)

	if !h.failed || h.failMsg != "任务缺少作品领域数据" {
		t.Fatalf("领域行缺失应显式失败, 实际 failed=%v msg=%q", h.failed, h.failMsg)
	}
}

// ==== 续传主链 ====

// TestResumeFromStaging_PartialContinuesAndCommits 未完成轨续传主链：
// 暂存枚举偏移下发插件 Resume（StreamOffsets）→ 认领轨按已落盘偏移续接（前缀保留）→
// 全部写满走提交点（rename 到最终路径+建行+挂载+pending 清+Finish）→ 暂存目录回收
func TestResumeFromStaging_PartialContinuesAndCommits(t *testing.T) {
	env := newStagingTestEnv(t)
	seedStaging(t, env, entity.StoreTypeImage, 0, bytes.Repeat([]byte("a"), 4))
	wt := makeResumeWorkTask(1)
	prefix := bytes.Repeat([]byte("a"), 4)
	rest := bytes.Repeat([]byte("b"), 6)
	var gotOffsets []*sdkdto.StoreResumeOffset
	exec := &fakePluginExec{
		resumeSpecs: []*sdkdto.StoreSpec{{
			Role: entity.StoreTypeImage, Generation: entity.GenerationDownloaded, Format: "png",
			Size: 10, Continuable: boolPtr(true),
			ReadCloser: io.NopCloser(bytes.NewReader(rest)),
		}},
		resumeResp:     stagingWorkResp(),
		captureOffsets: func(o []*sdkdto.StoreResumeOffset) { gotOffsets = o },
	}
	strategy, _ := newResumeTestStrategy(wt, exec, &stubResourceReader{resources: []*entity.Resource{}}, env)
	h, cancel := newConfirmHandle()
	defer cancel()
	h.resumeFlag = true

	strategy.Execute(h)

	if !h.finished || h.failed {
		t.Fatalf("续传完成场景应成功收口, 实际 finished=%v failed=%v(%s)", h.finished, h.failed, h.failMsg)
	}
	// 下发的续传锚 = 暂存 stat 偏移（4），身份化 role+store_seq
	if len(gotOffsets) != 1 || gotOffsets[0].Role != entity.StoreTypeImage || gotOffsets[0].StoreSeq != 0 || gotOffsets[0].Offset != 4 {
		t.Fatalf("StreamOffsets 应为 (image,0,offset=4), 实际 %+v", gotOffsets)
	}
	// 提交点：暂存 rename 到最终路径，内容 = 前缀 4 字节 + 续传 6 字节
	want := append(append([]byte{}, prefix...), rest...)
	got, rerr := os.ReadFile(finalAbsPath(env, "image_000.png"))
	if rerr != nil {
		t.Fatalf("最终文件应存在: %v", rerr)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("最终内容应前缀保留+续传, 期望 %q 实际 %q", want, got)
	}
	// 建行：relPath 落库、completed（桩记录）
	wantRel := path.Join("store", "resource", bucketOf("test-site", "sw1"), "test-site_sw1", "image_000.png")
	if len(env.streamer.commits) != 1 || env.streamer.commits[0].relPath != wantRel {
		t.Fatalf("提交点应建 1 行(final relPath), 实际 %+v", env.streamer.commits)
	}
	// 挂载：resource_store 关联插入
	if len(env.assocWrite.created) != 1 || env.assocWrite.created[0].StoreType != entity.StoreTypeImage {
		t.Fatalf("提交点应挂载 1 条关联(image), 实际 %+v", env.assocWrite.created)
	}
	// 暂存目录回收
	if _, err := os.Stat(env.stagingDir); !os.IsNotExist(err) {
		t.Fatalf("提交后暂存目录应回收, stat err=%v", err)
	}
	// 完整度重算以提交事务建出的资源定位
	if len(env.recompute.calledResourceIds) != 1 || env.recompute.calledResourceIds[0] != 1 {
		t.Fatalf("收口应重算资源(1)完整度, 实际 %v", env.recompute.calledResourceIds)
	}
}

// TestResumeFromStaging_ResumeWriteOffsetPluginPriority 插件指定写入偏移优先（风险锚）：
// 暂存 4 字节但插件指定 writeOffset=2 → 暂存截断到 2 后续写（插件对续传位置有确切认知）
func TestResumeFromStaging_ResumeWriteOffsetPluginPriority(t *testing.T) {
	env := newStagingTestEnv(t)
	seedStaging(t, env, entity.StoreTypeImage, 0, []byte("aaaa"))
	wt := makeResumeWorkTask(1)
	off := int64(2)
	exec := &fakePluginExec{
		resumeSpecs: []*sdkdto.StoreSpec{{
			Role: entity.StoreTypeImage, Generation: entity.GenerationDownloaded, Format: "png",
			Size: 8, Continuable: boolPtr(true), ResumeWriteOffset: &off,
			ReadCloser: io.NopCloser(bytes.NewReader([]byte("bbbbbb"))),
		}},
		resumeResp: stagingWorkResp(),
	}
	strategy, _ := newResumeTestStrategy(wt, exec, &stubResourceReader{resources: []*entity.Resource{}}, env)
	h, cancel := newConfirmHandle()
	defer cancel()
	h.resumeFlag = true

	strategy.Execute(h)

	if !h.finished || h.failed {
		t.Fatalf("应成功收口, 实际 finished=%v failed=%v(%s)", h.finished, h.failed, h.failMsg)
	}
	got, rerr := os.ReadFile(finalAbsPath(env, "image_000.png"))
	if rerr != nil {
		t.Fatalf("最终文件应存在: %v", rerr)
	}
	if string(got) != "aabbbbbb" {
		t.Fatalf("插件指定偏移应截断到 2 后续写, 期望 aabbbbbb 实际 %q", got)
	}
}

// TestResumeFromStaging_CompleteInStagingInstantCommit 写满轨瞬时完成：暂存字节数==声明
// 大小 → 续接打开（偏移=满），reader 立即 EOF → 完整性判定通过 → 直接进提交点
func TestResumeFromStaging_CompleteInStagingInstantCommit(t *testing.T) {
	env := newStagingTestEnv(t)
	seedStaging(t, env, entity.StoreTypeImage, 0, bytes.Repeat([]byte("c"), 10))
	wt := makeResumeWorkTask(1)
	exec := &fakePluginExec{
		resumeSpecs: []*sdkdto.StoreSpec{{
			Role: entity.StoreTypeImage, Generation: entity.GenerationDownloaded, Format: "png",
			Size: 10, Continuable: boolPtr(true),
			ReadCloser: io.NopCloser(bytes.NewReader(nil)), // 立即 EOF（如 416 转译形态）
		}},
		resumeResp: stagingWorkResp(),
	}
	strategy, _ := newResumeTestStrategy(wt, exec, &stubResourceReader{resources: []*entity.Resource{}}, env)
	h, cancel := newConfirmHandle()
	defer cancel()
	h.resumeFlag = true

	strategy.Execute(h)

	if !h.finished || h.failed {
		t.Fatalf("写满轨应瞬时完成提交, 实际 finished=%v failed=%v(%s)", h.finished, h.failed, h.failMsg)
	}
	got, rerr := os.ReadFile(finalAbsPath(env, "image_000.png"))
	if rerr != nil || len(got) != 10 {
		t.Fatalf("写满轨提交内容应保留, err=%v len=%d", rerr, len(got))
	}
}

// TestResumeFromStaging_StaleStagingRebuilds 陈旧暂存重建：暂存字节数超过声明大小（内容/
// 清单变更过的旧暂存）→ 截断重下（不从超限偏移续接）
func TestResumeFromStaging_StaleStagingRebuilds(t *testing.T) {
	env := newStagingTestEnv(t)
	seedStaging(t, env, entity.StoreTypeImage, 0, bytes.Repeat([]byte("x"), 20))
	wt := makeResumeWorkTask(1)
	exec := &fakePluginExec{
		resumeSpecs: []*sdkdto.StoreSpec{{
			Role: entity.StoreTypeImage, Generation: entity.GenerationDownloaded, Format: "png",
			Size: 10, Continuable: boolPtr(true),
			ReadCloser: io.NopCloser(bytes.NewReader(bytes.Repeat([]byte("y"), 10))),
		}},
		resumeResp: stagingWorkResp(),
	}
	strategy, _ := newResumeTestStrategy(wt, exec, &stubResourceReader{resources: []*entity.Resource{}}, env)
	h, cancel := newConfirmHandle()
	defer cancel()
	h.resumeFlag = true

	strategy.Execute(h)

	if !h.finished || h.failed {
		t.Fatalf("陈旧暂存应重建后成功, 实际 finished=%v failed=%v(%s)", h.finished, h.failed, h.failMsg)
	}
	got, rerr := os.ReadFile(finalAbsPath(env, "image_000.png"))
	if rerr != nil || string(got) != string(bytes.Repeat([]byte("y"), 10)) {
		t.Fatalf("陈旧暂存应整轨重下, err=%v got=%q", rerr, got)
	}
}

// TestResumeFromStaging_UncoveredRegenerates 未认领轨重产（derived 现语义）：Resume 只认领
// downloaded 轨，暂存中的 thumbnail 轨未被认领 → 经 Start(role) 整轨重产 → 与续接轨一同提交
func TestResumeFromStaging_UncoveredRegenerates(t *testing.T) {
	env := newStagingTestEnv(t)
	seedStaging(t, env, entity.StoreTypeImage, 0, bytes.Repeat([]byte("a"), 4))
	seedStaging(t, env, entity.StoreTypeThumbnail, 0, []byte("t"))
	wt := makeResumeWorkTask(1)
	var startRoles []string
	exec := &fakePluginExec{
		resumeSpecs: []*sdkdto.StoreSpec{{
			Role: entity.StoreTypeImage, Generation: entity.GenerationDownloaded, Format: "png",
			Size: 6, Continuable: boolPtr(true),
			ReadCloser: io.NopCloser(bytes.NewReader(bytes.Repeat([]byte("b"), 2))),
		}},
		resumeResp: stagingWorkResp(),
		startSpecs: []*sdkdto.StoreSpec{{
			Role: entity.StoreTypeThumbnail, Generation: entity.GenerationDerived, Format: "jpg",
			ReadCloser: io.NopCloser(bytes.NewReader([]byte("thumb!"))),
		}},
		captureStartRoles: func(roles []string) { startRoles = roles },
	}
	strategy, _ := newResumeTestStrategy(wt, exec, &stubResourceReader{resources: []*entity.Resource{}}, env)
	h, cancel := newConfirmHandle()
	defer cancel()
	h.resumeFlag = true

	strategy.Execute(h)

	if !h.finished || h.failed {
		t.Fatalf("未认领轨重产完成应成功, 实际 finished=%v failed=%v(%s)", h.finished, h.failed, h.failMsg)
	}
	if len(startRoles) != 1 || startRoles[0] != entity.StoreTypeThumbnail {
		t.Fatalf("未认领轨应以 [thumbnail] 重产, 实际 %v", startRoles)
	}
	// 两轨均提交：续接 image(4+2) 与重产 thumbnail(整轨覆盖旧暂存)；文件名恒带 role+seq 段
	imgGot, err1 := os.ReadFile(finalAbsPath(env, "image_000.png"))
	thumbGot, err2 := os.ReadFile(finalAbsPath(env, "thumbnail_000.jpg"))
	if err1 != nil || string(imgGot) != "aaaabb" {
		t.Fatalf("续接轨提交内容不符: err=%v got=%q", err1, imgGot)
	}
	if err2 != nil || string(thumbGot) != "thumb!" {
		t.Fatalf("重产轨提交内容不符: err=%v got=%q", err2, thumbGot)
	}
	if len(env.streamer.commits) != 2 {
		t.Fatalf("两轨均应建行, 实际 %d", len(env.streamer.commits))
	}
}

// TestResumeFromStaging_NoClaimFailsKeepsStaging 暂存非空但插件对全部轨道既不续传也不重产：
// 显式失败保留暂存（无提交元数据不可静默丢失），重试/重新执行可覆盖
func TestResumeFromStaging_NoClaimFailsKeepsStaging(t *testing.T) {
	env := newStagingTestEnv(t)
	seedStaging(t, env, entity.StoreTypeImage, 0, []byte("aaaa"))
	wt := makeResumeWorkTask(1)
	exec := &fakePluginExec{
		resumeResp: stagingWorkResp(),
		startSpecs: []*sdkdto.StoreSpec{}, // Resume 与 Start 均零认领
	}
	strategy, _ := newResumeTestStrategy(wt, exec, &stubResourceReader{}, env)
	h, cancel := newConfirmHandle()
	defer cancel()
	h.resumeFlag = true

	strategy.Execute(h)

	if !h.failed {
		t.Fatal("零认领应显式失败")
	}
	if h.failMsg != "插件未返回待续传资源，暂存已保留，请重试或重新执行任务" {
		t.Fatalf("零认领失败文案应可操作, 实际 %q", h.failMsg)
	}
	// 暂存保留（失败不清理，重试可覆盖）
	if _, err := os.Stat(filepath.Join(env.stagingDir, "image_000.png")); err != nil {
		t.Fatalf("失败应保留暂存: %v", err)
	}
	if len(env.streamer.commits) != 0 {
		t.Fatalf("失败不应建行, 实际 %d", len(env.streamer.commits))
	}
}

// TestResumeWorkMissingDegradesToFullRerun 作品定位失败（作品已删等）降级完整重新执行：
// 走板块组合（Start 被调）
func TestResumeWorkMissingDegradesToFullRerun(t *testing.T) {
	env := newStagingTestEnv(t)
	seedStaging(t, env, entity.StoreTypeImage, 0, []byte("aaaa"))
	env.locator.work = nil
	wt := makeResumeWorkTask(1)
	exec := &fakePluginExec{}
	strategy, _ := newResumeTestStrategy(wt, exec, &stubResourceReader{}, env)
	h, cancel := newConfirmHandle()
	defer cancel()
	h.resumeFlag = true

	strategy.Execute(h)

	if exec.startCalls != 1 {
		t.Fatalf("作品定位失败应降级完整重新执行(Start), 实际 %d 次", exec.startCalls)
	}
	if exec.resumeCalls != 0 {
		t.Fatalf("降级路径不应调用插件 Resume, 实际 %d 次", exec.resumeCalls)
	}
}

// TestResumeFromStaging_PluginNotFoundMessage 插件已停用的 Resume 错误翻译：
// 错误为扩展不存在时用可操作文案，不落泛化续传失败文案
func TestResumeFromStaging_PluginNotFoundMessage(t *testing.T) {
	env := newStagingTestEnv(t)
	seedStaging(t, env, entity.StoreTypeImage, 0, []byte("aaaa"))
	wt := makeResumeWorkTask(1)
	exec := &fakePluginExec{resumeErr: extension.ErrExtensionNotFound}
	strategy, _ := newResumeTestStrategy(wt, exec, &stubResourceReader{}, env)
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

// TestResumeFromStaging_GenericErrorKeepsMessage 其余 Resume 错误保持泛化续传失败文案
func TestResumeFromStaging_GenericErrorKeepsMessage(t *testing.T) {
	env := newStagingTestEnv(t)
	seedStaging(t, env, entity.StoreTypeImage, 0, []byte("aaaa"))
	wt := makeResumeWorkTask(1)
	exec := &fakePluginExec{resumeErr: errors.New("connection reset")}
	strategy, _ := newResumeTestStrategy(wt, exec, &stubResourceReader{}, env)
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

// ==== spec 配对 ====

// TestPairResumeSpecs Resume 认领配对：spec 按角色消费暂存轨队列（同 role 多轨按序对齐），
// 未认领轨按序返回；spec 数超出暂存轨数按最大序递增
func TestPairResumeSpecs(t *testing.T) {
	entries := []stagingEntry{
		{role: entity.StoreTypeImage, seq: 0, size: 5},
		{role: entity.StoreTypeImage, seq: 1, size: 0},
		{role: entity.StoreTypeThumbnail, seq: 0, size: 3},
	}
	img1 := &sdkdto.StoreSpec{Role: entity.StoreTypeImage, Generation: entity.GenerationDownloaded}
	img2 := &sdkdto.StoreSpec{Role: entity.StoreTypeImage, Generation: entity.GenerationDownloaded}
	extra := &sdkdto.StoreSpec{Role: entity.StoreTypeImage, Generation: entity.GenerationDownloaded}
	newRole := &sdkdto.StoreSpec{Role: entity.StoreTypeDocument, Generation: entity.GenerationDerived}
	specs := []*sdkdto.StoreSpec{img1, img2, extra, newRole}

	seqBySpec, uncovered := pairResumeSpecs(entries, specs)

	if seqBySpec[img1] != 0 || seqBySpec[img2] != 1 {
		t.Fatalf("同 role 多轨应按暂存序配对(0,1), 实际 %v/%v", seqBySpec[img1], seqBySpec[img2])
	}
	if seqBySpec[extra] != 2 {
		t.Fatalf("超出暂存轨数的 spec 应递增分配(2), 实际 %v", seqBySpec[extra])
	}
	if seqBySpec[newRole] != 0 {
		t.Fatalf("暂存无该 role 轨的首个新轨应从 0 起, 实际 %v", seqBySpec[newRole])
	}
	if len(uncovered) != 1 || uncovered[0].role != entity.StoreTypeThumbnail {
		t.Fatalf("未认领应为 thumbnail 轨, 实际 %+v", uncovered)
	}
}

// boolPtr 测试辅助
func boolPtr(b bool) *bool { return &b }
