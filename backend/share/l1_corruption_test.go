package share

// 收件资源损坏与进度超 100% 定位测试：
// 单机可控放大多作品并发接收（真实宿主会话 + 中继桩 + 真实收件执行器同进程），
// 覆盖并发多流完整性、断点续传/崩溃恢复、隧道中断重试续传、进度值域。
// 基建要点：确定性 PRNG 内容（seed=作品序）+ sha256 校验（不存全量字节，省内存）、
// 普通 dialer（大文件下 recordingDialer 缓存全量字节会爆内存）。
// 实测锚定：传输中隧道中断触发内层重试曾致收件文件损坏 + 进度超 100%（见 task_execution.go fetchWithRetry）。

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/export"
	importer "github.com/library-squirrel/backend/import"
)

// deterministicReader 确定性内容流：seed 区分文件，rand.NewSource(seed) 生成 size 字节。
// 同 seed 同 size 恒等 → 源文件与暂存/收件可比对。
func deterministicReader(seed, size int64) io.Reader {
	return io.LimitReader(rand.New(rand.NewSource(seed)), size)
}

// buildNWorkModel 构建 N 作品 × 每作品 1 大文件 的导出模型构建器。
// 返回 (模型, relPath→sha256) —— sourceHash 只存摘要，不存全量字节。
func buildNWorkModel(n int, size int64) func(t *testing.T, workDir string) (*export.ExportModel, map[string]string) {
	return func(t *testing.T, workDir string) (*export.ExportModel, map[string]string) {
		t.Helper()
		var works []export.WorkRecord
		var files []export.FileEntry
		sourceHash := make(map[string]string, n)
		for i := 1; i <= n; i++ {
			rel := fmt.Sprintf("store/resource/测试作者/work_%02d_000.jpg", i)
			abs := filepath.Join(workDir, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
				t.Fatal(err)
			}
			f, err := os.Create(abs)
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.New()
			if _, err := io.Copy(io.MultiWriter(f, sum), deterministicReader(int64(i), size)); err != nil {
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
			sourceHash[rel] = hex.EncodeToString(sum.Sum(nil))
			works = append(works, export.WorkRecord{
				ID: int64(i), SiteID: i64Ptr(1), SiteWorkID: strPtr(fmt.Sprintf("L1%04d", i)),
				SiteWorkName: strPtr(fmt.Sprintf("作品%02d", i)),
				Resources: []export.ResourceRecord{{ID: int64(1000 + i), ResourceType: "image",
					Stores: []export.StoreMount{{StoreType: "image", Generation: "downloaded", StoreSeq: 1, StoreID: int64(100 + i)}}}},
			})
			files = append(files, export.FileEntry{StoreID: int64(100 + i), StorePath: rel})
		}
		manifest := &export.Manifest{
			SchemaVersion: export.SchemaVersion,
			Sites:         []export.SiteRecord{{ID: 1, SiteName: strPtr("测试站")}},
			Works:         works,
			Files:         files,
		}
		return export.NewExportModel(manifest), sourceHash
	}
}

// l1HashIngestor 回灌导入桩：跨多次 Ingest 累积 entry.Path → sha256（并发多子任务共享实例；
// 各作品文件路径互异，按 path 累积不冲突）。
type l1HashIngestor struct {
	mu        sync.Mutex
	called    int
	hashes    map[string]string
	ingestErr error
}

func (f *l1HashIngestor) Ingest(ctx context.Context, manifest *export.Manifest, fileSource importer.FileSource, opts *importer.IngestOptions) (*importer.ImportResult, error) {
	f.mu.Lock()
	if f.ingestErr != nil {
		err := f.ingestErr
		f.mu.Unlock()
		return nil, err
	}
	f.called++
	f.mu.Unlock()
	for _, entry := range manifest.Files {
		if entry.Path == "" || entry.Missing {
			continue
		}
		rc, err := fileSource(entry.Path)
		if err != nil {
			if errors.Is(err, importer.ErrPackageFileMissing) {
				continue
			}
			return nil, err
		}
		sum := sha256.New()
		_, err = io.Copy(sum, rc)
		_ = rc.Close()
		if err != nil {
			return nil, err
		}
		f.mu.Lock()
		if f.hashes == nil {
			f.hashes = make(map[string]string)
		}
		f.hashes[entry.Path] = hex.EncodeToString(sum.Sum(nil))
		f.mu.Unlock()
	}
	return &importer.ImportResult{}, nil
}

// l1Env 大文件并发收件夹具：普通 dialer（不缓存传输字节）+ 可调 streamRate + hash 校验 ingestor。
type l1Env struct {
	stub       *relayStub
	hostSvc    *Service
	recvSvc    *Service
	em         *captureEmitter
	link       string
	recvDir    string
	manifest   *export.Manifest // 规划后共享 manifest
	ingestor   *l1HashIngestor
	sourceHash map[string]string // StorePath → sha256
}

// startL1Env 发布 N 作品分享并构建收件夹具；streamRate 为宿主单流限速（测试按需调）。
func startL1Env(t *testing.T, n int, size int64, streamRate int64) *l1Env {
	t.Helper()
	stub := startRelayStub(t)

	hostWorkDir := t.TempDir()
	model, sourceHash := buildNWorkModel(n, size)(t, hostWorkDir)
	if _, err := export.NewPacker().Plan(context.Background(), hostWorkDir, model); err != nil {
		t.Fatalf("规划导出模型失败: %v", err)
	}
	em := newCaptureEmitter()
	plainDial := func(addr string) (net.Conn, error) { return net.Dial("tcp", addr) }
	hostSvc := NewService(nil, &fakeCollector{model: model}, export.NewPacker(),
		func() string { return stub.addr }, func() string { return hostWorkDir },
		"test-instance-L1", em, nil, nil)
	hostSvc.setTunables(sessionRuntimeOptions{dialFn: plainDial, streamRate: streamRate})
	_, comp := publishAndWait(t, hostSvc, em, SharePublishOptions{})
	if !comp.Success {
		t.Fatalf("宿主发布失败: %s", comp.ErrMsg)
	}

	recvDir := t.TempDir()
	recvSvc := NewService(nil, nil, nil,
		func() string { return stub.addr }, func() string { return recvDir },
		"recipient-instance-L1", nil, nil, nil)
	recvSvc.setTunables(sessionRuntimeOptions{
		dialFn:          plainDial,
		streamRate:      streamRate,
		dialCoordinator: NewDialCoordinator(0, 0), // 无限制桩：绕过默认拨号门控（并发拉取不受速率限流）
	})
	t.Cleanup(func() {
		for _, d := range hostSvc.Sessions(context.Background()) {
			_ = hostSvc.Revoke(context.Background(), d.ShareID)
		}
	})
	return &l1Env{
		stub: stub, hostSvc: hostSvc, recvSvc: recvSvc, em: em,
		link: comp.Link, recvDir: recvDir,
		manifest: model.Manifest, ingestor: &l1HashIngestor{}, sourceHash: sourceHash,
	}
}

func (env *l1Env) manifestPath() string {
	return filepath.Join(env.recvDir, receiveStagingRootName, strconv.FormatInt(testParentTaskID, 10), "manifest.json")
}

func (env *l1Env) stagingDirOfTask(taskID int64) string {
	return filepath.Join(env.recvDir, receiveStagingRootName, strconv.FormatInt(taskID, 10))
}

// writeL1SharedManifest 将共享 manifest 序列化落盘到收件方父任务目录
func (env *l1Env) writeL1SharedManifest(t *testing.T) string {
	t.Helper()
	data, err := env.manifest.Serialize()
	require.NoError(t, err)
	abs := env.manifestPath()
	require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
	require.NoError(t, os.WriteFile(abs, data, 0o644))
	return path.Join(receiveStagingRootName, strconv.FormatInt(testParentTaskID, 10), "manifest.json")
}

// buildL1HandleForWork 构建指定作品的收件子任务执行句柄（cancel 供 live 取消路径）
func (env *l1Env) buildL1HandleForWork(t *testing.T, manifestID, taskID int64) (*fakeStrategyHandle, context.CancelFunc, *ReceiveExecution) {
	t.Helper()
	rel := env.writeL1SharedManifest(t)
	target, err := ParseShareLink(env.link)
	require.NoError(t, err)
	payload, err := newShareReceiveChildPayload(target, "", rel, manifestID)
	require.NoError(t, err)
	task := entity.NewTask()
	task.ID = taskID
	task.TaskType = sql.NullString{String: TaskTypeReceive, Valid: true}
	task.Payload = sql.NullString{String: payload, Valid: true}
	h, cancel := newReceiveHandle(task)
	exec := NewReceiveExecution(env.recvSvc, env.ingestor, nil, nil, nil)
	return h, cancel, exec
}

// l1RunOnce 单次收件执行（goroutine 安全：超时不 t.Fatal，返回 timedOut）
func l1RunOnce(h *fakeStrategyHandle, exec *ReceiveExecution, timeout time.Duration) (finished bool, errMsg string, timedOut bool) {
	done := make(chan struct{})
	go func() {
		exec.Execute(h)
		close(done)
	}()
	select {
	case <-done:
		fin, msg := handleOutcome(h)
		return fin, msg, false
	case <-time.After(timeout):
		return false, "", true
	}
}

// l1Job 单任务执行结果 + 进度值域观测
type l1Job struct {
	taskID     int64
	manifestID int64
	finished   bool
	errMsg     string
	timedOut   bool
	maxFrac    float64 // 进度事件中 finished/total 最大值
	over100    bool    // 出现 finished > total
}

// runL1Concurrent 并发执行 count 个子任务（semaphore 限流=并发流数），收集进度值域
func (env *l1Env) runL1Concurrent(t *testing.T, firstWork, count, concurrent int, timeout time.Duration) []*l1Job {
	t.Helper()
	handles := make([]*fakeStrategyHandle, 0, count)
	execs := make([]*ReceiveExecution, 0, count)
	jobs := make([]*l1Job, 0, count)
	for i := 0; i < count; i++ {
		manifestID := int64(firstWork + i)
		taskID := int64(700 + firstWork + i)
		h, _, exec := env.buildL1HandleForWork(t, manifestID, taskID)
		handles = append(handles, h)
		execs = append(execs, exec)
		jobs = append(jobs, &l1Job{taskID: taskID, manifestID: manifestID})
	}
	sem := make(chan struct{}, concurrent)
	var wg sync.WaitGroup
	for i := range jobs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			fin, msg, to := l1RunOnce(handles[i], execs[i], timeout)
			jobs[i].finished, jobs[i].errMsg, jobs[i].timedOut = fin, msg, to
			handles[i].mu.Lock()
			for _, p := range handles[i].progresses {
				if p[0] > 0 {
					f := float64(p[1]) / float64(p[0])
					if f > jobs[i].maxFrac {
						jobs[i].maxFrac = f
					}
					if p[1] > p[0] {
						jobs[i].over100 = true
					}
				}
			}
			handles[i].mu.Unlock()
		}(i)
	}
	wg.Wait()
	return jobs
}

