package share

// 收件人侧测试：链接解析校验、深链到达缓存/消费、ReceiveExecution 端到端拉取回灌
// （经中继桩 + 真实宿主会话）、暂存断点续传（offset 请求锚断言）、拨号终态拒绝、
// 暂停保留暂存、启动清扫。

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/duplicate"
	"github.com/library-squirrel/backend/export"
	importer "github.com/library-squirrel/backend/import"
	"github.com/library-squirrel/backend/resource"
	"github.com/library-squirrel/backend/taskManager"
)

// —— 链接解析 ——

// token22 合法 token 样例（22 字符 base64url）
const token22 = "AbCdEfGhIjKlMnOpQrStUv"

func b64key() string {
	key, _ := GenerateShareKey()
	return base64.RawURLEncoding.EncodeToString(key)
}

func TestParseShareLinkValid(t *testing.T) {
	key, _ := GenerateShareKey()
	keyB64 := base64.RawURLEncoding.EncodeToString(key)

	// 深链：中继带端口
	dl := fmt.Sprintf("library-squirrel://share/relay.example.com:9527/%s#k=%s", token22, keyB64)
	target, err := ParseShareLink(dl)
	require.NoError(t, err)
	assert.Equal(t, "relay.example.com:9527", target.RelayDial)
	assert.Equal(t, "relay.example.com:9527", target.RelayHost)
	assert.Equal(t, token22, target.Token)
	assert.Equal(t, key, target.Key)

	// 深链：中继无端口（补默认端口）
	dl2 := fmt.Sprintf("LIBRARY-SQUIRREL://share/relay.example.com/%s#k=%s", token22, keyB64)
	target2, err := ParseShareLink(dl2)
	require.NoError(t, err)
	assert.Equal(t, "relay.example.com:9527", target2.RelayDial)

	// https 分享链接
	hl := fmt.Sprintf("https://relay.example.com/s/%s#k=%s", token22, keyB64)
	target3, err := ParseShareLink(hl)
	require.NoError(t, err)
	assert.Equal(t, "relay.example.com:9527", target3.RelayDial)
	assert.Equal(t, "relay.example.com", target3.RelayHost)

	// 密钥走 query（兼容形态）
	hl2 := fmt.Sprintf("https://relay.example.com/s/%s?k=%s", token22, keyB64)
	_, err = ParseShareLink(hl2)
	require.NoError(t, err)
}

func TestParseShareLinkInvalid(t *testing.T) {
	keyB64 := b64key()
	cases := []struct {
		name string
		link string
	}{
		{"空链接", "  "},
		{"含空白", "library-squirrel://share/a/" + token22 + " #k=" + keyB64},
		{"scheme 不支持", "ftp://relay.example.com/s/" + token22},
		{"深链 host 非 share", "library-squirrel://open/relay.example.com/" + token22},
		{"深链路径段数不符", "library-squirrel://share/relay.example.com/" + token22 + "/extra"},
		{"落地页路径非 /s", "https://relay.example.com/x/" + token22},
		{"token 字符集", "library-squirrel://share/relay.example.com/not-a-valid-token-!!#k=" + keyB64},
		{"token 长度", "library-squirrel://share/relay.example.com/AbCdEfGh#k=" + keyB64},
		{"中继地址含斜杠", "library-squirrel://share/relay..example.com/a/" + token22 + "#k=" + keyB64},
		{"缺密钥", "library-squirrel://share/relay.example.com/" + token22},
		{"密钥空值", "library-squirrel://share/relay.example.com/" + token22 + "#k="},
		{"密钥非 base64", "library-squirrel://share/relay.example.com/" + token22 + "#k=!!!"},
		{"密钥长度不符", "library-squirrel://share/relay.example.com/" + token22 + "#k=" + base64.RawURLEncoding.EncodeToString([]byte("short"))},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseShareLink(c.link)
			assert.Error(t, err, "链接应被拒绝: %s", c.link)
		})
	}
}

// —— 深链到达缓存与消费 ——

func TestNotifyIncomingLinkConsumeAndDedupe(t *testing.T) {
	svc := NewService(nil, nil, nil, func() string { return "" }, func() string { return "" },
		"test-instance-0001", nil, nil)

	assert.False(t, svc.NotifyIncomingLink("https://evil.example.com/x"), "非深链形态应被拒")
	assert.Equal(t, "", svc.ConsumeIncomingLink(), "被拒链接不应缓存")

	link := "library-squirrel://share/relay.example.com/" + token22
	assert.True(t, svc.NotifyIncomingLink(link))
	assert.True(t, svc.NotifyIncomingLink(link), "窗口期重复应被去重接受")
	assert.Equal(t, link, svc.ConsumeIncomingLink())
	assert.Equal(t, "", svc.ConsumeIncomingLink(), "消费后应清空")
}

// —— ReceiveExecution 端到端 ——

// fakeIngestor 回灌导入桩：记录 manifest 与经文件源读到的全部内容
type fakeIngestor struct {
	mu        sync.Mutex
	called    int
	manifest  *export.Manifest
	opts      *importer.IngestOptions // 最近一次调用的替换选项（断言替换集）
	contents  map[string][]byte
	ingestErr error
}

