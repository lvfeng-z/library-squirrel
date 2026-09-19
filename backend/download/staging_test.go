package download

// 暂存写入器与提交点测试：写入流全量 sha256（全新/续传前缀入哈希）、finalize 比对（不符/
// 截断）、暂存文件名解析、提交点序列
// （替换软删+登记意图+落位抑制时点+建行挂载事务+暂存回收）、哈希双列落库、提交失败按声明
// 撤回、哈希不符失败保留暂存、替换矩阵（替换链坍缩到提交窗口：成功/失败/提交窗口内失败/
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
	"strings"
	"testing"
	"time"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/duplicate"
	"github.com/library-squirrel/backend/persistentStore"
	"github.com/library-squirrel/backend/resource"
	"github.com/library-squirrel/backend/shareLock"
	"github.com/library-squirrel/backend/storeRegistry"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
	"go.uber.org/zap"
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

// TestParseStagingFileName 暂存文件名 role_seq 键解析（StagingFileName 的逆）：含点号
// 扩展名段还原、无扩展名形态为空串
func TestParseStagingFileName(t *testing.T) {
	role, seq, ext, err := parseStagingFileName("videoTrack_007.mp4")
	if err != nil || role != "videoTrack" || seq != 7 || ext != ".mp4" {
		t.Fatalf("解析期望 (videoTrack,7,.mp4), 实际 (%s,%d,%s,%v)", role, seq, ext, err)
	}
	role, seq, ext, err = parseStagingFileName("image_000")
	if err != nil || role != "image" || seq != 0 || ext != "" {
		t.Fatalf("无扩展名形态应得空 ext, 实际 (%s,%d,%s,%v)", role, seq, ext, err)
	}
	if _, _, _, err := parseStagingFileName("noisy.txt"); err == nil {
		t.Fatal("无 role_seq 键应解析失败")
	}
}

// TestEnumerateStaging 暂存枚举：role_seq 还原 + 大小 + 扩展名 + 稳定排序；不可解析文件跳过
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
	entries, err := enumerateStaging(1, dir)
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
		ext  string
	}{
		{entity.StoreTypeImage, 0, 4, ".png"},
		{entity.StoreTypeImage, 1, 2, ".png"},
		{entity.StoreTypeThumbnail, 0, 1, ".jpg"},
	}
	for i, w := range want {
		if entries[i].role != w.role || entries[i].seq != w.seq || entries[i].size != w.size || entries[i].ext != w.ext {
			t.Fatalf("条目 %d 期望 %+v 实际 %+v", i, w, entries[i])
		}
	}
}

