package share

// 分享发布端到端测试：分享方客户端 × 中继桩（PROTOCOL.md 最小实现）。
// 覆盖：E2E 加密硬验收（决策14 ①②③）、拉取应答（manifest/分块文件/offset）、
// 路径白名单拒绝、撤销、注册被拒、保活 PING-PONG、断线重连 bind、发布取消。

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/export"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/shareLock"
	"github.com/library-squirrel/backend/taskManager"
	"github.com/lvfeng-z/library-squirrel-sdk/identity"
)

// —— 测试夹具 ——

// 发布成功路径记日志，测试进程无 logger.Init——注入 Nop 防全局 logger.Log 为 nil
func TestMain(m *testing.M) {
	logger.Log = zap.NewNop().Sugar()
	os.Exit(m.Run())
}

func strPtr(s string) *string { return &s }
func i64Ptr(v int64) *int64   { return &v }

// buildTestModel 构造含 3 个文件条目（小文件/跨块大文件/缺失文件）的导出模型，源文件落临时工作目录
func buildTestModel(t *testing.T, workDir string) (*export.ExportModel, map[string][]byte) {
	small := []byte("SMALL-PLAINTEXT-MARKER-分享明文锚点-0123456789")
	big := make([]byte, 128*1024+123)
	for i := range big {
		big[i] = byte(i % 251)
	}
	copy(big, "BIGFILE-PLAINTEXT-MARKER")
	relSmall := "store/resource/测试作者/pic_001.jpg"
	relBig := "store/resource/测试作者/video_000.mp4"
	relMissing := "store/resource/测试作者/doc.md"
	for _, p := range []string{relSmall, relBig} {
		var content []byte
		if p == relSmall {
			content = small
		} else {
			content = big
		}
		abs := filepath.Join(workDir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	manifest := &export.Manifest{
		SchemaVersion: export.SchemaVersion,
		Meta:          export.Meta{ExportedAt: 1756000000000, AppVersion: "test"},
		Sites:         []export.SiteRecord{{ID: 1, SiteKey: identity.Pixiv.Key, SiteName: strPtr("测试站")}},
		Works: []export.WorkRecord{{
			ID: 1, SiteID: i64Ptr(1), SiteWorkID: strPtr("1001"), SiteWorkName: strPtr("测试作品1001"),
			Resources: []export.ResourceRecord{{
				ID: 11, ResourceType: "image",
				Stores: []export.StoreMount{
					{StoreType: "image", Generation: "downloaded", StoreSeq: 1, StoreID: 101},
					{StoreType: "videoMain", Generation: "downloaded", StoreSeq: 1, StoreID: 102},
					{StoreType: "document", Generation: "downloaded", StoreSeq: 1, StoreID: 103},
				},
			}},
		}},
		Files: []export.FileEntry{
			{StoreID: 101, StorePath: relSmall},
			{StoreID: 102, StorePath: relBig},
			{StoreID: 103, StorePath: relMissing},
		},
	}
	return export.NewExportModel(manifest), map[string][]byte{relSmall: small, relBig: big}
}

// fakeCollector 直接返回预制模型（绕过 DB 收集，聚焦隧道/加密行为）
type fakeCollector struct {
	model *export.ExportModel
}

func (f *fakeCollector) Collect(ctx context.Context, workIDs []int64, workSetIDs []int64) (*export.ExportModel, error) {
	return f.model, nil
}

// blockingCollector 阻塞到 ctx 取消（取消路径测试）
type blockingCollector struct {
	model   *export.ExportModel
	started chan struct{}
}

func (b *blockingCollector) Collect(ctx context.Context, workIDs []int64, workSetIDs []int64) (*export.ExportModel, error) {
	close(b.started)
	<-ctx.Done()
	return nil, ctx.Err()
}

// captureEmitter 捕获事件供断言
type captureEmitter struct {
	mu        sync.Mutex
	completes map[string]ShareCompleteData
	states    map[string][]*ShareSessionDTO
	// dialQuotaFullCount 配额满通知次数（PushDialQuotaFull 计数）
	dialQuotaFullCount int
}

func newCaptureEmitter() *captureEmitter {
	return &captureEmitter{
		completes: make(map[string]ShareCompleteData),
		states:    make(map[string][]*ShareSessionDTO),
	}
}

func (c *captureEmitter) PushProgress(shareID, phase string) {}

func (c *captureEmitter) PushComplete(data ShareCompleteData) {
	c.mu.Lock()
	c.completes[data.ShareID] = data
	c.mu.Unlock()
}

func (c *captureEmitter) PushState(dto *ShareSessionDTO) {
	c.mu.Lock()
	c.states[dto.ShareID] = append(c.states[dto.ShareID], dto)
	c.mu.Unlock()
}

func (c *captureEmitter) PushReceiveLink(link string) {}

func (c *captureEmitter) PushDialQuotaFull() {
	c.mu.Lock()
	c.dialQuotaFullCount++
	c.mu.Unlock()
}

func (c *captureEmitter) waitComplete(t *testing.T, shareID string, timeout time.Duration) ShareCompleteData {
	deadline := time.Now().Add(timeout)
	for {
		c.mu.Lock()
		d, ok := c.completes[shareID]
		c.mu.Unlock()
		if ok {
			return d
		}
		if time.Now().After(deadline) {
			t.Fatalf("等待 share-events complete 超时: %s", shareID)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// waitState 等待会话进入指定状态（返回最后快照）
func (c *captureEmitter) waitState(t *testing.T, shareID, state string, timeout time.Duration) *ShareSessionDTO {
	deadline := time.Now().Add(timeout)
	for {
		c.mu.Lock()
		list := c.states[shareID]
		var last *ShareSessionDTO
		for _, d := range list {
			last = d
			if d.State == state {
				c.mu.Unlock()
				return d
			}
		}
		c.mu.Unlock()
		if time.Now().After(deadline) {
			t.Fatalf("等待会话状态 %s 超时（最后状态: %s）", state, stateOf(last))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func stateOf(d *ShareSessionDTO) string {
	if d == nil {
		return "<无事件>"
	}
	return d.State
}

// recordingDialer 记录发往中继的全部字节（E2E 硬验收 ③ 的发送侧断言锚）
type recordingDialer struct {
	mu   sync.Mutex
	sent bytes.Buffer
}

func (r *recordingDialer) dial(addr string) (net.Conn, error) {
	c, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	return &recordingConn{Conn: c, rec: r}, nil
}

func (r *recordingDialer) snapshot() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]byte(nil), r.sent.Bytes()...)
}

type recordingConn struct {
	net.Conn
	rec *recordingDialer
}

func (c *recordingConn) Write(b []byte) (int, error) {
	c.rec.mu.Lock()
	c.rec.sent.Write(b)
	c.rec.mu.Unlock()
	return c.Conn.Write(b)
}

// newTestService 组装面向桩中继的分享服务（planner 用生产 Packer，收集用桩模型）。
// lockReg 传 nil 即不接作品锁（多数测试不关心）；供流锁接线测试传真实注册中心断言登记/解除
func newTestService(t *testing.T, stub *relayStub, workDir string, model *export.ExportModel,
	em *captureEmitter, dialer *recordingDialer, lockReg shareLock.ShareLockRegistry) *Service {
	svc := NewService(nil, &fakeCollector{model: model}, export.NewPacker(),
		func() string { return stub.addr }, func() string { return workDir },
		"test-instance-0001", em, nil, lockReg)
	opts := sessionRuntimeOptions{streamRate: 8 << 20}
	if dialer != nil {
		opts.dialFn = dialer.dial
	}
	svc.setTunables(opts)
	// 测试收尾清理：撤销全部会话（终止直跑主体与客户端重连循环）
	t.Cleanup(func() {
		for _, d := range svc.Sessions(context.Background()) {
			_ = svc.Revoke(context.Background(), d.ShareID)
		}
	})
	return svc
}

// publishAndWait 经发布入口（Publish 直跑）启动宿主主体并等待完成事件
func publishAndWait(t *testing.T, svc *Service, em *captureEmitter, opts SharePublishOptions) (string, ShareCompleteData) {
	t.Helper()
	shareID, err := svc.Publish(context.Background(), []int64{1}, nil, opts)
	if err != nil {
		t.Fatalf("发布启动失败: %v", err)
	}
	return shareID, em.waitComplete(t, shareID, 8*time.Second)
}

// keyFromLink 从分享链接 fragment 解出密钥
func keyFromLink(t *testing.T, link string) (key []byte, b64 string) {
	t.Helper()
	idx := strings.Index(link, "#k=")
	if idx < 0 {
		t.Fatalf("链接无密钥 fragment: %s", link)
	}
	b64 = link[idx+3:]
	key, err := base64.RawURLEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("fragment 非法: %v", err)
	}
	return key, b64
}

// recipientDial 收件人拨号（返回连接；被拒时返回中继 ERROR）
func recipientDial(t *testing.T, addr, token, passwordHash string) (net.Conn, error) {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	h := helloPayload{Role: "recipient", Token: token, InstanceID: "recipient-test-device", PasswordHash: passwordHash}
	if err := newFrameWriter(conn, 5*time.Second).write(frameHello, 0, marshalHello(&h)); err != nil {
		_ = conn.Close()
		return nil, err
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	fr, err := readFrame(conn, defaultMaxFrame)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if fr.Type == frameError {
		var we wireErr
		_ = json.Unmarshal(fr.Payload, &we)
		_ = conn.Close()
		return nil, &we
	}
	if fr.Type != frameWelcome {
		_ = conn.Close()
		return nil, fmt.Errorf("拨号收到非预期帧 0x%02x", fr.Type)
	}
	return conn, nil
}

// recipientFetch 收件人拉取：发请求 + 半关闭，读全部应答记录至 STREAM_CLOSE。
// 返回应答头、拼接明文、收到的 DATA 帧数（分块断言用）。
func recipientFetch(t *testing.T, cip *e2eCipher, conn net.Conn, req streamRequest) (streamHeader, []byte, int, error) {
	t.Helper()
	pt, err := json.Marshal(req)
	if err != nil {
		return streamHeader{}, nil, 0, err
	}
	record, err := cip.sealRecord(pt)
	if err != nil {
		return streamHeader{}, nil, 0, err
	}
	w := newFrameWriter(conn, 5*time.Second)
	if err := w.write(frameData, 1, record); err != nil {
		return streamHeader{}, nil, 0, err
	}
	if err := w.write(frameStreamClose, 1, nil); err != nil {
		return streamHeader{}, nil, 0, err
	}
	var head streamHeader
	var content []byte
	frames := 0
	gotHead := false
	for {
		_ = conn.SetReadDeadline(time.Now().Add(15 * time.Second))
		fr, err := readFrame(conn, defaultMaxFrame)
		if err != nil {
			return head, content, frames, err
		}
		switch fr.Type {
		case frameStreamClose:
			return head, content, frames, nil
		case frameData:
			if fr.StreamID != 1 {
				return head, content, frames, fmt.Errorf("收件人连接流号非法: %d", fr.StreamID)
			}
			pt, err := cip.openRecord(fr.Payload)
			if err != nil {
				return head, content, frames, fmt.Errorf("解密应答记录失败: %w", err)
			}
			frames++
			if !gotHead {
				if err := json.Unmarshal(pt, &head); err != nil {
					return head, content, frames, fmt.Errorf("应答头解析失败: %w", err)
				}
				gotHead = true
			} else {
				content = append(content, pt...)
			}
		default:
			return head, content, frames, fmt.Errorf("拉取收到非预期帧 0x%02x", fr.Type)
		}
	}
}

// findEntry 按 StoreID 找包内路径（发布后 Plan 已填充）
func findEntry(t *testing.T, model *export.ExportModel, storeID int64) export.FileEntry {
	t.Helper()
	for _, f := range model.Manifest.Files {
		if f.StoreID == storeID {
			return f
		}
	}
	t.Fatalf("未找到 StoreID=%d 的文件条目", storeID)
	return export.FileEntry{}
}

// —— 测试用例 ——

// TestE2EHardAcceptance 决策14 硬验收：①线路字节≠明文 ②同密钥逐字节还原 ③密钥不出现在发往中继的任何字节
func TestE2EHardAcceptance(t *testing.T) {
	stub := startRelayStub(t)
	workDir := t.TempDir()
	model, files := buildTestModel(t, workDir)
	em := newCaptureEmitter()
	dialer := &recordingDialer{}
	svc := newTestService(t, stub, workDir, model, em, dialer, nil)

	shareID, comp := publishAndWait(t, svc, em, SharePublishOptions{
		Title:         "E2E 硬验收分享",
		Password:      "pw-测试-123",
		ExpireSeconds: 3600,
	})
	if !comp.Success {
		t.Fatalf("发布失败: %s", comp.ErrMsg)
	}
	token := comp.Session.Token
	key, keyB64 := keyFromLink(t, comp.Link)

	// 注册 HELLO 断言：密码只以摘要出现、有效期透传、元数据净化送达
	reg := stub.sessionOf(token).registerHELLO
	if reg.PasswordHash != PasswordHashHex("pw-测试-123") {
		t.Fatalf("注册 HELLO 密码摘要不符: %s", reg.PasswordHash)
	}
	if reg.ExpireSeconds == nil || *reg.ExpireSeconds != 3600 {
		t.Fatalf("注册 HELLO 有效期未透传: %v", reg.ExpireSeconds)
	}
	if reg.Meta == nil || reg.Meta.Title != "E2E 硬验收分享" {
		t.Fatalf("注册 HELLO 元数据缺失: %+v", reg.Meta)
	}

	// ② 同密钥解密逐字节还原：manifest（正确密码拨号）
	cip, err := newE2ECipher(key)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := recipientDial(t, stub.addr, token, PasswordHashHex("pw-测试-123"))
	if err != nil {
		t.Fatalf("收件人拨号失败: %v", err)
	}
	head, content, _, err := recipientFetch(t, cip, conn, streamRequest{Type: "manifest"})
	if err != nil {
		t.Fatalf("拉取 manifest 失败: %v", err)
	}
	if !head.OK || head.Kind != "manifest" {
		t.Fatalf("manifest 应答头异常: %+v", head)
	}
	want, err := model.Manifest.Serialize()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(content, want) {
		t.Fatal("manifest 解密结果与序列化产物非逐字节一致")
	}
	_ = conn.Close()

	// ② 小文件逐字节还原
	conn, err = recipientDial(t, stub.addr, token, PasswordHashHex("pw-测试-123"))
	if err != nil {
		t.Fatal(err)
	}
	head, content, _, err = recipientFetch(t, cip, conn, streamRequest{Type: "file", Path: findEntry(t, model, 101).Path})
	if err != nil {
		t.Fatalf("拉取文件失败: %v", err)
	}
	smallWant := files["store/resource/测试作者/pic_001.jpg"]
	if !head.OK || head.Kind != "file" || head.Size != int64(len(smallWant)) {
		t.Fatalf("文件应答头异常: %+v", head)
	}
	if !bytes.Equal(content, smallWant) {
		t.Fatal("文件解密结果与源内容非逐字节一致")
	}
	_ = conn.Close()

	// ③ 密钥字节不出现在任何发往中继的缓冲（发送侧断言）
	wire := dialer.snapshot()
	if bytes.Contains(wire, key) {
		t.Fatal("密钥原始字节出现在发往中继的字节流中")
	}
	if bytes.Contains(wire, []byte(keyB64)) {
		t.Fatal("密钥 base64url 编码出现在发往中继的字节流中")
	}
	// ① 明文不出现在线路（manifest、两个源文件、密码）
	if bytes.Contains(wire, want) {
		t.Fatal("manifest 明文出现在发往中继的字节流中")
	}
	for _, pt := range files {
		if bytes.Contains(wire, pt) {
			t.Fatal("源文件明文出现在发往中继的字节流中")
		}
	}
	if bytes.Contains(wire, []byte("pw-测试-123")) {
		t.Fatal("访问密码明文出现在发往中继的字节流中")
	}
	// 密码错误拨号被拒
	if _, err := recipientDial(t, stub.addr, token, PasswordHashHex("wrong")); err == nil {
		t.Fatal("错误密码拨号不应成功")
	} else if we, ok := err.(*wireErr); !ok || we.Code != errCodeBadPassword {
		t.Fatalf("错误密码应答 bad_password, got %v", err)
	}
	_ = shareID
}

// TestRecipientPullFileChunked 跨块大文件分块应答与 offset 断点拉取（背压分块路径）
func TestRecipientPullFileChunked(t *testing.T) {
	stub := startRelayStub(t)
	workDir := t.TempDir()
	model, files := buildTestModel(t, workDir)
	em := newCaptureEmitter()
	svc := newTestService(t, stub, workDir, model, em, nil, nil)

	_, comp := publishAndWait(t, svc, em, SharePublishOptions{})
	if !comp.Success {
		t.Fatalf("发布失败: %s", comp.ErrMsg)
	}
	key, _ := keyFromLink(t, comp.Link)
	cip, _ := newE2ECipher(key)
	bigWant := files["store/resource/测试作者/video_000.mp4"]

	conn, err := recipientDial(t, stub.addr, comp.Session.Token, "")
	if err != nil {
		t.Fatal(err)
	}
	head, content, frames, err := recipientFetch(t, cip, conn, streamRequest{Type: "file", Path: findEntry(t, model, 102).Path})
	if err != nil {
		t.Fatalf("拉取大文件失败: %v", err)
	}
	if !head.OK || head.Size != int64(len(bigWant)) {
		t.Fatalf("大文件应答头异常: %+v", head)
	}
	if !bytes.Equal(content, bigWant) {
		t.Fatal("大文件分块重组与源内容非逐字节一致")
	}
	// 分块断言：内容 + 头记录数须超过单块（chunk 16KiB，内容 ~128KiB）
	minFrames := 1 + len(bigWant)/(16*1024)
	if frames < minFrames {
		t.Fatalf("分块帧数不足: %d（期望 ≥ %d，背压分块路径未生效）", frames, minFrames)
	}
	_ = conn.Close()

	// offset 拉取：从偏移 100 起的尾部
	conn, err = recipientDial(t, stub.addr, comp.Session.Token, "")
	if err != nil {
		t.Fatal(err)
	}
	head, content, _, err = recipientFetch(t, cip, conn, streamRequest{Type: "file", Path: findEntry(t, model, 102).Path, Offset: 100})
	if err != nil {
		t.Fatalf("offset 拉取失败: %v", err)
	}
	if !head.OK || head.Offset != 100 || head.Size != int64(len(bigWant))-100 {
		t.Fatalf("offset 应答头异常: %+v", head)
	}
	if !bytes.Equal(content, bigWant[100:]) {
		t.Fatal("offset 拉取内容与源尾部不一致")
	}
	_ = conn.Close()
}

// TestManifestFileEntryFingerprintsFilled 锚定分享宿主在 Serialize 前预填 manifest 文件内容指纹：
// 非 Missing 文件 ContentFingerprint（size + 头部 64KB SHA256，`<size>:<hex>`）与全量 Sha256
// 均与直接对源内容计算一致；Missing 文件双字段留空。
func TestManifestFileEntryFingerprintsFilled(t *testing.T) {
	stub := startRelayStub(t)
	workDir := t.TempDir()
	model, files := buildTestModel(t, workDir)
	em := newCaptureEmitter()
	svc := newTestService(t, stub, workDir, model, em, nil, nil)

	_, comp := publishAndWait(t, svc, em, SharePublishOptions{Title: "指纹填充锚", ExpireSeconds: 3600})
	if !comp.Success {
		t.Fatalf("发布失败: %s", comp.ErrMsg)
	}

	require.Len(t, model.Manifest.Files, 3)
	for _, f := range model.Manifest.Files {
		if f.Missing {
			assert.Empty(t, f.ContentFingerprint, "缺失文件不填头部指纹")
			assert.Empty(t, f.Sha256, "缺失文件不填全量哈希")
			continue
		}
		content, ok := files[f.StorePath]
		require.True(t, ok, "非缺失文件应有源内容: %s", f.StorePath)
		require.NotEmpty(t, f.ContentFingerprint, "非缺失文件应填头部指纹: %s", f.StorePath)
		require.NotEmpty(t, f.Sha256, "非缺失文件应填全量哈希: %s", f.StorePath)

		// 头部指纹格式 `<size>:<hex>`（size 为文件实际字节数），与库内 content_fingerprint 同口径
		parts := strings.SplitN(f.ContentFingerprint, ":", 2)
		require.Len(t, parts, 2, "头部指纹格式应为 <size>:<hex>: %s", f.ContentFingerprint)
		assert.Equal(t, strconv.FormatInt(int64(len(content)), 10), parts[0], "头部指纹 size 分量应等于文件字节数")

		headLen := len(content)
		if headLen > 64*1024 {
			headLen = 64 * 1024
		}
		headSum := sha256.Sum256(content[:headLen])
		assert.Equal(t, hex.EncodeToString(headSum[:]), parts[1], "头部指纹哈希与源头部直算不一致")

		fullSum := sha256.Sum256(content)
		assert.Equal(t, hex.EncodeToString(fullSum[:]), f.Sha256, "全量哈希与源内容直算不一致")
	}
}

// TestPathWhitelistDenied 路径白名单拒绝：白名单外路径/缺失文件/非法请求/密文损坏一律拒绝
func TestPathWhitelistDenied(t *testing.T) {
	stub := startRelayStub(t)
	workDir := t.TempDir()
	model, _ := buildTestModel(t, workDir)
	em := newCaptureEmitter()
	svc := newTestService(t, stub, workDir, model, em, nil, nil)

	_, comp := publishAndWait(t, svc, em, SharePublishOptions{})
	if !comp.Success {
		t.Fatalf("发布失败: %s", comp.ErrMsg)
	}
	key, _ := keyFromLink(t, comp.Link)
	cip, _ := newE2ECipher(key)

	cases := []struct {
		name    string
		req     streamRequest
		wantErr string
	}{
		{"穿越路径", streamRequest{Type: "file", Path: "../../etc/passwd"}, streamErrNotFound},
		{"绝对路径", streamRequest{Type: "file", Path: "/etc/passwd"}, streamErrNotFound},
		{"白名单外包内路径", streamRequest{Type: "file", Path: "works/不存在/文件.jpg"}, streamErrNotFound},
		{"源缺失文件", streamRequest{Type: "file", Path: findEntry(t, model, 103).Path}, streamErrMissing},
		{"未知请求类型", streamRequest{Type: "exec"}, streamErrBadRequest},
		{"负偏移", streamRequest{Type: "file", Path: findEntry(t, model, 101).Path, Offset: -1}, streamErrBadRequest},
	}
	for _, c := range cases {
		conn, err := recipientDial(t, stub.addr, comp.Session.Token, "")
		if err != nil {
			t.Fatalf("[%s] 拨号失败: %v", c.name, err)
		}
		head, _, _, err := recipientFetch(t, cip, conn, c.req)
		if err != nil {
			t.Fatalf("[%s] 拉取失败: %v", c.name, err)
		}
		if head.OK || head.Error != c.wantErr {
			t.Fatalf("[%s] 应答异常: %+v（期望 error=%s）", c.name, head, c.wantErr)
		}
		_ = conn.Close()
	}

	// 密文损坏请求（无法解密）→ bad_request
	conn, err := recipientDial(t, stub.addr, comp.Session.Token, "")
	if err != nil {
		t.Fatal(err)
	}
	w := newFrameWriter(conn, 5*time.Second)
	if err := w.write(frameData, 1, []byte("不是合法密文记录")); err != nil {
		t.Fatal(err)
	}
	if err := w.write(frameStreamClose, 1, nil); err != nil {
		t.Fatal(err)
	}
	head, _, _, err := recipientFetch(t, cip, conn, streamRequest{Type: "manifest"})
	if err != nil {
		t.Fatalf("密文损坏请求读取失败: %v", err)
	}
	if head.OK || head.Error != streamErrBadRequest {
		t.Fatalf("密文损坏应答异常: %+v", head)
	}
	_ = conn.Close()
}

// TestRevokeLifecycle 撤销：REVOKE→RESULT→终态，后续拨号被拒
func TestRevokeLifecycle(t *testing.T) {
	stub := startRelayStub(t)
	workDir := t.TempDir()
	model, _ := buildTestModel(t, workDir)
	em := newCaptureEmitter()
	svc := newTestService(t, stub, workDir, model, em, nil, nil)

	shareID, comp := publishAndWait(t, svc, em, SharePublishOptions{})
	if !comp.Success {
		t.Fatalf("发布失败: %s", comp.ErrMsg)
	}
	token := comp.Session.Token

	if err := svc.Revoke(context.Background(), shareID); err != nil {
		t.Fatalf("撤销失败: %v", err)
	}
	// 会话事件推进到 revoked 终态
	em.waitState(t, shareID, stateRevoked, 5*time.Second)
	// 桩侧会话同样 revoked（RESULT 已送达）
	deadline := time.Now().Add(3 * time.Second)
	for stub.sessionOf(token).state != "revoked" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if stub.sessionOf(token).state != "revoked" {
		t.Fatal("中继桩侧会话未被撤销")
	}
	// 后续拨号被拒（revoked）
	if _, err := recipientDial(t, stub.addr, token, ""); err == nil {
		t.Fatal("撤销后拨号不应成功")
	} else if we, ok := err.(*wireErr); !ok || we.Code != errCodeRevoked {
		t.Fatalf("撤销后拨号应答 revoked, got %v", err)
	}
	// 会话仍列于清单（终态展示）
	found := false
	for _, d := range svc.Sessions(context.Background()) {
		if d.ShareID == shareID && d.State == stateRevoked {
			found = true
		}
	}
	if !found {
		t.Fatal("撤销后会话应保留在清单（终态）")
	}
}

// TestRegisterRejected 注册被拒（终态 code）：发布失败、会话不留驻
func TestRegisterRejected(t *testing.T) {
	stub := startRelayStub(t)
	stub.setRejectRegister(errCodeBanned)
	workDir := t.TempDir()
	model, _ := buildTestModel(t, workDir)
	em := newCaptureEmitter()
	svc := newTestService(t, stub, workDir, model, em, nil, nil)

	shareID, comp := publishAndWait(t, svc, em, SharePublishOptions{})
	if comp.Success {
		t.Fatal("被拒注册的发布不应成功")
	}
	if !strings.Contains(comp.ErrMsg, errCodeBanned) {
		t.Fatalf("失败原因应含中继错误码 %s: %s", errCodeBanned, comp.ErrMsg)
	}
	if list := svc.Sessions(context.Background()); len(list) != 0 {
		t.Fatalf("失败会话不应留驻清单: %d", len(list))
	}
	_ = shareID
}

// TestPingPongKeepalive 保活：中继 PING → 客户端 PONG
func TestPingPongKeepalive(t *testing.T) {
	stub := startRelayStub(t)
	workDir := t.TempDir()
	model, _ := buildTestModel(t, workDir)
	em := newCaptureEmitter()
	svc := newTestService(t, stub, workDir, model, em, nil, nil)

	_, comp := publishAndWait(t, svc, em, SharePublishOptions{})
	if !comp.Success {
		t.Fatalf("发布失败: %s", comp.ErrMsg)
	}
	if err := stub.sendPingToTunnel(comp.Session.Token); err != nil {
		t.Fatalf("发送 PING 失败: %v", err)
	}
	select {
	case <-stub.pongCh:
		// PONG 已回
	case <-time.After(3 * time.Second):
		t.Fatal("客户端未在 3s 内应答 PONG")
	}
}

// TestReconnectBind 断线重连：隧道被服务端断开后客户端 bind 重绑，链接恢复可用
func TestReconnectBind(t *testing.T) {
	stub := startRelayStub(t)
	workDir := t.TempDir()
	model, files := buildTestModel(t, workDir)
	em := newCaptureEmitter()
	svc := newTestService(t, stub, workDir, model, em, nil, nil)

	shareID, comp := publishAndWait(t, svc, em, SharePublishOptions{})
	if !comp.Success {
		t.Fatalf("发布失败: %s", comp.ErrMsg)
	}
	token := comp.Session.Token
	key, _ := keyFromLink(t, comp.Link)

	// 服务端强制断隧道 → 客户端应进入重连并以 bind 恢复在线
	stub.dropTunnel(token)
	em.waitState(t, shareID, stateReconnecting, 5*time.Second)

	deadline := time.Now().Add(15 * time.Second)
	for (stub.bindCountOf(token) < 1 || stub.tunnelOf(token) == nil) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if stub.bindCountOf(token) < 1 {
		t.Fatal("客户端未发起 bind 重连")
	}
	em.waitState(t, shareID, stateOnline, 10*time.Second)

	// 重连后拉取恢复（manifest + 文件各一次）
	cip, _ := newE2ECipher(key)
	conn, err := recipientDial(t, stub.addr, token, "")
	if err != nil {
		t.Fatalf("重连后拨号失败: %v", err)
	}
	head, content, _, err := recipientFetch(t, cip, conn, streamRequest{Type: "file", Path: findEntry(t, model, 101).Path})
	if err != nil {
		t.Fatalf("重连后拉取失败: %v", err)
	}
	if !head.OK || !bytes.Equal(content, files["store/resource/测试作者/pic_001.jpg"]) {
		t.Fatal("重连后拉取内容不一致")
	}
	_ = conn.Close()
}

// TestCancelPublish 发布取消（弹窗「取消」路径）：收集阶段取消 → complete 推送「已取消」、
// 会话不留驻、不产生分享记录
func TestCancelPublish(t *testing.T) {
	stub := startRelayStub(t)
	workDir := t.TempDir()
	model, _ := buildTestModel(t, workDir)
	em := newCaptureEmitter()
	bc := &blockingCollector{model: model, started: make(chan struct{})}
	svc := NewService(nil, bc, export.NewPacker(),
		func() string { return stub.addr }, func() string { return workDir },
		"test-instance-0001", em, nil, nil)
	svc.setTunables(sessionRuntimeOptions{streamRate: 8 << 20})

	shareID, err := svc.Publish(context.Background(), []int64{1}, nil, SharePublishOptions{})
	if err != nil {
		t.Fatal(err)
	}
	<-bc.started // 等进入收集阶段
	svc.CancelPublish(context.Background(), shareID)
	comp := em.waitComplete(t, shareID, 5*time.Second)
	if comp.Success {
		t.Fatal("取消的发布不应成功")
	}
	if comp.ErrMsg != "已取消" {
		t.Fatalf("取消原因不符: %q", comp.ErrMsg)
	}
	if list := svc.Sessions(context.Background()); len(list) != 0 {
		t.Fatal("取消后会话不应留驻清单")
	}
}

// —— 发布直跑 + 分享记录生命周期 ——

// failingCollector 收集恒失败（复原时作品已删场景）
type failingCollector struct {
	err error
}

func (f *failingCollector) Collect(ctx context.Context, workIDs []int64, workSetIDs []int64) (*export.ExportModel, error) {
	return nil, f.err
}

// fakeStrategyHandle 收件执行面句柄桩（记录终态与进度上报；ReceiveExecution 测试用）
type fakeStrategyHandle struct {
	task             *entity.Task
	runCtx           context.Context
	mu               sync.Mutex
	finished         bool
	failed           bool
	errMsg           string
	progresses       [][2]int64
	confirmDecision  taskManager.ReplaceDecision     // WaitReplaceConfirm 返回值（测试预设）
	confirmCanceled  bool                            // WaitReplaceConfirm 返回值（测试预设）
	confirmRaced     bool                            // 答复与暂停竞态注入：记录确认记忆但返回取消（对齐真实外层取消分支）
	confirmMemo      *taskManager.ReplaceConfirmMemo // 确认决策记忆（暂停/恢复保留，终态清空）
	confirmConflicts [][]taskManager.ConflictInfo
	rollback         *taskManager.TerminalRollback
}

func (h *fakeStrategyHandle) Task() *entity.Task      { return h.task }
func (h *fakeStrategyHandle) RunCtx() context.Context { return h.runCtx }
func (h *fakeStrategyHandle) Finish() {
	h.mu.Lock()
	h.finished = true
	h.confirmMemo = nil // 终态清空确认记忆（对齐真实 strategyHandle.Finish，重跑重新确认）
	h.mu.Unlock()
}
func (h *fakeStrategyHandle) Fail(errMsg string) {
	h.mu.Lock()
	h.failed = true
	h.errMsg = errMsg
	h.confirmMemo = nil // 终态清空确认记忆（对齐真实 setFailed，重试重新确认）
	h.mu.Unlock()
}
func (h *fakeStrategyHandle) ReportProgress(t, f int64) {
	h.mu.Lock()
	h.progresses = append(h.progresses, [2]int64{t, f})
	h.mu.Unlock()
}

// ConfirmMemo 返回已记住的确认决策记忆（无则 nil）
func (h *fakeStrategyHandle) ConfirmMemo() *taskManager.ReplaceConfirmMemo {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.confirmMemo
}
func (h *fakeStrategyHandle) WaitReplaceConfirm(conflicts []taskManager.ConflictInfo) (taskManager.ReplaceDecision, bool) {
	h.mu.Lock()
	h.confirmConflicts = append(h.confirmConflicts, conflicts)
	d, c := h.confirmDecision, h.confirmCanceled
	switch {
	case h.confirmRaced:
		// 答复与暂停竞态（对齐真实外层取消分支非阻塞消费残留答复记记忆）：记录决策后返回取消，
		// 返回决策值按真实语义固定 Skip（取消时调用方忽略决策，记忆承载实际答复）
		h.confirmMemo = &taskManager.ReplaceConfirmMemo{
			ConflictWorkIds: conflictWorkIDsOf(conflicts),
			Decision:        d,
		}
		c = true
		d = taskManager.ReplaceDecisionSkip
	case !c:
		// 正常答复：记录确认决策记忆（对齐真实 strategyHandle——答复消费即记记忆）
		h.confirmMemo = &taskManager.ReplaceConfirmMemo{
			ConflictWorkIds: conflictWorkIDsOf(conflicts),
			Decision:        d,
		}
	}
	h.mu.Unlock()
	return d, c
}
func (h *fakeStrategyHandle) SetTerminalRollback(rollback taskManager.TerminalRollback) {
	h.mu.Lock()
	if len(rollback.Victims) == 0 {
		h.mu.Unlock()
		return
	}
	// 合并累积（对齐真实 strategyHandle：多作品软删按 StoreID 去重登记，终态回滚覆盖全部软删行）
	if h.rollback == nil {
		h.rollback = &taskManager.TerminalRollback{}
	}
	seen := make(map[int64]struct{}, len(h.rollback.Victims))
	for _, v := range h.rollback.Victims {
		seen[v.StoreID] = struct{}{}
	}
	for _, v := range rollback.Victims {
		if _, dup := seen[v.StoreID]; dup {
			continue
		}
		seen[v.StoreID] = struct{}{}
		h.rollback.Victims = append(h.rollback.Victims, v)
	}
	h.mu.Unlock()
}
func (h *fakeStrategyHandle) isTerminal() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.finished || h.failed
}

// TestPublishPreconditionRejected 前置校验失败不启动发布主体
func TestPublishPreconditionRejected(t *testing.T) {
	svc := NewService(nil, &fakeCollector{model: nil}, export.NewPacker(),
		func() string { return "" }, func() string { return "" },
		"test-instance-0001", nil, nil, nil)
	if _, err := svc.Publish(context.Background(), []int64{1}, nil, SharePublishOptions{}); err != ErrShareRelayNotConfigured {
		t.Fatalf("中继未配置应拒绝: %v", err)
	}
}

// TestShareNotifyDialQuotaFullDedup：配额满通知冷却去重——冷却期内多次触发只推一次，
// 冷却滑出后再触发重新推送（风暴期多 fetch 并发阻塞只提示一次）。
func TestShareNotifyDialQuotaFullDedup(t *testing.T) {
	em := newCaptureEmitter()
	svc := NewService(nil, nil, nil, func() string { return "" }, func() string { return "" },
		"test-instance-0001", em, nil, nil)
	svc.notifyDialQuotaFull()
	svc.notifyDialQuotaFull()
	em.mu.Lock()
	n := em.dialQuotaFullCount
	em.mu.Unlock()
	if n != 1 {
		t.Fatalf("冷却期内重复触发应去重为一次，实际 %d", n)
	}
	// 回拨冷却起点越过冷却窗 → 再次触发应重新推送
	svc.dialQuotaFullMu.Lock()
	svc.lastDialQuotaFullNotify = time.Now().Add(-dialQuotaFullNotifyCooldown - time.Second)
	svc.dialQuotaFullMu.Unlock()
	svc.notifyDialQuotaFull()
	em.mu.Lock()
	n = em.dialQuotaFullCount
	em.mu.Unlock()
	if n != 2 {
		t.Fatalf("冷却滑出后再触发应推送，实际 %d", n)
	}
}

// openRecordTestDB 打开完整迁移的内存测试库（share_record 表就位）
func openRecordTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Fatalf("打开测试库失败: %v", err)
	}
	return db
}

// newRecordTestService 组装落记录的分享服务（发布直跑 + share_record 账本）
func newRecordTestService(t *testing.T, repo *Repository, stub *relayStub, workDir string,
	model *export.ExportModel, em *captureEmitter) *Service {
	t.Helper()
	svc := NewService(repo, &fakeCollector{model: model}, export.NewPacker(),
		func() string { return stub.addr }, func() string { return workDir },
		"test-instance-0001", em, nil, nil)
	svc.setTunables(sessionRuntimeOptions{streamRate: 8 << 20})
	t.Cleanup(func() {
		for _, d := range svc.Sessions(context.Background()) {
			_ = svc.Revoke(context.Background(), d.ShareID)
		}
	})
	return svc
}

// waitRecordState 轮询等待分享记录达到期望状态（记录写入/终态由直跑主体 goroutine 异步落）
func waitRecordState(t *testing.T, repo *Repository, shareID, state string) *entity.ShareRecord {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for {
		rec, err := repo.GetByShareID(context.Background(), shareID)
		if err != nil {
			t.Fatalf("查询分享记录失败: %v", err)
		}
		if rec != nil && rec.State == state {
			return rec
		}
		if time.Now().After(deadline) {
			cur := "<无记录>"
			if rec != nil {
				cur = rec.State
			}
			t.Fatalf("等待分享记录状态 %s 超时（当前: %s）", state, cur)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestPublishRecordLifecycle 发布直跑生命周期落账（无任务参与）：首次在线落 active 行、
// 记录可重建原链接；撤销 → revoked 并记撤销时刻
func TestPublishRecordLifecycle(t *testing.T) {
	stub := startRelayStub(t)
	repo := NewRepository(openRecordTestDB(t))
	workDir := t.TempDir()
	model, _ := buildTestModel(t, workDir)
	em := newCaptureEmitter()
	svc := newRecordTestService(t, repo, stub, workDir, model, em)

	shareID, comp := publishAndWait(t, svc, em, SharePublishOptions{
		Title:         "直跑发布",
		Password:      "pw-直跑",
		ExpireSeconds: 3600,
	})
	if !comp.Success {
		t.Fatalf("发布失败: %s", comp.ErrMsg)
	}
	if !strings.HasPrefix(shareID, "share-") {
		t.Fatalf("shareID 形态不符: %q", shareID)
	}

	rec := waitRecordState(t, repo, shareID, RecordStateActive)
	if rec.Token != comp.Session.Token {
		t.Fatalf("记录 token 与会话不一致: %s / %s", rec.Token, comp.Session.Token)
	}
	if rec.Title != "直跑发布" || !rec.PasswordProtected || rec.ExpireSeconds != 3600 || rec.ExpiresAt == 0 {
		t.Fatalf("记录参数不符: %+v", rec)
	}
	// 规划统计：2 个存在文件 + 1 个缺失条目
	if rec.FileCount != 2 || rec.MissingFiles != 1 || rec.TotalBytes == 0 {
		t.Fatalf("记录规划统计不符: fileCount=%d missing=%d totalBytes=%d", rec.FileCount, rec.MissingFiles, rec.TotalBytes)
	}
	if rec.RevokedAt != 0 {
		t.Fatal("active 记录不应有撤销时刻")
	}
	workIDs, _ := unmarshalInt64s(rec.WorkIDs)
	if len(workIDs) != 1 || workIDs[0] != 1 {
		t.Fatalf("记录分享对象不符: %v", workIDs)
	}
	// 记录可重建原链接（relayAddress+token+key）
	if dto := toShareRecordDTO(rec); dto.Link != comp.Link {
		t.Fatalf("记录重建链接与发布链接不一致:\n%s\n%s", dto.Link, comp.Link)
	}

	// 撤销 → revoked + 撤销时刻
	if err := svc.Revoke(context.Background(), shareID); err != nil {
		t.Fatal(err)
	}
	em.waitState(t, shareID, stateRevoked, 5*time.Second)
	rec = waitRecordState(t, repo, shareID, RecordStateRevoked)
	if rec.RevokedAt == 0 {
		t.Fatal("撤销终态应记撤销时刻")
	}
}

// TestRecordExpiredFromRelay 中继推送过期终态（隧道内 ERROR expired）→ 会话 expired → 记录 expired
func TestRecordExpiredFromRelay(t *testing.T) {
	stub := startRelayStub(t)
	repo := NewRepository(openRecordTestDB(t))
	workDir := t.TempDir()
	model, _ := buildTestModel(t, workDir)
	em := newCaptureEmitter()
	svc := newRecordTestService(t, repo, stub, workDir, model, em)

	shareID, comp := publishAndWait(t, svc, em, SharePublishOptions{})
	if !comp.Success {
		t.Fatalf("发布失败: %s", comp.ErrMsg)
	}
	if err := stub.pushTunnelError(comp.Session.Token, errCodeExpired); err != nil {
		t.Fatalf("推送过期终态失败: %v", err)
	}
	em.waitState(t, shareID, stateExpired, 5*time.Second)
	waitRecordState(t, repo, shareID, RecordStateExpired)
}

// TestDeleteRecord 删除分享记录：在线删除（撤销会话+物理删行）、无记录删除报不存在、
// 离线 active 记录删除
func TestDeleteRecord(t *testing.T) {
	stub := startRelayStub(t)
	repo := NewRepository(openRecordTestDB(t))
	workDir := t.TempDir()
	model, _ := buildTestModel(t, workDir)
	em := newCaptureEmitter()
	svc := newRecordTestService(t, repo, stub, workDir, model, em)

	shareID, comp := publishAndWait(t, svc, em, SharePublishOptions{})
	if !comp.Success {
		t.Fatalf("发布失败: %s", comp.ErrMsg)
	}
	// 在线删除：会话撤销 + 记录物理删除
	if err := svc.DeleteRecord(context.Background(), shareID); err != nil {
		t.Fatalf("在线删除失败: %v", err)
	}
	em.waitState(t, shareID, stateRevoked, 5*time.Second)
	if rec, _ := repo.GetByShareID(context.Background(), shareID); rec != nil {
		t.Fatalf("删除后记录行应不存在: %+v", rec)
	}
	// 无记录且无主体 → NotFound
	if err := svc.DeleteRecord(context.Background(), "share-不存在"); err == nil {
		t.Fatal("无记录删除应报不存在")
	}
	// 离线 active 记录（会话不在注册表）：本地删行（中继侧存续至到期）
	key := make([]byte, shareKeyLen)
	rec := entity.NewShareRecord()
	rec.ShareID = "share-9003"
	rec.Token = fmt.Sprintf("stubtoken%013d", 9003)
	rec.Title = "离线记录"
	rec.WorkIDs = marshalInt64s([]int64{1})
	rec.RelayAddress = stub.addr
	rec.KeyB64 = base64.RawURLEncoding.EncodeToString(key)
	rec.ExpireSeconds = -1
	rec.State = RecordStateActive
	if err := repo.Create(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteRecord(context.Background(), "share-9003"); err != nil {
		t.Fatalf("离线删除失败: %v", err)
	}
	if rec, _ := repo.GetByShareID(context.Background(), "share-9003"); rec != nil {
		t.Fatal("离线删除后记录行应不存在")
	}
}

// —— 发布选择集收窄 ——

// buildSelectionModel 构造发布选择集测试模型：fileWorkIDs 每作品配一个已落盘文件条目；
// bareWorkIDs 为活但无任何文件挂载的作品（身份在 Works 中、贡献 0 文件的边界形态）；
// workSetIDs 入 WorkSets。软删作品按定义不出现在模型中（Collect 活作品查询的形态）。
func buildSelectionModel(t *testing.T, workDir string, fileWorkIDs, bareWorkIDs, workSetIDs []int64) *export.ExportModel {
	t.Helper()
	manifest := &export.Manifest{
		SchemaVersion: export.SchemaVersion,
		Meta:          export.Meta{ExportedAt: 1756000000000, AppVersion: "test"},
		Sites:         []export.SiteRecord{{ID: 1, SiteKey: identity.Pixiv.Key, SiteName: strPtr("测试站")}},
	}
	for _, id := range fileWorkIDs {
		rel := fmt.Sprintf("store/resource/测试作者/pic_%03d.jpg", id)
		abs := filepath.Join(workDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(fmt.Sprintf("selection-%d", id)), 0o644); err != nil {
			t.Fatal(err)
		}
		storeID := int64(1000 + id)
		manifest.Works = append(manifest.Works, export.WorkRecord{
			ID: id, SiteID: i64Ptr(1), SiteWorkID: strPtr(fmt.Sprintf("%d", id)),
			SiteWorkName: strPtr(fmt.Sprintf("测试作品%d", id)),
			Resources: []export.ResourceRecord{{
				ID:           10 * id,
				ResourceType: "image",
				Stores:       []export.StoreMount{{StoreType: "image", Generation: "downloaded", StoreSeq: 1, StoreID: storeID}},
			}},
		})
		manifest.Files = append(manifest.Files, export.FileEntry{StoreID: storeID, StorePath: rel})
	}
	for _, id := range bareWorkIDs {
		manifest.Works = append(manifest.Works, export.WorkRecord{
			ID: id, SiteID: i64Ptr(1), SiteWorkID: strPtr(fmt.Sprintf("%d", id)),
			SiteWorkName: strPtr(fmt.Sprintf("测试作品%d", id)),
		})
	}
	for _, id := range workSetIDs {
		manifest.WorkSets = append(manifest.WorkSets, export.WorkSetRecord{
			ID: id, SiteID: i64Ptr(1), SiteWorkSetID: strPtr(fmt.Sprintf("ws-%d", id)),
			SiteWorkSetName: strPtr(fmt.Sprintf("测试作品集%d", id)),
		})
	}
	return export.NewExportModel(manifest)
}

// publishSelectionAndWait 与 publishAndWait 同构，但支持自定作品/作品集选择集
// （publishAndWait 固定作品 [1]）
func publishSelectionAndWait(t *testing.T, svc *Service, em *captureEmitter, workIDs, workSetIDs []int64, opts SharePublishOptions) ShareCompleteData {
	t.Helper()
	shareID, err := svc.Publish(context.Background(), workIDs, workSetIDs, opts)
	if err != nil {
		t.Fatalf("发布启动失败: %v", err)
	}
	return em.waitComplete(t, shareID, 8*time.Second)
}

// TestHostPublishFiltersSoftDeletedWorks 发布选择含软删作品（不在收集结果中）：标题作品数
// 与记录 work_ids 收窄为收集结果，与 manifest 一致
func TestHostPublishFiltersSoftDeletedWorks(t *testing.T) {
	stub := startRelayStub(t)
	repo := NewRepository(openRecordTestDB(t))
	workDir := t.TempDir()
	model := buildSelectionModel(t, workDir, []int64{1, 2}, nil, nil)
	em := newCaptureEmitter()
	svc := newRecordTestService(t, repo, stub, workDir, model, em)

	// 选择 [1, 222, 2]：222 已软删（活作品查询静默消失），1/2 可收集
	comp := publishSelectionAndWait(t, svc, em, []int64{1, 222, 2}, nil, SharePublishOptions{})
	if !comp.Success {
		t.Fatalf("发布失败: %s", comp.ErrMsg)
	}
	rec := waitRecordState(t, repo, comp.ShareID, RecordStateActive)
	workIDs, _ := unmarshalInt64s(rec.WorkIDs)
	if !slices.Equal(workIDs, []int64{1, 2}) {
		t.Fatalf("记录 work_ids 应仅含收集到的活作品（保序）: %v", workIDs)
	}
	if rec.Title != "分享 2 个作品" {
		t.Fatalf("标题应按活作品数计: %q", rec.Title)
	}
	if comp.Session == nil || comp.Session.Title != "分享 2 个作品" || comp.Session.WorkCount != 2 {
		t.Fatalf("会话快照标题/作品数应与 manifest 一致: %+v", comp.Session)
	}
}

// TestHostPublishKeepsAllCollectable 选择集全部可收集：选择集原样保留（标题/记录与选择集一致，无回归）
func TestHostPublishKeepsAllCollectable(t *testing.T) {
	stub := startRelayStub(t)
	repo := NewRepository(openRecordTestDB(t))
	workDir := t.TempDir()
	model := buildSelectionModel(t, workDir, []int64{1, 2, 3}, nil, nil)
	em := newCaptureEmitter()
	svc := newRecordTestService(t, repo, stub, workDir, model, em)

	comp := publishSelectionAndWait(t, svc, em, []int64{3, 1, 2}, nil, SharePublishOptions{})
	if !comp.Success {
		t.Fatalf("发布失败: %s", comp.ErrMsg)
	}
	rec := waitRecordState(t, repo, comp.ShareID, RecordStateActive)
	workIDs, _ := unmarshalInt64s(rec.WorkIDs)
	if !slices.Equal(workIDs, []int64{3, 1, 2}) {
		t.Fatalf("全可收集时记录 work_ids 应与选择集一致（保序）: %v", workIDs)
	}
	if rec.Title != "分享 3 个作品" {
		t.Fatalf("标题应按选择集计数: %q", rec.Title)
	}
}

// TestHostPublishWorkSetUnfiltered 作品集分享：作品集按单元收集，work_set_ids 原样保留、
// 标题按作品集计数（直接作品收窄不影响作品集口径）
func TestHostPublishWorkSetUnfiltered(t *testing.T) {
	stub := startRelayStub(t)
	repo := NewRepository(openRecordTestDB(t))
	workDir := t.TempDir()
	model := buildSelectionModel(t, workDir, []int64{1}, nil, []int64{7})
	em := newCaptureEmitter()
	svc := newRecordTestService(t, repo, stub, workDir, model, em)

	comp := publishSelectionAndWait(t, svc, em, nil, []int64{7}, SharePublishOptions{})
	if !comp.Success {
		t.Fatalf("发布失败: %s", comp.ErrMsg)
	}
	rec := waitRecordState(t, repo, comp.ShareID, RecordStateActive)
	workSetIDs, _ := unmarshalInt64s(rec.WorkSetIDs)
	if !slices.Equal(workSetIDs, []int64{7}) {
		t.Fatalf("记录 work_set_ids 应原样保留: %v", workSetIDs)
	}
	if rec.Title != "分享 1 个作品集" {
		t.Fatalf("标题应按作品集计数: %q", rec.Title)
	}
}

// TestHostPublishAliveNoStoreWorkCounted 活但无活 store 的作品：身份在收集结果（Works）中，
// 仍计入标题与记录 work_ids（贡献 0 文件属规划口径，不属选择集收窄）
func TestHostPublishAliveNoStoreWorkCounted(t *testing.T) {
	stub := startRelayStub(t)
	repo := NewRepository(openRecordTestDB(t))
	workDir := t.TempDir()
	model := buildSelectionModel(t, workDir, nil, []int64{5}, nil)
	em := newCaptureEmitter()
	svc := newRecordTestService(t, repo, stub, workDir, model, em)

	comp := publishSelectionAndWait(t, svc, em, []int64{5}, nil, SharePublishOptions{})
	if !comp.Success {
		t.Fatalf("发布失败: %s", comp.ErrMsg)
	}
	rec := waitRecordState(t, repo, comp.ShareID, RecordStateActive)
	workIDs, _ := unmarshalInt64s(rec.WorkIDs)
	if !slices.Equal(workIDs, []int64{5}) {
		t.Fatalf("活作品应保留在记录 work_ids 中: %v", workIDs)
	}
	if rec.Title != "分享 1 个作品" {
		t.Fatalf("活但无活 store 作品应计入标题: %q", rec.Title)
	}
	if rec.FileCount != 0 {
		t.Fatalf("无活 store 作品贡献 0 文件: %d", rec.FileCount)
	}
}

// TestHostRestoreStillRejectsMissing 复原路径：原记录清单中的对象在重新收集结果中缺失仍拒绝
// ——发布选择集收窄仅作用于发布形态，复原按原记录清单忠实比对
func TestHostRestoreStillRejectsMissing(t *testing.T) {
	stub := startRelayStub(t)
	repo := NewRepository(openRecordTestDB(t))
	workDir := t.TempDir()

	// 直造 active 记录：清单含作品 1/2，重新收集结果仅剩作品 1（2 已软删）
	key := make([]byte, shareKeyLen)
	rec := entity.NewShareRecord()
	rec.ShareID = "share-9101"
	rec.Token = fmt.Sprintf("stubtoken%013d", 9101)
	rec.Title = "含缺失对象"
	rec.WorkIDs = marshalInt64s([]int64{1, 2})
	rec.RelayAddress = stub.addr
	rec.KeyB64 = base64.RawURLEncoding.EncodeToString(key)
	rec.ExpireSeconds = -1
	rec.State = RecordStateActive
	if err := repo.Create(context.Background(), rec); err != nil {
		t.Fatal(err)
	}

	model := buildSelectionModel(t, workDir, []int64{1}, nil, nil)
	em := newCaptureEmitter()
	svc := NewService(repo, &fakeCollector{model: model}, export.NewPacker(),
		func() string { return stub.addr }, func() string { return workDir },
		"test-instance-0001", em, nil, nil)
	svc.RestoreAll(context.Background())
	rec2 := waitRecordState(t, repo, "share-9101", RecordStateFailed)
	if !strings.Contains(rec2.ErrMsg, "分享对象已删除") {
		t.Fatalf("失败原因应含分享对象删除语义: %q", rec2.ErrMsg)
	}
	if !strings.Contains(rec2.ErrMsg, "作品#2") {
		t.Fatalf("失败原因应点名缺失对象: %q", rec2.ErrMsg)
	}
}

// —— 启动自动复原 ——

// TestRestoreAllBind 复原主链路：重启（新 Service 实例）→ active 记录原 token bind 重绑 →
// 会话在线、记录保持 active、链接不变且可拉取
func TestRestoreAllBind(t *testing.T) {
	stub := startRelayStub(t)
	repo := NewRepository(openRecordTestDB(t))
	workDir := t.TempDir()
	model, _ := buildTestModel(t, workDir)
	em1 := newCaptureEmitter()
	svc1 := newRecordTestService(t, repo, stub, workDir, model, em1)

	shareID, comp := publishAndWait(t, svc1, em1, SharePublishOptions{Title: "复原分享"})
	if !comp.Success {
		t.Fatalf("发布失败: %s", comp.ErrMsg)
	}
	token := comp.Session.Token
	link := comp.Link
	// 模拟关闭：终止宿主主体（会话本地移除，记录保持 active）
	svc1.CancelPublish(context.Background(), shareID)

	// 模拟重启：全新 Service（进程内会话清空），同库同桩中继 → 启动自动复原
	em2 := newCaptureEmitter()
	svc2 := newRecordTestService(t, repo, stub, workDir, model, em2)
	svc2.RestoreAll(context.Background())

	em2.waitState(t, shareID, stateOnline, 8*time.Second)
	if stub.bindCountOf(token) < 1 {
		t.Fatal("复原未以原 token bind 重绑")
	}
	waitRecordState(t, repo, shareID, RecordStateActive)
	// 链接不变：记录重建链接与原发布一致，且可经原链接拉取 manifest
	rec, err := repo.GetByShareID(context.Background(), shareID)
	if err != nil || rec == nil {
		t.Fatalf("查询复原记录失败: %v %v", rec, err)
	}
	if dto := toShareRecordDTO(rec); dto.Link != link {
		t.Fatalf("复原后链接应不变:\n%s\n%s", dto.Link, link)
	}
	key, _ := keyFromLink(t, link)
	cip, err := newE2ECipher(key)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := recipientDial(t, stub.addr, token, "")
	if err != nil {
		t.Fatalf("复原后收件拨号失败: %v", err)
	}
	head, _, _, err := recipientFetch(t, cip, conn, streamRequest{Type: "manifest"})
	if err != nil || !head.OK || head.Kind != "manifest" {
		t.Fatalf("复原后拉取失败: head=%+v err=%v", head, err)
	}
	_ = conn.Close()
}

// waitHostExit 轮询等待 shareID 宿主主体退出（hostCancels 条目摘除）
func waitHostExit(t *testing.T, svc *Service, shareID string) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		_, inFlight := svc.hostCancels[shareID]
		svc.mu.Unlock()
		if !inFlight {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("宿主主体未退出 shareId=%s", shareID)
}

// waitBindCount 轮询等待 token 的 bind 次数达到 want。同实例复用场景 emitter 状态历史含
// 发布期 online 快照，不能作「复原已在线」信号，以中继侧 bind 计数为准。
func waitBindCount(t *testing.T, stub *relayStub, token string, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for stub.bindCountOf(token) < want && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if stub.bindCountOf(token) < want {
		t.Fatalf("等待 bind 次数达 %d 超时（当前 %d）", want, stub.bindCountOf(token))
	}
}

// TestRestoreAllSkipsInflightHost 复原入口在驻检测：主体在线在驻时重复 RestoreAll 不再拨号。
// 前端 reload 令 onDomReady 二次触发复原入口，若无在驻守卫会以同 token 再次 bind 顶替在驻
// 隧道，两会话互踢进入秒级重连热循环（吃满中继拨号限流）。
func TestRestoreAllSkipsInflightHost(t *testing.T) {
	stub := startRelayStub(t)
	repo := NewRepository(openRecordTestDB(t))
	workDir := t.TempDir()
	model, _ := buildTestModel(t, workDir)
	em := newCaptureEmitter()
	svc := newRecordTestService(t, repo, stub, workDir, model, em)

	shareID, comp := publishAndWait(t, svc, em, SharePublishOptions{Title: "在驻复原"})
	if !comp.Success {
		t.Fatalf("发布失败: %s", comp.ErrMsg)
	}
	token := comp.Session.Token
	// 首轮复原：发布主体退出（记录保持 active）后 RestoreAll 拉起复原主体至在线
	svc.CancelPublish(context.Background(), shareID)
	waitHostExit(t, svc, shareID)
	svc.RestoreAll(context.Background())
	waitBindCount(t, stub, token, 1, 8*time.Second)
	bindCount := stub.bindCountOf(token)

	// 重复复原（模拟 reload 二次触发复原入口）：主体在驻，不重复拨号
	svc.RestoreAll(context.Background())
	svc.RestoreAll(context.Background())
	time.Sleep(1500 * time.Millisecond) // 覆盖最小重连退避窗口：无守卫时第二主体此时必已 bind
	if got := stub.bindCountOf(token); got != bindCount {
		t.Fatalf("在驻期间重复复原不应再拨号: bind %d -> %d", bindCount, got)
	}
}

// TestRestoreAllAgainAfterHostExit 主体退出后在驻标记解除：取消复原主体（记录保持 active）
// 后再次 RestoreAll 可重新拨号复原——在驻守卫不得把分享永久锁死在跳过。
func TestRestoreAllAgainAfterHostExit(t *testing.T) {
	stub := startRelayStub(t)
	repo := NewRepository(openRecordTestDB(t))
	workDir := t.TempDir()
	model, _ := buildTestModel(t, workDir)
	em := newCaptureEmitter()
	svc := newRecordTestService(t, repo, stub, workDir, model, em)

	shareID, comp := publishAndWait(t, svc, em, SharePublishOptions{Title: "退出后再复原"})
	if !comp.Success {
		t.Fatalf("发布失败: %s", comp.ErrMsg)
	}
	token := comp.Session.Token
	// 首轮复原在线
	svc.CancelPublish(context.Background(), shareID)
	waitHostExit(t, svc, shareID)
	svc.RestoreAll(context.Background())
	waitBindCount(t, stub, token, 1, 8*time.Second)
	firstBinds := stub.bindCountOf(token)

	// 复原主体退出（记录保持 active）→ 在驻解除，再次复原可重新拨号
	svc.CancelPublish(context.Background(), shareID)
	waitHostExit(t, svc, shareID)
	svc.RestoreAll(context.Background())
	waitBindCount(t, stub, token, firstBinds+1, 8*time.Second)
	waitRecordState(t, repo, shareID, RecordStateActive)
}

// TestRestoreWorkDeleted 复原时作品已删（收集失败）→ 记录落 failed 记原因
func TestRestoreWorkDeleted(t *testing.T) {
	stub := startRelayStub(t)
	repo := NewRepository(openRecordTestDB(t))
	workDir := t.TempDir()
	model, _ := buildTestModel(t, workDir)
	em := newCaptureEmitter()
	svc1 := newRecordTestService(t, repo, stub, workDir, model, em)

	shareID, comp := publishAndWait(t, svc1, em, SharePublishOptions{})
	if !comp.Success {
		t.Fatalf("发布失败: %s", comp.ErrMsg)
	}
	svc1.CancelPublish(context.Background(), shareID)

	// 重启后作品已删：重新收集失败 → failed 记原因
	em2 := newCaptureEmitter()
	svc2 := NewService(repo, &failingCollector{err: errors.New("作品已删除")}, export.NewPacker(),
		func() string { return stub.addr }, func() string { return workDir },
		"test-instance-0001", em2, nil, nil)
	svc2.RestoreAll(context.Background())
	rec := waitRecordState(t, repo, shareID, RecordStateFailed)
	if !strings.Contains(rec.ErrMsg, "作品已删除") {
		t.Fatalf("失败原因应含收集错误: %q", rec.ErrMsg)
	}
}

// TestRestoreWorkSilentMissing 重启后作品软删：活作品查询令其静默消失（Collect 不报错），
// 复原须比对清单发现缺失并落 failed——「收集成功但内容残缺」不可复活会话
func TestRestoreWorkSilentMissing(t *testing.T) {
	stub := startRelayStub(t)
	repo := NewRepository(openRecordTestDB(t))
	workDir := t.TempDir()
	model, _ := buildTestModel(t, workDir)
	em := newCaptureEmitter()
	svc1 := newRecordTestService(t, repo, stub, workDir, model, em)

	shareID, comp := publishAndWait(t, svc1, em, SharePublishOptions{})
	if !comp.Success {
		t.Fatalf("发布失败: %s", comp.ErrMsg)
	}
	svc1.CancelPublish(context.Background(), shareID)

	// 重启：收集成功但清单缺作品（软删作品的活行查询形态）
	emptyModel := export.NewExportModel(&export.Manifest{SchemaVersion: export.SchemaVersion})
	em2 := newCaptureEmitter()
	svc2 := NewService(repo, &fakeCollector{model: emptyModel}, export.NewPacker(),
		func() string { return stub.addr }, func() string { return workDir },
		"test-instance-0001", em2, nil, nil)
	svc2.RestoreAll(context.Background())
	rec := waitRecordState(t, repo, shareID, RecordStateFailed)
	if !strings.Contains(rec.ErrMsg, "分享对象已删除") {
		t.Fatalf("失败原因应含分享对象删除语义: %q", rec.ErrMsg)
	}
	if !strings.Contains(rec.ErrMsg, "作品#1") {
		t.Fatalf("失败原因应点名缺失对象: %q", rec.ErrMsg)
	}
}

// TestRestoreRelayNotFound 中继侧会话不存在（bind 被 not_found 拒）→ 记录落 failed 记原因
func TestRestoreRelayNotFound(t *testing.T) {
	stub := startRelayStub(t)
	repo := NewRepository(openRecordTestDB(t))
	workDir := t.TempDir()
	model, _ := buildTestModel(t, workDir)

	// 直造 active 记录行（token 从未注册到中继——模拟中继会话已被清理）
	key := make([]byte, shareKeyLen)
	rec := entity.NewShareRecord()
	rec.ShareID = "share-9001"
	rec.Token = fmt.Sprintf("stubtoken%013d", 9001)
	rec.Title = "幽灵分享"
	rec.WorkIDs = marshalInt64s([]int64{1})
	rec.RelayAddress = stub.addr
	rec.KeyB64 = base64.RawURLEncoding.EncodeToString(key)
	rec.ExpireSeconds = -1
	rec.State = RecordStateActive
	if err := repo.Create(context.Background(), rec); err != nil {
		t.Fatal(err)
	}

	em := newCaptureEmitter()
	svc := newRecordTestService(t, repo, stub, workDir, model, em)
	svc.RestoreAll(context.Background())
	em.waitState(t, "share-9001", stateFailed, 8*time.Second)
	rec2 := waitRecordState(t, repo, "share-9001", RecordStateFailed)
	if !strings.Contains(rec2.ErrMsg, errCodeNotFound) {
		t.Fatalf("失败原因应含中继错误码 %s: %q", errCodeNotFound, rec2.ErrMsg)
	}
}

// TestRestoreRelayUnreachable 中继不可达：会话进入重连循环（reconnecting 运行态不入表），
// 记录保持 active、主体终止不落终态
func TestRestoreRelayUnreachable(t *testing.T) {
	repo := NewRepository(openRecordTestDB(t))
	workDir := t.TempDir()
	model, _ := buildTestModel(t, workDir)

	// 直造记录，中继地址指向本机不可达端口（连接即拒，重连退避快速循环）
	key := make([]byte, shareKeyLen)
	rec := entity.NewShareRecord()
	rec.ShareID = "share-9002"
	rec.Token = fmt.Sprintf("stubtoken%013d", 9002)
	rec.Title = "断连分享"
	rec.WorkIDs = marshalInt64s([]int64{1})
	rec.RelayAddress = "127.0.0.1:1"
	rec.KeyB64 = base64.RawURLEncoding.EncodeToString(key)
	rec.ExpireSeconds = -1
	rec.State = RecordStateActive
	if err := repo.Create(context.Background(), rec); err != nil {
		t.Fatal(err)
	}

	em := newCaptureEmitter()
	svc := NewService(repo, &fakeCollector{model: model}, export.NewPacker(),
		func() string { return "127.0.0.1:1" }, func() string { return workDir },
		"test-instance-0001", em, nil, nil)
	svc.RestoreAll(context.Background())
	em.waitState(t, "share-9002", stateReconnecting, 8*time.Second)
	waitRecordState(t, repo, "share-9002", RecordStateActive)
	// 主体终止（模拟关闭）：不落终态，记录保持 active
	svc.CancelPublish(context.Background(), "share-9002")
	time.Sleep(300 * time.Millisecond)
	waitRecordState(t, repo, "share-9002", RecordStateActive)
}

// TestMetaPayloadWorksName 设计十：register 帧 meta 携带 worksName（顺序对齐 manifest.Works、
// site_work_name 优先、次 nick_name、全空名以「作品 {ID}」占位）；bind 帧不携带；空清单不出现该键。
func TestMetaPayloadWorksName(t *testing.T) {
	stub := startRelayStub(t)
	workDir := t.TempDir()
	model, _ := buildTestModel(t, workDir)
	// 扩展清单：ID=2 仅 nick_name、ID=3 全空名——覆盖优先级回落与空名占位
	model.Manifest.Works = append(model.Manifest.Works,
		export.WorkRecord{ID: 2, SiteID: i64Ptr(1), SiteWorkID: strPtr("1002"), NickName: strPtr("昵称作品1002")},
		export.WorkRecord{ID: 3, SiteID: i64Ptr(1), SiteWorkID: strPtr("1003")},
	)

	em := newCaptureEmitter()
	svc := newTestService(t, stub, workDir, model, em, nil, nil)

	shareID, comp := publishAndWait(t, svc, em, SharePublishOptions{Title: "作品名元数据分享", Password: "pw"})
	if !comp.Success {
		t.Fatalf("发布失败: %s", comp.ErrMsg)
	}
	token := comp.Session.Token

	// ① register 帧 meta 含 worksName：顺序对齐 manifest.Works、优先级与占位
	reg := stub.sessionOf(token).registerHELLO
	if reg.Meta == nil {
		t.Fatalf("注册 HELLO 元数据缺失")
	}
	want := []string{"测试作品1001", "昵称作品1002", "作品 3"}
	if !slices.Equal(reg.Meta.WorksName, want) {
		t.Fatalf("worksName 不符: got %#v, want %#v", reg.Meta.WorksName, want)
	}

	// ② bind 帧不含 meta（复原重绑不携带元数据，中继侧已存）
	sess := svc.sessions[shareID]
	sess.mu.Lock()
	cur := sess.token
	sess.mu.Unlock()
	if cur == "" {
		t.Fatalf("发布后会话应已持有 token")
	}
	hb := sess.buildHello()
	if hb.Action != "bind" {
		t.Fatalf("持有 token 应走 bind 分支: %s", hb.Action)
	}
	if hb.Meta != nil {
		t.Fatalf("bind 帧不应携带 meta: %+v", hb.Meta)
	}

	// ③ 空作品清单 register：worksName 键不出现（omitempty）
	emptyModel := export.NewExportModel(&export.Manifest{
		SchemaVersion: export.SchemaVersion,
		Sites:         []export.SiteRecord{{ID: 1, SiteKey: identity.Pixiv.Key, SiteName: strPtr("测试站")}},
		Works:         []export.WorkRecord{},
	})
	key, err := GenerateShareKey()
	if err != nil {
		t.Fatal(err)
	}
	es, err := newShareSession(sessionConfig{
		id:         "empty-share",
		title:      "空分享",
		instanceID: "test-instance-0001",
		relayDial:  stub.addr,
		relayHost:  "localhost",
		workDir:    workDir,
		key:        key,
		model:      emptyModel,
	})
	if err != nil {
		t.Fatal(err)
	}
	eh := es.buildHello()
	if eh.Meta == nil {
		t.Fatalf("空分享 register 仍应有 meta")
	}
	raw, err := json.Marshal(eh)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("worksName")) {
		t.Fatalf("空分享 register 帧不应含 worksName 键: %s", raw)
	}
}

// TestMetaPayloadWorksNameSanitized 跨仓契约修复（阶段3）：metaWorksName 每个作品名须净化——
// 剔除控制字符、截断 200 rune（relay 侧 worksName 单名校验 ≤200 rune 且禁控制字符，不净化会被
// 中继以 malformed 拒绝）。与收件侧子任务命名、Receive 返回 workNames 共用 sanitizedWorkName。
func TestMetaPayloadWorksNameSanitized(t *testing.T) {
	stub := startRelayStub(t)
	workDir := t.TempDir()
	model, _ := buildTestModel(t, workDir)
	// 追加：含换行/制表/回车控制字符的作品名 + 超长（>200 rune）作品名
	long := strings.Repeat("长", 250)
	model.Manifest.Works = append(model.Manifest.Works,
		export.WorkRecord{ID: 2, SiteID: i64Ptr(1), SiteWorkID: strPtr("1002"), SiteWorkName: strPtr("作品\n带换行\t和制表\r控制")},
		export.WorkRecord{ID: 3, SiteID: i64Ptr(1), SiteWorkID: strPtr("1003"), SiteWorkName: strPtr(long)},
	)

	em := newCaptureEmitter()
	svc := newTestService(t, stub, workDir, model, em, nil, nil)
	_, comp := publishAndWait(t, svc, em, SharePublishOptions{Title: "作品名净化分享"})
	if !comp.Success {
		t.Fatalf("发布失败: %s", comp.ErrMsg)
	}
	reg := stub.sessionOf(comp.Session.Token).registerHELLO
	if reg.Meta == nil {
		t.Fatalf("注册 HELLO 元数据缺失")
	}
	if len(reg.Meta.WorksName) != 3 {
		t.Fatalf("worksName 数量不符: got %d, want 3", len(reg.Meta.WorksName))
	}
	// 控制字符（\n\t\r）被剔除
	if reg.Meta.WorksName[1] != "作品带换行和制表控制" {
		t.Fatalf("控制字符未净化: %q", reg.Meta.WorksName[1])
	}
	// 超长截断到 200 rune
	if got := len([]rune(reg.Meta.WorksName[2])); got != 200 {
		t.Fatalf("超长作品名未截断: %d rune", got)
	}
	if want := string([]rune(long)[:200]); reg.Meta.WorksName[2] != want {
		t.Fatalf("截断内容不符: got %q, want %q", reg.Meta.WorksName[2], want)
	}
}
