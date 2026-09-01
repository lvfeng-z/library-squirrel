package share

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// TestDialCoordinatorConcurrencyCap：N 个 goroutine 并发取槽，同时活跃数恒不超过
// maxConcurrent，超出者阻塞排队，靠 ReleaseSlot 依次放行。
func TestDialCoordinatorConcurrencyCap(t *testing.T) {
	const maxConcurrent = 3
	const total = 10
	c := NewDialCoordinator(maxConcurrent, 0)
	ctx := context.Background()

	var wg sync.WaitGroup
	var mu sync.Mutex
	active, maxActive := 0, 0
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := c.AcquireSlot(ctx); err != nil {
				t.Errorf("AcquireSlot: %v", err)
				return
			}
			mu.Lock()
			active++
			if active > maxActive {
				maxActive = active
			}
			mu.Unlock()
			time.Sleep(20 * time.Millisecond) // 模拟一条流的下载期
			mu.Lock()
			active--
			mu.Unlock()
			c.ReleaseSlot()
		}()
	}
	wg.Wait()
	if maxActive > maxConcurrent {
		t.Fatalf("同时活跃流数 = %d，期望 <= %d", maxActive, maxConcurrent)
	}
}

// TestDialCoordinatorSlotBlockAndRelease：槽取满后第 N+1 次取槽阻塞，ReleaseSlot 后放行。
func TestDialCoordinatorSlotBlockAndRelease(t *testing.T) {
	c := NewDialCoordinator(2, 0)
	ctx := context.Background()
	if err := c.AcquireSlot(ctx); err != nil {
		t.Fatalf("AcquireSlot: %v", err)
	}
	if err := c.AcquireSlot(ctx); err != nil {
		t.Fatalf("AcquireSlot: %v", err)
	}

	// 槽已满：第 3 个 AcquireSlot 阻塞
	done := make(chan error, 1)
	go func() { done <- c.AcquireSlot(ctx) }()
	select {
	case err := <-done:
		t.Fatalf("槽满时应阻塞，却返回 %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	// 释放一个槽 → 阻塞的取槽放行
	c.ReleaseSlot()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("释放后取槽应成功：%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("释放后阻塞的取槽应放行")
	}
}

// TestDialCoordinatorRateLimit：连续拨号用满配额后，超限拨号被推迟；最老事件
// 滑出窗口（回拨 dialTimes[0] 制造）后放行。
func TestDialCoordinatorRateLimit(t *testing.T) {
	c := NewDialCoordinator(0, 3)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := c.TakeDial(ctx); err != nil {
			t.Fatalf("TakeDial #%d: %v", i+1, err)
		}
	}

	// 配额已满：第 4 次拨号被推迟
	done := make(chan error, 1)
	go func() { done <- c.TakeDial(ctx) }()
	select {
	case err := <-done:
		t.Fatalf("超限拨号应被推迟，却返回 %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	// 回拨最老事件制造窗口滑出
	c.mu.Lock()
	c.dialTimes[0] = time.Now().Add(-2 * time.Minute)
	c.mu.Unlock()

	// 窗口滑出后放行
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("窗口滑出后拨号应成功：%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("窗口滑出后阻塞的拨号应放行")
	}
}

// TestDialCoordinatorCtxCancel：阻塞等待中 ctx 取消应立即返回 context.Canceled。
func TestDialCoordinatorCtxCancel(t *testing.T) {
	t.Run("AcquireSlot", func(t *testing.T) {
		c := NewDialCoordinator(1, 0)
		if err := c.AcquireSlot(context.Background()); err != nil {
			t.Fatalf("AcquireSlot: %v", err)
		}
		cctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- c.AcquireSlot(cctx) }()
		cancel()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("ctx 取消应返回 context.Canceled，got %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("ctx 取消后阻塞取槽应立即返回")
		}
	})
	t.Run("TakeDial", func(t *testing.T) {
		c := NewDialCoordinator(0, 1) // 速率 1/min，一次即满
		if err := c.TakeDial(context.Background()); err != nil {
			t.Fatalf("TakeDial: %v", err)
		}
		cctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- c.TakeDial(cctx) }()
		select {
		case err := <-done:
			t.Fatalf("配额满时拨号应阻塞，却返回 %v", err)
		case <-time.After(50 * time.Millisecond):
		}
		cancel()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("ctx 取消应返回 context.Canceled，got %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("ctx 取消后阻塞拨号应立即返回")
		}
	})
}

// TestDialCoordinatorReleaseSlotIdempotent：空槽时多余释放无害，槽计数不被破坏，
// 仍能取满 maxConcurrent 槽且超额者照常阻塞。
func TestDialCoordinatorReleaseSlotIdempotent(t *testing.T) {
	c := NewDialCoordinator(2, 0)
	for i := 0; i < 5; i++ {
		c.ReleaseSlot() // 不应阻塞、不应产生令牌
	}

	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if err := c.AcquireSlot(ctx); err != nil {
			t.Fatalf("AcquireSlot #%d: %v", i+1, err)
		}
	}
	done := make(chan error, 1)
	go func() { done <- c.AcquireSlot(ctx) }()
	select {
	case err := <-done:
		t.Fatalf("槽满时应阻塞，却返回 %v", err)
	case <-time.After(50 * time.Millisecond):
	}
}

// TestDialCoordinatorQuotaFullCallback：配额满进入阻塞态时配额满回调触发一次，
// 且跨复查循环不重复触发（一次 TakeDial 调用至多通知一次）。
func TestDialCoordinatorQuotaFullCallback(t *testing.T) {
	c := NewDialCoordinator(0, 1) // 速率 1/min：一次取令牌即满
	if err := c.TakeDial(context.Background()); err != nil {
		t.Fatalf("TakeDial 填满配额: %v", err)
	}
	var mu sync.Mutex
	calls := 0
	c.SetQuotaFullCallback(func() {
		mu.Lock()
		calls++
		mu.Unlock()
	})

	cctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- c.TakeDial(cctx) }()
	// 等待回调至少触发一次（阻塞已建立）
	deadline := time.Now().Add(time.Second)
	for {
		mu.Lock()
		n := calls
		mu.Unlock()
		if n >= 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("配额满回调未触发")
		}
		time.Sleep(5 * time.Millisecond)
	}
	// 阻塞持续多个复查周期，回调不得重复触发
	time.Sleep(3 * dialPollInterval)
	mu.Lock()
	if calls != 1 {
		t.Fatalf("回调应只触发一次，实际 %d", calls)
	}
	mu.Unlock()
	// 取消解除阻塞
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("ctx 取消应返回 context.Canceled，got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ctx 取消后阻塞拨号应立即返回")
	}
}
