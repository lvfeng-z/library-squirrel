package share

// TLS 拨号全链测试：自签证书中继桩（startRelayStubTLS）下发布/复原/收件拉取按端点
// 传输形态走 TLS。四类形态断言的落点：
//   - ①回环/私网/localhost 豁免走明文 —— TestTLSPublishTransportPerAddressForm
//   - ②http:// 前缀逃生口走明文     —— TestTLSPublishTransportPerAddressForm
//   - ③裸公网域名缺省走 TLS          —— TestTLSPublishTransportPerAddressForm /
//     TestTLSPublishRegisterRecipientPull / TestTLSRestoreBind /
//     TestTLSReceiveExecutionEndToEnd（receive_test.go，含导入回灌）
//   - ④TLS 证书校验失败文案与连接被拒可区分 —— TestTLSHandshakeFailureWording
//
// 测试对自签桩证书的信任仅经注入拨号器实现（redirectingDialer 携带桩证书信任池）；
// 生产路径用系统根证书严格校验、无跳过选项。

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/library-squirrel/backend/export"
)

// tlsRelayTestHost 假公网域名（.test 保留 TLD，不解析）：裸书写即公网缺省 TLS 443，
// 实际拨号经 redirectingDialer 重定向到本机 TLS 桩
const tlsRelayTestHost = "relay.example-share.test"

// tlsRelayFixture TLS 桩夹具：自签证书 + TLS 监听桩 + 配套信任池。证书 SAN 含
// 假公网域名（裸域名/显式 https 书写的 SNI 匹配）与 127.0.0.1（显式 https:// 回环
// 书写的 IP SAN 匹配）
type tlsRelayFixture struct {
	stub *relayStub
	cert tls.Certificate
	root *x509.CertPool // 含桩证书——注入拨号器/测试直拨的信任池
}

func startTLSRelayFixture(t *testing.T) *tlsRelayFixture {
	t.Helper()
	cert := selfSignRelayCert(t, []string{tlsRelayTestHost}, []net.IP{net.ParseIP("127.0.0.1")})
	stub := startRelayStubTLS(t, cert)
	root := x509.NewCertPool()
	root.AddCert(cert.Leaf)
	return &tlsRelayFixture{stub: stub, cert: cert, root: root}
}

// recipientDialTLS 收件人经 TLS 拨号入桩（recipientDial 的 TLS 形态：HELLO recipient →
// WELCOME；信任桩证书、ServerName 取假公网域名）
func (f *tlsRelayFixture) recipientDialTLS(t *testing.T, token, passwordHash string) (net.Conn, error) {
	t.Helper()
	conn, err := (&tls.Dialer{
		NetDialer: &net.Dialer{Timeout: 5 * time.Second},
		Config:    &tls.Config{ServerName: tlsRelayTestHost, RootCAs: f.root},
	}).Dial("tcp", f.stub.addr)
	if err != nil {
		return nil, err
	}
	h := helloPayload{Role: "recipient", Token: token, InstanceID: "recipient-tls-test-device", PasswordHash: passwordHash}
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

// redirectingDialer 端点记录 + 地址重定向拨号器：拨号目标一律重写为桩地址（桩监听
// 127.0.0.1 随机端口，假域名/私网/回环书写形态经它落到桩），按端点 TLS 形态选择传输，
// TLS 的 ServerName 取端点 host（桩证书按该 host 匹配）。记录全部拨号端点供传输形态
// 断言；tlsRoot=nil 服务明文桩——端点被误判为 TLS 时明文桩无法完成握手，拨号即失败。
type redirectingDialer struct {
	mu      sync.Mutex
	eps     []relayEndpoint
	target  string         // 桩实际地址（重定向目标）
	tlsRoot *x509.CertPool // TLS 桩证书信任池；nil=明文桩
}

func (d *redirectingDialer) dial(ep relayEndpoint) (net.Conn, error) {
	d.mu.Lock()
	d.eps = append(d.eps, ep)
	d.mu.Unlock()
	if !ep.TLS {
		return net.DialTimeout("tcp", d.target, 15*time.Second)
	}
	host, _, err := net.SplitHostPort(ep.Addr)
	if err != nil {
		return nil, fmt.Errorf("测试拨号端点地址非法: %w", err)
	}
	return (&tls.Dialer{
		NetDialer: &net.Dialer{Timeout: 15 * time.Second},
		Config:    &tls.Config{ServerName: host, RootCAs: d.tlsRoot},
	}).Dial("tcp", d.target)
}

// endpoints 已记录拨号端点快照
func (d *redirectingDialer) endpoints() []relayEndpoint {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]relayEndpoint(nil), d.eps...)
}

// newTLSService 组装面向重定向桩的分享服务：中继地址 getter 返回指定书写形态，
// 拨号经 redirectingDialer（repo 非空即落记录，复原类测试用）；测试收尾撤销全部会话
func newTLSService(t *testing.T, repo *Repository, relayAddr string, workDir string,
	model *export.ExportModel, em *captureEmitter, dialer *redirectingDialer) *Service {
	svc := NewService(repo, &fakeCollector{model: model}, export.NewPacker(),
		func() string { return relayAddr }, func() string { return workDir },
		"test-instance-0001", em, nil, nil)
	svc.setTunables(sessionRuntimeOptions{
		dialFn:     dialer.dial,
		streamRate: 8 << 20,
	})
	t.Cleanup(func() {
		for _, d := range svc.Sessions(context.Background()) {
			_ = svc.Revoke(context.Background(), d.ShareID)
		}
	})
	return svc
}

