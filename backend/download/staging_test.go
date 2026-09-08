package download

// 暂存写入器与提交点测试：写入流全量 sha256（全新/续传前缀入哈希）、finalize 比对（不符/
// 未声明）、暂存文件名解析、规划注册面（GetStoreRelPath 运行形态契约）、提交点序列
// （替换软删+rename+抑制登记窗口+建行挂载事务+暂存回收）、哈希双列落库、提交失败逆 rename
// 回退、哈希不符失败保留暂存、替换矩阵（替换链坍缩到提交窗口：成功/失败/提交窗口内失败/
// 软删锁拒绝/暂停停止四态+全角色展开）。

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/duplicate"
	"github.com/library-squirrel/backend/shareLock"
	"github.com/library-squirrel/backend/storeRegistry"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// ==== 暂存写入器 ====

// TestStagingWriter_FreshHashesStream 全新写入流全量哈希边写边算
func TestStagingWriter_FreshHashesStream(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image_000.png")
	w, err := newStagingWriterFresh(path, "")
	if err != nil {
		t.Fatalf("打开失败: %v", err)
	}
	data := bytes.Repeat([]byte("z"), 100)
	if _, err := w.Write(data[:40]); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if _, err := w.Write(data[40:]); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	actual, ferr := w.finalize()
	if ferr != nil {
		t.Fatalf("未声明期望应跳过比对: %v", ferr)
	}
	sum := sha256.Sum256(data)
	if actual != hex.EncodeToString(sum[:]) {
		t.Fatalf("实测哈希不符: 期望 %s 实际 %s", hex.EncodeToString(sum[:]), actual)
	}
	if !w.closed {
		t.Fatal("finalize 应关闭句柄")
	}
}

// TestStagingWriter_ResumePreHashesPrefix 续传打开把已落盘前缀读入哈希器：跨会话实测哈希
// 与文件内容一致（续写部分与前缀合并计算）
func TestStagingWriter_ResumePreHashesPrefix(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image_000.png")
	prefix := bytes.Repeat([]byte("a"), 4)
	if err := os.WriteFile(path, prefix, 0o644); err != nil {
		t.Fatalf("预置前缀失败: %v", err)
	}
	w, err := newStagingWriterResume(path, 4, "")
	if err != nil {
		t.Fatalf("续传打开失败: %v", err)
	}
	if _, err := w.Write([]byte("bb")); err != nil {
		t.Fatalf("续写失败: %v", err)
	}
	actual, ferr := w.finalize()
	if ferr != nil {
		t.Fatalf("finalize 失败: %v", ferr)
	}
	sum := sha256.Sum256([]byte("aaaabb"))
	if actual != hex.EncodeToString(sum[:]) {
		t.Fatalf("前缀+续写合并哈希不符: 期望 %s 实际 %s", hex.EncodeToString(sum[:]), actual)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "aaaabb" {
		t.Fatalf("文件内容应前缀+续写, 实际 %q", got)
	}
}

// TestStagingWriter_FinalizeMismatch 声明哈希不符：finalize 返回错误（任务失败收口用）
func TestStagingWriter_FinalizeMismatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image_000.png")
	w, err := newStagingWriterFresh(path, "deadbeef")
	if err != nil {
		t.Fatalf("打开失败: %v", err)
	}
	if _, err := w.Write([]byte("payload")); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if _, ferr := w.finalize(); ferr == nil {
		t.Fatal("声明不符应返回错误")
	}
	// 暂存保留（诊断可见，重试重下覆盖）
	if _, serr := os.Stat(path); serr != nil {
		t.Fatalf("不符暂存应保留: %v", serr)
	}
}

// TestStagingWriter_TruncateOnResume 续传打开截断到偏移（TOCTOU 多余数据丢弃）
func TestStagingWriter_TruncateOnResume(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image_000.png")
	if err := os.WriteFile(path, []byte("aaaaXXXX"), 0o644); err != nil {
		t.Fatalf("预置失败: %v", err)
	}
	w, err := newStagingWriterResume(path, 4, "")
	if err != nil {
		t.Fatalf("续传打开失败: %v", err)
	}
	if _, err := w.Write([]byte("bb")); err != nil {
		t.Fatalf("续写失败: %v", err)
	}
	_ = w.Close()
	got, _ := os.ReadFile(path)
	if string(got) != "aaaabb" {
		t.Fatalf("偏移后多余数据应截断, 实际 %q", got)
	}
}

