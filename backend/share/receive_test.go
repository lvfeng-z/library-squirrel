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
	"github.com/library-squirrel/backend/export"
	importer "github.com/library-squirrel/backend/import"
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
	contents  map[string][]byte
	ingestErr error
}

func (f *fakeIngestor) Ingest(ctx context.Context, manifest *export.Manifest, fileSource importer.FileSource) (*importer.ImportResult, error) {
	f.mu.Lock()
	if f.ingestErr != nil {
		err := f.ingestErr
		f.mu.Unlock()
		return nil, err
	}
	f.called++
	f.manifest = manifest
	f.contents = make(map[string][]byte)
	manifestCopy := f.manifest
	f.mu.Unlock()
	for _, entry := range manifestCopy.Files {
		if entry.Path == "" || entry.Missing {
			continue
		}
		rc, err := fileSource(entry.Path)
		if err != nil {
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

// startReceiveEnv 发布分享并构建收件执行器夹具（link 为完整分享链接）
func startReceiveEnv(t *testing.T, opts SharePublishOptions) *receiveTestEnv {
	t.Helper()
	stub := startRelayStub(t)

	// 宿主侧：模型 + 源文件落临时目录 + 规划（填充包内路径/大小/缺失）
	hostWorkDir := t.TempDir()
	model, sourceData := buildTestModel(t, hostWorkDir)
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
	exec := NewReceiveExecution(env.recvSvc, env.ingestor)
	return h, cancel, exec
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
	exec := NewReceiveExecution(env.recvSvc, env.ingestor)

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