// TestTLSPublishTransportPerAddressForm 地址书写形态 → 发布侧拨号传输形态（判定表全行）：
// 每种书写形态独立起桩（按期望传输形态选明文/TLS 桩）并完整发布注册，断言拨号端点的
// 传输形态与拨号地址；形态误判时拨号打到异形桩直接失败（发布失败）暴露。
// 模板中 "%s" 为桩端口占位（无端口书写不占位，端口取缺省）。
func TestTLSPublishTransportPerAddressForm(t *testing.T) {
	cases := []struct {
		name     string
		write    string // 中继地址书写形态模板
		wantTLS  bool   // 期望拨号传输形态
		wantAddr string // 期望拨号地址模板（缺省端口按传输形态补齐）
	}{
		// ①回环/RFC1918 私网/localhost 豁免：缺省明文 9527，显式端口明文拨该端口
		{"LoopbackDefaultPort", "127.0.0.1", false, "127.0.0.1:9527"},
		{"LoopbackExplicitPort", "127.0.0.1:%s", false, "127.0.0.1:%s"},
		{"LocalhostExempt", "localhost:%s", false, "localhost:%s"},
		{"PrivateExempt", "192.168.1.100:%s", false, "192.168.1.100:%s"},
		// ②http:// 前缀逃生口：显式明文（回环与公网域名书写均明文拨显式端口）
		{"HTTPExplicitLoopback", "http://127.0.0.1:%s", false, "127.0.0.1:%s"},
		{"HTTPExplicitPublicHost", "http://" + tlsRelayTestHost + ":%s", false, tlsRelayTestHost + ":%s"},
		// ③裸公网域名缺省 TLS 443；显式 https 前缀等价
		{"BarePublicHostDefaultTLS", tlsRelayTestHost, true, tlsRelayTestHost + ":443"},
		{"HTTPSExplicitPublicHost", "https://" + tlsRelayTestHost + ":%s", true, tlsRelayTestHost + ":%s"},
		// 显式 https 前缀覆盖回环豁免（证书含 IP SAN）
		{"HTTPSOverrideLoopback", "https://127.0.0.1:%s", true, "127.0.0.1:%s"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stub *relayStub
			var root *x509.CertPool
			if tc.wantTLS {
				f := startTLSRelayFixture(t)
				stub, root = f.stub, f.root
			} else {
				stub = startRelayStub(t)
			}
			_, portStr, err := net.SplitHostPort(stub.addr)
			if err != nil {
				t.Fatalf("桩地址非法: %v", err)
			}
			write := strings.ReplaceAll(tc.write, "%s", portStr)
			wantAddr := strings.ReplaceAll(tc.wantAddr, "%s", portStr)

			workDir := t.TempDir()
			model, _ := buildTestModel(t, workDir)
			em := newCaptureEmitter()
			dialer := &redirectingDialer{target: stub.addr, tlsRoot: root}
			svc := newTLSService(t, nil, write, workDir, model, em, dialer)

			_, comp := publishAndWait(t, svc, em, SharePublishOptions{Title: "TLS 形态 " + tc.name})
			if !comp.Success {
				t.Fatalf("发布失败: %s", comp.ErrMsg)
			}
			eps := dialer.endpoints()
			if len(eps) == 0 {
				t.Fatal("发布应至少产生一次拨号，实际无记录")
			}
			for _, ep := range eps {
				if ep.TLS != tc.wantTLS {
					t.Fatalf("拨号端点传输形态不符: %s TLS=%v，期望 TLS=%v", ep.Addr, ep.TLS, tc.wantTLS)
				}
				if ep.Addr != wantAddr {
					t.Fatalf("拨号地址不符: %s，期望 %s", ep.Addr, wantAddr)
				}
			}
		})
	}
}