// TestParseStagingFileName 暂存文件名 role_seq 键解析（StagingFileName 的逆）
func TestParseStagingFileName(t *testing.T) {
	role, seq, err := parseStagingFileName("videoTrack_007.mp4")
	if err != nil || role != "videoTrack" || seq != 7 {
		t.Fatalf("解析期望 (videoTrack,7), 实际 (%s,%d,%v)", role, seq, err)
	}
	if _, _, err := parseStagingFileName("noisy.txt"); err == nil {
		t.Fatal("无 role_seq 键应解析失败")
	}
}

// TestEnumerateStaging 暂存枚举：role_seq 还原 + 大小 + 稳定排序；不可解析文件跳过
func TestEnumerateStaging(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"image_001.png":     "aa",
		"image_000.png":     "aaaa",
		"thumbnail_000.jpg": "t",
		"stray.txt":         "x",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("预置 %s 失败: %v", name, err)
		}
	}
	entries, err := enumerateStaging(dir)
	if err != nil {
		t.Fatalf("枚举失败: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("不可解析文件应跳过, 期望 3 轨实际 %d: %+v", len(entries), entries)
	}
	// 稳定排序 (role, seq)：image#0, image#1, thumbnail#0
	want := []struct {
		role string
		seq  int
		size int64
	}{
		{entity.StoreTypeImage, 0, 4},
		{entity.StoreTypeImage, 1, 2},
		{entity.StoreTypeThumbnail, 0, 1},
	}
	for i, w := range want {
		if entries[i].role != w.role || entries[i].seq != w.seq || entries[i].size != w.size {
			t.Fatalf("条目 %d 期望 %+v 实际 %+v", i, w, entries[i])
		}
	}
}

// ==== 规划注册面（GetStoreRelPath 运行形态契约） ====

// TestStagingPlannerLifecycle 注册/查询/注销：命中返回最终路径；注销后回落（false）
func TestStagingPlannerLifecycle(t *testing.T) {
	p := NewStagingPlanner()
	p.Register(7, map[storeIdentity]string{
		{role: entity.StoreTypeImage, seq: 2}: "store/resource/a/x_image_002.png",
	})
	if rel, ok := p.FinalRelPath(7, entity.StoreTypeImage, 2); !ok || rel != "store/resource/a/x_image_002.png" {
		t.Fatalf("命中查询应返回注册的最终路径, ok=%v rel=%q", ok, rel)
	}
	if _, ok := p.FinalRelPath(7, entity.StoreTypeImage, 0); ok {
		t.Fatal("未注册 seq 不应命中")
	}
	if _, ok := p.FinalRelPath(8, entity.StoreTypeImage, 2); ok {
		t.Fatal("未注册任务不应命中")
	}
	p.Unregister(7)
	if _, ok := p.FinalRelPath(7, entity.StoreTypeImage, 2); ok {
		t.Fatal("注销后不应命中")
	}
}

