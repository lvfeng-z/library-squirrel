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
	"path"
	"path/filepath"
	"strconv"
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
	"github.com/library-squirrel/backend/task"
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
		"test-instance-0001", nil, nil, nil)

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
	workDir    string           // 宿主源文件目录
	recvDir    string           // 收件方工作目录（暂存根）
	manifest   *export.Manifest // 规划后共享 manifest（写本地共享文件用）
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
	return startReceiveEnvWithTaskCtl(t, opts, build, nil)
}

// startReceiveEnvWithTaskCtl 同 startReceiveEnvModel，但收件 Service 注入指定 taskCtl
// （阶段3 建树流程测试注入 fakeBuiltinTaskControl）。
func startReceiveEnvWithTaskCtl(t *testing.T, opts SharePublishOptions,
	build func(t *testing.T, workDir string) (*export.ExportModel, map[string][]byte),
	taskCtl BuiltinTaskControl) *receiveTestEnv {
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
	hostSvc := newTestService(t, stub, hostWorkDir, model, em, dialer, nil)
	_, comp := publishAndWait(t, hostSvc, em, opts)
	if !comp.Success {
		t.Fatalf("宿主发布失败: %s", comp.ErrMsg)
	}

	// 收件侧：独立 Service（工作目录指向收件方临时目录）
	recvDir := t.TempDir()
	recvSvc := NewService(nil, nil, nil,
		func() string { return stub.addr }, func() string { return recvDir },
		"recipient-instance-0001", nil, taskCtl, nil)
	recvSvc.setTunables(sessionRuntimeOptions{dialFn: dialer.dial, streamRate: 8 << 20})

	return &receiveTestEnv{
		stub: stub, hostSvc: hostSvc, recvSvc: recvSvc, em: em,
		link: comp.Link, workDir: hostWorkDir, recvDir: recvDir,
		manifest: model.Manifest, ingestor: &fakeIngestor{}, dialer: dialer, sourceData: sourceData,
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

// testParentTaskID 收件父子树测试的父任务 ID（共享 manifest 落盘目录锚；子任务 ID 用 777 起）
const testParentTaskID = 999

// writeSharedManifest 将共享 manifest（规划后）序列化落盘到收件方父任务目录，
// 返回 workDir 相对路径（正斜杠 relPath 域，与子任务载荷 ManifestPath 同构）
func (env *receiveTestEnv) writeSharedManifest(t *testing.T) string {
	t.Helper()
	data, err := env.manifest.Serialize()
	require.NoError(t, err)
	abs := env.manifestPathOf()
	require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
	require.NoError(t, os.WriteFile(abs, data, 0o644))
	return path.Join(receiveStagingRootName, strconv.FormatInt(testParentTaskID, 10), "manifest.json")
}

// buildReceiveHandle 构建收件子任务与执行句柄（默认负责共享 manifest 第一个作品；
// cancel 供暂停路径）
func (env *receiveTestEnv) buildReceiveHandle(t *testing.T, password string) (*fakeStrategyHandle, context.CancelFunc, *ReceiveExecution) {
	t.Helper()
	require.NotEmpty(t, env.manifest.Works, "共享 manifest 应含作品")
	return env.buildReceiveHandleForWork(t, password, env.manifest.Works[0].ID, 777)
}

// buildReceiveHandleForWork 构建收件子任务与执行句柄（指定负责的 manifest 作品 ID 与任务 ID）：
// 写本地共享 manifest + 子任务载荷（ManifestPath/ManifestID）落任务行
func (env *receiveTestEnv) buildReceiveHandleForWork(t *testing.T, password string, manifestID, taskID int64) (*fakeStrategyHandle, context.CancelFunc, *ReceiveExecution) {
	t.Helper()
	rel := env.writeSharedManifest(t)
	target, err := ParseShareLink(env.link)
	require.NoError(t, err)
	payload, err := newShareReceiveChildPayload(target, password, rel, manifestID)
	require.NoError(t, err)
	task := entity.NewTask()
	task.ID = taskID
	task.TaskType = sql.NullString{String: TaskTypeReceive, Valid: true}
	task.Payload = sql.NullString{String: payload, Valid: true}
	h, cancel := newReceiveHandle(task)
	exec := NewReceiveExecution(env.recvSvc, env.ingestor, nil, nil)
	return h, cancel, exec
}

// buildDupHandle 构建带查重/替换能力的收件子任务、执行句柄与执行器（默认第一个作品；
// checker/ops 缺省为空桩，断言经返回的 ops 落地）
func (env *receiveTestEnv) buildDupHandle(t *testing.T, password string,
	checker *fakeDuplicateChecker, ops *fakeReplaceOps) (*fakeStrategyHandle, context.CancelFunc, *ReceiveExecution, *fakeReplaceOps) {
	t.Helper()
	require.NotEmpty(t, env.manifest.Works, "共享 manifest 应含作品")
	return env.buildDupHandleForWork(t, password, env.manifest.Works[0].ID, 777, checker, ops)
}

// buildDupHandleForWork 构建带查重/替换能力的收件子任务、执行句柄与执行器（指定作品与任务 ID；
// cancel 供暂停路径；checker/ops 缺省为空桩，断言经返回的 ops 落地）
func (env *receiveTestEnv) buildDupHandleForWork(t *testing.T, password string, manifestID, taskID int64,
	checker *fakeDuplicateChecker, ops *fakeReplaceOps) (*fakeStrategyHandle, context.CancelFunc, *ReceiveExecution, *fakeReplaceOps) {
	t.Helper()
	h, cancel, _ := env.buildReceiveHandleForWork(t, password, manifestID, taskID)
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
	mu            sync.Mutex
	softCalls     []softDeleteCall
	softErr       error
	victimBase    int64 // 每次软删返回的 victim StoreID 起始（0=返回空清单）
	cancelAfter   context.CancelFunc
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
	return env.stagingDirOfTask(777)
}

// stagingDirOfTask 指定任务 ID 的暂存目录
func (env *receiveTestEnv) stagingDirOfTask(taskID int64) string {
	return filepath.Join(env.recvDir, receiveStagingRootName, strconv.FormatInt(taskID, 10))
}

// manifestPathOf 共享 manifest 的绝对路径（父任务目录内；子任务 Finish 清理自己暂存不动它）
func (env *receiveTestEnv) manifestPathOf() string {
	return filepath.Join(env.stagingDirOfTask(testParentTaskID), "manifest.json")
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
	assert.FileExists(t, env.manifestPathOf(), "共享 manifest 在父目录，子任务 Finish 清理自己暂存不动它")
}

// TestReceiveExecutionSubTaskFiltersWork 子任务只处理本作品（设计四/五/六）：
// 双作品模型，子任务负责作品 B（ID 2）→ 读本地共享 manifest → 过滤出本作品子集 →
// 只拉本作品引用文件 → 导入子 manifest（Works 仅本作品、Files 仅本作品引用文件）
func TestReceiveExecutionSubTaskFiltersWork(t *testing.T) {
	env := startReceiveEnvModel(t, SharePublishOptions{}, buildTwoWorkModel)
	h, _, exec := env.buildReceiveHandleForWork(t, "", 2, 777)
	finished, failed := waitExecuteDone(t, h, exec)
	require.True(t, finished, "应成功终态，失败信息: %s", failed)

	// 导入的是子 manifest：仅本作品 + 仅本作品引用文件
	env.ingestor.mu.Lock()
	defer env.ingestor.mu.Unlock()
	require.Equal(t, 1, env.ingestor.called, "回灌导入应恰好调用一次")
	require.Len(t, env.ingestor.manifest.Works, 1, "子 manifest 应只含本作品")
	require.Equal(t, int64(2), env.ingestor.manifest.Works[0].ID, "子 manifest 作品应为作品 B")
	require.Len(t, env.ingestor.manifest.Files, 1, "子 manifest 应只含本作品引用文件")
	require.Equal(t, int64(201), env.ingestor.manifest.Files[0].StoreID, "应只含作品 B 的 StoreID 201")
	// 作品 A 的文件（StoreID 101）不得进入子 manifest
	for _, entry := range env.ingestor.manifest.Files {
		require.NotEqual(t, int64(101), entry.StoreID, "作品 A 的文件不应进入子 manifest")
	}
	// 本作品文件内容经暂存导入（与源逐字节一致）
	file := env.ingestor.manifest.Files[0]
	assert.Equal(t, env.sourceData[file.StorePath], env.ingestor.contents[file.Path], "作品 B 文件内容应与源一致")
	assert.NoDirExists(t, env.stagingDirOf(), "成功后暂存应清理")

	// 网络请求：manifest 已本地读取（无 manifest 请求），仅拉作品 B 的文件（无作品 A 文件请求）
	target, err := ParseShareLink(env.link)
	require.NoError(t, err)
	reqs := parseRecordedRequests(t, env.dialer.snapshot(), target.Key)
	require.Len(t, reqs, 1, "应恰有作品 B 文件一个请求，实际: %+v", reqs)
	assert.Equal(t, "file", reqs[0].Type)
	assert.Equal(t, file.Path, reqs[0].Path)
}

// TestReceiveExecutionOverduePayloadFails 过时载荷（ManifestID==0，存量整体任务）显式 Fail
// 并返回用户可读文案（决策2 不兼容存量，不做迁移或降级）
func TestReceiveExecutionOverduePayloadFails(t *testing.T) {
	env := startReceiveEnv(t, SharePublishOptions{})
	target, err := ParseShareLink(env.link)
	require.NoError(t, err)
	// 旧载荷：newShareReceivePayload 不写子任务字段 → ManifestID==0（过时载荷）
	payload, err := newShareReceivePayload(target, "")
	require.NoError(t, err)
	task := entity.NewTask()
	task.ID = 777
	task.TaskType = sql.NullString{String: TaskTypeReceive, Valid: true}
	task.Payload = sql.NullString{String: payload, Valid: true}
	h, _ := newReceiveHandle(task)
	exec := NewReceiveExecution(env.recvSvc, env.ingestor, nil, nil)

	finished, failed := waitExecuteDone(t, h, exec)
	assert.False(t, finished, "过时载荷不应成功")
	assert.Equal(t, "请删除本任务后重新接收分享", failed)
}

// TestReceiveExecutionResumeFromStaging 断点续传：预置完整小文件 + 半个大文件暂存，
// 重执行仅请求大文件剩余部分（offset 锚 = 已暂存字节数；manifest 已本地读取，无网络请求）
func TestReceiveExecutionResumeFromStaging(t *testing.T) {
	env := startReceiveEnv(t, SharePublishOptions{})

	// 共享 manifest 已含规划后包内路径（startReceiveEnvModel 规划产物）
	var smallPath, bigPath string
	var bigSize int64
	var bigData []byte
	for i := range env.manifest.Files {
		f := &env.manifest.Files[i]
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
			bigData = env.sourceData[f.StorePath]
		}
	}
	require.NotEmpty(t, bigPath, "夹具应含两个非缺失文件")

	// 预置暂存：小文件全量 + 大文件前半
	staging := env.stagingDirOf()
	require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(staging, filepath.FromSlash(bigPath))), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(staging, filepath.FromSlash(smallPath)),
		env.sourceData[env.manifest.Files[0].StorePath], 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(staging, filepath.FromSlash(bigPath)), bigData[:bigSize/2], 0o644))

	h, _, exec := env.buildReceiveHandle(t, "")
	finished, failed := waitExecuteDone(t, h, exec)
	require.True(t, finished, "应成功终态，失败信息: %s", failed)

	// 请求锚断言：子任务只拉本作品文件；小文件已完整暂存跳过，仅大文件续传（无 manifest 请求）
	target, err := ParseShareLink(env.link)
	require.NoError(t, err)
	reqs := parseRecordedRequests(t, env.dialer.snapshot(), target.Key)
	require.Len(t, reqs, 1, "应恰有大文件续传一个请求，实际: %+v", reqs)
	assert.Equal(t, "file", reqs[0].Type)
	assert.Equal(t, bigPath, reqs[0].Path)
	assert.Equal(t, bigSize/2, reqs[0].Offset, "续传 offset 应为已暂存字节")

	// 续传产物与源一致
	env.ingestor.mu.Lock()
	defer env.ingestor.mu.Unlock()
	assert.Equal(t, bigData, env.ingestor.contents[bigPath], "续传拼接内容应与源一致")
}

