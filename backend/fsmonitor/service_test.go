package fsmonitor

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/util/fingerprint"
	"go.uber.org/zap"
)

// startProbe 启动链路探针：单一替身同时实现 LiveEventSource/ReconciliationScanner/
// OfflineChangeProvider/StoreReader/fingerprint.Computer，记录各能力被触达次数。
// 事件 channel 返回 nil（消费 goroutine 停驻等待 ctx 取消），查询类方法返回空集
type startProbe struct {
	mu      sync.Mutex
	starts  int // LiveEventSource.Start 调用数
	scans   int // ReconciliationScanner.Scan 调用数
	offline int // OfflineChangeProvider.ChangesSince 调用数
}

func (p *startProbe) Start(ctx context.Context) (<-chan FileChange, <-chan error, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.starts++
	return nil, nil, nil
}

func (p *startProbe) Stop() {}

func (p *startProbe) Scan(ctx context.Context) (DiffSet, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.scans++
	return DiffSet{}, nil
}

func (p *startProbe) ChangesSince(ctx context.Context, cursor OfflineCursor) ([]FileChange, OfflineCursor, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.offline++
	return nil, nil, nil
}

func (p *startProbe) GetByFingerprint(ctx context.Context, fp string, excludePath string) (*StoreRecord, error) {
	return nil, nil
}

func (p *startProbe) GetByFilePathComplete(ctx context.Context, filePath string) (*StoreRecord, error) {
	return nil, nil
}

func (p *startProbe) ListValidComplete(ctx context.Context) ([]StoreRecord, error) {
	return nil, nil
}

func (p *startProbe) Fingerprint(ctx context.Context, absPath string) (fingerprint.Fingerprint, error) {
	return fingerprint.Fingerprint{}, nil
}

type startProbeCounts struct {
	starts  int
	scans   int
	offline int
}

func (p *startProbe) snapshot() startProbeCounts {
	p.mu.Lock()
	defer p.mu.Unlock()
	return startProbeCounts{starts: p.starts, scans: p.scans, offline: p.offline}
}

// newStartProbeService 构造全能力注入的 Service（live 源/离线追溯/对账/关联层全就绪），
// 依赖触达情况全部落在探针上；workDir 由入参闭包提供
func newStartProbeService(t *testing.T, p *startProbe, workDir func() string) *Service {
	t.Helper()
	logger.Log = zap.NewNop().Sugar() // 测试进程无 logger.Init，置 nop 防 Start 日志调用 panic
	deps := &Deps{
		LiveSource:      p,
		OfflineProvider: p,
		Scanner:         p,
		Fingerprinter:   p,
		StoreReader:     p,
	}
	return NewService(deps, workDir, func() EventEmitter { return nil })
}

// TestStartSkipsWhenWorkDirUnconfigured 工作目录未配置（空串）时 Start 整体不启动：
// 无 goroutine 产生、三个子能力（live 源/离线追溯/对账扫描）零触达
func TestStartSkipsWhenWorkDirUnconfigured(t *testing.T) {
	p := &startProbe{}
	svc := newStartProbeService(t, p, func() string { return "" })

	baseline := runtime.NumGoroutine()
	svc.Start()
	waitGoroutineCountAtMost(t, baseline, "未配置态 Start 不应产生任何 goroutine")

	got := p.snapshot()
	if got.starts != 0 || got.scans != 0 || got.offline != 0 {
		t.Fatalf("未配置态不应触达任何子能力，实际 starts=%d scans=%d offline=%d", got.starts, got.scans, got.offline)
	}
	svc.Stop()
}

// TestStartRunsWhenWorkDirConfigured 对照组：已配置时 Start 触达三个子能力并产生后台
// goroutine（Stop 后回落）——锚定上例的零触达/零 goroutine 断言非空转
func TestStartRunsWhenWorkDirConfigured(t *testing.T) {
	p := &startProbe{}
	svc := newStartProbeService(t, p, func() string { return t.TempDir() })
	defer svc.Stop()

	baseline := runtime.NumGoroutine()
	svc.Start()
	waitForCondition(t, 2*time.Second, func() bool {
		s := p.snapshot()
		return s.starts >= 1 && s.scans >= 1 && s.offline >= 1
	})
	if runtime.NumGoroutine() <= baseline {
		t.Fatalf("已配置态 Start 应产生后台 goroutine，当前 %d 未高于基线 %d", runtime.NumGoroutine(), baseline)
	}

	svc.Stop()
	waitGoroutineCountAtMost(t, baseline, "Stop 后后台 goroutine 应退出")
}

// waitGoroutineCountAtMost 轮询等待 goroutine 数回落到 limit 以内（goroutine 退出有调度延迟）
func waitGoroutineCountAtMost(t *testing.T, limit int, msg string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= limit {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s：goroutine 数 %d 未回落到 %d 以内", msg, runtime.NumGoroutine(), limit)
}

// waitForCondition 轮询等待条件成立，超时致命
func waitForCondition(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("等待条件超时（%s）", timeout)
}