// TestPlannerDuringDownload_RunningLazyContract 运行中 lazy 契约（GetStoreRelPath 双形态的
// 运行形态源）：下载循环进行中（文件物理在暂存、最终路径无文件）规划表即返回最终路径——
// 插件 document lazy 生成依赖此契约取兄弟轨最终文件名，且不再依赖落盘事务可见性
func TestPlannerDuringDownload_RunningLazyContract(t *testing.T) {
	sess, h, cancel, env := newCommitTestSession(t)
	defer cancel()
	// 在途 reader 阻塞至 runCtx 取消：循环进行期间断言规划表
	cr := &ctxAwareReader{data: []byte("aa"), ctx: h.runCtx, blockedCh: make(chan struct{})}
	specs := []*sdkdto.StoreSpec{{
		Role: entity.StoreTypeImage, Generation: entity.GenerationDownloaded, Format: "png",
		Size: 10, ReadCloser: cr,
	}}

	done := make(chan comboResult, 1)
	go func() { done <- sess.startDownload(specs, stagingWorkResp()) }()

	<-cr.blockedCh // 循环进行中（在途读取挂起）

	rel, ok := env.planner.FinalRelPath(1, entity.StoreTypeImage, 0)
	if !ok {
		t.Fatal("运行中规划表应命中")
	}
	wantRel := "store/resource/author/[author]_[sw1]_name.png"
	if rel != wantRel {
		t.Fatalf("规划表应返回最终路径 %q, 实际 %q", wantRel, rel)
	}
	// 文件物理在暂存（部分写入），最终路径无文件——契约与物理位置解耦
	if _, err := os.Stat(filepath.Join(env.workDir, rel)); !os.IsNotExist(err) {
		t.Fatalf("暂存期内最终路径不应有文件, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(env.stagingDir, "image_000.png")); err != nil {
		t.Fatalf("暂存文件应在位: %v", err)
	}

	cancel() // 释放挂起的读取（中断返回）
	res := <-done
	if res != comboInterrupted {
		t.Fatalf("runCtx 取消应中断返回, 实际 %v", res)
	}
	// 中断返回后暂存保留、无任何建行
	if _, err := os.Stat(filepath.Join(env.stagingDir, "image_000.png")); err != nil {
		t.Fatalf("中断暂存应保留: %v", err)
	}
	if len(env.streamer.commits) != 0 {
		t.Fatalf("中断不应建行, 实际 %d", len(env.streamer.commits))
	}
}

// ==== 提交点 ====

// newCommitTestSession 构造提交点测试会话（真实暂存文件 + 桩依赖，taskId=1，workId=500）
func newCommitTestSession(t *testing.T) (*execSession, *confirmHandle, context.CancelFunc, *stagingTestEnv) {
	t.Helper()
	env := newStagingTestEnv(t)
	h, cancel := newConfirmHandle()
	wt := makeResumeWorkTask(1)
	deps := &Deps{
		WorkDirProvider:        stubWorkDirProvider{dir: env.workDir},
		FileNameFormatProvider: pathTestFormatProvider{format: "[${author}]_[${siteWorkId}]_${siteWorkName}"},
		ResourceReader:         &stubResourceReader{resources: []*entity.Resource{}},
		ResourceSaver:          &stubResourceSaver{},
		ResourceUpdater:        &stubResourceSaver{},
		StoreCommitter:         env.streamer,
		ResourceStoreWriter:    env.assocWrite,
		ResourceRecomputer:     env.recompute,
		Transactor:             stubTransactor{},
		StagingPaths:           stubStagingPaths{},
		Planner:                env.planner,
	}
	sess := newExecSession(deps, h, wt)
	sess.workId = 500
	sess.mode = runMode{storeScope: storeScope{kind: scopeAll}}
	return sess, h, cancel, env
}

// TestCommitAndFinish_RenamesCreatesRowsAndFinishes 提交点主链：全部写满 → rename 到最终
// 路径 → 事务建行（哈希双列）+挂载 → 完整度重算+Finish →
// 暂存目录回收。rename→建行事务内（rename 已完成）最终路径处于抑制登记态（抑制登记纪律）
func TestCommitAndFinish_RenamesCreatesRowsAndFinishes(t *testing.T) {
	sess, h, cancel, env := newCommitTestSession(t)
	defer cancel()
	// 抑制窗口断言：建行事务内（rename 已完成）最终路径处于抑制登记态
	suppressedInView := false
	env.streamer.hook = func(*stubStoreCommitter) {
		suppressedInView = storeRegistry.IsSuppressed("store/resource/author/[author]_[sw1]_name.png")
	}

	specs := []*sdkdto.StoreSpec{
		{Role: entity.StoreTypeImage, Generation: entity.GenerationDownloaded, Format: "png", Size: 3,
			ReadCloser: io.NopCloser(bytes.NewReader([]byte("abc")))},
		{Role: entity.StoreTypeThumbnail, Generation: entity.GenerationDerived, Format: "jpg", Size: 0,
			ReadCloser: io.NopCloser(bytes.NewReader([]byte("t")))},
	}

	res := sess.startDownload(specs, stagingWorkResp())

	if res != comboFinished {
		t.Fatalf("期望 comboFinished, 实际 %v", res)
	}
	if !h.finished || h.failed {
		t.Fatalf("提交点应成功收口: finished=%v failed=%v(%s)", h.finished, h.failed, h.failMsg)
	}
	if !suppressedInView {
		t.Fatal("rename→建行事务窗口内最终路径应处于抑制登记态（fsmonitor 误裁决防线）")
	}
	// 两轨最终文件就位
	for _, name := range []string{"[author]_[sw1]_name_image_000.png", "[author]_[sw1]_name_thumbnail_000.jpg"} {
		if _, err := os.Stat(finalAbsPath(env, name)); err != nil {
			t.Fatalf("最终文件应就位(%s): %v", name, err)
		}
	}
	// 建行：两行（多 store 判定成立，role+seq 消歧命名）
	if len(env.streamer.commits) != 2 {
		t.Fatalf("期望 2 行建行, 实际 %d: %+v", len(env.streamer.commits), env.streamer.commits)
	}
	if env.streamer.commits[0].fileName != "[author]_[sw1]_name_image_000.png" {
		t.Fatalf("建行 fileName 应为最终文件名, 实际 %q", env.streamer.commits[0].fileName)
	}
	// 挂载关联
	if len(env.assocWrite.created) != 2 {
		t.Fatalf("期望 2 条挂载关联, 实际 %d", len(env.assocWrite.created))
	}
	// 非替换提交零回滚登记（替换链坍缩后 plugin-download 不登记新建行，成功即终态）
	if len(h.rollbacks) != 0 {
		t.Fatalf("非替换提交应零回滚登记, 实际 %v", h.rollbacks)
	}
	// 完整度重算 + 暂存目录回收
	if len(env.recompute.calledResourceIds) != 1 {
		t.Fatalf("收口应重算完整度, 实际 %v", env.recompute.calledResourceIds)
	}
	if _, err := os.Stat(env.stagingDir); !os.IsNotExist(err) {
		t.Fatalf("提交后暂存目录应回收, stat err=%v", err)
	}
}

// TestCommitHashColumnsPassThrough 哈希双列落库：声明哈希透传 expected 列、实测哈希落
// actual 列；未声明轨 expected 为无效态（NULL）
func TestCommitHashColumnsPassThrough(t *testing.T) {
	sess, _, cancel, env := newCommitTestSession(t)
	defer cancel()
	payload := []byte("hashed")
	sum := sha256.Sum256(payload)
	declared := hex.EncodeToString(sum[:])
	sess.deps.Planner = env.planner
	specs := []*sdkdto.StoreSpec{
		{Role: entity.StoreTypeImage, Generation: entity.GenerationDownloaded, Format: "png", Size: int64(len(payload)),
			ExpectedSha256: &declared,
			ReadCloser:     io.NopCloser(bytes.NewReader(payload))},
	}

	res := sess.startDownload(specs, stagingWorkResp())

	if res != comboFinished || !sess.handleFinished() {
		t.Fatalf("声明且符应成功提交, res=%v failed=%v", res, sess.failMsgView())
	}
	if len(env.streamer.commits) != 1 {
		t.Fatalf("期望 1 行, 实际 %d", len(env.streamer.commits))
	}
	c := env.streamer.commits[0]
	if !c.expected.Valid || c.expected.String != declared {
		t.Fatalf("声明哈希应透传 expected 列, 实际 %+v", c.expected)
	}
	if !c.actual.Valid || c.actual.String != declared {
		t.Fatalf("实测哈希应落 actual 列, 实际 %+v", c.actual)
	}
}

// TestDownloadHashMismatchFailsKeepsStaging 声明哈希不符：任务 Fail（可操作文案）+暂存保留
// （诊断可见，重试重下覆盖）+零建行 +最终路径零残留
func TestDownloadHashMismatchFailsKeepsStaging(t *testing.T) {
	sess, h, cancel, env := newCommitTestSession(t)
	defer cancel()
	bad := "deadbeef"
	specs := []*sdkdto.StoreSpec{
		{Role: entity.StoreTypeImage, Generation: entity.GenerationDownloaded, Format: "png", Size: 6,
			ExpectedSha256: &bad,
			ReadCloser:     io.NopCloser(bytes.NewReader([]byte("actual!")))},
	}

	res := sess.startDownload(specs, stagingWorkResp())

	if res != comboFinished || !h.failed {
		t.Fatal("哈希不符应失败收口")
	}
	wantMsg := "资源完整性校验失败（image）：来源声明的哈希与下载内容不符"
	if len(h.failMsg) < len(wantMsg) || h.failMsg[:len(wantMsg)] != wantMsg {
		t.Fatalf("失败文案应以 %q 开头, 实际 %q", wantMsg, h.failMsg)
	}
	if _, err := os.Stat(filepath.Join(env.stagingDir, "image_000.png")); err != nil {
		t.Fatalf("不符暂存应保留: %v", err)
	}
	if len(env.streamer.commits) != 0 {
		t.Fatalf("不符不应建行, 实际 %d", len(env.streamer.commits))
	}
	if _, err := os.Stat(finalAbsPath(env, "[author]_[sw1]_name.png")); !os.IsNotExist(err) {
		t.Fatal("store/ 最终路径应零残留")
	}
}

// TestCommitTxFailureRevertsToStaging 提交事务失败补偿：已 rename 轨逆 rename 回退暂存
// （暂存保留供重试），零建行零挂载
func TestCommitTxFailureRevertsToStaging(t *testing.T) {
	sess, h, cancel, env := newCommitTestSession(t)
	defer cancel()
	env.streamer.hook = func(*stubStoreCommitter) {} // 占位（保持 hook 非 nil 语义清晰）
	// 让第二轨建行失败：事务内返回错误
	failing := &failingCommitter{delegate: env.streamer, failOnNth: 2}
	sess.deps.StoreCommitter = failing

	specs := []*sdkdto.StoreSpec{
		{Role: entity.StoreTypeImage, Generation: entity.GenerationDownloaded, Format: "png", Size: 3,
			ReadCloser: io.NopCloser(bytes.NewReader([]byte("abc")))},
		{Role: entity.StoreTypeThumbnail, Generation: entity.GenerationDerived, Format: "jpg", Size: 0,
			ReadCloser: io.NopCloser(bytes.NewReader([]byte("t")))},
	}

	res := sess.startDownload(specs, stagingWorkResp())

	if res != comboFinished || !h.failed {
		t.Fatal("提交失败应失败收口")
	}
	// 补偿：两轨均回退暂存（首轨已 rename 也回退）
	for _, name := range []string{"image_000.png", "thumbnail_000.jpg"} {
		if _, err := os.Stat(filepath.Join(env.stagingDir, name)); err != nil {
			t.Fatalf("提交失败轨应回退暂存(%s): %v", name, err)
		}
	}
	if len(env.assocWrite.created) != 0 {
		t.Fatalf("失败不应挂载, 实际 %d", len(env.assocWrite.created))
	}
}

// failingCommitter 前 n-1 次委派真桩、第 n 次失败（事务失败注入）
type failingCommitter struct {
	delegate  *stubStoreCommitter
	failOnNth int64
	n         int64
}

func (f *failingCommitter) CommitStore(ctx context.Context, relPath string, fileName string, expectedSha, actualSha sql.NullString) (int64, error) {
	f.n++
	if f.n == f.failOnNth {
		return 0, fmt.Errorf("注入建行失败")
	}
	return f.delegate.CommitStore(ctx, relPath, fileName, expectedSha, actualSha)
}

// handleFinished / failMsgView 会话侧终态观测（经 confirmHandle 记录）
func (sess *execSession) handleFinished() bool {
	if ch, ok := sess.handle.(*confirmHandle); ok {
		ch.mu.Lock()
		defer ch.mu.Unlock()
		return ch.finished && !ch.failed
	}
	return false
}

func (sess *execSession) failMsgView() string {
	if ch, ok := sess.handle.(*confirmHandle); ok {
		ch.mu.Lock()
		defer ch.mu.Unlock()
		return ch.failMsg
	}
	return ""
}

// ==== 替换矩阵（替换链坍缩到提交窗口：长下载全程零 DB 副作用） ====

// newReplaceStagingSession 组装替换矩阵会话：真实 ReplacementService（stub 依赖）+ 暂存桩依赖，
// 查重命中无冲突定位替换（existingWorkId=500）。lock 非 nil 时注入作品锁注册中心（守卫用例）
func newReplaceStagingSession(t *testing.T, lock shareLock.ShareLockRegistry) (*execSession, *confirmHandle, context.CancelFunc, *stagingTestEnv, *replaceStubs) {
	t.Helper()
	env := newStagingTestEnv(t)
	stubs := newReplaceStubs()
	h, cancel := newConfirmHandle()
	wt := makeResumeWorkTask(1)
	deps := &Deps{
		WorkDirProvider:        stubWorkDirProvider{dir: env.workDir},
		FileNameFormatProvider: pathTestFormatProvider{format: "[${author}]_[${siteWorkId}]_${siteWorkName}"},
		DuplicateChecker:       &fakeDupChecker{result: duplicate.DuplicateCheckResult{Class: duplicate.DuplicateHitNoConflict, WorkID: 500, WorkName: "已存在作品"}},
		SiteKeyResolver:        &fakeSiteKeyResolver{keys: map[int64]string{1: "test-site"}},
		WorkInfoSaver:          &stubWorkInfoSaver{savedWorkId: 500},
		ResourceReader:         stubs.res,
		ReplaceStoreOps:        newRealReplacementService(stubs, lock),
		ResourceSaver:          &stubResourceSaver{},
		ResourceUpdater:        &stubResourceSaver{},
		StoreCommitter:         env.streamer,
		ResourceStoreWriter:    env.assocWrite,
		ResourceRecomputer:     env.recompute,
		Transactor:             stubTransactor{},
		StagingPaths:           stubStagingPaths{},
		Planner:                env.planner,
	}
	sess := newExecSession(deps, h, wt)
	sess.mode = runMode{storeScope: storeScope{kind: scopeAll}}
	return sess, h, cancel, env, stubs
}

// seedReplaceVictims 预置替换受害者图（两条已完成活行，覆盖 All 模式全角色展开）：
// 作品 500 → 资源 700 → image(800) 与 thumbnail(801) 已完成活行
func seedReplaceVictims(stubs *replaceStubs) {
	res := entity.NewResource()
	res.ID = 700
	res.WorkID = 500
	stubs.res.resources = []*entity.Resource{res}
	stubs.rs.byResourceIds = []*entity.ResourceStore{
		makeReplaceAssoc(700, entity.StoreTypeImage, 0, 800),
		makeReplaceAssoc(700, entity.StoreTypeThumbnail, 0, 801),
	}
	stubs.rows.rows = []*entity.PersistentStore{
		makeReplaceStoreRow(800, 1, 0, 0, "store/resource/a/old_image.png"),
		makeReplaceStoreRow(801, 1, 0, 0, "store/resource/a/old_thumb.jpg"),
	}
}

// seedReplaceIncompleteVictim 预置未完成受害者（软删走废弃分支——替换场景腾空 rename
// 目标的未完成行分流锚点）：作品 500 → 资源 700 → image(800) 未完成活行
func seedReplaceIncompleteVictim(stubs *replaceStubs) {
	res := entity.NewResource()
	res.ID = 700
	res.WorkID = 500
	stubs.res.resources = []*entity.Resource{res}
	stubs.rs.byResourceIds = []*entity.ResourceStore{makeReplaceAssoc(700, entity.StoreTypeImage, 0, 800)}
	stubs.rows.rows = []*entity.PersistentStore{makeReplaceStoreRow(800, 0, 0, 0, "store/resource/a/partial.png")}
}

// TestReplaceMatrix_Success 替换×成功：暂存下载全程旧 store 不动 → 提交窗口软删全部角色
// 活行（All 模式经封闭枚举全集展开，覆盖 image+thumbnail 两角色——已完成行移入 backup 腾空
// rename 目标）→ rename+建行挂载 → 受害者登记随 Finish 清（终态成功不复活）、新建行零登记
// （替换链坍缩后 plugin-download 不登记新建行）
func TestReplaceMatrix_Success(t *testing.T) {
	sess, h, cancel, env, stubs := newReplaceStagingSession(t, nil)
	defer cancel()
	seedReplaceVictims(stubs)
	sess.pluginExec = &fakePluginExec{
		startSpecs: []*sdkdto.StoreSpec{{
			Role: entity.StoreTypeImage, Generation: entity.GenerationDownloaded, Format: "png", Size: 3,
			ReadCloser: io.NopCloser(bytes.NewReader([]byte("new"))),
		}},
		startResp: stagingWorkResp(),
	}

	res := sess.runSectionCombo()

	if res != comboFinished || !h.finished || h.failed {
		t.Fatalf("替换成功应成功收口: res=%v finished=%v failed=%v(%s)", res, h.finished, h.failed, h.failMsg)
	}
	// 提交窗口软删两角色活行（备份软删分派——已完成行移文件入 backup），保持终态不复活
	if len(stubs.replacer.backupIds) != 2 || stubs.replacer.backupIds[0] != 800 || stubs.replacer.backupIds[1] != 801 {
		t.Fatalf("提交窗口软删应覆盖全部角色活行(800,801), 实际 %v", stubs.replacer.backupIds)
	}
	if len(stubs.rows.restoredIds) != 0 {
		t.Fatalf("终态成功不应复活受害者, 实际 %v", stubs.rows.restoredIds)
	}
	// 软删后的受害者登记（commitStaged 内一体发生）；新建行零登记
	var victims, created []int64
	for _, rb := range h.rollbacks {
		for _, v := range rb.Victims {
			victims = append(victims, v.StoreID)
		}
		created = append(created, rb.CreatedStoreIDs...)
	}
	if len(victims) != 2 || victims[0] != 800 || victims[1] != 801 {
		t.Fatalf("受害者(800,801)应登记, 实际 %v", victims)
	}
	if len(created) != 0 {
		t.Fatalf("替换链坍缩后新建行零登记, 实际 %v", created)
	}
	// 新文件就位、暂存回收
	if _, err := os.Stat(finalAbsPath(env, "[author]_[sw1]_name.png")); err != nil {
		t.Fatalf("替换新文件应就位: %v", err)
	}
	if _, err := os.Stat(env.stagingDir); !os.IsNotExist(err) {
		t.Fatal("提交后暂存目录应回收")
	}
}

// TestReplaceMatrix_FailureBeforeDownload 替换×失败（下载前，Start 报错）：坍缩后下载窗口
// 零 DB 副作用——旧 store 一动不动、零回滚登记、零建行，暂存目录未创建
func TestReplaceMatrix_FailureBeforeDownload(t *testing.T) {
	sess, h, cancel, env, stubs := newReplaceStagingSession(t, nil)
	defer cancel()
	seedReplaceVictims(stubs)
	sess.pluginExec = &fakePluginExec{} // Start 报错（测试桩默认）

	res := sess.runSectionCombo()

	if res != comboFinished || !h.failed {
		t.Fatal("Start 失败应失败收口")
	}
	// 旧 store 未动（软删在提交窗口，下载未到达）
	if len(stubs.replacer.backupIds) != 0 || len(stubs.replacer.discardedIds) != 0 {
		t.Fatalf("下载前失败不应触碰旧 store, 实际 备份软删 %v 废弃 %v", stubs.replacer.backupIds, stubs.replacer.discardedIds)
	}
	if len(h.rollbacks) != 0 {
		t.Fatalf("零回滚登记（无软删即无回滚对象）, 实际 %v", h.rollbacks)
	}
	if len(env.streamer.commits) != 0 || len(env.assocWrite.created) != 0 {
		t.Fatalf("下载前失败应零建行零挂载, 实际 %d/%d", len(env.streamer.commits), len(env.assocWrite.created))
	}
	if _, err := os.Stat(env.stagingDir); !os.IsNotExist(err) {
		t.Fatal("下载未开始，暂存目录不应存在")
	}
}

// TestReplaceMatrix_FailureDuringCommit 替换×失败（提交窗口内）：未完成受害者走废弃软删
// （腾空 rename 目标的未完成行分流）→ 建行事务失败 → 补偿逆 rename 回退暂存（暂存保留）
// + 受害者登记交控制面复活（Fail 上报经 setFailed 单点同步触发，真实复活在 taskManager 侧，
// 此处断言登记到位）；会话侧零物理删零复活
func TestReplaceMatrix_FailureDuringCommit(t *testing.T) {
	sess, h, cancel, env, stubs := newReplaceStagingSession(t, nil)
	defer cancel()
	seedReplaceIncompleteVictim(stubs)
	// 第二轨建行失败注入（事务失败）
	failing := &failingCommitter{delegate: env.streamer, failOnNth: 2}
	sess.deps.StoreCommitter = failing
	sess.pluginExec = &fakePluginExec{
		startSpecs: []*sdkdto.StoreSpec{
			{Role: entity.StoreTypeImage, Generation: entity.GenerationDownloaded, Format: "png", Size: 3,
				ReadCloser: io.NopCloser(bytes.NewReader([]byte("new")))},
			{Role: entity.StoreTypeThumbnail, Generation: entity.GenerationDerived, Format: "jpg", Size: 0,
				ReadCloser: io.NopCloser(bytes.NewReader([]byte("t")))},
		},
		startResp: stagingWorkResp(),
	}

	res := sess.runSectionCombo()

	if res != comboFinished || !h.failed {
		t.Fatal("提交失败应失败收口")
	}
	// 提交窗口软删已发生（未完成行→废弃分支）且受害者已登记
	if len(stubs.replacer.discardedIds) != 1 || stubs.replacer.discardedIds[0] != 800 {
		t.Fatalf("未完成受害者(800)应走废弃软删, 实际 %v", stubs.replacer.discardedIds)
	}
	var victims []int64
	for _, rb := range h.rollbacks {
		for _, v := range rb.Victims {
			victims = append(victims, v.StoreID)
		}
	}
	if len(victims) != 1 || victims[0] != 800 {
		t.Fatalf("受害者(800)应登记供控制面复活, 实际 %v", victims)
	}
	// 补偿：两轨均回退暂存（暂存保留供重试），零挂载
	for _, name := range []string{"image_000.png", "thumbnail_000.jpg"} {
		if _, err := os.Stat(filepath.Join(env.stagingDir, name)); err != nil {
			t.Fatalf("提交失败轨应回退暂存(%s): %v", name, err)
		}
	}
	if len(env.assocWrite.created) != 0 {
		t.Fatalf("事务失败应零挂载, 实际 %d", len(env.assocWrite.created))
	}
	// 会话侧零物理删零复活（复活归控制面单点）
	if len(stubs.writer.deletedByStoreIds) != 0 || len(stubs.deleter.hardDeleted) != 0 {
		t.Fatalf("会话侧不应物理删除, 实际 摘关联 %v 物理删 %v", stubs.writer.deletedByStoreIds, stubs.deleter.hardDeleted)
	}
	if len(stubs.rows.restoredIds) != 0 {
		t.Fatalf("会话侧不应复活, 实际 %v", stubs.rows.restoredIds)
	}
}

// TestReplaceMatrix_SoftDeleteRejectedByLock 守卫链含锁锚：作品被分享拉取持有时提交窗口
// 软删被拒——提交序列在首步失败收口，旧 store 不动、零登记、暂存保留（软删为序列首步，
// rename 未开始）
func TestReplaceMatrix_SoftDeleteRejectedByLock(t *testing.T) {
	lock := shareLock.NewShareLockRegistry()
	lock.Register(context.Background(), []int64{500}, "session-x")
	sess, h, cancel, env, stubs := newReplaceStagingSession(t, lock)
	defer cancel()
	seedReplaceVictims(stubs)
	sess.pluginExec = &fakePluginExec{
		startSpecs: []*sdkdto.StoreSpec{{
			Role: entity.StoreTypeImage, Generation: entity.GenerationDownloaded, Format: "png", Size: 3,
			ReadCloser: io.NopCloser(bytes.NewReader([]byte("new"))),
		}},
		startResp: stagingWorkResp(),
	}

	res := sess.runSectionCombo()

	if res != comboFinished || !h.failed {
		t.Fatal("锁命中应令提交窗口软删失败转失败收口")
	}
	if len(stubs.replacer.backupIds) != 0 || len(stubs.replacer.discardedIds) != 0 {
		t.Fatalf("锁命中不应触碰 store 行，实际备份软删 %v 废弃 %v", stubs.replacer.backupIds, stubs.replacer.discardedIds)
	}
	if len(h.rollbacks) != 0 {
		t.Fatalf("锁拒绝分支无被软删行，不应登记回滚，实际 %v", h.rollbacks)
	}
	// 暂存保留（软删为序列首步，rename 未开始）；零建行
	if _, err := os.Stat(filepath.Join(env.stagingDir, "image_000.png")); err != nil {
		t.Fatalf("软删拒绝暂存应保留: %v", err)
	}
	if len(env.streamer.commits) != 0 {
		t.Fatalf("零建行, 实际 %d", len(env.streamer.commits))
	}
}

// TestReplaceMatrix_PauseDuringDownload 替换×暂停（软暂停排空；停止同形——中断信号同为
// runCancel 取消）：坍缩后下载窗口零 DB 副作用——旧 store 一动不动、零回滚登记，暂存保留
// 供恢复续传、零建行
func TestReplaceMatrix_PauseDuringDownload(t *testing.T) {
	sess, h, cancel, env, stubs := newReplaceStagingSession(t, nil)
	defer cancel()
	seedReplaceVictims(stubs)
	// 在途读取返回部分数据后阻塞：软暂停广播 → 排空落盘 → 暂停收敛
	cr := &ctxAwareReader{data: []byte("aa"), ctx: h.runCtx, blockedCh: make(chan struct{})}
	sess.pluginExec = &fakePluginExec{
		startSpecs: []*sdkdto.StoreSpec{{
			Role: entity.StoreTypeImage, Generation: entity.GenerationDownloaded, Format: "png", Size: 10,
			ReadCloser: cr,
		}},
		startResp: stagingWorkResp(),
	}

	done := make(chan comboResult, 1)
	go func() { done <- sess.runSectionCombo() }()

	<-cr.blockedCh     // 在途读取挂起（循环进行中）
	close(h.softPause) // 广播软暂停
	cancel()           // 许可挂起读取以取消返回（排空超时兜底形态，收敛暂停）

	select {
	case res := <-done:
		if res != comboInterrupted {
			t.Fatalf("暂停应中断返回, 实际 %v", res)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runSectionCombo 未退出")
	}
	if h.finished || h.failed {
		t.Fatal("暂停不应上报终态")
	}
	// 旧 store 不动（软删在提交窗口，下载中断未到达）、零登记
	if len(stubs.replacer.backupIds) != 0 || len(stubs.replacer.discardedIds) != 0 {
		t.Fatalf("暂停不应触碰旧 store, 实际 备份软删 %v 废弃 %v", stubs.replacer.backupIds, stubs.replacer.discardedIds)
	}
	if len(h.rollbacks) != 0 {
		t.Fatalf("暂停零回滚登记, 实际 %v", h.rollbacks)
	}
	// 暂存保留（部分写入）供恢复续传；零建行
	if _, err := os.Stat(filepath.Join(env.stagingDir, "image_000.png")); err != nil {
		t.Fatalf("暂停暂存应保留: %v", err)
	}
	if len(env.streamer.commits) != 0 {
		t.Fatalf("暂停零建行, 实际 %d", len(env.streamer.commits))
	}
}