// TestReceiveExecutionDialTerminal 中继终态拒绝（未知 token）→ 失败终态 + 用户可读文案
func TestReceiveExecutionDialTerminal(t *testing.T) {
	env := startReceiveEnv(t, SharePublishOptions{})
	rel := env.writeSharedManifest(t)
	// 篡改 token 为不存在的会话
	target, err := ParseShareLink(env.link)
	require.NoError(t, err)
	badTarget := &ReceiveTarget{
		RelayDial: target.RelayDial, RelayHost: target.RelayHost,
		Token: "zzzzzzzzzzzzzzzzzzzzzz", Key: target.Key,
	}
	payload, err := newShareReceiveChildPayload(badTarget, "", rel, env.manifest.Works[0].ID)
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

// TestCleanupOrphanReceiveStagingTreeForm 启动清扫的父子树形态：收件任务为「父容器 + 每作品一
// 子任务」，目录布局为父目录 {parentID}/ 含共享 manifest.json、子目录为各子任务文件暂存，三者
// 均按任务 ID 命名的平级子目录——清扫按任务行存在性逐目录独立判定：父行删除回收父目录（含
// manifest）、子行删除回收子目录；父目录 manifest 在父行删除前不动（子任务 Finish 只清自己的
// 暂存目录，见 TestReceiveExecutionEndToEnd 断言）。
func TestCleanupOrphanReceiveStagingTreeForm(t *testing.T) {
	workDir := t.TempDir()
	root := filepath.Join(workDir, receiveStagingRootName)

	const parentID, childA, childB = 999, 777, 778
	parentDir := filepath.Join(root, strconv.FormatInt(parentID, 10))
	childADir := filepath.Join(root, strconv.FormatInt(childA, 10))
	childBDir := filepath.Join(root, strconv.FormatInt(childB, 10))
	for _, d := range []string{parentDir, childADir, childBDir} {
		require.NoError(t, os.MkdirAll(d, 0o755))
	}
	require.NoError(t, os.WriteFile(filepath.Join(parentDir, "manifest.json"), []byte("{}"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(childADir, "a.bin"), []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(childBDir, "b.bin"), []byte("b"), 0o644))

	aliveSet := map[int64]bool{parentID: true, childA: true, childB: true}
	exists := func(id int64) bool { return aliveSet[id] }

	// 全树行存活：父目录（含 manifest）与全部子目录保留
	require.NoError(t, CleanupOrphanReceiveStaging(workDir, exists))
	assert.DirExists(t, parentDir, "父行存活时父目录应保留")
	assert.FileExists(t, filepath.Join(parentDir, "manifest.json"), "父行存活时共享 manifest 应保留")
	assert.DirExists(t, childADir, "子行存活时子目录应保留")
	assert.DirExists(t, childBDir, "子行存活时子目录应保留")

	// 仅子任务 B 行删除：回收 B 子目录，父目录（含 manifest）与 A 子目录保留
	delete(aliveSet, childB)
	require.NoError(t, CleanupOrphanReceiveStaging(workDir, exists))
	assert.DirExists(t, parentDir, "父行存活时父目录应保留")
	assert.FileExists(t, filepath.Join(parentDir, "manifest.json"), "manifest 应保留直至父行删除")
	assert.DirExists(t, childADir, "A 行存活时 A 子目录应保留")
	assert.NoDirExists(t, childBDir, "B 行删除后 B 子目录应被回收")

	// 父任务行删除：回收父目录（含 manifest）；A 子目录（行仍存活）保留
	delete(aliveSet, parentID)
	require.NoError(t, CleanupOrphanReceiveStaging(workDir, exists))
	assert.NoDirExists(t, parentDir, "父行删除后父目录（含 manifest）应被回收")
	assert.NoFileExists(t, filepath.Join(parentDir, "manifest.json"), "manifest 随父目录一并回收")
	assert.DirExists(t, childADir, "A 行仍存活时 A 子目录应保留")

	// 最后 A 行也删除：A 子目录回收
	delete(aliveSet, childA)
	require.NoError(t, CleanupOrphanReceiveStaging(workDir, exists))
	assert.NoDirExists(t, childADir, "A 行删除后 A 子目录应被回收")
}

// —— 查重接入普通下载轨道：端到端时序（设计五）与中断窗口三态（设计六） ——

// TestReceiveExecutionReplaceConfirmPerWork 每作品独立查重确认替换（设计七）：
// 双作品模型，子任务 A 与子任务 B 各自独立确认替换自己的冲突作品——每个子任务
// WaitReplaceConfirm 只含本作品冲突、只软删本作品、回灌替换集只含本作品
func TestReceiveExecutionReplaceConfirmPerWork(t *testing.T) {
	env := startReceiveEnvModel(t, SharePublishOptions{}, buildTwoWorkModel)

	// 子任务 A（作品 1）：冲突命中本库作品 500
	checkerA := &fakeDuplicateChecker{results: [][]duplicate.DuplicateCheckResult{
		{hitConflict(500, "作品A", []string{entity.StoreTypeImage})},
	}}
	opsA := &fakeReplaceOps{victimBase: 900}
	hA, _, execA, opsA := env.buildDupHandleForWork(t, "", 1, 777, checkerA, opsA)
	hA.confirmDecision = taskManager.ReplaceDecisionReplace
	finished, failed := waitExecuteDone(t, hA, execA)
	require.True(t, finished, "A 应成功终态，失败: %s", failed)
	hA.mu.Lock()
	require.Len(t, hA.confirmConflicts, 1, "A 应弹窗一次")
	require.Len(t, hA.confirmConflicts[0], 1, "A 弹窗只含本作品冲突")
	require.Equal(t, int64(500), hA.confirmConflicts[0][0].WorkID)
	require.Equal(t, []string{entity.StoreTypeImage}, hA.confirmConflicts[0][0].ConflictRoles)
	hA.mu.Unlock()
	opsA.mu.Lock()
	require.Len(t, opsA.softCalls, 1, "A 只软删本作品")
	require.Equal(t, int64(500), opsA.softCalls[0].workID)
	require.Equal(t, []string{entity.StoreTypeImage}, opsA.softCalls[0].roles)
	opsA.mu.Unlock()
	env.ingestor.mu.Lock()
	require.Equal(t, 1, env.ingestor.called, "A 导入一次")
	require.Len(t, env.ingestor.opts.ReplaceWorks, 1)
	_, okA := env.ingestor.opts.ReplaceWorks[1]
	require.True(t, okA, "A 替换集只含作品 1")
	env.ingestor.mu.Unlock()

	// 子任务 B（作品 2）：冲突命中本库作品 600
	checkerB := &fakeDuplicateChecker{results: [][]duplicate.DuplicateCheckResult{
		{hitConflict(600, "作品B", []string{entity.StoreTypeImage})},
	}}
	opsB := &fakeReplaceOps{victimBase: 900}
	hB, _, execB, opsB := env.buildDupHandleForWork(t, "", 2, 778, checkerB, opsB)
	hB.confirmDecision = taskManager.ReplaceDecisionReplace
	finished, failed = waitExecuteDone(t, hB, execB)
	require.True(t, finished, "B 应成功终态，失败: %s", failed)
	hB.mu.Lock()
	require.Len(t, hB.confirmConflicts, 1, "B 应弹窗一次")
	require.Len(t, hB.confirmConflicts[0], 1, "B 弹窗只含本作品冲突")
	require.Equal(t, int64(600), hB.confirmConflicts[0][0].WorkID)
	hB.mu.Unlock()
	opsB.mu.Lock()
	require.Len(t, opsB.softCalls, 1, "B 只软删本作品")
	require.Equal(t, int64(600), opsB.softCalls[0].workID)
	opsB.mu.Unlock()
	env.ingestor.mu.Lock()
	require.Equal(t, 2, env.ingestor.called, "A、B 各导入一次")
	require.Len(t, env.ingestor.opts.ReplaceWorks, 1)
	_, okB := env.ingestor.opts.ReplaceWorks[2]
	require.True(t, okB, "B 替换集只含作品 2")
	env.ingestor.mu.Unlock()

	// 各自暂存清理（manifest 在父目录 999，不动）
	assert.NoDirExists(t, env.stagingDirOfTask(777), "A 成功后暂存应清理")
	assert.NoDirExists(t, env.stagingDirOfTask(778), "B 成功后暂存应清理")
}

// TestReceiveExecutionReplaceSkipPerWork 每作品独立裁决跳过（设计七）：子任务冲突裁决跳过
// → 不软删、不登记回滚、本作品文件不拉取（子任务内整作品跳过，回灌维持全跳过旧语义）
func TestReceiveExecutionReplaceSkipPerWork(t *testing.T) {
	env := startReceiveEnvModel(t, SharePublishOptions{}, buildTwoWorkModel)
	checker := &fakeDuplicateChecker{results: [][]duplicate.DuplicateCheckResult{
		{hitConflict(500, "作品A", []string{entity.StoreTypeImage})},
	}}
	ops := &fakeReplaceOps{victimBase: 900}
	h, _, exec, ops := env.buildDupHandleForWork(t, "", 1, 777, checker, ops)
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

	// 本作品被跳过 → 无文件拉取请求（manifest 本地读取，无任何网络请求）
	target, err := ParseShareLink(env.link)
	require.NoError(t, err)
	reqs := parseRecordedRequests(t, env.dialer.snapshot(), target.Key)
	require.Len(t, reqs, 0, "跳过作品应无拉取请求，实际: %+v", reqs)
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

// TestReceiveExecutionReplaceMixedSkipAutoPerWork 混合决策作用域（每作品独立，设计七）：
// 子任务 A 冲突裁决跳过（文件不拉）、子任务 B 零交集自动增补（照常拉取挂载）——
// 跳过只作用于本作品，互不牵连
func TestReceiveExecutionReplaceMixedSkipAutoPerWork(t *testing.T) {
	env := startReceiveEnvModel(t, SharePublishOptions{}, buildTwoWorkModel)

	// 子任务 A（作品 1）：冲突 → 跳过
	checkerA := &fakeDuplicateChecker{results: [][]duplicate.DuplicateCheckResult{
		{hitConflict(500, "作品A", []string{entity.StoreTypeImage})},
	}}
	opsA := &fakeReplaceOps{victimBase: 900}
	hA, _, execA, opsA := env.buildDupHandleForWork(t, "", 1, 777, checkerA, opsA)
	hA.confirmDecision = taskManager.ReplaceDecisionSkip
	finished, failed := waitExecuteDone(t, hA, execA)
	require.True(t, finished, "A 应成功终态，失败: %s", failed)
	hA.mu.Lock()
	require.Len(t, hA.confirmConflicts, 1, "A 弹窗一次")
	require.Len(t, hA.confirmConflicts[0], 1, "A 弹窗只含本作品冲突")
	require.Equal(t, int64(500), hA.confirmConflicts[0][0].WorkID)
	hA.mu.Unlock()
	opsA.mu.Lock()
	require.Len(t, opsA.softCalls, 0, "A 跳过不软删")
	opsA.mu.Unlock()

	// 子任务 B（作品 2）：零交集 → 自动增补（不弹窗、软删 no-op、回灌 AutoMerge）
	checkerB := &fakeDuplicateChecker{results: [][]duplicate.DuplicateCheckResult{
		{noConflict(600, "作品B")},
	}}
	opsB := &fakeReplaceOps{}
	hB, _, execB, opsB := env.buildDupHandleForWork(t, "", 2, 778, checkerB, opsB)
	finished, failed = waitExecuteDone(t, hB, execB)
	require.True(t, finished, "B 应成功终态，失败: %s", failed)
	hB.mu.Lock()
	require.Len(t, hB.confirmConflicts, 0, "B 零交集不弹窗")
	hB.mu.Unlock()
	opsB.mu.Lock()
	require.Len(t, opsB.softCalls, 1, "B 走软删 no-op")
	require.Equal(t, int64(600), opsB.softCalls[0].workID)
	opsB.mu.Unlock()

	// 回灌：B 入自动增补集，A 不在任何替换集（跳过即整作品跳过）
	env.ingestor.mu.Lock()
	require.Equal(t, 2, env.ingestor.called, "A、B 各导入一次")
	_, okB := env.ingestor.opts.AutoMergeWorks[2]
	require.True(t, okB, "零交集作品 B 应入自动增补集")
	require.Empty(t, env.ingestor.opts.ReplaceWorks)
	env.ingestor.mu.Unlock()

	// 拉取请求：A 跳过不拉，B 拉本作品文件（共 1 个 file 请求；manifest 本地读取）
	target, err := ParseShareLink(env.link)
	require.NoError(t, err)
	reqs := parseRecordedRequests(t, env.dialer.snapshot(), target.Key)
	require.Len(t, reqs, 1, "应恰有 B 文件一个请求，实际: %+v", reqs)
	assert.Equal(t, "file", reqs[0].Type)
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
		{noConflict(500, "测试作品1001")},                                   // 恢复重跑：软删后零交集不重弹
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

// TestReceiveExecutionConfirmKeptAcrossPauseResume 确认决策跨暂停/恢复保留（单次会话内确认不丢）：
// 确认替换答复后软删窗口内暂停（ctx 取消）→ 确认记忆已记录、不上报终态 → 恢复重跑
// 记忆命中 → 不重弹、复用替换决策、回灌 ReplaceWorks 延续、Finished
func TestReceiveExecutionConfirmKeptAcrossPauseResume(t *testing.T) {
	env := startReceiveEnv(t, SharePublishOptions{})
	checker := &fakeDuplicateChecker{results: [][]duplicate.DuplicateCheckResult{
		{hitConflict(500, "测试作品1001", []string{entity.StoreTypeImage})}, // 首次：确认替换
		{hitConflict(500, "测试作品1001", []string{entity.StoreTypeImage})}, // 恢复重跑：仍命中冲突
	}}
	ops := &fakeReplaceOps{victimBase: 900}

	// 第一次执行：确认替换 → 软删后暂停（ctx 取消）→ 确认记忆已记录、不上报终态
	h1, cancel1, exec, ops := env.buildDupHandle(t, "", checker, ops)
	h1.confirmDecision = taskManager.ReplaceDecisionReplace
	ops.cancelAfter = cancel1
	finished1, failed1 := waitExecuteDone(t, h1, exec)
	assert.False(t, finished1, "暂停不应置成功终态")
	assert.Equal(t, "", failed1, "暂停不应置失败终态（恢复延续）")
	h1.mu.Lock()
	require.Len(t, h1.confirmConflicts, 1, "首次应弹窗一次")
	require.NotNil(t, h1.confirmMemo, "答复后应记录确认记忆")
	require.NotNil(t, h1.rollback, "软删后应登记回滚")
	h1.mu.Unlock()
	ops.mu.Lock()
	require.Len(t, ops.softCalls, 1, "首次应软删")
	ops.mu.Unlock()

	// 恢复：确认记忆随任务保留（真实系统存于 ManagedTask，暂停不清），新句柄承接后重跑
	// 复用决策不重弹（软删对活行 no-op，回滚并集去重）
	h2, _, exec, ops := env.buildDupHandle(t, "", checker, ops)
	h2.confirmMemo = h1.confirmMemo
	ops.cancelAfter = nil
	finished2, failed2 := waitExecuteDone(t, h2, exec)
	require.True(t, finished2, "恢复应成功，失败: %s", failed2)
	h2.mu.Lock()
	require.Len(t, h2.confirmConflicts, 0, "记忆命中不重弹")
	h2.mu.Unlock()
	ops.mu.Lock()
	require.Len(t, ops.softCalls, 2, "恢复复用决策应再次软删（对活行 no-op 由真实替换链保证）")
	ops.mu.Unlock()
	env.ingestor.mu.Lock()
	require.Equal(t, 1, env.ingestor.called, "恢复执行到达回灌")
	require.NotNil(t, env.ingestor.opts)
	_, ok := env.ingestor.opts.ReplaceWorks[1]
	require.True(t, ok, "记忆复用替换决策应入替换集")
	env.ingestor.mu.Unlock()
}

// TestReceiveExecutionReRunFromFinishedReconfirms 终态清空确认记忆：首次执行确认替换
// 完成后（Finished 清空记忆）重跑 → 重新命中冲突 → 重新弹窗确认
func TestReceiveExecutionReRunFromFinishedReconfirms(t *testing.T) {
	env := startReceiveEnv(t, SharePublishOptions{})
	checker := &fakeDuplicateChecker{results: [][]duplicate.DuplicateCheckResult{
		{hitConflict(500, "测试作品1001", []string{entity.StoreTypeImage})}, // 首次：确认替换完成
		{hitConflict(500, "测试作品1001", []string{entity.StoreTypeImage})}, // 重跑：终态清空后重新命中冲突
	}}
	ops := &fakeReplaceOps{victimBase: 900}

	// 首次执行：确认替换 → 软删 → 回灌 → Finished（终态清空确认记忆）
	h1, _, exec, ops := env.buildDupHandle(t, "", checker, ops)
	h1.confirmDecision = taskManager.ReplaceDecisionReplace
	finished1, failed1 := waitExecuteDone(t, h1, exec)
	require.True(t, finished1, "首次应成功，失败: %s", failed1)
	h1.mu.Lock()
	require.Len(t, h1.confirmConflicts, 1, "首次应弹窗一次")
	h1.mu.Unlock()

	// 重跑（新句柄，记忆随终态清空为 nil）→ 重新弹窗确认 → 替换完成
	h2, _, exec, ops := env.buildDupHandle(t, "", checker, ops)
	h2.confirmDecision = taskManager.ReplaceDecisionReplace
	finished2, failed2 := waitExecuteDone(t, h2, exec)
	require.True(t, finished2, "重跑应成功，失败: %s", failed2)
	h2.mu.Lock()
	require.Len(t, h2.confirmConflicts, 1, "终态清空后重跑应重新弹窗")
	h2.mu.Unlock()
	env.ingestor.mu.Lock()
	require.Equal(t, 2, env.ingestor.called, "两次执行均到达回灌")
	require.NotNil(t, env.ingestor.opts)
	_, ok := env.ingestor.opts.ReplaceWorks[1]
	require.True(t, ok, "重跑确认替换后应入替换集")
	env.ingestor.mu.Unlock()
}

// TestReceiveExecutionPauseBeforeConfirmReconfirms 暂停在确认弹窗等待期（未答复）：
// 无确认记忆 → 恢复重新弹窗（用户确实未答，恢复需重新征询）
func TestReceiveExecutionPauseBeforeConfirmReconfirms(t *testing.T) {
	env := startReceiveEnv(t, SharePublishOptions{})
	checker := &fakeDuplicateChecker{results: [][]duplicate.DuplicateCheckResult{
		{hitConflict(500, "测试作品1001", []string{entity.StoreTypeImage})}, // 首次：弹窗等待被取消
		{hitConflict(500, "测试作品1001", []string{entity.StoreTypeImage})}, // 恢复：重新弹窗
	}}
	ops := &fakeReplaceOps{victimBase: 900}

	// 第一次执行：确认弹窗等待期暂停（用户未答复，WaitReplaceConfirm 返回取消）
	h1, _, exec, ops := env.buildDupHandle(t, "", checker, ops)
	h1.confirmCanceled = true
	finished1, failed1 := waitExecuteDone(t, h1, exec)
	assert.False(t, finished1, "确认取消不应置成功终态")
	assert.Equal(t, "", failed1, "确认取消不应置失败终态（交控制面接管）")
	h1.mu.Lock()
	require.Len(t, h1.confirmConflicts, 1, "首次应弹窗一次")
	require.Nil(t, h1.confirmMemo, "未答复不应记录确认记忆")
	h1.mu.Unlock()
	ops.mu.Lock()
	require.Len(t, ops.softCalls, 0, "确认未答复不得软删")
	ops.mu.Unlock()

	// 恢复：无记忆 → 重新弹窗确认 → 替换完成
	h2, _, exec, ops := env.buildDupHandle(t, "", checker, ops)
	h2.confirmDecision = taskManager.ReplaceDecisionReplace
	finished2, failed2 := waitExecuteDone(t, h2, exec)
	require.True(t, finished2, "恢复应成功，失败: %s", failed2)
	h2.mu.Lock()
	require.Len(t, h2.confirmConflicts, 1, "无记忆恢复应重新弹窗")
	h2.mu.Unlock()
	env.ingestor.mu.Lock()
	require.Equal(t, 1, env.ingestor.called, "恢复执行到达回灌")
	require.NotNil(t, env.ingestor.opts)
	_, ok := env.ingestor.opts.ReplaceWorks[1]
	require.True(t, ok, "恢复确认替换后应入替换集")
	env.ingestor.mu.Unlock()
}

// TestReceiveExecutionConfirmRacedWithPauseKept 答复与暂停竞态（对齐真实外层取消分支非阻塞消费
// 残留答复记记忆）：答复已投递但被暂停打断 → 确认记忆已记录、返回取消 → 恢复记忆命中不重弹
func TestReceiveExecutionConfirmRacedWithPauseKept(t *testing.T) {
	env := startReceiveEnv(t, SharePublishOptions{})
	checker := &fakeDuplicateChecker{results: [][]duplicate.DuplicateCheckResult{
		{hitConflict(500, "测试作品1001", []string{entity.StoreTypeImage})}, // 首次：答复与取消竞态
		{hitConflict(500, "测试作品1001", []string{entity.StoreTypeImage})}, // 恢复：记忆命中不重弹
	}}
	ops := &fakeReplaceOps{victimBase: 900}

	// 第一次执行：答复已投递（记录确认记忆）与暂停并发 → WaitReplaceConfirm 返回取消
	h1, _, exec, ops := env.buildDupHandle(t, "", checker, ops)
	h1.confirmDecision = taskManager.ReplaceDecisionReplace
	h1.confirmRaced = true
	finished1, failed1 := waitExecuteDone(t, h1, exec)
	assert.False(t, finished1, "竞态取消不应置成功终态")
	assert.Equal(t, "", failed1, "竞态取消不应置失败终态（交控制面接管）")
	h1.mu.Lock()
	require.Len(t, h1.confirmConflicts, 1, "首次应弹窗一次")
	require.NotNil(t, h1.confirmMemo, "竞态答复应被消费记入确认记忆")
	h1.mu.Unlock()
	ops.mu.Lock()
	require.Len(t, ops.softCalls, 0, "竞态取消未走到软删")
	ops.mu.Unlock()

	// 恢复：确认记忆随任务保留 → 记忆命中 → 不重弹、复用替换决策完成
	h2, _, exec, ops := env.buildDupHandle(t, "", checker, ops)
	h2.confirmMemo = h1.confirmMemo
	finished2, failed2 := waitExecuteDone(t, h2, exec)
	require.True(t, finished2, "恢复应成功，失败: %s", failed2)
	h2.mu.Lock()
	require.Len(t, h2.confirmConflicts, 0, "竞态记忆命中恢复不重弹")
	h2.mu.Unlock()
	env.ingestor.mu.Lock()
	require.Equal(t, 1, env.ingestor.called, "恢复执行到达回灌")
	require.NotNil(t, env.ingestor.opts)
	_, ok := env.ingestor.opts.ReplaceWorks[1]
	require.True(t, ok, "竞态记忆复用替换决策应入替换集")
	env.ingestor.mu.Unlock()
}

// —— 阶段3：Receive 建树流程 ——

// fakeBuiltinTaskControl 阶段3 建树流程的收件任务控制桩：模拟 task.Service 建树行为
// （CreateBuiltinTaskTree 原子建树；CreateBuiltinTaskParent/CreateBuiltinTaskChildren 两段式），
// 记录启动/删除调用与建树结果供断言。createChildrenErr 注入建子失败路径。
type fakeBuiltinTaskControl struct {
	mu                sync.Mutex
	nextTaskID        int64
	parent            *entity.Task
	children          []*entity.Task
	startedIDs        []int64
	deletedIDs        []int64
	createChildrenErr error
}

func (f *fakeBuiltinTaskControl) CreateBuiltinTask(ctx context.Context, taskType string, taskName string, payload string) (int64, error) {
	return 0, nil
}

func (f *fakeBuiltinTaskControl) StartTasks(ctx context.Context, taskIds []int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.startedIDs = append(f.startedIDs, taskIds...)
	return nil
}

func (f *fakeBuiltinTaskControl) CreateBuiltinTaskTree(ctx context.Context, taskType string, parentName string, children []task.BuiltinTaskChild) (*entity.Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextTaskID++
	parent := entity.NewTask()
	parent.ID = f.nextTaskID
	parent.TaskName = sql.NullString{String: parentName, Valid: true}
	parent.TaskType = sql.NullString{String: taskType, Valid: true}
	parent.HasChild = sql.NullBool{Bool: true, Valid: true}
	for _, c := range children {
		f.nextTaskID++
		child := entity.NewTask()
		child.ID = f.nextTaskID
		child.Pid = sql.NullInt64{Int64: parent.ID, Valid: true}
		child.TaskName = sql.NullString{String: c.TaskName, Valid: true}
		child.TaskType = sql.NullString{String: taskType, Valid: true}
		child.Payload = sql.NullString{String: c.Payload, Valid: c.Payload != ""}
		child.HasChild = sql.NullBool{Bool: false, Valid: true}
		f.children = append(f.children, child)
	}
	f.parent = parent
	return parent, nil
}

func (f *fakeBuiltinTaskControl) CreateBuiltinTaskParent(ctx context.Context, taskType string, parentName string) (*entity.Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextTaskID++
	parent := entity.NewTask()
	parent.ID = f.nextTaskID
	parent.TaskName = sql.NullString{String: parentName, Valid: true}
	parent.TaskType = sql.NullString{String: taskType, Valid: true}
	parent.HasChild = sql.NullBool{Bool: true, Valid: true}
	f.parent = parent
	return parent, nil
}

func (f *fakeBuiltinTaskControl) CreateBuiltinTaskChildren(ctx context.Context, taskType string, parentID int64, children []task.BuiltinTaskChild) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createChildrenErr != nil {
		return f.createChildrenErr
	}
	for _, c := range children {
		f.nextTaskID++
		child := entity.NewTask()
		child.ID = f.nextTaskID
		child.Pid = sql.NullInt64{Int64: parentID, Valid: true}
		child.TaskName = sql.NullString{String: c.TaskName, Valid: true}
		child.TaskType = sql.NullString{String: taskType, Valid: true}
		child.Payload = sql.NullString{String: c.Payload, Valid: c.Payload != ""}
		child.HasChild = sql.NullBool{Bool: false, Valid: true}
		f.children = append(f.children, child)
	}
	return nil
}

func (f *fakeBuiltinTaskControl) DeleteTask(ctx context.Context, ids []int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deletedIDs = append(f.deletedIDs, ids...)
	return nil
}

// TestReceiveBuildsTaskTree 阶段3 建树流程：同步拉 manifest → 建父子树（父容器 + 每作品一子任务）→
// 共享 manifest 落盘父任务目录 → 整树启动 → 返回 DTO（parentTaskId/workCount/workNames）。
func TestReceiveBuildsTaskTree(t *testing.T) {
	taskCtl := &fakeBuiltinTaskControl{}
	env := startReceiveEnvWithTaskCtl(t, SharePublishOptions{}, buildTwoWorkModel, taskCtl)

	res, err := env.recvSvc.Receive(context.Background(), env.link, "")
	require.NoError(t, err)

	// 返回 DTO：parentTaskId/workCount/workNames（净化后作品名）
	parentID := taskCtl.parent.GetID()
	assert.Equal(t, parentID, res.ParentTaskID)
	assert.Equal(t, 2, res.WorkCount)
	assert.Equal(t, []string{"作品A", "作品B"}, res.WorkNames)

	// 父容器：has_child=true、pid NULL、task_type 落值、命名「拉取分享（{host}）」
	parent := taskCtl.parent
	require.NotNil(t, parent)
	assert.True(t, parent.HasChild.Valid && parent.HasChild.Bool)
	assert.False(t, parent.Pid.Valid)
	assert.Equal(t, TaskTypeReceive, parent.TaskType.String)
	target, err := ParseShareLink(env.link)
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("拉取分享（%s）", target.RelayHost), parent.TaskName.String)

	// 子任务：pid=父ID、has_child=false、task_type 落值、命名 = 净化后作品名、载荷含 ManifestPath+ManifestID
	require.Len(t, taskCtl.children, 2)
	wantManifestPath := path.Join(receiveStagingRootName, strconv.FormatInt(parentID, 10), "manifest.json")
	for i, want := range []struct {
		name       string
		manifestID int64
	}{{"作品A", 1}, {"作品B", 2}} {
		child := taskCtl.children[i]
		assert.Equal(t, parentID, child.Pid.Int64)
		assert.False(t, child.HasChild.Bool)
		assert.Equal(t, TaskTypeReceive, child.TaskType.String)
		assert.Equal(t, want.name, child.TaskName.String)
		payload, err := parseShareReceivePayload(child.Payload.String)
		require.NoError(t, err)
		assert.Equal(t, wantManifestPath, payload.ManifestPath)
		assert.Equal(t, want.manifestID, payload.ManifestID)
	}

	// 共享 manifest 落盘父任务目录（可反序列化、schemaVersion 匹配、含全部作品）
	absManifest := filepath.Join(env.recvDir, filepath.FromSlash(wantManifestPath))
	data, err := os.ReadFile(absManifest)
	require.NoError(t, err)
	manifest, err := export.Deserialize(data)
	require.NoError(t, err)
	assert.Equal(t, export.SchemaVersion, manifest.SchemaVersion)
	assert.Len(t, manifest.Works, 2)

	// 整树启动：传父任务 ID
	assert.Equal(t, []int64{parentID}, taskCtl.startedIDs)
}

// TestReceiveManifestFetchFailsNoTask 阶段3 失败语义：manifest 拉取失败不建任何任务，
// 返回用户可读文案（决策5/风险1——无任务可重试，用户修正后重新接收）。
func TestReceiveManifestFetchFailsNoTask(t *testing.T) {
	taskCtl := &fakeBuiltinTaskControl{}
	env := startReceiveEnvWithTaskCtl(t, SharePublishOptions{}, buildTwoWorkModel, taskCtl)
	// 篡改 token 为不存在会话（not_found 终态直达，不重试）
	target, err := ParseShareLink(env.link)
	require.NoError(t, err)
	badLink := BuildShareLink(target.RelayHost, "zzzzzzzzzzzzzzzzzzzzzz", target.Key)

	_, err = env.recvSvc.Receive(context.Background(), badLink, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "拉取分享清单失败")

	// 未建任何任务、未启动、未删除
	assert.Nil(t, taskCtl.parent)
	assert.Empty(t, taskCtl.children)
	assert.Empty(t, taskCtl.startedIDs)
	assert.Empty(t, taskCtl.deletedIDs)
}

// TestReceiveManifestPersistFailsRollback 阶段3 失败语义：父任务创建后 manifest 落盘失败
// → 显式删除已建父任务（DeleteTask 含子任务，此处尚无子任务），不留孤儿。
func TestReceiveManifestPersistFailsRollback(t *testing.T) {
	taskCtl := &fakeBuiltinTaskControl{}
	env := startReceiveEnvWithTaskCtl(t, SharePublishOptions{}, buildTwoWorkModel, taskCtl)
	// 令 workDir/share-receive 为普通文件：MkdirAll(workDir/share-receive/{parentID}) 失败
	blocker := filepath.Join(env.recvDir, receiveStagingRootName)
	require.NoError(t, os.MkdirAll(env.recvDir, 0o755))
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))

	_, err := env.recvSvc.Receive(context.Background(), env.link, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "保存分享清单失败")
	// 回滚：删除已建父任务；未建子任务、未启动
	require.NotNil(t, taskCtl.parent)
	assert.Equal(t, []int64{taskCtl.parent.GetID()}, taskCtl.deletedIDs)
	assert.Empty(t, taskCtl.children)
	assert.Empty(t, taskCtl.startedIDs)
}

