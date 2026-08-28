package share

// 供流侧作品锁接线测试：收件人拉取作品文件时分享方供流路径登记作品锁（会话 token 为会话键），
// 会话终态时解除——宿主本地替换/删除正被拉取作品的并发防护由此生效。

import (
	"context"
	"testing"
	"time"

	"github.com/library-squirrel/backend/shareLock"
)

// pullOneFile 经收件人协议拉取单个文件（返回应答头供断言）
func pullOneFile(t *testing.T, stub *relayStub, token string, cip *e2eCipher, path string) streamHeader {
	t.Helper()
	conn, err := recipientDial(t, stub.addr, token, "")
	if err != nil {
		t.Fatalf("收件人拨号失败: %v", err)
	}
	defer func() { _ = conn.Close() }()
	head, _, _, err := recipientFetch(t, cip, conn, streamRequest{Type: "file", Path: path})
	if err != nil {
		t.Fatalf("拉取文件失败: %v", err)
	}
	return head
}

// TestServeFileRegistersWorkLock 收件人首次拉取作品文件 → 供流路径以会话 token 登记作品锁；
// 同会话重复拉取保持锁定；以无关会话解除不生效，以本会话 token 解除才放行（会话键即 token）
func TestServeFileRegistersWorkLock(t *testing.T) {
	stub := startRelayStub(t)
	workDir := t.TempDir()
	model, _ := buildTestModel(t, workDir)
	em := newCaptureEmitter()
	lock := shareLock.NewShareLockRegistry()
	svc := newTestService(t, stub, workDir, model, em, nil, lock)

	_, comp := publishAndWait(t, svc, em, SharePublishOptions{})
	if !comp.Success {
		t.Fatalf("发布失败: %s", comp.ErrMsg)
	}
	key, _ := keyFromLink(t, comp.Link)
	cip, err := newE2ECipher(key)
	if err != nil {
		t.Fatal(err)
	}
	path := findEntry(t, model, 101).Path // 测试模型的文件 101 归属作品 1

	if lock.IsLocked(context.Background(), 1) {
		t.Fatal("拉取前作品不应被锁")
	}
	if head := pullOneFile(t, stub, comp.Session.Token, cip, path); !head.OK {
		t.Fatalf("文件拉取失败: %+v", head)
	}
	if !lock.IsLocked(context.Background(), 1) {
		t.Fatal("首次供流后所属作品应被登记锁定")
	}
	// 同会话再次拉取：作品保持锁定（登记收敛到每作品一次，重复供流无影响）
	if head := pullOneFile(t, stub, comp.Session.Token, cip, path); !head.OK {
		t.Fatalf("重复拉取失败: %+v", head)
	}
	if !lock.IsLocked(context.Background(), 1) {
		t.Fatal("同会话重复供流期间作品应保持锁定")
	}
	// 登记会话键 = 会话 token：无关会话解除不生效，token 解除才放行
	lock.Unregister(context.Background(), []int64{1}, "无关会话")
	if !lock.IsLocked(context.Background(), 1) {
		t.Fatal("无关会话解除不应放行作品锁")
	}
	lock.Unregister(context.Background(), []int64{1}, comp.Session.Token)
	if lock.IsLocked(context.Background(), 1) {
		t.Fatal("以会话 token 解除应放行作品锁")
	}
}

// TestSessionTeardownReleasesWorkLock 会话终态（撤销）时解除本会话登记的全部作品锁
func TestSessionTeardownReleasesWorkLock(t *testing.T) {
	stub := startRelayStub(t)
	workDir := t.TempDir()
	model, _ := buildTestModel(t, workDir)
	em := newCaptureEmitter()
	lock := shareLock.NewShareLockRegistry()
	svc := newTestService(t, stub, workDir, model, em, nil, lock)

	shareID, comp := publishAndWait(t, svc, em, SharePublishOptions{})
	if !comp.Success {
		t.Fatalf("发布失败: %s", comp.ErrMsg)
	}
	key, _ := keyFromLink(t, comp.Link)
	cip, _ := newE2ECipher(key)
	if head := pullOneFile(t, stub, comp.Session.Token, cip, findEntry(t, model, 101).Path); !head.OK {
		t.Fatalf("文件拉取失败: %+v", head)
	}
	if !lock.IsLocked(context.Background(), 1) {
		t.Fatal("供流后作品应被登记锁定")
	}

	if err := svc.Revoke(context.Background(), shareID); err != nil {
		t.Fatalf("撤销失败: %v", err)
	}
	em.waitState(t, shareID, stateRevoked, 5*time.Second)
	// 解锁发生在会话主循环退出的收尾路径（终态事件推送之后），轮询等待
	deadline := time.Now().Add(5 * time.Second)
	for lock.IsLocked(context.Background(), 1) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if lock.IsLocked(context.Background(), 1) {
		t.Fatal("会话终态后作品锁应被解除")
	}
}