// waitProgressAtLeast 轮询句柄进度直到 finished 达 target（live 取消测试用）
func waitProgressAtLeast(t *testing.T, h *fakeStrategyHandle, target int64, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	last := int64(0)
	for time.Now().Before(deadline) {
		h.mu.Lock()
		if n := len(h.progresses); n > 0 {
			last = h.progresses[n-1][1]
		}
		h.mu.Unlock()
		if last >= target {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("进度未达到 %d，最近=%d", target, last)
}

// assertJobsSucceeded 全部任务成功 + 无超时 + 进度不超 100%（T6 值域）
func (env *l1Env) assertJobsSucceeded(t *testing.T, jobs []*l1Job) {
	t.Helper()
	for _, j := range jobs {
		require.False(t, j.timedOut, "任务 %d 执行超时", j.taskID)
		require.True(t, j.finished, "任务 %d 应成功: %s", j.taskID, j.errMsg)
		require.False(t, j.over100, "任务 %d 进度超 100%%（maxFrac=%.4f）", j.taskID, j.maxFrac)
	}
}

// assertReceivedIntegrity 收件内容 hash 与源逐文件一致（fetch/暂存/导入面完整性）
func (env *l1Env) assertReceivedIntegrity(t *testing.T) {
	t.Helper()
	env.ingestor.mu.Lock()
	defer env.ingestor.mu.Unlock()
	require.NotZero(t, env.ingestor.called, "应至少有一次回灌导入")
	for _, entry := range env.manifest.Files {
		if entry.Path == "" || entry.Missing {
			continue
		}
		want, ok := env.sourceHash[entry.StorePath]
		require.True(t, ok, "源 hash 缺失: %s", entry.StorePath)
		got, ok := env.ingestor.hashes[entry.Path]
		require.True(t, ok, "收件未导入: %s", entry.Path)
		require.Equal(t, want, got, "内容损坏: %s", entry.Path)
	}
}

// TestL1ConcurrentIntegrity3 并发 3 流完整性（默认 MaxParallelImport=3 档）
func TestL1ConcurrentIntegrity3(t *testing.T) {
	env := startL1Env(t, 6, 16<<20, 256<<20)
	jobs := env.runL1Concurrent(t, 1, 6, 3, 60*time.Second)
	env.assertJobsSucceeded(t, jobs)
	env.assertReceivedIntegrity(t)
}

// TestL1ConcurrentIntegrity8 并发 8 流 + 排队完整性（12 作品、8 并发 → 8 流 + 4 排队）
func TestL1ConcurrentIntegrity8(t *testing.T) {
	env := startL1Env(t, 12, 16<<20, 256<<20)
	jobs := env.runL1Concurrent(t, 1, 12, 8, 120*time.Second)
	env.assertJobsSucceeded(t, jobs)
	env.assertReceivedIntegrity(t)
}

// TestL1ConcurrentIntegrity100MB 并发 8 流 + 100MB 大文件（计划目标尺寸，排除尺寸因素）
func TestL1ConcurrentIntegrity100MB(t *testing.T) {
	env := startL1Env(t, 9, 100<<20, 256<<20)
	jobs := env.runL1Concurrent(t, 1, 9, 8, 180*time.Second)
	env.assertJobsSucceeded(t, jobs)
	env.assertReceivedIntegrity(t)
}

// TestL1BackpressurePaced 单流真实限速 16MiB/s 背压分块路径（对齐生产单流限速）
func TestL1BackpressurePaced(t *testing.T) {
	env := startL1Env(t, 1, 100<<20, 16<<20)
	h, _, exec := env.buildL1HandleForWork(t, 1, 777)
	fin, msg, to := l1RunOnce(h, exec, 120*time.Second)
	require.False(t, to, "执行超时")
	require.True(t, fin, "限速拉取应成功: %s", msg)
	env.assertReceivedIntegrity(t)
}

// TestL1TunnelDropResume 传输中宿主隧道断连（模拟网络故障）→ 收件重试续传 → 内容完整
func TestL1TunnelDropResume(t *testing.T) {
	env := startL1Env(t, 1, 64<<20, 256<<20)
	target, err := ParseShareLink(env.link)
	require.NoError(t, err)
	h, _, exec := env.buildL1HandleForWork(t, 1, 777)
	done := make(chan struct{})
	go func() {
		exec.Execute(h)
		close(done)
	}()
	waitProgressAtLeast(t, h, (64<<20)/2, 30*time.Second)
	env.stub.dropTunnel(target.Token)
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		t.Fatal("隧道断连后执行未及时结束")
	}
	fin, msg := handleOutcome(h)
	require.True(t, fin, "断连后经重试续传应成功: %s", msg)
	env.assertReceivedIntegrity(t)
	// 进度值域：重试续传不得把部分字节双计（与损坏同源的 >100% 面）
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, p := range h.progresses {
		require.LessOrEqual(t, p[1], p[0], "进度不得超 total")
	}
}

// TestL1ResumePreWrittenPartial 断点续传（预写 1/3 暂存模拟崩溃残留）：续传后内容完整
func TestL1ResumePreWrittenPartial(t *testing.T) {
	env := startL1Env(t, 1, 96<<20, 256<<20)
	entry := env.manifest.Files[0]
	partial := int64(32 << 20)
	stagingFile := filepath.Join(env.stagingDirOfTask(777), filepath.FromSlash(entry.Path))
	require.NoError(t, os.MkdirAll(filepath.Dir(stagingFile), 0o755))
	f, err := os.Create(stagingFile)
	require.NoError(t, err)
	_, err = io.CopyN(f, deterministicReader(1, entry.Size), partial)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	h, _, exec := env.buildL1HandleForWork(t, 1, 777)
	fin, msg, to := l1RunOnce(h, exec, 60*time.Second)
	require.False(t, to, "执行超时")
	require.True(t, fin, "续传应成功: %s", msg)
	env.assertReceivedIntegrity(t)
}

// TestL1CrashCancelResume live 取消（模拟崩溃）→ 重建执行续传 → 内容完整
func TestL1CrashCancelResume(t *testing.T) {
	env := startL1Env(t, 1, 128<<20, 32<<20)
	h, cancel, exec := env.buildL1HandleForWork(t, 1, 777)
	done := make(chan struct{})
	go func() {
		exec.Execute(h)
		close(done)
	}()
	waitProgressAtLeast(t, h, (128<<20)/2, 15*time.Second)
	cancel()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("取消后执行未及时退出")
	}
	fin, msg := handleOutcome(h)
	require.False(t, fin, "取消后不应成功终态: %s", msg)

	// 重建执行（同作品同任务 ID → 同暂存目录）→ 从断点续传 → 内容完整
	h2, _, exec2 := env.buildL1HandleForWork(t, 1, 777)
	fin2, msg2, to2 := l1RunOnce(h2, exec2, 60*time.Second)
	require.False(t, to2, "续传超时")
	require.True(t, fin2, "续传应成功: %s", msg2)
	env.assertReceivedIntegrity(t)
}