// TestReceiveChildCreateFailsRollback 阶段3 失败语义：建子任务失败 → 显式删除已建任务树
// （父任务已建、子任务未建成），不留孤儿父任务。
func TestReceiveChildCreateFailsRollback(t *testing.T) {
	taskCtl := &fakeBuiltinTaskControl{createChildrenErr: errors.New("建子任务失败")}
	env := startReceiveEnvWithTaskCtl(t, SharePublishOptions{}, buildTwoWorkModel, taskCtl)

	_, err := env.recvSvc.Receive(context.Background(), env.link, "")
	require.Error(t, err)
	// 回滚：删除已建父任务；建子失败无子任务落盘、未启动
	require.NotNil(t, taskCtl.parent)
	assert.Equal(t, []int64{taskCtl.parent.GetID()}, taskCtl.deletedIDs)
	assert.Empty(t, taskCtl.children)
	assert.Empty(t, taskCtl.startedIDs)
}

// newSelfRefReceiveSvc 组装带本地分享账本的收件 Service（自指检测测试用：独立实例，
// dialer 独立于夹具共享的宿主记录器，供零外呼断言）
func newSelfRefReceiveSvc(env *receiveTestEnv, repo *Repository, taskCtl BuiltinTaskControl,
	dialer *recordingDialer) *Service {
	svc := NewService(repo, nil, nil,
		func() string { return env.stub.addr }, func() string { return env.recvDir },
		"recipient-instance-0001", nil, taskCtl, nil)
	svc.setTunables(sessionRuntimeOptions{dialFn: dialer.dial, streamRate: 8 << 20})
	return svc
}

