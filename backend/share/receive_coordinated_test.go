package share

// 收件拨号统筹接入的并发场景测试：fake 网络桩（计数拨号器 + 中继桩）锚定「10 并发子任务
// 拉取 → 同时活跃拨号 ≤ 并发槽 8、拨号总数受速率/重试上界约束、全完成无 rate_limited」。
// 与既有接收执行测试的差异：这里注入真实生产参数统筹器（8 槽/50 每分钟），验证门控接入后
// 的并发行为；既有测试注入无限制桩绕过门控聚焦各自业务语义。

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/library-squirrel/backend/export"
)

// buildTenWorkModel 10 作品导出模型（每作品 1 个非缺失文件）——收件拨号统筹并发场景锚定：
// 10 子任务并发拉取时同时活跃拨号受并发槽限制、拨号总数受速率门控约束。
func buildTenWorkModel(t *testing.T, workDir string) (*export.ExportModel, map[string][]byte) {
	t.Helper()
	const n = 10
	works := make([]export.WorkRecord, 0, n)
	files := make([]export.FileEntry, 0, n)
	contents := make(map[string][]byte, n)
	for i := 1; i <= n; i++ {
		rel := fmt.Sprintf("store/resource/测试作者/work_%02d.jpg", i)
		content := []byte(fmt.Sprintf("TEN-WORK-PLAINTEXT-%02d", i))
		abs := filepath.Join(workDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, content, 0o644); err != nil {
			t.Fatal(err)
		}
		contents[rel] = content
		storeID := int64(1000 + i)
		works = append(works, export.WorkRecord{
			ID: int64(i), SiteID: i64Ptr(1), SiteWorkID: strPtr(fmt.Sprintf("w%02d", i)),
			SiteWorkName: strPtr(fmt.Sprintf("并发作品%02d", i)),
			Resources: []export.ResourceRecord{{
				ID: int64(2000 + i), ResourceType: "image",
				Stores: []export.StoreMount{{StoreType: "image", Generation: "downloaded", StoreSeq: 1, StoreID: storeID}},
			}},
		})
		files = append(files, export.FileEntry{StoreID: storeID, StorePath: rel})
	}
	manifest := &export.Manifest{
		SchemaVersion: export.SchemaVersion,
		Sites:         []export.SiteRecord{{ID: 1, SiteName: strPtr("测试站")}},
		Works:         works,
		Files:         files,
	}
	return export.NewExportModel(manifest), contents
}

// countingDialer 并发拨号计数桩：包一层既有记录拨号器，统计同时进行中的拨号数峰值。
// dial 内先停留制造并发观察窗口（真实 TCP 建连瞬时完成，无停留统计不到并发峰值）；仅测试使用。
type countingDialer struct {
	rec    *recordingDialer
	mu     sync.Mutex
	active int
	max    int
}

func (d *countingDialer) dial(addr string) (net.Conn, error) {
	d.mu.Lock()
	d.active++
	if d.active > d.max {
		d.max = d.active
	}
	d.mu.Unlock()
	time.Sleep(30 * time.Millisecond) // 制造并发拨号停留窗，观察同时活跃数峰值
	conn, err := d.rec.dial(addr)
	d.mu.Lock()
	d.active--
	d.mu.Unlock()
	return conn, err
}

func (d *countingDialer) maxActive() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.max
}

// runConcurrentReceiveHandles 并发执行 manifest 全部作品的收件子任务（共享 recvSvc 门控），
// 等待全部终态后返回句柄列表。taskID 自 3000 起（避开共享 manifest 父任务 999 与单任务 777）。
func runConcurrentReceiveHandles(t *testing.T, env *receiveTestEnv) []*fakeStrategyHandle {
	t.Helper()
	handles := make([]*fakeStrategyHandle, 0, len(env.manifest.Works))
	execs := make([]*ReceiveExecution, 0, len(env.manifest.Works))
	for i, w := range env.manifest.Works {
		h, _, exec := env.buildReceiveHandleForWork(t, "", w.ID, 3000+int64(i))
		handles = append(handles, h)
		execs = append(execs, exec)
	}
	var wg sync.WaitGroup
	for i := range handles {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			execs[i].Execute(handles[i])
		}(i)
	}
	wg.Wait()
	return handles
}

// TestReceiveExecutionCoordinatedDialing 并发拨号统筹锚：10 子任务并发拉取 →
// 同时活跃拨号 ≤ 并发槽 8（未接入时 10 并发可同时拨号超限）、拨号总数受文件数×重试上界约束（无爆炸）。
func TestReceiveExecutionCoordinatedDialing(t *testing.T) {
	env := startReceiveEnvModel(t, SharePublishOptions{}, buildTenWorkModel)
	cd := &countingDialer{rec: env.dialer}
	// 注入真实生产参数统筹器（并发 8 槽 + 速率 50/min）+ 计数拨号桩
	env.recvSvc.setTunables(sessionRuntimeOptions{
		dialFn:          cd.dial,
		streamRate:      8 << 20,
		dialCoordinator: NewDialCoordinator(8, 50),
	})

	handles := runConcurrentReceiveHandles(t, env)

	// 全完成：10 个并发子任务均应成功终态
	for i, h := range handles {
		finished, failed := handleOutcome(h)
		require.Truef(t, finished, "并发子任务 %d 应成功完成，失败信息: %s", i, failed)
	}
	// 并发拨号 ≤ 并发槽 8（且确有多拨号重叠——非串行），门控把并发压制在槽位内
	assert.GreaterOrEqual(t, cd.maxActive(), 2, "并发子任务应观察到同时拨号重叠")
	assert.LessOrEqual(t, cd.maxActive(), 8, "同时进行中的拨号数应被并发槽限制在 8 内")
	// 拨号总数 ≥ 文件数（每文件至少拉取一次）且 ≤ 文件数×重试上界（无拒绝重试爆炸）
	target, err := ParseShareLink(env.link)
	require.NoError(t, err)
	reqs := parseRecordedRequests(t, env.dialer.snapshot(), target.Key)
	require.GreaterOrEqual(t, len(reqs), len(env.manifest.Files),
		"每文件应至少发起一次拉取请求，实际: %d", len(reqs))
	require.LessOrEqual(t, int64(len(reqs)), int64(len(env.manifest.Files))*receiveMaxAttempts,
		"拨号总数应受文件数×重试上界约束，实际: %d", len(reqs))
}

// TestReceiveExecutionNoRateLimitedUnderConcurrency 并发 10 场景断言无 rate_limited 失败
// （对照修复前并发自由竞争触发中继限流致任务 3/6 失败的场景）。
func TestReceiveExecutionNoRateLimitedUnderConcurrency(t *testing.T) {
	env := startReceiveEnvModel(t, SharePublishOptions{}, buildTenWorkModel)
	// 真实生产参数统筹器（8 槽/50 每分钟）：并发拉取对齐发送方会话上限与中继限流，不触发限流
	env.recvSvc.setTunables(sessionRuntimeOptions{
		dialFn:          env.dialer.dial,
		streamRate:      8 << 20,
		dialCoordinator: NewDialCoordinator(8, 50),
	})

	handles := runConcurrentReceiveHandles(t, env)

	// 并发 10 场景全完成且无 rate_limited 失败
	for i, h := range handles {
		finished, failed := handleOutcome(h)
		require.Truef(t, finished, "并发子任务 %d 应成功完成，失败信息: %s", i, failed)
		assert.NotContains(t, failed, "rate_limited", "并发拉取不应出现拨号限流错误")
	}
}