// TestEnumerateStaging_DuplicateKeyResolution 同键 (role,seq) 多文件收敛：字节多者择取
// （0 字节残留与真身并存的并存形态）；字节数并列（含皆零）丢弃该键不认领；两种处置均记
// Warn 日志
func TestEnumerateStaging_DuplicateKeyResolution(t *testing.T) {
	dir := t.TempDir()
	logs := observeLogs()
	defer func() { logger.Log = zap.NewNop().Sugar() }()

	// image#0：0 字节无扩展名残留 + 10B 真身 → 择取 .jpg
	// image#1：两个 5B 并列 → 丢弃
	// image#2：两个 0 字节 → 丢弃
	for name, content := range map[string]string{
		"image_000":      "",
		"image_000.jpg":  "0123456789",
		"image_001.png":  "aaaaa",
		"image_001.jpg":  "bbbbb",
		"image_002":      "",
		"image_002.webp": "",
		"image_003.png":  "solo",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("预置 %s 失败: %v", name, err)
		}
	}
	entries, err := enumerateStaging(7, dir)
	if err != nil {
		t.Fatalf("枚举失败: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("同键收敛后应剩 2 轨(择取的 image#0 与唯一文件的 image#3), 实际 %d: %+v", len(entries), entries)
	}
	if entries[0].role != entity.StoreTypeImage || entries[0].seq != 0 || entries[0].ext != ".jpg" || entries[0].size != 10 {
		t.Fatalf("image#0 应择取字节数最多的 .jpg 真身, 实际 %+v", entries[0])
	}
	if entries[1].seq != 3 || entries[1].ext != ".png" {
		t.Fatalf("无冲突键应原样保留, 实际 %+v", entries[1])
	}
	msgs := warnMessages(logs)
	if len(msgs) != 3 {
		t.Fatalf("三次同键处置应各记一条 Warn, 实际 %d: %v", len(msgs), msgs)
	}
	if !strings.Contains(msgs[0], "按字节数最多者择取") || !strings.Contains(msgs[0], ".jpg") {
		t.Fatalf("择取处置日志应含择取结果, 实际 %q", msgs[0])
	}
	for _, m := range msgs[1:] {
		if !strings.Contains(m, "并列") {
			t.Fatalf("并列处置日志应说明无法择取, 实际 %q", m)
		}
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
		WorkDirProvider:     stubWorkDirProvider{dir: env.workDir},
		SiteKeyResolver:     &fakeSiteKeyResolver{keys: map[int64]string{1: "test-site"}},
		ResourceReader:      &stubResourceReader{resources: []*entity.Resource{}},
		ResourceSaver:       &stubResourceSaver{},
		ResourceUpdater:     &stubResourceSaver{},
		StoreIngestor:       env.streamer,
		ResourceStoreWriter: env.assocWrite,
		ResourceRecomputer:  env.recompute,
		Transactor:          stubTransactor{ingestor: env.streamer},
		StagingPaths:        stubStagingPaths{},
	}
	sess := newExecSession(deps, h, wt)
	sess.workId = 500
	sess.mode = runMode{storeScope: storeScope{kind: scopeAll}}
	return sess, h, cancel, env
}

// TestCommitAndFinish_RenamesCreatesRowsAndFinishes 提交点主链（落位+抑制登记时点锚）：
// 全部写满 → 登记意图（处置按轨声明）→ 落位（同卷 rename，抑制先于 rename 登记）→
// 事务建行（哈希双列）+挂载 → 完整度重算+Finish → 暂存目录回收。落位抑制先于 rename
// （placeHook 时点：抑制已登记、文件未到最终路径）；建行事务内（rename 已完成）最终路径
// 仍处于抑制登记态（落位宽限窗口覆盖建行，fsmonitor 误裁决防线）；提交成功后登记行全收口
func TestCommitAndFinish_RenamesCreatesRowsAndFinishes(t *testing.T) {
	sess, h, cancel, env := newCommitTestSession(t)
	defer cancel()
	// 落位时点断言：首轨抑制已登记、rename 未发生（文件未到最终路径）
	suppressedBeforeRename := false
	fileAbsentAtSuppress := false
	env.streamer.placeHook = func(*stubStoreIngestor) {
		suppressedBeforeRename = storeRegistry.IsSuppressed("store/resource/" + bucketOf("test-site", "sw1") + "/test-site_sw1/image_000.png")
		_, statErr := os.Stat(finalAbsPath(env, "image_000.png"))
		fileAbsentAtSuppress = os.IsNotExist(statErr)
	}
	// 建行事务内（rename 已完成）最终路径处于抑制登记态
	suppressedInView := false
	movedInView := false
	env.streamer.hook = func(*stubStoreIngestor) {
		suppressedInView = storeRegistry.IsSuppressed("store/resource/" + bucketOf("test-site", "sw1") + "/test-site_sw1/image_000.png")
		_, statErr := os.Stat(finalAbsPath(env, "image_000.png"))
		movedInView = statErr == nil
	}

	specs := []*sdkdto.StoreSpec{
		{Role: entity.StoreTypeImage, Generation: entity.GenerationDownloaded, Format: "png", Size: 3,
			Continuable: boolPtr(true),
			ReadCloser:  io.NopCloser(bytes.NewReader([]byte("abc")))},
		{Role: entity.StoreTypeThumbnail, Generation: entity.GenerationDerived, Format: "jpg", Size: 0,
			ReadCloser: io.NopCloser(bytes.NewReader([]byte("t")))},
	}

	res := sess.startDownload(specs)

	if res != comboFinished {
		t.Fatalf("期望 comboFinished, 实际 %v", res)
	}
	if !h.finished || h.failed {
		t.Fatalf("提交点应成功收口: finished=%v failed=%v(%s)", h.finished, h.failed, h.failMsg)
	}
	if !suppressedBeforeRename || !fileAbsentAtSuppress {
		t.Fatal("落位抑制应先于 rename 登记（时点：抑制已登记、文件未到最终路径）")
	}
	if !suppressedInView {
		t.Fatal("落位→建行事务窗口内最终路径应处于抑制登记态（fsmonitor 误裁决防线）")
	}
	if !movedInView {
		t.Fatal("建行事务时点文件应已落位最终路径（落位先于建行）")
	}
	// 处置声明按轨判定：可续传 image 轨退回暂存、derived 轨丢弃；暂存路径为 workDir 相对正斜杠
	if len(env.streamer.preps) != 2 ||
		env.streamer.preps[0].AbortAction != entity.AbortActionReturnToStaging ||
		env.streamer.preps[1].AbortAction != entity.AbortActionDiscard {
		t.Fatalf("处置声明应为 (image=退回暂存, thumbnail=丢弃), 实际 %+v", env.streamer.preps)
	}
	if env.streamer.preps[0].StagingPath != "staging/download/1/image_000.png" ||
		env.streamer.preps[0].FilePath != "store/resource/"+bucketOf("test-site", "sw1")+"/test-site_sw1/image_000.png" {
		t.Fatalf("登记意图路径应 relPath 域正斜杠, 实际 %+v", env.streamer.preps[0])
	}
	if len(env.streamer.journals) != 0 {
		t.Fatalf("提交成功后登记行应全部收口, 实际剩 %d 条", len(env.streamer.journals))
	}
	// 两轨最终文件就位
	for _, name := range []string{"image_000.png", "thumbnail_000.jpg"} {
		if _, err := os.Stat(finalAbsPath(env, name)); err != nil {
			t.Fatalf("最终文件应就位(%s): %v", name, err)
		}
	}
	// 建行：两行（文件名恒带 role+seq 段）
	if len(env.streamer.commits) != 2 {
		t.Fatalf("期望 2 行建行, 实际 %d: %+v", len(env.streamer.commits), env.streamer.commits)
	}
	if env.streamer.commits[0].fileName != "image_000.png" {
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

// TestIngestAbortActionByContinuableMarker 处置声明判据=轨的可续传标记（唯一判据）：
// 插件声明可续传的轨退回暂存；derived 等不可续传轨（含插件未声明可续传的 downloaded 轨）丢弃
func TestIngestAbortActionByContinuableMarker(t *testing.T) {
	continuable := &streamController{continuable: true}
	if got := continuable.ingestAbortAction(); got != entity.AbortActionReturnToStaging {
		t.Fatalf("可续传轨应声明退回暂存, 实际 %s", got)
	}
	for name, sc := range map[string]*streamController{
		"derived 轨":  {continuable: false},
		"插件未声明可续传的轨": {continuable: false},
	} {
		if got := sc.ingestAbortAction(); got != entity.AbortActionDiscard {
			t.Fatalf("%s 应声明丢弃, 实际 %s", name, got)
		}
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
	specs := []*sdkdto.StoreSpec{
		{Role: entity.StoreTypeImage, Generation: entity.GenerationDownloaded, Format: "png", Size: int64(len(payload)),
			ExpectedSha256: &declared,
			ReadCloser:     io.NopCloser(bytes.NewReader(payload))},
	}

	res := sess.startDownload(specs)

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

	res := sess.startDownload(specs)

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
	if _, err := os.Stat(finalAbsPath(env, "image_000.png")); !os.IsNotExist(err) {
		t.Fatal("store/ 最终路径应零残留")
	}
}

// TestCommitTxFailureAbortsByDeclaredAction 提交事务失败按声明撤回：可续传轨（声明退回暂存）
// 逆 rename 回退暂存保住写满字节；derived 轨（声明丢弃）文件删除、两处皆无；零建行零挂载、
// 登记行全收口（事务回滚复原首轨的建行与删登记）
func TestCommitTxFailureAbortsByDeclaredAction(t *testing.T) {
	sess, h, cancel, env := newCommitTestSession(t)
	defer cancel()
	env.streamer.hook = func(*stubStoreIngestor) {} // 占位（保持 hook 非 nil 语义清晰）
	// 让第二轨建行失败：事务内返回错误
	failing := &failingIngestor{delegate: env.streamer, failOnNth: 2}
	sess.deps.StoreIngestor = failing

	specs := []*sdkdto.StoreSpec{
		{Role: entity.StoreTypeImage, Generation: entity.GenerationDownloaded, Format: "png", Size: 3,
			Continuable: boolPtr(true),
			ReadCloser:  io.NopCloser(bytes.NewReader([]byte("abc")))},
		{Role: entity.StoreTypeThumbnail, Generation: entity.GenerationDerived, Format: "jpg", Size: 0,
			ReadCloser: io.NopCloser(bytes.NewReader([]byte("t")))},
	}

	res := sess.startDownload(specs)

	if res != comboFinished || !h.failed {
		t.Fatal("提交失败应失败收口")
	}
	// 可续传轨退回暂存（内容保住）；丢弃轨两处皆无
	got, err := os.ReadFile(filepath.Join(env.stagingDir, "image_000.png"))
	if err != nil || string(got) != "abc" {
		t.Fatalf("可续传轨应退回暂存且内容保住, err=%v got=%q", err, got)
	}
	if _, err := os.Stat(filepath.Join(env.stagingDir, "thumbnail_000.jpg")); !os.IsNotExist(err) {
		t.Fatal("丢弃轨不应退回暂存")
	}
	for _, name := range []string{"image_000.png", "thumbnail_000.jpg"} {
		if _, err := os.Stat(finalAbsPath(env, name)); !os.IsNotExist(err) {
			t.Fatalf("撤回后最终路径应无残留(%s)", name)
		}
	}
	if len(env.streamer.commits) != 0 {
		t.Fatalf("事务失败建行应全部回滚, 实际 %d", len(env.streamer.commits))
	}
	if len(env.assocWrite.created) != 0 {
		t.Fatalf("失败不应挂载, 实际 %d", len(env.assocWrite.created))
	}
	if len(env.streamer.journals) != 0 {
		t.Fatalf("撤回应收口全部登记行, 实际剩 %d 条", len(env.streamer.journals))
	}
}

// failingIngestor 前 n-1 次建行委派真桩、第 n 次失败（事务失败注入）；登记/落位/撤回调委派真桩
type failingIngestor struct {
	delegate  *stubStoreIngestor
	failOnNth int64
	n         int64
}

func (f *failingIngestor) PrepareIngest(ctx context.Context, items []persistentStore.IngestItem) ([]int64, error) {
	return f.delegate.PrepareIngest(ctx, items)
}

func (f *failingIngestor) PlaceIngest(ctx context.Context, intentIds []int64) error {
	return f.delegate.PlaceIngest(ctx, intentIds)
}

func (f *failingIngestor) CommitIngest(ctx context.Context, intentId int64, relPath string, fileName string, expectedSha, actualSha sql.NullString) (int64, error) {
	f.n++
	if f.n == f.failOnNth {
		return 0, fmt.Errorf("注入建行失败")
	}
	return f.delegate.CommitIngest(ctx, intentId, relPath, fileName, expectedSha, actualSha)
}

func (f *failingIngestor) AbortIngest(ctx context.Context, intentIds []int64) error {
	return f.delegate.AbortIngest(ctx, intentIds)
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
		WorkDirProvider:     stubWorkDirProvider{dir: env.workDir},
		SiteKeyResolver:     &fakeSiteKeyResolver{keys: map[int64]string{1: "test-site"}},
		DuplicateChecker:    &fakeDupChecker{result: duplicate.DuplicateCheckResult{Class: duplicate.DuplicateHitNoConflict, WorkID: 500, WorkName: "已存在作品"}},
		WorkInfoSaver:       &stubWorkInfoSaver{savedWorkId: 500},
		ResourceReader:      stubs.res,
		ReplaceStoreOps:     newRealReplacementService(stubs, lock),
		ResourceSaver:       &stubResourceSaver{},
		ResourceUpdater:     &stubResourceSaver{},
		StoreIngestor:       env.streamer,
		ResourceStoreWriter: env.assocWrite,
		ResourceRecomputer:  env.recompute,
		Transactor:          stubTransactor{ingestor: env.streamer},
		StagingPaths:        stubStagingPaths{},
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
	if _, err := os.Stat(finalAbsPath(env, "image_000.png")); err != nil {
		t.Fatalf("替换新文件应就位: %v", err)
	}
	if _, err := os.Stat(env.stagingDir); !os.IsNotExist(err) {
		t.Fatal("提交后暂存目录应回收")
	}
}

// observingReplaceOps 软删观察桩：在替换软删调用时点读取并移出受害者物理文件（生产上
// 备份软删把已完成行文件移入 backup 的同构动作），记录调用时点旧路径的文件内容——内容
// 为旧内容即证明 rename 尚未写入该路径（提交序列顺序锚）
type observingReplaceOps struct {
	delegate            resource.ReplaceStoreOps
	victimAbs           string
	backupAbs           string
	contentAtSoftDelete string
	moved               bool
}

func (o *observingReplaceOps) SoftDeleteWorkStoreRoles(ctx context.Context, workId int64, roles []string) ([]resource.StoreRef, error) {
	if content, err := os.ReadFile(o.victimAbs); err == nil {
		o.contentAtSoftDelete = string(content)
		if rerr := os.Rename(o.victimAbs, o.backupAbs); rerr == nil {
			o.moved = true
		}
	}
	return o.delegate.SoftDeleteWorkStoreRoles(ctx, workId, roles)
}

func (o *observingReplaceOps) RestoreReplacedStores(ctx context.Context, scope resource.RestoreScope) error {
	return o.delegate.RestoreReplacedStores(ctx, scope)
}

// TestRedownloadSamePath_VictimFileMovedBeforeRename 风险1 时序锚定：ID 名下重下同作品
// 恒命中同一路径——受害者旧文件所在路径与新下载的派生路径相同（站点复合键 siteId=1→
// test-site、siteWorkId=sw1 → store/resource/{bucketOf("test-site","sw1")}/test-site_sw1/
// image_000.png，同键恒同桶→恒同路径）。提交序列的
// 替换软删（首步，生产上物理移文件入 backup）先于 rename 写入同路径：软删时点旧路径
// 内容仍为旧内容（若 rename 先行，该路径已被新内容覆盖）；移出后 rename 写入新内容，
// 旧内容留备份位
func TestRedownloadSamePath_VictimFileMovedBeforeRename(t *testing.T) {
	sess, h, cancel, env, stubs := newReplaceStagingSession(t, nil)
	defer cancel()
	// 受害者已完成 image 行的 file_path 预置为新下载将派生的同一路径（重下同路径锚定）
	finalRel := "store/resource/" + bucketOf("test-site", "sw1") + "/test-site_sw1/image_000.png"
	seedReplaceVictims(stubs)
	stubs.rows.rows = []*entity.PersistentStore{
		makeReplaceStoreRow(800, 1, 0, 0, finalRel),
		makeReplaceStoreRow(801, 1, 0, 0, "store/resource/"+bucketOf("test-site", "sw1")+"/test-site_sw1/old_thumb.jpg"),
	}
	// 物理旧文件预置在最终路径
	victimAbs := filepath.Join(env.workDir, filepath.FromSlash(finalRel))
	if err := os.MkdirAll(filepath.Dir(victimAbs), 0o755); err != nil {
		t.Fatalf("创建最终目录失败: %v", err)
	}
	if err := os.WriteFile(victimAbs, []byte("old-content"), 0o644); err != nil {
		t.Fatalf("预置受害者旧文件失败: %v", err)
	}
	backupAbs := filepath.Join(t.TempDir(), "victim-backup.png")
	obs := &observingReplaceOps{delegate: sess.deps.ReplaceStoreOps, victimAbs: victimAbs, backupAbs: backupAbs}
	sess.deps.ReplaceStoreOps = obs

	sess.pluginExec = &fakePluginExec{
		startSpecs: []*sdkdto.StoreSpec{{
			Role: entity.StoreTypeImage, Generation: entity.GenerationDownloaded, Format: "png", Size: 3,
			ReadCloser: io.NopCloser(bytes.NewReader([]byte("new"))),
		}},
	}

	res := sess.runSectionCombo()

	if res != comboFinished || !h.finished || h.failed {
		t.Fatalf("重下替换应成功收口: res=%v finished=%v failed=%v(%s)", res, h.finished, h.failed, h.failMsg)
	}
	// 软删先于 rename：软删时点最终路径内容为旧内容（若 rename 先行则已被新内容覆盖）
	if !obs.moved || obs.contentAtSoftDelete != "old-content" {
		t.Fatalf("替换软删应先于 rename 读取并移出旧文件: moved=%v contentAtSoftDelete=%q", obs.moved, obs.contentAtSoftDelete)
	}
	// rename 写入同一路径：最终路径为新内容、备份位保留旧内容
	got, rerr := os.ReadFile(victimAbs)
	if rerr != nil || string(got) != "new" {
		t.Fatalf("重下应写入与旧文件相同的最终路径: err=%v content=%q", rerr, got)
	}
	gotBackup, berr := os.ReadFile(backupAbs)
	if berr != nil || string(gotBackup) != "old-content" {
		t.Fatalf("受害者旧内容应移入备份位: err=%v content=%q", berr, gotBackup)
	}
	// 建行落库路径 = 受害者原路径（ID 名下重下命中同一路径）
	if len(env.streamer.commits) != 1 || env.streamer.commits[0].relPath != finalRel {
		t.Fatalf("建行 relPath 应为重下同路径 %s, 实际 %+v", finalRel, env.streamer.commits)
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
// （腾空 rename 目标的未完成行分流）→ 建行事务失败 → 按声明撤回（可续传 image 轨退回暂存
// 保住字节、derived 轨丢弃）+ 受害者登记交控制面复活（Fail 上报经 setFailed 单点同步触发，
// 真实复活在 taskManager 侧，此处断言登记到位）；会话侧零物理删零复活
func TestReplaceMatrix_FailureDuringCommit(t *testing.T) {
	sess, h, cancel, env, stubs := newReplaceStagingSession(t, nil)
	defer cancel()
	seedReplaceIncompleteVictim(stubs)
	// 第二轨建行失败注入（事务失败）
	failing := &failingIngestor{delegate: env.streamer, failOnNth: 2}
	sess.deps.StoreIngestor = failing
	sess.pluginExec = &fakePluginExec{
		startSpecs: []*sdkdto.StoreSpec{
			{Role: entity.StoreTypeImage, Generation: entity.GenerationDownloaded, Format: "png", Size: 3,
				Continuable: boolPtr(true),
				ReadCloser:  io.NopCloser(bytes.NewReader([]byte("new")))},
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
	// 撤回按声明：可续传 image 轨退回暂存（内容保住），derived 轨丢弃两处皆无
	got, err := os.ReadFile(filepath.Join(env.stagingDir, "image_000.png"))
	if err != nil || string(got) != "new" {
		t.Fatalf("可续传轨应退回暂存且内容保住, err=%v got=%q", err, got)
	}
	if _, err := os.Stat(filepath.Join(env.stagingDir, "thumbnail_000.jpg")); !os.IsNotExist(err) {
		t.Fatal("丢弃轨不应退回暂存")
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