func (f *fakeIngestor) Ingest(ctx context.Context, manifest *export.Manifest, fileSource importer.FileSource, opts *importer.IngestOptions) (*importer.ImportResult, error) {
	f.mu.Lock()
	if f.ingestErr != nil {
		err := f.ingestErr
		f.mu.Unlock()
		return nil, err
	}
	f.called++
	f.manifest = manifest
	f.opts = opts
	f.contents = make(map[string][]byte)
	manifestCopy := f.manifest
	f.mu.Unlock()
	for _, entry := range manifestCopy.Files {
		if entry.Path == "" || entry.Missing {
			continue
		}
		rc, err := fileSource(entry.Path)
		if err != nil {
			// 被裁决跳过作品的文件未暂存：缺席降级（对齐真实 ingest 不提取跳过作品的文件）
			if errors.Is(err, importer.ErrPackageFileMissing) {
				continue
			}
			return nil, err
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return nil, err
		}
		f.mu.Lock()
		f.contents[entry.Path] = data
		f.mu.Unlock()
	}
	return &importer.ImportResult{}, nil
}

// newReceiveHandle 构建收件执行句柄（复用 share_test.go 的 fakeStrategyHandle；
// 暂停路径需外部 cancel，经返回值交出）
func newReceiveHandle(task *entity.Task) (*fakeStrategyHandle, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	return &fakeStrategyHandle{task: task, runCtx: ctx}, cancel
}

// handleOutcome 读取句柄终态（finished, errMsg）
func handleOutcome(h *fakeStrategyHandle) (bool, string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.finished, h.errMsg
}

// receiveTestEnv 收件端到端夹具：中继桩 + 宿主会话（真实 Service/Packer）+ 收件 Service
type receiveTestEnv struct {
	stub       *relayStub
	hostSvc    *Service
	recvSvc    *Service
	em         *captureEmitter
	link       string
	workDir    string // 宿主源文件目录
	recvDir    string // 收件方工作目录（暂存根）
	ingestor   *fakeIngestor
	dialer     *recordingDialer
	sourceData map[string][]byte
}

// startReceiveEnv 发布分享并构建收件执行器夹具（link 为完整分享链接；默认单作品模型）
func startReceiveEnv(t *testing.T, opts SharePublishOptions) *receiveTestEnv {
	return startReceiveEnvModel(t, opts, buildTestModel)
}

// startReceiveEnvModel 发布分享并构建收件执行器夹具（自定义导出模型构建器）
func startReceiveEnvModel(t *testing.T, opts SharePublishOptions,
	build func(t *testing.T, workDir string) (*export.ExportModel, map[string][]byte)) *receiveTestEnv {
	t.Helper()
	stub := startRelayStub(t)

	// 宿主侧：模型 + 源文件落临时目录 + 规划（填充包内路径/大小/缺失）
	hostWorkDir := t.TempDir()
	model, sourceData := build(t, hostWorkDir)
	if _, err := export.NewPacker().Plan(context.Background(), hostWorkDir, model); err != nil {
		t.Fatalf("规划导出模型失败: %v", err)
	}
	em := newCaptureEmitter()
	dialer := &recordingDialer{}
	hostSvc := newTestService(t, stub, hostWorkDir, model, em, dialer)
	_, comp := publishAndWait(t, hostSvc, em, opts)
	if !comp.Success {
		t.Fatalf("宿主发布失败: %s", comp.ErrMsg)
	}

	// 收件侧：独立 Service（工作目录指向收件方临时目录）
	recvDir := t.TempDir()
	recvSvc := NewService(nil, nil, nil,
		func() string { return stub.addr }, func() string { return recvDir },
		"recipient-instance-0001", nil, nil)
	recvSvc.setTunables(sessionRuntimeOptions{dialFn: dialer.dial, streamRate: 8 << 20})

	return &receiveTestEnv{
		stub: stub, hostSvc: hostSvc, recvSvc: recvSvc, em: em,
		link: comp.Link, workDir: hostWorkDir, recvDir: recvDir,
		ingestor: &fakeIngestor{}, dialer: dialer, sourceData: sourceData,
	}
}