// TestTLSPublishRegisterRecipientPull TLS 桩下的发布→注册→收件拉取最短全链：
// 裸公网域名发布注册走 TLS、链接为 https 域名形态，收件人经 TLS 拨号拉 manifest 成功
func TestTLSPublishRegisterRecipientPull(t *testing.T) {
	f := startTLSRelayFixture(t)
	workDir := t.TempDir()
	model, _ := buildTestModel(t, workDir)
	em := newCaptureEmitter()
	dialer := &redirectingDialer{target: f.stub.addr, tlsRoot: f.root}
	svc := newTLSService(t, nil, tlsRelayTestHost, workDir, model, em, dialer)

	_, comp := publishAndWait(t, svc, em, SharePublishOptions{Title: "TLS 全链分享"})
	if !comp.Success {
		t.Fatalf("发布失败: %s", comp.ErrMsg)
	}
	for _, ep := range dialer.endpoints() {
		if !ep.TLS {
			t.Fatalf("裸公网域名发布拨号应全为 TLS: %+v", ep)
		}
	}
	wantPrefix := "https://" + tlsRelayTestHost + "/s/"
	if !strings.HasPrefix(comp.Link, wantPrefix) {
		t.Fatalf("链接应为 https 域名形态（前缀 %s）: %s", wantPrefix, comp.Link)
	}

	key, _ := keyFromLink(t, comp.Link)
	cip, err := newE2ECipher(key)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := f.recipientDialTLS(t, comp.Session.Token, "")
	if err != nil {
		t.Fatalf("TLS 收件拨号失败: %v", err)
	}
	head, _, _, err := recipientFetch(t, cip, conn, streamRequest{Type: "manifest"})
	if err != nil || !head.OK || head.Kind != "manifest" {
		t.Fatalf("TLS 拉取 manifest 失败: head=%+v err=%v", head, err)
	}
	_ = conn.Close()
}

// TestTLSRestoreBind TLS 桩下的分享记录复原：发布（register 走 TLS）→ 主体退出 →
// 全新服务 RestoreAll 以原 token bind 重绑（仍走 TLS）→ 原链接经 TLS 收件拨号拉取成功
func TestTLSRestoreBind(t *testing.T) {
	f := startTLSRelayFixture(t)
	repo := NewRepository(openRecordTestDB(t))
	workDir := t.TempDir()
	model, _ := buildTestModel(t, workDir)
	em1 := newCaptureEmitter()
	svc1 := newTLSService(t, repo, tlsRelayTestHost, workDir, model, em1,
		&redirectingDialer{target: f.stub.addr, tlsRoot: f.root})

	shareID, comp := publishAndWait(t, svc1, em1, SharePublishOptions{Title: "TLS 复原分享"})
	if !comp.Success {
		t.Fatalf("发布失败: %s", comp.ErrMsg)
	}
	token := comp.Session.Token
	link := comp.Link
	svc1.CancelPublish(context.Background(), shareID)

	// 模拟重启：全新服务（同库同 TLS 桩）→ 启动自动复原
	em2 := newCaptureEmitter()
	dialer2 := &redirectingDialer{target: f.stub.addr, tlsRoot: f.root}
	svc2 := newTLSService(t, repo, tlsRelayTestHost, workDir, model, em2, dialer2)
	svc2.RestoreAll(context.Background())

	em2.waitState(t, shareID, stateOnline, 8*time.Second)
	if f.stub.bindCountOf(token) < 1 {
		t.Fatal("复原未以原 token bind 重绑")
	}
	waitRecordState(t, repo, shareID, RecordStateActive)
	for _, ep := range dialer2.endpoints() {
		if !ep.TLS {
			t.Fatalf("复原 bind 拨号应走 TLS: %+v", ep)
		}
	}

	key, _ := keyFromLink(t, link)
	cip, err := newE2ECipher(key)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := f.recipientDialTLS(t, token, "")
	if err != nil {
		t.Fatalf("复原后 TLS 收件拨号失败: %v", err)
	}
	head, _, _, err := recipientFetch(t, cip, conn, streamRequest{Type: "manifest"})
	if err != nil || !head.OK || head.Kind != "manifest" {
		t.Fatalf("复原后 TLS 拉取失败: head=%+v err=%v", head, err)
	}
	_ = conn.Close()
}

// TestTLSHandshakeFailureWording 默认拨号器（dialRelayEndpoint，系统根证书）的 TLS
// 失败文案：对自签 TLS 桩报「中继 TLS 证书校验失败」；连接被拒（无监听端口）不含该
// 文案——两类错误可区分（边界条件：TLS 握手失败须指明是校验失败）。
func TestTLSHandshakeFailureWording(t *testing.T) {
	f := startTLSRelayFixture(t)

	// 自签证书不在系统根内：显式 https:// 前缀覆盖回环豁免得 TLS 端点，直拨本机 TLS 桩
	ep, _, err := normalizeRelayAddress("https://" + f.stub.addr)
	if err != nil {
		t.Fatalf("地址规范化失败: %v", err)
	}
	if !ep.TLS {
		t.Fatal("显式 https 前缀应覆盖回环豁免得 TLS 端点")
	}
	conn, err := dialRelayEndpoint(ep)
	if err == nil {
		_ = conn.Close()
		t.Fatal("系统根证书下对自签桩拨号应失败")
	}
	if !strings.Contains(err.Error(), "中继 TLS 证书校验失败") {
		t.Fatalf("TLS 握手失败应含证书校验失败文案，实际: %v", err)
	}

	// 连接被拒（本机无监听端口）：网络层错误，不含证书校验文案
	conn2, err2 := dialRelayEndpoint(relayEndpoint{Addr: "127.0.0.1:1", TLS: true})
	if err2 == nil {
		_ = conn2.Close()
		t.Fatal("无监听端口拨号应失败")
	}
	if strings.Contains(err2.Error(), "中继 TLS 证书校验失败") {
		t.Fatalf("连接被拒不应含证书校验失败文案，实际: %v", err2)
	}
}
