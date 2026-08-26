package share

// 分享发布端到端测试：分享方客户端 × 中继桩（PROTOCOL.md 最小实现）。
// 覆盖：E2E 加密硬验收（决策14 ①②③）、拉取应答（manifest/分块文件/offset）、
// 路径白名单拒绝、撤销、注册被拒、保活 PING-PONG、断线重连 bind、发布取消。

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/export"
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
	big := make([]byte, 40*1024+123)
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
		Sites:         []export.SiteRecord{{ID: 1, SiteName: strPtr("测试站")}},
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

// newTestService 组装面向桩中继的分享服务（planner 用生产 Packer，收集用桩模型）
func newTestService(t *testing.T, stub *relayStub, workDir string, model *export.ExportModel,
	em *captureEmitter, dialer *recordingDialer) *Service {
	svc := NewService(&fakeCollector{model: model}, export.NewPacker(),
		func() string { return stub.addr }, func() string { return workDir },
		"test-instance-0001", em)
	opts := sessionRuntimeOptions{streamRate: 8 << 20}
	if dialer != nil {
		opts.dialFn = dialer.dial
	}
	svc.setTunables(opts)
	// 测试收尾清理：撤销全部会话，终止客户端重连循环
	t.Cleanup(func() {
		for _, d := range svc.Sessions(context.Background()) {
			_ = svc.Revoke(context.Background(), d.ShareID)
		}
	})
	return svc
}

// publishAndWait 发布并等待完成事件
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
	svc := newTestService(t, stub, workDir, model, em, dialer)

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
	svc := newTestService(t, stub, workDir, model, em, nil)

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
	// 分块断言：内容 + 头记录数须超过单块（chunk 16KiB，内容 ~40KiB）
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

// TestPathWhitelistDenied 路径白名单拒绝：白名单外路径/缺失文件/非法请求/密文损坏一律拒绝
func TestPathWhitelistDenied(t *testing.T) {
	stub := startRelayStub(t)
	workDir := t.TempDir()
	model, _ := buildTestModel(t, workDir)
	em := newCaptureEmitter()
	svc := newTestService(t, stub, workDir, model, em, nil)

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
	svc := newTestService(t, stub, workDir, model, em, nil)

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
	svc := newTestService(t, stub, workDir, model, em, nil)

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
	svc := newTestService(t, stub, workDir, model, em, nil)

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
	svc := newTestService(t, stub, workDir, model, em, nil)

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

// TestCancelPublish 取消进行中的发布：complete 推送「已取消」
func TestCancelPublish(t *testing.T) {
	stub := startRelayStub(t)
	workDir := t.TempDir()
	model, _ := buildTestModel(t, workDir)
	em := newCaptureEmitter()
	svc := NewService(&blockingCollector{model: model, started: make(chan struct{})}, export.NewPacker(),
		func() string { return stub.addr }, func() string { return workDir },
		"test-instance-0001", em)
	svc.setTunables(sessionRuntimeOptions{streamRate: 8 << 20})

	shareID, err := svc.Publish(context.Background(), []int64{1}, nil, SharePublishOptions{})
	if err != nil {
		t.Fatal(err)
	}
	svc.CancelPublish(shareID)
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