// buildTwoWorkModel 双作品导出模型（任务粒度整体决策应用全部冲突作品的多作品验证）
func buildTwoWorkModel(t *testing.T, workDir string) (*export.ExportModel, map[string][]byte) {
	t.Helper()
	relA := "store/resource/测试作者/a_001.jpg"
	relB := "store/resource/测试作者/b_001.jpg"
	contentA := []byte("WORK-A-PLAINTEXT-CONTENT")
	contentB := []byte("WORK-B-PLAINTEXT-CONTENT")
	for _, p := range []string{relA, relB} {
		abs := filepath.Join(workDir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		content := contentA
		if p == relB {
			content = contentB
		}
		if err := os.WriteFile(abs, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	manifest := &export.Manifest{
		SchemaVersion: export.SchemaVersion,
		Sites:         []export.SiteRecord{{ID: 1, SiteName: strPtr("测试站")}},
		Works: []export.WorkRecord{
			{ID: 1, SiteID: i64Ptr(1), SiteWorkID: strPtr("1001"), SiteWorkName: strPtr("作品A"),
				Resources: []export.ResourceRecord{{ID: 11, ResourceType: "image",
					Stores: []export.StoreMount{{StoreType: "image", Generation: "downloaded", StoreSeq: 1, StoreID: 101}}}}},
			{ID: 2, SiteID: i64Ptr(1), SiteWorkID: strPtr("1002"), SiteWorkName: strPtr("作品B"),
				Resources: []export.ResourceRecord{{ID: 12, ResourceType: "image",
					Stores: []export.StoreMount{{StoreType: "image", Generation: "downloaded", StoreSeq: 1, StoreID: 201}}}}},
		},
		Files: []export.FileEntry{
			{StoreID: 101, StorePath: relA},
			{StoreID: 201, StorePath: relB},
		},
	}
	return export.NewExportModel(manifest), map[string][]byte{relA: contentA, relB: contentB}
}

// buildReceiveHandle 构建收件任务与执行句柄（载荷由链接解析产物构建；cancel 供暂停路径）
func (env *receiveTestEnv) buildReceiveHandle(t *testing.T, password string) (*fakeStrategyHandle, context.CancelFunc, *ReceiveExecution) {
	t.Helper()
	target, err := ParseShareLink(env.link)
	require.NoError(t, err)
	payload, err := newShareReceivePayload(target, password)
	require.NoError(t, err)
	task := entity.NewTask()
	task.ID = 777
	task.TaskType = sql.NullString{String: TaskTypeReceive, Valid: true}
	task.Payload = sql.NullString{String: payload, Valid: true}
	h, cancel := newReceiveHandle(task)
	exec := NewReceiveExecution(env.recvSvc, env.ingestor, nil, nil)
	return h, cancel, exec
}

// buildDupHandle 构建带查重/替换能力的收件任务、执行句柄与执行器（cancel 供暂停路径；
// checker/ops 缺省为空桩，断言经返回的 ops 落地）
func (env *receiveTestEnv) buildDupHandle(t *testing.T, password string,
	checker *fakeDuplicateChecker, ops *fakeReplaceOps) (*fakeStrategyHandle, context.CancelFunc, *ReceiveExecution, *fakeReplaceOps) {
	t.Helper()
	h, cancel, _ := env.buildReceiveHandle(t, password)
	if checker == nil {
		checker = &fakeDuplicateChecker{}
	}
	if ops == nil {
		ops = &fakeReplaceOps{}
	}
	exec := NewReceiveExecution(env.recvSvc, env.ingestor, checker, ops)
	return h, cancel, exec, ops
}

// —— 查重 / 替换链桩（收件查重接入普通下载轨道）——

// fakeDuplicateChecker 查重判定桩：每次 Check 调用取预置结果序列中的一份（短于条目数补齐 Miss）
type fakeDuplicateChecker struct {
	results [][]duplicate.DuplicateCheckResult // 第 i 次 Check 返回的逐条目结果
	calls   int
}

func (f *fakeDuplicateChecker) Check(ctx context.Context, items []duplicate.DuplicateCheckItem) ([]duplicate.DuplicateCheckResult, error) {
	out := make([]duplicate.DuplicateCheckResult, len(items))
	if f.calls < len(f.results) {
		copy(out, f.results[f.calls])
	}
	f.calls++
	return out, nil
}

// hitConflict 构造冲突命中判定结果（可指定本库作品 ID 与交集角色）
func hitConflict(workID int64, name string, conflictRoles []string) duplicate.DuplicateCheckResult {
	return duplicate.DuplicateCheckResult{
		Class: duplicate.DuplicateHitConflict, WorkID: workID, WorkName: name, ConflictRoles: conflictRoles,
	}
}

// noConflict 构造零交集命中判定结果（保留已有作品 ID 供替换定位）
func noConflict(workID int64, name string) duplicate.DuplicateCheckResult {
	return duplicate.DuplicateCheckResult{Class: duplicate.DuplicateHitNoConflict, WorkID: workID, WorkName: name}
}

type softDeleteCall struct {
	workID int64
	roles  []string
}

// fakeReplaceOps 替换链桩：记录软删调用与回滚复活范围；cancelAfter 供软删窗口暂停注入
type fakeReplaceOps struct {
	mu          sync.Mutex
	softCalls   []softDeleteCall
	softErr     error
	victimBase  int64 // 每次软删返回的 victim StoreID 起始（0=返回空清单）
	cancelAfter context.CancelFunc
	restoreScopes []resource.RestoreScope
}

func (f *fakeReplaceOps) SoftDeleteWorkStoreRoles(ctx context.Context, workId int64, roles []string) ([]resource.StoreRef, error) {
	f.mu.Lock()
	f.softCalls = append(f.softCalls, softDeleteCall{workID: workId, roles: append([]string(nil), roles...)})
	base := f.victimBase
	err := f.softErr
	cancel := f.cancelAfter
	n := len(f.softCalls)
	f.mu.Unlock()
	if cancel != nil {
		cancel() // 软删窗口内暂停注入（登记由执行器在返回后完成）
	}
	if err != nil {
		return nil, err
	}
	if base == 0 {
		return nil, nil
	}
	// 按调用序生成不同 victim（验证多作品回滚清单合并登记）
	return []resource.StoreRef{{StoreID: base + int64(n-1), ResourceID: 700, BackupID: 800 + int64(n-1), FilePath: "store/resource/x.png"}}, nil
}

func (f *fakeReplaceOps) RestoreReplacedStores(ctx context.Context, scope resource.RestoreScope) error {
	f.mu.Lock()
	f.restoreScopes = append(f.restoreScopes, scope)
	f.mu.Unlock()
	return nil
}

// stagingDirOf 夹具暂存目录（任务 777）
func (env *receiveTestEnv) stagingDirOf() string {
	return filepath.Join(env.recvDir, receiveStagingRootName, "777")
}

// waitExecuteDone 同步执行并等待终态（Execute 阻塞直至终态或取消，直接调用即可）
func waitExecuteDone(t *testing.T, h *fakeStrategyHandle, exec *ReceiveExecution) (bool, string) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		exec.Execute(h)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("收件执行超时")
	}
	return handleOutcome(h)
}

// parseRecordedRequests 从收件方发出字节流解出全部请求记录（密钥解密 DATA 帧）。
// 记录通道为宿主/收件共用连接（宿主隧道也在其中写应答记录），非 type 请求形态的记录
// （应答头 JSON 等）一律跳过。
func parseRecordedRequests(t *testing.T, raw []byte, key []byte) []streamRequest {
	t.Helper()
	cip, err := newE2ECipher(key)
	require.NoError(t, err)
	var reqs []streamRequest
	buf := raw
	for len(buf) >= frameHeaderSize {
		length := binary.BigEndian.Uint32(buf[8:12])
		if int(length) > len(buf)-frameHeaderSize {
			break // 帧不完整（记录截断在写出瞬间）
		}
		typ := buf[3]
		payload := buf[frameHeaderSize : frameHeaderSize+int(length)]
		if typ == frameData {
			plaintext, derr := cip.openRecord(payload)
			if derr == nil {
				var req streamRequest
				if json.Unmarshal(plaintext, &req) == nil && (req.Type == "manifest" || req.Type == "file") {
					reqs = append(reqs, req)
				}
			}
		}
		buf = buf[frameHeaderSize+int(length):]
	}
	return reqs
}

// TestReceiveExecutionEndToEnd 完整拉取回灌：Finish 终态、内容逐字节一致、暂存清理
func TestReceiveExecutionEndToEnd(t *testing.T) {
	env := startReceiveEnv(t, SharePublishOptions{})
	h, _, exec := env.buildReceiveHandle(t, "")
	finished, failed := waitExecuteDone(t, h, exec)
	require.True(t, finished, "应成功终态，失败信息: %s", failed)

	env.ingestor.mu.Lock()
	defer env.ingestor.mu.Unlock()
	require.Equal(t, 1, env.ingestor.called, "回灌导入应恰好调用一次")
	// 源内容逐字节一致（缺失文件条目除外——导入按缺席降级不读文件源）
	for _, entry := range env.ingestor.manifest.Files {
		if entry.Missing {
			continue
		}
		got, ok := env.ingestor.contents[entry.Path]
		require.True(t, ok, "文件未被导入: %s", entry.Path)
		if src, isSrc := env.sourceData[entry.StorePath]; isSrc {
			assert.Equal(t, src, got, "内容不一致: %s", entry.Path)
		}
	}
	assert.NoDirExists(t, env.stagingDirOf(), "成功后暂存应清理")
}

// TestReceiveExecutionResumeFromStaging 断点续传：预置完整小文件 + 半个大文件暂存，
// 重执行仅请求 manifest 与大文件剩余部分（offset 锚 = 已暂存字节数）
func TestReceiveExecutionResumeFromStaging(t *testing.T) {
	env := startReceiveEnv(t, SharePublishOptions{})

	// 包内路径经同一模型重新规划取得（规划为纯计算，对同模型产物确定）
	planModel, sourceData := buildTestModel(t, env.workDir)
	_, err := export.NewPacker().Plan(context.Background(), env.workDir, planModel)
	require.NoError(t, err)
	var smallPath, bigPath string
	var bigSize int64
	var bigData []byte
	for i := range planModel.Manifest.Files {
		f := &planModel.Manifest.Files[i]
		if f.Missing {
			continue
		}
		if smallPath == "" {
			smallPath = f.Path
			continue
		}
		if bigPath == "" {
			bigPath = f.Path
			bigSize = f.Size
			bigData = sourceData[f.StorePath]
		}
	}
	require.NotEmpty(t, bigPath, "夹具应含两个非缺失文件")

	// 预置暂存：小文件全量 + 大文件前半
	staging := env.stagingDirOf()
	require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(staging, filepath.FromSlash(bigPath))), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(staging, filepath.FromSlash(smallPath)),
		sourceData[planModel.Manifest.Files[0].StorePath], 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(staging, filepath.FromSlash(bigPath)), bigData[:bigSize/2], 0o644))

	h, _, exec := env.buildReceiveHandle(t, "")
	finished, failed := waitExecuteDone(t, h, exec)
	require.True(t, finished, "应成功终态，失败信息: %s", failed)

	// 请求锚断言：manifest + file(big, offset=bigSize/2)；小文件应跳过（无请求）
	target, err := ParseShareLink(env.link)
	require.NoError(t, err)
	reqs := parseRecordedRequests(t, env.dialer.snapshot(), target.Key)
	require.Len(t, reqs, 2, "应恰有 manifest 与大文件续传两个请求，实际: %+v", reqs)
	assert.Equal(t, "manifest", reqs[0].Type)
	assert.Equal(t, "file", reqs[1].Type)
	assert.Equal(t, bigPath, reqs[1].Path)
	assert.Equal(t, bigSize/2, reqs[1].Offset, "续传 offset 应为已暂存字节")

	// 续传产物与源一致
	env.ingestor.mu.Lock()
	defer env.ingestor.mu.Unlock()
	assert.Equal(t, bigData, env.ingestor.contents[bigPath], "续传拼接内容应与源一致")
}

