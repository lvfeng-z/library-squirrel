package share

import (
	"context"
	"sync"
	"time"

	"github.com/library-squirrel/backend/base/logger"
)

// dialWindow 拨号滑动窗口宽度：窗口内累计的拨号事件数即每分钟拨号速率。
const dialWindow = time.Minute

// dialPollInterval 满配额复查周期：等待滑出窗口期间按此间隔醒来重算放行时刻，
// 使最老事件滑出后能及时放行，而非按首次计算一次睡满剩余窗口。
const dialPollInterval = 50 * time.Millisecond

// slotBlockLogThreshold 流槽等待超过该时长才记日志（阻塞即卡顿信号；瞬时取得不刷屏）。
const slotBlockLogThreshold = 50 * time.Millisecond

// DialCoordinator 收件拨号统筹器：统一仲裁 share-receive 各子任务的文件拉取拨号。
// 双门控——并发流槽（防超发送方会话 8 流上限被拒）+ 拨号速率（防超中继每 IP 每分钟
// 限流）。纯并发/速率能力，不感知业务实体。进程级单例，app.go 装配注入。
type DialCoordinator struct {
	slots     chan struct{} // 并发流槽信号量（容量 = maxConcurrent）
	dialCap   int           // 每分钟拨号上限（<=0 不限）
	mu        sync.Mutex
	dialTimes []time.Time // 滑动窗口拨号事件（窗口 = 1 分钟）
	// quotaFullCb 配额满通知回调：TakeDial 进入阻塞态（配额已满）时触发一次，供装配方上报
	// 用户感知（如轻提示「拨号配额耗尽」）。纯能力维度，载荷仅时间语义，不感知业务实体；
	// nil=不通知（测试/未装配场景）
	quotaFullCb func()
}

// NewDialCoordinator 创建统筹器：maxConcurrent<=0 不限并发，maxDialsPerMinute<=0 不限速率。
func NewDialCoordinator(maxConcurrent, maxDialsPerMinute int) *DialCoordinator {
	c := &DialCoordinator{
		dialCap: maxDialsPerMinute,
	}
	if maxConcurrent > 0 {
		c.slots = make(chan struct{}, maxConcurrent)
	}
	return c
}

// DefaultDialCoordinator 进程级默认收件拨号统筹器（并发 8 槽 + 速率 50/min）：对齐发送方会话
// 流上限（8）与中继每 IP 每分钟限流（60，留余量）。app.go 装配时注入 Service；newReceiveClient
// 未收到注入时（单测直构 Service 漏覆写）回落此单例，门控在装配遗漏下仍生效。测试需经
// sessionRuntimeOptions.dialCoordinator 覆写无限制协调器（0/0）绕过，避免多次拨号被默认速率门控限流。
var DefaultDialCoordinator = NewDialCoordinator(8, 50)

// AcquireSlot 取并发流槽：阻塞至有空槽（ctx 可取消）；槽覆盖整条流的下载期。
func (c *DialCoordinator) AcquireSlot(ctx context.Context) error {
	if c.slots == nil {
		return nil // 不限并发
	}
	start := time.Now()
	select {
	case c.slots <- struct{}{}:
		if waited := time.Since(start); waited > slotBlockLogThreshold {
			logger.Log.Debugf("[share-recv] 并发流槽取得（等待%s，池活跃=%d/%d）", waited, len(c.slots), cap(c.slots))
		}
		return nil
	case <-ctx.Done():
		if waited := time.Since(start); waited > 0 {
			logger.Log.Debugf("[share-recv] 并发流槽等待被取消 已等%s err=%v", waited, ctx.Err())
		}
		return ctx.Err()
	}
}

// ReleaseSlot 释放并发流槽（流收尾时调用，幂等）。
func (c *DialCoordinator) ReleaseSlot() {
	if c.slots == nil {
		return
	}
	// 非阻塞接收：空槽时的多余释放直接忽略，不吞正常令牌。
	select {
	case <-c.slots:
		logger.Log.Debugf("[share-recv] 并发流槽归还 池活跃=%d/%d", len(c.slots), cap(c.slots))
	default:
	}
}

// SetQuotaFullCallback 设置配额满通知回调（进程装配用）：TakeDial 进入阻塞态时触发一次。
// 触发不经协调器锁（回调内若再取门控不构成持锁重入）；nil=关闭通知。
func (c *DialCoordinator) SetQuotaFullCallback(fn func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.quotaFullCb = fn
}

// TakeDial 取拨号令牌：滑动窗口速率门控，无配额则等待至滑出窗口（ctx 可取消）。每次拨号前调用。
func (c *DialCoordinator) TakeDial(ctx context.Context) error {
	if c.dialCap <= 0 {
		return nil // 不限速率
	}
	var blockedStart time.Time
	var quotaFullCb func()
	for {
		c.mu.Lock()
		now := time.Now()
		c.pruneDialTimes(now)
		if len(c.dialTimes) < c.dialCap {
			c.dialTimes = append(c.dialTimes, now)
			if !blockedStart.IsZero() {
				logger.Log.Debugf("[share-recv] 拨号速率令牌取得（此前等待%s，窗口=%d/%d）", time.Since(blockedStart), len(c.dialTimes), c.dialCap)
			}
			c.mu.Unlock()
			return nil
		}
		// 配额已满：等最老事件滑出窗口，按复查周期醒来重算。
		if blockedStart.IsZero() {
			blockedStart = time.Now()
			logger.Log.Debugf("[share-recv] 拨号速率配额满 等待窗口滑出（窗口=%d/%d，最老事件%v后滑出）",
				len(c.dialTimes), c.dialCap, c.dialTimes[0].Add(dialWindow).Sub(now))
			quotaFullCb = c.quotaFullCb // 捕获回调，解锁后触发（防持锁重入）
		}
		wait := c.dialTimes[0].Add(dialWindow).Sub(now)
		c.mu.Unlock()
		// 本次调用首次进入阻塞态：触发配额满通知（仅一次）
		if quotaFullCb != nil {
			cb := quotaFullCb
			quotaFullCb = nil
			cb()
		}
		if wait > 0 {
			if wait > dialPollInterval {
				wait = dialPollInterval
			}
			timer := time.NewTimer(wait)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			}
		}
	}
}

// pruneDialTimes 剔除已滑出窗口的拨号事件（now >= 事件时刻 + 窗口）。
func (c *DialCoordinator) pruneDialTimes(now time.Time) {
	cutoff := now.Add(-dialWindow)
	i := 0
	for i < len(c.dialTimes) && !c.dialTimes[i].After(cutoff) {
		i++
	}
	if i > 0 {
		c.dialTimes = c.dialTimes[i:]
	}
}