// TestReceiveSelfReferenceRejected 自指拒绝：链接 token 命中本地分享记录（本实例自产）
// → 接收启动即拒绝（哨兵错误可辨识），不拨中继、不拉 manifest、不建任何任务。
func TestReceiveSelfReferenceRejected(t *testing.T) {
	taskCtl := &fakeBuiltinTaskControl{}
	env := startReceiveEnvWithTaskCtl(t, SharePublishOptions{}, buildTwoWorkModel, taskCtl)
	target, err := ParseShareLink(env.link)
	require.NoError(t, err)
	// 本地账本含同 token 记录行（本实例发布过该分享）
	repo := NewRepository(openRecordTestDB(t))
	rec := entity.NewShareRecord()
	rec.ShareID = "share-selfref"
	rec.Token = target.Token
	rec.State = RecordStateActive
	require.NoError(t, repo.Create(context.Background(), rec))
	dialer := &recordingDialer{}
	recvSvc := newSelfRefReceiveSvc(env, repo, taskCtl, dialer)

	_, err = recvSvc.Receive(context.Background(), env.link, "")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrShareSelfReference)
	assert.Contains(t, err.Error(), "不能接收自己分享的内容")

	// 未拨中继（零字节外呼）、未建任务、未启动、未删除
	assert.Empty(t, dialer.snapshot())
	assert.Nil(t, taskCtl.parent)
	assert.Empty(t, taskCtl.children)
	assert.Empty(t, taskCtl.startedIDs)
	assert.Empty(t, taskCtl.deletedIDs)
}