// TestReceiveExecutionDialTerminal 中继终态拒绝（未知 token）→ 失败终态 + 用户可读文案
func TestReceiveExecutionDialTerminal(t *testing.T) {
	env := startReceiveEnv(t, SharePublishOptions{})
	// 篡改 token 为不存在的会话
	target, err := ParseShareLink(env.link)
	require.NoError(t, err)
	badTarget := &ReceiveTarget{
		RelayDial: target.RelayDial, RelayHost: target.RelayHost,
		Token: "zzzzzzzzzzzzzzzzzzzzzz", Key: target.Key,
	}
	payload, err := newShareReceivePayload(badTarget, "")
	require.NoError(t, err)
	task := entity.NewTask()
	task.ID = 777
	task.Payload = sql.NullString{String: payload, Valid: true}
	h, _ := newReceiveHandle(task)
	exec := NewReceiveExecution(env.recvSvc, env.ingestor, nil, nil)

	// 退避压缩：失败路径不含瞬态重试（not_found 终态直达），真实等待可控
	finished, failed := waitExecuteDone(t, h, exec)
	assert.False(t, finished)
	assert.Contains(t, failed, "分享已失效")
}

// TestReceiveExecutionPasswordProtected 密码分享：正确密码成功、缺密码被拒
func TestReceiveExecutionPasswordProtected(t *testing.T) {
	env := startReceiveEnv(t, SharePublishOptions{Password: "secret-pass"})

	// 缺密码 → bad_password 终态
	h1, _, exec1 := env.buildReceiveHandle(t, "")
	finished1, failed1 := waitExecuteDone(t, h1, exec1)
	assert.False(t, finished1)
	assert.Contains(t, failed1, "访问密码错误")

	// 正确密码 → 成功
	h2, _, exec2 := env.buildReceiveHandle(t, "secret-pass")
	finished2, failed2 := waitExecuteDone(t, h2, exec2)
	assert.True(t, finished2, "正确密码应成功，失败: %s", failed2)
}

