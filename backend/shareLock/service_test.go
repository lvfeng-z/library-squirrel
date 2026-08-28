package shareLock

import (
	"context"
	"sync"
	"testing"
)

// newReg 经生产构造函数创建注册中心（实例状态无全局污染，用例各自独立）
func newReg() ShareLockRegistry {
	return NewShareLockRegistry()
}

// TestRegisterAndIsLocked 登记后命中、未登记不命中
func TestRegisterAndIsLocked(t *testing.T) {
	r := newReg()
	r.Register(context.Background(), []int64{1, 2}, "s1")
	if !r.IsLocked(context.Background(), 1) {
		t.Fatal("登记后作品 1 应处于锁定")
	}
	if !r.IsLocked(context.Background(), 2) {
		t.Fatal("登记后作品 2 应处于锁定")
	}
	if r.IsLocked(context.Background(), 3) {
		t.Fatal("未登记的作品 3 不应锁定")
	}
}

// TestMultiSessionHolding 多会话持有同一作品：单会话解除不误放，全部解除才解锁
func TestMultiSessionHolding(t *testing.T) {
	r := newReg()
	ctx := context.Background()
	r.Register(ctx, []int64{10}, "s1")
	r.Register(ctx, []int64{10}, "s2")

	r.Unregister(ctx, []int64{10}, "s1")
	if !r.IsLocked(ctx, 10) {
		t.Fatal("会话 s2 仍持有，作品 10 不应因 s1 解除而放行")
	}

	r.Unregister(ctx, []int64{10}, "s2")
	if r.IsLocked(ctx, 10) {
		t.Fatal("全部会话解除后作品 10 应解锁")
	}
}

// TestUnregisterIdempotent 未登记组合的 Unregister 静默忽略（未登记作品 / 他会话持有）
func TestUnregisterIdempotent(t *testing.T) {
	r := newReg()
	ctx := context.Background()
	r.Register(ctx, []int64{20}, "s1")

	// 未登记过的作品：无副作用不报错
	r.Unregister(ctx, []int64{999}, "s1")
	// 该作品上未登记过的会话：不释放 s1 的持有
	r.Unregister(ctx, []int64{20}, "other")
	if !r.IsLocked(ctx, 20) {
		t.Fatal("他会话的 Unregister 不应释放 s1 的持有")
	}
}

// TestReRegisterIdempotent 同 (workID, session) 重复登记为引用集合语义：单次 Unregister 即解除
func TestReRegisterIdempotent(t *testing.T) {
	r := newReg()
	ctx := context.Background()
	r.Register(ctx, []int64{30}, "s1")
	r.Register(ctx, []int64{30}, "s1") // 重复登记不产生双重引用
	r.Unregister(ctx, []int64{30}, "s1")
	if r.IsLocked(ctx, 30) {
		t.Fatal("重复登记后单次解除应彻底解锁（引用集合语义，非计数）")
	}
}

// TestForceUnlock 强制解锁清除全部会话引用；此后原会话补解除静默忽略
func TestForceUnlock(t *testing.T) {
	r := newReg()
	ctx := context.Background()
	r.Register(ctx, []int64{40}, "s1")
	r.Register(ctx, []int64{40}, "s2")

	r.ForceUnlock(ctx, 40)
	if r.IsLocked(ctx, 40) {
		t.Fatal("强制解锁后应清除全部会话引用")
	}

	// 强制解锁后原会话补解除：幂等不报错、不复活锁
	r.Unregister(ctx, []int64{40}, "s1")
	r.Unregister(ctx, []int64{40}, "s2")
	if r.IsLocked(ctx, 40) {
		t.Fatal("强制解锁后的补解除不应复活锁")
	}
}

// TestForceUnlockUnregistered 对未登记作品强制解锁静默忽略
func TestForceUnlockUnregistered(t *testing.T) {
	r := newReg()
	r.ForceUnlock(context.Background(), 50)
	if r.IsLocked(context.Background(), 50) {
		t.Fatal("未登记作品强制解锁后不应锁定")
	}
}

// TestBatchScope 批量登记/解除的作用域：解除只作用于给定作品，其余作品不受影响
func TestBatchScope(t *testing.T) {
	r := newReg()
	ctx := context.Background()
	r.Register(ctx, []int64{1, 2, 3}, "s1")

	r.Unregister(ctx, []int64{2}, "s1")
	if !r.IsLocked(ctx, 1) || r.IsLocked(ctx, 2) || !r.IsLocked(ctx, 3) {
		t.Fatalf("解除作品 2 后应为 [1 锁, 2 解, 3 锁]，实际 [1:%v, 2:%v, 3:%v]",
			r.IsLocked(ctx, 1), r.IsLocked(ctx, 2), r.IsLocked(ctx, 3))
	}
}

// TestEmptyWorkIDs 空 workIDs 的登记/解除无副作用
func TestEmptyWorkIDs(t *testing.T) {
	r := newReg()
	ctx := context.Background()
	r.Register(ctx, nil, "s1")
	r.Register(ctx, []int64{}, "s1")
	r.Unregister(ctx, nil, "s1")
	if r.IsLocked(ctx, 1) {
		t.Fatal("空 workIDs 操作不应产生任何锁定")
	}
}

// TestConcurrent 并发登记/解除/查询/强制解锁无 race（须 go test -race）
func TestConcurrent(t *testing.T) {
	r := newReg()
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(4)
		go func() { defer wg.Done(); r.Register(ctx, []int64{1, 2}, "sA") }()
		go func() { defer wg.Done(); r.Register(ctx, []int64{2, 3}, "sB") }()
		go func() {
			defer wg.Done()
			r.Unregister(ctx, []int64{1, 2}, "sA")
			r.Unregister(ctx, []int64{2, 3}, "sB")
		}()
		go func() { defer wg.Done(); r.IsLocked(ctx, 1); r.IsLocked(ctx, 2); r.ForceUnlock(ctx, 3) }()
	}
	wg.Wait()
}