// TestReceiveSelfReferencePassCrossInstance 跨实例放行：本实例有自己的分享账本，但链接
// token 不在其中（同账号、他实例产出）→ 非自指，正常走原有建树流程。
func TestReceiveSelfReferencePassCrossInstance(t *testing.T) {
	taskCtl := &fakeBuiltinTaskControl{}
	env := startReceiveEnvWithTaskCtl(t, SharePublishOptions{}, buildTwoWorkModel, taskCtl)
	// 本地账本仅含本实例自己的分享（token 与接收链接不同）
	repo := NewRepository(openRecordTestDB(t))
	own := entity.NewShareRecord()
	own.ShareID = "share-own"
	own.Token = "XyZaBcDeFgHiJkLmNoPqRs"
	own.State = RecordStateActive
	require.NoError(t, repo.Create(context.Background(), own))
	recvSvc := newSelfRefReceiveSvc(env, repo, taskCtl, &recordingDialer{})

	res, err := recvSvc.Receive(context.Background(), env.link, "")
	require.NoError(t, err)
	// 正常建树：父容器 + 每作品一子任务 + 整树启动
	require.NotNil(t, taskCtl.parent)
	assert.Equal(t, taskCtl.parent.GetID(), res.ParentTaskID)
	assert.Len(t, taskCtl.children, 2)
	assert.Equal(t, []int64{taskCtl.parent.GetID()}, taskCtl.startedIDs)
}