// TestReceiveExecutionPauseKeepsStaging 暂停（ctx 取消）保留暂存且不置终态：
// 拨号恒失败（瞬态网络错误）进入退避窗口，窗口内取消模拟用户暂停
func TestReceiveExecutionPauseKeepsStaging(t *testing.T) {
	env := startReceiveEnv(t, SharePublishOptions{})
	// 收件侧拨号改为恒失败（瞬态网络错误语义），拉取停留在重试退避窗口
	env.recvSvc.setTunables(sessionRuntimeOptions{
		dialFn: func(addr string) (net.Conn, error) {
			return nil, errors.New("模拟网络不可达")
		},
	})
	h, cancel, exec := env.buildReceiveHandle(t, "")
	go func() {
		time.Sleep(200 * time.Millisecond) // 首次退避（1s）窗口内取消
		cancel()
	}()
	finished, failed := waitExecuteDone(t, h, exec)
	assert.False(t, finished, "暂停不应置成功终态")
	assert.Equal(t, "", failed, "暂停不应置失败终态（控制面接管）")
	assert.DirExists(t, env.stagingDirOf(), "暂停应保留暂存")
}

// TestCleanupOrphanReceiveStaging 启动清扫：仅回收任务行已不存在的暂存目录
func TestCleanupOrphanReceiveStaging(t *testing.T) {
	workDir := t.TempDir()
	root := filepath.Join(workDir, receiveStagingRootName)
	orphan := filepath.Join(root, "101")
	alive := filepath.Join(root, "102")
	for _, d := range []string{orphan, alive} {
		require.NoError(t, os.MkdirAll(d, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(d, "x.bin"), []byte("x"), 0o644))
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, "not-a-task"), []byte("junk"), 0o644))

	err := CleanupOrphanReceiveStaging(workDir, func(id int64) bool { return id == 102 })
	require.NoError(t, err)
	assert.NoDirExists(t, orphan, "任务行不存在的暂存应被回收")
	assert.DirExists(t, alive, "任务行仍存在的暂存应保留")
	_, statErr := os.Stat(filepath.Join(root, "not-a-task"))
	assert.NoError(t, statErr, "非任务目录不碰")

	require.NoError(t, CleanupOrphanReceiveStaging("", nil))
}

// —— 查重接入普通下载轨道：端到端时序（设计五）与中断窗口三态（设计六） ——

// TestReceiveExecutionReplaceConfirm 冲突弹窗决策替换（多作品整体决策）：
// 两个冲突作品经 WaitReplaceConfirm 整体答复替换 → 各自按交集角色软删 + 回滚登记 →
// 回灌 ReplaceWorks 全集（两 manifest 作品均入替换集）
func TestReceiveExecutionReplaceConfirm(t *testing.T) {
	env := startReceiveEnvModel(t, SharePublishOptions{}, buildTwoWorkModel)
	checker := &fakeDuplicateChecker{results: [][]duplicate.DuplicateCheckResult{
		{hitConflict(500, "作品A", []string{entity.StoreTypeImage}), hitConflict(600, "作品B", []string{entity.StoreTypeImage})},
	}}
	ops := &fakeReplaceOps{victimBase: 900}
	h, _, exec, ops := env.buildDupHandle(t, "", checker, ops)
	h.confirmDecision = taskManager.ReplaceDecisionReplace
	finished, failed := waitExecuteDone(t, h, exec)
	require.True(t, finished, "应成功终态，失败: %s", failed)

	// 确认载荷：两个冲突作品一次整体决策
	h.mu.Lock()
	require.Len(t, h.confirmConflicts, 1)
	require.Len(t, h.confirmConflicts[0], 2, "冲突列表应含两个作品")
	require.Equal(t, int64(500), h.confirmConflicts[0][0].WorkID)
	require.Equal(t, []string{entity.StoreTypeImage}, h.confirmConflicts[0][0].ConflictRoles)
	require.Equal(t, int64(600), h.confirmConflicts[0][1].WorkID)
	h.mu.Unlock()

	// 两个作品都按交集角色软删，回滚登记合并两 victim
	ops.mu.Lock()
	require.Len(t, ops.softCalls, 2)
	require.Equal(t, int64(500), ops.softCalls[0].workID)
	require.Equal(t, []string{entity.StoreTypeImage}, ops.softCalls[0].roles)
	require.Equal(t, int64(600), ops.softCalls[1].workID)
	ops.mu.Unlock()
	h.mu.Lock()
	require.NotNil(t, h.rollback)
	require.Len(t, h.rollback.Victims, 2, "多作品回滚清单应合并登记")
	h.mu.Unlock()

	// 回灌替换集（确认替换全集）
	env.ingestor.mu.Lock()
	require.Equal(t, 1, env.ingestor.called)
	require.NotNil(t, env.ingestor.opts)
	require.Len(t, env.ingestor.opts.ReplaceWorks, 2)
	_, okA := env.ingestor.opts.ReplaceWorks[1]
	_, okB := env.ingestor.opts.ReplaceWorks[2]
	env.ingestor.mu.Unlock()
	assert.True(t, okA && okB, "两个 manifest 作品都应入确认替换集")
	assert.NoDirExists(t, env.stagingDirOf(), "成功后暂存应清理")
}

