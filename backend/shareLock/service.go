// Package shareLock 分享拉取中的作品锁注册中心（进程内内存态能力包）。
// 只记作品 ID 与会话引用两层键，不感知任何业务领域语义；进程退出状态即消失，无持久化。
package shareLock

import (
	"context"
	"errors"
	"sync"
)

// ErrWorkLocked 作品正被分享拉取持有——触碰该作品活行 store 文件的操作（替换软删、
// 回收站复原置换、覆盖转移等）被拒时返回本哨兵错误，errors.Is 可判；
// 用户知情强制放行走 ForceUnlock 后重试原操作
var ErrWorkLocked = errors.New("该作品正在被分享拉取中")

// ShareLockRegistry 作品锁注册中心（内存态，跨重启自动解除）。
// Register 会话开始拉取某批作品时登记（记 workID + 会话引用）；Unregister 会话结束/停止时
// 解除本会话引用（其他会话仍持有时锁不释放）；IsLocked 供替换/软删/移动等触碰作品的操作前查询；
// ForceUnlock 清除某作品的全部会话引用（用户知情的强制放行）。
// session 为调用方自定义的会话标识（对注册中心不透明），同一 (workID, session) 重复登记幂等。
type ShareLockRegistry interface {
	Register(ctx context.Context, workIDs []int64, session string)
	Unregister(ctx context.Context, workIDs []int64, session string)
	IsLocked(ctx context.Context, workID int64) bool
	ForceUnlock(ctx context.Context, workID int64)
}

// registry ShareLockRegistry 的内存实现：workID → 持有该作品的会话集合。
// 不变式：holders 中存在的键其会话集合必非空（Unregister 清空集合时同步摘键）。
type registry struct {
	mu      sync.RWMutex
	holders map[int64]map[string]struct{}
}

// NewShareLockRegistry 创建空注册中心
func NewShareLockRegistry() ShareLockRegistry {
	return &registry{holders: make(map[int64]map[string]struct{})}
}

// Register 登记会话对一批作品的持有引用。workIDs 为空时无副作用。
func (r *registry) Register(ctx context.Context, workIDs []int64, session string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, id := range workIDs {
		set, ok := r.holders[id]
		if !ok {
			set = make(map[string]struct{})
			r.holders[id] = set
		}
		set[session] = struct{}{}
	}
}

// Unregister 解除会话对一批作品的持有引用；未登记的 (workID, session) 组合静默忽略（幂等）。
// 会话集合清空时摘除该 workID 键——其他会话仍持有时作品保持锁定。
func (r *registry) Unregister(ctx context.Context, workIDs []int64, session string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, id := range workIDs {
		set, ok := r.holders[id]
		if !ok {
			continue
		}
		delete(set, session)
		if len(set) == 0 {
			delete(r.holders, id)
		}
	}
}

// IsLocked 查询作品是否仍被任一会话持有
func (r *registry) IsLocked(ctx context.Context, workID int64) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, locked := r.holders[workID]
	return locked
}

// ForceUnlock 清除作品的全部会话引用（强制放行；未登记的作品同样静默忽略）
func (r *registry) ForceUnlock(ctx context.Context, workID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.holders, workID)
}