// TestReceiveExecutionReplaceSkip 冲突弹窗决策跳过（多作品整体跳过 + 跳过作品文件不拉）：
// 两个冲突作品整体答复跳过 → 不经软删路径 → 跳过作品文件条目不拉（仅 manifest 请求）→
// 回灌维持全跳过旧语义（nil opts）
func TestReceiveExecutionReplaceSkip(t *testing.T) {
	env := startReceiveEnvModel(t, SharePublishOptions{}, buildTwoWorkModel)
	checker := &fakeDuplicateChecker{results: [][]duplicate.DuplicateCheckResult{
		{hitConflict(500, "作品A", []string{entity.StoreTypeImage}), hitConflict(600, "作品B", []string{entity.StoreTypeImage})},
	}}
	ops := &fakeReplaceOps{victimBase: 900}
	h, _, exec, ops := env.buildDupHandle(t, "", checker, ops)
	h.confirmDecision = taskManager.ReplaceDecisionSkip
	finished, failed := waitExecuteDone(t, h, exec)
	require.True(t, finished, "应成功终态，失败: %s", failed)

	// 跳过：不软删、不登记回滚（活行在，恢复重跑重查重弹）
	ops.mu.Lock()
	require.Len(t, ops.softCalls, 0, "裁决跳过不应软删")
	ops.mu.Unlock()
	h.mu.Lock()
	require.Nil(t, h.rollback, "跳过不登记回滚")
	h.mu.Unlock()

	// 回灌无替换集（全跳过旧语义）
	env.ingestor.mu.Lock()
	require.Equal(t, 1, env.ingestor.called)
	require.Nil(t, env.ingestor.opts, "全跳过应保持 nil opts（现状语义）")
	env.ingestor.mu.Unlock()

	// 被裁决跳过作品的文件不拉取：仅 manifest 请求，无 file 请求
	target, err := ParseShareLink(env.link)
	require.NoError(t, err)
	reqs := parseRecordedRequests(t, env.dialer.snapshot(), target.Key)
	require.Len(t, reqs, 1, "应恰有 manifest 请求（跳过作品文件不拉），实际: %+v", reqs)
	assert.Equal(t, "manifest", reqs[0].Type)
}

// TestReceiveExecutionReplaceAutoMerge 零交集命中自动增补（决策5，不经确认直接挂载）：
// 查重落 HitNoConflict → 不弹窗 → 按 manifest 板块角色软删 no-op（活行交集空）→
// 回灌 AutoMergeWorks 静默增补
func TestReceiveExecutionReplaceAutoMerge(t *testing.T) {
	env := startReceiveEnv(t, SharePublishOptions{})
	checker := &fakeDuplicateChecker{results: [][]duplicate.DuplicateCheckResult{
		{noConflict(500, "测试作品1001")},
	}}
	ops := &fakeReplaceOps{} // victimBase=0 → 软删 no-op 返回空清单
	h, _, exec, ops := env.buildDupHandle(t, "", checker, ops)
	finished, failed := waitExecuteDone(t, h, exec)
	require.True(t, finished, "应成功终态，失败: %s", failed)

	h.mu.Lock()
	require.Len(t, h.confirmConflicts, 0, "零交集命中不弹窗")
	h.mu.Unlock()
	// 零交集作品走软删 no-op（manifest 板块角色全集对活行无交集）
	ops.mu.Lock()
	require.Len(t, ops.softCalls, 1)
	require.Equal(t, int64(500), ops.softCalls[0].workID)
	require.ElementsMatch(t,
		[]string{entity.StoreTypeImage, entity.StoreTypeVideoMain, entity.StoreTypeDocument},
		ops.softCalls[0].roles)
	ops.mu.Unlock()
	// 自动增补回灌（无确认替换）
	env.ingestor.mu.Lock()
	require.Equal(t, 1, env.ingestor.called)
	require.NotNil(t, env.ingestor.opts)
	_, ok := env.ingestor.opts.AutoMergeWorks[1]
	require.True(t, ok, "manifest 作品应入自动增补集")
	require.Empty(t, env.ingestor.opts.ReplaceWorks, "零交集不涉及确认替换")
	env.ingestor.mu.Unlock()
}

// TestReceiveExecutionReplaceMixedSkipAuto 混合多作品决策作用域：A 冲突裁决跳过、
// B 零交集自动增补不受牵连——跳过只作用于冲突作品（其文件不拉），零交集作品照常静默挂载
func TestReceiveExecutionReplaceMixedSkipAuto(t *testing.T) {
	env := startReceiveEnvModel(t, SharePublishOptions{}, buildTwoWorkModel)
	checker := &fakeDuplicateChecker{results: [][]duplicate.DuplicateCheckResult{
		{hitConflict(500, "作品A", []string{entity.StoreTypeImage}), noConflict(600, "作品B")},
	}}
	ops := &fakeReplaceOps{victimBase: 900}
	h, _, exec, ops := env.buildDupHandle(t, "", checker, ops)
	h.confirmDecision = taskManager.ReplaceDecisionSkip
	finished, failed := waitExecuteDone(t, h, exec)
	require.True(t, finished, "应成功终态，失败: %s", failed)

	// 确认载荷仅含冲突作品 A（B 零交集不经确认）
	h.mu.Lock()
	require.Len(t, h.confirmConflicts, 1)
	require.Len(t, h.confirmConflicts[0], 1, "仅冲突作品 A 弹窗")
	require.Equal(t, int64(500), h.confirmConflicts[0][0].WorkID)
	h.mu.Unlock()
	// 软删仅作用于自动增补作品 B（no-op）；A 跳过不经软删路径
	ops.mu.Lock()
	require.Len(t, ops.softCalls, 1)
	require.Equal(t, int64(600), ops.softCalls[0].workID)
	ops.mu.Unlock()

	// 回灌：B 入自动增补集，A 不在任何替换集（跳过即整作品跳过）
	env.ingestor.mu.Lock()
	require.Equal(t, 1, env.ingestor.called)
	require.NotNil(t, env.ingestor.opts)
	_, okB := env.ingestor.opts.AutoMergeWorks[2]
	require.True(t, okB, "零交集作品 B 应入自动增补集")
	require.Empty(t, env.ingestor.opts.ReplaceWorks)
	env.ingestor.mu.Unlock()

	// A 被跳过：仅拉取 B 的文件（manifest + 1 个 file 请求）
	target, err := ParseShareLink(env.link)
	require.NoError(t, err)
	reqs := parseRecordedRequests(t, env.dialer.snapshot(), target.Key)
	require.Len(t, reqs, 2, "应恰有 manifest + B 文件两个请求，实际: %+v", reqs)
	assert.Equal(t, "manifest", reqs[0].Type)
	assert.Equal(t, "file", reqs[1].Type)
}

// TestReceiveExecutionReplaceFailRollback 失败回滚复活：软删成功后回灌导入失败 →
// 执行器 Fail 且回滚清单已登记 → 控制面 setFailed 单点按登记清单触发复活
func TestReceiveExecutionReplaceFailRollback(t *testing.T) {
	env := startReceiveEnv(t, SharePublishOptions{})
	checker := &fakeDuplicateChecker{results: [][]duplicate.DuplicateCheckResult{
		{hitConflict(500, "测试作品1001", []string{entity.StoreTypeImage})},
	}}
	ops := &fakeReplaceOps{victimBase: 900}
	h, _, exec, ops := env.buildDupHandle(t, "", checker, ops)
	h.confirmDecision = taskManager.ReplaceDecisionReplace
	env.ingestor.ingestErr = errors.New("导入失败")
	finished, failed := waitExecuteDone(t, h, exec)
	require.False(t, finished)
	require.Contains(t, failed, "导入失败")

	// 软删已发生且回滚清单已登记（失败回滚单点可据其复活）
	ops.mu.Lock()
	require.Len(t, ops.softCalls, 1)
	ops.mu.Unlock()
	h.mu.Lock()
	require.NotNil(t, h.rollback)
	require.Len(t, h.rollback.Victims, 1)
	victims := append([]resource.StoreRef(nil), h.rollback.Victims...)
	h.mu.Unlock()

	// 模拟控制面 setFailed 单点：按登记清单触发复活
	require.NoError(t, ops.RestoreReplacedStores(context.Background(), resource.RestoreScope{Victims: victims}))
	ops.mu.Lock()
	require.Len(t, ops.restoreScopes, 1)
	assert.Equal(t, victims, ops.restoreScopes[0].Victims)
	ops.mu.Unlock()
}

// TestReceiveExecutionReplaceStopRollback 停止回滚复活：软删成功后窗口内停止（ctx 取消）→
// 不上报终态、回滚清单已登记 → 控制面 setFailed 单点按登记清单触发复活（设计六 ③）
func TestReceiveExecutionReplaceStopRollback(t *testing.T) {
	env := startReceiveEnv(t, SharePublishOptions{})
	checker := &fakeDuplicateChecker{results: [][]duplicate.DuplicateCheckResult{
		{hitConflict(500, "测试作品1001", []string{entity.StoreTypeImage})},
	}}
	h, cancel, exec, ops := env.buildDupHandle(t, "", checker, &fakeReplaceOps{victimBase: 900})
	h.confirmDecision = taskManager.ReplaceDecisionReplace
	ops.cancelAfter = cancel // 软删成功后暂停注入（软删窗口内停止）
	finished, failed := waitExecuteDone(t, h, exec)
	assert.False(t, finished, "停止不应置成功终态")
	assert.Equal(t, "", failed, "停止不应置失败终态（控制面接管）")

	// 软删已发生且回滚清单已登记（停止经 setFailed 单点触发复活）
	ops.mu.Lock()
	require.Len(t, ops.softCalls, 1)
	ops.mu.Unlock()
	h.mu.Lock()
	require.NotNil(t, h.rollback)
	require.Len(t, h.rollback.Victims, 1)
	victims := append([]resource.StoreRef(nil), h.rollback.Victims...)
	h.mu.Unlock()

	// 模拟控制面停止 → setFailed 单点：按登记清单触发复活
	require.NoError(t, ops.RestoreReplacedStores(context.Background(), resource.RestoreScope{Victims: victims}))
	ops.mu.Lock()
	require.Len(t, ops.restoreScopes, 1)
	assert.Equal(t, victims, ops.restoreScopes[0].Victims)
	ops.mu.Unlock()
}

// TestReceiveExecutionPauseResumeContinuation 中断窗口三态·暂停→恢复（设计六 ②）：
// 软删后窗口内暂停（ctx 取消、不上报终态）→ 恢复重跑重查落零交集分类 → 不重弹、
// 软删 no-op、回灌 AutoMergeWorks 延续替换挂载收尾
func TestReceiveExecutionPauseResumeContinuation(t *testing.T) {
	env := startReceiveEnv(t, SharePublishOptions{})
	checker := &fakeDuplicateChecker{results: [][]duplicate.DuplicateCheckResult{
		{hitConflict(500, "测试作品1001", []string{entity.StoreTypeImage})}, // 首次：命中冲突弹窗
		{noConflict(500, "测试作品1001")},                                  // 恢复重跑：软删后零交集不重弹
	}}
	ops := &fakeReplaceOps{victimBase: 900}

	// 第一次执行：确认替换 → 软删成功 → 软删窗口内暂停（ctx 取消）→ 不上报终态
	h1, cancel1, exec, ops := env.buildDupHandle(t, "", checker, ops)
	h1.confirmDecision = taskManager.ReplaceDecisionReplace
	ops.cancelAfter = cancel1
	finished1, failed1 := waitExecuteDone(t, h1, exec)
	assert.False(t, finished1, "暂停不应置成功终态")
	assert.Equal(t, "", failed1, "暂停不应置失败终态（恢复延续）")
	h1.mu.Lock()
	require.Len(t, h1.confirmConflicts, 1, "首次应弹窗一次")
	require.NotNil(t, h1.rollback, "软删后应登记回滚")
	h1.mu.Unlock()
	ops.mu.Lock()
	require.Len(t, ops.softCalls, 1, "首次应软删")
	ops.mu.Unlock()

	// 恢复：重跑 Execute（新句柄）→ 零交集分类 → 不重弹、软删 no-op、自动增补挂载收尾
	h2, _, exec, ops := env.buildDupHandle(t, "", checker, ops)
	ops.cancelAfter = nil
	finished2, failed2 := waitExecuteDone(t, h2, exec)
	require.True(t, finished2, "恢复应成功，失败: %s", failed2)
	h2.mu.Lock()
	require.Len(t, h2.confirmConflicts, 0, "软删后恢复不重弹（零交集分类延续替换）")
	h2.mu.Unlock()
	env.ingestor.mu.Lock()
	require.Equal(t, 1, env.ingestor.called, "仅恢复执行到达回灌")
	require.NotNil(t, env.ingestor.opts)
	_, ok := env.ingestor.opts.AutoMergeWorks[1]
	require.True(t, ok, "软删后恢复应入自动增补集延续替换")
	env.ingestor.mu.Unlock()
}

// TestReceiveExecutionRetryReconfirm 中断窗口三态·停止+重试（设计六 ③）：
// 停止 → setFailed 单点按登记清单复活 → 重试重查活行复原重新命中冲突 → 重新弹窗确认
func TestReceiveExecutionRetryReconfirm(t *testing.T) {
	env := startReceiveEnv(t, SharePublishOptions{})
	checker := &fakeDuplicateChecker{results: [][]duplicate.DuplicateCheckResult{
		{hitConflict(500, "测试作品1001", []string{entity.StoreTypeImage})}, // 首次：停止回滚前
		{hitConflict(500, "测试作品1001", []string{entity.StoreTypeImage})}, // 重试：回滚后活行复原重新命中冲突
	}}
	ops := &fakeReplaceOps{victimBase: 900}

	// 停止路径：确认替换 → 软删 → 窗口内停止（ctx 取消），登记回滚清单
	h1, cancel1, exec, ops := env.buildDupHandle(t, "", checker, ops)
	h1.confirmDecision = taskManager.ReplaceDecisionReplace
	ops.cancelAfter = cancel1
	finished1, failed1 := waitExecuteDone(t, h1, exec)
	assert.False(t, finished1)
	assert.Equal(t, "", failed1)
	h1.mu.Lock()
	require.NotNil(t, h1.rollback, "停止前应登记回滚")
	victims := append([]resource.StoreRef(nil), h1.rollback.Victims...)
	h1.mu.Unlock()

	// 停止 → setFailed 单点回滚复活
	require.NoError(t, ops.RestoreReplacedStores(context.Background(), resource.RestoreScope{Victims: victims}))

	// 重试：活行复原 → 重新命中冲突 → 重新弹窗确认 → 替换完成
	h2, _, exec, ops := env.buildDupHandle(t, "", checker, ops)
	h2.confirmDecision = taskManager.ReplaceDecisionReplace
	ops.cancelAfter = nil
	finished2, failed2 := waitExecuteDone(t, h2, exec)
	require.True(t, finished2, "重试应成功，失败: %s", failed2)
	h2.mu.Lock()
	require.Len(t, h2.confirmConflicts, 1, "重试应重新弹窗")
	h2.mu.Unlock()
	env.ingestor.mu.Lock()
	require.Equal(t, 1, env.ingestor.called)
	require.NotNil(t, env.ingestor.opts)
	_, ok := env.ingestor.opts.ReplaceWorks[1]
	require.True(t, ok, "重试确认替换后应入替换集")
	env.ingestor.mu.Unlock()
}

// TestReceiveExecutionConfirmCanceled 确认挂起被取消（防御性打断）：WaitReplaceConfirm
// 返回 canceled → 不上报终态、不软删（真正中断窗口在确认之后，交控制面接管）
func TestReceiveExecutionConfirmCanceled(t *testing.T) {
	env := startReceiveEnv(t, SharePublishOptions{})
	checker := &fakeDuplicateChecker{results: [][]duplicate.DuplicateCheckResult{
		{hitConflict(500, "测试作品1001", []string{entity.StoreTypeImage})},
	}}
	ops := &fakeReplaceOps{victimBase: 900}
	h, _, exec, ops := env.buildDupHandle(t, "", checker, ops)
	h.confirmCanceled = true // 确认挂起被取消（模态弹窗期间实际不可达，仅防御性）
	finished, failed := waitExecuteDone(t, h, exec)
	assert.False(t, finished, "取消确认不应置成功终态")
	assert.Equal(t, "", failed, "取消确认不应置失败终态（交控制面接管）")
	ops.mu.Lock()
	require.Len(t, ops.softCalls, 0, "确认未答复不得软删")
	ops.mu.Unlock()
}
